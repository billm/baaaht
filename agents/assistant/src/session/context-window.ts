// context-window.ts - Context window management for conversation history
//
// This file contains the context window manager that handles token limits
// and message pruning for efficient LLM context management.
//
// Copyright 2026 baaaht project

import type { Message } from './types.js';

// =============================================================================
// Constants
// =============================================================================

/**
 * Default token count estimates (approximate)
 * These are rough estimates for common scenarios
 */
const DEFAULT_TOKENS_PER_CHAR = 0.25; // ~4 characters per token
const DEFAULT_TOKENS_PER_WORD = 0.75; // ~1.33 words per token
const MIN_TOKENS_PER_MESSAGE = 10; // Minimum tokens for any message (metadata overhead)

/**
 * Default context window sizes for different models (in tokens)
 */
const DEFAULT_CONTEXT_SIZES = {
  claude_opus: 200000,
  claude_sonnet_4: 200000,
  claude_haiku: 200000,
  gpt_4: 8192,
  gpt_4_turbo: 128000,
  gpt_4o: 128000,
  default: 200000,
} as const;

/**
 * Default reserved tokens for system prompts and responses
 */
const DEFAULT_RESERVED_TOKENS = 4000; // Reserve space for system prompt + response

/**
 * Default pruning threshold - prune when exceeding this percentage of context
 */
const DEFAULT_PRUNING_THRESHOLD = 0.9; // Prune at 90% of context

// =============================================================================
// Type Definitions
// =============================================================================

/**
 * PruningStrategy determines how messages are pruned when context is exceeded
 */
export enum PruningStrategy {
  /**
   * Keep first N and last M messages (preserve conversation start and recent)
   */
  FIRST_AND_LAST = 'first_and_last',

  /**
   * Keep only the most recent messages (FIFO)
   */
  RECENT_ONLY = 'recent_only',

  /**
   * Keep system messages + most recent messages
   */
  SYSTEM_AND_RECENT = 'system_and_recent',

  /**
   * Keep messages with importance scores (metadata-based)
   */
  IMPORTANCE_BASED = 'importance_based',
}

/**
 * ContextWindowConfig configures the context window manager
 */
export interface ContextWindowConfig {
  /**
   * Maximum context window size in tokens
   * If not provided, will use the model's default context size
   */
  maxTokens?: number;

  /**
   * Reserved tokens for system prompts and responses
   * Default: 4000
   */
  reservedTokens?: number;

  /**
   * Pruning strategy to use when context is exceeded
   * Default: SYSTEM_AND_RECENT
   */
  pruningStrategy?: PruningStrategy;

  /**
   * Threshold at which to trigger pruning (0-1)
   * Default: 0.9 (90%)
   */
  pruningThreshold?: number;

  /**
   * Number of messages to keep when using FIRST_AND_LAST strategy
   * Format: {first: N, last: M}
   * Default: {first: 2, last: 10}
   */
  firstAndLastCount?: { first: number; last: number };

  /**
   * Whether to count tokens exactly (slower) or estimate (faster)
   * Default: false (estimate)
   */
  exactTokenCount?: boolean;

  /**
   * Custom token counter function
   * If not provided, uses built-in estimation
   */
  tokenCounter?: (text: string) => number;
}

/**
 * ContextWindowResult contains the result of a context window operation
 */
export interface ContextWindowResult {
  /**
   * Messages that fit within the context window
   */
  messages: Message[];

  /**
   * Total token count of the included messages
   */
  tokenCount: number;

  /**
   * Number of messages that were pruned
   */
  prunedCount: number;

  /**
   * Estimated remaining tokens available
   */
  remainingTokens: number;

  /**
   * Whether pruning was performed
   */
  wasPruned: boolean;
}

/**
 * TokenUsage contains token usage information
 */
export interface TokenUsage {
  /**
   * Total tokens used by messages
   */
  inputTokens: number;

  /**
   * Maximum tokens available for input
   */
  maxInputTokens: number;

  /**
   * Reserved tokens (system prompt + response)
   */
  reservedTokens: number;

  /**
   * Percentage of context used (0-1)
   */
  usagePercent: number;

  /**
   * Whether pruning is recommended
   */
  pruningRecommended: boolean;
}

// =============================================================================
// Error Types
// =============================================================================

/**
 * Error codes for context window operations
 */
export enum ContextWindowErrorCode {
  INVALID_CONFIG = 'INVALID_CONFIG',
  INVALID_MESSAGES = 'INVALID_MESSAGES',
  TOKEN_LIMIT_EXCEEDED = 'TOKEN_LIMIT_EXCEEDED',
  PRUNING_FAILED = 'PRUNING_FAILED',
}

/**
 * Error thrown when context window operation fails
 */
export class ContextWindowError extends Error {
  constructor(
    message: string,
    public code: ContextWindowErrorCode,
    public details?: string
  ) {
    super(message);
    this.name = 'ContextWindowError';
  }
}

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Estimate token count for a text string
 * Uses character-based estimation (faster but less accurate)
 */
export function estimateTokens(text: string): number {
  if (!text || text.length === 0) {
    return 0;
  }
  // Use character count as a rough estimate
  // Average English text is ~4 characters per token
  return Math.ceil(text.length * DEFAULT_TOKENS_PER_CHAR);
}

/**
 * Estimate token count for a message
 * Includes both content and metadata overhead
 */
export function estimateMessageTokens(message: Message): number {
  const contentTokens = estimateTokens(message.content);
  // Add overhead for role, metadata, etc.
  const metadataTokens = estimateTokens(
    JSON.stringify({ role: message.role, metadata: message.metadata })
  );
  return Math.max(MIN_TOKENS_PER_MESSAGE, contentTokens + metadataTokens);
}

/**
 * Calculate total token count for an array of messages
 */
export function calculateTotalTokens(messages: Message[]): number {
  return messages.reduce((sum, msg) => sum + estimateMessageTokens(msg), 0);
}

/**
 * Get default context size for a model
 */
export function getDefaultContextSize(model?: string): number {
  if (!model) {
    return DEFAULT_CONTEXT_SIZES.default;
  }

  // Match model name to context size
  const normalizedModel = model.toLowerCase();

  if (normalizedModel.includes('opus') || normalizedModel.includes('claude-3-opus')) {
    return DEFAULT_CONTEXT_SIZES.claude_opus;
  }
  if (normalizedModel.includes('sonnet') || normalizedModel.includes('claude-3.5-sonnet')) {
    return DEFAULT_CONTEXT_SIZES.claude_sonnet_4;
  }
  if (normalizedModel.includes('haiku') || normalizedModel.includes('claude-3-haiku')) {
    return DEFAULT_CONTEXT_SIZES.claude_haiku;
  }
  if (normalizedModel.includes('gpt-4') || normalizedModel.includes('gpt_4')) {
    if (normalizedModel.includes('turbo') || normalizedModel.includes('32k')) {
      return DEFAULT_CONTEXT_SIZES.gpt_4_turbo;
    }
    if (normalizedModel.includes('preview') || normalizedModel.includes('vision')) {
      return DEFAULT_CONTEXT_SIZES.gpt_4o;
    }
    return DEFAULT_CONTEXT_SIZES.gpt_4;
  }

  return DEFAULT_CONTEXT_SIZES.default;
}

/**
 * Create a deep copy of a message to prevent external modification
 */
function copyMessage(message: Message): Message {
  return {
    ...message,
    metadata: message.metadata ? { ...message.metadata, extra: message.metadata.extra ? { ...message.metadata.extra } : undefined } : undefined,
  };
}

/**
 * Create deep copies of messages to prevent external modification
 */
function copyMessages(messages: Message[]): Message[] {
  return messages.map(copyMessage);
}

// =============================================================================
// Context Window Manager
// =============================================================================

/**
 * ContextWindow manages message context within token limits
 */
export class ContextWindow {
  private config: Required<ContextWindowConfig>;
  private maxInputTokens: number;

  constructor(config: ContextWindowConfig = {}) {
    // Apply defaults
    const modelContextSize = getDefaultContextSize();

    this.config = {
      maxTokens: config.maxTokens ?? modelContextSize,
      reservedTokens: config.reservedTokens ?? DEFAULT_RESERVED_TOKENS,
      pruningStrategy: config.pruningStrategy ?? PruningStrategy.SYSTEM_AND_RECENT,
      pruningThreshold: config.pruningThreshold ?? DEFAULT_PRUNING_THRESHOLD,
      firstAndLastCount: config.firstAndLastCount ?? { first: 2, last: 10 },
      exactTokenCount: config.exactTokenCount ?? false,
      tokenCounter: config.tokenCounter ?? estimateTokens,
    };

    // Calculate maximum input tokens (total context - reserved)
    this.maxInputTokens = Math.max(
      0,
      this.config.maxTokens - this.config.reservedTokens
    );

    // Validate configuration
    this.validateConfig();
  }

  // ==========================================================================
  // Public Methods
  // ==========================================================================

  /**
   * FitMessages prunes messages to fit within the context window
   */
  fitMessages(messages: Message[]): ContextWindowResult {
    // Validate input
    if (!messages || messages.length === 0) {
      return {
        messages: [],
        tokenCount: 0,
        prunedCount: 0,
        remainingTokens: this.maxInputTokens,
        wasPruned: false,
      };
    }

    // Make a deep copy to prevent external modification
    const messagesCopy = copyMessages(messages);

    // Calculate initial token count
    const initialTokenCount = this.countTokens(messagesCopy);

    // Check if pruning is needed
    const thresholdTokens = this.maxInputTokens * this.config.pruningThreshold;
    const needsPruning = initialTokenCount > thresholdTokens;

    if (!needsPruning) {
      return {
        messages: messagesCopy,
        tokenCount: initialTokenCount,
        prunedCount: 0,
        remainingTokens: this.maxInputTokens - initialTokenCount,
        wasPruned: false,
      };
    }

    // Prune messages
    const prunedMessages = this.pruneMessages(messagesCopy);
    const finalTokenCount = this.countTokens(prunedMessages);

    return {
      messages: prunedMessages,
      tokenCount: finalTokenCount,
      prunedCount: messagesCopy.length - prunedMessages.length,
      remainingTokens: this.maxInputTokens - finalTokenCount,
      wasPruned: true,
    };
  }

  /**
   * CanAddMessages checks if new messages can be added without exceeding context
   */
  canAddMessages(existingMessages: Message[], newMessages: Message[]): boolean {
    const currentTokens = this.countTokens(existingMessages);
    const newTokens = this.countTokens(newMessages);
    return (currentTokens + newTokens) <= this.maxInputTokens;
  }

  /**
   * GetTokenUsage returns token usage information for messages
   */
  getTokenUsage(messages: Message[]): TokenUsage {
    const inputTokens = this.countTokens(messages);
    const usagePercent = inputTokens / this.maxInputTokens;
    const pruningThreshold = this.maxInputTokens * this.config.pruningThreshold;

    return {
      inputTokens,
      maxInputTokens: this.maxInputTokens,
      reservedTokens: this.config.reservedTokens,
      usagePercent,
      pruningRecommended: inputTokens > pruningThreshold,
    };
  }

  /**
   * GetMaxInputTokens returns the maximum tokens available for input messages
   */
  getMaxInputTokens(): number {
    return this.maxInputTokens;
  }

  /**
   * GetMaxTokens returns the total context window size
   */
  getMaxTokens(): number {
    return this.config.maxTokens;
  }

  // ==========================================================================
  // Private Methods
  // ==========================================================================

  /**
   * Count tokens for messages using configured token counter
   */
  private countTokens(messages: Message[]): number {
    if (this.config.exactTokenCount && this.config.tokenCounter) {
      // Use exact token counting
      return messages.reduce((sum, msg) => {
        return sum + this.config.tokenCounter!(msg.content);
      }, 0);
    }

    // Use estimation (faster)
    return calculateTotalTokens(messages);
  }

  /**
   * Prune messages based on the configured strategy
   */
  private pruneMessages(messages: Message[]): Message[] {
    switch (this.config.pruningStrategy) {
      case PruningStrategy.FIRST_AND_LAST:
        return this.pruneFirstAndLast(messages);
      case PruningStrategy.RECENT_ONLY:
        return this.pruneRecentOnly(messages);
      case PruningStrategy.SYSTEM_AND_RECENT:
        return this.pruneSystemAndRecent(messages);
      case PruningStrategy.IMPORTANCE_BASED:
        return this.pruneByImportance(messages);
      default:
        return this.pruneSystemAndRecent(messages);
    }
  }

  /**
   * Prune keeping first N and last M messages
   */
  private pruneFirstAndLast(messages: Message[]): Message[] {
    const { first, last } = this.config.firstAndLastCount;
    const result: Message[] = [];

    // Add first N messages
    const firstCount = Math.min(first, messages.length);
    for (let i = 0; i < firstCount; i++) {
      const msg = messages[i];
      result.push(msg);
    }

    // Add last M messages (if different from first)
    const lastCount = Math.min(last, messages.length - firstCount);
    const startIndex = Math.max(firstCount, messages.length - lastCount);
    for (let i = startIndex; i < messages.length; i++) {
      const msg = messages[i];
      result.push(msg);
    }

    // If still over limit, trim from the middle
    return this.fitToLimit(result);
  }

  /**
   * Prune keeping only recent messages (FIFO)
   */
  private pruneRecentOnly(messages: Message[]): Message[] {
    return this.fitToLimit(messages);
  }

  /**
   * Prune keeping system messages and recent messages
   */
  private pruneSystemAndRecent(messages: Message[]): Message[] {
    const systemMessages: Message[] = [];
    const otherMessages: Message[] = [];

    // Separate system messages from others
    for (const msg of messages) {
      if (msg.role === 'system') {
        systemMessages.push(msg);
      } else {
        otherMessages.push(msg);
      }
    }

    // Start with system messages
    let result = [...systemMessages];
    let currentTokens = this.countTokens(result);

    // Add non-system messages from most recent until we hit the limit
    for (let i = otherMessages.length - 1; i >= 0; i--) {
      const msg = otherMessages[i];
      const msgTokens = this.countTokens([msg]);

      if (currentTokens + msgTokens > this.maxInputTokens) {
        break;
      }

      result.unshift(msg); // Add to beginning to maintain order
      currentTokens += msgTokens;
    }

    return result;
  }

  /**
   * Prune based on message importance scores
   */
  private pruneByImportance(messages: Message[]): Message[] {
    // Sort messages by importance (metadata.importance if present)
    const sorted = [...messages].sort((a, b) => {
      const aImportance = a.metadata?.extra?.importance ?? 0;
      const bImportance = b.metadata?.extra?.importance ?? 0;
      return bImportance - aImportance; // Descending order
    });

    const result: Message[] = [];
    let currentTokens = 0;

    // Keep system messages always
    for (const msg of sorted) {
      if (msg.role === 'system') {
        result.push(msg);
        currentTokens += this.countTokens([msg]);
      }
    }

    // Add high-importance messages until we hit the limit
    for (const msg of sorted) {
      if (msg.role === 'system') {
        continue; // Already added
      }

      const msgTokens = this.countTokens([msg]);

      if (currentTokens + msgTokens > this.maxInputTokens) {
        break;
      }

      result.push(msg);
      currentTokens += msgTokens;
    }

    // Sort by timestamp to maintain chronological order
    result.sort((a, b) => a.timestamp.getTime() - b.timestamp.getTime());

    return result;
  }

  /**
   * Fit messages to the exact token limit by trimming from the beginning
   */
  private fitToLimit(messages: Message[]): Message[] {
    const result: Message[] = [];
    let currentTokens = 0;

    // Process from most recent to oldest
    for (let i = messages.length - 1; i >= 0; i--) {
      const msg = messages[i];
      const msgTokens = this.countTokens([msg]);

      if (currentTokens + msgTokens > this.maxInputTokens) {
        break;
      }

      result.unshift(msg);
      currentTokens += msgTokens;
    }

    return result;
  }

  /**
   * Validate the configuration
   */
  private validateConfig(): void {
    if (this.config.maxTokens <= 0) {
      throw new ContextWindowError(
        'maxTokens must be positive',
        ContextWindowErrorCode.INVALID_CONFIG
      );
    }

    if (this.config.reservedTokens < 0) {
      throw new ContextWindowError(
        'reservedTokens must be non-negative',
        ContextWindowErrorCode.INVALID_CONFIG
      );
    }

    if (this.config.reservedTokens >= this.config.maxTokens) {
      throw new ContextWindowError(
        'reservedTokens must be less than maxTokens',
        ContextWindowErrorCode.INVALID_CONFIG
      );
    }

    if (this.config.pruningThreshold <= 0 || this.config.pruningThreshold > 1) {
      throw new ContextWindowError(
        'pruningThreshold must be between 0 and 1',
        ContextWindowErrorCode.INVALID_CONFIG
      );
    }

    if (this.config.firstAndLastCount.first < 0 || this.config.firstAndLastCount.last < 0) {
      throw new ContextWindowError(
        'firstAndLastCount values must be non-negative',
        ContextWindowErrorCode.INVALID_CONFIG
      );
    }
  }
}

// =============================================================================
// Factory Functions
// =============================================================================

/**
 * Create a context window manager for a specific model
 */
export function createContextWindow(
  model?: string,
  config?: Partial<ContextWindowConfig>
): ContextWindow {
  const maxTokens = getDefaultContextSize(model);
  return new ContextWindow({
    ...config,
    maxTokens: config?.maxTokens ?? maxTokens,
  });
}

/**
 * Create a default context window manager
 */
export function createDefaultContextWindow(): ContextWindow {
  return new ContextWindow();
}
