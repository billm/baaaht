// researcher-delegation.ts - Researcher agent delegation implementation
//
// This file provides the implementation for delegating operations to the Researcher agent.
// The Researcher agent handles deep research tasks including comprehensive web research,
// source synthesis, and knowledge gathering. This module creates tasks, manages execution,
// and streams results back to the Assistant.
//
// NOTE: This is a stub implementation for Phase 2 preparation. Full implementation
// will be completed when the Researcher agent is developed.
//
// Copyright 2026 baaaht project

import type { Client } from '@grpc/grpc-js';
import {
  TaskState,
  type TaskConfig,
} from '../proto/agent.js';
import { DelegateOperation } from './types.js';
import type {
  DelegateParams,
  DelegateResult,
  DelegateMetadata,
} from './types.js';

// =============================================================================
// Configuration
// =============================================================================

/**
 * Configuration for Researcher delegation
 */
export interface ResearcherDelegationConfig {
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
   * Agent ID of the Researcher agent to delegate to
   * If not specified, the orchestrator will assign a Researcher agent
   */
  researcherAgentId?: string;
}

/**
 * Default configuration values
 */
const DEFAULT_CONFIG: Required<ResearcherDelegationConfig> = {
  defaultTimeout: 300000, // 5 minutes - research operations take longer
  maxTimeout: 900000,     // 15 minutes
  enableStreaming: true,
  researcherAgentId: '',
};

// =============================================================================
// Researcher Delegation Class
// =============================================================================

/**
 * ResearcherDelegation handles delegating operations to the Researcher agent
 *
 * This class manages the lifecycle of research delegations including:
 * - Creating task configurations from delegate parameters
 * - Executing tasks via the Orchestrator gRPC client
 * - Streaming results back to the caller
 * - Handling errors and retries
 * - Tracking metadata for each delegation
 *
 * Usage:
 * ```typescript
 * const delegation = new ResearcherDelegation(grpcClient, { researcherAgentId: 'researcher-1' });
 * const result = await delegation.delegate({
 *   target: DelegateTarget.RESEARCHER,
 *   operation: DelegateOperation.DEEP_RESEARCH,
 *   parameters: { query: 'latest trends in AI' },
 * });
 * ```
 *
 * NOTE: This is a stub implementation for Phase 2. Full implementation pending
 * Researcher agent development.
 */
export class ResearcherDelegation {
  private grpcClient: Client;
  private config: Required<ResearcherDelegationConfig>;

  /**
   * Creates a new ResearcherDelegation instance
   *
   * @param grpcClient - The gRPC client connected to the Orchestrator
   * @param config - Optional configuration for delegation behavior
   */
  constructor(grpcClient: Client, config: ResearcherDelegationConfig = {}) {
    this.grpcClient = grpcClient;
    this.config = { ...DEFAULT_CONFIG, ...config };
  }

  /**
   * Delegates an operation to the Researcher agent
   *
   * @param params - The validated delegation parameters
   * @param sessionId - The session ID for tracking the delegation
   * @returns The result of the delegation
   *
   * NOTE: Stub implementation - will throw an error until Researcher agent is available
   */
  async delegate(params: DelegateParams, sessionId: string): Promise<DelegateResult> {
    void sessionId;
    const startTime = Date.now();
    const metadata: DelegateMetadata = {
      target: params.target,
      operation: params.operation,
      createdAt: new Date(startTime),
    };

    try {
      void this.grpcClient;
      void this.createTaskConfig;

      // STUB: Researcher agent not yet implemented
      // When implemented, this will:
      // 1. Create task configuration from delegation parameters
      // 2. Execute the task via gRPC
      // 3. Wait for completion and return result

      metadata.completedAt = new Date();
      metadata.duration = Date.now() - startTime;

      return {
        success: false,
        error: 'Researcher agent not yet implemented (Phase 2)',
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
    // - DEEP_RESEARCH: comprehensive web research with multiple sources
    // - SYNTHESIZE_SOURCES: combine and analyze multiple research sources

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
  private buildCommand(operation: DelegateOperation, _parameters: Record<string, unknown>): string {
    switch (operation) {
      case DelegateOperation.DEEP_RESEARCH:
        return 'deep_research';
      case DelegateOperation.SYNTHESIZE_SOURCES:
        return 'synthesize_sources';
      default:
        return `researcher:${operation}`;
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
      case DelegateOperation.DEEP_RESEARCH:
        // STUB: Will build research arguments in Phase 2
        if (parameters.query) args.push(String(parameters.query));
        if (parameters.maxSources) args.push(`--max-sources=${String(parameters.maxSources)}`);
        if (parameters.depth) args.push(`--depth=${String(parameters.depth)}`);
        break;

      case DelegateOperation.SYNTHESIZE_SOURCES:
        // STUB: Will build synthesis arguments in Phase 2
        if (parameters.sources) args.push('--sources');
        if (parameters.format) args.push(`--format=${String(parameters.format)}`);
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
  updateConfig(config: Partial<ResearcherDelegationConfig>): void {
    this.config = { ...this.config, ...config };
  }

  /**
   * Gets the current configuration
   */
  getConfig(): Required<ResearcherDelegationConfig> {
    return { ...this.config };
  }
}

// =============================================================================
// Factory Functions
// =============================================================================

/**
 * Creates a new ResearcherDelegation instance with the specified gRPC client
 *
 * @param grpcClient - The gRPC client connected to the Orchestrator
 * @param config - Optional configuration for delegation behavior
 * @returns A new ResearcherDelegation instance
 */
export function createResearcherDelegation(
  grpcClient: Client,
  config?: ResearcherDelegationConfig
): ResearcherDelegation {
  return new ResearcherDelegation(grpcClient, config);
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Validates Researcher agent operation parameters
 *
 * @param operation - The operation to validate
 * @param parameters - The parameters to validate
 * @throws Error if parameters are invalid
 * NOTE: Stub implementation - will be implemented in Phase 2
 */
export function validateResearcherOperation(
  operation: DelegateOperation,
  parameters: Record<string, unknown>
): void {
  switch (operation) {
    case DelegateOperation.DEEP_RESEARCH:
      // STUB: Will validate research parameters in Phase 2
      if (!parameters.query || typeof parameters.query !== 'string') {
        throw new Error('deep_research requires a valid "query" parameter');
      }
      break;

    case DelegateOperation.SYNTHESIZE_SOURCES:
      // STUB: Will validate synthesis parameters in Phase 2
      if (!parameters.sources || !Array.isArray(parameters.sources)) {
        throw new Error('synthesize_sources requires a valid "sources" array parameter');
      }
      break;
  }
}

/**
 * Checks if an operation is supported by the Researcher agent
 *
 * @param operation - The operation to check
 * @returns True if the operation is supported
 */
export function isResearcherOperation(operation: DelegateOperation): boolean {
  const researcherOperations: DelegateOperation[] = [
    DelegateOperation.DEEP_RESEARCH,
    DelegateOperation.SYNTHESIZE_SOURCES,
  ];

  return researcherOperations.includes(operation);
}
