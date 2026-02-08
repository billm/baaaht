//go:build darwin

package container

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// AppleClient represents a client for Apple Containers
// This is a stub implementation for future Apple Containers integration
type AppleClient struct {
	cfg    config.RuntimeConfig
	logger *logger.Logger
	mu     sync.RWMutex
	closed bool
}

// NewAppleClient creates a new Apple Containers client
func NewAppleClient(cfg config.RuntimeConfig, log *logger.Logger) (*AppleClient, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	client := &AppleClient{
		cfg:    cfg,
		logger: log.With("component", "apple_client"),
		closed: false,
	}

	// Verify connection (stub - always succeeds for now)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		return nil, types.WrapError(types.ErrCodeUnavailable, "failed to connect to Apple Containers", err)
	}

	client.logger.Info("Apple Containers client initialized")
	return client, nil
}

// Ping verifies the connection to Apple Containers
func (c *AppleClient) Ping(ctx context.Context) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	c.mu.RUnlock()

	// Stub implementation - Apple Containers doesn't exist yet
	// This will be implemented when Apple Containers API is available
	return nil
}

// Close closes the connection to Apple Containers
func (c *AppleClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	c.logger.Info("Apple Containers client closed")
	return nil
}

// IsClosed returns true if the client is closed
func (c *AppleClient) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// Version returns the Apple Containers version information
func (c *AppleClient) Version(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return "", types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	c.mu.RUnlock()

	// Stub implementation
	return "1.0.0", nil
}

// Info returns information about Apple Containers
func (c *AppleClient) Info(ctx context.Context) (*types.DockerInfo, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	c.mu.RUnlock()

	// Stub implementation - return basic macOS system info
	// Use actual system values where possible to avoid misleading callers
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &types.DockerInfo{
		ServerVersion:     "1.0.0",
		APIVersion:        "1.0.0",
		OS:                "darwin",
		KernelVersion:     "Darwin Kernel",
		Architecture:      runtime.GOARCH,
		NCPU:              runtime.NumCPU(),
		Memory:            int64(memStats.Sys), // Memory obtained from OS by Go runtime (not total system RAM)
		ContainerCount:    0,
		RunningContainers: 0,
	}, nil
}

// AppleCreator handles container creation operations for Apple Containers
type AppleCreator struct {
	client *AppleClient
	logger *logger.Logger
	mu     sync.RWMutex
}

// NewAppleCreator creates a new Apple Containers creator
func NewAppleCreator(client *AppleClient, log *logger.Logger) (*AppleCreator, error) {
	if client == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "Apple client cannot be nil")
	}
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &AppleCreator{
		client: client,
		logger: log.With("component", "apple_creator"),
	}, nil
}

// Create creates a new container
func (c *AppleCreator) Create(ctx context.Context, cfg CreateConfig) (*CreateResult, error) {
	c.mu.RLock()
	if c.client.IsClosed() {
		c.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	c.mu.RUnlock()

	// Stub implementation - not yet implemented
	return nil, types.NewError(types.ErrCodeInternal, "Apple Containers create not yet implemented")
}

// PullImage pulls an image from the registry
func (c *AppleCreator) PullImage(ctx context.Context, image string, timeout time.Duration) error {
	c.mu.RLock()
	if c.client.IsClosed() {
		c.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	c.mu.RUnlock()

	// Stub implementation - not yet implemented
	return types.NewError(types.ErrCodeInternal, "Apple Containers pull image not yet implemented")
}

// ImageExists checks if an image exists locally
func (c *AppleCreator) ImageExists(ctx context.Context, image string) (bool, error) {
	c.mu.RLock()
	if c.client.IsClosed() {
		c.mu.RUnlock()
		return false, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	c.mu.RUnlock()

	// Stub implementation
	return false, nil
}

// AppleLifecycleManager handles container lifecycle operations for Apple Containers
type AppleLifecycleManager struct {
	client *AppleClient
	logger *logger.Logger
	mu     sync.RWMutex
}

// NewAppleLifecycleManager creates a new Apple Containers lifecycle manager
func NewAppleLifecycleManager(client *AppleClient, log *logger.Logger) (*AppleLifecycleManager, error) {
	if client == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "Apple client cannot be nil")
	}
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &AppleLifecycleManager{
		client: client,
		logger: log.With("component", "apple_lifecycle"),
	}, nil
}

// Start starts a container
func (m *AppleLifecycleManager) Start(ctx context.Context, cfg StartConfig) error {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation - not yet implemented
	return types.NewError(types.ErrCodeInternal, "Apple Containers start not yet implemented")
}

// Stop stops a container gracefully
func (m *AppleLifecycleManager) Stop(ctx context.Context, cfg StopConfig) error {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation - not yet implemented
	return types.NewError(types.ErrCodeInternal, "Apple Containers stop not yet implemented")
}

// Restart restarts a container
func (m *AppleLifecycleManager) Restart(ctx context.Context, cfg RestartConfig) error {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation - not yet implemented
	return types.NewError(types.ErrCodeInternal, "Apple Containers restart not yet implemented")
}

// Destroy removes a container
func (m *AppleLifecycleManager) Destroy(ctx context.Context, cfg DestroyConfig) error {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation - not yet implemented
	return types.NewError(types.ErrCodeInternal, "Apple Containers destroy not yet implemented")
}

// Pause pauses a container
func (m *AppleLifecycleManager) Pause(ctx context.Context, containerID string) error {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation - not yet implemented
	return types.NewError(types.ErrCodeInternal, "Apple Containers pause not yet implemented")
}

// Unpause unpauses a container
func (m *AppleLifecycleManager) Unpause(ctx context.Context, containerID string) error {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation - not yet implemented
	return types.NewError(types.ErrCodeInternal, "Apple Containers unpause not yet implemented")
}

// Kill forcefully terminates a container
func (m *AppleLifecycleManager) Kill(ctx context.Context, cfg KillConfig) error {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation - not yet implemented
	return types.NewError(types.ErrCodeInternal, "Apple Containers kill not yet implemented")
}

// Wait waits for a container to exit
func (m *AppleLifecycleManager) Wait(ctx context.Context, containerID string) (int, error) {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return -1, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation - not yet implemented
	return -1, types.NewError(types.ErrCodeInternal, "Apple Containers wait not yet implemented")
}

// Status returns the current status of a container
func (m *AppleLifecycleManager) Status(ctx context.Context, containerID string) (types.ContainerState, error) {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return "", types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation
	return types.ContainerStateUnknown, nil
}

// IsRunning checks if a container is currently running
func (m *AppleLifecycleManager) IsRunning(ctx context.Context, containerID string) (bool, error) {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return false, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation
	return false, nil
}

// AppleMonitor handles container monitoring operations for Apple Containers
type AppleMonitor struct {
	client *AppleClient
	logger *logger.Logger
	mu     sync.RWMutex
}

// NewAppleMonitor creates a new Apple Containers monitor
func NewAppleMonitor(client *AppleClient, log *logger.Logger) (*AppleMonitor, error) {
	if client == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "Apple client cannot be nil")
	}
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &AppleMonitor{
		client: client,
		logger: log.With("component", "apple_monitor"),
	}, nil
}

// HealthCheck performs a health check on a container
func (m *AppleMonitor) HealthCheck(ctx context.Context, containerID string) (*HealthCheckResult, error) {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation
	return &HealthCheckResult{
		ContainerID:   containerID,
		Status:        types.HealthUnknown,
		FailingStreak: 0,
		LastOutput:    "Apple Containers health check not yet implemented",
		CheckedAt:     time.Now(),
	}, nil
}

// HealthCheckWithRetry performs a health check with retries
func (m *AppleMonitor) HealthCheckWithRetry(ctx context.Context, containerID string, maxAttempts int, interval time.Duration) (*HealthCheckResult, error) {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation
	return &HealthCheckResult{
		ContainerID:   containerID,
		Status:        types.HealthUnknown,
		FailingStreak: 0,
		LastOutput:    "Apple Containers health check not yet implemented",
		CheckedAt:     time.Now(),
	}, nil
}

// Stats retrieves resource usage statistics for a container
func (m *AppleMonitor) Stats(ctx context.Context, containerID string) (*types.ContainerStats, error) {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation
	return nil, types.NewError(types.ErrCodeInternal, "Apple Containers stats not yet implemented")
}

// StatsStream returns a channel that receives stats updates
func (m *AppleMonitor) StatsStream(ctx context.Context, containerID string, interval time.Duration) (<-chan *types.ContainerStats, <-chan error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.client.IsClosed() {
		errCh := make(chan error, 1)
		statsCh := make(chan *types.ContainerStats)
		close(statsCh)
		errCh <- types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
		close(errCh)
		return statsCh, errCh
	}

	// Stub implementation
	errCh := make(chan error, 1)
	statsCh := make(chan *types.ContainerStats)
	close(statsCh)
	errCh <- types.NewError(types.ErrCodeInternal, "Apple Containers stats stream not yet implemented")
	close(errCh)
	return statsCh, errCh
}

// Logs retrieves logs from a container
func (m *AppleMonitor) Logs(ctx context.Context, cfg LogsConfig) (io.ReadCloser, error) {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation
	return nil, types.NewError(types.ErrCodeInternal, "Apple Containers logs not yet implemented")
}

// LogsLines reads logs and returns them as lines
func (m *AppleMonitor) LogsLines(ctx context.Context, cfg LogsConfig) ([]types.ContainerLog, error) {
	m.mu.RLock()
	if m.client.IsClosed() {
		m.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
	}
	m.mu.RUnlock()

	// Stub implementation
	return nil, types.NewError(types.ErrCodeInternal, "Apple Containers logs lines not yet implemented")
}

// EventsStream returns a stream of container events
func (m *AppleMonitor) EventsStream(ctx context.Context, containerID string) (<-chan EventsMessage, <-chan error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.client.IsClosed() {
		errCh := make(chan error, 1)
		eventCh := make(chan EventsMessage)
		close(eventCh)
		errCh <- types.NewError(types.ErrCodeUnavailable, "Apple client is closed")
		close(errCh)
		return eventCh, errCh
	}

	// Stub implementation
	errCh := make(chan error, 1)
	eventCh := make(chan EventsMessage)
	close(eventCh)
	errCh <- types.NewError(types.ErrCodeInternal, "Apple Containers events stream not yet implemented")
	close(errCh)
	return eventCh, errCh
}

// AppleRuntime implements the Runtime interface for Apple Containers
// It embeds AppleClient, AppleCreator, AppleLifecycleManager, and AppleMonitor to provide
// a unified interface for container management on macOS
type AppleRuntime struct {
	client           *AppleClient
	creator          *AppleCreator
	lifecycleManager *AppleLifecycleManager
	monitor          *AppleMonitor
	logger           *logger.Logger
	mu               sync.RWMutex
	closed           bool
}

// NewAppleRuntime creates a new Apple Containers runtime with the specified configuration
func NewAppleRuntime(cfg config.RuntimeConfig, log *logger.Logger) (*AppleRuntime, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Create the Apple client
	client, err := NewAppleClient(cfg, log)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create Apple Containers client", err)
	}

	// Create the creator
	creator, err := NewAppleCreator(client, log)
	if err != nil {
		client.Close()
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create Apple Containers creator", err)
	}

	// Create the lifecycle manager
	lifecycleManager, err := NewAppleLifecycleManager(client, log)
	if err != nil {
		client.Close()
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create Apple Containers lifecycle manager", err)
	}

	// Create the monitor
	monitor, err := NewAppleMonitor(client, log)
	if err != nil {
		client.Close()
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create Apple Containers monitor", err)
	}

	runtime := &AppleRuntime{
		client:           client,
		creator:          creator,
		lifecycleManager: lifecycleManager,
		monitor:          monitor,
		logger:           log.With("component", "apple_runtime"),
		closed:           false,
	}

	runtime.logger.Info("Apple Containers runtime initialized")

	return runtime, nil
}

// NewAppleRuntimeDefault creates a new Apple Containers runtime with default configuration
func NewAppleRuntimeDefault(log *logger.Logger) (*AppleRuntime, error) {
	cfg := config.DefaultRuntimeConfig()
	return NewAppleRuntime(cfg, log)
}

// Create creates a new container with the specified configuration
func (r *AppleRuntime) Create(ctx context.Context, cfg CreateConfig) (*CreateResult, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.creator.Create(ctx, cfg)
}

// Start starts a container
func (r *AppleRuntime) Start(ctx context.Context, cfg StartConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Start(ctx, cfg)
}

// Stop stops a container gracefully
func (r *AppleRuntime) Stop(ctx context.Context, cfg StopConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Stop(ctx, cfg)
}

// Restart restarts a container
func (r *AppleRuntime) Restart(ctx context.Context, cfg RestartConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Restart(ctx, cfg)
}

// Destroy removes a container
func (r *AppleRuntime) Destroy(ctx context.Context, cfg DestroyConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Destroy(ctx, cfg)
}

// Pause pauses a container
func (r *AppleRuntime) Pause(ctx context.Context, containerID string) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Pause(ctx, containerID)
}

// Unpause unpauses a container
func (r *AppleRuntime) Unpause(ctx context.Context, containerID string) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Unpause(ctx, containerID)
}

// Kill forcefully terminates a container
func (r *AppleRuntime) Kill(ctx context.Context, cfg KillConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Kill(ctx, cfg)
}

// Wait waits for a container to exit
func (r *AppleRuntime) Wait(ctx context.Context, containerID string) (int, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return -1, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Wait(ctx, containerID)
}

// Status returns the current status of a container
func (r *AppleRuntime) Status(ctx context.Context, containerID string) (types.ContainerState, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return "", types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Status(ctx, containerID)
}

// IsRunning checks if a container is currently running
func (r *AppleRuntime) IsRunning(ctx context.Context, containerID string) (bool, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return false, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.IsRunning(ctx, containerID)
}

// HealthCheck performs a health check on a container
func (r *AppleRuntime) HealthCheck(ctx context.Context, containerID string) (*HealthCheckResult, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.HealthCheck(ctx, containerID)
}

// HealthCheckWithRetry performs a health check with retries
func (r *AppleRuntime) HealthCheckWithRetry(ctx context.Context, containerID string, maxAttempts int, interval time.Duration) (*HealthCheckResult, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.HealthCheckWithRetry(ctx, containerID, maxAttempts, interval)
}

// Stats retrieves resource usage statistics for a container
func (r *AppleRuntime) Stats(ctx context.Context, containerID string) (*types.ContainerStats, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.Stats(ctx, containerID)
}

// StatsStream returns a channel that receives stats updates
func (r *AppleRuntime) StatsStream(ctx context.Context, containerID string, interval time.Duration) (<-chan *types.ContainerStats, <-chan error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		errCh := make(chan error, 1)
		statsCh := make(chan *types.ContainerStats)
		close(statsCh)
		errCh <- types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
		close(errCh)
		return statsCh, errCh
	}

	return r.monitor.StatsStream(ctx, containerID, interval)
}

// Logs retrieves logs from a container
func (r *AppleRuntime) Logs(ctx context.Context, cfg LogsConfig) (io.ReadCloser, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.Logs(ctx, cfg)
}

// LogsLines reads logs and returns them as lines
func (r *AppleRuntime) LogsLines(ctx context.Context, cfg LogsConfig) ([]types.ContainerLog, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.LogsLines(ctx, cfg)
}

// EventsStream returns a stream of container events
func (r *AppleRuntime) EventsStream(ctx context.Context, containerID string) (<-chan EventsMessage, <-chan error) {
	r.mu.RLock()

	if r.closed {
		r.mu.RUnlock()
		errCh := make(chan error, 1)
		eventCh := make(chan EventsMessage)
		close(eventCh)
		errCh <- types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
		close(errCh)
		return eventCh, errCh
	}
	r.mu.RUnlock()

	return r.monitor.EventsStream(ctx, containerID)
}

// PullImage pulls an image from the registry
func (r *AppleRuntime) PullImage(ctx context.Context, image string, timeout time.Duration) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.creator.PullImage(ctx, image, timeout)
}

// ImageExists checks if an image exists locally
func (r *AppleRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return false, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	return r.creator.ImageExists(ctx, image)
}

// Client returns the underlying client for direct runtime access
func (r *AppleRuntime) Client() interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}

// Type returns the runtime type identifier
func (r *AppleRuntime) Type() string {
	return string(types.RuntimeTypeAppleContainers)
}

// Info returns runtime metadata and capabilities
func (r *AppleRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Apple runtime is closed")
	}
	r.mu.RUnlock()

	// Get Apple Containers info
	appleInfo, err := r.client.Info(ctx)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to get Apple Containers info", err)
	}

	// Get version
	version, err := r.client.Version(ctx)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to get Apple Containers version", err)
	}

	// Build capabilities list (stub - will be updated when Apple Containers API is available)
	capabilities := []string{
		"containers",
		"images",
		"healthchecks",
		"resource_limits",
		"logs",
		"events",
	}

	return &RuntimeInfo{
		Type:         string(types.RuntimeTypeAppleContainers),
		Version:      appleInfo.ServerVersion,
		Platform:     appleInfo.OS,
		APIVersion:   version,
		Capabilities: capabilities,
		Metadata: map[string]string{
			"architecture": appleInfo.Architecture,
			"kernel":       appleInfo.KernelVersion,
			"ncpu":         fmt.Sprintf("%d", appleInfo.NCPU),
			"memory":       fmt.Sprintf("%d", appleInfo.Memory),
		},
	}, nil
}

// Close cleans up runtime resources
func (r *AppleRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true

	if err := r.client.Close(); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to close Apple Containers client", err)
	}

	r.logger.Info("Apple Containers runtime closed")
	return nil
}

// IsClosed returns true if the runtime is closed
func (r *AppleRuntime) IsClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.closed
}

// String returns a string representation of the Apple runtime
func (r *AppleRuntime) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return fmt.Sprintf("AppleRuntime{Closed: %v}", r.closed)
}

// Creator returns the embedded creator for direct access
func (r *AppleRuntime) Creator() *AppleCreator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.creator
}

// LifecycleManager returns the embedded lifecycle manager for direct access
func (r *AppleRuntime) LifecycleManager() *AppleLifecycleManager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lifecycleManager
}

// Monitor returns the embedded monitor for direct access
func (r *AppleRuntime) Monitor() *AppleMonitor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.monitor
}

// AppleClient returns the underlying Apple client for direct access
func (r *AppleRuntime) AppleClient() *AppleClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}
