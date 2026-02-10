package tools

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// ExecutionConfig holds configuration for executing a tool
type ExecutionConfig struct {
	ToolName    string            // Name of the tool to execute
	Parameters  map[string]string // Tool parameters
	SessionID   types.ID          // Session ID for tracking
	Timeout     time.Duration     // Execution timeout
	AutoCleanup bool              // Automatically clean up container after execution
}

// Executor handles tool execution by spawning ephemeral containers
type Executor struct {
	runtime  container.Runtime
	registry *Registry
	logger   *logger.Logger
	mu       sync.RWMutex
}

// NewExecutor creates a new tool executor
func NewExecutor(runtime container.Runtime, registry *Registry, log *logger.Logger) (*Executor, error) {
	if runtime == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime cannot be nil")
	}
	if registry == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "registry cannot be nil")
	}
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &Executor{
		runtime:  runtime,
		registry: registry,
		logger:   log.With("component", "tool_executor"),
	}, nil
}

// NewExecutorFromRegistry creates a new executor using the global registry
func NewExecutorFromRegistry(runtime container.Runtime, log *logger.Logger) (*Executor, error) {
	if runtime == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime cannot be nil")
	}
	return NewExecutor(runtime, GetGlobalRegistry(), log)
}

// Execute executes a tool with the given configuration
func (e *Executor) Execute(ctx context.Context, cfg ExecutionConfig) (*ToolExecutionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.validateConfig(cfg); err != nil {
		return nil, err
	}

	startTime := time.Now()
	executionID := types.GenerateID().String()

	e.logger.Info("Executing tool",
		"execution_id", executionID,
		"tool_name", cfg.ToolName,
		"session_id", cfg.SessionID,
		"timeout", cfg.Timeout)

	// Get tool from registry
	tool, err := e.registry.GetTool(cfg.ToolName)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to get tool from registry", err)
	}

	// Validate tool parameters
	if err := tool.Validate(cfg.Parameters); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalidArgument, "tool parameter validation failed", err)
	}

	// Get tool definition for container configuration
	definition := tool.Definition()

	// Determine timeout
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = definition.Timeout
		if timeout == 0 {
			timeout = 5 * time.Minute // Default timeout
		}
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create ephemeral container for tool execution
	containerID, err := e.createToolContainer(execCtx, cfg.ToolName, cfg.Parameters, cfg.SessionID, executionID, definition)
	if err != nil {
		return e.createErrorResult(executionID, cfg.ToolName, err, startTime, timeout)
	}

	// Ensure cleanup happens
	if cfg.AutoCleanup {
		defer e.cleanupContainer(context.Background(), containerID, cfg.ToolName)
	}

	// Start the container
	if err := e.runtime.Start(execCtx, container.StartConfig{
		ContainerID: containerID,
		Name:        e.generateContainerName(cfg.ToolName, executionID),
	}); err != nil {
		return e.createErrorResult(executionID, cfg.ToolName, err, startTime, timeout)
	}

	// Wait for container to complete
	exitCode, err := e.runtime.Wait(execCtx, containerID)
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return e.createTimeoutResult(executionID, cfg.ToolName, startTime, timeout, containerID)
		}
		return e.createErrorResult(executionID, cfg.ToolName, err, startTime, timeout)
	}

	// Collect output from container
	outputData, errorData, err := e.collectContainerOutput(ctx, containerID)
	if err != nil {
		e.logger.Warn("Failed to collect container output", "error", err, "container_id", containerID)
		// Continue with empty output on error
	}

	duration := time.Since(startTime)

	result := &ToolExecutionResult{
		ExecutionID: executionID,
		ToolName:    cfg.ToolName,
		Status:      e.determineStatus(int32(exitCode), err),
		ExitCode:    int32(exitCode),
		OutputData:  outputData,
		OutputText:  string(outputData),
		ErrorData:   errorData,
		ErrorText:   string(errorData),
		Metadata: map[string]string{
			"session_id":    cfg.SessionID.String(),
			"container_id":  containerID,
			"timeout":       timeout.String(),
		},
		Duration:    duration,
		CompletedAt: time.Now(),
		ContainerID: containerID,
	}

	e.logger.Info("Tool execution completed",
		"execution_id", executionID,
		"tool_name", cfg.ToolName,
		"exit_code", exitCode,
		"duration", duration,
		"container_id", containerID)

	return result, nil
}

// createToolContainer creates an ephemeral container for tool execution
func (e *Executor) createToolContainer(ctx context.Context, toolName string, parameters map[string]string, sessionID types.ID, executionID string, definition ToolDefinition) (string, error) {
	e.logger.Debug("Creating tool container",
		"tool_name", toolName,
		"execution_id", executionID,
		"image", definition.ContainerImage)

	// Build container configuration
	containerName := e.generateContainerName(toolName, executionID)

	// Build environment variables from parameters
	env := make(map[string]string)
	for k, v := range parameters {
		// Convert parameter names to env var format (TOOL_PARAM_NAME)
		envKey := fmt.Sprintf("TOOL_%s", strings.ToUpper(k))
		env[envKey] = v
	}
	env["TOOL_NAME"] = toolName
	env["EXECUTION_ID"] = executionID
	env["SESSION_ID"] = sessionID.String()

	// Build mounts from security policy
	mounts := e.buildMounts(toolName, parameters, definition.SecurityPolicy)

	// Create container config
	config := types.ContainerConfig{
		Image:          definition.ContainerImage,
		Command:        definition.Command,
		Env:            env,
		Labels:         e.buildContainerLabels(toolName, sessionID, executionID),
		Mounts:         mounts,
		Resources:      definition.ResourceLimits,
		ReadOnlyRootfs: definition.SecurityPolicy.ReadOnlyFilesystem,
		RemoveOnStop:   true, // Auto-remove on stop for ephemerality
	}

	// Apply network mode based on security policy
	if !definition.SecurityPolicy.AllowNetwork {
		config.NetworkMode = "none"
	}

	// Create the container
	result, err := e.runtime.Create(ctx, container.CreateConfig{
		Config:    config,
		Name:      containerName,
		SessionID: sessionID,
		AutoPull:  true,
	})

	if err != nil {
		return "", types.WrapError(types.ErrCodeInternal, "failed to create tool container", err)
	}

	e.logger.Debug("Tool container created",
		"tool_name", toolName,
		"container_id", result.ContainerID,
		"container_name", containerName)

	return result.ContainerID, nil
}

// buildMounts builds mount configuration based on security policy
func (e *Executor) buildMounts(toolName string, parameters map[string]string, policy ToolSecurityPolicy) []types.Mount {
	if !policy.AllowFilesystem {
		return nil
	}

	var mounts []types.Mount

	// Add allowed paths as mounts
	for i, path := range policy.AllowedPaths {
		// Determine if this should be read-only
		readOnly := policy.ReadOnlyFilesystem

		mounts = append(mounts, types.Mount{
			Type:     types.MountTypeBind,
			Source:   path,
			Target:   fmt.Sprintf("/mnt/scope%d", i),
			ReadOnly: readOnly,
		})
	}

	return mounts
}

// buildContainerLabels builds labels for the tool container
func (e *Executor) buildContainerLabels(toolName string, sessionID types.ID, executionID string) map[string]string {
	return map[string]string{
		"baaaht.managed":      "true",
		"baaaht.tool_name":    toolName,
		"baaaht.session_id":   sessionID.String(),
		"baaaht.execution_id": executionID,
		"baaaht.tool_type":    "ephemeral",
	}
}

// generateContainerName generates a unique container name for tool execution
func (e *Executor) generateContainerName(toolName string, executionID string) string {
	// Use first 8 chars of execution ID for brevity
	shortID := executionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	return fmt.Sprintf("tool-%s-%s", toolName, shortID)
}

// collectContainerOutput collects stdout and stderr from the container
func (e *Executor) collectContainerOutput(ctx context.Context, containerID string) (output, stderr []byte, err error) {
	// Get stdout logs
	stdoutReader, err := e.runtime.Logs(ctx, container.LogsConfig{
		ContainerID: containerID,
		Stdout:      true,
		Stderr:      false,
		Tail:        "all",
	})
	if err == nil && stdoutReader != nil {
		output, err = io.ReadAll(stdoutReader)
		stdoutReader.Close()
		if err != nil {
			e.logger.Warn("Failed to read stdout", "error", err, "container_id", containerID)
		}
	}

	// Get stderr logs
	stderrReader, err := e.runtime.Logs(ctx, container.LogsConfig{
		ContainerID: containerID,
		Stdout:      false,
		Stderr:      true,
		Tail:        "all",
	})
	if err == nil && stderrReader != nil {
		stderr, err = io.ReadAll(stderrReader)
		stderrReader.Close()
		if err != nil {
			e.logger.Warn("Failed to read stderr", "error", err, "container_id", containerID)
		}
	}

	return output, stderr, nil
}

// cleanupContainer cleans up a tool container
func (e *Executor) cleanupContainer(ctx context.Context, containerID string, toolName string) {
	e.logger.Debug("Cleaning up tool container",
		"tool_name", toolName,
		"container_id", containerID)

	// Force destroy the container
	err := e.runtime.Destroy(ctx, container.DestroyConfig{
		ContainerID:   containerID,
		Name:          toolName,
		Force:         true,
		RemoveVolumes: false,
	})

	if err != nil {
		e.logger.Warn("Failed to destroy tool container",
			"tool_name", toolName,
			"container_id", containerID,
			"error", err)
	}
}

// createErrorResult creates an error result
func (e *Executor) createErrorResult(executionID, toolName string, err error, startTime time.Time, timeout time.Duration) (*ToolExecutionResult, error) {
	return &ToolExecutionResult{
		ExecutionID: executionID,
		ToolName:    toolName,
		Status:      ToolExecutionStatusFailed,
		ExitCode:    -1,
		ErrorText:   err.Error(),
		Duration:    time.Since(startTime),
		CompletedAt: time.Now(),
	}, nil
}

// createTimeoutResult creates a timeout result
func (e *Executor) createTimeoutResult(executionID, toolName string, startTime time.Time, timeout time.Duration, containerID string) (*ToolExecutionResult, error) {
	// Kill the timed-out container
	e.cleanupContainer(context.Background(), containerID, toolName)

	return &ToolExecutionResult{
		ExecutionID: executionID,
		ToolName:    toolName,
		Status:      ToolExecutionStatusTimeout,
		ExitCode:    -1,
		ErrorText:   fmt.Sprintf("execution timeout after %v", timeout),
		Metadata: map[string]string{
			"timeout":      timeout.String(),
			"container_id": containerID,
		},
		Duration:    time.Since(startTime),
		CompletedAt: time.Now(),
		ContainerID: containerID,
	}, nil
}

// determineStatus determines the execution status from exit code and error
func (e *Executor) determineStatus(exitCode int32, err error) ToolExecutionStatus {
	if err != nil {
		return ToolExecutionStatusFailed
	}
	if exitCode == 0 {
		return ToolExecutionStatusCompleted
	}
	return ToolExecutionStatusFailed
}

// validateConfig validates the execution configuration
func (e *Executor) validateConfig(cfg ExecutionConfig) error {
	if cfg.ToolName == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "tool name is required")
	}
	if cfg.SessionID.IsEmpty() {
		return types.NewError(types.ErrCodeInvalidArgument, "session ID is required")
	}
	if cfg.Parameters == nil {
		cfg.Parameters = make(map[string]string)
	}
	return nil
}

// SetRegistry sets a new registry for the executor
func (e *Executor) SetRegistry(registry *Registry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry = registry
}

// Registry returns the current registry
func (e *Executor) Registry() *Registry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.registry
}

// Runtime returns the current runtime
func (e *Executor) Runtime() container.Runtime {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.runtime
}

// String returns a string representation of the executor
func (e *Executor) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return fmt.Sprintf("Executor{Runtime: %s, Registry: %v}", e.runtime.Type(), e.registry)
}
