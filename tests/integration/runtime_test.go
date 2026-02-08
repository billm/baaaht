package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRuntimeDetection tests automatic runtime detection based on platform and availability
func TestRuntimeDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping runtime detection test in short mode")
	}

	ctx := context.Background()

	t.Run("detect runtime on current platform", func(t *testing.T) {
		detectedType := container.DetectRuntime(ctx)

		// Should detect either Docker, Apple Containers, or return Auto if none available
		switch detectedType {
		case types.RuntimeTypeDocker, types.RuntimeTypeAppleContainers:
			t.Logf("Detected runtime: %s", detectedType)
		case types.RuntimeTypeAuto:
			// No runtime available - skip dependent tests
			t.Skip("No container runtime available on this system")
		default:
			t.Fatalf("Unexpected runtime type detected: %s", detectedType)
		}
	})

	t.Run("detect with config override", func(t *testing.T) {
		// Test that explicit type is respected
		detectedType := container.DetectRuntimeWithConfig(ctx, string(types.RuntimeTypeDocker))
		assert.Equal(t, types.RuntimeTypeDocker, detectedType)

		detectedType = container.DetectRuntimeWithConfig(ctx, string(types.RuntimeTypeAppleContainers))
		assert.Equal(t, types.RuntimeTypeAppleContainers, detectedType)

		detectedType = container.DetectRuntimeWithConfig(ctx, string(types.RuntimeTypeAuto))
		// Should return the detected runtime or Auto if none available
		assert.Contains(t, []types.RuntimeType{
			types.RuntimeTypeDocker,
			types.RuntimeTypeAppleContainers,
			types.RuntimeTypeAuto,
		}, detectedType)
	})

	t.Run("get platform info", func(t *testing.T) {
		platform := container.GetPlatform()
		assert.NotEmpty(t, platform, "Platform should not be empty")

		arch := container.GetArchitecture()
		assert.NotEmpty(t, arch, "Architecture should not be empty")

		t.Logf("Platform: %s, Architecture: %s", platform, arch)
	})
}

// TestRuntimeCreation tests runtime creation with different configurations
func TestRuntimeCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping runtime creation test in short mode")
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err, "Failed to create logger")

	t.Run("create runtime with explicit Docker type", func(t *testing.T) {
		cfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt, err := container.NewRuntime(ctx, cfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
		}

		require.NotNil(t, rt)
		assert.Equal(t, string(types.RuntimeTypeDocker), rt.Type())

		// Verify runtime info
		info, err := rt.Info(ctx)
		require.NoError(t, err)
		assert.Equal(t, string(types.RuntimeTypeDocker), info.Type)
		assert.NotEmpty(t, info.Version)

		// Close runtime
		err = rt.Close()
		assert.NoError(t, err)
	})

	t.Run("create runtime with auto detection", func(t *testing.T) {
		cfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeAuto),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt, err := container.NewRuntime(ctx, cfg)
		if err != nil {
			t.Skipf("No runtime available: %v", err)
		}

		require.NotNil(t, rt)
		assert.NotEmpty(t, rt.Type())

		// Close runtime
		err = rt.Close()
		assert.NoError(t, err)
	})

	t.Run("create runtime with invalid type", func(t *testing.T) {
		cfg := container.RuntimeConfig{
			Type:    "invalid-runtime-type",
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt, err := container.NewRuntime(ctx, cfg)
		assert.Error(t, err)
		assert.Nil(t, rt)
		assert.True(t, types.IsErrCode(err, types.ErrCodeInvalidArgument))
	})

	t.Run("create runtime with nil logger", func(t *testing.T) {
		cfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  nil,
			Timeout: 30 * time.Second,
		}

		rt, err := container.NewRuntime(ctx, cfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
		}

		// Should succeed with auto-created logger
		assert.NotNil(t, rt)
		_ = rt.Close()
	})
}

// TestRuntimeRegistry tests the runtime registry functionality
func TestRuntimeRegistry(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping runtime registry test in short mode")
	}

	// Create a fresh registry for testing
	log, err := logger.NewDefault()
	require.NoError(t, err)

	reg := container.NewRegistry(log)
	require.NotNil(t, reg)

	ctx := context.Background()

	t.Run("list runtimes before initialization", func(t *testing.T) {
		types := reg.ListRuntimes()
		// Registry starts empty
		assert.Empty(t, types, "New registry should have no factories")
	})

	t.Run("initialize and list runtimes", func(t *testing.T) {
		err := container.InitializeRuntimes(log)
		require.NoError(t, err)

		// Get global registry and check
		globalReg := container.GetGlobalRegistry()
		runtimes := globalReg.ListRuntimes()
		assert.NotEmpty(t, runtimes, "Should have registered runtimes")
		t.Logf("Registered runtimes: %v", runtimes)
	})

	t.Run("get runtime from registry", func(t *testing.T) {
		cfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt, err := container.GetRuntimeFromRegistry(ctx, string(types.RuntimeTypeDocker), cfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
		}

		require.NotNil(t, rt)
		assert.Equal(t, string(types.RuntimeTypeDocker), rt.Type())

		// Close the runtime
		err = rt.Close()
		assert.NoError(t, err)

		// Close it in the registry
		err = container.GetGlobalRegistry().CloseRuntime(string(types.RuntimeTypeDocker))
		assert.NoError(t, err)
	})

	t.Run("check runtime availability", func(t *testing.T) {
		available := container.AvailableRuntimes(ctx)
		t.Logf("Available runtimes: %v", available)

		// Should have at least one runtime if Docker is available
		if container.IsDockerAvailable(ctx) {
			assert.Contains(t, available, string(types.RuntimeTypeDocker))
		}
	})

	t.Run("registry has runtime check", func(t *testing.T) {
		globalReg := container.GetGlobalRegistry()

		// Check for Docker
		hasDocker := globalReg.HasRuntime(string(types.RuntimeTypeDocker))
		assert.True(t, hasDocker, "Should have Docker factory registered")
	})

	t.Run("get existing runtime", func(t *testing.T) {
		globalReg := container.GetGlobalRegistry()
		cfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		// Create a runtime
		rt1, err := globalReg.GetRuntime(string(types.RuntimeTypeDocker), cfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
		}
		require.NotNil(t, rt1)

		// Get the same runtime again - should return the same instance
		rt2, err := globalReg.GetRuntime(string(types.RuntimeTypeDocker), cfg)
		require.NoError(t, err)
		assert.Same(t, rt1, rt2, "Should return the same runtime instance")

		// Get existing runtime without creating
		rt3, err := globalReg.GetExistingRuntime(string(types.RuntimeTypeDocker))
		require.NoError(t, err)
		assert.Same(t, rt1, rt3, "Should return the same existing instance")

		// Cleanup
		err = globalReg.CloseRuntime(string(types.RuntimeTypeDocker))
		assert.NoError(t, err)
	})

	t.Run("list runtime instances", func(t *testing.T) {
		globalReg := container.GetGlobalRegistry()

		// Initially no instances (after previous cleanup)
		instances := globalReg.ListInstances()
		t.Logf("Active instances before: %v", instances)

		// Create a runtime
		cfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt, err := globalReg.GetRuntime(string(types.RuntimeTypeDocker), cfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
		}

		// Check instances
		instances = globalReg.ListInstances()
		assert.Contains(t, instances, string(types.RuntimeTypeDocker))

		// Cleanup
		_ = rt.Close()
	})
}

// TestRuntimeSwitching tests switching between different runtime implementations
func TestRuntimeSwitching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping runtime switching test in short mode")
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("switch from Docker to auto-detected", func(t *testing.T) {
		// Create a Docker runtime
		dockerCfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		dockerRT, err := container.NewRuntime(ctx, dockerCfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
		}

		require.NotNil(t, dockerRT)
		assert.Equal(t, string(types.RuntimeTypeDocker), dockerRT.Type())

		// Get runtime info
		dockerInfo, err := dockerRT.Info(ctx)
		require.NoError(t, err)
		t.Logf("Docker runtime info: Type=%s, Version=%s", dockerInfo.Type, dockerInfo.Version)

		// Close Docker runtime
		err = dockerRT.Close()
		assert.NoError(t, err)

		// Create an auto-detected runtime
		autoCfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeAuto),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		autoRT, err := container.NewRuntime(ctx, autoCfg)
		if err != nil {
			t.Skipf("No runtime available: %v", err)
		}

		require.NotNil(t, autoRT)

		// Get runtime info
		autoInfo, err := autoRT.Info(ctx)
		require.NoError(t, err)
		t.Logf("Auto-detected runtime info: Type=%s, Version=%s", autoInfo.Type, autoInfo.Version)

		// Close auto runtime
		err = autoRT.Close()
		assert.NoError(t, err)
	})

	t.Run("switch via registry", func(t *testing.T) {
		globalReg := container.GetGlobalRegistry()

		// Close any existing Docker runtime first
		_ = globalReg.CloseRuntime(string(types.RuntimeTypeDocker))

		// Get Docker runtime from registry
		dockerCfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		dockerRT, err := globalReg.GetRuntime(string(types.RuntimeTypeDocker), dockerCfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
		}

		require.NotNil(t, dockerRT)

		// Verify we got a Docker runtime
		assert.Equal(t, string(types.RuntimeTypeDocker), dockerRT.Type())

		// Close and switch to auto
		err = globalReg.CloseRuntime(string(types.RuntimeTypeDocker))
		assert.NoError(t, err)

		// Now get auto-detected runtime
		autoCfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeAuto),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		detectedType := container.DetectRuntime(ctx)
		if detectedType == types.RuntimeTypeAuto {
			t.Skip("No runtime available for switching test")
		}

		autoRT, err := container.NewRuntime(ctx, autoCfg)
		require.NoError(t, err)
		require.NotNil(t, autoRT)

		t.Logf("Switched from Docker to %s", autoRT.Type())

		err = autoRT.Close()
		assert.NoError(t, err)
	})

	t.Run("verify runtime isolation", func(t *testing.T) {
		// Create two separate runtime instances
		cfg1 := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt1, err := container.NewRuntime(ctx, cfg1)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
		}

		cfg2 := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt2, err := container.NewRuntime(ctx, cfg2)
		require.NoError(t, err)

		// They should be different instances
		assert.NotSame(t, rt1, rt2, "Runtimes should be separate instances")

		// But both should work
		info1, err := rt1.Info(ctx)
		require.NoError(t, err)

		info2, err := rt2.Info(ctx)
		require.NoError(t, err)

		assert.Equal(t, info1.Type, info2.Type)

		// Close both
		_ = rt1.Close()
		_ = rt2.Close()
	})
}

// TestRuntimeAvailability tests runtime availability checking
func TestRuntimeAvailability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping runtime availability test in short mode")
	}

	ctx := context.Background()

	t.Run("check Docker availability", func(t *testing.T) {
		available := container.IsDockerAvailable(ctx)
		t.Logf("Docker available: %v", available)

		if available {
			// If Docker is available, we should be able to create a runtime
			log, err := logger.NewDefault()
			require.NoError(t, err)

			cfg := container.RuntimeConfig{
				Type:    string(types.RuntimeTypeDocker),
				Logger:  log,
				Timeout: 10 * time.Second,
			}

			rt, err := container.NewRuntime(ctx, cfg)
			require.NoError(t, err, "Should create Docker runtime when available")
			require.NotNil(t, rt)

			info, err := rt.Info(ctx)
			require.NoError(t, err)
			assert.Equal(t, string(types.RuntimeTypeDocker), info.Type)

			_ = rt.Close()
		}
	})

	t.Run("check Apple Containers availability", func(t *testing.T) {
		available := container.IsAppleContainersAvailable()
		t.Logf("Apple Containers available: %v", available)

		// Apple Containers is only available on macOS and currently returns false
		// This test documents the expected behavior
		if available {
			log, err := logger.NewDefault()
			require.NoError(t, err)

			cfg := container.RuntimeConfig{
				Type:    string(types.RuntimeTypeAppleContainers),
				Logger:  log,
				Timeout: 10 * time.Second,
			}

			rt, err := container.NewRuntime(ctx, cfg)
			if err == nil {
				assert.Equal(t, string(types.RuntimeTypeAppleContainers), rt.Type())
				_ = rt.Close()
			}
		}
	})

	t.Run("available runtimes list", func(t *testing.T) {
		available := container.AvailableRuntimes(ctx)
		t.Logf("Available runtimes: %v", available)

		// Should return a list (possibly empty)
		assert.NotNil(t, available)

		// If Docker is available, it should be in the list
		if container.IsDockerAvailable(ctx) {
			assert.Contains(t, available, string(types.RuntimeTypeDocker))
		}
	})
}

// TestRuntimeWithContainer tests runtime operations with actual containers
func TestRuntimeWithContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping runtime container test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := container.RuntimeConfig{
		Type:    string(types.RuntimeTypeDocker),
		Logger:  log,
		Timeout: 30 * time.Second,
	}

	rt, err := container.NewRuntime(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, rt)

	defer func() {
		_ = rt.Close()
	}()

	sessionID := types.NewID("runtime-test-session")
	containerName := fmt.Sprintf("baaaht-runtime-test-%d", time.Now().Unix())

	t.Run("create and destroy container", func(t *testing.T) {
		createCfg := container.CreateConfig{
			Config: types.ContainerConfig{
				Image:   "alpine:latest",
				Command: []string{"echo", "runtime-test"},
				Env:     make(map[string]string),
				Labels: map[string]string{
					"baaaht.session_id": sessionID.String(),
					"baaaht.managed":    "true",
					"test":              "runtime-switching",
				},
			},
			Name:        containerName,
			SessionID:   sessionID,
			AutoPull:    true,
			PullTimeout: 5 * time.Minute,
		}

		result, err := rt.Create(ctx, createCfg)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEmpty(t, result.ContainerID)

		t.Logf("Container created: %s", result.ContainerID)

		// Destroy the container
		destroyCfg := container.DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		}

		err = rt.Destroy(ctx, destroyCfg)
		assert.NoError(t, err)
	})

	t.Run("runtime info matches type", func(t *testing.T) {
		info, err := rt.Info(ctx)
		require.NoError(t, err)
		assert.Equal(t, string(types.RuntimeTypeDocker), info.Type)
		assert.NotEmpty(t, info.Version)
		assert.NotEmpty(t, info.Platform)
	})
}

// TestRuntimeConcurrentAccess tests concurrent runtime usage
func TestRuntimeConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping runtime concurrent access test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("concurrent runtime creation", func(t *testing.T) {
		// Create multiple runtimes concurrently
		const numRuntimes = 3
		runtimes := make([]container.Runtime, numRuntimes)
		errors := make([]error, numRuntimes)

		for i := 0; i < numRuntimes; i++ {
			go func(idx int) {
				cfg := container.RuntimeConfig{
					Type:    string(types.RuntimeTypeDocker),
					Logger:  log,
					Timeout: 30 * time.Second,
				}
				runtimes[idx], errors[idx] = container.NewRuntime(ctx, cfg)
			}(i)
		}

		// Wait for all goroutines to complete
		// Give them time to finish
		time.Sleep(5 * time.Second)

		// Verify all runtimes were created successfully
		for i := 0; i < numRuntimes; i++ {
			if errors[i] == nil {
				require.NotNil(t, runtimes[i])
				assert.Equal(t, string(types.RuntimeTypeDocker), runtimes[i].Type())
				_ = runtimes[i].Close()
			} else {
				t.Logf("Runtime %d creation error: %v", i, errors[i])
			}
		}
	})

	t.Run("concurrent registry access", func(t *testing.T) {
		globalReg := container.GetGlobalRegistry()
		cfg := container.RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		// Access the registry concurrently
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				_, _ = globalReg.GetRuntime(string(types.RuntimeTypeDocker), cfg)
				done <- true
			}()
		}

		// Wait for all to complete
		for i := 0; i < 10; i++ {
			<-done
		}

		// Cleanup
		_ = globalReg.CloseRuntime(string(types.RuntimeTypeDocker))
	})
}
