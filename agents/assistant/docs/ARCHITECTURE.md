# Assistant Agent Architecture

**Primary Conversational Agent with Tool Delegation**

**Version:** 1.0
**Last Updated:** 2026-02-09
**Status:** Stable — Phase 1 Complete

> **Document Classification:** This is the **Technical Architecture Specification** for the Assistant Agent.
> It covers the agent's design, component interfaces, delegation patterns, and implementation details.
> For general platform architecture, see [../../docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md).

---

## 1. Executive Summary

The Assistant Agent is the **primary conversational interface** for the baaaht platform. It is the first point of contact for all user messages and is responsible for maintaining clean, focused conversation context while delegating any work requiring tools to specialized agents.

### Key Design Principles

| Principle | Description |
|---|---|
| **Delegation-Only Model** | The Assistant has exactly one tool: `delegate`. All tool-requiring work is dispatched to specialized agents (Worker, Researcher, Coder). |
| **Context Purity** | By never executing tools directly, the Assistant's context window remains free of raw tool execution traces, containing only user messages, high-level delegation requests, delegation results, and its own responses. |
| **Model Agnostic** | The Assistant communicates with LLMs through the LLM Gateway, supporting any configured model provider (Claude, GPT, local models). |
| **Session Isolation** | Each user/group conversation has an isolated session with its own context window, memory store, and lifecycle. |
| **Event-Driven** | Built on EventEmitter for observability. All significant lifecycle events are emitted for monitoring and debugging. |

---

## 2. Agent Taxonomy & Role

### 2.1 Position in Agent Ecosystem

```
┌─────────────────────────────────────────────────────────────┐
│                        baaaht Platform                       │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│   ┌──────────────┐      ┌──────────────┐                    │
│   │   Control    │      │   Control    │                    │
│   │  Channels    │      │  Channels    │                    │
│   │ (TUI/Web/    │──────▶│  (Slack/etc) │                    │
│   │  Slack)      │      │              │                    │
│   └──────┬───────┘      └──────┬───────┘                    │
│          │                     │                            │
│          └──────────┬──────────┘                            │
│                     ▼                                       │
│          ┌───────────────────┐                              │
│          │   Orchestrator    │                              │
│          │    (Go, Host)     │                              │
│          └─────────┬─────────┘                              │
│                    │                                        │
│                    │ gRPC (AgentMessage)                    │
│                    ▼                                        │
│          ┌─────────────────────────────────────────┐        │
│          │         Assistant Agent                 │        │
│          │         (This Document)                 │        │
│          │                                         │        │
│          │  ┌───────────────────────────────────┐  │        │
│          │  │   Session Manager                 │  │        │
│          │  │   - Context Window                │  │        │
│          │  │   - Message History               │  │        │
│          │  │   - Lifecycle                     │  │        │
│          │  └───────────────────────────────────┘  │        │
│          │  ┌───────────────────────────────────┐  │        │
│          │  │   LLM Gateway Client              │  │        │
│          │  │   - Streaming Completions         │  │        │
│          │  │   - Tool Calling                  │  │        │
│          │  └───────────────────────────────────┘  │        │
│          │  ┌───────────────────────────────────┐  │        │
│          │  │   Delegate Tool (ONLY TOOL)       │  │        │
│          │  │   - Worker Delegation             │  │        │
│          │  │   - Researcher Delegation (P2)    │  │        │
│          │  │   - Coder Delegation (P2)         │  │        │
│          │  └───────────────────────────────────┘  │        │
│          └─────────────────────────────────────────┘        │
│                    │                                        │
│                    │ Delegate (via Orchestrator)            │
│                    ▼                                        │
│          ┌─────────────────────────────────────────┐        │
│          │      Specialized Agents                 │        │
│          │                                         │        │
│          │  ┌─────────┐ ┌──────────┐ ┌─────────┐  │        │
│          │  │ Worker  │ │Researcher│ │  Coder  │  │        │
│          │  │         │ │          │ │         │  │        │
│          │  │ Tools:  │ │ Tools:   │ │ Tools:  │  │        │
│          │  │ - Files │ │ - Web    │ │ - Exec  │  │        │
│          │  │ - Web   │ │ - Deep   │ │ - Code  │  │        │
│          │  │         │ │   Search │ │         │  │        │
│          │  └─────────┘ └──────────┘ └─────────┘  │        │
│          └─────────────────────────────────────────┘        │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Comparison with Other Agents

| Agent | Primary Role | Tool Access | Model Guidance | Lifecycle |
|---|---|---|---|---|
| **Assistant** | Conversational interface; answers questions directly or delegates when action is needed | `delegate` only — no direct tools | Balanced (e.g., Sonnet, GPT-4o) | Persistent per session |
| **Worker** | General-purpose task execution | Files, web, basic operations | Balanced | Ephemeral per delegation |
| **Researcher** | Deep research with source synthesis | Web, files (read-only) | High-capability (e.g., Opus) | Ephemeral per delegation |
| **Coder** | Code analysis, generation, execution | Files, exec, code tools | Code-specialized | Ephemeral per delegation |
| **Memory** | Session archival and memory extraction | Read session, write memory | Fast/cheap (e.g., Haiku) | Orchestrator-triggered |
| **Heartbeat** | Scheduled proactive tasks | Web, message | Fast/cheap | Orchestrator-triggered |

---

## 3. Core Components

### 3.1 Agent Class (`src/agent.ts`)

The central orchestrator for all assistant operations.

```typescript
class Agent extends EventEmitter {
  // Lifecycle
  async initialize(): Promise<void>
  async register(agentId: string): Promise<void>
  start(): void
  async shutdown(): Promise<void>

  // Message Processing
  async receiveMessage(message: AgentMessage, respond: RespondFn): Promise<void>

  // Status
  getStatus(): AgentStatus
}
```

**Responsibilities:**
- Message queue management with concurrency control
- Session lifecycle via SessionManager
- LLM communication via LLMGatewayClient
- Tool call execution via delegation classes
- Event emission for observability

**State Machine:**
```
INITIALIZING → REGISTERING → IDLE → PROCESSING → IDLE → ... → UNREGISTERING → TERMINATED
```

### 3.2 Session Manager (`src/session/manager.ts`)

Manages conversation state across multiple messages.

```typescript
class SessionManager {
  // Session CRUD
  async create(metadata: SessionMetadata, options?: SessionOptions): Promise<Session>
  async get(sessionId: string): Promise<Session>
  async delete(sessionId: string): Promise<void>

  // Message Management
  async addMessage(sessionId: string, message: Message): Promise<void>
  async getMessages(sessionId: string, limit?: number): Promise<Message[]>

  // Lifecycle
  async close(): Promise<void>
  getStats(): SessionStats
}
```

**Session Data Model:**
```typescript
interface Session {
  id: string;
  metadata: SessionMetadata;
  context: {
    messages: Message[];  // Conversation history
    windowSize: number;   // Current context window size
  };
  state: SessionState;
  createdAt: Date;
  lastActivityAt: Date;
}
```

**Context Window Management:**
- Tracks token usage per session
- Enforces `maxSessionMessages` limit
- Compaction planned for Phase 3

### 3.3 LLM Gateway Client (`src/llm/gateway-client.ts`)

HTTP client for LLM API communication with streaming support.

```typescript
class LLMGatewayClient {
  // Completions
  async complete(params: CompletionParams): Promise<CompletionResult>
  stream(params: CompletionParams): AsyncIterableStream<StreamingChunk>

  // Models
  async listModels(): Promise<ModelInfo[]>

  // Health
  async healthCheck(): Promise<boolean>
}
```

**Streaming Protocol:**
- Server-Sent Events (SSE) over HTTP
- Chunk types: `content`, `toolCall`, `usage`, `complete`, `error`
- Automatic reconnection on transient failures

### 3.4 Delegate Tool (`src/tools/delegate.ts`)

The Assistant's sole tool interface.

```typescript
interface DelegateParams {
  target: 'worker' | 'researcher' | 'coder';
  operation: string;
  parameters: Record<string, unknown>;
  timeout?: number;
  priority?: TaskPriority;
}

interface DelegateResult {
  success: boolean;
  taskId?: string;
  data?: unknown;
  output?: string;
  error?: string;
  taskState?: TaskState;
  metadata: DelegateMetadata;
}
```

**Available Operations:**

| Target | Operations |
|---|---|
| **worker** | `read_file`, `write_file`, `delete_file`, `list_files`, `search_files`, `web_search`, `web_fetch` |
| **researcher** (Phase 2) | `deep_research`, `synthesize_sources` |
| **coder** (Phase 2) | `analyze_code`, `generate_code`, `review_code`, `execute_code` |

### 3.5 Delegation Implementations

#### Worker Delegation (`src/tools/worker-delegation.ts`)

```typescript
class WorkerDelegation {
  async delegate(params: DelegateParams, sessionId: string): Promise<DelegateResult>
}
```

**Flow:**
1. Create `TaskConfig` from delegation parameters
2. Map operation to `TaskType` (FILE_OPERATION, NETWORK_REQUEST, etc.)
3. Call `Orchestrator.ExecuteTask` via gRPC
4. Poll task status until completion
5. Parse and return result

**Parameter Mapping:**
- `operation` → `command` (e.g., `read_file`)
- `parameters` → `arguments[]` + `environment`
- `priority` → `TaskPriority` enum
- `timeout` → `timeoutNs` (nanoseconds)

#### Researcher/Coder Delegation (Phase 2)

Similar structure to WorkerDelegation with:
- Different operation sets
- Longer default timeouts
- Potentially different priority handling

---

## 4. Message Processing Flow

### 4.1 Complete Request Lifecycle

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          User Message Flow                               │
└──────────────────────────────────────────────────────────────────────────┘

1. INBOUND MESSAGE
   │
   │  AgentMessage {
   │    id: string
   │    sessionId: string
   │    content: string
   │    type: MessageType
   │  }
   │
   ▼
2. RECEIVE (receiveMessage)
   │
   ├─▶ Validate message (content, sessionId)
   ├─▶ Create ProcessMessage wrapper
   ├─▶ Add to messageQueue
   ├─▶ Emit MESSAGE_RECEIVED event
   │
   ▼
3. PROCESS (processMessage - from queue)
   │
   ├─▶ Ensure session exists (create if needed)
   ├─▶ Add user message to session history
   ├─▶ Build LLM request with:
   │    - Session context (messages)
   │    - Delegate tool definition
   │    - Model parameters
   │
   ▼
4. LLM INVOCATION (processWithLLM)
   │
   │  If enableStreaming = true:
   │  ├─▶ LLMGatewayClient.stream()
   │  ├─▶ For each chunk:
   │  │    ├─▶ content → accumulate
   │  │    ├─▶ toolCall → executeToolCall()
   │  │    ├─▶ usage → record
   │  │    └─▶ error → abort
   │  │
   │  Else (non-streaming):
   │  └─▶ LLMGatewayClient.complete()
   │     └─▶ Execute tool calls sequentially
   │
   ▼
5. TOOL CALL EXECUTION (executeToolCall)
   │
   ├─▶ Parse tool name and arguments
   ├─▶ If tool == 'delegate':
   │    ├─▶ Validate delegate params
   │    ├─▶ Execute delegation (Worker/Researcher/Coder)
   │    └─▶ Return result
   ├─▶ Else: error (unknown tool)
   │
   ▼
6. RESPONSE ASSEMBLY
   │
   ├─▶ Add assistant response to session history
   ├─▶ Build AgentResponse {
   │    content: string
   │    streamed: boolean
   │    toolCalls: ToolCallInfo[]
   │    usage: TokenUsage
   │    metadata: ResponseMetadata
   │  }
   │
   ▼
7. RESPOND (via callback)
   │
   └─▶ Emit MESSAGE_PROCESSED event
```

### 4.2 Tool Call Handling

```
┌──────────────────────────────────────────────────────────────────────────┐
│                       Tool Call Processing                               │
└──────────────────────────────────────────────────────────────────────────┘

LLM Response (streaming or complete)
   │
   │  ToolCall {
   │    id: string
   │    name: 'delegate'  // Only tool available
   │    arguments: JSON string
   │  }
   │
   ▼
Parse Arguments
   │
   ├─▶ JSON.parse(arguments)
   ├─▶ Validate DelegateParams
   │    ├─▶ target: valid agent?
   │    ├─▶ operation: supported by target?
   │    ├─▶ parameters: match operation schema?
   │    └─▶ timeout: within limits?
   │
   ▼
Execute Delegation
   │
   │  1. WorkerDelegation.delegate()
   │     ├─▶ Create TaskConfig
   │     ├─▶ gRPC: ExecuteTask()
   │     ├─▶ Poll task status
   │     └─▶ Return DelegateResult
   │
   │  OR (Phase 2)
   │
   │  2. ResearcherDelegation.delegate()
   │     └─▶ Similar flow, different operations
   │
   │  OR (Phase 2)
   │
   │  3. CoderDelegation.delegate()
   │     └─▶ Similar flow, different operations
   │
   ▼
Build ToolResult
   │
   └─▶ Return to LLM as tool result message
      └─▶ LLM generates final response to user
```

---

## 5. Communication Protocols

### 5.1 Orchestrator gRPC Protocol

**Service:** `AgentService` (defined in `proto/agent.proto`)

```protobuf
service AgentService {
  // Agent registration (called by agent)
  rpc RegisterAgent(RegisterAgentRequest) returns (RegisterAgentResponse);

  // Bidirectional message streaming
  rpc MessageStream(stream AgentMessage) returns (stream AgentMessage);

  // Task execution (for delegated agents)
  rpc ExecuteTask(ExecuteTaskRequest) returns (ExecuteTaskResponse);

  // Task status queries
  rpc GetTaskStatus(GetTaskStatusRequest) returns (GetTaskStatusResponse);
}
```

**Message Types:**
```typescript
interface AgentMessage {
  id: string;
  sessionId: string;
  type: MessageType;  // USER, ASSISTANT, SYSTEM
  content: string;
  timestamp: Date;
  labels?: Record<string, string>;
}
```

### 5.2 Orchestrator-Mediated LLM gRPC Protocol

**Endpoint:** Uses orchestrator gRPC address via `ORCHESTRATOR_URL` (default: `unix:///tmp/baaaht-grpc.sock`)

**RPCs:**
- `CompleteLLM` - Non-streaming completions
- `StreamLLM` - Streaming completions (bidirectional stream)
- `ListModels` - List available models
- `HealthCheck` - LLM service health check

**Completion Request:**
```typescript
interface CompletionParams {
  model: string;           // Model identifier
  messages: LLMMessage[];  // Conversation history
  maxTokens?: number;      // Max tokens in response
  temperature?: number;    // Sampling temperature
  tools?: Tool[];          // Available tools (delegate)
  sessionId: string;       // For token tracking
}
```

---

## 6. Event System

### 6.1 Event Types

```typescript
enum AgentEventType {
  // Lifecycle
  INITIALIZED = 'agent.initialized',
  REGISTERED = 'agent.registered',
  READY = 'agent.ready',
  UNREGISTERED = 'agent.unregistered',
  SHUTDOWN = 'agent.shutdown',

  // Messages
  MESSAGE_RECEIVED = 'message.received',
  MESSAGE_PROCESSING = 'message.processing',
  MESSAGE_PROCESSED = 'message.processed',
  MESSAGE_FAILED = 'message.failed',

  // LLM
  LLM_REQUEST_START = 'llm.request_start',
  LLM_STREAM_START = 'llm.stream_start',
  LLM_STREAM_CHUNK = 'llm.stream_chunk',
  LLM_STREAM_END = 'llm.stream_end',
  LLM_REQUEST_COMPLETE = 'llm.request_complete',
  LLM_ERROR = 'llm.error',

  // Tools
  TOOL_CALL_START = 'tool.call_start',
  TOOL_CALL_COMPLETE = 'tool.call_complete',
  TOOL_CALL_FAILED = 'tool.call_failed',

  // Sessions
  SESSION_CREATED = 'session.created',

  // Errors
  ERROR = 'error',
}
```

### 6.2 Event Payloads

```typescript
interface AgentEvent {
  type: AgentEventType;
  timestamp: Date;
  data?: Record<string, unknown>;
  error?: Error;
}

// Example: MESSAGE_PROCESSED
{
  type: 'message.processed',
  timestamp: '2026-02-09T10:30:00Z',
  data: {
    messageId: 'msg_123',
    sessionId: 'session_abc',
    durationMs: 1234,
    toolCallCount: 2,
  }
}
```

---

## 7. Configuration

### 7.1 Environment Variables

| Variable | Default | Description |
|---|---|---|
| `ORCHESTRATOR_URL` | `unix:///tmp/baaaht-grpc.sock` | Orchestrator gRPC address |
| `AGENT_NAME` | `assistant` | Agent identifier |
| `LOG_LEVEL` | `info` | Logging verbosity |
| `DEFAULT_MODEL` | `anthropic/claude-sonnet-4-20250514` | Default LLM model |

### 7.2 Agent Configuration

```typescript
interface AgentConfig {
  name?: string;
  description?: string;
  orchestratorUrl?: string;
  defaultModel?: string;
  maxConcurrentMessages?: number;
  messageTimeout?: number;
  heartbeatInterval?: number;
  maxRetries?: number;
  sessionTimeout?: number;
  maxSessionMessages?: number;
  contextWindowSize?: number;
  labels?: Record<string, string>;
  enableStreaming?: boolean;
  debug?: boolean;
}
```

---

## 8. Error Handling

### 8.1 Error Types

```typescript
enum AgentErrorCode {
  INIT_FAILED = 'INIT_FAILED',
  REGISTRATION_FAILED = 'REGISTRATION_FAILED',
  NOT_READY = 'NOT_READY',
  MESSAGE_INVALID = 'MESSAGE_INVALID',
  MESSAGE_PROCESSING_FAILED = 'MESSAGE_PROCESSING_FAILED',
  SHUTDOWN_IN_PROGRESS = 'SHUTDOWN_IN_PROGRESS',
  LLM_REQUEST_FAILED = 'LLM_REQUEST_FAILED',
  LLM_STREAM_FAILED = 'LLM_STREAM_FAILED',
  TOOL_NOT_AVAILABLE = 'TOOL_NOT_AVAILABLE',
  TOOL_EXECUTION_FAILED = 'TOOL_EXECUTION_FAILED',
}

interface AgentError extends Error {
  code: AgentErrorCode;
  retryable: boolean;
  details?: Record<string, unknown>;
}
```

### 8.2 Error Recovery

| Error | Retryable | Recovery Strategy |
|---|---|---|
| LLM request timeout | Yes | Retry with exponential backoff (max 3 attempts) |
| LLM stream failure | Yes | Re-invoke with non-streaming fallback |
| Tool execution timeout | No | Return error to LLM, let it decide next action |
| Orchestrator disconnected | Yes | Reconnect with exponential backoff |
| Invalid message | No | Return error immediately |

---

## 9. Security Considerations

### 9.1 Agent Isolation

The Assistant Agent runs in a container with:
- **No direct network access** (communicates via Orchestrator proxy)
- **Scoped filesystem mounts** (session-specific directories only)
- **No credential access** (API keys held by Orchestrator/LGW)
- **Resource limits** (CPU, memory, disk quotas enforced by Orchestrator)

### 9.2 Input Validation

- All delegate parameters validated before execution
- Path traversal checks on file operations
- Timeout enforcement on all delegations
- Tool name whitelisting (only `delegate` allowed)

### 9.3 Session Isolation

- Each session has isolated context window
- No cross-session data leakage
- Session IDs are opaque, non-guessable strings

---

## 10. Performance Characteristics

### 10.1 Concurrency

- **Max concurrent messages:** Configurable (default: 5)
- **Message queue:** In-memory, unbounded
- **Processing model:** One message per agent at a time (per session)

### 10.2 Latency Breakdown

| Component | Typical Latency | Notes |
|---|---|---|
| Orchestrator routing | < 5ms | Local gRPC over UDS |
| LLM Gateway request | Variable | Depends on model/provider |
| LLM first token | 500ms - 2s | Time to first token |
| Delegation spawn | 100ms - 1s | Container cold start |
| Tool execution | Variable | Depends on operation |

### 10.3 Memory Usage

- **Base memory:** ~50MB (Node.js runtime)
- **Per session:** ~1-5MB (message history)
- **Per active message:** ~10MB (LLM context + processing)

---

## 11. Testing Strategy

### 11.1 Unit Tests

Located in `tests/unit/`:
- `session-manager.test.ts` - Session lifecycle, context management
- `tool-calling.test.ts` - Tool call aggregation, execution
- `response-stream.test.ts` - Streaming response handling
- `shutdown.test.ts` - Graceful shutdown behavior

### 11.2 Integration Tests

Located in `tests/integration/`:
- `agent.test.ts` - End-to-end message processing with mocked Orchestrator

### 11.3 Mocking

- `tests/mocks/orchestrator.mock.ts` - gRPC client mocks
- LLM Gateway client accepts mock URLs for testing

---

## 12. Future Enhancements

### Phase 2: Enhanced Delegation
- [ ] Researcher agent integration
- [ ] Coder agent integration
- [ ] Parallel delegation support
- [ ] Delegation result caching

### Phase 3: Context Management
- [ ] Automatic context compaction
- [ ] Session branching
- [ ] Context window hints for LLM
- [ ] Long-term memory integration

### Phase 4: Advanced Features
- [ ] Multi-turn delegation (agent-to-agent chains)
- [ ] Streaming delegation results
- [ ] Delegation priority queuing
- [ ] Delegation analytics

---

## 13. References

- [Platform Architecture](../../docs/ARCHITECTURE.md) — Overall system design
- [Usage Guide](USAGE.md) — How to use and extend the Assistant
- [Delegation Patterns](DELEGATION.md) — Deep dive on delegation architecture
- [README](../README.md) — Project overview and setup

---

**Copyright © 2026 baaaht project**
