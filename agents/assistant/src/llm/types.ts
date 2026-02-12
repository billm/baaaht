// types.ts - Type definitions for assistant LLM client
//
// This file contains TypeScript types for the assistant LLM client,
// which communicates with orchestrator-mediated LLMService over gRPC.
//
// Copyright 2026 baaaht project

import type {
  LLMRequest,
  LLMResponse,
  LLMMessage,
  LLMParameters,
  Tool,
  ToolCall,
  TokenUsage,
  FinishReason,
  LLMMetadata,
  StreamChunk,
  StreamToolCall,
  StreamUsage,
  StreamError,
  StreamComplete,
} from '../proto/llm.js';

// =============================================================================
// LLM Gateway Client Configuration
// =============================================================================

/**
 * Configuration for the assistant LLM client
 */
export interface LLMGatewayClientConfig {
  /**
   * Address of the orchestrator gRPC service
   * Defaults to 'localhost:50051'
   */
  baseURL?: string;

  /**
   * Request timeout in milliseconds
   * Defaults to 120000 (120 seconds)
   */
  timeout?: number;

  /**
   * Agent ID to include in request metadata
   */
  agentId?: string;

  /**
   * Container ID to include in request metadata
   */
  containerId?: string;

  /**
   * Additional labels for request tracking
   */
  labels?: Record<string, string>;

  /**
   * Default provider to use for requests
   */
  defaultProvider?: string;

  /**
   * Default fallback providers
   */
  fallbackProviders?: string[];
}

// =============================================================================
// Request/Response Types
// =============================================================================

/**
 * Parameters for generating an LLM completion
 */
export interface CompletionParams {
  /**
   * Model identifier (e.g., "anthropic/claude-sonnet-4-20250514")
   */
  model: string;

  /**
   * Conversation messages
   */
  messages: LLMMessage[];

  /**
   * Maximum tokens to generate
   */
  maxTokens: number;

  /**
   * Sampling temperature (0.0 to 1.0)
   */
  temperature?: number;

  /**
   * Nucleus sampling threshold
   */
  topP?: number;

  /**
   * Top-k sampling parameter
   */
  topK?: number;

  /**
   * Sequences that stop generation
   */
  stopSequences?: string[];

  /**
   * Available tools for the LLM
   */
  tools?: Tool[];

  /**
   * Provider override (optional)
   */
  provider?: string;

  /**
   * Fallback provider chain (optional)
   */
  fallbackProviders?: string[];

  /**
   * Session identifier for tracking
   */
  sessionId?: string;

  /**
   * Additional metadata
   */
  metadata?: Partial<LLMMetadata>;
}

/**
 * Result of a non-streaming completion request
 */
export interface CompletionResult {
  /**
   * Unique identifier for this request
   */
  requestId: string;

  /**
   * Response content
   */
  content: string;

  /**
   * Tool calls made by the LLM
   */
  toolCalls?: ToolCall[];

  /**
   * Token usage information
   */
  usage?: TokenUsage;

  /**
   * Reason generation stopped
   */
  finishReason?: FinishReason;

  /**
   * Provider that handled the request
   */
  provider: string;

  /**
   * Model that handled the request
   */
  model: string;

  /**
   * Request metadata echoed back
   */
  metadata?: LLMMetadata;
}

// =============================================================================
// Streaming Types
// =============================================================================

/**
 * A chunk in a streaming completion response
 */
export type StreamingChunk =
  | { type: 'content'; data: StreamChunk }
  | { type: 'toolCall'; data: StreamToolCall }
  | { type: 'usage'; data: StreamUsage }
  | { type: 'error'; data: StreamError }
  | { type: 'complete'; data: StreamComplete };

/**
 * Async iterator for streaming completion chunks
 */
export interface CompletionStream extends AsyncIterable<StreamingChunk> {
  /**
   * Abort the stream
   */
  abort(): void;
}

// =============================================================================
// Error Types
// =============================================================================

/**
 * Error codes for LLM Gateway failures
 */
export enum LLMGatewayErrorCode {
  // Request errors
  INVALID_REQUEST = 'INVALID_REQUEST',
  MODEL_NOT_SUPPORTED = 'MODEL_NOT_SUPPORTED',
  AUTHENTICATION_FAILED = 'AUTHENTICATION_FAILED',
  RATE_LIMITED = 'RATE_LIMITED',
  CONTEXT_TOO_LONG = 'CONTEXT_TOO_LONG',

  // Provider errors
  PROVIDER_NOT_AVAILABLE = 'PROVIDER_NOT_AVAILABLE',
  PROVIDER_ERROR = 'PROVIDER_ERROR',
  UPSTREAM_ERROR = 'UPSTREAM_ERROR',

  // Stream errors
  STREAM_ERROR = 'STREAM_ERROR',
  STREAM_TIMEOUT = 'STREAM_TIMEOUT',

  // Network errors
  NETWORK_ERROR = 'NETWORK_ERROR',
  TIMEOUT = 'TIMEOUT',

  // Unknown errors
  UNKNOWN = 'UNKNOWN',
}

/**
 * Error thrown when LLM Gateway request fails
 */
export class LLMGatewayError extends Error {
  constructor(
    message: string,
    public code: LLMGatewayErrorCode,
    public retryable: boolean = false,
    public suggestedProvider?: string
  ) {
    super(message);
    this.name = 'LLMGatewayError';
  }
}

// =============================================================================
// Health and Status Types
// =============================================================================

/**
 * Health check response from LLM Gateway
 */
export interface HealthCheckResult {
  /**
   * Health status
   */
  health: string;

  /**
   * Gateway version
   */
  version?: string;

  /**
   * Available providers
   */
  availableProviders: string[];

  /**
   * Unavailable providers
   */
  unavailableProviders: string[];

  /**
   * Timestamp of health check
   */
  timestamp: Date;
}

/**
 * Status response from LLM Gateway
 */
export interface GatewayStatus {
  /**
   * Operational status
   */
  status: string;

  /**
   * When the gateway started
   */
  startedAt?: Date;

  /**
   * Uptime duration
   */
  uptime?: { seconds: bigint; nanos: number };

  /**
   * Current active requests
   */
  activeRequests?: number;

  /**
   * Total requests handled
   */
  totalRequests?: bigint;

  /**
   * Total tokens used across all requests
   */
  totalTokensUsed?: bigint;

  /**
   * Status per provider
   */
  providers?: Record<string, ProviderStatus>;
}

/**
 * Status for a specific provider
 */
export interface ProviderStatus {
  /**
   * Whether the provider is available
   */
  available: boolean;

  /**
   * Provider health status
   */
  health: string;

  /**
   * Active requests for this provider
   */
  activeRequests?: number;

  /**
   * Total requests for this provider
   */
  totalRequests?: bigint;

  /**
   * Average response time in milliseconds
   */
  avgResponseTimeMs?: number;

  /**
   * Last request timestamp
   */
  lastRequestAt?: Date;
}

// =============================================================================
// Model Info Types
// =============================================================================

/**
 * Information about an available model
 */
export interface ModelInfo {
  /**
   * Model identifier
   */
  id: string;

  /**
   * Human-readable name
   */
  name: string;

  /**
   * Provider name
   */
  provider: string;

  /**
   * Model capabilities
   */
  capabilities: ModelCapabilities;

  /**
   * Additional model metadata
   */
  metadata?: Record<string, string>;
}

/**
 * Model capabilities
 */
export interface ModelCapabilities {
  /**
   * Supports streaming responses
   */
  streaming?: boolean;

  /**
   * Supports function/tool calling
   */
  tools?: boolean;

  /**
   * Supports image/vision input
   */
  vision?: boolean;

  /**
   * Supports extended thinking/reasoning
   */
  thinking?: boolean;
}
