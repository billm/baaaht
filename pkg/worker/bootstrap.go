package worker

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

const (
	// DefaultShutdownTimeout is the default timeout for graceful shutdown
	DefaultShutdownTimeout = 30 * time.Second
	// DefaultOrchestratorAddr is the default orchestrator address
	DefaultOrchestratorAddr = "unix:///tmp/baaaht-grpc.sock"
)

// BootstrapResult contains the result of a worker bootstrap operation
type BootstrapResult struct {
	Agent      *Agent
	StartedAt  time.Time
	Version    string
	WorkerName string
	Error      error
}

// BootstrapConfig contains configuration for the worker bootstrap process
type BootstrapConfig struct {
	Logger            *logger.Logger
	Version           string
	OrchestratorAddr  string
	WorkerName        string
	DialTimeout       time.Duration
	RPCTimeout        time.Duration
	MaxRecvMsgSize    int
	MaxSendMsgSize    int
	ReconnectInterval time.Duration
	ReconnectMaxAttempts int
	HeartbeatInterval time.Duration
	ShutdownTimeout   time.Duration
	EnableHealthCheck bool
}

// NewDefaultBootstrapConfig creates a default bootstrap configuration
func NewDefaultBootstrapConfig() BootstrapConfig {
	log, err := logger.NewDefault()
	if err != nil {
		log = nil
	}

	// Get worker name from hostname
	workerName := "worker-" + fmt.Sprintf("%d", time.Now().Unix())
	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		workerName = "worker-" + hostname
	}

	return BootstrapConfig{
		Logger:              log,
		Version:             DefaultVersion,
		OrchestratorAddr:    DefaultOrchestratorAddr,
		WorkerName:          workerName,
		DialTimeout:         DefaultDialTimeout,
		RPCTimeout:          DefaultRPCTimeout,
		MaxRecvMsgSize:      DefaultMaxRecvMsgSize,
		MaxSendMsgSize:      DefaultMaxSendMsgSize,
		ReconnectInterval:   DefaultReconnectInterval,
		ReconnectMaxAttempts: DefaultReconnectMaxAttempts,
		HeartbeatInterval:   DefaultHeartbeatInterval,
		ShutdownTimeout:     DefaultShutdownTimeout,
		EnableHealthCheck:   true,
	}
}

// Bootstrap creates and initializes a new worker agent with the specified configuration
// It performs the following steps:
// 1. Creates the worker agent
// 2. Establishes connection to orchestrator
// 3. Registers the worker
// 4. Starts heartbeat loop
// 5. Performs health checks (if enabled)
// 6. Returns the initialized agent
func Bootstrap(ctx context.Context, cfg BootstrapConfig) (*BootstrapResult, error) {
	startedAt := time.Now()

	result := &BootstrapResult{
		StartedAt:  startedAt,
		Version:    cfg.Version,
		WorkerName: cfg.WorkerName,
	}

	// Create logger if not provided
	log := cfg.Logger
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			result.Error = types.WrapError(types.ErrCodeInternal, "failed to create logger", err)
			return result, result.Error
		}
	}

	log.Info("Starting worker bootstrap",
		"version", cfg.Version,
		"orchestrator_addr", cfg.OrchestratorAddr,
		"worker_name", cfg.WorkerName)

	// Create agent configuration
	agentCfg := AgentConfig{
		DialTimeout:         cfg.DialTimeout,
		RPCTimeout:          cfg.RPCTimeout,
		MaxRecvMsgSize:      cfg.MaxRecvMsgSize,
		MaxSendMsgSize:      cfg.MaxSendMsgSize,
		ReconnectInterval:   cfg.ReconnectInterval,
		ReconnectMaxAttempts: cfg.ReconnectMaxAttempts,
		HeartbeatInterval:   cfg.HeartbeatInterval,
	}

	// Create worker agent
	log.Info("Creating worker agent...")
	agent, err := NewAgent(cfg.OrchestratorAddr, agentCfg, log)
	if err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to create worker agent", err)
		return result, result.Error
	}
	result.Agent = agent

	// Connect to orchestrator
	log.Info("Connecting to orchestrator...", "address", cfg.OrchestratorAddr)
	if err := agent.Dial(ctx); err != nil {
		result.Error = types.WrapError(types.ErrCodeUnavailable, "failed to connect to orchestrator", err)
		// Cleanup on connection failure
		_ = agent.Close()
		return result, result.Error
	}
	log.Info("Connected to orchestrator successfully")

	// Register with orchestrator
	log.Info("Registering worker...", "name", cfg.WorkerName)
	if err := agent.Register(ctx, cfg.WorkerName); err != nil {
		result.Error = types.WrapError(types.ErrCodeUnavailable, "failed to register worker", err)
		// Cleanup on registration failure
		_ = agent.Close()
		return result, result.Error
	}
	log.Info("Worker registered successfully",
		"agent_id", agent.GetAgentID(),
		"name", cfg.WorkerName)

	// Perform health check if enabled
	if cfg.EnableHealthCheck {
		healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if !agent.IsConnected() {
			log.Error("Worker health check failed: not connected to orchestrator")
			result.Error = types.NewError(types.ErrCodeInternal, "worker health check failed: not connected")
			_ = agent.Close()
			return result, result.Error
		}

		if !agent.IsRegistered() {
			log.Error("Worker health check failed: not registered")
			result.Error = types.NewError(types.ErrCodeInternal, "worker health check failed: not registered")
			_ = agent.Close()
			return result, result.Error
		}

		log.Info("Worker health check passed")
	}

	log.Info("Worker bootstrapped successfully",
		"version", cfg.Version,
		"worker_name", cfg.WorkerName,
		"agent_id", agent.GetAgentID(),
		"duration", time.Since(startedAt))

	return result, nil
}

// BootstrapWithDefaults creates and initializes a new worker with default configuration
func BootstrapWithDefaults(ctx context.Context) (*BootstrapResult, error) {
	cfg := NewDefaultBootstrapConfig()
	return Bootstrap(ctx, cfg)
}

// BootstrapWorker is a simplified bootstrap function that returns only the agent
func BootstrapWorker(ctx context.Context, orchestratorAddr, workerName string, log *logger.Logger) (*Agent, error) {
	cfg := BootstrapConfig{
		Logger:            log,
		Version:           DefaultVersion,
		OrchestratorAddr:  orchestratorAddr,
		WorkerName:        workerName,
		DialTimeout:       DefaultDialTimeout,
		RPCTimeout:        DefaultRPCTimeout,
		MaxRecvMsgSize:    DefaultMaxRecvMsgSize,
		MaxSendMsgSize:    DefaultMaxSendMsgSize,
		ReconnectInterval: DefaultReconnectInterval,
		ReconnectMaxAttempts: DefaultReconnectMaxAttempts,
		HeartbeatInterval: DefaultHeartbeatInterval,
		ShutdownTimeout:   DefaultShutdownTimeout,
		EnableHealthCheck: true,
	}

	result, err := Bootstrap(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return result.Agent, nil
}

// IsReady checks if the worker is ready to accept tasks
func IsReady(agent *Agent) bool {
	if agent == nil {
		return false
	}

	return agent.IsConnected() && agent.IsRegistered()
}

// WaitForReady waits for the worker to be ready with a timeout
func WaitForReady(ctx context.Context, agent *Agent, timeout time.Duration, checkInterval time.Duration) error {
	if agent == nil {
		return types.NewError(types.ErrCodeInvalidArgument, "agent is nil")
	}

	if checkInterval == 0 {
		checkInterval = 100 * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return types.WrapError(types.ErrCodeTimeout, "worker not ready within timeout", ctx.Err())
		case <-ticker.C:
			if IsReady(agent) {
				return nil
			}
		}
	}
}

// Global worker instance
var (
	globalWorker  *Agent
	globalWorkerMu sync.RWMutex
	globalWorkerOnce sync.Once
)

// InitGlobal initializes the global worker with the specified configuration
func InitGlobal(ctx context.Context, cfg BootstrapConfig) error {
	var initErr error
	globalWorkerOnce.Do(func() {
		result, err := Bootstrap(ctx, cfg)
		if err != nil {
			initErr = err
			return
		}
		globalWorkerMu.Lock()
		globalWorker = result.Agent
		globalWorkerMu.Unlock()
	})
	return initErr
}

// InitGlobalWithDefaults initializes the global worker with default configuration
func InitGlobalWithDefaults(ctx context.Context) error {
	cfg := NewDefaultBootstrapConfig()
	return InitGlobal(ctx, cfg)
}

// Global returns the global worker instance
func Global() *Agent {
	globalWorkerMu.RLock()
	worker := globalWorker
	globalWorkerMu.RUnlock()

	if worker == nil {
		// Initialize with default settings if not already initialized
		ctx := context.Background()
		if err := InitGlobalWithDefaults(ctx); err != nil {
			// Return nil on error
			return nil
		}
		globalWorkerMu.RLock()
		worker = globalWorker
		globalWorkerMu.RUnlock()
	}

	return worker
}

// SetGlobal sets the global worker instance
func SetGlobal(agent *Agent) {
	globalWorkerMu.Lock()
	globalWorker = agent
	globalWorkerMu.Unlock()
	globalWorkerOnce = sync.Once{}
}

// IsGlobalInitialized returns true if the global worker has been initialized
func IsGlobalInitialized() bool {
	globalWorkerMu.RLock()
	defer globalWorkerMu.RUnlock()
	return globalWorker != nil
}

// CloseGlobal closes and clears the global worker instance
func CloseGlobal() error {
	globalWorkerMu.Lock()
	defer globalWorkerMu.Unlock()

	if globalWorker == nil {
		return nil
	}

	if err := globalWorker.Close(); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to close global worker", err)
	}

	globalWorker = nil
	globalWorkerOnce = sync.Once{}

	return nil
}

// String returns a string representation of the bootstrap result
func (r *BootstrapResult) String() string {
	if r.Error != nil {
		return fmt.Sprintf("BootstrapResult{version: %s, worker_name: %s, error: %v}",
			r.Version, r.WorkerName, r.Error)
	}
	return fmt.Sprintf("BootstrapResult{version: %s, worker_name: %s, started_at: %s, agent: %v}",
		r.Version, r.WorkerName, r.StartedAt.Format(time.RFC3339), r.Agent != nil)
}

// IsSuccessful returns true if the bootstrap was successful
func (r *BootstrapResult) IsSuccessful() bool {
	return r.Error == nil && r.Agent != nil
}

// Duration returns the duration of the bootstrap process
func (r *BootstrapResult) Duration() time.Duration {
	return time.Since(r.StartedAt)
}
