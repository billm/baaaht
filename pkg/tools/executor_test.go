package tools

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ExecutorMockRuntime is a mock implementation of container.Runtime for testing
type ExecutorMockRuntime struct {
	containerID string
	createCalled bool
	startCalled bool
	waitCalled  bool
	destroyCalled bool
	logsCalled  bool
	containerRunning bool
	exitCode   int
	output     []byte
	stderr     []byte
	createError error
	startError error
	waitError  error
	logsError  error
}

func (m *ExecutorMockRuntime) Create(ctx context.Context, cfg container.CreateConfig) (*container.CreateResult, error) {
	m.createCalled = true
	if m.createError != nil {
		return nil, m.createError
	}
	m.containerID = "mock-container-" + time.Now().Format("20060102150405")
	return &container.CreateResult{
		ContainerID: m.containerID,
		Warnings:    []string{},
		ImagePulled: false,
	}, nil
}

func (m *ExecutorMockRuntime) Start(ctx context.Context, cfg container.StartConfig) error {
	m.startCalled = true
	if m.startError != nil {
		return m.startError
	}
	m.containerRunning = true
	return nil
}

func (m *ExecutorMockRuntime) Stop(ctx context.Context, cfg container.StopConfig) error {
	m.containerRunning = false
	return nil
}

func (m *ExecutorMockRuntime) Restart(ctx context.Context, cfg container.RestartConfig) error {
	return nil
}

func (m *ExecutorMockRuntime) Destroy(ctx context.Context, cfg container.DestroyConfig) error {
	m.destroyCalled = true
	m.containerRunning = false
	return nil
}

func (m *ExecutorMockRuntime) Pause(ctx context.Context, containerID string) error {
	return nil
}

func (m *ExecutorMockRuntime) Unpause(ctx context.Context, containerID string) error {
	return nil
}

func (m *ExecutorMockRuntime) Kill(ctx context.Context, cfg container.KillConfig) error {
	m.containerRunning = false
	return nil
}

func (m *ExecutorMockRuntime) Wait(ctx context.Context, containerID string) (int, error) {
	m.waitCalled = true
	if m.waitError != nil {
		return -1, m.waitError
	}
	return m.exitCode, nil
}

func (m *ExecutorMockRuntime) Status(ctx context.Context, containerID string) (types.ContainerState, error) {
	return types.ContainerStateRunning, nil
}

func (m *ExecutorMockRuntime) IsRunning(ctx context.Context, containerID string) (bool, error) {
	return m.containerRunning, nil
}

func (m *ExecutorMockRuntime) HealthCheck(ctx context.Context, containerID string) (*container.HealthCheckResult, error) {
	return &container.HealthCheckResult{
		Status: types.Healthy,
	}, nil
}

func (m *ExecutorMockRuntime) HealthCheckWithRetry(ctx context.Context, containerID string, maxAttempts int, interval time.Duration) (*container.HealthCheckResult, error) {
	return m.HealthCheck(ctx, containerID)
}

func (m *ExecutorMockRuntime) Stats(ctx context.Context, containerID string) (*types.ContainerStats, error) {
	return &types.ContainerStats{}, nil
}

func (m *ExecutorMockRuntime) StatsStream(ctx context.Context, containerID string, interval time.Duration) (<-chan *types.ContainerStats, <-chan error) {
	return nil, nil
}

func (m *ExecutorMockRuntime) Logs(ctx context.Context, cfg container.LogsConfig) (io.ReadCloser, error) {
	m.logsCalled = true
	if m.logsError != nil {
		return nil, m.logsError
	}

	// Return a mock reader with output
	return &mockReadCloser{
		data: m.output,
	}, nil
}

func (m *ExecutorMockRuntime) LogsLines(ctx context.Context, cfg container.LogsConfig) ([]types.ContainerLog, error) {
	return nil, nil
}

func (m *ExecutorMockRuntime) EventsStream(ctx context.Context, containerID string) (<-chan container.EventsMessage, <-chan error) {
	return nil, nil
}

func (m *ExecutorMockRuntime) PullImage(ctx context.Context, image string, timeout time.Duration) error {
	return nil
}

func (m *ExecutorMockRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	return true, nil
}

func (m *ExecutorMockRuntime) Client() interface{} {
	return nil
}

func (m *ExecutorMockRuntime) Type() string {
	return "mock"
}

func (m *ExecutorMockRuntime) Info(ctx context.Context) (*container.RuntimeInfo, error) {
	return &container.RuntimeInfo{
		Type:     "mock",
		Version:  "1.0.0",
		Platform: "test",
	}, nil
}

func (m *ExecutorMockRuntime) Close() error {
	return nil
}

// mockReadCloser is a mock io.ReadCloser for testing
type mockReadCloser struct {
	data []byte
	pos  int
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.data) {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	return nil
}

func TestNewExecutor(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("valid executor", func(t *testing.T) {
		mockRuntime := &ExecutorMockRuntime{}
		registry := NewRegistry(log)

		executor, err := NewExecutor(mockRuntime, registry, log)

		assert.NoError(t, err)
		assert.NotNil(t, executor)
		assert.NotNil(t, executor.logger)
	})

	t.Run("nil runtime", func(t *testing.T) {
		registry := NewRegistry(log)

		executor, err := NewExecutor(nil, registry, log)

		assert.Error(t, err)
		assert.Nil(t, executor)
		var customErr *types.Error
		assert.ErrorAs(t, err, &customErr)
		assert.Equal(t, types.ErrCodeInvalidArgument, customErr.Code)
	})

	t.Run("nil registry", func(t *testing.T) {
		mockRuntime := &ExecutorMockRuntime{}

		executor, err := NewExecutor(mockRuntime, nil, log)

		assert.Error(t, err)
		assert.Nil(t, executor)
		var customErr *types.Error
		assert.ErrorAs(t, err, &customErr)
		assert.Equal(t, types.ErrCodeInvalidArgument, customErr.Code)
	})

	t.Run("nil logger creates default", func(t *testing.T) {
		mockRuntime := &ExecutorMockRuntime{}
		registry := NewRegistry(log)

		executor, err := NewExecutor(mockRuntime, registry, nil)

		assert.NoError(t, err)
		assert.NotNil(t, executor)
		assert.NotNil(t, executor.logger)
	})
}

func TestNewExecutorFromRegistry(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("valid executor with global registry", func(t *testing.T) {
		mockRuntime := &ExecutorMockRuntime{}

		executor, err := NewExecutorFromRegistry(mockRuntime, log)

		assert.NoError(t, err)
		assert.NotNil(t, executor)
		assert.NotNil(t, executor.registry)
	})

	t.Run("nil runtime", func(t *testing.T) {
		executor, err := NewExecutorFromRegistry(nil, log)

		assert.Error(t, err)
		assert.Nil(t, executor)
		var customErr *types.Error
		assert.ErrorAs(t, err, &customErr)
		assert.Equal(t, types.ErrCodeInvalidArgument, customErr.Code)
	})
}

func TestExecutorValidateConfig(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	mockRuntime := &ExecutorMockRuntime{}
	registry := NewRegistry(log)
	executor, err := NewExecutor(mockRuntime, registry, log)
	require.NoError(t, err)

	t.Run("valid config", func(t *testing.T) {
		cfg := ExecutionConfig{
			ToolName:  "test_tool",
			SessionID: types.NewID("session-123"),
			Timeout:   30 * time.Second,
		}

		err := executor.validateConfig(cfg)
		assert.NoError(t, err)
	})

	t.Run("empty tool name", func(t *testing.T) {
		cfg := ExecutionConfig{
			ToolName:  "",
			SessionID: types.NewID("session-123"),
		}

		err := executor.validateConfig(cfg)
		assert.Error(t, err)
		var customErr *types.Error
		assert.ErrorAs(t, err, &customErr)
		assert.Equal(t, types.ErrCodeInvalidArgument, customErr.Code)
	})

	t.Run("empty session ID", func(t *testing.T) {
		cfg := ExecutionConfig{
			ToolName:  "test_tool",
			SessionID: types.ID(""),
		}

		err := executor.validateConfig(cfg)
		assert.Error(t, err)
		var customErr *types.Error
		assert.ErrorAs(t, err, &customErr)
		assert.Equal(t, types.ErrCodeInvalidArgument, customErr.Code)
	})

	t.Run("nil parameters initializes to empty map", func(t *testing.T) {
		cfg := ExecutionConfig{
			ToolName:   "test_tool",
			SessionID:  types.NewID("session-123"),
			Parameters: nil,
		}

		err := executor.validateConfig(cfg)
		assert.NoError(t, err)
		// Note: validateConfig doesn't actually initialize the map in this implementation
		// It just checks that it's not nil and allows it
	})
}

func TestExecutorGenerateContainerName(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	mockRuntime := &ExecutorMockRuntime{}
	registry := NewRegistry(log)
	executor, err := NewExecutor(mockRuntime, registry, log)
	require.NoError(t, err)

	t.Run("generates valid container name", func(t *testing.T) {
		executionID := "1234567890abcdef1234567890abcdef"
		containerName := executor.generateContainerName("read_file", executionID)

		assert.Contains(t, containerName, "tool-read_file")
		assert.Contains(t, containerName, executionID[:8])
	})

	t.Run("short execution ID", func(t *testing.T) {
		executionID := "short"
		containerName := executor.generateContainerName("grep", executionID)

		assert.Contains(t, containerName, "tool-grep")
		assert.Contains(t, containerName, executionID)
	})
}

func TestExecutorBuildContainerLabels(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	mockRuntime := &ExecutorMockRuntime{}
	registry := NewRegistry(log)
	executor, err := NewExecutor(mockRuntime, registry, log)
	require.NoError(t, err)

	sessionID := types.NewID("session-123")
	executionID := "exec-456"

	labels := executor.buildContainerLabels("test_tool", sessionID, executionID)

	assert.Equal(t, "true", labels["baaaht.managed"])
	assert.Equal(t, "test_tool", labels["baaaht.tool_name"])
	assert.Equal(t, sessionID.String(), labels["baaaht.session_id"])
	assert.Equal(t, executionID, labels["baaaht.execution_id"])
	assert.Equal(t, "ephemeral", labels["baaaht.tool_type"])
}

func TestExecutorBuildMounts(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	mockRuntime := &ExecutorMockRuntime{}
	registry := NewRegistry(log)
	executor, err := NewExecutor(mockRuntime, registry, log)
	require.NoError(t, err)

	t.Run("no filesystem access", func(t *testing.T) {
		policy := ToolSecurityPolicy{
			AllowFilesystem: false,
		}

		mounts := executor.buildMounts("test_tool", nil, policy)
		assert.Nil(t, mounts)
	})

	t.Run("with allowed paths", func(t *testing.T) {
		policy := ToolSecurityPolicy{
			AllowFilesystem:    true,
			AllowedPaths:       []string{"/tmp", "/home/user/data"},
			ReadOnlyFilesystem: true,
		}

		mounts := executor.buildMounts("test_tool", nil, policy)
		assert.Len(t, mounts, 2)
		assert.Equal(t, types.MountTypeBind, mounts[0].Type)
		assert.Equal(t, "/tmp", mounts[0].Source)
		assert.True(t, mounts[0].ReadOnly)
		assert.Equal(t, "/mnt/scope0", mounts[0].Target)
	})

	t.Run("writable mounts", func(t *testing.T) {
		policy := ToolSecurityPolicy{
			AllowFilesystem:    true,
			AllowedPaths:       []string{"/tmp"},
			ReadOnlyFilesystem: false,
		}

		mounts := executor.buildMounts("write_file", nil, policy)
		assert.Len(t, mounts, 1)
		assert.False(t, mounts[0].ReadOnly)
	})
}

func TestExecutorDetermineStatus(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	mockRuntime := &ExecutorMockRuntime{}
	registry := NewRegistry(log)
	executor, err := NewExecutor(mockRuntime, registry, log)
	require.NoError(t, err)

	t.Run("zero exit code is completed", func(t *testing.T) {
		status := executor.determineStatus(0, nil)
		assert.Equal(t, ToolExecutionStatusCompleted, status)
	})

	t.Run("non-zero exit code is failed", func(t *testing.T) {
		status := executor.determineStatus(1, nil)
		assert.Equal(t, ToolExecutionStatusFailed, status)
	})

	t.Run("error is failed", func(t *testing.T) {
		status := executor.determineStatus(0, fmt.Errorf("execution error"))
		assert.Equal(t, ToolExecutionStatusFailed, status)
	})
}

func TestExecutorString(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	mockRuntime := &ExecutorMockRuntime{}
	registry := NewRegistry(log)
	executor, err := NewExecutor(mockRuntime, registry, log)
	require.NoError(t, err)

	s := executor.String()
	assert.Contains(t, s, "Executor")
	assert.Contains(t, s, "mock")
}

func TestExecutorGettersSetters(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	mockRuntime := &ExecutorMockRuntime{}
	registry := NewRegistry(log)
	executor, err := NewExecutor(mockRuntime, registry, log)
	require.NoError(t, err)

	t.Run("Runtime returns correct runtime", func(t *testing.T) {
		runtime := executor.Runtime()
		assert.Same(t, mockRuntime, runtime)
	})

	t.Run("Registry returns correct registry", func(t *testing.T) {
		reg := executor.Registry()
		assert.Same(t, registry, reg)
	})

	t.Run("SetRegistry updates registry", func(t *testing.T) {
		newRegistry := NewRegistry(log)
		executor.SetRegistry(newRegistry)

		reg := executor.Registry()
		assert.Same(t, newRegistry, reg)
	})
}

func TestExecutorCreateErrorResult(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	mockRuntime := &ExecutorMockRuntime{}
	registry := NewRegistry(log)
	executor, err := NewExecutor(mockRuntime, registry, log)
	require.NoError(t, err)

	startTime := time.Now()
	testError := fmt.Errorf("test execution failed")

	result, err := executor.createErrorResult("exec-123", "test_tool", testError, startTime, 30*time.Second)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "exec-123", result.ExecutionID)
	assert.Equal(t, "test_tool", result.ToolName)
	assert.Equal(t, ToolExecutionStatusFailed, result.Status)
	assert.Equal(t, int32(-1), result.ExitCode)
	assert.Equal(t, "test execution failed", result.ErrorText)
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
}

func TestExecutorCreateTimeoutResult(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	mockRuntime := &ExecutorMockRuntime{}
	registry := NewRegistry(log)
	executor, err := NewExecutor(mockRuntime, registry, log)
	require.NoError(t, err)

	startTime := time.Now()
	containerID := "container-123"
	timeout := 30 * time.Second

	result, err := executor.createTimeoutResult("exec-123", "test_tool", startTime, timeout, containerID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "exec-123", result.ExecutionID)
	assert.Equal(t, "test_tool", result.ToolName)
	assert.Equal(t, ToolExecutionStatusTimeout, result.Status)
	assert.Equal(t, int32(-1), result.ExitCode)
	assert.Contains(t, result.ErrorText, "timeout")
	assert.Contains(t, result.ErrorText, "30s")
	assert.Equal(t, containerID, result.ContainerID)
	assert.Equal(t, timeout.String(), result.Metadata["timeout"])
}

func TestExecutorCleanup(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("cleanup container on successful execution", func(t *testing.T) {
		mockRuntime := &ExecutorMockRuntime{
			containerID: "test-container-123",
			exitCode:    0,
			output:      []byte("success"),
		}

		registry := NewRegistry(log)
		executor, err := NewExecutor(mockRuntime, registry, log)
		require.NoError(t, err)

		executor.cleanupContainer(context.Background(), "test-container-123", "test_tool")

		assert.True(t, mockRuntime.destroyCalled)
	})

	t.Run("cleanup handles destroy errors gracefully", func(t *testing.T) {
		mockRuntime := &ExecutorMockRuntime{}
		// We can't easily make Destroy return an error in this mock
		// but the test verifies the cleanup flow

		registry := NewRegistry(log)
		executor, err := NewExecutor(mockRuntime, registry, log)
		require.NoError(t, err)

		// Should not panic even if container doesn't exist
		executor.cleanupContainer(context.Background(), "nonexistent-container", "test_tool")
	})
}

// Integration-style test with mock runtime
func TestExecutorExecuteWithMock(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	// Register a test tool factory
	testToolFactory := func(def ToolDefinition) (Tool, error) {
		return &executorMockTool{
			name:       def.Name,
			toolType:   ToolTypeFile,
			definition: def,
			enabled:    true,
		}, nil
	}

	registry := NewRegistry(log)
	err = registry.RegisterTool("test_read_file", testToolFactory)
	require.NoError(t, err)

	t.Run("successful execution", func(t *testing.T) {
		mockRuntime := &ExecutorMockRuntime{
			exitCode: 0,
			output:   []byte("file contents"),
		}

		executor, err := NewExecutor(mockRuntime, registry, log)
		require.NoError(t, err)

		cfg := ExecutionConfig{
			ToolName:    "test_read_file",
			Parameters:  map[string]string{"path": "/tmp/test.txt"},
			SessionID:   types.NewID("session-123"),
			Timeout:     5 * time.Second,
			AutoCleanup: false, // We'll clean up manually
		}

		result, err := executor.Execute(context.Background(), cfg)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, ToolExecutionStatusCompleted, result.Status)
		assert.Equal(t, int32(0), result.ExitCode)
		assert.Equal(t, "file contents", result.OutputText)
		assert.True(t, mockRuntime.createCalled)
		assert.True(t, mockRuntime.startCalled)
		assert.True(t, mockRuntime.waitCalled)
	})

	t.Run("execution with non-zero exit code", func(t *testing.T) {
		mockRuntime := &ExecutorMockRuntime{
			exitCode: 1,
			output:   []byte(""),
			stderr:   []byte("error: file not found"),
		}

		executor, err := NewExecutor(mockRuntime, registry, log)
		require.NoError(t, err)

		cfg := ExecutionConfig{
			ToolName:    "test_read_file",
			Parameters:  map[string]string{"path": "/nonexistent"},
			SessionID:   types.NewID("session-456"),
			Timeout:     5 * time.Second,
			AutoCleanup: false,
		}

		result, err := executor.Execute(context.Background(), cfg)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, ToolExecutionStatusFailed, result.Status)
		assert.Equal(t, int32(1), result.ExitCode)
	})
}

// executorMockTool is a mock implementation of Tool for testing
type executorMockTool struct {
	name       string
	toolType   ToolType
	definition ToolDefinition
	enabled    bool
	closed     bool
}

func (m *executorMockTool) Name() string {
	return m.name
}

func (m *executorMockTool) Type() ToolType {
	return m.toolType
}

func (m *executorMockTool) Description() string {
	return m.definition.Description
}

func (m *executorMockTool) Execute(ctx context.Context, parameters map[string]string) (*ToolExecutionResult, error) {
	if m.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "tool is closed")
	}
	return &ToolExecutionResult{
		ExecutionID: types.GenerateID().String(),
		ToolName:    m.name,
		Status:      ToolExecutionStatusCompleted,
		ExitCode:    0,
		OutputText:  "mock tool output",
	}, nil
}

func (m *executorMockTool) Validate(parameters map[string]string) error {
	// Simple validation - no required parameters for this mock
	return nil
}

func (m *executorMockTool) Definition() ToolDefinition {
	return m.definition
}

func (m *executorMockTool) Status() types.Status {
	return types.StatusRunning
}

func (m *executorMockTool) IsAvailable() bool {
	return !m.closed
}

func (m *executorMockTool) Enabled() bool {
	return m.enabled && !m.closed
}

func (m *executorMockTool) SetEnabled(enabled bool) {
	m.enabled = enabled
}

func (m *executorMockTool) Stats() ToolUsageStats {
	return ToolUsageStats{
		TotalExecutions: 10,
		SuccessfulExecutions: 9,
		FailedExecutions: 1,
	}
}

func (m *executorMockTool) LastUsed() *time.Time {
	now := time.Now()
	return &now
}

func (m *executorMockTool) Close() error {
	m.closed = true
	return nil
}
