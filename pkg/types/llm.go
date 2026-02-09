package types

import "time"

// LLMProvider represents the LLM provider
type LLMProvider string

const (
	LLMProviderOpenAI    LLMProvider = "openai"
	LLMProviderAnthropic LLMProvider = "anthropic"
	LLMProviderAzure     LLMProvider = "azure"
	LLMProviderGoogle    LLMProvider = "google"
)

// LLMModel represents a specific model identifier
type LLMModel string

const (
	// OpenAI models
	LLMModelGPT4o          LLMModel = "gpt-4o"
	LLMModelGPT4oMini      LLMModel = "gpt-4o-mini"
	LLMModelGPT4Turbo      LLMModel = "gpt-4-turbo"
	LLMModelGPT35Turbo     LLMModel = "gpt-3.5-turbo"
	LLMModelO1Preview      LLMModel = "o1-preview"
	LLMModelO1Mini         LLMModel = "o1-mini"

	// Anthropic models
	LLMModelClaudeOpus     LLMModel = "claude-3-5-opus-20241022"
	LLMModelClaudeSonnet   LLMModel = "claude-3-5-sonnet-20241022"
	LLMModelClaudeHaiku    LLMModel = "claude-3-5-haiku-20241022"
)

// LLMMessageRole represents the role of a message in a conversation
type LLMMessageRole string

const (
	LLMMessageRoleSystem    LLMMessageRole = "system"
	LLMMessageRoleUser      LLMMessageRole = "user"
	LLMMessageRoleAssistant LLMMessageRole = "assistant"
)

// LLMMessage represents a message in a conversation with an LLM
type LLMMessage struct {
	Role    LLMMessageRole `json:"role"`
	Content string         `json:"content"`
}

// LLMTokenUsage represents token usage statistics for a request
type LLMTokenUsage struct {
	PromptTokens     int32 `json:"prompt_tokens"`
	CompletionTokens int32 `json:"completion_tokens"`
	TotalTokens      int32 `json:"total_tokens"`
}

// LLMRequest represents a request to an LLM provider
type LLMRequest struct {
	ID             ID             `json:"id"`
	Provider       LLMProvider    `json:"provider"`
	Model          LLMModel       `json:"model"`
	Messages       []LLMMessage   `json:"messages"`
	MaxTokens      *int32         `json:"max_tokens,omitempty"`
	Temperature    *float64       `json:"temperature,omitempty"`
	TopP           *float64       `json:"top_p,omitempty"`
	Stream         bool           `json:"stream"`
	StopSequences  []string       `json:"stop_sequences,omitempty"`
	Metadata       LLMMetadata    `json:"metadata,omitempty"`
	RequestedAt    Timestamp      `json:"requested_at"`
	SessionID      *ID            `json:"session_id,omitempty"`
	ContainerID    *ID            `json:"container_id,omitempty"`
}

// LLMMetadata contains additional information about an LLM request/response
type LLMMetadata struct {
	Labels      map[string]string `json:"labels,omitempty"`
	RequestType string            `json:"request_type,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// LLMResponse represents a response from an LLM provider
type LLMResponse struct {
	RequestID    ID              `json:"request_id"`
	Provider     LLMProvider     `json:"provider"`
	Model        LLMModel        `json:"model"`
	Content      string          `json:"content"`
	FinishReason string          `json:"finish_reason"`
	TokenUsage   LLMTokenUsage   `json:"token_usage"`
	Metadata     LLMMetadata     `json:"metadata,omitempty"`
	CompletedAt  Timestamp       `json:"completed_at"`
	Duration     time.Duration   `json:"duration"`
}

// LLMStreamChunk represents a chunk of a streaming response
type LLMStreamChunk struct {
	RequestID ID        `json:"request_id"`
	Chunk     string    `json:"chunk"`
	Done      bool      `json:"done"`
	Timestamp Timestamp `json:"timestamp"`
}

// LLMFinishReason represents why a completion finished
type LLMFinishReason string

const (
	LLMFinishReasonStop      LLMFinishReason = "stop"
	LLMFinishReasonLength    LLMFinishReason = "length"
	LLMFinishReasonContentFilter LLMFinishReason = "content_filter"
	LLMFinishReasonToolCalls LLMFinishReason = "tool_calls"
)

// LLMProviderConfig represents configuration for a specific LLM provider
type LLMProviderConfig struct {
	Provider      LLMProvider    `json:"provider"`
	APIEndpoint   string         `json:"api_endpoint,omitempty"`
	APIKey        string         `json:"api_key,omitempty"`
	Models        []LLMModel     `json:"models,omitempty"`
	Enabled       bool           `json:"enabled"`
	Priority      int            `json:"priority"` // Lower is higher priority for failover
	Timeout       time.Duration  `json:"timeout,omitempty"`
	MaxRetries    int            `json:"max_retries,omitempty"`
	RetryDelay    time.Duration  `json:"retry_delay,omitempty"`
}

// LLMConfig represents the overall LLM gateway configuration
type LLMConfig struct {
	DefaultProvider LLMProvider        `json:"default_provider"`
	Providers       []LLMProviderConfig `json:"providers"`
	FailoverEnabled bool               `json:"failover_enabled"`
	MaxConcurrent   int                `json:"max_concurrent,omitempty"`
	RequestTimeout  time.Duration      `json:"request_timeout,omitempty"`
}

// LLMStats represents statistics for LLM usage
type LLMStats struct {
	Provider          LLMProvider `json:"provider"`
	Model             LLMModel    `json:"model"`
	RequestCount      int64       `json:"request_count"`
	TokenCount        int64       `json:"token_count"`
	ErrorCount        int64       `json:"error_count"`
	AvgLatencyMs      int64       `json:"avg_latency_ms"`
	Timestamp         Timestamp   `json:"timestamp"`
}

// LLMError represents an error from an LLM provider
type LLMError struct {
	RequestID ID         `json:"request_id"`
	Provider  LLMProvider `json:"provider"`
	Code      string     `json:"code"`
	Message   string     `json:"message"`
	Retryable bool       `json:"retryable"`
	Timestamp Timestamp  `json:"timestamp"`
}

// LLMFilter defines filters for querying LLM requests
type LLMFilter struct {
	Provider   *LLMProvider     `json:"provider,omitempty"`
	SessionID  *ID              `json:"session_id,omitempty"`
	StartTime  *Timestamp       `json:"start_time,omitempty"`
	EndTime    *Timestamp       `json:"end_time,omitempty"`
	StatusCode *int             `json:"status_code,omitempty"`
}
