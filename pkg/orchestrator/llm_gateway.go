package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/credentials"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// LLMGatewayManager manages the lifecycle of the LLM Gateway container.
// The LLM Gateway is responsible for isolating LLM API credentials from agent
// containers, providing a unified interface for LLM requests, and tracking
// token usage for cost monitoring.
type LLMGatewayManager struct {
	runtime   container.Runtime
	config    config.LLMConfig
	credStore *credentials.Store
	logger    *logger.Logger
	mu        sync.RWMutex

	// Container state
	containerID string
	started     bool
	closed      bool

	// Lifecycle managers
	lifecycle *container.LifecycleManager
	monitor   *container.Monitor
	creator   *container.Creator

	// Health monitoring
	healthCheckInterval    time.Duration
	healthCheckCtx         context.Context
	healthCheckCancel      context.CancelFunc
	healthCheckDone        chan struct{}
	consecutiveFailures    int
	maxConsecutiveFailures int
}

// NewLLMGatewayManager creates a new LLM Gateway manager.
// The manager is responsible for creating, starting, stopping, and monitoring
// the LLM Gateway container.
func NewLLMGatewayManager(
	runtime container.Runtime,
	cfg config.LLMConfig,
	credStore *credentials.Store,
	log *logger.Logger,
) (*LLMGatewayManager, error) {
	if runtime == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime cannot be nil")
	}
	if credStore == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "credential store cannot be nil")
	}
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Create lifecycle managers
	lifecycle, err := container.NewLifecycleManagerFromRuntime(runtime, log)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create lifecycle manager", err)
	}

	monitor, err := container.NewMonitorFromRuntime(runtime, log)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create monitor", err)
	}

	creator, err := container.NewCreatorFromRuntime(runtime, log)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create creator", err)
	}

	// Health check context for cancellation
	healthCtx, healthCancel := context.WithCancel(context.Background())

	return &LLMGatewayManager{
		runtime:                runtime,
		config:                 cfg,
		credStore:              credStore,
		logger:                 log.With("component", "llm_gateway_manager"),
		lifecycle:              lifecycle,
		monitor:                monitor,
		creator:                creator,
		started:                false,
		closed:                 false,
		healthCheckInterval:    30 * time.Second,
		healthCheckCtx:         healthCtx,
		healthCheckCancel:      healthCancel,
		healthCheckDone:        make(chan struct{}),
		consecutiveFailures:    0,
		maxConsecutiveFailures: 3,
	}, nil
}

// Start creates and starts the LLM Gateway container.
// The container is configured with:
// - Egress-only network access
// - API credentials injected via environment variables
// - Resource limits (CPU, memory)
// - Health check endpoint
func (m *LLMGatewayManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return types.NewError(types.ErrCodeUnavailable, "LLM Gateway manager is closed")
	}

	if m.started {
		return types.NewError(types.ErrCodeFailedPrecondition, "LLM Gateway is already started")
	}

	if !m.config.Enabled {
		m.logger.Info("LLM Gateway is disabled in configuration")
		return nil
	}

	m.logger.Info("Starting LLM Gateway",
		"image", m.config.ContainerImage,
		"default_provider", m.config.DefaultProvider,
		"default_model", m.config.DefaultModel)

	// Validate credentials exist before starting
	if err := m.validateCredentials(ctx); err != nil {
		return types.WrapError(types.ErrCodeInvalidArgument, "credential validation failed", err)
	}

	// Create the container with proper security configuration
	containerID, err := m.createGatewayContainer(ctx)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create LLM Gateway container", err)
	}

	m.containerID = containerID

	// Start the container
	startCfg := container.StartConfig{
		ContainerID: m.containerID,
		Name:        "baaaht-llm-gateway",
	}
	if err := m.lifecycle.Start(ctx, startCfg); err != nil {
		// Clean up the container if start fails
		_ = m.cleanupContainer(ctx)
		return types.WrapError(types.ErrCodeInternal, "failed to start LLM Gateway container", err)
	}

	// Wait for the container to become healthy
	healthCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result, err := m.monitor.HealthCheckWithRetry(healthCtx, m.containerID, 30, 2*time.Second)
	if err != nil {
		m.logger.Warn("LLM Gateway health check failed, container may not be fully ready",
			"error", err)
		// Don't fail startup if health check fails, container may still be starting
	} else {
		m.logger.Info("LLM Gateway is healthy",
			"status", result.Status,
			"container_id", m.containerID)
	}

	m.started = true
	m.logger.Info("LLM Gateway started successfully",
		"container_id", m.containerID)

	// Recreate health monitoring context and channel for this start
	m.healthCheckCtx, m.healthCheckCancel = context.WithCancel(context.Background())
	m.healthCheckDone = make(chan struct{})

	// Start background health monitoring
	go m.healthMonitorLoop()

	return nil
}

// Stop gracefully shuts down the LLM Gateway container.
// The container is sent a SIGTERM signal and given time to clean up.
func (m *LLMGatewayManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return types.NewError(types.ErrCodeFailedPrecondition, "LLM Gateway is not started")
	}

	m.logger.Info("Stopping LLM Gateway", "container_id", m.containerID)

	// Stop health monitoring goroutine
	m.healthCheckCancel()
	select {
	case <-m.healthCheckDone:
		m.logger.Debug("Health monitoring stopped")
	case <-time.After(5 * time.Second):
		m.logger.Warn("Health monitoring did not stop gracefully")
	case <-ctx.Done():
		m.logger.Warn("Health monitoring stop canceled")
	}

	timeout := 30 * time.Second
	stopCfg := container.StopConfig{
		ContainerID: m.containerID,
		Name:        "baaaht-llm-gateway",
		Timeout:     &timeout,
	}

	if err := m.lifecycle.Stop(ctx, stopCfg); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to stop LLM Gateway container", err)
	}

	m.started = false
	m.consecutiveFailures = 0
	m.logger.Info("LLM Gateway stopped successfully")

	return nil
}

// HealthCheck performs a health check on the LLM Gateway container.
// Returns the current health status of the container.
func (m *LLMGatewayManager) HealthCheck(ctx context.Context) (*container.HealthCheckResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started || m.containerID == "" {
		return &container.HealthCheckResult{
			Status: types.Unhealthy,
		}, nil
	}

	return m.monitor.HealthCheck(ctx, m.containerID)
}

// Restart restarts the LLM Gateway container.
// This is useful for recovering from transient failures or applying configuration changes.
func (m *LLMGatewayManager) Restart(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return types.NewError(types.ErrCodeFailedPrecondition, "LLM Gateway is not started")
	}

	m.logger.Info("Restarting LLM Gateway", "container_id", m.containerID)

	timeout := 30 * time.Second
	restartCfg := container.RestartConfig{
		ContainerID: m.containerID,
		Name:        "baaaht-llm-gateway",
		Timeout:     &timeout,
	}

	if err := m.lifecycle.Restart(ctx, restartCfg); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to restart LLM Gateway container", err)
	}

	// Wait for the container to become healthy again
	healthCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result, err := m.monitor.HealthCheckWithRetry(healthCtx, m.containerID, 30, 2*time.Second)
	if err != nil {
		m.logger.Warn("LLM Gateway health check after restart failed",
			"error", err)
	} else {
		m.logger.Info("LLM Gateway is healthy after restart",
			"status", result.Status)
	}

	m.logger.Info("LLM Gateway restarted successfully")

	return nil
}

// IsRunning returns true if the LLM Gateway container is currently running.
func (m *LLMGatewayManager) IsRunning(ctx context.Context) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started || m.containerID == "" {
		return false, nil
	}

	running, err := m.lifecycle.IsRunning(ctx, m.containerID)
	if err != nil {
		return false, types.WrapError(types.ErrCodeInternal, "failed to check container status", err)
	}

	return running, nil
}

// GetContainerID returns the container ID of the LLM Gateway.
// Returns empty string if the gateway is not started.
func (m *LLMGatewayManager) GetContainerID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.containerID
}

// IsStarted returns true if the LLM Gateway has been started.
func (m *LLMGatewayManager) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

// IsEnabled returns true if the LLM Gateway is enabled in configuration.
func (m *LLMGatewayManager) IsEnabled() bool {
	return m.config.Enabled
}

// Close shuts down the LLM Gateway manager and cleans up resources.
// This method should be called when the manager is no longer needed.
func (m *LLMGatewayManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.logger.Info("Closing LLM Gateway manager")

	// Stop health monitoring
	m.healthCheckCancel()
	select {
	case <-m.healthCheckDone:
		m.logger.Debug("Health monitoring stopped during close")
	case <-time.After(2 * time.Second):
		m.logger.Warn("Health monitoring did not stop gracefully during close")
	}

	// Stop and destroy the container if it's running
	if m.containerID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt graceful shutdown first
		if m.started {
			if err := m.stopInternal(ctx); err != nil {
				m.logger.Error("Failed to stop LLM Gateway during close",
					"error", err)
			}
		}

		// Always attempt to clean up the container, even if stop failed
		if err := m.cleanupContainer(ctx); err != nil {
			m.logger.Warn("Failed to cleanup LLM Gateway container during close",
				"error", err)
		}
	}

	m.closed = true
	m.started = false
	m.containerID = ""
	m.consecutiveFailures = 0

	m.logger.Info("LLM Gateway manager closed")

	return nil
}

// validateCredentials checks that required LLM provider credentials exist.
func (m *LLMGatewayManager) validateCredentials(ctx context.Context) error {
	// Check that the default provider has credentials configured
	if m.config.DefaultProvider == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "default provider is not configured")
	}

	// Verify at least one enabled provider is properly configured
	// (either has credentials in store OR doesn't require credentials)
	hasValidProvider := false
	for name, provider := range m.config.Providers {
		if !provider.Enabled {
			continue
		}

		// Check if this provider requires credentials from the store
		envVar, requiresKey := credentials.GetCredentialEnvVar(name)
		if !requiresKey || envVar == "" {
			// Provider doesn't require credentials from store (e.g., local LLMs like lmstudio/ollama)
			hasValidProvider = true
			m.logger.Debug("Provider doesn't require credential store",
				"provider", name)
			break
		}

		// Provider requires credentials, check if they exist in store
		if _, err := m.credStore.GetLLMCredential(ctx, name); err == nil {
			hasValidProvider = true
			m.logger.Debug("Found valid provider credentials in store",
				"provider", name)
			break
		} else {
			m.logger.Debug("No usable credentials found in store for provider",
				"provider", name, "error", err)
		}
	}

	if !hasValidProvider {
		return types.NewError(types.ErrCodeInvalidArgument,
			"no enabled LLM providers found with credentials in the credential store")
	}

	return nil
}

// createGatewayContainer creates the LLM Gateway container with security configuration.
// Implements security best practices including:
// - Non-root user (UID 1000)
// - Read-only root filesystem
// - No-new-privileges security option
// - Dropped capabilities (ALL)
// - Minimal added capabilities (NET_BIND_SERVICE for binding to health check port)
// - Health check on HTTP endpoint
// - Resource limits (CPU, memory)
// - No volume mounts (stateless operation)
func (m *LLMGatewayManager) createGatewayContainer(ctx context.Context) (string, error) {
	// Build environment variables with API keys
	env := m.buildEnvironmentVariables()

	// Set pids limit for additional isolation
	pidsLimit := int64(100)

	// Container configuration following security best practices
	containerCfg := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:     m.config.ContainerImage,
			Env:       env,
			Labels:    m.buildLabels(),
			Resources: m.buildResourceLimits(),
			Mounts:    []types.Mount{}, // Stateless - no volume mounts
			Ports: []types.PortBinding{
				{
					ContainerPort: 8080,
					HostPort:      8080,
					Protocol:      "tcp",
					HostIP:        "127.0.0.1",
				},
			},
			Networks: []string{"bridge"}, // Use bridge network for egress-only access
			// Security constraints
			User:           "1000:1000",                   // Non-root user
			ReadOnlyRootfs: true,                          // Read-only root filesystem
			SecurityOpt:    []string{"no-new-privileges"}, // Prevent privilege escalation
			CapDrop:        []string{"ALL"},               // Drop all Linux capabilities
			CapAdd:         []string{"NET_BIND_SERVICE"},  // Add back only what's needed
			// Health check configuration
			HealthCheck: &types.HealthCheckConfig{
				Test:        []string{"CMD", "wget", "-O-", "-q", "http://localhost:8080/health"},
				Interval:    5 * time.Second,
				Timeout:     3 * time.Second,
				StartPeriod: 5 * time.Second,
				Retries:     3,
			},
		},
		Name:        "baaaht-llm-gateway",
		SessionID:   types.GenerateID(), // LLM Gateway has its own session
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	// Update resource limits to include pids limit
	containerCfg.Config.Resources.PidsLimit = &pidsLimit

	result, err := m.creator.Create(ctx, containerCfg)
	if err != nil {
		return "", err
	}

	return result.ContainerID, nil
}

// buildEnvironmentVariables builds the environment variables for the LLM Gateway container.
// This includes API keys and base URLs for configured providers.
func (m *LLMGatewayManager) buildEnvironmentVariables() map[string]string {
	env := make(map[string]string)

	// Add provider API keys and base URLs
	for name, provider := range m.config.Providers {
		if !provider.Enabled {
			continue
		}

		// Map provider names to their environment variable names
		switch name {
		case "anthropic":
			if provider.APIKey != "" {
				env["ANTHROPIC_API_KEY"] = provider.APIKey
			}
			if provider.BaseURL != "" {
				env["ANTHROPIC_BASE_URL"] = provider.BaseURL
			}
		case "openai":
			if provider.APIKey != "" {
				env["OPENAI_API_KEY"] = provider.APIKey
			}
			if provider.BaseURL != "" {
				env["OPENAI_BASE_URL"] = provider.BaseURL
			}
		case "azure":
			if provider.APIKey != "" {
				env["AZURE_API_KEY"] = provider.APIKey
			}
			if provider.BaseURL != "" {
				env["AZURE_BASE_URL"] = provider.BaseURL
			}
		case "google":
			if provider.APIKey != "" {
				env["GOOGLE_API_KEY"] = provider.APIKey
			}
			if provider.BaseURL != "" {
				env["GOOGLE_BASE_URL"] = provider.BaseURL
			}
		default:
			// For custom/local providers (e.g., lmstudio, ollama) that use
			// OpenAI-compatible APIs, pass config via generic env vars.
			// The gateway will pick these up via LLM_PROVIDER_<NAME>_* pattern.
			upperName := strings.ToUpper(name)
			if provider.APIKey != "" {
				env[fmt.Sprintf("LLM_PROVIDER_%s_API_KEY", upperName)] = provider.APIKey
			}
			if provider.BaseURL != "" {
				env[fmt.Sprintf("LLM_PROVIDER_%s_BASE_URL", upperName)] = provider.BaseURL
			}
			env[fmt.Sprintf("LLM_PROVIDER_%s_ENABLED", upperName)] = "true"
		}

		m.logger.Debug("Added config for provider", "provider", name)
	}

	// Add configuration environment variables
	env["LLM_DEFAULT_PROVIDER"] = m.config.DefaultProvider
	env["LLM_DEFAULT_MODEL"] = m.config.DefaultModel
	env["LLM_TIMEOUT"] = m.config.Timeout.String()
	env["LLM_MAX_CONCURRENT_REQUESTS"] = fmt.Sprintf("%d", m.config.MaxConcurrentRequests)

	return env
}

// buildLabels creates labels for the LLM Gateway container.
func (m *LLMGatewayManager) buildLabels() map[string]string {
	return map[string]string{
		"baaaht.managed":     "true",
		"baaaht.component":   "llm-gateway",
		"baaaht.llm_version": "1.0.0",
	}
}

// buildResourceLimits creates resource limits for the LLM Gateway container.
func (m *LLMGatewayManager) buildResourceLimits() types.ResourceLimits {
	// Set reasonable defaults for an LLM gateway
	// CPU: 1 core, Memory: 1GB
	return types.ResourceLimits{
		NanoCPUs:    1_000_000_000, // 1 CPU core
		MemoryBytes: 1_073_741_824, // 1 GB
	}
}

// cleanupContainer removes the LLM Gateway container.
func (m *LLMGatewayManager) cleanupContainer(ctx context.Context) error {
	if m.containerID == "" {
		return nil
	}

	destroyCfg := container.DestroyConfig{
		ContainerID:   m.containerID,
		Name:          "baaaht-llm-gateway",
		Force:         true,
		RemoveVolumes: false,
	}

	if err := m.lifecycle.Destroy(ctx, destroyCfg); err != nil {
		m.logger.Warn("Failed to destroy LLM Gateway container",
			"error", err)
		return err
	}

	m.containerID = ""
	return nil
}

// stopInternal is the internal stop implementation without locking.
func (m *LLMGatewayManager) stopInternal(ctx context.Context) error {
	if m.containerID == "" {
		return nil
	}

	timeout := 30 * time.Second
	stopCfg := container.StopConfig{
		ContainerID: m.containerID,
		Name:        "baaaht-llm-gateway",
		Timeout:     &timeout,
	}

	if err := m.lifecycle.Stop(ctx, stopCfg); err != nil {
		return err
	}

	m.started = false
	return nil
}

// healthMonitorLoop runs a background goroutine that monitors the LLM Gateway health.
// On unhealthy detection, it attempts to restart the container with exponential backoff.
func (m *LLMGatewayManager) healthMonitorLoop() {
	defer close(m.healthCheckDone)

	ticker := time.NewTicker(m.healthCheckInterval)
	defer ticker.Stop()

	m.logger.Info("Starting health monitoring",
		"interval", m.healthCheckInterval,
		"max_failures", m.maxConsecutiveFailures)

	for {
		select {
		case <-m.healthCheckCtx.Done():
			m.logger.Info("Health monitoring stopped")
			return

		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// performHealthCheck performs a single health check and handles failures.
func (m *LLMGatewayManager) performHealthCheck() {
	m.mu.RLock()
	containerID := m.containerID
	started := m.started
	m.mu.RUnlock()

	if !started || containerID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(m.healthCheckCtx, 10*time.Second)
	defer cancel()

	result, err := m.monitor.HealthCheck(ctx, containerID)
	if err != nil {
		m.mu.Lock()
		m.consecutiveFailures++
		m.logger.Warn("Health check failed",
			"container_id", containerID,
			"error", err,
			"consecutive_failures", m.consecutiveFailures)

		// Attempt restart if we haven't exceeded max failures
		if m.consecutiveFailures >= m.maxConsecutiveFailures {
			m.logger.Error("LLM Gateway unhealthy, attempting auto-restart",
				"container_id", containerID,
				"consecutive_failures", m.consecutiveFailures)
			m.mu.Unlock()

			m.attemptAutoRestart()
		} else {
			m.mu.Unlock()
		}
		return
	}

	// Reset consecutive failures on successful health check
	if result.Status == types.Healthy {
		m.mu.Lock()
		if m.consecutiveFailures > 0 {
			m.logger.Info("LLM Gateway health restored",
				"container_id", containerID,
				"previous_failures", m.consecutiveFailures)
			m.consecutiveFailures = 0
		}
		m.mu.Unlock()

		m.logger.Debug("LLM Gateway is healthy",
			"container_id", containerID)
	} else if result.Status == types.Unhealthy {
		m.mu.Lock()
		m.consecutiveFailures++
		m.logger.Warn("LLM Gateway is unhealthy",
			"container_id", containerID,
			"failing_streak", result.FailingStreak,
			"consecutive_failures", m.consecutiveFailures,
			"last_output", result.LastOutput)

		// Attempt restart if we haven't exceeded max failures
		if m.consecutiveFailures >= m.maxConsecutiveFailures {
			m.logger.Error("LLM Gateway unhealthy, attempting auto-restart",
				"container_id", containerID,
				"consecutive_failures", m.consecutiveFailures)
			m.mu.Unlock()

			m.attemptAutoRestart()
		} else {
			m.mu.Unlock()
		}
	}
}

// attemptAutoRestart attempts to restart the LLM Gateway container.
// Uses exponential backoff for restart attempts.
func (m *LLMGatewayManager) attemptAutoRestart() {
	m.mu.Lock()

	// Check if we're still in a state where restart is needed
	if !m.started || m.closed {
		m.mu.Unlock()
		return
	}

	// Calculate backoff time based on consecutive failures
	backoffTime := time.Duration(1<<uint(m.consecutiveFailures)) * time.Second
	if backoffTime > 60*time.Second {
		backoffTime = 60 * time.Second
	}

	containerID := m.containerID
	m.mu.Unlock()

	m.logger.Info("Attempting auto-restart with exponential backoff",
		"container_id", containerID,
		"backoff", backoffTime)

	// Wait for backoff period
	select {
	case <-time.After(backoffTime):
	case <-m.healthCheckCtx.Done():
		m.logger.Info("Auto-restart canceled")
		return
	}

	// Attempt restart
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := m.Restart(ctx); err != nil {
		m.logger.Error("Auto-restart failed",
			"container_id", containerID,
			"error", err)
		// Don't increment failures here - the next health check will handle it
		return
	}

	// Reset consecutive failures on successful restart
	m.mu.Lock()
	m.consecutiveFailures = 0
	m.mu.Unlock()

	m.logger.Info("Auto-restart succeeded",
		"container_id", containerID)
}

// String returns a string representation of the LLM Gateway manager.
func (m *LLMGatewayManager) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	started := "no"
	if m.started {
		started = "yes"
	}

	return fmt.Sprintf("LLMGatewayManager{started: %s, enabled: %v, container_id: %s, image: %s}",
		started, m.config.Enabled, m.containerID, m.config.ContainerImage)
}
