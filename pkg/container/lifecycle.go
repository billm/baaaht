package container

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/docker/docker/api/types"
)

// LifecycleManager handles container lifecycle operations (start, stop, restart, destroy)
type LifecycleManager struct {
	client *Client
	logger *logger.Logger
	mu     sync.RWMutex
}

// NewLifecycleManager creates a new lifecycle manager
func NewLifecycleManager(client *Client, log *logger.Logger) (*LifecycleManager, error) {
	if client == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "Docker client cannot be nil")
	}
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &LifecycleManager{
		client: client,
		logger: log.With("component", "lifecycle_manager"),
	}, nil
}

// StartConfig holds configuration for starting a container
type StartConfig struct {
	ContainerID string
	Name        string // Optional: for better logging
}

// Start starts a container
func (l *LifecycleManager) Start(ctx context.Context, cfg StartConfig) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.validateContainerID(cfg.ContainerID); err != nil {
		return err
	}

	l.logger.Info("Starting container",
		"container_id", cfg.ContainerID,
		"name", cfg.Name)

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	if err := l.client.cli.ContainerStart(timeoutCtx, cfg.ContainerID, types.ContainerStartOptions{}); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to start container", err)
	}

	l.logger.Info("Container started successfully",
		"container_id", cfg.ContainerID,
		"name", cfg.Name)

	return nil
}

// StopConfig holds configuration for stopping a container
type StopConfig struct {
	ContainerID string
	Name        string // Optional: for better logging
	Timeout     *time.Duration
}

// Stop stops a container gracefully
func (l *LifecycleManager) Stop(ctx context.Context, cfg StopConfig) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.validateContainerID(cfg.ContainerID); err != nil {
		return err
	}

	l.logger.Info("Stopping container",
		"container_id", cfg.ContainerID,
		"name", cfg.Name,
		"timeout", cfg.Timeout)

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	// Default timeout is 10 seconds if not specified
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = *cfg.Timeout
	}

	if err := l.client.cli.ContainerStop(timeoutCtx, cfg.ContainerID, &timeout); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to stop container", err)
	}

	l.logger.Info("Container stopped successfully",
		"container_id", cfg.ContainerID,
		"name", cfg.Name)

	return nil
}

// RestartConfig holds configuration for restarting a container
type RestartConfig struct {
	ContainerID string
	Name        string // Optional: for better logging
	Timeout     *time.Duration
}

// Restart restarts a container
func (l *LifecycleManager) Restart(ctx context.Context, cfg RestartConfig) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.validateContainerID(cfg.ContainerID); err != nil {
		return err
	}

	l.logger.Info("Restarting container",
		"container_id", cfg.ContainerID,
		"name", cfg.Name,
		"timeout", cfg.Timeout)

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	// Default timeout is 10 seconds if not specified
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = *cfg.Timeout
	}

	if err := l.client.cli.ContainerRestart(timeoutCtx, cfg.ContainerID, &timeout); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to restart container", err)
	}

	l.logger.Info("Container restarted successfully",
		"container_id", cfg.ContainerID,
		"name", cfg.Name)

	return nil
}

// DestroyConfig holds configuration for destroying a container
type DestroyConfig struct {
	ContainerID string
	Name        string // Optional: for better logging
	Force       bool   // Force removal even if running
	RemoveVolumes bool // Remove associated volumes
}

// Destroy removes a container
func (l *LifecycleManager) Destroy(ctx context.Context, cfg DestroyConfig) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.validateContainerID(cfg.ContainerID); err != nil {
		return err
	}

	l.logger.Info("Destroying container",
		"container_id", cfg.ContainerID,
		"name", cfg.Name,
		"force", cfg.Force,
		"remove_volumes", cfg.RemoveVolumes)

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	options := types.ContainerRemoveOptions{
		Force:         cfg.Force,
		RemoveVolumes: cfg.RemoveVolumes,
	}

	if err := l.client.cli.ContainerRemove(timeoutCtx, cfg.ContainerID, options); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to destroy container", err)
	}

	l.logger.Info("Container destroyed successfully",
		"container_id", cfg.ContainerID,
		"name", cfg.Name)

	return nil
}

// StopAndDestroy stops a container and then destroys it
func (l *LifecycleManager) StopAndDestroy(ctx context.Context, containerID string, timeout *time.Duration) error {
	l.logger.Info("Stopping and destroying container",
		"container_id", containerID,
		"timeout", timeout)

	// First attempt to stop the container
	stopCfg := StopConfig{
		ContainerID: containerID,
		Timeout:     timeout,
	}
	if err := l.stop(ctx, stopCfg); err != nil {
		// Log the error but continue to destroy
		l.logger.Warn("Failed to stop container before destroy, will force remove",
			"container_id", containerID,
			"error", err)
	}

	// Then destroy the container
	destroyCfg := DestroyConfig{
		ContainerID:   containerID,
		Force:         true, // Force remove since stop may have failed
		RemoveVolumes: false,
	}

	if err := l.destroy(ctx, destroyCfg); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to destroy container", err)
	}

	l.logger.Info("Container stopped and destroyed successfully",
		"container_id", containerID)

	return nil
}

// Pause pauses a container
func (l *LifecycleManager) Pause(ctx context.Context, containerID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.validateContainerID(containerID); err != nil {
		return err
	}

	l.logger.Info("Pausing container", "container_id", containerID)

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	if err := l.client.cli.ContainerPause(timeoutCtx, containerID); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to pause container", err)
	}

	l.logger.Info("Container paused successfully", "container_id", containerID)
	return nil
}

// Unpause unpauses a container
func (l *LifecycleManager) Unpause(ctx context.Context, containerID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.validateContainerID(containerID); err != nil {
		return err
	}

	l.logger.Info("Unpausing container", "container_id", containerID)

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	if err := l.client.cli.ContainerUnpause(timeoutCtx, containerID); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to unpause container", err)
	}

	l.logger.Info("Container unpaused successfully", "container_id", containerID)
	return nil
}

// Status returns the current status of a container
func (l *LifecycleManager) Status(ctx context.Context, containerID string) (types.ContainerState, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if err := l.validateContainerID(containerID); err != nil {
		return "", err
	}

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	containerJSON, err := l.client.cli.ContainerInspect(timeoutCtx, containerID)
	if err != nil {
		return "", types.WrapError(types.ErrCodeInternal, "failed to inspect container", err)
	}

	return types.ContainerState(containerJSON.State.Status), nil
}

// IsRunning checks if a container is currently running
func (l *LifecycleManager) IsRunning(ctx context.Context, containerID string) (bool, error) {
	state, err := l.Status(ctx, containerID)
	if err != nil {
		return false, err
	}
	return state == types.ContainerStateRunning, nil
}

// KillConfig holds configuration for killing a container
type KillConfig struct {
	ContainerID string
	Name        string // Optional: for better logging
	Signal      string // Signal to send (default: SIGKILL)
}

// Kill forcefully terminates a container
func (l *LifecycleManager) Kill(ctx context.Context, cfg KillConfig) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.validateContainerID(cfg.ContainerID); err != nil {
		return err
	}

	signal := cfg.Signal
	if signal == "" {
		signal = "SIGKILL"
	}

	l.logger.Info("Killing container",
		"container_id", cfg.ContainerID,
		"name", cfg.Name,
		"signal", signal)

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	if err := l.client.cli.ContainerKill(timeoutCtx, cfg.ContainerID, signal); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to kill container", err)
	}

	l.logger.Info("Container killed successfully",
		"container_id", cfg.ContainerID,
		"name", cfg.Name)

	return nil
}

// Wait waits for a container to exit
func (l *LifecycleManager) Wait(ctx context.Context, containerID string) (int, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if err := l.validateContainerID(containerID); err != nil {
		return -1, err
	}

	l.logger.Info("Waiting for container to exit", "container_id", containerID)

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	statusCh, errCh := l.client.cli.ContainerWait(timeoutCtx, containerID, container.WaitConditionNotRunning)

	select {
	case status := <-statusCh:
		l.logger.Info("Container exited",
			"container_id", containerID,
			"exit_code", status.StatusCode)
		return int(status.StatusCode), nil
	case err := <-errCh:
		return -1, types.WrapError(types.ErrCodeInternal, "container wait error", err)
	case <-timeoutCtx.Done():
		return -1, types.WrapError(types.ErrCodeTimeout, "timeout waiting for container to exit", timeoutCtx.Err())
	}
}

// validateContainerID validates that a container ID is not empty
func (l *LifecycleManager) validateContainerID(containerID string) error {
	if containerID == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "container ID cannot be empty")
	}
	return nil
}

// start is the internal implementation of start (without lock)
func (l *LifecycleManager) start(ctx context.Context, cfg StartConfig) error {
	if err := l.validateContainerID(cfg.ContainerID); err != nil {
		return err
	}

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	if err := l.client.cli.ContainerStart(timeoutCtx, cfg.ContainerID, types.ContainerStartOptions{}); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to start container", err)
	}

	return nil
}

// stop is the internal implementation of stop (without lock)
func (l *LifecycleManager) stop(ctx context.Context, cfg StopConfig) error {
	if err := l.validateContainerID(cfg.ContainerID); err != nil {
		return err
	}

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = *cfg.Timeout
	}

	if err := l.client.cli.ContainerStop(timeoutCtx, cfg.ContainerID, &timeout); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to stop container", err)
	}

	return nil
}

// destroy is the internal implementation of destroy (without lock)
func (l *LifecycleManager) destroy(ctx context.Context, cfg DestroyConfig) error {
	if err := l.validateContainerID(cfg.ContainerID); err != nil {
		return err
	}

	timeoutCtx, cancel := l.client.WithTimeout(ctx)
	defer cancel()

	options := types.ContainerRemoveOptions{
		Force:         cfg.Force,
		RemoveVolumes: cfg.RemoveVolumes,
	}

	if err := l.client.cli.ContainerRemove(timeoutCtx, cfg.ContainerID, options); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to destroy container", err)
	}

	return nil
}

// String returns a string representation of the LifecycleManager
func (l *LifecycleManager) String() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return fmt.Sprintf("LifecycleManager{Client: %v}", l.client)
}
