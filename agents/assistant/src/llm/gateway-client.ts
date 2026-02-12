// gateway-client.ts - gRPC client for LLM service communication
//
// This file provides a TypeScript gRPC client for communicating with the
// orchestrator-mediated LLMService with streaming support.
//
// Copyright 2026 baaaht project

import {
  credentials,
  loadPackageDefinition,
  type ChannelCredentials,
  type Client,
  type ClientDuplexStream,
} from '@grpc/grpc-js';
import { loadSync } from '@grpc/proto-loader';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';
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
const DEFAULT_BASE_URL = 'localhost:50051';
const DEFAULT_TIMEOUT = 120000; // 120 seconds
const DEFAULT_STREAM_TIMEOUT = 300000; // 5 minutes for streaming

type LLMServiceClient = Client & {
  completeLLM?: (
    request: CompleteLLMRequest,
    options: { deadline: Date },
    callback: (err: Error | null, response: CompleteLLMResponse) => void
  ) => void;
  streamLLM?: () => ClientDuplexStream<StreamLLMRequest, StreamLLMResponse>;
  listModels?: (
    request: { provider?: string },
    options: { deadline: Date },
    callback: (err: Error | null, response: { models?: Array<{ id?: string; name?: string; provider?: string; capabilities?: ModelCapabilities; metadata?: Record<string, string> }> }) => void
  ) => void;
  getCapabilities?: (
    request: { provider?: string },
    options: { deadline: Date },
    callback: (err: Error | null, response: { providers?: Array<{ name?: string; available?: boolean; models?: Array<{ id?: string; name?: string; provider?: string; capabilities?: ModelCapabilities; metadata?: Record<string, string> }>; metadata?: Record<string, string> }> }) => void
  ) => void;
  healthCheck?: (
    request: Record<string, never>,
    options: { deadline: Date },
    callback: (err: Error | null, response: { health?: string | number; version?: string; availableProviders?: string[]; unavailableProviders?: string[]; timestamp?: Date }) => void
  ) => void;
  getStatus?: (
    request: Record<string, never>,
    options: { deadline: Date },
    callback: (err: Error | null, response: { status?: string | number; startedAt?: Date; uptime?: { seconds: string | bigint; nanos: number }; activeRequests?: number; totalRequests?: string | bigint; totalTokensUsed?: string | bigint; providers?: Record<string, ProviderStatus> }) => void
  ) => void;
};

/**
 * Assistant LLM gRPC Client
 *
 * Provides a gRPC client for communicating with orchestrator LLMService.
 * Supports both streaming and non-streaming completions.
 *
 * Usage:
 * ```typescript
 * const client = new LLMGatewayClient({
 *   baseURL: 'localhost:50051',
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
  private credentials: ChannelCredentials;
  private client: LLMServiceClient | null = null;
  private connected: boolean = false;
  private connectPromise: Promise<void> | null = null;

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

    this.credentials = credentials.createInsecure();
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
    await this.connect();

    const requestId = this.generateRequestId();
    const request = this.buildRequest(requestId, params);

    const body: CompleteLLMRequest = { request };

    const timeout = options.timeout ?? this.config.timeout;

    try {
      const response = await this.callComplete(body, timeout, options.signal);

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

    const controller = new AbortController();
    const signal = options.signal
      ? AbortSignal.any([options.signal, controller.signal])
      : controller.signal;

    const timeoutId = setTimeout(() => controller.abort(), timeout);

    const streamIterator = this.createStreamIterator(requestId, request, signal, timeoutId);

    return {
      [Symbol.asyncIterator]: () => streamIterator,
      abort: () => {
        clearTimeout(timeoutId);
        controller.abort();
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
    await this.connect();

    try {
      const response = await this.callHealthCheck(this.config.timeout);

      return {
        health: String(response.health ?? 'UNKNOWN'),
        version: response.version,
        availableProviders: response.availableProviders ?? [],
        unavailableProviders: response.unavailableProviders ?? [],
        timestamp: response.timestamp ?? new Date(),
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
    await this.connect();

    try {
      const response = await this.callGetStatus(this.config.timeout);

      return {
        status: String(response.status ?? 'UNKNOWN'),
        startedAt: response.startedAt,
        uptime: response.uptime,
        activeRequests: response.activeRequests,
        totalRequests: response.totalRequests !== undefined ? BigInt(response.totalRequests) : undefined,
        totalTokensUsed: response.totalTokensUsed !== undefined ? BigInt(response.totalTokensUsed) : undefined,
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
    await this.connect();

    try {
      const response = await this.callListModels(provider, this.config.timeout);

      return (response.models ?? []).map((m) => ({
        id: m.id ?? '',
        name: m.name ?? m.id ?? '',
        provider: m.provider ?? '',
        capabilities: m.capabilities ?? { streaming: true },
        metadata: m.metadata,
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
    await this.connect();

    try {
      const response = await this.callGetCapabilities(provider, this.config.timeout);

      return (response.providers ?? []).map((p) => ({
        name: p.name ?? '',
        available: p.available ?? false,
        models: p.models?.map((m) => ({
          id: m.id ?? '',
          name: m.name ?? m.id ?? '',
          provider: p.name ?? '',
          capabilities: m.capabilities ?? {},
          metadata: m.metadata,
        })),
        metadata: p.metadata,
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
    signal: AbortSignal,
    timeoutId: ReturnType<typeof setTimeout>
  ): AsyncIterator<StreamingChunk> {
    const body: StreamLLMRequest = { payload: { request } };

    try {
      await this.connect();
      const grpcStream = this.callStreamLLM();

      const queue: StreamingChunk[] = [];
      let finished = false;
      let streamError: unknown;
      let notify: (() => void) | null = null;

      const wake = () => {
        if (notify) {
          notify();
          notify = null;
        }
      };

      const onData = (response: StreamLLMResponse) => {
        const chunk = this.parseStreamChunk(requestId, response);
        if (!chunk) {
          return;
        }

        queue.push(chunk);
        if (chunk.type === 'complete') {
          finished = true;
        }
        wake();
      };

      const onError = (err: unknown) => {
        streamError = err;
        finished = true;
        wake();
      };

      const onEnd = () => {
        finished = true;
        wake();
      };

      const onAbort = () => {
        grpcStream.cancel();
        finished = true;
        wake();
      };

      grpcStream.on('data', onData);
      grpcStream.on('error', onError);
      grpcStream.on('end', onEnd);

      signal.addEventListener('abort', onAbort, { once: true });
      grpcStream.write(body);

      try {
        while (!finished || queue.length > 0) {
          if (signal.aborted) {
            return;
          }

          if (queue.length > 0) {
            const next = queue.shift();
            if (next) {
              yield next;
            }
            continue;
          }

          await new Promise<void>((resolve) => {
            notify = resolve;
          });
        }

        if (streamError && !signal.aborted) {
          throw streamError;
        }
      } finally {
        clearTimeout(timeoutId);
        signal.removeEventListener('abort', onAbort);
        grpcStream.off('data', onData);
        grpcStream.off('error', onError);
        grpcStream.off('end', onEnd);
        grpcStream.end();
      }
    } catch (err) {
      clearTimeout(timeoutId);
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
   * Connects to the LLM service
   */
  private async connect(): Promise<void> {
    if (this.connected) {
      return;
    }

    if (this.connectPromise) {
      await this.connectPromise;
      return;
    }

    this.connectPromise = new Promise<void>((resolve, reject) => {
      try {
        const protoPath = this.resolveProtoPath();
        const protoIncludeDir = path.dirname(protoPath);

        const packageDefinition = loadSync(
          protoPath,
          {
            keepCase: false,
            longs: String,
            enums: String,
            defaults: false,
            oneofs: true,
            includeDirs: [protoIncludeDir],
          }
        );

        const protoDescriptor = loadPackageDefinition(packageDefinition) as {
          llm?: {
            v1?: {
              LLMService?: new (
                address: string,
                creds: ChannelCredentials,
                options: Record<string, unknown>
              ) => LLMServiceClient;
            };
          };
        };

        const llmService = protoDescriptor.llm?.v1?.LLMService;
        if (!llmService) {
          throw new Error('LLMService not found in loaded proto descriptor');
        }

        const address = this.resolveAddress(this.config.baseURL);
        this.client = new llmService(
          address,
          this.credentials,
          {
            'grpc.max_receive_message_length': -1,
            'grpc.max_send_message_length': -1,
          }
        );

        const deadline = this.getDeadline(this.config.timeout);
        this.client.waitForReady(deadline, (err) => {
          if (err) {
            this.client = null;
            this.connected = false;
            reject(new Error(`Failed to connect to LLM service: ${err.message}`));
            return;
          }

          this.connected = true;
          resolve();
        });
      } catch (err) {
        const error = err as Error;
        reject(new Error(`Failed to create LLM gRPC client: ${error.message}`));
      }
    });

    try {
      await this.connectPromise;
    } finally {
      this.connectPromise = null;
    }
  }

  private callComplete(
    request: CompleteLLMRequest,
    timeout: number,
    signal?: AbortSignal
  ): Promise<CompleteLLMResponse> {
    return new Promise((resolve, reject) => {
      const client = this.requireClient();
      const method = client.completeLLM;
      if (!method) {
        reject(new Error('completeLLM method unavailable on LLM gRPC client'));
        return;
      }

      if (signal?.aborted) {
        reject(this.createError('Request aborted', LLMGatewayErrorCode.TIMEOUT, true));
        return;
      }

      const onAbort = () => {
        reject(this.createError('Request aborted', LLMGatewayErrorCode.TIMEOUT, true));
      };
      signal?.addEventListener('abort', onAbort, { once: true });

      method.call(client, request, { deadline: this.getDeadline(timeout) }, (err, response) => {
        signal?.removeEventListener('abort', onAbort);
        if (err) {
          reject(err);
          return;
        }
        resolve(response);
      });
    });
  }

  private callStreamLLM(): ClientDuplexStream<StreamLLMRequest, StreamLLMResponse> {
    const client = this.requireClient();
    const method = client.streamLLM;
    if (!method) {
      throw new Error('streamLLM method unavailable on LLM gRPC client');
    }
    return method.call(client);
  }

  private callHealthCheck(timeout: number): Promise<{ health?: string | number; version?: string; availableProviders?: string[]; unavailableProviders?: string[]; timestamp?: Date }> {
    return new Promise((resolve, reject) => {
      const client = this.requireClient();
      const method = client.healthCheck;
      if (!method) {
        reject(new Error('healthCheck method unavailable on LLM gRPC client'));
        return;
      }

      method.call(client, {}, { deadline: this.getDeadline(timeout) }, (err, response) => {
        if (err) {
          reject(err);
          return;
        }
        resolve(response);
      });
    });
  }

  private callGetStatus(timeout: number): Promise<{ status?: string | number; startedAt?: Date; uptime?: { seconds: string | bigint; nanos: number }; activeRequests?: number; totalRequests?: string | bigint; totalTokensUsed?: string | bigint; providers?: Record<string, ProviderStatus> }> {
    return new Promise((resolve, reject) => {
      const client = this.requireClient();
      const method = client.getStatus;
      if (!method) {
        reject(new Error('getStatus method unavailable on LLM gRPC client'));
        return;
      }

      method.call(client, {}, { deadline: this.getDeadline(timeout) }, (err, response) => {
        if (err) {
          reject(err);
          return;
        }
        resolve(response);
      });
    });
  }

  private callListModels(provider: string | undefined, timeout: number): Promise<{ models?: Array<{ id?: string; name?: string; provider?: string; capabilities?: ModelCapabilities; metadata?: Record<string, string> }> }> {
    return new Promise((resolve, reject) => {
      const client = this.requireClient();
      const method = client.listModels;
      if (!method) {
        reject(new Error('listModels method unavailable on LLM gRPC client'));
        return;
      }

      method.call(client, { provider }, { deadline: this.getDeadline(timeout) }, (err, response) => {
        if (err) {
          reject(err);
          return;
        }
        resolve(response);
      });
    });
  }

  private callGetCapabilities(provider: string | undefined, timeout: number): Promise<{ providers?: Array<{ name?: string; available?: boolean; models?: Array<{ id?: string; name?: string; provider?: string; capabilities?: ModelCapabilities; metadata?: Record<string, string> }>; metadata?: Record<string, string> }> }> {
    return new Promise((resolve, reject) => {
      const client = this.requireClient();
      const method = client.getCapabilities;
      if (!method) {
        reject(new Error('getCapabilities method unavailable on LLM gRPC client'));
        return;
      }

      method.call(client, { provider }, { deadline: this.getDeadline(timeout) }, (err, response) => {
        if (err) {
          reject(err);
          return;
        }
        resolve(response);
      });
    });
  }

  private requireClient(): LLMServiceClient {
    if (!this.client || !this.connected) {
      throw new Error('LLM gRPC client is not connected');
    }
    return this.client;
  }

  private resolveAddress(address: string): string {
    if (address.startsWith('unix://')) {
      return address;
    }
    if (address.startsWith('/')) {
      return `unix://${address}`;
    }
    return address;
  }

  private getDeadline(timeout: number): Date {
    const deadline = new Date();
    deadline.setMilliseconds(deadline.getMilliseconds() + timeout);
    return deadline;
  }

  private resolveProtoPath(): string {
    const candidates = [
      fileURLToPath(new URL('../../../../proto/llm.proto', import.meta.url)),
      fileURLToPath(new URL('../../../proto/llm.proto', import.meta.url)),
      path.resolve(process.cwd(), '../../proto/llm.proto'),
      path.resolve(process.cwd(), '../proto/llm.proto'),
      path.resolve(process.cwd(), 'proto/llm.proto'),
    ];

    for (const candidate of candidates) {
      if (fs.existsSync(candidate)) {
        return candidate;
      }
    }

    throw new Error(`llm.proto not found. Checked: ${candidates.join(', ')}`);
  }

  /**
   * Generates a unique request ID
   */
  private generateRequestId(): string {
    return `req_${Date.now()}_${Math.random().toString(36).substring(2, 11)}`;
  }

  /**
   * Maps gRPC/transport errors to client error codes
   */
  private mapGrpcCodeToErrorCode(code: number | undefined): LLMGatewayErrorCode {
    switch (code) {
      case 3:
        return LLMGatewayErrorCode.INVALID_REQUEST;
      case 7:
      case 16:
        return LLMGatewayErrorCode.AUTHENTICATION_FAILED;
      case 8:
      case 4:
        return LLMGatewayErrorCode.TIMEOUT;
      case 14:
        return LLMGatewayErrorCode.PROVIDER_NOT_AVAILABLE;
      default:
        return LLMGatewayErrorCode.UPSTREAM_ERROR;
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

    if (typeof err === 'object' && err !== null && 'code' in err) {
      const code = typeof (err as { code?: unknown }).code === 'number'
        ? (err as { code: number }).code
        : undefined;

      const message = 'message' in err && typeof (err as { message?: unknown }).message === 'string'
        ? (err as { message: string }).message
        : 'gRPC request failed';

      const error = new Error(message) as LLMGatewayError;
      error.code = this.mapGrpcCodeToErrorCode(code);
      error.name = 'LLMGatewayError';
      error.retryable = code === 4 || code === 8 || code === 14;
      return error;
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
