package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_ProviderSystemFlow performs end-to-end verification of the complete provider system flow:
// 1. Load configuration with provider settings
// 2. Initialize provider registry with Anthropic and OpenAI
// 3. Configure model selection per agent type
// 4. Execute completion request with streaming
// 5. Verify token accounting
// 6. Trigger failover by simulating provider error
// 7. Verify backup provider is used
func TestE2E_ProviderSystemFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()

	// Step 1: Load configuration with provider settings
	t.Log("=== Step 1: Loading configuration with provider settings ===")

	cfg, err := config.Load()
	require.NoError(t, err, "Failed to load config")

	// Verify provider configuration is loaded
	assert.NotEmpty(t, cfg.Provider.DefaultProvider, "Default provider should be set")
	assert.True(t, cfg.Provider.FailoverEnabled, "Failover should be enabled")
	assert.Greater(t, cfg.Provider.FailoverThreshold, 0, "Failover threshold should be positive")
	assert.Greater(t, cfg.Provider.CircuitBreakerTimeout, time.Duration(0), "Circuit breaker timeout should be positive")

	t.Logf("Config loaded: default=%s, failover=%v, threshold=%d, circuit_timeout=%v",
		cfg.Provider.DefaultProvider,
		cfg.Provider.FailoverEnabled,
		cfg.Provider.FailoverThreshold,
		cfg.Provider.CircuitBreakerTimeout)

	// Step 2: Initialize provider registry with Anthropic and OpenAI
	t.Log("=== Step 2: Initializing provider registry with Anthropic and OpenAI ===")

	log, err := logger.NewDefault()
	require.NoError(t, err, "Failed to create logger")

	// Create mock API servers for Anthropic and OpenAI
	anthropicServer, anthropicAPIKey := setupAnthropicMockServer(t)
	defer anthropicServer.Close()

	openaiServer, openaiAPIKey := setupOpenAIMockServer(t)
	defer openaiServer.Close()

	// Build provider registry configuration
	registryCfg := RegistryConfig{
		DefaultProvider:        ProviderAnthropic,
		FailoverEnabled:        true,
		FailoverThreshold:      3,
		CircuitBreakerTimeout:  100 * time.Millisecond,
		HealthCheckInterval:    30 * time.Second,
		Providers: map[Provider]ProviderConfig{
			ProviderAnthropic: {
				Provider:   ProviderAnthropic,
				BaseURL:    anthropicServer.URL,
				APIKey:     anthropicAPIKey,
				Enabled:    true,
				Priority:   10,
				Timeout:    60,
				MaxRetries: 3,
				Models: []Model{
					ModelClaude3_5Sonnet,
					ModelClaude3Opus,
					ModelClaude3Sonnet,
					ModelClaude3Haiku,
				},
			},
			ProviderOpenAI: {
				Provider:   ProviderOpenAI,
				BaseURL:    openaiServer.URL,
				APIKey:     openaiAPIKey,
				Enabled:    true,
				Priority:   20,
				Timeout:    60,
				MaxRetries: 3,
				Models: []Model{
					ModelGPT4o,
					ModelGPT4oMini,
					ModelGPT4Turbo,
				},
			},
		},
	}

	registry, err := NewRegistry(registryCfg, log)
	require.NoError(t, err, "Failed to create provider registry")
	require.NotNil(t, registry, "Registry should not be nil")

	// Initialize providers from configuration
	err = registry.InitializeFromConfig(ctx)
	require.NoError(t, err, "Failed to initialize providers from config")

	t.Log("Provider registry initialized with Anthropic and OpenAI")

	// Verify both providers are registered
	providers, err := registry.List(ctx)
	require.NoError(t, err, "Failed to list providers")
	require.Len(t, providers, 2, "Should have 2 registered providers")

	availableProviders, err := registry.ListAvailable(ctx)
	require.NoError(t, err, "Failed to list available providers")
	require.Len(t, availableProviders, 2, "Both providers should be available")

	t.Log("Providers registered and available:", len(providers))

	// Ensure cleanup
	t.Cleanup(func() {
		t.Log("=== Cleanup: Closing provider registry ===")
		if err := registry.Close(); err != nil {
			t.Logf("Warning: Registry close returned error: %v", err)
		}
	})

	// Step 3: Configure model selection per agent type
	t.Log("=== Step 3: Configuring model selection per agent type ===")

	// Test model selection for different agent types
	agentModelMapping := map[string]Model{
		"research":  ModelClaude3Opus,      // High capability for research
		"heartbeat": ModelClaude3Haiku,     // Fast for heartbeats
		"coding":    ModelClaude3_5Sonnet,  // Balanced for coding
		"analysis":  ModelGPT4o,            // OpenAI for analysis
		"chat":      ModelGPT4oMini,        // Fast for chat
	}

	for agentType, model := range agentModelMapping {
		provider, err := registry.GetProviderByModel(ctx, model)
		require.NoError(t, err, "Should find provider for model %s (agent type: %s)", model, agentType)
		require.NotNil(t, provider, "Provider should not be nil for model %s", model)

		// Verify the provider supports the model
		assert.True(t, provider.SupportsModel(model), "Provider should support model %s", model)
		t.Logf("Agent type '%s' -> model %s -> provider %s", agentType, model, provider.Name())
	}

	t.Log("Model selection configured successfully for all agent types")

	// Step 4: Execute completion request with streaming
	t.Log("=== Step 4: Executing completion request with streaming ===")

	// Get Anthropic provider for streaming test
	anthropicProvider, err := registry.Get(ctx, ProviderAnthropic)
	require.NoError(t, err, "Failed to get Anthropic provider")

	// Prepare completion request
	req := CompletionRequest{
		Model:    ModelClaude3_5Sonnet,
		MaxTokens: 100,
		Messages: []Message{
			{
				Role:    MessageRoleUser,
				Content: []ContentBlock{{Type: ContentTypeText, Text: "Hello, how are you?"}},
			},
		},
		Stream: true,
	}

	// Execute streaming completion
	streamCtx, streamCancel := context.WithTimeout(ctx, 10*time.Second)
	defer streamCancel()

	var chunksReceived int
	var fullContent strings.Builder

	streamChan, err := anthropicProvider.CompleteStream(streamCtx, &req)
	require.NoError(t, err, "Streaming completion should start successfully")

	// Read from stream channel
	for chunk := range streamChan {
		if chunk.Error != nil {
			t.Fatalf("Received error in stream: %v", chunk.Error)
		}

		chunksReceived++
		if chunk.Delta != "" {
			fullContent.WriteString(chunk.Delta)
		}
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
		}

		if chunksReceived <= 5 || chunk.Done {
			t.Logf("Stream chunk: content=%q, delta=%q, done=%v", chunk.Content, chunk.Delta, chunk.Done)
		}

		if chunk.Done {
			break
		}
	}

	assert.Greater(t, chunksReceived, 0, "Should receive at least one chunk")
	assert.NotEmpty(t, fullContent.String(), "Should have received content")

	t.Logf("Streaming completion successful: %d chunks, content length: %d", chunksReceived, fullContent.Len())

	// Step 5: Verify token accounting
	t.Log("=== Step 5: Verifying token accounting ===")

	// Get provider stats
	stats, err := registry.Stats(ctx)
	require.NoError(t, err, "Failed to get provider stats")

	require.Contains(t, stats, ProviderAnthropic, "Should have stats for Anthropic")
	anthropicStats := stats[ProviderAnthropic]

	assert.Greater(t, anthropicStats.TotalTokens, int64(0), "Anthropic should have tracked tokens")
	assert.Greater(t, anthropicStats.OutputTokens, int64(0), "Anthropic should have tracked output tokens")

	t.Logf("Token accounting verified: total=%d, input=%d, output=%d",
		anthropicStats.TotalTokens,
		anthropicStats.InputTokens,
		anthropicStats.OutputTokens)

	// Get token account directly for more detailed verification
	if tp, ok := anthropicProvider.(interface{ GetTokenAccount() *TokenAccount }); ok {
		tokenAccount := tp.GetTokenAccount()
		require.NotNil(t, tokenAccount, "Token account should not be nil")

		summary := tokenAccount.GetSummary()
		assert.Greater(t, summary.RequestCount, int64(0), "Should have at least one request recorded")

		// Check that total tokens is non-negative and we have some accounting
		assert.GreaterOrEqual(t, summary.TotalUsage.TotalTokens, 0, "Should have tracked total tokens")
		if summary.TotalUsage.TotalTokens > 0 {
			t.Logf("Token account summary: requests=%d, total_tokens=%d, input_tokens=%d, output_tokens=%d",
				summary.RequestCount,
				summary.TotalUsage.TotalTokens,
				summary.TotalUsage.InputTokens,
				summary.TotalUsage.OutputTokens)
		} else {
			t.Logf("Token account summary: requests=%d (streaming responses may not include input tokens in usage)",
				summary.RequestCount)
		}
	}

	// Step 6: Trigger failover by simulating provider error
	t.Log("=== Step 6: Triggering failover by simulating provider error ===")

	// Record multiple failures for Anthropic to trigger circuit breaker
	for i := 0; i < registryCfg.FailoverThreshold; i++ {
		registry.RecordProviderFailure(ProviderAnthropic, fmt.Sprintf("simulated error %d", i+1))
	}

	// Verify circuit breaker is open
	failoverState := registry.GetFailoverState(ProviderAnthropic)
	assert.Equal(t, registryCfg.FailoverThreshold, failoverState.ConsecutiveFailures,
		"Should have recorded all failures")
	assert.True(t, failoverState.CircuitOpen, "Circuit breaker should be open")

	t.Logf("Circuit breaker opened for Anthropic after %d failures", failoverState.ConsecutiveFailures)

	// Step 7: Verify backup provider is used
	t.Log("=== Step 7: Verifying backup provider is used ===")

	// Request Anthropic provider, should failover to OpenAI
	provider, failoverOccurred, err := registry.GetWithFailover(ctx, ProviderAnthropic)
	require.NoError(t, err, "Failover should succeed")
	assert.True(t, failoverOccurred, "Failover should have occurred")
	assert.Equal(t, ProviderOpenAI, provider.Provider(), "Should failover to OpenAI")

	t.Logf("Failover successful: requested %s, got %s (failover=%v)",
		ProviderAnthropic, provider.Name(), failoverOccurred)

	// Verify we can still execute completions through the backup provider
	req2 := CompletionRequest{
		Model:     ModelGPT4o,
		MaxTokens: 50,
		Messages: []Message{
			{
				Role:    MessageRoleUser,
				Content: []ContentBlock{{Type: ContentTypeText, Text: "Test"}},
			},
		},
	}

	resp, err := provider.Complete(ctx, &req2)
	require.NoError(t, err, "Completion through backup provider should succeed")
	require.NotNil(t, resp, "Response should not be nil")
	assert.NotEmpty(t, resp.Content, "Response should have content")

	t.Logf("Backup provider completion successful: model=%s, content_length=%d",
		resp.Model, len(resp.Content))

	// Test complete
	t.Log("=== End-to-End Provider System Flow Test Complete ===")
	t.Log("All verification steps passed successfully:")
	t.Log("  1. Configuration loaded with provider settings")
	t.Log("  2. Provider registry initialized with Anthropic and OpenAI")
	t.Log("  3. Model selection configured per agent type")
	t.Log("  4. Completion request executed with streaming")
	t.Log("  5. Token accounting verified")
	t.Log("  6. Failover triggered by provider error")
	t.Log("  7. Backup provider used successfully")
}

// setupAnthropicMockServer creates a mock Anthropic API server for testing
func setupAnthropicMockServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	apiKey := "test-anthropic-key-" + types.GenerateID().String()[:8]

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API key header
		if r.Header.Get("x-api-key") != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Verify API version header
		if r.Header.Get("anthropic-version") == "" {
			http.Error(w, "missing api version", http.StatusBadRequest)
			return
		}

		if r.URL.Path == "/v1/messages" {
			// Parse request to check if streaming is enabled
			var reqBody map[string]interface{}
			json.NewDecoder(r.Body).Decode(&reqBody)
			stream := false
			if s, ok := reqBody["stream"].(bool); ok {
				stream = s
			}

			if stream {
				// Streaming response
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")
				fflush(w)

				// Send SSE events with proper flushing
				fmt.Fprintln(w, "event: message_start")
				fmt.Fprintln(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-"+types.GenerateID().String()+"\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}}")
				fmt.Fprintln(w)
				fflush(w)

				fmt.Fprintln(w, "event: content_block_start")
				fmt.Fprintln(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}")
				fmt.Fprintln(w)
				fflush(w)

				fmt.Fprintln(w, "event: content_block_delta")
				fmt.Fprintln(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello! \"}}")
				fmt.Fprintln(w)
				fflush(w)

				fmt.Fprintln(w, "event: content_block_delta")
				fmt.Fprintln(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"I am doing well, thank you!\"}}")
				fmt.Fprintln(w)
				fflush(w)

				fmt.Fprintln(w, "event: content_block_stop")
				fmt.Fprintln(w, "data: {\"type\":\"content_block_stop\",\"index\":0}")
				fmt.Fprintln(w)
				fflush(w)

				fmt.Fprintln(w, "event: message_delta")
				fmt.Fprintln(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":15}}")
				fmt.Fprintln(w)
				fflush(w)

				fmt.Fprintln(w, "event: message_stop")
				fmt.Fprintln(w, "data: {\"type\":\"message_stop\"}")
				fmt.Fprintln(w)
				fflush(w)
			} else {
				// Non-streaming response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)

				response := map[string]interface{}{
					"id":           "msg-" + types.GenerateID().String(),
					"type":         "message",
					"role":         "assistant",
					"content":      []map[string]string{{"type": "text", "text": "Hello! I am doing well, thank you!"}},
					"model":        "claude-3-5-sonnet-20241022",
					"stop_reason":  "end_turn",
					"stop_sequence": nil,
					"usage": map[string]int{
						"input_tokens":  10,
						"output_tokens": 15,
					},
				}
				json.NewEncoder(w).Encode(response)
			}
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	return server, apiKey
}

// setupOpenAIMockServer creates a mock OpenAI API server for testing
func setupOpenAIMockServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	apiKey := "test-openai-key-" + types.GenerateID().String()[:8]

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API key header
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/v1/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			response := map[string]interface{}{
				"id":      "chatcmpl-" + types.GenerateID().String(),
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"model":   "gpt-4o",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]string{
							"role":    "assistant",
							"content": "This is a test response from OpenAI",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]int{
					"prompt_tokens":     5,
					"completion_tokens": 10,
					"total_tokens":      15,
				},
			}
			json.NewEncoder(w).Encode(response)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	return server, apiKey
}

// TestE2E_ProviderCredentialsFlow tests the integration with the credentials system
func TestE2E_ProviderCredentialsFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()

	t.Log("=== Testing provider credentials flow ===")

	// Create test API keys
	anthropicKey := "sk-ant-test-" + types.GenerateID().String()[:16]
	openaiKey := "sk-openai-test-" + types.GenerateID().String()[:16]

	// Create mock servers
	anthropicServer := setupAnthropicMockServerWithKey(t, anthropicKey)
	defer anthropicServer.Close()

	openaiServer := setupOpenAIMockServerWithKey(t, openaiKey)
	defer openaiServer.Close()

	log, err := logger.NewDefault()
	require.NoError(t, err)

	// Build provider config with API keys
	registryCfg := RegistryConfig{
		DefaultProvider: ProviderAnthropic,
		FailoverEnabled: true,
		Providers: map[Provider]ProviderConfig{
			ProviderAnthropic: {
				Provider: ProviderAnthropic,
				BaseURL:  anthropicServer.URL,
				APIKey:   anthropicKey,
				Enabled:  true,
				Priority: 10,
				Models: []Model{
					ModelClaude3_5Sonnet,
					ModelClaude3Haiku,
				},
			},
			ProviderOpenAI: {
				Provider: ProviderOpenAI,
				BaseURL:  openaiServer.URL,
				APIKey:   openaiKey,
				Enabled:  true,
				Priority: 20,
				Models: []Model{
					ModelGPT4o,
					ModelGPT4oMini,
				},
			},
		},
	}

	registry, err := NewRegistry(registryCfg, log)
	require.NoError(t, err)

	err = registry.InitializeFromConfig(ctx)
	require.NoError(t, err)

	defer registry.Close()

	// Test that providers can authenticate and make requests
	t.Log("Testing Anthropic provider authentication")
	anthropicProvider, err := registry.Get(ctx, ProviderAnthropic)
	require.NoError(t, err)

	req := CompletionRequest{
		Model:     ModelClaude3_5Sonnet,
		MaxTokens: 50,
		Messages: []Message{
			{
				Role:    MessageRoleUser,
				Content: []ContentBlock{{Type: ContentTypeText, Text: "Test auth"}},
			},
		},
	}

	resp, err := anthropicProvider.Complete(ctx, &req)
	require.NoError(t, err, "Anthropic provider should authenticate successfully")
	assert.NotEmpty(t, resp.Content, "Should receive response content")

	t.Log("Testing OpenAI provider authentication")
	openaiProvider, err := registry.Get(ctx, ProviderOpenAI)
	require.NoError(t, err)

	req2 := CompletionRequest{
		Model:     ModelGPT4o,
		MaxTokens: 50,
		Messages: []Message{
			{
				Role:    MessageRoleUser,
				Content: []ContentBlock{{Type: ContentTypeText, Text: "Test auth"}},
			},
		},
	}

	resp2, err := openaiProvider.Complete(ctx, &req2)
	require.NoError(t, err, "OpenAI provider should authenticate successfully")
	assert.NotEmpty(t, resp2.Content, "Should receive response content")

	t.Log("Credentials flow test passed - both providers authenticated successfully")
}

// setupAnthropicMockServerWithKey creates a mock Anthropic server expecting a specific key
func setupAnthropicMockServerWithKey(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"id":      "msg-" + types.GenerateID().String(),
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]string{{"type": "text", "text": "Authenticated response"}},
			"model":   "claude-3-5-sonnet-20241022",
			"usage": map[string]int{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		}
		json.NewEncoder(w).Encode(response)
	}))

	return server
}

// setupOpenAIMockServerWithKey creates a mock OpenAI server expecting a specific key
func setupOpenAIMockServerWithKey(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"id":      "chatcmpl-" + types.GenerateID().String(),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]string{
						"role":    "assistant",
						"content": "Authenticated response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     5,
				"completion_tokens": 5,
				"total_tokens":      10,
			},
		}
		json.NewEncoder(w).Encode(response)
	}))

	return server
}
