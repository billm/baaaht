package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/credentials"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/ipc"
	"github.com/billm/baaaht/orchestrator/pkg/memory"
	"github.com/billm/baaaht/orchestrator/pkg/policy"
	"github.com/billm/baaaht/orchestrator/pkg/scheduler"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Orchestrator manages all subsystems of the baaaht platform
// It is the central coordination point for container lifecycles, events,
// sessions, IPC, policy enforcement, credential management, and task scheduling.
type Orchestrator struct {
	mu     sync.RWMutex
	cfg    config.Config
	logger *logger.Logger
	closed bool

	// Core subsystems
	dockerClient   *container.Client
	runtime        container.Runtime
	sessionMgr     *session.Manager
	eventBus       *events.Bus
	eventRouter    *events.Router
	ipcBroker      *ipc.Broker
	policyEnforcer *policy.Enforcer
	policyReloader *policy.Reloader
	credStore      *credentials.Store
	scheduler      *scheduler.Scheduler
	memoryStore    *memory.Store
	memoryExtractor *memory.Extractor
	memoryHandler  *memory.SessionArchivalHandler
	llmGateway     *LLMGatewayManager
	llmCredentialManager *credentials.LLMCredentialManager

	// Lifecycle management
	started        bool
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	wg             sync.WaitGroup
}

// New creates a new Orchestrator with the specified configuration
func New(cfg config.Config, log *logger.Logger) (*Orchestrator, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalidArgument, "invalid configuration", err)
	}

	// Create shutdown context
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	o := &Orchestrator{
		cfg:            cfg,
		logger:         log.With("component", "orchestrator"),
		closed:         false,
		started:        false,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}

	o.logger.Info("Orchestrator created",
		"docker_host", cfg.Docker.Host,
		"max_sessions", cfg.Session.MaxSessions,
		"scheduler_workers", cfg.Scheduler.Workers)

	return o, nil
}

// NewDefault creates a new Orchestrator with default configuration
func NewDefault(log *logger.Logger) (*Orchestrator, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to load configuration", err)
	}
	return New(*cfg, log)
}

// Initialize initializes all subsystems in dependency order
// Subsystems are initialized in the following order:
// 1. Logger (already created)
// 2. Docker Client
// 3. Event Bus
// 4. Event Router
// 5. Session Manager
// 6. IPC Broker
// 7. Policy Enforcer
// 8. Credential Store
// 9. Scheduler
// 10. Memory System
// 11. LLM Gateway (optional)
func (o *Orchestrator) Initialize(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return types.NewError(types.ErrCodeUnavailable, "orchestrator is closed")
	}

	if o.started {
		return types.NewError(types.ErrCodeFailedPrecondition, "orchestrator already initialized")
	}

	o.logger.Info("Initializing orchestrator subsystems")

	// 1. Initialize Docker client
	if err := o.initDockerClient(ctx); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to initialize Docker client", err)
	}

	// 2. Initialize event bus
	if err := o.initEventBus(ctx); err != nil {
		// Cleanup previously initialized subsystems
		_ = o.cleanupDockerClient()
		return types.WrapError(types.ErrCodeInternal, "failed to initialize event bus", err)
	}

	// 3. Initialize event router
	if err := o.initEventRouter(ctx); err != nil {
		// Cleanup previously initialized subsystems
		_ = o.cleanupEventBus()
		_ = o.cleanupDockerClient()
		return types.WrapError(types.ErrCodeInternal, "failed to initialize event router", err)
	}

	// 4. Initialize session manager
	if err := o.initSessionManager(ctx); err != nil {
		// Cleanup previously initialized subsystems
		_ = o.cleanupEventRouter()
		_ = o.cleanupEventBus()
		_ = o.cleanupDockerClient()
		return types.WrapError(types.ErrCodeInternal, "failed to initialize session manager", err)
	}

	// 5. Initialize IPC broker
	if err := o.initIPCBroker(ctx); err != nil {
		// Cleanup previously initialized subsystems
		_ = o.cleanupSessionManager()
		_ = o.cleanupEventRouter()
		_ = o.cleanupEventBus()
		_ = o.cleanupDockerClient()
		return types.WrapError(types.ErrCodeInternal, "failed to initialize IPC broker", err)
	}

	// 6. Initialize policy enforcer
	if err := o.initPolicyEnforcer(ctx); err != nil {
		// Cleanup previously initialized subsystems
		_ = o.cleanupIPCBroker()
		_ = o.cleanupSessionManager()
		_ = o.cleanupEventRouter()
		_ = o.cleanupEventBus()
		_ = o.cleanupDockerClient()
		return types.WrapError(types.ErrCodeInternal, "failed to initialize policy enforcer", err)
	}

	// 7. Initialize credential store
	if err := o.initCredentialStore(ctx); err != nil {
		// Cleanup previously initialized subsystems
		_ = o.cleanupPolicyEnforcer()
		_ = o.cleanupIPCBroker()
		_ = o.cleanupSessionManager()
		_ = o.cleanupEventRouter()
		_ = o.cleanupEventBus()
		_ = o.cleanupDockerClient()
		return types.WrapError(types.ErrCodeInternal, "failed to initialize credential store", err)
	}

	// 8. Initialize scheduler
	if err := o.initScheduler(ctx); err != nil {
		// Cleanup previously initialized subsystems
		_ = o.cleanupCredentialStore()
		_ = o.cleanupPolicyEnforcer()
		_ = o.cleanupIPCBroker()
		_ = o.cleanupSessionManager()
		_ = o.cleanupEventRouter()
		_ = o.cleanupEventBus()
		_ = o.cleanupDockerClient()
		return types.WrapError(types.ErrCodeInternal, "failed to initialize scheduler", err)
	}

	// 9. Initialize memory system
	if err := o.initMemorySystem(ctx); err != nil {
		// Cleanup previously initialized subsystems
		_ = o.cleanupScheduler()
		_ = o.cleanupCredentialStore()
		_ = o.cleanupPolicyEnforcer()
		_ = o.cleanupIPCBroker()
		_ = o.cleanupSessionManager()
		_ = o.cleanupEventRouter()
		_ = o.cleanupEventBus()
		_ = o.cleanupDockerClient()
		return types.WrapError(types.ErrCodeInternal, "failed to initialize memory system", err)
	}

	// 10. Initialize LLM Gateway (optional, non-critical)
	if err := o.initLLMGateway(ctx); err != nil {
		// LLM Gateway initialization failures are non-critical
		// Log a warning and continue with LLM disabled
		o.logger.Warn("LLM Gateway initialization failed, continuing with LLM disabled",
			"error", err)
	}

	o.started = true
	o.logger.Info("Orchestrator subsystems initialized successfully")

	return nil
}

// initDockerClient initializes the Docker client and runtime
func (o *Orchestrator) initDockerClient(ctx context.Context) error {
	o.logger.Debug("Initializing Docker client and runtime", "host", o.cfg.Docker.Host)

	// Create Docker runtime (which includes the client)
	runtime, err := container.NewDockerRuntime(o.cfg.Docker, o.logger)
	if err != nil {
		return err
	}

	o.runtime = runtime
	o.dockerClient = runtime.DockerClient()
	o.logger.Info("Docker client and runtime initialized")
	return nil
}

// initEventBus initializes the event bus
func (o *Orchestrator) initEventBus(ctx context.Context) error {
	o.logger.Debug("Initializing event bus")

	bus, err := events.New(o.logger)
	if err != nil {
		return err
	}

	o.eventBus = bus
	o.logger.Info("Event bus initialized")
	return nil
}

// initEventRouter initializes the event router
func (o *Orchestrator) initEventRouter(ctx context.Context) error {
	o.logger.Debug("Initializing event router")

	router, err := events.NewRouter(o.logger)
	if err != nil {
		return err
	}

	o.eventRouter = router
	o.logger.Info("Event router initialized")
	return nil
}

// initSessionManager initializes the session manager
func (o *Orchestrator) initSessionManager(ctx context.Context) error {
	o.logger.Debug("Initializing session manager")

	mgr, err := session.New(o.cfg.Session, o.logger)
	if err != nil {
		return err
	}

	o.sessionMgr = mgr
	o.logger.Info("Session manager initialized")
	return nil
}

// initIPCBroker initializes the IPC broker
func (o *Orchestrator) initIPCBroker(ctx context.Context) error {
	o.logger.Debug("Initializing IPC broker")

	broker, err := ipc.New(o.cfg.IPC, o.logger)
	if err != nil {
		return err
	}

	o.ipcBroker = broker
	o.logger.Info("IPC broker initialized")
	return nil
}

// initPolicyEnforcer initializes the policy enforcer
func (o *Orchestrator) initPolicyEnforcer(ctx context.Context) error {
	o.logger.Debug("Initializing policy enforcer")

	enforcer, err := policy.New(o.cfg.Policy, o.logger)
	if err != nil {
		return err
	}

	// Load policy from YAML if ConfigPath is set
	if o.cfg.Policy.ConfigPath != "" {
		o.logger.Debug("Loading policy from file", "path", o.cfg.Policy.ConfigPath)
		pol, err := policy.LoadFromFile(o.cfg.Policy.ConfigPath)
		if err != nil {
			// Close the enforcer before returning error
			_ = enforcer.Close()
			return types.WrapError(types.ErrCodeInvalidArgument, "failed to load policy from "+o.cfg.Policy.ConfigPath, err)
		}

		// Set the loaded policy on the enforcer
		if err := enforcer.SetPolicy(ctx, pol); err != nil {
			// Close the enforcer before returning error
			_ = enforcer.Close()
			return types.WrapError(types.ErrCodeInternal, "failed to set policy", err)
		}

		o.logger.Info("Policy enforcer initialized with policy from file",
			"policy_id", pol.ID,
			"name", pol.Name,
			"mode", pol.Mode,
			"path", o.cfg.Policy.ConfigPath)

		// Initialize policy reloader if ReloadOnChanges is enabled
		if o.cfg.Policy.ReloadOnChanges {
			o.logger.Info("Policy hot-reload enabled",
				"policy_path", o.cfg.Policy.ConfigPath)

			reloader := policy.NewReloader(o.cfg.Policy.ConfigPath, pol, o.logger)

			// Add callback to update the enforcer when policy is reloaded
			reloader.AddCallback(func(reloadCtx context.Context, newPolicy *policy.Policy) error {
				o.logger.Info("Reloading policy",
					"policy_id", newPolicy.ID,
					"name", newPolicy.Name,
					"mode", newPolicy.Mode)
				return enforcer.SetPolicy(reloadCtx, newPolicy)
			})

			// Start the reloader
			reloader.Start()
			o.policyReloader = reloader

			o.logger.Info("Policy reloader started",
				"policy_path", o.cfg.Policy.ConfigPath)
		}
	} else {
		o.logger.Info("Policy enforcer initialized with default policy")
	}

	o.policyEnforcer = enforcer
	return nil
}

// initCredentialStore initializes the credential store
func (o *Orchestrator) initCredentialStore(ctx context.Context) error {
	o.logger.Debug("Initializing credential store")

	store, err := credentials.NewStore(o.cfg.Credentials, o.logger)
	if err != nil {
		return err
	}

	o.credStore = store
	o.logger.Info("Credential store initialized")
	return nil
}

// initScheduler initializes the scheduler
func (o *Orchestrator) initScheduler(ctx context.Context) error {
	o.logger.Debug("Initializing scheduler")

	sched, err := scheduler.New(o.cfg.Scheduler, o.logger)
	if err != nil {
		return err
	}

	o.scheduler = sched
	o.logger.Info("Scheduler initialized")
	return nil
}

// initMemorySystem initializes the memory system and registers the handler
func (o *Orchestrator) initMemorySystem(ctx context.Context) error {
	o.logger.Debug("Initializing memory system")

	// Create memory store
	store, err := memory.NewStore(o.cfg.Memory, o.logger)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create memory store", err)
	}
	o.memoryStore = store

	// Create memory extractor
	extractor, err := memory.NewDefaultExtractor(store, o.logger)
	if err != nil {
		_ = store.Close()
		return types.WrapError(types.ErrCodeInternal, "failed to create memory extractor", err)
	}
	o.memoryExtractor = extractor

	// Create session archival handler
	handler, err := memory.NewSessionArchivalHandler(extractor, store, o.logger)
	if err != nil {
		_ = store.Close()
		return types.WrapError(types.ErrCodeInternal, "failed to create session archival handler", err)
	}
	o.memoryHandler = handler

	// Register handler with event router
	_, err = o.eventRouter.AddRoute(ctx, string(types.EventTypeSessionArchived), handler)
	if err != nil {
		_ = handler.Close()
		_ = store.Close()
		return types.WrapError(types.ErrCodeInternal, "failed to register memory handler", err)
	}

	o.logger.Info("Memory system initialized",
		"enabled", o.cfg.Memory.Enabled,
		"user_path", o.cfg.Memory.UserMemoryPath,
		"group_path", o.cfg.Memory.GroupMemoryPath)

	return nil
}

// initLLMGateway initializes the LLM Gateway manager if LLM is enabled in configuration.
// This is a non-critical initialization - failures will log a warning but not prevent
// the orchestrator from starting. The LLM Gateway provides isolated API credential
// management for LLM providers.
func (o *Orchestrator) initLLMGateway(ctx context.Context) error {
	// Check if LLM is enabled in configuration
	if !o.cfg.LLM.Enabled {
		o.logger.Info("LLM Gateway is disabled in configuration")
		return nil
	}

	o.logger.Debug("Initializing LLM Gateway",
		"container_image", o.cfg.LLM.ContainerImage,
		"default_provider", o.cfg.LLM.DefaultProvider,
		"default_model", o.cfg.LLM.DefaultModel)

	// Create LLM credential manager
	o.llmCredentialManager = credentials.NewLLMCredentialManager(
		o.credStore,
		o.cfg.LLM,
		o.logger,
	)

	// Validate that LLM credentials exist for enabled providers
	if err := o.llmCredentialManager.ValidateLLMCredentials(); err != nil {
		o.logger.Warn("LLM credential validation failed, LLM Gateway will not be started",
			"error", err)
		// Don't fail - continue with LLM disabled
		return nil
	}

	// Create LLM Gateway manager
	llmGatewayMgr, err := NewLLMGatewayManager(
		o.runtime,
		o.cfg.LLM,
		o.credStore,
		o.logger,
	)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create LLM Gateway manager", err)
	}

	o.llmGateway = llmGatewayMgr

	// Start the LLM Gateway container
	if err := o.llmGateway.Start(ctx); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to start LLM Gateway", err)
	}

	o.logger.Info("LLM Gateway initialized successfully",
		"container_id", o.llmGateway.GetContainerID(),
		"enabled", true)

	return nil
}

// Close gracefully shuts down the orchestrator and all subsystems
// Subsystems are closed in reverse dependency order
func (o *Orchestrator) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return nil
	}

	if o.logger != nil {
		o.logger.Info("Closing orchestrator")
	}

	// Signal shutdown
	if o.shutdownCancel != nil {
		o.shutdownCancel()
	}

	// Close subsystems in reverse order
	if o.memoryHandler != nil {
		if err := o.memoryHandler.Close(); err != nil {
			o.logger.Error("Failed to close memory handler", "error", err)
		}
	}

	if o.memoryExtractor != nil {
		if err := o.memoryExtractor.Close(); err != nil {
			o.logger.Error("Failed to close memory extractor", "error", err)
		}
	}
	if o.memoryStore != nil {
		if err := o.memoryStore.Close(); err != nil {
			o.logger.Error("Failed to close memory store", "error", err)
		}
	}

	if o.llmGateway != nil {
		if err := o.llmGateway.Close(); err != nil {
			o.logger.Error("Failed to close LLM Gateway", "error", err)
		}
	}

	if o.scheduler != nil {
		if err := o.scheduler.Close(); err != nil {
			o.logger.Error("Failed to close scheduler", "error", err)
		}
	}

	if o.credStore != nil {
		if err := o.credStore.Close(); err != nil {
			o.logger.Error("Failed to close credential store", "error", err)
		}
	}

	if o.policyReloader != nil {
		o.policyReloader.Stop()
	}

	if o.policyEnforcer != nil {
		if err := o.policyEnforcer.Close(); err != nil {
			o.logger.Error("Failed to close policy enforcer", "error", err)
		}
	}

	if o.ipcBroker != nil {
		if err := o.ipcBroker.Close(); err != nil {
			o.logger.Error("Failed to close IPC broker", "error", err)
		}
	}

	if o.sessionMgr != nil {
		if err := o.sessionMgr.Close(); err != nil {
			o.logger.Error("Failed to close session manager", "error", err)
		}
	}

	if o.eventRouter != nil {
		if err := o.eventRouter.Close(); err != nil {
			o.logger.Error("Failed to close event router", "error", err)
		}
	}

	if o.eventBus != nil {
		if err := o.eventBus.Close(); err != nil {
			o.logger.Error("Failed to close event bus", "error", err)
		}
	}

	if o.runtime != nil {
		if err := o.runtime.Close(); err != nil {
			o.logger.Error("Failed to close container runtime", "error", err)
		}
	}

	o.closed = true
	o.started = false
	if o.logger != nil {
		o.logger.Info("Orchestrator closed")
	}

	return nil
}

// IsStarted returns true if the orchestrator has been initialized
func (o *Orchestrator) IsStarted() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.started
}

// IsClosed returns true if the orchestrator has been closed
func (o *Orchestrator) IsClosed() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.closed
}

// Config returns the orchestrator configuration
func (o *Orchestrator) Config() config.Config {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.cfg
}

// UpdateConfig updates the orchestrator's configuration
// This is typically called when configuration is reloaded via SIGHUP
func (o *Orchestrator) UpdateConfig(cfg config.Config) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return types.NewError(types.ErrCodeUnavailable, "orchestrator is closed")
	}

	// Validate the new configuration
	if err := cfg.Validate(); err != nil {
		return types.WrapError(types.ErrCodeInvalidArgument, "invalid configuration", err)
	}

	oldCfg := o.cfg
	o.cfg = cfg

	o.logger.Info("Configuration updated",
		"docker_host", cfg.Docker.Host,
		"max_sessions", cfg.Session.MaxSessions,
		"scheduler_workers", cfg.Scheduler.Workers)

	// TODO: Notify subsystems of configuration changes
	// For now, subsystems will use the new config on next operation
	// that reads from o.cfg

	_ = oldCfg // Avoid unused variable warning
	return nil
}

// DockerClient returns the Docker client
func (o *Orchestrator) DockerClient() *container.Client {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.dockerClient
}

// SessionManager returns the session manager
func (o *Orchestrator) SessionManager() *session.Manager {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.sessionMgr
}

// EventBus returns the event bus
func (o *Orchestrator) EventBus() *events.Bus {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.eventBus
}

// EventRouter returns the event router
func (o *Orchestrator) EventRouter() *events.Router {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.eventRouter
}

// IPCBroker returns the IPC broker
func (o *Orchestrator) IPCBroker() *ipc.Broker {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.ipcBroker
}

// PolicyEnforcer returns the policy enforcer
func (o *Orchestrator) PolicyEnforcer() *policy.Enforcer {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.policyEnforcer
}

// PolicyReloader returns the policy reloader
func (o *Orchestrator) PolicyReloader() *policy.Reloader {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.policyReloader
}

// CredentialStore returns the credential store
func (o *Orchestrator) CredentialStore() *credentials.Store {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.credStore
}

// Scheduler returns the task scheduler
func (o *Orchestrator) Scheduler() *scheduler.Scheduler {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.scheduler
}

// MemoryStore returns the memory store
func (o *Orchestrator) MemoryStore() *memory.Store {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.memoryStore
}

// MemoryExtractor returns the memory extractor
func (o *Orchestrator) MemoryExtractor() *memory.Extractor {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.memoryExtractor
}

// MemoryHandler returns the memory handler
func (o *Orchestrator) MemoryHandler() *memory.SessionArchivalHandler {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.memoryHandler
}

// LLMGateway returns the LLM Gateway manager
func (o *Orchestrator) LLMGateway() *LLMGatewayManager {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.llmGateway
}

// LLMCredentialManager returns the LLM credential manager
func (o *Orchestrator) LLMCredentialManager() *credentials.LLMCredentialManager {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.llmCredentialManager
}

// ShutdownContext returns the shutdown context for cancellation
func (o *Orchestrator) ShutdownContext() context.Context {
	return o.shutdownCtx
}

// HealthCheck returns the health status of all subsystems
func (o *Orchestrator) HealthCheck(ctx context.Context) map[string]types.Health {
	o.mu.RLock()
	defer o.mu.RUnlock()

	health := make(map[string]types.Health)

	// Check Docker client
	if o.dockerClient != nil {
		if err := o.dockerClient.Ping(ctx); err != nil {
			health["docker"] = types.Unhealthy
		} else {
			health["docker"] = types.Healthy
		}
	} else {
		health["docker"] = types.Unhealthy
	}

	// Check session manager
	if o.sessionMgr != nil && !o.closed {
		health["session"] = types.Healthy
	} else {
		health["session"] = types.Unhealthy
	}

	// Check event bus
	if o.eventBus != nil && !o.closed {
		health["event_bus"] = types.Healthy
	} else {
		health["event_bus"] = types.Unhealthy
	}

	// Check event router
	if o.eventRouter != nil && !o.closed {
		health["event_router"] = types.Healthy
	} else {
		health["event_router"] = types.Unhealthy
	}

	// Check IPC broker
	if o.ipcBroker != nil && !o.closed {
		health["ipc"] = types.Healthy
	} else {
		health["ipc"] = types.Unhealthy
	}

	// Check policy enforcer
	if o.policyEnforcer != nil && !o.closed {
		health["policy"] = types.Healthy
	} else {
		health["policy"] = types.Unhealthy
	}

	// Check credential store
	if o.credStore != nil && !o.closed {
		health["credentials"] = types.Healthy
	} else {
		health["credentials"] = types.Unhealthy
	}

	// Check scheduler
	if o.scheduler != nil && !o.closed {
		health["scheduler"] = types.Healthy
	} else {
		health["scheduler"] = types.Unhealthy
	}

	// Check memory system
	if o.memoryStore != nil && !o.closed {
		health["memory"] = types.Healthy
	} else {
		health["memory"] = types.Unhealthy
	}

	// Check LLM Gateway
	if o.llmGateway != nil && o.llmGateway.IsEnabled() {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		result, err := o.llmGateway.HealthCheck(ctx)
		if err != nil {
			health["llm_gateway"] = types.Unhealthy
		} else {
			health["llm_gateway"] = result.Status
		}
	} else {
		health["llm_gateway"] = types.Healthy // Disabled is healthy
	}

	return health
}

// Stats returns statistics for all subsystems
func (o *Orchestrator) Stats(ctx context.Context) map[string]interface{} {
	o.mu.RLock()
	defer o.mu.RUnlock()

	stats := make(map[string]interface{})

	// Session manager stats
	if o.sessionMgr != nil {
		active, idle, closing, closed := o.sessionMgr.Stats(ctx)
		stats["sessions"] = map[string]int{
			"active":  active,
			"idle":    idle,
			"closing": closing,
			"closed":  closed,
		}
	}

	// Event bus stats
	if o.eventBus != nil {
		busStats := o.eventBus.Stats()
		stats["event_bus"] = map[string]int{
			"total_subscriptions":  busStats.TotalSubscriptions,
			"active_subscriptions": busStats.ActiveSubscriptions,
			"pending_events":       busStats.PendingEvents,
		}
	}

	// Event router stats
	if o.eventRouter != nil {
		routerStats := o.eventRouter.Stats()
		stats["event_router"] = map[string]int{
			"total_routes":    routerStats.TotalRoutes,
			"exact_routes":    routerStats.ExactRoutes,
			"wildcard_routes": routerStats.WildcardRoutes,
			"active_routes":   routerStats.ActiveRoutes,
			"inactive_routes": routerStats.InactiveRoutes,
		}
	}

	// IPC broker stats
	if o.ipcBroker != nil {
		brokerStats := o.ipcBroker.Stats()
		stats["ipc"] = map[string]int{
			"messages_sent":     int(brokerStats.MessagesSent),
			"messages_received": int(brokerStats.MessagesReceived),
			"messages_failed":   int(brokerStats.MessagesFailed),
			"active_handlers":   brokerStats.ActiveHandlers,
		}
	}

	// Scheduler stats
	if o.scheduler != nil {
		schedStats := o.scheduler.Stats()
		stats["scheduler"] = schedStats
	}

	// LLM Gateway stats
	if o.llmGateway != nil {
		stats["llm_gateway"] = map[string]interface{}{
			"enabled":       o.llmGateway.IsEnabled(),
			"started":       o.llmGateway.IsStarted(),
			"container_id":  o.llmGateway.GetContainerID(),
			"container_image": o.cfg.LLM.ContainerImage,
			"default_provider": o.cfg.LLM.DefaultProvider,
			"default_model":    o.cfg.LLM.DefaultModel,
		}
	}

	return stats
}

// String returns a string representation of the orchestrator
func (o *Orchestrator) String() string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	started := "no"
	if o.started {
		started = "yes"
	}

	closed := "no"
	if o.closed {
		closed = "yes"
	}

	return fmt.Sprintf("Orchestrator{started: %s, closed: %s, docker: %v, sessions: %v, scheduler: %v, llm_gateway: %v}",
		started, closed,
		o.dockerClient != nil,
		o.sessionMgr != nil,
		o.scheduler != nil,
		o.llmGateway != nil)
}

// Helper cleanup methods (called during initialization failures)

func (o *Orchestrator) cleanupDockerClient() error {
	if o.runtime != nil {
		return o.runtime.Close()
	}
	return nil
}

func (o *Orchestrator) cleanupEventBus() error {
	if o.eventBus != nil {
		return o.eventBus.Close()
	}
	return nil
}

func (o *Orchestrator) cleanupEventRouter() error {
	if o.eventRouter != nil {
		return o.eventRouter.Close()
	}
	return nil
}

func (o *Orchestrator) cleanupSessionManager() error {
	if o.sessionMgr != nil {
		return o.sessionMgr.Close()
	}
	return nil
}

func (o *Orchestrator) cleanupIPCBroker() error {
	if o.ipcBroker != nil {
		return o.ipcBroker.Close()
	}
	return nil
}

func (o *Orchestrator) cleanupPolicyEnforcer() error {
	if o.policyReloader != nil {
		o.policyReloader.Stop()
	}
	if o.policyEnforcer != nil {
		return o.policyEnforcer.Close()
	}
	return nil
}

func (o *Orchestrator) cleanupCredentialStore() error {
	if o.credStore != nil {
		return o.credStore.Close()
	}
	return nil
}

func (o *Orchestrator) cleanupScheduler() error {
	if o.scheduler != nil {
		return o.scheduler.Close()
	}
	return nil
}

func (o *Orchestrator) cleanupMemorySystem() error {
	if o.memoryHandler != nil {
		_ = o.memoryHandler.Close()
	}
	if o.memoryStore != nil {
		return o.memoryStore.Close()
	}
	return nil
}

func (o *Orchestrator) cleanupLLMGateway() error {
	if o.llmGateway != nil {
		return o.llmGateway.Close()
	}
	return nil
}
