// gateway-client.ts - HTTP client for LLM Gateway communication
//
// This file provides a TypeScript HTTP client for communicating with the
// LLM Gateway service over HTTP with streaming support.
//
// Copyright 2026 baaaht project

import type {
  LLMRequest,
  LLMResponse,
  CompleteLLMRequest,
  CompleteLLMResponse,
  StreamLLMRequest,
  StreamLLMResponse,
} from '../proto/llm.js';
import type {
  LLMGatewayClientConfig,
  CompletionParams,
  CompletionResult,
  CompletionStream,
  StreamingChunk,
  HealthCheckResult,
  GatewayStatus,
  ProviderStatus,
  ModelInfo,
  ModelCapabilities,
} from './types.js';
import { LLMGatewayErrorCode, LLMGatewayError } from './types.js';

// Default configuration values
const DEFAULT_BASE_URL = 'http://localhost:8080';
const DEFAULT_TIMEOUT = 120000; // 120 seconds
const DEFAULT_STREAM_TIMEOUT = 300000; // 5 minutes for streaming

// Gateway endpoints
const ENDPOINTS = {
  COMPLETE: '/v1/llm/complete',
  STREAM: '/v1/llm/stream',
  HEALTH: '/v1/llm/health',
  STATUS: '/v1/llm/status',
  MODELS: '/v1/llm/models',
  CAPABILITIES: '/v1/llm/capabilities',
} as const;

/**
 * LLM Gateway HTTP Client
 *
 * Provides HTTP client for communicating with the LLM Gateway service.
 * Supports both streaming and non-streaming completions.
 *
 * Usage:
 * ```typescript
 * const client = new LLMGatewayClient({
 *   baseURL: 'http://localhost:8080',
 *   agentId: 'assistant-1',
 * });
 *
 * // Non-streaming
 * const result = await client.complete({
 *   model: 'anthropic/claude-sonnet-4-20250514',
 *   messages: [{ role: 'user', content: 'Hello!' }],
 *   maxTokens: 1024,
 * });
 *
 * // Streaming
 * for await (const chunk of client.stream({ ... })) {
 *   if (chunk.type === 'content') {
 *     process.stdout.write(chunk.data.content ?? '');
 *   }
 * }
 * ```
 */
export class LLMGatewayClient {
  private config: Required<Omit<LLMGatewayClientConfig, 'labels' | 'fallbackProviders'>> & {
    labels: Record<string, string>;
    fallbackProviders: string[];
  };

  /**
   * Creates a new LLM Gateway client
   */
  constructor(config: LLMGatewayClientConfig = {}) {
    this.config = {
      baseURL: config.baseURL ?? DEFAULT_BASE_URL,
      timeout: config.timeout ?? DEFAULT_TIMEOUT,
      agentId: config.agentId ?? '',
      containerId: config.containerId ?? '',
      labels: config.labels ?? {},
      defaultProvider: config.defaultProvider ?? '',
      fallbackProviders: config.fallbackProviders ?? [],
    };
  }

  /**
   * Generates a non-streaming completion
   *
   * @param params - Completion parameters
   * @param options - Request options
   * @returns Completion result
   * @throws LLMGatewayError on failure
   */
  async complete(
    params: CompletionParams,
    options: { timeout?: number; signal?: AbortSignal } = {}
  ): Promise<CompletionResult> {
    const requestId = this.generateRequestId();
    const request = this.buildRequest(requestId, params);

    const body: CompleteLLMRequest = { request };

    const timeout = options.timeout ?? this.config.timeout;

    try {
      const response = await this.fetch<CompleteLLMResponse>(
        ENDPOINTS.COMPLETE,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
          signal: options.signal,
        },
        timeout
      );

      if (!response.response) {
        throw this.createError('No response data received', 'UNKNOWN');
      }

      return this.toCompletionResult(requestId, response.response);
    } catch (err) {
      throw this.handleError(err);
    }
  }

  /**
   * Generates a streaming completion
   *
   * @param params - Completion parameters
   * @param options - Request options
   * @returns Async iterable of chunks
   * @throws LLMGatewayError on failure
   */
  stream(
    params: CompletionParams,
    options: { timeout?: number; signal?: AbortSignal } = {}
  ): CompletionStream {
    const requestId = this.generateRequestId();
    const request = this.buildRequest(requestId, params);
    const timeout = options.timeout ?? DEFAULT_STREAM_TIMEOUT;

    let controller: AbortController | null = null;
    if (!options.signal) {
      controller = new AbortController();
      options.signal = controller.signal;
    }

    const streamIterator = this.createStreamIterator(requestId, request, timeout, options.signal);

    return {
      [Symbol.asyncIterator]: () => streamIterator,
      abort: () => {
        controller?.abort();
      },
    };
  }

  /**
   * Checks the health of the LLM Gateway
   *
   * @returns Health check result
   * @throws LLMGatewayError on failure
   */
  async healthCheck(): Promise<HealthCheckResult> {
    try {
      const response = await this.fetch<{ health?: string; version?: string; availableProviders?: string[]; unavailableProviders?: string[]; timestamp?: string }>(
        ENDPOINTS.HEALTH,
        { method: 'GET' },
        this.config.timeout
      );

      return {
        health: response.health ?? 'UNKNOWN',
        version: response.version,
        availableProviders: response.availableProviders ?? [],
        unavailableProviders: response.unavailableProviders ?? [],
        timestamp: response.timestamp ? new Date(response.timestamp) : new Date(),
      };
    } catch (err) {
      throw this.handleError(err);
    }
  }

  /**
   * Gets the current status of the LLM Gateway
   *
   * @returns Gateway status
   * @throws LLMGatewayError on failure
   */
  async getStatus(): Promise<GatewayStatus> {
    try {
      const response = await this.fetch<any>(
        ENDPOINTS.STATUS,
        { method: 'GET' },
        this.config.timeout
      );

      return {
        status: response.status ?? 'UNKNOWN',
        startedAt: response.startedAt ? new Date(response.startedAt) : undefined,
        uptime: response.uptime,
        activeRequests: response.activeRequests,
        totalRequests: response.totalRequests ? BigInt(response.totalRequests) : undefined,
        totalTokensUsed: response.totalTokensUsed ? BigInt(response.totalTokensUsed) : undefined,
        providers: response.providers as Record<string, ProviderStatus> | undefined,
      };
    } catch (err) {
      throw this.handleError(err);
    }
  }

  /**
   * Lists available models
   *
   * @param provider - Optional provider filter
   * @returns List of model information
   * @throws LLMGatewayError on failure
   */
  async listModels(provider?: string): Promise<ModelInfo[]> {
    try {
      const url = provider ? `${ENDPOINTS.MODELS}?provider=${encodeURIComponent(provider)}` : ENDPOINTS.MODELS;
      const response = await this.fetch<{ models?: any[] }>(
        url,
        { method: 'GET' },
        this.config.timeout
      );

      return (response.models ?? []).map((m: any) => ({
        id: m.id ?? '',
        name: m.name ?? m.id ?? '',
        provider: m.provider ?? '',
        capabilities: m.capabilities as ModelCapabilities ?? { streaming: true },
        metadata: m.metadata as Record<string, string> | undefined,
      }));
    } catch (err) {
      throw this.handleError(err);
    }
  }

  /**
   * Gets provider capabilities
   *
   * @param provider - Optional specific provider
   * @returns Provider capabilities
   * @throws LLMGatewayError on failure
   */
  async getCapabilities(provider?: string): Promise<{ name: string; available: boolean; models?: ModelInfo[]; metadata?: Record<string, string> }[]> {
    try {
      const url = provider
        ? `${ENDPOINTS.CAPABILITIES}?provider=${encodeURIComponent(provider)}`
        : ENDPOINTS.CAPABILITIES;
      const response = await this.fetch<{ providers?: any[] }>(
        url,
        { method: 'GET' },
        this.config.timeout
      );

      return (response.providers ?? []).map((p: any) => ({
        name: p.name ?? '',
        available: p.available ?? false,
        models: p.models?.map((m: any) => ({
          id: m.id ?? '',
          name: m.name ?? m.id ?? '',
          provider: p.name ?? '',
          capabilities: m.capabilities as ModelCapabilities ?? {},
          metadata: m.metadata as Record<string, string> | undefined,
        })),
        metadata: p.metadata as Record<string, string> | undefined,
      }));
    } catch (err) {
      throw this.handleError(err);
    }
  }

  // =============================================================================
  // Private Methods
  // =============================================================================

  /**
   * Creates an async iterator for streaming responses
   */
  private async *createStreamIterator(
    requestId: string,
    request: LLMRequest,
    timeout: number,
    signal: AbortSignal
  ): AsyncIterator<StreamingChunk> {
    const body: StreamLLMRequest = { payload: { request } };

    try {
      const response = await fetch(`${this.config.baseURL}${ENDPOINTS.STREAM}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        signal,
      });

      if (!response.ok) {
        throw this.createError(
          `HTTP ${response.status}: ${response.statusText}`,
          this.mapHttpStatusToErrorCode(response.status)
        );
      }

      if (!response.body) {
        throw this.createError('No response body', 'NETWORK_ERROR');
      }

      // Parse SSE stream
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      try {
        while (true) {
          // Check for abort
          if (signal.aborted) {
            reader.cancel();
            return;
          }

          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() ?? '';

          for (const line of lines) {
            if (line.startsWith('data: ')) {
              const data = line.slice(6).trim();
              if (data === '[DONE]') {
                return;
              }

              try {
                const json = JSON.parse(data) as StreamLLMResponse;
                const chunk = this.parseStreamChunk(requestId, json);
                if (chunk) {
                  yield chunk;
                }

                // End of stream
                if (chunk?.type === 'complete') {
                  return;
                }
              } catch {
                // Ignore invalid JSON
              }
            }
          }
        }
      } finally {
        reader.releaseLock();
      }
    } catch (err) {
      if (signal.aborted) {
        return;
      }
      throw this.handleError(err);
    }
  }

  /**
   * Parses a stream chunk from the response
   */
  private parseStreamChunk(requestId: string, response: StreamLLMResponse): StreamingChunk | null {
    const payload = response.payload;
    if (!payload) return null;

    if ('chunk' in payload && payload.chunk) {
      return { type: 'content', data: payload.chunk };
    }
    if ('toolCall' in payload && payload.toolCall) {
      return { type: 'toolCall', data: payload.toolCall };
    }
    if ('usage' in payload && payload.usage) {
      return { type: 'usage', data: payload.usage };
    }
    if ('error' in payload && payload.error) {
      const error = payload.error;
      return {
        type: 'error',
        data: error,
      };
    }
    if ('complete' in payload && payload.complete) {
      return { type: 'complete', data: payload.complete };
    }
    if ('heartbeat' in payload) {
      // Skip heartbeats
      return null;
    }

    return null;
  }

  /**
   * Builds an LLM request from completion parameters
   */
  private buildRequest(requestId: string, params: CompletionParams): LLMRequest {
    const request: LLMRequest = {
      requestId,
      model: params.model,
      messages: params.messages,
      parameters: {
        maxTokens: params.maxTokens,
        temperature: params.temperature,
        topP: params.topP,
        topK: params.topK,
        stopSequences: params.stopSequences,
        stream: false,
      },
      metadata: {
        createdAt: new Date(),
        agentId: this.config.agentId || params.metadata?.agentId,
        containerId: this.config.containerId || params.metadata?.containerId,
        labels: { ...this.config.labels, ...params.metadata?.labels },
        correlationId: params.metadata?.correlationId ?? requestId,
      },
    };

    if (params.tools && params.tools.length > 0) {
      request.tools = params.tools;
    }

    if (params.provider || this.config.defaultProvider) {
      request.provider = params.provider ?? this.config.defaultProvider;
    }

    if (params.fallbackProviders || this.config.fallbackProviders.length > 0) {
      request.fallbackProviders = params.fallbackProviders ?? this.config.fallbackProviders;
    }

    if (params.sessionId) {
      request.sessionId = params.sessionId;
    }

    return request;
  }

  /**
   * Converts LLMResponse to CompletionResult
   */
  private toCompletionResult(requestId: string, response: LLMResponse): CompletionResult {
    return {
      requestId: response.requestId ?? requestId,
      content: response.content ?? '',
      toolCalls: response.toolCalls,
      usage: response.usage,
      finishReason: response.finishReason,
      provider: response.provider ?? 'unknown',
      model: response.model ?? '',
      metadata: response.metadata,
    };
  }

  /**
   * Performs an HTTP fetch with timeout
   */
  private async fetch<T>(
    endpoint: string,
    init: RequestInit & { signal?: AbortSignal },
    timeout: number
  ): Promise<T> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), timeout);

    // Chain the abort signals
    const signal = init.signal
      ? AbortSignal.any([init.signal, controller.signal])
      : controller.signal;

    try {
      const response = await fetch(`${this.config.baseURL}${endpoint}`, {
        ...init,
        signal,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        throw this.createError(
          `HTTP ${response.status}: ${response.statusText}`,
          this.mapHttpStatusToErrorCode(response.status)
        );
      }

      const contentType = response.headers.get('content-type');
      if (contentType?.includes('application/json')) {
        return await response.json() as T;
      }

      return {} as T;
    } catch (err) {
      clearTimeout(timeoutId);
      throw err;
    }
  }

  /**
   * Generates a unique request ID
   */
  private generateRequestId(): string {
    return `req_${Date.now()}_${Math.random().toString(36).substring(2, 11)}`;
  }

  /**
   * Maps HTTP status code to error code
   */
  private mapHttpStatusToErrorCode(status: number): LLMGatewayErrorCode {
    switch (status) {
      case 400:
        return 'INVALID_REQUEST';
      case 401:
        return 'AUTHENTICATION_FAILED';
      case 429:
        return 'RATE_LIMITED';
      case 413:
        return 'CONTEXT_TOO_LONG';
      case 502:
      case 503:
      case 504:
        return 'PROVIDER_NOT_AVAILABLE';
      default:
        return 'UPSTREAM_ERROR';
    }
  }

  /**
   * Creates an LLMGatewayError from an unknown error
   */
  private createError(message: string, code: LLMGatewayErrorCode, retryable: boolean = false): LLMGatewayError {
    const error = new Error(message) as LLMGatewayError;
    error.code = code;
    error.retryable = retryable;
    error.name = 'LLMGatewayError';
    return error;
  }

  /**
   * Handles and converts errors to LLMGatewayError
   */
  private handleError(err: unknown): LLMGatewayError {
    if (this.isLLMGatewayError(err)) {
      return err;
    }

    if (err instanceof Error) {
      if (err.name === 'AbortError' || err.name === 'TypeError') {
        const error = new Error(err.message) as LLMGatewayError;
        error.code = err.name === 'AbortError' ? 'TIMEOUT' : 'NETWORK_ERROR';
        error.name = 'LLMGatewayError';
        error.retryable = err.name === 'AbortError';
        return error;
      }
    }

    const error = new Error(
      err instanceof Error ? err.message : 'Unknown error occurred'
    ) as LLMGatewayError;
    error.code = 'UNKNOWN';
    error.name = 'LLMGatewayError';
    return error;
  }

  /**
   * Type guard for LLMGatewayError
   */
  private isLLMGatewayError(err: unknown): err is LLMGatewayError {
    return (
      typeof err === 'object' &&
      err !== null &&
      'name' in err &&
      err.name === 'LLMGatewayError' &&
      'code' in err
    );
  }
}

// =============================================================================
// Factory Functions
// =============================================================================

/**
 * Creates a new LLM Gateway client with default configuration
 *
 * @param config - Optional client configuration
 * @returns Configured LLM Gateway client
 */
export function createLLMGatewayClient(config?: LLMGatewayClientConfig): LLMGatewayClient {
  return new LLMGatewayClient(config);
}
