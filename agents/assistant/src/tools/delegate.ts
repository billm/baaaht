// delegate.ts - Delegate tool for dispatching work to specialized agents
//
// This file provides the delegate tool, which is the Assistant's only tool.
// It dispatches work to specialized agents (Worker, Researcher, Coder) that
// have direct tool access.
//
// Copyright 2026 baaaht project

import type { Tool, ToolInputSchema } from '../proto/llm.js';
import { TaskPriority, AgentType } from '../proto/agent.js';
import {
  DelegateTarget,
  DelegateOperation,
} from './types.js';
import type {
  DelegateParams,
  DelegateResult,
  DelegateToolConfig,
} from './types.js';

// =============================================================================
// Default Configuration
// =============================================================================

/**
 * Default configuration for the delegate tool
 */
export const DEFAULT_DELEGATE_CONFIG: Required<DelegateToolConfig> = {
  defaultTimeout: 60000, // 60 seconds
  maxTimeout: 300000, // 5 minutes
  defaultPriority: TaskPriority.TASK_PRIORITY_NORMAL, // default priority (numeric enum)
  enableStreaming: true,
  maxConcurrent: 10,
  retry: {
    maxAttempts: 3,
    initialDelay: 1000, // 1 second
    maxDelay: 10000, // 10 seconds
    backoffMultiplier: 2,
  },
};

// =============================================================================
// Tool Definition
// =============================================================================

/**
 * Creates the tool definition for the delegate tool
 *
 * This tool is registered with the LLM so it can request delegations
 * when the user asks for operations that require specialized agents.
 *
 * @param config - Optional configuration for the delegate tool
 * @returns The tool definition for registration with the LLM
 */
export function createDelegateToolDefinition(config?: DelegateToolConfig): Tool {
  void config;

  return {
    name: 'delegate',
    description: `Delegate an operation to a specialized agent.

This is the only tool available to the Assistant. Use it to dispatch work to agents
with specific capabilities:
- worker: File operations, web searches, and basic tasks
- researcher: Deep research tasks (coming in Phase 2)
- coder: Code analysis and generation (coming in Phase 2)

Available operations:
- File operations: read_file, write_file, delete_file, list_files, search_files
- Web operations: web_search, web_fetch
- Research operations: deep_research, synthesize_sources (Phase 2)
- Code operations: analyze_code, generate_code, review_code, execute_code (Phase 2)

The Assistant has no direct tool access - all work requiring tools must be delegated.`,
    inputSchema: createDelegateInputSchema(),
  };
}

/**
 * Creates the JSON schema for delegate tool input
 */
function createDelegateInputSchema(): ToolInputSchema {
  return {
    type: 'object',
    properties: {
      target: {
        type: 'string',
        description: 'The target agent to delegate to. Supported values: "worker", "researcher" (Phase 2), "coder" (Phase 2).',
        enum: ['worker', 'researcher', 'coder'],
      },
      operation: {
        type: 'string',
        description: 'The operation to perform. Supported operations depend on the target agent.',
        enum: [
          // File operations
          'read_file',
          'write_file',
          'delete_file',
          'list_files',
          'search_files',
          // Web operations
          'web_search',
          'web_fetch',
          // Research operations (Phase 2)
          'deep_research',
          'synthesize_sources',
          // Code operations (Phase 2)
          'analyze_code',
          'generate_code',
          'review_code',
          'execute_code',
        ],
      },
      parameters: {
        type: 'object',
        description: 'Operation-specific parameters. The structure depends on the operation being performed.',
      },
      timeout: {
        type: 'number',
        description: 'Optional timeout in milliseconds. Defaults to 60000 (60 seconds). Maximum is 300000 (5 minutes).',
      },
      priority: {
        type: 'number',
        description:
          'Optional task priority as a numeric level. Supported values: 1 (low), 2 (normal), 3 (high), 4 (critical). Defaults to 2 (normal).',
      },
    },
    required: ['target', 'operation', 'parameters'],
  };
}

// =============================================================================
// Parameter Validation
// =============================================================================

/**
 * Validates delegate parameters
 *
 * @param params - The parameters to validate
 * @param config - Optional configuration for validation
 * @returns Validated parameters with defaults applied
 * @throws Error if parameters are invalid
 */
export function validateDelegateParams(
  params: DelegateParams,
  config?: DelegateToolConfig
): DelegateParams {
  const effectiveConfig = { ...DEFAULT_DELEGATE_CONFIG, ...config };

  // Validate target
  if (!Object.values(DelegateTarget).includes(params.target)) {
    throw new Error(`Invalid target: ${params.target}. Must be one of: ${Object.values(DelegateTarget).join(', ')}`);
  }

  // Validate operation
  if (!Object.values(DelegateOperation).includes(params.operation)) {
    throw new Error(`Invalid operation: ${params.operation}. Must be one of: ${Object.values(DelegateOperation).join(', ')}`);
  }

  // Validate target-operation compatibility
  validateTargetOperationCompatibility(params.target, params.operation);

  // Validate operation-specific parameters
  validateOperationParameters(params.operation, params.parameters);

  // Apply defaults
  const validated: DelegateParams = {
    ...params,
    timeout: params.timeout ?? effectiveConfig.defaultTimeout,
    priority: params.priority ?? effectiveConfig.defaultPriority,
  };

  // Validate timeout
  if (validated.timeout! > effectiveConfig.maxTimeout) {
    throw new Error(`Timeout exceeds maximum: ${validated.timeout} > ${effectiveConfig.maxTimeout}`);
  }

  return validated;
}

/**
 * Validates that an operation is compatible with its target agent
 */
function validateTargetOperationCompatibility(target: DelegateTarget, operation: DelegateOperation): void {
  const workerOps: DelegateOperation[] = [
    DelegateOperation.READ_FILE,
    DelegateOperation.WRITE_FILE,
    DelegateOperation.DELETE_FILE,
    DelegateOperation.LIST_FILES,
    DelegateOperation.SEARCH_FILES,
    DelegateOperation.WEB_SEARCH,
    DelegateOperation.WEB_FETCH,
  ];

  const researcherOps: DelegateOperation[] = [
    DelegateOperation.DEEP_RESEARCH,
    DelegateOperation.SYNTHESIZE_SOURCES,
  ];

  const coderOps: DelegateOperation[] = [
    DelegateOperation.ANALYZE_CODE,
    DelegateOperation.GENERATE_CODE,
    DelegateOperation.REVIEW_CODE,
    DelegateOperation.EXECUTE_CODE,
  ];

  switch (target) {
    case DelegateTarget.WORKER:
      if (!workerOps.includes(operation)) {
        throw new Error(`Operation ${operation} is not supported by worker agent. Supported: ${workerOps.join(', ')}`);
      }
      break;

    case DelegateTarget.RESEARCHER:
      if (!researcherOps.includes(operation)) {
        throw new Error(`Operation ${operation} is not supported by researcher agent. Supported: ${researcherOps.join(', ')}`);
      }
      break;

    case DelegateTarget.CODER:
      if (!coderOps.includes(operation)) {
        throw new Error(`Operation ${operation} is not supported by coder agent. Supported: ${coderOps.join(', ')}`);
      }
      break;
  }
}

/**
 * Validates operation-specific parameters
 */
function validateOperationParameters(operation: DelegateOperation, parameters: Record<string, unknown>): void {
  switch (operation) {
    case DelegateOperation.READ_FILE:
      validateReadFileParams(parameters);
      break;

    case DelegateOperation.WRITE_FILE:
      validateWriteFileParams(parameters);
      break;

    case DelegateOperation.DELETE_FILE:
      validateDeleteFileParams(parameters);
      break;

    case DelegateOperation.LIST_FILES:
      validateListFilesParams(parameters);
      break;

    case DelegateOperation.SEARCH_FILES:
      validateSearchFilesParams(parameters);
      break;

    case DelegateOperation.WEB_SEARCH:
      validateWebSearchParams(parameters);
      break;

    case DelegateOperation.WEB_FETCH:
      validateWebFetchParams(parameters);
      break;

    // Phase 2 operations - stub validation for now
    case DelegateOperation.DEEP_RESEARCH:
    case DelegateOperation.SYNTHESIZE_SOURCES:
    case DelegateOperation.ANALYZE_CODE:
    case DelegateOperation.GENERATE_CODE:
    case DelegateOperation.REVIEW_CODE:
    case DelegateOperation.EXECUTE_CODE:
      // For Phase 2, just validate that parameters exist
      if (Object.keys(parameters).length === 0) {
        throw new Error(`Operation ${operation} requires parameters`);
      }
      break;
  }
}

/**
 * Validates read_file parameters
 */
function validateReadFileParams(params: Record<string, unknown>): void {
  if (typeof params.path !== 'string' || params.path.length === 0) {
    throw new Error('read_file requires "path" parameter');
  }
  if (params.offset !== undefined && typeof params.offset !== 'number') {
    throw new Error('read_file "offset" must be a number');
  }
  if (params.limit !== undefined && typeof params.limit !== 'number') {
    throw new Error('read_file "limit" must be a number');
  }
}

/**
 * Validates write_file parameters
 */
function validateWriteFileParams(params: Record<string, unknown>): void {
  if (typeof params.path !== 'string' || params.path.length === 0) {
    throw new Error('write_file requires "path" parameter');
  }
  if (typeof params.content !== 'string') {
    throw new Error('write_file requires "content" parameter');
  }
}

/**
 * Validates delete_file parameters
 */
function validateDeleteFileParams(params: Record<string, unknown>): void {
  if (typeof params.path !== 'string' || params.path.length === 0) {
    throw new Error('delete_file requires "path" parameter');
  }
}

/**
 * Validates list_files parameters
 */
function validateListFilesParams(params: Record<string, unknown>): void {
  if (typeof params.path !== 'string' || params.path.length === 0) {
    throw new Error('list_files requires "path" parameter');
  }
}

/**
 * Validates search_files parameters
 */
function validateSearchFilesParams(params: Record<string, unknown>): void {
  if (typeof params.path !== 'string' || params.path.length === 0) {
    throw new Error('search_files requires "path" parameter');
  }
  if (typeof params.pattern !== 'string' || params.pattern.length === 0) {
    throw new Error('search_files requires "pattern" parameter');
  }
}

/**
 * Validates web_search parameters
 */
function validateWebSearchParams(params: Record<string, unknown>): void {
  if (typeof params.query !== 'string' || params.query.length === 0) {
    throw new Error('web_search requires "query" parameter');
  }
}

/**
 * Validates web_fetch parameters
 */
function validateWebFetchParams(params: Record<string, unknown>): void {
  if (typeof params.url !== 'string' || params.url.length === 0) {
    throw new Error('web_fetch requires "url" parameter');
  }
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Parses raw LLM tool call arguments into DelegateParams
 *
 * @param toolCall - The tool call from the LLM response
 * @returns Parsed delegate parameters
 * @throws Error if arguments are invalid or cannot be parsed
 */
export function parseDelegateToolCall(toolCall: { arguments: string }): DelegateParams {
  try {
    const parsed = JSON.parse(toolCall.arguments) as DelegateParams;

    // Ensure required fields are present
    if (!parsed.target || !parsed.operation || !parsed.parameters) {
      throw new Error('Missing required fields: target, operation, parameters');
    }

    return parsed;
  } catch (err) {
    const error = err as Error;
    throw new Error(`Failed to parse delegate tool call: ${error.message}`);
  }
}

/**
 * Formats a delegate result for inclusion in an LLM response
 *
 * @param result - The delegate result to format
 * @returns A formatted string for inclusion in the conversation
 */
export function formatDelegateResult(result: DelegateResult): string {
  if (!result.success) {
    return `Delegation failed: ${result.error}`;
  }

  let output = `Successfully delegated ${result.metadata?.operation} to ${result.metadata?.target}`;

  if (result.output) {
    output += `\n\nResult:\n${result.output}`;
  }

  if (result.metadata?.duration) {
    output += `\n\nCompleted in ${result.metadata.duration}ms`;
  }

  return output;
}

/**
 * Gets the agent type enum value for a delegate target
 *
 * @param target - The delegate target
 * @returns The corresponding agent type from proto
 */
export function targetToAgentType(target: DelegateTarget): AgentType {
  switch (target) {
    case DelegateTarget.WORKER:
      return AgentType.AGENT_TYPE_WORKER;
    case DelegateTarget.RESEARCHER:
      return AgentType.AGENT_TYPE_WORKER; // Researcher is a specialized worker for now
    case DelegateTarget.CODER:
      return AgentType.AGENT_TYPE_WORKER; // Coder is a specialized worker for now
    default:
      return AgentType.AGENT_TYPE_WORKER;
  }
}
