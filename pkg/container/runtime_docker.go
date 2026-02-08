package container

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// DockerRuntime implements the Runtime interface for Docker
// It embeds Client, Creator, LifecycleManager, and Monitor to provide
// a unified interface for container management
type DockerRuntime struct {
	client           *Client
	creator          *Creator
	lifecycleManager *LifecycleManager
	monitor          *Monitor
	logger           *logger.Logger
	mu               sync.RWMutex
	closed           bool
}

// NewDockerRuntime creates a new Docker runtime with the specified configuration
func NewDockerRuntime(cfg config.DockerConfig, log *logger.Logger) (*DockerRuntime, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Create the Docker client
	client, err := New(cfg, log)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create Docker client", err)
	}

	// Create the creator
	creator, err := NewCreator(client, log)
	if err != nil {
		client.Close()
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create container creator", err)
	}

	// Create the lifecycle manager
	lifecycleManager, err := NewLifecycleManager(client, log)
	if err != nil {
		client.Close()
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create lifecycle manager", err)
	}

	// Create the monitor
	monitor, err := NewMonitor(client, log)
	if err != nil {
		client.Close()
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create container monitor", err)
	}

	runtime := &DockerRuntime{
		client:           client,
		creator:          creator,
		lifecycleManager: lifecycleManager,
		monitor:          monitor,
		logger:           log.With("component", "docker_runtime"),
		closed:           false,
	}

	runtime.logger.Info("Docker runtime initialized", "host", cfg.Host)

	return runtime, nil
}

// NewDockerRuntimeDefault creates a new Docker runtime with default configuration
func NewDockerRuntimeDefault(log *logger.Logger) (*DockerRuntime, error) {
	cfg := config.DefaultDockerConfig()
	return NewDockerRuntime(cfg, log)
}

// Create creates a new container with the specified configuration
func (r *DockerRuntime) Create(ctx context.Context, cfg CreateConfig) (*CreateResult, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.creator.Create(ctx, cfg)
}

// Start starts a container
func (r *DockerRuntime) Start(ctx context.Context, cfg StartConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Start(ctx, cfg)
}

// Stop stops a container gracefully
func (r *DockerRuntime) Stop(ctx context.Context, cfg StopConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Stop(ctx, cfg)
}

// Restart restarts a container
func (r *DockerRuntime) Restart(ctx context.Context, cfg RestartConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Restart(ctx, cfg)
}

// Destroy removes a container
func (r *DockerRuntime) Destroy(ctx context.Context, cfg DestroyConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Destroy(ctx, cfg)
}

// Pause pauses a container
func (r *DockerRuntime) Pause(ctx context.Context, containerID string) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Pause(ctx, containerID)
}

// Unpause unpauses a container
func (r *DockerRuntime) Unpause(ctx context.Context, containerID string) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Unpause(ctx, containerID)
}

// Kill forcefully terminates a container
func (r *DockerRuntime) Kill(ctx context.Context, cfg KillConfig) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Kill(ctx, cfg)
}

// Wait waits for a container to exit
func (r *DockerRuntime) Wait(ctx context.Context, containerID string) (int, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return -1, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Wait(ctx, containerID)
}

// Status returns the current status of a container
func (r *DockerRuntime) Status(ctx context.Context, containerID string) (types.ContainerState, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return "", types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.Status(ctx, containerID)
}

// IsRunning checks if a container is currently running
func (r *DockerRuntime) IsRunning(ctx context.Context, containerID string) (bool, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return false, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.lifecycleManager.IsRunning(ctx, containerID)
}

// HealthCheck performs a health check on a container
func (r *DockerRuntime) HealthCheck(ctx context.Context, containerID string) (*HealthCheckResult, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.HealthCheck(ctx, containerID)
}

// HealthCheckWithRetry performs a health check with retries
func (r *DockerRuntime) HealthCheckWithRetry(ctx context.Context, containerID string, maxAttempts int, interval time.Duration) (*HealthCheckResult, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.HealthCheckWithRetry(ctx, containerID, maxAttempts, interval)
}

// Stats retrieves resource usage statistics for a container
func (r *DockerRuntime) Stats(ctx context.Context, containerID string) (*types.ContainerStats, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.Stats(ctx, containerID)
}

// StatsStream returns a channel that receives stats updates
func (r *DockerRuntime) StatsStream(ctx context.Context, containerID string, interval time.Duration) (<-chan *types.ContainerStats, <-chan error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		errCh := make(chan error, 1)
		statsCh := make(chan *types.ContainerStats)
		close(statsCh)
		errCh <- types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
		close(errCh)
		return statsCh, errCh
	}

	return r.monitor.StatsStream(ctx, containerID, interval)
}

// Logs retrieves logs from a container
func (r *DockerRuntime) Logs(ctx context.Context, cfg LogsConfig) (io.ReadCloser, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.Logs(ctx, cfg)
}

// LogsLines reads logs and returns them as lines
func (r *DockerRuntime) LogsLines(ctx context.Context, cfg LogsConfig) ([]types.ContainerLog, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.monitor.LogsLines(ctx, cfg)
}

// EventsStream returns a stream of container events
func (r *DockerRuntime) EventsStream(ctx context.Context, containerID string) (<-chan EventsMessage, <-chan error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		errCh := make(chan error, 1)
		eventCh := make(chan EventsMessage)
		close(eventCh)
		errCh <- types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
		close(errCh)
		return eventCh, errCh
	}
	r.mu.RUnlock()

	// Get the raw event channels from the monitor
	rawEventCh, rawErrCh := r.monitor.EventsStream(ctx, containerID)

	// Wrap the event channel to convert to EventsMessage (interface{})
	eventCh := make(chan EventsMessage, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		for {
			select {
			case event, ok := <-rawEventCh:
				if !ok {
					return
				}
				select {
				case eventCh <- event:
				case <-ctx.Done():
					return
				}
			case err, ok := <-rawErrCh:
				if !ok {
					return
				}
				select {
				case errCh <- err:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventCh, errCh
}

// PullImage pulls an image from the registry
func (r *DockerRuntime) PullImage(ctx context.Context, image string, timeout time.Duration) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.creator.PullImage(ctx, image, timeout)
}

// ImageExists checks if an image exists locally
func (r *DockerRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return false, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	return r.creator.ImageExists(ctx, image)
}

// Client returns the underlying client for direct runtime access
func (r *DockerRuntime) Client() interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}

// Type returns the runtime type identifier
func (r *DockerRuntime) Type() string {
	return string(types.RuntimeTypeDocker)
}

// Info returns runtime metadata and capabilities
func (r *DockerRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Docker runtime is closed")
	}
	r.mu.RUnlock()

	// Get Docker info
	dockerInfo, err := r.client.Info(ctx)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to get Docker info", err)
	}

	// Get version
	version, err := r.client.Version(ctx)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to get Docker version", err)
	}

	// Build capabilities list
	capabilities := []string{
		"containers",
		"images",
		"networks",
		"volumes",
		"healthchecks",
		"resource_limits",
		"logs",
		"exec",
		"events",
	}

	return &RuntimeInfo{
		Type:         string(types.RuntimeTypeDocker),
		Version:      dockerInfo.ServerVersion,
		Platform:     dockerInfo.OS,
		APIVersion:   version,
		Capabilities: capabilities,
		Metadata: map[string]string{
			"architecture": dockerInfo.Architecture,
			"kernel":       dockerInfo.KernelVersion,
			"ncpu":         fmt.Sprintf("%d", dockerInfo.NCPU),
			"memory":       fmt.Sprintf("%d", dockerInfo.Memory),
		},
	}, nil
}

// Close cleans up runtime resources
func (r *DockerRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true

	if err := r.client.Close(); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to close Docker client", err)
	}

	r.logger.Info("Docker runtime closed")
	return nil
}

// IsClosed returns true if the runtime is closed
func (r *DockerRuntime) IsClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.closed
}

// String returns a string representation of the Docker runtime
func (r *DockerRuntime) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return fmt.Sprintf("DockerRuntime{Closed: %v, Client: %v}", r.closed, r.client)
}

// Creator returns the embedded creator for direct access
func (r *DockerRuntime) Creator() *Creator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.creator
}

// LifecycleManager returns the embedded lifecycle manager for direct access
func (r *DockerRuntime) LifecycleManager() *LifecycleManager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lifecycleManager
}

// Monitor returns the embedded monitor for direct access
func (r *DockerRuntime) Monitor() *Monitor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.monitor
}

// DockerClient returns the underlying Docker client for direct access
func (r *DockerRuntime) DockerClient() *Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}
