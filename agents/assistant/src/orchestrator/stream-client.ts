// stream-client.ts - Bidirectional streaming client for Orchestrator communication
//
// This file provides TypeScript clients for bidirectional gRPC streaming
// with the Orchestrator's AgentService for both agent communication and
// task execution with real-time updates.
//
// Copyright 2026 baaaht project

import { EventEmitter } from 'events';
import type {
  StreamAgentRequest,
  StreamAgentResponse,
  StreamAgentRequestPayload,
  StreamAgentResponsePayload,
  AgentMessage,
  StreamTaskRequest,
  StreamTaskResponse,
  StreamTaskRequestPayload,
  StreamTaskResponsePayload,
  TaskInput,
  TaskOutput,
  TaskStatusUpdate,
  TaskProgress,
  TaskComplete,
  TaskError,
  CancelCommand,
} from '../proto/agent.js';
import type { Client } from '@grpc/grpc-js';

interface BidiStream<Req, Res> {
  write(request: Req): void;
  cancel(): void;
  on(event: 'data', listener: (response: Res) => void): this;
  on(event: 'error', listener: (err: Error) => void): this;
  on(event: 'end', listener: () => void): this;
}

interface AgentStreamingClient extends Client {
  streamAgent(): BidiStream<StreamAgentRequest, StreamAgentResponse>;
  streamTask(): BidiStream<StreamTaskRequest, StreamTaskResponse>;
}

/**
 * StreamOptions holds configuration for streaming connections
 */
export interface StreamOptions {
  /**
   * Whether to automatically reconnect on connection loss
   * Defaults to false
   */
  autoReconnect?: boolean;

  /**
   * Maximum number of reconnection attempts
   * Defaults to 3
   */
  maxReconnectAttempts?: number;

  /**
   * Delay between reconnection attempts in milliseconds
   * Defaults to 1000 (1 second)
   */
  reconnectDelay?: number;

  /**
   * Request timeout in milliseconds
   * Defaults to 30000 (30 seconds)
   */
  timeout?: number;
}

/**
 * StreamEvent represents events emitted by the stream client
 */
export enum StreamEventType {
  /** Stream established successfully */
  ESTABLISHED = 'established',
  /** Stream closed normally */
  CLOSED = 'closed',
  /** Stream error occurred */
  ERROR = 'error',
  /** Message received */
  MESSAGE = 'message',
  /** Task output received */
  OUTPUT = 'output',
  /** Task status update received */
  STATUS = 'status',
  /** Task progress update received */
  PROGRESS = 'progress',
  /** Task completed */
  COMPLETE = 'complete',
  /** Task error received */
  TASK_ERROR = 'task_error',
  /** Heartbeat ack received */
  HEARTBEAT = 'heartbeat',
}

/**
 * StreamEventData represents the data structure for stream events
 */
export interface StreamEventData {
  type: StreamEventType;
  data?: unknown;
  error?: Error;
}

/**
 * StreamAgentClient handles bidirectional streaming for agent communication
 *
 * This client manages the StreamAgent RPC which allows real-time bidirectional
 * messaging between the agent and orchestrator.
 *
 * Usage:
 * ```typescript
 * const streamClient = new StreamAgentClient(client, 'agent-id-123');
 * streamClient.on(StreamEventType.MESSAGE, (msg) => console.log(msg));
 * await streamClient.connect();
 * await streamClient.sendMessage(message);
 * await streamClient.close();
 * ```
 */
export class StreamAgentClient extends EventEmitter {
  private client: Client;
  private agentId: string;
  private options: Required<StreamOptions>;
  private stream: BidiStream<StreamAgentRequest, StreamAgentResponse> | null = null;
  private connected: boolean = false;
  private reconnectAttempts: number = 0;
  private reconnectTimeout: NodeJS.Timeout | null = null;

  /**
   * Creates a new StreamAgentClient
   *
   * @param client - The gRPC client connected to the orchestrator
   * @param agentId - The agent ID for this connection
   * @param options - Optional stream configuration
   */
  constructor(client: Client, agentId: string, options: StreamOptions = {}) {
    super();
    this.client = client;
    this.agentId = agentId;
    this.options = {
      autoReconnect: options.autoReconnect ?? false,
      maxReconnectAttempts: options.maxReconnectAttempts ?? 3,
      reconnectDelay: options.reconnectDelay ?? 1000,
      timeout: options.timeout ?? 30000,
    };
  }

  /**
   * Connects to the orchestrator and establishes the bidirectional stream
   *
   * @throws Error if connection fails or already connected
   */
  async connect(): Promise<void> {
    if (this.connected) {
      throw new Error('Stream already connected');
    }

    try {
      // Create the stream using the gRPC client
      const grpcClient = this.client as AgentStreamingClient;
      this.stream = grpcClient.streamAgent();

      // Set up event handlers for the stream
      this.stream.on('data', (response: StreamAgentResponse) => {
        this.handleResponse(response);
      });

      this.stream.on('error', (err: Error) => {
        this.handleError(err);
      });

      this.stream.on('end', () => {
        this.handleEnd();
      });

      // Send initial message with agent ID to establish the stream
      const initialRequest: StreamAgentRequest = {
        agentId: this.agentId,
      };

      await this.write(initialRequest);
      this.connected = true;
      this.reconnectAttempts = 0;

      this.emit(StreamEventType.ESTABLISHED, { type: StreamEventType.ESTABLISHED });
    } catch (err) {
      const error = err as Error;
      this.stream = null;
      throw new Error(`Failed to establish stream: ${error.message}`);
    }
  }

  /**
   * Sends a message through the stream
   *
   * @param message - The agent message to send
   * @throws Error if not connected or send fails
   */
  async sendMessage(message: AgentMessage): Promise<void> {
    if (!this.connected || !this.stream) {
      throw new Error('Stream not connected');
    }

    const request: StreamAgentRequest = {
      agentId: this.agentId,
      payload: { message } as StreamAgentRequestPayload,
    };

    await this.write(request);
  }

  /**
   * Sends a heartbeat to keep the stream alive
   *
   * @throws Error if not connected or send fails
   */
  async sendHeartbeat(): Promise<void> {
    if (!this.connected || !this.stream) {
      throw new Error('Stream not connected');
    }

    const request: StreamAgentRequest = {
      agentId: this.agentId,
      payload: { heartbeat: null } as StreamAgentRequestPayload,
    };

    await this.write(request);
  }

  /**
   * Closes the stream and cleans up resources
   */
  async close(): Promise<void> {
    this.connected = false;

    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }

    if (this.stream) {
      try {
        this.stream.cancel();
      } catch {
        // Ignore errors during cancellation
      }
      this.stream = null;
    }

    this.emit(StreamEventType.CLOSED, { type: StreamEventType.CLOSED });
    this.removeAllListeners();
  }

  /**
   * Checks if the stream is connected
   */
  isConnected(): boolean {
    return this.connected;
  }

  /**
   * Handles incoming responses from the orchestrator
   *
   * @private
   */
  private handleResponse(response: StreamAgentResponse): void {
    if (!response.payload) {
      return;
    }

    // Handle heartbeat response
    if ('heartbeat' in response.payload) {
      this.emit(StreamEventType.HEARTBEAT, {
        type: StreamEventType.HEARTBEAT,
      });
      return;
    }

    // Handle message response
    if ('message' in response.payload && response.payload.message) {
      this.emit(StreamEventType.MESSAGE, {
        type: StreamEventType.MESSAGE,
        data: response.payload.message,
      });
    }
  }

  /**
   * Handles stream errors
   *
   * @private
   */
  private handleError(err: Error): void {
    this.emit(StreamEventType.ERROR, {
      type: StreamEventType.ERROR,
      error: err,
    });

    // Attempt to reconnect if configured
    if (this.options.autoReconnect && this.reconnectAttempts < this.options.maxReconnectAttempts) {
      this.reconnectAttempts++;
      this.reconnectTimeout = setTimeout(() => {
        this.attemptReconnect();
      }, this.options.reconnectDelay);
    } else {
      this.connected = false;
    }
  }

  /**
   * Handles stream end
   *
   * @private
   */
  private handleEnd(): void {
    this.connected = false;
    this.emit(StreamEventType.CLOSED, { type: StreamEventType.CLOSED });
  }

  /**
   * Attempts to reconnect the stream
   *
   * @private
   */
  private async attemptReconnect(): Promise<void> {
    try {
      await this.connect();
    } catch (err) {
      // Reconnect failed, will be handled by handleError
    }
  }

  /**
   * Writes data to the stream
   *
   * @private
   */
  private async write(request: StreamAgentRequest): Promise<void> {
    return new Promise((resolve, reject) => {
      if (!this.stream) {
        reject(new Error('Stream not available'));
        return;
      }

      try {
        this.stream.write(request);
        resolve();
      } catch (err) {
        const error = err as Error;
        reject(new Error(`Failed to write to stream: ${error.message}`));
      }
    });
  }
}

/**
 * StreamTaskClient handles bidirectional streaming for task execution
 *
 * This client manages the StreamTask RPC which allows real-time task execution
 * with streaming output, progress updates, and status changes.
 *
 * Usage:
 * ```typescript
 * const taskStream = new StreamTaskClient(client, 'task-id-456');
 * taskStream.on(StreamEventType.OUTPUT, (output) => console.log(output.text));
 * await taskStream.connect();
 * await taskStream.sendInput(data);
 * await taskStream.close();
 * ```
 */
export class StreamTaskClient extends EventEmitter {
  private client: Client;
  private taskId: string;
  private options: Required<StreamOptions>;
  private stream: BidiStream<StreamTaskRequest, StreamTaskResponse> | null = null;
  private connected: boolean = false;
  private reconnectAttempts: number = 0;
  private reconnectTimeout: NodeJS.Timeout | null = null;

  /**
   * Creates a new StreamTaskClient
   *
   * @param client - The gRPC client connected to the orchestrator
   * @param taskId - The task ID for this stream
   * @param options - Optional stream configuration
   */
  constructor(client: Client, taskId: string, options: StreamOptions = {}) {
    super();
    this.client = client;
    this.taskId = taskId;
    this.options = {
      autoReconnect: options.autoReconnect ?? false,
      maxReconnectAttempts: options.maxReconnectAttempts ?? 3,
      reconnectDelay: options.reconnectDelay ?? 1000,
      timeout: options.timeout ?? 30000,
    };
  }

  /**
   * Connects to the orchestrator and establishes the task stream
   *
   * @throws Error if connection fails or already connected
   */
  async connect(): Promise<void> {
    if (this.connected) {
      throw new Error('Stream already connected');
    }

    try {
      // Create the stream using the gRPC client
      const grpcClient = this.client as AgentStreamingClient;
      this.stream = grpcClient.streamTask();

      // Set up event handlers for the stream
      this.stream.on('data', (response: StreamTaskResponse) => {
        this.handleResponse(response);
      });

      this.stream.on('error', (err: Error) => {
        this.handleError(err);
      });

      this.stream.on('end', () => {
        this.handleEnd();
      });

      // Send initial message with task ID to establish the stream
      const initialRequest: StreamTaskRequest = {
        taskId: this.taskId,
      };

      await this.write(initialRequest);
      this.connected = true;
      this.reconnectAttempts = 0;

      this.emit(StreamEventType.ESTABLISHED, { type: StreamEventType.ESTABLISHED });
    } catch (err) {
      const error = err as Error;
      this.stream = null;
      throw new Error(`Failed to establish task stream: ${error.message}`);
    }
  }

  /**
   * Sends input data to the task
   *
   * @param data - The input data to send
   * @param metadata - Optional metadata for the input
   * @throws Error if not connected or send fails
   */
  async sendInput(data: Uint8Array, metadata?: Record<string, string>): Promise<void> {
    if (!this.connected || !this.stream) {
      throw new Error('Stream not connected');
    }

    const input: TaskInput = {
      data,
      metadata: metadata ?? {},
    };

    const request: StreamTaskRequest = {
      taskId: this.taskId,
      payload: { input } as StreamTaskRequestPayload,
    };

    await this.write(request);
  }

  /**
   * Sends a heartbeat to keep the stream alive
   *
   * @throws Error if not connected or send fails
   */
  async sendHeartbeat(): Promise<void> {
    if (!this.connected || !this.stream) {
      throw new Error('Stream not connected');
    }

    const request: StreamTaskRequest = {
      taskId: this.taskId,
      payload: { heartbeat: null } as StreamTaskRequestPayload,
    };

    await this.write(request);
  }

  /**
   * Cancels the running task
   *
   * @param reason - Optional reason for cancellation
   * @param force - Whether to force cancellation
   * @throws Error if not connected or send fails
   */
  async cancel(reason?: string, force: boolean = false): Promise<void> {
    if (!this.connected || !this.stream) {
      throw new Error('Stream not connected');
    }

    const cancel: CancelCommand = {
      reason: reason ?? 'Client requested cancellation',
      force,
    };

    const request: StreamTaskRequest = {
      taskId: this.taskId,
      payload: { cancel } as StreamTaskRequestPayload,
    };

    await this.write(request);
  }

  /**
   * Closes the stream and cleans up resources
   */
  async close(): Promise<void> {
    this.connected = false;

    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }

    if (this.stream) {
      try {
        this.stream.cancel();
      } catch {
        // Ignore errors during cancellation
      }
      this.stream = null;
    }

    this.emit(StreamEventType.CLOSED, { type: StreamEventType.CLOSED });
    this.removeAllListeners();
  }

  /**
   * Checks if the stream is connected
   */
  isConnected(): boolean {
    return this.connected;
  }

  /**
   * Handles incoming responses from the orchestrator
   *
   * @private
   */
  private handleResponse(response: StreamTaskResponse): void {
    if (!response.payload) {
      return;
    }

    // Handle heartbeat response
    if ('heartbeat' in response.payload) {
      this.emit(StreamEventType.HEARTBEAT, {
        type: StreamEventType.HEARTBEAT,
      });
      return;
    }

    // Handle output
    if ('output' in response.payload && response.payload.output) {
      this.emit(StreamEventType.OUTPUT, {
        type: StreamEventType.OUTPUT,
        data: response.payload.output,
      });
      return;
    }

    // Handle status update
    if ('status' in response.payload && response.payload.status) {
      this.emit(StreamEventType.STATUS, {
        type: StreamEventType.STATUS,
        data: response.payload.status,
      });
      return;
    }

    // Handle progress update
    if ('progress' in response.payload && response.payload.progress) {
      this.emit(StreamEventType.PROGRESS, {
        type: StreamEventType.PROGRESS,
        data: response.payload.progress,
      });
      return;
    }

    // Handle task completion
    if ('complete' in response.payload && response.payload.complete) {
      this.emit(StreamEventType.COMPLETE, {
        type: StreamEventType.COMPLETE,
        data: response.payload.complete,
      });
      this.connected = false; // Stream ends on completion
      return;
    }

    // Handle task error
    if ('error' in response.payload && response.payload.error) {
      this.emit(StreamEventType.TASK_ERROR, {
        type: StreamEventType.TASK_ERROR,
        data: response.payload.error,
      });
      return;
    }
  }

  /**
   * Handles stream errors
   *
   * @private
   */
  private handleError(err: Error): void {
    this.emit(StreamEventType.ERROR, {
      type: StreamEventType.ERROR,
      error: err,
    });

    // Attempt to reconnect if configured
    if (this.options.autoReconnect && this.reconnectAttempts < this.options.maxReconnectAttempts) {
      this.reconnectAttempts++;
      this.reconnectTimeout = setTimeout(() => {
        this.attemptReconnect();
      }, this.options.reconnectDelay);
    } else {
      this.connected = false;
    }
  }

  /**
   * Handles stream end
   *
   * @private
   */
  private handleEnd(): void {
    this.connected = false;
    this.emit(StreamEventType.CLOSED, { type: StreamEventType.CLOSED });
  }

  /**
   * Attempts to reconnect the stream
   *
   * @private
   */
  private async attemptReconnect(): Promise<void> {
    try {
      await this.connect();
    } catch (err) {
      // Reconnect failed, will be handled by handleError
    }
  }

  /**
   * Writes data to the stream
   *
   * @private
   */
  private async write(request: StreamTaskRequest): Promise<void> {
    return new Promise((resolve, reject) => {
      if (!this.stream) {
        reject(new Error('Stream not available'));
        return;
      }

      try {
        this.stream.write(request);
        resolve();
      } catch (err) {
        const error = err as Error;
        reject(new Error(`Failed to write to stream: ${error.message}`));
      }
    });
  }
}

/**
 * Creates a new StreamAgentClient and connects to the orchestrator
 *
 * @param client - The gRPC client connected to the orchestrator
 * @param agentId - The agent ID for this connection
 * @param options - Optional stream configuration
 * @returns A connected StreamAgentClient
 */
export async function createStreamAgentClient(
  client: Client,
  agentId: string,
  options?: StreamOptions
): Promise<StreamAgentClient> {
  const streamClient = new StreamAgentClient(client, agentId, options);
  await streamClient.connect();
  return streamClient;
}

/**
 * Creates a new StreamTaskClient and connects to the orchestrator
 *
 * @param client - The gRPC client connected to the orchestrator
 * @param taskId - The task ID for this stream
 * @param options - Optional stream configuration
 * @returns A connected StreamTaskClient
 */
export async function createStreamTaskClient(
  client: Client,
  taskId: string,
  options?: StreamOptions
): Promise<StreamTaskClient> {
  const streamClient = new StreamTaskClient(client, taskId, options);
  await streamClient.connect();
  return streamClient;
}
