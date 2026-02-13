// tool-calling.ts - Tool calling logic for function calling with LLM
//
// This file provides utilities for handling function calling with the LLM,
// including tool call parsing, execution, aggregation for streaming, and
// result formatting.
//
// Copyright 2026 baaaht project

import type {
  ToolCall,
  Tool,
  LLMMessage,
} from '../proto/llm.js';
import type {
  ToolDefinition,
  ToolResult,
  DelegateParams,
} from '../tools/types.js';
import type {
  ToolCallInfo,
  ToolExecutionResult,
  ToolExecutionContext,
  AgentError,
} from './types.js';
import {
  validateDelegateParams,
} from '../tools/delegate.js';
import { DelegateError } from '../tools/types.js';
import { AgentErrorCode } from './types.js';

function createAgentError(
  message: string,
  code: AgentErrorCode,
  retryable: boolean = false,
  details?: Record<string, unknown>
): AgentError {
  const error = new Error(message) as AgentError;
  error.code = code;
  error.retryable = retryable;
  error.details = details;
  error.name = 'AgentError';
  return error;
}

// =============================================================================
// Tool Calling Configuration
// =============================================================================

/**
 * Configuration for tool calling behavior
 */
export interface ToolCallingConfig {
  /**
   * Maximum number of tool calls per request
   */
  maxToolCalls?: number;

  /**
   * Maximum tool execution time in milliseconds
   */
  maxExecutionTime?: number;

  /**
   * Whether to continue on tool errors
   */
  continueOnError?: boolean;

  /**
   * Whether to include tool results in response
   */
  includeResults?: boolean;
}

/**
 * Default tool calling configuration
 */
const DEFAULT_TOOL_CALLING_CONFIG: Required<ToolCallingConfig> = {
  maxToolCalls: 10,
  maxExecutionTime: 300000, // 5 minutes
  continueOnError: false,
  includeResults: true,
};

// =============================================================================
// Tool Call Aggregation (Streaming)
// =============================================================================

/**
 * Aggregates streaming tool call chunks into complete tool calls
 *
 * Handles the case where tool calls are streamed in chunks, with arguments
 * arriving as deltas that need to be accumulated.
 */
export class ToolCallAggregator {
  private pendingToolCalls = new Map<string, AggregatedToolCall>();
  private completedToolCalls: ToolCall[] = [];
  private config: Required<ToolCallingConfig>;

  constructor(config: ToolCallingConfig = {}) {
    this.config = { ...DEFAULT_TOOL_CALLING_CONFIG, ...config };
  }

  /**
   * Add a streaming tool call chunk
   *
   * @param toolCallId - Tool call identifier
   * @param name - Tool/function name
   * @param argumentsDelta - Partial arguments JSON string
   * @returns The aggregated tool call, or null if not complete
   */
  addChunk(
    toolCallId: string,
    name: string,
    argumentsDelta?: string
  ): ToolCall | null {
    let aggregated = this.pendingToolCalls.get(toolCallId);

    if (!aggregated) {
      // Check max tool calls limit
      if (this.pendingToolCalls.size + this.completedToolCalls.length >= this.config.maxToolCalls) {
        throw createAgentError(
          `Maximum tool calls limit exceeded: ${this.config.maxToolCalls}`,
          AgentErrorCode.TOOL_EXECUTION_FAILED
        );
      }

      // Create new aggregated tool call
      aggregated = {
        id: toolCallId,
        name,
        argumentsBuffer: '',
      };
      this.pendingToolCalls.set(toolCallId, aggregated);
    }

    // Append arguments delta
    if (argumentsDelta) {
      aggregated.argumentsBuffer += argumentsDelta;
    }

    // Try to parse complete arguments
    try {
      // Validate JSON
      aggregated.argumentsBuffer = JSON.stringify(JSON.parse(aggregated.argumentsBuffer));

      // Mark as complete
      const toolCall: ToolCall = {
        id: toolCallId,
        name,
        arguments: aggregated.argumentsBuffer,
      };

      this.pendingToolCalls.delete(toolCallId);
      this.completedToolCalls.push(toolCall);

      return toolCall;
    } catch {
      // Arguments not complete yet
      return null;
    }
  }

  /**
   * Get all completed tool calls
   *
   * @returns Array of completed tool calls
   */
  getCompleted(): ToolCall[] {
    return [...this.completedToolCalls];
  }

  /**
   * Get any incomplete/pending tool calls
   *
   * @returns Array of pending tool call IDs
   */
  getPending(): string[] {
    return Array.from(this.pendingToolCalls.keys());
  }

  /**
   * Reset the aggregator state
   */
  reset(): void {
    this.pendingToolCalls.clear();
    this.completedToolCalls = [];
  }

  /**
   * Force complete any pending tool calls
   * Returns incomplete tool calls as-is
   *
   * @returns Array of all tool calls (complete and incomplete)
   */
  forceComplete(): ToolCall[] {
    const allCalls = [...this.completedToolCalls];

    for (const [id, aggregated] of this.pendingToolCalls.entries()) {
      allCalls.push({
        id,
        name: aggregated.name,
        arguments: aggregated.argumentsBuffer || '{}',
      });
    }

    this.reset();
    return allCalls;
  }
}

/**
 * Internal representation of an aggregated tool call
 */
interface AggregatedToolCall {
  id: string;
  name: string;
  argumentsBuffer: string;
}

// =============================================================================
// Tool Call Builder
// =============================================================================

/**
 * Builds tool definitions for LLM function calling
 *
 * Converts tool definitions to the format expected by the LLM Gateway.
 */
export class ToolCallBuilder {
  /**
   * Convert tool definitions to LLM tools format
   *
   * @param tools - Array of tool definitions
   * @returns Array of LLM Tool objects
   */
  static buildTools(tools: ToolDefinition[]): Tool[] {
    return tools.map((tool) => ({
      name: tool.name,
      description: tool.description,
      inputSchema: tool.inputSchema,
    }));
  }

  /**
   * Create a tool definition with JSON schema
   *
   * @param name - Tool name
   * @param description - Tool description
   * @param parameters - Parameter schema definition
   * @returns Tool definition
   */
  static createToolDefinition(
    name: string,
    description: string,
    parameters: ToolDefinition['inputSchema']
  ): ToolDefinition {
    return {
      name,
      description,
      inputSchema: parameters,
    };
  }

  /**
   * Validate tool definition
   *
   * @param tool - Tool definition to validate
   * @returns True if valid, throws otherwise
   */
  static validateToolDefinition(tool: ToolDefinition): boolean {
    if (!tool.name || typeof tool.name !== 'string') {
      throw createAgentError(
        'Tool name is required and must be a string',
        AgentErrorCode.TOOL_NOT_AVAILABLE
      );
    }

    if (!tool.description || typeof tool.description !== 'string') {
      throw createAgentError(
        'Tool description is required and must be a string',
        AgentErrorCode.TOOL_NOT_AVAILABLE
      );
    }

    if (!tool.inputSchema || typeof tool.inputSchema !== 'object') {
      throw createAgentError(
        'Tool inputSchema is required and must be an object',
        AgentErrorCode.TOOL_NOT_AVAILABLE
      );
    }

    if (tool.inputSchema.type !== 'object') {
      throw createAgentError(
        'Tool inputSchema type must be "object"',
        AgentErrorCode.TOOL_NOT_AVAILABLE
      );
    }

    if (!tool.inputSchema.properties || typeof tool.inputSchema.properties !== 'object') {
      throw createAgentError(
        'Tool inputSchema properties is required and must be an object',
        AgentErrorCode.TOOL_NOT_AVAILABLE
      );
    }

    return true;
  }
}

// =============================================================================
// Tool Call Executor
// =============================================================================

/**
 * Executes tool calls with proper error handling and tracking
 *
 * Manages the execution of tool calls including:
 * - Parameter validation
 * - Tool execution with timeout
 * - Result formatting
 * - Error handling
 */
export class ToolCallExecutor {
  private config: Required<ToolCallingConfig>;

  constructor(config: ToolCallingConfig = {}) {
    this.config = { ...DEFAULT_TOOL_CALLING_CONFIG, ...config };
  }

  /**
   * Execute a tool call
   *
   * @param context - Tool execution context
   * @param executeFn - Function to execute the tool
   * @returns Tool execution result
   */
  async executeToolCall(
    context: ToolExecutionContext,
    executeFn: (params: DelegateParams) => Promise<ToolResult>
  ): Promise<ToolExecutionResult> {
    const startTime = Date.now();
    const { toolCall, sessionId, messageId } = context;
    void sessionId;
    void messageId;

    try {
      // Parse tool call
      const toolName = toolCall.name ?? '';
      const params = JSON.parse(toolCall.arguments ?? '{}') as DelegateParams;

      // Validate parameters
      if (toolName === 'delegate') {
        validateDelegateParams(params);
      } else {
        throw createAgentError(
          `Unknown tool: ${toolName}`,
          AgentErrorCode.TOOL_NOT_AVAILABLE
        );
      }

      // Create abort controller for timeout
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), this.config.maxExecutionTime);

      try {
        // Execute tool
        const result = await Promise.race([
          executeFn(params),
          new Promise<ToolResult>((_, reject) =>
            controller.signal.addEventListener('abort', () =>
              reject(new Error('Tool execution timeout'))
            )
          ),
        ]);

        clearTimeout(timeoutId);

        const durationMs = Date.now() - startTime;

        return {
          toolCallId: toolCall.id ?? '',
          toolName,
          result,
          durationMs,
          timestamp: new Date(),
        };
      } catch (err) {
        clearTimeout(timeoutId);
        throw err;
      }
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));
      const durationMs = Date.now() - startTime;

      // Check if it's a delegate error
      if (error instanceof DelegateError) {
        return {
          toolCallId: toolCall.id ?? '',
          toolName: toolCall.name ?? '',
          result: {
            success: false,
            error: error.message,
          },
          durationMs,
          timestamp: new Date(),
        };
      }

      // Generic error
      return {
        toolCallId: toolCall.id ?? '',
        toolName: toolCall.name ?? '',
        result: {
          success: false,
          error: error.message,
        },
        durationMs,
        timestamp: new Date(),
      };
    }
  }

  /**
   * Execute multiple tool calls
   *
   * @param toolCalls - Array of tool calls to execute
   * @param context - Tool execution context
   * @param executeFn - Function to execute a tool
   * @returns Array of tool execution results
   */
  async executeToolCalls(
    toolCalls: ToolCall[],
    context: Omit<ToolExecutionContext, 'toolCall'>,
    executeFn: (params: DelegateParams) => Promise<ToolResult>
  ): Promise<ToolExecutionResult[]> {
    const results: ToolExecutionResult[] = [];

    for (const toolCall of toolCalls) {
      if (!this.config.continueOnError && results.length > 0) {
        const lastResult = results[results.length - 1];
        if (!lastResult.result.success) {
          // Stop on error
          break;
        }
      }

      const result = await this.executeToolCall(
        { ...context, toolCall },
        executeFn
      );
      results.push(result);
    }

    return results;
  }

  /**
   * Convert execution result to ToolCallInfo
   *
   * @param executionResult - Tool execution result
   * @returns ToolCallInfo
   */
  static toToolCallInfo(executionResult: ToolExecutionResult): ToolCallInfo {
    return {
      id: executionResult.toolCallId,
      name: executionResult.toolName,
      arguments: {}, // Arguments not stored in execution result
      result: executionResult.result,
      success: executionResult.result.success,
      durationMs: executionResult.durationMs,
      timestamp: executionResult.timestamp,
    };
  }
}

// =============================================================================
// Tool Result Formatter
// =============================================================================

/**
 * Formats tool results for return to the LLM
 *
 * Converts tool execution results into the format expected by the LLM
 * for function calling responses.
 */
export class ToolResultFormatter {
  /**
   * Format a tool execution result for the LLM
   *
   * @param result - Tool execution result
   * @returns Formatted tool result message
   */
  static formatResult(result: ToolExecutionResult): LLMMessage {
    const { toolName, result: toolResult } = result;

    if (toolResult.success) {
      // Success response
      return {
        role: 'user',
        content: this.formatSuccessContent(toolName, toolResult),
      };
    } else {
      // Error response
      return {
        role: 'user',
        content: this.formatErrorContent(toolName, toolResult),
      };
    }
  }

  /**
   * Format multiple tool results for the LLM
   *
   * @param results - Array of tool execution results
   * @returns Array of formatted result messages
   */
  static formatResults(results: ToolExecutionResult[]): LLMMessage[] {
    return results.map((result) => this.formatResult(result));
  }

  /**
   * Format success content
   *
   * @param toolName - Tool name
   * @param result - Tool result
   * @returns Formatted content string
   */
  private static formatSuccessContent(toolName: string, result: ToolResult): string {
    if (result.output) {
      return result.output;
    }

    if (result.data !== undefined) {
      return JSON.stringify(result.data, null, 2);
    }

    return `Tool ${toolName} executed successfully`;
  }

  /**
   * Format error content
   *
   * @param toolName - Tool name
   * @param result - Tool result
   * @returns Formatted error string
   */
  private static formatErrorContent(toolName: string, result: ToolResult): string {
    const error = result.error ?? 'Unknown error';
    return `Error executing tool ${toolName}: ${error}`;
  }

  /**
   * Build tool result message for LLM continuation
   *
   * @param results - Tool execution results
   * @returns Message for LLM continuation
   */
  static buildToolResultMessage(results: ToolExecutionResult[]): string {
    const parts: string[] = [];

    for (const result of results) {
      if (result.result.success) {
        parts.push(`Tool "${result.toolName}" completed successfully`);
        if (result.result.output) {
          parts.push(`Output: ${result.result.output}`);
        }
      } else {
        parts.push(`Tool "${result.toolName}" failed: ${result.result.error ?? 'Unknown error'}`);
      }
    }

    return parts.join('\n\n');
  }
}

// =============================================================================
// Tool Call Validator
// =============================================================================

/**
 * Validates tool calls before execution
 *
 * Performs validation checks including:
 * - Tool name validity
 * - Parameter structure
 * - Required parameters
 * - Parameter types
 */
export class ToolCallValidator {
  private availableTools: Map<string, ToolDefinition>;

  constructor(tools: ToolDefinition[]) {
    this.availableTools = new Map();
    for (const tool of tools) {
      this.availableTools.set(tool.name, tool);
    }
  }

  /**
   * Validate a tool call
   *
   * @param toolCall - Tool call to validate
   * @returns True if valid, throws error otherwise
   */
  validateToolCall(toolCall: ToolCall): boolean {
    const toolName = toolCall.name ?? '';

    // Check if tool exists
    if (!this.availableTools.has(toolName)) {
      throw createAgentError(
        `Tool not found: ${toolName}`,
        AgentErrorCode.TOOL_NOT_AVAILABLE
      );
    }

    // Parse arguments
    let params: Record<string, unknown>;
    try {
      params = JSON.parse(toolCall.arguments ?? '{}');
    } catch {
      throw createAgentError(
        `Invalid JSON in tool arguments: ${toolCall.arguments}`,
        AgentErrorCode.MESSAGE_INVALID
      );
    }

    // Get tool definition
    const tool = this.availableTools.get(toolName)!;

    // Validate against schema
    return this.validateParameters(params, tool.inputSchema);
  }

  /**
   * Validate parameters against schema
   *
   * @param params - Parameters to validate
   * @param schema - JSON schema
   * @returns True if valid
   */
  private validateParameters(
    params: Record<string, unknown>,
    schema: ToolDefinition['inputSchema']
  ): boolean {
    // Check required parameters
    if (schema.required) {
      for (const required of schema.required) {
        if (!(required in params)) {
          throw createAgentError(
            `Missing required parameter: ${required}`,
            AgentErrorCode.MESSAGE_INVALID
          );
        }
      }
    }

    // Validate parameter types
    for (const [key, value] of Object.entries(params)) {
      const property = schema.properties[key];
      if (!property) {
        // Ignore extra parameters
        continue;
      }

      this.validateParameterValue(key, value, property);
    }

    return true;
  }

  /**
   * Validate a parameter value
   *
   * @param name - Parameter name
   * @param value - Parameter value
   * @param property - Property definition
   */
  private validateParameterValue(
    name: string,
    value: unknown,
    property: ToolDefinition['inputSchema']['properties'][string]
  ): void {
    if (value === null || value === undefined) {
      return;
    }

    switch (property.type) {
      case 'string':
        if (typeof value !== 'string') {
          throw createAgentError(
            `Parameter "${name}" must be a string`,
            AgentErrorCode.MESSAGE_INVALID
          );
        }
        break;

      case 'number':
        if (typeof value !== 'number') {
          throw createAgentError(
            `Parameter "${name}" must be a number`,
            AgentErrorCode.MESSAGE_INVALID
          );
        }
        break;

      case 'boolean':
        if (typeof value !== 'boolean') {
          throw createAgentError(
            `Parameter "${name}" must be a boolean`,
            AgentErrorCode.MESSAGE_INVALID
          );
        }
        break;

      case 'array':
        if (!Array.isArray(value)) {
          throw createAgentError(
            `Parameter "${name}" must be an array`,
            AgentErrorCode.MESSAGE_INVALID
          );
        }
        break;

      case 'object':
        if (typeof value !== 'object' || Array.isArray(value)) {
          throw createAgentError(
            `Parameter "${name}" must be an object`,
            AgentErrorCode.MESSAGE_INVALID
          );
        }
        break;
    }
  }

  /**
   * Get available tool names
   *
   * @returns Array of tool names
   */
  getAvailableToolNames(): string[] {
    return Array.from(this.availableTools.keys());
  }
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Create a tool call aggregator with default config
 *
 * @param config - Optional configuration
 * @returns New tool call aggregator
 */
export function createToolCallAggregator(
  config?: ToolCallingConfig
): ToolCallAggregator {
  return new ToolCallAggregator(config);
}

/**
 * Create a tool call executor with default config
 *
 * @param config - Optional configuration
 * @returns New tool call executor
 */
export function createToolCallExecutor(
  config?: ToolCallingConfig
): ToolCallExecutor {
  return new ToolCallExecutor(config);
}

/**
 * Create a tool call validator
 *
 * @param tools - Available tool definitions
 * @returns New tool call validator
 */
export function createToolCallValidator(
  tools: ToolDefinition[]
): ToolCallValidator {
  return new ToolCallValidator(tools);
}

/**
 * Check if a response contains tool calls
 *
 * @param toolCalls - Array of tool calls (may be undefined)
 * @returns True if tool calls are present
 */
export function hasToolCalls(toolCalls?: ToolCall[]): boolean {
  return toolCalls !== undefined && toolCalls.length > 0;
}

/**
 * Extract tool call IDs from tool calls
 *
 * @param toolCalls - Array of tool calls
 * @returns Array of tool call IDs
 */
export function extractToolCallIds(toolCalls: ToolCall[]): string[] {
  return toolCalls.map((tc) => tc.id ?? '').filter(Boolean);
}
