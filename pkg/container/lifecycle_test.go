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

func TestNewLifecycleManager(t *testing.T) {
	t.Run("create with nil client", func(t *testing.T) {
		lm, err := NewLifecycleManager(nil, nil)
		assert.Error(t, err)
		assert.Nil(t, lm)
	})

	t.Run("create with nil logger uses default", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping integration test in short mode")
		}

		if err := CheckEnvironment(); err != nil {
			t.Skip("Docker not available:", err)
		}

		client, err := NewDefault(nil)
		require.NoError(t, err)
		defer client.Close()

		lm, err := NewLifecycleManager(client, nil)
		assert.NoError(t, err)
		assert.NotNil(t, lm)
	})

	t.Run("create with valid client and logger", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping integration test in short mode")
		}

		if err := CheckEnvironment(); err != nil {
			t.Skip("Docker not available:", err)
		}

		client, err := NewDefault(nil)
		require.NoError(t, err)
		defer client.Close()

		log, err := logger.NewDefault()
		require.NoError(t, err)

		lm, err := NewLifecycleManager(client, log)
		assert.NoError(t, err)
		assert.NotNil(t, lm)
	})
}

func TestLifecycleStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-lifecycle-start-test"

	// Create a stopped container
	result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Ensure cleanup
	defer func() {
		_ = lm.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("start created container", func(t *testing.T) {
		err := lm.Start(ctx, StartConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		// Verify it's running
		running, err := lm.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.True(t, running)
	})

	t.Run("start already running container should succeed", func(t *testing.T) {
		// Container is already running from previous test
		err := lm.Start(ctx, StartConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		// Docker API allows starting an already running container
		assert.NoError(t, err)
	})

	t.Run("start with empty container ID", func(t *testing.T) {
		err := lm.Start(ctx, StartConfig{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "container ID cannot be empty")
	})

	t.Run("start with invalid container ID", func(t *testing.T) {
		err := lm.Start(ctx, StartConfig{
			ContainerID: "invalid-container-id-12345",
		})
		assert.Error(t, err)
	})

	t.Run("start with context cancellation", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		err := lm.Start(cancelledCtx, StartConfig{
			ContainerID: result.ContainerID,
		})
		assert.Error(t, err)
	})
}

func TestLifecycleStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-lifecycle-stop-test"

	// Create and start a container
	result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
	require.NoError(t, err)

	// Start the container
	err = lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	// Ensure cleanup
	defer func() {
		_ = lm.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("stop running container", func(t *testing.T) {
		err := lm.Stop(ctx, StopConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		// Verify it's stopped
		running, err := lm.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.False(t, running)
	})

	t.Run("stop already stopped container should succeed", func(t *testing.T) {
		// Container is already stopped from previous test
		err := lm.Stop(ctx, StopConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		// Docker API allows stopping an already stopped container
		assert.NoError(t, err)
	})

	t.Run("stop with custom timeout", func(t *testing.T) {
		// Start the container again
		err := lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
		require.NoError(t, err)

		timeout := 5 * time.Second
		err = lm.Stop(ctx, StopConfig{
			ContainerID: result.ContainerID,
			Timeout:     &timeout,
		})
		assert.NoError(t, err)
	})

	t.Run("stop with empty container ID", func(t *testing.T) {
		err := lm.Stop(ctx, StopConfig{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "container ID cannot be empty")
	})
}

func TestLifecycleRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-lifecycle-restart-test"

	// Create and start a container
	result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
	require.NoError(t, err)

	err = lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	// Ensure cleanup
	defer func() {
		_ = lm.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("restart running container", func(t *testing.T) {
		err := lm.Restart(ctx, RestartConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		// Verify it's still running after restart
		running, err := lm.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.True(t, running)
	})

	t.Run("restart with custom timeout", func(t *testing.T) {
		timeout := 3 * time.Second
		err := lm.Restart(ctx, RestartConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Timeout:     &timeout,
		})
		assert.NoError(t, err)

		running, err := lm.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.True(t, running)
	})

	t.Run("restart with empty container ID", func(t *testing.T) {
		err := lm.Restart(ctx, RestartConfig{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "container ID cannot be empty")
	})
}

func TestLifecycleDestroy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")

	t.Run("destroy stopped container", func(t *testing.T) {
		containerName := "baaaht-lifecycle-destroy-stopped-test"

		// Create a container
		result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
		require.NoError(t, err)

		// Destroy it
		err = lm.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		// Verify it's destroyed by checking status (should fail)
		_, err = lm.Status(ctx, result.ContainerID)
		assert.Error(t, err)
	})

	t.Run("destroy running container with force", func(t *testing.T) {
		containerName := "baaaht-lifecycle-destroy-running-test"

		// Create and start a container
		result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
		require.NoError(t, err)

		err = lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
		require.NoError(t, err)

		// Destroy it with force
		err = lm.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
		assert.NoError(t, err)

		// Verify it's destroyed
		_, err = lm.Status(ctx, result.ContainerID)
		assert.Error(t, err)
	})

	t.Run("destroy with empty container ID", func(t *testing.T) {
		err := lm.Destroy(ctx, DestroyConfig{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "container ID cannot be empty")
	})
}

func TestLifecycleStopAndDestroy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-lifecycle-stop-destroy-test"

	// Create and start a container
	result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
	require.NoError(t, err)

	err = lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	t.Run("stop and destroy running container", func(t *testing.T) {
		err := lm.StopAndDestroy(ctx, result.ContainerID, nil)
		assert.NoError(t, err)

		// Verify it's destroyed
		_, err = lm.Status(ctx, result.ContainerID)
		assert.Error(t, err)
	})

	t.Run("stop and destroy with custom timeout", func(t *testing.T) {
		// Create another container
		result2, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName+"-2", sessionID)
		require.NoError(t, err)

		err = lm.Start(ctx, StartConfig{ContainerID: result2.ContainerID})
		require.NoError(t, err)

		timeout := 5 * time.Second
		err = lm.StopAndDestroy(ctx, result2.ContainerID, &timeout)
		assert.NoError(t, err)

		// Verify it's destroyed
		_, err = lm.Status(ctx, result2.ContainerID)
		assert.Error(t, err)
	})
}

func TestLifecyclePauseUnpause(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-lifecycle-pause-test"

	// Create and start a container
	result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
	require.NoError(t, err)

	err = lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	// Ensure cleanup
	defer func() {
		_ = lm.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("pause running container", func(t *testing.T) {
		err := lm.Pause(ctx, result.ContainerID)
		assert.NoError(t, err)

		// Verify state is paused
		state, err := lm.Status(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.Equal(t, types.ContainerStatePaused, state)
	})

	t.Run("unpause paused container", func(t *testing.T) {
		err := lm.Unpause(ctx, result.ContainerID)
		assert.NoError(t, err)

		// Give it a moment to unpause
		time.Sleep(100 * time.Millisecond)

		// Verify state is running again
		running, err := lm.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.True(t, running)
	})

	t.Run("pause with empty container ID", func(t *testing.T) {
		err := lm.Pause(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "container ID cannot be empty")
	})

	t.Run("unpause with empty container ID", func(t *testing.T) {
		err := lm.Unpause(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "container ID cannot be empty")
	})
}

func TestLifecycleStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-lifecycle-status-test"

	// Create a container
	result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
	require.NoError(t, err)

	// Ensure cleanup
	defer func() {
		_ = lm.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("status of created container", func(t *testing.T) {
		state, err := lm.Status(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.Equal(t, types.ContainerStateCreated, state)
	})

	t.Run("status of running container", func(t *testing.T) {
		err := lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
		require.NoError(t, err)

		state, err := lm.Status(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.Equal(t, types.ContainerStateRunning, state)
	})

	t.Run("isRunning returns true for running container", func(t *testing.T) {
		running, err := lm.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.True(t, running)
	})

	t.Run("status with empty container ID", func(t *testing.T) {
		_, err := lm.Status(ctx, "")
		assert.Error(t, err)
	})

	t.Run("status with invalid container ID", func(t *testing.T) {
		_, err := lm.Status(ctx, "invalid-container-id")
		assert.Error(t, err)
	})
}

func TestLifecycleKill(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-lifecycle-kill-test"

	// Create a long-running container
	result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
	require.NoError(t, err)

	// Start container with sleep command to keep it running
	// We need to use the Docker client directly for this
	err = lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
	require.NoError(t, err)

	// Ensure cleanup
	defer func() {
		_ = lm.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	}()

	t.Run("kill running container with SIGKILL", func(t *testing.T) {
		err := lm.Kill(ctx, KillConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
		})
		assert.NoError(t, err)

		// Give it a moment to be killed
		time.Sleep(100 * time.Millisecond)

		// Verify it's not running
		running, err := lm.IsRunning(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.False(t, running)
	})

	t.Run("kill with empty container ID", func(t *testing.T) {
		err := lm.Kill(ctx, KillConfig{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "container ID cannot be empty")
	})
}

func TestLifecycleWait(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")
	containerName := "baaaht-lifecycle-wait-test"

	t.Run("wait for exited container", func(t *testing.T) {
		// Create a container that exits quickly
		result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
		require.NoError(t, err)

		// Start and immediately stop
		err = lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
		require.NoError(t, err)

		err = lm.Stop(ctx, StopConfig{ContainerID: result.ContainerID})
		require.NoError(t, err)

		// Wait for exit
		exitCode, err := lm.Wait(ctx, result.ContainerID)
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)

		// Cleanup
		_ = lm.Destroy(ctx, DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		})
	})

	t.Run("wait with empty container ID", func(t *testing.T) {
		_, err := lm.Wait(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "container ID cannot be empty")
	})

	t.Run("wait with context timeout", func(t *testing.T) {
		// Create a long-running container
		result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName+"-2", sessionID)
		require.NoError(t, err)

		err = lm.Start(ctx, StartConfig{ContainerID: result.ContainerID})
		require.NoError(t, err)

		// Ensure cleanup
		defer func() {
			_ = lm.Destroy(ctx, DestroyConfig{
				ContainerID: result.ContainerID,
				Force:       true,
			})
		}()

		// Create a context with very short timeout
		shortCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()

		// Wait should timeout
		_, err = lm.Wait(shortCtx, result.ContainerID)
		assert.Error(t, err)
	})
}

func TestLifecycleString(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	str := lm.String()
	assert.Contains(t, str, "LifecycleManager")
	assert.Contains(t, str, "Client")
}

// Test concurrent operations to ensure thread safety
func TestLifecycleConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	lm, err := NewLifecycleManager(client, nil)
	require.NoError(t, err)

	ctx := context.Background()
	sessionID := types.NewID("test-session")

	// Create multiple containers
	containers := make([]string, 3)
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("baaaht-concurrent-test-%d", i)
		result, err := creator.CreateWithDefaults(ctx, "alpine:latest", name, sessionID)
		require.NoError(t, err)
		containers[i] = result.ContainerID

		// Ensure cleanup
		defer func(containerID string, containerName string) {
			_ = lm.Destroy(ctx, DestroyConfig{
				ContainerID: containerID,
				Name:        containerName,
				Force:       true,
			})
		}(result.ContainerID, name)
	}

	// Perform concurrent operations
	t.Run("concurrent starts", func(t *testing.T) {
		done := make(chan bool, 3)

		for _, containerID := range containers {
			go func(cid string) {
				err := lm.Start(ctx, StartConfig{ContainerID: cid})
				assert.NoError(t, err)
				done <- true
			}(containerID)
		}

		// Wait for all to complete
		for i := 0; i < 3; i++ {
			<-done
		}

		// Verify all are running
		for _, containerID := range containers {
			running, err := lm.IsRunning(ctx, containerID)
			assert.NoError(t, err)
			assert.True(t, running)
		}
	})

	t.Run("concurrent stops", func(t *testing.T) {
		done := make(chan bool, 3)

		for _, containerID := range containers {
			go func(cid string) {
				err := lm.Stop(ctx, StopConfig{ContainerID: cid})
				assert.NoError(t, err)
				done <- true
			}(containerID)
		}

		// Wait for all to complete
		for i := 0; i < 3; i++ {
			<-done
		}
	})
}
