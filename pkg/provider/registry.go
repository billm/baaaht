package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Registry manages provider instances with registration and lookup
type Registry struct {
	providers map[Provider]LLMProvider
	mu        sync.RWMutex
	cfg       RegistryConfig
	logger    *logger.Logger
	closed    bool
	defaultProvider Provider
	failoverManager *FailoverManager
}

// NewRegistry creates a new provider registry with the specified configuration
func NewRegistry(cfg RegistryConfig, log *logger.Logger) (*Registry, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Validate configuration
	if err := validateRegistryConfig(&cfg); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalidArgument, "invalid registry configuration", err)
	}

	// Initialize providers map if nil
	if cfg.Providers == nil {
		cfg.Providers = make(map[Provider]ProviderConfig)
	}

	// Set default provider if not specified
	if cfg.DefaultProvider == "" {
		if len(cfg.Providers) > 0 {
			// Use first available provider as default
			for p := range cfg.Providers {
				cfg.DefaultProvider = p
				break
			}
		} else {
			cfg.DefaultProvider = ProviderAnthropic
		}
	}

	registry := &Registry{
		providers:       make(map[Provider]LLMProvider),
		cfg:             cfg,
		logger:          log.With("component", "provider_registry"),
		closed:          false,
		defaultProvider: cfg.DefaultProvider,
		failoverManager: NewFailoverManager(cfg, log),
	}

	registry.logger.Info("Provider registry initialized",
		"default_provider", cfg.DefaultProvider,
		"configured_providers", len(cfg.Providers),
		"failover_enabled", cfg.FailoverEnabled)

	return registry, nil
}

// NewDefaultRegistry creates a new provider registry with default configuration
func NewDefaultRegistry(log *logger.Logger) (*Registry, error) {
	cfg := DefaultConfig()
	return NewRegistry(cfg, log)
}

// validateRegistryConfig validates the registry configuration
func validateRegistryConfig(cfg *RegistryConfig) error {
	// Validate default provider if specified
	if cfg.DefaultProvider != "" {
		if !cfg.DefaultProvider.IsValid() {
			return fmt.Errorf("invalid default provider: %s", cfg.DefaultProvider)
		}
	}

	// Validate each provider configuration
	for providerID, providerCfg := range cfg.Providers {
		if !providerID.IsValid() {
			return fmt.Errorf("invalid provider ID: %s", providerID)
		}

		// Validate that provider config matches the provider ID
		if providerCfg.Provider != "" && providerCfg.Provider != providerID {
			return fmt.Errorf("provider config mismatch: key is %s but config specifies %s",
				providerID, providerCfg.Provider)
		}

		// Ensure Provider field is set
		if providerCfg.Provider == "" {
			providerCfg.Provider = providerID
		}
	}

	return nil
}

// Register registers a provider with the registry
func (r *Registry) Register(ctx context.Context, provider LLMProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	if provider == nil {
		return types.NewError(types.ErrCodeInvalidArgument, "provider cannot be nil")
	}

	providerID := provider.Provider()

	// Check if provider already exists
	if _, exists := r.providers[providerID]; exists {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("provider already registered: %s", providerID))
	}

	// Store the provider
	r.providers[providerID] = provider

	r.logger.Info("Provider registered",
		"provider", providerID,
		"name", provider.Name(),
		"status", provider.Status(),
		"available", provider.IsAvailable())

	return nil
}

// RegisterFromConfig registers a provider from configuration
func (r *Registry) RegisterFromConfig(ctx context.Context, providerCfg ProviderConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	providerID := providerCfg.Provider

	// Validate provider ID
	if !providerID.IsValid() {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid provider ID: %s", providerID))
	}

	// Check if provider already exists
	if _, exists := r.providers[providerID]; exists {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("provider already registered: %s", providerID))
	}

	// Create provider instance based on type
	var provider LLMProvider
	var err error

	switch providerID {
	case ProviderAnthropic:
		provider, err = NewAnthropicProvider(providerCfg, r.logger)
	case ProviderOpenAI:
		provider, err = NewOpenAIProvider(providerCfg, r.logger)
	default:
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("unknown provider type: %s", providerID))
	}

	if err != nil {
		return types.WrapError(types.ErrCodeInternal,
			fmt.Sprintf("failed to create provider %s", providerID), err)
	}

	// Store the provider
	r.providers[providerID] = provider

	r.logger.Info("Provider registered from config",
		"provider", providerID,
		"name", provider.Name(),
		"enabled", providerCfg.Enabled,
		"priority", providerCfg.Priority)

	return nil
}

// Unregister removes a provider from the registry
func (r *Registry) Unregister(ctx context.Context, providerID Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	provider, exists := r.providers[providerID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("provider not found: %s", providerID))
	}

	// Close the provider
	if err := provider.Close(); err != nil {
		r.logger.Warn("Failed to close provider during unregistration",
			"provider", providerID,
			"error", err)
	}

	delete(r.providers, providerID)

	r.logger.Info("Provider unregistered", "provider", providerID)

	return nil
}

// Get retrieves a provider by ID
func (r *Registry) Get(ctx context.Context, providerID Provider) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	if !providerID.IsValid() {
		return nil, types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid provider ID: %s", providerID))
	}

	provider, exists := r.providers[providerID]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("provider not found: %s", providerID))
	}

	return provider, nil
}

// GetDefault returns the default provider
func (r *Registry) GetDefault(ctx context.Context) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	provider, exists := r.providers[r.defaultProvider]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("default provider not found: %s", r.defaultProvider))
	}

	return provider, nil
}

// SetDefault sets the default provider
func (r *Registry) SetDefault(ctx context.Context, providerID Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	if !providerID.IsValid() {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid provider ID: %s", providerID))
	}

	if _, exists := r.providers[providerID]; !exists {
		return types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("provider not found: %s", providerID))
	}

	oldDefault := r.defaultProvider
	r.defaultProvider = providerID

	r.logger.Info("Default provider changed",
		"old_default", oldDefault,
		"new_default", providerID)

	return nil
}

// List returns all registered providers
func (r *Registry) List(ctx context.Context) ([]LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	result := make([]LLMProvider, 0, len(r.providers))
	for _, provider := range r.providers {
		result = append(result, provider)
	}

	return result, nil
}

// ListAvailable returns all available (enabled and healthy) providers
func (r *Registry) ListAvailable(ctx context.Context) ([]LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	result := make([]LLMProvider, 0)
	for _, provider := range r.providers {
		if provider.IsAvailable() {
			result = append(result, provider)
		}
	}

	return result, nil
}

// GetProviderByModel returns the provider that supports the given model
func (r *Registry) GetProviderByModel(ctx context.Context, model Model) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	if model.IsEmpty() {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "model cannot be empty")
	}

	// Find provider that supports this model
	for _, provider := range r.providers {
		if provider.SupportsModel(model) && provider.IsAvailable() {
			return provider, nil
		}
	}

	return nil, types.NewError(types.ErrCodeNotFound,
		fmt.Sprintf("no available provider found for model: %s", model))
}

// Stats returns statistics for all registered providers
func (r *Registry) Stats(ctx context.Context) (map[Provider]*ProviderStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	result := make(map[Provider]*ProviderStats)

	for providerID, provider := range r.providers {
		// Build basic stats from provider info
		stats := &ProviderStats{
			Provider: providerID,
			Status:   provider.Status(),
		}

		// Try to get token accounting if provider supports it
		if tp, ok := provider.(interface{ GetTokenAccount() *TokenAccount }); ok {
			account := tp.GetTokenAccount()
			if account != nil {
				summary := account.GetSummary()
				stats.TotalTokens = int64(summary.TotalUsage.TotalTokens)
				stats.InputTokens = int64(summary.TotalUsage.InputTokens)
				stats.OutputTokens = int64(summary.TotalUsage.OutputTokens)
				stats.CacheReadTokens = int64(summary.TotalUsage.CacheReadTokens)
				stats.CacheWriteTokens = int64(summary.TotalUsage.CacheWriteTokens)
			}
		}

		result[providerID] = stats
	}

	return result, nil
}

// Close closes all registered providers and the registry
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.logger.Info("Provider registry shutting down...")

	// Close failover manager
	if r.failoverManager != nil {
		if err := r.failoverManager.Close(); err != nil {
			r.logger.Warn("Failed to close failover manager", "error", err)
		}
	}

	// Close all providers
	var lastErr error
	for providerID, provider := range r.providers {
		if err := provider.Close(); err != nil {
			r.logger.Warn("Failed to close provider",
				"provider", providerID,
				"error", err)
			lastErr = err
		}
	}

	// Clear the providers map
	r.providers = make(map[Provider]LLMProvider)
	r.closed = true

	r.logger.Info("Provider registry closed")

	return lastErr
}

// IsClosed returns true if the registry is closed
func (r *Registry) IsClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.closed
}

// GetWithFailover returns a provider with automatic failover to backup providers
// If the primary provider fails, it will try backup providers in priority order
// Returns the provider, whether failover occurred, and an error
func (r *Registry) GetWithFailover(ctx context.Context, primary Provider) (LLMProvider, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, false, types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	return r.failoverManager.SelectProviderWithFailover(
		ctx,
		primary,
		func(p Provider) (LLMProvider, error) {
			provider, exists := r.providers[p]
			if !exists {
				return nil, types.NewError(types.ErrCodeNotFound, "provider not found: "+p.String())
			}
			return provider, nil
		},
		func() ([]LLMProvider, error) {
			result := make([]LLMProvider, 0, len(r.providers))
			for _, provider := range r.providers {
				result = append(result, provider)
			}
			return result, nil
		},
	)
}

// RecordProviderFailure records a failure for a provider in the failover manager
func (r *Registry) RecordProviderFailure(providerID Provider, reason string) {
	r.failoverManager.RecordFailure(providerID, reason)
}

// RecordProviderSuccess records a success for a provider in the failover manager
func (r *Registry) RecordProviderSuccess(providerID Provider) {
	r.failoverManager.RecordSuccess(providerID)
}

// GetFailoverState returns the current failover state for a provider
func (r *Registry) GetFailoverState(providerID Provider) FailoverState {
	return r.failoverManager.GetProviderState(providerID)
}

// GetAllFailoverStates returns failover states for all providers
func (r *Registry) GetAllFailoverStates() map[Provider]FailoverState {
	return r.failoverManager.GetAllProviderStates()
}

// IsProviderHealthy checks if a provider is healthy considering failover state
func (r *Registry) IsProviderHealthy(providerID Provider) bool {
	// Use the health checker if available for more accurate health status
	if r.failoverManager != nil {
		return r.failoverManager.IsProviderHealthy(providerID)
	}
	return true
}

// HealthCheck performs a health check on a provider
func (r *Registry) HealthCheck(ctx context.Context, providerID Provider) (HealthStatus, error) {
	if r.failoverManager == nil {
		return HealthStatusHealthy, nil
	}

	return r.failoverManager.HealthCheck(ctx, providerID, func(p Provider) (LLMProvider, error) {
		r.mu.RLock()
		defer r.mu.RUnlock()

		if r.closed {
			return nil, types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
		}

		provider, exists := r.providers[p]
		if !exists {
			return nil, types.NewError(types.ErrCodeNotFound, "provider not found: "+p.String())
		}
		return provider, nil
	})
}

// GetHealthStatus returns the current health status for a provider
func (r *Registry) GetHealthStatus(providerID Provider) HealthStatus {
	if r.failoverManager == nil {
		return HealthStatusHealthy
	}
	return r.failoverManager.GetHealthStatus(providerID)
}

// SetHealthStatus sets the health status for a provider
func (r *Registry) SetHealthStatus(providerID Provider, status HealthStatus) {
	if r.failoverManager != nil {
		r.failoverManager.SetHealthStatus(providerID, status)
	}
}

// GetAllHealthStatuses returns all provider health statuses
func (r *Registry) GetAllHealthStatuses() map[Provider]HealthStatus {
	if r.failoverManager == nil {
		return make(map[Provider]HealthStatus)
	}
	return r.failoverManager.GetAllHealthStatuses()
}

// ResetProviderFailover resets the failover state for a specific provider
func (r *Registry) ResetProviderFailover(providerID Provider) {
	r.failoverManager.Reset(providerID)
}

// ResetAllFailover resets all failover states
func (r *Registry) ResetAllFailover() {
	r.failoverManager.ResetAll()
}

// InitializeFromConfig initializes the registry from configuration
func (r *Registry) InitializeFromConfig(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "provider registry is closed")
	}

	r.logger.Info("Initializing providers from configuration",
		"provider_count", len(r.cfg.Providers))

	// Register each provider from configuration
	for providerID, providerCfg := range r.cfg.Providers {
		// Skip disabled providers
		if !providerCfg.Enabled {
			r.logger.Debug("Skipping disabled provider", "provider", providerID)
			continue
		}

		// Ensure Provider field matches the key
		providerCfg.Provider = providerID

		// Check if already registered
		if _, exists := r.providers[providerID]; exists {
			r.logger.Debug("Provider already registered, skipping", "provider", providerID)
			continue
		}

		// Create provider instance
		var provider LLMProvider
		var err error

		switch providerID {
		case ProviderAnthropic:
			provider, err = NewAnthropicProvider(providerCfg, r.logger)
		case ProviderOpenAI:
			provider, err = NewOpenAIProvider(providerCfg, r.logger)
		default:
			r.logger.Warn("Unknown provider type in configuration", "provider", providerID)
			continue
		}

		if err != nil {
			r.logger.Warn("Failed to create provider from configuration",
				"provider", providerID,
				"error", err)
			// Continue with other providers
			continue
		}

		// Store the provider
		r.providers[providerID] = provider

		r.logger.Info("Provider initialized from config",
			"provider", providerID,
			"name", provider.Name(),
			"status", provider.Status())
	}

	r.logger.Info("Provider initialization complete",
		"registered_providers", len(r.providers))

	return nil
}

// Config returns the registry configuration
func (r *Registry) Config() RegistryConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cfg
}

// Global registry instance
var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

// InitGlobal initializes the global provider registry
func InitGlobal(cfg RegistryConfig, log *logger.Logger) error {
	var initErr error
	registryOnce.Do(func() {
		registry, err := NewRegistry(cfg, log)
		if err != nil {
			initErr = err
			return
		}

		// Initialize providers from configuration
		if err := registry.InitializeFromConfig(context.Background()); err != nil {
			initErr = err
			return
		}

		globalRegistry = registry
	})
	return initErr
}

// Global returns the global provider registry instance
func Global() *Registry {
	if globalRegistry == nil {
		// Initialize with default settings if not already initialized
		registry, err := NewDefaultRegistry(nil)
		if err != nil {
			// Return a closed registry on error
			registry = &Registry{closed: true}
		}
		globalRegistry = registry
	}
	return globalRegistry
}

// SetGlobal sets the global provider registry instance
func SetGlobal(r *Registry) {
	globalRegistry = r
	registryOnce = sync.Once{}
}
