// manager.ts - Session state manager implementation
//
// This file contains the session manager implementation that handles
// session lifecycle, message history, and state transitions.
//
// Copyright 2026 baaaht project

import { EventEmitter } from 'events';
import type {
  Session,
  SessionState,
  SessionStatus,
  SessionMetadata,
  SessionConfig,
  SessionContext,
  Message,
  SessionFilter,
  SessionStats,
  SessionManagerConfig,
  SessionError,
  SessionErrorCode,
  SessionUpdateOptions,
  isValidStateTransition,
  TERMINAL_STATES,
  ACTIVE_STATES,
} from './types.js';

// =============================================================================
// Constants
// =============================================================================

const SESSION_ID_PREFIX_LENGTH = 8;
const DEFAULT_CLEANUP_INTERVAL = 60 * 1000; // 1 minute

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Generate a unique session ID
 */
function generateSessionId(): string {
  return `${Date.now()}-${Math.random().toString(36).substring(2, 15)}`;
}

/**
 * Generate a unique message ID
 */
function generateMessageId(): string {
  return `${Date.now()}-${Math.random().toString(36).substring(2, 15)}`;
}

/**
 * Get session ID prefix for display
 */
function getSessionIdPrefix(sessionId: string): string {
  return sessionId.length > SESSION_ID_PREFIX_LENGTH
    ? sessionId.substring(0, SESSION_ID_PREFIX_LENGTH)
    : sessionId;
}

/**
 * Create a deep copy of a session to prevent external modification
 */
function copySession(session: Session): Session {
  return {
    ...session,
    containers: [...session.containers],
    context: copySessionContext(session.context),
    metadata: copySessionMetadata(session.metadata),
  };
}

/**
 * Create a deep copy of session context
 */
function copySessionContext(context: SessionContext): SessionContext {
  return {
    messages: context.messages ? [...context.messages] : undefined,
    currentTaskId: context.currentTaskId,
    taskHistory: context.taskHistory ? [...context.taskHistory] : undefined,
    preferences: context.preferences ? { ...context.preferences } : undefined,
    config: context.config ? { ...context.config } : undefined,
    resourceUsage: context.resourceUsage ? { ...context.resourceUsage } : undefined,
  };
}

/**
 * Create a deep copy of session metadata
 */
function copySessionMetadata(metadata: SessionMetadata): SessionMetadata {
  return {
    ...metadata,
    labels: metadata.labels ? { ...metadata.labels } : undefined,
    tags: metadata.tags ? [...metadata.tags] : undefined,
  };
}

// =============================================================================
// Session With State Machine
// =============================================================================

/**
 * SessionWithStateMachine wraps a Session with state machine logic
 */
class SessionWithStateMachine {
  private session: Session;

  constructor(session: Session) {
    this.session = session;
  }

  /**
   * Get the underlying session
   */
  get Session(): Session {
    return this.session;
  }

  /**
   * Get the current state
   */
  get currentState(): SessionState {
    return this.session.state;
  }

  /**
   * Check if the session is in a terminal state
   */
  isTerminal(): boolean {
    return TERMINAL_STATES.has(this.session.state);
  }

  /**
   * Check if the session is active (non-terminal)
   */
  isActive(): boolean {
    return ACTIVE_STATES.has(this.session.state);
  }

  /**
   * Transition to a new state
   */
  transition(to: SessionState): void {
    if (!isValidStateTransition(this.session.state, to)) {
      throw new SessionError(
        `Invalid state transition from ${this.session.state} to ${to}`,
        SessionErrorCode.INVALID_STATE
      );
    }
    this.session.state = to;
  }

  /**
   * Activate the session (transition to ACTIVE)
   */
  activate(): void {
    this.transition(SessionState.ACTIVE);
  }

  /**
   * Idle the session (transition to IDLE)
   */
  idle(): void {
    this.transition(SessionState.IDLE);
  }

  /**
   * Close the session (transition to CLOSED)
   */
  close(): void {
    this.transition(SessionState.CLOSED);
  }

  /**
   * Force close the session (any state -> CLOSED)
   */
  forceClose(): void {
    this.session.state = SessionState.CLOSED;
  }
}

// =============================================================================
// Session Manager
// =============================================================================

/**
 * Manager manages session lifecycles
 */
export class SessionManager extends EventEmitter {
  private sessions: Map<string, SessionWithStateMachine>;
  private config: Required<SessionManagerConfig>;
  private closed: boolean;
  private cleanupTimer?: NodeJS.Timeout;

  constructor(config: SessionManagerConfig = {}) {
    super();

    this.sessions = new Map();
    this.closed = false;

    // Apply defaults
    this.config = {
      maxSessions: config.maxSessions ?? 100,
      idleTimeout: config.idleTimeout ?? 30 * 60 * 1000, // 30 minutes
      timeout: config.timeout ?? 60 * 60 * 1000, // 1 hour
      persistenceEnabled: config.persistenceEnabled ?? false,
      cleanupInterval: config.cleanupInterval ?? DEFAULT_CLEANUP_INTERVAL,
    };

    // Start cleanup loop
    this.startCleanupLoop();
  }

  // ==========================================================================
  // Session Lifecycle Methods
  // ==========================================================================

  /**
   * Create creates a new session with the given metadata and configuration
   */
  async create(
    metadata: SessionMetadata,
    sessionConfig: SessionConfig = {}
  ): Promise<string> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    // Check max sessions limit
    if (this.config.maxSessions > 0 && this.sessions.size >= this.config.maxSessions) {
      throw new SessionError(
        `Maximum sessions limit reached: ${this.config.maxSessions}`,
        SessionErrorCode.RESOURCE_EXHAUSTED
      );
    }

    // Generate session ID
    const sessionId = generateSessionId();

    // Calculate expiration
    const now = new Date();
    let expiresAt: Date | undefined;
    if (sessionConfig.maxDuration && sessionConfig.maxDuration > 0) {
      expiresAt = new Date(now.getTime() + sessionConfig.maxDuration);
    }

    // Create session
    const session: Session = {
      id: sessionId,
      state: SessionState.INITIALIZING,
      status: SessionStatus.STARTING,
      createdAt: now,
      updatedAt: now,
      expiresAt,
      metadata,
      context: {
        messages: [],
        taskHistory: [],
        config: sessionConfig,
      },
      containers: [],
    };

    // Wrap with state machine
    const sessionWithSM = new SessionWithStateMachine(session);

    // Store session
    this.sessions.set(sessionId, sessionWithSM);

    // Transition to active state
    sessionWithSM.activate();
    session.status = SessionStatus.RUNNING;

    this.emit('sessionCreated', { sessionId, metadata });
    this.emit('sessionActivated', { sessionId, state: SessionState.ACTIVE });

    return sessionId;
  }

  /**
   * Get retrieves a session by ID
   */
  async get(sessionId: string): Promise<Session> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const sessionWithSM = this.sessions.get(sessionId);
    if (!sessionWithSM) {
      throw new SessionError(
        `Session not found: ${sessionId}`,
        SessionErrorCode.NOT_FOUND
      );
    }

    // Return a copy to prevent external modification
    return copySession(sessionWithSM.Session);
  }

  /**
   * List retrieves all sessions matching the filter
   */
  async list(filter?: SessionFilter): Promise<Session[]> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const result: Session[] = [];

    for (const sessionWithSM of this.sessions.values()) {
      const session = sessionWithSM.Session;

      // Apply filter
      if (filter && !this.matchesFilter(session, filter)) {
        continue;
      }

      result.push(copySession(session));
    }

    return result;
  }

  /**
   * Update updates a session's metadata
   */
  async update(sessionId: string, options: SessionUpdateOptions): Promise<void> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const sessionWithSM = this.sessions.get(sessionId);
    if (!sessionWithSM) {
      throw new SessionError(
        `Session not found: ${sessionId}`,
        SessionErrorCode.NOT_FOUND
      );
    }

    // Don't allow updates to closed sessions
    if (sessionWithSM.isTerminal()) {
      throw new SessionError(
        'Cannot update closed session',
        SessionErrorCode.FAILED_PRECONDITION
      );
    }

    const session = sessionWithSM.Session;

    // Apply updates
    if (options.metadata) {
      if (options.metadata.name !== undefined) {
        session.metadata.name = options.metadata.name;
      }
      if (options.metadata.description !== undefined) {
        session.metadata.description = options.metadata.description;
      }
      if (options.metadata.labels) {
        session.metadata.labels = {
          ...session.metadata.labels,
          ...options.metadata.labels,
        };
      }
      if (options.metadata.tags) {
        session.metadata.tags = options.metadata.tags;
      }
    }

    if (options.config) {
      session.context.config = {
        ...session.context.config,
        ...options.config,
      };
    }

    session.updatedAt = new Date();

    this.emit('sessionUpdated', { sessionId, options });
  }

  /**
   * CloseSession transitions a session to closed state
   */
  async closeSession(sessionId: string): Promise<void> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const sessionWithSM = this.sessions.get(sessionId);
    if (!sessionWithSM) {
      throw new SessionError(
        `Session not found: ${sessionId}`,
        SessionErrorCode.NOT_FOUND
      );
    }

    const currentState = sessionWithSM.currentState;

    // Already closed - idempotent operation
    if (currentState === SessionState.CLOSED) {
      return;
    }

    // Close the session
    sessionWithSM.close();
    const session = sessionWithSM.Session;
    session.status = SessionStatus.STOPPED;
    session.closedAt = new Date();

    this.emit('sessionClosed', { sessionId, state: SessionState.CLOSED });
  }

  /**
   * Delete removes a session from the manager
   */
  async delete(sessionId: string): Promise<void> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const sessionWithSM = this.sessions.get(sessionId);
    if (!sessionWithSM) {
      throw new SessionError(
        `Session not found: ${sessionId}`,
        SessionErrorCode.NOT_FOUND
      );
    }

    // Only allow deletion of closed sessions
    if (!sessionWithSM.isTerminal()) {
      throw new SessionError(
        'Cannot delete active session, close it first',
        SessionErrorCode.FAILED_PRECONDITION
      );
    }

    this.sessions.delete(sessionId);
    this.emit('sessionDeleted', { sessionId });
  }

  // ==========================================================================
  // Message Methods
  // ==========================================================================

  /**
   * AddMessage adds a message to a session's conversation history
   */
  async addMessage(sessionId: string, message: Partial<Message>): Promise<void> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const sessionWithSM = this.sessions.get(sessionId);
    if (!sessionWithSM) {
      throw new SessionError(
        `Session not found: ${sessionId}`,
        SessionErrorCode.NOT_FOUND
      );
    }

    // Don't allow adding messages to closed sessions
    if (sessionWithSM.isTerminal()) {
      throw new SessionError(
        'Cannot add message to closed session',
        SessionErrorCode.FAILED_PRECONDITION
      );
    }

    const session = sessionWithSM.Session;

    // Ensure message has ID and timestamp
    const fullMessage: Message = {
      id: message.id ?? generateMessageId(),
      timestamp: message.timestamp ?? new Date(),
      role: message.role!,
      content: message.content!,
      metadata: message.metadata,
    };

    // Add message to history
    if (!session.context.messages) {
      session.context.messages = [];
    }
    session.context.messages.push(fullMessage);
    session.updatedAt = new Date();

    // Transition from idle to active when a message is added
    if (sessionWithSM.currentState === SessionState.IDLE) {
      sessionWithSM.activate();
      this.emit('sessionActivated', { sessionId, state: SessionState.ACTIVE });
    }

    this.emit('messageAdded', { sessionId, message: fullMessage });
  }

  /**
   * GetMessages retrieves all messages for a session
   */
  async getMessages(sessionId: string): Promise<Message[]> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const sessionWithSM = this.sessions.get(sessionId);
    if (!sessionWithSM) {
      throw new SessionError(
        `Session not found: ${sessionId}`,
        SessionErrorCode.NOT_FOUND
      );
    }

    const session = sessionWithSM.Session;
    return session.context.messages ? [...session.context.messages] : [];
  }

  // ==========================================================================
  // State and Stats Methods
  // ==========================================================================

  /**
   * GetState returns the current state of a session
   */
  async getState(sessionId: string): Promise<SessionState> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const sessionWithSM = this.sessions.get(sessionId);
    if (!sessionWithSM) {
      throw new SessionError(
        `Session not found: ${sessionId}`,
        SessionErrorCode.NOT_FOUND
      );
    }

    return sessionWithSM.currentState;
  }

  /**
   * GetStats returns statistics for a session
   */
  async getStats(sessionId: string): Promise<SessionStats> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const sessionWithSM = this.sessions.get(sessionId);
    if (!sessionWithSM) {
      throw new SessionError(
        `Session not found: ${sessionId}`,
        SessionErrorCode.NOT_FOUND
      );
    }

    const session = sessionWithSM.Session;

    return {
      sessionId,
      messageCount: session.context.messages?.length ?? 0,
      containerCount: session.containers.length,
      taskCount: session.context.taskHistory?.length ?? 0,
      durationSeconds: Math.floor(
        (Date.now() - session.createdAt.getTime()) / 1000
      ),
      timestamp: new Date(),
    };
  }

  /**
   * Stats returns manager statistics
   */
  getStats(): {
    active: number;
    idle: number;
    closing: number;
    closed: number;
    total: number;
  } {
    let active = 0;
    let idle = 0;
    let closing = 0;
    let closed = 0;

    for (const sessionWithSM of this.sessions.values()) {
      const state = sessionWithSM.currentState;
      switch (state) {
        case SessionState.ACTIVE:
          active++;
          break;
        case SessionState.IDLE:
          idle++;
          break;
        case SessionState.CLOSING:
          closing++;
          break;
        case SessionState.CLOSED:
          closed++;
          break;
      }
    }

    return {
      active,
      idle,
      closing,
      closed,
      total: this.sessions.size,
    };
  }

  // ==========================================================================
  // Manager Lifecycle Methods
  // ==========================================================================

  /**
   * Close closes the session manager and cleans up all sessions
   */
  async close(): Promise<void> {
    if (this.closed) {
      return;
    }

    // Stop cleanup timer
    if (this.cleanupTimer) {
      clearInterval(this.cleanupTimer);
      this.cleanupTimer = undefined;
    }

    // Close all active sessions
    for (const [sessionId, sessionWithSM] of this.sessions.entries()) {
      if (!sessionWithSM.isTerminal()) {
        const session = sessionWithSM.Session;
        sessionWithSM.forceClose();
        session.status = SessionStatus.STOPPED;
        session.closedAt = new Date();
        this.emit('sessionClosed', { sessionId, state: SessionState.CLOSED });
      }
    }

    this.closed = true;
    this.emit('managerClosed');
  }

  // ==========================================================================
  // Private Methods
  // ==========================================================================

  /**
   * Start the cleanup loop
   */
  private startCleanupLoop(): void {
    this.cleanupTimer = setInterval(() => {
      this.cleanup().catch((error) => {
        this.emit('error', error);
      });
    }, this.config.cleanupInterval);
  }

  /**
   * Cleanup removes expired sessions and transitions idle sessions
   */
  private cleanup(): void {
    const now = Date.now();

    for (const [sessionId, sessionWithSM] of this.sessions.entries()) {
      const session = sessionWithSM.Session;

      // Skip terminal sessions
      if (sessionWithSM.isTerminal()) {
        continue;
      }

      // Check expiration
      if (session.expiresAt && now > session.expiresAt.getTime()) {
        sessionWithSM.close();
        session.status = SessionStatus.STOPPING;
        this.emit('sessionExpired', { sessionId });
        continue;
      }

      // Check idle timeout
      if (
        this.config.idleTimeout > 0 &&
        sessionWithSM.currentState === SessionState.ACTIVE
      ) {
        const idleTime = now - session.updatedAt.getTime();
        if (idleTime > this.config.idleTimeout) {
          sessionWithSM.idle();
          this.emit('sessionIdled', { sessionId, idleTime });
        }
      }
    }
  }

  /**
   * MatchesFilter checks if a session matches the given filter
   */
  private matchesFilter(session: Session, filter: SessionFilter): boolean {
    if (filter.state !== undefined && session.state !== filter.state) {
      return false;
    }

    if (filter.status !== undefined && session.status !== filter.status) {
      return false;
    }

    if (
      filter.ownerId !== undefined &&
      session.metadata.ownerId !== filter.ownerId
    ) {
      return false;
    }

    if (filter.label) {
      for (const [key, value] of Object.entries(filter.label)) {
        if (!session.metadata.labels || session.metadata.labels[key] !== value) {
          return false;
        }
      }
    }

    return true;
  }

  /**
   * ValidateSession checks if a session exists and is active
   */
  async validateSession(sessionId: string): Promise<void> {
    if (this.closed) {
      throw new SessionError(
        'Session manager is closed',
        SessionErrorCode.UNAVAILABLE
      );
    }

    const sessionWithSM = this.sessions.get(sessionId);
    if (!sessionWithSM) {
      throw new SessionError(
        `Session not found: ${sessionId}`,
        SessionErrorCode.NOT_FOUND
      );
    }

    if (!sessionWithSM.isActive()) {
      throw new SessionError(
        `Session is not active: ${sessionId} (state: ${sessionWithSM.currentState})`,
        SessionErrorCode.FAILED_PRECONDITION
      );
    }

    // Check expiration
    const session = sessionWithSM.Session;
    if (session.expiresAt && Date.now() > session.expiresAt.getTime()) {
      throw new SessionError(
        `Session has expired: ${sessionId}`,
        SessionErrorCode.FAILED_PRECONDITION
      );
    }
  }

  /**
   * String representation of the manager
   */
  toString(): string {
    const stats = this.getStats();
    return `SessionManager{active: ${stats.active}, idle: ${stats.idle}, closing: ${stats.closing}, closed: ${stats.closed}, total: ${stats.total}}`;
  }
}

// =============================================================================
// Re-exports
// =============================================================================

export * from './types.js';
