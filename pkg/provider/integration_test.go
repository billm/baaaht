package provider

import (
	"context"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ProviderSystemEndToEnd performs an end-to-end test of the provider system
// This test validates:
// - Registry initialization and configuration
// - Provider registration and lifecycle
// - Model-based provider selection
// - Failover behavior with circuit breaker
// - Health checking system
// - Token accounting
// - Graceful shutdown
func TestIntegration_ProviderSystemEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err, "Failed to create logger")

	// Step 1: Create provider registry with realistic configuration
	t.Log("=== Step 1: Creating provider registry ===")

	cfg := RegistryConfig{
		DefaultProvider:      ProviderAnthropic,
		FailoverEnabled:      true,
		FailoverThreshold:    3,
		HealthCheckInterval:  30 * time.Second,
		CircuitBreakerTimeout: 100 * time.Millisecond,
		Providers: map[Provider]ProviderConfig{
			ProviderAnthropic: {
				Provider:   ProviderAnthropic,
				Enabled:    true,
				Priority:   10,
				Timeout:    60,
				MaxRetries: 3,
			},
			ProviderOpenAI: {
				Provider:   ProviderOpenAI,
				Enabled:    true,
				Priority:   20,
				Timeout:    60,
				MaxRetries: 3,
			},
		},
	}

	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err, "Failed to create registry")
	require.NotNil(t, registry, "Registry should not be nil")
	require.False(t, registry.IsClosed(), "Registry should not be closed")

	t.Logf("Registry created with default provider: %s", cfg.DefaultProvider)

	// Ensure cleanup
	t.Cleanup(func() {
		t.Log("=== Step 8: Cleaning up registry ===")
		if err := registry.Close(); err != nil {
			t.Logf("Warning: Registry close returned error: %v", err)
		}
		assert.True(t, registry.IsClosed(), "Registry should be closed after cleanup")
	})

	// Step 2: Register providers
	t.Log("=== Step 2: Registering providers ===")

	// Create mock providers that simulate realistic behavior
	anthropicProvider := &mockLLMProvider{
		id:           ProviderAnthropic,
		name:         "Anthropic Claude",
		status:       ProviderStatusAvailable,
		tokenAccount: NewTokenAccount(ProviderAnthropic),
	}

	openaiProvider := &mockLLMProvider{
		id:           ProviderOpenAI,
		name:         "OpenAI GPT",
		status:       ProviderStatusAvailable,
		tokenAccount: NewTokenAccount(ProviderOpenAI),
	}

	err = registry.Register(ctx, anthropicProvider)
	require.NoError(t, err, "Failed to register Anthropic provider")
	t.Log("Anthropic provider registered")

	err = registry.Register(ctx, openaiProvider)
	require.NoError(t, err, "Failed to register OpenAI provider")
	t.Log("OpenAI provider registered")

	// Verify providers are registered
	providers, err := registry.List(ctx)
	require.NoError(t, err, "Failed to list providers")
	require.Len(t, providers, 2, "Should have 2 registered providers")

	// Step 3: Test provider selection by model
	t.Log("=== Step 3: Testing provider selection by model ===")

	// Test Anthropic model selection
	provider, err := registry.GetProviderByModel(ctx, ModelClaude3_5Sonnet)
	require.NoError(t, err, "Should find provider for Claude model")
	require.Equal(t, ProviderAnthropic, provider.Provider(), "Should select Anthropic provider")
	t.Logf("Provider for %s: %s", ModelClaude3_5Sonnet, provider.Name())

	// Test OpenAI model selection
	provider, err = registry.GetProviderByModel(ctx, ModelGPT4o)
	require.NoError(t, err, "Should find provider for GPT-4 model")
	require.Equal(t, ProviderOpenAI, provider.Provider(), "Should select OpenAI provider")
	t.Logf("Provider for %s: %s", ModelGPT4o, provider.Name())

	// Step 4: Test failover behavior
	t.Log("=== Step 4: Testing failover behavior ===")

	// Record failures for Anthropic to trigger circuit breaker
	registry.RecordProviderFailure(ProviderAnthropic, "API timeout")
	registry.RecordProviderFailure(ProviderAnthropic, "Rate limit exceeded")
	registry.RecordProviderFailure(ProviderAnthropic, "Connection error")

	// Verify circuit breaker is open
	state := registry.GetFailoverState(ProviderAnthropic)
	assert.Equal(t, 3, state.ConsecutiveFailures, "Should have 3 consecutive failures")
	assert.True(t, state.CircuitOpen, "Circuit breaker should be open")
	t.Logf("Circuit breaker opened for %s after %d failures", ProviderAnthropic, state.ConsecutiveFailures)

	// Test failover to backup provider
	provider, failoverOccurred, err := registry.GetWithFailover(ctx, ProviderAnthropic)
	require.NoError(t, err, "Failover should succeed")
	assert.True(t, failoverOccurred, "Failover should have occurred")
	assert.Equal(t, ProviderOpenAI, provider.Provider(), "Should failover to OpenAI")
	t.Logf("Failover successful: %s -> %s", ProviderAnthropic, provider.Name())

	// Reset circuit breaker for next tests
	registry.ResetProviderFailover(ProviderAnthropic)
	t.Log("Circuit breaker reset")

	// Step 5: Test health checking
	t.Log("=== Step 5: Testing health checking ===")

	// Set health status
	registry.SetHealthStatus(ProviderAnthropic, HealthStatusHealthy)
	status := registry.GetHealthStatus(ProviderAnthropic)
	assert.Equal(t, HealthStatusHealthy, status, "Anthropic should be healthy")

	// Perform health check
	healthStatus, err := registry.HealthCheck(ctx, ProviderAnthropic)
	require.NoError(t, err, "Health check should succeed")
	assert.Equal(t, HealthStatusHealthy, healthStatus, "Health status should be healthy")
	t.Logf("Health check for %s: %s", ProviderAnthropic, healthStatus)

	// Mark provider as degraded
	registry.SetHealthStatus(ProviderOpenAI, HealthStatusDegraded)
	status = registry.GetHealthStatus(ProviderOpenAI)
	assert.Equal(t, HealthStatusDegraded, status, "OpenAI should be degraded")

	// Get all health statuses
	allStatuses := registry.GetAllHealthStatuses()
	assert.Len(t, allStatuses, 2, "Should have health statuses for 2 providers")
	t.Logf("All health statuses: %+v", allStatuses)

	// Step 6: Test token accounting
	t.Log("=== Step 6: Testing token accounting ===")

	// Simulate API usage
	requestID := types.GenerateID().String()

	// Record token usage for Anthropic
	anthropicProvider.tokenAccount.Record(requestID, ModelClaude3_5Sonnet, TokenUsage{
		InputTokens:     1000,
		OutputTokens:    500,
		CacheReadTokens: 200,
		TotalTokens:     1700,
	})

	// Record token usage for OpenAI
	openaiProvider.tokenAccount.Record(requestID, ModelGPT4o, TokenUsage{
		InputTokens:  800,
		OutputTokens: 400,
		TotalTokens:  1200,
	})

	// Get stats with token accounting
	stats, err := registry.Stats(ctx)
	require.NoError(t, err, "Failed to get stats")
	require.Len(t, stats, 2, "Should have stats for 2 providers")

	anthropicStats := stats[ProviderAnthropic]
	assert.Equal(t, int64(1700), anthropicStats.TotalTokens, "Anthropic total tokens should match")
	assert.Equal(t, int64(200), anthropicStats.CacheReadTokens, "Anthropic cache read tokens should match")
	t.Logf("Anthropic stats: total=%d, input=%d, output=%d, cache_read=%d",
		anthropicStats.TotalTokens,
		anthropicStats.InputTokens,
		anthropicStats.OutputTokens,
		anthropicStats.CacheReadTokens)

	openaiStats := stats[ProviderOpenAI]
	assert.Equal(t, int64(1200), openaiStats.TotalTokens, "OpenAI total tokens should match")
	t.Logf("OpenAI stats: total=%d, input=%d, output=%d",
		openaiStats.TotalTokens,
		openaiStats.InputTokens,
		openaiStats.OutputTokens)

	// Step 7: Test configuration retrieval
	t.Log("=== Step 7: Testing configuration retrieval ===")

	retrievedCfg := registry.Config()
	assert.Equal(t, cfg.DefaultProvider, retrievedCfg.DefaultProvider, "Default provider should match")
	assert.Equal(t, cfg.FailoverEnabled, retrievedCfg.FailoverEnabled, "Failover enabled should match")
	assert.Equal(t, cfg.FailoverThreshold, retrievedCfg.FailoverThreshold, "Failover threshold should match")
	t.Logf("Config retrieved: default=%s, failover=%v, threshold=%d",
		retrievedCfg.DefaultProvider,
		retrievedCfg.FailoverEnabled,
		retrievedCfg.FailoverThreshold)

	// Test complete
	t.Log("=== Provider System Integration Test Complete ===")
	t.Log("All steps passed successfully:")
	t.Log("  1. Registry created and initialized")
	t.Log("  2. Providers registered successfully")
	t.Log("  3. Provider selection by model working")
	t.Log("  4. Failover with circuit breaker working")
	t.Log("  5. Health checking system working")
	t.Log("  6. Token accounting working")
	t.Log("  7. Configuration retrieval working")
	t.Log("  8. Registry cleanup (via cleanup function)")
}

// TestIntegration_ProviderFailoverWorkflow tests a complete failover workflow
func TestIntegration_ProviderFailoverWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Log("=== Testing complete failover workflow ===")

	cfg := RegistryConfig{
		DefaultProvider:      ProviderAnthropic,
		FailoverEnabled:      true,
		FailoverThreshold:    2,
		CircuitBreakerTimeout: 50 * time.Millisecond,
		Providers: map[Provider]ProviderConfig{
			ProviderAnthropic: {Provider: ProviderAnthropic, Enabled: true, Priority: 10},
			ProviderOpenAI:    {Provider: ProviderOpenAI, Enabled: true, Priority: 20},
		},
	}

	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)
	defer registry.Close()

	// Register providers
	anthropic := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Anthropic",
		status: ProviderStatusAvailable,
	}
	openai := &mockLLMProvider{
		id:     ProviderOpenAI,
		name:   "OpenAI",
		status: ProviderStatusAvailable,
	}

	err = registry.Register(ctx, anthropic)
	require.NoError(t, err)
	err = registry.Register(ctx, openai)
	require.NoError(t, err)

	// Initially, primary should be available
	provider, failover, err := registry.GetWithFailover(ctx, ProviderAnthropic)
	require.NoError(t, err)
	assert.False(t, failover, "No failover should occur initially")
	assert.Equal(t, ProviderAnthropic, provider.Provider())
	t.Log("Step 1: Primary provider available - no failover")

	// Record failures to trigger circuit breaker
	registry.RecordProviderFailure(ProviderAnthropic, "error 1")
	registry.RecordProviderFailure(ProviderAnthropic, "error 2")

	state := registry.GetFailoverState(ProviderAnthropic)
	assert.True(t, state.CircuitOpen, "Circuit should be open")
	t.Log("Step 2: Circuit breaker opened after failures")

	// Now failover should occur
	provider, failover, err = registry.GetWithFailover(ctx, ProviderAnthropic)
	require.NoError(t, err)
	assert.True(t, failover, "Failover should occur")
	assert.Equal(t, ProviderOpenAI, provider.Provider())
	t.Log("Step 3: Failover to backup provider successful")

	// Wait for circuit breaker timeout
	time.Sleep(60 * time.Millisecond)

	state = registry.GetFailoverState(ProviderAnthropic)
	assert.False(t, state.CircuitOpen, "Circuit should be closed after timeout")
	t.Log("Step 4: Circuit breaker closed after timeout")

	// Primary should be available again
	provider, failover, err = registry.GetWithFailover(ctx, ProviderAnthropic)
	require.NoError(t, err)
	assert.False(t, failover, "No failover should occur after circuit timeout")
	assert.Equal(t, ProviderAnthropic, provider.Provider())
	t.Log("Step 5: Primary provider available after circuit timeout")

	// Record success should reset failure count
	registry.RecordProviderSuccess(ProviderAnthropic)
	state = registry.GetFailoverState(ProviderAnthropic)
	assert.Equal(t, 0, state.ConsecutiveFailures, "Failures should be reset")
	t.Log("Step 6: Success resets failure count")

	t.Log("Failover workflow test passed")
}

// TestIntegration_ProviderHealthWorkflow tests health checking workflow
func TestIntegration_ProviderHealthWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Log("=== Testing health checking workflow ===")

	cfg := RegistryConfig{
		Providers:       map[Provider]ProviderConfig{},
		FailoverEnabled: true,
	}

	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)
	defer registry.Close()

	// Register providers with different statuses
	healthyProvider := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Healthy Provider",
		status: ProviderStatusAvailable,
	}
	unhealthyProvider := &mockLLMProvider{
		id:     ProviderOpenAI,
		name:   "Unhealthy Provider",
		status: ProviderStatusUnavailable,
	}

	err = registry.Register(ctx, healthyProvider)
	require.NoError(t, err)
	err = registry.Register(ctx, unhealthyProvider)
	require.NoError(t, err)

	// Test health check for healthy provider
	status, err := registry.HealthCheck(ctx, ProviderAnthropic)
	require.NoError(t, err)
	assert.Equal(t, HealthStatusHealthy, status)
	t.Logf("Health check for healthy provider: %s", status)

	// Test health check for unhealthy provider
	status, err = registry.HealthCheck(ctx, ProviderOpenAI)
	require.NoError(t, err)
	assert.Equal(t, HealthStatusUnhealthy, status)
	t.Logf("Health check for unhealthy provider: %s", status)

	// Test setting and getting health status
	registry.SetHealthStatus(ProviderAnthropic, HealthStatusDegraded)
	status = registry.GetHealthStatus(ProviderAnthropic)
	assert.Equal(t, HealthStatusDegraded, status)
	t.Log("Health status set to degraded")

	// Set health status for OpenAI as well
	registry.SetHealthStatus(ProviderOpenAI, HealthStatusUnhealthy)

	// Test IsProviderHealthy
	assert.True(t, registry.IsProviderHealthy(ProviderAnthropic), "Degraded is still healthy")
	assert.False(t, registry.IsProviderHealthy(ProviderOpenAI), "Unhealthy is not healthy")

	// Get all health statuses
	allStatuses := registry.GetAllHealthStatuses()
	assert.Contains(t, allStatuses, ProviderAnthropic)
	assert.Contains(t, allStatuses, ProviderOpenAI)
	assert.Equal(t, HealthStatusDegraded, allStatuses[ProviderAnthropic])
	assert.Equal(t, HealthStatusUnhealthy, allStatuses[ProviderOpenAI])
	t.Logf("All health statuses: %+v", allStatuses)

	t.Log("Health checking workflow test passed")
}

// TestIntegration_ProviderTokenAccountingWorkflow tests token accounting workflow
func TestIntegration_ProviderTokenAccountingWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Log("=== Testing token accounting workflow ===")

	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}

	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)
	defer registry.Close()

	// Register provider with token accounting
	provider := &mockLLMProvider{
		id:           ProviderAnthropic,
		name:         "Test Provider",
		status:       ProviderStatusAvailable,
		tokenAccount: NewTokenAccount(ProviderAnthropic),
	}

	err = registry.Register(ctx, provider)
	require.NoError(t, err)

	// Record multiple requests
	requests := []struct {
		model       Model
		input       int
		output      int
		cacheRead   int
		cacheWrite  int
	}{
		{ModelClaude3_5Sonnet, 1000, 500, 200, 100},
		{ModelClaude3Opus, 1500, 800, 300, 150},
		{ModelClaude3Haiku, 500, 200, 50, 25},
	}

	for i, req := range requests {
		requestID := types.GenerateID().String()
		provider.tokenAccount.Record(requestID, req.model, TokenUsage{
			InputTokens:      req.input,
			OutputTokens:     req.output,
			CacheReadTokens:  req.cacheRead,
			CacheWriteTokens: req.cacheWrite,
			TotalTokens:      req.input + req.output + req.cacheRead + req.cacheWrite,
		})
		t.Logf("Recorded request %d: model=%s, input=%d, output=%d",
			i+1, req.model, req.input, req.output)
	}

	// Get stats
	stats, err := registry.Stats(ctx)
	require.NoError(t, err)

	anthropicStats := stats[ProviderAnthropic]
	assert.Equal(t, int64(3000), anthropicStats.InputTokens, "Total input tokens should match")
	assert.Equal(t, int64(1500), anthropicStats.OutputTokens, "Total output tokens should match")
	assert.Equal(t, int64(550), anthropicStats.CacheReadTokens, "Total cache read tokens should match")
	assert.Equal(t, int64(275), anthropicStats.CacheWriteTokens, "Total cache write tokens should match")

	t.Logf("Token accounting summary:")
	t.Logf("  Input tokens: %d", anthropicStats.InputTokens)
	t.Logf("  Output tokens: %d", anthropicStats.OutputTokens)
	t.Logf("  Total tokens: %d", anthropicStats.TotalTokens)
	t.Logf("  Cache read: %d", anthropicStats.CacheReadTokens)
	t.Logf("  Cache write: %d", anthropicStats.CacheWriteTokens)

	// Get summary
	summary := provider.tokenAccount.GetSummary()
	assert.Equal(t, int64(3), summary.RequestCount, "Should have 3 requests")
	assert.Equal(t, 5325, summary.TotalUsage.TotalTokens, "Total tokens should match (input + output + cache read + cache write)")
	t.Logf("Total requests: %d", summary.RequestCount)
	t.Logf("Total tokens: %d", summary.TotalUsage.TotalTokens)
	t.Logf("Average tokens per request: %.2f", summary.AverageTokensPerRequest)

	t.Log("Token accounting workflow test passed")
}

// TestIntegration_ProviderRegistryLifecycle tests registry lifecycle
func TestIntegration_ProviderRegistryLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Log("=== Testing registry lifecycle ===")

	// Create registry
	cfg := RegistryConfig{
		DefaultProvider: ProviderAnthropic,
		Providers:       map[Provider]ProviderConfig{},
	}

	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, registry)
	assert.False(t, registry.IsClosed())
	t.Log("Registry created")

	// Register provider
	provider := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Test",
		status: ProviderStatusAvailable,
		closed: false,
	}

	err = registry.Register(ctx, provider)
	require.NoError(t, err)
	t.Log("Provider registered")

	// Verify provider is available
	retrieved, err := registry.Get(ctx, ProviderAnthropic)
	require.NoError(t, err)
	assert.Equal(t, provider, retrieved)
	t.Log("Provider retrieved successfully")

	// Close registry
	err = registry.Close()
	require.NoError(t, err)
	assert.True(t, registry.IsClosed())
	assert.True(t, provider.closed, "Provider should be closed")
	t.Log("Registry closed")

	// Operations after close should fail
	_, err = registry.Get(ctx, ProviderAnthropic)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "registry is closed")
	t.Log("Operations correctly fail after close")

	// Double close should be safe
	err = registry.Close()
	assert.NoError(t, err)
	t.Log("Double close is safe")

	t.Log("Registry lifecycle test passed")
}

// TestIntegration_ProviderConfiguration tests configuration management
func TestIntegration_ProviderConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_, err := logger.NewDefault()
	require.NoError(t, err)

	t.Log("=== Testing provider configuration ===")

	// Test default configuration
	defaultCfg := DefaultConfig()
	assert.Equal(t, ProviderAnthropic, defaultCfg.DefaultProvider)
	assert.True(t, defaultCfg.FailoverEnabled)
	assert.Equal(t, 3, defaultCfg.FailoverThreshold)
	t.Log("Default configuration loaded")

	// Test configuration validation
	validCfg := RegistryConfig{
		DefaultProvider: ProviderAnthropic,
		Providers: map[Provider]ProviderConfig{
			ProviderAnthropic: {
				Provider:   ProviderAnthropic,
				BaseURL:    DefaultAnthropicBaseURL,
				Enabled:    true,
				Priority:   10,
				Timeout:    60,
				MaxRetries: 3,
				Models:     []Model{ModelClaude3_5Sonnet},
			},
		},
		FailoverEnabled:        true,
		FailoverThreshold:      3,
		CircuitBreakerTimeout:  60 * time.Second,
		HealthCheckInterval:    30 * time.Second,
	}

	err = validCfg.Validate()
	assert.NoError(t, err, "Valid configuration should pass")
	t.Log("Valid configuration validated")

	// Test invalid configuration
	invalidCfg := RegistryConfig{
		DefaultProvider: Provider("invalid"),
		Providers:       map[Provider]ProviderConfig{},
	}

	err = invalidCfg.Validate()
	assert.Error(t, err, "Invalid configuration should fail")
	t.Log("Invalid configuration correctly rejected")

	// Test ApplyDefaults
	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}
	cfg.ApplyDefaults()
	assert.NotEmpty(t, cfg.Providers, "ApplyDefaults should populate providers")
	assert.NotZero(t, cfg.FailoverThreshold, "ApplyDefaults should set failover threshold")
	t.Log("ApplyDefaults working correctly")

	// Test GetEnabledProviders
	enabled := validCfg.GetEnabledProviders()
	assert.NotEmpty(t, enabled, "Should have enabled providers")
	t.Logf("Enabled providers: %d", len(enabled))

	t.Log("Provider configuration test passed")
}

// TestIntegration_ProviderModelSupport tests model support functionality
func TestIntegration_ProviderModelSupport(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err)
	_ = log // Use log to avoid unused variable error

	t.Log("=== Testing model support ===")

	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)
	defer registry.Close()

	// Register Anthropic provider
	anthropic := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Anthropic",
		status: ProviderStatusAvailable,
	}
	err = registry.Register(ctx, anthropic)
	require.NoError(t, err)

	// Test model support
	assert.True(t, anthropic.SupportsModel(ModelClaude3_5Sonnet))
	assert.True(t, anthropic.SupportsModel(ModelClaude3Opus))
	assert.False(t, anthropic.SupportsModel(ModelGPT4o))
	t.Log("Model support checks working")

	// Test model info
	info, err := anthropic.ModelInfo(ModelClaude3_5Sonnet)
	require.NoError(t, err)
	assert.Equal(t, ModelClaude3_5Sonnet, info.ID)
	assert.Equal(t, ProviderAnthropic, info.Provider)
	t.Logf("Model info: name=%s, context_size=%d", info.Name, info.ContextSize)

	// Test list models
	models, err := anthropic.ListModels()
	require.NoError(t, err)
	assert.NotEmpty(t, models)
	t.Logf("Available models: %d", len(models))

	t.Log("Model support test passed")
}
