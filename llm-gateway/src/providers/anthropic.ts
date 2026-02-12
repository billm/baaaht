// Anthropic provider using the official @anthropic-ai/sdk

import Anthropic from "@anthropic-ai/sdk";
import type { MessageStreamEvents } from "@anthropic-ai/sdk/lib/MessageStream";
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

export class AnthropicProvider implements Provider {
  readonly name = "anthropic";
  private client: Anthropic;
  private apiKey: string;

  constructor(config: ProviderConfig) {
    this.apiKey = config.apiKey;
    this.client = new Anthropic({
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
        id: "anthropic/claude-sonnet-4-20250514",
        name: "Claude Sonnet 4",
        provider: "anthropic",
        capabilities: {
          streaming: true,
          tools: true,
          vision: true,
          thinking: true,
        },
        metadata: {
          context_window: "200000",
          pricing_input: "$3.00/M tokens",
          pricing_output: "$15.00/M tokens",
        },
      },
      {
        id: "anthropic/claude-3-5-sonnet-20241022",
        name: "Claude 3.5 Sonnet",
        provider: "anthropic",
        capabilities: {
          streaming: true,
          tools: true,
          vision: true,
          thinking: false,
        },
        metadata: { context_window: "200000" },
      },
    ];
  }

  async complete(request: LLMRequest): Promise<LLMResponse> {
    const modelName = this.extractModelName(request.model);
    const params = this.buildParams(request, modelName);

    log("debug", "Anthropic complete request", {
      request_id: request.request_id,
      model: modelName,
    });

    const response = await this.client.messages.create(params) as Anthropic.Message;

    return this.convertResponse(request, response);
  }

  async stream(
    request: LLMRequest,
    onEvent: (event: StreamEvent) => void
  ): Promise<void> {
    const modelName = this.extractModelName(request.model);
    const params = this.buildParams(request, modelName);

    log("debug", "Anthropic stream request", {
      request_id: request.request_id,
      model: modelName,
    });

    const stream = this.client.messages.stream(params);

    let chunkIndex = 0;
    let fullContent = "";
    let inputTokens = 0;
    let outputTokens = 0;
    // Track tool call arguments being built up
    const toolCallArgs: Map<number, { id: string; name: string; args: string }> =
      new Map();

    stream.on("message", (msg) => {
      inputTokens = msg.usage?.input_tokens ?? inputTokens;
      outputTokens = msg.usage?.output_tokens ?? outputTokens;
    });

    stream.on("contentBlock", (block) => {
      if (block?.type === "tool_use") {
        const idx = toolCallArgs.size;
        toolCallArgs.set(idx, {
          id: block.id || "",
          name: block.name || "",
          args: "",
        });
      }
    });

    stream.on("text", (text: string) => {
      fullContent += text;
      onEvent({
        type: "chunk",
        request_id: request.request_id,
        content: text,
        index: chunkIndex++,
        is_last: false,
      });
    });

    stream.on("inputJson", (partialJson) => {
      // Find the last active tool call
      const lastIdx = toolCallArgs.size - 1;
      const tc = toolCallArgs.get(lastIdx);
      if (tc) {
        tc.args += partialJson;
        onEvent({
          type: "tool_call",
          request_id: request.request_id,
          tool_call_id: tc.id,
          name: tc.name,
          arguments_delta: partialJson,
        });
      }
    });

    // Wait for the stream to finish
    const finalMessage = await stream.finalMessage();

    inputTokens = finalMessage.usage?.input_tokens ?? inputTokens;
    outputTokens = finalMessage.usage?.output_tokens ?? outputTokens;

    const usage: TokenUsage = {
      input_tokens: inputTokens,
      output_tokens: outputTokens,
      total_tokens: inputTokens + outputTokens,
    };

    // Collect finished tool calls from tracked args
    const toolCalls: ToolCall[] = Array.from(toolCallArgs.values()).map((tc) => ({
      id: tc.id,
      name: tc.name,
      arguments: tc.args,
    }));

    // Send final usage
    onEvent({
      type: "usage",
      request_id: request.request_id,
      usage,
    });

    // Send last chunk marker
    onEvent({
      type: "chunk",
      request_id: request.request_id,
      content: "",
      index: chunkIndex,
      is_last: true,
    });

    // Send completion
    onEvent({
      type: "complete",
      request_id: request.request_id,
      response: {
        request_id: request.request_id,
        content: fullContent,
        tool_calls: toolCalls,
        usage,
        finish_reason: this.mapStopReason(finalMessage.stop_reason),
        provider: "anthropic",
        model: request.model,
        completed_at: new Date().toISOString(),
        metadata: request.metadata,
      },
    });
  }

  // ---- helpers ----

  private extractModelName(model: string): string {
    // "anthropic/claude-sonnet-4-20250514" -> "claude-sonnet-4-20250514"
    const slash = model.indexOf("/");
    return slash >= 0 ? model.slice(slash + 1) : model;
  }

  private buildParams(
    request: LLMRequest,
    modelName: string
  ): Anthropic.MessageCreateParams {
    const messages: Anthropic.MessageParam[] = [];
    let system: string | undefined;

    for (const msg of request.messages) {
      if (msg.role === "system") {
        system = msg.content;
      } else {
        messages.push({
          role: msg.role as "user" | "assistant",
          content: msg.content,
        });
      }
    }

    const params: Anthropic.MessageCreateParams = {
      model: modelName,
      max_tokens: request.parameters?.max_tokens || 4096,
      messages,
    };

    if (system) params.system = system;
    if (request.parameters?.temperature !== undefined)
      params.temperature = request.parameters.temperature;
    if (request.parameters?.top_p !== undefined)
      params.top_p = request.parameters.top_p;
    if (request.parameters?.top_k !== undefined)
      params.top_k = request.parameters.top_k;
    if (request.parameters?.stop_sequences?.length)
      params.stop_sequences = request.parameters.stop_sequences;

    // Convert tools
    if (request.tools?.length) {
      params.tools = request.tools.map((t) => ({
        name: t.name,
        description: t.description,
        input_schema: (t.input_schema || {
          type: "object",
          properties: {},
        }) as Anthropic.Tool.InputSchema,
      }));
    }

    return params;
  }

  private convertResponse(
    request: LLMRequest,
    response: Anthropic.Message
  ): LLMResponse {
    let content = "";
    const toolCalls: ToolCall[] = [];

    for (const block of response.content) {
      if (block.type === "text") {
        content += block.text;
      } else if (block.type === "tool_use") {
        toolCalls.push({
          id: block.id,
          name: block.name,
          arguments: JSON.stringify(block.input),
        });
      }
    }

    return {
      request_id: request.request_id,
      content,
      tool_calls: toolCalls,
      usage: {
        input_tokens: response.usage.input_tokens,
        output_tokens: response.usage.output_tokens,
        total_tokens:
          response.usage.input_tokens + response.usage.output_tokens,
      },
      finish_reason: this.mapStopReason(response.stop_reason),
      provider: "anthropic",
      model: request.model,
      completed_at: new Date().toISOString(),
      metadata: request.metadata,
    };
  }

  private mapStopReason(reason: string | null): string {
    switch (reason) {
      case "end_turn":
        return "STOP";
      case "max_tokens":
        return "LENGTH";
      case "tool_use":
        return "TOOL_USES";
      case "stop_sequence":
        return "STOP";
      default:
        return "STOP";
    }
  }
}
