package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/orchestrator"
	"github.com/billm/baaaht/orchestrator/pkg/scheduler"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadTestConfig loads config and overrides paths that require root to use temp dirs
func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load()
	require.NoError(t, err, "Failed to load config")

	tmpDir := t.TempDir()
	cfg.Credentials.StorePath = filepath.Join(tmpDir, "credentials")
	cfg.Session.StoragePath = filepath.Join(tmpDir, "sessions")
	cfg.IPC.SocketPath = filepath.Join(tmpDir, "ipc.sock")
	return cfg
}

// createLongRunningContainer creates a container that stays alive (sleep 300)
func createLongRunningContainer(t *testing.T, ctx context.Context, creator *container.Creator, name string, sessionID types.ID) *container.CreateResult {
	t.Helper()
	cfg := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels:  make(map[string]string),
		},
		Name:        name,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}
	cfg.Config.Labels["baaaht.session_id"] = sessionID.String()
	cfg.Config.Labels["baaaht.managed"] = "true"

	result, err := creator.Create(ctx, cfg)
	require.NoError(t, err, "Failed to create container")
	return result
}

// TestE2EOrchestratorWorkflow performs an end-to-end test of the complete orchestrator workflow
func TestE2EOrchestratorWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	// Step 1: Bootstrap orchestrator (start orchestrator)
	t.Log("=== Step 1: Bootstrapping orchestrator ===")

	cfg := loadTestConfig(t)

	// Disable health checks for faster testing
	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-e2e-1.0.0",
		ShutdownTimeout:     30 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	require.NotNil(t, result.Orchestrator, "Orchestrator should not be nil")
	require.True(t, result.IsSuccessful(), "Bootstrap should be successful")

	orch := result.Orchestrator
	t.Logf("Orchestrator bootstrapped successfully in %v", result.Duration())

	// Ensure cleanup
	t.Cleanup(func() {
		t.Log("=== Step 6: Shutting down orchestrator ===")
		_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := orch.Close(); err != nil {
			t.Logf("Warning: Orchestrator close returned error: %v", err)
		}

		t.Log("Orchestrator shutdown complete")
	})

	// Verify all subsystems are initialized
	assert.True(t, orch.IsStarted(), "Orchestrator should be started")
	assert.False(t, orch.IsClosed(), "Orchestrator should not be closed")

	// Step 2: Create a test session
	t.Log("=== Step 2: Creating test session ===")

	sessionMgr := orch.SessionManager()
	require.NotNil(t, sessionMgr, "Session manager should not be nil")

	sessionMetadata := types.SessionMetadata{
		Name:        "e2e-test-session",
		Description: "End-to-end test session",
		OwnerID:     "e2e-test-user",
		Labels: map[string]string{
			"test":     "e2e",
			"workflow": "complete",
		},
	}

	sessionConfig := types.SessionConfig{
		MaxContainers: 10,
		MaxDuration:   1 * time.Hour,
		IdleTimeout:   30 * time.Minute,
	}

	sessionID, err := sessionMgr.Create(ctx, sessionMetadata, sessionConfig)
	require.NoError(t, err, "Failed to create session")
	require.False(t, sessionID.IsEmpty(), "Session ID should not be empty")

	t.Logf("Session created: %s", sessionID)

	// Retrieve and verify session
	session, err := sessionMgr.Get(ctx, sessionID)
	require.NoError(t, err, "Failed to get session")
	require.Equal(t, sessionID, session.ID)
	require.Equal(t, "e2e-test-session", session.Metadata.Name)
	require.Equal(t, types.SessionStateActive, session.State)

	// Step 3: Launch a test container
	t.Log("=== Step 3: Launching test container ===")

	dockerClient := orch.DockerClient()
	require.NotNil(t, dockerClient, "Docker client should not be nil")

	creator, err := container.NewCreator(dockerClient, log)
	require.NoError(t, err, "Failed to create container creator")

	// Use a unique container name for each test
	containerName := fmt.Sprintf("baaaht-e2e-test-%d", time.Now().Unix())

	t.Logf("Creating container: %s", containerName)

	createResult := createLongRunningContainer(t, ctx, creator, containerName, sessionID)
	require.NotNil(t, createResult, "Create result should not be nil")
	require.NotEmpty(t, createResult.ContainerID, "Container ID should not be empty")

	containerID := createResult.ContainerID
	t.Logf("Container created: %s", containerID)

	// Start the container
	lifecycleMgr, err := container.NewLifecycleManager(dockerClient, log)
	require.NoError(t, err, "Failed to create lifecycle manager")

	startCfg := container.StartConfig{
		ContainerID: containerID,
		Name:        containerName,
	}

	err = lifecycleMgr.Start(ctx, startCfg)
	require.NoError(t, err, "Failed to start container")
	t.Logf("Container started: %s", containerName)

	// Ensure container is cleaned up
	t.Cleanup(func() {
		t.Log("Cleaning up container...")
		destroyCfg := container.DestroyConfig{
			ContainerID:   containerID,
			Name:          containerName,
			Force:         true,
			RemoveVolumes: true,
		}
		if err := lifecycleMgr.Destroy(ctx, destroyCfg); err != nil {
			t.Logf("Warning: Failed to destroy container: %v", err)
		}
	})

	// Verify container is running
	status, err := lifecycleMgr.Status(ctx, containerID)
	require.NoError(t, err, "Failed to get container status")
	require.Equal(t, types.ContainerStateRunning, status, "Container should be running")
	isRunning, _ := lifecycleMgr.IsRunning(ctx, containerID)
	require.True(t, isRunning, "Container should be running")

	// Step 4: Send event through IPC (event bus)
	t.Log("=== Step 4: Sending event through event bus ===")

	eventBus := orch.EventBus()
	require.NotNil(t, eventBus, "Event bus should not be nil")

	// Create a counter for event delivery
	var eventReceived atomic.Int32

	// Subscribe to container started events
	eventType := types.EventTypeContainerStarted
	filter := types.EventFilter{
		Type: &eventType,
	}

	handler := types.EventFunc(func(ctx context.Context, event types.Event) error {
		t.Logf("Event received: type=%s, id=%s", event.Type, event.ID)
		eventReceived.Add(1)
		return nil
	})

	subID, err := eventBus.Subscribe(ctx, filter, handler)
	require.NoError(t, err, "Failed to subscribe to events")
	require.NotEmpty(t, subID, "Subscription ID should not be empty")
	t.Logf("Subscribed to events: %s", subID)

	// Publish a test event
	containerIDTyped := types.ID(containerID)
	testEvent := types.Event{
		Type:      types.EventTypeContainerStarted,
		Source:    "e2e-test",
		Timestamp: types.NewTimestampFromTime(time.Now()),
		Data: map[string]interface{}{
			"container_id":   containerID,
			"container_name": containerName,
			"session_id":     sessionID,
		},
		Metadata: types.EventMetadata{
			SessionID:   &sessionID,
			ContainerID: &containerIDTyped,
			Priority:    types.PriorityNormal,
		},
	}

	err = eventBus.Publish(ctx, testEvent)
	require.NoError(t, err, "Failed to publish event")
	t.Logf("Event published: %s", testEvent.ID)

	// Wait a bit for async event delivery
	time.Sleep(500 * time.Millisecond)

	// Verify event was received
	receivedCount := eventReceived.Load()
	assert.Greater(t, receivedCount, int32(0), "At least one event should have been received")

	// Unsubscribe from events
	err = eventBus.Unsubscribe(subID)
	require.NoError(t, err, "Failed to unsubscribe")

	// Step 5: Verify container is monitored
	t.Log("=== Step 5: Verifying container is monitored ===")

	monitor, err := container.NewMonitor(dockerClient, log)
	require.NoError(t, err, "Failed to create monitor")

	// Perform health check
	healthResult, err := monitor.HealthCheck(ctx, containerID)
	require.NoError(t, err, "Failed to perform health check")
	require.NotNil(t, healthResult, "Health check result should not be nil")

	t.Logf("Container health check: status=%s", healthResult.Status)

	// Container should be healthy (running without healthcheck = healthy inference)
	assert.Equal(t, types.Healthy, healthResult.Status, "Container should be healthy")

	// Get container stats
	stats, err := monitor.Stats(ctx, containerID)
	require.NoError(t, err, "Failed to get container stats")
	require.NotNil(t, stats, "Container stats should not be nil")

	t.Logf("Container stats: cpu=%.2f%%, memory=%d bytes, pids=%d",
		stats.Resources.CPUPercent, stats.Resources.MemoryUsage, stats.Resources.PidsCount)

	// Verify we got some stats (may be zero for idle container)
	assert.GreaterOrEqual(t, stats.Resources.PidsCount, int64(1), "Container should have at least 1 PID")

	// Get orchestrator stats
	orchStats := orch.Stats(ctx)
	require.NotNil(t, orchStats, "Orchestrator stats should not be nil")

	t.Logf("Orchestrator stats: %+v", orchStats)

	// Verify session is tracked in session manager
	sessions, err := sessionMgr.List(ctx, nil)
	require.NoError(t, err, "Failed to list sessions")
	require.GreaterOrEqual(t, len(sessions), 1, "At least one session should exist")

	// Verify we can get the session by ID
	testSession, err := sessionMgr.Get(ctx, sessionID)
	require.NoError(t, err, "Failed to get test session")
	require.Equal(t, sessionID, testSession.ID)
	t.Logf("Session state: %s, status: %s", testSession.State, testSession.Status)

	// Test scheduler (submit a simple task)
	t.Log("=== Step 5b: Testing scheduler ===")

	sched := orch.Scheduler()
	require.NotNil(t, sched, "Scheduler should not be nil")

	taskExecuted := make(chan struct{}, 1)

	taskHandler := func(ctx context.Context, task *scheduler.Task) error {
		t.Log("Task executed successfully")
		close(taskExecuted)
		return nil
	}

	// Submit task with name option
	taskID, err := sched.Submit(ctx, taskHandler, scheduler.WithTaskName("e2e-test-task"))
	require.NoError(t, err, "Failed to submit task")
	require.NotEmpty(t, taskID, "Task ID should not be empty")
	t.Logf("Task submitted: %s", taskID)

	// Wait for task execution or timeout
	select {
	case <-taskExecuted:
		t.Log("Task executed successfully")
	case <-time.After(5 * time.Second):
		t.Error("Task execution timeout")
	}

	// Verify task stats
	taskStats := sched.Stats()
	t.Logf("Scheduler stats: %+v", taskStats)
	assert.Greater(t, taskStats.TotalTasks, int64(0), "At least one task should have been submitted")

	// Stop the container
	t.Log("=== Step 5c: Stopping container ===")
	timeout := 10 * time.Second
	stopCfg := container.StopConfig{
		ContainerID: containerID,
		Name:        containerName,
		Timeout:     &timeout,
	}

	err = lifecycleMgr.Stop(ctx, stopCfg)
	require.NoError(t, err, "Failed to stop container")
	t.Logf("Container stopped: %s", containerName)

	// Verify container is stopped
	status, err = lifecycleMgr.Status(ctx, containerID)
	require.NoError(t, err, "Failed to get container status after stop")
	assert.Equal(t, types.ContainerStateExited, status, "Container should be exited")

	// Test complete
	t.Log("=== E2E Test Complete ===")
	t.Log("All steps passed successfully:")
	t.Log("  1. Orchestrator bootstrapped")
	t.Log("  2. Session created and verified")
	t.Log("  3. Container created and started")
	t.Log("  4. Event published and received")
	t.Log("  5. Container monitored (health check and stats)")
	t.Log("  6. Scheduler task executed")
	t.Log("  7. Container stopped")
	t.Log("  8. Orchestrator shutdown (via cleanup)")
}

// TestE2EOrchestratorGracefulShutdown tests graceful shutdown with active containers
func TestE2EOrchestratorGracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	t.Log("=== Testing graceful shutdown with active containers ===")

	cfg := loadTestConfig(t)

	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:            *cfg,
		Logger:            log,
		Version:           "test-e2e-shutdown-1.0.0",
		ShutdownTimeout:   10 * time.Second,
		EnableHealthCheck: false,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err)
	orch := result.Orchestrator

	// Create a session
	sessionMgr := orch.SessionManager()
	sessionID, err := sessionMgr.Create(ctx, types.SessionMetadata{
		Name:    "shutdown-test-session",
		OwnerID: "e2e-test-user",
	}, types.SessionConfig{
		MaxContainers: 5,
	})
	require.NoError(t, err)

	// Create and start a container
	dockerClient := orch.DockerClient()
	creator, err := container.NewCreator(dockerClient, log)
	require.NoError(t, err)

	containerName := fmt.Sprintf("baaaht-shutdown-test-%d", time.Now().Unix())
	createResult := createLongRunningContainer(t, ctx, creator, containerName, sessionID)

	lifecycleMgr, err := container.NewLifecycleManager(dockerClient, log)
	require.NoError(t, err)

	err = lifecycleMgr.Start(ctx, container.StartConfig{
		ContainerID: createResult.ContainerID,
		Name:        containerName,
	})
	require.NoError(t, err)

	// Cleanup container after test
	defer func() {
		_ = lifecycleMgr.Destroy(ctx, container.DestroyConfig{
			ContainerID:   createResult.ContainerID,
			Name:          containerName,
			Force:         true,
			RemoveVolumes: true,
		})
	}()

	t.Logf("Container %s is running", containerName)

	// Shutdown orchestrator with active container
	t.Log("Shutting down orchestrator with active container...")

	// Close should handle cleanup gracefully even with active containers
	startTime := time.Now()
	err = orch.Close()
	elapsed := time.Since(startTime)

	// Close should succeed without error
	assert.NoError(t, err, "Orchestrator close should succeed")
	t.Logf("Graceful shutdown completed in %v", elapsed)

	// Verify orchestrator is closed
	assert.True(t, orch.IsClosed(), "Orchestrator should be closed")
	assert.False(t, orch.IsStarted(), "Orchestrator should not be started")

	t.Log("Graceful shutdown test passed")
}

// TestE2EOrchestratorWithShutdownManager tests the orchestrator with shutdown manager
func TestE2EOrchestratorWithShutdownManager(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	t.Log("=== Testing orchestrator with shutdown manager ===")

	cfg := loadTestConfig(t)

	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:            *cfg,
		Logger:            log,
		Version:           "test-e2e-shutdownmgr-1.0.0",
		ShutdownTimeout:   10 * time.Second,
		EnableHealthCheck: false,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err)
	orch := result.Orchestrator

	// Create shutdown manager
	shutdownMgr := orchestrator.NewShutdownManager(orch, 10*time.Second, log)

	// Add a shutdown hook
	hookExecuted := false
	shutdownMgr.AddHook(func(ctx context.Context) error {
		t.Log("Shutdown hook executed")
		hookExecuted = true
		return nil
	})

	// Start signal handling (simulated)
	shutdownMgr.Start()

	// Create a session and container for realistic testing
	sessionMgr := orch.SessionManager()
	sessionID, err := sessionMgr.Create(ctx, types.SessionMetadata{
		Name:    "shutdownmgr-test-session",
		OwnerID: "e2e-test-user",
	}, types.SessionConfig{})
	require.NoError(t, err)

	dockerClient := orch.DockerClient()
	creator, err := container.NewCreator(dockerClient, log)
	require.NoError(t, err)

	containerName := fmt.Sprintf("baaaht-shutdownmgr-%d", time.Now().Unix())
	createResult := createLongRunningContainer(t, ctx, creator, containerName, sessionID)

	// Cleanup
	defer func() {
		lifecycleMgr, _ := container.NewLifecycleManager(dockerClient, log)
		_ = lifecycleMgr.Destroy(ctx, container.DestroyConfig{
			ContainerID:   createResult.ContainerID,
			Name:          containerName,
			Force:         true,
			RemoveVolumes: true,
		})
	}()

	t.Log("Triggering shutdown via shutdown manager...")

	// Trigger shutdown
	go func() {
		time.Sleep(100 * time.Millisecond)
		shutdownMgr.Shutdown(context.Background(), "test shutdown")
	}()

	// Wait for shutdown completion
	waitCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = shutdownMgr.WaitCompletion(waitCtx)
	assert.NoError(t, err, "WaitCompletion should succeed")

	// Verify hook was executed
	assert.True(t, hookExecuted, "Shutdown hook should have been executed")

	// Verify shutdown state
	state := shutdownMgr.State()
	assert.Equal(t, orchestrator.ShutdownStateComplete, state, "Shutdown state should be complete")

	t.Log("Shutdown manager test passed")
}

// TestE2EOrchestratorMultipleSessions tests handling multiple concurrent sessions
func TestE2EOrchestratorMultipleSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	t.Log("=== Testing multiple concurrent sessions ===")

	cfg := loadTestConfig(t)

	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:            *cfg,
		Logger:            log,
		Version:           "test-e2e-multisession-1.0.0",
		ShutdownTimeout:   15 * time.Second,
		EnableHealthCheck: false,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err)
	orch := result.Orchestrator

	defer func() {
		_ = orch.Close()
	}()

	sessionMgr := orch.SessionManager()

	// Create multiple sessions concurrently
	numSessions := 3
	sessionIDs := make([]types.ID, numSessions)

	for i := 0; i < numSessions; i++ {
		sessionMetadata := types.SessionMetadata{
			Name:    fmt.Sprintf("multi-session-%d", i),
			OwnerID: "e2e-test-user",
			Labels: map[string]string{
				"test":  "multi",
				"index": fmt.Sprintf("%d", i),
			},
		}

		sessionID, err := sessionMgr.Create(ctx, sessionMetadata, types.SessionConfig{
			MaxContainers: 5,
		})
		require.NoError(t, err, "Failed to create session %d", i)
		sessionIDs[i] = sessionID
		t.Logf("Created session %d: %s", i, sessionID)
	}

	// List all sessions
	sessions, err := sessionMgr.List(ctx, nil)
	require.NoError(t, err, "Failed to list sessions")
	require.GreaterOrEqual(t, len(sessions), numSessions, "Should have at least %d sessions", numSessions)

	t.Logf("Total sessions: %d", len(sessions))

	// Verify each session can be retrieved
	for i, sessionID := range sessionIDs {
		session, err := sessionMgr.Get(ctx, sessionID)
		require.NoError(t, err, "Failed to get session %d", i)
		require.Equal(t, sessionID, session.ID)
		require.Equal(t, fmt.Sprintf("multi-session-%d", i), session.Metadata.Name)
		t.Logf("Session %d verified: %s (state=%s)", i, session.ID, session.State)
	}

	t.Log("Multiple sessions test passed")
}

// TestE2EOrchestratorWaitForSignalLifecycle tests the exact lifecycle pattern from main.go:
// bootstrap -> Start() -> WaitCompletion() -> Stop(). Verifies that Start() does NOT
// trigger an immediate shutdown and WaitCompletion blocks until a real signal arrives.
// This would have caught the original bug where waitForShutdown() called ShutdownAndWait()
// (initiating shutdown) instead of WaitCompletion() (waiting for shutdown).
func TestE2EOrchestratorWaitForSignalLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	t.Log("=== Testing main.go lifecycle: Start() should not trigger shutdown ===")

	cfg := loadTestConfig(t)

	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:            *cfg,
		Logger:            log,
		Version:           "test-e2e-lifecycle-1.0.0",
		ShutdownTimeout:   10 * time.Second,
		EnableHealthCheck: false,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err)
	orch := result.Orchestrator

	// Create shutdown manager (mirrors main.go)
	shutdownMgr := orchestrator.NewShutdownManager(orch, 10*time.Second, log)

	// Buffer size 2: hooks run in both pre-shutdown and post-shutdown phases
	hookExecuted := make(chan struct{}, 2)
	shutdownMgr.AddHook(func(ctx context.Context) error {
		hookExecuted <- struct{}{}
		return nil
	})

	// Start signal handling (mirrors main.go)
	shutdownMgr.Start()

	// WaitCompletion in a goroutine (mirrors waitForShutdown in main.go)
	lifecycleDone := make(chan struct{})
	go func() {
		defer close(lifecycleDone)
		_ = shutdownMgr.WaitCompletion(context.Background())
		shutdownMgr.Stop()
	}()

	// KEY ASSERTION: After Start(), the orchestrator should still be running
	// If this fails, it means Start() or WaitCompletion() is triggering immediate shutdown
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, orchestrator.ShutdownStateRunning, shutdownMgr.State(),
		"Orchestrator should still be in Running state after Start()")
	assert.False(t, shutdownMgr.IsShuttingDown(),
		"Orchestrator should not be shutting down after Start()")
	assert.True(t, orch.IsStarted(),
		"Orchestrator should still be started")

	select {
	case <-lifecycleDone:
		t.Fatal("Lifecycle completed without signal — this is the immediate-shutdown bug")
	default:
		t.Log("Good: orchestrator is still running after 500ms")
	}

	select {
	case <-hookExecuted:
		t.Fatal("Shutdown hook fired without signal — shutdown was triggered prematurely")
	default:
		t.Log("Good: no shutdown hooks have fired")
	}

	// Now trigger shutdown explicitly to clean up
	t.Log("Triggering shutdown to clean up...")
	go func() {
		_ = shutdownMgr.Shutdown(context.Background(), "test cleanup")
	}()

	select {
	case <-lifecycleDone:
		t.Log("Lifecycle completed after explicit shutdown trigger")
	case <-time.After(10 * time.Second):
		t.Fatal("Lifecycle did not complete after shutdown trigger")
	}

	assert.True(t, shutdownMgr.IsComplete(), "Shutdown should be complete")
	t.Log("Lifecycle pattern test passed")
}
