// types.ts - Type definitions for session management
//
// This file contains TypeScript types for session state management,
// conversation history, and session lifecycle.
//
// Copyright 2026 baaaht project

// =============================================================================
// Session State Enums
// =============================================================================

/**
 * SessionState represents the lifecycle state of a session
 */
export enum SessionState {
  INITIALIZING = 'initializing',
  ACTIVE = 'active',
  IDLE = 'idle',
  CLOSING = 'closing',
  CLOSED = 'closed',
}

/**
 * SessionStatus represents the operational status of a session
 */
export enum SessionStatus {
  STARTING = 'starting',
  RUNNING = 'running',
  STOPPING = 'stopping',
  STOPPED = 'stopped',
  ERROR = 'error',
}

/**
 * MessageRole represents the role of the message sender
 */
export enum MessageRole {
  USER = 'user',
  SYSTEM = 'system',
  ASSISTANT = 'assistant',
  TOOL = 'tool',
}

// =============================================================================
// Core Session Types
// =============================================================================

/**
 * Session represents a user session with conversation context
 */
export interface Session {
  id: string;
  state: SessionState;
  status: SessionStatus;
  createdAt: Date;
  updatedAt: Date;
  expiresAt?: Date;
  closedAt?: Date;
  metadata: SessionMetadata;
  context: SessionContext;
  containers: string[];
}

/**
 * SessionMetadata contains descriptive information about a session
 */
export interface SessionMetadata {
  name: string;
  description?: string;
  labels?: Record<string, string>;
  ownerId?: string;
  tags?: string[];
}

/**
 * SessionContext holds the conversation and operational context
 */
export interface SessionContext {
  messages?: Message[];
  currentTaskId?: string;
  taskHistory?: string[];
  preferences?: UserPreferences;
  config?: SessionConfig;
  resourceUsage?: ResourceUsage;
}

/**
 * Message represents a message in the conversation
 */
export interface Message {
  id: string;
  timestamp: Date;
  role: MessageRole;
  content: string;
  metadata?: MessageMetadata;
}

/**
 * MessageMetadata contains additional information about a message
 */
export interface MessageMetadata {
  containerId?: string;
  toolName?: string;
  toolCallId?: string;
  extra?: Record<string, string>;
}

/**
 * UserPreferences contains user-specific preferences
 */
export interface UserPreferences {
  timezone?: string;
  language?: string;
  theme?: string;
  notifications?: boolean;
}

/**
 * SessionConfig contains session-specific configuration
 */
export interface SessionConfig {
  maxContainers?: number;
  maxDuration?: number; // milliseconds
  idleTimeout?: number; // milliseconds
  resourceLimits?: ResourceLimits;
  allowedImages?: string[];
  policy?: SessionPolicy;
}

/**
 * SessionPolicy defines security policies for a session
 */
export interface SessionPolicy {
  allowNetwork: boolean;
  allowedNetworks?: string[];
  allowVolumeMount: boolean;
  allowedPaths?: string[];
  requireApproval?: boolean;
}

/**
 * ResourceLimits defines resource constraints for a session
 */
export interface ResourceLimits {
  nanoCpus?: bigint;
  memoryBytes?: bigint;
  memorySwap?: bigint;
  pidsLimit?: bigint;
}

/**
 * ResourceUsage represents current resource usage
 */
export interface ResourceUsage {
  cpuPercent?: number;
  memoryUsage?: bigint;
  memoryLimit?: bigint;
  memoryPercent?: number;
  networkRx?: bigint;
  networkTx?: bigint;
  blockRead?: bigint;
  blockWrite?: bigint;
  pidsCount?: bigint;
}

/**
 * SessionFilter defines filters for querying sessions
 */
export interface SessionFilter {
  state?: SessionState;
  status?: SessionStatus;
  ownerId?: string;
  label?: Record<string, string>;
}

/**
 * SessionStats represents statistics about a session
 */
export interface SessionStats {
  sessionId: string;
  messageCount: number;
  containerCount: number;
  taskCount: number;
  durationSeconds: number;
  timestamp: Date;
}

/**
 * SessionManagerConfig contains configuration for the session manager
 */
export interface SessionManagerConfig {
  maxSessions?: number;
  idleTimeout?: number; // milliseconds
  timeout?: number; // milliseconds
  persistenceEnabled?: boolean;
  cleanupInterval?: number; // milliseconds
}

// =============================================================================
// Error Types
// =============================================================================

/**
 * Error codes for session management failures
 */
export enum SessionErrorCode {
  NOT_FOUND = 'NOT_FOUND',
  ALREADY_EXISTS = 'ALREADY_EXISTS',
  INVALID_STATE = 'INVALID_STATE',
  UNAVAILABLE = 'UNAVAILABLE',
  RESOURCE_EXHAUSTED = 'RESOURCE_EXHAUSTED',
  FAILED_PRECONDITION = 'FAILED_PRECONDITION',
  INTERNAL = 'INTERNAL',
}

/**
 * Error thrown when session operation fails
 */
export class SessionError extends Error {
  constructor(
    message: string,
    public code: SessionErrorCode,
    public details?: string
  ) {
    super(message);
    this.name = 'SessionError';
  }
}

// =============================================================================
// State Machine Types
// =============================================================================

/**
 * Valid state transitions for a session
 */
export const VALID_STATE_TRANSITIONS: Record<SessionState, SessionState[]> = {
  [SessionState.INITIALIZING]: [SessionState.ACTIVE, SessionState.CLOSED],
  [SessionState.ACTIVE]: [SessionState.IDLE, SessionState.CLOSING, SessionState.CLOSED],
  [SessionState.IDLE]: [SessionState.ACTIVE, SessionState.CLOSING, SessionState.CLOSED],
  [SessionState.CLOSING]: [SessionState.CLOSED],
  [SessionState.CLOSED]: [],
};

/**
 * Check if a state transition is valid
 */
export function isValidStateTransition(
  from: SessionState,
  to: SessionState
): boolean {
  return VALID_STATE_TRANSITIONS[from].includes(to);
}

/**
 * Terminal states that cannot be transitioned out of
 */
export const TERMINAL_STATES: Set<SessionState> = new Set([
  SessionState.CLOSED,
]);

/**
 * Active states (non-terminal)
 */
export const ACTIVE_STATES: Set<SessionState> = new Set([
  SessionState.ACTIVE,
  SessionState.IDLE,
]);

// =============================================================================
// Utility Types
// =============================================================================

/**
 * Deep partial type for nested optional objects
 */
export type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends object ? DeepPartial<T[P]> : T[P];
};

/**
 * Session update options
 */
export interface SessionUpdateOptions {
  metadata?: DeepPartial<SessionMetadata>;
  config?: DeepPartial<SessionConfig>;
}
