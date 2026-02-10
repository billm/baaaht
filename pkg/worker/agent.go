package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

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
	// DefaultMaxRecvMsgSize is the default maximum received message size
	DefaultMaxRecvMsgSize = 1024 * 1024 * 100 // 100MB
	// DefaultMaxSendMsgSize is the default maximum sent message size
	DefaultMaxSendMsgSize = 1024 * 1024 * 100 // 100MB
)

// Agent represents a Worker agent with gRPC client connection to orchestrator
type Agent struct {
	orchestratorAddr      string
	conn                  *grpc.ClientConn
	logger                *logger.Logger
	mu                    sync.RWMutex
	closed                bool
	dialTimeout           time.Duration
	rpcTimeout            time.Duration
	maxRecvMsgSize        int
	maxSendMsgSize        int
	stats                 AgentStats
	reconnectInterval     time.Duration
	reconnectMaxAttempts  int
	reconnectAttempts     int
	reconnectCtx          context.Context
	reconnectCancel       context.CancelFunc
	reconnectCloseCh      chan struct{}
	reconnectWg           sync.WaitGroup
}

// AgentStats represents agent statistics
type AgentStats struct {
	ConnectTime       time.Time `json:"connect_time"`
	IsConnected       bool      `json:"is_connected"`
	TotalRPCs         int64     `json:"total_rpcs"`
	FailedRPCs        int64     `json:"failed_rpcs"`
	ReconnectAttempts int64     `json:"reconnect_attempts"`
	LastRPCError      string    `json:"last_rpc_error,omitempty"`
}

// AgentConfig contains agent configuration
type AgentConfig struct {
	DialTimeout        time.Duration
	RPCTimeout         time.Duration
	MaxRecvMsgSize     int
	MaxSendMsgSize     int
	ReconnectInterval  time.Duration
	ReconnectMaxAttempts int
	DialOptions        []grpc.DialOption
}

// NewAgent creates a new Worker agent with gRPC connection to orchestrator
func NewAgent(orchestratorAddr string, cfg AgentConfig, log *logger.Logger) (*Agent, error) {
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

	a := &Agent{
		orchestratorAddr:     orchestratorAddr,
		logger:               log.With("component", "worker_agent", "orchestrator_addr", orchestratorAddr),
		closed:               false,
		dialTimeout:          dialTimeout,
		rpcTimeout:           rpcTimeout,
		maxRecvMsgSize:       maxRecvSize,
		maxSendMsgSize:       maxSendSize,
		reconnectInterval:    reconnectInterval,
		reconnectMaxAttempts: reconnectMaxAttempts,
		reconnectCloseCh:     make(chan struct{}),
		stats:                AgentStats{},
	}

	a.logger.Info("Worker agent initialized",
		"orchestrator_addr", orchestratorAddr,
		"dial_timeout", a.dialTimeout.String(),
		"rpc_timeout", a.rpcTimeout.String(),
		"max_recv_msg_size", a.maxRecvMsgSize,
		"max_send_msg_size", a.maxSendMsgSize,
		"reconnect_interval", a.reconnectInterval.String(),
		"reconnect_max_attempts", a.reconnectMaxAttempts)

	return a, nil
}

// buildDialOptions constructs the gRPC dial options
func (a *Agent) buildDialOptions() []grpc.DialOption {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(a.maxRecvMsgSize),
			grpc.MaxCallSendMsgSize(a.maxSendMsgSize),
		),
		grpc.WithBlock(),
	}

	return opts
}

// Dial establishes a connection to the orchestrator
func (a *Agent) Dial(ctx context.Context) error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return types.NewError(types.ErrCodeUnavailable, "agent is closed")
	}
	if a.conn != nil && a.conn.GetState() == connectivity.Ready {
		a.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "already connected")
	}
	a.mu.Unlock()

	a.logger.Info("Dialing orchestrator", "addr", a.orchestratorAddr)

	// Create dial context with timeout
	dialCtx, cancel := context.WithTimeout(ctx, a.dialTimeout)
	defer cancel()

	// Dial the server
	conn, err := grpc.DialContext(dialCtx, a.orchestratorAddr, a.buildDialOptions()...)
	if err != nil {
		a.logger.Error("Failed to dial orchestrator", "addr", a.orchestratorAddr, "error", err)
		return types.WrapError(types.ErrCodeUnavailable, "failed to dial orchestrator", err)
	}

	a.mu.Lock()
	a.conn = conn
	a.stats.ConnectTime = time.Now()
	a.stats.IsConnected = true
	a.stats.ReconnectAttempts = 0 // Reset reconnection attempts on successful connect
	a.mu.Unlock()

	// Start reconnection monitor
	a.startReconnectMonitor(ctx)

	a.logger.Info("Connected to orchestrator", "addr", a.orchestratorAddr)
	return nil
}

// startReconnectMonitor starts a goroutine that monitors the connection state
// and automatically reconnects when the connection is lost
func (a *Agent) startReconnectMonitor(ctx context.Context) {
	a.mu.Lock()
	if a.reconnectCancel != nil {
		// Already monitoring
		a.mu.Unlock()
		return
	}
	// Create a context for the reconnection goroutine
	reconnectCtx, cancel := context.WithCancel(context.Background())
	a.reconnectCtx = reconnectCtx
	a.reconnectCancel = cancel
	a.mu.Unlock()

	a.reconnectWg.Add(1)
	go a.reconnectLoop(ctx, reconnectCtx)
}

// reconnectLoop is the main reconnection monitoring goroutine
func (a *Agent) reconnectLoop(dialCtx, reconnectCtx context.Context) {
	defer a.reconnectWg.Done()

	a.logger.Debug("Starting reconnection monitor")

	for {
		// Check if we should stop
		select {
		case <-a.reconnectCloseCh:
			a.logger.Debug("Reconnection monitor stopped via close channel")
			return
		case <-reconnectCtx.Done():
			a.logger.Debug("Reconnection monitor stopped via context cancellation")
			return
		default:
		}

		// Get connection state
		a.mu.RLock()
		conn := a.conn
		closed := a.closed
		a.mu.RUnlock()

		if closed || conn == nil {
			// No active connection to monitor, wait
			select {
			case <-a.reconnectCloseCh:
				return
			case <-reconnectCtx.Done():
				return
			case <-time.After(a.reconnectInterval):
				continue
			}
		}

		// Wait for state change with timeout
		waitCtx, cancel := context.WithTimeout(context.Background(), a.reconnectInterval)
		conn.WaitForStateChange(waitCtx, conn.GetState())
		cancel()

		// Check current state
		state := conn.GetState()
		a.logger.Debug("Connection state changed", "state", state.String())

		// Check if connection is in a failure state
		if state == connectivity.TransientFailure || state == connectivity.Shutdown {
			a.mu.Lock()
			a.stats.IsConnected = false
			a.mu.Unlock()

			a.logger.Warn("Connection lost, attempting to reconnect",
				"state", state,
				"addr", a.orchestratorAddr)

			// Attempt reconnection
			if a.shouldReconnect() {
				if err := a.reconnect(dialCtx); err != nil {
					a.logger.Error("Reconnection attempt failed",
						"attempt", a.reconnectAttempts,
						"error", err)
				}
			} else {
				a.logger.Error("Max reconnection attempts reached, giving up",
					"max_attempts", a.reconnectMaxAttempts)
				return
			}
		}
	}
}

// shouldReconnect checks if we should attempt another reconnection
func (a *Agent) shouldReconnect() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.reconnectMaxAttempts == 0 {
		// Infinite retries
		return true
	}

	if a.reconnectAttempts >= a.reconnectMaxAttempts {
		return false
	}

	a.reconnectAttempts++
	a.stats.ReconnectAttempts = int64(a.reconnectAttempts)
	return true
}

// reconnect attempts to re-establish the connection
func (a *Agent) reconnect(ctx context.Context) error {
	a.logger.Info("Attempting to reconnect", "attempt", a.reconnectAttempts, "addr", a.orchestratorAddr)

	// Close existing connection
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()

	if conn != nil {
		if err := conn.Close(); err != nil {
			a.logger.Warn("Failed to close old connection during reconnect", "error", err)
		}
	}

	// Create dial context with timeout
	dialCtx, cancel := context.WithTimeout(ctx, a.dialTimeout)
	defer cancel()

	// Dial the server
	newConn, err := grpc.DialContext(dialCtx, a.orchestratorAddr, a.buildDialOptions()...)
	if err != nil {
		a.logger.Error("Failed to reconnect to orchestrator", "addr", a.orchestratorAddr, "error", err)
		return types.WrapError(types.ErrCodeUnavailable, "failed to reconnect to orchestrator", err)
	}

	a.mu.Lock()
	a.conn = newConn
	a.stats.ConnectTime = time.Now()
	a.stats.IsConnected = true
	a.mu.Unlock()

	a.logger.Info("Successfully reconnected to orchestrator", "addr", a.orchestratorAddr)
	return nil
}

// Close closes the agent connection
func (a *Agent) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "agent already closed")
	}
	a.closed = true
	a.stats.IsConnected = false

	// Cancel any ongoing reconnection context
	if a.reconnectCancel != nil {
		a.reconnectCancel()
		a.reconnectCancel = nil
	}
	a.mu.Unlock()

	// Signal reconnection goroutine to stop
	close(a.reconnectCloseCh)

	// Wait for reconnection goroutine to finish
	a.reconnectWg.Wait()

	a.logger.Info("Closing worker agent", "addr", a.orchestratorAddr)

	// Close the connection
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()

	if conn != nil {
		if err := conn.Close(); err != nil {
			a.logger.Error("Failed to close connection", "error", err)
			return types.WrapError(types.ErrCodeInternal, "failed to close connection", err)
		}
	}

	a.mu.Lock()
	a.conn = nil
	a.mu.Unlock()

	a.logger.Info("Worker agent closed", "addr", a.orchestratorAddr)
	return nil
}

// GetConn returns the underlying grpc.ClientConn
func (a *Agent) GetConn() *grpc.ClientConn {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.conn
}

// Stats returns the current agent statistics
func (a *Agent) Stats() AgentStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.stats
}

// IsConnected returns true if the agent is connected
func (a *Agent) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.stats.IsConnected && a.conn != nil && a.conn.GetState() == connectivity.Ready
}

// String returns a string representation of the agent
func (a *Agent) String() string {
	stats := a.Stats()
	return fmt.Sprintf("Agent{OrchestratorAddr: %s, Connected: %v, ConnectTime: %v}",
		a.orchestratorAddr, stats.IsConnected, stats.ConnectTime)
}

// OrchestratorAddr returns the orchestrator address the agent is connected to
func (a *Agent) OrchestratorAddr() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.orchestratorAddr
}

// GetState returns the current connection state
func (a *Agent) GetState() connectivity.State {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.conn == nil {
		return connectivity.Idle
	}
	return a.conn.GetState()
}

// WaitForStateChange blocks until the state changes or ctx expires
func (a *Agent) WaitForStateChange(ctx context.Context, sourceState connectivity.State) bool {
	a.mu.RLock()
	conn := a.conn
	a.mu.RUnlock()

	if conn == nil {
		return false
	}

	return conn.WaitForStateChange(ctx, sourceState)
}

// ResetConnection closes and re-establishes the connection
func (a *Agent) ResetConnection(ctx context.Context) error {
	a.logger.Info("Resetting connection", "addr", a.orchestratorAddr)

	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()

	// Close existing connection
	if conn != nil {
		if err := conn.Close(); err != nil {
			a.logger.Warn("Failed to close existing connection", "error", err)
		}
	}

	a.mu.Lock()
	a.conn = nil
	a.stats.IsConnected = false
	a.mu.Unlock()

	// Re-establish connection
	return a.Dial(ctx)
}
