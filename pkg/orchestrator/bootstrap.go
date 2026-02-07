package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/baaaht/orchestrator/internal/config"
	"github.com/baaaht/orchestrator/internal/logger"
	"github.com/baaaht/orchestrator/pkg/types"
)

const (
	// DefaultVersion is the default version of the orchestrator
	DefaultVersion = "0.1.0"
	// DefaultShutdownTimeout is the default timeout for graceful shutdown
	DefaultShutdownTimeout = 30 * time.Second
)

// BootstrapResult contains the result of a bootstrap operation
type BootstrapResult struct {
	Orchestrator *Orchestrator
	StartedAt    time.Time
	Version      string
	Error        error
}

// BootstrapConfig contains configuration for the bootstrap process
type BootstrapConfig struct {
	Config           config.Config
	Logger           *logger.Logger
	Version          string
	ShutdownTimeout  time.Duration
	EnableHealthCheck bool
	HealthCheckInterval time.Duration
}

// NewDefaultBootstrapConfig creates a default bootstrap configuration
func NewDefaultBootstrapConfig() BootstrapConfig {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	log, err := logger.NewDefault()
	if err != nil {
		log = nil
	}

	return BootstrapConfig{
		Config:            *cfg,
		Logger:            log,
		Version:           DefaultVersion,
		ShutdownTimeout:   DefaultShutdownTimeout,
		EnableHealthCheck: true,
		HealthCheckInterval: 30 * time.Second,
	}
}

// Bootstrap creates and initializes a new orchestrator with the specified configuration
// It performs the following steps:
// 1. Creates the orchestrator instance
// 2. Initializes all subsystems
// 3. Performs health checks (if enabled)
// 4. Returns the initialized orchestrator
func Bootstrap(ctx context.Context, cfg BootstrapConfig) (*BootstrapResult, error) {
	startedAt := time.Now()

	result := &BootstrapResult{
		StartedAt: startedAt,
		Version:   cfg.Version,
	}

	// Validate configuration
	if err := cfg.Config.Validate(); err != nil {
		result.Error = types.WrapError(types.ErrCodeInvalidArgument, "invalid configuration", err)
		return result, result.Error
	}

	// Create orchestrator
	orch, err := New(cfg.Config, cfg.Logger)
	if err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to create orchestrator", err)
		return result, result.Error
	}
	result.Orchestrator = orch

	// Initialize orchestrator
	if err := orch.Initialize(ctx); err != nil {
		result.Error = types.WrapError(types.ErrCodeInternal, "failed to initialize orchestrator", err)
		// Cleanup on initialization failure
		_ = orch.Close()
		return result, result.Error
	}

	// Perform health check if enabled
	if cfg.EnableHealthCheck {
		healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		health := orch.HealthCheck(healthCtx)
		unhealthy := false
		for subsystem, status := range health {
			if status != types.Healthy {
				orch.Logger().Error("Subsystem health check failed",
					"subsystem", subsystem,
					"status", status)
				unhealthy = true
			}
		}

		if unhealthy {
			result.Error = types.NewError(types.ErrCodeInternal, "orchestrator health check failed")
			_ = orch.Close()
			return result, result.Error
		}
	}

	orch.Logger().Info("Orchestrator bootstrapped successfully",
		"version", cfg.Version,
		"duration", time.Since(startedAt))

	return result, nil
}

// BootstrapWithDefaults creates and initializes a new orchestrator with default configuration
func BootstrapWithDefaults(ctx context.Context) (*BootstrapResult, error) {
	cfg := NewDefaultBootstrapConfig()
	return Bootstrap(ctx, cfg)
}

// BootstrapOrchestrator is a simplified bootstrap function that returns only the orchestrator
func BootstrapOrchestrator(ctx context.Context, cfg config.Config, log *logger.Logger) (*Orchestrator, error) {
	bootstrapCfg := BootstrapConfig{
		Config:            cfg,
		Logger:            log,
		Version:           DefaultVersion,
		ShutdownTimeout:   DefaultShutdownTimeout,
		EnableHealthCheck: true,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := Bootstrap(ctx, bootstrapCfg)
	if err != nil {
		return nil, err
	}
	return result.Orchestrator, nil
}

// IsReady checks if the orchestrator is ready to accept requests
func IsReady(ctx context.Context, orch *Orchestrator) bool {
	if orch == nil {
		return false
	}

	if !orch.IsStarted() || orch.IsClosed() {
		return false
	}

	// Perform a quick health check
	health := orch.HealthCheck(ctx)
	for _, status := range health {
		if status != types.Healthy {
			return false
		}
	}

	return true
}

// WaitForReady waits for the orchestrator to be ready with a timeout
func WaitForReady(ctx context.Context, orch *Orchestrator, timeout time.Duration, checkInterval time.Duration) error {
	if orch == nil {
		return types.NewError(types.ErrCodeInvalidArgument, "orchestrator is nil")
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
			return types.WrapError(types.ErrCodeTimeout, "orchestrator not ready within timeout", ctx.Err())
		case <-ticker.C:
			if IsReady(ctx, orch) {
				return nil
			}
		}
	}
}

// GetVersion returns the version of the orchestrator
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

// Global orchestrator instance
var (
	globalOrchestrator *Orchestrator
	globalBootstrapMu  sync.RWMutex
	globalOnce         sync.Once
)

// InitGlobal initializes the global orchestrator with the specified configuration
func InitGlobal(ctx context.Context, cfg config.Config, log *logger.Logger) error {
	var initErr error
	globalOnce.Do(func() {
		result, err := Bootstrap(ctx, BootstrapConfig{
			Config:            cfg,
			Logger:            log,
			Version:           DefaultVersion,
			ShutdownTimeout:   DefaultShutdownTimeout,
			EnableHealthCheck: true,
			HealthCheckInterval: 30 * time.Second,
		})
		if err != nil {
			initErr = err
			return
		}
		globalBootstrapMu.Lock()
		globalOrchestrator = result.Orchestrator
		globalBootstrapMu.Unlock()
	})
	return initErr
}

// InitGlobalWithDefaults initializes the global orchestrator with default configuration
func InitGlobalWithDefaults(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to load configuration", err)
	}

	log, err := logger.NewDefault()
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create logger", err)
	}

	return InitGlobal(ctx, *cfg, log)
}

// Global returns the global orchestrator instance
func Global() *Orchestrator {
	globalBootstrapMu.RLock()
	orch := globalOrchestrator
	globalBootstrapMu.RUnlock()

	if orch == nil {
		// Initialize with default settings if not already initialized
		ctx := context.Background()
		if err := InitGlobalWithDefaults(ctx); err != nil {
			// Return nil on error
			return nil
		}
		globalBootstrapMu.RLock()
		orch = globalOrchestrator
		globalBootstrapMu.RUnlock()
	}

	return orch
}

// SetGlobal sets the global orchestrator instance
func SetGlobal(orch *Orchestrator) {
	globalBootstrapMu.Lock()
	globalOrchestrator = orch
	globalBootstrapMu.Unlock()
	globalOnce = sync.Once{}
}

// IsGlobalInitialized returns true if the global orchestrator has been initialized
func IsGlobalInitialized() bool {
	globalBootstrapMu.RLock()
	defer globalBootstrapMu.RUnlock()
	return globalOrchestrator != nil
}

// CloseGlobal closes and clears the global orchestrator instance
func CloseGlobal() error {
	globalBootstrapMu.Lock()
	defer globalBootstrapMu.Unlock()

	if globalOrchestrator == nil {
		return nil
	}

	if err := globalOrchestrator.Close(); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to close global orchestrator", err)
	}

	globalOrchestrator = nil
	globalOnce = sync.Once{}

	return nil
}

// String returns a string representation of the bootstrap result
func (r *BootstrapResult) String() string {
	if r.Error != nil {
		return fmt.Sprintf("BootstrapResult{version: %s, error: %v}", r.Version, r.Error)
	}
	return fmt.Sprintf("BootstrapResult{version: %s, started_at: %s, orchestrator: %v}",
		r.Version, r.StartedAt.Format(time.RFC3339), r.Orchestrator != nil)
}

// IsSuccessful returns true if the bootstrap was successful
func (r *BootstrapResult) IsSuccessful() bool {
	return r.Error == nil && r.Orchestrator != nil
}

// Duration returns the duration of the bootstrap process
func (r *BootstrapResult) Duration() time.Duration {
	return time.Since(r.StartedAt)
}
