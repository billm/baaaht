package integration

import (
	"context"
	"path/filepath"
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
	credStore, err := credentials.NewStore(log)
	require.NoError(t, err, "Failed to create credential store")

	// Store test credentials
	err = credStore.StoreCredential(ctx, "llm", "test", "test-api-key-12345")
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
