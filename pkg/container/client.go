package container

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/docker/docker/api"
	"github.com/docker/docker/client"
)

// Client wraps the Docker client with connection management and retry logic
type Client struct {
	cli        *client.Client
	cfg        config.DockerConfig
	logger     *logger.Logger
	mu         sync.RWMutex
	httpClient *http.Client
	closed     bool
}

// New creates a new Docker client with the specified configuration
func New(cfg config.DockerConfig, log *logger.Logger) (*Client, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Build Docker client options
	opts := []client.Opt{
		client.WithHost(cfg.Host),
		client.WithAPIVersionNegotiation(),
	}

	// Only use a custom HTTP client for TCP/HTTPS connections.
	// For Unix sockets, the Docker SDK handles transport internally.
	var httpClient *http.Client
	if !strings.HasPrefix(cfg.Host, "unix://") {
		httpClient = &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: !cfg.TLSVerify,
				},
				DialContext: (&net.Dialer{
					Timeout:   cfg.Timeout,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:          10,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
		opts = append(opts, client.WithHTTPClient(httpClient))
	}

	// Configure TLS if certificates are provided
	if cfg.TLSCert != "" || cfg.TLSKey != "" || cfg.TLSCACert != "" {
		opts = append(opts, client.WithTLSClientConfig(
			cfg.TLSCACert,
			cfg.TLSCert,
			cfg.TLSKey,
		))
	}

	// Create the Docker client
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create Docker client", err)
	}

	c := &Client{
		cli:        cli,
		cfg:        cfg,
		logger:     log.With("component", "docker_client"),
		httpClient: httpClient,
		closed:     false,
	}

	// Verify the connection
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		// Close the client on connection failure
		_ = c.Close()
		return nil, types.WrapError(types.ErrCodeUnavailable, "failed to connect to Docker daemon", err)
	}

	c.logger.Info("Docker client initialized", "host", cfg.Host, "version", cli.ClientVersion())
	return c, nil
}

// NewDefault creates a new Docker client with default configuration
func NewDefault(log *logger.Logger) (*Client, error) {
	cfg := config.DefaultDockerConfig()
	return New(cfg, log)
}

// Ping verifies the connection to the Docker daemon
func (c *Client) Ping(ctx context.Context) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "Docker client is closed")
	}
	c.mu.RUnlock()

	_, err := c.cli.Ping(ctx)
	if err != nil {
		return types.WrapError(types.ErrCodeUnavailable, "Docker daemon ping failed", err)
	}
	return nil
}

// Client returns the underlying Docker client
// This method is provided for advanced use cases where direct access to the Docker client is needed
func (c *Client) Client() *client.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cli
}

// Version returns the Docker server version information
func (c *Client) Version(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return "", types.NewError(types.ErrCodeUnavailable, "Docker client is closed")
	}
	c.mu.RUnlock()

	// Use retry logic for version calls
	var lastErr error
	for i := 0; i <= c.cfg.MaxRetries; i++ {
		if i > 0 {
			c.logger.Debug("Retrying version call", "attempt", i, "max_retries", c.cfg.MaxRetries)
			time.Sleep(c.cfg.RetryDelay)
		}

		// Create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		defer cancel()

		ver, err := c.cli.ServerVersion(timeoutCtx)
		if err == nil {
			return ver.APIVersion, nil
		}
		lastErr = err
	}

	return "", types.WrapError(types.ErrCodeTimeout, "failed to get Docker version after retries", lastErr)
}

// Info returns information about the Docker server
func (c *Client) Info(ctx context.Context) (*types.DockerInfo, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, types.NewError(types.ErrCodeUnavailable, "Docker client is closed")
	}
	c.mu.RUnlock()

	// Use retry logic
	var lastErr error
	for i := 0; i <= c.cfg.MaxRetries; i++ {
		if i > 0 {
			c.logger.Debug("Retrying info call", "attempt", i, "max_retries", c.cfg.MaxRetries)
			time.Sleep(c.cfg.RetryDelay)
		}

		// Create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		defer cancel()

		info, err := c.cli.Info(timeoutCtx)
		if err == nil {
			return &types.DockerInfo{
				ServerVersion:     info.ServerVersion,
				APIVersion:        c.cli.ClientVersion(),
				OS:                info.OperatingSystem,
				KernelVersion:     info.KernelVersion,
				Architecture:      info.Architecture,
				NCPU:              info.NCPU,
				Memory:            info.MemTotal,
				ContainerCount:    info.Containers,
				RunningContainers: info.ContainersRunning,
			}, nil
		}
		lastErr = err
	}

	return nil, types.WrapError(types.ErrCodeTimeout, "failed to get Docker info after retries", lastErr)
}

// Close closes the connection to the Docker daemon
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	if err := c.cli.Close(); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to close Docker client", err)
	}

	c.logger.Info("Docker client closed")
	return nil
}

// IsClosed returns true if the client is closed
func (c *Client) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// Config returns the Docker configuration
func (c *Client) Config() config.DockerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg
}

// WithTimeout creates a new context with the client's timeout
func (c *Client) WithTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	c.mu.RLock()
	timeout := c.cfg.Timeout
	c.mu.RUnlock()
	return context.WithTimeout(parent, timeout)
}

// executeWithRetry executes a function with retry logic
func (c *Client) executeWithRetry(ctx context.Context, operation string, fn func(context.Context) error) error {
	var lastErr error

	for i := 0; i <= c.cfg.MaxRetries; i++ {
		if i > 0 {
			c.logger.Debug("Retrying operation", "operation", operation, "attempt", i, "max_retries", c.cfg.MaxRetries)
			select {
			case <-ctx.Done():
				return types.WrapError(types.ErrCodeTimeout, "operation canceled during retry delay", ctx.Err())
			case <-time.After(c.cfg.RetryDelay):
			}
		}

		// Create timeout context for each attempt
		timeoutCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		err := fn(timeoutCtx)
		cancel()

		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry context errors
		if ctx.Err() != nil {
			return types.WrapError(types.ErrCodeTimeout, fmt.Sprintf("operation %s canceled", operation), ctx.Err())
		}
	}

	return types.WrapError(types.ErrCodeTimeout, fmt.Sprintf("operation %s failed after retries", operation), lastErr)
}

// Reconnect attempts to reconnect to the Docker daemon
func (c *Client) Reconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("Attempting to reconnect to Docker daemon")

	// Close existing client
	if !c.closed {
		_ = c.cli.Close()
	}

	// Create new client
	opts := []client.Opt{
		client.WithHost(c.cfg.Host),
		client.WithAPIVersionNegotiation(),
	}

	// Only use custom HTTP client for non-Unix socket connections
	if c.httpClient != nil {
		opts = append(opts, client.WithHTTPClient(c.httpClient))
	}

	if c.cfg.TLSCert != "" || c.cfg.TLSKey != "" || c.cfg.TLSCACert != "" {
		opts = append(opts, client.WithTLSClientConfig(
			c.cfg.TLSCACert,
			c.cfg.TLSCert,
			c.cfg.TLSKey,
		))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create Docker client during reconnect", err)
	}

	c.cli = cli
	c.closed = false

	// Verify connection - call underlying client directly to avoid deadlock
	// since we already hold c.mu.Lock()
	if _, err := c.cli.Ping(ctx); err != nil {
		return types.WrapError(types.ErrCodeUnavailable, "failed to verify connection after reconnect", err)
	}

	c.logger.Info("Successfully reconnected to Docker daemon")
	return nil
}

// String returns a string representation of the client
func (c *Client) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return fmt.Sprintf("DockerClient{Host: %s, APIVersion: %s, Timeout: %s, Closed: %v}",
		c.cfg.Host, api.DefaultVersion, c.cfg.Timeout, c.closed)
}

// Global client instance
var (
	globalClient   *Client
	globalClientMu sync.RWMutex
	globalOnce     sync.Once

	// Global runtime instance (new pattern using factory)
	globalRuntime     Runtime
	globalRuntimeMu   sync.RWMutex
	globalRuntimeOnce sync.Once
)

// InitGlobal initializes the global Docker client with the specified configuration
// This function is kept for backward compatibility. It uses the runtime factory internally.
func InitGlobal(cfg config.DockerConfig, log *logger.Logger) error {
	var initErr error
	globalOnce.Do(func() {
		cli, err := New(cfg, log)
		if err != nil {
			initErr = err
			return
		}
		globalClient = cli
	})
	return initErr
}

// Global returns the global Docker client instance
// This function is kept for backward compatibility
func Global() *Client {
	globalClientMu.RLock()
	if globalClient != nil {
		client := globalClient
		globalClientMu.RUnlock()
		return client
	}
	globalClientMu.RUnlock()

	// Initialize with default settings if not already initialized
	globalClientMu.Lock()
	defer globalClientMu.Unlock()

	// Double-check after acquiring write lock
	if globalClient != nil {
		return globalClient
	}

	cli, err := NewDefault(nil)
	if err != nil {
		// Return a closed client on error
		return &Client{closed: true}
	}
	globalClient = cli
	return globalClient
}

// SetGlobal sets the global Docker client instance
func SetGlobal(c *Client) {
	globalClientMu.Lock()
	globalClient = c
	globalOnce = sync.Once{}
	globalClientMu.Unlock()
}

// InitGlobalRuntime initializes the global runtime using the runtime factory
// This is the recommended way to initialize the global container runtime
func InitGlobalRuntime(ctx context.Context, cfg RuntimeConfig) error {
	var initErr error
	globalRuntimeOnce.Do(func() {
		rt, err := NewRuntime(ctx, cfg)
		if err != nil {
			initErr = err
			return
		}
		globalRuntime = rt
	})
	return initErr
}

// InitGlobalRuntimeDefault initializes the global runtime with auto-detection
// This is a convenience function that uses default configuration
func InitGlobalRuntimeDefault(ctx context.Context) error {
	log, err := logger.NewDefault()
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
	}

	cfg := RuntimeConfig{
		Type:    string(types.RuntimeTypeAuto),
		Timeout: 30 * time.Second,
		Logger:  log,
	}

	return InitGlobalRuntime(ctx, cfg)
}

// GlobalRuntime returns the global Runtime instance
// If not yet initialized, it will create one with auto-detection
func GlobalRuntime() Runtime {
	globalRuntimeMu.RLock()
	if globalRuntime != nil {
		runtime := globalRuntime
		globalRuntimeMu.RUnlock()
		return runtime
	}
	globalRuntimeMu.RUnlock()

	// Initialize with auto-detection if not already initialized
	globalRuntimeMu.Lock()
	defer globalRuntimeMu.Unlock()

	// Double-check after acquiring write lock
	if globalRuntime != nil {
		return globalRuntime
	}

	ctx := context.Background()
	rt, err := NewRuntimeDefault(ctx)
	if err != nil {
		// Return a mock runtime that always returns errors
		return &errorRuntime{err: err}
	}
	globalRuntime = rt
	return globalRuntime
}

// SetGlobalRuntime sets the global Runtime instance
func SetGlobalRuntime(rt Runtime) {
	globalRuntimeMu.Lock()
	globalRuntime = rt
	globalRuntimeOnce = sync.Once{}
	globalRuntimeMu.Unlock()
}

// GlobalClient returns the global Docker client from the runtime
// This is a convenience function that extracts the client from the global runtime
func GlobalClient() *Client {
	rt := GlobalRuntime()
	if drt, ok := rt.(*DockerRuntime); ok {
		return drt.DockerClient()
	}
	// Fallback to legacy global client
	return Global()
}

// errorRuntime is a runtime that always returns errors
type errorRuntime struct {
	err error
}

func (r *errorRuntime) Create(ctx context.Context, cfg CreateConfig) (*CreateResult, error) {
	return nil, r.err
}

func (r *errorRuntime) Start(ctx context.Context, cfg StartConfig) error {
	return r.err
}

func (r *errorRuntime) Stop(ctx context.Context, cfg StopConfig) error {
	return r.err
}

func (r *errorRuntime) Restart(ctx context.Context, cfg RestartConfig) error {
	return r.err
}

func (r *errorRuntime) Destroy(ctx context.Context, cfg DestroyConfig) error {
	return r.err
}

func (r *errorRuntime) Pause(ctx context.Context, containerID string) error {
	return r.err
}

func (r *errorRuntime) Unpause(ctx context.Context, containerID string) error {
	return r.err
}

func (r *errorRuntime) Kill(ctx context.Context, cfg KillConfig) error {
	return r.err
}

func (r *errorRuntime) Wait(ctx context.Context, containerID string) (int, error) {
	return 0, r.err
}

func (r *errorRuntime) Status(ctx context.Context, containerID string) (types.ContainerState, error) {
	return types.ContainerStateUnknown, r.err
}

func (r *errorRuntime) IsRunning(ctx context.Context, containerID string) (bool, error) {
	return false, r.err
}

func (r *errorRuntime) HealthCheck(ctx context.Context, containerID string) (*HealthCheckResult, error) {
	return nil, r.err
}

func (r *errorRuntime) HealthCheckWithRetry(ctx context.Context, containerID string, maxAttempts int, interval time.Duration) (*HealthCheckResult, error) {
	return nil, r.err
}

func (r *errorRuntime) Stats(ctx context.Context, containerID string) (*types.ContainerStats, error) {
	return nil, r.err
}

func (r *errorRuntime) StatsStream(ctx context.Context, containerID string, interval time.Duration) (<-chan *types.ContainerStats, <-chan error) {
	errCh := make(chan error, 1)
	statsCh := make(chan *types.ContainerStats)
	close(statsCh)
	errCh <- r.err
	close(errCh)
	return statsCh, errCh
}

func (r *errorRuntime) Logs(ctx context.Context, cfg LogsConfig) (io.ReadCloser, error) {
	return nil, r.err
}

func (r *errorRuntime) LogsLines(ctx context.Context, cfg LogsConfig) ([]types.ContainerLog, error) {
	return nil, r.err
}

func (r *errorRuntime) EventsStream(ctx context.Context, containerID string) (<-chan EventsMessage, <-chan error) {
	errCh := make(chan error, 1)
	eventCh := make(chan EventsMessage)
	close(eventCh)
	errCh <- r.err
	close(errCh)
	return eventCh, errCh
}

func (r *errorRuntime) PullImage(ctx context.Context, image string, timeout time.Duration) error {
	return r.err
}

func (r *errorRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	return false, r.err
}

func (r *errorRuntime) Client() interface{} {
	return nil
}

func (r *errorRuntime) Type() string {
	return "error"
}

func (r *errorRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	return nil, r.err
}

func (r *errorRuntime) Close() error {
	return nil
}

// CheckEnvironment verifies that Docker is available in the environment
func CheckEnvironment() error {
	// Check if DOCKER_HOST is set or if the default socket exists
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		// Check for default Unix socket
		if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
			return types.NewError(types.ErrCodeUnavailable, "Docker daemon socket not found at /var/run/docker.sock")
		}
	}
	// Verify the daemon is actually responding
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return types.WrapError(types.ErrCodeUnavailable, "failed to create Docker client", err)
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = cli.Ping(ctx)
	if err != nil {
		return types.WrapError(types.ErrCodeUnavailable, "Docker daemon is not responding", err)
	}
	return nil
}

// IsDockerRunning checks if the Docker daemon is running
func IsDockerRunning(ctx context.Context) bool {
	cli := Global()
	if cli.IsClosed() {
		return false
	}
	if err := cli.Ping(ctx); err != nil {
		return false
	}
	return true
}
