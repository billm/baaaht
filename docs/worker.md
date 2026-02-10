# Worker

The worker is a container-native tool execution agent that connects to the orchestrator via gRPC and executes isolated tool containers. It handles file operations, web requests, and other tasks with proper policy enforcement and resource isolation.

## Overview

The worker is a specialized agent that:

- **Tool Execution**: Executes tools in isolated containers with resource limits
- **gRPC Communication**: Connects to the orchestrator via Unix domain sockets
- **Task Routing**: Routes tasks to appropriate tool implementations
- **Policy Enforcement**: Enforces security policies on tool containers
- **Auto-Reconnection**: Automatically reconnects to the orchestrator on connection loss
- **Heartbeat**: Sends periodic heartbeats to maintain connection health

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Worker Agent                         │
│  ┌──────────────┐  ┌─────────────┐  ┌────────────────────┐ │
│  │    gRPC      │  │   Task      │  │      Tool          │ │
│  │   Client     │  │   Router    │  │    Executor        │ │
│  └──────────────┘  └─────────────┘  └────────────────────┘ │
│  ┌──────────────┐  ┌─────────────┐  ┌────────────────────┐ │
│  │ Reconnection │  │  Heartbeat  │  │   Policy           │ │
│  │   Monitor    │  │   Monitor   │  │   Enforcer         │ │
│  └──────────────┘  └─────────────┘  └────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC (Unix Socket)
                              ▼
                    ┌─────────────────┐
                    │  Orchestrator   │
                    └─────────────────┘
```

## Building

### Build from source

```bash
# Build the worker binary
go build -o bin/worker ./cmd/worker

# Or use Make
make worker
```

### Docker build

```bash
docker build -f cmd/worker/Dockerfile -t baaaht/worker:latest .
```

## Running

```bash
# Run with default configuration
./bin/worker

# Run with custom orchestrator address
./bin/worker --orchestrator-addr unix:///tmp/baaaht-grpc.sock

# Run with custom worker name
./bin/worker --name my-worker

# Run with custom timeout settings
./bin/worker --dial-timeout 60s --rpc-timeout 30s

# Run with logging
./bin/worker --log-level debug
```

## Configuration

The worker can be configured via:

1. **Command-line flags**: Override specific settings
2. **Environment variables**: Prefix with `WORKER_` (e.g., `WORKER_LOG_LEVEL`)
3. **Configuration file**: YAML or JSON format at `~/.config/baaaht/config.yaml`

### Configuration Options

| Flag | Description | Default |
|------|-------------|---------|
| `--orchestrator-addr` | Orchestrator gRPC address | `unix:///tmp/baaaht-grpc.sock` |
| `--name` | Worker name | `worker-<hostname>` |
| `--dial-timeout` | Timeout for connecting to orchestrator | `30s` |
| `--rpc-timeout` | Timeout for RPC calls | `10s` |
| `--max-recv-msg-size` | Maximum received message size (bytes) | `104857600` (100MB) |
| `--max-send-msg-size` | Maximum sent message size (bytes) | `104857600` (100MB) |
| `--reconnect-interval` | Interval between reconnection attempts | `5s` |
| `--reconnect-max-attempts` | Maximum reconnection attempts (0 = infinite) | `0` |
| `--heartbeat-interval` | Interval between heartbeats | `30s` |
| `--log-level` | Logging level | `info` |
| `--log-format` | Log format (json, text) | `text` |
| `--log-output` | Log output (stdout, stderr, or file path) | `stdout` |

## Tool Types

The worker supports the following tool types:

### File Operations

| Tool Type | Description | Image | Command |
|-----------|-------------|-------|---------|
| `file_read` | Read file contents | `alpine:3.19` | `cat` |
| `file_write` | Write content to file | `alpine:3.19` | `tee` |
| `file_edit` | Edit file contents | `alpine:3.19` | `sed` |
| `grep` | Search file contents | `alpine:3.19` | `grep` |
| `find` | Find files | `alpine:3.19` | `find` |
| `list` | List directory contents | `alpine:3.19` | `ls -la` |

### Network Operations

| Tool Type | Description | Image | Command |
|-----------|-------------|-------|---------|
| `web_search` | Perform web search | `curlimages/curl:8.6.0` | `curl` |
| `fetch_url` | Fetch URL content | `curlimages/curl:8.6.0` | `curl` |

## Programmatic Usage

### Creating a Worker Agent

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/billm/baaaht/orchestrator/internal/logger"
    "github.com/billm/baaaht/orchestrator/pkg/worker"
)

func main() {
    ctx := context.Background()

    // Create logger
    log, err := logger.NewDefault()
    if err != nil {
        log.Fatal(err)
    }

    // Create bootstrap config
    cfg := worker.BootstrapConfig{
        Logger:              log,
        Version:             "1.0.0",
        OrchestratorAddr:    "unix:///tmp/baaaht-grpc.sock",
        WorkerName:          "my-worker",
        DialTimeout:         30 * time.Second,
        RPCTimeout:          10 * time.Second,
        MaxRecvMsgSize:      100 * 1024 * 1024, // 100MB
        MaxSendMsgSize:      100 * 1024 * 1024, // 100MB
        ReconnectInterval:   5 * time.Second,
        ReconnectMaxAttempts: 0, // Infinite retries
        HeartbeatInterval:   30 * time.Second,
        ShutdownTimeout:     30 * time.Second,
        EnableHealthCheck:   true,
    }

    // Bootstrap worker
    result, err := worker.Bootstrap(ctx, cfg)
    if err != nil {
        log.Fatal("Failed to bootstrap worker:", err)
    }

    log.Printf("Worker started successfully: %s", result.Agent.GetAgentID())

    // Worker is now ready to receive tasks
    // ... (main loop)
}
```

### Using the Tool Executor

```go
// Create an executor
executor, err := worker.NewExecutorDefault(log)
if err != nil {
    log.Fatal(err)
}
defer executor.Close()

// Execute a file read task
ctx := context.Background()
content, err := worker.FileRead(ctx, executor, "/path/to/project", "main.go")
if err != nil {
    log.Printf("File read failed: %v", err)
} else {
    log.Printf("File content: %s", content)
}

// Execute a web search
html, err := worker.WebSearch(ctx, executor, "https://example.com")
if err != nil {
    log.Printf("Web search failed: %v", err)
} else {
    log.Printf("Response: %s", html)
}
```

### Custom Tool Execution

```go
// Execute a task with custom configuration
taskCfg := worker.TaskConfig{
    ToolType:    worker.ToolTypeFileRead,
    Args:        []string{"/workspace/main.go"},
    MountSource: "/path/to/project",
    Timeout:     60 * time.Second, // Override default timeout
}

result := executor.ExecuteTask(ctx, taskCfg)
if result.Error != nil {
    log.Printf("Task failed: %v", result.Error)
} else {
    log.Printf("Exit code: %d", result.ExitCode)
    log.Printf("Stdout: %s", result.Stdout)
    log.Printf("Stderr: %s", result.Stderr)
}
```

### Task Cancellation

```go
// Execute a task asynchronously
taskCfg := worker.TaskConfig{
    ToolType:    worker.ToolTypeGrep,
    Args:        []string{"-r", "TODO", "/workspace"},
    MountSource: "/path/to/project",
}

go func() {
    result := executor.ExecuteTask(ctx, taskCfg)
    // Handle result...
}()

// Cancel the task by ID
taskID := "..." // Get task ID from executor
err := executor.CancelTask(ctx, taskID, true) // force = true
if err != nil {
    log.Printf("Cancellation failed: %v", err)
}
```

## Worker Lifecycle

### Bootstrap Process

1. **Create Agent**: Initialize the worker agent with configuration
2. **Connect**: Establish gRPC connection to orchestrator
3. **Register**: Register with orchestrator, receive agent ID
4. **Heartbeat**: Start heartbeat loop to maintain connection
5. **Ready**: Worker is ready to receive tasks

### Shutdown Process

1. **Stop Tasks**: Cancel running tasks gracefully
2. **Unregister**: Unregister from orchestrator
3. **Close Connection**: Close gRPC connection
4. **Cleanup**: Release resources

## Connection Management

### Auto-Reconnection

The worker automatically reconnects to the orchestrator when the connection is lost:

```go
// Reconnection is enabled by default
cfg := worker.BootstrapConfig{
    ReconnectInterval:   5 * time.Second,
    ReconnectMaxAttempts: 0, // 0 = infinite retries
}
```

### Connection States

| State | Description |
|-------|-------------|
| `Idle` | Not connected |
| `Connecting` | Attempting to connect |
| `Ready` | Connected and ready |
| `TransientFailure` | Temporary failure, will reconnect |
| `Shutdown` | Connection is shutting down |

## Heartbeat

The worker sends periodic heartbeats to the orchestrator to maintain connection health:

```go
// Default heartbeat interval is 30 seconds
cfg := worker.BootstrapConfig{
    HeartbeatInterval: 30 * time.Second,
}
```

Heartbeats include:
- Agent ID
- Active tasks (optional)
- Timestamp

## Tool Container Resource Limits

Each tool container has the following default resource limits:

| Tool Type | CPU Limit | Memory Limit | PID Limit | Timeout |
|-----------|-----------|--------------|-----------|---------|
| File operations | 0.1 CPU | 64MB | 10 | 30s |
| Grep | 0.2 CPU | 128MB | 20 | 60s |
| Find | 0.2 CPU | 128MB | 20 | 60s |
| Web operations | 0.2 CPU | 128MB | 10 | 30-60s |

## Security

### Container Isolation

- Each tool runs in an isolated Docker container
- Containers use read-only root filesystem when possible
- File operations use bind mounts with read-only mode for safety
- Network is disabled for file operations (`NetworkMode: "none"`)

### Policy Enforcement

The worker enforces policies on tool containers:

```go
// Create executor with policy enforcer
executor, err := worker.NewExecutor(worker.ExecutorConfig{
    Runtime:        runtime,
    PolicyEnforcer: policyEnforcer,
    SessionID:      sessionID,
    Logger:         log,
})
```

### Resource Limits

Tool containers have strict resource limits:
- CPU: 0.1-0.2 cores
- Memory: 64-128MB
- PIDs: 10-20 processes
- Timeout: 30-60 seconds

## Development

### Running Tests

```bash
# Run all worker tests
go test ./pkg/worker/...

# Run specific test
go test ./pkg/worker/executor_test.go

# Run with coverage
go test -cover ./pkg/worker/...
```

### Adding New Tools

To add a new tool type:

1. Define the tool type constant in `tools.go`:
```go
const (
    ToolTypeMyTool ToolType = "my_tool"
)
```

2. Create the tool specification function:
```go
func MyTool() *ToolSpec {
    return &ToolSpec{
        Type:        ToolTypeMyTool,
        Name:        "my_tool",
        Description: "My custom tool",
        Image:       "alpine:3.19",
        Command:     []string{"my-command"},
        WorkingDir:  "/workspace",
        Env:         make(map[string]string),
        Mounts: []types.Mount{
            {
                Type:     types.MountTypeBind,
                Source:   "", // Filled in at runtime
                Target:   "/workspace",
                ReadOnly: true,
            },
        },
        Resources: types.ResourceLimits{
            NanoCPUs:    100_000_000,
            MemoryBytes: 64 * 1024 * 1024,
            PidsLimit:   int64Ptr(10),
        },
        ReadOnlyRootfs: true,
        Timeout:        30 * time.Second,
        NetworkMode:    "none",
    }
}
```

3. Add the tool to `GetToolSpec`:
```go
case ToolTypeMyTool:
    return MyTool(), nil
```

4. Add the tool to `AllToolSpecs`:
```go
func AllToolSpecs() []*ToolSpec {
    return []*ToolSpec{
        // ... existing tools
        MyTool(),
    }
}
```

## Troubleshooting

### Connection Issues

**Problem**: Worker cannot connect to orchestrator

```bash
# Check if orchestrator is running
ps aux | grep orchestrator

# Check if socket exists
ls -la /tmp/baaaht-grpc.sock

# Check worker logs
./bin/worker --log-level debug
```

### Task Execution Failures

**Problem**: Tool containers fail to execute

```bash
# Check Docker is running
docker ps

# Check tool images are available
docker images | grep -E "alpine|curl"

# Check worker logs for errors
./bin/worker --log-level debug
```

### Permission Issues

**Problem**: Worker cannot access mounted paths

```bash
# Check file permissions
ls -la /path/to/project

# Ensure worker has read access
chmod +r /path/to/project

# Check mount paths in worker logs
./bin/worker --log-level debug 2>&1 | grep mount
```

## See Also

- [gRPC Communication](GRPC.md) - gRPC API details
- [Architecture](ARCHITECTURE.md) - System architecture overview
- [README](../README.md) - Main project README
