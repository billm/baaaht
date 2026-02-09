package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
				// Streaming will be true after subtask-2-2
				assert.False(t, info.Capabilities.Streaming)
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

func TestAnthropicProvider_CompleteStream_NotImplemented(t *testing.T) {
	provider := createTestAnthropicProvider(t)

	req := &CompletionRequest{
		Model: ModelClaude3_5Sonnet,
		Messages: []Message{
			{Role: MessageRoleUser, Content: "Hello"},
		},
		MaxTokens: 100,
	}

	chunk, err := provider.CompleteStream(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, chunk)
	assert.Contains(t, err.Error(), "streaming not yet implemented")
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
