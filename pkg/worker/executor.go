package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/policy"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Executor handles tool container execution with policy enforcement
type Executor struct {
	runtime        container.Runtime
	policyEnforcer *policy.Enforcer
	logger         *logger.Logger
	sessionID      types.ID
	mu             sync.RWMutex
	closed         bool

	// Task tracking for cancellation
	runningTasks map[string]*runningTask // taskID -> running task info
}

// runningTask holds information about a currently running task
type runningTask struct {
	TaskID        string
	ContainerID   string
	ContainerName string
	Ctx           context.Context
	CancelFunc    context.CancelFunc
	StartTime     time.Time
}

// ExecutorConfig holds configuration for creating a new Executor
type ExecutorConfig struct {
	Runtime        container.Runtime
	PolicyEnforcer *policy.Enforcer
	SessionID      types.ID
	Logger         *logger.Logger
}

// TaskConfig holds configuration for executing a task
type TaskConfig struct {
	// ToolType specifies which tool to use
	ToolType ToolType

	// Args are additional arguments to pass to the tool command
	Args []string

	// MountSource is the source path for filesystem mounts
	// Required for file operation tools
	MountSource string

	// Timeout overrides the tool's default timeout
	// If zero, the tool's default timeout is used
	Timeout time.Duration

	// ContainerName is an optional name for the container
	// If empty, a name will be generated
	ContainerName string
}

// TaskResult holds the result of executing a task
type TaskResult struct {
	// ExitCode is the container's exit code
	ExitCode int

	// Stdout contains the standard output from the container
	Stdout string

	// Stderr contains the standard error output from the container
	Stderr string

	// Error contains any error that occurred during execution
	// This is different from stderr - it's about the execution itself
	Error error

	// ContainerID is the ID of the container that was created
	ContainerID string

	// StartedAt is when the container started
	StartedAt time.Time

	// CompletedAt is when the container finished
	CompletedAt time.Time

	// Duration is how long the task took to execute
	Duration time.Duration
}

// NewExecutor creates a new tool container executor
func NewExecutor(cfg ExecutorConfig) (*Executor, error) {
	// Create default logger if not provided
	log := cfg.Logger
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Use default session ID if not provided
	sessionID := cfg.SessionID
	if sessionID.IsEmpty() {
		sessionID = types.GenerateID()
	}

	exec := &Executor{
		runtime:        cfg.Runtime,
		policyEnforcer: cfg.PolicyEnforcer,
		logger:         log.With("component", "worker_executor", "session_id", sessionID),
		sessionID:      sessionID,
		closed:         false,
		runningTasks:   make(map[string]*runningTask),
	}

	runtimeType := "none"
	if cfg.Runtime != nil {
		runtimeType = cfg.Runtime.Type()
	}

	exec.logger.Info("Executor initialized",
		"runtime_type", runtimeType,
		"policy_enforcer", cfg.PolicyEnforcer != nil)

	return exec, nil
}

// NewExecutorFromRuntime creates a new executor with just a runtime
// This is a convenience function that creates an executor without a policy enforcer
func NewExecutorFromRuntime(runtime container.Runtime, log *logger.Logger) (*Executor, error) {
	if runtime == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime cannot be nil")
	}

	return NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Logger:  log,
	})
}

// NewExecutorDefault creates a new executor with a global runtime
// This creates a runtime with auto-detection if one doesn't exist
func NewExecutorDefault(log *logger.Logger) (*Executor, error) {
	ctx := context.Background()
	runtime := container.GlobalRuntime()

	// If the global runtime is an error runtime, try to create a new one
	if runtime.Type() == "error" {
		var err error
		runtime, err = container.NewRuntimeDefault(ctx)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default runtime", err)
		}
	}

	return NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Logger:  log,
	})
}

// SetPolicyEnforcer sets or updates the policy enforcer for the executor
func (e *Executor) SetPolicyEnforcer(enforcer *policy.Enforcer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policyEnforcer = enforcer

	if enforcer != nil {
		e.logger.Debug("Policy enforcer set")
	} else {
		e.logger.Debug("Policy enforcer cleared")
	}
}

// PolicyEnforcer returns the current policy enforcer
func (e *Executor) PolicyEnforcer() *policy.Enforcer {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.policyEnforcer
}

// Runtime returns the container runtime
func (e *Executor) Runtime() container.Runtime {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.runtime
}

// SessionID returns the session ID for this executor
func (e *Executor) SessionID() types.ID {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sessionID
}

// SetSessionID sets a new session ID for the executor
func (e *Executor) SetSessionID(sessionID types.ID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessionID = sessionID
	e.logger.Debug("Session ID updated", "session_id", sessionID)
}

// IsClosed returns true if the executor is closed
func (e *Executor) IsClosed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closed
}

// Close closes the executor and releases resources
func (e *Executor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}

	e.closed = true
	e.logger.Info("Executor closed")

	return nil
}

// String returns a string representation of the executor
func (e *Executor) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	enforcerStatus := "none"
	if e.policyEnforcer != nil {
		enforcerStatus = "enabled"
	}

	runtimeType := "none"
	if e.runtime != nil {
		runtimeType = e.runtime.Type()
	}

	return fmt.Sprintf("Executor{Runtime: %s, PolicyEnforcer: %s, SessionID: %s, Closed: %v}",
		runtimeType, enforcerStatus, e.sessionID, e.closed)
}

// ValidateConfig validates a container configuration against the policy enforcer
func (e *Executor) ValidateConfig(ctx context.Context, config types.ContainerConfig) (*policy.ValidationResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "executor is closed")
	}

	// If no policy enforcer is set, allow everything
	if e.policyEnforcer == nil {
		e.logger.Debug("No policy enforcer set, allowing configuration")
		return &policy.ValidationResult{Allowed: true}, nil
	}

	// Validate against policy
	result, err := e.policyEnforcer.ValidateContainerConfig(ctx, e.sessionID, config)
	if err != nil {
		e.logger.Warn("Policy validation failed", "error", err)
		return nil, types.WrapError(types.ErrCodeInternal, "policy validation error", err)
	}

	// Log warnings
	for _, warning := range result.Warnings {
		e.logger.Warn("Policy validation warning",
			"rule", warning.Rule,
			"message", warning.Message,
			"component", warning.Component)
	}

	// Log violations
	for _, violation := range result.Violations {
		if violation.Severity == string(policy.SeverityError) {
			e.logger.Warn("Policy violation detected",
				"rule", violation.Rule,
				"message", violation.Message,
				"component", violation.Component)
		} else {
			e.logger.Info("Policy validation notice",
				"rule", violation.Rule,
				"message", violation.Message,
				"component", violation.Component)
		}
	}

	return result, nil
}

// EnforceConfig enforces policy on a container configuration and returns a modified config
func (e *Executor) EnforceConfig(ctx context.Context, config types.ContainerConfig) (types.ContainerConfig, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return config, types.NewError(types.ErrCodeUnavailable, "executor is closed")
	}

	// If no policy enforcer is set, return config as-is
	if e.policyEnforcer == nil {
		return config, nil
	}

	// Enforce policy
	enforced, err := e.policyEnforcer.EnforceContainerConfig(ctx, e.sessionID, config)
	if err != nil {
		e.logger.Warn("Policy enforcement failed", "error", err)
		if typedErr, ok := err.(*types.Error); ok {
			return config, typedErr
		}
		return config, types.WrapError(types.ErrCodeInternal, "policy enforcement error", err)
	}

	return enforced, nil
}

// ExecuteTask executes a task using a tool container
// The method handles the complete lifecycle: create, start, wait, capture output, and cleanup
func (e *Executor) ExecuteTask(ctx context.Context, cfg TaskConfig) *TaskResult {
	startTime := time.Now()

	// Generate a task ID for tracking
	taskID := types.GenerateID().String()

	// Create a cancelable context for this task
	taskCtx, cancelFunc := context.WithCancel(ctx)

	result := &TaskResult{
		StartedAt: startTime,
	}

	// Track this running task
	taskInfo := &runningTask{
		TaskID:        taskID,
		ContainerID:   "", // Will be set after container creation
		ContainerName: "", // Will be set after container creation
		Ctx:           taskCtx,
		CancelFunc:    cancelFunc,
		StartTime:     startTime,
	}

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		result.Error = types.NewError(types.ErrCodeUnavailable, "executor is closed")
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(startTime)
		cancelFunc()
		return result
	}
	e.runningTasks[taskID] = taskInfo
	e.mu.Unlock()

	if e.runtime == nil {
		result.Error = types.NewError(types.ErrCodeUnavailable, "runtime is not configured")
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(startTime)
		return result
	}

	// Ensure cleanup: remove from tracking and cancel context
	defer func() {
		e.mu.Lock()
		delete(e.runningTasks, taskID)
		e.mu.Unlock()
		cancelFunc()
	}()

	// Get the tool specification
	toolSpec, err := GetToolSpec(cfg.ToolType)
	if err != nil {
		result.Error = types.WrapError(types.ErrCodeInvalidArgument, "invalid tool type", err)
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(startTime)
		e.logger.Warn("Failed to get tool spec", "tool_type", cfg.ToolType, "error", err)
		return result
	}

	e.logger.Info("Executing task",
		"tool_type", cfg.ToolType,
		"tool_name", toolSpec.Name,
		"args", cfg.Args)

	// Convert tool spec to container config
	containerConfig := toolSpec.ToContainerConfig(e.sessionID, cfg.MountSource)
	// Keep container available until we collect logs, then destroy explicitly.
	containerConfig.RemoveOnStop = false

	// Add runtime arguments
	if len(cfg.Args) > 0 {
		// Append args to the command's args
		containerConfig.Args = append(containerConfig.Args, cfg.Args...)
	}

	// Enforce policy on configuration (includes validation)
	enforcedConfig, err := e.EnforceConfig(ctx, containerConfig)
	if err != nil {
		if typedErr, ok := err.(*types.Error); ok {
			result.Error = typedErr
		} else {
			result.Error = types.WrapError(types.ErrCodeInternal, "policy enforcement failed", err)
		}
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(startTime)
		e.logger.Warn("Policy enforcement error", "error", err)
		return result
	}

	containerConfig = enforcedConfig

	// Generate container name if not provided
	containerName := cfg.ContainerName
	if containerName == "" {
		containerName = fmt.Sprintf("tool-%s-%s", cfg.ToolType, e.sessionID.String()[:8])
	}

	// Determine timeout
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = toolSpec.Timeout
	}

	// Create context with timeout for execution
	execCtx, cancel := context.WithTimeout(taskCtx, timeout)
	defer cancel()

	// Create the container
	createCfg := container.CreateConfig{
		Config:    containerConfig,
		Name:      containerName,
		SessionID: e.sessionID,
		AutoPull:  true,
	}

	createResult, err := e.runtime.Create(execCtx, createCfg)
	if err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to create container", err)
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(startTime)
		e.logger.Warn("Failed to create container", "error", err)
		return result
	}

	result.ContainerID = createResult.ContainerID

	// Update task tracking with container info
	e.mu.Lock()
	taskInfo.ContainerID = result.ContainerID
	taskInfo.ContainerName = containerName
	e.mu.Unlock()

	e.logger.Info("Container created", "container_id", result.ContainerID, "name", containerName)

	// Ensure cleanup happens even if errors occur
	defer func() {
		destroyCfg := container.DestroyConfig{
			ContainerID: result.ContainerID,
			Name:        containerName,
			Force:       true,
		}
		if destroyErr := e.runtime.Destroy(context.Background(), destroyCfg); destroyErr != nil {
			e.logger.Warn("Failed to destroy container",
				"container_id", result.ContainerID,
				"error", destroyErr)
		} else {
			e.logger.Info("Container destroyed", "container_id", result.ContainerID)
		}
	}()

	// Start the container
	startCfg := container.StartConfig{
		ContainerID: result.ContainerID,
		Name:        containerName,
	}
	if err := e.runtime.Start(execCtx, startCfg); err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to start container", err)
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(startTime)
		e.logger.Warn("Failed to start container", "error", err)
		return result
	}

	e.logger.Info("Container started", "container_id", result.ContainerID)

	// Wait for container to complete
	exitCode, err := e.runtime.Wait(execCtx, result.ContainerID)
	if err != nil {
		// Check if it was a timeout
		if execCtx.Err() == context.DeadlineExceeded {
			result.Error = types.NewError(types.ErrCodeTimeout, "task execution timed out")
		} else {
			result.Error = types.WrapError(types.ErrCodeInternal, "container wait failed", err)
		}
		result.ExitCode = -1
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(startTime)
		e.logger.Warn("Container wait failed", "error", err)
		return result
	}

	result.ExitCode = exitCode
	e.logger.Info("Container exited", "container_id", result.ContainerID, "exit_code", exitCode)

	// Capture logs (use a fresh context with timeout since execCtx may be canceled)
	logsCtx, logsCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer logsCancel()

	logsCfg := container.LogsConfig{
		ContainerID: result.ContainerID,
		Stdout:      true,
		Stderr:      true,
		Tail:        "all",
		Timestamps:  false,
	}

	logReader, err := e.runtime.Logs(logsCtx, logsCfg)
	if err != nil {
		e.logger.Warn("Failed to retrieve logs", "error", err)
		// Continue anyway - we have the exit code
	} else {
		defer logReader.Close()

		// Read all logs
		var stdoutBuf, stderrBuf bytes.Buffer

		// Docker logs multiplex stdout and stderr together
		// We need to demultiplex them
		// Docker format: 8-byte header + payload
		// Header: [0] = stream type (1=stdout, 2=stderr), [1-3] = reserved, [4-7] = size (big-endian)
		buf := make([]byte, 8192)
		for {
			n, readErr := logReader.Read(buf)
			if n > 0 {
				data := buf[:n]
				i := 0
				for i < len(data) {
					// Need at least 8 bytes for header
					if i+8 > len(data) {
						break
					}

					// Parse header at current offset
					streamType := data[i]
					// Payload size is big-endian uint32 at bytes 4-7 of header
					payloadSize := int(data[i+4])<<24 | int(data[i+5])<<16 | int(data[i+6])<<8 | int(data[i+7])
					payloadStart := i + 8
					payloadEnd := payloadStart + payloadSize

					// Check if we have the full payload
					if payloadEnd > len(data) {
						// Partial frame - break and get more data on next read
						break
					}

					payload := data[payloadStart:payloadEnd]

					if streamType == 1 {
						stdoutBuf.Write(payload)
					} else if streamType == 2 {
						stderrBuf.Write(payload)
					}

					// Advance to next frame
					i = payloadEnd
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					e.logger.Warn("Error reading logs", "error", readErr)
				}
				break
			}
		}

		result.Stdout = stdoutBuf.String()
		result.Stderr = stderrBuf.String()
	}

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(startTime)

	e.logger.Info("Task completed",
		"container_id", result.ContainerID,
		"exit_code", result.ExitCode,
		"duration", result.Duration.String())

	return result
}

// CancelTask cancels a running task by stopping its container
func (e *Executor) CancelTask(ctx context.Context, taskID string, force bool) error {
	// Fetch task info under lock, then release before runtime operations
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return types.NewError(types.ErrCodeUnavailable, "executor is closed")
	}

	// Find the running task
	taskInfo, exists := e.runningTasks[taskID]
	if !exists {
		e.mu.Unlock()
		return types.NewError(types.ErrCodeNotFound, "task not found or already completed")
	}

	// Remove from tracking immediately to prevent concurrent access
	delete(e.runningTasks, taskID)
	e.mu.Unlock()

	e.logger.Info("Cancelling task",
		"task_id", taskID,
		"container_id", taskInfo.ContainerID,
		"force", force)

	// Cancel the context to stop ongoing operations
	if taskInfo.CancelFunc != nil {
		taskInfo.CancelFunc()
	}

	// Stop the container if it exists (outside of lock)
	if taskInfo.ContainerID != "" {
		if !force {
			// Try graceful stop first
			stopCfg := container.StopConfig{
				ContainerID: taskInfo.ContainerID,
			}

			// Use a short timeout for stopping
			timeout := 2 * time.Second
			stopCfg.Timeout = &timeout

			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			if err := e.runtime.Stop(stopCtx, stopCfg); err != nil {
				e.logger.Warn("Failed to stop container during cancellation",
					"container_id", taskInfo.ContainerID,
					"error", err)
				// Continue to try to destroy the container
			} else {
				e.logger.Info("Container stopped during cancellation",
					"container_id", taskInfo.ContainerID)
			}
		}

		// Destroy the container (force flag here determines if we force-destroy)
		destroyCfg := container.DestroyConfig{
			ContainerID: taskInfo.ContainerID,
			Name:        taskInfo.ContainerName,
			Force:       force,
		}

		if err := e.runtime.Destroy(context.Background(), destroyCfg); err != nil {
			e.logger.Warn("Failed to destroy container during cancellation",
				"container_id", taskInfo.ContainerID,
				"error", err)
		} else {
			e.logger.Info("Container destroyed during cancellation",
				"container_id", taskInfo.ContainerID)
		}
	}

	return nil
}

// GetRunningTasks returns a list of currently running task IDs
func (e *Executor) GetRunningTasks() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	taskIDs := make([]string, 0, len(e.runningTasks))
	for taskID := range e.runningTasks {
		taskIDs = append(taskIDs, taskID)
	}
	return taskIDs
}

// IsTaskRunning returns true if a task with the given ID is currently running
func (e *Executor) IsTaskRunning(taskID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	_, exists := e.runningTasks[taskID]
	return exists
}
