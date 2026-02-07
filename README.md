# Orchestrator

The central nervous system of baaaht. This Go-based host process manages container lifecycles, routes events between components, manages sessions, brokers IPC, schedules tasks, and enforces security policies.

## Overview

The Orchestrator is the foundation of the entire baaaht platform. It provides:

- **Container Lifecycle Management**: Create, monitor, and destroy containers via Docker runtime
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

- Go 1.21 or later
- Docker running locally
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

- `docker.host`: Docker daemon socket path (default: `/var/run/docker.sock`)
- `log.level`: Logging level - debug, info, warn, error (default: `info`)
- `session.timeout`: Session inactivity timeout (default: `30m`)
- `policy.max_containers`: Maximum concurrent containers (default: `10`)
- `policy.max_memory`: Maximum memory per container (default: `512MB`)

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
