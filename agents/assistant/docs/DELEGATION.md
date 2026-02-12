# Delegation Patterns

**Architecture and Implementation of Agent Delegation in baaaht**

**Version:** 1.0
**Last Updated:** 2026-02-09

---

## 1. Overview

The Assistant Agent uses a **delegation-only model** — its sole tool is `delegate`, which dispatches work to specialized agents with direct tool access. This design keeps the Assistant's context window clean and focused on conversation, improving efficiency and reducing token costs.

### 1.1 Why Delegation?

| Benefit | Description |
|---|---|
| **Context Purity** | Assistant's context contains only user messages, delegation requests/responses, and its own replies — never raw tool execution traces |
| **Token Efficiency** | Delegation results are summarized, not full tool execution logs |
| **Separation of Concerns** | Assistant focuses on conversation; specialized agents focus on execution |
| **Scalability** | Multiple specialized agents can be added without modifying the Assistant |
| **Security** | Assistant has no direct tool access — all actions go through the Orchestrator's policy engine |

### 1.2 Delegation Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Delegation Flow                                 │
└─────────────────────────────────────────────────────────────────────────┘

User Message → Assistant (LLM)
                     │
                     ▼
               LLM decides action needed
                     │
                     ▼
          LLM invokes delegate tool
                     │
     ┌───────────────┼───────────────┐
     │               │               │
     ▼               ▼               ▼
  Worker          Researcher        Coder
  Agent           Agent            Agent
  (File/Web)      (Deep Search)    (Code/Exec)
     │               │               │
     └───────────────┼───────────────┘
                     │
                     ▼
              Results returned
                     │
                     ▼
          LLM generates final response
                     │
                     ▼
               Response to user
```

---

## 2. Delegate Tool Specification

### 2.1 Tool Definition

```typescript
interface Tool {
  name: 'delegate';
  description: string;
  inputSchema: ToolInputSchema;
}

interface ToolInputSchema {
  type: 'object';
  properties: {
    target: {
      type: 'string';
      enum: ['worker', 'researcher', 'coder'];
      description: 'Target agent for delegation';
    };
    operation: {
      type: 'string';
      enum: [/* operations */];
      description: 'Operation to perform';
    };
    parameters: {
      type: 'object';
      description: 'Operation-specific parameters';
    };
    timeout: {
      type: 'number';
      description: 'Optional timeout in milliseconds';
    };
    priority: {
      type: 'string';
      enum: ['low', 'normal', 'high', 'critical'];
      description: 'Task priority';
    };
  };
  required: ['target', 'operation', 'parameters'];
}
```

### 2.2 Delegate Parameters

```typescript
interface DelegateParams {
  // Target agent
  target: DelegateTarget;  // 'worker' | 'researcher' | 'coder'

  // Operation to perform
  operation: DelegateOperation;

  // Operation-specific parameters
  parameters: Record<string, unknown>;

  // Optional settings
  timeout?: number;      // Max execution time (ms)
  priority?: TaskPriority;  // Execution priority
}

enum DelegateTarget {
  WORKER = 'worker',
  RESEARCHER = 'researcher',  // Phase 2
  CODER = 'coder',            // Phase 2
}

enum DelegateOperation {
  // File operations
  READ_FILE = 'read_file',
  WRITE_FILE = 'write_file',
  DELETE_FILE = 'delete_file',
  LIST_FILES = 'list_files',
  SEARCH_FILES = 'search_files',

  // Web operations
  WEB_SEARCH = 'web_search',
  WEB_FETCH = 'web_fetch',

  // Research operations (Phase 2)
  DEEP_RESEARCH = 'deep_research',
  SYNTHESIZE_SOURCES = 'synthesize_sources',

  // Code operations (Phase 2)
  ANALYZE_CODE = 'analyze_code',
  GENERATE_CODE = 'generate_code',
  REVIEW_CODE = 'review_code',
  EXECUTE_CODE = 'execute_code',
}
```

### 2.3 Delegate Result

```typescript
interface DelegateResult {
  // Success status
  success: boolean;

  // Task tracking
  taskId?: string;
  taskState?: TaskState;

  // Result data
  data?: unknown;
  output?: string;
  error?: string;

  // Metadata
  metadata: DelegateMetadata;
}

interface DelegateMetadata {
  target: DelegateTarget;
  operation: DelegateOperation;
  createdAt: Date;
  completedAt?: Date;
  duration?: number;  // milliseconds
  agentId?: string;   // Agent that handled the delegation
}
```

---

## 3. Worker Delegation

### 3.1 Supported Operations

| Operation | Description | Parameters |
|---|---|---|
| `read_file` | Read file contents | `path`, `offset?`, `limit?` |
| `write_file` | Write content to file | `path`, `content`, `createParents?` |
| `delete_file` | Delete file or directory | `path`, `recursive?` |
| `list_files` | List directory contents | `path`, `pattern?`, `maxDepth?` |
| `search_files` | Search files by pattern | `path`, `pattern`, `recursive?` |
| `web_search` | Search the web | `query`, `maxResults?` |
| `web_fetch` | Fetch a URL | `url`, `method?`, `headers?`, `body?` |

### 3.2 Parameter Examples

```typescript
// Read file
{
  target: 'worker',
  operation: 'read_file',
  parameters: {
    path: '/tmp/config.json',
    offset: 0,
    limit: 1024,
  }
}

// Write file
{
  target: 'worker',
  operation: 'write_file',
  parameters: {
    path: '/tmp/output.txt',
    content: 'Hello, World!',
    createParents: true,
  }
}

// Web search
{
  target: 'worker',
  operation: 'web_search',
  parameters: {
    query: 'TypeScript best practices 2026',
    maxResults: 5,
  }
}

// Web fetch
{
  target: 'worker',
  operation: 'web_fetch',
  parameters: {
    url: 'https://api.example.com/data',
    method: 'GET',
    headers: {
      'Authorization': 'Bearer token123',
    },
  }
}
```

### 3.3 Implementation Class

```typescript
class WorkerDelegation {
  constructor(grpcClient: Client, config?: WorkerDelegationConfig);

  async delegate(
    params: DelegateParams,
    sessionId: string
  ): Promise<DelegateResult>;

  updateConfig(config: Partial<WorkerDelegationConfig>): void;
  getConfig(): Required<WorkerDelegationConfig>;
}

interface WorkerDelegationConfig {
  defaultTimeout?: number;    // Default: 60000 (1 min)
  maxTimeout?: number;        // Default: 300000 (5 min)
  enableStreaming?: boolean;  // Default: true
  workerAgentId?: string;     // Specific agent (empty = any)
}
```

### 3.4 Execution Flow

```
Assistant calls delegate()
         │
         ▼
1. Validate parameters
   ├─▶ Check target == 'worker'
   ├─▶ Check operation supported
   └─▶ Validate operation parameters
         │
         ▼
2. Create TaskConfig
   ├─▶ Build command from operation
   ├─▶ Build arguments array
   ├─▶ Build environment variables
   └─▶ Set timeout
         │
         ▼
3. Execute task via gRPC
   ├─▶ Call Orchestrator.ExecuteTask()
   ├─▶ Pass TaskConfig and sessionId
   └─▶ Receive taskId in response
         │
         ▼
4. Poll for completion
   ├─▶ Call GetTaskStatus() every 500ms
   ├─▶ Check for terminal state
   └─▶ Return result on completion
         │
         ▼
5. Parse and return result
   ├─▶ Extract output from TaskResult
   ├─▶ Parse JSON if applicable
   └─▶ Format DelegateResult
```

---

## 4. Researcher Delegation (Phase 2)

### 4.1 Supported Operations

| Operation | Description | Parameters |
|---|---|---|
| `deep_research` | Conduct multi-step research | `query`, `maxDepth?`, `sources?` |
| `synthesize_sources` | Synthesize research findings | `sources`, `format?` |

### 4.2 Parameter Examples

```typescript
// Deep research
{
  target: 'researcher',
  operation: 'deep_research',
  parameters: {
    query: 'Impact of AI on software development in 2026',
    maxDepth: 3,
    sources: ['web', 'academic'],
  },
  timeout: 300000,  // 5 minutes for deep research
}

// Synthesize sources
{
  target: 'researcher',
  operation: 'synthesize_sources',
  parameters: {
    sources: [
      'https://example.com/article1',
      'https://example.com/article2',
    ],
    format: 'summary',
  }
}
```

### 4.3 Implementation Class

```typescript
class ResearcherDelegation {
  constructor(grpcClient: Client, config?: ResearcherDelegationConfig);

  async delegate(
    params: DelegateParams,
    sessionId: string
  ): Promise<DelegateResult>;
}

interface ResearcherDelegationConfig {
  defaultTimeout?: number;    // Default: 300000 (5 min)
  maxTimeout?: number;        // Default: 600000 (10 min)
  enableStreaming?: boolean;  // Default: true
  researcherAgentId?: string;
}
```

---

## 5. Coder Delegation (Phase 2)

### 5.1 Supported Operations

| Operation | Description | Parameters |
|---|---|---|
| `analyze_code` | Analyze codebase | `path`, `patterns?` |
| `generate_code` | Generate code | `spec`, `language?`, `framework?` |
| `review_code` | Review code changes | `diff`, `guidelines?` |
| `execute_code` | Execute code safely | `code`, `language?`, `timeout?` |

### 5.2 Parameter Examples

```typescript
// Analyze code
{
  target: 'coder',
  operation: 'analyze_code',
  parameters: {
    path: '/workspace/src',
    patterns: ['**/*.ts', '**/*.tsx'],
  }
}

// Generate code
{
  target: 'coder',
  operation: 'generate_code',
  parameters: {
    spec: 'Create a REST API endpoint for user management',
    language: 'typescript',
    framework: 'express',
  }
}

// Review code
{
  target: 'coder',
  operation: 'review_code',
  parameters: {
    diff: '@@ file.patch',
    guidelines: 'security-first, clean-code',
  }
}

// Execute code
{
  target: 'coder',
  operation: 'execute_code',
  parameters: {
    code: 'console.log("Hello, World!");',
    language: 'javascript',
    timeout: 5000,
  }
}
```

### 5.3 Implementation Class

```typescript
class CoderDelegation {
  constructor(grpcClient: Client, config?: CoderDelegationConfig);

  async delegate(
    params: DelegateParams,
    sessionId: string
  ): Promise<DelegateResult>;
}

interface CoderDelegationConfig {
  defaultTimeout?: number;    // Default: 120000 (2 min)
  maxTimeout?: number;        // Default: 300000 (5 min)
  enableStreaming?: boolean;  // Default: true
  coderAgentId?: string;
}
```

---

## 6. Validation

### 6.1 Parameter Validation

All delegations are validated before execution:

```typescript
function validateDelegateParams(
  params: DelegateParams,
  config?: DelegateToolConfig
): DelegateParams {
  // 1. Validate target
  if (!Object.values(DelegateTarget).includes(params.target)) {
    throw new Error(`Invalid target: ${params.target}`);
  }

  // 2. Validate operation
  if (!Object.values(DelegateOperation).includes(params.operation)) {
    throw new Error(`Invalid operation: ${params.operation}`);
  }

  // 3. Validate target-operation compatibility
  validateTargetOperationCompatibility(params.target, params.operation);

  // 4. Validate operation-specific parameters
  validateOperationParameters(params.operation, params.parameters);

  // 5. Apply defaults
  return {
    ...params,
    timeout: params.timeout ?? config.defaultTimeout,
    priority: params.priority ?? config.defaultPriority,
  };
}
```

### 6.2 Target-Operation Compatibility

```typescript
function validateTargetOperationCompatibility(
  target: DelegateTarget,
  operation: DelegateOperation
): void {
  const workerOps = [
    DelegateOperation.READ_FILE,
    DelegateOperation.WRITE_FILE,
    DelegateOperation.DELETE_FILE,
    DelegateOperation.LIST_FILES,
    DelegateOperation.SEARCH_FILES,
    DelegateOperation.WEB_SEARCH,
    DelegateOperation.WEB_FETCH,
  ];

  const researcherOps = [
    DelegateOperation.DEEP_RESEARCH,
    DelegateOperation.SYNTHESIZE_SOURCES,
  ];

  const coderOps = [
    DelegateOperation.ANALYZE_CODE,
    DelegateOperation.GENERATE_CODE,
    DelegateOperation.REVIEW_CODE,
    DelegateOperation.EXECUTE_CODE,
  ];

  switch (target) {
    case DelegateTarget.WORKER:
      if (!workerOps.includes(operation)) {
        throw new Error(`Operation ${operation} not supported by worker`);
      }
      break;
    // ... other cases
  }
}
```

---

## 7. Error Handling

### 7.1 Error Types

```typescript
enum DelegateErrorCode {
  INVALID_TARGET = 'INVALID_TARGET',
  INVALID_OPERATION = 'INVALID_OPERATION',
  INVALID_PARAMETERS = 'INVALID_PARAMETERS',
  TASK_TIMEOUT = 'TASK_TIMEOUT',
  TASK_FAILED = 'TASK_FAILED',
  AGENT_UNAVAILABLE = 'AGENT_UNAVAILABLE',
}

class DelegateError extends Error {
  code: DelegateErrorCode;
  details?: Record<string, unknown>;
}
```

### 7.2 Error Recovery

```typescript
try {
  const result = await workerDelegation.delegate(params, sessionId);
  if (!result.success) {
    // Handle delegation failure
    console.error('Delegation failed:', result.error);
  }
} catch (error) {
  if (error instanceof DelegateError) {
    switch (error.code) {
      case DelegateErrorCode.TASK_TIMEOUT:
        // Retry with longer timeout
        break;
      case DelegateErrorCode.AGENT_UNAVAILABLE:
        // Report unavailable service
        break;
      default:
        // Log and propagate
    }
  }
}
```

---

## 8. Timeout Handling

### 8.1 Timeout Hierarchy

```
┌─────────────────────────────────────────────────────────────┐
│                    Timeout Hierarchy                         │
└─────────────────────────────────────────────────────────────┘

Agent messageTimeout (default: 120s)
    │
    ├─▶ LLM completion timeout
    │
    └─▶ Delegation timeout (default: 60s, max: 300s)
            │
            └─▶ Individual tool execution timeout
```

### 8.2 Timeout Configuration

```typescript
interface DelegateToolConfig {
  defaultTimeout: number;  // Default: 60000 (60 seconds)
  maxTimeout: number;      // Maximum: 300000 (5 minutes)
}

const config: DelegateToolConfig = {
  defaultTimeout: 60000,
  maxTimeout: 300000,
};

// Override per-delegation
{
  target: 'worker',
  operation: 'web_search',
  parameters: { query: 'test' },
  timeout: 30000,  // 30 seconds for this operation only
}
```

---

## 9. Priority Handling

### 9.1 Priority Levels

```typescript
enum TaskPriority {
  TASK_PRIORITY_LOW = 0,
  TASK_PRIORITY_NORMAL = 2,
  TASK_PRIORITY_HIGH = 3,
  TASK_PRIORITY_CRITICAL = 4,
}
```

### 9.2 Priority Usage

```typescript
// Normal priority (default)
{
  target: 'worker',
  operation: 'read_file',
  parameters: { path: '/tmp/file.txt' },
  priority: 'normal',
}

// High priority for urgent tasks
{
  target: 'worker',
  operation: 'web_search',
  parameters: { query: 'security alert' },
  priority: 'high',
}

// Low priority for background tasks
{
  target: 'worker',
  operation: 'list_files',
  parameters: { path: '/tmp' },
  priority: 'low',
}
```

---

## 10. Streaming Results

### 10.1 Streaming Delegation

```typescript
interface StreamingDelegationConfig {
  enableStreaming: boolean;  // Default: true
}

// Enable streaming for real-time updates
const delegation = new WorkerDelegation(grpcClient, {
  enableStreaming: true,
});

// Results stream as they're available
const result = await delegation.delegate(params, sessionId);
// Output is incrementally built as chunks arrive
```

### 10.2 Chunk Types

```typescript
type StreamingChunk =
  | { type: 'progress'; data: { message: string } }
  | { type: 'data'; data: { chunk: unknown } }
  | { type: 'complete'; data: DelegateResult }
  | { type: 'error'; data: { error: string } };
```

---

## 11. Implementation Guide

### 11.1 Adding a New Delegation Type

To add support for a new specialized agent:

**Step 1: Define the target enum**
```typescript
// src/tools/types.ts
export enum DelegateTarget {
  WORKER = 'worker',
  RESEARCHER = 'researcher',
  CODER = 'coder',
  MY_AGENT = 'my_agent',  // New agent
}
```

**Step 2: Define operations**
```typescript
export enum DelegateOperation {
  // ... existing
  MY_OPERATION = 'my_operation',
}
```

**Step 3: Create delegation class**
```typescript
// src/tools/my-delegation.ts
export class MyDelegation {
  async delegate(
    params: DelegateParams,
    sessionId: string
  ): Promise<DelegateResult> {
    // Implementation
  }
}
```

**Step 4: Update compatibility validation**
```typescript
// src/tools/delegate.ts
function validateTargetOperationCompatibility(
  target: DelegateTarget,
  operation: DelegateOperation
): void {
  const myAgentOps = [
    DelegateOperation.MY_OPERATION,
  ];

  switch (target) {
    case DelegateTarget.MY_AGENT:
      if (!myAgentOps.includes(operation)) {
        throw new Error(`Operation ${operation} not supported by my_agent`);
      }
      break;
  }
}
```

**Step 5: Wire up in agent**
```typescript
// src/agent.ts
private async executeDelegation(
  params: DelegateParams
): Promise<ToolResult> {
  switch (params.target) {
    case DelegateTarget.MY_AGENT:
      const myDelegation = new MyDelegation(this.grpcClient);
      return await myDelegation.delegate(params, sessionId);
    // ... other cases
  }
}
```

---

## 12. Testing Delegations

### 12.1 Unit Testing

```typescript
describe('WorkerDelegation', () => {
  it('should delegate read_file operation', async () => {
    const mockGrpc = createMockGrpcClient();
    const delegation = new WorkerDelegation(mockGrpc);

    const result = await delegation.delegate({
      target: DelegateTarget.WORKER,
      operation: DelegateOperation.READ_FILE,
      parameters: { path: '/tmp/test.txt' },
    }, 'session-001');

    expect(result.success).toBe(true);
    expect(result.taskId).toBeTruthy();
  });

  it('should handle timeout errors', async () => {
    const mockGrpc = createTimeoutGrpcClient();
    const delegation = new WorkerDelegation(mockGrpc, {
      defaultTimeout: 1000,
    });

    const result = await delegation.delegate({
      target: DelegateTarget.WORKER,
      operation: DelegateOperation.READ_FILE,
      parameters: { path: '/tmp/slow.txt' },
    }, 'session-001');

    expect(result.success).toBe(false);
    expect(result.taskState).toBe(TaskState.TASK_STATE_TIMEOUT);
  });
});
```

### 12.2 Integration Testing

```typescript
describe('Delegation Integration', () => {
  it('should complete full delegation flow', async () => {
    // Create agent with real Orchestrator connection
    const agent = createTestAgent();

    // Send message that requires delegation
    const response = await agent.receiveMessage({
      id: 'test-msg',
      sessionId: 'test-session',
      content: 'Read the file /tmp/config.json',
      type: MessageType.MESSAGE_TYPE_USER,
    });

    // Verify delegation occurred
    expect(response.toolCalls).toHaveLength(1);
    expect(response.toolCalls[0].name).toBe('delegate');
    expect(response.toolCalls[0].success).toBe(true);
  });
});
```

---

## 13. Monitoring

### 13.1 Delegation Metrics

Track these metrics for delegation health:

| Metric | Description | Alert Threshold |
|---|---|---|
| `delegation.total` | Total delegations attempted | — |
| `delegation.success_rate` | Percentage of successful delegations | < 95% |
| `delegation.latency.p99` | 99th percentile delegation latency | > 30s |
| `delegation.timeout_rate` | Percentage of delegations timing out | > 5% |
| `delegation.errors.by_target` | Error rate by target agent | > 10% |
| `delegation.errors.by_operation` | Error rate by operation | > 10% |

### 13.2 Event Monitoring

```typescript
// Track delegation events
agent.on(AgentEventType.TOOL_CALL_START, (event) => {
  metrics.increment('delegation.started', {
    target: event.data.target,
    operation: event.data.operation,
  });
});

agent.on(AgentEventType.TOOL_CALL_COMPLETE, (event) => {
  const duration = event.data.durationMs;
  metrics.timing('delegation.duration', duration, {
    target: event.data.target,
    operation: event.data.operation,
  });
});

agent.on(AgentEventType.TOOL_CALL_FAILED, (event) => {
  metrics.increment('delegation.failed', {
    target: event.data.target,
    operation: event.data.operation,
    error: event.data.error,
  });
});
```

---

## 14. Best Practices

### 14.1 For Delegation Authors

1. **Validate all parameters** before execution
2. **Set appropriate timeouts** for each operation type
3. **Return structured errors** with actionable information
4. **Include operation metadata** for debugging
5. **Support streaming** for long-running operations

### 14.2 For Agent Users

1. **Specify timeouts** for operations that may take longer
2. **Use appropriate priorities** for time-sensitive tasks
3. **Handle delegation failures gracefully** in user responses
4. **Monitor delegation metrics** for performance issues
5. **Test delegation paths** in integration tests

### 14.3 For Operations Teams

1. **Monitor delegation latency** and error rates
2. **Set alerts** for timeout spikes
3. **Track agent availability** and spawn failures
4. **Log delegation metadata** for debugging
5. **Profile token usage** per delegation type

---

## 15. Future Enhancements

### 15.1 Planned Features

- **Parallel Delegation:** Execute multiple delegations concurrently
- **Delegation Chaining:** Results from one delegation inform another
- **Result Caching:** Cache delegation results for identical requests
- **Streaming Results:** Real-time result streaming to users
- **Delegation History:** Track and audit all delegations
- **Custom Agents:** Support user-defined specialized agents

### 15.2 Research Directions

- **Adaptive Timeouts:** Learn optimal timeouts per operation
- **Smart Routing:** Route to best agent based on request characteristics
- **Result Summarization:** Auto-summarize large delegation results
- **Delegation Composition:** Combine multiple delegations into one

---

## 16. References

- [Architecture Documentation](ARCHITECTURE.md) — Overall agent architecture
- [Usage Guide](USAGE.md) — How to use the Assistant
- [Platform Architecture](../../docs/ARCHITECTURE.md) — System-wide design

---

**Copyright © 2026 baaaht project**
