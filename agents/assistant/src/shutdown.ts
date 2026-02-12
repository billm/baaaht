// shutdown.ts - Graceful shutdown handler for the Assistant agent
//
// This file handles graceful shutdown of the Assistant agent, including
// signal handling, shutdown hooks, and timeout management.
//
// Copyright 2026 baaaht project

// =============================================================================
// Shutdown State Enumeration
// =============================================================================

/**
 * ShutdownState represents the current state of the shutdown process
 */
export enum ShutdownState {
  /**
   * The agent is running normally
   */
  RUNNING = 'running',

  /**
   * Shutdown has been initiated
   */
  INITIATED = 'initiated',

  /**
   * Subsystems are being stopped
   */
  STOPPING = 'stopping',

  /**
   * Shutdown is complete
   */
  COMPLETE = 'complete',
}

// =============================================================================
// Shutdown Hook Type
// =============================================================================

/**
 * ShutdownHook is a function that can be called during shutdown
 *
 * @param signal - AbortSignal for cancellation
 * @returns Promise that resolves when the hook is complete
 */
export type ShutdownHook = (signal: AbortSignal) => Promise<void>;

// =============================================================================
// Logger Interface
// =============================================================================

/**
 * Logger interface for shutdown logging
 */
export interface ShutdownLogger {
  info(message: string, meta?: Record<string, unknown>): void;
  warn(message: string, meta?: Record<string, unknown>): void;
  error(message: string, meta?: Record<string, unknown>): void;
  debug(message: string, meta?: Record<string, unknown>): void;
}

/**
 * Default console-based logger implementation
 */
class DefaultLogger implements ShutdownLogger {
  private readonly prefix: string;

  constructor(prefix: string = 'ShutdownManager') {
    this.prefix = prefix;
  }

  info(message: string, meta?: Record<string, unknown>): void {
    console.log(`[${this.prefix}] [INFO] ${message}`, meta ? JSON.stringify(meta) : '');
  }

  warn(message: string, meta?: Record<string, unknown>): void {
    console.warn(`[${this.prefix}] [WARN] ${message}`, meta ? JSON.stringify(meta) : '');
  }

  error(message: string, meta?: Record<string, unknown>): void {
    console.error(`[${this.prefix}] [ERROR] ${message}`, meta ? JSON.stringify(meta) : '');
  }

  debug(message: string, meta?: Record<string, unknown>): void {
    if (process.env.DEBUG) {
      console.debug(`[${this.prefix}] [DEBUG] ${message}`, meta ? JSON.stringify(meta) : '');
    }
  }
}

// =============================================================================
// Shutdown Configuration
// =============================================================================

/**
 * ShutdownConfig contains configuration for the shutdown manager
 */
export interface ShutdownConfig {
  /**
   * Shutdown timeout in milliseconds
   * Defaults to 30000 (30 seconds)
   */
  shutdownTimeout?: number;

  /**
   * Hook timeout in milliseconds
   * Defaults to 5000 (5 seconds)
   */
  hookTimeout?: number;

  /**
   * Logger instance
   */
  logger?: ShutdownLogger;
}

// =============================================================================
// Default Configuration
// =============================================================================

const DEFAULT_SHUTDOWN_CONFIG: Required<ShutdownConfig> = {
  shutdownTimeout: 30000, // 30 seconds
  hookTimeout: 5000, // 5 seconds
  logger: new DefaultLogger(),
};

// =============================================================================
// Shutdown Manager Class
// =============================================================================

/**
 * ShutdownManager manages the graceful shutdown process
 *
 * This class handles:
 * - Signal handling (SIGINT, SIGTERM)
 * - Shutdown hook execution (pre-shutdown and post-shutdown)
 * - Timeout management
 * - State tracking
 * - Completion notification
 */
export class ShutdownManager {
  private config: Required<ShutdownConfig>;
  private state: ShutdownState;
  private hooks: ShutdownHook[];
  private logger: ShutdownLogger;

  private started: boolean;
  private completionPromise: Promise<void> | null;
  private completionResolve: (() => void) | null;
  private shutdownReason: string;
  private shutdownStartedAt: Date | null;

  private abortController: AbortController;
  private signalHandlers: {
    sigint: (() => void) | null;
    sigterm: (() => void) | null;
  };

  // Mutex-like lock for preventing concurrent shutdown
  private shutdownLock: boolean;

  /**
   * Creates a new ShutdownManager
   *
   * @param config - Shutdown configuration
   */
  constructor(config: ShutdownConfig = {}) {
    this.config = {
      ...DEFAULT_SHUTDOWN_CONFIG,
      ...config,
    };

    // Use provided logger or default
    this.logger = this.config.logger ?? new DefaultLogger();

    this.state = ShutdownState.RUNNING;
    this.hooks = [];
    this.started = false;
    this.completionPromise = null;
    this.completionResolve = null;
    this.shutdownReason = '';
    this.shutdownStartedAt = null;

    this.abortController = new AbortController();
    this.signalHandlers = {
      sigint: null,
      sigterm: null,
    };

    this.shutdownLock = false;

    this.logger.debug('ShutdownManager created', {
      timeout: this.config.shutdownTimeout,
      hookTimeout: this.config.hookTimeout,
    });
  }

  // ==========================================================================
  // Public Methods - Lifecycle
  // ==========================================================================

  /**
   * Start begins listening for shutdown signals
   */
  start(): void {
    if (this.started) {
      this.logger.debug('Shutdown manager already started');
      return;
    }

    // Register signal handlers for common shutdown signals
    this.signalHandlers.sigint = () => this.handleSignal('SIGINT');
    this.signalHandlers.sigterm = () => this.handleSignal('SIGTERM');

    process.on('SIGINT', this.signalHandlers.sigint);
    process.on('SIGTERM', this.signalHandlers.sigterm);

    this.started = true;

    this.logger.info('Shutdown manager started', {
      timeout: this.config.shutdownTimeout,
      signals: ['SIGINT', 'SIGTERM'],
    });
  }

  /**
   * Stop stops the shutdown manager (cancels signal handling)
   */
  stop(): void {
    if (!this.started) {
      return;
    }

    if (this.signalHandlers.sigint) {
      process.off('SIGINT', this.signalHandlers.sigint);
    }
    if (this.signalHandlers.sigterm) {
      process.off('SIGTERM', this.signalHandlers.sigterm);
    }

    this.abortController.abort();
    this.started = false;

    this.logger.debug('Shutdown manager stopped');
  }

  // ==========================================================================
  // Public Methods - Shutdown
  // ==========================================================================

  /**
   * Shutdown initiates a graceful shutdown with the specified reason
   *
   * @param reason - The reason for shutdown
   * @returns Promise that resolves when shutdown is complete
   */
  async shutdown(reason: string): Promise<void> {
    // Acquire shutdown lock to prevent concurrent shutdowns
    if (this.shutdownLock) {
      // If shutdown is already in progress, wait for it
      if (this.completionPromise) {
        return this.completionPromise;
      }
      return;
    }

    this.shutdownLock = true;

    try {
      // Check if already shutting down
      if (this.state !== ShutdownState.RUNNING) {
        throw new Error(`Shutdown already initiated (state: ${this.state})`);
      }

      this.state = ShutdownState.INITIATED;
      this.shutdownReason = reason;
      this.shutdownStartedAt = new Date();

      this.logger.info('Shutdown initiated', { reason });

      // Create abort signal with timeout
      const abortController = new AbortController();
      const timeoutId = setTimeout(() => {
        abortController.abort();
        this.logger.warn('Shutdown timeout reached', {
          timeout: this.config.shutdownTimeout,
        });
      }, this.config.shutdownTimeout);

      const signal = abortController.signal;

      try {
        // Execute pre-shutdown hooks
        await this.executeHooks(signal, 'pre-shutdown');

        // Transition to stopping state
        this.setState(ShutdownState.STOPPING);

        // Execute post-shutdown hooks
        await this.executeHooks(signal, 'post-shutdown');

        // Mark shutdown as complete
        this.setState(ShutdownState.COMPLETE);

        // Resolve completion promise
        if (this.completionResolve) {
          this.completionResolve();
          this.completionResolve = null;
        }
        this.completionPromise = null;

        const duration = this.shutdownStartedAt
          ? Date.now() - this.shutdownStartedAt.getTime()
          : 0;

        this.logger.info('Shutdown complete', {
          reason,
          durationMs: duration,
        });
      } finally {
        clearTimeout(timeoutId);
      }
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));
      this.logger.error('Shutdown failed', { error: error.message });
      throw error;
    } finally {
      // Release lock
      this.shutdownLock = false;
    }
  }

  /**
   * ShutdownAndWait initiates shutdown and waits for completion
   *
   * @param reason - The reason for shutdown
   * @param timeout - Optional timeout in milliseconds
   * @returns Promise that resolves when shutdown is complete or rejects on timeout
   */
  async shutdownAndWait(reason: string, timeout?: number): Promise<void> {
    // Launch shutdown in background
    const shutdownPromise = this.shutdown(reason);

    // Wait for either completion or timeout
    if (timeout) {
      await Promise.race([
        shutdownPromise,
        new Promise<void>((_, reject) =>
          setTimeout(() => reject(new Error(`Shutdown wait timeout after ${timeout}ms`)), timeout)
        ),
      ]);
    } else {
      await shutdownPromise;
    }
  }

  // ==========================================================================
  // Public Methods - Hooks
  // ==========================================================================

  /**
   * AddHook adds a shutdown hook that will be called during shutdown
   *
   * @param hook - The shutdown hook to add
   */
  addHook(hook: ShutdownHook): void {
    this.hooks.push(hook);
    this.logger.debug('Shutdown hook registered', {
      totalHooks: this.hooks.length,
    });
  }

  /**
   * RemoveHook removes a shutdown hook
   *
   * @param hook - The shutdown hook to remove
   * @returns True if the hook was removed, false otherwise
   */
  removeHook(hook: ShutdownHook): boolean {
    const index = this.hooks.indexOf(hook);
    if (index !== -1) {
      this.hooks.splice(index, 1);
      this.logger.debug('Shutdown hook removed', {
        totalHooks: this.hooks.length,
      });
      return true;
    }
    return false;
  }

  // ==========================================================================
  // Public Methods - State Query
  // ==========================================================================

  /**
   * State returns the current shutdown state
   */
  getState(): ShutdownState {
    return this.state;
  }

  /**
   * IsShuttingDown returns true if shutdown has been initiated
   */
  isShuttingDown(): boolean {
    return (
      this.state === ShutdownState.INITIATED ||
      this.state === ShutdownState.STOPPING ||
      this.state === ShutdownState.COMPLETE
    );
  }

  /**
   * IsComplete returns true if shutdown is complete
   */
  isComplete(): boolean {
    return this.state === ShutdownState.COMPLETE;
  }

  /**
   * ShutdownReason returns the reason for shutdown
   */
  getShutdownReason(): string {
    return this.shutdownReason;
  }

  /**
   * GetSignal returns the abort signal for cancellation
   */
  getSignal(): AbortSignal {
    return this.abortController.signal;
  }

  /**
   * WaitCompletion waits for shutdown to complete
   *
   * @param timeout - Optional timeout in milliseconds
   * @returns Promise that resolves when shutdown is complete
   */
  async waitCompletion(timeout?: number): Promise<void> {
    if (this.isComplete()) {
      return;
    }

    // Create completion promise if not exists
    if (!this.completionPromise) {
      this.completionPromise = new Promise<void>((resolve) => {
        this.completionResolve = resolve;
      });
    }

    if (timeout) {
      await Promise.race([
        this.completionPromise,
        new Promise<void>((_, reject) =>
          setTimeout(() => reject(new Error(`Wait completion timeout after ${timeout}ms`)), timeout)
        ),
      ]);
    } else {
      await this.completionPromise;
    }
  }

  // ==========================================================================
  // Private Methods - Signal Handling
  // ==========================================================================

  /**
   * Handle an incoming shutdown signal
   *
   * @private
   */
  private handleSignal(signalName: string): void {
    const reason = `signal received: ${signalName}`;
    this.logger.info('Shutdown signal received', { signal: signalName });

    // Initiate shutdown in background to avoid blocking signal handling
    this.shutdownAndWait(reason, this.config.shutdownTimeout).catch((err) => {
      this.logger.error('Shutdown from signal failed', {
        error: err instanceof Error ? err.message : String(err),
      });
    });
  }

  // ==========================================================================
  // Private Methods - Hook Execution
  // ==========================================================================

  /**
   * Execute all registered shutdown hooks
   *
   * @private
   * @param signal - AbortSignal for cancellation
   * @param phase - The shutdown phase (pre-shutdown or post-shutdown)
   */
  private async executeHooks(signal: AbortSignal, phase: string): Promise<void> {
    // Copy hooks to avoid modification during iteration
    const hooks = [...this.hooks];

    this.logger.debug('Executing shutdown hooks', {
      phase,
      count: hooks.length,
    });

    const errors: unknown[] = [];

    for (let i = 0; i < hooks.length; i++) {
      const hook = hooks[i];
      const hookName = `${phase}-hook-${i}`;

      // Check if already aborted
      if (signal.aborted) {
        this.logger.warn('Shutdown hook execution canceled', { phase });
        throw new Error('Shutdown aborted during hook execution');
      }

      this.logger.debug('Executing shutdown hook', { phase, hook: hookName });

      try {
        // Execute hook with timeout
        await Promise.race([
          hook(signal),
          new Promise<void>((_, reject) =>
            setTimeout(() => reject(new Error(`Hook timeout after ${this.config.hookTimeout}ms`)), this.config.hookTimeout)
          ),
        ]);
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        this.logger.error('Shutdown hook failed', {
          phase,
          hook: hookName,
          error: error.message,
        });
        errors.push(error);
      }

      // Check if aborted during hook execution
      if (signal.aborted) {
        this.logger.warn('Shutdown hook execution canceled', { phase });
        throw new Error('Shutdown aborted during hook execution');
      }
    }

    if (errors.length > 0) {
      throw new Error(
        `${phase} hooks failed: ${errors.map((e) => (e instanceof Error ? e.message : String(e))).join(', ')}`
      );
    }
  }

  // ==========================================================================
  // Private Methods - State Management
  // ==========================================================================

  /**
   * Set the shutdown state
   *
   * @private
   */
  private setState(state: ShutdownState): void {
    const previousState = this.state;
    this.state = state;
    this.logger.debug('Shutdown state changed', {
      from: previousState,
      to: state,
    });
  }

  // ==========================================================================
  // Public Methods - String Representation
  // ==========================================================================

  /**
   * String returns a string representation of the shutdown manager
   */
  toString(): string {
    return `ShutdownManager{state: ${this.state}, timeout: ${this.config.shutdownTimeout}ms, hooks: ${this.hooks.length}, started: ${this.started}}`;
  }
}

// =============================================================================
// Factory Functions
// =============================================================================

/**
 * Create a new ShutdownManager instance
 *
 * @param config - Shutdown configuration
 * @returns A new ShutdownManager instance
 */
export function createShutdownManager(config?: ShutdownConfig): ShutdownManager {
  return new ShutdownManager(config);
}

// =============================================================================
// Re-exports
// =============================================================================
