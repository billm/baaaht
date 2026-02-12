package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/ipc"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/tools"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
	"google.golang.org/grpc"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	// DefaultVersion is the default version of the gRPC server
	DefaultVersion = "0.1.0"
	// BootstrapDefaultShutdownTimeout is the default timeout for graceful shutdown
	BootstrapDefaultShutdownTimeout = 10 * time.Second
)

// BootstrapResult contains the result of a gRPC bootstrap operation
type BootstrapResult struct {
	Server     *Server
	Health     *HealthServer
	StartedAt  time.Time
	Version    string
	SocketPath string
	Error      error
}

// BootstrapConfig contains configuration for the gRPC bootstrap process
type BootstrapConfig struct {
	Config              config.GRPCConfig
	Logger              *logger.Logger
	SessionManager      *session.Manager
	EventBus            *events.Bus
	IPCBroker           *ipc.Broker
	ContainerRuntime    container.Runtime
	Version             string
	ShutdownTimeout     time.Duration
	EnableHealthCheck   bool
	HealthCheckInterval time.Duration
	// Optional interceptors to add to the gRPC server
	Interceptors []grpc.ServerOption
}

// NewDefaultBootstrapConfig creates a default gRPC bootstrap configuration
func NewDefaultBootstrapConfig(cfg config.GRPCConfig, log *logger.Logger) BootstrapConfig {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	return BootstrapConfig{
		Config:              cfg,
		Logger:              log,
		Version:             DefaultVersion,
		ShutdownTimeout:     BootstrapDefaultShutdownTimeout,
		EnableHealthCheck:   true,
		HealthCheckInterval: 30 * time.Second,
	}
}

// Bootstrap creates and initializes a new gRPC server with all services registered
// It performs the following steps:
// 1. Creates the gRPC server with UDS transport
// 2. Creates the health check server
// 3. Creates and registers all gRPC services (Orchestrator, Agent, Gateway, LLM, Tool)
// 4. Starts the gRPC server
// 5. Performs health checks (if enabled)
// 6. Returns the initialized gRPC server
func Bootstrap(ctx context.Context, cfg BootstrapConfig) (*BootstrapResult, error) {
	startedAt := time.Now()

	result := &BootstrapResult{
		StartedAt:  startedAt,
		Version:    cfg.Version,
		SocketPath: cfg.Config.SocketPath,
	}

	log := cfg.Logger
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			result.Error = types.WrapError(types.ErrCodeInternal, "failed to create logger", err)
			return result, result.Error
		}
	}

	// Validate dependencies
	if err := validateDependencies(cfg); err != nil {
		result.Error = types.WrapError(types.ErrCodeInvalidArgument, "invalid dependencies", err)
		return result, result.Error
	}

	// Create server config
	serverCfg := ServerConfig{
		Path:              cfg.Config.SocketPath,
		MaxRecvMsgSize:    cfg.Config.MaxRecvMsgSize,
		MaxSendMsgSize:    cfg.Config.MaxSendMsgSize,
		ConnectionTimeout: cfg.Config.Timeout,
		ShutdownTimeout:   cfg.ShutdownTimeout,
		Interceptors:      cfg.Interceptors,
	}

	// Create gRPC server
	server, err := NewServer(cfg.Config.SocketPath, serverCfg, log)
	if err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to create gRPC server", err)
		return result, result.Error
	}
	result.Server = server

	// Create health check server
	health, err := NewHealthServer(HealthServerConfig{}, log)
	if err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to create health server", err)
		// Cleanup on failure
		_ = server.Stop()
		return result, result.Error
	}
	result.Health = health

	// Register health check service
	grpc_health.RegisterHealthServer(server.GetServer(), health)

	// Create service dependencies
	deps := &serviceDependencies{
		sessionManager: cfg.SessionManager,
		eventBus:       cfg.EventBus,
		ipcBroker:      cfg.IPCBroker,
	}

	// Create tool registry and executor
	toolRegistry := tools.GetGlobalRegistry()
	_ = tools.RegisterBuiltinTools()

	// Create executor (requires container runtime)
	if cfg.ContainerRuntime != nil {
		executor, err := tools.NewExecutorFromRegistry(cfg.ContainerRuntime, log)
		if err != nil {
			result.Error = types.WrapError(types.ErrCodeInternal, "failed to create tool executor", err)
			// Cleanup on failure
			_ = server.Stop()
			_ = health.Shutdown()
			return result, result.Error
		}
		deps.executor = executor
		log.Info("Tool executor created", "runtime", cfg.ContainerRuntime.String())
	} else {
		log.Warn("Container runtime not provided, tool service will not execute tools")
	}

	// Create and register OrchestratorService
	orchSvc := NewOrchestratorService(deps, log)
	if err := server.RegisterService(&proto.OrchestratorService_ServiceDesc, orchSvc); err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to register orchestrator service", err)
		// Cleanup on failure
		_ = server.Stop()
		return result, result.Error
	}

	// Set health status for orchestrator service
	health.SetServingStatus("orchestrator", grpc_health.HealthCheckResponse_SERVING)

	// Create and register AgentService
	agentSvc := NewAgentService(deps, log)
	if err := server.RegisterService(&proto.AgentService_ServiceDesc, agentSvc); err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to register agent service", err)
		// Cleanup on failure
		_ = server.Stop()
		return result, result.Error
	}

	// Set health status for agent service
	health.SetServingStatus("agent", grpc_health.HealthCheckResponse_SERVING)

	// Create and register GatewayService
	gatewaySvc := NewGatewayService(deps, log)
	if err := server.RegisterService(&proto.GatewayService_ServiceDesc, gatewaySvc); err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to register gateway service", err)
		// Cleanup on failure
		_ = server.Stop()
		return result, result.Error
	}

	// Set health status for gateway service
	health.SetServingStatus("gateway", grpc_health.HealthCheckResponse_SERVING)

	// Create and register LLMService
	llmSvc := NewLLMService(deps, log)
	if err := server.RegisterService(&proto.LLMService_ServiceDesc, llmSvc); err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to register LLM service", err)
		// Cleanup on failure
		_ = server.Stop()
		return result, result.Error
	}

	// Set health status for LLM service
	health.SetServingStatus("llm", grpc_health.HealthCheckResponse_SERVING)

	// Create and register ToolService
	toolSvc := NewToolService(deps, log)
	if err := server.RegisterService(&proto.ToolService_ServiceDesc, toolSvc); err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to register tool service", err)
		// Cleanup on failure
		_ = server.Stop()
		return result, result.Error
	}

	// Set health status for tool service
	health.SetServingStatus("tool", grpc_health.HealthCheckResponse_SERVING)

	// Start the gRPC server
	if err := server.Start(ctx); err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to start gRPC server", err)
		// Cleanup on failure
		_ = server.Stop()
		return result, result.Error
	}

	// Perform health check if enabled
	if cfg.EnableHealthCheck {
		// Check that all services are serving
		statuses := health.GetAllStatuses()
		unhealthy := false
		for service, status := range statuses {
			if status != grpc_health.HealthCheckResponse_SERVING {
				log.Error("gRPC service health check failed",
					"service", service,
					"status", status.String())
				unhealthy = true
			}
		}

		if unhealthy {
			result.Error = types.NewError(types.ErrCodeInternal, "gRPC health check failed")
			_ = server.Stop()
			return result, result.Error
		}
	}

	log.Info("gRPC server bootstrapped successfully",
		"version", cfg.Version,
		"socket_path", cfg.Config.SocketPath,
		"duration", time.Since(startedAt))

	return result, nil
}

// BootstrapWithDefaults creates and initializes a new gRPC server with default configuration
func BootstrapWithDefaults(ctx context.Context, cfg config.GRPCConfig, sessionMgr *session.Manager, eventBus *events.Bus) (*BootstrapResult, error) {
	log, err := logger.NewDefault()
	if err != nil {
		return nil, err
	}

	bootstrapCfg := NewDefaultBootstrapConfig(cfg, log)
	bootstrapCfg.SessionManager = sessionMgr
	bootstrapCfg.EventBus = eventBus
	return Bootstrap(ctx, bootstrapCfg)
}

// IsReady checks if the gRPC server is ready to accept requests
func IsReady(ctx context.Context, server *Server) bool {
	if server == nil {
		return false
	}

	return server.IsServing()
}

// WaitForReady waits for the gRPC server to be ready with a timeout
func WaitForReady(ctx context.Context, server *Server, timeout time.Duration, checkInterval time.Duration) error {
	if server == nil {
		return types.NewError(types.ErrCodeInvalidArgument, "gRPC server is nil")
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
			return types.WrapError(types.ErrCodeTimeout, "gRPC server not ready within timeout", ctx.Err())
		case <-ticker.C:
			if IsReady(ctx, server) {
				return nil
			}
		}
	}
}

// GetVersion returns the version of the gRPC server
func GetVersion() string {
	return DefaultVersion
}

// GetVersionInfo returns detailed version information
func GetVersionInfo() map[string]interface{} {
	return map[string]interface{}{
		"version":    DefaultVersion,
		"build_time": "unknown",
		"git_commit": "unknown",
	}
}

// Global gRPC server instance
var (
	globalServer     *Server
	globalHealth     *HealthServer
	globalBootstrap  *BootstrapResult
	globalBootstrapMu sync.RWMutex
	globalOnce       sync.Once
)

// InitGlobal initializes the global gRPC server with the specified configuration
func InitGlobal(ctx context.Context, cfg BootstrapConfig) error {
	var initErr error
	globalOnce.Do(func() {
		result, err := Bootstrap(ctx, cfg)
		if err != nil {
			initErr = err
			return
		}
		globalBootstrapMu.Lock()
		globalServer = result.Server
		globalHealth = result.Health
		globalBootstrap = result
		globalBootstrapMu.Unlock()
	})
	return initErr
}

// InitGlobalWithDefaults initializes the global gRPC server with default configuration
func InitGlobalWithDefaults(ctx context.Context, grpcCfg config.GRPCConfig, sessionMgr *session.Manager, eventBus *events.Bus) error {
	log, err := logger.NewDefault()
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create logger", err)
	}

	cfg := NewDefaultBootstrapConfig(grpcCfg, log)
	cfg.SessionManager = sessionMgr
	cfg.EventBus = eventBus
	return InitGlobal(ctx, cfg)
}

// Global returns the global gRPC server instance
func Global() *Server {
	globalBootstrapMu.RLock()
	server := globalServer
	globalBootstrapMu.RUnlock()

	return server
}

// GlobalHealth returns the global health server instance
func GlobalHealth() *HealthServer {
	globalBootstrapMu.RLock()
	health := globalHealth
	globalBootstrapMu.RUnlock()

	return health
}

// GlobalResult returns the global bootstrap result
func GlobalResult() *BootstrapResult {
	globalBootstrapMu.RLock()
	result := globalBootstrap
	globalBootstrapMu.RUnlock()

	return result
}

// SetGlobal sets the global gRPC server instance
func SetGlobal(server *Server, health *HealthServer, result *BootstrapResult) {
	globalBootstrapMu.Lock()
	globalServer = server
	globalHealth = health
	globalBootstrap = result
	globalBootstrapMu.Unlock()
	globalOnce = sync.Once{}
}

// IsGlobalInitialized returns true if the global gRPC server has been initialized
func IsGlobalInitialized() bool {
	globalBootstrapMu.RLock()
	defer globalBootstrapMu.RUnlock()
	return globalServer != nil
}

// CloseGlobal closes and clears the global gRPC server instance
func CloseGlobal() error {
	globalBootstrapMu.Lock()
	defer globalBootstrapMu.Unlock()

	if globalServer == nil {
		return nil
	}

	// Close health server
	if globalHealth != nil {
		globalHealth.Shutdown()
	}

	// Stop gRPC server
	if err := globalServer.Stop(); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to close global gRPC server", err)
	}

	globalServer = nil
	globalHealth = nil
	globalBootstrap = nil
	globalOnce = sync.Once{}

	return nil
}

// String returns a string representation of the bootstrap result
func (r *BootstrapResult) String() string {
	if r.Error != nil {
		return fmt.Sprintf("GRPCBootstrapResult{version: %s, socket_path: %s, error: %v}",
			r.Version, r.SocketPath, r.Error)
	}
	return fmt.Sprintf("GRPCBootstrapResult{version: %s, socket_path: %s, started_at: %s, serving: %v}",
		r.Version, r.SocketPath, r.StartedAt.Format(time.RFC3339), r.Server != nil && r.Server.IsServing())
}

// IsSuccessful returns true if the bootstrap was successful
func (r *BootstrapResult) IsSuccessful() bool {
	return r.Error == nil && r.Server != nil && r.Server.IsServing()
}

// Duration returns the duration of the bootstrap process
func (r *BootstrapResult) Duration() time.Duration {
	return time.Since(r.StartedAt)
}

// serviceDependencies implements ServiceDependencies, GatewayServiceDependencies, and AgentServiceDependencies interfaces
type serviceDependencies struct {
	sessionManager *session.Manager
	eventBus       *events.Bus
	ipcBroker      *ipc.Broker
	executor       *tools.Executor
}

func (d *serviceDependencies) SessionManager() *session.Manager {
	return d.sessionManager
}

func (d *serviceDependencies) EventBus() *events.Bus {
	return d.eventBus
}

func (d *serviceDependencies) IPCBroker() *ipc.Broker {
	return d.ipcBroker
}

func (d *serviceDependencies) Executor() *tools.Executor {
	return d.executor
}

// validateDependencies validates that all required dependencies are provided
func validateDependencies(cfg BootstrapConfig) error {
	if cfg.SessionManager == nil {
		return types.NewError(types.ErrCodeInvalidArgument, "session manager is required")
	}
	if cfg.EventBus == nil {
		return types.NewError(types.ErrCodeInvalidArgument, "event bus is required")
	}
	// IPC Broker is optional for now
	return nil
}
