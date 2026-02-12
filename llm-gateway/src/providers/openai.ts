// OpenAI provider using the official openai SDK

import OpenAI from "openai";
import { ProviderConfig } from "../config";
import { log } from "../logger";
import {
  LLMRequest,
  LLMResponse,
  StreamEvent,
  ToolCall,
  TokenUsage,
} from "../types";
import { ModelInfo, Provider } from "./provider";

function traceValue(value?: string): string {
  return value && value.trim() ? value : "-";
}

function traceFlow(
  stage: string,
  correlationId: string | undefined,
  sessionId: string | undefined,
  requestId: string,
  details?: Record<string, unknown>
): void {
  const line = `TRACE_FLOW stage=${stage} cid=${traceValue(correlationId)} sid=${traceValue(sessionId)} rid=${traceValue(requestId)}`;
  if (details && Object.keys(details).length > 0) {
    log("info", line, details);
    return;
  }
  log("info", line);
}

export class OpenAIProvider implements Provider {
  readonly name = "openai";
  private client: OpenAI;
  private apiKey: string;
  private baseUrl: string;

  constructor(config: ProviderConfig) {
    this.apiKey = config.apiKey;
    this.baseUrl = config.baseUrl || "https://api.openai.com/v1";
    this.client = new OpenAI({
      apiKey: config.apiKey,
      baseURL: config.baseUrl || undefined,
    });
  }

  isAvailable(): boolean {
    return !!this.apiKey;
  }

  listModels(): ModelInfo[] {
    return [
      {
        id: "openai/gpt-4o",
        name: "GPT-4o",
        provider: "openai",
        capabilities: {
          streaming: true,
          tools: true,
          vision: true,
          thinking: false,
        },
        metadata: { context_window: "128000" },
      },
      {
        id: "openai/gpt-4o-mini",
        name: "GPT-4o Mini",
        provider: "openai",
        capabilities: {
          streaming: true,
          tools: true,
          vision: true,
          thinking: false,
        },
        metadata: { context_window: "128000" },
      },
      {
        id: "openai/gpt-4-turbo",
        name: "GPT-4 Turbo",
        provider: "openai",
        capabilities: {
          streaming: true,
          tools: true,
          vision: true,
          thinking: false,
        },
        metadata: { context_window: "128000" },
      },
    ];
  }

  async complete(request: LLMRequest): Promise<LLMResponse> {
    const modelName = this.extractModelName(request.model);
    const params = this.buildParams(request, modelName);

    const startedAt = Date.now();
    log("info", "Dispatching upstream completion request", {
      request_id: request.request_id,
      model: modelName,
      base_url: this.baseUrl,
      message_count: request.messages.length,
      has_tools: !!request.tools?.length,
    });
    traceFlow("gateway.provider.complete.dispatch", request.metadata?.correlation_id, request.session_id, request.request_id, {
      model: modelName,
      base_url: this.baseUrl,
      message_count: request.messages.length,
    });

    try {
      const response = (await this.client.chat.completions.create(
        params
      )) as OpenAI.ChatCompletion;

      log("info", "Upstream completion response received", {
        request_id: request.request_id,
        model: modelName,
        base_url: this.baseUrl,
        prompt_tokens: response.usage?.prompt_tokens || 0,
        completion_tokens: response.usage?.completion_tokens || 0,
        total_tokens: response.usage?.total_tokens || 0,
        elapsed_ms: Date.now() - startedAt,
      });
      traceFlow("gateway.provider.complete.response", request.metadata?.correlation_id, request.session_id, request.request_id, {
        model: modelName,
        elapsed_ms: Date.now() - startedAt,
        total_tokens: response.usage?.total_tokens || 0,
      });

      return this.convertResponse(request, response);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      log("error", "Upstream completion request failed", {
        request_id: request.request_id,
        model: modelName,
        base_url: this.baseUrl,
        elapsed_ms: Date.now() - startedAt,
        error: errMsg,
      });
      traceFlow("gateway.provider.complete.failed", request.metadata?.correlation_id, request.session_id, request.request_id, {
        model: modelName,
        elapsed_ms: Date.now() - startedAt,
        error: errMsg,
      });
      throw err;
    }
  }

  async stream(
    request: LLMRequest,
    onEvent: (event: StreamEvent) => void
  ): Promise<void> {
    const modelName = this.extractModelName(request.model);
    const params = this.buildParams(request, modelName);
    const startedAt = Date.now();
    log("info", "Dispatching upstream stream request", {
      request_id: request.request_id,
      model: modelName,
      base_url: this.baseUrl,
      message_count: request.messages.length,
      has_tools: !!request.tools?.length,
    });
    traceFlow("gateway.provider.stream.dispatch", request.metadata?.correlation_id, request.session_id, request.request_id, {
      model: modelName,
      base_url: this.baseUrl,
      message_count: request.messages.length,
    });

    try {
      const stream = await this.client.chat.completions.create({
        ...params,
        stream: true,
        stream_options: { include_usage: true },
      });

      let chunkIndex = 0;
      let fullContent = "";
      let inputTokens = 0;
      let outputTokens = 0;
      const toolCalls: Map<number, ToolCall> = new Map();

      for await (const chunk of stream) {
        const delta = chunk.choices?.[0]?.delta;

        if (delta?.content) {
          fullContent += delta.content;
          onEvent({
            type: "chunk",
            request_id: request.request_id,
            content: delta.content,
            index: chunkIndex++,
            is_last: false,
          });
        }

        if (delta?.tool_calls) {
          for (const tc of delta.tool_calls) {
            const idx = tc.index;
            if (!toolCalls.has(idx)) {
              toolCalls.set(idx, {
                id: tc.id || "",
                name: tc.function?.name || "",
                arguments: "",
              });
            }
            const existing = toolCalls.get(idx)!;
            if (tc.id) existing.id = tc.id;
            if (tc.function?.name) existing.name = tc.function.name;
            if (tc.function?.arguments) {
              existing.arguments += tc.function.arguments;
              onEvent({
                type: "tool_call",
                request_id: request.request_id,
                tool_call_id: existing.id,
                name: existing.name,
                arguments_delta: tc.function.arguments,
              });
            }
          }
        }

        if (chunk.usage) {
          inputTokens = chunk.usage.prompt_tokens;
          outputTokens = chunk.usage.completion_tokens;
        }
      }

      const usage: TokenUsage = {
        input_tokens: inputTokens,
        output_tokens: outputTokens,
        total_tokens: inputTokens + outputTokens,
      };

      onEvent({
        type: "usage",
        request_id: request.request_id,
        usage,
      });

      onEvent({
        type: "chunk",
        request_id: request.request_id,
        content: "",
        index: chunkIndex,
        is_last: true,
      });

      onEvent({
        type: "complete",
        request_id: request.request_id,
        response: {
          request_id: request.request_id,
          content: fullContent,
          tool_calls: Array.from(toolCalls.values()),
          usage,
          finish_reason: "STOP",
          provider: "openai",
          model: request.model,
          completed_at: new Date().toISOString(),
          metadata: request.metadata,
        },
      });

      log("info", "Upstream stream completed", {
        request_id: request.request_id,
        model: modelName,
        base_url: this.baseUrl,
        chunk_count: chunkIndex,
        tool_call_count: toolCalls.size,
        total_tokens: usage.total_tokens,
        elapsed_ms: Date.now() - startedAt,
      });
      traceFlow("gateway.provider.stream.response", request.metadata?.correlation_id, request.session_id, request.request_id, {
        model: modelName,
        elapsed_ms: Date.now() - startedAt,
        chunk_count: chunkIndex,
        total_tokens: usage.total_tokens,
      });
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      log("error", "Upstream stream request failed", {
        request_id: request.request_id,
        model: modelName,
        base_url: this.baseUrl,
        elapsed_ms: Date.now() - startedAt,
        error: errMsg,
      });
      traceFlow("gateway.provider.stream.failed", request.metadata?.correlation_id, request.session_id, request.request_id, {
        model: modelName,
        elapsed_ms: Date.now() - startedAt,
        error: errMsg,
      });
      throw err;
    }
  }

  // ---- helpers ----

  private extractModelName(model: string): string {
    const slash = model.indexOf("/");
    return slash >= 0 ? model.slice(slash + 1) : model;
  }

  private buildParams(
    request: LLMRequest,
    modelName: string
  ): OpenAI.ChatCompletionCreateParams {
    const messages: OpenAI.ChatCompletionMessageParam[] = request.messages.map(
      (msg) => ({
        role: msg.role as "system" | "user" | "assistant",
        content: msg.content,
      })
    );

    const params: OpenAI.ChatCompletionCreateParams = {
      model: modelName,
      messages,
    };

    if (request.parameters?.max_tokens)
      params.max_tokens = request.parameters.max_tokens;
    if (request.parameters?.temperature !== undefined)
      params.temperature = request.parameters.temperature;
    if (request.parameters?.top_p !== undefined)
      params.top_p = request.parameters.top_p;
    if (request.parameters?.stop_sequences?.length)
      params.stop = request.parameters.stop_sequences;

    // Convert tools
    if (request.tools?.length) {
      params.tools = request.tools.map((t) => ({
        type: "function" as const,
        function: {
          name: t.name,
          description: t.description,
          parameters: t.input_schema
            ? {
                type: t.input_schema.type,
                properties: t.input_schema.properties
                  ? Object.fromEntries(
                      Object.entries(t.input_schema.properties).map(
                        ([k, v]) => [
                          k,
                          {
                            type: v.type,
                            description: v.description,
                            enum: v.enum,
                          },
                        ]
                      )
                    )
                  : undefined,
                required: t.input_schema.required,
              }
            : undefined,
        },
      }));
    }

    return params;
  }

  private convertResponse(
    request: LLMRequest,
    response: OpenAI.ChatCompletion
  ): LLMResponse {
    const choice = response.choices[0];
    const content = choice?.message?.content || "";
    const toolCalls: ToolCall[] = (choice?.message?.tool_calls || []).map(
      (tc) => ({
        id: tc.id,
        name: tc.function.name,
        arguments: tc.function.arguments,
      })
    );

    return {
      request_id: request.request_id,
      content,
      tool_calls: toolCalls,
      usage: {
        input_tokens: response.usage?.prompt_tokens || 0,
        output_tokens: response.usage?.completion_tokens || 0,
        total_tokens: response.usage?.total_tokens || 0,
      },
      finish_reason: this.mapFinishReason(choice?.finish_reason),
      provider: "openai",
      model: request.model,
      completed_at: new Date().toISOString(),
      metadata: request.metadata,
    };
  }

  private mapFinishReason(reason: string | null | undefined): string {
    switch (reason) {
      case "stop":
        return "STOP";
      case "length":
        return "LENGTH";
      case "tool_calls":
        return "TOOL_USES";
      case "content_filter":
        return "FILTER";
      default:
        return "STOP";
    }
  }
}
