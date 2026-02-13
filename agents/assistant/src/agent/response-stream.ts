// response-stream.ts - Response streaming and aggregation for Assistant agent
//
// This file provides utilities for streaming and aggregating LLM responses,
// including content chunks, tool calls, and usage information. It follows
// the patterns established in pkg/provider/streaming.go.
//
// Copyright 2026 baaaht project

import type {
  ToolCall,
  TokenUsage,
  FinishReason,
} from '../proto/llm.js';
import type {
  ParsedStreamEvent,
  ContentDeltaEvent,
  ToolUseChunkEvent,
  UsageEvent,
  ErrorEvent,
  CompleteEvent,
  StreamingEventType,
} from '../llm/stream-parser.js';

// =============================================================================
// Response Stream Event Types
// =============================================================================

/**
 * Represents the type of response stream event
 */
export enum ResponseStreamEventType {
  StreamStart = 'stream.start',
  StreamChunk = 'stream.chunk',
  StreamEnd = 'stream.end',
  StreamError = 'stream.error',
  TokenDelta = 'token.delta',
  ContentDelta = 'content.delta',
  ToolUseStart = 'tool_use.start',
  ToolUseChunk = 'tool_use.chunk',
  ToolUseEnd = 'tool_use.end',
}

// =============================================================================
// Response Stream Event
// =============================================================================

/**
 * Response stream event metadata
 */
export interface ResponseStreamEventMetadata {
  /** Request identifier */
  requestId?: string;
  /** Session identifier */
  sessionId?: string;
  /** Correlation identifier for tracking */
  correlationId?: string;
  /** Event timestamp in milliseconds */
  timestamp?: number;
  /** Optional labels for filtering/routing */
  labels?: Record<string, string>;
}

/**
 * Response stream event
 */
export interface ResponseStreamEvent {
  /** Unique event identifier */
  id: string;
  /** Event type */
  type: ResponseStreamEventType;
  /** Stream identifier for grouping related events */
  streamId: string;
  /** Provider name */
  provider: string;
  /** Model identifier */
  model: string;
  /** Optional event data */
  data?: Record<string, unknown>;
  /** Event metadata */
  metadata?: ResponseStreamEventMetadata;
}

// =============================================================================
// Response Stream Handler
// =============================================================================

/**
 * Handler for processing response stream events
 */
export interface ResponseStreamEventHandler {
  /**
   * Processes a response stream event
   *
   * @param event - The event to process
   * @returns Promise that resolves when handling is complete
   */
  handle(event: ResponseStreamEvent): Promise<void> | void;

  /**
   * Returns true if this handler can process the given event type
   *
   * @param eventType - The event type to check
   * @returns True if the handler can process this event type
   */
  canHandle(eventType: ResponseStreamEventType): boolean;
}

/**
 * Function adapter for ResponseStreamEventHandler
 */
export type ResponseStreamEventFunc = (
  event: ResponseStreamEvent
) => Promise<void> | void;

/**
 * Creates a ResponseStreamEventHandler from a function
 *
 * @param fn - Function to handle events
 * @param canHandleFn - Optional function to check if event can be handled
 * @returns A ResponseStreamEventHandler adapter
 */
export function createResponseStreamEventHandler(
  fn: ResponseStreamEventFunc,
  canHandleFn?: (eventType: ResponseStreamEventType) => boolean
): ResponseStreamEventHandler {
  return {
    handle: fn,
    canHandle: canHandleFn ?? (() => true),
  };
}

// =============================================================================
// Response Stream Filter
// =============================================================================

/**
 * Filter for response stream events
 */
export interface ResponseStreamEventFilter {
  /** Event type to filter by */
  type?: ResponseStreamEventType;
  /** Stream ID to filter by */
  streamId?: string;
  /** Provider to filter by */
  provider?: string;
  /** Model to filter by */
  model?: string;
  /** Session ID to filter by */
  sessionId?: string;
  /** Correlation ID to filter by */
  correlationId?: string;
  /** Labels to filter by (all must match) */
  labels?: Record<string, string>;
}

/**
 * Tests if an event matches the given filter
 *
 * @param event - The event to test
 * @param filter - The filter to apply
 * @returns True if the event matches the filter
 */
export function responseEventMatchesFilter(
  event: ResponseStreamEvent,
  filter: ResponseStreamEventFilter
): boolean {
  if (filter.type !== undefined && event.type !== filter.type) {
    return false;
  }
  if (filter.streamId !== undefined && event.streamId !== filter.streamId) {
    return false;
  }
  if (filter.provider !== undefined && event.provider !== filter.provider) {
    return false;
  }
  if (filter.model !== undefined && event.model !== filter.model) {
    return false;
  }
  if (
    filter.sessionId !== undefined &&
    event.metadata?.sessionId !== filter.sessionId
  ) {
    return false;
  }
  if (
    filter.correlationId !== undefined &&
    event.metadata?.correlationId !== filter.correlationId
  ) {
    return false;
  }
  if (filter.labels && Object.keys(filter.labels).length > 0) {
    for (const [key, value] of Object.entries(filter.labels)) {
      if (event.metadata?.labels?.[key] !== value) {
        return false;
      }
    }
  }
  return true;
}

// =============================================================================
// Response Stream Middleware
// =============================================================================

/**
 * Middleware for processing response stream events before handlers
 */
export interface ResponseStreamMiddleware {
  /**
   * Processes an event and returns the modified event
   *
   * @param event - The event to process
   * @returns Promise resolving to the processed event
   */
  process(event: ResponseStreamEvent): Promise<ResponseStreamEvent>;
}

/**
 * Function adapter for ResponseStreamMiddleware
 */
export type ResponseStreamMiddlewareFunc = (
  event: ResponseStreamEvent
) => Promise<ResponseStreamEvent>;

/**
 * Creates a ResponseStreamMiddleware from a function
 *
 * @param fn - Function to process events
 * @returns A ResponseStreamMiddleware adapter
 */
export function createResponseStreamMiddleware(
  fn: ResponseStreamMiddlewareFunc
): ResponseStreamMiddleware {
  return {
    process: fn,
  };
}

// =============================================================================
// Response Stream Subscription
// =============================================================================

/**
 * Represents a subscription to response stream events
 */
export interface ResponseStreamSubscription {
  /** Unique subscription identifier */
  id: string;
  /** Filter for events to receive */
  filter: ResponseStreamEventFilter;
  /** Handler for matching events */
  handler: ResponseStreamEventHandler;
  /** Whether the subscription is active */
  active: boolean;
  /** Provider for this subscription */
  provider: string;
}

// =============================================================================
// Response Stream Handle
// =============================================================================

/**
 * Handle to an active response stream
 */
export interface ResponseStreamHandle {
  /** Stream identifier */
  id: string;
  /** Provider name */
  provider: string;
  /** Model identifier */
  model: string;
  /** Whether the stream is active */
  active: boolean;
  /** Readable channel for chunks */
  chunks: ReadableStream<ParsedStreamEvent>;

  /**
   * Closes the stream handle
   */
  close(): void;
}

/**
 * Default implementation of ResponseStreamHandle
 */
export class ResponseStreamHandleImpl implements ResponseStreamHandle {
  private _active = true;
  private _controller?: ReadableStreamDefaultController<ParsedStreamEvent>;
  public readonly chunks: ReadableStream<ParsedStreamEvent>;

  constructor(
    public id: string,
    public provider: string,
    public model: string
  ) {
    const stream = new ReadableStream<ParsedStreamEvent>({
      start: (controller) => {
        this._controller = controller;
      },
      cancel: () => {
        this._active = false;
      },
    });
    this.chunks = stream;
  }

  get active(): boolean {
    return this._active;
  }

  close(): void {
    this._active = false;
    this._controller?.close();
  }

  /**
   * Enqueues a chunk to the stream
   *
   * @param chunk - Chunk to enqueue
   */
  enqueue(chunk: ParsedStreamEvent): void {
    if (this._active && this._controller) {
      this._controller.enqueue(chunk);
    }
  }

  /**
   * Signals an error on the stream
   *
   * @param error - Error to signal
   */
  error(error: Error): void {
    if (this._controller) {
      this._controller.error(error);
    }
    this._active = false;
  }
}

// =============================================================================
// Response Stream Callback
// =============================================================================

/**
 * Callback function for streaming response chunks
 */
export type ResponseStreamCallback = (
  event: ParsedStreamEvent
) => Promise<void> | void;

// =============================================================================
// Response Stream Context
// =============================================================================

/**
 * Context for a streaming response operation
 */
export interface ResponseStreamContext {
  /** Request identifier */
  requestId: string;
  /** Provider name */
  provider: string;
  /** Model identifier */
  model: string;
  /** Abort signal for cancellation */
  signal?: AbortSignal;
  /** Event handler for the stream */
  handler?: ResponseStreamEventHandler;
  /** Additional metadata */
  metadata?: Record<string, unknown>;
}

/**
 * Creates a child context with cancellation
 *
 * @param signal - Optional abort signal
 * @returns Tuple of [AbortSignal, AbortController]
 */
export function createStreamContext(
  signal?: AbortSignal
): [AbortSignal, AbortController] {
  void signal;
  const controller = new AbortController();
  return [controller.signal, controller];
}

// =============================================================================
// Response Stream Options
// =============================================================================

/**
 * Configuration options for response streaming
 */
export interface ResponseStreamOptions {
  /** Buffer size for the event channel (default: 10) */
  bufferSize?: number;
  /** Whether to include token usage in chunks (default: true) */
  includeUsage?: boolean;
  /** Whether to include raw provider responses (default: false) */
  includeRaw?: boolean;
  /** Callback invoked for each chunk */
  callback?: ResponseStreamCallback;
  /** Middleware chain to apply to events */
  middleware?: ResponseStreamMiddleware[];
}

/**
 * Default response stream options
 */
export function defaultResponseStreamOptions(): ResponseStreamOptions {
  return {
    bufferSize: 10,
    includeUsage: true,
    includeRaw: false,
    middleware: [],
  };
}

/**
 * Merges provided options with defaults
 *
 * @param options - Options to merge
 * @returns Merged options
 */
export function mergeResponseStreamOptions(
  options?: ResponseStreamOptions
): ResponseStreamOptions {
  const defaults = defaultResponseStreamOptions();
  if (!options) {
    return defaults;
  }
  return {
    bufferSize: options.bufferSize ?? defaults.bufferSize,
    includeUsage: options.includeUsage ?? defaults.includeUsage,
    includeRaw: options.includeRaw ?? defaults.includeRaw,
    callback: options.callback ?? defaults.callback,
    middleware: options.middleware ?? defaults.middleware,
  };
}

// =============================================================================
// Aggregated Response
// =============================================================================

/**
 * Aggregated response from streaming chunks
 */
export interface AggregatedResponse {
  /** Response content accumulated from chunks */
  content: string;
  /** Tool calls accumulated from chunks */
  toolCalls: ToolCall[];
  /** Token usage information */
  usage?: TokenUsage;
  /** Reason generation stopped */
  finishReason?: FinishReason;
  /** Provider that handled the request */
  provider: string;
  /** Model that handled the request */
  model: string;
  /** Whether the stream completed successfully */
  complete: boolean;
  /** Error if the stream failed */
  error?: Error;
}

// =============================================================================
// Response Aggregator
// =============================================================================

/**
 * Configuration for ResponseAggregator
 */
export interface ResponseAggregatorOptions {
  /** Maximum content length in characters (default: 1MB) */
  maxContentLength?: number;
  /** Maximum number of tool calls (default: 50) */
  maxToolCalls?: number;
}

/**
 * Default aggregator options
 */
const DEFAULT_AGGREGATOR_OPTIONS: Required<ResponseAggregatorOptions> = {
  maxContentLength: 1024 * 1024, // 1MB
  maxToolCalls: 50,
};

/**
 * Aggregates streaming events into a complete response
 *
 * This class handles:
 * - Accumulating content chunks
 * - Assembling tool calls from delta chunks
 * - Tracking token usage
 * - Detecting stream completion
 */
export class ResponseAggregator {
  private options: Required<ResponseAggregatorOptions>;
  private content = '';
  private toolCallsMap = new Map<string, { name: string; arguments: string }>();
  private usage?: TokenUsage;
  private finishReason?: FinishReason;
  private complete = false;
  private error?: Error;

  constructor(options: ResponseAggregatorOptions = {}) {
    this.options = { ...DEFAULT_AGGREGATOR_OPTIONS, ...options };
  }

  /**
   * Processes a parsed stream event
   *
   * @param event - The event to process
   * @returns The aggregated response if complete, null otherwise
   */
  process(event: ParsedStreamEvent): AggregatedResponse | null {
    if (this.complete) {
      return this.toResponse();
    }

    if (this.error) {
      return this.toResponse();
    }

    switch (event.type) {
      case 'content.delta':
        return this.processContentDelta(event as ContentDeltaEvent);

      case 'tool_use.chunk':
        return this.processToolUseChunk(event as ToolUseChunkEvent);

      case 'stream.chunk':
        return this.processUsageEvent(event as UsageEvent);

      case 'stream.error':
        return this.processErrorEvent(event as ErrorEvent);

      case 'stream.end':
        return this.processCompleteEvent(event as CompleteEvent);

      default:
        return null;
    }
  }

  /**
   * Processes a content delta event
   */
  private processContentDelta(event: ContentDeltaEvent): AggregatedResponse | null {
    if (event.data.content) {
      if (this.content.length + event.data.content.length > this.options.maxContentLength) {
        this.error = new Error(`Maximum content length exceeded: ${this.options.maxContentLength}`);
        return this.toResponse();
      }
      this.content += event.data.content;
    }
    return null;
  }

  /**
   * Processes a tool use chunk event
   */
  private processToolUseChunk(event: ToolUseChunkEvent): AggregatedResponse | null {
    const { toolCallId, name, argumentsDelta } = event.data;

    if (!toolCallId) {
      return null;
    }

    // Check max tool calls limit
    if (!this.toolCallsMap.has(toolCallId) && this.toolCallsMap.size >= this.options.maxToolCalls) {
      this.error = new Error(`Maximum tool calls limit exceeded: ${this.options.maxToolCalls}`);
      return this.toResponse();
    }

    // Get or create tool call entry
    let toolCall = this.toolCallsMap.get(toolCallId);
    if (!toolCall) {
      toolCall = { name: name || '', arguments: '' };
      this.toolCallsMap.set(toolCallId, toolCall);
    }

    // Update name and arguments
    if (name) {
      toolCall.name = name;
    }
    if (argumentsDelta) {
      toolCall.arguments += argumentsDelta;
    }

    return null;
  }

  /**
   * Processes a usage event
   */
  private processUsageEvent(event: UsageEvent): AggregatedResponse | null {
    if (event.data.usage) {
      this.usage = {
        inputTokens: event.data.usage.inputTokens ?? this.usage?.inputTokens,
        outputTokens: event.data.usage.outputTokens ?? this.usage?.outputTokens,
        totalTokens:
          (event.data.usage.totalTokens ??
          this.usage?.totalTokens),
      };
    }
    return null;
  }

  /**
   * Processes an error event
   */
  private processErrorEvent(event: ErrorEvent): AggregatedResponse {
    this.complete = true;
    this.error = new Error(event.data.message || 'Stream error');
    return this.toResponse();
  }

  /**
   * Processes a complete event
   */
  private processCompleteEvent(event: CompleteEvent): AggregatedResponse {
    this.complete = true;

    // Extract final response data if available
    if (event.data.response) {
      const { response } = event.data;

      if (response.content) {
        this.content = response.content;
      }

      if (response.toolCalls && response.toolCalls.length > 0) {
        this.toolCallsMap.clear();
        for (const tc of response.toolCalls) {
          if (tc.id) {
            this.toolCallsMap.set(tc.id, {
              name: tc.name || '',
              arguments: tc.arguments || '{}',
            });
          }
        }
      }

      if (response.usage) {
        this.usage = response.usage;
      }

      if (response.finishReason) {
        this.finishReason = response.finishReason as unknown as FinishReason;
      }
    }

    return this.toResponse();
  }

  /**
   * Converts the aggregator state to an AggregatedResponse
   */
  private toResponse(): AggregatedResponse {
    // Convert tool calls map to array
    const toolCalls: ToolCall[] = Array.from(this.toolCallsMap.entries()).map(
      ([id, data]) => ({
        id,
        name: data.name,
        arguments: data.arguments || '{}',
      })
    );

    return {
      content: this.content,
      toolCalls,
      usage: this.usage,
      finishReason: this.finishReason,
      provider: 'unknown',
      model: 'unknown',
      complete: this.complete,
      error: this.error,
    };
  }

  /**
   * Resets the aggregator state
   */
  reset(): void {
    this.content = '';
    this.toolCallsMap.clear();
    this.usage = undefined;
    this.finishReason = undefined;
    this.complete = false;
    this.error = undefined;
  }

  /**
   * Returns the current aggregated content
   */
  getContent(): string {
    return this.content;
  }

  /**
   * Returns the current aggregated tool calls
   */
  getToolCalls(): ToolCall[] {
    return Array.from(this.toolCallsMap.entries()).map(([id, data]) => ({
      id,
      name: data.name,
      arguments: data.arguments || '{}',
    }));
  }

  /**
   * Returns whether aggregation is complete
   */
  isComplete(): boolean {
    return this.complete;
  }

  /**
   * Returns whether an error occurred
   */
  hasError(): boolean {
    return this.error !== undefined;
  }

  /**
   * Returns the error if one occurred
   */
  getError(): Error | undefined {
    return this.error;
  }
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Creates a unique stream identifier
 *
 * @returns A unique stream identifier
 */
export function createResponseStreamId(): string {
  return `response_stream_${Date.now()}_${Math.random().toString(36).substring(2, 11)}`;
}

/**
 * Creates a ResponseStreamEvent from a ParsedStreamEvent
 *
 * @param parsedEvent - The parsed stream event
 * @param provider - Provider name
 * @param model - Model identifier
 * @returns A response stream event
 */
export function createResponseStreamEvent(
  parsedEvent: ParsedStreamEvent,
  provider: string,
  model: string
): ResponseStreamEvent {
  const mapEventType = (eventType: StreamingEventType): ResponseStreamEventType => {
    switch (eventType) {
      case 'stream.start':
        return ResponseStreamEventType.StreamStart;
      case 'stream.chunk':
        return ResponseStreamEventType.StreamChunk;
      case 'stream.end':
        return ResponseStreamEventType.StreamEnd;
      case 'stream.error':
        return ResponseStreamEventType.StreamError;
      case 'content.delta':
        return ResponseStreamEventType.ContentDelta;
      case 'tool_use.start':
        return ResponseStreamEventType.ToolUseStart;
      case 'tool_use.chunk':
        return ResponseStreamEventType.ToolUseChunk;
      case 'tool_use.end':
        return ResponseStreamEventType.ToolUseEnd;
      default:
        return ResponseStreamEventType.StreamChunk;
    }
  };

  return {
    id: parsedEvent.id,
    type: mapEventType(parsedEvent.type),
    streamId: parsedEvent.streamId,
    provider,
    model,
    data: parsedEvent.data as Record<string, unknown>,
    metadata: {
      requestId: 'requestId' in parsedEvent.data ? parsedEvent.data.requestId as string : undefined,
      timestamp: parsedEvent.timestamp,
    },
  };
}

/**
 * Checks if an event is a content delta event
 *
 * @param event - The event to check
 * @returns True if the event is a content delta
 */
export function isContentDelta(
  event: ResponseStreamEvent
): event is ResponseStreamEvent & { data: { content?: string } } {
  return event.type === ResponseStreamEventType.ContentDelta;
}

/**
 * Checks if an event is a tool use event
 *
 * @param event - The event to check
 * @returns True if the event is a tool use event
 */
export function isToolUseEvent(
  event: ResponseStreamEvent
): event is ResponseStreamEvent & { data: { toolCallId?: string; name?: string } } {
  return (
    event.type === ResponseStreamEventType.ToolUseStart ||
    event.type === ResponseStreamEventType.ToolUseChunk ||
    event.type === ResponseStreamEventType.ToolUseEnd
  );
}

/**
 * Checks if an event is an error event
 *
 * @param event - The event to check
 * @returns True if the event is an error event
 */
export function isErrorEvent(event: ResponseStreamEvent): boolean {
  return event.type === ResponseStreamEventType.StreamError;
}

/**
 * Checks if an event is a complete event
 *
 * @param event - The event to check
 * @returns True if the event is a complete event
 */
export function isCompleteEvent(event: ResponseStreamEvent): boolean {
  return event.type === ResponseStreamEventType.StreamEnd;
}
