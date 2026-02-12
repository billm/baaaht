# Copilot Instructions for baaaht

baaaht is a security-first, container-native agentic AI assistant platform. The orchestrator (Go) manages container lifecycles, while specialized agents (TypeScript) handle user interactions and workloads.

## Build, Test, and Lint

### Go (Orchestrator)

```bash
# Build all tools
make build

# Build specific tools
make orchestrator  # Build orchestrator binary
make worker        # Build worker agent binary
make tui           # Build TUI binary

# Install dependencies
go mod download

# Tests
make test              # All tests with coverage
make test-unit         # Unit tests only (./pkg/...)
make test-integration  # Integration tests only (./tests/integration/...)

# Run single test
go test -v -run TestFunctionName ./pkg/container

# Linting and formatting
make lint       # Run go vet and staticcheck
make fmt        # Format code with go fmt and gofmt -s
make mod-tidy   # Tidy and verify modules
```

### TypeScript (Assistant Agent)

```bash
cd agents/assistant

# Build
npm run build

# Tests
npm test                  # All tests
npm run test:unit         # Unit tests only
npm run test:integration  # Integration tests only

# Type checking and linting
npm run typecheck    # TypeScript type checking
npm run lint         # ESLint
npm run lint:fix     # Auto-fix linting issues

# Development
npm run dev  # Run with ts-node
npm start    # Run built version
```

### TypeScript (LLM Gateway)

```bash
cd llm-gateway

# Build
npm ci && npx tsc

# Or via Make
make llm-gateway-build  # Build LLM Gateway
make llm-gateway-docker # Build Docker image
```

### Protocol Buffers

```bash
# Generate Go code from .proto files
make proto-gen

# Clean generated proto files
make proto-clean
```

## Architecture Overview

baaaht uses **OS-level container isolation** as the primary security boundary. Every agent, tool, and gateway workload runs in an isolated container with minimal privileges.

### High-Level Components

```
┌──────────────────────────────────────────────────────────┐
│                   Orchestrator (Go)                      │
│  • Container lifecycle management (Docker/Apple)         │
│  • Event routing & message brokering                     │
│  • Session management & IPC                              │
│  • Policy enforcement (resource limits, mount allowlists)│
│  • Credential isolation                                  │
└──────────────────────────────────────────────────────────┘
                           ↕ gRPC
┌──────────────────────────────────────────────────────────┐
│               Agent Containers (TypeScript)              │
│  • Assistant: Primary conversational interface           │
│  • Worker: Tool execution (file ops, web, shell)         │
│  • LLM Gateway: API credential isolation & proxying      │
└──────────────────────────────────────────────────────────┘
```

### Key Design Principles

1. **Security by Isolation**: Agents run in containers with enforced resource limits and mount allowlists
2. **Delegation Pattern**: Assistant has no tools—delegates all work to specialized agents (Worker, Researcher, Coder)
3. **Credential Separation**: API keys stored in orchestrator, injected via secure channels only
4. **Provider Agnostic**: Unified LLM abstraction (15+ providers via LLM Gateway)

### Directory Structure

```
cmd/                    # Binaries (one per tool)
├── orchestrator/       # Main orchestrator entry point
├── worker/             # Worker agent
└── tui/                # Terminal UI

pkg/                    # Public Go packages (shared across tools)
├── container/          # Unified runtime interface (Docker, Apple)
├── events/             # Event routing system
├── session/            # Session state management
├── policy/             # Security policy enforcement
├── credentials/        # Credential storage and injection
├── grpc/               # gRPC services
├── orchestrator/       # Main orchestration logic
├── worker/             # Worker agent implementation
└── types/              # Shared data types

agents/assistant/       # TypeScript assistant agent
├── src/
│   ├── agent.ts             # Main Agent class
│   ├── orchestrator/        # gRPC client for orchestrator
│   ├── llm/                 # LLM Gateway client with streaming
│   ├── tools/               # Tool implementations (delegate only)
│   └── proto/               # Generated protobuf types

llm-gateway/            # TypeScript LLM gateway service
proto/                  # Protocol Buffer definitions
internal/               # Private Go packages
tests/                  # Integration and E2E tests
```

## Key Conventions

### Go Conventions

#### Logger Pattern

Constructors taking `*logger.Logger` generally accept `nil` and fall back to `logger.NewDefault()`, then add a component field:

```go
if log == nil {
    log = logger.NewDefault()
}
log = log.With("component", "container-manager")
```

#### Error Handling

- Public API errors use `types.Error` with error codes (e.g., `types.ErrNotFound`, `types.ErrInvalidConfig`)
- Wrap errors with context: `fmt.Errorf("failed to create container: %w", err)`

#### Container Runtime Abstraction

Use the `container.Runtime` interface, not Docker-specific APIs directly:

```go
runtime, err := container.NewRuntimeDefault(ctx)  // Auto-detect best runtime
// NOT: docker.NewClientWithOpts()
```

Runtime auto-detection prefers Docker on Linux, Apple Containers on macOS if available.

#### Context Lifecycle

- Long-running monitors (health checks, reconnect loops) should use `context.Background()` or a long-lived client context, not the dial context from `Dial()`
- Release locks before calling external runtime operations (`Stop`, `Destroy`) that may block

#### Types and Configuration

- `types.ContainerConfig.Command` + `Args` are merged to become Docker `Cmd` (not `Entrypoint`)
- Docker multiplexed logs have 8-byte headers: byte[0]=stream (1=stdout, 2=stderr), bytes[4-7]=size (big-endian uint32)

#### Protobuf Duration vs Timestamp

- Use `google.protobuf.Duration` for time spans (uptime, timeout, duration)
- Use `google.protobuf.Timestamp` for points in time (started_at, created_at)

#### Path Security

Always clean paths with `filepath.Clean()` before matching against patterns to prevent directory traversal:

```go
cleanPath := filepath.Clean(userProvidedPath)
```

Mount allowlist paths support shell-style wildcards (`*`) via `matchPattern` in `pkg/policy/rules.go`. The `*` converts to regex `.*` and matches across path separators (not true shell glob semantics).

#### Concurrent File Writing

Use exclusive locks (`Lock/Unlock`), not read locks (`RLock/RUnlock`), when writing to files to prevent concurrent writes from corrupting output.

#### Loop Variable Capture

In range loops, use `for i := range slice { item := &slice[i] }` instead of `for _, item := range slice { &item }` to avoid capturing the loop variable address.

### TypeScript Conventions

#### Enum Usage

Always use enum values, not string literals, even for string enums:

```typescript
// Good
error.code = LLMGatewayErrorCode.INVALID_REQUEST;
agentType: AgentType.AGENT_TYPE_WORKER

// Bad
error.code = "INVALID_REQUEST";
agentType: "worker"
```

#### Build Configuration

Assistant agent TypeScript builds with strict mode enabled:
- `strict: true`
- `noUnusedLocals: true`
- `noUnusedParameters: true`

#### Error Handling

LLM Gateway errors include structured error codes from `LLMGatewayErrorCode` enum for proper error categorization.

### Protobuf

Generated protobuf code lives in:
- Go: `proto/*.pb.go` and `proto/*_grpc.pb.go`
- TypeScript: `agents/assistant/src/proto/` (must regenerate manually with `npm run proto`)

Key protobuf files:
- `agent.proto`: Agent registration, lifecycle, and messaging
- `orchestrator.proto`: Orchestrator service definitions
- `llm.proto`: LLM Gateway service and streaming
- `tool.proto`: Tool definitions and execution
- `common.proto`: Shared types (Duration, ResourceUsage, etc.)

### Skills System

Skills are defined in `SKILL.md` files with JSON frontmatter. See `examples/SKILL.md` for the format.

Required fields: `name`, `display_name`, `description`

Capabilities types:
- `tool`: Executable tool (command, API call)
- `prompt`: System prompt enhancement
- `behavior`: Agent behavior modification

Agent types: `assistant`, `worker`, or `all`

### Policy Enforcement

Security policies are defined in YAML:
- `examples/mount_policy.yaml`: Mount allowlists with wildcard support
- `examples/policies.yaml`: Resource limits, network policies, audit settings

Policies are loaded and enforced by `pkg/policy/enforcer.go`. The policy engine supports:
- Resource quotas (CPU, memory, containers)
- Mount restrictions with pattern matching
- Audit logging of policy violations

## Testing

### Running Tests

Run specific test files:
```bash
go test -v ./pkg/container/runtime_test.go
```

Run tests with build tags (if used):
```bash
go test -tags integration -v ./tests/integration/...
```

### Test Structure

- Unit tests: `pkg/*/` with `*_test.go` suffix
- Integration tests: `tests/integration/`
- Use `testify` for assertions: `require.NoError(t, err)`, `assert.Equal(t, expected, actual)`
- Mock implementations often in same file as test (e.g., `ExecutorMockRuntime` in `pkg/tools/executor_test.go`)

## gRPC Communication

All inter-component communication uses gRPC over Unix Domain Sockets (UDS):

- Orchestrator ↔ Agents: Bidirectional streaming for messages
- Orchestrator ↔ LLM Gateway: Request/response for LLM API proxying
- Auto-reconnection with exponential backoff built into clients

Service definitions in `proto/` directory. Key services:
- `OrchestratorService`: Agent registration, heartbeat, message routing
- `AgentService`: Agent-to-agent communication
- `LLMService`: LLM API proxying with streaming support
- `ToolService`: Tool execution and management

## Docker and Containers

### Container Management

The orchestrator creates containers with:
- Resource limits enforced via Docker/runtime APIs
- Mount allowlists checked against policy before container creation
- Labels for tracking: `baaaht.session_id`, `baaaht.agent_type`, `baaaht.version`

### Health Checks

Use `runtime.HealthCheckWithRetry()` to wait for containers to become healthy with retry logic and exponential backoff.

### Logs

Prefer `LogsLines()` for structured log entries with timestamps. Use `Logs()` for raw streaming.

## Common Patterns

### Session ID Generation

```go
sessionID := types.NewID()  // UUID v4
```

### Resource Limits

```go
resources := types.ResourceLimits{
    MemoryMB:  512,
    CPUShares: 1024,
    PidsLimit: 100,
}
```

### Event Publishing

```go
event := types.NewEvent(types.EventTypeAgentRegistered, sessionID)
event.Payload = agentInfo
bus.Publish(ctx, event)
```
