package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
)

const (
	// Anthropic API version
	anthropicAPIVersion = "2023-06-01"

	// Anthropic API endpoints
	anthropicMessagesEndpoint = "/v1/messages"

	// Anthropic HTTP headers
	anthropicVersionHeader = "anthropic-version"
	anthropicKeyHeader     = "x-api-key"
)

// anthropicProvider implements the LLMProvider interface for Anthropic Claude
type anthropicProvider struct {
	config ProviderConfig
	client *http.Client
	logger *logger.Logger
	status ProviderStatus
	mu     sync.RWMutex
	closed bool
}

// NewAnthropicProvider creates a new Anthropic provider instance
func NewAnthropicProvider(config ProviderConfig, log *logger.Logger) (LLMProvider, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, WrapProviderError(ErrCodeProviderNotAvailable, "failed to create default logger", err)
		}
	}

	if config.APIKey == "" {
		return nil, NewProviderError(ErrCodeAuthenticationFailed, "Anthropic API key is required")
	}

	if config.BaseURL == "" {
		config.BaseURL = DefaultAnthropicBaseURL
	}

	if config.Timeout == 0 {
		config.Timeout = DefaultProviderTimeout
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(config.Timeout) * time.Second,
	}

	provider := &anthropicProvider{
		config: config,
		client: client,
		logger: log.With("component", "anthropic_provider", "provider", ProviderAnthropic),
		status: ProviderStatusAvailable,
		closed: false,
	}

	provider.logger.Info("Anthropic provider initialized",
		"base_url", config.BaseURL,
		"timeout", config.Timeout,
		"models", len(config.Models))

	return provider, nil
}

// Provider returns the provider identifier
func (p *anthropicProvider) Provider() Provider {
	return ProviderAnthropic
}

// Name returns the human-readable name of the provider
func (p *anthropicProvider) Name() string {
	if name, ok := p.config.Metadata["name"].(string); ok {
		return name
	}
	return "Anthropic"
}

// Status returns the current status of the provider
func (p *anthropicProvider) Status() ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// IsAvailable returns true if the provider is available
func (p *anthropicProvider) IsAvailable() bool {
	return p.Status().IsAvailable()
}

// Complete generates a completion for the given request
func (p *anthropicProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if p.closed {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider is closed")
	}

	if !p.IsAvailable() {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider is not available")
	}

	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Convert unified request to Anthropic format
	anthropicReq, err := p.convertRequest(req)
	if err != nil {
		return nil, WrapProviderError(ErrCodeInvalidRequest, "failed to convert request", err)
	}

	// Marshal request body
	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, WrapProviderError(ErrCodeInvalidRequest, "failed to marshal request", err)
	}

	// Create HTTP request
	url := p.config.BaseURL + anthropicMessagesEndpoint
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, WrapProviderError(ErrCodeProviderNotAvailable, "failed to create HTTP request", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(anthropicVersionHeader, anthropicAPIVersion)
	httpReq.Header.Set(anthropicKeyHeader, p.config.APIKey)

	p.logger.Debug("Sending Anthropic completion request",
		"model", req.Model,
		"messages", len(req.Messages),
		"max_tokens", req.MaxTokens)

	// Execute request
	resp, err := p.client.Do(httpReq)
	if err != nil {
		p.updateStatus(ProviderStatusError)
		return nil, WrapProviderError(ErrCodeUpstreamError, "HTTP request failed", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, WrapProviderError(ErrCodeInvalidResponse, "failed to read response body", err)
	}

	// Handle error responses
	if resp.StatusCode != http.StatusOK {
		return p.handleErrorResponse(resp.StatusCode, respBody)
	}

	// Parse response
	var anthropicResp anthropicMessageResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, WrapProviderError(ErrCodeInvalidResponse, "failed to unmarshal response", err)
	}

	// Convert to unified response
	completion := p.convertResponse(&anthropicResp, req.Model)

	// Update status to available on success
	p.updateStatus(ProviderStatusAvailable)

	p.logger.Debug("Anthropic completion successful",
		"model", completion.Model,
		"stop_reason", completion.StopReason,
		"input_tokens", completion.Usage.InputTokens,
		"output_tokens", completion.Usage.OutputTokens)

	return completion, nil
}

// CompleteStream generates a streaming completion for the given request
func (p *anthropicProvider) CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan *CompletionChunk, error) {
	if p.closed {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider is closed")
	}

	if !p.IsAvailable() {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider is not available")
	}

	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Streaming will be implemented in subtask-2-2
	return nil, NewProviderError(ErrCodeStreamError, "streaming not yet implemented")
}

// SupportsModel returns true if the provider supports the given model
func (p *anthropicProvider) SupportsModel(model Model) bool {
	for _, m := range p.config.Models {
		if m == model {
			return true
		}
	}
	return isAnthropicModel(model)
}

// ModelInfo returns information about the specified model
func (p *anthropicProvider) ModelInfo(model Model) (*ModelInfo, error) {
	if !p.SupportsModel(model) {
		return nil, NewProviderError(ErrCodeModelNotSupported,
			fmt.Sprintf("model %s is not supported by Anthropic provider", model))
	}

	info, ok := anthropicModelInfo[model]
	if !ok {
		// Return default model info for unknown Anthropic models
		return &ModelInfo{
			ID:          model,
			Provider:    ProviderAnthropic,
			Name:        string(model),
			Description: "Anthropic Claude model",
			ContextSize: 200000,
			Capabilities: ModelCapabilities{
				Streaming:       false, // Will be true after subtask-2-2
				FunctionCalling: true,
				Vision:          true,
				PromptCaching:   true,
				JSONMode:        false,
				SystemMessages:  true,
				Temperature:     true,
				TopP:            true,
				TopK:            true,
				MaxTokens:       true,
				StopSequences:   true,
			},
		}, nil
	}

	return info, nil
}

// ListModels returns all available models for this provider
func (p *anthropicProvider) ListModels() ([]Model, error) {
	if len(p.config.Models) > 0 {
		return p.config.Models, nil
	}

	// Return default Anthropic models if none configured
	return []Model{
		ModelClaude3_5Sonnet,
		ModelClaude3_5SonnetNew,
		ModelClaude3Opus,
		ModelClaude3Sonnet,
		ModelClaude3Haiku,
	}, nil
}

// Close closes the provider and releases resources
func (p *anthropicProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	p.status = ProviderStatusUnavailable

	p.logger.Info("Anthropic provider closed")
	return nil
}

// validateRequest validates the completion request
func (p *anthropicProvider) validateRequest(req *CompletionRequest) error {
	if req == nil {
		return NewProviderError(ErrCodeInvalidRequest, "request cannot be nil")
	}

	if req.Model.IsEmpty() {
		return NewProviderError(ErrCodeInvalidRequest, "model is required")
	}

	if !p.SupportsModel(req.Model) {
		return NewProviderError(ErrCodeModelNotSupported,
			fmt.Sprintf("model %s is not supported", req.Model))
	}

	if len(req.Messages) == 0 {
		return NewProviderError(ErrCodeInvalidRequest, "at least one message is required")
	}

	if req.MaxTokens <= 0 {
		return NewProviderError(ErrCodeInvalidRequest, "max_tokens must be positive")
	}

	return nil
}

// updateStatus updates the provider status
func (p *anthropicProvider) updateStatus(status ProviderStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
}

// handleErrorResponse handles error responses from the Anthropic API
func (p *anthropicProvider) handleErrorResponse(statusCode int, body []byte) (*CompletionResponse, error) {
	var errResp struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		p.updateStatus(ProviderStatusError)
		return nil, WrapProviderError(ErrCodeInvalidResponse,
			fmt.Sprintf("HTTP %d: failed to parse error response", statusCode), err)
	}

	// Map Anthropic error types to our error codes
	var errCode string
	switch statusCode {
	case http.StatusUnauthorized:
		errCode = ErrCodeAuthenticationFailed
		p.updateStatus(ProviderStatusUnavailable)
	case http.StatusTooManyRequests:
		errCode = ErrCodeRateLimited
	case http.StatusBadRequest:
		errCode = ErrCodeInvalidRequest
	case http.StatusRequestEntityTooLarge:
		errCode = ErrCodeContextTooLong
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		errCode = ErrCodeUpstreamError
		p.updateStatus(ProviderStatusError)
	default:
		errCode = ErrCodeUpstreamError
		p.updateStatus(ProviderStatusError)
	}

	p.logger.Error("Anthropic API error",
		"status_code", statusCode,
		"error_type", errResp.Error.Type,
		"message", errResp.Error.Message)

	return nil, NewProviderError(errCode, errResp.Error.Message)
}

// convertRequest converts a unified request to Anthropic format
func (p *anthropicProvider) convertRequest(req *CompletionRequest) (*anthropicMessageRequest, error) {
	// Convert messages
	messages := make([]anthropicMessage, 0)
	var systemMessage string

	for _, msg := range req.Messages {
		if msg.Role == MessageRoleSystem {
			// Anthropic expects system message separately
			if content, ok := msg.Content.(string); ok {
				systemMessage = content
			} else {
				// For complex content, serialize to JSON
				data, err := json.Marshal(msg.Content)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal system message: %w", err)
				}
				systemMessage = string(data)
			}
		} else {
			// Convert user/assistant messages
			anthropicMsg, err := p.convertMessage(msg)
			if err != nil {
				return nil, fmt.Errorf("failed to convert message: %w", err)
			}
			messages = append(messages, *anthropicMsg)
		}
	}

	anthropicReq := &anthropicMessageRequest{
		Model:     req.Model.String(),
		MaxTokens: req.MaxTokens,
		Messages:  messages,
	}

	if systemMessage != "" {
		anthropicReq.System = systemMessage
	}

	if req.Temperature != nil {
		anthropicReq.Temperature = req.Temperature
	}

	if req.TopP != nil {
		anthropicReq.TopP = req.TopP
	}

	if req.TopK != nil {
		anthropicReq.TopK = req.TopK
	}

	if len(req.StopSequences) > 0 {
		anthropicReq.StopSequences = req.StopSequences
	}

	return anthropicReq, nil
}

// convertMessage converts a unified message to Anthropic format
func (p *anthropicProvider) convertMessage(msg Message) (*anthropicMessage, error) {
	anthropicMsg := &anthropicMessage{
		Role: msg.Role.String(),
	}

	// Handle content (string or content blocks)
	switch content := msg.Content.(type) {
	case string:
		anthropicMsg.Content = content
	case []ContentBlock:
		blocks := make([]anthropicContentBlock, len(content))
		for i, block := range content {
			converted, err := p.convertContentBlock(block)
			if err != nil {
				return nil, err
			}
			blocks[i] = converted
		}
		anthropicMsg.Content = blocks
	default:
		// Try to serialize as JSON
		data, err := json.Marshal(content)
		if err != nil {
			return nil, fmt.Errorf("unsupported content type: %T", msg.Content)
		}
		anthropicMsg.Content = string(data)
	}

	return anthropicMsg, nil
}

// convertContentBlock converts a unified content block to Anthropic format
func (p *anthropicProvider) convertContentBlock(block ContentBlock) (anthropicContentBlock, error) {
	anthropicBlock := anthropicContentBlock{
		Type: string(block.Type),
	}

	switch block.Type {
	case ContentTypeText:
		anthropicBlock.Text = block.Text
	case ContentTypeImage:
		if block.Source == nil {
			return anthropicContentBlock{}, fmt.Errorf("image content block missing source")
		}
		anthropicBlock.Source = anthropicImageSource{
			Type:      block.Source.Type,
			MediaType: block.Source.MediaType,
			Data:      block.Source.Data,
		}
	default:
		// For unsupported content types, serialize as JSON
		data, err := json.Marshal(block)
		if err != nil {
			return anthropicContentBlock{}, fmt.Errorf("failed to serialize content block: %w", err)
		}
		anthropicBlock.Text = string(data)
	}

	return anthropicBlock, nil
}

// convertResponse converts an Anthropic response to unified format
func (p *anthropicProvider) convertResponse(resp *anthropicMessageResponse, model Model) *CompletionResponse {
	completion := &CompletionResponse{
		ID:       resp.ID,
		Model:    model,
		Provider: ProviderAnthropic,
		Usage: TokenUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}

	// Extract content from response
	if len(resp.Content) > 0 {
		var contentBuilder string
		for _, block := range resp.Content {
			if block.Type == "text" {
				contentBuilder += block.Text
			}
		}
		completion.Content = contentBuilder
	}

	// Map stop reason
	completion.StopReason = mapStopReason(resp.StopReason)

	// Add cache tokens if present
	completion.Usage.CacheReadTokens = resp.Usage.CacheReadInputTokens
	completion.Usage.CacheWriteTokens = resp.Usage.CacheCreationInputTokens
	completion.Usage.TotalTokens = completion.Usage.Total()

	return completion
}

// mapStopReason maps Anthropic stop reason to unified format
func mapStopReason(reason string) StopReason {
	switch reason {
	case "end_turn":
		return StopReasonEndTurn
	case "max_tokens":
		return StopReasonMaxTokens
	case "stop_sequence":
		return StopReasonStopSequence
	case "tool_use":
		return StopReasonToolUse
	default:
		return StopReasonEndTurn
	}
}

// Anthropic API request/response types

type anthropicMessageRequest struct {
	Model         string                      `json:"model"`
	MaxTokens     int                         `json:"max_tokens"`
	Messages      []anthropicMessage          `json:"messages"`
	System        string                      `json:"system,omitempty"`
	Temperature   *float64                    `json:"temperature,omitempty"`
	TopP          *float64                    `json:"top_p,omitempty"`
	TopK          *int                        `json:"top_k,omitempty"`
	StopSequences []string                    `json:"stop_sequences,omitempty"`
	Stream        bool                        `json:"stream,omitempty"`
	Metadata      *anthropicRequestMetadata   `json:"metadata,omitempty"`
}

type anthropicMessage struct {
	Role    string                `json:"role"`
	Content interface{}           `json:"content"` // string or []anthropicContentBlock
}

type anthropicContentBlock struct {
	Type   string             `json:"type"`
	Text   string             `json:"text,omitempty"`
	Source anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicRequestMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type anthropicMessageResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []anthropicContentBlock `json:"content"`
	StopReason   string                  `json:"stop_reason"`
	Usage        anthropicUsage          `json:"usage"`
	Model        string                  `json:"model"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens"`
}

// Model information for Anthropic models
var anthropicModelInfo = map[Model]*ModelInfo{
	ModelClaude3_5Sonnet: {
		ID:          ModelClaude3_5Sonnet,
		Provider:    ProviderAnthropic,
		Name:        "Claude 3.5 Sonnet (Latest)",
		Description: "Most capable model for complex tasks, with 200K context window",
		ContextSize: 200000,
		Capabilities: ModelCapabilities{
			Streaming:       false, // Will be true after subtask-2-2
			FunctionCalling: true,
			Vision:          true,
			PromptCaching:   true,
			JSONMode:        false,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            true,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
	ModelClaude3_5SonnetNew: {
		ID:          ModelClaude3_5SonnetNew,
		Provider:    ProviderAnthropic,
		Name:        "Claude 3.5 Sonnet",
		Description: "Most capable model for complex tasks, with 200K context window",
		ContextSize: 200000,
		Capabilities: ModelCapabilities{
			Streaming:       false, // Will be true after subtask-2-2
			FunctionCalling: true,
			Vision:          true,
			PromptCaching:   true,
			JSONMode:        false,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            true,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
	ModelClaude3Opus: {
		ID:          ModelClaude3Opus,
		Provider:    ProviderAnthropic,
		Name:        "Claude 3 Opus",
		Description: "Most powerful model for complex reasoning, with 200K context window",
		ContextSize: 200000,
		Capabilities: ModelCapabilities{
			Streaming:       false, // Will be true after subtask-2-2
			FunctionCalling: true,
			Vision:          true,
			PromptCaching:   false,
			JSONMode:        false,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            true,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
	ModelClaude3Sonnet: {
		ID:          ModelClaude3Sonnet,
		Provider:    ProviderAnthropic,
		Name:        "Claude 3 Sonnet",
		Description: "Balanced model for many tasks, with 200K context window",
		ContextSize: 200000,
		Capabilities: ModelCapabilities{
			Streaming:       false, // Will be true after subtask-2-2
			FunctionCalling: true,
			Vision:          true,
			PromptCaching:   false,
			JSONMode:        false,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            true,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
	ModelClaude3Haiku: {
		ID:          ModelClaude3Haiku,
		Provider:    ProviderAnthropic,
		Name:        "Claude 3 Haiku",
		Description: "Fastest model for simple tasks, with 200K context window",
		ContextSize: 200000,
		Capabilities: ModelCapabilities{
			Streaming:       false, // Will be true after subtask-2-2
			FunctionCalling: true,
			Vision:          true,
			PromptCaching:   false,
			JSONMode:        false,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            true,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
}
