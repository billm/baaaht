// agent.ts - Main Assistant agent implementation
//
// This file contains the Agent class which handles message processing,
// LLM communication, session management, and tool delegation.
//
// Copyright 2026 baaaht project

import { EventEmitter } from 'events';
import type { Client } from '@grpc/grpc-js';
import type {
  AgentMessage,
} from './proto/agent.js';
import { AgentState, MessageType } from './proto/agent.js';
import { StreamAgentClient, StreamEventType } from './orchestrator/stream-client.js';
import type {
  LLMRequest,
  LLMMessage as LLMMsg,
  LLMResponse,
  ToolCall,
  FinishReason,
} from './proto/llm.js';
import type {
  Session,
  SessionMetadata,
  Message as SessionMessage,
  MessageRole,
} from './session/types.js';
import type {
  CompletionParams,
  CompletionResult,
  StreamingChunk,
} from './llm/types.js';
import type {
  ToolDefinition,
  ToolResult,
  DelegateParams,
} from './tools/types.js';
import type {
  AgentConfig,
  AgentStatus,
  ProcessMessage,
  AgentResponse,
  AgentError,
  AgentErrorCode,
  ToolCallInfo,
  MessageProcessResult,
  AgentDependencies,
  AgentEventType,
  ResponseMetadata,
  ToolExecutionResult,
  MessageMetadata,
} from './agent/types.js';
import { LLMGatewayClient } from './llm/gateway-client.js';
import { SessionManager } from './session/manager.js';
import { createDelegateToolDefinition } from './tools/delegate.js';
import { WorkerDelegation } from './tools/worker-delegation.js';
import { ResearcherDelegation } from './tools/researcher-delegation.js';
import { CoderDelegation } from './tools/coder-delegation.js';
import { DelegateTarget } from './tools/types.js';

// =============================================================================
// Default Configuration
// =============================================================================

const DEFAULT_AGENT_CONFIG: Required<Omit<AgentConfig, 'labels' | 'debug' | 'enableStreaming'>> & {
  labels: Record<string, string>;
  debug: boolean;
  enableStreaming: boolean;
} = {
  name: 'assistant',
  description: 'Primary conversational agent with tool delegation',
  orchestratorUrl: 'localhost:50051',
  llmGatewayUrl: 'http://localhost:8080',
  defaultModel: 'anthropic/claude-sonnet-4-20250514',
  maxConcurrentMessages: 5,
  messageTimeout: 120000, // 2 minutes
  heartbeatInterval: 30000, // 30 seconds
  maxRetries: 3,
  sessionTimeout: 3600000, // 1 hour
  maxSessionMessages: 100,
  contextWindowSize: 200000,
  labels: {},
  enableStreaming: true,
  debug: false,
};

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Generate a unique message ID
 */
function generateMessageId(): string {
  return `msg_${Date.now()}_${Math.random().toString(36).substring(2, 11)}`;
}

/**
 * Generate a unique response ID
 */
function generateResponseId(): string {
  return `resp_${Date.now()}_${Math.random().toString(36).substring(2, 11)}`;
}

/**
 * Create an AgentError
 */
function createAgentError(
  message: string,
  code: AgentErrorCode,
  retryable: boolean = false,
  details?: Record<string, unknown>
): AgentError {
  const error = new Error(message) as AgentError;
  error.code = code;
  error.retryable = retryable;
  error.details = details;
  error.name = 'AgentError';
  return error;
}

/**
 * Deep sleep utility
 */
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// =============================================================================
// Main Agent Class
// =============================================================================

/**
 * Agent handles message processing with LLM integration and tool delegation
 *
 * This is the core of the Assistant agent that:
 * - Receives messages from the Orchestrator
 * - Manages session context
 * - Communicates with the LLM Gateway
 * - Handles tool calling via delegation
 * - Streams responses back to the user
 */
export class Agent extends EventEmitter {
  private config: typeof DEFAULT_AGENT_CONFIG;
  private dependencies: AgentDependencies;
  private llmClient: LLMGatewayClient;
  private sessionManager: SessionManager;

  // Agent state
  private agentId: string;
  private state: AgentState;
  private registered: boolean;
  private ready: boolean;
  private startedAt: Date;
  private lastActivityAt: Date;

  // Processing state
  private processingMessages: Map<string, ProcessMessage>;
  private messageQueue: ProcessMessage[];
  private activeMessageCount: number;
  private totalMessagesProcessed: number;
  private totalToolCalls: number;

  // Stream communication
  private streamClient: StreamAgentClient | null;

  // Shutdown state
  private isShutdown: boolean;
  private shutdownPromise: Promise<void> | null;

  /**
   * Creates a new Agent instance
   *
   * @param config - Agent configuration
   * @param dependencies - Agent dependencies
   */
  constructor(config: AgentConfig = {}, dependencies: AgentDependencies) {
    super();

    // Merge config with defaults
    this.config = {
      ...DEFAULT_AGENT_CONFIG,
      ...config,
      labels: { ...DEFAULT_AGENT_CONFIG.labels, ...config.labels },
    };

    this.dependencies = dependencies;

    // Create LLM Gateway client
    this.llmClient = new LLMGatewayClient({
      baseURL: this.config.llmGatewayUrl,
      agentId: '', // Will be set after registration
    });

    // Create session manager
    this.sessionManager = dependencies.sessionManager ?? new SessionManager({
      maxSessions: 100,
      idleTimeout: this.config.sessionTimeout,
      timeout: this.config.sessionTimeout,
      maxDuration: this.config.sessionTimeout,
    });

    // Initialize state
    this.agentId = '';
    this.state = AgentState.AGENT_STATE_INITIALIZING;
    this.registered = false;
    this.ready = false;
    this.startedAt = new Date();
    this.lastActivityAt = new Date();

    this.processingMessages = new Map();
    this.messageQueue = [];
    this.activeMessageCount = 0;
    this.totalMessagesProcessed = 0;
    this.totalToolCalls = 0;

    this.streamClient = null;

    this.isShutdown = false;
    this.shutdownPromise = null;

    // Set up error handlers
    this.setupErrorHandlers();
  }

  // ==========================================================================
  // Lifecycle Methods
  // ==========================================================================

  /**
   * Initialize the agent
   *
   * @throws AgentError if initialization fails
   */
  async initialize(): Promise<void> {
    if (this.state !== AgentState.AGENT_STATE_INITIALIZING) {
      throw createAgentError(
        'Agent already initialized',
        AgentErrorCode.INIT_FAILED
      );
    }

    try {
      this.emit(AgentEventType.INITIALIZED, {
        type: AgentEventType.INITIALIZED,
        timestamp: new Date(),
        data: { config: this.config },
      });

      this.state = AgentState.AGENT_STATE_REGISTERING;
    } catch (err) {
      const error = err as Error;
      throw createAgentError(
        `Agent initialization failed: ${error.message}`,
        AgentErrorCode.INIT_FAILED,
        false,
        { originalError: error.message }
      );
    }
  }

  /**
   * Register the agent with the orchestrator
   *
   * @param agentId - Agent ID assigned by orchestrator
   * @throws AgentError if registration fails
   */
  async register(agentId: string): Promise<void> {
    if (this.state !== AgentState.AGENT_STATE_REGISTERING) {
      throw createAgentError(
        'Agent not in registering state',
        AgentErrorCode.REGISTRATION_FAILED
      );
    }

    try {
      this.agentId = agentId;
      this.llmClient = new LLMGatewayClient({
        baseURL: this.config.llmGatewayUrl,
        agentId: this.agentId,
      });

      this.streamClient = new StreamAgentClient(this.dependencies.grpcClient, this.agentId);
      this.streamClient.on(StreamEventType.MESSAGE, (event: { data?: unknown }) => {
        void this.handleOrchestratorStreamMessage(event.data as AgentMessage);
      });
      this.streamClient.on(StreamEventType.ERROR, (event: { error?: Error }) => {
        if (event.error) {
          this.emit(AgentEventType.ERROR, {
            type: AgentEventType.ERROR,
            timestamp: new Date(),
            error: event.error,
          });
        }
      });
      await this.streamClient.connect();

      this.registered = true;
      this.state = AgentState.AGENT_STATE_IDLE;
      this.ready = true;

      this.emit(AgentEventType.REGISTERED, {
        type: AgentEventType.REGISTERED,
        timestamp: new Date(),
        data: { agentId: this.agentId },
      });

      this.emit(AgentEventType.READY, {
        type: AgentEventType.READY,
        timestamp: new Date(),
        data: { agentId: this.agentId },
      });

      if (this.config.debug) {
        this.debug(`Agent registered with ID: ${this.agentId}`);
      }
    } catch (err) {
      const error = err as Error;
      throw createAgentError(
        `Agent registration failed: ${error.message}`,
        AgentErrorCode.REGISTRATION_FAILED,
        false,
        { originalError: error.message }
      );
    }
  }

  /**
   * Unregister the agent from the orchestrator
   */
  async unregister(): Promise<void> {
    if (!this.registered) {
      return;
    }

    this.registered = false;
    this.ready = false;
    this.state = AgentState.AGENT_STATE_UNREGISTERING;

    if (this.streamClient) {
      await this.streamClient.close();
      this.streamClient = null;
    }

    // Wait for active messages to complete
    while (this.activeMessageCount > 0) {
      this.debug('Waiting for active messages to complete...');
      await sleep(100);
    }

    this.emit(AgentEventType.UNREGISTERED, {
      type: AgentEventType.UNREGISTERED,
      timestamp: new Date(),
      data: { agentId: this.agentId },
    });
  }

  /**
   * Start the message processing loop
   */
  start(): void {
    if (this.isShutdown) {
      throw createAgentError(
        'Cannot start: agent is shutdown',
        AgentErrorCode.SHUTDOWN_IN_PROGRESS
      );
    }

    this.processMessagesLoop().catch((error) => {
      this.emit(AgentEventType.ERROR, {
        type: AgentEventType.ERROR,
        timestamp: new Date(),
        error,
      });
    });
  }

  /**
   * Shutdown the agent gracefully
   */
  async shutdown(): Promise<void> {
    if (this.isShutdown) {
      return;
    }

    this.isShutdown = true;
    this.state = AgentState.AGENT_STATE_TERMINATED;

    // Unregister from orchestrator
    await this.unregister();

    // Close session manager
    await this.sessionManager.close();

    // Remove all listeners
    this.removeAllListeners();

    this.emit(AgentEventType.SHUTDOWN, {
      type: AgentEventType.SHUTDOWN,
      timestamp: new Date(),
      data: { agentId: this.agentId },
    });

    if (this.config.debug) {
      this.debug('Agent shutdown complete');
    }
  }

  // ==========================================================================
  // Message Processing
  // ==========================================================================

  /**
   * Receive a message from the orchestrator
   *
   * @param message - Agent message from orchestrator
   * @param respond - Callback to send response
   */
  async receiveMessage(
    message: AgentMessage,
    respond: (response: AgentResponse) => void | Promise<void>
  ): Promise<void> {
    if (this.isShutdown) {
      throw createAgentError(
        'Cannot receive message: agent is shutdown',
        AgentErrorCode.SHUTDOWN_IN_PROGRESS
      );
    }

    if (!this.ready) {
      throw createAgentError(
        'Cannot receive message: agent not ready',
        AgentErrorCode.NOT_READY
      );
    }

    // Update activity timestamp
    this.lastActivityAt = new Date();

    const dataMessage = message.payload && 'dataMessage' in message.payload
      ? message.payload.dataMessage
      : undefined;
    const content = dataMessage?.data ? new TextDecoder().decode(dataMessage.data) : '';
    const sessionId = message.metadata?.sessionId ?? '';
    const correlationId = message.metadata?.correlationId ?? message.id ?? generateMessageId();

    if (!content) {
      throw createAgentError(
        'Message content is empty',
        AgentErrorCode.MESSAGE_INVALID
      );
    }

    if (!sessionId) {
      throw createAgentError(
        'Session ID is required',
        AgentErrorCode.MESSAGE_INVALID
      );
    }

    // Create process message
    const processMessage: ProcessMessage = {
      id: generateMessageId(),
      sessionId,
      content,
      metadata: {
        correlationId,
        source: message.type?.toString() ?? 'unknown',
        originalMessage: message,
      },
      receivedAt: new Date(),
      respond,
    };

    // Add to queue
    this.messageQueue.push(processMessage);

    this.emit(AgentEventType.MESSAGE_RECEIVED, {
      type: AgentEventType.MESSAGE_RECEIVED,
      timestamp: new Date(),
      data: {
        messageId: processMessage.id,
        sessionId,
        correlationId,
      },
    });

    if (this.config.debug) {
      this.debug(`Message received: ${processMessage.id} (session: ${sessionId})`);
    }
  }

  private async handleOrchestratorStreamMessage(message: AgentMessage): Promise<void> {
    if (!message || !message.payload || !('dataMessage' in message.payload)) {
      return;
    }

    if (message.sourceId === this.agentId) {
      return;
    }

    if (message.sourceId && message.sourceId !== 'orchestrator') {
      return;
    }

    if (message.targetId && message.targetId !== this.agentId) {
      return;
    }

    const sessionId = message.metadata?.sessionId ?? '';
    const correlationId = message.metadata?.correlationId ?? message.id ?? generateMessageId();
    if (!sessionId) {
      this.emit(AgentEventType.ERROR, {
        type: AgentEventType.ERROR,
        timestamp: new Date(),
        error: createAgentError('Missing session ID in stream message', AgentErrorCode.MESSAGE_INVALID),
      });
      return;
    }

    await this.receiveMessage(message, async (response: AgentResponse) => {
      await this.sendResponseViaStream(sessionId, correlationId, response);
    });
  }

  private async sendResponseViaStream(
    sessionId: string,
    correlationId: string,
    response: AgentResponse
  ): Promise<void> {
    if (!this.streamClient || !this.streamClient.isConnected()) {
      throw createAgentError('Stream client not connected', AgentErrorCode.ORCHESTRATOR_CONNECTION_FAILED, true);
    }

    const responseContent = response.error
      ? response.error.message
      : (response.content ?? '');

    const responseMessage: AgentMessage = {
      id: generateMessageId(),
      type: response.error ? MessageType.MESSAGE_TYPE_ERROR : MessageType.MESSAGE_TYPE_DATA,
      timestamp: new Date(),
      sourceId: this.agentId,
      targetId: 'orchestrator',
      payload: {
        dataMessage: {
          contentType: response.error ? 'application/vnd.baaaht.conversation.error+text' : 'text/plain',
          data: new TextEncoder().encode(responseContent),
          headers: {
            session_id: sessionId,
            correlation_id: correlationId,
          },
        },
      },
      metadata: {
        sessionId,
        correlationId,
      },
    };

    await this.streamClient.sendMessage(responseMessage);
  }

  /**
   * Message processing loop
   *
   * @private
   */
  private async processMessagesLoop(): Promise<void> {
    while (!this.isShutdown) {
      try {
        // Check if we can process more messages
        if (
          this.messageQueue.length === 0 ||
          this.activeMessageCount >= this.config.maxConcurrentMessages
        ) {
          await sleep(10);
          continue;
        }

        // Get next message from queue
        const processMessage = this.messageQueue.shift();
        if (!processMessage) {
          continue;
        }

        // Process the message
        this.processMessage(processMessage).catch((error) => {
          this.emit(AgentEventType.ERROR, {
            type: AgentEventType.ERROR,
            timestamp: new Date(),
            error,
            data: { messageId: processMessage.id },
          });
        });
      } catch (err) {
        const error = err as Error;
        this.emit(AgentEventType.ERROR, {
          type: AgentEventType.ERROR,
          timestamp: new Date(),
          error,
        });
      }
    }
  }

  /**
   * Process a single message
   *
   * @private
   */
  private async processMessage(processMessage: ProcessMessage): Promise<void> {
    const { id, sessionId, content, metadata, respond } = processMessage;
    const startTime = Date.now();

    this.activeMessageCount++;
    this.processingMessages.set(id, processMessage);

    this.emit(AgentEventType.MESSAGE_PROCESSING, {
      type: AgentEventType.MESSAGE_PROCESSING,
      timestamp: new Date(),
      data: { messageId: id, sessionId },
    });

    try {
      // Ensure session exists
      await this.ensureSession(sessionId);

      // Add user message to session
      await this.sessionManager.addMessage(sessionId, {
        id: metadata?.correlationId ?? generateMessageId(),
        timestamp: new Date(),
        role: MessageRole.USER,
        content,
        metadata: metadata?.labels,
      });

      // Get session context
      const session = await this.sessionManager.get(sessionId);
      const messages = session.context.messages ?? [];

      // Build LLM request
      const llmMessages: LLMMsg[] = messages.map((m) => ({
        role: m.role,
        content: m.content,
      }));

      const completionParams: CompletionParams = {
        model: this.config.defaultModel,
        messages: llmMessages,
        maxTokens: 4096,
        temperature: 0.7,
        sessionId,
        tools: [createDelegateToolDefinition()],
      };

      // Process with LLM
      const result = await this.processWithLLM(id, sessionId, completionParams);

      // Add assistant response to session
      if (result.content) {
        await this.sessionManager.addMessage(sessionId, {
          id: generateMessageId(),
          timestamp: new Date(),
          role: MessageRole.ASSISTANT,
          content: result.content,
          metadata: {
            toolCalls: result.toolCalls?.map((tc) => ({
              id: tc.id,
              name: tc.name,
              success: tc.success,
            })),
          },
        });
      }

      // Build response
      const durationMs = Date.now() - startTime;
      const response: AgentResponse = {
        content: result.content,
        streamed: result.streamed,
        toolCalls: result.toolCalls,
        usage: result.usage,
        finishReason: result.finishReason,
        metadata: {
          responseId: generateResponseId(),
          sessionId,
          requestId: id,
          timestamp: new Date(),
          processingDurationMs: durationMs,
          model: this.config.defaultModel,
          provider: 'unknown',
        },
      };

      // Send response
      await respond(response);

      this.totalMessagesProcessed++;
      this.emit(AgentEventType.MESSAGE_PROCESSED, {
        type: AgentEventType.MESSAGE_PROCESSED,
        timestamp: new Date(),
        data: {
          messageId: id,
          sessionId,
          durationMs,
          toolCallCount: result.toolCalls?.length ?? 0,
        },
      });

      if (this.config.debug) {
        this.debug(
          `Message processed: ${id} (${durationMs}ms, ${result.toolCalls?.length ?? 0} tool calls)`
        );
      }
    } catch (err) {
      const error = err as Error;
      const agentError =
        error instanceof Error && 'code' in error
          ? (error as AgentError)
          : createAgentError(
              error.message,
              AgentErrorCode.MESSAGE_PROCESSING_FAILED,
              false
            );

      // Send error response
      const response: AgentResponse = {
        content: '',
        error: agentError,
      };

      try {
        await respond(response);
      } catch {
        // Ignore response errors
      }

      this.emit(AgentEventType.MESSAGE_FAILED, {
        type: AgentEventType.MESSAGE_FAILED,
        timestamp: new Date(),
        error: agentError,
        data: { messageId: id, sessionId },
      });

      if (this.config.debug) {
        this.debug(`Message failed: ${id} - ${agentError.message}`);
      }
    } finally {
      this.activeMessageCount--;
      this.processingMessages.delete(id);
    }
  }

  /**
   * Process a message with the LLM
   *
   * @private
   */
  private async processWithLLM(
    messageId: string,
    sessionId: string,
    params: CompletionParams
  ): Promise<MessageProcessResult> {
    const startTime = Date.now();

    try {
      this.emit(AgentEventType.LLM_REQUEST_START, {
        type: AgentEventType.LLM_REQUEST_START,
        timestamp: new Date(),
        data: { messageId, sessionId },
      });

      let content = '';
      let toolCalls: ToolCallInfo[] = [];
      let usage;
      let finishReason: FinishReason | undefined;
      let streamed = false;

      if (this.config.enableStreaming) {
        // Streaming completion
        const stream = this.llmClient.stream(params);

        this.emit(AgentEventType.LLM_STREAM_START, {
          type: AgentEventType.LLM_STREAM_START,
          timestamp: new Date(),
          data: { messageId, sessionId },
        });

        for await (const chunk of stream) {
          if (this.isShutdown) {
            stream.abort();
            throw createAgentError(
              'Agent shutdown during LLM stream',
              AgentErrorCode.LLM_STREAM_FAILED
            );
          }

          if (chunk.type === 'content') {
            content += chunk.data.content ?? '';
            this.emit(AgentEventType.LLM_STREAM_CHUNK, {
              type: AgentEventType.LLM_STREAM_CHUNK,
              timestamp: new Date(),
              data: { messageId, chunk },
            });
          } else if (chunk.type === 'toolCall' && chunk.data) {
            // Handle tool call
            const toolCall = chunk.data;
            const toolResult = await this.executeToolCall(
              messageId,
              sessionId,
              toolCall
            );

            toolCalls.push(toolResult);
          } else if (chunk.type === 'usage') {
            usage = chunk.data;
          } else if (chunk.type === 'complete') {
            finishReason = chunk.data.finishReason;
          } else if (chunk.type === 'error') {
            throw createAgentError(
              chunk.data.message ?? 'LLM stream error',
              AgentErrorCode.LLM_STREAM_FAILED,
              true
            );
          }
        }

        streamed = true;

        this.emit(AgentEventType.LLM_STREAM_END, {
          type: AgentEventType.LLM_STREAM_END,
          timestamp: new Date(),
          data: { messageId, sessionId, finishReason },
        });
      } else {
        // Non-streaming completion
        const result: CompletionResult = await this.llmClient.complete(params);

        content = result.content ?? '';
        usage = result.usage;
        finishReason = result.finishReason;

        // Handle tool calls
        if (result.toolCalls && result.toolCalls.length > 0) {
          for (const toolCall of result.toolCalls) {
            const toolResult = await this.executeToolCall(
              messageId,
              sessionId,
              toolCall
            );
            toolCalls.push(toolResult);
          }
        }

        this.emit(AgentEventType.LLM_REQUEST_COMPLETE, {
          type: AgentEventType.LLM_REQUEST_COMPLETE,
          timestamp: new Date(),
          data: { messageId, sessionId, finishReason },
        });
      }

      const durationMs = Date.now() - startTime;

      return {
        content,
        toolCalls,
        usage,
        finishReason,
        durationMs,
        success: true,
        streamed,
      };
    } catch (err) {
      const error = err as Error;
      this.emit(AgentEventType.LLM_ERROR, {
        type: AgentEventType.LLM_ERROR,
        timestamp: new Date(),
        error,
        data: { messageId, sessionId },
      });

      throw createAgentError(
        `LLM processing failed: ${error.message}`,
        AgentErrorCode.LLM_REQUEST_FAILED,
        true,
        { originalError: error.message }
      );
    }
  }

  /**
   * Execute a tool call
   *
   * @private
   */
  private async executeToolCall(
    messageId: string,
    sessionId: string,
    toolCall: ToolCall
  ): Promise<ToolCallInfo> {
    const startTime = Date.now();

    this.emit(AgentEventType.TOOL_CALL_START, {
      type: AgentEventType.TOOL_CALL_START,
      timestamp: new Date(),
      data: { messageId, sessionId, toolName: toolCall.name },
    });

    this.totalToolCalls++;

    try {
      const toolName = toolCall.name ?? '';
      let result: ToolResult;

      if (toolName === 'delegate') {
        // Parse delegate parameters
        const params = JSON.parse(toolCall.arguments ?? '{}') as DelegateParams;

        // Execute delegation
        result = await this.executeDelegation(messageId, sessionId, params);
      } else {
        throw createAgentError(
          `Unknown tool: ${toolName}`,
          AgentErrorCode.TOOL_NOT_AVAILABLE
        );
      }

      const durationMs = Date.now() - startTime;

      const toolCallInfo: ToolCallInfo = {
        id: toolCall.id ?? '',
        name: toolName,
        arguments: JSON.parse(toolCall.arguments ?? '{}'),
        result,
        success: result.success,
        durationMs,
        timestamp: new Date(),
      };

      this.emit(AgentEventType.TOOL_CALL_COMPLETE, {
        type: AgentEventType.TOOL_CALL_COMPLETE,
        timestamp: new Date(),
        data: { messageId, sessionId, toolName, durationMs, success: true },
      });

      return toolCallInfo;
    } catch (err) {
      const error = err as Error;
      const durationMs = Date.now() - startTime;

      const toolCallInfo: ToolCallInfo = {
        id: toolCall.id ?? '',
        name: toolCall.name ?? '',
        arguments: JSON.parse(toolCall.arguments ?? '{}'),
        success: false,
        durationMs,
        timestamp: new Date(),
        result: {
          success: false,
          error: error.message,
        },
      };

      this.emit(AgentEventType.TOOL_CALL_FAILED, {
        type: AgentEventType.TOOL_CALL_FAILED,
        timestamp: new Date(),
        error,
        data: {
          messageId,
          sessionId,
          toolName: toolCall.name,
          durationMs,
        },
      });

      return toolCallInfo;
    }
  }

  /**
   * Execute a delegation to another agent
   *
   * @private
   */
  private async executeDelegation(
    messageId: string,
    sessionId: string,
    params: DelegateParams
  ): Promise<ToolResult> {
    const startTime = Date.now();

    this.emit(AgentEventType.DELEGATION_START, {
      type: AgentEventType.DELEGATION_START,
      timestamp: new Date(),
      data: { messageId, sessionId, target: params.target, operation: params.operation },
    });

    try {
      let result: ToolResult;

      switch (params.target) {
        case DelegateTarget.WORKER: {
          const delegation = new WorkerDelegation(this.dependencies.grpcClient);
          const delegateResult = await delegation.delegate(params, sessionId);
          result = {
            success: delegateResult.success,
            data: delegateResult.data,
            error: delegateResult.error,
            output: delegateResult.output,
            metadata: delegateResult.metadata,
          };
          break;
        }
        case DelegateTarget.RESEARCHER: {
          const delegation = new ResearcherDelegation(this.dependencies.grpcClient);
          const delegateResult = await delegation.delegate(params, sessionId);
          result = {
            success: delegateResult.success,
            data: delegateResult.data,
            error: delegateResult.error,
            output: delegateResult.output,
            metadata: delegateResult.metadata,
          };
          break;
        }
        case DelegateTarget.CODER: {
          const delegation = new CoderDelegation(this.dependencies.grpcClient);
          const delegateResult = await delegation.delegate(params, sessionId);
          result = {
            success: delegateResult.success,
            data: delegateResult.data,
            error: delegateResult.error,
            output: delegateResult.output,
            metadata: delegateResult.metadata,
          };
          break;
        }
        default:
          throw createAgentError(
            `Unknown delegate target: ${params.target}`,
            AgentErrorCode.TOOL_NOT_AVAILABLE
          );
      }

      const durationMs = Date.now() - startTime;

      this.emit(AgentEventType.DELEGATION_COMPLETE, {
        type: AgentEventType.DELEGATION_COMPLETE,
        timestamp: new Date(),
        data: { messageId, sessionId, target: params.target, durationMs, success: result.success },
      });

      return result;
    } catch (err) {
      const error = err as Error;
      const durationMs = Date.now() - startTime;

      this.emit(AgentEventType.DELEGATION_FAILED, {
        type: AgentEventType.DELEGATION_FAILED,
        timestamp: new Date(),
        error,
        data: { messageId, sessionId, target: params.target, durationMs },
      });

      throw createAgentError(
        `Delegation failed: ${error.message}`,
        AgentErrorCode.TOOL_EXECUTION_FAILED,
        false,
        { target: params.target, operation: params.operation }
      );
    }
  }

  /**
   * Ensure a session exists for the given session ID
   *
   * @private
   */
  private async ensureSession(sessionId: string): Promise<void> {
    try {
      await this.sessionManager.get(sessionId);
    } catch {
      // Session doesn't exist, create it
      const metadata: SessionMetadata = {
        name: `Session ${sessionId}`,
        ownerId: this.agentId,
      };

      await this.sessionManager.create(metadata, {
        maxDuration: this.config.sessionTimeout,
      });

      this.emit(AgentEventType.SESSION_CREATED, {
        type: AgentEventType.SESSION_CREATED,
        timestamp: new Date(),
        data: { sessionId },
      });

      if (this.config.debug) {
        this.debug(`Session created: ${sessionId}`);
      }
    }
  }

  // ==========================================================================
  // Status and Metrics
  // ==========================================================================

  /**
   * Get the current agent status
   */
  getStatus(): AgentStatus {
    const uptimeSeconds = Math.floor(
      (Date.now() - this.startedAt.getTime()) / 1000
    );

    return {
      agentId: this.agentId,
      state: this.state,
      activeSessions: this.sessionManager.getStats().active,
      processingMessages: this.activeMessageCount,
      totalMessagesProcessed: this.totalMessagesProcessed,
      totalToolCalls: this.totalToolCalls,
      lastActivityAt: this.lastActivityAt,
      uptimeSeconds,
    };
  }

  // ==========================================================================
  // Private Helper Methods
  // ==========================================================================

  /**
   * Set up error handlers
   *
   * @private
   */
  private setupErrorHandlers(): void {
    this.on('error', (error) => {
      // Log errors but don't crash
      if (this.config.debug) {
        this.debug(`Agent error: ${error.message}`);
      }
    });

    this.on(AgentEventType.MESSAGE_FAILED, (event) => {
      if (this.config.debug) {
        this.debug(
          `Message failed: ${(event.data as { messageId: string }).messageId}`
        );
      }
    });

    this.on(AgentEventType.TOOL_CALL_FAILED, (event) => {
      if (this.config.debug) {
        this.debug(
          `Tool call failed: ${(event.data as { toolName: string }).toolName}`
        );
      }
    });
  }

  /**
   * Debug logging
   *
   * @private
   */
  private debug(message: string): void {
    if (this.config.debug) {
      console.log(`[Agent:${this.agentId || 'init'}] ${message}`);
    }
  }
}

// =============================================================================
// Factory Functions
// =============================================================================

/**
 * Create a new Agent instance
 *
 * @param config - Agent configuration
 * @param dependencies - Agent dependencies
 * @returns Configured Agent instance
 */
export function createAgent(
  config: AgentConfig = {},
  dependencies: AgentDependencies
): Agent {
  return new Agent(config, dependencies);
}

// =============================================================================
// Re-exports
// =============================================================================

export * from './agent/types.js';
