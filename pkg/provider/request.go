package provider

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
	Type       ContentType        `json:"type"`
	Text       string             `json:"text,omitempty"`
	Source     *ImageSource       `json:"source,omitempty"`
	ToolUse    *ToolUseBlock      `json:"tool_use,omitempty"`
	ToolResult *ToolResultBlock   `json:"tool_result,omitempty"`
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
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", "image/jpeg", etc.
	Data      string `json:"data"`       // base64 encoded image data
}

// ToolUseBlock represents a tool use request
type ToolUseBlock struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ToolResultBlock represents a tool use result
type ToolResultBlock struct {
	ToolUseID string      `json:"tool_use_id"`
	Content   interface{} `json:"content"`
	IsError   bool        `json:"is_error,omitempty"`
}
