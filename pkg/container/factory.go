package container

import (
	"context"
	"fmt"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// NewRuntime creates a new Runtime instance based on the provided configuration
// The runtime type is determined by cfg.Type:
// - "auto": Automatically detect the best available runtime
// - "docker": Create a Docker runtime
// - "apple": Create an Apple Containers runtime (macOS only)
//
// If cfg.Type is "auto", the factory will:
// 1. On Linux: prefer Docker
// 2. On macOS: prefer Apple Containers if available, otherwise Docker
// 3. On other platforms: try Docker as fallback
//
// The cfg.Logger is used for runtime operations. If nil, a default logger will be created.
// The cfg.Timeout is used for connection attempts. If zero, a sensible default is used.
func NewRuntime(ctx context.Context, cfg RuntimeConfig) (Runtime, error) {
	// Validate config
	if cfg.Type == "" {
		cfg.Type = string(types.RuntimeTypeAuto)
	}

	// Determine the runtime type to use
	runtimeType := cfg.Type

	// Handle auto-detection
	if runtimeType == string(types.RuntimeTypeAuto) {
		detectedType := DetectRuntime(ctx)
		runtimeType = string(detectedType)

		// If no runtime is available, return an error
		if runtimeType == string(types.RuntimeTypeAuto) {
			return nil, types.NewError(types.ErrCodeUnavailable,
				"no container runtime available; please install Docker or another supported runtime")
		}
	}

	// Create the appropriate runtime based on type
	switch runtimeType {
	case string(types.RuntimeTypeDocker):
		return newDockerRuntimeFromConfig(cfg)
	case string(types.RuntimeTypeAppleContainers):
		return newAppleRuntimeFromConfig(cfg)
	default:
		// Check if a custom runtime factory is registered
		factory, ok := GetRuntimeFactory(runtimeType)
		if ok {
			return factory(cfg)
		}
		return nil, types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("unsupported runtime type: %s", runtimeType))
	}
}

// NewRuntimeDefault creates a new Runtime instance with auto-detection
// This is a convenience function that uses default configuration and auto-detects the best runtime
func NewRuntimeDefault(ctx context.Context) (Runtime, error) {
	log, err := logger.NewDefault()
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
	}

	cfg := RuntimeConfig{
		Type:    string(types.RuntimeTypeAuto),
		Timeout: 30 * time.Second,
		Logger:  log,
	}

	return NewRuntime(ctx, cfg)
}

// newDockerRuntimeFromConfig creates a Docker runtime from RuntimeConfig
func newDockerRuntimeFromConfig(cfg RuntimeConfig) (*DockerRuntime, error) {
	// Create logger
	var log *logger.Logger
	if cfg.Logger != nil {
		if l, ok := cfg.Logger.(*logger.Logger); ok {
			log = l
		} else {
			// Logger is not the expected type, create a new one
			var err error
			log, err = logger.NewDefault()
			if err != nil {
				return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
			}
		}
	} else {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Convert RuntimeConfig to DockerConfig
	dockerCfg := config.DockerConfig{
		Host:       cfg.Endpoint,
		APIVersion: "1.44",
	}

	// Set timeout
	if cfg.Timeout > 0 {
		dockerCfg.Timeout = cfg.Timeout
	} else {
		dockerCfg.Timeout = config.DefaultRuntimeConfig().Timeout
	}

	// Set TLS settings from options if provided
	if cfg.Options != nil {
		if tlsCert, ok := cfg.Options["tls_cert"].(string); ok {
			dockerCfg.TLSCert = tlsCert
		}
		if tlsKey, ok := cfg.Options["tls_key"].(string); ok {
			dockerCfg.TLSKey = tlsKey
		}
		if tlsCA, ok := cfg.Options["tls_ca"].(string); ok {
			dockerCfg.TLSCACert = tlsCA
		}
		if tlsVerify, ok := cfg.Options["tls_verify"].(bool); ok {
			dockerCfg.TLSVerify = tlsVerify
		}
	}

	// Use default host if not specified
	if dockerCfg.Host == "" {
		dockerCfg.Host = config.DefaultDockerConfig().Host
	}

	// Create the Docker runtime
	dockerRuntime, err := NewDockerRuntime(dockerCfg, log)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create Docker runtime", err)
	}

	return dockerRuntime, nil
}

// InitializeRuntimes registers the default runtime factories with the global registry
// This function should be called during application initialization to ensure
// that the standard runtime types are available
func InitializeRuntimes(log *logger.Logger) error {
	reg := GetGlobalRegistry()

	// Register Docker runtime factory
	dockerFactory := func(cfg RuntimeConfig) (Runtime, error) {
		return newDockerRuntimeFromConfig(cfg)
	}
	if err := reg.RegisterRuntime(string(types.RuntimeTypeDocker), dockerFactory); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to register Docker runtime factory", err)
	}

	// Register Apple Containers runtime factory
	appleFactory := func(cfg RuntimeConfig) (Runtime, error) {
		return newAppleRuntimeFromConfig(cfg)
	}
	if err := reg.RegisterRuntime(string(types.RuntimeTypeAppleContainers), appleFactory); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to register Apple Containers runtime factory", err)
	}

	if log != nil {
		log.Debug("Runtime factories initialized", "runtimes", []string{
			string(types.RuntimeTypeDocker),
			string(types.RuntimeTypeAppleContainers),
		})
	}

	return nil
}

// GetRuntimeFromRegistry retrieves a runtime from the global registry
// If the runtime hasn't been created yet, it will be created using the registered factory
// This is the preferred way to get runtime instances in production code
func GetRuntimeFromRegistry(ctx context.Context, runtimeType string, cfg RuntimeConfig) (Runtime, error) {
	// Initialize runtimes if not already done
	if !GetGlobalRegistry().HasRuntime(runtimeType) {
		if err := InitializeRuntimes(nil); err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to initialize runtimes", err)
		}
	}

	// Get the runtime from the registry
	reg := GetGlobalRegistry()
	rt, err := reg.GetRuntime(runtimeType, cfg)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal,
			fmt.Sprintf("failed to get runtime from registry: %s", runtimeType), err)
	}

	return rt, nil
}

// CreateRuntimeWithConfig creates a runtime with the specified configuration
// This is a convenience function that combines NewRuntime with error handling
// and is suitable for use in application initialization code
func CreateRuntimeWithConfig(ctx context.Context, runtimeType string, log *logger.Logger) (Runtime, error) {
	cfg := RuntimeConfig{
		Type:    runtimeType,
		Logger:  log,
		Timeout: 30 * time.Second,
	}

	return NewRuntime(ctx, cfg)
}

// RuntimeFactoryFunc is a function that creates a Runtime instance
// This type is used for registering custom runtime factories
type RuntimeFactoryFunc func(cfg RuntimeConfig) (Runtime, error)

// RegisterCustomRuntime registers a custom runtime factory with the global registry
// This allows extending the orchestrator with custom runtime implementations
func RegisterCustomRuntime(runtimeType string, factory RuntimeFactoryFunc) error {
	reg := GetGlobalRegistry()

	if err := reg.RegisterRuntime(runtimeType, RuntimeFactory(factory)); err != nil {
		return types.WrapError(types.ErrCodeInternal,
			fmt.Sprintf("failed to register custom runtime factory: %s", runtimeType), err)
	}

	return nil
}

// AvailableRuntimes returns a list of runtime types that are available on the current system
// This checks both registered factories and runtime availability
func AvailableRuntimes(ctx context.Context) []string {
	reg := GetGlobalRegistry()
	registered := reg.ListRuntimes()

	var available []string
	for _, rt := range registered {
		switch rt {
		case string(types.RuntimeTypeDocker):
			if IsDockerAvailable(ctx) {
				available = append(available, rt)
			}
		case string(types.RuntimeTypeAppleContainers):
			if IsAppleContainersAvailable() {
				available = append(available, rt)
			}
		default:
			// For custom runtimes, assume they're available if registered
			available = append(available, rt)
		}
	}

	return available
}

// DetectBestRuntime returns the best available runtime type for the current system
// This is a convenience wrapper around DetectRuntime that ensures runtimes are initialized
func DetectBestRuntime(ctx context.Context) types.RuntimeType {
	// Initialize runtimes to ensure factories are registered
	_ = InitializeRuntimes(nil)

	return DetectRuntime(ctx)
}
