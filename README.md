# Orchestrator

The central nervous system of baaaht. This Go-based host process manages container lifecycles, routes events between components, manages sessions, brokers IPC, schedules tasks, and enforces security policies.

## Overview

The Orchestrator is the foundation of the entire baaaht platform. It provides:

- **Container Lifecycle Management**: Create, monitor, and destroy containers via Docker or Apple Containers runtime
- **Event Routing**: Dispatch messages to appropriate handlers through a publish-subscribe system
- **Session Management**: Track session state and maintain conversation context
- **IPC Brokering**: Mediate all inter-container communication
- **Policy Engine**: Enforce resource quotas and mount restrictions
- **Credential Management**: Securely store and inject API keys without exposing them to agent containers
- **Task Scheduling**: Queue and execute background tasks with worker pools

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Orchestrator Host                        │
│  ┌──────────────┐  ┌─────────────┐  ┌────────────────────┐ │
│  │   Container  │  │    Event    │  │     Session        │ │
│  │   Manager    │  │    Router   │  │     Manager        │ │
│  └──────────────┘  └─────────────┘  └────────────────────┘ │
│  ┌──────────────┐  ┌─────────────┐  ┌────────────────────┐ │
│  │  IPC Broker  │  │   Policy    │  │   Credential       │ │
│  │              │  │   Engine    │  │     Manager        │ │
│  └──────────────┘  └─────────────┘  └────────────────────┘ │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              Task Scheduler                           │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Project Structure

This is a monorepo containing multiple baaaht tools. Each tool has its own directory under `cmd/`.

```
.
├── cmd/                       # CLI tools (one per binary)
│   └── orchestrator/          # Orchestrator tool
│       └── main.go            # Orchestrator entry point
├── pkg/                       # Public packages (shared across tools)
│   ├── container/             # Container lifecycle management
│   ├── events/                # Event routing system
│   ├── session/               # Session state management
│   ├── ipc/                   # Inter-process communication
│   ├── policy/                # Security policy enforcement
│   ├── credentials/           # Credential storage and injection
│   ├── scheduler/             # Task scheduling
│   ├── worker/                # Worker agent for tool execution
│   ├── orchestrator/          # Main orchestrator logic
│   └── types/                 # Shared data types
├── internal/                  # Private packages
│   ├── config/                # Configuration management
│   └── logger/                # Structured logging
├── tests/                     # Integration and E2E tests
│   ├── integration/
│   └── e2e/
├── Makefile                   # Build automation
├── go.mod                     # Go module definition
└── README.md                  # This file
```

## Building

### Prerequisites

- Go 1.24 or later
- Docker running locally (or Apple Containers on macOS)
- Make (optional)

### Build from source

```bash
# Install dependencies
go mod download

# Build the orchestrator binary
go build -o bin/orchestrator ./cmd/orchestrator

# Or use Make (builds all tools)
make build

# Build specific tool
make orchestrator
```

### Docker build

```bash
docker build -t baaaht/orchestrator:latest .
```

## Running

```bash
# Run with default configuration
./bin/orchestrator

# Run with custom configuration
./bin/orchestrator --config /path/to/config.yaml

# Run via Docker
docker run -v /var/run/docker.sock:/var/run/docker.sock baaaht/orchestrator:latest
```

By default, the orchestrator auto-starts the Assistant backend from a production image (`ghcr.io/billm/baaaht/agent-assistant:latest`) and runs it with hardened runtime defaults:

- non-root user (`1000:1000`)
- `no-new-privileges`
- dropped Linux capabilities (`ALL`)
- read-only root filesystem (enabled by default)

Development mode is still supported explicitly via `--assistant-dev-mode`, which bind-mounts local source and `proto/` for fast iteration.
For container-to-orchestrator connectivity, the orchestrator also starts a TCP gRPC listener (default `0.0.0.0:50051`) and the assistant uses `host.docker.internal:50051`.

You can override assistant startup behavior with CLI flags:

```bash
# Disable assistant auto-start
./bin/orchestrator --assistant-autostart=false

# Use a custom assistant container image
./bin/orchestrator --assistant-image ghcr.io/billm/baaaht/agent-assistant:sha-$(git rev-parse --short HEAD)

# Configure TCP gRPC bridge for containerized assistant
./bin/orchestrator --grpc-tcp-enabled --grpc-tcp-listen-addr 0.0.0.0:50051 --assistant-orchestrator-addr host.docker.internal:50051

# Enable assistant development mode (bind mounts + dev command)
./bin/orchestrator --assistant-dev-mode --assistant-image node:22-alpine --assistant-command sh --assistant-args "-lc","npm install && npm run dev" --assistant-workdir agents/assistant

# Documented development-only security exception (if needed)
./bin/orchestrator --assistant-dev-mode --assistant-readonly-rootfs=false

# Override production command/image explicitly
./bin/orchestrator --assistant-image ghcr.io/billm/baaaht/agent-assistant:sha-$(git rev-parse --short HEAD) --assistant-command node_modules/.bin/tsx --assistant-args src/index.ts

### Agent image strategy

The repository now supports a shared image model for agent containers:

- `agents/base/Dockerfile` defines a hardened Node.js base image and bundles shared protobuf definitions at `/proto` for agent runtime gRPC loading.
- `agents/assistant/Dockerfile` consumes that base image via `BASE_IMAGE`.
- `make agent-images-build` builds both images with `sha-<short-git-sha>` (default 7 chars) and `latest` tags.
- `.github/workflows/images.yml` publishes base first, then assistant using the same deterministic SHA tag while also publishing `latest`.

Rollout path:

1. Build and publish the shared base image.
2. Build and publish the assistant image from that base.
3. Run orchestrator with the published assistant SHA tag.
4. Migrate additional agents to inherit from the same base image contract.

### Assistant migration checklist

1. Build and publish images with deterministic tags:
    - `make agent-images-push`
2. Update orchestrator startup to pinned assistant image tag:
    - `./bin/orchestrator --assistant-image ghcr.io/billm/baaaht/agent-assistant:sha-$(git rev-parse --short HEAD)`
3. Verify runtime hardening and startup:
    - container runs as non-root, has `no-new-privileges`, `cap-drop=ALL`, and read-only rootfs unless explicitly disabled.
4. Validate assistant connectivity and response path through orchestrator gRPC.

Rollback:

- Revert to the previous known-good assistant tag with `--assistant-image`.
- If needed during local development, temporarily switch to dev mode:
  - `./bin/orchestrator --assistant-dev-mode --assistant-image node:22-alpine --assistant-command sh --assistant-args "-lc","npm install && npm run dev" --assistant-readonly-rootfs=false`
```

## Development

### Running tests

```bash
# Run all tests
make test

# Run unit tests only
make test-unit

# Run integration tests
make test-integration
```

### Code quality

```bash
# Run linters
make lint

# Format code
make fmt

# Tidy dependencies
make mod-tidy
```

## Configuration

The orchestrator can be configured via:

1. **Environment variables**: Prefix with `ORCHESTRATOR_`
2. **Configuration file**: YAML or JSON format
3. **Command-line flags**: Override specific settings

Key configuration options:

- `runtime.type`: Container runtime to use (auto, docker, apple; default: auto)
- `runtime.timeout`: Default timeout for operations (default: 30s)
- `docker.host`: Docker daemon socket path (default: `/var/run/docker.sock`)
- `log.level`: Logging level - debug, info, warn, error (default: `info`)
- `session.timeout`: Session inactivity timeout (default: `30m`)
- `policy.max_containers`: Maximum concurrent containers (default: `10`)
- `policy.max_memory`: Maximum memory per container (default: `512MB`)

## Worker Agent

The worker is a specialized agent that executes tools in isolated containers. It connects to the orchestrator via gRPC and handles file operations, web requests, and other tasks with proper policy enforcement.

For detailed documentation on the worker, see [docs/worker.md](docs/worker.md).

### Key Features

- **Tool Execution**: Execute tools in isolated containers with resource limits
- **gRPC Communication**: Connect to orchestrator via Unix domain sockets
- **Task Routing**: Route tasks to appropriate tool implementations
- **Auto-Reconnection**: Automatically reconnect on connection loss
- **Heartbeat**: Send periodic heartbeats to maintain connection health

### Quick Start

```bash
# Build the worker
go build -o bin/worker ./cmd/worker

# Run with default settings
./bin/worker

# Run with custom orchestrator address
./bin/worker --orchestrator-addr unix:///tmp/baaaht-grpc.sock
```

## Container Runtime System

The orchestrator provides a unified interface for managing container runtimes through the `pkg/container` package. This abstraction allows working with different container backends (Docker, Apple Containers) using the same API.

### Runtime Detection

The orchestrator automatically detects the best available runtime:

- **Linux**: Prefers Docker
- **macOS**: Prefers Apple Containers if available, otherwise Docker
- **Windows**: Tries Docker as fallback

### Basic Usage Example

```go
package main

import (
    "context"
    "log"

    "github.com/billm/baaaht/orchestrator/pkg/container"
    "github.com/billm/baaaht/orchestrator/pkg/types"
)

func main() {
    ctx := context.Background()

    // Create a runtime with auto-detection
    runtime, err := container.NewRuntimeDefault(ctx)
    if err != nil {
        log.Fatal(err)
    }
    defer runtime.Close()

    // Get runtime information
    info, err := runtime.Info(ctx)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Using runtime: %s %s", info.Type, info.Version)

    // Create a container
    result, err := runtime.Create(ctx, container.CreateConfig{
        Config: types.ContainerConfig{
            Image: "nginx:latest",
            Env: map[string]string{
                "FOO": "bar",
            },
            Labels: map[string]string{
                "app": "my-app",
            },
        },
        Name:      "my-nginx",
        SessionID: types.NewID(),
        AutoPull:  true,
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Created container: %s", result.ContainerID)

    // Start the container
    err = runtime.Start(ctx, container.StartConfig{
        ContainerID: result.ContainerID,
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Container started successfully")
}
```

### Monitoring Container Stats

```go
// Get a stream of container stats
statsChan, errChan := runtime.StatsStream(ctx, containerID, 1*time.Second)

for {
    select {
    case stats, ok := <-statsChan:
        if !ok {
            return
        }
        log.Printf("CPU: %.2f%% | Memory: %s | Net RX: %s | Net TX: %s",
            stats.CPUPercentage,
            formatBytes(stats.MemoryUsage),
            formatBytes(stats.NetworkRX),
            formatBytes(stats.NetworkTX))
    case err, ok := <-errChan:
        if !ok {
            return
        }
        log.Printf("Error: %v", err)
    }
}
```

### Health Checks with Retry

```go
// Wait for container to become healthy
result, err := runtime.HealthCheckWithRetry(ctx, containerID, 10, 5*time.Second)
if err != nil {
    log.Fatal(err)
}

if result.Status == "healthy" {
    log.Println("Container is healthy")
} else {
    log.Printf("Container health: %s", result.Status)
}
```

### Managing Container Lifecycle

```go
// Stop a container gracefully
err = runtime.Stop(ctx, container.StopConfig{
    ContainerID: containerID,
    Timeout:     10 * time.Second,
})
if err != nil {
    log.Fatal(err)
}

// Or forcefully kill a container
err = runtime.Kill(ctx, container.KillConfig{
    ContainerID: containerID,
    Signal:      "SIGKILL",
})
if err != nil {
    log.Fatal(err)
}

// Remove the container
err = runtime.Destroy(ctx, container.DestroyConfig{
    ContainerID: containerID,
    Force:       true,
})
if err != nil {
    log.Fatal(err)
}
```

### Retrieving Container Logs

```go
// Get logs as structured lines
logs, err := runtime.LogsLines(ctx, container.LogsConfig{
    ContainerID: containerID,
    Tail:        "100",     // Last 100 lines
    Since:       time.Now().Add(-1 * time.Hour),
})
if err != nil {
    log.Fatal(err)
}

for _, logEntry := range logs {
    log.Printf("[%s] %s: %s", logEntry.Timestamp, logEntry.Stream, logEntry.Line)
}

// Or get a raw stream
reader, err := runtime.Logs(ctx, container.LogsConfig{
    ContainerID: containerID,
    Follow:      true, // Stream logs as they arrive
    Stdout:      true,
    Stderr:      true,
})
if err != nil {
    log.Fatal(err)
}
defer reader.Close()

// Read from reader...
```

### Runtime Factory Pattern

The container package supports custom runtime implementations through a factory pattern:

```go
// Register a custom runtime
container.RegisterCustomRuntime("custom", func(cfg container.RuntimeConfig) (container.Runtime, error) {
    return &MyCustomRuntime{config: cfg}, nil
})

// Use the custom runtime from the registry
factory, err := container.GetRuntimeFromRegistry("custom")
if err != nil {
    // handle error
}

runtime, err := factory(ctx, container.RuntimeConfig{
    Timeout: 30 * time.Second,
})
```

### Available Runtime Types

| Runtime Type | Description | Platforms |
|-------------|-------------|-----------|
| `auto` | Auto-detect best runtime | All |
| `docker` | Docker daemon | Linux, macOS, Windows |
| `apple` | Apple Containers (planned / stub, currently disabled) | Planned: macOS 15.0+ |

### Container Configuration Options

The `types.ContainerConfig` struct provides comprehensive configuration:

```go
type ContainerConfig struct {
    Image           string                 // Container image
    Command         []string               // Command to run
    Args            []string               // Entrypoint arguments
    Env             map[string]string      // Environment variables
    WorkingDir      string                 // Working directory
    Labels          map[string]string      // Container labels
    Mounts          []Mount                // Volume mounts
    Ports           []PortBinding          // Port bindings
    Networks        []string               // Network names
    NetworkMode     string                 // Network mode (bridge, host, etc.)
    Resources       ResourceLimits         // CPU/memory limits
    RestartPolicy   RestartPolicy          // Restart policy
    RemoveOnStop    bool                   // Auto-remove on stop
    ReadOnlyRootfs  bool                   // Read-only root filesystem
}
```

## Security

The orchestrator enforces strict security boundaries:

- **Container Isolation**: Each agent runs in an isolated Docker container
- **Credential Separation**: API keys are stored in the host process and injected via secure channels only
- **Policy Enforcement**: Resource quotas, mount restrictions, and network policies are enforced at the OS level
- **No Code Execution**: The orchestrator never executes LLM-generated code

## Contributing

This is the core orchestrator for baaaht. All other services depend on it.

1. Follow Go best practices and idioms
2. Write comprehensive tests (unit and integration)
3. Ensure all tests pass before submitting
4. Use `go fmt` for formatting
5. Run `go vet` and `staticcheck` before committing

## License

See [LICENSE](LICENSE) file for details.
