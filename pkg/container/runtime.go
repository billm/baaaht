// Package container provides a unified interface for container runtime management.
//
// This package abstracts different container runtimes (Docker, Apple Containers, etc.)
// behind a common Runtime interface, allowing the orchestrator to work with multiple
// container backends without changing application code.
//
// # Core Concepts
//
// ## Runtime Interface
//
// The Runtime interface is the core abstraction. It provides methods for:
//   - Container lifecycle (Create, Start, Stop, Restart, Destroy)
//   - Monitoring (Status, Stats, Logs, Events)
//   - Health checks and resource usage tracking
//
// ## Runtime Factory
//
// Runtime instances are created through a factory pattern:
//   - NewRuntimeDefault() creates a runtime with auto-detection
//   - NewRuntime() creates a runtime with explicit configuration
//   - RegisterRuntime() registers custom runtime implementations
//
// # Basic Usage
//
//	// Create a runtime with auto-detection
//	runtime, err := container.NewRuntimeDefault(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer runtime.Close()
//
//	// Create and start a container
//	result, err := runtime.Create(ctx, container.CreateConfig{
//	    Config: types.ContainerConfig{
//	        Image: "nginx:latest",
//	        Env:   map[string]string{"FOO": "bar"},
//	    },
//	    Name:      "my-container",
//	    SessionID: types.NewID(),
//	    AutoPull:  true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	err = runtime.Start(ctx, container.StartConfig{
//	    ContainerID: result.ContainerID,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Runtime Detection
//
// The package supports automatic runtime detection based on the platform:
//   - Linux: Prefers Docker
//   - macOS: Prefers Apple Containers if available, otherwise Docker
//   - Windows: Tries Docker as fallback
//
// Use DetectRuntime() to check available runtimes or NewRuntimeDefault()
// to automatically use the best available runtime.
//
// # Custom Runtimes
//
// You can register custom runtime implementations:
//
//	container.RegisterRuntime("custom", func(cfg container.RuntimeConfig) (container.Runtime, error) {
//	    return &MyCustomRuntime{cfg: cfg}, nil
//	})
//
// # Thread Safety
//
// The Runtime interface implementations are generally safe for concurrent use.
// However, individual operations are not atomic. For complex multi-step
// operations, use external synchronization.
package container

import (
	"context"
	"io"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Runtime defines the interface for container runtime implementations.
//
// Different runtimes (Docker, Apple Containers, etc.) implement this interface
// to provide a unified API for container management. This abstraction allows
// the orchestrator to work with different container backends without changing
// application code.
//
// # Lifecycle Management
//
// The Runtime interface provides methods for the complete container lifecycle:
//   - Create, Start, Stop, Restart, Destroy: Core lifecycle operations
//   - Pause, Unpause, Kill: Additional control operations
//   - Wait: Block until a container exits
//
// # Monitoring and Observability
//
// The interface provides several methods for monitoring containers:
//   - Status, IsRunning: Check container state
//   - HealthCheck, HealthCheckWithRetry: Check container health
//   - Stats, StatsStream: Get resource usage statistics
//   - Logs, LogsLines: Retrieve container logs
//   - EventsStream: Subscribe to container events
//
// # Example: Creating a Container
//
//	// Create a new runtime with auto-detection
//	runtime, err := container.NewRuntimeDefault(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer runtime.Close()
//
//	// Create a container
//	result, err := runtime.Create(ctx, container.CreateConfig{
//	    Config: types.ContainerConfig{
//	        Image: "nginx:latest",
//	        Env: map[string]string{
//	            "FOO": "bar",
//	        },
//	    },
//	    Name:      "my-container",
//	    SessionID: types.NewID(),
//	    AutoPull:  true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Start the container
//	err = runtime.Start(ctx, container.StartConfig{
//	    ContainerID: result.ContainerID,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Example: Monitoring Container Stats
//
//	// Get a stream of container stats
//	statsChan, errChan := runtime.StatsStream(ctx, containerID, 1*time.Second)
//
//	for {
//	    select {
//	    case stats, ok := <-statsChan:
//	        if !ok {
//	            return
//	        }
//	        fmt.Printf("CPU: %.2f%%, Memory: %s\n",
//	            stats.CPUPercentage,
//	            formatBytes(stats.MemoryUsage))
//	    case err, ok := <-errChan:
//	        if !ok {
//	            return
//	        }
//	        log.Printf("Error: %v", err)
//	    }
//	}
type Runtime interface {
	// Create creates a new container with the specified configuration.
	//
	// The container is created but not started. Use Start to begin execution.
	// If AutoPull is true and the image doesn't exist locally, it will be
	// pulled automatically before creation.
	Create(ctx context.Context, cfg CreateConfig) (*CreateResult, error)

	// Start starts a container.
	//
	// The container must have been previously created. This method begins
	// execution of the container's main process.
	Start(ctx context.Context, cfg StartConfig) error

	// Stop stops a container gracefully.
	//
	// The container is sent a SIGTERM signal and given time to clean up.
	// Use Timeout to control how long to wait before forcefully terminating.
	Stop(ctx context.Context, cfg StopConfig) error

	// Restart restarts a container.
	//
	// This is equivalent to Stop followed by Start, but is performed as
	// a single atomic operation.
	Restart(ctx context.Context, cfg RestartConfig) error

	// Destroy removes a container.
	//
	// The container is stopped if running and then removed. Any resources
	// associated with the container are cleaned up.
	Destroy(ctx context.Context, cfg DestroyConfig) error

	// Pause pauses a container.
	//
	// All processes in the container are suspended using the cgroup freezer.
	Pause(ctx context.Context, containerID string) error

	// Unpause unpauses a container.
	//
	// All processes in the container are resumed.
	Unpause(ctx context.Context, containerID string) error

	// Kill forcefully terminates a container.
	//
	// The container is immediately terminated without allowing graceful
	// shutdown. Use Signal to specify which signal to send (default: SIGKILL).
	Kill(ctx context.Context, cfg KillConfig) error

	// Wait waits for a container to exit.
	//
	// This blocks until the container exits and returns the exit code.
	Wait(ctx context.Context, containerID string) (int, error)

	// Status returns the current status of a container.
	//
	// Returns the ContainerState including running status, exit code, etc.
	Status(ctx context.Context, containerID string) (types.ContainerState, error)

	// IsRunning checks if a container is currently running.
	//
	// This is a convenience method that returns true if Status indicates
	// the container is running.
	IsRunning(ctx context.Context, containerID string) (bool, error)

	// HealthCheck performs a health check on a container.
	//
	// Returns the health status if the container has a health check configured.
	HealthCheck(ctx context.Context, containerID string) (*HealthCheckResult, error)

	// HealthCheckWithRetry performs a health check with retries.
	//
	// This will retry the health check up to maxAttempts times, waiting
	// interval between attempts. Useful for waiting for a container to become healthy.
	HealthCheckWithRetry(ctx context.Context, containerID string, maxAttempts int, interval time.Duration) (*HealthCheckResult, error)

	// Stats retrieves resource usage statistics for a container.
	//
	// Returns a single snapshot of current resource usage including CPU, memory,
	// network I/O, and disk I/O.
	Stats(ctx context.Context, containerID string) (*types.ContainerStats, error)

	// StatsStream returns a channel that receives stats updates.
	//
	// The channel will receive new stats at the specified interval until the
	// context is cancelled or an error occurs. The error channel will receive
	// any errors that occur during streaming.
	StatsStream(ctx context.Context, containerID string, interval time.Duration) (<-chan *types.ContainerStats, <-chan error)

	// Logs retrieves logs from a container.
	//
	// Returns an io.ReadCloser that streams the container logs. The caller
	// is responsible for closing the reader.
	Logs(ctx context.Context, cfg LogsConfig) (io.ReadCloser, error)

	// LogsLines reads logs and returns them as lines.
	//
	// This is a convenience method that reads logs and parses them into
	// structured ContainerLog entries with timestamps and stream information.
	LogsLines(ctx context.Context, cfg LogsConfig) ([]types.ContainerLog, error)

	// EventsStream returns a stream of container events.
	//
	// The channel will receive events such as start, stop, die, etc. until
	// the context is cancelled or an error occurs.
	EventsStream(ctx context.Context, containerID string) (<-chan EventsMessage, <-chan error)

	// PullImage pulls an image from the registry.
	//
	// The timeout specifies how long to wait for the pull to complete.
	// Progress will be logged to the runtime's logger.
	PullImage(ctx context.Context, image string, timeout time.Duration) error

	// ImageExists checks if an image exists locally.
	//
	// Returns true if the image is present in the local image cache.
	ImageExists(ctx context.Context, image string) (bool, error)

	// Client returns the underlying client for direct runtime access.
	//
	// This allows advanced users to access runtime-specific features not
	// exposed by the Runtime interface. The type returned depends on the
	// runtime implementation (e.g., *docker.Client for Docker runtime).
	Client() interface{}

	// Type returns the runtime type identifier.
	//
	// Returns values like "docker", "apple", etc.
	Type() string

	// Info returns runtime metadata and capabilities.
	//
	// Includes version, platform, API version, and a list of supported capabilities.
	Info(ctx context.Context) (*RuntimeInfo, error)

	// Close cleans up runtime resources.
	//
	// This should be called when the runtime is no longer needed to release
	// any resources such as connections or goroutines.
	Close() error
}

// EventsMessage represents a container event message
// Using an alias to avoid importing Docker types directly in the interface
type EventsMessage interface{}

// RuntimeInfo holds metadata about a runtime implementation
type RuntimeInfo struct {
	Type         string            `json:"type"`           // Runtime type (docker, apple, etc.)
	Version      string            `json:"version"`        // Runtime version
	Platform     string            `json:"platform"`       // Platform (linux, darwin, etc.)
	APIVersion   string            `json:"api_version"`    // API version
	Capabilities []string          `json:"capabilities"`   // Supported capabilities
	Metadata     map[string]string `json:"metadata"`       // Additional runtime-specific metadata
}

// RuntimeConfig holds configuration for runtime initialization.
//
// This configuration is used when creating a new Runtime instance through
// NewRuntime or NewRuntimeDefault.
type RuntimeConfig struct {
	// Type specifies which runtime to use ("auto", "docker", "apple")
	// If "auto", the system will detect the best available runtime
	Type string `json:"type"`

	// Timeout is the default timeout for operations
	// If zero, a sensible default will be used
	Timeout time.Duration `json:"timeout"`

	// Logger is the logger to use for runtime operations
	// If nil, a default logger will be created
	Logger interface{} `json:"-"`

	// Endpoint is the runtime endpoint (e.g., Docker daemon socket)
	// If empty, the default endpoint for the runtime will be used
	Endpoint string `json:"endpoint,omitempty"`

	// Additional runtime-specific options
	Options map[string]interface{} `json:"options,omitempty"`
}

// RuntimeFactory is a function that creates a Runtime instance.
//
// Factory functions are registered with RegisterRuntime and used by
// NewRuntime to create runtime instances of specific types.
//
// # Example
//
//	factory := func(cfg container.RuntimeConfig) (container.Runtime, error) {
//	    return MyCustomRuntime{config: cfg}, nil
//	}
//	container.RegisterRuntime("custom", factory)
type RuntimeFactory func(cfg RuntimeConfig) (Runtime, error)

// runtimeRegistry holds registered runtime factories
var runtimeRegistry = make(map[string]RuntimeFactory)

// RegisterRuntime registers a runtime factory for a given type.
//
// This allows custom runtime implementations to be registered and used
// with the NewRuntime function. The runtimeType can be any string identifier.
//
// # Example
//
//	container.RegisterRuntime("myruntime", func(cfg container.RuntimeConfig) (container.Runtime, error) {
//	    return &MyRuntime{cfg: cfg}, nil
//	})
func RegisterRuntime(runtimeType string, factory RuntimeFactory) {
	runtimeRegistry[runtimeType] = factory
}

// GetRuntimeFactory returns the factory for a given runtime type.
//
// Returns the factory function and true if the runtime type is registered,
// nil and false otherwise.
func GetRuntimeFactory(runtimeType string) (RuntimeFactory, bool) {
	factory, ok := runtimeRegistry[runtimeType]
	return factory, ok
}

// ListRuntimes returns a list of registered runtime types.
//
// This returns the types that have been registered via RegisterRuntime.
// It does not indicate whether the runtimes are available on the current
// system, only that they have been registered.
func ListRuntimes() []string {
	types := make([]string, 0, len(runtimeRegistry))
	for t := range runtimeRegistry {
		types = append(types, t)
	}
	return types
}
