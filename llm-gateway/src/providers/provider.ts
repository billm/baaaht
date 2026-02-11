// Provider interface - each LLM provider implements this.

import { LLMRequest, LLMResponse, StreamEvent, TokenUsage } from "../types";

export interface Provider {
  readonly name: string;

  /** Non-streaming completion */
  complete(request: LLMRequest): Promise<LLMResponse>;

  /** Streaming completion - yields events */
  stream(
    request: LLMRequest,
    onEvent: (event: StreamEvent) => void
  ): Promise<void>;

  /** List available models */
  listModels(): ModelInfo[];

  /** Check if this provider is available (has valid credentials) */
  isAvailable(): boolean;
}

export interface ModelInfo {
  id: string;
  name: string;
  provider: string;
  capabilities: {
    streaming: boolean;
    tools: boolean;
    vision: boolean;
    thinking: boolean;
  };
  metadata?: Record<string, string>;
}

/** Statistics tracked per‐provider */
export interface ProviderStats {
  totalRequests: number;
  totalTokens: number;
  totalErrors: number;
  avgResponseTimeMs: number;
  lastError?: string;
  responseTimes: number[]; // last N response times for running average
}

export function newProviderStats(): ProviderStats {
  return {
    totalRequests: 0,
    totalTokens: 0,
    totalErrors: 0,
    avgResponseTimeMs: 0,
    responseTimes: [],
  };
}

export function recordRequest(
  stats: ProviderStats,
  usage: TokenUsage,
  elapsedMs: number
): void {
  stats.totalRequests++;
  stats.totalTokens += usage.total_tokens;

  // Keep a sliding window of 100 response times
  stats.responseTimes.push(elapsedMs);
  if (stats.responseTimes.length > 100) {
    stats.responseTimes.shift();
  }
  stats.avgResponseTimeMs =
    stats.responseTimes.reduce((a, b) => a + b, 0) /
    stats.responseTimes.length;
}

export function recordError(stats: ProviderStats, err: string): void {
  stats.totalErrors++;
  stats.lastError = err;
}
