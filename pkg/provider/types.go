package provider

import (
	"context"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Provider represents an LLM provider (e.g., Anthropic, OpenAI)
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
)

// String returns the string representation of the provider
func (p Provider) String() string {
	return string(p)
}

// IsValid returns true if the provider is valid
func (p Provider) IsValid() bool {
	switch p {
	case ProviderAnthropic, ProviderOpenAI:
		return true
	default:
		return false
	}
}

// ProviderStatus represents the operational status of a provider
type ProviderStatus string

const (
	ProviderStatusUnknown     ProviderStatus = "unknown"
	ProviderStatusStarting    ProviderStatus = "starting"
	ProviderStatusAvailable   ProviderStatus = "available"
	ProviderStatusUnavailable ProviderStatus = "unavailable"
	ProviderStatusDegraded    ProviderStatus = "degraded"
	ProviderStatusError       ProviderStatus = "error"
)

// String returns the string representation of the status
func (s ProviderStatus) String() string {
	return string(s)
}

// IsAvailable returns true if the provider is available for requests
func (s ProviderStatus) IsAvailable() bool {
	switch s {
	case ProviderStatusAvailable, ProviderStatusDegraded:
		return true
	default:
		return false
	}
}

// Model represents a model identifier
type Model string

const (
	// Anthropic models
	ModelClaude3_5Sonnet      Model = "claude-3-5-sonnet-20241022"
	ModelClaude3_5SonnetNew   Model = "claude-3-5-sonnet-20240620"
	ModelClaude3Opus          Model = "claude-3-opus-20240229"
	ModelClaude3Sonnet        Model = "claude-3-sonnet-20240229"
	ModelClaude3Haiku         Model = "claude-3-haiku-20240307"

	// OpenAI models
	ModelGPT4o               Model = "gpt-4o"
	ModelGPT4oMini           Model = "gpt-4o-mini"
	ModelGPT4Turbo           Model = "gpt-4-turbo"
	ModelGPT4                Model = "gpt-4"
	ModelGPT35Turbo          Model = "gpt-3.5-turbo"
)

// String returns the string representation of the model
func (m Model) String() string {
	return string(m)
}

// IsEmpty returns true if the model is empty
func (m Model) IsEmpty() bool {
	return string(m) == ""
}

// ProviderFor returns the provider for this model
func (m Model) ProviderFor() Provider {
	switch {
	case isAnthropicModel(m):
		return ProviderAnthropic
	case isOpenAIModel(m):
		return ProviderOpenAI
	default:
		return ""
	}
}

// isAnthropicModel returns true if the model is an Anthropic model
func isAnthropicModel(m Model) bool {
	switch m {
	case ModelClaude3_5Sonnet,
		ModelClaude3_5SonnetNew,
		ModelClaude3Opus,
		ModelClaude3Sonnet,
		ModelClaude3Haiku:
		return true
	default:
		return false
	}
}

// isOpenAIModel returns true if the model is an OpenAI model
func isOpenAIModel(m Model) bool {
	switch m {
	case ModelGPT4o,
		ModelGPT4oMini,
		ModelGPT4Turbo,
		ModelGPT4,
		ModelGPT35Turbo:
		return true
	default:
		return false
	}
}

// ModelInfo contains metadata about a model
type ModelInfo struct {
	ID          Model             `json:"id"`
	Provider    Provider          `json:"provider"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	ContextSize int               `json:"context_size"` // Max context window in tokens
	Capabilities ModelCapabilities `json:"capabilities"`
}

// ModelCapabilities represents the capabilities of a model
type ModelCapabilities struct {
	Streaming        bool `json:"streaming"`
	FunctionCalling  bool `json:"function_calling"`
	Vision           bool `json:"vision"`
	PromptCaching    bool `json:"prompt_caching"`
	JSONMode         bool `json:"json_mode"`
	SystemMessages   bool `json:"system_messages"`
	Temperature      bool `json:"temperature"`
	TopP             bool `json:"top_p"`
	TopK             bool `json:"top_k"`
	MaxTokens        bool `json:"max_tokens"`
	StopSequences    bool `json:"stop_sequences"`
}

// LLMProvider defines the interface for LLM providers
type LLMProvider interface {
	// Provider returns the provider identifier
	Provider() Provider

	// Name returns the human-readable name of the provider
	Name() string

	// Status returns the current status of the provider
	Status() ProviderStatus

	// IsAvailable returns true if the provider is available
	IsAvailable() bool

	// Complete generates a completion for the given request
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

	// CompleteStream generates a streaming completion for the given request
	CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan *CompletionChunk, error)

	// SupportsModel returns true if the provider supports the given model
	SupportsModel(model Model) bool

	// ModelInfo returns information about the specified model
	ModelInfo(model Model) (*ModelInfo, error)

	// ListModels returns all available models for this provider
	ListModels() ([]Model, error)

	// Close closes the provider and releases resources
	Close() error
}

// CompletionRequest represents a request for a completion
// This is the unified request format that all providers implement
type CompletionRequest struct {
	// Model specifies the model to use
	Model Model `json:"model"`

	// Messages is the conversation history
	Messages []Message `json:"messages"`

	// MaxTokens specifies the maximum number of tokens to generate
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature controls randomness (0.0 to 1.0)
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP controls diversity via nucleus sampling (0.0 to 1.0)
	TopP *float64 `json:"top_p,omitempty"`

	// TopK controls diversity via top-k sampling
	TopK *int `json:"top_k,omitempty"`

	// StopSequences are sequences where the API will stop generating further tokens
	StopSequences []string `json:"stop_sequences,omitempty"`

	// Stream specifies whether to stream the response
	Stream bool `json:"stream,omitempty"`

	// Metadata contains additional request metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Message represents a message in the conversation
type Message struct {
	Role    MessageRole `json:"role"`
	Content interface{} `json:"content"` // string or []ContentBlock
}

// MessageRole represents the role of a message sender
type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

// String returns the string representation of the role
func (r MessageRole) String() string {
	return string(r)
}

// ContentBlock represents a block of content in a message
type ContentBlock struct {
	Type     ContentType           `json:"type"`
	Text     string                `json:"text,omitempty"`
	Source   *ImageSource          `json:"source,omitempty"`
	ToolUse  *ToolUseBlock         `json:"tool_use,omitempty"`
	ToolResult *ToolResultBlock    `json:"tool_result,omitempty"`
}

// ContentType represents the type of content block
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

// ImageSource represents an image in a message
type ImageSource struct {
	Type      string `json:"type"`      // "base64"
	MediaType string `json:"media_type"` // "image/png", "image/jpeg", etc.
	Data      string `json:"data"`      // base64 encoded image data
}

// ToolUseBlock represents a tool use request
type ToolUseBlock struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Input    map[string]interface{} `json:"input"`
}

// ToolResultBlock represents a tool use result
type ToolResultBlock struct {
	ToolUseID string      `json:"tool_use_id"`
	Content   interface{} `json:"content"`
	IsError   bool        `json:"is_error,omitempty"`
}

// CompletionResponse represents the response from a completion request
type CompletionResponse struct {
	// ID is the unique identifier for this completion
	ID string `json:"id"`

	// Model is the model that generated the completion
	Model Model `json:"model"`

	// Provider is the provider that generated the completion
	Provider Provider `json:"provider"`

	// Content is the generated content
	Content string `json:"content"`

	// StopReason is the reason the completion stopped
	StopReason StopReason `json:"stop_reason"`

	// Usage contains token usage information
	Usage TokenUsage `json:"usage"`

	// Metadata contains additional response metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// StopReason represents why a completion stopped
type StopReason string

const (
	StopReasonEndTurn      StopReason = "end_turn"
	StopReasonMaxTokens    StopReason = "max_tokens"
	StopReasonStopSequence StopReason = "stop_sequence"
	StopReasonToolUse      StopReason = "tool_use"
)

// String returns the string representation of the stop reason
func (s StopReason) String() string {
	return string(s)
}

// CompletionChunk represents a chunk of a streaming completion
type CompletionChunk struct {
	// ID is the unique identifier for this completion stream
	ID string `json:"id"`

	// Model is the model generating the completion
	Model Model `json:"model"`

	// Provider is the provider generating the completion
	Provider Provider `json:"provider"`

	// Content is the chunk of content
	Content string `json:"content,omitempty"`

	// Delta is the incremental text change
	Delta string `json:"delta,omitempty"`

	// StopReason is set on the final chunk
	StopReason *StopReason `json:"stop_reason,omitempty"`

	// Usage is set on the final chunk
	Usage *TokenUsage `json:"usage,omitempty"`

	// Error contains any error that occurred
	Error error `json:"error,omitempty"`

	// Done is true for the final chunk
	Done bool `json:"done,omitempty"`
}

// TokenUsage represents token usage for a completion
type TokenUsage struct {
	InputTokens      int `json:"input_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`  // Prompt caching (Anthropic)
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"` // Prompt caching (Anthropic)
	OutputTokens     int `json:"output_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Total returns the total tokens used
func (u *TokenUsage) Total() int {
	return u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens + u.OutputTokens
}

// ProviderConfig represents configuration for a provider
type ProviderConfig struct {
	// Provider is the provider identifier
	Provider Provider `json:"provider"`

	// APIKey is the API key for authentication
	APIKey string `json:"-"` // Don't serialize

	// BaseURL is the base URL for API requests (optional, for testing/custom endpoints)
	BaseURL string `json:"base_url,omitempty"`

	// Timeout is the request timeout
	Timeout int `json:"timeout,omitempty"` // seconds

	// MaxRetries is the maximum number of retries for failed requests
	MaxRetries int `json:"max_retries,omitempty"`

	// Models is the list of available models for this provider
	Models []Model `json:"models,omitempty"`

	// Enabled specifies whether this provider is enabled
	Enabled bool `json:"enabled"`

	// Priority specifies the priority of this provider for failover (lower = higher priority)
	Priority int `json:"priority,omitempty"`

	// Metadata contains additional provider configuration
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ProviderStats represents statistics about a provider
type ProviderStats struct {
	Provider         Provider       `json:"provider"`
	Status           ProviderStatus `json:"status"`
	TotalRequests    int64          `json:"total_requests"`
	SuccessRequests  int64          `json:"success_requests"`
	FailedRequests   int64          `json:"failed_requests"`
	TotalTokens      int64          `json:"total_tokens"`
	InputTokens      int64          `json:"input_tokens"`
	OutputTokens     int64          `json:"output_tokens"`
	CacheReadTokens  int64          `json:"cache_read_tokens"`
	CacheWriteTokens int64          `json:"cache_write_tokens"`
	AvgLatencyMs     int64          `json:"avg_latency_ms"`
	LastError        string         `json:"last_error,omitempty"`
}

// IsAvailable returns true if the provider has a healthy status
func (s *ProviderStats) IsAvailable() bool {
	return s.Status.IsAvailable()
}

// SuccessRate returns the success rate as a percentage (0-100)
func (s *ProviderStats) SuccessRate() float64 {
	if s.TotalRequests == 0 {
		return 0
	}
	return float64(s.SuccessRequests) / float64(s.TotalRequests) * 100
}

// Error codes specific to providers
const (
	ErrCodeProviderNotAvailable   = "PROVIDER_NOT_AVAILABLE"
	ErrCodeProviderNotFound       = "PROVIDER_NOT_FOUND"
	ErrCodeModelNotSupported      = "MODEL_NOT_SUPPORTED"
	ErrCodeModelNotFound          = "MODEL_NOT_FOUND"
	ErrCodeInvalidRequest         = "INVALID_REQUEST"
	ErrCodeAuthenticationFailed   = "AUTHENTICATION_FAILED"
	ErrCodeRateLimited            = "RATE_LIMITED"
	ErrCodeContextTooLong         = "CONTEXT_TOO_LONG"
	ErrCodeInvalidResponse        = "INVALID_RESPONSE"
	ErrCodeStreamError            = "STREAM_ERROR"
	ErrCodeProviderTimeout        = "PROVIDER_TIMEOUT"
	ErrCodeUpstreamError          = "UPSTREAM_ERROR"
)

// NewProviderError creates a new provider-specific error
func NewProviderError(code, message string) *types.Error {
	return types.NewError(code, message)
}

// WrapProviderError wraps an existing error with a provider error code
func WrapProviderError(code, message string, err error) *types.Error {
	return types.WrapError(code, message, err)
}
