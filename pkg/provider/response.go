package provider

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
