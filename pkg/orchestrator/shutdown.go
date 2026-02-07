package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// ShutdownState represents the current state of the shutdown process
type ShutdownState string

const (
	// ShutdownStateRunning indicates the orchestrator is running normally
	ShutdownStateRunning ShutdownState = "running"
	// ShutdownStateInitiated indicates shutdown has been initiated
	ShutdownStateInitiated ShutdownState = "initiated"
	// ShutdownStateStopping indicates subsystems are being stopped
	ShutdownStateStopping ShutdownState = "stopping"
	// ShutdownStateComplete indicates shutdown is complete
	ShutdownStateComplete ShutdownState = "complete"
)

// ShutdownHook is a function that can be called during shutdown
type ShutdownHook func(ctx context.Context) error

// ShutdownManager manages the graceful shutdown process
type ShutdownManager struct {
	mu              sync.RWMutex
	orch            *Orchestrator
	state           ShutdownState
	shutdownTimeout time.Duration
	hooks           []ShutdownHook
	logger          *logger.Logger
	signalChan      chan os.Signal
	shutdownCtx     context.Context
	shutdownCancel  context.CancelFunc
	started         bool
	completionChan  chan struct{}
	shutdownReason  string
}

// NewShutdownManager creates a new shutdown manager
func NewShutdownManager(orch *Orchestrator, timeout time.Duration, log *logger.Logger) *ShutdownManager {
	if log == nil {
		// Create a default logger if none provided - this should rarely happen
		log, _ = logger.NewDefault()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ShutdownManager{
		orch:            orch,
		state:           ShutdownStateRunning,
		shutdownTimeout: timeout,
		hooks:           make([]ShutdownHook, 0),
		logger:          log.With("component", "shutdown_manager"),
		signalChan:      make(chan os.Signal, 1),
		shutdownCtx:     ctx,
		shutdownCancel:  cancel,
		started:         false,
		completionChan:  make(chan struct{}),
		shutdownReason:  "",
	}
}

// Start begins listening for shutdown signals
func (sm *ShutdownManager) Start() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.started {
		return
	}

	// Register signal handlers for common shutdown signals
	signals := []os.Signal{
		syscall.SIGINT,
		syscall.SIGTERM,
	}
	// SIGQUIT is not available on Windows
	// Add build tag specific signals in platform files if needed
	// syscall.SIGQUIT is available on Unix-like systems

	signal.Notify(sm.signalChan, signals...)

	sm.started = true
	sm.logger.Info("Shutdown manager started",
		"timeout", sm.shutdownTimeout,
		"signals", len(signals))

	// Start signal handler goroutine
	go sm.handleSignals()
}

// Stop stops the shutdown manager (cancels signal handling)
func (sm *ShutdownManager) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.started {
		return
	}

	signal.Stop(sm.signalChan)
	sm.shutdownCancel()
	sm.started = false

	sm.logger.Debug("Shutdown manager stopped")
}

// Shutdown initiates a graceful shutdown with the specified reason
func (sm *ShutdownManager) Shutdown(ctx context.Context, reason string) error {
	sm.mu.Lock()

	if sm.state != ShutdownStateRunning {
		sm.mu.Unlock()
		return types.NewError(types.ErrCodeFailedPrecondition, "shutdown already initiated")
	}

	sm.state = ShutdownStateInitiated
	sm.shutdownReason = reason
	sm.logger.Info("Shutdown initiated", "reason", reason)
	sm.mu.Unlock()

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, sm.shutdownTimeout)
	defer cancel()

	// Execute pre-shutdown hooks
	if err := sm.executeHooks(shutdownCtx, "pre-shutdown"); err != nil {
		sm.logger.Error("Pre-shutdown hooks failed", "error", err)
	}

	// Stop accepting new work
	sm.setState(ShutdownStateStopping)

	// Close the orchestrator
	if err := sm.orch.Close(); err != nil {
		sm.logger.Error("Orchestrator close failed", "error", err)
		// Continue with shutdown anyway
	}

	// Execute post-shutdown hooks
	if err := sm.executeHooks(shutdownCtx, "post-shutdown"); err != nil {
		sm.logger.Error("Post-shutdown hooks failed", "error", err)
	}

	// Mark shutdown as complete
	sm.setState(ShutdownStateComplete)
	close(sm.completionChan)

	sm.logger.Info("Shutdown complete", "reason", reason, "duration", time.Since(sm.getStartTime()))

	return nil
}

// ShutdownAndWait initiates shutdown and waits for completion
func (sm *ShutdownManager) ShutdownAndWait(ctx context.Context, reason string) error {
	// Launch shutdown in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- sm.Shutdown(ctx, reason)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return types.WrapError(types.ErrCodeCanceled, "shutdown wait canceled", ctx.Err())
	case <-sm.completionChan:
		return nil
	}
}

// AddHook adds a shutdown hook that will be called during shutdown
func (sm *ShutdownManager) AddHook(hook ShutdownHook) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.hooks = append(sm.hooks, hook)
	sm.logger.Debug("Shutdown hook registered", "total_hooks", len(sm.hooks))
}

// State returns the current shutdown state
func (sm *ShutdownManager) State() ShutdownState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

// IsShuttingDown returns true if shutdown has been initiated
func (sm *ShutdownManager) IsShuttingDown() bool {
	state := sm.State()
	return state == ShutdownStateInitiated || state == ShutdownStateStopping || state == ShutdownStateComplete
}

// IsComplete returns true if shutdown is complete
func (sm *ShutdownManager) IsComplete() bool {
	return sm.State() == ShutdownStateComplete
}

// ShutdownReason returns the reason for shutdown
func (sm *ShutdownManager) ShutdownReason() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.shutdownReason
}

// WaitCompletion waits for shutdown to complete
func (sm *ShutdownManager) WaitCompletion(ctx context.Context) error {
	select {
	case <-sm.completionChan:
		return nil
	case <-ctx.Done():
		return types.WrapError(types.ErrCodeCanceled, "wait for completion canceled", ctx.Err())
	}
}

// Context returns the shutdown context
func (sm *ShutdownManager) Context() context.Context {
	return sm.shutdownCtx
}

// handleSignals handles incoming shutdown signals
func (sm *ShutdownManager) handleSignals() {
	for {
		select {
		case sig := <-sm.signalChan:
			reason := fmt.Sprintf("signal received: %s", sig)
			sm.logger.Info("Shutdown signal received", "signal", sig)

			// Initiate shutdown in goroutine to avoid blocking signal handling
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), sm.shutdownTimeout)
				defer cancel()
				if err := sm.ShutdownAndWait(ctx, reason); err != nil {
					sm.logger.Error("Shutdown failed", "error", err)
				}
			}()

		case <-sm.shutdownCtx.Done():
			sm.logger.Debug("Signal handler stopping")
			return
		}
	}
}

// executeHooks executes all registered shutdown hooks
func (sm *ShutdownManager) executeHooks(ctx context.Context, phase string) error {
	sm.mu.RLock()
	hooks := make([]ShutdownHook, len(sm.hooks))
	copy(hooks, sm.hooks)
	sm.mu.RUnlock()

	sm.logger.Debug("Executing shutdown hooks", "phase", phase, "count", len(hooks))

	var errors []error
	for i, hook := range hooks {
		hookCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		hookName := fmt.Sprintf("hook-%d", i)

		sm.logger.Debug("Executing shutdown hook", "phase", phase, "hook", hookName)

		func() {
			defer cancel()
			if err := hook(hookCtx); err != nil {
				sm.logger.Error("Shutdown hook failed",
					"phase", phase,
					"hook", hookName,
					"error", err)
				errors = append(errors, err)
			}
		}()

		// Check if context was canceled
		select {
		case <-ctx.Done():
			sm.logger.Warn("Shutdown hook execution canceled", "phase", phase)
			return types.WrapError(types.ErrCodeCanceled, "hook execution canceled", ctx.Err())
		default:
		}
	}

	if len(errors) > 0 {
		return types.WrapError(types.ErrCodePartialFailure, fmt.Sprintf("%s hooks failed", phase), errors[0])
	}

	return nil
}

// setState sets the shutdown state
func (sm *ShutdownManager) setState(state ShutdownState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state = state
	sm.logger.Debug("Shutdown state changed", "state", state)
}

// getStartTime returns the time when shutdown was initiated
func (sm *ShutdownManager) getStartTime() time.Time {
	// This is a simplified version - in production you'd store the actual start time
	return time.Now()
}

// ShutdownGracefully shuts down the orchestrator gracefully with a timeout
func ShutdownGracefully(orch *Orchestrator, timeout time.Duration) error {
	if orch == nil {
		return types.NewError(types.ErrCodeInvalidArgument, "orchestrator is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return orch.Close()
}

// ShutdownWithSignalHandling creates a shutdown manager and starts signal handling
func ShutdownWithSignalHandling(orch *Orchestrator, timeout time.Duration, log *logger.Logger) (*ShutdownManager, error) {
	if orch == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "orchestrator is nil")
	}

	sm := NewShutdownManager(orch, timeout, log)
	sm.Start()

	return sm, nil
}

// ShutdownOnContextCancel initiates shutdown when the context is canceled
func ShutdownOnContextCancel(orch *Orchestrator, ctx context.Context, log *logger.Logger) (*ShutdownManager, error) {
	if orch == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "orchestrator is nil")
	}

	sm := NewShutdownManager(orch, 30*time.Second, log)

	go func() {
		<-ctx.Done()
		reason := fmt.Sprintf("context canceled: %v", ctx.Err())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), sm.shutdownTimeout)
		defer cancel()
		if err := sm.ShutdownAndWait(shutdownCtx, reason); err != nil {
			log.Error("Context-based shutdown failed", "error", err)
		}
	}()

	return sm, nil
}

// String returns a string representation of the shutdown state
func (s ShutdownState) String() string {
	return string(s)
}

// String returns a string representation of the shutdown manager
func (sm *ShutdownManager) String() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return fmt.Sprintf("ShutdownManager{state: %s, timeout: %v, hooks: %d, started: %t}",
		sm.state, sm.shutdownTimeout, len(sm.hooks), sm.started)
}
