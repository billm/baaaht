// HTTP server exposing the gateway API.
// Endpoints mirror the protobuf LLMService shape so the orchestrator can call them.

import * as http from "http";
import { GatewayConfig, getProviderConfig } from "./config";
import { log } from "./logger";
import {
  AnthropicProvider,
  ModelInfo,
  OpenAIProvider,
  Provider,
  ProviderStats,
  newProviderStats,
  recordError,
  recordRequest,
} from "./providers";
import {
  GatewayStatus,
  LLMRequest,
  LLMResponse,
  ProviderStatus,
  StreamEvent,
} from "./types";

function traceValue(value?: string): string {
  return value && value.trim() ? value : "-";
}

function traceFlow(
  stage: string,
  correlationId?: string,
  sessionId?: string,
  requestId?: string,
  details?: Record<string, unknown>
): void {
  const line = `TRACE_FLOW stage=${stage} cid=${traceValue(correlationId)} sid=${traceValue(sessionId)} rid=${traceValue(requestId)}`;
  if (details && Object.keys(details).length > 0) {
    log("info", line, details);
    return;
  }
  log("info", line);
}

export class Gateway {
  private config: GatewayConfig;
  private providers: Map<string, Provider> = new Map();
  private stats: Map<string, ProviderStats> = new Map();
  private server: http.Server | null = null;
  private startedAt: Date;
  private activeRequests = 0;
  private totalRequests = 0;
  private totalTokens = 0;

  constructor(config: GatewayConfig) {
    this.config = config;
    this.startedAt = new Date();
    this.initProviders();
  }

  private initProviders(): void {
    for (const pcfg of this.config.providers) {
      switch (pcfg.name) {
        case "anthropic": {
          const provider = new AnthropicProvider(pcfg);
          this.providers.set("anthropic", provider);
          this.stats.set("anthropic", newProviderStats());
          log("info", "Initialized Anthropic provider");
          break;
        }
        case "openai": {
          const provider = new OpenAIProvider(pcfg);
          this.providers.set("openai", provider);
          this.stats.set("openai", newProviderStats());
          log("info", "Initialized OpenAI provider");
          break;
        }
        default: {
          // Treat unknown providers as OpenAI-compatible (covers lmstudio, ollama, etc.)
          // These providers expose an OpenAI-compatible API at a custom base URL.
          log("info", `Initializing OpenAI-compatible provider: ${pcfg.name}`, {
            baseUrl: pcfg.baseUrl,
          });
          const provider = new OpenAIProvider(pcfg);
          // Override the name property for resolution
          const namedProvider: Provider = {
            name: pcfg.name,
            isAvailable: () => provider.isAvailable(),
            listModels: () => provider.listModels(),
            complete: (req) => provider.complete(req),
            stream: (req, cb) => provider.stream(req, cb),
          };
          this.providers.set(pcfg.name, namedProvider);
          this.stats.set(pcfg.name, newProviderStats());
          break;
        }
      }
    }

    if (this.providers.size === 0) {
      log("warn", "No LLM providers configured — gateway will return errors for completion requests");
    }
  }

  /** Resolve the provider for a request, with fallback chain support. */
  private resolveProvider(request: LLMRequest): Provider | null {
    // Explicit provider override
    if (request.provider) {
      const p = this.providers.get(request.provider);
      if (p?.isAvailable()) return p;
    }

    // Derive from model name (e.g. "anthropic/claude-..." → "anthropic")
    const slash = request.model.indexOf("/");
    if (slash > 0) {
      const derived = request.model.slice(0, slash);
      const p = this.providers.get(derived);
      if (p?.isAvailable()) return p;
    }

    // Fallback chain
    if (request.fallback_providers) {
      for (const name of request.fallback_providers) {
        const p = this.providers.get(name);
        if (p?.isAvailable()) return p;
      }
    }

    // Default provider
    const def = this.providers.get(this.config.defaultProvider);
    if (def?.isAvailable()) return def;

    // Any available provider
    for (const p of this.providers.values()) {
      if (p.isAvailable()) return p;
    }

    return null;
  }

  // ---------------------------------------------------------------------------
  // HTTP server
  // ---------------------------------------------------------------------------

  start(): Promise<void> {
    return new Promise((resolve, reject) => {
      this.server = http.createServer((req, res) => {
        this.handleRequest(req, res).catch((err) => {
          log("error", "Unhandled error in request handler", {
            error: String(err),
          });
          if (!res.headersSent) {
            res.writeHead(500, { "Content-Type": "application/json" });
            res.end(JSON.stringify({ error: "internal server error" }));
          }
        });
      });

      this.server.listen(this.config.port, () => {
        log("info", `Gateway listening on port ${this.config.port}`);
        resolve();
      });

      this.server.on("error", reject);
    });
  }

  stop(): Promise<void> {
    return new Promise((resolve) => {
      if (this.server) {
        this.server.close(() => resolve());
      } else {
        resolve();
      }
    });
  }

  private async handleRequest(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): Promise<void> {
    const url = req.url || "/";
    const method = req.method || "GET";

    if (method === "POST" && (url === "/v1/completions" || url === "/v1/stream")) {
      log("info", "Gateway received LLM HTTP request", {
        method,
        path: url,
        remote: req.socket.remoteAddress,
        user_agent: req.headers["user-agent"] || "",
        content_length: req.headers["content-length"] || "",
      });
      traceFlow("gateway.http.ingress", undefined, undefined, undefined, {
        path: url,
        method,
      });
    }

    // Route
    if (method === "GET" && url === "/health") {
      return this.handleHealth(res);
    }
    if (method === "GET" && url === "/v1/status") {
      return this.handleStatus(res);
    }
    if (method === "GET" && url.startsWith("/v1/models")) {
      return this.handleListModels(req, res);
    }
    if (method === "GET" && url.startsWith("/v1/capabilities")) {
      return this.handleCapabilities(req, res);
    }
    if (method === "POST" && url === "/v1/completions") {
      return this.handleComplete(req, res);
    }
    if (method === "POST" && url === "/v1/stream") {
      return this.handleStream(req, res);
    }

    res.writeHead(404, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ error: "not found" }));
  }

  // ---------------------------------------------------------------------------
  // Endpoint handlers
  // ---------------------------------------------------------------------------

  private handleHealth(res: http.ServerResponse): void {
    const availableProviders: string[] = [];
    const unavailableProviders: string[] = [];

    for (const [name, p] of this.providers) {
      if (p.isAvailable()) {
        availableProviders.push(name);
      } else {
        unavailableProviders.push(name);
      }
    }

    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify({
        health: "HEALTHY",
        version: "1.0.0",
        available_providers: availableProviders,
        unavailable_providers: unavailableProviders,
        timestamp: new Date().toISOString(),
      })
    );
  }

  private handleStatus(res: http.ServerResponse): void {
    const providers: Record<string, ProviderStatus> = {};
    for (const [name, p] of this.providers) {
      const s = this.stats.get(name)!;
      providers[name] = {
        name,
        available: p.isAvailable(),
        health: p.isAvailable() ? "HEALTHY" : "UNHEALTHY",
        total_requests: s.totalRequests,
        total_tokens: s.totalTokens,
        avg_response_time_ms: s.avgResponseTimeMs,
        last_error: s.lastError,
      };
    }

    const status: GatewayStatus = {
      status: "RUNNING",
      started_at: this.startedAt.toISOString(),
      uptime_ms: Date.now() - this.startedAt.getTime(),
      active_requests: this.activeRequests,
      total_requests: this.totalRequests,
      total_tokens_used: this.totalTokens,
      providers,
    };

    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(status));
  }

  private handleListModels(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    const url = new URL(req.url || "/", `http://${req.headers.host}`);
    const providerFilter = url.searchParams.get("provider") || "";

    const models: ModelInfo[] = [];
    for (const [name, p] of this.providers) {
      if (providerFilter && name !== providerFilter) continue;
      models.push(...p.listModels());
    }

    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ models }));
  }

  private handleCapabilities(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): void {
    const url = new URL(req.url || "/", `http://${req.headers.host}`);
    const providerFilter = url.searchParams.get("provider") || "";

    const providers: Array<{
      name: string;
      available: boolean;
      models: ModelInfo[];
      metadata: Record<string, string>;
    }> = [];

    for (const [name, p] of this.providers) {
      if (providerFilter && name !== providerFilter) continue;
      providers.push({
        name,
        available: p.isAvailable(),
        models: p.listModels(),
        metadata: {},
      });
    }

    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ providers }));
  }

  private async handleComplete(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): Promise<void> {
    const body = await readBody(req);
    let llmReq: LLMRequest;
    try {
      llmReq = JSON.parse(body) as LLMRequest;
    } catch {
      log("error", "Completion request rejected: invalid JSON", {
        body_length: body.length,
      });
      res.writeHead(400, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "invalid JSON body" }));
      return;
    }

    log("info", "Completion request parsed", {
      request_id: llmReq.request_id,
      session_id: llmReq.session_id,
      model: llmReq.model,
      provider_hint: llmReq.provider,
      message_count: llmReq.messages?.length ?? 0,
    });

    if (!llmReq.model) {
      llmReq.model = this.config.defaultModel;
      log("info", "Completion request model defaulted from gateway config", {
        request_id: llmReq.request_id,
        default_model: this.config.defaultModel,
      });
    }
    traceFlow(
      "gateway.complete.recv_request",
      llmReq.metadata?.correlation_id,
      llmReq.session_id,
      llmReq.request_id,
      {
        model: llmReq.model,
        provider_hint: llmReq.provider,
        message_count: llmReq.messages?.length ?? 0,
      }
    );

    if (!llmReq.request_id || !llmReq.model || !llmReq.messages?.length) {
      log("warn", "Completion request rejected: missing required fields", {
        request_id: llmReq.request_id,
        model: llmReq.model,
        message_count: llmReq.messages?.length ?? 0,
      });
      res.writeHead(400, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          error: "request_id, model, and messages are required",
        })
      );
      return;
    }

    const provider = this.resolveProvider(llmReq);
    if (!provider) {
      log("warn", "Completion request rejected: no available provider", {
        request_id: llmReq.request_id,
        model: llmReq.model,
        provider_hint: llmReq.provider,
      });
      res.writeHead(503, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "no available provider for request" }));
      return;
    }

    // Concurrency gate
    if (this.activeRequests >= this.config.maxConcurrentRequests) {
      log("warn", "Completion request rejected: concurrency limit", {
        request_id: llmReq.request_id,
        active_requests: this.activeRequests,
        max_concurrent: this.config.maxConcurrentRequests,
      });
      res.writeHead(429, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "too many concurrent requests" }));
      return;
    }

    this.activeRequests++;
    const start = Date.now();

    try {
      log("info", "Processing completion", {
        request_id: llmReq.request_id,
        model: llmReq.model,
        provider: provider.name,
      });
      traceFlow(
        "gateway.complete.provider_dispatch",
        llmReq.metadata?.correlation_id,
        llmReq.session_id,
        llmReq.request_id,
        {
          provider: provider.name,
          model: llmReq.model,
        }
      );

      const response: LLMResponse = await provider.complete(llmReq);

      const elapsed = Date.now() - start;
      const stats = this.stats.get(provider.name);
      if (stats) recordRequest(stats, response.usage, elapsed);

      this.totalRequests++;
      this.totalTokens += response.usage.total_tokens;

      log("info", "Completion done", {
        request_id: llmReq.request_id,
        provider: provider.name,
        tokens: response.usage.total_tokens,
        elapsed_ms: elapsed,
      });
      traceFlow(
        "gateway.complete.provider_done",
        llmReq.metadata?.correlation_id,
        llmReq.session_id,
        llmReq.request_id,
        {
          provider: provider.name,
          tokens: response.usage.total_tokens,
          elapsed_ms: elapsed,
        }
      );

      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ response }));
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      log("error", "Completion failed", {
        request_id: llmReq.request_id,
        error: errMsg,
      });
      traceFlow(
        "gateway.complete.failed",
        llmReq.metadata?.correlation_id,
        llmReq.session_id,
        llmReq.request_id,
        {
          error: errMsg,
        }
      );
      const stats = this.stats.get(provider.name);
      if (stats) recordError(stats, errMsg);

      res.writeHead(502, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          error: errMsg,
          retryable: true,
        })
      );
    } finally {
      this.activeRequests--;
    }
  }

  private async handleStream(
    req: http.IncomingMessage,
    res: http.ServerResponse
  ): Promise<void> {
    const body = await readBody(req);
    let llmReq: LLMRequest;
    try {
      llmReq = JSON.parse(body) as LLMRequest;
    } catch {
      log("error", "Stream request rejected: invalid JSON", {
        body_length: body.length,
      });
      res.writeHead(400, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "invalid JSON body" }));
      return;
    }

    log("info", "Stream request parsed", {
      request_id: llmReq.request_id,
      session_id: llmReq.session_id,
      model: llmReq.model,
      provider_hint: llmReq.provider,
      message_count: llmReq.messages?.length ?? 0,
    });

    if (!llmReq.model) {
      llmReq.model = this.config.defaultModel;
      log("info", "Stream request model defaulted from gateway config", {
        request_id: llmReq.request_id,
        default_model: this.config.defaultModel,
      });
    }
    traceFlow(
      "gateway.stream.recv_request",
      llmReq.metadata?.correlation_id,
      llmReq.session_id,
      llmReq.request_id,
      {
        model: llmReq.model,
        provider_hint: llmReq.provider,
        message_count: llmReq.messages?.length ?? 0,
      }
    );

    if (!llmReq.request_id || !llmReq.model || !llmReq.messages?.length) {
      log("warn", "Stream request rejected: missing required fields", {
        request_id: llmReq.request_id,
        model: llmReq.model,
        message_count: llmReq.messages?.length ?? 0,
      });
      res.writeHead(400, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          error: "request_id, model, and messages are required",
        })
      );
      return;
    }

    const provider = this.resolveProvider(llmReq);
    if (!provider) {
      log("warn", "Stream request rejected: no available provider", {
        request_id: llmReq.request_id,
        model: llmReq.model,
        provider_hint: llmReq.provider,
      });
      res.writeHead(503, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "no available provider for request" }));
      return;
    }

    if (this.activeRequests >= this.config.maxConcurrentRequests) {
      log("warn", "Stream request rejected: concurrency limit", {
        request_id: llmReq.request_id,
        active_requests: this.activeRequests,
        max_concurrent: this.config.maxConcurrentRequests,
      });
      res.writeHead(429, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "too many concurrent requests" }));
      return;
    }

    this.activeRequests++;
    const start = Date.now();

    // Use Server-Sent Events for streaming
    res.writeHead(200, {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    });

    try {
      log("info", "Processing stream", {
        request_id: llmReq.request_id,
        model: llmReq.model,
        provider: provider.name,
      });
      traceFlow(
        "gateway.stream.provider_dispatch",
        llmReq.metadata?.correlation_id,
        llmReq.session_id,
        llmReq.request_id,
        {
          provider: provider.name,
          model: llmReq.model,
        }
      );

      await provider.stream(llmReq, (event: StreamEvent) => {
        res.write(`data: ${JSON.stringify(event)}\n\n`);

        // Track usage when we see the complete event
        if (event.type === "complete") {
          const elapsed = Date.now() - start;
          const stats = this.stats.get(provider.name);
          if (stats) recordRequest(stats, event.response.usage, elapsed);
          this.totalRequests++;
          this.totalTokens += event.response.usage.total_tokens;

          log("info", "Stream done", {
            request_id: llmReq.request_id,
            provider: provider.name,
            tokens: event.response.usage.total_tokens,
            elapsed_ms: elapsed,
          });
          traceFlow(
            "gateway.stream.provider_done",
            llmReq.metadata?.correlation_id,
            llmReq.session_id,
            llmReq.request_id,
            {
              provider: provider.name,
              tokens: event.response.usage.total_tokens,
              elapsed_ms: elapsed,
            }
          );
        }
      });

      res.write("data: [DONE]\n\n");
      res.end();
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      log("error", "Stream failed", {
        request_id: llmReq.request_id,
        error: errMsg,
      });
      traceFlow(
        "gateway.stream.failed",
        llmReq.metadata?.correlation_id,
        llmReq.session_id,
        llmReq.request_id,
        {
          error: errMsg,
        }
      );
      const stats = this.stats.get(provider.name);
      if (stats) recordError(stats, errMsg);

      // Send error event
      const errorEvent: StreamEvent = {
        type: "error",
        request_id: llmReq.request_id,
        code: "PROVIDER_ERROR",
        message: errMsg,
        retryable: true,
      };
      res.write(`data: ${JSON.stringify(errorEvent)}\n\n`);
      res.end();
    } finally {
      this.activeRequests--;
    }
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function readBody(req: http.IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    req.on("data", (chunk: Buffer) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf-8")));
    req.on("error", reject);
  });
}
