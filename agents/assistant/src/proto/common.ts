// common.ts - TypeScript types for common protocol buffer definitions
//
// This file contains TypeScript types shared across multiple gRPC services.
// Auto-generated from proto/common.proto
//
// Copyright 2026 baaaht project

// =============================================================================
// Common Enums
// =============================================================================

/**
 * Status represents the operational status of components
 */
export enum Status {
  STATUS_UNSPECIFIED = 0,
  STATUS_UNKNOWN = 1,
  STATUS_STARTING = 2,
  STATUS_RUNNING = 3,
  STATUS_STOPPING = 4,
  STATUS_STOPPED = 5,
  STATUS_ERROR = 6,
  STATUS_TERMINATED = 7,
}

/**
 * Health represents the health state of a component
 */
export enum Health {
  HEALTH_UNSPECIFIED = 0,
  HEALTH_UNKNOWN = 1,
  HEALTH_HEALTHY = 2,
  HEALTH_UNHEALTHY = 3,
  HEALTH_DEGRADED = 4,
  HEALTH_CHECKING = 5,
}

/**
 * Priority represents the priority level
 */
export enum Priority {
  PRIORITY_UNSPECIFIED = 0,
  PRIORITY_LOW = 1,
  PRIORITY_NORMAL = 2,
  PRIORITY_HIGH = 3,
  PRIORITY_CRITICAL = 4,
}

// =============================================================================
// Resource Types
// =============================================================================

/**
 * ResourceLimits defines resource constraints for a container/session
 */
export interface ResourceLimits {
  nanoCpus?: bigint;      // CPU in 1e-9 units
  memoryBytes?: bigint;   // Memory in bytes
  memorySwap?: bigint;    // Memory swap in bytes
  pidsLimit?: bigint;     // Max processes, optional
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

// =============================================================================
// LLM Types
// =============================================================================

/**
 * Provider represents the LLM provider
 */
export enum Provider {
  PROVIDER_UNSPECIFIED = 0,
  PROVIDER_ANTHROPIC = 1,
  PROVIDER_OPENAI = 2,
  PROVIDER_OPENROUTER = 3,
  PROVIDER_OLLAMA = 4,
  PROVIDER_LMSTUDIO = 5,
}

/**
 * ModelCapabilities describes the capabilities of a model
 */
export interface ModelCapabilities {
  streaming?: boolean;       // Supports streaming responses
  tools?: boolean;           // Supports function/tool calling
  vision?: boolean;          // Supports image/vision input
  thinking?: boolean;        // Supports extended thinking/reasoning
}

/**
 * TokenUsage represents token consumption for an LLM request
 */
export interface TokenUsage {
  inputTokens?: number;      // Tokens sent in the request
  outputTokens?: number;     // Tokens received in the response
  totalTokens?: number;      // Sum of input and output tokens
}
