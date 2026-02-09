package policy

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// ReloadState represents the current state of the policy reloader
type ReloadState string

const (
	// ReloadStateIdle indicates the reloader is idle
	ReloadStateIdle ReloadState = "idle"
	// ReloadStateReloading indicates a reload is in progress
	ReloadStateReloading ReloadState = "reloading"
	// ReloadStateStopped indicates the reloader is stopped
	ReloadStateStopped ReloadState = "stopped"
)

// ReloadCallback is a function that is called when policy is reloaded
// The new policy is passed as an argument, allowing the caller to apply it
type ReloadCallback func(ctx context.Context, newPolicy *Policy) error

// Reloader manages policy reloading via SIGHUP signals
type Reloader struct {
	mu            sync.RWMutex
	policyPath    string
	currentPolicy *Policy
	state         ReloadState
	signalChan    chan os.Signal
	reloadCtx     context.Context
	reloadCancel  context.CancelFunc
	started       bool
	callbacks     []ReloadCallback
}

// NewReloader creates a new policy reloader
func NewReloader(policyPath string, initialPolicy *Policy) *Reloader {
	ctx, cancel := context.WithCancel(context.Background())

	return &Reloader{
		policyPath:    policyPath,
		currentPolicy: initialPolicy,
		state:         ReloadStateIdle,
		signalChan:    make(chan os.Signal, 1),
		reloadCtx:     ctx,
		reloadCancel:  cancel,
		started:       false,
		callbacks:     make([]ReloadCallback, 0),
	}
}

// Start begins listening for SIGHUP signals to trigger policy reload
func (r *Reloader) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return
	}

	// Reset context and state if restarting
	if r.state == ReloadStateStopped {
		ctx, cancel := context.WithCancel(context.Background())
		r.reloadCtx = ctx
		r.reloadCancel = cancel
		r.state = ReloadStateIdle
	}

	// Register signal handler for SIGHUP
	signal.Notify(r.signalChan, syscall.SIGHUP)

	r.started = true
	log.Printf("[policy_reloader] started, policy_path=%s", r.policyPath)

	// Start signal handler goroutine
	go r.handleSignals()
}

// Stop stops the policy reloader (cancels signal handling)
func (r *Reloader) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.started {
		return
	}

	signal.Stop(r.signalChan)
	r.reloadCancel()
	r.started = false
	r.state = ReloadStateStopped

	log.Print("[policy_reloader] stopped")
}

// Reload reloads the policy from the file
func (r *Reloader) Reload(ctx context.Context) error {
	r.mu.Lock()

	if r.state == ReloadStateReloading {
		r.mu.Unlock()
		log.Print("[policy_reloader] reload already in progress, skipping")
		return nil
	}

	r.state = ReloadStateReloading
	log.Printf("[policy_reloader] policy reload initiated, policy_path=%s", r.policyPath)
	r.mu.Unlock()

	// Load the new policy from the reloader's policy path
	newPolicy, err := LoadFromFile(r.policyPath)
	if err != nil {
		r.setState(ReloadStateIdle)
		return fmt.Errorf("failed to load policy from %s: %w", r.policyPath, err)
	}

	// Execute callbacks with the new policy
	if err := r.executeCallbacks(ctx, newPolicy); err != nil {
		log.Printf("[policy_reloader] reload callbacks failed: %v", err)
		r.setState(ReloadStateIdle)
		return fmt.Errorf("reload callbacks failed: %w", err)
	}

	// Update the current policy
	r.mu.Lock()
	r.currentPolicy = newPolicy
	r.state = ReloadStateIdle
	r.mu.Unlock()

	log.Print("[policy_reloader] policy reloaded successfully")

	return nil
}

// AddCallback adds a callback that will be called when policy is reloaded
func (r *Reloader) AddCallback(callback ReloadCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.callbacks = append(r.callbacks, callback)
	log.Printf("[policy_reloader] reload callback registered, total_callbacks=%d", len(r.callbacks))
}

// GetPolicy returns the current policy
func (r *Reloader) GetPolicy() *Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentPolicy
}

// State returns the current reload state
func (r *Reloader) State() ReloadState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

// IsReloading returns true if a reload is in progress
func (r *Reloader) IsReloading() bool {
	return r.State() == ReloadStateReloading
}

// handleSignals handles incoming SIGHUP signals
func (r *Reloader) handleSignals() {
	for {
		select {
		case sig := <-r.signalChan:
			log.Printf("[policy_reloader] reload signal received, signal=%v", sig)

			// Initiate reload in goroutine to avoid blocking signal handling
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30)
				defer cancel()
				if err := r.Reload(ctx); err != nil {
					log.Printf("[policy_reloader] policy reload failed: %v", err)
				}
			}()

		case <-r.reloadCtx.Done():
			log.Print("[policy_reloader] signal handler stopping")
			return
		}
	}
}

// executeCallbacks executes all registered reload callbacks
func (r *Reloader) executeCallbacks(ctx context.Context, newPolicy *Policy) error {
	r.mu.RLock()
	callbacks := make([]ReloadCallback, len(r.callbacks))
	copy(callbacks, r.callbacks)
	r.mu.RUnlock()

	log.Printf("[policy_reloader] executing reload callbacks, count=%d", len(callbacks))

	for i, callback := range callbacks {
		callbackName := fmt.Sprintf("callback-%d", i)
		log.Printf("[policy_reloader] executing reload callback, callback=%s", callbackName)

		if err := callback(ctx, newPolicy); err != nil {
			log.Printf("[policy_reloader] reload callback failed, callback=%s, error=%v", callbackName, err)
			return err
		}
	}

	return nil
}

// setState sets the reload state
func (r *Reloader) setState(state ReloadState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
	log.Printf("[policy_reloader] reload state changed, state=%s", state)
}

// String returns a string representation of the reload state
func (s ReloadState) String() string {
	return string(s)
}

// String returns a string representation of the reloader
func (r *Reloader) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return fmt.Sprintf("Reloader{state: %s, policy_path: %s, callbacks: %d}",
		r.state, r.policyPath, len(r.callbacks))
}
