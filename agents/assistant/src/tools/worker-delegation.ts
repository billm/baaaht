// worker-delegation.ts - Worker agent delegation implementation
//
// This file provides the implementation for delegating operations to the Worker agent.
// The Worker agent handles file operations (read, write, delete, list, search) and
// web operations (search, fetch). This module creates tasks, manages execution,
// and streams results back to the Assistant.
//
// Copyright 2026 baaaht project

import type { Client } from '@grpc/grpc-js';
import {
  TaskType,
  TaskPriority,
  TaskState,
  type TaskConfig,
  type TaskResult,
  type ExecuteTaskRequest,
  type ExecuteTaskResponse,
  type Task,
} from '../proto/agent.js';
import { DelegateOperation } from './types.js';
import type {
  DelegateParams,
  DelegateResult,
  DelegateMetadata,
  ReadFileParams,
  WriteFileParams,
  DeleteFileParams,
  ListFilesParams,
  SearchFilesParams,
  WebSearchParams,
  WebFetchParams,
} from './types.js';

// =============================================================================
// Configuration
// =============================================================================

/**
 * Configuration for Worker delegation
 */
export interface WorkerDelegationConfig {
  /**
   * Default timeout for delegation operations in milliseconds
   */
  defaultTimeout?: number;

  /**
   * Maximum timeout for delegation operations in milliseconds
   */
  maxTimeout?: number;

  /**
   * Whether to enable streaming results
   */
  enableStreaming?: boolean;

  /**
   * Agent ID of the Worker agent to delegate to
   * If not specified, the orchestrator will assign a Worker agent
   */
  workerAgentId?: string;
}

/**
 * Default configuration values
 */
const DEFAULT_CONFIG: Required<WorkerDelegationConfig> = {
  defaultTimeout: 60000,
  maxTimeout: 300000,
  enableStreaming: true,
  workerAgentId: '',
};

// =============================================================================
// Worker Delegation Class
// =============================================================================

/**
 * WorkerDelegation handles delegating operations to the Worker agent
 *
 * This class manages the lifecycle of delegations including:
 * - Creating task configurations from delegate parameters
 * - Executing tasks via the Orchestrator gRPC client
 * - Streaming results back to the caller
 * - Handling errors and retries
 * - Tracking metadata for each delegation
 *
 * Usage:
 * ```typescript
 * const delegation = new WorkerDelegation(grpcClient, { workerAgentId: 'worker-1' });
 * const result = await delegation.delegate({
 *   target: DelegateTarget.WORKER,
 *   operation: DelegateOperation.READ_FILE,
 *   parameters: { path: '/path/to/file.txt' },
 * });
 * ```
 */
export class WorkerDelegation {
  private grpcClient: Client;
  private config: Required<WorkerDelegationConfig>;

  /**
   * Creates a new WorkerDelegation instance
   *
   * @param grpcClient - The gRPC client connected to the Orchestrator
   * @param config - Optional configuration for delegation behavior
   */
  constructor(grpcClient: Client, config: WorkerDelegationConfig = {}) {
    this.grpcClient = grpcClient;
    this.config = { ...DEFAULT_CONFIG, ...config };
  }

  /**
   * Delegates an operation to the Worker agent
   *
   * @param params - The validated delegation parameters
   * @param sessionId - The session ID for tracking the delegation
   * @returns The result of the delegation
   */
  async delegate(params: DelegateParams, sessionId: string): Promise<DelegateResult> {
    const startTime = Date.now();
    const metadata: DelegateMetadata = {
      target: params.target,
      operation: params.operation,
      createdAt: new Date(startTime),
    };

    try {
      // Create task configuration from delegation parameters
      const taskConfig = this.createTaskConfig(params.operation, params.parameters);

      // Determine task priority
      const priority = this.mapPriority(params.priority);

      // Create the execution request
      const request: ExecuteTaskRequest = {
        agentId: this.config.workerAgentId || '', // Empty = orchestrator assigns
        sessionId,
        config: taskConfig,
        type: this.mapOperationToTaskType(params.operation),
        priority,
      };

      // Execute the task via gRPC
      const response = await this.executeTask(request);

      // Wait for task completion and get result
      const result = await this.waitForCompletion(response.taskId!);

      // Update metadata
      metadata.completedAt = new Date();
      metadata.duration = Date.now() - startTime;
      metadata.agentId = result.agentId;

      // Return formatted result
      return {
        success: result.state === TaskState.TASK_STATE_COMPLETED,
        taskId: response.taskId,
        data: result.data,
        output: result.output,
        error: result.error,
        taskState: result.state,
        metadata,
      };
    } catch (err) {
      const error = err as Error;

      metadata.completedAt = new Date();
      metadata.duration = Date.now() - startTime;

      return {
        success: false,
        error: `Delegation failed: ${error.message}`,
        taskState: TaskState.TASK_STATE_FAILED,
        metadata,
      };
    }
  }

  /**
   * Creates a TaskConfig from delegation parameters
   *
   * @private
   */
  private createTaskConfig(operation: DelegateOperation, parameters: Record<string, unknown>): TaskConfig {
    // Build command from operation
    const command = this.buildCommand(operation, parameters);

    // Build arguments array
    const arguments_ = this.buildArguments(operation, parameters);

    // Build environment variables for parameters
    const environment = this.buildEnvironment(operation, parameters);

    // Build parameters map
    const parametersMap = this.buildParametersMap(operation, parameters);

    // Calculate timeout in nanoseconds
    const timeoutNs = BigInt(this.config.defaultTimeout * 1_000_000);

    return {
      command,
      arguments: arguments_,
      environment,
      workingDirectory: parameters.workingDirectory as string | undefined,
      timeoutNs,
      parameters: parametersMap,
    };
  }

  /**
   * Builds the command string for an operation
   *
   * @private
   */
  private buildCommand(operation: DelegateOperation, _parameters: Record<string, unknown>): string {
    switch (operation) {
      case DelegateOperation.READ_FILE:
        return 'read_file';
      case DelegateOperation.WRITE_FILE:
        return 'write_file';
      case DelegateOperation.DELETE_FILE:
        return 'delete_file';
      case DelegateOperation.LIST_FILES:
        return 'list_files';
      case DelegateOperation.SEARCH_FILES:
        return 'search_files';
      case DelegateOperation.WEB_SEARCH:
        return 'web_search';
      case DelegateOperation.WEB_FETCH:
        return 'web_fetch';
      default:
        return `worker:${operation}`;
    }
  }

  /**
   * Builds the arguments array for an operation
   *
   * @private
   */
  private buildArguments(operation: DelegateOperation, parameters: Record<string, unknown>): string[] {
    const args: string[] = [];

    switch (operation) {
      case DelegateOperation.READ_FILE:
        if (parameters.path) args.push(String(parameters.path));
        if (parameters.offset !== undefined) args.push(`--offset=${String(parameters.offset)}`);
        if (parameters.limit !== undefined) args.push(`--limit=${String(parameters.limit)}`);
        break;

      case DelegateOperation.WRITE_FILE:
        if (parameters.path) args.push(String(parameters.path));
        if (parameters.createParents) args.push('--create-parents');
        break;

      case DelegateOperation.DELETE_FILE:
        if (parameters.path) args.push(String(parameters.path));
        if (parameters.recursive) args.push('--recursive');
        break;

      case DelegateOperation.LIST_FILES:
        if (parameters.path) args.push(String(parameters.path));
        if (parameters.pattern) args.push(`--pattern=${String(parameters.pattern)}`);
        if (parameters.maxDepth !== undefined) args.push(`--max-depth=${String(parameters.maxDepth)}`);
        break;

      case DelegateOperation.SEARCH_FILES:
        if (parameters.path) args.push(String(parameters.path));
        if (parameters.pattern) args.push(String(parameters.pattern));
        if (parameters.recursive) args.push('--recursive');
        break;

      case DelegateOperation.WEB_SEARCH:
        if (parameters.query) args.push(String(parameters.query));
        if (parameters.maxResults !== undefined) args.push(`--max-results=${String(parameters.maxResults)}`);
        break;

      case DelegateOperation.WEB_FETCH:
        if (parameters.url) args.push(String(parameters.url));
        if (parameters.method) args.push(`--method=${String(parameters.method)}`);
        break;
    }

    return args;
  }

  /**
   * Builds environment variables from parameters
   *
   * @private
   */
  private buildEnvironment(operation: DelegateOperation, parameters: Record<string, unknown>): Record<string, string> {
    const env: Record<string, string> = {};

    // For write_file, pass content via environment
    if (operation === DelegateOperation.WRITE_FILE && parameters.content) {
      env.FILE_CONTENT = String(parameters.content);
    }

    // For web_fetch, pass headers via environment
    if (operation === DelegateOperation.WEB_FETCH && parameters.headers) {
      env.FETCH_HEADERS = JSON.stringify(parameters.headers);
    }

    // For web_fetch, pass body via environment
    if (operation === DelegateOperation.WEB_FETCH && parameters.body) {
      env.FETCH_BODY = String(parameters.body);
    }

    return env;
  }

  /**
   * Builds the parameters map for an operation
   *
   * @private
   */
  private buildParametersMap(operation: DelegateOperation, parameters: Record<string, unknown>): Record<string, string> {
    const paramsMap: Record<string, string> = {};

    // Copy all string/number/boolean parameters
    for (const [key, value] of Object.entries(parameters)) {
      if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
        paramsMap[key] = String(value);
      }
    }

    // Add operation type
    paramsMap.operation = operation;

    return paramsMap;
  }

  /**
   * Maps a DelegateOperation to a TaskType
   *
   * @private
   */
  private mapOperationToTaskType(operation: DelegateOperation): TaskType {
    switch (operation) {
      case DelegateOperation.READ_FILE:
      case DelegateOperation.WRITE_FILE:
      case DelegateOperation.DELETE_FILE:
      case DelegateOperation.LIST_FILES:
      case DelegateOperation.SEARCH_FILES:
        return TaskType.TASK_TYPE_FILE_OPERATION;

      case DelegateOperation.WEB_SEARCH:
      case DelegateOperation.WEB_FETCH:
        return TaskType.TASK_TYPE_NETWORK_REQUEST;

      default:
        return TaskType.TASK_TYPE_TOOL_EXECUTION;
    }
  }

  /**
   * Maps a delegate priority to a TaskPriority
   *
   * @private
   */
  private mapPriority(priority?: TaskPriority): TaskPriority {
    if (priority !== undefined) {
      return priority;
    }
    return TaskPriority.TASK_PRIORITY_NORMAL;
  }

  /**
   * Executes a task via the gRPC client
   *
   * @private
   */
  private executeTask(request: ExecuteTaskRequest): Promise<ExecuteTaskResponse> {
    return new Promise((resolve, reject) => {
      const deadline = this.getDeadline();
      const taskClient = this.grpcClient as unknown as {
        executeTask: (
          request: ExecuteTaskRequest,
          options: { deadline: Date },
          callback: (err: Error | null, response: ExecuteTaskResponse) => void
        ) => void;
      };

      taskClient.executeTask(
        request,
        { deadline },
        (err: Error | null, response: ExecuteTaskResponse) => {
          if (err) {
            reject(new Error(`Task execution failed: ${err.message}`));
            return;
          }

          if (!response.taskId) {
            reject(new Error('No task ID returned from orchestrator'));
            return;
          }

          resolve(response);
        }
      );
    });
  }

  /**
   * Waits for a task to complete and returns the result
   *
   * @private
   */
  private async waitForCompletion(taskId: string): Promise<{
    state: TaskState;
    data?: unknown;
    output?: string;
    error?: string;
    agentId?: string;
  }> {
    // For now, implement polling-based waiting
    // In the future, this could use the StreamTaskClient for real-time updates

    const timeout = this.config.defaultTimeout;
    const startTime = Date.now();
    const pollInterval = 500; // Poll every 500ms

    while (Date.now() - startTime < timeout) {
      try {
        const status = await this.getTaskStatus(taskId);

        if (status.task) {
          const task = status.task;

          // Check if task is in a terminal state
          if (
            task.state === TaskState.TASK_STATE_COMPLETED ||
            task.state === TaskState.TASK_STATE_FAILED ||
            task.state === TaskState.TASK_STATE_CANCELLED ||
            task.state === TaskState.TASK_STATE_TIMEOUT
          ) {
            // Extract result
            const result: {
              state: TaskState;
              data?: unknown;
              output?: string;
              error?: string;
              agentId?: string;
            } = {
              state: task.state!,
              agentId: task.agentId,
            };

            if (task.result) {
              result.data = this.parseTaskResult(task.result);
              result.output = task.result.outputText;
            }

            if (task.error) {
              result.error = task.error.message;
            }

            return result;
          }
        }
      } catch {
        // Continue polling on error
      }

      // Wait before next poll
      await new Promise((resolve) => setTimeout(resolve, pollInterval));
    }

    // Timeout reached
    return {
      state: TaskState.TASK_STATE_TIMEOUT,
      error: `Task ${taskId} timed out after ${timeout}ms`,
    };
  }

  /**
   * Gets the current status of a task
   *
   * @private
   */
  private getTaskStatus(taskId: string): Promise<{ task?: Task }> {
    return new Promise((resolve, reject) => {
      const deadline = this.getDeadline();
      const taskClient = this.grpcClient as unknown as {
        getTaskStatus: (
          request: { taskId: string },
          options: { deadline: Date },
          callback: (err: Error | null, response: { task?: Task }) => void
        ) => void;
      };

      taskClient.getTaskStatus(
        { taskId },
        { deadline },
        (err: Error | null, response: { task?: Task }) => {
          if (err) {
            reject(err);
            return;
          }

          resolve(response);
        }
      );
    });
  }

  /**
   * Parses a task result into structured data
   *
   * @private
   */
  private parseTaskResult(result: TaskResult): unknown {
    if (result.outputData) {
      // Try to parse as JSON
      try {
        return JSON.parse(new TextDecoder().decode(result.outputData));
      } catch {
        // Return as string if not JSON
        return new TextDecoder().decode(result.outputData);
      }
    }

    if (result.outputText) {
      // Try to parse as JSON
      try {
        return JSON.parse(result.outputText);
      } catch {
        return result.outputText;
      }
    }

    return null;
  }

  /**
   * Creates a deadline for gRPC requests based on timeout configuration
   *
   * @private
   */
  private getDeadline(): Date {
    const deadline = new Date();
    deadline.setMilliseconds(deadline.getMilliseconds() + this.config.defaultTimeout);
    return deadline;
  }

  /**
   * Updates the configuration
   *
   * @param config - New configuration values to merge
   */
  updateConfig(config: Partial<WorkerDelegationConfig>): void {
    this.config = { ...this.config, ...config };
  }

  /**
   * Gets the current configuration
   */
  getConfig(): Required<WorkerDelegationConfig> {
    return { ...this.config };
  }
}

// =============================================================================
// Factory Functions
// =============================================================================

/**
 * Creates a new WorkerDelegation instance with the specified gRPC client
 *
 * @param grpcClient - The gRPC client connected to the Orchestrator
 * @param config - Optional configuration for delegation behavior
 * @returns A new WorkerDelegation instance
 */
export function createWorkerDelegation(
  grpcClient: Client,
  config?: WorkerDelegationConfig
): WorkerDelegation {
  return new WorkerDelegation(grpcClient, config);
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Validates Worker agent operation parameters
 *
 * @param operation - The operation to validate
 * @param parameters - The parameters to validate
 * @throws Error if parameters are invalid
 */
export function validateWorkerOperation(
  operation: DelegateOperation,
  parameters: Record<string, unknown>
): void {
  switch (operation) {
    case DelegateOperation.READ_FILE: {
      const params = parameters as unknown as ReadFileParams;
      if (!params.path || typeof params.path !== 'string') {
        throw new Error('read_file requires a valid "path" parameter');
      }
      break;
    }

    case DelegateOperation.WRITE_FILE: {
      const params = parameters as unknown as WriteFileParams;
      if (!params.path || typeof params.path !== 'string') {
        throw new Error('write_file requires a valid "path" parameter');
      }
      if (params.content === undefined || typeof params.content !== 'string') {
        throw new Error('write_file requires a valid "content" parameter');
      }
      break;
    }

    case DelegateOperation.DELETE_FILE: {
      const params = parameters as unknown as DeleteFileParams;
      if (!params.path || typeof params.path !== 'string') {
        throw new Error('delete_file requires a valid "path" parameter');
      }
      break;
    }

    case DelegateOperation.LIST_FILES: {
      const params = parameters as unknown as ListFilesParams;
      if (!params.path || typeof params.path !== 'string') {
        throw new Error('list_files requires a valid "path" parameter');
      }
      break;
    }

    case DelegateOperation.SEARCH_FILES: {
      const params = parameters as unknown as SearchFilesParams;
      if (!params.path || typeof params.path !== 'string') {
        throw new Error('search_files requires a valid "path" parameter');
      }
      if (!params.pattern || typeof params.pattern !== 'string') {
        throw new Error('search_files requires a valid "pattern" parameter');
      }
      break;
    }

    case DelegateOperation.WEB_SEARCH: {
      const params = parameters as unknown as WebSearchParams;
      if (!params.query || typeof params.query !== 'string') {
        throw new Error('web_search requires a valid "query" parameter');
      }
      break;
    }

    case DelegateOperation.WEB_FETCH: {
      const params = parameters as unknown as WebFetchParams;
      if (!params.url || typeof params.url !== 'string') {
        throw new Error('web_fetch requires a valid "url" parameter');
      }
      break;
    }
  }
}

/**
 * Checks if an operation is supported by the Worker agent
 *
 * @param operation - The operation to check
 * @returns True if the operation is supported
 */
export function isWorkerOperation(operation: DelegateOperation): boolean {
  const workerOperations: DelegateOperation[] = [
    DelegateOperation.READ_FILE,
    DelegateOperation.WRITE_FILE,
    DelegateOperation.DELETE_FILE,
    DelegateOperation.LIST_FILES,
    DelegateOperation.SEARCH_FILES,
    DelegateOperation.WEB_SEARCH,
    DelegateOperation.WEB_FETCH,
  ];

  return workerOperations.includes(operation);
}
