package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// ReloadState represents the current state of the config reloader
type ReloadState string

const (
	// ReloadStateIdle indicates the reloader is idle
	ReloadStateIdle ReloadState = "idle"
	// ReloadStateReloading indicates a reload is in progress
	ReloadStateReloading ReloadState = "reloading"
	// ReloadStateStopped indicates the reloader is stopped
	ReloadStateStopped ReloadState = "stopped"
)

// ReloadCallback is a function that is called when configuration is reloaded
// The new config is passed as an argument, allowing the caller to apply it
type ReloadCallback func(ctx context.Context, newConfig *Config) error

// Reloader manages configuration reloading via SIGHUP signals
type Reloader struct {
	mu             sync.RWMutex
	configPath     string
	currentConfig  *Config
	state          ReloadState
	signalChan     chan os.Signal
	reloadCtx      context.Context
	reloadCancel   context.CancelFunc
	started        bool
	callbacks      []ReloadCallback
}

// NewReloader creates a new config reloader
func NewReloader(configPath string, initialConfig *Config) *Reloader {
	ctx, cancel := context.WithCancel(context.Background())

	return &Reloader{
		configPath:    configPath,
		currentConfig: initialConfig,
		state:         ReloadStateIdle,
		signalChan:    make(chan os.Signal, 1),
		reloadCtx:     ctx,
		reloadCancel:  cancel,
		started:       false,
		callbacks:     make([]ReloadCallback, 0),
	}
}

// Start begins listening for SIGHUP signals to trigger config reload
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
	log.Printf("[config_reloader] started, config_path=%s", r.configPath)

	// Start signal handler goroutine
	go r.handleSignals()
}

// Stop stops the config reloader (cancels signal handling)
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

	log.Print("[config_reloader] stopped")
}

// Reload reloads the configuration from the file
func (r *Reloader) Reload(ctx context.Context) error {
	r.mu.Lock()

	if r.state == ReloadStateReloading {
		r.mu.Unlock()
		log.Print("[config_reloader] reload already in progress, skipping")
		return nil
	}

	r.state = ReloadStateReloading
	log.Printf("[config_reloader] configuration reload initiated, config_path=%s", r.configPath)
	r.mu.Unlock()

	// Load the new configuration using the same logic as the initial load,
	// ensuring environment variable overrides are applied.
	newConfig, err := Load(r.configPath)
	if err != nil {
		r.setState(ReloadStateIdle)
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Execute callbacks with the new config
	if err := r.executeCallbacks(ctx, newConfig); err != nil {
		log.Printf("[config_reloader] reload callbacks failed: %v", err)
		r.setState(ReloadStateIdle)
		return fmt.Errorf("reload callbacks failed: %w", err)
	}

	// Update the current config
	r.mu.Lock()
	r.currentConfig = newConfig
	r.state = ReloadStateIdle
	r.mu.Unlock()

	log.Print("[config_reloader] configuration reloaded successfully")

	return nil
}

// AddCallback adds a callback that will be called when config is reloaded
func (r *Reloader) AddCallback(callback ReloadCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.callbacks = append(r.callbacks, callback)
	log.Printf("[config_reloader] reload callback registered, total_callbacks=%d", len(r.callbacks))
}

// GetConfig returns the current configuration
func (r *Reloader) GetConfig() *Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentConfig
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
			log.Printf("[config_reloader] reload signal received, signal=%v", sig)

			// Initiate reload in goroutine to avoid blocking signal handling
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30)
				defer cancel()
				if err := r.Reload(ctx); err != nil {
					log.Printf("[config_reloader] configuration reload failed: %v", err)
				}
			}()

		case <-r.reloadCtx.Done():
			log.Print("[config_reloader] signal handler stopping")
			return
		}
	}
}

// executeCallbacks executes all registered reload callbacks
func (r *Reloader) executeCallbacks(ctx context.Context, newConfig *Config) error {
	r.mu.RLock()
	callbacks := make([]ReloadCallback, len(r.callbacks))
	copy(callbacks, r.callbacks)
	r.mu.RUnlock()

	log.Printf("[config_reloader] executing reload callbacks, count=%d", len(callbacks))

	for i, callback := range callbacks {
		callbackName := fmt.Sprintf("callback-%d", i)
		log.Printf("[config_reloader] executing reload callback, callback=%s", callbackName)

		if err := callback(ctx, newConfig); err != nil {
			log.Printf("[config_reloader] reload callback failed, callback=%s, error=%v", callbackName, err)
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
	log.Printf("[config_reloader] reload state changed, state=%s", state)
}

// String returns a string representation of the reload state
func (s ReloadState) String() string {
	return string(s)
}

// String returns a string representation of the reloader
func (r *Reloader) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return fmt.Sprintf("Reloader{state: %s, config_path: %s, callbacks: %d}",
		r.state, r.configPath, len(r.callbacks))
}
