// stream-parser.ts - SSE (Server-Sent Events) stream parser for LLM Gateway
//
// This file provides utilities for parsing Server-Sent Events streams
// from the LLM Gateway service, extracting JSON data and converting
// to strongly-typed streaming events.
//
// Copyright 2026 baaaht project

import type {
  StreamLLMResponse,
  StreamChunk,
  StreamToolCall,
  StreamUsage,
  StreamError,
  StreamComplete,
} from '../proto/llm.js';

// =============================================================================
// Streaming Event Types
// =============================================================================

/**
 * Represents the type of streaming event
 */
export enum StreamingEventType {
  StreamStart = 'stream.start',
  StreamChunk = 'stream.chunk',
  StreamEnd = 'stream.end',
  StreamError = 'stream.error',
  ContentDelta = 'content.delta',
  ToolUseStart = 'tool_use.start',
  ToolUseChunk = 'tool_use.chunk',
  ToolUseEnd = 'tool_use.end',
}

/**
 * Base interface for all streaming events
 */
export interface StreamingEvent {
  /** Unique identifier for this event */
  id: string;
  /** Type of the streaming event */
  type: StreamingEventType;
  /** Stream identifier for grouping related events */
  streamId: string;
  /** Timestamp when the event was created */
  timestamp: number;
}

/**
 * Content delta event - emitted when new content is available
 */
export interface ContentDeltaEvent extends StreamingEvent {
  type: StreamingEventType.ContentDelta;
  /** The content delta */
  data: {
    /** Request identifier */
    requestId?: string;
    /** Content text chunk */
    content?: string;
    /** Chunk index for ordering */
    index?: number;
    /** Whether this is the last content chunk */
    isLast?: boolean;
  };
}

/**
 * Tool use chunk event - emitted during tool call streaming
 */
export interface ToolUseChunkEvent extends StreamingEvent {
  type: StreamingEventType.ToolUseChunk;
  /** Tool call information */
  data: {
    /** Request identifier */
    requestId?: string;
    /** Tool call identifier */
    toolCallId?: string;
    /** Tool/function name */
    name?: string;
    /** Partial arguments (JSON string) */
    argumentsDelta?: string;
  };
}

/**
 * Usage event - emitted with token usage information
 */
export interface UsageEvent extends StreamingEvent {
  type: StreamingEventType.StreamChunk;
  /** Token usage information */
  data: {
    /** Request identifier */
    requestId?: string;
    /** Current usage totals */
    usage?: {
      /** Input tokens used */
      inputTokens?: bigint;
      /** Output tokens generated */
      outputTokens?: bigint;
      /** Total tokens used */
      totalTokens?: bigint;
    };
  };
}

/**
 * Error event - emitted when an error occurs during streaming
 */
export interface ErrorEvent extends StreamingEvent {
  type: StreamingEventType.StreamError;
  /** Error information */
  data: {
    /** Request identifier */
    requestId?: string;
    /** Error code */
    code?: string;
    /** Error message */
    message?: string;
    /** Whether the request can be retried */
    retryable?: boolean;
    /** Suggested fallback provider */
    suggestedProvider?: string;
  };
}

/**
 * Complete event - emitted when the stream completes successfully
 */
export interface CompleteEvent extends StreamingEvent {
  type: StreamingEventType.StreamEnd;
  /** Completion information */
  data: {
    /** Request identifier */
    requestId?: string;
    /** Final complete response */
    response?: {
      /** Response content */
      content?: string;
      /** Tool calls made by the LLM */
      toolCalls?: Array<{
        id?: string;
        name?: string;
        arguments?: string;
      }>;
      /** Token usage information */
      usage?: {
        inputTokens?: bigint;
        outputTokens?: bigint;
        totalTokens?: bigint;
      };
      /** Reason generation stopped */
      finishReason?: string;
      /** Provider that handled the request */
      provider?: string;
      /** Model that handled the request */
      model?: string;
    };
  };
}

/**
 * Union type of all possible streaming events
 */
export type ParsedStreamEvent =
  | ContentDeltaEvent
  | ToolUseChunkEvent
  | UsageEvent
  | ErrorEvent
  | CompleteEvent;

// =============================================================================
// SSE Parser Configuration
// =============================================================================

/**
 * Configuration options for the SSE parser
 */
export interface StreamParserOptions {
  /** Maximum buffer size for accumulated data (default: 1MB) */
  maxBufferSize?: number;
  /** Whether to include raw events (default: false) */
  includeRaw?: boolean;
  /** Custom event handler callback */
  onEvent?: (event: ParsedStreamEvent) => void;
  /** Error handler callback */
  onError?: (error: Error) => void;
}

/**
 * Default parser options
 */
const DEFAULT_PARSER_OPTIONS: Required<StreamParserOptions> = {
  maxBufferSize: 1024 * 1024, // 1MB
  includeRaw: false,
  onEvent: () => {},
  onError: () => {},
};

// =============================================================================
// SSE Parser Implementation
// =============================================================================

/**
 * Parses Server-Sent Events (SSE) streams from the LLM Gateway
 *
 * This class handles:
 * - SSE line parsing (data: prefix, [DONE] sentinel)
 * - JSON deserialization with error recovery
 * - Event type detection and normalization
 * - Buffer management for partial chunks
 */
export class SSEParser {
  private buffer = '';
  private options: Required<StreamParserOptions>;
  private streamId: string;
  private eventId = 0;

  /**
   * Creates a new SSE parser
   *
   * @param streamId - Unique identifier for this stream
   * @param options - Parser configuration options
   */
  constructor(streamId: string, options: StreamParserOptions = {}) {
    this.streamId = streamId;
    this.options = { ...DEFAULT_PARSER_OPTIONS, ...options };
  }

  /**
   * Parses incoming data chunks and emits events
   *
   * @param chunk - Raw data chunk from the stream
   * @returns Array of parsed events
   */
  parse(chunk: string): ParsedStreamEvent[] {
    const events: ParsedStreamEvent[] = [];

    // Add chunk to buffer
    this.buffer += chunk;

    // Check buffer size
    if (this.buffer.length > this.options.maxBufferSize) {
      const error = new Error(`Buffer size exceeded: ${this.buffer.length} bytes`);
      this.options.onError(error);
      this.buffer = '';
      return events;
    }

    // Split into lines
    const lines = this.buffer.split('\n');
    // Keep the last potentially incomplete line in the buffer
    this.buffer = lines.pop() ?? '';

    // Process each complete line
    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) continue;

      try {
        const event = this.parseLine(trimmed);
        if (event) {
          events.push(event);
          this.options.onEvent(event);
        }
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        this.options.onError(error);
      }
    }

    return events;
  }

  /**
   * Parses a single SSE line
   *
   * @param line - SSE line (e.g., "data: {...}")
   * @returns Parsed event or null if line should be ignored
   */
  private parseLine(line: string): ParsedStreamEvent | null {
    // Check for SSE data prefix
    if (!line.startsWith('data: ')) {
      return null;
    }

    // Extract and trim the data payload
    const data = line.slice(6).trim();

    // Check for stream end sentinel
    if (data === '[DONE]') {
      return this.createCompleteEvent();
    }

    // Parse JSON
    const response = JSON.parse(data) as StreamLLMResponse;

    // Convert to streaming event
    return this.toStreamingEvent(response);
  }

  /**
   * Converts a StreamLLMResponse to a ParsedStreamEvent
   *
   * @param response - Stream response from the gateway
   * @returns Parsed streaming event
   */
  private toStreamingEvent(response: StreamLLMResponse): ParsedStreamEvent | null {
    const payload = response.payload;
    if (!payload) return null;

    const baseEvent = {
      id: `${this.streamId}_${this.eventId++}`,
      streamId: this.streamId,
      timestamp: Date.now(),
    };

    if ('chunk' in payload && payload.chunk) {
      return {
        ...baseEvent,
        type: StreamingEventType.ContentDelta,
        data: {
          requestId: payload.chunk.requestId,
          content: payload.chunk.content,
          index: payload.chunk.index,
          isLast: payload.chunk.isLast,
        },
      };
    }

    if ('toolCall' in payload && payload.toolCall) {
      return {
        ...baseEvent,
        type: StreamingEventType.ToolUseChunk,
        data: {
          requestId: payload.toolCall.requestId,
          toolCallId: payload.toolCall.toolCallId,
          name: payload.toolCall.name,
          argumentsDelta: payload.toolCall.argumentsDelta,
        },
      };
    }

    if ('usage' in payload && payload.usage) {
      return {
        ...baseEvent,
        type: StreamingEventType.StreamChunk,
        data: {
          requestId: payload.usage.requestId,
          usage: payload.usage.usage,
        },
      };
    }

    if ('error' in payload && payload.error) {
      return {
        ...baseEvent,
        type: StreamingEventType.StreamError,
        data: {
          requestId: payload.error.requestId,
          code: payload.error.code,
          message: payload.error.message,
          retryable: payload.error.retryable,
          suggestedProvider: payload.error.suggestedProvider,
        },
      };
    }

    if ('complete' in payload && payload.complete) {
      return {
        ...baseEvent,
        type: StreamingEventType.StreamEnd,
        data: {
          requestId: payload.complete.requestId,
          response: payload.complete.response,
        },
      };
    }

    if ('heartbeat' in payload) {
      // Heartbeats are ignored
      return null;
    }

    return null;
  }

  /**
   * Creates a stream complete event for [DONE] sentinel
   */
  private createCompleteEvent(): CompleteEvent {
    return {
      id: `${this.streamId}_${this.eventId++}`,
      type: StreamingEventType.StreamEnd,
      streamId: this.streamId,
      timestamp: Date.now(),
      data: {
        requestId: undefined,
        response: undefined,
      },
    };
  }

  /**
   * Resets the parser state
   */
  reset(): void {
    this.buffer = '';
    this.eventId = 0;
  }

  /**
   * Returns the current buffer contents
   */
  getBuffer(): string {
    return this.buffer;
  }
}

// =============================================================================
// Streaming Event Handler Interface
// =============================================================================

/**
 * Interface for handling streaming events
 */
export interface StreamingEventHandler {
  /**
   * Handles a streaming event
   *
   * @param event - The streaming event to handle
   */
  handle(event: ParsedStreamEvent): void | Promise<void>;

  /**
   * Returns true if this handler can process the given event type
   *
   * @param eventType - The event type to check
   */
  canHandle(eventType: StreamingEventType): boolean;
}

/**
 * Function adapter for StreamingEventHandler
 */
export type StreamingEventFunc = (event: ParsedStreamEvent) => void | Promise<void>;

/**
 * Creates a StreamingEventHandler from a function
 *
 * @param fn - Function to handle events
 * @param canHandleFn - Optional function to check if event can be handled
 * @returns A StreamingEventHandler adapter
 */
export function createEventHandler(
  fn: StreamingEventFunc,
  canHandleFn?: (eventType: StreamingEventType) => boolean
): StreamingEventHandler {
  return {
    handle: fn,
    canHandle: canHandleFn ?? (() => true),
  };
}

// =============================================================================
// Streaming Event Filter
// =============================================================================

/**
 * Filter for selecting specific streaming events
 */
export interface StreamEventFilter {
  /** Event type to filter by (optional) */
  type?: StreamingEventType;
  /** Stream ID to filter by (optional) */
  streamId?: string;
  /** Request ID to filter by (optional) */
  requestId?: string;
}

/**
 * Tests if an event matches the given filter
 *
 * @param event - The event to test
 * @param filter - The filter to apply
 * @returns True if the event matches the filter
 */
export function eventMatchesFilter(event: ParsedStreamEvent, filter: StreamEventFilter): boolean {
  if (filter.type !== undefined && event.type !== filter.type) {
    return false;
  }
  if (filter.streamId !== undefined && event.streamId !== filter.streamId) {
    return false;
  }
  if (filter.requestId !== undefined) {
    const eventRequestId = 'requestId' in event.data ? event.data.requestId : undefined;
    if (eventRequestId !== filter.requestId) {
      return false;
    }
  }
  return true;
}

// =============================================================================
// Streaming Options
// =============================================================================

/**
 * Configuration options for streaming behavior
 */
export interface StreamingOptions {
  /** Buffer size for the chunk channel (default: 10) */
  bufferSize?: number;
  /** Whether to include token usage in chunks (default: true) */
  includeUsage?: boolean;
  /** Whether to include raw provider responses (default: false) */
  includeRaw?: boolean;
  /** Callback invoked for each chunk */
  callback?: (chunk: ParsedStreamEvent) => void | Promise<void>;
}

/**
 * Returns default streaming options
 */
export function defaultStreamingOptions(): StreamingOptions {
  return {
    bufferSize: 10,
    includeUsage: true,
    includeRaw: false,
  };
}

/**
 * Merges provided options with defaults
 *
 * @param options - Options to merge
 * @returns Merged options
 */
export function mergeStreamingOptions(options?: StreamingOptions): StreamingOptions {
  const defaults = defaultStreamingOptions();
  if (!options) {
    return defaults;
  }
  return {
    bufferSize: options.bufferSize ?? defaults.bufferSize,
    includeUsage: options.includeUsage ?? defaults.includeUsage,
    includeRaw: options.includeRaw ?? defaults.includeRaw,
    callback: options.callback,
  };
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Creates a unique stream ID
 *
 * @returns A unique stream identifier
 */
export function createStreamId(): string {
  return `stream_${Date.now()}_${Math.random().toString(36).substring(2, 11)}`;
}

/**
 * Checks if an event is a content delta event
 *
 * @param event - The event to check
 * @returns True if the event is a content delta
 */
export function isContentDeltaEvent(event: ParsedStreamEvent): event is ContentDeltaEvent {
  return event.type === StreamingEventType.ContentDelta;
}

/**
 * Checks if an event is a tool use chunk event
 *
 * @param event - The event to check
 * @returns True if the event is a tool use chunk
 */
export function isToolUseChunkEvent(event: ParsedStreamEvent): event is ToolUseChunkEvent {
  return event.type === StreamingEventType.ToolUseChunk;
}

/**
 * Checks if an event is a usage event
 *
 * @param event - The event to check
 * @returns True if the event is a usage event
 */
export function isUsageEvent(event: ParsedStreamEvent): event is UsageEvent {
  return event.type === StreamingEventType.StreamChunk && 'usage' in event.data;
}

/**
 * Checks if an event is an error event
 *
 * @param event - The event to check
 * @returns True if the event is an error event
 */
export function isErrorEvent(event: ParsedStreamEvent): event is ErrorEvent {
  return event.type === StreamingEventType.StreamError;
}

/**
 * Checks if an event is a complete event
 *
 * @param event - The event to check
 * @returns True if the event is a complete event
 */
export function isCompleteEvent(event: ParsedStreamEvent): event is CompleteEvent {
  return event.type === StreamingEventType.StreamEnd;
}
