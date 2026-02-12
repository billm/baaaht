// Shared types matching the protobuf definitions in proto/llm.proto

export interface LLMMessage {
  role: string;
  content: string;
  extra?: Record<string, string>;
}

export interface LLMParameters {
  max_tokens?: number;
  temperature?: number;
  top_p?: number;
  top_k?: number;
  stop_sequences?: string[];
  stream?: boolean;
}

export interface Tool {
  name: string;
  description: string;
  input_schema?: ToolInputSchema;
}

export interface ToolInputSchema {
  type: string;
  properties?: Record<string, ToolProperty>;
  required?: string[];
}

export interface ToolProperty {
  type: string;
  description?: string;
  enum?: string[];
}

export interface LLMMetadata {
  created_at?: string;
  agent_id?: string;
  container_id?: string;
  labels?: Record<string, string>;
  timeout_ms?: number;
  correlation_id?: string;
}

export interface LLMRequest {
  request_id: string;
  session_id?: string;
  model: string;
  messages: LLMMessage[];
  parameters?: LLMParameters;
  tools?: Tool[];
  provider?: string;
  fallback_providers?: string[];
  metadata?: LLMMetadata;
}

export interface ToolCall {
  id: string;
  name: string;
  arguments: string;
}

export interface TokenUsage {
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
}

export interface LLMResponse {
  request_id: string;
  content: string;
  tool_calls: ToolCall[];
  usage: TokenUsage;
  finish_reason: string;
  provider: string;
  model: string;
  completed_at: string;
  metadata?: LLMMetadata;
}

// Streaming types

export interface StreamChunk {
  type: "chunk";
  request_id: string;
  content: string;
  index: number;
  is_last: boolean;
}

export interface StreamToolCall {
  type: "tool_call";
  request_id: string;
  tool_call_id: string;
  name: string;
  arguments_delta: string;
}

export interface StreamUsage {
  type: "usage";
  request_id: string;
  usage: TokenUsage;
}

export interface StreamError {
  type: "error";
  request_id: string;
  code: string;
  message: string;
  retryable: boolean;
  suggested_provider?: string;
}

export interface StreamComplete {
  type: "complete";
  request_id: string;
  response: LLMResponse;
}

export type StreamEvent =
  | StreamChunk
  | StreamToolCall
  | StreamUsage
  | StreamError
  | StreamComplete;

// Status types

export interface ProviderStatus {
  name: string;
  available: boolean;
  health: string;
  total_requests: number;
  total_tokens: number;
  avg_response_time_ms: number;
  last_error?: string;
}

export interface GatewayStatus {
  status: string;
  started_at: string;
  uptime_ms: number;
  active_requests: number;
  total_requests: number;
  total_tokens_used: number;
  providers: Record<string, ProviderStatus>;
}
