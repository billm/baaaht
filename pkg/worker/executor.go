package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/policy"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Executor handles tool container execution with policy enforcement
type Executor struct {
	runtime        container.Runtime
	policyEnforcer *policy.Enforcer
	logger         *logger.Logger
	sessionID      types.ID
	mu             sync.RWMutex
	closed         bool
}

// ExecutorConfig holds configuration for creating a new Executor
type ExecutorConfig struct {
	Runtime        container.Runtime
	PolicyEnforcer *policy.Enforcer
	SessionID      types.ID
	Logger         *logger.Logger
}

// NewExecutor creates a new tool container executor
func NewExecutor(cfg ExecutorConfig) (*Executor, error) {
	// Validate required fields
	if cfg.Runtime == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime cannot be nil")
	}

	// Create default logger if not provided
	log := cfg.Logger
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Use default session ID if not provided
	sessionID := cfg.SessionID
	if sessionID.IsEmpty() {
		sessionID = types.GenerateID()
	}

	exec := &Executor{
		runtime:        cfg.Runtime,
		policyEnforcer: cfg.PolicyEnforcer,
		logger:         log.With("component", "worker_executor", "session_id", sessionID),
		sessionID:      sessionID,
		closed:         false,
	}

	exec.logger.Info("Executor initialized",
		"runtime_type", cfg.Runtime.Type(),
		"policy_enforcer", cfg.PolicyEnforcer != nil)

	return exec, nil
}

// NewExecutorFromRuntime creates a new executor with just a runtime
// This is a convenience function that creates an executor without a policy enforcer
func NewExecutorFromRuntime(runtime container.Runtime, log *logger.Logger) (*Executor, error) {
	if runtime == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime cannot be nil")
	}

	return NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Logger:  log,
	})
}

// NewExecutorDefault creates a new executor with a global runtime
// This creates a runtime with auto-detection if one doesn't exist
func NewExecutorDefault(log *logger.Logger) (*Executor, error) {
	ctx := context.Background()
	runtime := container.GlobalRuntime()

	// If the global runtime is an error runtime, try to create a new one
	if runtime.Type() == "error" {
		var err error
		runtime, err = container.NewRuntimeDefault(ctx)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default runtime", err)
		}
	}

	return NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Logger:  log,
	})
}

// SetPolicyEnforcer sets or updates the policy enforcer for the executor
func (e *Executor) SetPolicyEnforcer(enforcer *policy.Enforcer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policyEnforcer = enforcer

	if enforcer != nil {
		e.logger.Debug("Policy enforcer set")
	} else {
		e.logger.Debug("Policy enforcer cleared")
	}
}

// PolicyEnforcer returns the current policy enforcer
func (e *Executor) PolicyEnforcer() *policy.Enforcer {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.policyEnforcer
}

// Runtime returns the container runtime
func (e *Executor) Runtime() container.Runtime {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.runtime
}

// SessionID returns the session ID for this executor
func (e *Executor) SessionID() types.ID {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sessionID
}

// SetSessionID sets a new session ID for the executor
func (e *Executor) SetSessionID(sessionID types.ID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessionID = sessionID
	e.logger.Debug("Session ID updated", "session_id", sessionID)
}

// IsClosed returns true if the executor is closed
func (e *Executor) IsClosed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closed
}

// Close closes the executor and releases resources
func (e *Executor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}

	e.closed = true
	e.logger.Info("Executor closed")

	return nil
}

// String returns a string representation of the executor
func (e *Executor) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	enforcerStatus := "none"
	if e.policyEnforcer != nil {
		enforcerStatus = "enabled"
	}

	return fmt.Sprintf("Executor{Runtime: %s, PolicyEnforcer: %s, SessionID: %s, Closed: %v}",
		e.runtime.Type(), enforcerStatus, e.sessionID, e.closed)
}

// ValidateConfig validates a container configuration against the policy enforcer
func (e *Executor) ValidateConfig(ctx context.Context, config types.ContainerConfig) (*policy.ValidationResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "executor is closed")
	}

	// If no policy enforcer is set, allow everything
	if e.policyEnforcer == nil {
		e.logger.Debug("No policy enforcer set, allowing configuration")
		return &policy.ValidationResult{Allowed: true}, nil
	}

	// Validate against policy
	result, err := e.policyEnforcer.ValidateContainerConfig(ctx, e.sessionID, config)
	if err != nil {
		e.logger.Warn("Policy validation failed", "error", err)
		return nil, types.WrapError(types.ErrCodeInternal, "policy validation error", err)
	}

	// Log warnings
	for _, warning := range result.Warnings {
		e.logger.Warn("Policy validation warning",
			"rule", warning.Rule,
			"message", warning.Message,
			"component", warning.Component)
	}

	// Log violations
	for _, violation := range result.Violations {
		if violation.Severity == string(policy.SeverityError) {
			e.logger.Warn("Policy violation detected",
				"rule", violation.Rule,
				"message", violation.Message,
				"component", violation.Component)
		} else {
			e.logger.Info("Policy validation notice",
				"rule", violation.Rule,
				"message", violation.Message,
				"component", violation.Component)
		}
	}

	return result, nil
}

// EnforceConfig enforces policy on a container configuration and returns a modified config
func (e *Executor) EnforceConfig(ctx context.Context, config types.ContainerConfig) (types.ContainerConfig, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return config, types.NewError(types.ErrCodeUnavailable, "executor is closed")
	}

	// If no policy enforcer is set, return config as-is
	if e.policyEnforcer == nil {
		return config, nil
	}

	// Enforce policy
	enforced, err := e.policyEnforcer.EnforceContainerConfig(ctx, e.sessionID, config)
	if err != nil {
		e.logger.Warn("Policy enforcement failed", "error", err)
		return config, types.WrapError(types.ErrCodeInternal, "policy enforcement error", err)
	}

	return enforced, nil
}
