// llm.ts - TypeScript types for LLM protocol buffer definitions
//
// This file contains TypeScript types for the LLMService which provides
// streaming LLM request handling, token tracking, and provider abstraction.
// Auto-generated from proto/llm.proto
//
// Copyright 2026 baaaht project

import type {
  Health,
  ModelCapabilities,
  Status,
  TokenUsage,
} from './common.js';
export type { TokenUsage } from './common.js';

// =============================================================================
// LLM Request/Response Types
// =============================================================================

/**
 * LLMRequest represents a request to an LLM provider
 */
export interface LLMRequest {
  requestId?: string;                 // Unique identifier for this request
  sessionId?: string;                 // Session identifier for tracking
  model?: string;                     // Model identifier
  messages?: LLMMessage[];            // Conversation messages
  parameters?: LLMParameters;         // Generation parameters
  tools?: Tool[];                     // Available tools for the LLM
  provider?: string;                  // Provider override (optional)
  fallbackProviders?: string[];       // Fallback provider chain (optional)
  metadata?: LLMMetadata;             // Additional metadata
}

/**
 * LLMMessage represents a message in the conversation
 */
export interface LLMMessage {
  role?: string;                      // Role: "user", "assistant", "system"
  content?: string;                   // Message content
  extra?: Record<string, string>;     // Additional fields (e.g., cache_control)
}

/**
 * LLMParameters contains generation parameters for the LLM
 */
export interface LLMParameters {
  maxTokens?: number;                 // Maximum tokens to generate
  temperature?: number;               // Sampling temperature (0.0 to 1.0)
  topP?: number;                      // Nucleus sampling threshold
  topK?: number;                      // Top-k sampling parameter
  stopSequences?: string[];           // Sequences that stop generation
  stream?: boolean;                   // Whether to stream responses
}

/**
 * Tool represents a tool/function available to the LLM
 */
export interface Tool {
  name?: string;                      // Tool name
  description?: string;               // Tool description
  inputSchema?: ToolInputSchema;      // JSON schema for tool input
}

/**
 * ToolInputSchema represents the JSON schema for tool input
 */
export interface ToolInputSchema {
  type?: string;                      // JSON Schema type (e.g., "object")
  properties?: Record<string, ToolProperty>; // Input properties
  required?: string[];                // Required properties
}

/**
 * ToolProperty represents a property in the tool input schema
 */
export interface ToolProperty {
  type?: string;                      // Property type
  description?: string;               // Property description
  enum?: string[];                    // Enum values (if applicable)
}

/**
 * LLMMetadata contains additional metadata for LLM requests
 */
export interface LLMMetadata {
  createdAt?: Date;
  agentId?: string;                   // Optional: Agent making the request
  containerId?: string;               // Optional: Container making the request
  labels?: Record<string, string>;    // Optional labels for tracking
  timeoutMs?: bigint;                 // Request timeout in milliseconds
  correlationId?: string;             // For correlating requests/responses
}

/**
 * LLMResponse represents a response from an LLM provider
 */
export interface LLMResponse {
  requestId?: string;                 // Correlates with the request
  content?: string;                   // Response content
  toolCalls?: ToolCall[];             // Tool calls made by the LLM
  usage?: TokenUsage;                 // Token usage information
  finishReason?: FinishReason;        // Reason generation stopped
  provider?: string;                  // Provider that handled the request
  model?: string;                     // Model that handled the request
  completedAt?: Date;
  metadata?: LLMMetadata;             // Request metadata echoed back
}

/**
 * ToolCall represents a tool call made by the LLM
 */
export interface ToolCall {
  id?: string;                        // Tool call ID
  name?: string;                      // Tool/function name
  arguments?: string;                 // JSON string of arguments
}

/**
 * FinishReason represents the reason generation stopped
 */
export enum FinishReason {
  FINISH_REASON_UNSPECIFIED = 0,
  FINISH_REASON_STOP = 1,             // Natural stop
  FINISH_REASON_LENGTH = 2,           // Max tokens reached
  FINISH_REASON_TOOL_USES = 3,        // Tool use requested
  FINISH_REASON_ERROR = 4,            // Error occurred
  FINISH_REASON_FILTER = 5,           // Content filtered
}

// =============================================================================
// Streaming Types
// =============================================================================

/**
 * StreamLLMRequest is a request in the streaming LLM flow
 */
export interface StreamLLMRequest {
  payload?: StreamLLMRequestPayload;
}

/**
 * StreamLLMRequestPayload represents the payload of a stream LLM request (oneof field)
 */
export type StreamLLMRequestPayload =
  | { request: LLMRequest }
  | { heartbeat: null };

/**
 * StreamLLMResponse is a response in the streaming LLM flow
 */
export interface StreamLLMResponse {
  payload?: StreamLLMResponsePayload;
}

/**
 * StreamLLMResponsePayload represents the payload of a stream LLM response (oneof field)
 */
export type StreamLLMResponsePayload =
  | { chunk: StreamChunk }
  | { toolCall: StreamToolCall }
  | { usage: StreamUsage }
  | { error: StreamError }
  | { complete: StreamComplete }
  | { heartbeat: null };

/**
 * StreamChunk represents a content chunk in streaming response
 */
export interface StreamChunk {
  requestId?: string;
  content?: string;                   // Content delta
  index?: number;                     // Chunk index (for ordering)
  isLast?: boolean;                   // Whether this is the last content chunk
}

/**
 * StreamToolCall represents a tool call during streaming
 */
export interface StreamToolCall {
  requestId?: string;
  toolCallId?: string;
  name?: string;
  argumentsDelta?: string;            // Partial arguments (JSON string)
}

/**
 * StreamUsage represents token usage during streaming
 */
export interface StreamUsage {
  requestId?: string;
  usage?: TokenUsage;                 // Current usage totals
}

/**
 * StreamError represents an error during streaming
 */
export interface StreamError {
  requestId?: string;
  code?: string;                      // Error code
  message?: string;                   // Error message
  retryable?: boolean;                // Whether the request can be retried
  suggestedProvider?: string;         // Optional: Suggested fallback provider
}

/**
 * StreamComplete indicates the stream has completed
 */
export interface StreamComplete {
  requestId?: string;
  response?: LLMResponse;             // Final complete response
  finishReason?: FinishReason;
}

// =============================================================================
// Non-Streaming Types
// =============================================================================

/**
 * CompleteLLMRequest is a non-streaming LLM request
 */
export interface CompleteLLMRequest {
  request?: LLMRequest;
}

/**
 * CompleteLLMResponse is a non-streaming LLM response
 */
export interface CompleteLLMResponse {
  response?: LLMResponse;
}

// =============================================================================
// Configuration and Capabilities Types
// =============================================================================

/**
 * ListModelsRequest requests available models
 */
export interface ListModelsRequest {
  provider?: string;                  // Optional: Filter by provider
}

/**
 * ListModelsResponse returns available models
 */
export interface ListModelsResponse {
  models?: ModelInfo[];
}

/**
 * ModelInfo contains information about a model
 */
export interface ModelInfo {
  id?: string;                        // Model identifier
  name?: string;                      // Human-readable name
  provider?: string;                  // Provider name
  capabilities?: ModelCapabilities;   // Model capabilities
  metadata?: Record<string, string>;  // Additional model metadata
}

/**
 * GetCapabilitiesRequest requests provider capabilities
 */
export interface GetCapabilitiesRequest {
  provider?: string;                  // Optional: Specific provider
}

/**
 * GetCapabilitiesResponse returns provider capabilities
 */
export interface GetCapabilitiesResponse {
  providers?: ProviderCapabilities[];
}

/**
 * ProviderCapabilities describes a provider's capabilities
 */
export interface ProviderCapabilities {
  name?: string;                      // Provider name
  available?: boolean;                // Whether provider is configured
  models?: ModelInfo[];               // Available models
  metadata?: Record<string, string>;  // Additional provider metadata
}

// =============================================================================
// Health and Status Types
// =============================================================================

/**
 * LLMHealthCheckResponse contains health check information
 */
export interface LLMHealthCheckResponse {
  health?: Health;
  version?: string;
  availableProviders?: string[];
  unavailableProviders?: string[];
  timestamp?: Date;
}

/**
 * LLMStatusResponse contains status information
 */
export interface LLMStatusResponse {
  status?: Status;
  startedAt?: Date;
  uptime?: { seconds: bigint; nanos: number };
  activeRequests?: number;
  totalRequests?: bigint;
  totalTokensUsed?: bigint;           // Total tokens across all requests
  providers?: Record<string, ProviderStatus>;
}

/**
 * ProviderStatus contains status for a specific provider
 */
export interface ProviderStatus {
  available?: boolean;
  health?: Health;
  activeRequests?: number;
  totalRequests?: bigint;
  avgResponseTimeMs?: number;         // Average response time
  lastRequestAt?: Date;
}
