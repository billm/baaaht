// coder-delegation.ts - Coder agent delegation implementation
//
// This file provides the implementation for delegating operations to the Coder agent.
// The Coder agent handles code analysis, generation, review, and execution tasks.
// This module creates tasks, manages execution, and streams results back to the Assistant.
//
// NOTE: This is a stub implementation for Phase 2 preparation. Full implementation
// will be completed when the Coder agent is developed.
//
// Copyright 2026 baaaht project

// =============================================================================
// Configuration
// =============================================================================

/**
 * Configuration for Coder delegation
 */
export interface CoderDelegationConfig {
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
   * Agent ID of the Coder agent to delegate to
   * If not specified, the orchestrator will assign a Coder agent
   */
  coderAgentId?: string;
}

/**
 * Default configuration values
 */
const DEFAULT_CONFIG: Required<CoderDelegationConfig> = {
  defaultTimeout: 120000, // 2 minutes - code operations
  maxTimeout: 600000,     // 10 minutes
  enableStreaming: true,
  coderAgentId: '',
};

// =============================================================================
// Coder Delegation Class
// =============================================================================

/**
 * CoderDelegation handles delegating operations to the Coder agent
 *
 * This class manages the lifecycle of code delegations including:
 * - Creating task configurations from delegate parameters
 * - Executing tasks via the Orchestrator gRPC client
 * - Streaming results back to the caller
 * - Handling errors and retries
 * - Tracking metadata for each delegation
 *
 * Usage:
 * ```typescript
 * const delegation = new CoderDelegation(grpcClient, { coderAgentId: 'coder-1' });
 * const result = await delegation.delegate({
 *   target: DelegateTarget.CODER,
 *   operation: DelegateOperation.ANALYZE_CODE,
 *   parameters: { path: '/path/to/code.ts', analysisType: 'complexity' },
 * });
 * ```
 *
 * NOTE: This is a stub implementation for Phase 2. Full implementation pending
 * Coder agent development.
 */
export class CoderDelegation {
  private grpcClient: Client;
  private config: Required<CoderDelegationConfig>;

  /**
   * Creates a new CoderDelegation instance
   *
   * @param grpcClient - The gRPC client connected to the Orchestrator
   * @param config - Optional configuration for delegation behavior
   */
  constructor(grpcClient: Client, config: CoderDelegationConfig = {}) {
    this.grpcClient = grpcClient;
    this.config = { ...DEFAULT_CONFIG, ...config };
  }

  /**
   * Delegates an operation to the Coder agent
   *
   * @param params - The validated delegation parameters
   * @param sessionId - The session ID for tracking the delegation
   * @returns The result of the delegation
   *
   * NOTE: Stub implementation - will throw an error until Coder agent is available
   */
  async delegate(params: DelegateParams, sessionId: string): Promise<DelegateResult> {
    const startTime = Date.now();
    const metadata: DelegateMetadata = {
      target: params.target,
      operation: params.operation,
      createdAt: new Date(startTime),
    };

    try {
      // STUB: Coder agent not yet implemented
      // When implemented, this will:
      // 1. Create task configuration from delegation parameters
      // 2. Execute the task via gRPC
      // 3. Wait for completion and return result

      metadata.completedAt = new Date();
      metadata.duration = Date.now() - startTime;

      return {
        success: false,
        error: 'Coder agent not yet implemented (Phase 2)',
        taskState: TaskState.TASK_STATE_FAILED,
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
   * NOTE: Stub implementation - will be implemented in Phase 2
   */
  private createTaskConfig(operation: DelegateOperation, parameters: Record<string, unknown>): TaskConfig {
    // STUB: Return minimal config
    // Full implementation will build proper task configs for:
    // - ANALYZE_CODE: static analysis, complexity metrics, security scanning
    // - GENERATE_CODE: code generation from specifications
    // - REVIEW_CODE: code review with best practices checking
    // - EXECUTE_CODE: safe code execution in sandboxed environment

    return {
      command: this.buildCommand(operation, parameters),
      arguments: this.buildArguments(operation, parameters),
      environment: {},
      workingDirectory: undefined,
      timeoutNs: BigInt(this.config.defaultTimeout * 1_000_000),
      parameters: this.buildParametersMap(operation, parameters),
    };
  }

  /**
   * Builds the command string for an operation
   *
   * @private
   */
  private buildCommand(operation: DelegateOperation, parameters: Record<string, unknown>): string {
    switch (operation) {
      case DelegateOperation.ANALYZE_CODE:
        return 'analyze_code';
      case DelegateOperation.GENERATE_CODE:
        return 'generate_code';
      case DelegateOperation.REVIEW_CODE:
        return 'review_code';
      case DelegateOperation.EXECUTE_CODE:
        return 'execute_code';
      default:
        return `coder:${operation}`;
    }
  }

  /**
   * Builds the arguments array for an operation
   *
   * @private
   * NOTE: Stub implementation - will be implemented in Phase 2
   */
  private buildArguments(operation: DelegateOperation, parameters: Record<string, unknown>): string[] {
    const args: string[] = [];

    switch (operation) {
      case DelegateOperation.ANALYZE_CODE:
        // STUB: Will build analysis arguments in Phase 2
        if (parameters.path) args.push(String(parameters.path));
        if (parameters.analysisType) args.push(`--type=${String(parameters.analysisType)}`);
        break;

      case DelegateOperation.GENERATE_CODE:
        // STUB: Will build generation arguments in Phase 2
        if (parameters.spec) args.push('--spec');
        if (parameters.language) args.push(`--language=${String(parameters.language)}`);
        break;

      case DelegateOperation.REVIEW_CODE:
        // STUB: Will build review arguments in Phase 2
        if (parameters.path) args.push(String(parameters.path));
        if (parameters.strict) args.push('--strict');
        break;

      case DelegateOperation.EXECUTE_CODE:
        // STUB: Will build execution arguments in Phase 2
        if (parameters.language) args.push(`--language=${String(parameters.language)}`);
        if (parameters.sandboxed) args.push('--sandboxed');
        break;
    }

    return args;
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
   * Updates the configuration
   *
   * @param config - New configuration values to merge
   */
  updateConfig(config: Partial<CoderDelegationConfig>): void {
    this.config = { ...this.config, ...config };
  }

  /**
   * Gets the current configuration
   */
  getConfig(): Required<CoderDelegationConfig> {
    return { ...this.config };
  }
}

// =============================================================================
// Factory Functions
// =============================================================================

/**
 * Creates a new CoderDelegation instance with the specified gRPC client
 *
 * @param grpcClient - The gRPC client connected to the Orchestrator
 * @param config - Optional configuration for delegation behavior
 * @returns A new CoderDelegation instance
 */
export function createCoderDelegation(
  grpcClient: Client,
  config?: CoderDelegationConfig
): CoderDelegation {
  return new CoderDelegation(grpcClient, config);
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Validates Coder agent operation parameters
 *
 * @param operation - The operation to validate
 * @param parameters - The parameters to validate
 * @throws Error if parameters are invalid
 * NOTE: Stub implementation - will be implemented in Phase 2
 */
export function validateCoderOperation(
  operation: DelegateOperation,
  parameters: Record<string, unknown>
): void {
  switch (operation) {
    case DelegateOperation.ANALYZE_CODE:
      // STUB: Will validate analysis parameters in Phase 2
      if (!parameters.path || typeof parameters.path !== 'string') {
        throw new Error('analyze_code requires a valid "path" parameter');
      }
      break;

    case DelegateOperation.GENERATE_CODE:
      // STUB: Will validate generation parameters in Phase 2
      if (!parameters.spec || typeof parameters.spec !== 'string') {
        throw new Error('generate_code requires a valid "spec" parameter');
      }
      break;

    case DelegateOperation.REVIEW_CODE:
      // STUB: Will validate review parameters in Phase 2
      if (!parameters.path || typeof parameters.path !== 'string') {
        throw new Error('review_code requires a valid "path" parameter');
      }
      break;

    case DelegateOperation.EXECUTE_CODE:
      // STUB: Will validate execution parameters in Phase 2
      if (!parameters.code || typeof parameters.code !== 'string') {
        throw new Error('execute_code requires a valid "code" parameter');
      }
      break;
  }
}

/**
 * Checks if an operation is supported by the Coder agent
 *
 * @param operation - The operation to check
 * @returns True if the operation is supported
 */
export function isCoderOperation(operation: DelegateOperation): boolean {
  const coderOperations: DelegateOperation[] = [
    DelegateOperation.ANALYZE_CODE,
    DelegateOperation.GENERATE_CODE,
    DelegateOperation.REVIEW_CODE,
    DelegateOperation.EXECUTE_CODE,
  ];

  return coderOperations.includes(operation);
}
