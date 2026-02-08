# gRPC Communication

This guide explains how to use the gRPC communication system in the baaaht orchestrator.

## Overview

The orchestrator provides a gRPC API over Unix Domain Sockets for:
- **Session management**: Create and manage user sessions
- **Container operations**: Create, monitor, and destroy containers
- **Event streaming**: Subscribe to system events in real-time
- **Task execution**: Execute tasks on agent workers
- **Message passing**: Bidirectional streaming for real-time chat

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Orchestrator Host                    │
│  ┌──────────────────────────────────────────────────┐  │
│  │              gRPC Server (UDS)                    │  │
│  │  ┌──────────────┐  ┌─────────────┐  ┌─────────┐  │  │
│  │  │ Orchestrator │  │    Agent    │  │ Gateway │  │  │
│  │  │   Service    │  │   Service   │  │ Service │  │  │
│  │  └──────────────┘  └─────────────┘  └─────────┘  │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
        │                        ▲
        │                        │
    ┌───┴────┐              ┌────┴───┐
    │ Agent  │              │ Gateway│
    │Process │              │Process │
    └────────┘              └────────┘
```

## Connection

### Default Socket Path
```bash
/tmp/baaaht-grpc.sock
```

### Custom Socket Path
Set via environment variable:
```bash
export GRPC_SOCKET_PATH=/tmp/my-orchestrator.sock
```

Or command-line flag:
```bash
orchestrator --grpc-socket-path /tmp/my-orchestrator.sock
```

## Client Usage

### Go Client

```go
import (
    "context"
    "github.com/billm/baaaht/orchestrator/pkg/grpc"
    "github.com/billm/baaaht/orchestrator/proto"
)

// Create client
client, err := grpc.NewClient("/tmp/baaaht-grpc.sock", grpc.ClientConfig{}, logger)
if err != nil {
    log.Fatal(err)
}

// Connect
if err := client.Dial(context.Background()); err != nil {
    log.Fatal(err)
}
defer client.Close()

// Create orchestrator client
orchClient := proto.NewOrchestratorServiceClient(client.GetConn())

// Call RPC
resp, err := orchClient.CreateSession(ctx, &proto.CreateSessionRequest{
    Metadata: &proto.SessionMetadata{
        Name: "my-session",
    },
})
```

### Command Line (grpcurl)

```bash
# List sessions
grpcurl -unix /tmp/baaaht-grpc.sock \
    proto.orchestrator.v1.OrchestratorService/ListSessions \
    '{}'

# Create session
grpcurl -unix /tmp/baaaht-grpc.sock \
    -d '{"metadata": {"name": "test-session"}}' \
    proto.orchestrator.v1.OrchestratorService/CreateSession

# Health check
grpcurl -unix /tmp/baaaht-grpc.sock \
    grpc.health.v1.Health/Check
```

## Common Operations

### 1. Session Management

#### Create a Session
```go
req := &proto.CreateSessionRequest{
    Metadata: &proto.SessionMetadata{
        Name:        "my-session",
        Description: "Example session",
        OwnerID:     "user-123",
    },
    Config: &proto.SessionConfig{
        MaxContainers:  10,
        IdleTimeoutNs:  3600_000_000_000, // 1 hour
        ResourceLimits: &common.v1.ResourceLimits{
            NanoCpu:    1_000_000_000, // 1 CPU
            MemoryBytes: 1024_1024_1024, // 1GB
        },
    },
}

resp, err := orchClient.CreateSession(ctx, req)
// resp.SessionId contains the new session ID
```

#### Get Session Info
```go
resp, err := orchClient.GetSession(ctx, &proto.GetSessionRequest{
    SessionId: "session-123",
})
```

#### List Sessions
```go
resp, err := orchClient.ListSessions(ctx, &proto.ListSessionsRequest{
    Filter: &proto.SessionFilter{
        State:   proto.SessionState_SESSION_STATE_ACTIVE,
        OwnerID: "user-123",
    },
})
```

#### Close a Session
```go
resp, err := orchClient.CloseSession(ctx, &proto.CloseSessionRequest{
    SessionId: "session-123",
    Reason:    "User logged out",
})
```

### 2. Message Handling

#### Send a Message
```go
resp, err := orchClient.SendMessage(ctx, &proto.SendMessageRequest{
    SessionId: "session-123",
    Message: &proto.Message{
        Role:    proto.MessageRole_MESSAGE_ROLE_USER,
        Content: "Hello, orchestrator!",
    },
})
```

#### Stream Messages (Bidirectional)
```go
stream, err := orchClient.StreamMessages(ctx)
if err != nil {
    log.Fatal(err)
}

// Send messages
go func() {
    for _, msg := range messages {
        if err := stream.Send(&proto.StreamMessageRequest{
            SessionId: "session-123",
            Payload: &proto.StreamMessageRequest_Message{
                Message: msg,
            },
        }); err != nil {
            log.Printf("Send error: %v", err)
            return
        }
    }
    stream.CloseSend()
}()

// Receive messages
for {
    resp, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }

    switch p := resp.Payload.(type) {
    case *proto.StreamMessageResponse_Message:
        fmt.Printf("Message: %v\n", p.Message)
    case *proto.StreamMessageResponse_Event:
        fmt.Printf("Event: %v\n", p.Event)
    }
}
```

### 3. Container Operations

#### Create a Container
```go
req := &proto.CreateContainerRequest{
    SessionId: "session-123",
    Config: &proto.ContainerConfig{
        Image:      "python:3.11",
        Command:    []string{"python"},
        Args:       []string{"-u", "script.py"},
        WorkingDir: "/workspace",
        Env: map[string]string{
            "PYTHONUNBUFFERED": "1",
        },
        ResourceLimits: &common.v1.ResourceLimits{
            NanoCpu:     500_000_000, // 0.5 CPU
            MemoryBytes: 512_1024_1024, // 512MB
        },
    },
}

resp, err := orchClient.CreateContainer(ctx, req)
// resp.ContainerId contains the new container ID
```

#### Get Container Info
```go
resp, err := orchClient.GetContainer(ctx, &proto.GetContainerRequest{
    ContainerId: "container-123",
})
```

#### Stop a Container
```go
resp, err := orchClient.StopContainer(ctx, &proto.StopContainerRequest{
    ContainerId: "container-123",
    TimeoutNs:   10_000_000_000, // 10 seconds
})
```

### 4. Event Subscription

#### Subscribe to Events (Server Streaming)
```go
stream, err := orchClient.SubscribeEvents(ctx, &proto.SubscribeEventsRequest{
    Filter: &proto.EventFilter{
        Type:      proto.EventType_EVENT_TYPE_CONTAINER_CREATED,
        SessionId: "session-123",
    },
})
if err != nil {
    log.Fatal(err)
}

for {
    event, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Event: %s - %s\n", event.Type, event.Source)
}
```

### 5. Agent Communication

#### Register an Agent
```go
agentClient := proto.NewAgentServiceClient(conn)

resp, err := agentClient.Register(ctx, &proto.RegisterRequest{
    Name: "python-executor",
    Type: proto.AgentType_AGENT_TYPE_WORKER,
    Metadata: &proto.AgentMetadata{
        Version:  "1.0.0",
        Hostname: "host-1",
    },
    Capabilities: &proto.AgentCapabilities{
        SupportedTasks:    []string{"code-execution", "file-operation"},
        MaxConcurrentTasks: 5,
        SupportsStreaming:  true,
    },
})
// resp.AgentId contains the new agent ID
```

#### Execute a Task with Streaming
```go
stream, err := agentClient.StreamTask(ctx)
if err != nil {
    log.Fatal(err)
}

// Send task
err = stream.Send(&proto.StreamTaskRequest{
    TaskId: "task-123",
    Payload: &proto.StreamTaskRequest_Input{
        Input: &proto.TaskInput{
            Data: []byte("print('hello')"),
        },
    },
})

// Receive output
for {
    resp, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }

    switch p := resp.Payload.(type) {
    case *proto.StreamTaskResponse_Output:
        fmt.Printf("Output: %s\n", p.Output.Text)
    case *proto.StreamTaskResponse_Progress:
        fmt.Printf("Progress: %.0f%%\n", p.Progress.Percent*100)
    case *proto.StreamTaskResponse_Complete:
        fmt.Printf("Complete: %v\n", p.Complete)
    }
}
```

### 6. Gateway Communication

#### Create Gateway Session
```go
gatewayClient := proto.NewGatewayServiceClient(conn)

resp, err := gatewayClient.CreateGatewaySession(ctx, &proto.CreateGatewaySessionRequest{
    Metadata: &proto.GatewaySessionMetadata{
        Name:        "web-session",
        ClientType:  "web",
        ClientVersion: "1.0.0",
        UserID:      "user-123",
    },
    Config: &proto.GatewaySessionConfig{
        EnableStreaming: true,
        StreamConfig: &proto.StreamConfig{
            ChunkSize:  1024,
            EnableDelta: true,
        },
    },
})
```

#### Stream Chat (Bidirectional)
```go
stream, err := gatewayClient.StreamChat(ctx)
if err != nil {
    log.Fatal(err)
}

// Send message
go func() {
    stream.Send(&proto.StreamChatRequest{
        SessionId: "gateway-session-123",
        Payload: &proto.StreamChatRequest_Message{
            Message: &proto.GatewayMessage{
                Role:    proto.GatewayMessageRole_GATEWAY_MESSAGE_ROLE_USER,
                Content: "Hello!",
            },
        },
    })
    stream.CloseSend()
}()

// Receive response chunks
for {
    resp, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }

    switch p := resp.Payload.(type) {
    case *proto.StreamChatResponse_Chunk:
        fmt.Printf("Chunk: %s\n", p.Chunk.Text)
    case *proto.StreamChatResponse_Status:
        fmt.Printf("Status: %v\n", p.Status)
    }
}
```

## Configuration

### Server Configuration

```go
config := grpc.ServerConfig{
    Path:              "/tmp/baaaht-grpc.sock",
    MaxRecvMsgSize:    100 * 1024 * 1024, // 100 MB
    MaxSendMsgSize:    100 * 1024 * 1024, // 100 MB
    ConnectionTimeout: 30 * time.Second,
    ShutdownTimeout:   10 * time.Second,
}
```

### Client Configuration

```go
config := grpc.ClientConfig{
    DialTimeout:        30 * time.Second,
    RPCTimeout:         10 * time.Second,
    MaxRecvMsgSize:     100 * 1024 * 1024,
    MaxSendMsgSize:     100 * 1024 * 1024,
    ReconnectInterval:  5 * time.Second,
    ReconnectMaxAttempts: 0, // 0 = infinite retry
}
```

## Error Handling

All RPCs return standard gRPC errors. Handle them appropriately:

```go
resp, err := client.CreateSession(ctx, req)
if err != nil {
    status, ok := status.FromError(err)
    if ok {
        switch status.Code() {
        case codes.InvalidArgument:
            // Bad request
        case codes.NotFound:
            // Resource doesn't exist
        case codes.Unavailable:
            // Service temporarily unavailable (may retry)
        case codes.Unauthenticated:
            // Missing or invalid credentials
        default:
            // Other error
        }
        log.Printf("RPC failed: %s - %s", status.Code(), status.Message())
    }
    return
}
```

## Health Checks

```go
import grpc_health "google.golang.org/grpc/health/grpc_health_v1"

healthClient := grpc_health.NewHealthClient(conn)

resp, err := healthClient.Check(ctx, &grpc_health.HealthCheckRequest{
    Service: "orchestrator",
})
if err != nil {
    log.Fatal(err)
}

if resp.Status == grpc_health.HealthCheckResponse_SERVING {
    fmt.Println("Service is healthy")
}
```

## Best Practices

1. **Context Management**: Always use contexts with timeouts for RPCs
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Connection Reuse**: Create a single client connection and reuse it
   ```go
   // Good: One connection for the application lifetime
   conn, _ := grpc.NewClientConn(path)

   // Bad: New connection for each RPC
   ```

3. **Stream Cleanup**: Always close streams when done
   ```go
   defer stream.CloseSend()
   ```

4. **Error Handling**: Check and handle errors from each RPC
   ```go
   if err != nil {
       // Handle error appropriately
   }
   ```

5. **Resource Cleanup**: Always close client connections
   ```go
   defer client.Close()
   ```

## Testing

See `pkg/grpc/integration_test.go` for examples of:
- Server/client connection
- Session CRUD operations
- Agent registration
- Event streaming
- Bidirectional streaming

## Related Documentation

- [Protocol Buffer Definitions](../proto/README.md) - Proto file documentation
- [Architecture](ARCHITECTURE.md) - System architecture
- [Go Package](../pkg/grpc/) - Implementation details
