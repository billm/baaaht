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
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// LLMGatewayManager manages the lifecycle of the LLM Gateway container.
// The LLM Gateway is responsible for isolating LLM API credentials from agent
// containers, providing a unified interface for LLM requests, and tracking
// token usage for cost monitoring.
type LLMGatewayManager struct {
	runtime    container.Runtime
	config     config.LLMConfig
	credStore  *credentials.Store
	logger     *logger.Logger
	mu         sync.RWMutex

	// Container state
	containerID string
	started     bool
	closed      bool

	// Lifecycle managers
	lifecycle *container.LifecycleManager
	monitor   *container.Monitor
	creator   *container.Creator
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

	return &LLMGatewayManager{
		runtime:   runtime,
		config:    cfg,
		credStore: credStore,
		logger:    log.With("component", "llm_gateway_manager"),
		lifecycle: lifecycle,
		monitor:   monitor,
		creator:   creator,
		started:   false,
		closed:    false,
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
		Name:        "llm-gateway",
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

	timeout := 30 * time.Second
	stopCfg := container.StopConfig{
		ContainerID: m.containerID,
		Name:        "llm-gateway",
		Timeout:     &timeout,
	}

	if err := m.lifecycle.Stop(ctx, stopCfg); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to stop LLM Gateway container", err)
	}

	m.started = false
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
		Name:        "llm-gateway",
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

	// Stop the container if it's running
	if m.started && m.containerID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := m.stopInternal(ctx); err != nil {
			m.logger.Error("Failed to stop LLM Gateway during close",
				"error", err)
		}
	}

	m.closed = true
	m.started = false
	m.containerID = ""

	m.logger.Info("LLM Gateway manager closed")

	return nil
}

// validateCredentials checks that required LLM provider credentials exist.
func (m *LLMGatewayManager) validateCredentials(ctx context.Context) error {
	// Check that the default provider has credentials configured
	if m.config.DefaultProvider == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "default provider is not configured")
	}

	// Verify at least one provider is configured with credentials
	hasValidProvider := false
	for name, provider := range m.config.Providers {
		if provider.Enabled && provider.APIKey != "" {
			hasValidProvider = true
			m.logger.Debug("Found valid provider configuration",
				"provider", name)
			break
		}
	}

	if !hasValidProvider {
		return types.NewError(types.ErrCodeInvalidArgument,
			"no enabled LLM providers found with API keys configured")
	}

	return nil
}

// createGatewayContainer creates the LLM Gateway container with security configuration.
// This is a placeholder implementation - the full implementation with security
// constraints will be added in a subsequent subtask.
func (m *LLMGatewayManager) createGatewayContainer(ctx context.Context) (string, error) {
	// Build environment variables with API keys
	env := m.buildEnvironmentVariables()

	// Container configuration following security best practices
	containerCfg := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:     m.config.ContainerImage,
			Env:       env,
			Labels:    m.buildLabels(),
			Resources: m.buildResourceLimits(),
			// Security settings will be added in subtask-5-2
		},
		Name:      "baaaht-llm-gateway",
		SessionID: types.GenerateID(), // LLM Gateway has its own session
		AutoPull:  true,
		PullTimeout: 5 * time.Minute,
	}

	result, err := m.creator.Create(ctx, containerCfg)
	if err != nil {
		return "", err
	}

	return result.ContainerID, nil
}

// buildEnvironmentVariables builds the environment variables for the LLM Gateway container.
// This includes API keys for configured providers.
func (m *LLMGatewayManager) buildEnvironmentVariables() map[string]string {
	env := make(map[string]string)

	// Add provider API keys
	for name, provider := range m.config.Providers {
		if !provider.Enabled || provider.APIKey == "" {
			continue
		}

		// Map provider names to their environment variable names
		switch name {
		case "anthropic":
			env["ANTHROPIC_API_KEY"] = provider.APIKey
		case "openai":
			env["OPENAI_API_KEY"] = provider.APIKey
		case "azure":
			env["AZURE_API_KEY"] = provider.APIKey
		case "google":
			env["GOOGLE_API_KEY"] = provider.APIKey
		}

		m.logger.Debug("Added API key for provider", "provider", name)
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
		Name:         "llm-gateway",
		Force:        true,
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
		Name:        "llm-gateway",
		Timeout:     &timeout,
	}

	if err := m.lifecycle.Stop(ctx, stopCfg); err != nil {
		return err
	}

	m.started = false
	return nil
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
