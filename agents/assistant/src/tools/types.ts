// types.ts - Type definitions for Assistant agent tools
//
// This file contains TypeScript types for the Assistant's tool system,
// which includes the delegate tool for dispatching work to specialized agents.
//
// Copyright 2026 baaaht project

import type {
  Task,
  TaskConfig,
  TaskState,
  TaskType,
  TaskPriority,
  TaskResult,
  AgentType,
} from '../proto/agent.js';

// =============================================================================
// Tool Definitions
// =============================================================================

/**
 * Definition of a tool available to the Assistant
 */
export interface ToolDefinition {
  /**
   * Unique tool name
   */
  name: string;

  /**
   * Human-readable description of what the tool does
   */
  description: string;

  /**
   * JSON Schema for the tool's input parameters
   */
  inputSchema: ToolInputSchema;
}

/**
 * JSON Schema for tool input validation
 */
export interface ToolInputSchema {
  /**
   * JSON Schema type (e.g., "object")
   */
  type: string;

  /**
   * Input property definitions
   */
  properties: Record<string, ToolProperty>;

  /**
   * Required property names
   */
  required?: string[];
}

/**
 * Definition of a tool input property
 */
export interface ToolProperty {
  /**
   * Property type (e.g., "string", "number", "boolean", "array", "object")
   */
  type: string;

  /**
   * Human-readable description
   */
  description: string;

  /**
   * Enum values (if applicable)
   */
  enum?: string[];

  /**
   * Array item type (if type is "array")
   */
  items?: ToolProperty;

  /**
   * Object property definitions (if type is "object")
   */
  properties?: Record<string, ToolProperty>;

  /**
   * Whether this property is required
   */
  required?: boolean;
}

/**
 * Result of a tool execution
 */
export interface ToolResult {
  /**
   * Whether the tool execution was successful
   */
  success: boolean;

  /**
   * Result data (if successful)
   */
  data?: unknown;

  /**
   * Error message (if failed)
   */
  error?: string;

  /**
   * Output text for display
   */
  output?: string;

  /**
   * Additional metadata
   */
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Delegate Tool Types
// =============================================================================

/**
 * Target agent type for delegation
 */
export enum DelegateTarget {
  /**
   * Worker agent - handles file operations, web searches, and basic tasks
   */
  WORKER = 'worker',

  /**
   * Researcher agent - handles deep research tasks (Phase 2)
   */
  RESEARCHER = 'researcher',

  /**
   * Coder agent - handles code analysis and generation (Phase 2)
   */
  CODER = 'coder',
}

/**
 * Operation types that can be delegated
 */
export enum DelegateOperation {
  // File operations
  READ_FILE = 'read_file',
  WRITE_FILE = 'write_file',
  DELETE_FILE = 'delete_file',
  LIST_FILES = 'list_files',
  SEARCH_FILES = 'search_files',

  // Web operations
  WEB_SEARCH = 'web_search',
  WEB_FETCH = 'web_fetch',

  // Research operations (Phase 2)
  DEEP_RESEARCH = 'deep_research',
  SYNTHESIZE_SOURCES = 'synthesize_sources',

  // Code operations (Phase 2)
  ANALYZE_CODE = 'analyze_code',
  GENERATE_CODE = 'generate_code',
  REVIEW_CODE = 'review_code',
  EXECUTE_CODE = 'execute_code',
}

/**
 * Parameters for the delegate tool
 */
export interface DelegateParams {
  /**
   * Target agent to delegate to
   */
  target: DelegateTarget;

  /**
   * Operation to perform
   */
  operation: DelegateOperation;

  /**
   * Operation-specific parameters
   */
  parameters: Record<string, unknown>;

  /**
   * Optional timeout in milliseconds
   */
  timeout?: number;

  /**
   * Optional priority for the delegated task
   */
  priority?: TaskPriority;
}

/**
 * Result of a delegation operation
 */
export interface DelegateResult {
  /**
   * Whether delegation was successful
   */
  success: boolean;

  /**
   * Task ID for tracking
   */
  taskId?: string;

  /**
   * Result data from the delegated agent
   */
  data?: unknown;

  /**
   * Output text for display
   */
  output?: string;

  /**
   * Error message (if failed)
   */
  error?: string;

  /**
   * Final task state
   */
  taskState?: TaskState;

  /**
   * Additional metadata
   */
  metadata?: DelegateMetadata;
}

/**
 * Metadata about a delegation operation
 */
export interface DelegateMetadata {
  /**
   * Target agent that handled the delegation
   */
  target: DelegateTarget;

  /**
   * Operation performed
   */
  operation: DelegateOperation;

  /**
   * Timestamp when delegation was created
   */
  createdAt: Date;

  /**
   * Timestamp when delegation completed
   */
  completedAt?: Date;

  /**
   * Execution duration in milliseconds
   */
  duration?: number;

  /**
   * Agent ID that handled the task
   */
  agentId?: string;
}

// =============================================================================
// Delegation Configuration
// =============================================================================

/**
 * Configuration for the delegate tool
 */
export interface DelegateToolConfig {
  /**
   * Default timeout for delegations in milliseconds
   */
  defaultTimeout?: number;

  /**
   * Maximum timeout for delegations in milliseconds
   */
  maxTimeout?: number;

  /**
   * Default task priority
   */
  defaultPriority?: TaskPriority;

  /**
   * Whether to enable streaming results from delegated agents
   */
  enableStreaming?: boolean;

  /**
   * Maximum number of concurrent delegations
   */
  maxConcurrent?: number;

  /**
   * Retry configuration
   */
  retry?: RetryConfig;
}

/**
 * Retry configuration for delegations
 */
export interface RetryConfig {
  /**
   * Maximum number of retry attempts
   */
  maxAttempts?: number;

  /**
   * Initial retry delay in milliseconds
   */
  initialDelay?: number;

  /**
   * Maximum retry delay in milliseconds
   */
  maxDelay?: number;

  /**
   * Backoff multiplier (e.g., 2 for exponential backoff)
   */
  backoffMultiplier?: number;
}

// =============================================================================
// Error Types
// =============================================================================

/**
 * Error codes for delegation failures
 */
export enum DelegateErrorCode {
  // Target errors
  TARGET_NOT_FOUND = 'TARGET_NOT_FOUND',
  TARGET_UNAVAILABLE = 'TARGET_UNAVAILABLE',
  TARGET_NOT_SUPPORTED = 'TARGET_NOT_SUPPORTED',

  // Operation errors
  OPERATION_NOT_SUPPORTED = 'OPERATION_NOT_SUPPORTED',
  OPERATION_FAILED = 'OPERATION_FAILED',
  OPERATION_TIMEOUT = 'OPERATION_TIMEOUT',
  OPERATION_CANCELLED = 'OPERATION_CANCELLED',

  // Parameter errors
  INVALID_PARAMETERS = 'INVALID_PARAMETERS',
  MISSING_REQUIRED_PARAMETER = 'MISSING_REQUIRED_PARAMETER',

  // Task errors
  TASK_CREATION_FAILED = 'TASK_CREATION_FAILED',
  TASK_EXECUTION_FAILED = 'TASK_EXECUTION_FAILED',

  // System errors
  ORCHESTRATOR_UNAVAILABLE = 'ORCHESTRATOR_UNAVAILABLE',
  NETWORK_ERROR = 'NETWORK_ERROR',
  UNKNOWN_ERROR = 'UNKNOWN_ERROR',
}

/**
 * Error thrown when delegation fails
 */
export class DelegateError extends Error {
  constructor(
    message: string,
    public code: DelegateErrorCode,
    public retryable: boolean = false,
    public target?: DelegateTarget,
    public operation?: DelegateOperation
  ) {
    super(message);
    this.name = 'DelegateError';
  }
}

// =============================================================================
// Worker Agent Delegation Types
// =============================================================================

/**
 * Parameters for file read operations
 */
export interface ReadFileParams {
  /**
   * Path to the file to read
   */
  path: string;

  /**
   * Optional offset to start reading from
   */
  offset?: number;

  /**
   * Optional limit on bytes to read
   */
  limit?: number;
}

/**
 * Parameters for file write operations
 */
export interface WriteFileParams {
  /**
   * Path to the file to write
   */
  path: string;

  /**
   * Content to write
   */
  content: string;

  /**
   * Whether to create parent directories if needed
   */
  createParents?: boolean;
}

/**
 * Parameters for file delete operations
 */
export interface DeleteFileParams {
  /**
   * Path to the file to delete
   */
  path: string;

  /**
   * Whether to recursively delete directories
   */
  recursive?: boolean;
}

/**
 * Parameters for file list operations
 */
export interface ListFilesParams {
  /**
   * Directory path to list
   */
  path: string;

  /**
   * Optional pattern to filter results (glob pattern)
   */
  pattern?: string;

  /**
   * Maximum depth for recursive listing
   */
  maxDepth?: number;
}

/**
 * Parameters for file search operations
 */
export interface SearchFilesParams {
  /**
   * Directory path to search in
   */
  path: string;

  /**
   * Search pattern or regex
   */
  pattern: string;

  /**
   * Whether to use regex matching
   */
  regex?: boolean;

  /**
   * Whether to search recursively
   */
  recursive?: boolean;
}

/**
 * Parameters for web search operations
 */
export interface WebSearchParams {
  /**
   * Search query
   */
  query: string;

  /**
   * Maximum number of results
   */
  maxResults?: number;
}

/**
 * Parameters for web fetch operations
 */
export interface WebFetchParams {
  /**
   * URL to fetch
   */
  url: string;

  /**
   * HTTP method (GET, POST, etc.)
   */
  method?: string;

  /**
   * Request headers
   */
  headers?: Record<string, string>;

  /**
   * Request body (for POST requests)
   */
  body?: string;
}
