package provider

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
)

// HealthStatus represents the health status of a provider
type HealthStatus string

const (
	HealthStatusUnknown     HealthStatus = "unknown"
	HealthStatusHealthy     HealthStatus = "healthy"
	HealthStatusUnhealthy   HealthStatus = "unhealthy"
	HealthStatusDegraded    HealthStatus = "degraded"
)

// String returns the string representation of the health status
func (s HealthStatus) String() string {
	return string(s)
}

// IsHealthy returns true if the status is healthy or degraded
func (s HealthStatus) IsHealthy() bool {
	return s == HealthStatusHealthy || s == HealthStatusDegraded
}

// FailoverManager manages automatic failover between providers
type FailoverManager struct {
	mu             sync.RWMutex
	cfg            RegistryConfig
	logger         *logger.Logger
	providerStates map[Provider]*failoverState
	closed         bool
	healthChecker  *HealthChecker
}

// failoverState tracks the failover state for a single provider
type failoverState struct {
	consecutiveFailures int
	circuitOpenUntil    time.Time
	lastFailureTime     time.Time
	lastFailureReason   string
	healthStatus        HealthStatus
	lastHealthCheckTime time.Time
}

// HealthChecker manages health status checking for providers
type HealthChecker struct {
	mu             sync.RWMutex
	logger         *logger.Logger
	providerStates map[Provider]*failoverState
	closed         bool
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(log *logger.Logger) *HealthChecker {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	hc := &HealthChecker{
		logger:         log.With("component", "health_checker"),
		providerStates: make(map[Provider]*failoverState),
		closed:         false,
	}

	hc.logger.Info("Health checker initialized")

	return hc
}

// Check checks the health status of a provider
// This follows the pattern from grpc/health.go Check method
func (hc *HealthChecker) Check(ctx context.Context, providerID Provider, getProviderFunc func(Provider) (LLMProvider, error)) (HealthStatus, error) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if hc.closed {
		return HealthStatusUnhealthy, NewProviderError(ErrCodeProviderNotAvailable, "health checker is closed")
	}

	// Get the provider
	provider, err := getProviderFunc(providerID)
	if err != nil {
		hc.logger.Debug("Health check failed - provider not found",
			"provider", providerID,
			"error", err)
		return HealthStatusUnhealthy, err
	}

	// Check if provider is available via its IsAvailable method
	if !provider.IsAvailable() {
		hc.logger.Debug("Health check - provider unavailable",
			"provider", providerID,
			"status", provider.Status())
		return HealthStatusUnhealthy, nil
	}

	// Provider is available, check the stored health state
	state, exists := hc.providerStates[providerID]
	if !exists {
		// No prior health state, provider is considered healthy
		return HealthStatusHealthy, nil
	}

	// Return the stored health status
	return state.healthStatus, nil
}

// GetStatus returns the current health status for a provider
// This follows the pattern from grpc/health.go GetStatus method
func (hc *HealthChecker) GetStatus(providerID Provider) HealthStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if hc.closed {
		return HealthStatusUnhealthy
	}

	state, exists := hc.providerStates[providerID]
	if !exists {
		// No health state recorded, default to healthy
		return HealthStatusHealthy
	}

	return state.healthStatus
}

// SetStatus sets the health status for a provider
// This follows the pattern from grpc/health.go SetServingStatus method
func (hc *HealthChecker) SetStatus(providerID Provider, status HealthStatus) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if hc.closed {
		return
	}

	state := hc.getOrCreateState(providerID)
	oldStatus := state.healthStatus
	state.healthStatus = status
	state.lastHealthCheckTime = time.Now()

	hc.logger.Info("Health status updated",
		"provider", providerID,
		"old_status", oldStatus,
		"new_status", status)
}

// IsHealthy returns true if the provider is healthy
// This follows the pattern from grpc/health.go IsServing method
func (hc *HealthChecker) IsHealthy(providerID Provider) bool {
	return hc.GetStatus(providerID).IsHealthy()
}

// SetHealthy sets the provider health status to healthy
func (hc *HealthChecker) SetHealthy(providerID Provider) {
	hc.SetStatus(providerID, HealthStatusHealthy)
}

// SetUnhealthy sets the provider health status to unhealthy
func (hc *HealthChecker) SetUnhealthy(providerID Provider) {
	hc.SetStatus(providerID, HealthStatusUnhealthy)
}

// SetDegraded sets the provider health status to degraded
func (hc *HealthChecker) SetDegraded(providerID Provider) {
	hc.SetStatus(providerID, HealthStatusDegraded)
}

// GetAllStatuses returns a copy of all provider health statuses
// This follows the pattern from grpc/health.go GetAllStatuses method
func (hc *HealthChecker) GetAllStatuses() map[Provider]HealthStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	result := make(map[Provider]HealthStatus, len(hc.providerStates))
	for providerID, state := range hc.providerStates {
		result[providerID] = state.healthStatus
	}
	return result
}

// Close closes the health checker
func (hc *HealthChecker) Close() error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if hc.closed {
		return nil
	}

	hc.logger.Info("Health checker closing...")
	hc.closed = true
	hc.providerStates = make(map[Provider]*failoverState)

	return nil
}

// getOrCreateState gets or creates a health state for a provider
// Must be called with lock held
func (hc *HealthChecker) getOrCreateState(providerID Provider) *failoverState {
	if state, exists := hc.providerStates[providerID]; exists {
		return state
	}

	state := &failoverState{
		healthStatus: HealthStatusHealthy, // Default to healthy
	}
	hc.providerStates[providerID] = state
	return state
}

// setProviderStates sets the provider states map (used by FailoverManager)
func (hc *HealthChecker) setProviderStates(states map[Provider]*failoverState) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.providerStates = states
}

// NewFailoverManager creates a new failover manager
func NewFailoverManager(cfg RegistryConfig, log *logger.Logger) *FailoverManager {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			// If we can't create a logger, create a no-op fallback
			log = &logger.Logger{}
		}
	}

	providerStates := make(map[Provider]*failoverState)

	fm := &FailoverManager{
		cfg:            cfg,
		logger:         log.With("component", "failover_manager"),
		providerStates: providerStates,
		closed:         false,
	}

	// Initialize health checker with shared provider states
	fm.healthChecker = NewHealthChecker(log)
	fm.healthChecker.setProviderStates(providerStates)

	fm.logger.Info("Failover manager initialized",
		"enabled", cfg.FailoverEnabled,
		"threshold", cfg.FailoverThreshold,
		"circuit_breaker_timeout", cfg.CircuitBreakerTimeout)

	return fm
}

// RecordFailure records a failure for the specified provider
func (fm *FailoverManager) RecordFailure(providerID Provider, reason string) {
	if !fm.cfg.FailoverEnabled {
		return
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.closed {
		return
	}

	state := fm.getOrCreateState(providerID)
	state.consecutiveFailures++
	state.lastFailureTime = time.Now()
	state.lastFailureReason = reason

	// Check if we should open the circuit
	if state.consecutiveFailures >= fm.cfg.FailoverThreshold {
		state.circuitOpenUntil = time.Now().Add(fm.cfg.CircuitBreakerTimeout)
		fm.logger.Warn("Circuit breaker opened for provider",
			"provider", providerID,
			"consecutive_failures", state.consecutiveFailures,
			"threshold", fm.cfg.FailoverThreshold,
			"opens_until", state.circuitOpenUntil,
			"last_error", reason)
	} else {
		fm.logger.Debug("Provider failure recorded",
			"provider", providerID,
			"consecutive_failures", state.consecutiveFailures,
			"threshold", fm.cfg.FailoverThreshold,
			"error", reason)
	}
}

// RecordSuccess records a success for the specified provider
func (fm *FailoverManager) RecordSuccess(providerID Provider) {
	if !fm.cfg.FailoverEnabled {
		return
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.closed {
		return
	}

	state := fm.getOrCreateState(providerID)

	// Reset failure count on success
	if state.consecutiveFailures > 0 {
		fm.logger.Debug("Provider success recorded, resetting failure count",
			"provider", providerID,
			"previous_failures", state.consecutiveFailures)
	}

	state.consecutiveFailures = 0
	state.circuitOpenUntil = time.Time{}
	state.lastFailureReason = ""
}

// IsProviderAvailable checks if a provider is available considering circuit breaker state
func (fm *FailoverManager) IsProviderAvailable(providerID Provider) bool {
	if !fm.cfg.FailoverEnabled {
		return true
	}

	fm.mu.RLock()
	defer fm.mu.RUnlock()

	if fm.closed {
		return false
	}

	state, exists := fm.providerStates[providerID]
	if !exists {
		// No failures recorded, provider is available
		return true
	}

	// Check if circuit is open
	if !state.circuitOpenUntil.IsZero() {
		// Circuit is open only if the open time is strictly in the future
		if state.circuitOpenUntil.After(time.Now()) {
			// Circuit is still open
			return false
		}
		// Circuit timeout has expired, allow retry
	}

	return true
}

// ShouldAttemptProvider returns true if the provider should be attempted
// This considers both availability and whether we should allow a retry after circuit timeout
func (fm *FailoverManager) ShouldAttemptProvider(providerID Provider) bool {
	if !fm.cfg.FailoverEnabled {
		return true
	}

	fm.mu.RLock()
	defer fm.mu.RUnlock()

	if fm.closed {
		return false
	}

	state, exists := fm.providerStates[providerID]
	if !exists {
		return true
	}

	// If circuit is open and timeout hasn't expired, don't attempt
	if !state.circuitOpenUntil.IsZero() && state.circuitOpenUntil.After(time.Now()) {
		return false
	}

	return true
}

// GetProviderState returns the current failover state for a provider
func (fm *FailoverManager) GetProviderState(providerID Provider) FailoverState {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	state := FailoverState{
		Provider:          providerID,
		ConsecutiveFailures: 0,
		CircuitOpen:       false,
		LastFailureReason: "",
	}

	if s, exists := fm.providerStates[providerID]; exists {
		state.ConsecutiveFailures = s.consecutiveFailures
		state.CircuitOpen = !s.circuitOpenUntil.IsZero() && time.Now().Before(s.circuitOpenUntil)
		state.LastFailureReason = s.lastFailureReason
		if !s.lastFailureTime.IsZero() {
			state.LastFailureTime = &s.lastFailureTime
		}
	}

	return state
}

// GetAllProviderStates returns failover states for all providers
func (fm *FailoverManager) GetAllProviderStates() map[Provider]FailoverState {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	result := make(map[Provider]FailoverState)

	for providerID := range fm.providerStates {
		result[providerID] = fm.GetProviderState(providerID)
	}

	return result
}

// Reset resets the failover state for a specific provider
func (fm *FailoverManager) Reset(providerID Provider) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if state, exists := fm.providerStates[providerID]; exists {
		state.consecutiveFailures = 0
		state.circuitOpenUntil = time.Time{}
		state.lastFailureReason = ""
		state.healthStatus = HealthStatusHealthy // Reset health status to healthy
		state.lastHealthCheckTime = time.Time{}

		fm.logger.Info("Failover state reset for provider", "provider", providerID)
	}
}

// ResetAll resets all failover states
func (fm *FailoverManager) ResetAll() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	for providerID := range fm.providerStates {
		fm.providerStates[providerID] = &failoverState{
			healthStatus: HealthStatusHealthy, // Initialize as healthy
		}
	}

	fm.logger.Info("All failover states reset")
}

// Close closes the failover manager
func (fm *FailoverManager) Close() error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.closed {
		return nil
	}

	fm.logger.Info("Failover manager closing...")

	// Close health checker
	if fm.healthChecker != nil {
		if err := fm.healthChecker.Close(); err != nil {
			fm.logger.Warn("Failed to close health checker", "error", err)
		}
	}

	fm.closed = true
	fm.providerStates = make(map[Provider]*failoverState)

	return nil
}

// getOrCreateState gets or creates a failover state for a provider
func (fm *FailoverManager) getOrCreateState(providerID Provider) *failoverState {
	if state, exists := fm.providerStates[providerID]; exists {
		return state
	}

	state := &failoverState{}
	fm.providerStates[providerID] = state
	return state
}

// SelectProviderWithFailover selects a provider with automatic failover
// Returns the selected provider and whether failover occurred
func (fm *FailoverManager) SelectProviderWithFailover(
	ctx context.Context,
	primary Provider,
	getProviderFunc func(Provider) (LLMProvider, error),
	listProvidersFunc func() ([]LLMProvider, error),
) (LLMProvider, bool, error) {
	if !fm.cfg.FailoverEnabled {
		provider, err := getProviderFunc(primary)
		return provider, false, err
	}

	// Try primary provider first
	if fm.ShouldAttemptProvider(primary) {
		provider, err := getProviderFunc(primary)
		if err == nil && provider != nil && provider.IsAvailable() {
			return provider, false, nil
		}
		// Primary not available, record the failure
		if err != nil {
			fm.RecordFailure(primary, err.Error())
		}
	}

	// Primary failed, try backup providers sorted by priority
	fm.logger.Debug("Primary provider unavailable, trying failover",
		"primary", primary)

	providers, err := listProvidersFunc()
	if err != nil {
		return nil, false, WrapProviderError(ErrCodeProviderNotFound,
			"failed to list providers for failover", err)
	}

	// Filter available providers excluding the failed primary
	var candidates []LLMProvider
	for _, p := range providers {
		if p.Provider() != primary && fm.ShouldAttemptProvider(p.Provider()) {
			candidates = append(candidates, p)
		}
	}

	if len(candidates) == 0 {
		return nil, false, NewProviderError(ErrCodeProviderNotAvailable,
			"no available backup providers")
	}

	// Sort candidates by priority from config
	fm.sortByPriority(candidates)

	// Try each backup provider in priority order
	for _, provider := range candidates {
		if provider.IsAvailable() {
			fm.logger.Info("Failover: using backup provider",
				"primary", primary,
				"backup", provider.Provider(),
				"priority", fm.getProviderPriority(provider.Provider()))
			return provider, true, nil
		}
	}

	return nil, false, NewProviderError(ErrCodeProviderNotAvailable,
		"all providers are unavailable")
}

// sortByPriority sorts providers by their priority (lower number = higher priority)
func (fm *FailoverManager) sortByPriority(providers []LLMProvider) {
	for i := 0; i < len(providers); i++ {
		for j := i + 1; j < len(providers); j++ {
			p1 := fm.getProviderPriority(providers[i].Provider())
			p2 := fm.getProviderPriority(providers[j].Provider())
			if p2 < p1 {
				providers[i], providers[j] = providers[j], providers[i]
			}
		}
	}
}

// getProviderPriority returns the priority for a provider from config
func (fm *FailoverManager) getProviderPriority(providerID Provider) int {
	if cfg, exists := fm.cfg.Providers[providerID]; exists {
		if cfg.Priority > 0 {
			return cfg.Priority
		}
	}
	return DefaultProviderPriority
}

// HealthCheck checks the health of a provider
// This is the primary health check method following the grpc pattern
func (fm *FailoverManager) HealthCheck(ctx context.Context, providerID Provider, getProviderFunc func(Provider) (LLMProvider, error)) (HealthStatus, error) {
	if fm.healthChecker == nil {
		return HealthStatusHealthy, nil
	}
	return fm.healthChecker.Check(ctx, providerID, getProviderFunc)
}

// GetHealthStatus returns the current health status for a provider
func (fm *FailoverManager) GetHealthStatus(providerID Provider) HealthStatus {
	if fm.healthChecker == nil {
		return HealthStatusHealthy
	}
	return fm.healthChecker.GetStatus(providerID)
}

// SetHealthStatus sets the health status for a provider
func (fm *FailoverManager) SetHealthStatus(providerID Provider, status HealthStatus) {
	if fm.healthChecker != nil {
		fm.healthChecker.SetStatus(providerID, status)
	}
}

// IsProviderHealthy returns true if the provider is healthy
// This considers both the health status and the circuit breaker state
func (fm *FailoverManager) IsProviderHealthy(providerID Provider) bool {
	if fm.healthChecker == nil {
		// Fall back to circuit breaker check if no health checker
		return fm.IsProviderAvailable(providerID)
	}

	// Check health status from health checker
	if !fm.healthChecker.IsHealthy(providerID) {
		return false
	}

	// Also check circuit breaker state
	return fm.IsProviderAvailable(providerID)
}

// GetAllHealthStatuses returns all provider health statuses
func (fm *FailoverManager) GetAllHealthStatuses() map[Provider]HealthStatus {
	if fm.healthChecker == nil {
		return make(map[Provider]HealthStatus)
	}
	return fm.healthChecker.GetAllStatuses()
}

// FailoverState represents the failover state for a provider
type FailoverState struct {
	Provider            Provider     `json:"provider"`
	ConsecutiveFailures int          `json:"consecutive_failures"`
	CircuitOpen         bool         `json:"circuit_open"`
	LastFailureTime     *time.Time   `json:"last_failure_time,omitempty"`
	LastFailureReason   string       `json:"last_failure_reason,omitempty"`
}

// IsHealthy returns true if the provider is healthy (circuit is closed)
func (s *FailoverState) IsHealthy() bool {
	return !s.CircuitOpen
}

// String returns a string representation of the failover state
func (s *FailoverState) String() string {
	if s.CircuitOpen {
		return s.Provider.String() + " [CIRCUIT OPEN]"
	}
	if s.ConsecutiveFailures > 0 {
		return fmt.Sprintf("%s [%d failures]", s.Provider.String(), s.ConsecutiveFailures)
	}
	return s.Provider.String() + " [healthy]"
}
