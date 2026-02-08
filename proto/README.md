# Protocol Buffer Definitions

This directory contains the gRPC service definitions and Protocol Buffer messages for the baaaht orchestrator communication system.

## Overview

The orchestrator uses gRPC over Unix Domain Sockets (UDS) for efficient, type-safe inter-process communication between components:

- **Orchestrator**: Central coordination service
- **Agents**: Worker processes that execute tasks
- **Gateways**: User-facing interfaces for real-time communication
- **Tools**: Specialized services for specific operations

## Files

### common.proto
Shared types and enumerations used across all services.

**Key Types:**
- `Status`: Component operational status (RUNNING, STOPPED, ERROR, etc.)
- `Health`: Health check states (HEALTHY, UNHEALTHY, DEGRADED)
- `Priority`: Task/message priority levels (LOW, NORMAL, HIGH, CRITICAL)
- `ResourceLimits`: Resource constraints (CPU, memory, PIDs)
- `ResourceUsage`: Current resource consumption metrics

### orchestrator.proto
Defines the **OrchestratorService** - the main API for agents and gateways to interact with the orchestrator.

**Service RPCs:**

| RPC | Type | Description |
|-----|------|-------------|
| `CreateSession` | Unary | Create a new session with context |
| `GetSession` | Unary | Retrieve session by ID |
| `ListSessions` | Unary | List all sessions matching filter |
| `UpdateSession` | Unary | Update session metadata or context |
| `CloseSession` | Unary | Close a session and release resources |
| `SendMessage` | Unary | Send a message to a session |
| `StreamMessages` | Bidirectional Streaming | Real-time bidirectional message exchange |
| `CreateContainer` | Unary | Create a container within a session |
| `GetContainer` | Unary | Retrieve container by ID |
| `ListContainers` | Unary | List containers matching filter |
| `StopContainer` | Unary | Stop a running container |
| `RemoveContainer` | Unary | Remove a container from system |
| `SubscribeEvents` | Server Streaming | Subscribe to system events |
| `HealthCheck` | Unary | Check orchestrator health |
| `GetStatus` | Unary | Get orchestrator status and stats |

**Key Message Types:**
- `Session`: User session with conversation context
- `Container`: Container instance managed by orchestrator
- `Message`: Chat message in conversation
- `Event`: System event for monitoring

### agent.proto
Defines the **AgentService** - API for orchestrator to communicate with worker agents.

**Service RPCs:**

| RPC | Type | Description |
|-----|------|-------------|
| `Register` | Unary | Register a new agent |
| `Unregister` | Unary | Unregister an agent |
| `Heartbeat` | Unary | Send keep-alive signal |
| `ExecuteTask` | Unary | Execute a task on agent |
| `StreamTask` | Bidirectional Streaming | Execute task with real-time updates |
| `CancelTask` | Unary | Cancel a running task |
| `ListTasks` | Unary | List all tasks on agent |
| `GetTaskStatus` | Unary | Get task status and progress |
| `StreamAgent` | Bidirectional Streaming | General bidirectional communication |
| `SendMessage` | Unary | Send message to agent |
| `HealthCheck` | Unary | Check agent health |
| `GetStatus` | Unary | Get agent status and resource usage |
| `GetCapabilities` | Unary | Query agent capabilities |

**Key Message Types:**
- `Agent`: Agent instance with metadata
- `Task`: Task with configuration and state
- `AgentMessage`: Message sent to/from agent
- `AgentCapabilities`: Supported operations and limits

### gateway.proto
Defines the **GatewayService** - API for orchestrator to push responses to user-facing gateways.

**Service RPCs:**

| RPC | Type | Description |
|-----|------|-------------|
| `CreateGatewaySession` | Unary | Create a new gateway session |
| `GetGatewaySession` | Unary | Retrieve gateway session by ID |
| `ListGatewaySessions` | Unary | List gateway sessions |
| `CloseGatewaySession` | Unary | Close a gateway session |
| `GatewaySendMessage` | Unary | Send message to gateway |
| `StreamChat` | Bidirectional Streaming | Real-time chat with streaming responses |
| `StreamResponses` | Server Streaming | Stream response chunks |
| `SubscribeToEvents` | Server Streaming | Subscribe to gateway events |
| `HealthCheck` | Unary | Check gateway health |
| `GetStatus` | Unary | Get gateway status and stats |

**Key Message Types:**
- `GatewaySession`: User session linked to orchestrator session
- `GatewayMessage`: Message in gateway
- `ResponseChunk`: Chunk of streaming response
- `StreamStatus`: Status of active stream

## Versioning

Protocol buffer definitions follow semantic versioning:
- **Major version**: Breaking changes to message/service definitions
- **Minor version**: New services or messages added
- **Patch version**: Bug fixes, documentation updates

Current version: **v1** (as indicated by package names: `orchestrator.v1`, `agent.v1`, `gateway.v1`, `common.v1`)

## Compilation

Generate Go code from proto files:

```bash
# Using Make
make proto-gen

# Or directly with protoc
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/*.proto
```

This generates:
- `*.pb.go` - Message types
- `*_grpc.pb.go` - gRPC service interfaces

## Transport

All services use **Unix Domain Sockets** for transport:
- Default socket path: `/tmp/baaaht-grpc.sock`
- No network exposure (all traffic stays on host)
- Low latency compared to TCP
- File system permissions for access control

## Streaming

The protocol supports three types of streaming RPCs:

1. **Unary**: Request/Response (traditional RPC)
   - Example: `CreateSession`, `GetStatus`

2. **Server Streaming**: Single request, stream of responses
   - Example: `SubscribeEvents`, `StreamResponses`

3. **Bidirectional Streaming**: Both client and server send streams
   - Example: `StreamMessages`, `StreamTask`, `StreamChat`

## Error Handling

All RPCs return standard gRPC status codes:
- `OK` (0): Success
- `INVALID_ARGUMENT` (3): Bad request
- `NOT_FOUND` (5): Resource doesn't exist
- `ALREADY_EXISTS` (6): Resource already exists
- `UNAVAILABLE` (14): Service temporarily unavailable
- `UNAUTHENTICATED` (16): Missing or invalid credentials

## Authentication

RPCs can require authentication via bearer tokens in metadata:
```
authorization: Bearer <token>
```

Tokens are managed by the orchestrator and validated by interceptors.

## Related Documentation

- [gRPC Usage Guide](../docs/GRPC.md) - How to use gRPC services
- [Architecture](../docs/ARCHITECTURE.md) - System architecture overview
- [Go Package Documentation](../pkg/grpc/) - Go implementation details
