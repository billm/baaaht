package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAnthropicProvider(t *testing.T) {
	tests := []struct {
		name    string
		config  ProviderConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: ProviderConfig{
				Provider: ProviderAnthropic,
				APIKey:   "test-api-key",
				BaseURL:  "https://api.anthropic.com",
				Timeout:  60,
				Models: []Model{
					ModelClaude3_5Sonnet,
					ModelClaude3Haiku,
				},
			},
			wantErr: false,
		},
		{
			name: "missing api key",
			config: ProviderConfig{
				Provider: ProviderAnthropic,
				BaseURL:  "https://api.anthropic.com",
				Timeout:  60,
			},
			wantErr: true,
			errMsg:  "API key is required",
		},
		{
			name: "empty base url uses default",
			config: ProviderConfig{
				Provider: ProviderAnthropic,
				APIKey:   "test-api-key",
				Timeout:  60,
			},
			wantErr: false,
		},
		{
			name: "zero timeout uses default",
			config: ProviderConfig{
				Provider: ProviderAnthropic,
				APIKey:   "test-api-key",
				BaseURL:  "https://api.anthropic.com",
			},
			wantErr: false,
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewAnthropicProvider(tt.config, log)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, ProviderAnthropic, provider.Provider())
				assert.NotEmpty(t, provider.Name())
			}
		})
	}
}

func TestAnthropicProvider_Provider(t *testing.T) {
	provider := createTestAnthropicProvider(t)
	assert.Equal(t, ProviderAnthropic, provider.Provider())
}

func TestAnthropicProvider_Name(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     string
	}{
		{
			name:     "custom name in metadata",
			metadata: map[string]interface{}{"name": "Custom Anthropic"},
			want:     "Custom Anthropic",
		},
		{
			name:     "no name in metadata",
			metadata: map[string]interface{}{},
			want:     "Anthropic",
		},
		{
			name:     "nil metadata",
			metadata: nil,
			want:     "Anthropic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ProviderConfig{
				Provider: ProviderAnthropic,
				APIKey:   "test-api-key",
				Metadata: tt.metadata,
			}
			log, err := logger.NewDefault()
			require.NoError(t, err)

			provider, err := NewAnthropicProvider(config, log)
			require.NoError(t, err)
			assert.Equal(t, tt.want, provider.Name())
		})
	}
}

func TestAnthropicProvider_Status(t *testing.T) {
	provider := createTestAnthropicProvider(t)
	assert.Equal(t, ProviderStatusAvailable, provider.Status())
	assert.True(t, provider.IsAvailable())
}

func TestAnthropicProvider_SupportsModel(t *testing.T) {
	config := ProviderConfig{
		Provider: ProviderAnthropic,
		APIKey:   "test-api-key",
		Models: []Model{
			ModelClaude3_5Sonnet,
			ModelClaude3Haiku,
		},
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewAnthropicProvider(config, log)
	require.NoError(t, err)

	tests := []struct {
		model      Model
		want       bool
		name       string
	}{
		{
			model: ModelClaude3_5Sonnet,
			want:  true,
			name:  "supported model in config",
		},
		{
			model: ModelClaude3Haiku,
			want:  true,
			name:  "another supported model in config",
		},
		{
			model: ModelClaude3Opus,
			want:  true,
			name:  "anthropic model not in config",
		},
		{
			model: ModelGPT4o,
			want:  false,
			name:  "openai model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, provider.SupportsModel(tt.model))
		})
	}
}

func TestAnthropicProvider_ModelInfo(t *testing.T) {
	provider := createTestAnthropicProvider(t)

	tests := []struct {
		name    string
		model   Model
		wantErr bool
	}{
		{
			name:    "known model",
			model:   ModelClaude3_5Sonnet,
			wantErr: false,
		},
		{
			name:    "another known model",
			model:   ModelClaude3Haiku,
			wantErr: false,
		},
		{
			name:    "unsupported model",
			model:   ModelGPT4o,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := provider.ModelInfo(tt.model)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, info)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, info)
				assert.Equal(t, tt.model, info.ID)
				assert.Equal(t, ProviderAnthropic, info.Provider)
				assert.NotEmpty(t, info.Name)
				assert.Greater(t, info.ContextSize, 0)
				// Streaming is now implemented
				assert.True(t, info.Capabilities.Streaming)
			}
		})
	}
}

func TestAnthropicProvider_ListModels(t *testing.T) {
	tests := []struct {
		name           string
		configured     []Model
		expectedCount  int
	}{
		{
			name:           "models configured",
			configured:     []Model{ModelClaude3_5Sonnet, ModelClaude3Haiku},
			expectedCount:  2,
		},
		{
			name:           "no models configured - returns defaults",
			configured:     nil,
			expectedCount:  5, // All default Anthropic models
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ProviderConfig{
				Provider: ProviderAnthropic,
				APIKey:   "test-api-key",
				Models:   tt.configured,
			}
			log, err := logger.NewDefault()
			require.NoError(t, err)

			provider, err := NewAnthropicProvider(config, log)
			require.NoError(t, err)

			models, err := provider.ListModels()
			assert.NoError(t, err)
			assert.Len(t, models, tt.expectedCount)
		})
	}
}

func TestAnthropicProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))

		// Parse request body
		var req anthropicMessageRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)

		assert.Equal(t, "claude-3-5-sonnet-20241022", req.Model)
		assert.Equal(t, 100, req.MaxTokens)
		assert.Len(t, req.Messages, 1)
		assert.Equal(t, "user", req.Messages[0].Role)
		assert.Equal(t, "Hello, Claude!", req.Messages[0].Content)
		assert.Equal(t, "You are a helpful assistant.", req.System)

		// Send mock response
		resp := anthropicMessageResponse{
			ID:   "msg-123",
			Type: "message",
			Role: "assistant",
			Content: []anthropicContentBlock{
				{
					Type: "text",
					Text: "Hello! How can I help you today?",
				},
			},
			StopReason: "end_turn",
			Usage: anthropicUsage{
				InputTokens:  10,
				OutputTokens: 15,
			},
			Model: "claude-3-5-sonnet-20241022",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := ProviderConfig{
		Provider: ProviderAnthropic,
		APIKey:   "test-api-key",
		BaseURL:  server.URL,
		Timeout:  60,
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewAnthropicProvider(config, log)
	require.NoError(t, err)

	req := &CompletionRequest{
		Model: ModelClaude3_5Sonnet,
		Messages: []Message{
			{
				Role:    MessageRoleSystem,
				Content: "You are a helpful assistant.",
			},
			{
				Role:    MessageRoleUser,
				Content: "Hello, Claude!",
			},
		},
		MaxTokens: 100,
	}

	resp, err := provider.Complete(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	assert.Equal(t, "msg-123", resp.ID)
	assert.Equal(t, ModelClaude3_5Sonnet, resp.Model)
	assert.Equal(t, ProviderAnthropic, resp.Provider)
	assert.Equal(t, "Hello! How can I help you today?", resp.Content)
	assert.Equal(t, StopReasonEndTurn, resp.StopReason)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 15, resp.Usage.OutputTokens)
	assert.Equal(t, 25, resp.Usage.TotalTokens)
}

func TestAnthropicProvider_Complete_WithError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   interface{}
		wantErr    bool
		errCode    string
	}{
		{
			name:       "authentication error",
			statusCode: http.StatusUnauthorized,
			response: map[string]interface{}{
				"error": map[string]string{
					"type":    "authentication_error",
					"message": "Invalid API key",
				},
			},
			wantErr: true,
			errCode: ErrCodeAuthenticationFailed,
		},
		{
			name:       "rate limit error",
			statusCode: http.StatusTooManyRequests,
			response: map[string]interface{}{
				"error": map[string]string{
					"type":    "rate_limit_error",
					"message": "Rate limit exceeded",
				},
			},
			wantErr: true,
			errCode: ErrCodeRateLimited,
		},
		{
			name:       "invalid request error",
			statusCode: http.StatusBadRequest,
			response: map[string]interface{}{
				"error": map[string]string{
					"type":    "invalid_request_error",
					"message": "Invalid request",
				},
			},
			wantErr: true,
			errCode: ErrCodeInvalidRequest,
		},
		{
			name:       "context too long",
			statusCode: http.StatusRequestEntityTooLarge,
			response: map[string]interface{}{
				"error": map[string]string{
					"type":    "invalid_request_error",
					"message": "Prompt too long",
				},
			},
			wantErr: true,
			errCode: ErrCodeContextTooLong,
		},
		{
			name:       "internal server error",
			statusCode: http.StatusInternalServerError,
			response: map[string]interface{}{
				"error": map[string]string{
					"type":    "internal_server_error",
					"message": "Internal error",
				},
			},
			wantErr: true,
			errCode: ErrCodeUpstreamError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			config := ProviderConfig{
				Provider: ProviderAnthropic,
				APIKey:   "test-api-key",
				BaseURL:  server.URL,
				Timeout:  60,
			}
			log, err := logger.NewDefault()
			require.NoError(t, err)

			provider, err := NewAnthropicProvider(config, log)
			require.NoError(t, err)

			req := &CompletionRequest{
				Model: ModelClaude3_5Sonnet,
				Messages: []Message{
					{
						Role:    MessageRoleUser,
						Content: "Hello!",
					},
				},
				MaxTokens: 100,
			}

			resp, err := provider.Complete(context.Background(), req)

			assert.True(t, tt.wantErr)
			assert.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.errCode)
		})
	}
}

func TestAnthropicProvider_Complete_ValidationErrors(t *testing.T) {
	provider := createTestAnthropicProvider(t)

	tests := []struct {
		name    string
		req     *CompletionRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
			errMsg:  "request cannot be nil",
		},
		{
			name: "empty model",
			req: &CompletionRequest{
				Model:     "",
				Messages:  []Message{{Role: MessageRoleUser, Content: "Hello"}},
				MaxTokens: 100,
			},
			wantErr: true,
			errMsg:  "model is required",
		},
		{
			name: "unsupported model",
			req: &CompletionRequest{
				Model:     ModelGPT4o,
				Messages:  []Message{{Role: MessageRoleUser, Content: "Hello"}},
				MaxTokens: 100,
			},
			wantErr: true,
			errMsg:  "not supported",
		},
		{
			name: "no messages",
			req: &CompletionRequest{
				Model:     ModelClaude3_5Sonnet,
				Messages:  []Message{},
				MaxTokens: 100,
			},
			wantErr: true,
			errMsg:  "at least one message",
		},
		{
			name: "zero max tokens",
			req: &CompletionRequest{
				Model:     ModelClaude3_5Sonnet,
				Messages:  []Message{{Role: MessageRoleUser, Content: "Hello"}},
				MaxTokens: 0,
			},
			wantErr: true,
			errMsg:  "max_tokens must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := provider.Complete(context.Background(), tt.req)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, resp)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAnthropicProvider_CompleteStream_Success(t *testing.T) {
	// Track received chunks
	var receivedChunks []*CompletionChunk
	streamEnded := make(chan bool, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))

		// Parse request body
		var req anthropicMessageRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)
		assert.True(t, req.Stream) // Streaming must be enabled

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Flush headers
		fflush(w)

		// Send SSE events with flush after each
		fmt.Fprintln(w, "event: message_start")
		fmt.Fprintln(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-123\",\"type\":\"message\",\"role\":\"assistant\"}}")
		fmt.Fprintln(w)
		fflush(w)

		fmt.Fprintln(w, "event: content_block_start")
		fmt.Fprintln(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}")
		fmt.Fprintln(w)
		fflush(w)

		fmt.Fprintln(w, "event: content_block_delta")
		fmt.Fprintln(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}")
		fmt.Fprintln(w)
		fflush(w)

		fmt.Fprintln(w, "event: content_block_delta")
		fmt.Fprintln(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"!\"}}")
		fmt.Fprintln(w)
		fflush(w)

		fmt.Fprintln(w, "event: content_block_stop")
		fmt.Fprintln(w, "data: {\"type\":\"content_block_stop\",\"index\":0}")
		fmt.Fprintln(w)
		fflush(w)

		fmt.Fprintln(w, "event: message_delta")
		fmt.Fprintln(w, "data: {\"type\":\"message_delta\",\"delta\":{\"type\":\"stop_reason\",\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}")
		fmt.Fprintln(w)
		fflush(w)

		fmt.Fprintln(w, "event: message_stop")
		fmt.Fprintln(w, "data: {\"type\":\"message_stop\"}")
		fmt.Fprintln(w)
		fflush(w)
	}))
	defer server.Close()

	config := ProviderConfig{
		Provider: ProviderAnthropic,
		APIKey:   "test-api-key",
		BaseURL:  server.URL,
		Timeout:  60,
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewAnthropicProvider(config, log)
	require.NoError(t, err)

	req := &CompletionRequest{
		Model: ModelClaude3_5Sonnet,
		Messages: []Message{
			{Role: MessageRoleUser, Content: "Hello"},
		},
		MaxTokens: 100,
	}

	chunkChan, err := provider.CompleteStream(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, chunkChan)

	// Collect chunks in a goroutine
	go func() {
		for chunk := range chunkChan {
			receivedChunks = append(receivedChunks, chunk)
			if chunk.Done || chunk.Error != nil {
				streamEnded <- true
			}
		}
	}()

	// Wait for stream to complete
	select {
	case <-streamEnded:
		// Give a small delay to ensure all chunks are received
		time.Sleep(10 * time.Millisecond)
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not complete within timeout")
	}

	// Check for errors
	for _, chunk := range receivedChunks {
		if chunk.Error != nil {
			t.Fatalf("Received error chunk: %v", chunk.Error)
		}
	}

	// Verify we received chunks
	assert.Greater(t, len(receivedChunks), 0, "Should receive at least one chunk")

	// Verify final chunk
	finalChunk := receivedChunks[len(receivedChunks)-1]
	assert.Equal(t, "msg-123", finalChunk.ID)
	assert.Equal(t, ModelClaude3_5Sonnet, finalChunk.Model)
	assert.Equal(t, ProviderAnthropic, finalChunk.Provider)
	assert.Equal(t, "Hello!", finalChunk.Content)
	assert.True(t, finalChunk.Done)
	assert.NotNil(t, finalChunk.StopReason, "StopReason should not be nil")
	assert.Equal(t, StopReasonEndTurn, *finalChunk.StopReason)
	assert.NotNil(t, finalChunk.Usage)
	assert.Equal(t, 2, finalChunk.Usage.OutputTokens)
}

func TestAnthropicProvider_CompleteStream_ContentDeltas(t *testing.T) {
	var receivedChunks []*CompletionChunk

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send events
		fmt.Fprintln(w, "event: message_start")
		fmt.Fprintln(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-456\",\"type\":\"message\",\"role\":\"assistant\"}}")
		fmt.Fprintln(w)

		fmt.Fprintln(w, "event: content_block_start")
		fmt.Fprintln(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}")
		fmt.Fprintln(w)

		// Send multiple text deltas
		fmt.Fprintln(w, "event: content_block_delta")
		fmt.Fprintln(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"The\"}}")
		fmt.Fprintln(w)

		fmt.Fprintln(w, "event: content_block_delta")
		fmt.Fprintln(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" answer\"}}")
		fmt.Fprintln(w)

		fmt.Fprintln(w, "event: content_block_delta")
		fmt.Fprintln(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" is 42\"}}")
		fmt.Fprintln(w)

		fmt.Fprintln(w, "event: content_block_stop")
		fmt.Fprintln(w, "data: {\"type\":\"content_block_stop\",\"index\":0}")
		fmt.Fprintln(w)

		fmt.Fprintln(w, "event: message_delta")
		fmt.Fprintln(w, "data: {\"type\":\"message_delta\",\"delta\":{\"type\":\"stop_reason\",\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":4}}")
		fmt.Fprintln(w)

		fmt.Fprintln(w, "event: message_stop")
		fmt.Fprintln(w, "data: {\"type\":\"message_stop\"}")
		fmt.Fprintln(w)

		fflush(w)
	}))
	defer server.Close()

	config := ProviderConfig{
		Provider: ProviderAnthropic,
		APIKey:   "test-api-key",
		BaseURL:  server.URL,
		Timeout:  60,
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewAnthropicProvider(config, log)
	require.NoError(t, err)

	req := &CompletionRequest{
		Model: ModelClaude3_5Sonnet,
		Messages: []Message{
			{Role: MessageRoleUser, Content: "What is the answer?"},
		},
		MaxTokens: 100,
	}

	chunkChan, err := provider.CompleteStream(context.Background(), req)
	require.NoError(t, err)

	// Collect all chunks
	for chunk := range chunkChan {
		receivedChunks = append(receivedChunks, chunk)
		if chunk.Done {
			break
		}
	}

	// Verify we got the right number of chunks (3 deltas + final)
	assert.Equal(t, 4, len(receivedChunks))

	// Verify deltas
	assert.Equal(t, "The", receivedChunks[0].Delta)
	assert.Equal(t, " answer", receivedChunks[1].Delta)
	assert.Equal(t, " is 42", receivedChunks[2].Delta)

	// Verify accumulated content
	assert.Equal(t, "The", receivedChunks[0].Content)
	assert.Equal(t, "The answer", receivedChunks[1].Content)
	assert.Equal(t, "The answer is 42", receivedChunks[2].Content)
	assert.Equal(t, "The answer is 42", receivedChunks[3].Content)

	// Verify final chunk
	assert.True(t, receivedChunks[3].Done)
	assert.Equal(t, "The answer is 42", receivedChunks[3].Content)
}

func TestAnthropicProvider_CompleteStream_ValidationError(t *testing.T) {
	provider := createTestAnthropicProvider(t)

	req := &CompletionRequest{
		Model:     ModelGPT4o, // Unsupported model
		Messages:  []Message{{Role: MessageRoleUser, Content: "Hello"}},
		MaxTokens: 100,
	}

	chunkChan, err := provider.CompleteStream(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, chunkChan)
	assert.Contains(t, err.Error(), "not supported")
}

func TestAnthropicProvider_CompleteStream_ClosedProvider(t *testing.T) {
	provider := createTestAnthropicProvider(t)
	provider.Close()

	req := &CompletionRequest{
		Model:     ModelClaude3_5Sonnet,
		Messages:  []Message{{Role: MessageRoleUser, Content: "Hello"}},
		MaxTokens: 100,
	}

	chunkChan, err := provider.CompleteStream(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, chunkChan)
	assert.Contains(t, err.Error(), "provider is closed")
}

func TestAnthropicProvider_CompleteStream_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send initial events
		fmt.Fprintln(w, "event: message_start")
		fmt.Fprintln(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-789\",\"type\":\"message\",\"role\":\"assistant\"}}")
		fmt.Fprintln(w)
		fflush(w)

		// Wait before sending more (will be cancelled)
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	config := ProviderConfig{
		Provider: ProviderAnthropic,
		APIKey:   "test-api-key",
		BaseURL:  server.URL,
		Timeout:  60,
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewAnthropicProvider(config, log)
	require.NoError(t, err)

	req := &CompletionRequest{
		Model: ModelClaude3_5Sonnet,
		Messages: []Message{
			{Role: MessageRoleUser, Content: "Hello"},
		},
		MaxTokens: 100,
	}

	ctx, cancel := context.WithCancel(context.Background())
	chunkChan, err := provider.CompleteStream(ctx, req)
	require.NoError(t, err)

	// Cancel context immediately
	cancel()

	// Should receive an error chunk
	for chunk := range chunkChan {
		if chunk.Error != nil {
			assert.Equal(t, context.Canceled, chunk.Error)
			return
		}
	}

	t.Error("Expected to receive error chunk after context cancellation")
}

// fflush flushes the response writer
func fflush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func TestAnthropicProvider_Close(t *testing.T) {
	provider := createTestAnthropicProvider(t)

	assert.Equal(t, ProviderStatusAvailable, provider.Status())
	assert.True(t, provider.IsAvailable())

	err := provider.Close()
	assert.NoError(t, err)
	assert.Equal(t, ProviderStatusUnavailable, provider.Status())
	assert.False(t, provider.IsAvailable())

	// Close should be idempotent
	err = provider.Close()
	assert.NoError(t, err)
}

func TestAnthropicProvider_Complete_AfterClose(t *testing.T) {
	provider := createTestAnthropicProvider(t)
	provider.Close()

	req := &CompletionRequest{
		Model: ModelClaude3_5Sonnet,
		Messages: []Message{
			{Role: MessageRoleUser, Content: "Hello"},
		},
		MaxTokens: 100,
	}

	resp, err := provider.Complete(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "provider is closed")
}

func TestMapStopReason(t *testing.T) {
	tests := []struct {
		reason string
		want   StopReason
	}{
		{"end_turn", StopReasonEndTurn},
		{"max_tokens", StopReasonMaxTokens},
		{"stop_sequence", StopReasonStopSequence},
		{"tool_use", StopReasonToolUse},
		{"unknown", StopReasonEndTurn},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			assert.Equal(t, tt.want, mapStopReason(tt.reason))
		})
	}
}

func TestAnthropicTokens(t *testing.T) {
	t.Run("regular completion records token usage", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := anthropicMessageResponse{
				ID:   "msg-123",
				Type: "message",
				Role: "assistant",
				Content: []anthropicContentBlock{
					{Type: "text", Text: "Hello!"},
				},
				StopReason: "end_turn",
				Usage: anthropicUsage{
					InputTokens:              100,
					OutputTokens:             50,
					CacheReadInputTokens:     500,
					CacheCreationInputTokens: 100,
				},
				Model: "claude-3-5-sonnet-20241022",
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		config := ProviderConfig{
			Provider: ProviderAnthropic,
			APIKey:   "test-api-key",
			BaseURL:  server.URL,
			Timeout:  60,
		}
		log, err := logger.NewDefault()
		require.NoError(t, err)

		provider, err := NewAnthropicProvider(config, log)
		require.NoError(t, err)

		req := &CompletionRequest{
			Model: ModelClaude3_5Sonnet,
			Messages: []Message{
				{Role: MessageRoleUser, Content: "Hello"},
			},
			MaxTokens: 100,
		}

		resp, err := provider.Complete(context.Background(), req)
		require.NoError(t, err)

		// Get the token account via type assertion
		anthropicProvider, ok := provider.(*anthropicProvider)
		require.True(t, ok, "Provider should be anthropicProvider")

		account := anthropicProvider.GetTokenAccount()
		require.NotNil(t, account)

		// Verify usage was recorded
		totalUsage := account.GetTotalUsage()
		assert.Equal(t, 100, totalUsage.InputTokens)
		assert.Equal(t, 50, totalUsage.OutputTokens)
		assert.Equal(t, 500, totalUsage.CacheReadTokens)
		assert.Equal(t, 100, totalUsage.CacheWriteTokens)

		// Verify request count
		assert.Equal(t, int64(1), account.GetRequestCount())

		// Verify response has correct usage
		assert.Equal(t, 100, resp.Usage.InputTokens)
		assert.Equal(t, 50, resp.Usage.OutputTokens)
		assert.Equal(t, 500, resp.Usage.CacheReadTokens)
		assert.Equal(t, 100, resp.Usage.CacheWriteTokens)
		assert.Equal(t, 750, resp.Usage.TotalTokens) // 100 + 50 + 500 + 100
	})

	t.Run("streaming completion records token usage", func(t *testing.T) {
		streamEnded := make(chan bool, 1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			fflush(w)

			// Send streaming events
			fmt.Fprintln(w, "event: message_start")
			fmt.Fprintln(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-456\",\"type\":\"message\",\"role\":\"assistant\"}}")
			fmt.Fprintln(w)
			fflush(w)

			fmt.Fprintln(w, "event: content_block_start")
			fmt.Fprintln(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}")
			fmt.Fprintln(w)
			fflush(w)

			fmt.Fprintln(w, "event: content_block_delta")
			fmt.Fprintln(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}")
			fmt.Fprintln(w)
			fflush(w)

			fmt.Fprintln(w, "event: content_block_stop")
			fmt.Fprintln(w, "data: {\"type\":\"content_block_stop\",\"index\":0}")
			fmt.Fprintln(w)
			fflush(w)

			fmt.Fprintln(w, "event: message_delta")
			fmt.Fprintln(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":200,\"output_tokens\":30,\"cache_read_input_tokens\":1000,\"cache_creation_input_tokens\":200}}")
			fmt.Fprintln(w)
			fflush(w)

			fmt.Fprintln(w, "event: message_stop")
			fmt.Fprintln(w, "data: {\"type\":\"message_stop\"}")
			fmt.Fprintln(w)
			fflush(w)
		}))
		defer server.Close()

		config := ProviderConfig{
			Provider: ProviderAnthropic,
			APIKey:   "test-api-key",
			BaseURL:  server.URL,
			Timeout:  60,
		}
		log, err := logger.NewDefault()
		require.NoError(t, err)

		provider, err := NewAnthropicProvider(config, log)
		require.NoError(t, err)

		req := &CompletionRequest{
			Model: ModelClaude3_5Sonnet,
			Messages: []Message{
				{Role: MessageRoleUser, Content: "Hello"},
			},
			MaxTokens: 100,
		}

		chunkChan, err := provider.CompleteStream(context.Background(), req)
		require.NoError(t, err)

		// Collect chunks
		go func() {
			for range chunkChan {
			}
			streamEnded <- true
		}()

		select {
		case <-streamEnded:
		case <-time.After(5 * time.Second):
			t.Fatal("Stream did not complete within timeout")
		}

		// Get the token account via type assertion
		anthropicProvider, ok := provider.(*anthropicProvider)
		require.True(t, ok, "Provider should be anthropicProvider")

		account := anthropicProvider.GetTokenAccount()
		require.NotNil(t, account)

		// Verify usage was recorded
		totalUsage := account.GetTotalUsage()
		assert.Equal(t, 200, totalUsage.InputTokens)
		assert.Equal(t, 30, totalUsage.OutputTokens)
		assert.Equal(t, 1000, totalUsage.CacheReadTokens)
		assert.Equal(t, 200, totalUsage.CacheWriteTokens)

		// Verify request count
		assert.Equal(t, int64(1), account.GetRequestCount())
	})

	t.Run("token account persists across multiple requests", func(t *testing.T) {
		requestCount := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++

			resp := anthropicMessageResponse{
				ID:   fmt.Sprintf("msg-%d", requestCount),
				Type: "message",
				Role: "assistant",
				Content: []anthropicContentBlock{
					{Type: "text", Text: "Response"},
				},
				StopReason: "end_turn",
				Usage: anthropicUsage{
					InputTokens:  50 * requestCount,
					OutputTokens: 25 * requestCount,
				},
				Model: "claude-3-5-sonnet-20241022",
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		config := ProviderConfig{
			Provider: ProviderAnthropic,
			APIKey:   "test-api-key",
			BaseURL:  server.URL,
			Timeout:  60,
		}
		log, err := logger.NewDefault()
		require.NoError(t, err)

		provider, err := NewAnthropicProvider(config, log)
		require.NoError(t, err)

		req := &CompletionRequest{
			Model: ModelClaude3_5Sonnet,
			Messages: []Message{
				{Role: MessageRoleUser, Content: "Test"},
			},
			MaxTokens: 100,
		}

		// Make first request
		_, err = provider.Complete(context.Background(), req)
		require.NoError(t, err)

		// Make second request
		_, err = provider.Complete(context.Background(), req)
		require.NoError(t, err)

		// Make third request
		_, err = provider.Complete(context.Background(), req)
		require.NoError(t, err)

		// Get the token account
		anthropicProvider, ok := provider.(*anthropicProvider)
		require.True(t, ok)

		account := anthropicProvider.GetTokenAccount()
		require.NotNil(t, account)

		// Verify accumulated usage across all requests
		totalUsage := account.GetTotalUsage()
		assert.Equal(t, 300, totalUsage.InputTokens)    // 50 + 100 + 150
		assert.Equal(t, 150, totalUsage.OutputTokens)   // 25 + 50 + 75

		// Verify request count
		assert.Equal(t, int64(3), account.GetRequestCount())

		// Verify average tokens per request
		avgTokens := account.GetAverageTokensPerRequest()
		assert.InDelta(t, 150.0, avgTokens, 0.01) // (300+150)/3 = 150
	})
}

func TestAnthropicSSEReader_ParseEvents(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantEvents int
		wantTypes  []string
	}{
		{
			name: "message_start event",
			input: "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-123\",\"type\":\"message\"}}\n\n",
			wantEvents: 1,
			wantTypes:  []string{"message_start"},
		},
		{
			name: "content_block_delta event",
			input: "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n",
			wantEvents: 1,
			wantTypes:  []string{"content_block_delta"},
		},
		{
			name: "multiple events",
			input: `event: message_start
data: {"type":"message_start","message":{"id":"msg-123"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: message_stop
data: {"type":"message_stop"}
`,
			wantEvents: 3,
			wantTypes:  []string{"message_start", "content_block_delta", "message_stop"},
		},
		{
			name: "ping event",
			input: "event: ping\ndata: {}\n\n",
			wantEvents: 1,
			wantTypes:  []string{"ping"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newAnthropicSSEReader(strings.NewReader(tt.input))

			events := []string{}
			for i := 0; i < tt.wantEvents; i++ {
				event, err := reader.NextEvent()
				assert.NoError(t, err)
				assert.NotNil(t, event)
				events = append(events, event.Type)
			}

			// Should return EOF after all events
			_, err := reader.NextEvent()
			assert.Equal(t, io.EOF, err)

			assert.Equal(t, tt.wantTypes, events)
		})
	}
}

func TestAnthropicSSEReader_ComplexStream(t *testing.T) {
	// Simulate a real streaming response
	input := `event: message_start
data: {"type":"message_start","message":{"id":"msg-123","type":"message","role":"assistant","model":"claude-3-5-sonnet-20241022"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}

event: message_stop
data: {"type":"message_stop"}
`

	reader := newAnthropicSSEReader(strings.NewReader(input))

	events := []*anthropicStreamEvent{}
	for {
		event, err := reader.NextEvent()
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)
		events = append(events, event)
	}

	// Verify all events were parsed
	assert.Equal(t, 7, len(events))

	// Verify message_start
	assert.Equal(t, "message_start", events[0].Type)
	assert.NotNil(t, events[0].MessageStart)
	assert.Equal(t, "msg-123", events[0].MessageStart.Message.ID)

	// Verify content_block_start
	assert.Equal(t, "content_block_start", events[1].Type)
	assert.NotNil(t, events[1].ContentBlockStart)

	// Verify first delta
	assert.Equal(t, "content_block_delta", events[2].Type)
	assert.NotNil(t, events[2].ContentBlockDelta)
	assert.Equal(t, "Hello", events[2].ContentBlockDelta.Delta.Text)

	// Verify second delta
	assert.Equal(t, "content_block_delta", events[3].Type)
	assert.Equal(t, " world", events[3].ContentBlockDelta.Delta.Text)

	// Verify content_block_stop
	assert.Equal(t, "content_block_stop", events[4].Type)

	// Verify message_delta
	assert.Equal(t, "message_delta", events[5].Type)
	assert.NotNil(t, events[5].MessageDelta)
	assert.Equal(t, "end_turn", events[5].MessageDelta.Delta.StopReason)
	assert.NotNil(t, events[5].MessageDelta.Usage)
	assert.Equal(t, 2, events[5].MessageDelta.Usage.OutputTokens)

	// Verify message_stop
	assert.Equal(t, "message_stop", events[6].Type)
}

// Helper functions

func createTestAnthropicProvider(t *testing.T) LLMProvider {
	t.Helper()

	config := ProviderConfig{
		Provider: ProviderAnthropic,
		APIKey:   "test-api-key",
		BaseURL:  "https://api.anthropic.com",
		Timeout:  60,
		Models: []Model{
			ModelClaude3_5Sonnet,
			ModelClaude3Haiku,
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewAnthropicProvider(config, log)
	require.NoError(t, err)

	return provider
}
