// lifecycle.ts - Agent lifecycle management for state transitions and graceful shutdown
//
// This file handles the lifecycle management for the Assistant agent, including
// state transitions, shutdown coordination, and lifecycle event emission.
//
// Copyright 2026 baaaht project

import { EventEmitter } from 'events';
import { Agent, AgentState, type AgentStatus } from './agent.js';
import type { SessionManager } from './session/manager.js';
import type { AgentRegistry } from './orchestrator/registration.js';

// =============================================================================
// Lifecycle State Enumeration
// =============================================================================

/**
 * LifecycleState represents the current state in the agent lifecycle
 */
export enum LifecycleState {
  /**
   * Lifecycle has not started
   */
  UNINITIALIZED = 'uninitialized',

  /**
   * Lifecycle is starting up
   */
  STARTING = 'starting',

  /**
   * Lifecycle is running normally
   */
  RUNNING = 'running',

  /**
   * Lifecycle is shutting down gracefully
   */
  SHUTTING_DOWN = 'shutting_down',

  /**
   * Lifecycle is terminated
   */
  TERMINATED = 'terminated',

  /**
   * Lifecycle encountered an error
   */
  ERROR = 'error',
}

// =============================================================================
// Lifecycle Event Types
// =============================================================================

/**
 * LifecycleEventType represents the types of lifecycle events
 */
export enum LifecycleEventType {
  /**
   * Lifecycle is starting
   */
  STARTING = 'lifecycle.starting',

  /**
   * Lifecycle started successfully
   */
  STARTED = 'lifecycle.started',

  /**
   * Lifecycle is shutting down
   */
  SHUTTING_DOWN = 'lifecycle.shutting_down',

  /**
   * Lifecycle shutdown complete
   */
  SHUTDOWN_COMPLETE = 'lifecycle.shutdown_complete',

  /**
   * Lifecycle error occurred
   */
  ERROR = 'lifecycle.error',

  /**
   * Agent state changed
   */
  AGENT_STATE_CHANGED = 'lifecycle.agent_state_changed',

  /**
   * Health check completed
   */
  HEALTH_CHECK_COMPLETED = 'lifecycle.health_check_completed',
}

// =============================================================================
// Lifecycle Event Interface
// =============================================================================

/**
 * LifecycleEvent represents a lifecycle event
 */
export interface LifecycleEvent {
  /**
   * The event type
   */
  type: LifecycleEventType;

  /**
   * The timestamp when the event occurred
   */
  timestamp: Date;

  /**
   * The previous lifecycle state
   */
  previousState?: LifecycleState;

  /**
   * The new lifecycle state
   */
  newState?: LifecycleState;

  /**
   * Event-specific data
   */
  data?: Record<string, unknown>;

  /**
   * Error if the event represents an error
   */
  error?: Error;
}

// =============================================================================
// Lifecycle Configuration
// =============================================================================

/**
 * LifecycleConfig contains configuration for lifecycle management
 */
export interface LifecycleConfig {
  /**
   * Shutdown timeout in milliseconds
   * Defaults to 30000 (30 seconds)
   */
  shutdownTimeout?: number;

  /**
   * Grace period for forced shutdown in milliseconds
   * Defaults to 5000 (5 seconds)
   */
  gracePeriod?: number;

  /**
   * Enable health checks during lifecycle
   * Defaults to true
   */
  enableHealthChecks?: boolean;

  /**
   * Health check interval in milliseconds
   * Defaults to 30000 (30 seconds)
   */
  healthCheckInterval?: number;

  /**
   * Whether to perform cleanup on shutdown
   * Defaults to true
   */
  enableCleanup?: boolean;

  /**
   * Maximum number of shutdown retries
   * Defaults to 3
   */
  maxShutdownRetries?: number;
}

// =============================================================================
// Lifecycle Status
// =============================================================================

/**
 * LifecycleStatus represents the current status of the lifecycle
 */
export interface LifecycleStatus {
  /**
   * Current lifecycle state
   */
  state: LifecycleState;

  /**
   * Whether the lifecycle is healthy
   */
  healthy: boolean;

  /**
   * Timestamp when the lifecycle started
   */
  startedAt: Date | null;

  /**
   * Timestamp of the last state transition
   */
  lastTransitionAt: Date | null;

  /**
   * Uptime in seconds
   */
  uptimeSeconds: number;

  /**
   * Number of shutdown attempts
   */
  shutdownAttempts: number;

  /**
   * Whether shutdown was forced
   */
  forced: boolean;
}

// =============================================================================
// Default Configuration
// =============================================================================

const DEFAULT_LIFECYCLE_CONFIG: Required<LifecycleConfig> = {
  shutdownTimeout: 30000, // 30 seconds
  gracePeriod: 5000, // 5 seconds
  enableHealthChecks: true,
  healthCheckInterval: 30000, // 30 seconds
  enableCleanup: true,
  maxShutdownRetries: 3,
};

// =============================================================================
// Lifecycle Manager Class
// =============================================================================

/**
 * LifecycleManager manages the lifecycle of the Assistant agent
 *
 * This class handles:
 * - State transitions between lifecycle phases
 * - Graceful shutdown coordination
 * - Health check monitoring
 * - Cleanup on termination
 */
export class LifecycleManager extends EventEmitter {
  private config: Required<LifecycleConfig>;
  private state: LifecycleState;
  private agent: Agent | null;
  private sessionManager: SessionManager | null;
  private registry: AgentRegistry | null;

  private startedAt: Date | null;
  private lastTransitionAt: Date | null;
  private shutdownAttempts: number;
  private forcedShutdown: boolean;
  private healthCheckIntervalId: NodeJS.Timeout | null;
  private shutdownPromise: Promise<void> | null;
  private isShuttingDown: boolean;

  private abortController: AbortController | null;

  /**
   * Creates a new LifecycleManager
   *
   * @param config - Lifecycle configuration
   */
  constructor(config: LifecycleConfig = {}) {
    super();

    this.config = {
      ...DEFAULT_LIFECYCLE_CONFIG,
      ...config,
    };

    this.state = LifecycleState.UNINITIALIZED;
    this.agent = null;
    this.sessionManager = null;
    this.registry = null;

    this.startedAt = null;
    this.lastTransitionAt = null;
    this.shutdownAttempts = 0;
    this.forcedShutdown = false;
    this.healthCheckIntervalId = null;
    this.shutdownPromise = null;
    this.isShuttingDown = false;

    this.abortController = null;

    this.setMaxListeners(100);
  }

  // ==========================================================================
  // Public Methods - Lifecycle Control
  // ==========================================================================

  /**
   * Initialize the lifecycle manager with agent components
   *
   * @param agent - The agent instance
   * @param sessionManager - The session manager instance
   * @param registry - The agent registry instance
   */
  initialize(
    agent: Agent,
    sessionManager: SessionManager,
    registry: AgentRegistry
  ): void {
    if (this.state !== LifecycleState.UNINITIALIZED) {
      throw new Error(`Cannot initialize: lifecycle state is ${this.state}`);
    }

    this.agent = agent;
    this.sessionManager = sessionManager;
    this.registry = registry;

    // Set up agent state change listener
    this.agent.on('agent.state_changed', (event: { data: { state: AgentState } }) => {
      this.emit(LifecycleEventType.AGENT_STATE_CHANGED, {
        type: LifecycleEventType.AGENT_STATE_CHANGED,
        timestamp: new Date(),
        data: { agentState: event.data.state },
      });
    });

    this.transitionTo(LifecycleState.STARTING);
  }

  /**
   * Start the lifecycle
   */
  async start(): Promise<void> {
    if (this.state !== LifecycleState.STARTING) {
      throw new Error(`Cannot start: lifecycle state is ${this.state}`);
    }

    if (!this.agent) {
      throw new Error('Agent not initialized');
    }

    this.startedAt = new Date();

    // Start health check interval if enabled
    if (this.config.enableHealthChecks) {
      this.startHealthCheck();
    }

    // Emit started event
    this.emit(LifecycleEventType.STARTED, this.createEvent(LifecycleEventType.STARTED));

    // Transition to running
    this.transitionTo(LifecycleState.RUNNING);
  }

  /**
   * Shutdown the lifecycle gracefully
   *
   * @param reason - Optional reason for shutdown
   * @param force - Whether to force shutdown immediately
   * @returns Promise that resolves when shutdown is complete
   */
  async shutdown(reason?: string, force: boolean = false): Promise<void> {
    // Prevent multiple shutdown calls
    if (this.isShuttingDown) {
      return this.shutdownPromise ?? Promise.resolve();
    }

    this.isShuttingDown = true;

    // Create shutdown promise
    this.shutdownPromise = this.performShutdown(reason, force);

    try {
      await this.shutdownPromise;
    } finally {
      this.isShuttingDown = false;
      this.shutdownPromise = null;
    }
  }

  /**
   * Check if the lifecycle is shutting down
   */
  isShuttingDownInProgress(): boolean {
    return this.isShuttingDown;
  }

  // ==========================================================================
  // Public Methods - Status and Health
  // ==========================================================================

  /**
   * Get the current lifecycle status
   */
  getStatus(): LifecycleStatus {
    const uptimeSeconds = this.startedAt
      ? Math.floor((Date.now() - this.startedAt.getTime()) / 1000)
      : 0;

    return {
      state: this.state,
      healthy: this.isHealthy(),
      startedAt: this.startedAt,
      lastTransitionAt: this.lastTransitionAt,
      uptimeSeconds,
      shutdownAttempts: this.shutdownAttempts,
      forced: this.forcedShutdown,
    };
  }

  /**
   * Check if the lifecycle is healthy
   */
  isHealthy(): boolean {
    return (
      this.state === LifecycleState.RUNNING ||
      this.state === LifecycleState.STARTING
    );
  }

  /**
   * Check if the lifecycle is terminated
   */
  isTerminated(): boolean {
    return this.state === LifecycleState.TERMINATED;
  }

  /**
   * Get the agent
   */
  getAgent(): Agent | null {
    return this.agent;
  }

  /**
   * Get the session manager
   */
  getSessionManager(): SessionManager | null {
    return this.sessionManager;
  }

  /**
   * Get the registry
   */
  getRegistry(): AgentRegistry | null {
    return this.registry;
  }

  /**
   * Get the abort signal for cancellation
   */
  getAbortSignal(): AbortSignal | undefined {
    return this.abortController?.signal;
  }

  // ==========================================================================
  // Private Methods - State Management
  // ==========================================================================

  /**
   * Transition to a new lifecycle state
   *
   * @private
   */
  private transitionTo(newState: LifecycleState): void {
    const previousState = this.state;
    this.state = newState;
    this.lastTransitionAt = new Date();

    // Emit state change event
    this.emit(LifecycleEventType.AGENT_STATE_CHANGED, {
      type: LifecycleEventType.AGENT_STATE_CHANGED,
      timestamp: new Date(),
      previousState,
      newState,
    });
  }

  /**
   * Create a lifecycle event
   *
   * @private
   */
  private createEvent(
    type: LifecycleEventType,
    data?: Record<string, unknown>,
    error?: Error
  ): LifecycleEvent {
    return {
      type,
      timestamp: new Date(),
      newState: this.state,
      data,
      error,
    };
  }

  // ==========================================================================
  // Private Methods - Shutdown
  // ==========================================================================

  /**
   * Perform the actual shutdown
   *
   * @private
   */
  private async performShutdown(
    reason?: string,
    force: boolean = false
  ): Promise<void> {
    const previousState = this.state;
    this.transitionTo(LifecycleState.SHUTTING_DOWN);

    this.emit(
      LifecycleEventType.SHUTTING_DOWN,
      this.createEvent(LifecycleEventType.SHUTTING_DOWN, { reason })
    );

    this.shutdownAttempts++;
    this.forcedShutdown = force;

    // Cancel all operations via abort controller
    if (this.abortController) {
      this.abortController.abort();
      this.abortController = null;
    }

    try {
      if (force) {
        // Force shutdown - no timeout
        await this.forceShutdown(reason);
      } else {
        // Graceful shutdown with timeout
        await this.gracefulShutdown(reason);
      }

      // Transition to terminated
      this.transitionTo(LifecycleState.TERMINATED);

      this.emit(
        LifecycleEventType.SHUTDOWN_COMPLETE,
        this.createEvent(LifecycleEventType.SHUTDOWN_COMPLETE, {
          reason,
          forced: force,
          attempts: this.shutdownAttempts,
        })
      );
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));

      // Transition to error state
      this.transitionTo(LifecycleState.ERROR);

      this.emit(
        LifecycleEventType.ERROR,
        this.createEvent(LifecycleEventType.ERROR, { reason }, error)
      );

      throw error;
    } finally {
      // Stop health check interval
      this.stopHealthCheck();
    }
  }

  /**
   * Perform graceful shutdown
   *
   * @private
   */
  private async gracefulShutdown(reason?: string): Promise<void> {
    const startTime = Date.now();
    const deadline = startTime + this.config.shutdownTimeout;

    // Step 1: Wait for active messages to complete or timeout
    if (this.agent) {
      const agentStatus = this.agent.getStatus();
      const timeout = Math.min(5000, deadline - Date.now());

      while (agentStatus.processingMessages > 0 && Date.now() < deadline) {
        await this.sleep(100);
      }
    }

    // Step 2: Shutdown agent
    if (this.agent && Date.now() < deadline) {
      try {
        await Promise.race([
          this.agent.shutdown(),
          this.createTimeout(deadline - Date.now()),
        ]);
      } catch (err) {
        // Continue with shutdown even if agent shutdown fails
        const error = err instanceof Error ? err : new Error(String(err));
        this.emit('warning', {
          message: 'Agent shutdown failed, continuing',
          error: error.message,
        });
      }
    }

    // Step 3: Unregister from orchestrator
    if (this.registry && this.registry.isRegistered() && Date.now() < deadline) {
      try {
        await Promise.race([
          this.registry.unregister(reason ?? 'Lifecycle shutdown'),
          this.createTimeout(deadline - Date.now()),
        ]);
      } catch (err) {
        // Continue with shutdown even if unregister fails
        const error = err instanceof Error ? err : new Error(String(err));
        this.emit('warning', {
          message: 'Unregistration failed, continuing',
          error: error.message,
        });
      }
    }

    // Step 4: Close session manager
    if (this.sessionManager && Date.now() < deadline) {
      try {
        await Promise.race([
          this.sessionManager.close(),
          this.createTimeout(deadline - Date.now()),
        ]);
      } catch (err) {
        // Continue with shutdown even if session manager close fails
        const error = err instanceof Error ? err : new Error(String(err));
        this.emit('warning', {
          message: 'Session manager close failed, continuing',
          error: error.message,
        });
      }
    }

    // Step 5: Perform cleanup if enabled
    if (this.config.enableCleanup) {
      await this.performCleanup(deadline - Date.now());
    }
  }

  /**
   * Perform force shutdown
   *
   * @private
   */
  private async forceShutdown(reason?: string): Promise<void> {
    // Force shutdown - minimal cleanup, no waiting

    // Close agent immediately
    if (this.agent) {
      try {
        await this.agent.shutdown();
      } catch {
        // Ignore errors during force shutdown
      }
    }

    // Unregister immediately
    if (this.registry && this.registry.isRegistered()) {
      try {
        await this.registry.unregister(reason ?? 'Force shutdown');
      } catch {
        // Ignore errors during force shutdown
      }
    }

    // Close session manager immediately
    if (this.sessionManager) {
      try {
        this.sessionManager.close();
      } catch {
        // Ignore errors during force shutdown
      }
    }
  }

  /**
   * Perform cleanup on shutdown
   *
   * @private
   */
  private async performCleanup(timeoutMs: number): Promise<void> {
    const startTime = Date.now();

    // Clean up any remaining resources
    // This is a placeholder for future cleanup tasks

    if (Date.now() - startTime < timeoutMs) {
      // Remove all event listeners to prevent memory leaks
      this.removeAllListeners();
    }
  }

  // ==========================================================================
  // Private Methods - Health Check
  // ==========================================================================

  /**
   * Start the health check interval
   *
   * @private
   */
  private startHealthCheck(): void {
    if (this.healthCheckIntervalId !== null) {
      return;
    }

    this.healthCheckIntervalId = setInterval(
      () => this.performHealthCheck(),
      this.config.healthCheckInterval
    );
  }

  /**
   * Stop the health check interval
   *
   * @private
   */
  private stopHealthCheck(): void {
    if (this.healthCheckIntervalId !== null) {
      clearInterval(this.healthCheckIntervalId);
      this.healthCheckIntervalId = null;
    }
  }

  /**
   * Perform a health check
   *
   * @private
   */
  private async performHealthCheck(): Promise<void> {
    try {
      let healthy = true;
      let details: Record<string, boolean> = {};

      // Check agent health
      if (this.agent) {
        const agentStatus = this.agent.getStatus();
        details.agent = agentStatus.state === 'AGENT_STATE_IDLE';
        healthy = healthy && details.agent;
      } else {
        details.agent = false;
        healthy = false;
      }

      // Check registry health
      if (this.registry) {
        details.registry = this.registry.isRegistered();
        healthy = healthy && details.registry;
      } else {
        details.registry = false;
        healthy = false;
      }

      // Check session manager health
      if (this.sessionManager) {
        const stats = this.sessionManager.getStats();
        details.sessionManager = stats.active >= 0;
        healthy = healthy && details.sessionManager;
      } else {
        details.sessionManager = false;
        healthy = false;
      }

      this.emit(
        LifecycleEventType.HEALTH_CHECK_COMPLETED,
        this.createEvent(LifecycleEventType.HEALTH_CHECK_COMPLETED, {
          healthy,
          details,
        })
      );

      // Transition to error state if unhealthy
      if (!healthy && this.state === LifecycleState.RUNNING) {
        this.transitionTo(LifecycleState.ERROR);
      }
    } catch (err) {
      this.emit(
        LifecycleEventType.ERROR,
        this.createEvent(
          LifecycleEventType.ERROR,
          { during: 'health_check' },
          err instanceof Error ? err : new Error(String(err))
        )
      );
    }
  }

  // ==========================================================================
  // Private Helper Methods
  // ==========================================================================

  /**
   * Sleep for a specified duration
   *
   * @private
   */
  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  /**
   * Create a timeout promise
   *
   * @private
   */
  private createTimeout(ms: number): Promise<never> {
    return new Promise((_, reject) => {
      setTimeout(() => {
        reject(new Error(`Timeout after ${ms}ms`));
      }, ms);
    });
  }
}

// =============================================================================
// Factory Functions
// =============================================================================

/**
 * Create a new LifecycleManager instance
 *
 * @param config - Lifecycle configuration
 * @returns A new LifecycleManager instance
 */
export function createLifecycleManager(config?: LifecycleConfig): LifecycleManager {
  return new LifecycleManager(config);
}

// =============================================================================
// Re-exports
// =============================================================================
