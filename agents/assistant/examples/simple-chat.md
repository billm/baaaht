# Simple Chat Example

**Basic Conversational Flow — No Delegation Required**

This example demonstrates the Assistant Agent handling a straightforward conversation that doesn't require any tool delegation. The Assistant responds directly using its knowledge without invoking the `delegate` tool.

---

## Scenario

A user asks the Assistant a general knowledge question that can be answered directly from the LLM's training data.

---

## Conversation Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    Simple Chat Flow                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  User ──▶ Orchestrator ──▶ Assistant Agent                     │
│                                  │                              │
│                                  ▼                              │
│                         LLM Gateway                             │
│                                  │                              │
│                                  ▼                              │
│                         Direct Response                         │
│                                  │                              │
│                                  ▼                              │
│  User ◀── Orchestrator ◀── Assistant Agent                     │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Message Exchange

### Step 1: User sends message

**User Message:**
```json
{
  "id": "msg-001",
  "sessionId": "session-abc123",
  "content": "What is the difference between a process and a thread in operating systems?",
  "type": "MESSAGE_TYPE_USER",
  "timestamp": "2026-02-10T10:30:00Z"
}
```

### Step 2: Orchestrator forwards to Assistant

**AgentMessage Received:**
```typescript
// Assistant receives via gRPC StreamAgentClient
interface AgentMessage {
  id: string;           // "msg-001"
  sessionId: string;    // "session-abc123"
  content: string;      // "What is the difference between..."
  type: MessageType;    // MESSAGE_TYPE_USER
  metadata?: Record<string, unknown>;
}
```

### Step 3: Assistant retrieves session context

```typescript
// SessionManager retrieves existing session
const session = await sessionManager.get('session-abc123');

// Session contains previous messages (if any)
interface Session {
  id: string;
  state: SessionState;  // SESSION_STATE_ACTIVE
  messages: Message[];
  metadata: SessionMetadata;
}
```

### Step 4: Assistant builds LLM request

```typescript
// Build completion request with session context
const request: LLMRequest = {
  model: 'anthropic/claude-sonnet-4-20250514',
  messages: session.messages.map(m => ({
    role: m.role === MESSAGE_ROLE_USER ? 'user' : 'assistant',
    content: m.content,
  })),
  tools: [
    // Only tool is 'delegate'
    createDelegateToolDefinition(),
  ],
  stream: true,
  maxTokens: 4096,
};
```

### Step 5: LLM Gateway streams response

```typescript
// Response streams back with content blocks
for await (const chunk of llmGateway.stream(request)) {
  switch (chunk.type) {
    case 'content':
      // Text content - accumulate
      aggregator.addContent(chunk.content);
      break;
    case 'toolCall':
      // Tool use - NOT USED in this example
      break;
    case 'usage':
      // Token usage metadata
      break;
    case 'complete':
      // Response complete
      break;
  }
}
```

### Step 6: Assistant sends response to user

**Assistant Response:**
```json
{
  "id": "msg-002",
  "sessionId": "session-abc123",
  "content": "In operating systems, a **process** and a **thread** are both units of execution, but they have key differences:\n\n**Process:**\n- Independent execution unit with its own memory space\n- Has its own resources (file descriptors, memory, handles)\n- Processes are isolated from each other\n- Communication between processes requires IPC mechanisms\n- Higher overhead for creation and context switching\n\n**Thread:**\n- Lightweight execution unit within a process\n- Shares the process's memory and resources with other threads\n- Threads can directly access shared data\n- Communication between threads is simpler (shared memory)\n- Lower overhead for creation and context switching\n\n**Key analogy:** A process is like a house with its own address, utilities, and resources. Threads are like rooms within that house - they share the house's resources but are separate spaces of activity.\n\nThis is why multithreading is often more efficient than multiprocessing, but requires careful synchronization to avoid conflicts.",
  "type": "MESSAGE_TYPE_ASSISTANT",
  "toolCalls": [],
  "timestamp": "2026-02-10T10:30:03Z",
  "usage": {
    "inputTokens": 45,
    "outputTokens": 235,
    "totalTokens": 280
  }
}
```

---

## Session State After Conversation

```typescript
// Session now contains the conversation
{
  id: "session-abc123",
  state: SessionState.SESSION_STATE_ACTIVE,
  messages: [
    {
      id: "msg-001",
      role: MessageRole.MESSAGE_ROLE_USER,
      content: "What is the difference between a process and a thread...",
      timestamp: new Date("2026-02-10T10:30:00Z"),
    },
    {
      id: "msg-002",
      role: MessageRole.MESSAGE_ROLE_ASSISTANT,
      content: "In operating systems, a **process** and a **thread**...",
      timestamp: new Date("2026-02-10T10:30:03Z"),
    }
  ],
  metadata: {
    createdAt: new Date("2026-02-10T10:30:00Z"),
    lastActivityAt: new Date("2026-02-10T10:30:03Z"),
    messageCount: 2,
    totalTokens: 280,
  }
}
```

---

## Key Observations

| Aspect | What Happened |
|---|---|
| **Tool Usage** | None — `delegate` tool was registered but not invoked |
| **Delegation** | None — question answered directly from LLM knowledge |
| **Session Context** | Updated with both user and assistant messages |
| **Token Efficiency** | High — no delegation overhead or tool execution traces |
| **Response Time** | Fast — single LLM completion (~3 seconds) |

---

## When This Pattern Applies

Use direct response (no delegation) when:

1. **Answering general knowledge questions** — Information in training data
2. **Explaining concepts** — Definitions, comparisons, tutorials
3. **Providing guidance** — Best practices, design patterns, recommendations
4. **Analyzing provided context** — User has shared all necessary information
5. **Creative tasks** — Writing, brainstorming, ideation

---

## Code Reference

See [`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) for implementation details on:
- Message processing loop
- Session context management
- LLM client integration

See [`docs/USAGE.md`](../docs/USAGE.md) for:
- Running the Assistant Agent
- Configuration options
- Monitoring and debugging

---

**Copyright © 2026 baaaht project**
