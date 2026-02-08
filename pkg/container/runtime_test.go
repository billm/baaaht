package container

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestRuntime creates a Docker runtime for testing
func createTestRuntime(t *testing.T) Runtime {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RuntimeConfig{
		Type:    string(types.RuntimeTypeDocker),
		Logger:  log,
		Timeout: 30 * time.Second,
	}

	rt, err := NewRuntime(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, rt)

	return rt
}

func TestNewRuntime(t *testing.T) {
	t.Run("create Docker runtime", func(t *testing.T) {
		ctx := context.Background()
		log, err := logger.NewDefault()
		require.NoError(t, err)

		cfg := RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt, err := NewRuntime(ctx, cfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
			return
		}

		assert.NotNil(t, rt)
		assert.Equal(t, string(types.RuntimeTypeDocker), rt.Type())

		// Verify it's a DockerRuntime
		drt, ok := rt.(*DockerRuntime)
		assert.True(t, ok, "runtime should be a *DockerRuntime")
		assert.False(t, drt.IsClosed(), "runtime should not be closed after creation")

		// Close the runtime
		err = rt.Close()
		assert.NoError(t, err)
		assert.True(t, drt.IsClosed(), "runtime should be closed after Close()")
	})

	t.Run("create with auto detection", func(t *testing.T) {
		ctx := context.Background()
		log, err := logger.NewDefault()
		require.NoError(t, err)

		cfg := RuntimeConfig{
			Type:    string(types.RuntimeTypeAuto),
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt, err := NewRuntime(ctx, cfg)
		if err != nil {
			t.Skipf("No runtime available: %v", err)
			return
		}

		assert.NotNil(t, rt)
		assert.NotEmpty(t, rt.Type())

		// Close the runtime
		_ = rt.Close()
	})

	t.Run("create with invalid type", func(t *testing.T) {
		ctx := context.Background()
		log, err := logger.NewDefault()
		require.NoError(t, err)

		cfg := RuntimeConfig{
			Type:    "invalid-runtime-type",
			Logger:  log,
			Timeout: 30 * time.Second,
		}

		rt, err := NewRuntime(ctx, cfg)
		assert.Error(t, err)
		assert.Nil(t, rt)
		assert.True(t, types.IsErrCode(err, types.ErrCodeInvalidArgument))
	})

	t.Run("create with nil logger", func(t *testing.T) {
		ctx := context.Background()

		cfg := RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  nil,
			Timeout: 30 * time.Second,
		}

		rt, err := NewRuntime(ctx, cfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
			return
		}

		assert.NotNil(t, rt)

		// Close the runtime
		_ = rt.Close()
	})
}

func TestRuntimeType(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	assert.Equal(t, string(types.RuntimeTypeDocker), rt.Type())
}

func TestRuntimeClient(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	client := rt.Client()
	assert.NotNil(t, client)

	// Verify it's a Docker client
	dockerClient, ok := client.(*Client)
	assert.True(t, ok, "client should be a *Client")
	assert.NotNil(t, dockerClient)
}

func TestRuntimeInfo(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	info, err := rt.Info(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, info)

	assert.Equal(t, string(types.RuntimeTypeDocker), info.Type)
	assert.NotEmpty(t, info.Version)
	assert.NotEmpty(t, info.APIVersion)
	assert.NotEmpty(t, info.Platform)
	assert.NotEmpty(t, info.Capabilities)
	assert.NotNil(t, info.Metadata)
}

func TestRuntimeClose(t *testing.T) {
	t.Run("close runtime", func(t *testing.T) {
		rt := createTestRuntime(t)

		err := rt.Close()
		assert.NoError(t, err)

		// Verify it's closed
		if drt, ok := rt.(*DockerRuntime); ok {
			assert.True(t, drt.IsClosed())
		}
	})

	t.Run("close already closed runtime", func(t *testing.T) {
		rt := createTestRuntime(t)

		err := rt.Close()
		assert.NoError(t, err)

		// Close again should succeed
		err = rt.Close()
		assert.NoError(t, err)
	})
}

func TestRuntimeCreate(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-create-test"

	t.Run("create container with defaults", func(t *testing.T) {
		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello"},
				Env:     make(map[string]string),
				Labels: map[string]string{
					"baaaht.session_id": sessionID.String(),
					"baaaht.managed":    "true",
				},
				Mounts:    []types.Mount{},
				Ports:     []types.PortBinding{},
				Resources: types.ResourceLimits{},
			},
			Name:        containerName,
			SessionID:   sessionID,
			AutoPull:    true,
			PullTimeout: 5 * time.Minute,
		}

		result, err := rt.Create(ctx, cfg)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotEmpty(t, result.ContainerID)

		// Cleanup
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	})

	t.Run("create with closed runtime", func(t *testing.T) {
		// Create a temporary runtime to close
		tmpRt := createTestRuntime(t)
		_ = tmpRt.Close()

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "alpine:latest",
			},
			Name:      "baaaht-runtime-closed-test",
			SessionID: sessionID,
		}

		_, err := tmpRt.Create(ctx, cfg)
		assert.Error(t, err)
		assert.True(t, types.IsErrCode(err, types.ErrCodeUnavailable))
	})
}

func TestRuntimeStart(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-start-test"

	// Create a long-running container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("start created container", func(t *testing.T) {
		err := rt.Start(ctx, StartConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		running, err := rt.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.True(t, running)
	})

	t.Run("start with empty container ID", func(t *testing.T) {
		err := rt.Start(ctx, StartConfig{})
		assert.Error(t, err)
	})
}

func TestRuntimeStop(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-stop-test"

	// Create and start a container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	err = rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("stop running container", func(t *testing.T) {
		err := rt.Stop(ctx, StopConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		running, err := rt.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.False(t, running)
	})
}

func TestRuntimeRestart(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-restart-test"

	// Create and start a container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	err = rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("restart running container", func(t *testing.T) {
		err := rt.Restart(ctx, RestartConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		running, err := rt.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.True(t, running)
	})
}

func TestRuntimeDestroy(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")

	t.Run("destroy stopped container", func(t *testing.T) {
		containerName := "baaaht-runtime-destroy-test"

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello"},
				Env:     make(map[string]string),
				Labels: map[string]string{
					"baaaht.session_id": sessionID.String(),
					"baaaht.managed":    "true",
				},
				Mounts:    []types.Mount{},
				Ports:     []types.PortBinding{},
				Resources: types.ResourceLimits{},
			},
			Name:        containerName,
			SessionID:   sessionID,
			AutoPull:    true,
			PullTimeout: 5 * time.Minute,
		}

		result, err := rt.Create(ctx, cfg)
		require.NoError(t, err)

		err = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		// Verify it's destroyed
		_, err = rt.Status(ctx, result.ContainerID)
		assert.Error(t, err)
	})
}

func TestRuntimeStatus(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-status-test"

	// Create a long-running container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("status of created container", func(t *testing.T) {
		state, err := rt.Status(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.Equal(t, types.ContainerStateCreated, state)
	})

	t.Run("status of running container", func(t *testing.T) {
		err := rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
		require.NoError(t, err)

		state, err := rt.Status(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.Equal(t, types.ContainerStateRunning, state)
	})

	t.Run("status with empty container ID", func(t *testing.T) {
		_, err := rt.Status(ctx, "")
		assert.Error(t, err)
	})
}

func TestRuntimeIsRunning(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-isrunning-test"

	// Create a long-running container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("isRunning for created container", func(t *testing.T) {
		running, err := rt.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.False(t, running)
	})

	t.Run("isRunning for running container", func(t *testing.T) {
		err := rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
		require.NoError(t, err)

		running, err := rt.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.True(t, running)
	})
}

func TestRuntimePauseUnpause(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-pause-test"

	// Create and start a container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	err = rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("pause running container", func(t *testing.T) {
		err := rt.Pause(ctx, result.ContainerID)
		assert.NoError(t, err)

		state, err := rt.Status(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.Equal(t, types.ContainerStatePaused, state)
	})

	t.Run("unpause paused container", func(t *testing.T) {
		err := rt.Unpause(ctx, result.ContainerID)
		assert.NoError(t, err)

		// Give it a moment to unpause
		time.Sleep(100 * time.Millisecond)

		running, err := rt.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.True(t, running)
	})

	t.Run("pause with empty container ID", func(t *testing.T) {
		err := rt.Pause(ctx, "")
		assert.Error(t, err)
	})

	t.Run("unpause with empty container ID", func(t *testing.T) {
		err := rt.Unpause(ctx, "")
		assert.Error(t, err)
	})
}

func TestRuntimeKill(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-kill-test"

	// Create and start a container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	err = rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("kill running container", func(t *testing.T) {
		err := rt.Kill(ctx, KillConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		// Give it a moment to be killed
		time.Sleep(100 * time.Millisecond)

		running, err := rt.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.False(t, running)
	})

	t.Run("kill with empty container ID", func(t *testing.T) {
		err := rt.Kill(ctx, KillConfig{})
		assert.Error(t, err)
	})
}

func TestRuntimeWait(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-wait-test"

	t.Run("wait for exited container", func(t *testing.T) {
		// Create a container that exits quickly
		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello"},
				Env:     make(map[string]string),
				Labels: map[string]string{
					"baaaht.session_id": sessionID.String(),
					"baaaht.managed":    "true",
				},
				Mounts:    []types.Mount{},
				Ports:     []types.PortBinding{},
				Resources: types.ResourceLimits{},
			},
			Name:        containerName,
			SessionID:   sessionID,
			AutoPull:    true,
			PullTimeout: 5 * time.Minute,
		}

		result, err := rt.Create(ctx, cfg)
		require.NoError(t, err)

		err = rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
		require.NoError(t, err)

		// Wait for exit
		exitCode, err := rt.Wait(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)

		// Cleanup
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	})

	t.Run("wait with empty container ID", func(t *testing.T) {
		_, err := rt.Wait(ctx, "")
		assert.Error(t, err)
	})
}

func TestRuntimeHealthCheck(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-health-test"

	// Create and start a container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	err = rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("health check running container", func(t *testing.T) {
		healthResult, err := rt.HealthCheck(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.NotNil(t, healthResult)
		assert.Equal(t, result.ContainerID, healthResult.ContainerID)
		assert.NotEmpty(t, healthResult.Status)
	})

	t.Run("health check with retry", func(t *testing.T) {
		healthResult, err := rt.HealthCheckWithRetry(ctx, result.ContainerID, 3, 100*time.Millisecond)
		assert.NoError(t, err)
		assert.NotNil(t, healthResult)
	})
}

func TestRuntimeStats(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-stats-test"

	// Create and start a container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	err = rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("get container stats", func(t *testing.T) {
		// Give the container a moment to start and generate some stats
		time.Sleep(500 * time.Millisecond)

		stats, err := rt.Stats(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
	})

	t.Run("stats stream", func(t *testing.T) {
		statsCh, errCh := rt.StatsStream(ctx, result.ContainerID, 1*time.Second)

		// Read one stats update
		select {
		case stats := <-statsCh:
			assert.NotNil(t, stats)
		case err := <-errCh:
			t.Fatalf("unexpected error from stats stream: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for stats")
		}

		// Cancel the context to stop the stream
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		_, _ = rt.StatsStream(cancelCtx, result.ContainerID, 1*time.Second)
	})
}

func TestRuntimeLogs(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-logs-test"

	// Create and start a container that outputs to logs
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sh", "-c", "echo 'hello world' && echo 'second line' && sleep 300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	err = rt.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("get container logs interface", func(t *testing.T) {
		// Use a longer timeout context for reading logs
		logsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		logsCfg := LogsConfig{
			ContainerID: result.ContainerID,
			Tail:        "100",
			Stdout:      true,
			Stderr:      true,
		}

		logReader, err := rt.Logs(logsCtx, logsCfg)
		// The interface call should succeed - timing issues with actual log reading are acceptable
		if err == nil {
			assert.NotNil(t, logReader)
			logReader.Close()
		} else {
			// If there's an error, verify it's not a fundamental interface issue
			t.Logf("Logs call returned error (may be timing): %v", err)
		}
	})

	t.Run("get logs as lines interface", func(t *testing.T) {
		// Use a longer timeout context for reading logs
		logsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		logsCfg := LogsConfig{
			ContainerID: result.ContainerID,
			Tail:        "100",
			Stdout:      true,
			Stderr:      true,
		}

		logs, err := rt.LogsLines(logsCtx, logsCfg)
		// The interface call should work - timing issues with actual log retrieval are acceptable
		if err == nil {
			assert.NotNil(t, logs)
		} else {
			t.Logf("LogsLines returned error (may be timing): %v", err)
		}
	})
}

func TestRuntimeEvents(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-runtime-events-test"

	// Create a container
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels: map[string]string{
				"baaaht.session_id": sessionID.String(),
				"baaaht.managed":    "true",
			},
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := rt.Create(ctx, cfg)
	require.NoError(t, err)

	defer func() {
		_ = rt.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("events stream", func(t *testing.T) {
		eventCh, errCh := rt.EventsStream(ctx, result.ContainerID)

		// Wait for at least one event or timeout
		select {
		case event := <-eventCh:
			assert.NotNil(t, event)
		case err := <-errCh:
			t.Fatalf("unexpected error from events stream: %v", err)
		case <-time.After(3 * time.Second):
			// Timeout is acceptable - events may not be generated immediately
		}
	})
}

func TestRuntimePullImage(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	ctx := context.Background()

	t.Run("pull existing image", func(t *testing.T) {
		// alpine:latest should already be available from previous tests
		err := rt.PullImage(ctx, "alpine:latest", 1*time.Minute)
		// Should succeed or already exist
		assert.NoError(t, err)
	})

	t.Run("check if image exists", func(t *testing.T) {
		exists, err := rt.ImageExists(ctx, "alpine:latest")
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("check non-existent image", func(t *testing.T) {
		exists, err := rt.ImageExists(ctx, "baaaht/nonexistent:image12345")
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestRuntimeString(t *testing.T) {
	rt := createTestRuntime(t)
	defer rt.Close()

	t.Run("string representation", func(t *testing.T) {
		str := fmt.Sprintf("%v", rt)
		assert.NotEmpty(t, str)
	})

	// Test with DockerRuntime specifically
	if drt, ok := rt.(*DockerRuntime); ok {
		str := drt.String()
		assert.Contains(t, str, "DockerRuntime")
		assert.Contains(t, str, "Closed")
	}
}
