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
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/emptypb"

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

func (a *Agent) incrementTotalRPCs() {
	a.mu.Lock()
	a.stats.TotalRPCs++
	a.mu.Unlock()
}

func (a *Agent) recordRPCFailure(err error) {
	a.mu.Lock()
	a.stats.FailedRPCs++
	a.stats.LastRPCError = err.Error()
	a.mu.Unlock()
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
	a.incrementTotalRPCs()
	resp, err := client.Register(rpcCtx, req)
	if err != nil {
		a.recordRPCFailure(err)
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
	a.incrementTotalRPCs()
	resp, err := client.Unregister(rpcCtx, req)
	if err != nil {
		a.recordRPCFailure(err)
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
	a.incrementTotalRPCs()
	resp, err := client.Heartbeat(rpcCtx, req)
	if err != nil {
		a.recordRPCFailure(err)
		a.logger.Warn("Failed to send heartbeat", "agent_id", agentID, "error", err)
		return
	}

	a.logger.Debug("Heartbeat successful",
		"agent_id", agentID,
		"timestamp", resp.Timestamp,
		"pending_tasks", len(resp.PendingTasks))

	// TODO: Handle pending tasks from response
}

// =============================================================================
// Task Reception
// =============================================================================

// ListenForTasks establishes a bidirectional stream for task execution
func (a *Agent) ListenForTasks(ctx context.Context, taskID string) (proto.AgentService_StreamTaskClient, error) {
	a.mu.RLock()
	registered := a.registered
	agentID := a.agentID
	conn := a.conn
	a.mu.RUnlock()

	if !registered {
		return nil, types.NewError(types.ErrCodeInvalid, "agent not registered")
	}

	if agentID == "" {
		return nil, types.NewError(types.ErrCodeInvalid, "agent ID is empty")
	}

	if conn == nil {
		return nil, types.NewError(types.ErrCodeUnavailable, "not connected to orchestrator")
	}

	a.logger.Info("Listening for tasks", "agent_id", agentID, "task_id", taskID)

	// Create RPC client
	client := proto.NewAgentServiceClient(conn)

	// Establish bidirectional stream
	stream, err := client.StreamTask(ctx)
	if err != nil {
		a.logger.Error("Failed to establish stream", "task_id", taskID, "error", err)
		return nil, types.WrapError(types.ErrCodeUnavailable, "failed to establish stream", err)
	}

	// Send first message with task_id
	req := &proto.StreamTaskRequest{
		TaskId: taskID,
		Payload: &proto.StreamTaskRequest_Heartbeat{
			Heartbeat: &emptypb.Empty{},
		},
	}

	a.incrementTotalRPCs()
	if err := stream.Send(req); err != nil {
		a.recordRPCFailure(err)
		a.logger.Error("Failed to send initial stream message", "task_id", taskID, "error", err)
		return nil, types.WrapError(types.ErrCodeInternal, "failed to send initial stream message", err)
	}

	a.logger.Info("Stream established for task", "task_id", taskID)

	return stream, nil
}

// SendTaskProgress sends a progress update for a task via the stream
func (a *Agent) SendTaskProgress(stream proto.AgentService_StreamTaskClient, percent float64, message string, details map[string]string) error {
	if stream == nil {
		return types.NewError(types.ErrCodeInvalid, "stream is nil")
	}

	req := &proto.StreamTaskRequest{
		Payload: &proto.StreamTaskRequest_Progress{
			Progress: &proto.TaskProgress{
				Percent: percent,
				Message: message,
				Details: details,
			},
		},
	}

	if err := stream.Send(req); err != nil {
		a.logger.Error("Failed to send task progress", "error", err)
		return types.WrapError(types.ErrCodeInternal, "failed to send task progress", err)
	}

	a.logger.Debug("Sent task progress", "percent", percent, "message", message)
	return nil
}

// SendTaskOutput sends an output update for a task via the stream
func (a *Agent) SendTaskOutput(stream proto.AgentService_StreamTaskClient, data []byte, text, streamType string) error {
	if stream == nil {
		return types.NewError(types.ErrCodeInvalid, "stream is nil")
	}

	req := &proto.StreamTaskRequest{
		Payload: &proto.StreamTaskRequest_Output{
			Output: &proto.TaskOutput{
				Data:        data,
				Text:        text,
				StreamType:  streamType,
			},
		},
	}

	if err := stream.Send(req); err != nil {
		a.logger.Error("Failed to send task output", "error", err)
		return types.WrapError(types.ErrCodeInternal, "failed to send task output", err)
	}

	a.logger.Debug("Sent task output", "stream_type", streamType, "text_length", len(text))
	return nil
}

// SendTaskStatus sends a status update for a task via the stream
func (a *Agent) SendTaskStatus(stream proto.AgentService_StreamTaskClient, state proto.TaskState, message string) error {
	if stream == nil {
		return types.NewError(types.ErrCodeInvalid, "stream is nil")
	}

	req := &proto.StreamTaskRequest{
		Payload: &proto.StreamTaskRequest_Status{
			Status: &proto.TaskStatusUpdate{
				State:     state,
				Message:   message,
				Timestamp: timestamppb.Now(),
			},
		},
	}

	if err := stream.Send(req); err != nil {
		a.logger.Error("Failed to send task status", "error", err)
		return types.WrapError(types.ErrCodeInternal, "failed to send task status", err)
	}

	a.logger.Debug("Sent task status", "state", state.String(), "message", message)
	return nil
}

// SendTaskComplete sends a task completion message via the stream
func (a *Agent) SendTaskComplete(stream proto.AgentService_StreamTaskClient, taskID string, exitCode int32, outputText, errorText string, metadata map[string]string) error {
	if stream == nil {
		return types.NewError(types.ErrCodeInvalid, "stream is nil")
	}

	// Calculate execution duration - use zero if no start time available
	// Note: Callers should track task start time and pass it for accurate duration
	duration := time.Duration(0)

	result := &proto.TaskResult{
		ExitCode:           exitCode,
		OutputText:         outputText,
		ErrorText:          errorText,
		Metadata:           metadata,
		CompletedAt:        timestamppb.Now(),
		ExecutionDurationNs: duration.Nanoseconds(),
	}

	req := &proto.StreamTaskRequest{
		Payload: &proto.StreamTaskRequest_Complete{
			Complete: &proto.TaskComplete{
				TaskId:      taskID,
				Result:      result,
				CompletedAt: timestamppb.Now(),
			},
		},
	}

	if err := stream.Send(req); err != nil {
		a.logger.Error("Failed to send task complete", "error", err)
		return types.WrapError(types.ErrCodeInternal, "failed to send task complete", err)
	}

	a.logger.Info("Sent task complete", "task_id", taskID, "exit_code", exitCode)
	return nil
}

// SendTaskError sends an error message for a task via the stream
func (a *Agent) SendTaskError(stream proto.AgentService_StreamTaskClient, code, message string, details []string) error {
	if stream == nil {
		return types.NewError(types.ErrCodeInvalid, "stream is nil")
	}

	req := &proto.StreamTaskRequest{
		Payload: &proto.StreamTaskRequest_ErrorMsg{
			ErrorMsg: &proto.TaskError{
				Code:        code,
				Message:     message,
				Details:     details,
				OccurredAt:  timestamppb.Now(),
			},
		},
	}

	if err := stream.Send(req); err != nil {
		a.logger.Error("Failed to send task error", "error", err)
		return types.WrapError(types.ErrCodeInternal, "failed to send task error", err)
	}

	a.logger.Debug("Sent task error", "code", code, "message", message)
	return nil
}

// SendStreamHeartbeat sends a heartbeat via the stream to keep it alive
func (a *Agent) SendStreamHeartbeat(stream proto.AgentService_StreamTaskClient) error {
	if stream == nil {
		return types.NewError(types.ErrCodeInvalid, "stream is nil")
	}

	req := &proto.StreamTaskRequest{
		Payload: &proto.StreamTaskRequest_Heartbeat{
			Heartbeat: &emptypb.Empty{},
		},
	}

	if err := stream.Send(req); err != nil {
		a.logger.Debug("Failed to send stream heartbeat", "error", err)
		return types.WrapError(types.ErrCodeInternal, "failed to send stream heartbeat", err)
	}

	return nil
}

// =============================================================================
// Task Routing
// =============================================================================

// RouteTask routes a task to the appropriate executor based on task type
// It parses the task configuration and executes the corresponding tool operation
func (a *Agent) RouteTask(ctx context.Context, executor *Executor, taskType proto.TaskType, config *proto.TaskConfig, mountSource string) (string, error) {
	a.logger.Info("Routing task",
		"task_type", taskType.String(),
		"command", config.Command,
		"args", config.Arguments)

	// Extract operation from command (e.g., "file_read", "web_search", etc.)
	// If command is empty, look for it in parameters
	operation := config.Command
	if opParam, ok := config.Parameters["operation"]; ok && operation == "" {
		operation = opParam
	}
	if operation == "" {
		operation = config.Parameters["op"]
	}

	// Use Arguments for additional arguments
	args := config.Arguments

	switch taskType {
	case proto.TaskType_TASK_TYPE_FILE_OPERATION:
		// Route file operations to appropriate tool
		return a.routeFileOperation(ctx, executor, operation, args, config, mountSource)

	case proto.TaskType_TASK_TYPE_NETWORK_REQUEST:
		// Route network requests to web tools
		return a.routeNetworkRequest(ctx, executor, operation, args, config)

	case proto.TaskType_TASK_TYPE_TOOL_EXECUTION:
		// Direct tool execution
		return a.routeToolExecution(ctx, executor, operation, args, config, mountSource)

	case proto.TaskType_TASK_TYPE_CODE_EXECUTION,
		proto.TaskType_TASK_TYPE_DATA_PROCESSING,
		proto.TaskType_TASK_TYPE_CONTAINER_OPERATION,
		proto.TaskType_TASK_TYPE_CUSTOM:
		// For custom tasks, execute as a shell command
		return a.executeCustomCommand(ctx, executor, config, mountSource)

	default:
		return "", fmt.Errorf("unsupported task type: %s", taskType.String())
	}
}

// routeFileOperation routes file operations to the appropriate tool
func (a *Agent) routeFileOperation(ctx context.Context, executor *Executor, operation string, args []string, config *proto.TaskConfig, mountSource string) (string, error) {
	switch operation {
	case "read", "file_read", "cat":
		// File read operation
		filePath := a.extractFilePath(config, args)
		return FileRead(ctx, executor, mountSource, filePath)

	case "write", "file_write":
		// File write operation
		filePath := a.extractFilePath(config, args)
		content := a.extractContent(config)
		return "", FileWrite(ctx, executor, mountSource, filePath, content)

	case "edit", "file_edit", "sed":
		// File edit operation - uses ExecuteTask with sed tool
		return a.executeFileEdit(ctx, executor, config, mountSource)

	case "grep", "search":
		// Grep/search operation
		return a.executeGrep(ctx, executor, config, mountSource)

	case "find":
		// Find operation
		return a.executeFind(ctx, executor, config, mountSource)

	case "list", "ls":
		// List directory operation
		return a.executeList(ctx, executor, config, mountSource)

	default:
		// Try to execute as direct tool
		toolType := a.mapOperationToToolType(operation)
		if toolType != "" {
			return a.executeWithTool(ctx, executor, toolType, args, mountSource)
		}
		return "", fmt.Errorf("unknown file operation: %s", operation)
	}
}

// routeNetworkRequest routes network requests to the appropriate web tool
func (a *Agent) routeNetworkRequest(ctx context.Context, executor *Executor, operation string, args []string, config *proto.TaskConfig) (string, error) {
	switch operation {
	case "search", "web_search":
		// Web search operation
		url := a.extractURLFromConfig(config, args)
		return WebSearch(ctx, executor, url)

	case "fetch", "web_fetch", "get", "curl":
		// URL fetch operation
		url := a.extractURLFromConfig(config, args)
		return FetchURL(ctx, executor, url)

	default:
		return "", fmt.Errorf("unknown network request operation: %s", operation)
	}
}

// routeToolExecution routes tool execution to the specified tool
func (a *Agent) routeToolExecution(ctx context.Context, executor *Executor, operation string, args []string, config *proto.TaskConfig, mountSource string) (string, error) {
	// Map operation to tool type
	toolType := a.mapOperationToToolType(operation)
	if toolType == "" {
		return "", fmt.Errorf("unknown tool type: %s", operation)
	}

	return a.executeWithTool(ctx, executor, toolType, args, mountSource)
}

// executeCustomCommand executes a custom command using the sh tool
func (a *Agent) executeCustomCommand(ctx context.Context, executor *Executor, config *proto.TaskConfig, mountSource string) (string, error) {
	// Build shell command from config
	command := config.Command
	if command == "" {
		return "", fmt.Errorf("command is required for custom tasks")
	}

	// Execute using a shell command
	taskCfg := TaskConfig{
		ToolType:    ToolTypeFileWrite, // Reuse file write tool for shell execution
		Args:        []string{"sh", "-c", command},
		MountSource: mountSource,
	}

	result := executor.ExecuteTask(ctx, taskCfg)
	if result.Error != nil {
		return "", fmt.Errorf("failed to execute custom command: %w", result.Error)
	}

	if result.ExitCode != 0 {
		errMsg := result.Stderr
		if errMsg == "" {
			errMsg = result.Stdout
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		return "", fmt.Errorf("custom command failed: %s", errMsg)
	}

	return result.Stdout, nil
}

// executeFileEdit executes a file edit operation using sed
func (a *Agent) executeFileEdit(ctx context.Context, executor *Executor, config *proto.TaskConfig, mountSource string) (string, error) {
	filePath := a.extractFilePath(config, nil)
	search := config.Parameters["search"]
	replace := config.Parameters["replace"]

	if filePath == "" {
		return "", fmt.Errorf("file path is required for edit operation")
	}
	if search == "" {
		return "", fmt.Errorf("search pattern is required for edit operation")
	}

	// Build sed command: sed -i "s/search/replace/g" filepath
	sedCmd := fmt.Sprintf("s/%s/%s/g", search, replace)
	if replace == "" {
		// If replace is empty, delete matching lines
		sedCmd = fmt.Sprintf("/%s/d", search)
	}

	// Ensure the path is absolute within the container workspace
	argPath := filePath
	if !startsWith(filePath, "/workspace") {
		if filePath[0] != '/' {
			argPath = "/workspace/" + filePath
		} else {
			argPath = "/workspace" + filePath
		}
	}

	taskCfg := TaskConfig{
		ToolType:    ToolTypeFileEdit,
		Args:        []string{"-i", sedCmd, argPath},
		MountSource: mountSource,
	}

	result := executor.ExecuteTask(ctx, taskCfg)
	if result.Error != nil {
		return "", fmt.Errorf("failed to execute file edit: %w", result.Error)
	}

	if result.ExitCode != 0 {
		errMsg := result.Stderr
		if errMsg == "" {
			errMsg = result.Stdout
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		return "", fmt.Errorf("file edit failed: %s", errMsg)
	}

	return result.Stdout, nil
}

// executeGrep executes a grep search operation
func (a *Agent) executeGrep(ctx context.Context, executor *Executor, config *proto.TaskConfig, mountSource string) (string, error) {
	pattern := config.Parameters["pattern"]
	if pattern == "" && len(config.Arguments) > 0 {
		pattern = config.Arguments[0]
	}
	if pattern == "" {
		return "", fmt.Errorf("pattern is required for grep operation")
	}

	path := config.Parameters["path"]
	if path == "" {
		path = "."
	}

	// Ensure the path is absolute within the container workspace
	argPath := path
	if !startsWith(path, "/workspace") {
		if path[0] != '/' {
			argPath = "/workspace/" + path
		} else {
			argPath = "/workspace" + path
		}
	}

	// Build grep args: -r -n for recursive with line numbers
	args := []string{"-r", "-n", pattern, argPath}

	taskCfg := TaskConfig{
		ToolType:    ToolTypeGrep,
		Args:        args,
		MountSource: mountSource,
	}

	result := executor.ExecuteTask(ctx, taskCfg)
	if result.Error != nil {
		return "", fmt.Errorf("failed to execute grep: %w", result.Error)
	}

	if result.ExitCode != 0 && result.ExitCode != 1 {
		// Grep returns 1 when no matches found, which is valid
		errMsg := result.Stderr
		if errMsg == "" {
			errMsg = result.Stdout
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		return "", fmt.Errorf("grep failed: %s", errMsg)
	}

	return result.Stdout, nil
}

// executeFind executes a find operation
func (a *Agent) executeFind(ctx context.Context, executor *Executor, config *proto.TaskConfig, mountSource string) (string, error) {
	path := config.Parameters["path"]
	if path == "" {
		path = "."
	}

	name := config.Parameters["name"]
	pattern := config.Parameters["pattern"]
	fileType := config.Parameters["type"]

	// Ensure the path is absolute within the container workspace
	argPath := path
	if !startsWith(path, "/workspace") {
		if path[0] != '/' {
			argPath = "/workspace/" + path
		} else {
			argPath = "/workspace" + path
		}
	}

	// Build find args
	args := []string{argPath}

	if name != "" {
		args = append(args, "-name", name)
	}
	if pattern != "" {
		args = append(args, "-name", pattern)
	}
	if fileType != "" {
		args = append(args, "-type", fileType)
	}

	taskCfg := TaskConfig{
		ToolType:    ToolTypeFind,
		Args:        args,
		MountSource: mountSource,
	}

	result := executor.ExecuteTask(ctx, taskCfg)
	if result.Error != nil {
		return "", fmt.Errorf("failed to execute find: %w", result.Error)
	}

	if result.ExitCode != 0 {
		errMsg := result.Stderr
		if errMsg == "" {
			errMsg = result.Stdout
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		return "", fmt.Errorf("find failed: %s", errMsg)
	}

	return result.Stdout, nil
}

// executeList executes a directory listing operation
func (a *Agent) executeList(ctx context.Context, executor *Executor, config *proto.TaskConfig, mountSource string) (string, error) {
	path := config.Parameters["path"]
	recursiveStr := config.Parameters["recursive"]
	recursive := recursiveStr == "true" || recursiveStr == "1"

	return ListFiles(ctx, executor, mountSource, path, recursive)
}

// executeWithTool executes a task using a specific tool type
func (a *Agent) executeWithTool(ctx context.Context, executor *Executor, toolType ToolType, args []string, mountSource string) (string, error) {
	taskCfg := TaskConfig{
		ToolType:    toolType,
		Args:        args,
		MountSource: mountSource,
	}

	result := executor.ExecuteTask(ctx, taskCfg)
	if result.Error != nil {
		return "", fmt.Errorf("failed to execute tool %s: %w", toolType, result.Error)
	}

	if result.ExitCode != 0 {
		errMsg := result.Stderr
		if errMsg == "" {
			errMsg = result.Stdout
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		return "", fmt.Errorf("tool %s failed: %s", toolType, errMsg)
	}

	return result.Stdout, nil
}

// mapOperationToToolType maps an operation name to a ToolType
func (a *Agent) mapOperationToToolType(operation string) ToolType {
	switch operation {
	case "file_read", "read", "cat":
		return ToolTypeFileRead
	case "file_write", "write", "tee":
		return ToolTypeFileWrite
	case "file_edit", "edit", "sed":
		return ToolTypeFileEdit
	case "grep", "search":
		return ToolTypeGrep
	case "find":
		return ToolTypeFind
	case "list", "ls":
		return ToolTypeList
	case "web_search", "search_web":
		return ToolTypeWebSearch
	case "fetch", "web_fetch", "curl":
		return ToolTypeFetchURL
	default:
		return ""
	}
}

// extractFilePath extracts file path from config or args
func (a *Agent) extractFilePath(config *proto.TaskConfig, args []string) string {
	// Try parameters first
	if path, ok := config.Parameters["path"]; ok && path != "" {
		return path
	}
	if target, ok := config.Parameters["target"]; ok && target != "" {
		return target
	}
	if file, ok := config.Parameters["file"]; ok && file != "" {
		return file
	}

	// Try args
	if len(args) > 0 {
		return args[0]
	}

	// Try first argument from config
	if len(config.Arguments) > 0 {
		return config.Arguments[0]
	}

	return ""
}

// extractContent extracts content from config
func (a *Agent) extractContent(config *proto.TaskConfig) string {
	// Try parameters first
	if content, ok := config.Parameters["content"]; ok {
		return content
	}

	// Try InputData
	if len(config.InputData) > 0 {
		return string(config.InputData)
	}

	return ""
}

// extractURLFromConfig extracts URL from config or args
func (a *Agent) extractURLFromConfig(config *proto.TaskConfig, args []string) string {
	// Try parameters first
	if url, ok := config.Parameters["url"]; ok && url != "" {
		return url
	}

	// Try args
	if len(args) > 0 {
		return args[0]
	}

	// Try first argument from config
	if len(config.Arguments) > 0 {
		return config.Arguments[0]
	}

	return ""
}

// =============================================================================
// Task Cancellation
// =============================================================================

// HandleCancelCommand handles a cancel command from the stream
func (a *Agent) HandleCancelCommand(ctx context.Context, executor *Executor, taskID string, cancelCmd *proto.CancelCommand) error {
	if executor == nil {
		return types.NewError(types.ErrCodeInvalid, "executor is nil")
	}

	if cancelCmd == nil {
		return types.NewError(types.ErrCodeInvalid, "cancel command is nil")
	}

	a.logger.Info("Handling cancel command",
		"task_id", taskID,
		"reason", cancelCmd.Reason,
		"force", cancelCmd.Force)

	// Cancel the task in the executor
	if err := executor.CancelTask(ctx, taskID, cancelCmd.Force); err != nil {
		a.logger.Error("Failed to cancel task in executor",
			"task_id", taskID,
			"error", err)
		return types.WrapError(types.ErrCodeInternal, "failed to cancel task", err)
	}

	a.logger.Info("Task cancelled successfully", "task_id", taskID)
	return nil
}

// ProcessStreamRequest processes a stream task request and handles cancellation
func (a *Agent) ProcessStreamRequest(ctx context.Context, executor *Executor, req *proto.StreamTaskRequest, taskID string) error {
	if req == nil {
		return types.NewError(types.ErrCodeInvalid, "stream request is nil")
	}

	// Check if it's a cancel command
	if cancelCmd := req.GetCancel(); cancelCmd != nil {
		return a.HandleCancelCommand(ctx, executor, taskID, cancelCmd)
	}

	// Other request types (Input, Heartbeat) are handled elsewhere
	return nil
}

// SendCancelRequest sends a cancel request via the stream
func (a *Agent) SendCancelRequest(stream proto.AgentService_StreamTaskClient, taskID string, reason string, force bool) error {
	if stream == nil {
		return types.NewError(types.ErrCodeInvalid, "stream is nil")
	}

	req := &proto.StreamTaskRequest{
		TaskId: taskID,
		Payload: &proto.StreamTaskRequest_Cancel{
			Cancel: &proto.CancelCommand{
				Reason: reason,
				Force:  force,
			},
		},
	}

	if err := stream.Send(req); err != nil {
		a.logger.Error("Failed to send cancel request", "task_id", taskID, "error", err)
		return types.WrapError(types.ErrCodeInternal, "failed to send cancel request", err)
	}

	a.logger.Info("Cancel request sent", "task_id", taskID, "reason", reason, "force", force)
	return nil
}
