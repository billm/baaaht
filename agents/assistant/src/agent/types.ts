// types.ts - Type definitions for the Assistant agent
//
// This file contains TypeScript types for the Agent class which handles
// message processing, LLM communication, and tool delegation.
//
// Copyright 2026 baaaht project

import { EventEmitter } from 'events';
import type { Client } from '@grpc/grpc-js';
import type {
} from '../session/types.js';
import type {
  AgentMessage,
  AgentState,
} from '../proto/agent.js';
import type {
  ToolCall,
  TokenUsage,
  FinishReason,
} from '../proto/llm.js';
import type { CompletionParams } from '../llm/types.js';
import type { ToolDefinition, ToolResult } from '../tools/types.js';

// =============================================================================
// Agent Configuration
// =============================================================================

/**
 * Configuration for the Assistant agent
 */
export interface AgentConfig {
  /**
   * Agent name for identification
   */
  name?: string;

  /**
   * Agent description
   */
  description?: string;

  /**
   * Orchestrator connection URL
   */
  orchestratorUrl?: string;

  /**
   * Default LLM model to use
   */
  defaultModel?: string;

  /**
   * Default LLM provider to use
   */
  defaultProvider?: string;

  /**
   * Maximum concurrent message processing
   */
  maxConcurrentMessages?: number;

  /**
   * Message processing timeout in milliseconds
   */
  messageTimeout?: number;

  /**
   * Heartbeat interval in milliseconds
   */
  heartbeatInterval?: number;

  /**
   * Maximum retries for failed operations
   */
  maxRetries?: number;

  /**
   * Session timeout in milliseconds
   */
  sessionTimeout?: number;

  /**
   * Maximum number of messages per session context
   */
  maxSessionMessages?: number;

  /**
   * Context window size in tokens
   */
  contextWindowSize?: number;

  /**
   * Additional metadata labels
   */
  labels?: Record<string, string>;

  /**
   * Enable streaming responses
   */
  enableStreaming?: boolean;

  /**
   * Debug mode for verbose logging
   */
  debug?: boolean;
}

// =============================================================================
// Agent State
// =============================================================================

/**
 * Current state of the agent
 */
export interface AgentStatus {
  /**
   * Agent ID assigned by orchestrator
   */
  agentId: string;

  /**
   * Current agent state
   */
  state: AgentState;

  /**
   * Number of active sessions
   */
  activeSessions: number;

  /**
   * Number of messages being processed
   */
  processingMessages: number;

  /**
   * Total messages processed
   */
  totalMessagesProcessed: number;

  /**
   * Total tool calls executed
   */
  totalToolCalls: number;

  /**
   * Last activity timestamp
   */
  lastActivityAt: Date;

  /**
   * Agent uptime in seconds
   */
  uptimeSeconds: number;
}

// =============================================================================
// Message Processing Types
// =============================================================================

/**
 * A message to be processed by the agent
 */
export interface ProcessMessage {
  /**
   * Unique message ID
   */
  id: string;

  /**
   * Session ID for context
   */
  sessionId: string;

  /**
   * Message from user
   */
  content: string;

  /**
   * Message metadata
   */
  metadata?: MessageMetadata;

  /**
   * Timestamp when message was received
   */
  receivedAt: Date;

  /**
   * Response callback
   */
  respond: (response: AgentResponse) => void | Promise<void>;
}

/**
 * Metadata for incoming messages
 */
export interface MessageMetadata {
  /**
   * Correlation ID for tracking
   */
  correlationId?: string;

  /**
   * Message priority
   */
  priority?: 'low' | 'normal' | 'high' | 'critical';

  /**
   * Optional labels
   */
  labels?: Record<string, string>;

  /**
   * Source of the message
   */
  source?: string;

  /**
   * Original message from orchestrator
   */
  originalMessage?: AgentMessage;
}

/**
 * Response from the agent
 */
export interface AgentResponse {
  /**
   * Response content
   */
  content: string;

  /**
   * Whether response was streamed
   */
  streamed?: boolean;

  /**
   * Tool calls made during processing
   */
  toolCalls?: ToolCallInfo[];

  /**
   * Token usage information
   */
  usage?: TokenUsage;

  /**
   * Finish reason
   */
  finishReason?: FinishReason;

  /**
   * Any errors that occurred
   */
  error?: AgentError;

  /**
   * Response metadata
   */
  metadata?: ResponseMetadata;
}

/**
 * Information about a tool call made during processing
 */
export interface ToolCallInfo {
  /**
   * Tool call ID
   */
  id: string;

  /**
   * Tool name
   */
  name: string;

  /**
   * Tool arguments
   */
  arguments: Record<string, unknown>;

  /**
   * Tool result
   */
  result?: ToolResult;

  /**
   * Whether the tool call succeeded
   */
  success: boolean;

  /**
   * Duration of tool execution
   */
  durationMs: number;

  /**
   * Timestamp when tool was called
   */
  timestamp: Date;
}

/**
 * Metadata for responses
 */
export interface ResponseMetadata {
  /**
   * Response ID
   */
  responseId: string;

  /**
   * Session ID
   */
  sessionId: string;

  /**
   * Request ID being responded to
   */
  requestId: string;

  /**
   * Timestamp when response was generated
   */
  timestamp: Date;

  /**
   * Processing duration in milliseconds
   */
  processingDurationMs: number;

  /**
   * Model used for generation
   */
  model: string;

  /**
   * Provider used
   */
  provider: string;
}

// =============================================================================
// Error Types
// =============================================================================

/**
 * Error codes for agent failures
 */
export enum AgentErrorCode {
  // Initialization errors
  INIT_FAILED = 'INIT_FAILED',
  CONFIG_INVALID = 'CONFIG_INVALID',

  // Connection errors
  ORCHESTRATOR_CONNECTION_FAILED = 'ORCHESTRATOR_CONNECTION_FAILED',
  LLM_GATEWAY_CONNECTION_FAILED = 'LLM_GATEWAY_CONNECTION_FAILED',
  REGISTRATION_FAILED = 'REGISTRATION_FAILED',

  // Message processing errors
  MESSAGE_INVALID = 'MESSAGE_INVALID',
  MESSAGE_TIMEOUT = 'MESSAGE_TIMEOUT',
  MESSAGE_PROCESSING_FAILED = 'MESSAGE_PROCESSING_FAILED',

  // Session errors
  SESSION_NOT_FOUND = 'SESSION_NOT_FOUND',
  SESSION_EXPIRED = 'SESSION_EXPIRED',
  SESSION_CREATE_FAILED = 'SESSION_CREATE_FAILED',

  // LLM errors
  LLM_REQUEST_FAILED = 'LLM_REQUEST_FAILED',
  LLM_STREAM_FAILED = 'LLM_STREAM_FAILED',
  LLM_TIMEOUT = 'LLM_TIMEOUT',

  // Tool execution errors
  TOOL_EXECUTION_FAILED = 'TOOL_EXECUTION_FAILED',
  TOOL_NOT_AVAILABLE = 'TOOL_NOT_AVAILABLE',
  DELEGATION_FAILED = 'DELEGATION_FAILED',

  // System errors
  SHUTDOWN_IN_PROGRESS = 'SHUTDOWN_IN_PROGRESS',
  NOT_READY = 'NOT_READY',
  UNKNOWN_ERROR = 'UNKNOWN_ERROR',
}

/**
 * Error thrown by the agent
 */
export class AgentError extends Error {
  constructor(
    message: string,
    public code: AgentErrorCode,
    public retryable: boolean = false,
    public details?: Record<string, unknown>
  ) {
    super(message);
    this.name = 'AgentError';
  }
}

// =============================================================================
// Tool Execution Types
// =============================================================================

/**
 * Context for tool execution
 */
export interface ToolExecutionContext {
  /**
   * Session ID
   */
  sessionId: string;

  /**
   * Message ID being processed
   */
  messageId: string;

  /**
   * Tool call being executed
   */
  toolCall: ToolCall;

  /**
   * Available tools
   */
  availableTools: ToolDefinition[];

  /**
   * Abort signal for cancellation
   */
  signal?: AbortSignal;
}

/**
 * Result of a tool execution
 */
export interface ToolExecutionResult {
  /**
   * Tool call ID
   */
  toolCallId: string;

  /**
   * Tool name
   */
  toolName: string;

  /**
   * Tool result
   */
  result: ToolResult;

  /**
   * Duration in milliseconds
   */
  durationMs: number;

  /**
   * Timestamp
   */
  timestamp: Date;
}

// =============================================================================
// LLM Integration Types
// =============================================================================

/**
 * Request parameters for LLM completion
 */
export interface LLMCompletionRequest extends Omit<CompletionParams, 'tools'> {
  /**
   * Session ID for context
   */
  sessionId: string;

  /**
   * Message ID being processed
   */
  messageId: string;

  /**
   * Available tools for function calling
   */
  tools?: ToolDefinition[];

  /**
   * Whether to stream the response
   */
  stream?: boolean;

  /**
   * Abort signal for cancellation
   */
  signal?: AbortSignal;
}

/**
 * Handler for LLM streaming responses
 */
export interface LLMStreamHandler {
  /**
   * Called when a content chunk is received
   */
  onContent?: (chunk: string) => void | Promise<void>;

  /**
   * Called when a tool call is received
   */
  onToolCall?: (toolCall: ToolCall) => void | Promise<void>;

  /**
   * Called when usage info is received
   */
  onUsage?: (usage: TokenUsage) => void | Promise<void>;

  /**
   * Called when an error occurs
   */
  onError?: (error: Error) => void | Promise<void>;

  /**
   * Called when stream completes
   */
  onComplete?: (finishReason: FinishReason) => void | Promise<void>;
}

// =============================================================================
// Agent Events
// =============================================================================

/**
 * Events emitted by the Agent
 */
export enum AgentEventType {
  // Lifecycle events
  INITIALIZED = 'initialized',
  REGISTERED = 'registered',
  UNREGISTERED = 'unregistered',
  READY = 'ready',
  SHUTDOWN = 'shutdown',
  ERROR = 'error',

  // Message processing events
  MESSAGE_RECEIVED = 'messageReceived',
  MESSAGE_PROCESSING = 'messageProcessing',
  MESSAGE_PROCESSED = 'messageProcessed',
  MESSAGE_FAILED = 'messageFailed',

  // LLM events
  LLM_REQUEST_START = 'llmRequestStart',
  LLM_REQUEST_COMPLETE = 'llmRequestComplete',
  LLM_STREAM_START = 'llmStreamStart',
  LLM_STREAM_CHUNK = 'llmStreamChunk',
  LLM_STREAM_END = 'llmStreamEnd',
  LLM_ERROR = 'llmError',

  // Tool events
  TOOL_CALL_START = 'toolCallStart',
  TOOL_CALL_COMPLETE = 'toolCallComplete',
  TOOL_CALL_FAILED = 'toolCallFailed',

  // Delegation events
  DELEGATION_START = 'delegation_start',
  DELEGATION_COMPLETE = 'delegation_complete',
  DELEGATION_FAILED = 'delegation_failed',

  // Session events
  SESSION_CREATED = 'sessionCreated',
  SESSION_ACTIVATED = 'sessionActivated',
  SESSION_CLOSED = 'sessionClosed',
}

/**
 * Event data for agent events
 */
export interface AgentEvent {
  /**
   * Event type
   */
  type: AgentEventType;

  /**
   * Event timestamp
   */
  timestamp: Date;

  /**
   * Event-specific data
   */
  data?: unknown;

  /**
   * Optional error
   */
  error?: Error;
}

// =============================================================================
// Agent Dependencies
// =============================================================================

/**
 * Dependencies required by the Agent
 */
export interface AgentDependencies {
  /**
   * gRPC client for orchestrator communication
   */
  grpcClient: Client;

  /**
   * Session manager for context
   */
  sessionManager: import('../session/manager.js').SessionManager;

  /**
   * Event emitter for internal events
   */
  eventEmitter: EventEmitter;
}

// =============================================================================
// Helper Types
// =============================================================================

/**
 * Result of processing a message
 */
export interface MessageProcessResult {
  /**
   * Response content
   */
  content: string;

  /**
    * Whether response used streaming mode
    */
    streamed?: boolean;

    /**
   * Tool calls made
   */
  toolCalls: ToolCallInfo[];

  /**
   * Token usage
   */
  usage?: TokenUsage;

  /**
   * Finish reason
   */
  finishReason?: FinishReason;

  /**
   * Processing duration in milliseconds
   */
  durationMs: number;

  /**
   * Whether processing succeeded
   */
  success: boolean;

  /**
   * Error if processing failed
   */
  error?: AgentError;
}
