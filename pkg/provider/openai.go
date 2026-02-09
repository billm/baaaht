package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
)

const (
	// OpenAI API endpoints
	openaiChatCompletionsEndpoint = "/v1/chat/completions"

	// OpenAI HTTP headers
	openaiAuthHeader     = "Authorization"
	openaiAuthBearerType = "Bearer"
)

// openaiProvider implements the LLMProvider interface for OpenAI
type openaiProvider struct {
	config       ProviderConfig
	client       *http.Client
	logger       *logger.Logger
	status       ProviderStatus
	mu           sync.RWMutex
	closed       bool
	tokenAccount *TokenAccount
}

// NewOpenAIProvider creates a new OpenAI provider instance
func NewOpenAIProvider(config ProviderConfig, log *logger.Logger) (LLMProvider, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, WrapProviderError(ErrCodeProviderNotAvailable, "failed to create default logger", err)
		}
	}

	if config.APIKey == "" {
		return nil, NewProviderError(ErrCodeAuthenticationFailed, "OpenAI API key is required")
	}

	if config.BaseURL == "" {
		config.BaseURL = DefaultOpenAIBaseURL
	}

	if config.Timeout == 0 {
		config.Timeout = DefaultProviderTimeout
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(config.Timeout) * time.Second,
	}

	provider := &openaiProvider{
		config:       config,
		client:       client,
		logger:       log.With("component", "openai_provider", "provider", ProviderOpenAI),
		status:       ProviderStatusAvailable,
		closed:       false,
		tokenAccount: NewTokenAccount(ProviderOpenAI),
	}

	provider.logger.Info("OpenAI provider initialized",
		"base_url", config.BaseURL,
		"timeout", config.Timeout,
		"models", len(config.Models))

	return provider, nil
}

// Provider returns the provider identifier
func (p *openaiProvider) Provider() Provider {
	return ProviderOpenAI
}

// Name returns the human-readable name of the provider
func (p *openaiProvider) Name() string {
	if name, ok := p.config.Metadata["name"].(string); ok {
		return name
	}
	return "OpenAI"
}

// Status returns the current status of the provider
func (p *openaiProvider) Status() ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// IsAvailable returns true if the provider is available
func (p *openaiProvider) IsAvailable() bool {
	return p.Status().IsAvailable()
}

// Complete generates a completion for the given request
func (p *openaiProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if p.closed {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider is closed")
	}

	if !p.IsAvailable() {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider is not available")
	}

	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Convert unified request to OpenAI format
	openaiReq, err := p.convertRequest(req)
	if err != nil {
		return nil, WrapProviderError(ErrCodeInvalidRequest, "failed to convert request", err)
	}

	// Marshal request body
	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, WrapProviderError(ErrCodeInvalidRequest, "failed to marshal request", err)
	}

	// Create HTTP request
	url := p.config.BaseURL + openaiChatCompletionsEndpoint
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, WrapProviderError(ErrCodeProviderNotAvailable, "failed to create HTTP request", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(openaiAuthHeader, openaiAuthBearerType+" "+p.config.APIKey)

	p.logger.Debug("Sending OpenAI completion request",
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
	var openaiResp openaiChatCompletionResponse
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		return nil, WrapProviderError(ErrCodeInvalidResponse, "failed to unmarshal response", err)
	}

	// Convert to unified response
	completion := p.convertResponse(&openaiResp, req.Model)

	// Update status to available on success
	p.updateStatus(ProviderStatusAvailable)

	// Record token usage
	p.tokenAccount.Record(completion.ID, req.Model, completion.Usage)

	p.logger.Debug("OpenAI completion successful",
		"model", completion.Model,
		"stop_reason", completion.StopReason,
		"input_tokens", completion.Usage.InputTokens,
		"output_tokens", completion.Usage.OutputTokens)

	return completion, nil
}

// CompleteStream generates a streaming completion for the given request
func (p *openaiProvider) CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan *CompletionChunk, error) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider is closed")
	}

	if !p.IsAvailable() {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider is not available")
	}

	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Convert unified request to OpenAI format
	openaiReq, err := p.convertRequest(req)
	if err != nil {
		return nil, WrapProviderError(ErrCodeInvalidRequest, "failed to convert request", err)
	}

	// Enable streaming
	openaiReq.Stream = true

	// Marshal request body
	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, WrapProviderError(ErrCodeInvalidRequest, "failed to marshal request", err)
	}

	// Create HTTP request
	url := p.config.BaseURL + openaiChatCompletionsEndpoint
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, WrapProviderError(ErrCodeProviderNotAvailable, "failed to create HTTP request", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(openaiAuthHeader, openaiAuthBearerType+" "+p.config.APIKey)

	p.logger.Debug("Sending OpenAI streaming request",
		"model", req.Model,
		"messages", len(req.Messages),
		"max_tokens", req.MaxTokens)

	// Create streaming client without timeout (context manages cancellation)
	streamingClient := &http.Client{
		Timeout: 0, // No timeout for streaming, context handles cancellation
	}

	// Execute request
	resp, err := streamingClient.Do(httpReq)
	if err != nil {
		p.updateStatus(ProviderStatusError)
		return nil, WrapProviderError(ErrCodeUpstreamError, "HTTP request failed", err)
	}

	// Handle error responses
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)

		// Parse error response to get the message
		var errResp openaiErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			// Map status code to error code
			var errCode string
			switch resp.StatusCode {
			case http.StatusUnauthorized:
				errCode = ErrCodeAuthenticationFailed
			case http.StatusTooManyRequests:
				errCode = ErrCodeRateLimited
			case http.StatusBadRequest:
				errCode = ErrCodeInvalidRequest
			case http.StatusRequestEntityTooLarge:
				errCode = ErrCodeContextTooLong
			default:
				errCode = ErrCodeUpstreamError
			}
			return nil, NewProviderError(errCode, errResp.Error.Message)
		}

		// Fallback error if we can't parse the response
		return nil, NewProviderError(ErrCodeUpstreamError, fmt.Sprintf("HTTP %d: streaming request failed", resp.StatusCode))
	}

	// Create chunk channel
	chunkChan := make(chan *CompletionChunk, 10)

	// Start goroutine to process stream
	go p.processStreamingResponse(ctx, resp, req.Model, chunkChan)

	return chunkChan, nil
}

// processStreamingResponse processes the SSE streaming response from OpenAI
func (p *openaiProvider) processStreamingResponse(ctx context.Context, resp *http.Response, model Model, chunkChan chan<- *CompletionChunk) {
	defer close(chunkChan)
	defer resp.Body.Close()

	// Update status to available on successful connection
	p.updateStatus(ProviderStatusAvailable)

	// Create SSE reader
	sseReader := newOpenAISSEReader(resp.Body)

	var messageID string
	var contentBuilder strings.Builder
	var finishReason *string
	streamDone := false

	// Process events until stream ends or context is cancelled
	for {
		select {
		case <-ctx.Done():
			// Context cancelled, send error chunk and stop
			if !streamDone {
				chunkChan <- &CompletionChunk{
					Model:    model,
					Provider: ProviderOpenAI,
					Error:    ctx.Err(),
				}
			}
			p.logger.Debug("Streaming cancelled by context")
			return
		default:
			chunk, err := sseReader.NextChunk()
			if err != nil {
				if err == io.EOF {
					// Stream ended normally
					p.logger.Debug("Streaming completed normally")
					return
				}
				// Error reading stream
				if !streamDone {
					chunkChan <- &CompletionChunk{
						Model:    model,
						Provider: ProviderOpenAI,
						Error:    WrapProviderError(ErrCodeStreamError, "failed to read stream", err),
					}
					p.updateStatus(ProviderStatusError)
				}
				p.logger.Error("Failed to read streaming event", "error", err)
				return
			}

			// Don't process more chunks after finish reason
			if streamDone {
				continue
			}

			// Process the chunk
			if chunk.ID != "" {
				messageID = chunk.ID
			}

			// Extract delta content if present
			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				delta := choice.Delta

				// Append content
				if delta.Content != "" {
					contentBuilder.WriteString(delta.Content)
					chunkChan <- &CompletionChunk{
						ID:       messageID,
						Model:    model,
						Provider: ProviderOpenAI,
						Delta:    delta.Content,
						Content:  contentBuilder.String(),
					}
				}

				// Check for finish reason
				if choice.FinishReason != nil {
					finishReason = choice.FinishReason
					stopReason := p.mapStopReason(*finishReason)
					chunkChan <- &CompletionChunk{
						ID:       messageID,
						Model:    model,
						Provider: ProviderOpenAI,
						Content:  contentBuilder.String(),
						Done:     true,
						StopReason: &stopReason,
					}
					streamDone = true
				}
			}
		}
	}
}

// SupportsModel returns true if the provider supports the given model
func (p *openaiProvider) SupportsModel(model Model) bool {
	for _, m := range p.config.Models {
		if m == model {
			return true
		}
	}
	return isOpenAIModel(model)
}

// ModelInfo returns information about the specified model
func (p *openaiProvider) ModelInfo(model Model) (*ModelInfo, error) {
	if !p.SupportsModel(model) {
		return nil, NewProviderError(ErrCodeModelNotSupported,
			fmt.Sprintf("model %s is not supported by OpenAI provider", model))
	}

	info, ok := openaiModelInfo[model]
	if !ok {
		// Return default model info for unknown OpenAI models
		return &ModelInfo{
			ID:          model,
			Provider:    ProviderOpenAI,
			Name:        string(model),
			Description: "OpenAI GPT model",
			ContextSize: 128000,
			Capabilities: ModelCapabilities{
				Streaming:       true,
				FunctionCalling: true,
				Vision:          true,
				PromptCaching:   false,
				JSONMode:        true,
				SystemMessages:  true,
				Temperature:     true,
				TopP:            true,
				TopK:            false,
				MaxTokens:       true,
				StopSequences:   true,
			},
		}, nil
	}

	return info, nil
}

// ListModels returns all available models for this provider
func (p *openaiProvider) ListModels() ([]Model, error) {
	if len(p.config.Models) > 0 {
		return p.config.Models, nil
	}

	// Return default OpenAI models if none configured
	return []Model{
		ModelGPT4o,
		ModelGPT4oMini,
		ModelGPT4Turbo,
		ModelGPT4,
		ModelGPT35Turbo,
	}, nil
}

// Close closes the provider and releases resources
func (p *openaiProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	p.status = ProviderStatusUnavailable

	p.logger.Info("OpenAI provider closed")
	return nil
}

// GetTokenAccount returns the token account for this provider
func (p *openaiProvider) GetTokenAccount() *TokenAccount {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tokenAccount
}

// validateRequest validates the completion request
func (p *openaiProvider) validateRequest(req *CompletionRequest) error {
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
func (p *openaiProvider) updateStatus(status ProviderStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
}

// handleErrorResponse handles error responses from the OpenAI API
func (p *openaiProvider) handleErrorResponse(statusCode int, body []byte) (*CompletionResponse, error) {
	var errResp openaiErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		p.updateStatus(ProviderStatusError)
		return nil, WrapProviderError(ErrCodeInvalidResponse,
			fmt.Sprintf("HTTP %d: failed to parse error response", statusCode), err)
	}

	// Map OpenAI error types to our error codes
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

	p.logger.Error("OpenAI API error",
		"status_code", statusCode,
		"error_type", errResp.Error.Type,
		"message", errResp.Error.Message)

	return nil, NewProviderError(errCode, errResp.Error.Message)
}

// convertRequest converts a unified request to OpenAI format
func (p *openaiProvider) convertRequest(req *CompletionRequest) (*openaiChatCompletionRequest, error) {
	// Convert messages
	messages := make([]openaiMessage, 0, len(req.Messages))

	for _, msg := range req.Messages {
		openaiMsg, err := p.convertMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		messages = append(messages, *openaiMsg)
	}

	openaiReq := &openaiChatCompletionRequest{
		Model:    req.Model.String(),
		Messages: messages,
	}

	// Set max_tokens if provided (it's optional in OpenAI)
	if req.MaxTokens > 0 {
		openaiReq.MaxTokens = req.MaxTokens
	}

	if req.Temperature != nil {
		openaiReq.Temperature = req.Temperature
	}

	if req.TopP != nil {
		openaiReq.TopP = req.TopP
	}

	if len(req.StopSequences) > 0 {
		if len(req.StopSequences) == 1 {
			openaiReq.Stop = req.StopSequences[0]
		} else {
			openaiReq.Stop = req.StopSequences
		}
	}

	return openaiReq, nil
}

// convertMessage converts a unified message to OpenAI format
func (p *openaiProvider) convertMessage(msg Message) (*openaiMessage, error) {
	openaiMsg := &openaiMessage{
		Role: msg.Role.String(),
	}

	// Handle content (string or content blocks)
	switch content := msg.Content.(type) {
	case string:
		openaiMsg.Content = content
	case []ContentBlock:
		// OpenAI supports array of content objects for multimodal
		blocks := make([]interface{}, len(content))
		for i, block := range content {
			converted, err := p.convertContentBlock(block)
			if err != nil {
				return nil, err
			}
			blocks[i] = converted
		}
		openaiMsg.Content = blocks
	default:
		// Try to serialize as JSON
		data, err := json.Marshal(content)
		if err != nil {
			return nil, fmt.Errorf("unsupported content type: %T", msg.Content)
		}
		openaiMsg.Content = string(data)
	}

	return openaiMsg, nil
}

// convertContentBlock converts a unified content block to OpenAI format
func (p *openaiProvider) convertContentBlock(block ContentBlock) (interface{}, error) {
	switch block.Type {
	case ContentTypeText:
		return map[string]interface{}{
			"type": "text",
			"text": block.Text,
		}, nil
	case ContentTypeImage:
		if block.Source == nil {
			return nil, fmt.Errorf("image content block missing source")
		}
		return map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]string{
				"url": block.Source.Type + ":" + block.Source.MediaType + "," + block.Source.Data,
			},
		}, nil
	default:
		// For unsupported content types, serialize as JSON
		data, err := json.Marshal(block)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize content block: %w", err)
		}
		return string(data), nil
	}
}

// convertResponse converts an OpenAI response to unified format
func (p *openaiProvider) convertResponse(resp *openaiChatCompletionResponse, model Model) *CompletionResponse {
	completion := &CompletionResponse{
		ID:       resp.ID,
		Model:    model,
		Provider: ProviderOpenAI,
		Usage: TokenUsage{
			InputTokens:      resp.Usage.PromptTokens,
			OutputTokens:     resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	// Extract content from response
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		// Content can be a string or a more complex structure
		if content, ok := choice.Message.Content.(string); ok {
			completion.Content = content
		} else {
			// Try to convert to string
			completion.Content = fmt.Sprintf("%v", choice.Message.Content)
		}

		// Map finish reason
		if choice.FinishReason != nil {
			completion.StopReason = p.mapStopReason(*choice.FinishReason)
		}
	}

	// Calculate total tokens
	completion.Usage.TotalTokens = completion.Usage.Total()

	return completion
}

// mapStopReason maps OpenAI finish reason to unified format
func (p *openaiProvider) mapStopReason(reason string) StopReason {
	switch reason {
	case "stop":
		return StopReasonEndTurn
	case "length":
		return StopReasonMaxTokens
	case "content_filter":
		return StopReasonStopSequence
	case "tool_calls":
		return StopReasonToolUse
	default:
		return StopReasonEndTurn
	}
}

// OpenAI API request/response types

type openaiChatCompletionRequest struct {
	Model       string                 `json:"model"`
	Messages    []openaiMessage        `json:"messages"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Temperature *float64               `json:"temperature,omitempty"`
	TopP        *float64               `json:"top_p,omitempty"`
	Stop        interface{}            `json:"stop,omitempty"` // string or []string
	Stream      bool                   `json:"stream,omitempty"`
}

type openaiMessage struct {
	Role    string      `json:"role"`    // system, user, assistant
	Content interface{} `json:"content"` // string or []interface{}
}

type openaiChatCompletionResponse struct {
	ID      string                    `json:"id"`
	Object  string                    `json:"object"`
	Created int64                     `json:"created"`
	Model   string                    `json:"model"`
	Choices []openaiChoice            `json:"choices"`
	Usage   openaiUsage               `json:"usage"`
}

type openaiChoice struct {
	Index        int            `json:"index"`
	Message      openaiMessage  `json:"message"`
	FinishReason *string        `json:"finish_reason"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openaiErrorResponse struct {
	Error openaiErrorDetail `json:"error"`
}

type openaiErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
	Code    string `json:"code,omitempty"`
}

// OpenAI streaming types

type openaiStreamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []openaiStreamChoice `json:"choices"`
}

type openaiStreamChoice struct {
	Index        int                  `json:"index"`
	Delta        openaiStreamDelta    `json:"delta"`
	FinishReason *string              `json:"finish_reason"`
}

type openaiStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// openaiSSEReader reads Server-Sent Events from OpenAI's streaming API
type openaiSSEReader struct {
	scanner *bufio.Scanner
}

// newOpenAISSEReader creates a new SSE reader for OpenAI
func newOpenAISSEReader(r io.Reader) *openaiSSEReader {
	return &openaiSSEReader{
		scanner: bufio.NewScanner(r),
	}
}

// NextChunk reads the next SSE chunk from the stream
func (r *openaiSSEReader) NextChunk() (*openaiStreamChunk, error) {
	for r.scanner.Scan() {
		line := r.scanner.Text()

		// Skip empty lines and non-data lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Extract data
		data := strings.TrimPrefix(line, "data: ")

		// Check for [DONE] marker
		if data == "[DONE]" {
			return nil, io.EOF
		}

		// Parse JSON
		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("failed to parse SSE chunk: %w", err)
		}

		return &chunk, nil
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}

	return nil, io.EOF
}

// Model information for OpenAI models
var openaiModelInfo = map[Model]*ModelInfo{
	ModelGPT4o: {
		ID:          ModelGPT4o,
		Provider:    ProviderOpenAI,
		Name:        "GPT-4o",
		Description: "Most capable multimodal model with vision, with 128K context window",
		ContextSize: 128000,
		Capabilities: ModelCapabilities{
			Streaming:       true,
			FunctionCalling: true,
			Vision:          true,
			PromptCaching:   false,
			JSONMode:        true,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            false,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
	ModelGPT4oMini: {
		ID:          ModelGPT4oMini,
		Provider:    ProviderOpenAI,
		Name:        "GPT-4o Mini",
		Description: "Fast and affordable small model with vision, with 128K context window",
		ContextSize: 128000,
		Capabilities: ModelCapabilities{
			Streaming:       true,
			FunctionCalling: true,
			Vision:          true,
			PromptCaching:   false,
			JSONMode:        true,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            false,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
	ModelGPT4Turbo: {
		ID:          ModelGPT4Turbo,
		Provider:    ProviderOpenAI,
		Name:        "GPT-4 Turbo",
		Description: "High-intelligence model with vision, with 128K context window",
		ContextSize: 128000,
		Capabilities: ModelCapabilities{
			Streaming:       true,
			FunctionCalling: true,
			Vision:          true,
			PromptCaching:   false,
			JSONMode:        true,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            false,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
	ModelGPT4: {
		ID:          ModelGPT4,
		Provider:    ProviderOpenAI,
		Name:        "GPT-4",
		Description: "Original GPT-4 model with 8K context window",
		ContextSize: 8192,
		Capabilities: ModelCapabilities{
			Streaming:       true,
			FunctionCalling: true,
			Vision:          false,
			PromptCaching:   false,
			JSONMode:        true,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            false,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
	ModelGPT35Turbo: {
		ID:          ModelGPT35Turbo,
		Provider:    ProviderOpenAI,
		Name:        "GPT-3.5 Turbo",
		Description: "Fast and efficient model with 16K context window",
		ContextSize: 16385,
		Capabilities: ModelCapabilities{
			Streaming:       true,
			FunctionCalling: true,
			Vision:          false,
			PromptCaching:   false,
			JSONMode:        true,
			SystemMessages:  true,
			Temperature:     true,
			TopP:            true,
			TopK:            false,
			MaxTokens:       true,
			StopSequences:   true,
		},
	},
}
