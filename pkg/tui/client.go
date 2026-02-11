package tui

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

const (
	// DefaultMaxRecvMsgSize is the default maximum message size for receiving (in bytes)
	DefaultMaxRecvMsgSize = 1024 * 1024 * 100 // 100 MB
	// DefaultMaxSendMsgSize is the default maximum message size for sending (in bytes)
	DefaultMaxSendMsgSize = 1024 * 1024 * 100 // 100 MB
	// DefaultDialTimeout is the default timeout for dialing a connection
	DefaultDialTimeout = 30 * time.Second
	// DefaultRPCTimeout is the default timeout for RPC calls
	DefaultRPCTimeout = 10 * time.Second
	// DefaultReconnectInterval is the default interval between reconnection attempts
	DefaultReconnectInterval = 5 * time.Second
	// DefaultReconnectMaxAttempts is the default maximum number of reconnection attempts
	DefaultReconnectMaxAttempts = 0 // 0 means infinite
)

// OrchestratorClient represents a gRPC client for the TUI to connect to the orchestrator
type OrchestratorClient struct {
	path                 string
	conn                 *grpc.ClientConn
	client               proto.OrchestratorServiceClient
	logger               *logger.Logger
	mu                   sync.RWMutex
	closed               bool
	dialTimeout          time.Duration
	rpcTimeout           time.Duration
	maxRecvMsgSize       int
	maxSendMsgSize       int
	dialOptions          []grpc.DialOption
	stats                ClientStats
	reconnectInterval    time.Duration
	reconnectMaxAttempts int
	reconnectAttempts    int
	reconnectCtx         context.Context
	reconnectCancel      context.CancelFunc
	reconnectCloseCh     chan struct{}
	reconnectWg          sync.WaitGroup
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
	DialTimeout          time.Duration
	RPCTimeout           time.Duration
	MaxRecvMsgSize       int
	MaxSendMsgSize       int
	ReconnectInterval    time.Duration
	ReconnectMaxAttempts int
	DialOptions          []grpc.DialOption
}

// NewOrchestratorClient creates a new gRPC client for connecting to the orchestrator
func NewOrchestratorClient(path string, cfg ClientConfig, log *logger.Logger) (*OrchestratorClient, error) {
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

	c := &OrchestratorClient{
		path:                 path,
		logger:               log.With("component", "tui_client", "socket_path", path),
		closed:               false,
		dialTimeout:          dialTimeout,
		rpcTimeout:           rpcTimeout,
		maxRecvMsgSize:       maxRecvSize,
		maxSendMsgSize:       maxSendSize,
		dialOptions:          cfg.DialOptions,
		reconnectInterval:    reconnectInterval,
		reconnectMaxAttempts: reconnectMaxAttempts,
		reconnectCloseCh:     make(chan struct{}),
		stats:                ClientStats{},
	}

	// Build dial options with custom UDS dialer
	c.dialOptions = c.buildDialOptions()

	c.logger.Info("TUI gRPC client initialized",
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
func (c *OrchestratorClient) buildDialOptions() []grpc.DialOption {
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
func (c *OrchestratorClient) unixDialer(ctx context.Context, _ string) (net.Conn, error) {
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
func (c *OrchestratorClient) Dial(ctx context.Context) error {
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

	c.logger.Info("Dialing orchestrator gRPC server", "path", c.path)

	// Create dial context with timeout
	dialCtx, cancel := context.WithTimeout(ctx, c.dialTimeout)
	defer cancel()

	// Dial the server
	conn, err := grpc.DialContext(dialCtx, c.path, c.dialOptions...)
	if err != nil {
		c.logger.Error("Failed to dial orchestrator gRPC server", "path", c.path, "error", err)
		return types.WrapError(types.ErrCodeUnavailable, "failed to dial orchestrator gRPC server", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.client = proto.NewOrchestratorServiceClient(conn)
	c.stats.ConnectTime = time.Now()
	c.stats.IsConnected = true
	c.stats.ReconnectAttempts = 0 // Reset reconnection attempts on successful connect
	c.mu.Unlock()

	// Start reconnection monitor with a long-lived context
	c.startReconnectMonitor(context.Background())

	c.logger.Info("Connected to orchestrator gRPC server", "path", c.path)
	return nil
}

// startReconnectMonitor starts a goroutine that monitors the connection state
// and automatically reconnects when the connection is lost
func (c *OrchestratorClient) startReconnectMonitor(ctx context.Context) {
	c.mu.Lock()
	if c.reconnectCancel != nil {
		// Already monitoring
		c.mu.Unlock()
		return
	}
	// Create a context for the reconnection goroutine
	reconnectCtx, cancel := context.WithCancel(context.Background())
	c.reconnectCtx = reconnectCtx
	c.reconnectCancel = cancel
	c.mu.Unlock()

	c.reconnectWg.Add(1)
	go c.reconnectLoop(ctx, reconnectCtx)
}

// reconnectLoop is the main reconnection monitoring goroutine
func (c *OrchestratorClient) reconnectLoop(dialCtx, reconnectCtx context.Context) {
	defer c.reconnectWg.Done()

	c.logger.Debug("Starting reconnection monitor")

	for {
		// Check if we should stop
		select {
		case <-c.reconnectCloseCh:
			c.logger.Debug("Reconnection monitor stopped via close channel")
			return
		case <-reconnectCtx.Done():
			c.logger.Debug("Reconnection monitor stopped via context cancellation")
			return
		default:
		}

		// Get connection state
		c.mu.RLock()
		conn := c.conn
		closed := c.closed
		c.mu.RUnlock()

		if closed || conn == nil {
			// No active connection to monitor, wait
			select {
			case <-c.reconnectCloseCh:
				return
			case <-reconnectCtx.Done():
				return
			case <-time.After(c.reconnectInterval):
				continue
			}
		}

		// Wait for state change with timeout
		waitCtx, cancel := context.WithTimeout(context.Background(), c.reconnectInterval)
		conn.WaitForStateChange(waitCtx, conn.GetState())
		cancel()

		// Check current state
		state := conn.GetState()
		c.logger.Debug("Connection state changed", "state", state.String())

		// Check if connection is in a failure state
		if state == connectivity.TransientFailure || state == connectivity.Shutdown {
			c.mu.Lock()
			c.stats.IsConnected = false
			c.mu.Unlock()

			c.logger.Warn("Connection lost, attempting to reconnect",
				"state", state,
				"path", c.path)

			// Attempt reconnection
			if c.shouldReconnect() {
				if err := c.reconnect(dialCtx); err != nil {
					c.logger.Error("Reconnection attempt failed",
						"attempt", c.reconnectAttempts,
						"error", err)
				}
			} else {
				c.logger.Error("Max reconnection attempts reached, giving up",
					"max_attempts", c.reconnectMaxAttempts)
				return
			}
		}
	}
}

// shouldReconnect checks if we should attempt another reconnection
func (c *OrchestratorClient) shouldReconnect() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.reconnectMaxAttempts == 0 {
		// Infinite retries
		return true
	}

	if c.reconnectAttempts >= c.reconnectMaxAttempts {
		return false
	}

	c.reconnectAttempts++
	c.stats.ReconnectAttempts = int64(c.reconnectAttempts)
	return true
}

// reconnect attempts to re-establish the connection
func (c *OrchestratorClient) reconnect(ctx context.Context) error {
	c.logger.Info("Attempting to reconnect", "attempt", c.reconnectAttempts, "path", c.path)

	// Close existing connection
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn != nil {
		if err := conn.Close(); err != nil {
			c.logger.Warn("Failed to close old connection during reconnect", "error", err)
		}
	}

	// Create dial context with timeout
	dialCtx, cancel := context.WithTimeout(ctx, c.dialTimeout)
	defer cancel()

	// Dial the server
	newConn, err := grpc.DialContext(dialCtx, c.path, c.dialOptions...)
	if err != nil {
		c.logger.Error("Failed to reconnect to orchestrator gRPC server", "path", c.path, "error", err)
		return types.WrapError(types.ErrCodeUnavailable, "failed to reconnect to orchestrator gRPC server", err)
	}

	c.mu.Lock()
	c.conn = newConn
	c.client = proto.NewOrchestratorServiceClient(newConn)
	c.stats.ConnectTime = time.Now()
	c.stats.IsConnected = true
	c.mu.Unlock()

	c.logger.Info("Successfully reconnected to orchestrator gRPC server", "path", c.path)
	return nil
}

// Close closes the client connection
func (c *OrchestratorClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "client already closed")
	}
	c.closed = true
	c.stats.IsConnected = false

	// Cancel any ongoing reconnection context
	if c.reconnectCancel != nil {
		c.reconnectCancel()
		c.reconnectCancel = nil
	}
	c.mu.Unlock()

	// Signal reconnection goroutine to stop
	close(c.reconnectCloseCh)

	// Wait for reconnection goroutine to finish
	c.reconnectWg.Wait()

	c.logger.Info("Closing TUI gRPC client", "path", c.path)

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
	c.client = nil
	c.mu.Unlock()

	c.logger.Info("TUI gRPC client closed", "path", c.path)
	return nil
}

// GetConn returns the underlying grpc.ClientConn
func (c *OrchestratorClient) GetConn() *grpc.ClientConn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// GetClient returns the orchestrator service client
func (c *OrchestratorClient) GetClient() proto.OrchestratorServiceClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

// Stats returns the current client statistics
func (c *OrchestratorClient) Stats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// IsConnected returns true if the client is connected
func (c *OrchestratorClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats.IsConnected && c.conn != nil && c.conn.GetState() == connectivity.Ready
}

// String returns a string representation of the client
func (c *OrchestratorClient) String() string {
	stats := c.Stats()
	return fmt.Sprintf("OrchestratorClient{Path: %s, Connected: %v, ConnectTime: %v}",
		c.path, stats.IsConnected, stats.ConnectTime)
}

// SocketPath returns the Unix socket path the client is connected to
func (c *OrchestratorClient) SocketPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.path
}

// HealthCheck performs a health check on the connected orchestrator server
func (c *OrchestratorClient) HealthCheck(ctx context.Context) (*proto.HealthCheckResponse, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	resp, err := client.HealthCheck(ctx, &emptypb.Empty{})
	if err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.stats.LastRPCError = err.Error()
		c.mu.Unlock()
		return nil, types.WrapError(types.ErrCodeInternal, "health check failed", err)
	}

	c.mu.Lock()
	c.stats.TotalRPCs++
	c.mu.Unlock()

	return resp, nil
}

// GetStatus returns the current status of the orchestrator
func (c *OrchestratorClient) GetStatus(ctx context.Context) (*proto.StatusResponse, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	resp, err := client.GetStatus(ctx, &emptypb.Empty{})
	if err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.stats.LastRPCError = err.Error()
		c.mu.Unlock()
		return nil, types.WrapError(types.ErrCodeInternal, "get status failed", err)
	}

	c.mu.Lock()
	c.stats.TotalRPCs++
	c.mu.Unlock()

	return resp, nil
}

// CreateSession creates a new session
func (c *OrchestratorClient) CreateSession(ctx context.Context, req *proto.CreateSessionRequest) (*proto.CreateSessionResponse, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	resp, err := client.CreateSession(ctx, req)
	if err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.stats.LastRPCError = err.Error()
		c.mu.Unlock()
		return nil, types.WrapError(types.ErrCodeInternal, "create session failed", err)
	}

	c.mu.Lock()
	c.stats.TotalRPCs++
	c.mu.Unlock()

	return resp, nil
}

// GetSession retrieves a session by its ID
func (c *OrchestratorClient) GetSession(ctx context.Context, req *proto.GetSessionRequest) (*proto.GetSessionResponse, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	resp, err := client.GetSession(ctx, req)
	if err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.stats.LastRPCError = err.Error()
		c.mu.Unlock()
		return nil, types.WrapError(types.ErrCodeInternal, "get session failed", err)
	}

	c.mu.Lock()
	c.stats.TotalRPCs++
	c.mu.Unlock()

	return resp, nil
}

// ListSessions retrieves all sessions matching the specified filter
func (c *OrchestratorClient) ListSessions(ctx context.Context, req *proto.ListSessionsRequest) (*proto.ListSessionsResponse, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	resp, err := client.ListSessions(ctx, req)
	if err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.stats.LastRPCError = err.Error()
		c.mu.Unlock()
		return nil, types.WrapError(types.ErrCodeInternal, "list sessions failed", err)
	}

	c.mu.Lock()
	c.stats.TotalRPCs++
	c.mu.Unlock()

	return resp, nil
}

// CloseSession closes a session
func (c *OrchestratorClient) CloseSession(ctx context.Context, req *proto.CloseSessionRequest) (*proto.CloseSessionResponse, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	resp, err := client.CloseSession(ctx, req)
	if err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.stats.LastRPCError = err.Error()
		c.mu.Unlock()
		return nil, types.WrapError(types.ErrCodeInternal, "close session failed", err)
	}

	c.mu.Lock()
	c.stats.TotalRPCs++
	c.mu.Unlock()

	return resp, nil
}

// SendMessage sends a message to a session
func (c *OrchestratorClient) SendMessage(ctx context.Context, req *proto.SendMessageRequest) (*proto.SendMessageResponse, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	resp, err := client.SendMessage(ctx, req)
	if err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.stats.LastRPCError = err.Error()
		c.mu.Unlock()
		return nil, types.WrapError(types.ErrCodeInternal, "send message failed", err)
	}

	c.mu.Lock()
	c.stats.TotalRPCs++
	c.mu.Unlock()

	return resp, nil
}

// StreamMessages establishes a bidirectional stream for real-time message exchange
func (c *OrchestratorClient) StreamMessages(ctx context.Context) (proto.OrchestratorService_StreamMessagesClient, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	stream, err := client.StreamMessages(ctx)
	if err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.stats.LastRPCError = err.Error()
		c.mu.Unlock()
		return nil, types.WrapError(types.ErrCodeInternal, "stream messages failed", err)
	}

	return stream, nil
}

// SendMsg sends a user message via the StreamMessages bidirectional stream
func (c *OrchestratorClient) SendMsg(ctx context.Context, sessionID, content string) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return types.NewError(types.ErrCodeUnavailable, "client not connected")
	}

	// Establish the stream
	stream, err := c.StreamMessages(ctx)
	if err != nil {
		return err
	}

	// Create the stream request with the user message
	req := &proto.StreamMessageRequest{
		SessionId: sessionID,
		Payload: &proto.StreamMessageRequest_Message{
			Message: &proto.Message{
				Role:    proto.MessageRole_MESSAGE_ROLE_USER,
				Content: content,
			},
		},
	}

	// Send the message
	if err := stream.Send(req); err != nil {
		c.mu.Lock()
		c.stats.FailedRPCs++
		c.stats.LastRPCError = err.Error()
		c.mu.Unlock()
		return types.WrapError(types.ErrCodeInternal, "send message failed", err)
	}

	c.mu.Lock()
	c.stats.TotalRPCs++
	c.mu.Unlock()

	return nil
}

// GetState returns the current connection state
func (c *OrchestratorClient) GetState() connectivity.State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil {
		return connectivity.Idle
	}
	return c.conn.GetState()
}

// WaitForStateChange blocks until the state changes or ctx expires
func (c *OrchestratorClient) WaitForStateChange(ctx context.Context, sourceState connectivity.State) bool {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return false
	}

	return conn.WaitForStateChange(ctx, sourceState)
}

// ResetConnection closes and re-establishes the connection
func (c *OrchestratorClient) ResetConnection(ctx context.Context) error {
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
	c.client = nil
	c.stats.IsConnected = false
	c.mu.Unlock()

	// Re-establish connection
	return c.Dial(ctx)
}
