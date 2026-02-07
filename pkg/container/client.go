package container

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
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

	// Create HTTP client with timeout
	httpClient := &http.Client{
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

	// Build Docker client options
	opts := []client.Opt{
		client.WithHost(cfg.Host),
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(httpClient),
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
				APIVersion:       info.APIVersion,
				OS:               info.OperatingSystem,
				KernelVersion:    info.KernelVersion,
				Architecture:     info.Architecture,
				NCPU:             info.NCPU,
				Memory:           info.MemTotal,
				ContainerCount:   info.Containers,
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
		client.WithHTTPClient(c.httpClient),
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

	// Verify connection
	if err := c.Ping(ctx); err != nil {
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
	globalClient *Client
	globalOnce   sync.Once
)

// InitGlobal initializes the global Docker client with the specified configuration
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
func Global() *Client {
	if globalClient == nil {
		// Initialize with default settings if not already initialized
		cli, err := NewDefault(nil)
		if err != nil {
			// Return a closed client on error
			return &Client{closed: true}
		}
		globalClient = cli
	}
	return globalClient
}

// SetGlobal sets the global Docker client instance
func SetGlobal(c *Client) {
	globalClient = c
	globalOnce = sync.Once{}
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
