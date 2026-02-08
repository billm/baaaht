package container

import (
	"fmt"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Registry manages container runtime implementations
// It provides a centralized way to register, retrieve, and manage runtime instances
type Registry struct {
	mu       sync.RWMutex
	factories map[string]RuntimeFactory
	runtimes  map[string]Runtime
	logger   *logger.Logger
}

// NewRegistry creates a new runtime registry
func NewRegistry(log *logger.Logger) *Registry {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			// If we can't create a logger, continue without one
			log = &logger.Logger{}
		}
	}

	return &Registry{
		factories: make(map[string]RuntimeFactory),
		runtimes:  make(map[string]Runtime),
		logger:    log.With("component", "runtime_registry"),
	}
}

// RegisterRuntime registers a runtime factory for a given runtime type
// If a factory already exists for the type, it will be overwritten
func (r *Registry) RegisterRuntime(runtimeType string, factory RuntimeFactory) error {
	if runtimeType == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "runtime type cannot be empty")
	}
	if factory == nil {
		return types.NewError(types.ErrCodeInvalidArgument, "runtime factory cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[runtimeType] = factory
	r.logger.Debug("Runtime factory registered", "type", runtimeType)

	return nil
}

// UnregisterRuntime removes a runtime factory from the registry
// If a runtime instance exists, it will be closed before removal
func (r *Registry) UnregisterRuntime(runtimeType string) error {
	if runtimeType == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "runtime type cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Close and remove any existing runtime instance
	if rt, ok := r.runtimes[runtimeType]; ok {
		if err := rt.Close(); err != nil {
			r.logger.Warn("Failed to close runtime during unregistration", "type", runtimeType, "error", err)
		}
		delete(r.runtimes, runtimeType)
	}

	delete(r.factories, runtimeType)
	r.logger.Debug("Runtime factory unregistered", "type", runtimeType)

	return nil
}

// GetRuntime retrieves or creates a runtime instance for the given type
// If an instance already exists, it will be returned
// If no instance exists, a new one will be created using the registered factory
func (r *Registry) GetRuntime(runtimeType string, config RuntimeConfig) (Runtime, error) {
	if runtimeType == "" {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime type cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if we already have an instance
	if rt, ok := r.runtimes[runtimeType]; ok {
		// For DockerRuntime, verify the runtime is still valid (not closed)
		if drt, ok := rt.(*DockerRuntime); ok {
			if !drt.IsClosed() {
				return rt, nil
			}
			// Runtime is closed, remove it and create a new one
			delete(r.runtimes, runtimeType)
		} else {
			// For other runtime types, return the existing instance
			return rt, nil
		}
	}

	// Get the factory
	factory, ok := r.factories[runtimeType]
	if !ok {
		return nil, types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("no factory registered for runtime type: %s", runtimeType))
	}

	// Create a new runtime instance
	runtime, err := factory(config)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal,
			fmt.Sprintf("failed to create runtime of type: %s", runtimeType), err)
	}

	// Cache the instance
	r.runtimes[runtimeType] = runtime
	r.logger.Debug("Runtime instance created", "type", runtimeType)

	return runtime, nil
}

// GetRuntimeByType retrieves a runtime instance for the given RuntimeType
// This is a convenience method that converts RuntimeType to string
func (r *Registry) GetRuntimeByType(runtimeType types.RuntimeType, config RuntimeConfig) (Runtime, error) {
	return r.GetRuntime(string(runtimeType), config)
}

// GetExistingRuntime returns an existing runtime instance without creating a new one
// Returns nil if no instance exists
func (r *Registry) GetExistingRuntime(runtimeType string) (Runtime, error) {
	if runtimeType == "" {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime type cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.runtimes[runtimeType]
	if !ok {
		return nil, types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("no runtime instance found for type: %s", runtimeType))
	}

	// For DockerRuntime, verify the runtime is still valid
	if drt, ok := rt.(*DockerRuntime); ok && drt.IsClosed() {
		return nil, types.NewError(types.ErrCodeUnavailable,
			fmt.Sprintf("runtime instance for type %s is closed", runtimeType))
	}

	return rt, nil
}

// HasRuntime returns true if a factory is registered for the given runtime type
func (r *Registry) HasRuntime(runtimeType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.factories[runtimeType]
	return ok
}

// HasRuntimeInstance returns true if a runtime instance exists for the given type
func (r *Registry) HasRuntimeInstance(runtimeType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.runtimes[runtimeType]
	if !ok {
		return false
	}

	// Check if runtime is still valid
	if drt, ok := rt.(*DockerRuntime); ok {
		return !drt.IsClosed()
	}

	return true
}

// ListRuntimes returns a list of registered runtime types
func (r *Registry) ListRuntimes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}

// ListInstances returns a list of runtime types that have active instances
func (r *Registry) ListInstances() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := make([]string, 0, len(r.runtimes))
	for t, rt := range r.runtimes {
		// For DockerRuntime, only include non-closed instances
		if drt, ok := rt.(*DockerRuntime); ok {
			if !drt.IsClosed() {
				instances = append(instances, t)
			}
		} else {
			// Include non-Docker runtimes (we can't check if they're closed)
			instances = append(instances, t)
		}
	}
	return instances
}

// CloseRuntime closes a specific runtime instance
func (r *Registry) CloseRuntime(runtimeType string) error {
	if runtimeType == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "runtime type cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	rt, ok := r.runtimes[runtimeType]
	if !ok {
		return types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("no runtime instance found for type: %s", runtimeType))
	}

	err := rt.Close()
	delete(r.runtimes, runtimeType)

	if err != nil {
		return types.WrapError(types.ErrCodeInternal,
			fmt.Sprintf("failed to close runtime: %s", runtimeType), err)
	}

	r.logger.Debug("Runtime instance closed", "type", runtimeType)
	return nil
}

// Close closes all runtime instances and clears the registry
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []string

	// Close all runtime instances
	for runtimeType, rt := range r.runtimes {
		if err := rt.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", runtimeType, err))
		}
	}

	// Clear the registry
	r.runtimes = make(map[string]Runtime)
	r.factories = make(map[string]RuntimeFactory)

	if len(errs) > 0 {
		return types.NewError(types.ErrCodeInternal,
			fmt.Sprintf("failed to close some runtimes: %v", errs))
	}

	r.logger.Debug("Registry closed")
	return nil
}

// String returns a string representation of the registry
func (r *Registry) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return fmt.Sprintf("Registry{Factories: %d, Instances: %d}",
		len(r.factories), len(r.runtimes))
}

// globalRegistry is the default global registry instance
var globalRegistry *Registry
var globalRegistryOnce sync.Once

// GetGlobalRegistry returns the global runtime registry
// The registry is initialized on first call
func GetGlobalRegistry() *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewRegistry(nil)
	})
	return globalRegistry
}
