// bootstrap.ts - Agent bootstrap process for registration with Orchestrator
//
// This file handles the bootstrap process for the Assistant agent, including
// initialization, registration with the Orchestrator, and readiness checks.
//
// Copyright 2026 baaaht project

import { EventEmitter } from 'events';
import { AgentType, AgentState } from './proto/agent.js';
import type { OrchestratorClientConfig } from './orchestrator/grpc-client.js';
import type {
  AgentConfig,
  AgentDependencies,
  AgentStatus,
} from './agent/types.js';
import type { SessionManagerConfig } from './session/types.js';
import { Agent, createAgent } from './agent.js';
import { SessionManager } from './session/manager.js';
import { AgentRegistry, createAgentRegistry, type AgentRegistrationInfo } from './orchestrator/registration.js';

// =============================================================================
// Constants
// =============================================================================

const DEFAULT_VERSION = '0.1.0';
const DEFAULT_SHUTDOWN_TIMEOUT = 30000; // 30 seconds
const DEFAULT_READY_CHECK_TIMEOUT = 10000; // 10 seconds
const DEFAULT_READY_CHECK_INTERVAL = 100; // 100ms

// =============================================================================
// Bootstrap Configuration
// =============================================================================

/**
 * BootstrapConfig contains configuration for the bootstrap process
 */
export interface BootstrapConfig {
  /**
   * Agent configuration
   */
  agentConfig?: AgentConfig;

  /**
   * Orchestrator client configuration
   */
  orchestratorConfig?: OrchestratorClientConfig;

  /**
   * Session manager configuration
   */
  sessionManagerConfig?: SessionManagerConfig;

  /**
   * Agent registration information
   */
  registrationInfo?: AgentRegistrationInfo;

  /**
   * Agent version
   */
  version?: string;

  /**
   * Shutdown timeout in milliseconds
   */
  shutdownTimeout?: number;

  /**
   * Enable health check after bootstrap
   */
  enableHealthCheck?: boolean;

  /**
   * Whether to start heartbeat automatically
   */
  startHeartbeat?: boolean;

  /**
   * Whether to start the agent message processing loop automatically
   */
  startAgent?: boolean;
}

// =============================================================================
// Bootstrap Result
// =============================================================================

/**
 * BootstrapResult contains the result of a bootstrap operation
 */
export interface BootstrapResult {
  /**
   * The bootstrapped agent instance
   */
  agent: Agent | null;

  /**
   * The session manager instance
   */
  sessionManager: SessionManager;

  /**
   * The agent registry instance
   */
  registry: AgentRegistry;

  /**
   * The agent ID assigned by the orchestrator
   */
  agentId: string;

  /**
   * The version of the agent
   */
  version: string;

  /**
   * Timestamp when bootstrap started
   */
  startedAt: Date;

  /**
   * Timestamp when bootstrap completed
   */
  completedAt: Date;

  /**
   * Error if bootstrap failed
   */
  error: Error | null;
}

// =============================================================================
// Bootstrap Error
// =============================================================================

/**
 * BootstrapError represents an error during the bootstrap process
 */
export class BootstrapError extends Error {
  /**
   * The error code
   */
  readonly code: string;

  /**
   * Whether the error is retryable
   */
  readonly retryable: boolean;

  /**
   * The bootstrap step that failed
   */
  readonly step: string;

  constructor(
    message: string,
    code: string,
    step: string,
    retryable: boolean = false,
    options?: ErrorOptions
  ) {
    super(message, options);
    this.name = 'BootstrapError';
    this.code = code;
    this.step = step;
    this.retryable = retryable;
  }
}

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Create default bootstrap configuration
 */
function createDefaultBootstrapConfig(): Required<
  Omit<BootstrapConfig, 'agentConfig' | 'orchestratorConfig' | 'sessionManagerConfig' | 'registrationInfo'>
> & {
  agentConfig: AgentConfig;
  orchestratorConfig: OrchestratorClientConfig;
  sessionManagerConfig: SessionManagerConfig;
  registrationInfo: AgentRegistrationInfo;
} {
  return {
    agentConfig: {
      name: 'assistant',
      description: 'Primary conversational agent with tool delegation',
    },
    orchestratorConfig: {
      address: process.env.ORCHESTRATOR_URL ?? '/tmp/baaaht-grpc.sock',
      useUnixSocket: process.env.ORCHESTRATOR_URL === undefined,
    },
    sessionManagerConfig: {
      maxSessions: 100,
      idleTimeout: 3600000, // 1 hour
      timeout: 3600000,
    },
    registrationInfo: {
    registrationInfo: {
      name: 'assistant',
      type: AgentType.AGENT_TYPE_WORKER,
    },
    version: process.env.npm_package_version ?? DEFAULT_VERSION,
    shutdownTimeout: DEFAULT_SHUTDOWN_TIMEOUT,
    enableHealthCheck: true,
    startHeartbeat: true,
    startAgent: true,
  };
}

/**
 * Merge user config with defaults
 */
function mergeBootstrapConfig(
  userConfig: BootstrapConfig = {}
): Required<BootstrapConfig> & {
  agentConfig: AgentConfig;
  orchestratorConfig: OrchestratorClientConfig;
  sessionManagerConfig: SessionManagerConfig;
  registrationInfo: AgentRegistrationInfo;
} {
  const defaults = createDefaultBootstrapConfig();

  return {
    agentConfig: { ...defaults.agentConfig, ...userConfig.agentConfig },
    orchestratorConfig: { ...defaults.orchestratorConfig, ...userConfig.orchestratorConfig },
    sessionManagerConfig: { ...defaults.sessionManagerConfig, ...userConfig.sessionManagerConfig },
    registrationInfo: { ...defaults.registrationInfo, ...userConfig.registrationInfo },
    version: userConfig.version ?? defaults.version,
    shutdownTimeout: userConfig.shutdownTimeout ?? defaults.shutdownTimeout,
    enableHealthCheck: userConfig.enableHealthCheck ?? defaults.enableHealthCheck,
    startHeartbeat: userConfig.startHeartbeat ?? defaults.startHeartbeat,
    startAgent: userConfig.startAgent ?? defaults.startAgent,
  };
}

// =============================================================================
// Bootstrap Functions
// =============================================================================

/**
 * Bootstrap creates and initializes a new Assistant agent with the specified configuration
 *
 * The bootstrap process performs the following steps:
 * 1. Creates the session manager
 * 2. Creates the agent registry
 * 3. Creates and initializes the agent
 * 4. Registers with the orchestrator
 * 5. Starts the heartbeat loop (if enabled)
 * 6. Performs health checks (if enabled)
 * 7. Starts the agent message processing loop (if enabled)
 *
 * @param config - Bootstrap configuration
 * @param signal - Optional AbortSignal for cancellation
 * @returns The bootstrap result
 */
export async function bootstrap(
  config?: BootstrapConfig,
  signal?: AbortSignal
): Promise<BootstrapResult> {
  const startedAt = new Date();

  const result: BootstrapResult = {
    agent: null,
    sessionManager: null as unknown as SessionManager,
    registry: null as unknown as AgentRegistry,
    agentId: '',
    version: '',
    startedAt,
    completedAt: new Date(),
    error: null,
  };

  try {
    // Merge configuration
    const mergedConfig = mergeBootstrapConfig(config);
    result.version = mergedConfig.version;

    // Check for abort signal
    if (signal?.aborted) {
      throw new BootstrapError(
        'Bootstrap aborted by signal',
        'ABORTED',
        'bootstrap_start',
        false
      );
    }

    // Step 1: Create session manager
    result.sessionManager = new SessionManager(mergedConfig.sessionManagerConfig);

    // Step 2: Create agent registry
    result.registry = createAgentRegistry({
      interval: 30000, // 30 seconds
      maxMissed: 3,
    });

    // Step 3: Register with orchestrator (creates gRPC client)
    const registrationResult = await result.registry.register(
      mergedConfig.registrationInfo,
      mergedConfig.orchestratorConfig
    );

    result.agentId = registrationResult.agentId;

    if (signal?.aborted) {
      throw new BootstrapError(
        'Bootstrap aborted by signal during registration',
        'ABORTED',
        'agent_register',
        false
      );
    }

    // Step 4: Get gRPC client from registry
    const orchestratorClient = result.registry.getClient();
    if (!orchestratorClient) {
      throw new BootstrapError(
        'Failed to get orchestrator client after registration',
        'CLIENT_UNAVAILABLE',
        'grpc_client',
        false
      );
    }
    const grpcClient = orchestratorClient.getRawClient();

    // Step 5: Create agent dependencies
    const eventEmitter = new EventEmitter();
    const dependencies: AgentDependencies = {
      grpcClient,
      sessionManager: result.sessionManager,
      eventEmitter,
    };

    // Step 6: Create and initialize agent
    result.agent = createAgent(mergedConfig.agentConfig, dependencies);
    await result.agent.initialize();

    if (signal?.aborted) {
      throw new BootstrapError(
        'Bootstrap aborted by signal during initialization',
        'ABORTED',
        'agent_init',
        false
      );
    }

    // Step 7: Complete agent registration
    await result.agent.register(result.agentId);

    if (signal?.aborted) {
      throw new BootstrapError(
        'Bootstrap aborted by signal during agent registration',
        'ABORTED',
        'agent_register_complete',
        false
      );
    }

    // Step 8: Start heartbeat loop (if enabled)
    if (mergedConfig.startHeartbeat) {
      result.registry.startHeartbeat();
    }

    // Step 8: Perform health check (if enabled)
    if (mergedConfig.enableHealthCheck) {
      const isHealthy = await waitForReady(result.agent, DEFAULT_READY_CHECK_TIMEOUT);
      if (!isHealthy) {
        throw new BootstrapError(
          'Agent health check failed',
          'HEALTH_CHECK_FAILED',
          'health_check',
          true
        );
      }
    }

    // Step 9: Start agent message processing (if enabled)
    if (mergedConfig.startAgent) {
      result.agent.start();
    }

    result.completedAt = new Date();

    return result;
  } catch (err) {
    const error = err instanceof Error ? err : new Error(String(err));

    // Clean up on failure
    if (result.agent) {
      try {
        await result.agent.shutdown();
      } catch {
        // Ignore shutdown errors during cleanup
      }
    }

    if (result.registry && result.registry.isRegistered()) {
      try {
        await result.registry.unregister('Bootstrap failed');
      } catch {
        // Ignore unregister errors during cleanup
      }
    }

    if (result.sessionManager) {
      try {
        await result.sessionManager.close();
      } catch {
        // Ignore session manager close errors during cleanup
      }
    }

    result.error = error instanceof BootstrapError ? error : new BootstrapError(
      error.message,
      'BOOTSTRAP_FAILED',
      'bootstrap',
      false,
      { cause: error }
    );

    result.completedAt = new Date();

    return result;
  }
}

/**
 * BootstrapWithDefaults creates and initializes a new agent with default configuration
 *
 * @param signal - Optional AbortSignal for cancellation
 * @returns The bootstrap result
 */
export async function bootstrapWithDefaults(signal?: AbortSignal): Promise<BootstrapResult> {
  return bootstrap(undefined, signal);
}

/**
 * BootstrapAgent is a simplified bootstrap function that returns only the agent
 *
 * @param config - Optional bootstrap configuration
 * @param signal - Optional AbortSignal for cancellation
 * @returns The bootstrapped agent or null if bootstrap failed
 */
export async function bootstrapAgent(
  config?: BootstrapConfig,
  signal?: AbortSignal
): Promise<Agent | null> {
  const result = await bootstrap(config, signal);
  return result.agent;
}

// =============================================================================
// Ready Check Functions
// =============================================================================

/**
 * IsReady checks if the agent is ready to accept requests
 *
 * @param agent - The agent to check
 * @returns True if the agent is ready
 */
export function isReady(agent: Agent | null): boolean {
  if (agent === null) {
    return false;
  }

  const status = agent.getStatus();

  // Agent must be in IDLE state (registered and ready)
  return status.state === AgentState.AGENT_STATE_IDLE;
}

/**
 * WaitForReady waits for the agent to be ready with a timeout
 *
 * @param agent - The agent to wait for
 * @param timeout - Maximum time to wait in milliseconds
 * @param checkInterval - Time between checks in milliseconds
 * @param signal - Optional AbortSignal for cancellation
 * @returns Promise that resolves when ready or rejects on timeout
 */
export async function waitForReady(
  agent: Agent | null,
  timeout: number = DEFAULT_READY_CHECK_TIMEOUT,
  checkInterval: number = DEFAULT_READY_CHECK_INTERVAL,
  signal?: AbortSignal
): Promise<boolean> {
  if (agent === null) {
    return false;
  }

  const startTime = Date.now();
  const deadline = startTime + timeout;

  while (Date.now() < deadline) {
    // Check for abort signal
    if (signal?.aborted) {
      return false;
    }

    // Check if ready
    if (isReady(agent)) {
      return true;
    }

    // Wait before next check
    await new Promise((resolve) => setTimeout(resolve, checkInterval));
  }

  // Timeout reached
  return false;
}

// =============================================================================
// Version Information
// =============================================================================

/**
 * GetVersion returns the version of the agent
 */
export function getVersion(): string {
  return process.env.npm_package_version ?? DEFAULT_VERSION;
}

/**
 * GetVersionInfo returns detailed version information
 */
export function getVersionInfo(): Record<string, string> {
  return {
    version: getVersion(),
    buildTime: process.env.BUILD_TIME ?? 'unknown',
    gitCommit: process.env.GIT_COMMIT ?? 'unknown',
    nodeVersion: process.version,
  };
}

// =============================================================================
// BootstrapResult Helper Methods
// =============================================================================

/**
 * Check if a bootstrap result indicates success
 */
export function isBootstrapSuccessful(result: BootstrapResult): boolean {
  return result.error === null && result.agent !== null;
}

/**
 * Get the duration of a bootstrap operation
 */
export function getBootstrapDuration(result: BootstrapResult): number {
  return result.completedAt.getTime() - result.startedAt.getTime();
}

/**
 * Format a bootstrap result as a string
 */
export function formatBootstrapResult(result: BootstrapResult): string {
  if (result.error !== null) {
    return `BootstrapResult{version: ${result.version}, error: ${result.error.message}, step: ${(result.error as BootstrapError).step}}`;
  }

  const duration = getBootstrapDuration(result);
  return `BootstrapResult{version: ${result.version}, agentId: ${result.agentId}, duration: ${duration}ms}`;
}

// =============================================================================
// Global Bootstrap State (for standalone processes)
// =============================================================================

let globalBootstrapResult: BootstrapResult | null = null;
let globalBootstrapPromise: Promise<BootstrapResult> | null = null;

/**
 * InitGlobal initializes the global agent instance
 *
 * @param config - Optional bootstrap configuration
 * @param signal - Optional AbortSignal for cancellation
 * @returns Promise that resolves with the bootstrap result
 */
export async function initGlobal(
  config?: BootstrapConfig,
  signal?: AbortSignal
): Promise<BootstrapResult> {
  // Return existing promise if bootstrap is in progress
  if (globalBootstrapPromise !== null) {
    return globalBootstrapPromise;
  }

  // Set up the bootstrap promise
  globalBootstrapPromise = bootstrap(config, signal);

  try {
    const result = await globalBootstrapPromise;

    if (isBootstrapSuccessful(result)) {
      globalBootstrapResult = result;
    }

    return result;
  } finally {
    globalBootstrapPromise = null;
  }
}

/**
 * GetGlobal returns the global agent instance
 *
 * @returns The global agent or null if not initialized
 */
export function getGlobal(): Agent | null {
  return globalBootstrapResult?.agent ?? null;
}

/**
 * IsGlobalInitialized returns true if the global agent has been initialized
 */
export function isGlobalInitialized(): boolean {
  return globalBootstrapResult !== null && isBootstrapSuccessful(globalBootstrapResult);
}

/**
 * CloseGlobal closes and clears the global agent instance
 */
export async function closeGlobal(): Promise<void> {
  if (globalBootstrapResult?.agent !== null) {
    try {
      await globalBootstrapResult.agent.shutdown();
    } catch {
      // Ignore shutdown errors
    }
  }

  if (globalBootstrapResult?.registry && globalBootstrapResult.registry.isRegistered()) {
    try {
      await globalBootstrapResult.registry.unregister('Global shutdown');
    } catch {
      // Ignore unregister errors
    }
  }

  if (globalBootstrapResult?.sessionManager) {
    try {
      await globalBootstrapResult.sessionManager.close();
    } catch {
      // Ignore session manager close errors
    }
  }

  globalBootstrapResult = null;
}
