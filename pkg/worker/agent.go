package worker

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
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
	// DefaultHeartbeatInterval is the default interval between heartbeats
	DefaultHeartbeatInterval = 30 * time.Second
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

	// Heartbeat fields
	heartbeatInterval time.Duration
	heartbeatCtx      context.Context
	heartbeatCancel   context.CancelFunc
	heartbeatCloseCh  chan struct{}
	heartbeatWg       sync.WaitGroup

	// Registration fields
	agentID             string
	registrationToken   string
	name                string
	registered          bool
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
	DialTimeout         time.Duration
	RPCTimeout          time.Duration
	MaxRecvMsgSize      int
	MaxSendMsgSize      int
	ReconnectInterval   time.Duration
	ReconnectMaxAttempts int
	HeartbeatInterval   time.Duration
	DialOptions         []grpc.DialOption
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

	// Set default heartbeat interval
	heartbeatInterval := cfg.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = DefaultHeartbeatInterval
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
		heartbeatInterval:    heartbeatInterval,
		heartbeatCloseCh:     make(chan struct{}),
		stats:                AgentStats{},
	}

	a.logger.Info("Worker agent initialized",
		"orchestrator_addr", orchestratorAddr,
		"dial_timeout", a.dialTimeout.String(),
		"rpc_timeout", a.rpcTimeout.String(),
		"max_recv_msg_size", a.maxRecvMsgSize,
		"max_send_msg_size", a.maxSendMsgSize,
		"reconnect_interval", a.reconnectInterval.String(),
		"reconnect_max_attempts", a.reconnectMaxAttempts,
		"heartbeat_interval", a.heartbeatInterval.String())

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

// Close closes the agent connection and unregisters from the orchestrator
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

	// Cancel any ongoing heartbeat context
	if a.heartbeatCancel != nil {
		a.heartbeatCancel()
		a.heartbeatCancel = nil
	}

	// Check if agent is registered
	registered := a.registered
	conn := a.conn
	a.mu.Unlock()

	// Unregister from orchestrator if registered
	if registered && conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), a.rpcTimeout)
		defer cancel()

		reason := "worker agent closing"
		if err := a.Unregister(ctx, reason); err != nil {
			a.logger.Warn("Failed to unregister agent during close", "error", err)
			// Continue with cleanup even if unregister fails
		}
	}

	// Signal reconnection goroutine to stop
	close(a.reconnectCloseCh)

	// Signal heartbeat goroutine to stop
	close(a.heartbeatCloseCh)

	// Wait for reconnection goroutine to finish
	a.reconnectWg.Wait()

	// Wait for heartbeat goroutine to finish
	a.heartbeatWg.Wait()

	a.logger.Info("Closing worker agent", "addr", a.orchestratorAddr)

	// Close the connection
	a.mu.Lock()
	conn = a.conn
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

// =============================================================================
// Agent Registration
// =============================================================================

// Register registers the worker agent with the orchestrator
func (a *Agent) Register(ctx context.Context, name string) error {
	a.mu.Lock()
	if a.registered {
		a.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "agent already registered")
	}
	a.mu.Unlock()

	a.logger.Info("Registering agent", "name", name)

	// Create RPC client
	client := proto.NewAgentServiceClient(a.conn)

	// Create RPC context with timeout
	rpcCtx, cancel := context.WithTimeout(ctx, a.rpcTimeout)
	defer cancel()

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		a.logger.Warn("Failed to get hostname", "error", err)
		hostname = "unknown"
	}

	// Get PID
	pid := strconv.Itoa(os.Getpid())

	// Create metadata
	metadata := &proto.AgentMetadata{
		Version:     getWorkerVersion(),
		Description: "Worker agent for containerized tool execution",
		Labels: map[string]string{
			"component": "worker",
			"runtime":   runtime.GOOS + "/" + runtime.GOARCH,
		},
		Hostname: hostname,
		Pid:      pid,
	}

	// Create capabilities
	capabilities := &proto.AgentCapabilities{
		SupportedTasks: []string{
			"file_operation",
			"network_request",
			"tool_execution",
		},
		SupportedTools: []string{
			"file_read",
			"file_write",
			"file_edit",
			"grep",
			"find",
			"web_search",
			"web_fetch",
		},
		MaxConcurrentTasks: 5,
		ResourceLimits: &proto.ResourceLimits{
			NanoCpus:    2000000000, // 2 CPUs
			MemoryBytes: 2 * 1024 * 1024 * 1024, // 2GB
		},
		SupportsStreaming:    true,
		SupportsCancellation: true,
		SupportedProtocols:   []string{"grpc"},
	}

	// Create register request
	req := &proto.RegisterRequest{
		Name:         name,
		Type:         proto.AgentType_AGENT_TYPE_WORKER,
		Metadata:     metadata,
		Capabilities: capabilities,
	}

	// Call Register RPC
	a.stats.TotalRPCs++
	resp, err := client.Register(rpcCtx, req)
	if err != nil {
		a.stats.FailedRPCs++
		a.mu.Lock()
		a.stats.LastRPCError = err.Error()
		a.mu.Unlock()
		a.logger.Error("Failed to register agent", "error", err)
		return types.WrapError(types.ErrCodeUnavailable, "failed to register agent", err)
	}

	// Store registration info
	a.mu.Lock()
	a.agentID = resp.AgentId
	a.registrationToken = resp.RegistrationToken
	a.name = name
	a.registered = true
	a.mu.Unlock()

	// Start heartbeat loop
	a.startHeartbeatMonitor(ctx)

	a.logger.Info("Agent registered successfully",
		"agent_id", resp.AgentId,
		"name", name)

	return nil
}

// GetAgentID returns the agent ID
func (a *Agent) GetAgentID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentID
}

// IsRegistered returns true if the agent is registered
func (a *Agent) IsRegistered() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.registered
}

// getWorkerVersion returns the worker version from build info
func getWorkerVersion() string {
	// TODO: Get version from build info using ldflags
	return "dev"
}

// Unregister unregisters the worker agent from the orchestrator
func (a *Agent) Unregister(ctx context.Context, reason string) error {
	a.mu.Lock()
	if !a.registered {
		a.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "agent not registered")
	}
	agentID := a.agentID
	a.mu.Unlock()

	a.logger.Info("Unregistering agent", "agent_id", agentID, "reason", reason)

	// Create RPC client
	client := proto.NewAgentServiceClient(a.conn)

	// Create RPC context with timeout
	rpcCtx, cancel := context.WithTimeout(ctx, a.rpcTimeout)
	defer cancel()

	// Create unregister request
	req := &proto.UnregisterRequest{
		AgentId: agentID,
		Reason:  reason,
	}

	// Call Unregister RPC
	a.stats.TotalRPCs++
	resp, err := client.Unregister(rpcCtx, req)
	if err != nil {
		a.stats.FailedRPCs++
		a.mu.Lock()
		a.stats.LastRPCError = err.Error()
		a.mu.Unlock()
		a.logger.Error("Failed to unregister agent", "agent_id", agentID, "error", err)
		return types.WrapError(types.ErrCodeUnavailable, "failed to unregister agent", err)
	}

	// Clear registration state
	a.mu.Lock()
	a.registered = false
	a.agentID = ""
	a.registrationToken = ""
	a.name = ""
	a.mu.Unlock()

	a.logger.Info("Agent unregistered successfully",
		"agent_id", agentID,
		"message", resp.Message)

	return nil
}

// =============================================================================
// Agent Heartbeat
// =============================================================================

// startHeartbeatMonitor starts a goroutine that sends heartbeats at regular intervals
func (a *Agent) startHeartbeatMonitor(ctx context.Context) {
	a.mu.Lock()
	if a.heartbeatCancel != nil {
		// Already running
		a.mu.Unlock()
		return
	}
	// Create a context for the heartbeat goroutine
	heartbeatCtx, cancel := context.WithCancel(context.Background())
	a.heartbeatCtx = heartbeatCtx
	a.heartbeatCancel = cancel
	a.mu.Unlock()

	a.heartbeatWg.Add(1)
	go a.heartbeatLoop(ctx, heartbeatCtx)
}

// heartbeatLoop is the main heartbeat goroutine
func (a *Agent) heartbeatLoop(dialCtx, heartbeatCtx context.Context) {
	defer a.heartbeatWg.Done()

	a.logger.Debug("Starting heartbeat monitor",
		"interval", a.heartbeatInterval.String())

	// Create a ticker for heartbeats
	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	// Send initial heartbeat immediately
	a.sendHeartbeat(dialCtx)

	for {
		select {
		case <-a.heartbeatCloseCh:
			a.logger.Debug("Heartbeat monitor stopped via close channel")
			return
		case <-heartbeatCtx.Done():
			a.logger.Debug("Heartbeat monitor stopped via context cancellation")
			return
		case <-ticker.C:
			// Send heartbeat on each tick
			a.sendHeartbeat(dialCtx)
		}
	}
}

// sendHeartbeat sends a single heartbeat to the orchestrator
func (a *Agent) sendHeartbeat(ctx context.Context) {
	a.mu.RLock()
	agentID := a.agentID
	registered := a.registered
	closed := a.closed
	a.mu.RUnlock()

	if !registered || closed || agentID == "" {
		// Not registered or closed, skip heartbeat
		return
	}

	a.logger.Debug("Sending heartbeat", "agent_id", agentID)

	// Create RPC client
	client := proto.NewAgentServiceClient(a.conn)

	// Create RPC context with timeout
	rpcCtx, cancel := context.WithTimeout(ctx, a.rpcTimeout)
	defer cancel()

	// Create heartbeat request
	// Note: We could optionally include resource usage and active tasks here
	// For now, we send minimal heartbeat
	req := &proto.HeartbeatRequest{
		AgentId:     agentID,
		ActiveTasks: []string{}, // TODO: Track active tasks
	}

	// Call Heartbeat RPC
	a.stats.TotalRPCs++
	resp, err := client.Heartbeat(rpcCtx, req)
	if err != nil {
		a.stats.FailedRPCs++
		a.mu.Lock()
		a.stats.LastRPCError = err.Error()
		a.mu.Unlock()
		a.logger.Warn("Failed to send heartbeat", "agent_id", agentID, "error", err)
		return
	}

	a.logger.Debug("Heartbeat successful",
		"agent_id", agentID,
		"timestamp", resp.Timestamp,
		"pending_tasks", len(resp.PendingTasks))

	// TODO: Handle pending tasks from response
}
