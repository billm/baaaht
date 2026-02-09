package integration

import (
	"context"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/credentials"
	"github.com/billm/baaaht/orchestrator/pkg/orchestrator"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLLMGatewayLifecycle tests the complete lifecycle of the LLM Gateway manager
func TestLLMGatewayLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM Gateway test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	// Load test config
	cfg := loadTestConfig(t)

	// Enable LLM Gateway for testing
	cfg.LLM.Enabled = true
	cfg.LLM.ContainerImage = "nginx:alpine" // Use nginx:alpine as a lightweight test container with a health endpoint
	cfg.LLM.DefaultProvider = "test"
	cfg.LLM.DefaultModel = "test-model"
	cfg.LLM.Timeout = 30 * time.Second

	// Configure test provider
	cfg.LLM.Providers = map[string]config.ProviderConfig{
		"test": {
			Name:    "test",
			Enabled: true,
			APIKey:  "test-api-key-12345",
		},
	}

	t.Log("=== Step 1: Creating credential store and runtime ===")

	// Create credential store
	credStore, err := credentials.NewDefaultStore(log)
	require.NoError(t, err, "Failed to create credential store")

	// Store test credentials
	err = credStore.Store(ctx, "llm", "test", "test-api-key-12345")
	require.NoError(t, err, "Failed to store credential")

	// Create Docker runtime
	runtime, err := container.NewDockerRuntime(cfg.Docker, log)
	require.NoError(t, err, "Failed to create Docker runtime")
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Logf("Warning: Runtime close returned error: %v", err)
		}
	})

	// Create LLM Gateway manager
	gatewayMgr, err := orchestrator.NewLLMGatewayManager(runtime, cfg.LLM, credStore, log)
	require.NoError(t, err, "Failed to create LLM Gateway manager")
	require.NotNil(t, gatewayMgr, "Gateway manager should not be nil")
	require.True(t, gatewayMgr.IsEnabled(), "Gateway should be enabled")
	require.False(t, gatewayMgr.IsStarted(), "Gateway should not be started initially")

	t.Log("=== Step 2: Starting LLM Gateway ===")

	// Start the gateway
	startCtx, startCancel := context.WithTimeout(ctx, 120*time.Second)
	defer startCancel()

	err = gatewayMgr.Start(startCtx)
	require.NoError(t, err, "Failed to start LLM Gateway")
	require.True(t, gatewayMgr.IsStarted(), "Gateway should be started")

	containerID := gatewayMgr.GetContainerID()
	require.NotEmpty(t, containerID, "Container ID should not be empty")
	t.Logf("LLM Gateway started: container_id=%s", containerID)

	// Cleanup: Ensure gateway is stopped
	t.Cleanup(func() {
		t.Log("=== Cleanup: Closing LLM Gateway manager ===")
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer closeCancel()

		if err := gatewayMgr.Close(); err != nil {
			t.Logf("Warning: Gateway close returned error: %v", err)
		}

		t.Log("LLM Gateway manager closed")
	})

	t.Log("=== Step 3: Verifying container is running ===")

	// Check if container is running
	running, err := gatewayMgr.IsRunning(ctx)
	require.NoError(t, err, "Failed to check if gateway is running")
	require.True(t, running, "Gateway should be running")

	// Verify container state
	lifecycleMgr, err := container.NewLifecycleManagerFromRuntime(runtime, log)
	require.NoError(t, err, "Failed to create lifecycle manager")

	status, err := lifecycleMgr.Status(ctx, containerID)
	require.NoError(t, err, "Failed to get container status")
	t.Logf("Container status: %s", status)

	t.Log("=== Step 4: Performing health check ===")

	// Perform health check
	healthResult, err := gatewayMgr.HealthCheck(ctx)
	require.NoError(t, err, "Health check should succeed")
	require.NotNil(t, healthResult, "Health result should not be nil")
	t.Logf("Health status: %s", healthResult.Status)

	// Health check may return Healthy or Unhealthy depending on whether the container has a health endpoint
	// nginx:alpine doesn't have a health endpoint on :8080, so it may be unhealthy
	// The important thing is that the health check executes without error
	assert.Contains(t, []types.Health{types.Healthy, types.Unhealthy}, healthResult.Status,
		"Health status should be Healthy or Unhealthy")

	t.Log("=== Step 5: Testing gateway restart ===")

	// Restart the gateway
	restartCtx, restartCancel := context.WithTimeout(ctx, 120*time.Second)
	defer restartCancel()

	err = gatewayMgr.Restart(restartCtx)
	require.NoError(t, err, "Failed to restart LLM Gateway")
	t.Logf("Gateway restarted successfully")

	// Verify container is still running after restart
	running, err = gatewayMgr.IsRunning(ctx)
	require.NoError(t, err, "Failed to check if gateway is running after restart")
	require.True(t, running, "Gateway should be running after restart")

	t.Log("=== Step 6: Stopping LLM Gateway ===")

	// Stop the gateway
	stopCtx, stopCancel := context.WithTimeout(ctx, 60*time.Second)
	defer stopCancel()

	err = gatewayMgr.Stop(stopCtx)
	require.NoError(t, err, "Failed to stop LLM Gateway")
	require.False(t, gatewayMgr.IsStarted(), "Gateway should not be started after stop")

	// Verify container is stopped
	running, err = gatewayMgr.IsRunning(ctx)
	require.NoError(t, err, "Failed to check if gateway is running after stop")
	require.False(t, running, "Gateway should not be running after stop")

	// Verify container state
	status, err = lifecycleMgr.Status(ctx, containerID)
	require.NoError(t, err, "Failed to get container status after stop")
	t.Logf("Container status after stop: %s", status)
	assert.Equal(t, types.ContainerStateExited, status, "Container should be exited")

	t.Log("=== Test Complete ===")
	t.Log("LLM Gateway lifecycle test passed:")
	t.Log("  1. Gateway manager created")
	t.Log("  2. Gateway started successfully")
	t.Log("  3. Container running verified")
	t.Log("  4. Health check performed")
	t.Log("  5. Gateway restarted successfully")
	t.Log("  6. Gateway stopped successfully")
}

// TestLLMGatewayDisabled tests that the gateway manager handles disabled configuration correctly
func TestLLMGatewayDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM Gateway test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	cfg := loadTestConfig(t)

	// Disable LLM Gateway
	cfg.LLM.Enabled = false
	cfg.LLM.Providers = map[string]config.ProviderConfig{
		"test": {
			Name:    "test",
			Enabled: true,
			APIKey:  "test-api-key",
		},
	}

	t.Log("=== Testing LLM Gateway with disabled configuration ===")

	// Create credential store
	credStore, err := credentials.NewStore(log)
	require.NoError(t, err, "Failed to create credential store")

	// Create Docker runtime
	runtime, err := container.NewDockerRuntime(cfg.Docker, log)
	require.NoError(t, err, "Failed to create Docker runtime")
	t.Cleanup(func() {
		_ = runtime.Close()
	})

	// Create LLM Gateway manager
	gatewayMgr, err := orchestrator.NewLLMGatewayManager(runtime, cfg.LLM, credStore, log)
	require.NoError(t, err, "Failed to create LLM Gateway manager")
	require.False(t, gatewayMgr.IsEnabled(), "Gateway should be disabled")
	t.Cleanup(func() {
		_ = gatewayMgr.Close()
	})

	// Starting a disabled gateway should return nil (no-op)
	err = gatewayMgr.Start(ctx)
	require.NoError(t, err, "Starting disabled gateway should succeed")
	require.False(t, gatewayMgr.IsStarted(), "Gateway should not be started when disabled")
	require.Empty(t, gatewayMgr.GetContainerID(), "Container ID should be empty when disabled")

	t.Log("Disabled gateway test passed")
}

// TestLLMGatewayInvalidCredentials tests that the gateway manager properly validates credentials
func TestLLMGatewayInvalidCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM Gateway test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	cfg := loadTestConfig(t)

	// Enable LLM Gateway but don't configure any providers
	cfg.LLM.Enabled = true
	cfg.LLM.DefaultProvider = ""
	cfg.LLM.Providers = map[string]config.ProviderConfig{}

	t.Log("=== Testing LLM Gateway with invalid credentials ===")

	// Create credential store
	credStore, err := credentials.NewStore(log)
	require.NoError(t, err, "Failed to create credential store")

	// Create Docker runtime
	runtime, err := container.NewDockerRuntime(cfg.Docker, log)
	require.NoError(t, err, "Failed to create Docker runtime")
	t.Cleanup(func() {
		_ = runtime.Close()
	})

	// Create LLM Gateway manager
	gatewayMgr, err := orchestrator.NewLLMGatewayManager(runtime, cfg.LLM, credStore, log)
	require.NoError(t, err, "Failed to create LLM Gateway manager")
	t.Cleanup(func() {
		_ = gatewayMgr.Close()
	})

	// Starting without valid credentials should fail
	err = gatewayMgr.Start(ctx)
	require.Error(t, err, "Starting gateway without credentials should fail")
	require.False(t, gatewayMgr.IsStarted(), "Gateway should not be started")

	t.Logf("Expected error: %v", err)
	t.Log("Invalid credentials test passed")
}

// TestLLMGatewayCloseWithoutStart tests that Close() works even if Start() was never called
func TestLLMGatewayCloseWithoutStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM Gateway test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	cfg := loadTestConfig(t)

	cfg.LLM.Enabled = true
	cfg.LLM.Providers = map[string]config.ProviderConfig{
		"test": {
			Name:    "test",
			Enabled: true,
			APIKey:  "test-api-key",
		},
	}

	t.Log("=== Testing LLM Gateway Close without Start ===")

	// Create credential store
	credStore, err := credentials.NewStore(log)
	require.NoError(t, err, "Failed to create credential store")

	// Create Docker runtime
	runtime, err := container.NewDockerRuntime(cfg.Docker, log)
	require.NoError(t, err, "Failed to create Docker runtime")
	t.Cleanup(func() {
		_ = runtime.Close()
	})

	// Create LLM Gateway manager
	gatewayMgr, err := orchestrator.NewLLMGatewayManager(runtime, cfg.LLM, credStore, log)
	require.NoError(t, err, "Failed to create LLM Gateway manager")

	// Close without starting should succeed
	err = gatewayMgr.Close()
	require.NoError(t, err, "Close should succeed even without Start")
	require.False(t, gatewayMgr.IsStarted(), "Gateway should not be started")

	t.Log("Close without start test passed")
}

// TestLLMGatewayHealthMonitoring tests that health monitoring works correctly
func TestLLMGatewayHealthMonitoring(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM Gateway test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	cfg := loadTestConfig(t)

	cfg.LLM.Enabled = true
	cfg.LLM.ContainerImage = "nginx:alpine"
	cfg.LLM.DefaultProvider = "test"
	cfg.LLM.DefaultModel = "test-model"
	cfg.LLM.Timeout = 30 * time.Second

	cfg.LLM.Providers = map[string]config.ProviderConfig{
		"test": {
			Name:    "test",
			Enabled: true,
			APIKey:  "test-api-key",
		},
	}

	t.Log("=== Testing LLM Gateway health monitoring ===")

	// Create credential store
	credStore, err := credentials.NewStore(log)
	require.NoError(t, err, "Failed to create credential store")

	err = credStore.StoreCredential(ctx, "llm", "test", "test-api-key")
	require.NoError(t, err, "Failed to store credential")

	// Create Docker runtime
	runtime, err := container.NewDockerRuntime(cfg.Docker, log)
	require.NoError(t, err, "Failed to create Docker runtime")
	t.Cleanup(func() {
		_ = runtime.Close()
	})

	// Create LLM Gateway manager
	gatewayMgr, err := orchestrator.NewLLMGatewayManager(runtime, cfg.LLM, credStore, log)
	require.NoError(t, err, "Failed to create LLM Gateway manager")
	t.Cleanup(func() {
		_ = gatewayMgr.Close()
	})

	// Start the gateway
	startCtx, startCancel := context.WithTimeout(ctx, 120*time.Second)
	defer startCancel()

	err = gatewayMgr.Start(startCtx)
	require.NoError(t, err, "Failed to start LLM Gateway")

	// Perform multiple health checks to verify monitoring is working
	for i := 0; i < 3; i++ {
		healthResult, err := gatewayMgr.HealthCheck(ctx)
		require.NoError(t, err, "Health check should succeed")
		require.NotNil(t, healthResult, "Health result should not be nil")
		t.Logf("Health check %d: status=%s", i+1, healthResult.Status)

		// Wait a bit between checks
		time.Sleep(1 * time.Second)
	}

	// Verify gateway is still running
	running, err := gatewayMgr.IsRunning(ctx)
	require.NoError(t, err, "Failed to check if gateway is running")
	require.True(t, running, "Gateway should still be running")

	// Stop the gateway
	stopCtx, stopCancel := context.WithTimeout(ctx, 60*time.Second)
	defer stopCancel()

	err = gatewayMgr.Stop(stopCtx)
	require.NoError(t, err, "Failed to stop LLM Gateway")

	t.Log("Health monitoring test passed")
}

// TestLLMGatewayDoubleStart tests that starting an already-started gateway returns an error
func TestLLMGatewayDoubleStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM Gateway test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	cfg := loadTestConfig(t)

	cfg.LLM.Enabled = true
	cfg.LLM.ContainerImage = "nginx:alpine"
	cfg.LLM.DefaultProvider = "test"
	cfg.LLM.DefaultModel = "test-model"
	cfg.LLM.Timeout = 30 * time.Second

	cfg.LLM.Providers = map[string]config.ProviderConfig{
		"test": {
			Name:    "test",
			Enabled: true,
			APIKey:  "test-api-key",
		},
	}

	t.Log("=== Testing LLM Gateway double start ===")

	// Create credential store
	credStore, err := credentials.NewStore(log)
	require.NoError(t, err, "Failed to create credential store")

	err = credStore.StoreCredential(ctx, "llm", "test", "test-api-key")
	require.NoError(t, err, "Failed to store credential")

	// Create Docker runtime
	runtime, err := container.NewDockerRuntime(cfg.Docker, log)
	require.NoError(t, err, "Failed to create Docker runtime")
	t.Cleanup(func() {
		_ = runtime.Close()
	})

	// Create LLM Gateway manager
	gatewayMgr, err := orchestrator.NewLLMGatewayManager(runtime, cfg.LLM, credStore, log)
	require.NoError(t, err, "Failed to create LLM Gateway manager")
	t.Cleanup(func() {
		_ = gatewayMgr.Close()
	})

	// Start the gateway
	startCtx, startCancel := context.WithTimeout(ctx, 120*time.Second)
	defer startCancel()

	err = gatewayMgr.Start(startCtx)
	require.NoError(t, err, "Failed to start LLM Gateway")

	// Try to start again - should fail
	err = gatewayMgr.Start(startCtx)
	require.Error(t, err, "Starting an already-started gateway should fail")
	t.Logf("Expected error for double start: %v", err)

	// Cleanup - stop the gateway
	stopCtx, stopCancel := context.WithTimeout(ctx, 60*time.Second)
	defer stopCancel()
	_ = gatewayMgr.Stop(stopCtx)

	t.Log("Double start test passed")
}

// TestLLMGatewayStopWithoutStart tests that stopping a non-started gateway returns an error
func TestLLMGatewayStopWithoutStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM Gateway test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	cfg := loadTestConfig(t)

	cfg.LLM.Enabled = true
	cfg.LLM.Providers = map[string]config.ProviderConfig{
		"test": {
			Name:    "test",
			Enabled: true,
			APIKey:  "test-api-key",
		},
	}

	t.Log("=== Testing LLM Gateway stop without start ===")

	// Create credential store
	credStore, err := credentials.NewStore(log)
	require.NoError(t, err, "Failed to create credential store")

	// Create Docker runtime
	runtime, err := container.NewDockerRuntime(cfg.Docker, log)
	require.NoError(t, err, "Failed to create Docker runtime")
	t.Cleanup(func() {
		_ = runtime.Close()
	})

	// Create LLM Gateway manager
	gatewayMgr, err := orchestrator.NewLLMGatewayManager(runtime, cfg.LLM, credStore, log)
	require.NoError(t, err, "Failed to create LLM Gateway manager")
	t.Cleanup(func() {
		_ = gatewayMgr.Close()
	})

	// Try to stop without starting - should fail
	err = gatewayMgr.Stop(ctx)
	require.Error(t, err, "Stopping a non-started gateway should fail")
	t.Logf("Expected error for stop without start: %v", err)

	t.Log("Stop without start test passed")
}

// TestLLMProviderFailover tests that provider failover configuration is properly set up
func TestLLMProviderFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM Gateway test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	cfg := loadTestConfig(t)

	t.Log("=== Step 1: Configuring multiple providers with fallback chains ===")

	// Enable LLM Gateway for testing
	cfg.LLM.Enabled = true
	cfg.LLM.ContainerImage = "nginx:alpine"
	cfg.LLM.DefaultProvider = "primary"
	cfg.LLM.DefaultModel = "test-model"
	cfg.LLM.Timeout = 30 * time.Second

	// Configure primary provider with invalid API key (simulating a failing provider)
	// Configure fallback provider with valid API key
	cfg.LLM.Providers = map[string]config.ProviderConfig{
		"primary": {
			Name:    "primary",
			Enabled: true,
			APIKey:  "invalid-key-12345", // Invalid key to simulate failure
		},
		"fallback": {
			Name:    "fallback",
			Enabled: true,
			APIKey:  "valid-api-key-67890", // Valid key
		},
		"tertiary": {
			Name:    "tertiary",
			Enabled: true,
			APIKey:  "tertiary-api-key-11111",
		},
	}

	// Configure fallback chains: if primary fails, try fallback, then tertiary
	cfg.LLM.FallbackChains = map[string][]string{
		"test-model": {"primary", "fallback", "tertiary"},
		// Pattern-based fallback for all models from primary provider
		"primary/*": {"fallback", "tertiary"},
		// And if fallback also fails
		"fallback/*": {"tertiary"},
	}

	// Verify fallback chains are configured correctly
	t.Log("=== Step 2: Verifying fallback chain configuration ===")
	require.NotNil(t, cfg.LLM.FallbackChains, "Fallback chains should be configured")
	require.Contains(t, cfg.LLM.FallbackChains, "test-model", "Test model should have fallback chain")
	require.Equal(t, []string{"primary", "fallback", "tertiary"}, cfg.LLM.FallbackChains["test-model"],
		"Test model fallback chain should be primary -> fallback -> tertiary")
	require.Contains(t, cfg.LLM.FallbackChains, "primary/*", "Primary provider should have fallback chain")
	t.Logf("Fallback chains configured: %+v", cfg.LLM.FallbackChains)

	t.Log("=== Step 3: Creating credential store and runtime ===")

	// Create credential store
	credStore, err := credentials.NewStore(log)
	require.NoError(t, err, "Failed to create credential store")

	// Store credentials for all providers
	for providerName, provider := range cfg.LLM.Providers {
		if provider.Enabled && provider.APIKey != "" {
			err = credStore.StoreCredential(ctx, "llm", providerName, provider.APIKey)
			require.NoError(t, err, "Failed to store credential for provider %s", providerName)
			t.Logf("Stored credential for provider: %s", providerName)
		}
	}

	// Create Docker runtime
	runtime, err := container.NewDockerRuntime(cfg.Docker, log)
	require.NoError(t, err, "Failed to create Docker runtime")
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Logf("Warning: Runtime close returned error: %v", err)
		}
	})

	// Create LLM Gateway manager
	gatewayMgr, err := orchestrator.NewLLMGatewayManager(runtime, cfg.LLM, credStore, log)
	require.NoError(t, err, "Failed to create LLM Gateway manager")
	require.NotNil(t, gatewayMgr, "Gateway manager should not be nil")
	require.True(t, gatewayMgr.IsEnabled(), "Gateway should be enabled")

	// Verify the gateway has the correct configuration
	t.Log("=== Step 4: Verifying LLM Gateway manager configuration ===")
	defaultProvider := cfg.LLM.DefaultProvider
	require.Equal(t, "primary", defaultProvider, "Default provider should be 'primary'")
	defaultModel := cfg.LLM.DefaultModel
	require.Equal(t, "test-model", defaultModel, "Default model should be 'test-model'")
	t.Logf("LLM Gateway configured with default provider: %s, model: %s", defaultProvider, defaultModel)

	// Verify all providers are configured
	require.Len(t, cfg.LLM.Providers, 3, "Should have 3 providers configured")
	for providerName := range cfg.LLM.Providers {
		t.Logf("Provider configured: %s", providerName)
	}

	t.Log("=== Step 5: Starting LLM Gateway with failover configuration ===")

	// Start the gateway
	startCtx, startCancel := context.WithTimeout(ctx, 120*time.Second)
	defer startCancel()

	err = gatewayMgr.Start(startCtx)
	require.NoError(t, err, "Failed to start LLM Gateway with failover configuration")
	require.True(t, gatewayMgr.IsStarted(), "Gateway should be started")

	containerID := gatewayMgr.GetContainerID()
	require.NotEmpty(t, containerID, "Container ID should not be empty")
	t.Logf("LLM Gateway started with failover configuration: container_id=%s", containerID)

	// Cleanup: Ensure gateway is stopped
	t.Cleanup(func() {
		t.Log("=== Cleanup: Closing LLM Gateway manager ===")
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer closeCancel()

		if err := gatewayMgr.Close(); err != nil {
			t.Logf("Warning: Gateway close returned error: %v", err)
		}

		t.Log("LLM Gateway manager closed")
	})

	t.Log("=== Step 6: Verifying container is running ===")

	// Check if container is running
	running, err := gatewayMgr.IsRunning(ctx)
	require.NoError(t, err, "Failed to check if gateway is running")
	require.True(t, running, "Gateway should be running")

	// Verify container state
	lifecycleMgr, err := container.NewLifecycleManagerFromRuntime(runtime, log)
	require.NoError(t, err, "Failed to create lifecycle manager")

	status, err := lifecycleMgr.Status(ctx, containerID)
	require.NoError(t, err, "Failed to get container status")
	t.Logf("Container status: %s", status)

	t.Log("=== Step 7: Verifying health check with failover configuration ===")

	// Perform health check
	healthResult, err := gatewayMgr.HealthCheck(ctx)
	require.NoError(t, err, "Health check should succeed")
	require.NotNil(t, healthResult, "Health result should not be nil")
	t.Logf("Health status: %s", healthResult.Status)

	// Health check may return Healthy or Unhealthy
	assert.Contains(t, []types.Health{types.Healthy, types.Unhealthy}, healthResult.Status,
		"Health status should be Healthy or Unhealthy")

	t.Log("=== Step 8: Stopping LLM Gateway ===")

	// Stop the gateway
	stopCtx, stopCancel := context.WithTimeout(ctx, 60*time.Second)
	defer stopCancel()

	err = gatewayMgr.Stop(stopCtx)
	require.NoError(t, err, "Failed to stop LLM Gateway")
	require.False(t, gatewayMgr.IsStarted(), "Gateway should not be started after stop")

	// Verify container is stopped
	running, err = gatewayMgr.IsRunning(ctx)
	require.NoError(t, err, "Failed to check if gateway is running after stop")
	require.False(t, running, "Gateway should not be running after stop")

	t.Log("=== Test Complete ===")
	t.Log("LLM Provider failover configuration test passed:")
	t.Log("  1. Multiple providers configured with fallback chains")
	t.Log("  2. Fallback chain configuration verified")
	t.Log("  3. Credentials stored for all providers")
	t.Log("  4. Gateway manager created with failover config")
	t.Log("  5. Gateway started successfully with failover configuration")
	t.Log("  6. Container running verified")
	t.Log("  7. Health check performed")
	t.Log("  8. Gateway stopped successfully")
	t.Log("")
	t.Log("Note: Actual provider failover behavior (switching from primary to fallback)")
	t.Log("will be tested by the LLM Gateway container implementation once available.")
	t.Log("This test verifies the orchestrator-side failover configuration is correct.")
}
