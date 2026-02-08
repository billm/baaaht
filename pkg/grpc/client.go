package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

const (
	// DefaultDialTimeout is the default timeout for dialing a connection
	DefaultDialTimeout = 30 * time.Second
	// DefaultRPCTimeout is the default timeout for RPC calls
	DefaultRPCTimeout = 10 * time.Second
	// DefaultReconnectInterval is the default interval between reconnection attempts
	DefaultReconnectInterval = 5 * time.Second
	// DefaultReconnectMaxAttempts is the default maximum number of reconnection attempts
	DefaultReconnectMaxAttempts = 0 // 0 means infinite
)

// Client represents a gRPC client with Unix Domain Socket transport
type Client struct {
	path               string
	conn               *grpc.ClientConn
	logger             *logger.Logger
	mu                 sync.RWMutex
	closed             bool
	dialTimeout        time.Duration
	rpcTimeout         time.Duration
	maxRecvMsgSize     int
	maxSendMsgSize     int
	dialOptions        []grpc.DialOption
	stats              ClientStats
	reconnectInterval  time.Duration
	reconnectMaxAttempts int
	reconnectAttempts  int
	reconnectCancel    context.CancelFunc
	reconnectWg        sync.WaitGroup
}

// ClientStats represents client statistics
type ClientStats struct {
	ConnectTime       time.Time `json:"connect_time"`
	IsConnected       bool      `json:"is_connected"`
	TotalRPCs         int64     `json:"total_rpcs"`
	FailedRPCs        int64     `json:"failed_rpcs"`
	ReconnectAttempts int64     `json:"reconnect_attempts"`
	LastRPCError      string    `json:"last_rpc_error,omitempty"`
}

// ClientConfig contains client configuration
type ClientConfig struct {
	DialTimeout        time.Duration
	RPCTimeout         time.Duration
	MaxRecvMsgSize     int
	MaxSendMsgSize     int
	ReconnectInterval  time.Duration
	ReconnectMaxAttempts int
	DialOptions        []grpc.DialOption
}

// NewClient creates a new gRPC client with UDS transport
func NewClient(path string, cfg ClientConfig, log *logger.Logger) (*Client, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Validate and set default max message sizes
	maxRecvSize := cfg.MaxRecvMsgSize
	if maxRecvSize <= 0 {
		maxRecvSize = DefaultMaxRecvMsgSize
	}
	maxSendSize := cfg.MaxSendMsgSize
	if maxSendSize <= 0 {
		maxSendSize = DefaultMaxSendMsgSize
	}

	// Validate and set default timeouts
	dialTimeout := cfg.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = DefaultDialTimeout
	}
	rpcTimeout := cfg.RPCTimeout
	if rpcTimeout <= 0 {
		rpcTimeout = DefaultRPCTimeout
	}

	// Set default reconnection settings
	reconnectInterval := cfg.ReconnectInterval
	if reconnectInterval <= 0 {
		reconnectInterval = DefaultReconnectInterval
	}
	reconnectMaxAttempts := cfg.ReconnectMaxAttempts
	if reconnectMaxAttempts < 0 {
		reconnectMaxAttempts = DefaultReconnectMaxAttempts
	}

	c := &Client{
		path:                path,
		logger:              log.With("component", "grpc_client", "socket_path", path),
		closed:              false,
		dialTimeout:         dialTimeout,
		rpcTimeout:          rpcTimeout,
		maxRecvMsgSize:      maxRecvSize,
		maxSendMsgSize:      maxSendSize,
		dialOptions:         cfg.DialOptions,
		reconnectInterval:   reconnectInterval,
		reconnectMaxAttempts: reconnectMaxAttempts,
		stats:               ClientStats{},
	}

	// Build dial options with custom UDS dialer
	c.dialOptions = c.buildDialOptions()

	c.logger.Info("gRPC client initialized",
		"path", path,
		"dial_timeout", c.dialTimeout.String(),
		"rpc_timeout", c.rpcTimeout.String(),
		"max_recv_msg_size", c.maxRecvMsgSize,
		"max_send_msg_size", c.maxSendMsgSize,
		"reconnect_interval", c.reconnectInterval.String(),
		"reconnect_max_attempts", c.reconnectMaxAttempts)

	return c, nil
}

// buildDialOptions constructs the gRPC dial options
func (c *Client) buildDialOptions() []grpc.DialOption {
	opts := []grpc.DialOption{
		grpc.WithContextDialer(c.unixDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(c.maxRecvMsgSize),
			grpc.MaxCallSendMsgSize(c.maxSendMsgSize),
		),
		grpc.WithBlock(),
	}

	// Add custom dial options
	opts = append(opts, c.dialOptions...)

	return opts
}

// unixDialer is a custom dialer for Unix Domain Sockets
func (c *Client) unixDialer(ctx context.Context, _ string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.path)
	if err != nil {
		c.logger.Debug("UDS dial failed", "path", c.path, "error", err)
		return nil, err
	}
	c.logger.Debug("UDS dial successful", "path", c.path)
	return conn, nil
}

// Dial establishes a connection to the gRPC server
func (c *Client) Dial(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return types.NewError(types.ErrCodeUnavailable, "client is closed")
	}
	if c.conn != nil && c.conn.GetState() == connectivity.Ready {
		c.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "already connected")
	}
	c.mu.Unlock()

	c.logger.Info("Dialing gRPC server", "path", c.path)

	// Create dial context with timeout
	dialCtx, cancel := context.WithTimeout(ctx, c.dialTimeout)
	defer cancel()

	// Dial the server
	conn, err := grpc.DialContext(dialCtx, c.path, c.dialOptions...)
	if err != nil {
		c.logger.Error("Failed to dial gRPC server", "path", c.path, "error", err)
		return types.WrapError(types.ErrCodeUnavailable, "failed to dial gRPC server", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.stats.ConnectTime = time.Now()
	c.stats.IsConnected = true
	c.stats.ReconnectAttempts = 0 // Reset reconnection attempts on successful connect
	c.mu.Unlock()

	c.logger.Info("Connected to gRPC server", "path", c.path)
	return nil
}

// Close closes the client connection
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "client already closed")
	}
	c.closed = true
	c.stats.IsConnected = false

	// Cancel any ongoing reconnection
	if c.reconnectCancel != nil {
		c.reconnectCancel()
		c.reconnectCancel = nil
	}
	c.mu.Unlock()

	// Wait for reconnection goroutine to finish
	c.reconnectWg.Wait()

	c.logger.Info("Closing gRPC client", "path", c.path)

	// Close the connection
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn != nil {
		if err := conn.Close(); err != nil {
			c.logger.Error("Failed to close connection", "error", err)
			return types.WrapError(types.ErrCodeInternal, "failed to close connection", err)
		}
	}

	c.mu.Lock()
	c.conn = nil
	c.mu.Unlock()

	c.logger.Info("gRPC client closed", "path", c.path)
	return nil
}

// GetConn returns the underlying grpc.ClientConn
func (c *Client) GetConn() *grpc.ClientConn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// Stats returns the current client statistics
func (c *Client) Stats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// IsConnected returns true if the client is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats.IsConnected && c.conn != nil && c.conn.GetState() == connectivity.Ready
}

// String returns a string representation of the client
func (c *Client) String() string {
	stats := c.Stats()
	return fmt.Sprintf("Client{Path: %s, Connected: %v, ConnectTime: %v}",
		c.path, stats.IsConnected, stats.ConnectTime)
}

// SocketPath returns the Unix socket path the client is connected to
func (c *Client) SocketPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.path
}

// HealthCheck performs a health check on the connected server
func (c *Client) HealthCheck(ctx context.Context) (*grpc_health_v1.HealthCheckResponse, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	healthClient := grpc_health_v1.NewHealthClient(conn)
	resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.mu.Unlock()
		return nil, types.WrapError(types.ErrCodeInternal, "health check failed", err)
	}

	c.mu.Lock()
	c.stats.TotalRPCs++
	c.mu.Unlock()

	return resp, nil
}

// NewClientConn creates a new gRPC client connection using the default UDS dialer
// This is a convenience function for simple use cases
func NewClientConn(path string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// Create UDS dialer
	unixDialer := func(ctx context.Context, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", path)
	}

	// Default dial options
	defaultOpts := []grpc.DialOption{
		grpc.WithContextDialer(unixDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	}

	// Append custom options
	defaultOpts = append(defaultOpts, opts...)

	// Dial with timeout
	ctx, cancel := context.WithTimeout(context.Background(), DefaultDialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, path, defaultOpts...)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeUnavailable, "failed to create gRPC connection", err)
	}

	return conn, nil
}

// GetState returns the current connection state
func (c *Client) GetState() connectivity.State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil {
		return connectivity.Idle
	}
	return c.conn.GetState()
}

// WaitForStateChange blocks until the state changes or ctx expires
func (c *Client) WaitForStateChange(ctx context.Context, sourceState connectivity.State) bool {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return false
	}

	return conn.WaitForStateChange(ctx, sourceState)
}

// ResetConnection closes and re-establishes the connection
func (c *Client) ResetConnection(ctx context.Context) error {
	c.logger.Info("Resetting connection", "path", c.path)

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	// Close existing connection
	if conn != nil {
		if err := conn.Close(); err != nil {
			c.logger.Warn("Failed to close existing connection", "error", err)
		}
	}

	c.mu.Lock()
	c.conn = nil
	c.stats.IsConnected = false
	c.mu.Unlock()

	// Re-establish connection
	return c.Dial(ctx)
}
