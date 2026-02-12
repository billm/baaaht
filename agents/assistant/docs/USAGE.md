# Assistant Agent Usage Guide

**Developer Guide for Using and Extending the Assistant Agent**

**Version:** 1.0
**Last Updated:** 2026-02-09

---

## 1. Quick Start

### 1.1 Running the Assistant Agent

```bash
# Development mode
npm run dev

# Production mode
npm run build
npm start

# With custom configuration
ORCHESTRATOR_ADDRESS=unix:///tmp/baaaht.sock \
LLM_GATEWAY_URL=http://localhost:8080 \
npm start
```

### 1.2 Basic Usage Example

```typescript
import { createAgent } from './agent.js';
import { createGrpcClient } from './orchestrator/grpc-client.js';
import { SessionManager } from './session/manager.js';

// Create dependencies
const grpcClient = createGrpcClient('unix:///tmp/orchestrator.sock');
const sessionManager = new SessionManager({
  maxSessions: 100,
  idleTimeout: 3600000, // 1 hour
});

// Create and start agent
const agent = createAgent({
  defaultModel: 'anthropic/claude-sonnet-4-20250514',
  enableStreaming: true,
  debug: true,
}, {
  grpcClient,
  sessionManager,
});

// Initialize and register
await agent.initialize();
await agent.register('assistant-001');

// Start message processing loop
agent.start();

// Handle a message
agent.receiveMessage(
  {
    id: 'msg-001',
    sessionId: 'session-001',
    content: 'Hello, Assistant!',
    type: MessageType.MESSAGE_TYPE_USER,
  },
  async (response) => {
    console.log('Response:', response.content);
  }
);
```

---

## 2. Configuration

### 2.1 Agent Configuration Options

```typescript
interface AgentConfig {
  // Identity
  name?: string;              // Default: 'assistant'
  description?: string;       // Default: 'Primary conversational agent'

  // Communication
  orchestratorUrl?: string;   // Default: 'localhost:50051'
  llmGatewayUrl?: string;     // Default: 'http://localhost:8080'

  // LLM Settings
  defaultModel?: string;      // Default: 'anthropic/claude-sonnet-4-20250514'
  enableStreaming?: boolean;  // Default: true

  // Concurrency
  maxConcurrentMessages?: number;  // Default: 5
  messageTimeout?: number;         // Default: 120000 (2 min)

  // Session Management
  sessionTimeout?: number;         // Default: 3600000 (1 hour)
  maxSessionMessages?: number;     // Default: 100
  contextWindowSize?: number;      // Default: 200000 tokens

  // Diagnostics
  debug?: boolean;           // Default: false
  labels?: Record<string, string>;  // Custom labels for metrics
}
```

### 2.2 Environment Variables

| Variable | Type | Default | Description |
|---|---|---|---|
| `ORCHESTRATOR_ADDRESS` | string | `unix:///tmp/orchestrator.sock` | gRPC server address |
| `LLM_GATEWAY_URL` | string | `http://localhost:8080` | LLM Gateway base URL |
| `AGENT_NAME` | string | `assistant` | Agent identifier |
| `LOG_LEVEL` | string | `info` | Logging level (debug, info, warn, error) |
| `DEFAULT_MODEL` | string | `anthropic/claude-sonnet-4-20250514` | Default LLM model |

### 2.3 Session Manager Configuration

```typescript
interface SessionManagerConfig {
  maxSessions: number;       // Maximum active sessions
  idleTimeout: number;       // Milliseconds before idle session is archived
  timeout: number;           // Maximum session lifetime
  maxDuration: number;       // Hard limit on session duration
}

const sessionManager = new SessionManager({
  maxSessions: 100,
  idleTimeout: 3600000,      // 1 hour
  timeout: 3600000,
  maxDuration: 86400000,     // 24 hours
});
```

---

## 3. Message Handling

### 3.1 Sending Messages to the Agent

```typescript
// Simple message
await agent.receiveMessage(
  {
    id: 'msg-001',
    sessionId: 'session-001',
    content: 'What can you help me with?',
    type: MessageType.MESSAGE_TYPE_USER,
  },
  async (response) => {
    if (response.error) {
      console.error('Error:', response.error.message);
      return;
    }
    console.log('Response:', response.content);
    console.log('Usage:', response.usage);
  }
);

// Message with labels
await agent.receiveMessage(
  {
    id: 'msg-002',
    sessionId: 'session-001',
    content: 'Search for recent TypeScript news',
    type: MessageType.MESSAGE_TYPE_USER,
    timestamp: new Date(),
    labels: {
      source: 'slack',
      channel: 'general',
      userId: 'user-123',
    },
  },
  handleResponse
);
```

### 3.2 Response Structure

```typescript
interface AgentResponse {
  // Response content
  content: string;              // Assistant's reply

  // Streaming info
  streamed: boolean;            // Was response streamed?

  // Tool calls made
  toolCalls?: ToolCallInfo[];   // Delegations performed

  // Token usage
  usage?: {
    promptTokens: number;
    completionTokens: number;
    totalTokens: number;
  };

  // Completion reason
  finishReason?: 'stop' | 'length' | 'tool_calls' | 'error';

  // Error (if failed)
  error?: AgentError;

  // Metadata
  metadata: {
    responseId: string;
    sessionId: string;
    requestId: string;
    timestamp: Date;
    processingDurationMs: number;
    model: string;
    provider: string;
  };
}
```

### 3.3 Handling Errors

```typescript
agent.on(AgentEventType.MESSAGE_FAILED, (event) => {
  const { messageId, sessionId, error } = event.data;

  console.error(`Message ${messageId} failed:`, error);

  // Check if retryable
  if (error.retryable) {
    // Implement retry logic
  }
});

// Error response checking
await agent.receiveMessage(message, (response) => {
  if (response.error) {
    switch (response.error.code) {
      case AgentErrorCode.NOT_READY:
        // Agent not ready yet
        break;
      case AgentErrorCode.MESSAGE_INVALID:
        // Invalid message format
        break;
      case AgentErrorCode.LLM_REQUEST_FAILED:
        // LLM communication failed
        break;
      default:
        // Other error
    }
  }
});
```

---

## 4. Event Monitoring

### 4.1 Listening to Events

```typescript
// Lifecycle events
agent.on(AgentEventType.READY, () => {
  console.log('Agent is ready for messages');
});

agent.on(AgentEventType.SHUTDOWN, () => {
  console.log('Agent is shutting down');
});

// Message events
agent.on(AgentEventType.MESSAGE_RECEIVED, (event) => {
  console.log('Message received:', event.data.messageId);
});

agent.on(AgentEventType.MESSAGE_PROCESSED, (event) => {
  const { messageId, durationMs, toolCallCount } = event.data;
  console.log(`Message ${messageId} processed in ${durationMs}ms`);
  console.log(`Tool calls: ${toolCallCount}`);
});

// LLM events
agent.on(AgentEventType.LLM_STREAM_START, (event) => {
  console.log('LLM stream started');
});

agent.on(AgentEventType.LLM_STREAM_CHUNK, (event) => {
  // Forward chunk to user
  const chunk = event.data.chunk;
  if (chunk.type === 'content') {
    console.log('Content chunk:', chunk.data.content);
  }
});

agent.on(AgentEventType.LLM_STREAM_END, (event) => {
  console.log('LLM stream ended:', event.data.finishReason);
});

// Tool call events
agent.on(AgentEventType.TOOL_CALL_START, (event) => {
  console.log('Tool call started:', event.data.toolName);
});

agent.on(AgentEventType.TOOL_CALL_COMPLETE, (event) => {
  const { toolName, durationMs, success } = event.data;
  console.log(`Tool ${toolName} ${success ? 'succeeded' : 'failed'} in ${durationMs}ms`);
});
```

### 4.2 Event Types Reference

See [ARCHITECTURE.md](ARCHITECTURE.md#6-event-system) for the complete list of event types.

---

## 5. Session Management

### 5.1 Working with Sessions

```typescript
// Get a session
const session = await agent.sessionManager.get('session-001');

console.log('Session ID:', session.id);
console.log('Message count:', session.context.messages.length);
console.log('State:', session.state);

// Create a new session
const newSession = await agent.sessionManager.create(
  {
    name: 'My Session',
    ownerId: 'user-123',
  },
  {
    maxDuration: 3600000, // 1 hour
  }
);

// List messages in a session
const messages = await agent.sessionManager.getMessages('session-001', 10);
for (const message of messages) {
  console.log(`[${message.role}] ${message.content}`);
}

// Delete a session
await agent.sessionManager.delete('session-001');
```

### 5.2 Session State Monitoring

```typescript
agent.on(AgentEventType.SESSION_CREATED, (event) => {
  console.log('New session:', event.data.sessionId);
});

// Get session statistics
const stats = agent.sessionManager.getStats();
console.log('Active sessions:', stats.active);
console.log('Idle sessions:', stats.idle);
console.log('Archived sessions:', stats.archived);
console.log('Total messages:', stats.totalMessages);
```

---

## 6. Delegation

### 6.1 Understanding Delegation

The Assistant delegates work to specialized agents via the `delegate` tool. This happens automatically when the LLM determines it needs to perform an action.

**You don't manually invoke delegations** — the LLM decides when to delegate based on:
- The user's request
- The delegate tool's description
- The available operations

### 6.2 Delegation Flow

```
User: "Read the file /tmp/data.txt"
   │
   ▼
Assistant receives message
   │
   ▼
LLM determines a file read is needed
   │
   ▼
LLM invokes delegate tool:
   {
     target: "worker",
     operation: "read_file",
     parameters: { path: "/tmp/data.txt" }
   }
   │
   ▼
Orchestrator spawns Worker agent
   │
   ▼
Worker reads file and returns result
   │
   ▼
Result returned as delegate tool result
   │
   ▼
LLM generates final response to user
```

### 6.3 Custom Delegation Handlers

To add custom delegation logic (e.g., for new specialized agents):

```typescript
// src/tools/my-delegation.ts
import { DelegateParams, DelegateResult } from './types.js';

export class MyDelegation {
  async delegate(params: DelegateParams, sessionId: string): Promise<DelegateResult> {
    // 1. Validate parameters
    // 2. Create task configuration
    // 3. Execute via Orchestrator
    // 4. Wait for completion
    // 5. Return result
  }
}

// Integrate with agent
// In agent.ts executeDelegation():
if (params.target === 'my_agent') {
  const myDelegation = new MyDelegation(this.grpcClient);
  return await myDelegation.delegate(params, sessionId);
}
```

See [DELEGATION.md](DELEGATION.md) for detailed delegation patterns.

---

## 7. Testing

### 7.1 Unit Testing

```typescript
// tests/unit/my-feature.test.ts
import { describe, it, expect, beforeEach } from '@jest/globals';
import { createAgent } from '../../src/agent.js';
import { MockGrpcClient } from '../mocks/orchestrator.mock.js';

describe('Agent', () => {
  let agent;
  let mockGrpc;

  beforeEach(() => {
    mockGrpc = new MockGrpcClient();
    agent = createAgent({}, {
      grpcClient: mockGrpc,
      sessionManager: new SessionManager(),
    });
  });

  it('should process messages', async () => {
    await agent.initialize();
    await agent.register('test-agent');

    const responsePromise = new Promise((resolve) => {
      agent.receiveMessage(
        {
          id: 'test-msg',
          sessionId: 'test-session',
          content: 'Hello',
          type: MessageType.MESSAGE_TYPE_USER,
        },
        resolve
      );
    });

    const response = await responsePromise;
    expect(response.content).toBeTruthy();
  });
});
```

### 7.2 Running Tests

```bash
# All tests
npm test

# Unit tests only
npm run test:unit

# Integration tests only
npm run test:integration

# With coverage
npm run test:coverage

# Watch mode
npm run test:watch
```

### 7.3 Mocking the Orchestrator

```typescript
// tests/mocks/orchestrator.mock.ts
export class MockGrpcClient {
  async executeTask(request, metadata, callback) {
    // Simulate task execution
    callback(null, {
      taskId: 'mock-task-001',
      state: TaskState.TASK_STATE_COMPLETED,
      result: {
        outputText: 'Mock result',
      },
    });
  }

  async getTaskStatus(request, metadata, callback) {
    // Return mock task status
  }

  async registerAgent(request, metadata, callback) {
    // Return mock agent ID
  }
}
```

---

## 8. Debugging

### 8.1 Enable Debug Mode

```typescript
const agent = createAgent({
  debug: true,  // Enables verbose logging
}, dependencies);
```

### 8.2 Log Output

With debug mode enabled, the agent logs:
- Message lifecycle events
- LLM request/response details
- Tool call execution
- Delegation status
- Error stack traces

```
[Agent:assistant-001] Agent registered with ID: assistant-001
[Agent:assistant-001] Message received: msg_001 (session: session-001)
[Agent:assistant-001] Session created: session-001
[Agent:assistant-001] LLM stream started
[Agent:assistant-001] Tool call started: delegate
[Agent:assistant-001] Tool call complete: delegate (1234ms)
[Agent:assistant-001] LLM stream ended: stop
[Agent:assistant-001] Message processed: msg_001 (2345ms, 1 tool calls)
```

### 8.3 Common Issues

| Issue | Cause | Solution |
|---|---|---|
| Agent not ready | `register()` not called | Call `await agent.register(agentId)` before sending messages |
| LLM timeout | Gateway unreachable | Check `LLM_GATEWAY_URL` and Gateway health |
| Delegation fails | Worker agent not available | Check Orchestrator logs for container spawn errors |
| Session not found | Invalid session ID | Ensure session exists before sending messages |

---

## 9. Extending the Agent

### 9.1 Adding New Tool Operations

To add a new operation to the delegate tool:

1. **Define the operation enum:**
   ```typescript
   // src/tools/types.ts
   export enum DelegateOperation {
     // ... existing operations
     MY_NEW_OPERATION = 'my_new_operation',
   }
   ```

2. **Add parameter types:**
   ```typescript
   export interface MyNewOperationParams {
     param1: string;
     param2?: number;
   }
   ```

3. **Update validation:**
   ```typescript
   // src/tools/delegate.ts
   function validateOperationParameters(operation, parameters) {
     switch (operation) {
       // ... existing cases
       case DelegateOperation.MY_NEW_OPERATION:
         validateMyNewOperationParams(parameters);
         break;
     }
   }
   ```

4. **Implement delegation handler:**
   ```typescript
   // src/tools/my-delegation.ts
   export class MyDelegation {
     async delegate(params: DelegateParams, sessionId: string): Promise<DelegateResult> {
       // Implementation
     }
   }
   ```

5. **Wire up in agent:**
   ```typescript
   // src/agent.ts
   private async executeDelegation(params: DelegateParams): Promise<ToolResult> {
     switch (params.target) {
       // ... existing cases
       case 'my_agent':
         const myDelegation = new MyDelegation(this.grpcClient);
         return await myDelegation.delegate(params, sessionId);
     }
   }
   ```

### 9.2 Custom Event Handlers

```typescript
// Add custom monitoring
agent.on(AgentEventType.MESSAGE_PROCESSED, (event) => {
  // Send metrics to monitoring system
  metrics.track('agent.message.processed', {
    duration: event.data.durationMs,
    toolCalls: event.data.toolCallCount,
  });
});

// Custom error logging
agent.on(AgentEventType.ERROR, (event) => {
  logger.error('Agent error', {
    error: event.error.message,
    code: event.error.code,
    details: event.error.details,
  });
});
```

### 9.3 Middleware Pattern

For custom message processing logic:

```typescript
class AgentMiddleware {
  constructor(private agent: Agent) {}

  async processMessage(message: AgentMessage): Promise<AgentMessage> {
    // Pre-process message
    const processed = this.transform(message);

    // Send to agent
    await this.agent.receiveMessage(processed, this.handleResponse.bind(this));

    return processed;
  }

  private transform(message: AgentMessage): AgentMessage {
    // Custom transformation logic
    return message;
  }

  private async handleResponse(response: AgentResponse): Promise<void> {
    // Post-process response
    this.emitMetrics(response);
  }
}
```

---

## 10. Performance Tuning

### 10.1 Concurrency Settings

```typescript
const agent = createAgent({
  // Increase for higher throughput
  maxConcurrentMessages: 10,

  // Decrease timeout for faster failure detection
  messageTimeout: 60000, // 1 minute
}, dependencies);
```

### 10.2 Session Management

```typescript
const sessionManager = new SessionManager({
  // Reduce memory footprint
  maxSessions: 50,

  // Archive idle sessions faster
  idleTimeout: 1800000, // 30 minutes

  // Limit message history per session
  maxMessages: 50,
});
```

### 10.3 Streaming vs Non-Streaming

```typescript
// Streaming: Lower perceived latency, more events
const agent = createAgent({
  enableStreaming: true,
}, dependencies);

// Non-streaming: Simpler, less overhead
const agent = createAgent({
  enableStreaming: false,
}, dependencies);
```

---

## 11. Deployment

### 11.1 Container Configuration

```dockerfile
# Dockerfile
FROM node:22-alpine

WORKDIR /app

# Copy package files
COPY package*.json ./
COPY agents/assistant/package*.json ./agents/assistant/

# Install dependencies
RUN cd agents/assistant && npm ci --only=production

# Copy source
COPY agents/assistant ./agents/assistant

# Build
RUN cd agents/assistant && npm run build

# Run
CMD ["node", "agents/assistant/dist/index.js"]
```

### 11.2 Health Checks

```typescript
// src/health.ts
export function createHealthCheck(agent: Agent) {
  return {
    async check(): Promise<{ healthy: boolean; details: unknown }> {
      const status = agent.getStatus();

      return {
        healthy: status.state === AgentState.AGENT_STATE_IDLE,
        details: {
          uptime: status.uptimeSeconds,
          activeSessions: status.activeSessions,
          processingMessages: status.processingMessages,
        },
      };
    },
  };
}

// Use in HTTP endpoint
app.get('/health', async (req, res) => {
  const health = await healthCheck.check();
  res.status(health.healthy ? 200 : 503).json(health.details);
});
```

---

## 12. Best Practices

### 12.1 Error Handling

```typescript
// Always check for errors
agent.receiveMessage(message, (response) => {
  if (response.error) {
    // Log and handle
    logger.error('Message failed', response.error);
    return;
  }

  // Process successful response
});
```

### 12.2 Resource Cleanup

```typescript
// Always shutdown gracefully
process.on('SIGTERM', async () => {
  console.log('Shutting down agent...');
  await agent.shutdown();
  process.exit(0);
});
```

### 12.3 Session Lifecycle

```typescript
// Create sessions with appropriate metadata
const session = await sessionManager.create({
  name: 'User Support Session',
  ownerId: userId,
  labels: {
    channel: 'web',
    tier: 'premium',
  },
});

// Monitor for session expiration
agent.on(AgentEventType.SESSION_ARCHIVED, (event) => {
  // Clean up resources
  cleanupSessionResources(event.data.sessionId);
});
```

### 12.4 Event Listener Management

```typescript
// Remove listeners when no longer needed
const messageHandler = (event) => console.log(event.data);
agent.on(AgentEventType.MESSAGE_RECEIVED, messageHandler);

// Later:
agent.off(AgentEventType.MESSAGE_RECEIVED, messageHandler);

// Or use once() for single-fire handlers
agent.once(AgentEventType.READY, () => {
  console.log('Agent ready!');
});
```

---

## 13. Troubleshooting

### 13.1 Common Issues and Solutions

**Agent fails to register:**
- Check Orchestrator is running
- Verify `ORCHESTRATOR_ADDRESS` is correct
- Check gRPC connection logs

**Messages time out:**
- Increase `messageTimeout` configuration
- Check LLM Gateway health
- Verify network connectivity

**Delegations fail:**
- Ensure specialized agents are available
- Check Orchestrator container logs
- Verify agent has proper permissions

**Memory leaks:**
- Monitor session count
- Set appropriate `maxSessions` limit
- Implement session archival

### 13.2 Diagnostic Commands

```bash
# Check agent status
curl http://localhost:3000/status

# View active sessions
curl http://localhost:3000/sessions

# Get agent metrics
curl http://localhost:3000/metrics

# Trigger graceful shutdown
curl -X POST http://localhost:3000/shutdown
```

---

## 14. References

- [Architecture Documentation](ARCHITECTURE.md) — Technical design details
- [Delegation Patterns](DELEGATION.md) — Delegation architecture guide
- [Main README](../README.md) — Project overview

---

**Copyright © 2026 baaaht project**
