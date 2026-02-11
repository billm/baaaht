// agent.ts - TypeScript types for Agent protocol buffer definitions
//
// This file contains TypeScript types for the AgentService which provides
// agent registration, task execution, and bidirectional streaming.
// Auto-generated from proto/agent.proto
//
// Copyright 2026 baaaht project

import type {
  ResourceLimits,
  ResourceUsage,
  Status,
} from './common.js';

// =============================================================================
// Agent Types
// =============================================================================

/**
 * Agent represents an agent instance
 */
export interface Agent {
  id?: string;
  name?: string;
  type?: AgentType;
  state?: AgentState;
  status?: Status;
  registeredAt?: Date;
  lastHeartbeat?: Date;
  metadata?: AgentMetadata;
  capabilities?: AgentCapabilities;
  resources?: ResourceUsage;
  activeTasks?: string[];
}

/**
 * AgentType represents the type of agent
 */
export enum AgentType {
  AGENT_TYPE_UNSPECIFIED = 0,
  AGENT_TYPE_ORCHESTRATOR = 1,
  AGENT_TYPE_GATEWAY = 2,
  AGENT_TYPE_WORKER = 3,
  AGENT_TYPE_TOOL = 4,
  AGENT_TYPE_MONITOR = 5,
}

/**
 * AgentState represents the lifecycle state of an agent
 */
export enum AgentState {
  AGENT_STATE_UNSPECIFIED = 0,
  AGENT_STATE_INITIALIZING = 1,
  AGENT_STATE_REGISTERING = 2,
  AGENT_STATE_IDLE = 3,
  AGENT_STATE_BUSY = 4,
  AGENT_STATE_UNREGISTERING = 5,
  AGENT_STATE_TERMINATED = 6,
}

/**
 * AgentMetadata contains descriptive information about an agent
 */
export interface AgentMetadata {
  version?: string;
  description?: string;           // Optional
  labels?: Record<string, string>;
  ownerId?: string;               // Optional
  tags?: string[];
  hostname?: string;
  pid?: string;                   // Process ID as string
}

/**
 * AgentCapabilities describes what an agent can do
 */
export interface AgentCapabilities {
  supportedTasks?: string[];
  supportedTools?: string[];
  maxConcurrentTasks?: number;
  resourceLimits?: ResourceLimits;
  supportsStreaming?: boolean;
  supportsCancellation?: boolean;
  supportedProtocols?: string[];
}

// =============================================================================
// Task Types
// =============================================================================

/**
 * Task represents a task assigned to an agent
 */
export interface Task {
  id?: string;
  name?: string;
  sessionId?: string;
  type?: TaskType;
  state?: TaskState;
  priority?: TaskPriority;
  createdAt?: Date;
  startedAt?: Date;    // Optional
  completedAt?: Date;  // Optional
  expiresAt?: Date;    // Optional
  config?: TaskConfig;
  result?: TaskResult; // Optional
  error?: TaskError;   // Optional
  progress?: number;   // 0.0 to 1.0
  agentId?: string;
}

/**
 * TaskType represents the type of task
 */
export enum TaskType {
  TASK_TYPE_UNSPECIFIED = 0,
  TASK_TYPE_CODE_EXECUTION = 1,
  TASK_TYPE_FILE_OPERATION = 2,
  TASK_TYPE_NETWORK_REQUEST = 3,
  TASK_TYPE_DATA_PROCESSING = 4,
  TASK_TYPE_CONTAINER_OPERATION = 5,
  TASK_TYPE_TOOL_EXECUTION = 6,
  TASK_TYPE_CUSTOM = 7,
}

/**
 * TaskState represents the state of a task
 */
export enum TaskState {
  TASK_STATE_UNSPECIFIED = 0,
  TASK_STATE_PENDING = 1,
  TASK_STATE_QUEUED = 2,
  TASK_STATE_RUNNING = 3,
  TASK_STATE_PAUSED = 4,
  TASK_STATE_COMPLETED = 5,
  TASK_STATE_FAILED = 6,
  TASK_STATE_CANCELLED = 7,
  TASK_STATE_TIMEOUT = 8,
}

/**
 * TaskPriority represents the priority of a task
 */
export enum TaskPriority {
  TASK_PRIORITY_UNSPECIFIED = 0,
  TASK_PRIORITY_LOW = 1,
  TASK_PRIORITY_NORMAL = 2,
  TASK_PRIORITY_HIGH = 3,
  TASK_PRIORITY_CRITICAL = 4,
}

/**
 * TaskConfig contains configuration for a task
 */
export interface TaskConfig {
  command?: string;
  arguments?: string[];
  environment?: Record<string, string>;
  workingDirectory?: string;
  timeoutNs?: bigint;       // Duration in nanoseconds
  maxRetries?: bigint;
  retryOnFailure?: boolean;
  inputData?: Uint8Array;   // Optional
  parameters?: Record<string, string>;
  constraints?: TaskConstraints;
}

/**
 * TaskConstraints defines constraints for task execution
 */
export interface TaskConstraints {
  maxMemoryBytes?: bigint;
  maxCpuNs?: bigint;           // CPU time in nanoseconds
  maxExecutionTimeNs?: bigint;
  allowedOperations?: string[];
  deniedOperations?: string[];
}

/**
 * TaskResult contains the result of a completed task
 */
export interface TaskResult {
  exitCode?: number;
  outputData?: Uint8Array;
  outputText?: string;
  errorText?: string;
  metadata?: Record<string, string>;
  completedAt?: Date;
  executionDurationNs?: bigint;
}

/**
 * TaskError contains error information for a failed task
 */
export interface TaskError {
  code?: string;
  message?: string;
  details?: string[];
  stackTrace?: string;         // Optional
  occurredAt?: Date;
}

// =============================================================================
// Message Types
// =============================================================================

/**
 * AgentMessage represents a message to or from an agent
 */
export interface AgentMessage {
  id?: string;
  type?: MessageType;
  timestamp?: Date;
  sourceId?: string;
  targetId?: string;
  payload?: AgentMessagePayload;
  metadata?: AgentMessageMetadata;
}

/**
 * MessageType represents the type of message
 */
export enum MessageType {
  MESSAGE_TYPE_UNSPECIFIED = 0,
  MESSAGE_TYPE_TASK_REQUEST = 1,
  MESSAGE_TYPE_TASK_RESPONSE = 2,
  MESSAGE_TYPE_TASK_UPDATE = 3,
  MESSAGE_TYPE_CONTROL = 4,
  MESSAGE_TYPE_DATA = 5,
  MESSAGE_TYPE_EVENT = 6,
  MESSAGE_TYPE_HEARTBEAT = 7,
  MESSAGE_TYPE_ERROR = 8,
}

/**
 * AgentMessagePayload represents the payload of an agent message (oneof field)
 */
export type AgentMessagePayload =
  | { taskMessage: TaskMessage }
  | { controlMessage: ControlMessage }
  | { dataMessage: DataMessage }
  | { eventMessage: EventMessage };

/**
 * TaskMessage contains task-related information
 */
export interface TaskMessage {
  taskId?: string;
  sessionId?: string;
  state?: TaskState;
  progress?: number;
  statusMessage?: string;
  output?: Uint8Array;
}

/**
 * ControlMessage contains control commands
 */
export interface ControlMessage {
  command?: ControlCommand;
  parameters?: Record<string, string>;
}

/**
 * ControlCommand represents a control command
 */
export enum ControlCommand {
  CONTROL_COMMAND_UNSPECIFIED = 0,
  CONTROL_COMMAND_PAUSE = 1,
  CONTROL_COMMAND_RESUME = 2,
  CONTROL_COMMAND_CANCEL = 3,
  CONTROL_COMMAND_SHUTDOWN = 4,
  CONTROL_COMMAND_RESTART = 5,
  CONTROL_COMMAND_STATUS = 6,
}

/**
 * DataMessage contains arbitrary data
 */
export interface DataMessage {
  contentType?: string;
  data?: Uint8Array;
  headers?: Record<string, string>;
}

/**
 * EventMessage contains event information
 */
export interface EventMessage {
  eventType?: string;
  source?: string;
  data?: Record<string, string>;
}

/**
 * AgentMessageMetadata contains additional message information
 */
export interface AgentMessageMetadata {
  correlationId?: string;
  sessionId?: string;
  headers?: Record<string, string>;
  priority?: number;
}

// =============================================================================
// Request/Response Messages - Registration
// =============================================================================

/**
 * RegisterRequest
 */
export interface RegisterRequest {
  name?: string;
  type?: AgentType;
  metadata?: AgentMetadata;
  capabilities?: AgentCapabilities;
}

/**
 * RegisterResponse
 */
export interface RegisterResponse {
  agentId?: string;
  agent?: Agent;
  registrationToken?: string;
}

/**
 * UnregisterRequest
 */
export interface UnregisterRequest {
  agentId?: string;
  reason?: string;  // Optional
}

/**
 * UnregisterResponse
 */
export interface UnregisterResponse {
  success?: boolean;
  message?: string;
}

/**
 * HeartbeatRequest
 */
export interface HeartbeatRequest {
  agentId?: string;
  resources?: ResourceUsage;  // Optional
  activeTasks?: string[];
}

/**
 * HeartbeatResponse
 */
export interface HeartbeatResponse {
  timestamp?: Date;
  pendingTasks?: string[];
}

// =============================================================================
// Request/Response Messages - Task Execution
// =============================================================================

/**
 * ExecuteTaskRequest
 */
export interface ExecuteTaskRequest {
  agentId?: string;
  sessionId?: string;
  config?: TaskConfig;
  type?: TaskType;
  priority?: TaskPriority;
}

/**
 * ExecuteTaskResponse
 */
export interface ExecuteTaskResponse {
  taskId?: string;
  task?: Task;
}

/**
 * StreamTaskRequest
 */
export interface StreamTaskRequest {
  taskId?: string;
  payload?: StreamTaskRequestPayload;
}

/**
 * StreamTaskRequestPayload represents the payload of a stream task request (oneof field)
 */
export type StreamTaskRequestPayload =
  | { input: TaskInput }
  | { heartbeat: null }
  | { cancel: CancelCommand };

/**
 * TaskInput contains input data for a streaming task
 */
export interface TaskInput {
  data?: Uint8Array;
  metadata?: Record<string, string>;
}

/**
 * CancelCommand cancels a task
 */
export interface CancelCommand {
  reason?: string;  // Optional
  force?: boolean;
}

/**
 * StreamTaskResponse
 */
export interface StreamTaskResponse {
  payload?: StreamTaskResponsePayload;
}

/**
 * StreamTaskResponsePayload represents the payload of a stream task response (oneof field)
 */
export type StreamTaskResponsePayload =
  | { output: TaskOutput }
  | { status: TaskStatusUpdate }
  | { progress: TaskProgress }
  | { complete: TaskComplete }
  | { error: TaskError }
  | { heartbeat: null };

/**
 * TaskOutput contains output data from a task
 */
export interface TaskOutput {
  data?: Uint8Array;
  text?: string;           // Optional, for text output
  streamType?: string;     // stdout, stderr
}

/**
 * TaskStatusUpdate contains status update for a task
 */
export interface TaskStatusUpdate {
  state?: TaskState;
  message?: string;
  timestamp?: Date;
}

/**
 * TaskProgress contains progress information for a task
 */
export interface TaskProgress {
  percent?: number;        // 0.0 to 1.0
  message?: string;        // Optional
  details?: Record<string, string>;
}

/**
 * TaskComplete indicates task completion
 */
export interface TaskComplete {
  taskId?: string;
  result?: TaskResult;
  completedAt?: Date;
}

/**
 * CancelTaskRequest
 */
export interface CancelTaskRequest {
  taskId?: string;
  reason?: string;  // Optional
  force?: boolean;
}

/**
 * CancelTaskResponse
 */
export interface CancelTaskResponse {
  taskId?: string;
  state?: TaskState;
  cancelled?: boolean;
}

/**
 * ListTasksRequest
 */
export interface ListTasksRequest {
  agentId?: string;
  filter?: TaskFilter;
}

/**
 * TaskFilter defines filters for querying tasks
 */
export interface TaskFilter {
  state?: TaskState;              // Optional
  type?: TaskType;                // Optional
  priority?: TaskPriority;        // Optional
  sessionId?: string;             // Optional
  createdAfter?: Date;            // Optional
  createdBefore?: Date;           // Optional
}

/**
 * ListTasksResponse
 */
export interface ListTasksResponse {
  tasks?: Task[];
  totalCount?: number;
}

/**
 * GetTaskStatusRequest
 */
export interface GetTaskStatusRequest {
  taskId?: string;
}

/**
 * GetTaskStatusResponse
 */
export interface GetTaskStatusResponse {
  task?: Task;
}

// =============================================================================
// Request/Response Messages - Communication
// =============================================================================

/**
 * StreamAgentRequest
 */
export interface StreamAgentRequest {
  agentId?: string;
  payload?: StreamAgentRequestPayload;
}

/**
 * StreamAgentRequestPayload represents the payload of a stream agent request (oneof field)
 */
export type StreamAgentRequestPayload =
  | { message: AgentMessage }
  | { heartbeat: null };

/**
 * StreamAgentResponse
 */
export interface StreamAgentResponse {
  payload?: StreamAgentResponsePayload;
}

/**
 * StreamAgentResponsePayload represents the payload of a stream agent response (oneof field)
 */
export type StreamAgentResponsePayload =
  | { message: AgentMessage }
  | { heartbeat: null };

/**
 * AgentSendMessageRequest
 */
export interface AgentSendMessageRequest {
  agentId?: string;
  message?: AgentMessage;
}

/**
 * AgentSendMessageResponse
 */
export interface AgentSendMessageResponse {
  messageId?: string;
  timestamp?: Date;
}

// =============================================================================
// Request/Response Messages - Health and Status
// =============================================================================

/**
 * AgentHealthCheckResponse
 */
export interface AgentHealthCheckResponse {
  status?: Status;
  version?: string;
  subsystems?: string[];
}

/**
 * AgentStatusResponse
 */
export interface AgentStatusResponse {
  status?: Status;
  state?: AgentState;
  activeTasks?: number;
  startedAt?: Date;
  uptime?: Date;
  resources?: ResourceUsage;
}

/**
 * CapabilitiesResponse
 */
export interface CapabilitiesResponse {
  capabilities?: AgentCapabilities;
}
