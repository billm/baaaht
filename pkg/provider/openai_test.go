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

func TestNewOpenAIProvider(t *testing.T) {
	tests := []struct {
		name    string
		config  ProviderConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: ProviderConfig{
				Provider: ProviderOpenAI,
				APIKey:   "test-api-key",
				BaseURL:  "https://api.openai.com",
				Timeout:  60,
				Models: []Model{
					ModelGPT4o,
					ModelGPT4oMini,
				},
			},
			wantErr: false,
		},
		{
			name: "missing api key",
			config: ProviderConfig{
				Provider: ProviderOpenAI,
				BaseURL:  "https://api.openai.com",
				Timeout:  60,
			},
			wantErr: true,
			errMsg:  "API key is required",
		},
		{
			name: "empty base url uses default",
			config: ProviderConfig{
				Provider: ProviderOpenAI,
				APIKey:   "test-api-key",
				Timeout:  60,
			},
			wantErr: false,
		},
		{
			name: "zero timeout uses default",
			config: ProviderConfig{
				Provider: ProviderOpenAI,
				APIKey:   "test-api-key",
				BaseURL:  "https://api.openai.com",
			},
			wantErr: false,
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewOpenAIProvider(tt.config, log)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, ProviderOpenAI, provider.Provider())
				assert.NotEmpty(t, provider.Name())
			}
		})
	}
}

func TestOpenAIProvider_Provider(t *testing.T) {
	provider := createTestOpenAIProvider(t)
	assert.Equal(t, ProviderOpenAI, provider.Provider())
}

func TestOpenAIProvider_Name(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     string
	}{
		{
			name:     "custom name in metadata",
			metadata: map[string]interface{}{"name": "Custom OpenAI"},
			want:     "Custom OpenAI",
		},
		{
			name:     "no name in metadata",
			metadata: map[string]interface{}{},
			want:     "OpenAI",
		},
		{
			name:     "nil metadata",
			metadata: nil,
			want:     "OpenAI",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ProviderConfig{
				Provider: ProviderOpenAI,
				APIKey:   "test-api-key",
				Metadata: tt.metadata,
			}
			log, err := logger.NewDefault()
			require.NoError(t, err)

			provider, err := NewOpenAIProvider(config, log)
			require.NoError(t, err)
			assert.Equal(t, tt.want, provider.Name())
		})
	}
}

func TestOpenAIProvider_Status(t *testing.T) {
	provider := createTestOpenAIProvider(t)
	assert.Equal(t, ProviderStatusAvailable, provider.Status())
	assert.True(t, provider.IsAvailable())
}

func TestOpenAIProvider_SupportsModel(t *testing.T) {
	config := ProviderConfig{
		Provider: ProviderOpenAI,
		APIKey:   "test-api-key",
		Models: []Model{
			ModelGPT4o,
			ModelGPT4oMini,
		},
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewOpenAIProvider(config, log)
	require.NoError(t, err)

	tests := []struct {
		model Model
		want  bool
		name  string
	}{
		{
			model: ModelGPT4o,
			want:  true,
			name:  "supported model in config",
		},
		{
			model: ModelGPT4oMini,
			want:  true,
			name:  "another supported model in config",
		},
		{
			model: ModelGPT4Turbo,
			want:  true,
			name:  "openai model not in config",
		},
		{
			model: ModelClaude3_5Sonnet,
			want:  false,
			name:  "anthropic model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, provider.SupportsModel(tt.model))
		})
	}
}

func TestOpenAIProvider_ModelInfo(t *testing.T) {
	provider := createTestOpenAIProvider(t)

	tests := []struct {
		name    string
		model   Model
		wantErr bool
	}{
		{
			name:    "known model",
			model:   ModelGPT4o,
			wantErr: false,
		},
		{
			name:    "another known model",
			model:   ModelGPT4oMini,
			wantErr: false,
		},
		{
			name:    "unsupported model",
			model:   ModelClaude3_5Sonnet,
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
				assert.Equal(t, ProviderOpenAI, info.Provider)
				assert.NotEmpty(t, info.Name)
				assert.Greater(t, info.ContextSize, 0)
				assert.True(t, info.Capabilities.Streaming)
			}
		})
	}
}

func TestOpenAIProvider_ListModels(t *testing.T) {
	tests := []struct {
		name          string
		configured    []Model
		expectedCount int
	}{
		{
			name:          "models configured",
			configured:    []Model{ModelGPT4o, ModelGPT4oMini},
			expectedCount: 2,
		},
		{
			name:          "no models configured - returns defaults",
			configured:    nil,
			expectedCount: 5, // All default OpenAI models
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ProviderConfig{
				Provider: ProviderOpenAI,
				APIKey:   "test-api-key",
				Models:   tt.configured,
			}
			log, err := logger.NewDefault()
			require.NoError(t, err)

			provider, err := NewOpenAIProvider(config, log)
			require.NoError(t, err)

			models, err := provider.ListModels()
			assert.NoError(t, err)
			assert.Len(t, models, tt.expectedCount)
		})
	}
}

func TestOpenAIProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

		// Parse request body
		var req openaiChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)

		assert.Equal(t, "gpt-4o", req.Model)
		assert.Equal(t, 100, req.MaxTokens)
		assert.Len(t, req.Messages, 2)
		assert.Equal(t, "system", req.Messages[0].Role)
		assert.Equal(t, "You are a helpful assistant.", req.Messages[0].Content)
		assert.Equal(t, "user", req.Messages[1].Role)
		assert.Equal(t, "Hello, GPT!", req.Messages[1].Content)

		// Send mock response
		resp := openaiChatCompletionResponse{
			ID:     "chatcmpl-123",
			Object: "chat.completion",
			Created: time.Now().Unix(),
			Model:  "gpt-4o",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiMessage{
						Role:    "assistant",
						Content: "Hello! How can I help you today?",
					},
					FinishReason: stringPtr("stop"),
				},
			},
			Usage: openaiUsage{
				PromptTokens:     10,
				CompletionTokens: 15,
				TotalTokens:      25,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := ProviderConfig{
		Provider: ProviderOpenAI,
		APIKey:   "test-api-key",
		BaseURL:  server.URL,
		Timeout:  60,
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewOpenAIProvider(config, log)
	require.NoError(t, err)

	req := &CompletionRequest{
		Model: ModelGPT4o,
		Messages: []Message{
			{
				Role:    MessageRoleSystem,
				Content: "You are a helpful assistant.",
			},
			{
				Role:    MessageRoleUser,
				Content: "Hello, GPT!",
			},
		},
		MaxTokens: 100,
	}

	resp, err := provider.Complete(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	assert.Equal(t, "chatcmpl-123", resp.ID)
	assert.Equal(t, ModelGPT4o, resp.Model)
	assert.Equal(t, ProviderOpenAI, resp.Provider)
	assert.Equal(t, "Hello! How can I help you today?", resp.Content)
	assert.Equal(t, StopReasonEndTurn, resp.StopReason)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 15, resp.Usage.OutputTokens)
	assert.Equal(t, 25, resp.Usage.TotalTokens)
}

func TestOpenAIProvider_Complete_WithError(t *testing.T) {
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
					"message": "Invalid API key",
					"type":    "invalid_request_error",
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
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
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
					"message": "Invalid request",
					"type":    "invalid_request_error",
				},
			},
			wantErr: true,
			errCode: ErrCodeInvalidRequest,
		},
		{
			name:       "internal server error",
			statusCode: http.StatusInternalServerError,
			response: map[string]interface{}{
				"error": map[string]string{
					"message": "Internal error",
					"type":    "server_error",
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
				Provider: ProviderOpenAI,
				APIKey:   "test-api-key",
				BaseURL:  server.URL,
				Timeout:  60,
			}
			log, err := logger.NewDefault()
			require.NoError(t, err)

			provider, err := NewOpenAIProvider(config, log)
			require.NoError(t, err)

			req := &CompletionRequest{
				Model: ModelGPT4o,
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

func TestOpenAIProvider_Complete_ValidationErrors(t *testing.T) {
	provider := createTestOpenAIProvider(t)

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
				Model:     ModelClaude3_5Sonnet,
				Messages:  []Message{{Role: MessageRoleUser, Content: "Hello"}},
				MaxTokens: 100,
			},
			wantErr: true,
			errMsg:  "not supported",
		},
		{
			name: "no messages",
			req: &CompletionRequest{
				Model:     ModelGPT4o,
				Messages:  []Message{},
				MaxTokens: 100,
			},
			wantErr: true,
			errMsg:  "at least one message",
		},
		{
			name: "zero max tokens",
			req: &CompletionRequest{
				Model:     ModelGPT4o,
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

func TestOpenAIProvider_CompleteStream_Success(t *testing.T) {
	// Track received chunks
	var receivedChunks []*CompletionChunk
	streamEnded := make(chan bool, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

		// Parse request body
		var req openaiChatCompletionRequest
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
		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}")
		fflush(w)

		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}")
		fflush(w)

		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"!\"},\"finish_reason\":null}]}")
		fflush(w)

		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}")
		fflush(w)

		fmt.Fprintln(w, "data: [DONE]")
		fflush(w)
	}))
	defer server.Close()

	config := ProviderConfig{
		Provider: ProviderOpenAI,
		APIKey:   "test-api-key",
		BaseURL:  server.URL,
		Timeout:  60,
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewOpenAIProvider(config, log)
	require.NoError(t, err)

	req := &CompletionRequest{
		Model: ModelGPT4o,
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

	// Verify content chunks
	contentChunks := 0
	for _, chunk := range receivedChunks {
		if chunk.Delta != "" {
			contentChunks++
		}
	}
	assert.Equal(t, 2, contentChunks, "Should receive 2 content delta chunks")

	// Verify final chunk
	finalChunk := receivedChunks[len(receivedChunks)-1]
	assert.Equal(t, "chatcmpl-123", finalChunk.ID)
	assert.Equal(t, ModelGPT4o, finalChunk.Model)
	assert.Equal(t, ProviderOpenAI, finalChunk.Provider)
	assert.Equal(t, "Hello!", finalChunk.Content)
	assert.True(t, finalChunk.Done)
	assert.NotNil(t, finalChunk.StopReason, "StopReason should not be nil")
	assert.Equal(t, StopReasonEndTurn, *finalChunk.StopReason)
}

func TestOpenAIProvider_CompleteStream_ContentDeltas(t *testing.T) {
	var receivedChunks []*CompletionChunk

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send events
		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-456\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}")

		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-456\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"The\"},\"finish_reason\":null}]}")

		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-456\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" answer\"},\"finish_reason\":null}]}")

		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-456\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" is 42\"},\"finish_reason\":null}]}")

		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-456\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}")

		fmt.Fprintln(w, "data: [DONE]")

		fflush(w)
	}))
	defer server.Close()

	config := ProviderConfig{
		Provider: ProviderOpenAI,
		APIKey:   "test-api-key",
		BaseURL:  server.URL,
		Timeout:  60,
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewOpenAIProvider(config, log)
	require.NoError(t, err)

	req := &CompletionRequest{
		Model: ModelGPT4o,
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

func TestOpenAIProvider_CompleteStream_ValidationError(t *testing.T) {
	provider := createTestOpenAIProvider(t)

	req := &CompletionRequest{
		Model:     ModelClaude3_5Sonnet, // Unsupported model
		Messages:  []Message{{Role: MessageRoleUser, Content: "Hello"}},
		MaxTokens: 100,
	}

	chunkChan, err := provider.CompleteStream(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, chunkChan)
	assert.Contains(t, err.Error(), "not supported")
}

func TestOpenAIProvider_CompleteStream_ClosedProvider(t *testing.T) {
	provider := createTestOpenAIProvider(t)
	provider.Close()

	req := &CompletionRequest{
		Model:     ModelGPT4o,
		Messages:  []Message{{Role: MessageRoleUser, Content: "Hello"}},
		MaxTokens: 100,
	}

	chunkChan, err := provider.CompleteStream(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, chunkChan)
	assert.Contains(t, err.Error(), "provider is closed")
}

func TestOpenAIProvider_CompleteStream_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send initial event
		fmt.Fprintln(w, "data: {\"id\":\"chatcmpl-789\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}")
		fflush(w)

		// Wait before sending more (will be cancelled)
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	config := ProviderConfig{
		Provider: ProviderOpenAI,
		APIKey:   "test-api-key",
		BaseURL:  server.URL,
		Timeout:  60,
	}
	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewOpenAIProvider(config, log)
	require.NoError(t, err)

	req := &CompletionRequest{
		Model: ModelGPT4o,
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

func TestOpenAIProvider_Close(t *testing.T) {
	provider := createTestOpenAIProvider(t)

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

func TestOpenAIProvider_Complete_AfterClose(t *testing.T) {
	provider := createTestOpenAIProvider(t)
	provider.Close()

	req := &CompletionRequest{
		Model: ModelGPT4o,
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

func TestOpenAIProvider_MapStopReason(t *testing.T) {
	provider := createTestOpenAIProvider(t)
	openaiProvider, ok := provider.(*openaiProvider)
	require.True(t, ok, "Provider should be openaiProvider")

	tests := []struct {
		reason string
		want   StopReason
	}{
		{"stop", StopReasonEndTurn},
		{"length", StopReasonMaxTokens},
		{"content_filter", StopReasonStopSequence},
		{"tool_calls", StopReasonToolUse},
		{"unknown", StopReasonEndTurn},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			assert.Equal(t, tt.want, openaiProvider.mapStopReason(tt.reason))
		})
	}
}

func TestOpenAIProvider_Tokens(t *testing.T) {
	t.Run("regular completion records token usage", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := openaiChatCompletionResponse{
				ID:     "chatcmpl-123",
				Object: "chat.completion",
				Created: time.Now().Unix(),
				Model:  "gpt-4o",
				Choices: []openaiChoice{
					{
						Index: 0,
						Message: openaiMessage{
							Role:    "assistant",
							Content: "Hello!",
						},
						FinishReason: stringPtr("stop"),
					},
				},
				Usage: openaiUsage{
					PromptTokens:     100,
					CompletionTokens: 50,
					TotalTokens:      150,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		config := ProviderConfig{
			Provider: ProviderOpenAI,
			APIKey:   "test-api-key",
			BaseURL:  server.URL,
			Timeout:  60,
		}
		log, err := logger.NewDefault()
		require.NoError(t, err)

		provider, err := NewOpenAIProvider(config, log)
		require.NoError(t, err)

		req := &CompletionRequest{
			Model: ModelGPT4o,
			Messages: []Message{
				{Role: MessageRoleUser, Content: "Hello"},
			},
			MaxTokens: 100,
		}

		resp, err := provider.Complete(context.Background(), req)
		require.NoError(t, err)

		// Get the token account via type assertion
		openaiProvider, ok := provider.(*openaiProvider)
		require.True(t, ok, "Provider should be openaiProvider")

		account := openaiProvider.GetTokenAccount()
		require.NotNil(t, account)

		// Verify usage was recorded
		totalUsage := account.GetTotalUsage()
		assert.Equal(t, 100, totalUsage.InputTokens)
		assert.Equal(t, 50, totalUsage.OutputTokens)

		// Verify request count
		assert.Equal(t, int64(1), account.GetRequestCount())

		// Verify response has correct usage
		assert.Equal(t, 100, resp.Usage.InputTokens)
		assert.Equal(t, 50, resp.Usage.OutputTokens)
		assert.Equal(t, 150, resp.Usage.TotalTokens)
	})

	t.Run("token account persists across multiple requests", func(t *testing.T) {
		requestCount := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++

			resp := openaiChatCompletionResponse{
				ID:     fmt.Sprintf("chatcmpl-%d", requestCount),
				Object: "chat.completion",
				Created: time.Now().Unix(),
				Model:  "gpt-4o",
				Choices: []openaiChoice{
					{
						Index: 0,
						Message: openaiMessage{
							Role:    "assistant",
							Content: "Response",
						},
						FinishReason: stringPtr("stop"),
					},
				},
				Usage: openaiUsage{
					PromptTokens:     50 * requestCount,
					CompletionTokens: 25 * requestCount,
					TotalTokens:      75 * requestCount,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		config := ProviderConfig{
			Provider: ProviderOpenAI,
			APIKey:   "test-api-key",
			BaseURL:  server.URL,
			Timeout:  60,
		}
		log, err := logger.NewDefault()
		require.NoError(t, err)

		provider, err := NewOpenAIProvider(config, log)
		require.NoError(t, err)

		req := &CompletionRequest{
			Model: ModelGPT4o,
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
		openaiProvider, ok := provider.(*openaiProvider)
		require.True(t, ok)

		account := openaiProvider.GetTokenAccount()
		require.NotNil(t, account)

		// Verify accumulated usage across all requests
		totalUsage := account.GetTotalUsage()
		assert.Equal(t, 300, totalUsage.InputTokens)  // 50 + 100 + 150
		assert.Equal(t, 150, totalUsage.OutputTokens) // 25 + 50 + 75

		// Verify request count
		assert.Equal(t, int64(3), account.GetRequestCount())

		// Verify average tokens per request
		avgTokens := account.GetAverageTokensPerRequest()
		assert.InDelta(t, 150.0, avgTokens, 0.01) // (300+150)/3 = 150
	})
}

func TestOpenAISSEReader_ParseChunks(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantChunks  int
		wantContent []string
	}{
		{
			name: "single chunk",
			input: "data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\n",
			wantChunks: 1,
			wantContent: []string{"Hello"},
		},
		{
			name: "multiple chunks",
			input: `data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"content":" world"}}]}

data: [DONE]
`,
			wantChunks:  2,
			wantContent: []string{"Hello", " world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newOpenAISSEReader(strings.NewReader(tt.input))

			chunks := []string{}
			for i := 0; i < tt.wantChunks; i++ {
				chunk, err := reader.NextChunk()
				assert.NoError(t, err)
				assert.NotNil(t, chunk)
				if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
					chunks = append(chunks, chunk.Choices[0].Delta.Content)
				}
			}

			// Should return DONE after all chunks
			_, err := reader.NextChunk()
			assert.Equal(t, io.EOF, err)

			assert.Equal(t, tt.wantContent, chunks)
		})
	}
}

// Helper functions

func createTestOpenAIProvider(t *testing.T) LLMProvider {
	t.Helper()

	config := ProviderConfig{
		Provider: ProviderOpenAI,
		APIKey:   "test-api-key",
		BaseURL:  "https://api.openai.com",
		Timeout:  60,
		Models: []Model{
			ModelGPT4o,
			ModelGPT4oMini,
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	provider, err := NewOpenAIProvider(config, log)
	require.NoError(t, err)

	return provider
}
