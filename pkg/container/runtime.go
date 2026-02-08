package container

import (
	"context"
	"io"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Runtime defines the interface for container runtime implementations
// Different runtimes (Docker, Apple Containers, etc.) implement this interface
// to provide a unified API for container management.
type Runtime interface {
	// Create creates a new container with the specified configuration
	Create(ctx context.Context, cfg CreateConfig) (*CreateResult, error)

	// Start starts a container
	Start(ctx context.Context, cfg StartConfig) error

	// Stop stops a container gracefully
	Stop(ctx context.Context, cfg StopConfig) error

	// Restart restarts a container
	Restart(ctx context.Context, cfg RestartConfig) error

	// Destroy removes a container
	Destroy(ctx context.Context, cfg DestroyConfig) error

	// Pause pauses a container
	Pause(ctx context.Context, containerID string) error

	// Unpause unpauses a container
	Unpause(ctx context.Context, containerID string) error

	// Kill forcefully terminates a container
	Kill(ctx context.Context, cfg KillConfig) error

	// Wait waits for a container to exit
	Wait(ctx context.Context, containerID string) (int, error)

	// Status returns the current status of a container
	Status(ctx context.Context, containerID string) (types.ContainerState, error)

	// IsRunning checks if a container is currently running
	IsRunning(ctx context.Context, containerID string) (bool, error)

	// HealthCheck performs a health check on a container
	HealthCheck(ctx context.Context, containerID string) (*HealthCheckResult, error)

	// HealthCheckWithRetry performs a health check with retries
	HealthCheckWithRetry(ctx context.Context, containerID string, maxAttempts int, interval time.Duration) (*HealthCheckResult, error)

	// Stats retrieves resource usage statistics for a container
	Stats(ctx context.Context, containerID string) (*types.ContainerStats, error)

	// StatsStream returns a channel that receives stats updates
	StatsStream(ctx context.Context, containerID string, interval time.Duration) (<-chan *types.ContainerStats, <-chan error)

	// Logs retrieves logs from a container
	Logs(ctx context.Context, cfg LogsConfig) (io.ReadCloser, error)

	// LogsLines reads logs and returns them as lines
	LogsLines(ctx context.Context, cfg LogsConfig) ([]types.ContainerLog, error)

	// EventsStream returns a stream of container events
	EventsStream(ctx context.Context, containerID string) (<-chan EventsMessage, <-chan error)

	// PullImage pulls an image from the registry
	PullImage(ctx context.Context, image string, timeout time.Duration) error

	// ImageExists checks if an image exists locally
	ImageExists(ctx context.Context, image string) (bool, error)

	// Client returns the underlying client for direct runtime access
	// This allows advanced users to access runtime-specific features
	Client() interface{}

	// Type returns the runtime type identifier
	Type() string

	// Info returns runtime metadata and capabilities
	Info(ctx context.Context) (*RuntimeInfo, error)

	// Close cleans up runtime resources
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

// RuntimeConfig holds configuration for runtime initialization
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

// RuntimeFactory is a function that creates a Runtime instance
type RuntimeFactory func(cfg RuntimeConfig) (Runtime, error)

// runtimeRegistry holds registered runtime factories
var runtimeRegistry = make(map[string]RuntimeFactory)

// RegisterRuntime registers a runtime factory for a given type
func RegisterRuntime(runtimeType string, factory RuntimeFactory) {
	runtimeRegistry[runtimeType] = factory
}

// GetRuntimeFactory returns the factory for a given runtime type
func GetRuntimeFactory(runtimeType string) (RuntimeFactory, bool) {
	factory, ok := runtimeRegistry[runtimeType]
	return factory, ok
}

// ListRuntimes returns a list of registered runtime types
func ListRuntimes() []string {
	types := make([]string, 0, len(runtimeRegistry))
	for t := range runtimeRegistry {
		types = append(types, t)
	}
	return types
}
