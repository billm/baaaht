# Architecture & Technical Design — baaaht

**A Secure, Container-Native, Multi-User Agentic AI Assistant Platform**

**Version:** 1.1  
**Last Updated:** 2026-02-05  
**Status:** Draft — Architecture Review Complete

> **Document Classification:** This is the **Technical Architecture Specification** for baaaht.
> It covers system design, component interfaces, data models, security architecture, and implementation details.
> For product-level requirements (what and why), see [PRD.md](PRD.md).
> For implementation task slices, see [IMPLEMENTATION.md](IMPLEMENTATION.md).

---

```
     .ooo.     ___
   .oOoooOo.  /O O\   Baaaht!
 \.OooOooOooO/  o  \
  .OooOooOooO\_____/
   `OooOooO'
    ||   ||
```

## 1. Executive Summary

**baaaht** is a container-native, security-first, modular GenAI assistant platform designed for self-hosted, single-instance deployment serving individuals or small teams. The platform uses Go for systems-level orchestration and TypeScript for the agent, LLM, and tool ecosystem. It operates in both **private 1:1 contexts** (e.g., Slack DM, personal web chat, local TUI) and **group contexts** (e.g., Slack channels, shared sessions).

The platform draws on proven patterns from three predecessor projects:

- **nanobot** — Chat platform abstraction layer, cron scheduling, progressive skill loading, provider abstraction
- **nanoclaw** — Container-first security isolation, per-group filesystem boundaries, file-based IPC, mount allowlists
- **pi-agent** — Rich TUI with session branching, multi-provider LLM API with 15+ providers, extension system, Agent Skills standard

baaaht combines these into a single cohesive architecture with a unifying design constraint: **every agent, tool, and gateway workload runs in an isolated container with minimal privileges, and security boundaries are enforced at the OS level, not the application level.** The Orchestration Core runs on the host as the single trusted process managing container lifecycles.

### Key Differentiators

| Differentiator | Description |
|---|---|
| **Security by Isolation** | OS-level containerization as the primary security boundary — not application-level checks |
| **Multi-Context Awareness** | First-class support for 1:1 and group contexts with strict data segregation |
| **Agentic Architecture** | Assistant delegates all tool-requiring work to specialized agents (worker, researcher, coder), keeping its context clean. Lightweight utility agents (memory, heartbeat) handle background tasks such as session memory extraction and proactive scheduled checks. |
| **Provider Agnostic** | Unified LLM abstraction supporting cloud APIs, OpenAI-compatible endpoints, and local inference |
| **Extensibility** | Agent Skills (SKILL.md) for capability expansion. MCP (Model Context Protocol) support planned for a future phase. |
| **Proactive & Scheduled** | Built-in heartbeat system for proactive monitoring, plus user-defined scheduled tasks via natural language |
| **Self-Hosted** | Single-instance deployment; user owns all data, credentials, and infrastructure |

---

## 2. Design Principles

### 2.1 Core Principles

| Principle | Description |
|---|---|
| **Security First** | Container isolation, privilege separation, strict filesystem boundaries, authenticated IPC. Security is structural, not policy-based. |
| **Total Segregation** | Every user, group, and session gets isolated filesystems and memory. A compromise in one context cannot leak data into another. |
| **Modular Architecture** | Agents, LLM communication, tools, and channels are independently deployable as containers. The Orchestration Core runs on the host as the single trusted process managing container lifecycles. |
| **Agentic Specialization** | Multiple focused agents, each with scoped responsibilities, independent model selection, and distinct tool access policies. |
| **Provider Agnosticism** | The LLM layer abstracts all model providers. Switching from Claude to GPT to a local Llama model is a configuration change, not a code change. |
| **Extensibility** | Skills and MCP servers allow feature expansion without modifying core code. |
| **Operational Simplicity** | Predictable structure, observable behavior, predictable scheduling. The system should be debuggable by reading files on disk. |
| **Data Sovereignty** | Single-instance, self-hosted deployment. User owns all data, credentials, and infrastructure. No external telemetry or data sharing. |
| **Human-Readable State** | All persistent state uses human-readable formats (JSONL sessions, markdown memory, YAML config, JSON IPC). The system is debuggable by reading files on disk. |

### 2.2 Non-Goals

- No WhatsApp integration (removed from scope)
- No monolithic runtime — all agent, tool, and gateway workloads are containerized; the Orchestrator is the sole host process
- No agent-to-host direct filesystem access
- No agent direct internet access by default (proxy-only egress)
- No multi-tenant SaaS — this is a self-hosted platform
- No voice/audio interaction
- No email channel
- No native mobile app (Web UI is mobile-responsive)
- No GPU pod management (deferred to future versions)

---

## 3. Target Users

### Primary Users

| User | Profile | Use Cases |
|---|---|---|
| **Privacy-Conscious Developers** | Technical users who want full control of data and infrastructure | Code assistance, research automation, task management, local codebase analysis with mount allowlist access controls |
| **Power Users** | Comfortable with CLI and configuration; need automation | Complex workflows, multi-platform coordination, scheduled tasks, proactive monitoring via heartbeats |
| **Small Teams** | Distributed teams sharing a single deployment via Slack, Web UI, or both | Team coordination, knowledge management, automated reporting |

### Administrator / Operator

| User | Profile | Use Cases |
|---|---|---|
| **Administrator** | The person who deploys and maintains the baaaht instance. May also be a primary user. | System configuration, provider management, user/group administration, mount allowlist management, monitoring, troubleshooting via TUI |

### Secondary Users

| User | Profile | Use Cases |
|---|---|---|
| **Home Users** | Non-technical household members or team members using a deployment managed by an administrator | Daily briefings, reminders, information retrieval |

---

## 4. High-Level Architecture

### 4.1 The Four-Layer Segregation Model

baaaht is organized into four distinct tiers with strict isolation boundaries:

1. **Orchestration Core ("The Shepherd")** — A Go-based host process responsible for container lifecycle, event routing, scheduling, and credential management. It **never** executes LLM-generated code.

2. **Identity Containers ("The Brains")** — Each active user or group context gets a persistent **Assistant Agent** container that lasts for the session's lifetime. Specialized agents (**Worker**, **Researcher**, **Coder**) are spawned as short-lived containers per delegation. Utility agents (**Memory**, **Heartbeat**) are Orchestrator-triggered and ephemeral. All agent containers have scoped filesystem mounts.

3. **Tool Containers ("The Hands")** — All tool operations (including file reads, searches, shell execution, and web fetching) run in short-lived sidecar containers with restricted mounts. They are stateless and ephemeral.

4. **Communication Layer ("The Gateways")** — Dedicated containers for Slack and Web UI. The TUI runs as a host-side Go binary connecting to the Orchestrator via gRPC over UDS, following the same protocol contract as containerized gateways.

> **LLM Gateway Placement:** The LLM Gateway is a long-running infrastructure container that doesn't fit neatly into the identity or tool layers. It sits between Layers 1 and 2 — managed by the Orchestrator, holding API credentials (like a Layer 1 concern), but running as a container (like Layer 2/3). It is listed separately in §11.1 and has unique security properties (egress-only network, no user data).

### 4.2 Architecture Diagram

```
                ┌──────────────────────────────────┐
                │       Control Channels           │
                │  ┌─────┐  ┌─────┐  ┌──────────┐  │
                │  │ TUI │  │ Web │  │  Slack   │  │
                │  │(Go) │  │(TS) │  │(TS/per-  │  │
                │  │     │  │     │  │workspace)│  │
                │  └──┬──┘  └──┬──┘  └────┬─────┘  │
                └─────┼────────┼──────────┼────────┘
                      │        │          │
                      ▼        ▼          ▼
         ┌──────────────────────────────────────────┐
         │          Orchestration Core (Go)         │
         │                                          │
         │  ┌──────────┐ ┌──────────┐ ┌──────────┐  │
         │  │ Event    │ │ Session  │ │Container │  │
         │  │ Router   │ │ Lifecycle│ │ Manager  │  │
         │  └──────────┘ └──────────┘ └──────────┘  │
         │  ┌──────────┐ ┌──────────┐ ┌──────────┐  │
         │  │ Job      │ │ Policy   │ │ IPC      │  │
         │  │Scheduler │ │ Engine   │ │ Broker   │  │
         │  └──────────┘ └──────────┘ └──────────┘  │
         └───────┬────────────┬────────────┬────────┘
                 │            │            │
        ┌────────┘       ┌────┘            └────┐
        ▼                ▼                      ▼
  ┌────────────┐   ┌────────────┐         ┌───────────┐
  │Agent Core  │   │LLM Gateway │         │   Tool    │
  │  (TS)      │   │   (TS)     │         │Containers │
  │            │   │            │         │   (TS)    │
  │┌──────────┐│   │┌──────────┐│         │           │
  ││Assistant*││   ││ Provider ││         │ Stateless │
  ││ Worker   ││   ││ Registry ││         │ Ephemeral │
  ││Researcher││   ││ Streaming││         │ Sandboxed │
  ││ Coder    ││   ││ Token    ││         │           │
  ││──────────││   ││ Tracking ││         └───────────┘
  ││Memory  † ││   │└──────────┘│
  ││Heartbeat†││   └────────────┘         ┌───────────┐
  │└──────────┘│                          │MCP Servers│
  └────────────┘                          │ (Phase 2) │
                                          └───────────┘
  * Assistant's only tool is `delegate` — dispatches to Worker/Researcher/Coder
  † Memory and Heartbeat are Orchestrator-triggered utility agents (not Assistant-delegated)
```

### 4.3 Language Split

| Component | Language | Rationale |
|---|---|---|
| **Orchestration Core** | Go | Systems-level concerns: container lifecycle, process management, scheduling, resource enforcement. Go's concurrency model (goroutines) and compiled binary distribution are ideal. |
| **TUI** | Go (Bubbletea + Lipgloss) | Native integration with the orchestrator. Bubbletea provides a mature, performant terminal UI framework. |
| **Agent Core** | TypeScript (Node.js 22+) | LLM SDKs, MCP servers, Agent Skills tooling, and the broader AI agent ecosystem are heavily TypeScript. Async patterns, JSON handling, and streaming are natural in TS. |
| **LLM Gateway** | TypeScript | Provider SDKs (Anthropic, OpenAI, etc.) are TypeScript-first. Streaming multiplexing, token tracking, and format translation benefit from TS async patterns. |
| **Tool Containers** | TypeScript | Tools are built from the same TypeScript codebase as agents. TypeScript enables shared type definitions for tool interfaces. |
| **Web UI** | TypeScript (React) | Standard web frontend stack. |
| **Slack Gateway** | TypeScript | Slack SDK is JavaScript/TypeScript native. |
| **MCP Servers** | Any (protocol standard) | MCP defines a language-agnostic protocol (stdio or HTTP/SSE). Server implementations may be in any language. |

**Why TypeScript for agents over Go:** The LLM ecosystem is TypeScript-heavy. Most model SDKs (Anthropic, OpenAI), MCP server implementations, Agent Skills tooling, and tool-calling frameworks have first-class TypeScript support. Agent logic involves heavy JSON manipulation, async streaming, and dynamic tool dispatch — patterns where TypeScript excels. Go is better suited for the systems-level orchestration where performance, concurrency, and binary distribution matter more than ecosystem integration.

---

## 5. Orchestration Core

The Orchestration Core is the **central nervous system** of baaaht. It is written in Go and runs on the host. It is the only component with direct access to the host filesystem, container runtime, and credentials.

### 5.1 Responsibilities

| Function | Description |
|---|---|
| **Event Ingestion** | Receives events from all control channels (Slack, Web, TUI) and routes them to the appropriate handler |
| **Session Lifecycle** | Creates, manages, and tears down per-user and per-group sessions. Tracks session state machines. |
| **Message Routing** | Forwards inbound user messages to the appropriate agent container. For user-initiated messages, the Orchestrator routes to the Assistant without inspecting content. The Orchestrator directly triggers utility agents (Memory, Heartbeat) for system-initiated work such as archival and scheduled tasks. |
| **Container Management** | Provisions, monitors, health-checks, and tears down agent, tool, and gateway containers. See §5.3 for the `ContainerRuntime` interface. |
| **IPC Brokering** | Mediates all inter-container communication. No container talks directly to another. |
| **Task Scheduling** | Manages scheduled background tasks including heartbeats, cron-style recurring tasks, interval-based tasks, and one-shot scheduled tasks. See §15 for the full Task Scheduling subsystem. |
| **Policy Enforcement** | Enforces resource quotas (CPU, memory, disk), mount restrictions, and network policies. See §5.5 for the consolidated policy engine. |
| **Credential Management** | Holds API keys and secrets on the host. Injects them into containers via the LLM Gateway proxy — never directly into agent containers. |
| **Web Proxy** | Makes outbound HTTP requests on behalf of tool containers (via `ProxyWebRequest` gRPC). Enforces URL filtering, rate limits, and content size limits. See §13.3 for the full web proxy architecture. |

### 5.2 Design Model

- **Event-driven for real-time traffic** — gRPC-based communication (agent messages, LLM streaming, channel events) is event-driven. Polling is used for file-based IPC and scheduler evaluation where event-driven mechanisms are impractical.
- **Declarative scheduling** — cron expressions, intervals, and one-shot timestamps
- **State machine per session** — each session has a well-defined lifecycle (created → active → idle → archived). Archival is triggered by inactivity timeout (default: 24h since last user message) or user request. A Memory Agent runs on archival to extract memorable content.
- **Pluggable container runtime** — abstraction over Docker and macOS Containers

### 5.3 Container Runtime Abstraction

| Platform | Runtime | Notes |
|---|---|---|
| **macOS** | Apple Containers (preferred) | Native macOS containerization. Lightweight, fast. Preferred when available. |
| **macOS** | Docker (fallback) | Via Docker Desktop or Colima. Used when Apple Containers are unavailable. |
| **Linux** | Docker | Primary and only option on Linux. |
| **Linux** | Podman | Optional alternative. |

The orchestrator provides a `ContainerRuntime` interface that abstracts container creation, lifecycle management, mount binding, and cleanup. Implementations exist for Docker (via Docker SDK) and Apple Containers.

### 5.4 Error Handling & Recovery

The Orchestrator is the single point of control and must handle failures gracefully:

**Container Failures:**

| Failure | Response |
|---|---|
| Container fails to start | Retry once. If second attempt fails, return error to requesting channel. Log with full error context. |
| Container times out | Kill container, return timeout error to agent/user. Preserve any partial results written to IPC. |
| Container exits non-zero | Capture exit code and stderr. Return error to requesting agent. Log for debugging. |
| LLM Gateway becomes unhealthy | Health check detects failure → restart container. Queue incoming LLM requests during restart (brief backpressure). |
| Gateway container crashes | Auto-restart. Channels reconnect automatically (Slack reconnects via WebSocket, Web clients reconnect). |

**Additional Failure Modes:**

| Failure | Response |
|---|---|
| Disk space exhaustion | Orchestrator monitors disk usage. At configurable threshold (default: 90%), emit warning log. At critical threshold (default: 95%), refuse new session creation and tool container spawns. Archive idle sessions aggressively. |
| Partial work from crashed agent | Tool results already written to file-based IPC are preserved. The Orchestrator returns a partial-failure error to the requesting agent/user, noting which operations completed. No automatic rollback of file mutations. |
| Container runtime unresponsive | If `ContainerRuntime` API calls time out (e.g., Docker daemon frozen), the Orchestrator enters degraded mode — queues new container requests and retries with backoff. Reports degraded status via health endpoint. |

**Orchestrator Recovery:**

If the Orchestrator process crashes and restarts:

1. **State recovery:** The Orchestrator persists its state to `data/state/` — active session metadata, scheduled tasks, and container mappings. On restart, it reads this state and rebuilds its in-memory model.
2. **Orphaned containers:** On startup, the Orchestrator queries the container runtime for any containers with the `baaaht` label. Running containers are either adopted (if they match known sessions) or terminated (if orphaned).
3. **File-based IPC durability:** Any IPC requests written to disk by containers during the outage are processed upon restart — this is an explicit advantage of retaining file-based IPC for async operations.
4. **gRPC reconnection:** Containers that were connected via gRPC will detect the socket closure and retry connection. Long-lived containers (gateways, assistant agents) implement reconnection with exponential backoff as a default behavior of the shared gRPC client library (`agent/src/ipc/` for TypeScript, `internal/ipc/` for Go).

**LLM Provider Failures:**
The LLM Gateway handles provider-level failures:
- **Rate limits:** Queue requests and retry with backoff per provider's retry-after headers
- **Provider outage:** Automatic failover to the next provider in the configured fallback chain (§7.3)
- **Timeout:** Configurable per-provider timeout. On timeout, return error to agent — agent can retry or report failure to the user

### 5.5 Policy Enforcement

The Policy Engine is a subsystem of the Orchestrator responsible for enforcing security and resource constraints. It is consulted before every container creation and IPC request.

**Policy Categories:**

| Category | Enforcement Point | Details |
|---|---|---|
| **Mount Policy** | Container creation | Validates requested mounts against the user's `mount-allowlist.json`. Blocks sensitive paths (`.ssh`, `.aws`, etc.). Prevents path traversal. See §13.6. |
| **Resource Quotas** | Container creation | Applies CPU, memory, and disk limits from system configuration. Per-container overrides possible for specific agent roles. |
| **Network Policy** | Container creation | Assigns network mode per container type: no network (agents, tools), egress-only (LLM Gateway), or configured ports (Web Gateway). |
| **IPC Authorization** | Every IPC request | Validates sender identity against the requested operation. Ensures agents can only access their own context's data. |
| **Execution Policy** | Tool execution | Applies command pattern blocking for `exec` tool. Enforces execution timeouts. |

**Policy Interface:**

```go
// PolicyEngine validates operations before execution
type PolicyEngine interface {
    // ValidateMounts checks requested mounts against allowlist and blocked patterns
    ValidateMounts(userId string, mounts []MountSpec) error

    // GetResourceLimits returns the resource limits for a container role
    GetResourceLimits(role ContainerRole) ResourceLimits

    // GetNetworkPolicy returns the network mode for a container role
    GetNetworkPolicy(role ContainerRole) NetworkPolicy

    // AuthorizeIPC checks if a sender can perform an IPC operation
    AuthorizeIPC(senderId string, operation IPCOperation, targetContext string) error

    // ValidateExecCommand checks a shell command against blocked patterns
    ValidateExecCommand(command string) error
}
```

**Policy Configuration:**

Resource quotas and network policies are configured in `config.yaml` (see §16.1). Mount policies are defined in a global `mount-allowlist.json` (see §13.6) — the same allowlist applies to all users. Per-user mount allowlists are a potential future enhancement. Blocked command patterns are defined in code (see §8.5) but can be extended via configuration.

### 5.6 Container Runtime Interface

The Orchestrator abstracts container operations behind a `ContainerRuntime` interface, enabling support for Docker, Apple Containers, and future runtimes.

```go
// ContainerRuntime abstracts container lifecycle operations
type ContainerRuntime interface {
    // Create creates a new container with the given specification
    // Returns the container ID
    Create(ctx context.Context, spec ContainerSpec) (string, error)

    // Start starts a created container
    Start(ctx context.Context, containerId string) error

    // Stop stops a running container with a timeout for graceful shutdown
    Stop(ctx context.Context, containerId string, timeout time.Duration) error

    // Remove removes a stopped container
    Remove(ctx context.Context, containerId string) error

    // Kill forcibly terminates a container
    Kill(ctx context.Context, containerId string) error

    // ListByLabel returns containers matching the given label selector
    ListByLabel(ctx context.Context, labels map[string]string) ([]ContainerInfo, error)

    // InspectHealth returns the health status of a container
    InspectHealth(ctx context.Context, containerId string) (HealthStatus, error)

    // Logs returns a reader for container logs
    Logs(ctx context.Context, containerId string, opts LogOptions) (io.ReadCloser, error)

    // Exec runs a command in a running container (for health probes)
    Exec(ctx context.Context, containerId string, cmd []string) (ExecResult, error)
}

// ContainerSpec defines how to create a container
type ContainerSpec struct {
    Image       string              // Container image to use
    Name        string              // Container name (must be unique)
    Labels      map[string]string   // Labels for identification (includes "baaaht" label)
    Mounts      []MountSpec         // Volume mounts
    Env         []string            // Environment variables
    Network     NetworkPolicy       // Network configuration
    Resources   ResourceLimits      // CPU/memory/disk limits
    User        string              // User to run as (default: "1000")
    ReadOnlyFS  bool                // Read-only root filesystem (default: true)
    Entrypoint  []string            // Container entrypoint
    Cmd         []string            // Command arguments
}

// HealthStatus represents container health
type HealthStatus struct {
    Status      string    // "healthy", "unhealthy", "starting", "none"
    FailingStreak int     // Consecutive failed health checks
    LastCheck   time.Time // Timestamp of last health check
    LastOutput  string    // Output from last health check
}
```

**Runtime Implementations:**

| Runtime | Package | Notes |
|---|---|---|
| Docker | `internal/container/docker/` | Uses Docker SDK for Go. Primary implementation. |
| Apple Containers | `internal/container/apple/` | Native macOS runtime. Uses `container` CLI tool. Preferred on macOS when available. |
| Podman | `internal/container/podman/` | Docker-compatible API. Optional on Linux. |

**Runtime Selection:**

The Orchestrator selects a runtime at startup based on configuration and availability:

1. If `container.runtime` is set to a specific runtime, use that (fail if unavailable)
2. If set to `"auto"` (default):
   - On macOS: Try Apple Containers first, fall back to Docker
   - On Linux: Use Docker (or Podman if configured)

---

## 6. Agent Core

The Agent Core is a TypeScript-based runtime that executes agent workloads inside containerized environments. Each agent runs in its own container with scoped filesystem mounts, tool access, and LLM model selection.

### 6.1 Agent Taxonomy

| Agent | Role | Model Guidance | Tool Access |
|---|---|---|---|
| **Assistant** | Primary entry point for all inbound messages. Conversational agent — answers user questions directly from its own knowledge, or delegates to specialized agents when external action is required. Has a single `delegate` meta-tool for dispatching work — it never executes traditional tools (read, write, exec, web, etc.) itself. Its context contains only user messages, delegation requests/responses, and its own replies — not raw tool execution traces. | Balanced model (e.g., Sonnet, GPT-4o) | `delegate` only — dispatches to Worker/Researcher/Coder via Orchestrator |
| **Worker** | General-purpose task execution agent. Handles delegated requests from the Assistant that don't require specialized agents — web searches, file reads, message sending, simple lookups. Receives a structured request describing the task and desired response format, executes it, and returns results. | Balanced model (e.g., Sonnet, GPT-4o) | `read_file`, `list_dir`, `grep`, `find`, `web_search`, `web_fetch`, `message`, `write_file` (session data only) |
| **Researcher** | Executes long-running background research tasks. Reports findings asynchronously. Delegated to by the Assistant for deep research requiring multiple searches, source synthesis, or extended analysis. | High-capability model (e.g., Opus, o1). A balanced model is acceptable for cost control. | `read_file`, `list_dir`, `grep`, `find`, `web_search`, `web_fetch`, `write_file` (session data only) |
| **Coder** | Code analysis, generation, review, and execution. Delegated to by the Assistant for code-related tasks. Has exclusive access to code execution tools. | Code-specialized model (e.g., Sonnet, Codex) | `read_file`, `write_file`, `edit_file`, `list_dir`, `grep`, `find`, `exec` |
| **Memory** | Lightweight utility agent. Runs on session archival to extract memorable content from conversation history into the user's long-term memory store. Also invoked via Assistant delegation for explicit "remember this" requests. Triggered by the Orchestrator on archival; triggered by the Assistant via `delegate` for user-initiated memory operations. | Fast, cheap model (e.g., Haiku, GPT-4o-mini) | `read_file` (session data), `write_file` (user/group memory) |
| **Heartbeat** | Lightweight utility agent. Executes scheduled heartbeat prompts (email triage, status checks, digests). Runs in its own container per invocation, asynchronously. Triggered by the task scheduler, not the Assistant. | Fast, cheap model (e.g., Haiku, GPT-4o-mini) | `read_file`, `web_search`, `web_fetch`, `message` — the `message` tool is a fallback for direct user notification; primary path is routing results through the Assistant for natural formatting |

**Assistant Delegation Model:**

The Assistant is intentionally kept free of traditional tools (file I/O, web access, code execution, etc.) to maintain a clean conversation context. It has a single **`delegate` meta-tool** that the LLM invokes to dispatch work to specialized agents.

When the user asks something that requires action (web search, file read, code generation, etc.), the Assistant:

1. Invokes the `delegate` tool with a structured request specifying the target agent, task description, relevant context, and desired response format
2. The Orchestrator receives the tool call, spawns the appropriate agent container (Worker, Researcher, or Coder), and forwards the request
3. The delegated agent executes the task in its own container with full tool access
4. The result is returned to the Orchestrator, which passes it back to the Assistant as the `delegate` tool result
5. The Assistant incorporates the result into its reply to the user

**Delegate Tool Specification:**

```typescript
interface DelegateToolParams {
  agent: 'worker' | 'researcher' | 'coder' | 'memory';  // Target agent
  task: string;                                // Natural language task description
  context?: string;                            // Relevant conversation context the agent needs
  responseFormat?: string;                     // How results should be formatted
  priority?: 'normal' | 'background';          // Background = async, don't wait
}
```

The `delegate` tool is the **only tool** the Assistant has. Its tool calls appear in the session context, but they contain only the high-level delegation request and response — never the internal tool execution traces from the delegated agent. This keeps the Assistant's context window focused and efficient.

**Context Passing:** The Assistant is responsible for including sufficient context in the `context` field of the delegation request. The delegated agent does not have access to the Assistant's conversation history — it only receives what the Assistant explicitly passes. This means the Assistant must extract and forward any relevant details the agent needs (e.g., file paths mentioned earlier, user preferences, prior decisions).

**Context Accumulation:** Over multi-turn conversations with repeated delegations, the Assistant's context grows with each delegation result. The Assistant should summarize prior delegation results rather than passing full results verbatim in subsequent `context` fields. If the accumulated context exceeds practical limits, the Assistant should prioritize the most recent and relevant delegation results. No hard limit is enforced on `DelegateToolParams.context` — the natural constraint is the delegated agent's context window.

### 6.2 Agent Personality Framework

Each agent has two identity documents:

- **`SOUL.md`** — Personality, tone, communication style, behavioral guidelines. Defines *how* the agent communicates.
- **`IDENTITY.md`** — Operational role, capabilities, constraints, tool usage patterns. Defines *what* the agent does.

This separation allows:
- Swapping personality without changing function (e.g., formal vs casual assistant)
- Reusing functional roles with different personalities
- Clear behavioral isolation between agents

### 6.3 Agent Execution Flow

**Assistant Agent Flow (delegate tool only):**
```
Inbound Message (from Orchestrator via IPC)
  │
  ▼
Context Builder
  ├── Agent identity (SOUL.md + IDENTITY.md)
  ├── User/Group context (USER.md / GROUP.md)
  ├── Conversation history (user messages + assistant replies + delegate calls/results)
  ├── Long-term memory (memory/)
  └── Available skills metadata
  │
  ▼
LLM Invocation (via LLM Gateway) — with `delegate` tool definition
  │
  ▼
Response Processing
  ├── If `delegate` tool call → send to Orchestrator
  │     → Orchestrator spawns Worker/Researcher/Coder agent
  │     → Delegated agent executes with full tools → returns result
  │     → Result returned as delegate tool result → re-invoke LLM
  └── If final response → return via IPC to Orchestrator
  │
  ▼
Session Persistence (append to session.jsonl — user msgs + delegate calls/results + assistant replies)
```

**Tool-Using Agent Flow (Worker, Researcher, Coder):**
```
Delegation Request (from Orchestrator, originated by Assistant)
  │
  ▼
Context Builder
  ├── Agent identity (SOUL.md + IDENTITY.md)
  ├── Delegation request (task description + desired response format)
  └── Tool definitions
  │
  ▼
LLM Invocation (via LLM Gateway)
  │
  ▼
Response Processing
  ├── If tool calls → execute via Tool Container → append results → loop
  │     ├── Agent calls gRPC `RequestToolExecution` on Orchestrator
  │     ├── Orchestrator spawns ephemeral tool container
  │     ├── Tool container executes, writes result to file-based IPC (or returns via gRPC for sync tools)
  │     ├── Orchestrator collects result on container exit
  │     └── Result returned to agent via the `RequestToolExecution` gRPC response
  └── If final response → return formatted result to Orchestrator
  │
  ▼
Result returned to requesting agent
```

### 6.4 Context Management

Borrowing from pi-agent's context compaction strategy:

- **Automatic compaction** *(Phase 3)* — When conversation context approaches model token limits, older messages are summarized while preserving critical context.
- **Session branching** *(Phase 3)* — Fork from any point in conversation history to explore alternative approaches. The data model supports tree-structured JSONL from Phase 1 (§14.2), but the UI navigation and branching commands are Phase 3.
- **Memory tiers:**
  - **Session memory** — Ephemeral, per-conversation context *(Phase 1)*
  - **User/Group memory** — Persistent knowledge base per identity *(Phase 2, with Memory Agent)*
  - **Agent memory** — Learned patterns and preferences per agent role *(deferred — no storage location defined yet, see §14.1. Would require an `agents/{role}/memory/` directory.)*

### 6.5 Performance Considerations

The delegation model introduces latency that must be understood and managed:

**Delegation Latency:**
Every user request requiring tool use involves at minimum two LLM invocations: one for the Assistant to decide on delegation, and one for the delegated agent to execute with tools. Expected latency for a delegated request:

| Step | Expected Latency | Notes |
|---|---|---|
| Assistant LLM call (decide to delegate) | 1-5s | Depends on context size and model |
| Orchestrator routing + container spawn | 1-5s | Container startup is the bottleneck |
| Delegated agent LLM call (with tools) | 2-30s | May involve multiple tool-call loops |
| Tool container overhead (within delegation) | 0.5-10s | Each tool invocation spawns an ephemeral container. A Coder turn with 10 tools adds 5-10s of pure spawn time. |
| Result return to Assistant + final LLM call | 1-5s | Assistant formulates final reply |
| **Total** | **5-55s** | For a single delegation; Coder tasks may exceed 60s before Phase 3 pooling |

**Mitigations:**
- **Container reuse:** Worker/Coder containers for ongoing sessions could be kept warm rather than spawned per-delegation (future optimization, Phase 3 container pooling). An interim mitigation (pre-Phase 3): keep recently-used delegated agent containers alive with a short idle timeout (e.g., 60 seconds) before teardown, avoiding respawn for rapid successive delegations.
- **Direct answers:** The Assistant should answer simple questions from its own knowledge without delegation. The `delegate` tool should only be invoked when external action is genuinely needed.
- **Background delegation:** For research tasks, the `priority: 'background'` flag allows the Assistant to respond immediately ("I'll look into that") while the Researcher works asynchronously.

**Container Startup Impact:**
Each tool invocation within a delegated agent spawns an ephemeral tool container. A single Coder agent turn might invoke 5-15 tools (read, grep, edit, etc.), each requiring a container spawn. Even with sub-second container starts, this compounds significantly. Phase 3 container pooling (pre-warmed containers) is designed to address this.

**Apple Containers Risk:**
Apple Containers (macOS) is a new technology announced at WWDC 2025. While it is the preferred runtime on macOS for its lightweight, fast startup characteristics, teams should be aware:
- API surface may evolve during early releases
- Feature parity with Docker is not guaranteed (e.g., seccomp profiles, network policies)
- The `ContainerRuntime` abstraction layer ensures Docker remains a fully functional fallback
- Development should initially target Docker for stability, with Apple Containers as an optimization layer

---

## 7. LLM Communication Layer

A **provider-agnostic gateway** that abstracts all LLM interactions. Runs as a dedicated TypeScript container. Agent containers **never** hold API keys — all LLM calls are proxied through this gateway.

### 7.1 Supported Providers

| Category | Providers |
|---|---|
| **Cloud APIs** | Anthropic (Claude), OpenAI (GPT), Google (Gemini), Mistral, Groq, DeepSeek, xAI |
| **Aggregators** | OpenRouter (100+ models), Amazon Bedrock, Azure OpenAI, Google Vertex AI |
| **Local Inference** | Ollama, LM Studio, vLLM, any OpenAI-compatible endpoint |

### 7.2 Model Selection Format

```
{provider}/{model_name}

Examples:
  anthropic/claude-sonnet-4-20250514
  openai/gpt-4o
  openrouter/meta-llama/llama-3.1-70b-instruct
  ollama/llama3.1
  lmstudio/local-model
```

### 7.3 Gateway Responsibilities

| Function | Description |
|---|---|
| **Credential Injection** | API keys held by the Orchestrator are injected into the LLM Gateway container only. Agent containers send unauthenticated LLM requests to the Orchestrator via gRPC (`StreamLLM`), which routes them to the LLM Gateway. The gateway adds credentials before forwarding to the provider. No container communicates directly with the LLM Gateway — all requests are brokered by the Orchestrator. |
| **Provider Abstraction** | Unified request/response format regardless of provider, defined by the `LLMRequest` and `LLMEvent` protobuf messages (§12.3). Handles format translation for tool calls, streaming, and thinking blocks. |
| **Token Accounting** | Tracks token usage per request. Reports to Orchestrator for cost monitoring. |
| **Rate Limiting** | Per-provider rate limit enforcement. Queues requests when limits are approached. |
| **Failover Routing** | The LLM Gateway owns provider-level failover: automatic failover to backup providers when primary is unavailable, using configured fallback chains (§16.2). The Orchestrator owns gateway-level failover (restarting the LLM Gateway container if it becomes unhealthy, §18.3). The gateway reports the actual provider used in response metadata for accurate token accounting. |
| **Cost Optimization** | When a model is available through multiple providers (e.g., Claude via both Anthropic and Amazon Bedrock), route to the cheapest option. Only applies when provider-agnostic model aliases or multi-provider availability is configured — does not apply when a model is pinned to a single provider via `{provider}/{model}` format. |
| **Streaming Multiplexing** | Streams responses back to agent containers in real-time. Supports multiple concurrent streams. |
| **Prompt Caching** | Leverages provider-specific caching (Anthropic prompt caching, OpenAI context caching) to reduce cost and latency. Primary benefit is for the Assistant agent (long-lived context with stable system prompt prefix). Delegated agents have short contexts but benefit from cached system prompt + tool definitions. |

### 7.5 Error Handling

LLM provider errors are handled at two levels:

- **LLM Gateway (provider-level):** Rate limits (queue + retry with backoff), provider timeouts (configurable per-provider), format errors (retry once). See §5.4 for full error handling policy.
- **Orchestrator (gateway-level):** LLM Gateway container health monitoring and restart. See §18.3.

### 7.4 Cross-Provider Compatibility

Following pi-agent's proven approach:
- Thinking blocks converted to `<thinking>` tags across providers
- Tool call formats translated between Anthropic, OpenAI, and other schemas
- Context preserved when switching models mid-session
- Automatic format negotiation per provider capability

**Feature Compatibility:**

| Feature | Handling |
|---|---|
| Vision/image input | Supported where provider allows. Falls back to text description if unsupported. |
| Tool calling | Required for agents. Providers without tool-calling support are not usable for agents (usable for simple completions only). |
| Thinking blocks | Translated to `<thinking>` tags. Omitted if provider doesn't support them. |
| Token counting | Provider-reported when available. Estimated via tiktoken fallback for providers that don't report usage. |
| System prompt format | Translated per provider (system message vs. preamble vs. instruction prefix). |
| Max output tokens | Clamped to provider's maximum. Configurable per-agent. |

---

## 8. Tool System

### 8.1 Design Philosophy

Tools are **fully isolated, containerized primitives**. Every tool invocation runs in a short-lived sidecar container with restricted mounts. Tools are stateless — they receive input, produce output, and the container is destroyed.

> **Note:** Phase 3 container pooling (§19) modifies this model slightly — pooled containers are reused across invocations. When reusing a container, the workspace mount is reset between invocations to prevent state leakage.

> **Exception:** The Memory Agent writes directly to user and group memory stores via privileged agent container mounts (§11.3), bypassing the tool container model. This is intentional — memory extraction is an agent-level operation with scoped filesystem access, not a tool invocation.

### 8.2 Built-in Tools

| Tool | Description | Safety Controls |
|---|---|---|
| **read_file** | Read file contents with optional offset/limit | Path validation, mount enforcement |
| **write_file** | Write/create files with atomic writes | Directory auto-creation, mount enforcement |
| **edit_file** | Precise string replacement with exact match validation | Exact matching required, atomic modification |
| **list_dir** | Directory listing with metadata | Mount enforcement |
| **grep** | Search file contents using regex patterns | Mount enforcement |
| **find** | Find files by glob patterns | Mount enforcement |
| **exec** | Execute shell commands | Pattern blocking, timeout, workspace restriction, no network by default |
| **web_search** | Search via configurable search API | API-proxied, no direct network from tool container |
| **web_fetch** | Fetch and parse web content | HTML sanitization, max 5 redirects, 1MB content size limit, SSRF prevention via URL filtering (§13.3) |
| **message** | Send messages to users/groups via Orchestrator IPC | Authorization checks, rate limiting |

### 8.3 Tool Container Constraints

Every tool container:

- **Mounts only** `sessions/{session-id}/data/` as its read-write workspace **by default**
- **Coder tool containers** additionally mount paths from the user's mount allowlist (`mount-allowlist.json`). Project directories are mounted read-write; other allowed paths follow their configured permissions. This gives the Coder access to the user's actual project files while maintaining the allowlist as the security boundary.
- **Cannot access** the host filesystem, user data, group data, or agent identity files
- **Runs as non-root** (UID 1000)
- **Has no network access** by default (web tools use an explicit proxy)
- **Has a timeout** (default: 60 seconds, configurable)
- **Is destroyed after execution** (ephemeral)

### 8.4 Tool Interface Contract

```typescript
interface Tool {
  name: string;
  description: string;
  parameters: JSONSchema;  // TypeBox schema for parameter validation

  execute(params: Record<string, unknown>): Promise<ToolResult>;
}

interface ToolConfig {
  timeout?: number;               // Execution timeout in seconds (default: 60, per §8.3)
  networkMode?: 'none' | 'proxy'; // 'none' = no network, 'proxy' = web proxy via Orchestrator
  requiredMounts?: string[];      // Additional mount paths beyond default /workspace/
  allowedAgents?: string[];       // Agent roles permitted to use this tool (e.g., ['coder'])
}

interface ToolResult {
  content: string;
  isError?: boolean;
  durationMs?: number;            // Execution duration for metrics (§18.2)
  metadata?: Record<string, unknown>;
}
```

### 8.5 Command Safety

Shell command execution (`exec` tool) includes pattern-based blocking from nanobot's proven safety system:

**Blocked Patterns:**
- Destructive: `rm -rf /`, `del /f`, `rmdir /s`, `format`, `mkfs`
- System: `shutdown`, `reboot`, fork bombs
- Exfiltration: `curl | bash`, piping to remote
- Direct disk: `> /dev/sd*`
- Container escape: `docker`, `podman`, `container` CLI commands
- Privilege escalation: `sudo`, `su`, `doas`, `chmod +s`, `chown`
- Network tools: `curl`, `wget`, `nc`, `netcat`, `ssh`, `scp` (defense-in-depth — no network access, but blocked to prevent confusion)
- Environment inspection: `printenv`, `env` (prevents credential discovery attempts)

Blocked patterns are defined in the agent codebase (`agent/src/tools/safety/`) and can be extended via `config.yaml` (see §16.1 `tools.blocked_patterns`).

**Execution Controls:**
- Configurable timeout (default: 60s)
- Workspace path restriction
- No network access from exec containers

**Path Validation (all file-accepting tools):**

All tools that accept file paths (`read_file`, `write_file`, `edit_file`, `grep`, `find`, `list_dir`, `exec`) enforce the same path validation:
- **Path traversal prevention** — `../` sequences are rejected before processing
- **Symlink resolution** — Symlinks are resolved to their real path and validated against allowed mount boundaries
- **Mount boundary enforcement** — Resolved paths must fall within the tool container's mounted directories
- **Null byte rejection** — Paths containing null bytes are rejected to prevent C-string truncation attacks

---

## 9. Skill & Extension System

baaaht supports two complementary extensibility standards: **Agent Skills** (SKILL.md) for procedural knowledge and prompt-based capabilities, and **MCP** (Model Context Protocol) for tool-providing services.

### 9.1 Agent Skills (SKILL.md)

Following the [Agent Skills specification](https://agentskills.io/specification):

```
{skill-name}/
├── SKILL.md           # Required: frontmatter + instructions
├── scripts/           # Optional: executable code
├── references/        # Optional: additional documentation
└── assets/            # Optional: templates, resources
```

**SKILL.md Format:**
```yaml
---
name: code-review
description: Reviews code for bugs, style issues, and security vulnerabilities.
  Use when reviewing pull requests or code changes.
license: MIT
metadata:
  author: baaaht-contrib
  version: "1.0"
---

# Code Review

## When to use this skill
Use this skill when the user asks for a code review...

## Steps
1. Read the changed files...
```

**Progressive Disclosure:**
1. **Discovery** (~100 tokens) — Only `name` and `description` loaded at startup for all skills
2. **Activation** (<5000 tokens) — Full `SKILL.md` body loaded when the agent determines the skill is relevant
3. **Execution** (as needed) — Referenced files (`scripts/`, `references/`, `assets/`) loaded on demand

**Skill Categories:**
- **Always-Loaded** — Full SKILL.md body injected into agent context on every invocation, bypassing the evaluate step (e.g., GitHub integration)
- **On-Demand** — Loaded when the agent determines relevance (e.g., code review)
- **Progressive** — Graceful fallback when requirements aren't met (e.g., missing CLI dependency)

### 9.1.1 Skill Ownership & Location

Skills belong to **users and groups**, not to agent containers. This ensures:
- Different users can have different skills installed
- A skill installed for User A is not automatically available to User B
- Group-level skills are shared among group members but isolated from other groups

**Skill storage hierarchy:**

| Location | Scope | Visibility |
|---|---|---|
| `skills/` (system) | Global bundled skills | Available to all users (read-only) |
| `users/{user-id}/skills/` | Per-user installed skills | Only this user's sessions |
| `groups/{group-id}/skills/` | Per-group installed skills | Only this group's sessions |

**How agents access skills:**
Agent containers do not mount skill directories directly. Instead:
1. The Orchestrator reads the skill metadata (name + description) from all applicable skill directories for the current context (system + user + group)
2. Skill metadata is injected into the agent container as a read-only manifest at `/agent/skills/manifest.json`
3. When the agent activates a skill, it requests the full `SKILL.md` content from the Orchestrator via gRPC (`GetSkillContent`)
4. The Orchestrator reads the file from the appropriate location and returns it
5. If a skill has scripts that need execution, they run in a tool container with the skill's `scripts/` directory mounted read-only
6. References and assets (`references/`, `assets/`) are accessed via the same `GetSkillContent` gRPC call with a path parameter (e.g., `GetSkillContent({skill: "github", path: "references/api-guide.md"})`)

> **`GetSkillManifest` RPC:** This is called by agents on startup to receive the initial skill manifest. It can also be called after mid-session skill installation to refresh the manifest without restarting the agent container.

This keeps the agent container's mount surface minimal while giving it access to the skill content it needs.

**Which agents receive skills:**
Skill manifests are injected into the **Assistant**, **Worker**, **Researcher**, and **Coder** agent containers. Utility agents (**Memory**, **Heartbeat**) do not receive skill manifests — their tasks are narrowly scoped and skill-driven behavior is not appropriate.

### 9.1.2 Skill Dependencies & Addons

Some skills require external tools or packages (e.g., the GitHub skill needs the `gh` CLI). baaaht handles this through a **skill addon** system declared in the skill's `SKILL.md` frontmatter:

```yaml
---
name: github
description: GitHub repository operations (issues, PRs, releases, etc.)
metadata:
  author: baaaht-core
  version: "1.0"
addons:
  packages:
    - name: gh
      source: apt         # apt | apk | npm | pip (container-friendly only; no brew)
      verify: "gh --version"
  env:
    - GH_TOKEN            # Required environment variable
---
```

**How addon dependencies are resolved:**

1. **At skill install time**, the Orchestrator reads the `addons` section and builds a **skill layer** — a container image layer on top of the common base image that includes the required packages.
2. Skill layers are cached so they only rebuild when the skill's addon definition changes.
3. When a tool container is spawned for a skill that has addons, the Orchestrator uses the skill-specific image (base + addon layer) instead of the bare base image.
4. Environment variables listed in `addons.env` are injected by the Orchestrator if they exist in the system credential store — they are **never** exposed to the agent container itself.

**Container image layering:**
```
┌─────────────────────┐
│  Skill addon layer  │  ← gh CLI, jq, etc. (built at install time)
├─────────────────────┤
│  Role layer         │  ← Agent/tool role-specific deps
├─────────────────────┤
│  Common base image  │  ← Node.js 22, core runtime
└─────────────────────┘
```

This approach avoids bloating the base image with every possible dependency, keeps skill dependencies isolated, and ensures reproducible builds.

### 9.2 MCP (Model Context Protocol) *(Deferred — Phase 2+)*

MCP servers provide tool definitions and execution capabilities as external services. Full MCP integration is deferred to a future phase, pending a dedicated design that addresses:

- **Server lifecycle management** — Configuration, registration, start/stop/restart, health checking
- **Transport bridging** — MCP uses stdio or HTTP/SSE, which are outside the gRPC/file-based IPC model (§12). The Orchestrator must bridge MCP transports to maintain the "all IPC is brokered" invariant.
- **Tool routing** — How agents distinguish MCP tools from built-in tools, and how the Orchestrator routes accordingly
- **Mount and network policies** — Container isolation model for MCP server containers
- **Tool discovery** — How MCP tool definitions are surfaced to agents alongside built-in and skill tools

**Planned integration model:**
- MCP servers run as sidecar containers managed by the Orchestrator
- Agent containers discover available MCP tools from the Orchestrator (same as built-in tools)
- Tool calls to MCP tools are routed through the Orchestrator to the appropriate MCP server container
- MCP server containers have their own mount restrictions and network policies

### 9.3 Skill Lifecycle

```
discover → evaluate → activate → execute → deactivate

- discover: Skill metadata loaded at startup
- evaluate: Agent decides if skill is relevant to current task
- activate: Full SKILL.md loaded into agent context
- execute: Agent follows instructions, runs scripts, reads references
- deactivate: Skill removed from context when no longer needed. Deactivation occurs when the agent determines the skill is no longer relevant to the current task, or automatically when the session ends.
```

### 9.4 Bundled Skills

| Skill | Description | Type | Addons |
|---|---|---|---|
| **github** | Repository operations (issues, PRs, releases) | Always-loaded | `gh` CLI (apt/brew) |
| **web-research** | Deep web research with source synthesis | On-demand | None (uses built-in web tools) |
| **code-review** | Code review with style, bug, and security analysis | On-demand | None (uses built-in read tools) |
| **summarize** | Summarize URLs, files, and documents | On-demand | None (uses built-in web/read tools) |
| **skill-creator** | Generate new skills from natural language descriptions | On-demand | None (uses built-in write tools) |

### 9.5 Skill Installation

Users install third-party skills by asking the assistant to install a skill from a provided GitHub repository URL. The installation flow spans two components — the Orchestrator handles privileged operations (network access, filesystem writes outside the session), and a tool container handles validation in a sandbox.

**Installation Flow:**

1. The Assistant delegates the install request to a Worker agent
2. The Worker calls `RequestToolExecution` with the install-skill tool and the GitHub URL
3. The **Orchestrator** clones the repository (or fetches a specific subdirectory) into a temporary directory on the host — the Orchestrator is the only component with network access for this operation
4. The Orchestrator spawns a **tool container** with the cloned directory mounted read-only at `/workspace/skill/`
5. The tool container validates that a `SKILL.md` file exists with valid frontmatter, checks structural requirements, and reports validation results via file-based IPC
6. On successful validation, the **Orchestrator** copies the skill directory to the user's skill location (`users/{user-id}/skills/{skill-name}/`)
7. If the skill declares `addons`, the Orchestrator builds the skill addon layer (see §9.1.2)
8. The Orchestrator refreshes the skill manifest for the user's sessions

**Example interaction:**
```
User: Install the skill from https://github.com/example/research-skill
Assistant: I'll install that skill for you.
  → clones repo → validates SKILL.md → copies to your skills directory → rebuilds addon layer
  "The research-skill skill has been installed and is now available in your sessions."
```

**Security considerations:**
- ⚠️ **This is a convenience pattern with weak security guarantees.** Skills from arbitrary GitHub repos may contain malicious scripts or instructions.
- Skills with `scripts/` directories execute in sandboxed tool containers (mitigating runtime risk), but malicious `SKILL.md` instructions could manipulate agent behavior via prompt injection.
- Future improvements may include: skill signing, a curated registry, content scanning, or a review-before-install workflow.
- Users are responsible for vetting skills they install. The system logs all skill installations for audit.

**Group skill installation:**
- Only admin users can install skills at the group level (`groups/{group-id}/skills/`)
- Standard users can only install skills in their own user directory

---

## 10. Control Channels

### 10.1 TUI (Go + Bubbletea)

The TUI provides a rich terminal interface for direct interaction with baaaht. Built in Go with Bubbletea/Lipgloss, it runs as a **separate process** that connects to the Orchestration Core via gRPC over UDS, following the same gateway pattern as other control channels.

**Deployment Model:**
- The TUI is a standalone Go binary (e.g., `baaaht-tui`) that connects to a running Orchestrator
- No authentication — TUI access implies host-level trust (same machine)
- Requires a CLI flag to specify the user context to attach to (e.g., `--user billm`)
- Intended primarily for the administrator / primary user of the deployment

**Features:**
- Interactive multi-line editor with autocomplete
- Streaming response display with thinking block visibility
- Real-time tool execution visualization
- Session branching tree navigation (`/tree`)
- Multi-session view (toggle between user/group sessions via `/sessions` command)
- Group session access via `--group {group-id}` flag or `/group {group-id}` command within TUI
- Container health monitoring (CPU/memory for active containers)
- Model switching (`/model`)
- Keyboard-driven workflow
- Theme support (dark/light, customizable)

**TUI Commands:**
| Command | Purpose |
|---|---|
| `/new` | Start fresh session |
| `/resume` | Browse and continue previous sessions |
| `/tree` | Navigate session branch tree |
| `/fork` | Fork current conversation |
| `/model` | Switch LLM model |
| `/compact` | Manually trigger context compaction |
| `/export` | Export session to HTML |
| `/status` | Show system/container status |
| `/skill:{name}` | Activate a specific skill |
| `/archive` | Archive the current session |
| `/quit` | Exit |

### 10.2 Web UI (React + TypeScript)

A containerized web application providing browser-based access.

**Features:**
- Real-time chat interface with streaming responses
- Session management (new, resume, branch)
- Group session access via room selection — admin users create rooms (groups) through the Web UI; standard users join existing rooms and interact with the group's shared Assistant context
- File upload and multi-modal input
- Artifact rendering (code, HTML, diagrams)
- Mobile-responsive design

**Multi-Modal Input Flow:**

When a user uploads a file or image via the Web UI:
1. The Web Gateway receives the file and writes it to the session's data directory (`sessions/{session-id}/data/uploads/`)
2. The inbound message includes a reference to the uploaded file path
3. For images: if the current LLM provider supports vision (e.g., Claude, GPT-4o), the image is included in the LLM request as a base64-encoded image content block. If the provider doesn't support vision, the image is described as an attached file reference.
4. For other files (documents, code): the file contents are read by a Worker agent and included as text context
5. Files persist in the session data directory for the session's lifetime

**Architecture:**
- Runs as a dedicated container
- Communicates with the Orchestrator via gRPC (internal) and exposes WebSocket to browsers (protocol bridge)
- Serves static assets and handles WebSocket upgrade for browser clients
- No direct access to agent containers or data

**Authentication:**
- OIDC (OpenID Connect) authentication — supports any OIDC-compliant provider (Google, GitHub, Okta, Keycloak, etc.)
- Auto-user creation on first successful authentication — no manual user registration required
- User identity mapped to `users/{oidc-subject-id}/`
- Session tokens issued by the Web Gateway after OIDC flow completes

### 10.3 Slack Gateway (TypeScript)

A containerized Slack integration with **per-workspace container isolation**.

**Features:**
- Responds to `@mention` triggers in channels
- DM support for 1:1 conversations
- Thread-based responses for group contexts
- Per-user and per-channel context mapping

**Identity Mapping:**
| Slack Context | baaaht Context |
|---|---|
| Direct Message | `users/{slack-user-id}` — 1:1 private session |
| Channel Message | `groups/{slack-channel-id}` — Group shared session |

**Container Isolation:**
- One Slack gateway container per Slack workspace
- Gateway container has no access to agent data — it only sends/receives messages via the Orchestrator IPC
- If a user interacts in both DM and a channel, two separate sessions are managed with independent memory

### 10.4 Channel Abstraction

All control channels implement a common interface:

> **Note:** TypeScript interfaces below apply to Web and Slack gateways. The TUI (Go) implements equivalent behavior; the gRPC protobuf definitions (§12.3) are the shared contract between Go and TypeScript components.

```typescript
interface ControlChannel {
  // Receive inbound messages from the channel
  onMessage(handler: (msg: InboundMessage) => void): void;

  // Send outbound messages to the channel
  send(msg: OutboundMessage): Promise<void>;

  // Channel lifecycle
  connect(): Promise<void>;
  disconnect(): Promise<void>;
  healthCheck(): Promise<ChannelHealth>;
}

interface InboundMessage {
  id: string;
  channelType: 'tui' | 'web' | 'slack';
  senderId: string;          // User identity
  contextId: string;         // User ID (1:1) or Group ID (group)
  contextType: 'user' | 'group';
  content: string;
  attachments?: Attachment[]; // File uploads, images (see §10.2 multi-modal flow)
  timestamp: number;
  metadata?: Record<string, unknown>;  // Channel-specific data
}

interface Attachment {
  filename: string;
  mimeType: string;
  path: string;              // Path within session data dir (e.g., "uploads/image.png")
  sizeBytes: number;
}

interface OutboundMessage {
  targetContextId: string;
  content: string;
  replyToMessageId?: string;
  metadata?: Record<string, unknown>;
}
// Note: OutboundMessage here is the TypeScript gateway representation.
// The protobuf OutboundMessage in §12.3 is its serialized gRPC equivalent.
```

---

## 11. Container Architecture

### 11.1 Container Inventory

| Container | Purpose | Lifecycle | Count |
|---|---|---|---|
| **Orchestrator** | Global scheduler, container manager, IPC broker | Long-running (host process) | 1 (runs on host, not containerized) |
| **LLM Gateway** | Provider-agnostic LLM proxy | Long-running | 1 |
| **Assistant Agent** | Conversational AI. Primary entry point — delegates to other agents for tool work. | Per-session (reused) | 1 per active session |
| **Worker Agent** | General-purpose task execution (web search, file read, message sending) | On-demand (per delegation) | 1 per task |
| **Researcher Agent** | Long-running research | On-demand | 1 per task |
| **Coder Agent** | Code analysis and generation | On-demand | 1 per task |
| **Memory Agent** | Session archival memory extraction | On-demand (triggered by archival) | 1 per archival task |
| **Heartbeat Agent** | Scheduled heartbeat prompt execution | On-demand (triggered by cron) | 1 per heartbeat invocation |
| **Tool Containers** | Execution sandbox for tools | Ephemeral (per invocation) | Dynamic |
| **Slack Gateway** | Slack I/O | Long-running | 1 per workspace |
| **Web Gateway** | Web UI serving + WebSocket | Long-running | 1 |
| **MCP Servers** | External tool providers | Long-running | 1 per MCP server |

### 11.2 Container Security Controls

Every non-host container enforces these constraints:

| Control | Implementation |
|---|---|
| **Non-root execution** | All containers run as non-root user (UID 1000) |
| **Read-only root filesystem** | Container filesystem is read-only except explicitly mounted volumes |
| **Seccomp profiles** | Restricted system call set — no `ptrace`, no `mount`, no raw sockets. Profile defined in `container/seccomp/default.json` in the source repository. **Fallback:** If the runtime does not support seccomp (e.g., Apple Containers), equivalent restrictions should be applied via the runtime's native security model. The `ContainerRuntime` abstraction handles this per-runtime. |
| **Capability dropping** | All Linux capabilities dropped except explicitly required ones |
| **Resource quotas** | CPU, memory, and disk limits enforced per container |
| **No host network** | Containers use isolated networking. No host network mode. |
| **No privileged mode** | Never `--privileged` |

### 11.3 Mount Security Model

The Orchestrator enforces a strict mount policy. An agent container for User A can **never** see data belonging to User B.

**Agent Container Mounts:**

| Mount Path (in container) | Source | Permission | Notes |
|---|---|---|---|
| `/agent/soul/` | `agents/{role}/SOUL.md` | Read-only | Agent personality |
| `/agent/identity/` | `agents/{role}/IDENTITY.md` | Read-only | Agent operational role |
| `/context/user/` | `users/{user-id}/` | Read-only (default) | User preferences and memory. **Memory Agent gets read-write** to write extracted memories. |
| `/context/group/` | `groups/{group-id}/` | Read-only (default) | Group preferences and memory. **Memory Agent gets read-write** for group archival. |
| `/session/` | `sessions/{session-id}/` | Read-write | Session history and data |
| `/agent/skills/manifest.json` | Generated skill manifest | Read-only | Skill discovery metadata (see §9.1.1) |

**Tool Container Mounts:**

| Mount Path (in container) | Source | Permission | Notes |
|---|---|---|---|
| `/workspace/` | `sessions/{session-id}/data/` | Read-write | **The only writable mount** (for all tool containers) |
| `/project/{name}/` | User-configured paths from `mount-allowlist.json` | Configured per path | **Coder tool containers only.** Allows access to real project files. |
| `/skill/scripts/` | `users/{user-id}/skills/{skill-name}/scripts/` | Read-only | **Skill script execution only.** Mounted when running skill scripts (see §9.1.1). |

**Mount Security Enforcement:**
- **Allowlist-based** — Only paths explicitly configured can be mounted (from `mount-allowlist.json`)
- **Blocked patterns** — `.ssh`, `.gnupg`, `.aws`, `.env`, `credentials`, and other sensitive paths are always blocked
- **Path traversal prevention** — No `..` sequences allowed; symlinks resolved and validated
- **Automatic permission downgrade** — Non-admin contexts get read-only access to shared resources

---

## 12. Inter-Container Communication (IPC)

### 12.1 IPC Strategy

All inter-container communication is **brokered by the Orchestrator**. No container communicates directly with another container. This ensures the Orchestrator can enforce policies, log events, and maintain a complete audit trail.

**Streaming Proxy Overhead:** This design means every LLM streaming token flows Agent → Orchestrator → LLM Gateway → Provider and back. The Orchestrator proxies all concurrent streams, adding a small latency per token (sub-millisecond for UDS forwarding). The implementation should use zero-copy stream forwarding to minimize overhead. This is an intentional trade-off: the security and observability benefits of centralized brokering outweigh the marginal latency cost. If profiling reveals the Orchestrator as a throughput bottleneck under high concurrency, a direct agent → LLM Gateway connection could be considered as an optimization (low security risk since the gateway holds no user data), but this is not planned.

### 12.2 Transport Mechanisms

baaaht uses **two** IPC mechanisms, chosen to minimize protocol surface area:

| Transport | Use Case | Direction |
|---|---|---|
| **gRPC over Unix Domain Sockets (UDS)** | All structured, real-time communication between the Orchestrator and containers — including control traffic, LLM streaming, TUI interaction, and Web UI updates | Bidirectional |
| **File-based IPC** | Async, fire-and-forget operations from containers to the Orchestrator — tool results, outbound message requests, task scheduling requests | Container → Orchestrator |

**Why gRPC for everything real-time:**
- gRPC natively supports **bidirectional streaming**, which covers the use cases previously assigned to WebSockets (LLM response streaming, Web UI real-time updates) and named pipes (TUI streaming)
- Using a single protocol for all real-time communication simplifies debugging, monitoring, and security enforcement
- gRPC over Unix Domain Sockets (UDS) — a local-only socket file on the filesystem — avoids TCP overhead while maintaining the same API surface as network gRPC. No network exposure, no port management.
- Both Go and TypeScript have mature gRPC libraries with streaming support
- The Web Gateway translates between gRPC (internal) and WebSocket (browser-facing) at the edge — browsers don't speak gRPC, so the gateway serves as a protocol bridge

**Why file-based IPC is retained:**
- Some operations are inherently async and don't need a response (e.g., "send this message to Slack")
- File-based IPC survives container restarts — if an agent writes a message request and the Orchestrator restarts, the request isn't lost
- Tool containers prefer file-based IPC for fire-and-forget results (write output, exit). However, tool containers that need a synchronous response — such as `web_search` and `web_fetch` — make gRPC calls via the mounted UDS socket. A single RPC round-trip completes in milliseconds, well within a tool container's lifetime.
- Human-debuggable: you can `cat` an IPC file to see what happened

**What UDS means in practice:**
A Unix Domain Socket is a file on the filesystem (e.g., `/data/ipc/orchestrator.sock`) that processes connect to like a network socket, but all communication stays within the kernel — no TCP, no network stack, no port exposure. The Orchestrator creates the socket file and mounts it into containers that need it.

### 12.3 gRPC Service Definitions

```protobuf
// Core orchestrator services exposed to containers
service Orchestrator {
  // Agent lifecycle
  rpc RegisterAgent(AgentRegistration) returns (AgentConfig);
  rpc ReportStatus(AgentStatus) returns (Ack);

  // Message routing
  rpc SendMessage(OutboundMessage) returns (Ack);
  rpc RouteToAgent(AgentRoutingRequest) returns (AgentRoutingResponse);
  rpc DelegateToAgent(DelegateRequest) returns (DelegateResponse);  // Used by Assistant's delegate tool

  // Tool execution
  rpc RequestToolExecution(ToolRequest) returns (ToolResponse);

  // Web proxy (called by tool containers for web_search / web_fetch)
  rpc ProxyWebRequest(WebProxyRequest) returns (WebProxyResponse);

  // LLM proxy (bidirectional streaming)
  rpc StreamLLM(LLMRequest) returns (stream LLMEvent);

  // Task scheduling (see §15.10 for message types)
  rpc ScheduleTask(TaskDefinition) returns (TaskId);
  rpc ListTasks(TaskFilter) returns (TaskList);
  rpc GetTask(TaskId) returns (TaskDefinition);
  rpc PauseTask(TaskId) returns (Ack);
  rpc ResumeTask(TaskId) returns (Ack);
  rpc CancelTask(TaskId) returns (Ack);
  rpc GetNextRuns(TaskFilter) returns (NextRunList);

  // Skill access (Orchestrator-mediated)
  rpc GetSkillManifest(SkillManifestRequest) returns (SkillManifest);
  rpc GetSkillContent(SkillContentRequest) returns (SkillContent);

  // Channel streaming (for TUI, Web Gateway, Slack Gateway)
  rpc SubscribeEvents(EventSubscription) returns (stream OrchestratorEvent);
  rpc PublishEvent(ChannelEvent) returns (Ack);
}

// Container-side service for Orchestrator-initiated health checks (§18.3)
service ContainerHealth {
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}

// LLM Gateway service — called by the Orchestrator to proxy LLM requests
// The Orchestrator receives StreamLLM calls from agents and forwards them here.
service LLMGateway {
  // Stream an LLM request — gateway adds credentials, routes to provider, streams response
  rpc StreamCompletion(LLMCompletionRequest) returns (stream LLMCompletionEvent);

  // List available models across all configured providers
  rpc ListModels(ListModelsRequest) returns (ListModelsResponse);

  // Report current provider health and rate limit status
  rpc GetProviderStatus(ProviderStatusRequest) returns (ProviderStatusResponse);
}
```

### 12.4 File-Based IPC Layout

For operations that don't require real-time bidirectional communication (drawing from nanoclaw's proven pattern). All file-based IPC is **container → Orchestrator** only — the Orchestrator watches these directories and processes files as they appear.

```
data/ipc/
├── {context-id}/
│   ├── messages/           # Outgoing message requests (container writes, Orchestrator reads and routes)
│   │   └── {uuid}.json
│   ├── tasks/              # Task scheduling requests (container writes, Orchestrator processes)
│   │   └── {uuid}.json
│   └── results/            # Tool execution results (tool container writes output, Orchestrator collects)
│       └── {uuid}.json
└── system/
    ├── available_groups.json    # Orchestrator-maintained, containers read
    └── container_status.json    # Orchestrator-maintained, containers read
```

> **Note:** The `system/` directory is the exception — it is **Orchestrator → container** (Orchestrator writes, containers read). All per-context directories are container → Orchestrator.

### 12.5 IPC Security

| Control | Implementation |
|---|---|
| **Mutual authentication** | gRPC connections authenticated with per-container bearer tokens. Token lifecycle: the Orchestrator generates a unique token per container at creation time, injects it via the `BAAAHT_CONTAINER_TOKEN` environment variable, and validates the token on every incoming gRPC call. Tokens are ephemeral — they exist only for the container's lifetime and are not persisted. |
| **Encryption** | Not used for UDS (kernel-local traffic never traverses a network — an attacker who can read UDS traffic already has root). If multi-host deployment is added in the future, mTLS will be required for network gRPC. |
| **Policy filtering** | Orchestrator validates every IPC request against the sender's permissions |
| **Audit logging** | All IPC requests logged with timestamp, sender, operation, and result |
| **Directory-based authorization** | File-based IPC uses directory paths as identity (each context gets its own IPC directory) |

---

## 13. Security Architecture

### 13.1 Security Model Summary

baaaht's security is **structural, not policy-based**. Security boundaries are enforced by the operating system (container isolation, filesystem namespaces, network isolation) rather than application-level permission checks.

```
┌──────────────────────────────────────────────────────┐
│                     Host (Trusted)                   │
│                                                      │
│  Orchestrator ← Only component with full access      │
│  Credentials  ← Never exposed to agent containers    │
│  Mount Policy ← Enforced before container creation   │
│                                                      │
├───────────── Container Boundary ─────────────────────┤
│                                                      │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐  │
│  │ Agent       │  │ LLM Gateway │  │ Tool         │  │
│  │ Container   │  │             │  │ Container    │  │
│  │             │  │ Has API     │  │              │  │
│  │ No API keys │  │ keys only   │  │ No API keys  │  │
│  │ Scoped      │  │ No user     │  │ Minimal      │  │
│  │ mounts      │  │ data        │  │ mounts       │  │
│  │ No network* │  │ Egress only │  │ No network   │  │
│  └─────────────┘  └─────────────┘  └──────────────┘  │
│                                                      │
│  * Agents communicate only via Orchestrator IPC      │
└──────────────────────────────────────────────────────┘
```

### 13.2 Credential Management

**Zero-Token Exposure to Agents:**
Agent containers do not hold API keys. The flow:

1. Agent container sends an unauthenticated LLM request via gRPC to the Orchestrator
2. Orchestrator routes the request to the LLM Gateway
3. LLM Gateway (the only container with API keys) adds credentials and forwards to the provider
4. Response streams back through the same chain

This prevents prompt injection attacks from extracting API keys via `echo $ANTHROPIC_API_KEY` or similar techniques.

**Credential Storage:**
- API keys stored in an encrypted configuration file on the host (`config/secrets.enc`)
- Encryption: AES-256-GCM with a key derived from a user-provided passphrase (PBKDF2) or the system keychain (macOS Keychain / Linux Secret Service)
- Decrypted at runtime by the Orchestrator — decrypted keys are held only in memory, never written to disk in plaintext
- Injected into the LLM Gateway container via environment variables (not mounted files)
- The `baaaht` CLI prompts for the passphrase on first startup; subsequent runs use the keychain if available
- Future: Vault-backed secrets with automatic rotation

**Skill Environment Variables (Second Credential Path):**
Skills may declare required environment variables in their `addons.env` field (§9.1.2). These are injected by the Orchestrator directly into **tool containers** (not agent containers) when executing skill scripts. This is a separate credential path from the LLM Gateway — the same credential store (`secrets.enc`) is used, but the injection target is different. Agent containers never receive these variables.

### 13.3 Network Security

| Component | Network Policy |
|---|---|
| **Agent containers** | No outbound network. All external communication via Orchestrator IPC. |
| **Tool containers** | No outbound network. Web tools use a proxy through the Orchestrator. |
| **LLM Gateway** | Outbound to LLM provider APIs only. No inbound from internet. |
| **Slack Gateway** | Outbound to Slack API only. Inbound WebSocket from Slack. |
| **Web Gateway** | Inbound HTTP/WebSocket on configured port. No other network access. |

**Web Proxy Architecture:**

Tool containers for `web_search` and `web_fetch` have no direct network access. Instead, web requests are proxied through the Orchestrator via a synchronous gRPC call:

1. An agent calls `RequestToolExecution` on the Orchestrator, which spawns a `web_search` or `web_fetch` tool container with the Orchestrator's UDS socket mounted
2. The tool container calls `ProxyWebRequest` on the Orchestrator via gRPC, specifying the URL, method, headers, and any search query
3. The Orchestrator validates the request against URL allowlists, applies rate limits and content size limits, then makes the HTTP request from the host
4. The Orchestrator returns the response (status, headers, body) to the tool container via the gRPC response
5. The tool container processes the response (HTML sanitization, content extraction) and writes its result via file-based IPC

This ensures that even if a tool container is compromised, it cannot make arbitrary network requests. The Orchestrator acts as a controlled egress point with full audit logging of all web requests.

### 13.4 Filesystem Security

| Data Area | Agent Access | Tool Access | Orchestrator Access |
|---|---|---|---|
| `agents/` | Read-only (own role only) | None | Read-only |
| `users/{id}/` | Read-only (own user only). **Memory Agent: read-write** (for archival memory extraction). | None | Read-write |
| `groups/{id}/` | Read-only (own group only). **Memory Agent: read-write** (for group archival). | None | Read-write |
| `sessions/{id}/session.jsonl` | Read-write (own session) | None | Read-write |
| `sessions/{id}/data/` | Read-write (via `/session/` mount — see §11.3) | **Read-write** | Read-write |
| User project paths (from `mount-allowlist.json`) | Never | **Configured per path** (Coder tool containers only, see §8.3) | Read-write |
| `sessions/{id}/memory/` | Read-write (own session) | None | Read-write |
| Host filesystem | **Never** | **Never** | Full |
| Credentials | **Never** | **Never** | Full |

### 13.5 Threat Model

| Threat | Mitigation | Status |
|---|---|---|
| **Prompt injection → credential theft** | API keys never in agent containers. LLM Gateway proxy. | ✅ Mitigated by design |
| **Prompt injection → data exfiltration** | No direct network from agent/tool containers. All egress proxied through Orchestrator with URL filtering and audit logging. Residual risk: encoded exfiltration via crafted query parameters in `web_search`/`web_fetch` requests (e.g., `https://evil.com/?data=SECRET`). | ⚠️ Partial (proxied egress) |
| **Cross-user data leakage** | Per-user/group filesystem isolation. Mount policy enforced by Orchestrator. | ✅ Mitigated by design |
| **Container escape** | Seccomp profiles, capability dropping, non-root, read-only rootfs. | ✅ Defense in depth |
| **Malicious tool execution** | Tools run in ephemeral containers with minimal mounts. Pattern blocking for dangerous commands. | ✅ Mitigated |
| **IPC tampering** | Mutual authentication, encrypted transport, policy filtering. | ✅ Mitigated |
| **Resource exhaustion** | Per-container CPU/memory/disk quotas. Task execution timeouts. | ✅ Mitigated |
| **Replay attacks** | Message deduplication with ordered ID tracking per channel. | ⚠️ Partial (per channel) |
| **Malicious skill scripts** | Skill scripts execute in sandboxed tool containers with no network, minimal mounts, and non-root execution. Container isolation limits blast radius. | ✅ Mitigated by container isolation |
| **Malicious skill prompt injection** | Malicious `SKILL.md` content can manipulate agent behavior via prompt injection (e.g., instructions to exfiltrate data via `web_search` queries). Skills are human-readable for manual vetting. Future: review-before-install workflow, content scanning. | ⚠️ Auditable, not verified |
| **Malicious MCP servers** | MCP servers run containerized with restricted mounts and network. User responsibility to vet. | ⚠️ Auditable, not verified |
| **LLM Gateway compromise** | LLM Gateway holds all API keys in environment variables. A container escape or RCE would expose every key. Mitigations: seccomp, read-only rootfs, no inbound network, capability dropping, minimal attack surface (no user input processing). | ⚠️ Primary residual risk |
| **Orchestrator compromise** | Full system compromise — the Orchestrator has access to all data, credentials, and container control. Mitigations: no direct user input processing (all user input goes through agents), no LLM-generated code execution on host, Go memory safety, minimal external dependencies, code review. The Orchestrator is the single trusted component. | ⚠️ Catastrophic but low-probability |

### 13.6 Mount Allowlist

Drawing from nanoclaw's mount-allowlist pattern:

```json
// $BAAAHT_DATA/config/mount-allowlist.json (see §14.1 for data directory layout)
{
  "allowedMounts": [
    {
      "hostPath": "/Users/billm/projects",
      "containerPath": "/workspace/projects",
      "options": "ro",
      "type": "project"          // "project" = Coder tool container mount (rw for code ops)
    },
    {
      "hostPath": "/Users/billm/reference-docs",
      "containerPath": "/workspace/reference",
      "options": "ro",
      "type": "reference"        // "reference" = read-only reference data (no write access)
    }
  ],
  "blockedPatterns": [
    ".ssh", ".gnupg", ".aws", ".env",
    "credentials", ".kube", ".docker",
    "*.pem", "*.key", "id_rsa",
    ".config/baaaht", ".local/share/baaaht",
    ".bash_history", ".zsh_history"
  ]
}
```

---

## 14. Data Model & Persistence

### 14.1 Data Directory Layout

This is the **runtime data directory** layout — the persistent state that baaaht manages on disk. This is not the source code layout (container image definitions, source, etc. live in the source repository).

```
$BAAAHT_DATA/                       # Default: ~/.local/share/baaaht/
├── agents/                         # Agent identity definitions (static)
│   └── {agent-role}/
│       ├── SOUL.md                 # Personality and communication style
│       └── IDENTITY.md             # Operational role and constraints
│
├── users/                          # Per-user persistent data
│   └── {user-id}/
│       ├── USER.md                 # User preferences and context
│       ├── HEARTBEAT.md            # Cadence-based recurring prompts (see §15.3)
│       ├── memory/                 # User's long-term memory store
│       │   └── (abstract storage)
│       └── skills/                 # Per-user installed skills
│           └── {skill-name}/
│               └── SKILL.md
│
├── groups/                         # Per-group persistent data
│   └── {group-id}/
│       ├── GROUP.md                # Group preferences and shared context
│       ├── memory/                 # Group's long-term memory store
│       │   └── (abstract storage)
│       └── skills/                 # Per-group installed skills
│           └── {skill-name}/
│               └── SKILL.md
│
├── sessions/                       # Per-session working data
│   └── {session-id}/
│       ├── session.jsonl           # Conversation history (tree-structured)
│       ├── data/                   # Tool workspace (ONLY dir tools can write to)
│       │   └── (agent working files)
│       └── memory/                 # Session-specific ephemeral memory
│           └── (abstract storage)
│
├── skills/                         # System-level bundled skills (read-only)
│   └── {skill-name}/
│       ├── SKILL.md
│       ├── scripts/
│       ├── references/
│       └── assets/
│
├── config/                         # System configuration
│   ├── config.yaml                 # Global system configuration
│   ├── mount-allowlist.json        # Mount security policy
│   ├── providers.yaml              # LLM provider configuration
│   └── secrets.enc                 # Encrypted credentials (host-only)
│
└── data/                           # Runtime data
    ├── ipc/                        # Inter-container communication
    │   ├── orchestrator.sock       # gRPC Unix Domain Socket
    │   ├── {context-id}/           # File-based IPC per context
    │   └── system/                 # Orchestrator-maintained status files (read by containers)
    │       ├── available_groups.json
    │       └── container_status.json  # IPC-visible snapshot of active containers (subset of state/containers.json)
    ├── logs/                       # Container execution logs
    │   └── {container-id}/         # Per-container log directory
    └── state/                      # Orchestrator persistent state (crash recovery)
        ├── sessions.json           # Active session metadata (id, user, context, container id, last activity)
        ├── scheduler.json          # All scheduled tasks (heartbeats, scheduled, system) — see §15.8
        └── containers.json         # Full container inventory for orphan detection on restart (Orchestrator-only)
```

### 14.1.1 Source Code Layout

The source repository is organized around the language split (Go orchestrator + TypeScript agents/gateways):

```
baaaht/
├── cmd/                            # Go CLI entry points
│   ├── baaaht/                     # Orchestrator daemon binary
│   └── baaaht-tui/                 # TUI client binary
│
├── internal/                       # Go internal packages (orchestrator)
│   ├── orchestrator/               # Event router, session lifecycle
│   ├── container/                  # ContainerRuntime interface + implementations
│   │   ├── docker/                 # Docker runtime implementation
│   │   ├── apple/                  # Apple Containers implementation
│   │   └── podman/                 # Podman runtime implementation (optional)
│   ├── ipc/                        # gRPC server, file-based IPC watcher
│   ├── scheduler/                  # Cron/interval/one-shot scheduler
│   ├── policy/                     # Mount validation, resource quotas
│   └── config/                     # Configuration loading
│
├── proto/                          # gRPC protobuf definitions (shared)
│
├── agent/                          # TypeScript agent core (shared across agents)
│   ├── src/
│   │   ├── runtime/                # Agent loop, context builder, tool dispatch
│   │   ├── tools/                  # Built-in tool implementations
│   │   ├── skills/                 # Skill loader, manifest parser
│   │   ├── memory/                 # MemoryStore interface + filesystem impl
│   │   ├── session/                # SessionStore interface + JSONL impl
│   │   └── ipc/                    # gRPC client, file-based IPC writer
│   └── package.json
│
├── gateway/                        # TypeScript gateway implementations
│   ├── llm/                        # LLM Gateway (provider abstraction)
│   ├── web/                        # Web UI gateway (React + API)
│   └── slack/                      # Slack gateway
│
├── container/                      # Container image definitions
│   ├── base/                       # Common base image (Node.js 22)
│   ├── agent/                      # Agent container image
│   ├── tool/                       # Tool container image
│   ├── llm-gateway/                # LLM Gateway image
│   ├── web-gateway/                # Web Gateway image
│   └── slack-gateway/              # Slack Gateway image
│
├── agents/                         # Agent identity definitions (SOUL.md, IDENTITY.md)
│   ├── assistant/
│   ├── worker/
│   ├── researcher/
│   ├── coder/
│   ├── memory/
│   └── heartbeat/
│
├── skills/                         # Bundled system skills
│   ├── github/
│   ├── web-research/
│   ├── code-review/
│   ├── summarize/
│   └── skill-creator/
│
├── tui/                            # Go TUI implementation (Bubbletea)
│   └── internal/
│
├── docs/                           # Documentation and PRDs
│
└── config/                         # Default configuration templates
    ├── config.yaml.example
    └── providers.yaml.example
```

### 14.2 Session Store

Session persistence is accessed through an abstract interface, with an initial JSONL implementation:

```typescript
interface SessionStore {
  // Create a new session
  create(sessionId: string, header: SessionHeader): Promise<void>;

  // Append an entry (message, compaction, branch, model change, etc.)
  append(sessionId: string, entry: SessionEntry): Promise<void>;

  // Read all entries for a session
  read(sessionId: string): Promise<SessionEntry[]>;

  // Read entries along a specific branch path (root → leaf)
  readBranch(sessionId: string, leafEntryId: string): Promise<SessionEntry[]>;

  // List sessions with optional filtering
  list(filter?: SessionFilter): Promise<SessionHeader[]>;

  // Archive a session (moves from active to cold storage)
  archive(sessionId: string): Promise<void>;
}

interface SessionHeader {
  id: string;
  version: number;
  userId: string;
  groupId?: string;
  contextType: 'user' | 'group';
  created: string;               // ISO 8601
  title?: string;                // Auto-generated or user-set session title
  status: 'active' | 'archived';
}

// Discriminated union for session log entries
type SessionEntry =
  | MessageEntry
  | DelegateEntry
  | CompactionEntry
  | ModelChangeEntry;

interface BaseEntry {
  id: string;
  parent: string;               // ID of parent entry (tree structure)
  timestamp: string;            // ISO 8601
}

interface MessageEntry extends BaseEntry {
  type: 'message';
  message: {
    role: 'user' | 'assistant' | 'system';
    content: string;
    channelType?: 'tui' | 'web' | 'slack';  // Which channel the message came from
  };
}

interface DelegateEntry extends BaseEntry {
  type: 'delegate';
  agent: 'worker' | 'researcher' | 'coder' | 'memory';
  task: string;
  result: string;
}

interface CompactionEntry extends BaseEntry {
  type: 'compaction';
  summary: string;
  compactedEntryIds: string[];  // Entries summarized by this compaction
}

interface ModelChangeEntry extends BaseEntry {
  type: 'model_change';
  previousModel: string;
  newModel: string;
}

interface SessionFilter {
  userId?: string;
  groupId?: string;
  contextType?: 'user' | 'group';
  status?: 'active' | 'archived';
  createdAfter?: string;        // ISO 8601
  createdBefore?: string;       // ISO 8601
}
```

**Initial Implementation: JSONL Backend**

Sessions use tree-structured JSONL (from pi-agent) enabling branching without duplicating conversation history:

```jsonl
{"type":"header","version":1,"id":"session-abc","created":"2026-02-05T10:00:00Z","userId":"user-123","contextType":"user"}
{"type":"message","id":"msg-1","parent":"session-abc","message":{"role":"user","content":"Hello"}}
{"type":"message","id":"msg-2","parent":"msg-1","message":{"role":"assistant","content":"Hi! How can I help?"}}
{"type":"message","id":"msg-3","parent":"msg-2","message":{"role":"user","content":"Review this code"}}
{"type":"message","id":"msg-4","parent":"msg-2","message":{"role":"user","content":"Actually, help me debug instead"}}
{"type":"compaction","id":"compact-1","parent":"msg-3","summary":"User asked for code review..."}
{"type":"delegate","id":"del-1","parent":"msg-3","agent":"worker","task":"Search for recent news about AI safety","result":"Found 3 relevant articles..."}
```

The `parent` field creates a tree: `msg-3` and `msg-4` are branches from the same point (`msg-2`). This enables `/tree` navigation and `/fork` without losing history.

**Future Backends:**
- SQLite — Faster queries for large session histories, FTS for search across sessions
- Hybrid — JSONL for active sessions, SQLite for archived/searchable sessions

### 14.3 Memory Abstraction

Memory is accessed through an abstract interface, with an initial filesystem implementation:

```typescript
interface MemoryStore {
  // Write a memory entry
  write(key: string, content: string, metadata?: MemoryMetadata): Promise<void>;

  // Read a specific memory entry
  read(key: string): Promise<MemoryEntry | null>;

  // Search memories by query (implementation-defined: keyword, semantic, etc.)
  search(query: string, options?: SearchOptions): Promise<MemoryEntry[]>;

  // List all memory entries
  list(filter?: MemoryFilter): Promise<MemoryEntry[]>;

  // Delete a memory entry
  delete(key: string): Promise<void>;
}

interface MemoryEntry {
  key: string;
  content: string;
  metadata: MemoryMetadata;
  createdAt: number;
  updatedAt: number;
}

interface MemoryMetadata {
  source?: string;      // Where the memory came from (user, agent, skill, etc.)
  tags?: string[];      // Categorization tags
  ttl?: number;         // Time-to-live in seconds (0 = permanent). Expired entries are removed by a system task (§15.1) running at configurable interval (default: daily).
  importance?: number;  // Priority ranking for retrieval
}
```

**Initial Implementation: Filesystem Backend**
- Memory entries stored as individual markdown files
- Metadata stored as YAML frontmatter
- Search implemented as full-text search over file contents
- Index maintained as a lightweight JSON manifest

**Future Backends:**
- SQLite — Structured queries, FTS5 for full-text search
- SQLite + vector extension — Semantic search via embeddings
- Dedicated vector DB (ChromaDB, Qdrant) — Full semantic memory

### 14.4 Session Archival

Sessions are archived based on a combination of user requests and system policy.

**Archival Triggers:**

| Trigger | Description |
|---|---|
| **User request** | User explicitly archives a session via `/archive` command or UI action |
| **Inactivity timeout** | System automatically archives sessions with no user messages for a configurable duration (default: 24 hours) |

**Archival Process:**

1. The Orchestrator identifies sessions eligible for archival (inactivity timeout exceeded, or user-requested)
2. A **Memory Agent** container is spawned to process the session before archival
3. The Memory Agent reviews the conversation history and extracts content that should be persisted to the user's long-term memory — user preferences, decisions, facts, and other memorable information that hasn't already been captured
4. Extracted memories are written to the user's memory store (`users/{user-id}/memory/`)
5. **For group sessions:** Memories relevant to the group (shared decisions, group context, and attributed user contributions) are written to `groups/{group-id}/memory/`. The Memory Agent does **not** write to individual users' memory stores during group archival — all group session memories are consolidated in the group memory store to avoid multi-user mount complexity. Users retain access to group memories through their group context mounts.
6. The session is moved from active to archived storage
7. The session's agent container (if still running) is torn down

**Archival State Tracking:**
Session state (active vs. archived) is tracked in `data/state/sessions.json`. Each entry has a `status` field (`"active"` or `"archived"`) and an `archivedAt` timestamp. Archived sessions remain in `sessions/{session-id}/` on disk — they are not moved to a separate directory. The `/resume` command filters by status and can reactivate an archived session by setting its status back to `"active"` and spawning a new Assistant container.

```json
// Example entry in data/state/sessions.json
{
  "id": "session-abc",
  "userId": "billm",
  "contextType": "user",
  "containerId": "abc123",
  "lastActivity": "2026-02-06T14:00:00Z",
  "status": "active",           // "active" | "archived"
  "archivedAt": null             // ISO 8601 timestamp when archived
}
```

**Archival Configuration:**

```yaml
# In config.yaml
sessions:
  archival:
    inactivity_timeout: 86400    # seconds (default: 24 hours since last user message)
    memory_extraction: true       # Run Memory Agent on archival (default: true)
    archive_storage: "local"      # "local" (future: "s3", "gcs")
```

**Archived sessions are not deleted** — they remain on disk in an archived state and can be resumed if needed. The `/resume` command in the TUI and Web UI can browse archived sessions.

---

## 15. Task Scheduling

The Task Scheduling subsystem manages all background and scheduled work in baaaht. It is a core Orchestrator component implemented in `internal/scheduler/`.

### 15.1 Task Categories

Tasks are divided into three categories based on their purpose and trigger mechanism:

| Category | Purpose | Trigger | Prompt Source | Output Handling | Default Context Mode |
|---|---|---|---|---|---|
| **Heartbeat** | Recurring background monitoring (email triage, status checks, digests). Cadence matters, exact timing does not. | Cadence-based (hourly, daily, weekly) with random jitter | User's `HEARTBEAT.md` file | Results sent to Assistant, which messages user on their preferred channel | `session` |
| **Scheduled Task** | User-requested tasks where timing is specific ("remind me at 8am", "every Monday at 9am") | Cron expression, interval, or one-shot timestamp | User-provided prompt via natural language | Results sent to specified notification channel | `isolated` |
| **System Task** | Internal maintenance (session archival, memory cleanup, TTL expiration) | Event-driven or interval | None (system-defined) | Logged, no user notification | `silent` |

### 15.2 Task Definition

All tasks share a common definition structure:

> **Note:** Presented as TypeScript for readability. The canonical definition is the protobuf `TaskDefinition` message (§15.10). The Go scheduler uses the protobuf directly; TypeScript clients use a generated type.

```typescript
interface TaskDefinition {
  id: string;                           // Unique task identifier (UUID)
  type: 'heartbeat' | 'scheduled' | 'system';
  
  // Ownership
  userId: string;                       // User who owns the task
  groupId?: string;                     // Group context (if applicable)
  
  // Schedule (one of these is required)
  schedule: {
    cron?: string;                      // Standard 5-field cron expression
    interval?: number;                  // Seconds between runs
    once?: string;                      // ISO 8601 timestamp for one-shot
    cadence?: 'hourly' | 'daily' | 'weekly' | 'biweekly' | 'monthly';  // For heartbeats
  };
  
  // Execution
  agent: string;                        // Target agent role (e.g., 'heartbeat', 'worker')
  prompt?: string;                      // Task prompt (not needed for heartbeats — sourced from HEARTBEAT.md)
  contextMode: 'session' | 'isolated' | 'silent';
  
  // Output
  notification: {
    channel: string;                    // 'slack/dm', 'slack/{channel-id}', 'web', 'tui', 'none'
                                         // Note: 'tui' only delivers if TUI is currently connected;
                                         // otherwise falls back to next preferred channel or 'none'
    onSuccess: boolean;                 // Send notification on success
    onFailure: boolean;                 // Send notification on failure
  };
  
  // State
  status: 'active' | 'paused' | 'completed' | 'failed';
  lastRun?: string;                     // ISO 8601 timestamp
  nextRun?: string;                     // ISO 8601 timestamp (computed)
  failureCount: number;                 // Consecutive failures
  
  // Metadata
  createdAt: string;                    // ISO 8601 timestamp
  createdBy: 'user' | 'system';         // Who created this task
  description?: string;                 // Human-readable description
}
```

### 15.3 Heartbeat System

Heartbeats are recurring background tasks that run for each user and group at a configured cadence. They are the primary mechanism for proactive assistant behavior — the assistant "wakes up" periodically to check on things the user cares about.

**Heartbeat Flow:**

```
Scheduler evaluates due heartbeats (polling interval: orchestrator.poll_intervals.scheduler)
  │
  ▼
For each due heartbeat:
  ├── Read HEARTBEAT.md for the user/group
  ├── Spawn Heartbeat Agent container with prompt from HEARTBEAT.md
  ├── Agent executes (web search, email check, etc.)
  ├── Agent produces result (may be empty if nothing to report)
  │
  ▼
If result is non-empty:
  ├── Route result to Assistant agent
  ├── Assistant formulates message for user
  ├── Send message via user's preferred channel
  │     └── Session context: most recent session on the preferred channel
  │
  ▼
Container torn down
```

**Heartbeat Configuration (`HEARTBEAT.md`):**

Each user and group has a `HEARTBEAT.md` file defining their recurring prompts. The format uses markdown sections with structured metadata:

- Each `##` heading starts a new heartbeat entry
- `Cadence:` and `Prompt:` are required fields (case-insensitive, leading `- ` optional)
- Multi-line prompts: all text after `Prompt:` until the next `##` heading or end of file is treated as the prompt
- Parse errors: if a section is malformed (missing cadence or prompt), it is skipped with a warning log. Other valid sections are still processed.

```markdown
# HEARTBEAT.md — User: billm

## Email Triage
- Cadence: hourly
- Prompt: Check my email and let me know if anything looks urgent or needs a response today

## Project Status  
- Cadence: daily
- Prompt: Review my open GitHub PRs and issues, summarize anything that needs attention

## Weekly Digest
- Cadence: weekly
- Prompt: Summarize this week's activity from my session history and highlight key decisions made
```

**Cadence Levels:**

| Cadence | Meaning | Jitter Window |
|---|---|---|
| `hourly` | Once per hour | Random minute (0-59) |
| `daily` | Once per day | Random hour (6-22) + random minute |
| `weekly` | Once per week | Random day (Mon-Fri) + random hour (6-22) |
| `biweekly` | Every two weeks | Random day + random hour |
| `monthly` | Once per month | Random day (1-28) + random hour |

The Orchestrator translates cadence levels to cron expressions with random jitter at startup. Jitter prevents all heartbeats from firing simultaneously and distributes load across the schedule window.

### 15.4 Scheduled Tasks

Scheduled tasks are created by users via natural language through the Assistant. When a user says "remind me to check the build at 8am tomorrow" or "every Monday, summarize my PRs", the Assistant delegates to a Worker agent to create the appropriate task.

**Task Creation Flow:**

```
User: "Every morning at 9am, check CNN headlines and summarize"
  │
  ▼
Assistant recognizes scheduling intent → delegates to Worker
  │
  ▼
Worker parses request:
  ├── Schedule: cron "0 9 * * *"
  ├── Agent: worker
  ├── Prompt: "Check CNN headlines and summarize"
  ├── Notification: user's preferred channel
  │
  ▼
Worker calls Orchestrator gRPC: ScheduleTask(TaskDefinition)
  │
  ▼
Orchestrator validates and persists task to scheduler.json
  │
  ▼
Worker returns confirmation to Assistant
  │
  ▼
Assistant: "Done! I'll check CNN headlines every morning at 9am and let you know."
```

**Schedule Types:**

| Type | Syntax | Example |
|---|---|---|
| **Cron** | Standard 5-field | `0 9 * * *` (daily at 9am) |
| **Interval** | Seconds | `3600` (every hour) |
| **One-shot** | ISO 8601 | `2026-03-01T08:00:00Z` (once at that time) |

### 15.5 Task Execution Flow

All task types follow the same execution pattern:

```
Scheduler polling loop (every orchestrator.poll_intervals.scheduler ms)
  │
  ▼
Evaluate all active tasks: is nextRun <= now?
  │
  ▼
For each due task:
  ├── 1. Resolve prompt:
  │     ├── Heartbeat: read from HEARTBEAT.md
  │     └── Scheduled: use task.prompt
  │
  ├── 2. Resolve session context:
  │     ├── 'session': find most recent session for user's preferred channel
  │     ├── 'isolated': create ephemeral session (no history)
  │     └── 'silent': no session, no output
  │
  ├── 3. Spawn agent container:
  │     └── Agent type from task.agent (typically 'heartbeat' or 'worker')
  │
  ├── 4. Agent executes prompt with full tool access
  │
  ├── 5. Collect result:
  │     ├── If non-empty and notification enabled:
  │     │     └── Route to Assistant → message to notification channel
  │     └── If empty or silent: log only
  │
  ├── 6. Update task state:
  │     ├── lastRun = now
  │     ├── nextRun = calculate from schedule
  │     ├── status = 'completed' (for one-shot) or remains 'active'
  │     └── failureCount = 0 (on success) or increment (on failure)
  │
  └── 7. Tear down agent container
```

### 15.6 Context Modes

| Mode | Session | History | Output |
|---|---|---|---|
| **session** | User's most recent session on preferred channel | Full conversation history and memory loaded | Results sent to channel |
| **isolated** | Ephemeral session created for this execution | No prior context | Results sent to channel |
| **silent** | No session | No context | Logged only, no user notification |

### 15.7 Preferred Channel Resolution

Several task execution flows reference the user's "preferred channel" for notification delivery. The preferred channel is resolved as follows:

1. **Explicit preference** — If the user has a `preferred_channel` field set in their `USER.md` profile (e.g., `preferred_channel: slack`), use that channel
2. **Most recently active** — If no explicit preference, use the channel the user most recently sent a message from
3. **Fallback** — If the user has never connected, log the notification only (equivalent to `silent` context mode)

Valid channel values: `slack`, `web`, `tui`

### 15.7 Error Handling

Task failures are logged to file by default. The error notification channel is configurable in system configuration for future expansion (Slack, email, etc.).

| Failure Type | Behavior |
|---|---|
| Agent container fails to start | Retry once. If retry fails, increment `failureCount`, log error, continue to next task. |
| Agent execution times out | Kill container, increment `failureCount`, log error. |
| Agent returns error result | Increment `failureCount`, log error. If `notification.onFailure` is true, notify user. |
| 5 consecutive failures | Pause task automatically. Notify admin. Requires manual intervention to resume. |

**Error Logging Configuration:**

```yaml
# In config.yaml
scheduler:
  error_handling:
    log_file: "data/logs/scheduler-errors.log"
    max_consecutive_failures: 5        # Auto-pause threshold
    notify_channel: "log"              # "log" | "slack" | "email" (future)
```

### 15.8 Task Persistence

Tasks are persisted to `data/state/scheduler.json` and recovered on Orchestrator restart:

```json
{
  "tasks": [
    {
      "id": "task-abc123",
      "type": "heartbeat",
      "userId": "billm",
      "schedule": { "cadence": "hourly" },
      "agent": "heartbeat",
      "contextMode": "session",
      "notification": { "channel": "slack/dm", "onSuccess": true, "onFailure": true },
      "status": "active",
      "lastRun": "2026-02-06T14:23:00Z",
      "nextRun": "2026-02-06T15:17:00Z",
      "failureCount": 0,
      "createdAt": "2026-02-01T10:00:00Z",
      "createdBy": "system",
      "description": "Email Triage (from HEARTBEAT.md)"
    }
  ],
  "heartbeatHashes": {
    "billm": "sha256:abc123..."
  }
}
```

The `heartbeatHashes` field tracks the hash of each user's `HEARTBEAT.md` file. When the file changes, the Orchestrator detects the hash mismatch and regenerates heartbeat tasks.

### 15.9 Task Management

| Operation | Access | Method |
|---|---|---|
| **List tasks** | Any user (own tasks only) | gRPC `ListTasks` or natural language via Assistant |
| **Create task** | Any user (own context) | Natural language via Assistant → Worker → gRPC `ScheduleTask` |
| **Pause/Resume** | Any user (own tasks) | Natural language via Assistant or gRPC `PauseTask`/`ResumeTask` |
| **Cancel** | Any user (own tasks) | Natural language via Assistant or gRPC `CancelTask` |
| **Cross-context management** | Admin users only | gRPC with admin authentication |

### 15.10 gRPC Task Scheduling Service

Task scheduling RPCs are part of the Orchestrator service (§12.3). The message types used:

```protobuf
message TaskFilter {
  string user_id = 1;
  string group_id = 2;
  string type = 3;           // "heartbeat", "scheduled", "system", or empty for all
  string status = 4;         // "active", "paused", "completed", "failed", or empty for all
}
```

---

## 16. Configuration

### 16.1 Configuration File

```yaml
# config/config.yaml — Global system configuration

system:
  name: "baaaht"
  timezone: "America/New_York"
  log_level: "info"
  admin_users: ["billm"]

container:
  runtime: "auto"              # "auto" | "docker" | "apple-containers"
  default_timeout: 300         # 5 minutes
  max_concurrent: 10
  resource_limits:
    cpu: "1.0"
    memory: "512m"
    disk: "1g"

orchestrator:
  poll_intervals:
    messages: 2000             # ms — file-based IPC message directory polling
    scheduler: 60000           # ms
    ipc: 1000                  # ms — file-based IPC results directory polling

agents:
  assistant:
    model: "anthropic/claude-sonnet-4-20250514"
    max_iterations: 20
    temperature: 0.7
  worker:
    model: "anthropic/claude-sonnet-4-20250514"
    max_iterations: 15
  researcher:
    model: "anthropic/claude-sonnet-4-20250514"
    max_iterations: 50
    timeout: 600               # 10 minutes for research tasks
  coder:
    model: "anthropic/claude-sonnet-4-20250514"
    max_iterations: 30
  memory:
    model: "anthropic/claude-haiku-3"
    max_iterations: 10
  heartbeat:
    model: "anthropic/claude-haiku-3"
    max_iterations: 15

sessions:
  archival:
    inactivity_timeout: 86400  # seconds (default: 24h since last user message)
    memory_extraction: true    # Run Memory Agent on archival

channels:
  tui:
    enabled: true
    theme: "dark"
  web:
    enabled: true
    port: 8080
    host: "localhost"
    auth:
      provider: "oidc"
      issuer: "${OIDC_ISSUER}"         # e.g., https://accounts.google.com
      client_id: "${OIDC_CLIENT_ID}"
      client_secret: "${OIDC_CLIENT_SECRET}"
      auto_create_users: true
  slack:
    enabled: false
    workspaces:
      - name: "my-team"
        bot_token: "${SLACK_BOT_TOKEN}"
        app_token: "${SLACK_APP_TOKEN}"
        trigger_pattern: "^@baaaht\\b"

skills:
  directories:                       # System-level bundled skills
    - "./skills"
    - "~/.config/baaaht/skills"
  auto_discover: true
  # User/group skills are auto-discovered from users/{id}/skills/ and groups/{id}/skills/

scheduler:
  error_handling:
    log_file: "data/logs/scheduler-errors.log"
    max_consecutive_failures: 5        # Auto-pause threshold
    notify_channel: "log"              # "log" | "slack" (future)

tools:
  default_timeout: 60                  # seconds
  timeout_overrides:                   # Per-tool timeout overrides
    exec: 60
    web_search: 30
    web_fetch: 30
  blocked_patterns: []                 # Additional patterns beyond built-in (§8.5)
  web_proxy:
    max_redirects: 5
    max_content_size: "1MB"
    rate_limit: 60                     # requests per minute per session
    # URL allowlist/blocklist: not implemented in MVP (all URLs allowed)
```

### 16.2 Provider Configuration

```yaml
# config/providers.yaml

providers:
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
    default: true

  openai:
    api_key: "${OPENAI_API_KEY}"

  openrouter:
    api_key: "${OPENROUTER_API_KEY}"

  ollama:
    api_base: "http://localhost:11434"

  lmstudio:
    api_base: "http://localhost:1234/v1"

  # Providers accessed via their native APIs
  google:
    api_key: "${GOOGLE_API_KEY}"

  mistral:
    api_key: "${MISTRAL_API_KEY}"

  groq:
    api_key: "${GROQ_API_KEY}"
    openai_compatible: true          # Groq uses OpenAI-compatible API

  deepseek:
    api_key: "${DEEPSEEK_API_KEY}"
    api_base: "https://api.deepseek.com/v1"
    openai_compatible: true

  xai:
    api_key: "${XAI_API_KEY}"
    api_base: "https://api.x.ai/v1"
    openai_compatible: true

  # Custom OpenAI-compatible endpoints
  custom:
    - name: "my-vllm"
      api_base: "http://gpu-server:8000/v1"
      api_key: "${VLLM_API_KEY}"
      openai_compatible: true

# Fallback chains — when the primary provider for a request fails,
# try alternatives in order. Each entry maps a provider to its fallbacks.
fallback_chains:
  anthropic: ["openrouter", "bedrock"]     # If Anthropic API is down, try OpenRouter, then Bedrock
  openai: ["openrouter"]                    # If OpenAI is down, try OpenRouter
  # Providers without fallback entries have no automatic failover
```

### 16.3 Environment Variable Overrides

Any configuration value can be overridden with environment variables using `BAAAHT__` prefix:

```bash
export BAAAHT__AGENTS__ASSISTANT__MODEL="openai/gpt-4o"
export BAAAHT__CHANNELS__WEB__PORT=3000
export BAAAHT__CONTAINER__RUNTIME="docker"
```

**Array values:** Array elements cannot be individually overridden via environment variables. To override an array, provide a JSON-encoded value:

```bash
export BAAAHT__SYSTEM__ADMIN_USERS='["billm","alice"]'
```

---

## 17. Multi-Tenancy & Session Model

### 17.1 Context Types

| Context | Scope | Session Mapping | Memory Scope |
|---|---|---|---|
| **User** | 1:1 private (Slack DM, Web login, TUI) | One active session per user, shared across connected channels | Private to user |
| **Group** | Shared multi-user (Slack channel) | One active session per group, shared across connected channels | Shared among group members |

**Multi-Channel Session Sharing (Deferred — Post-MVP):**

In a future version, a user's active session will be shared across all connected channels. If a user has both Web UI and Slack DM open, both channels will feed input to and receive output from the same session:

- **Input:** Messages from any connected channel are routed to the session's Assistant agent
- **Output:** Responses are sent back to all currently connected channels for that session
- **Disconnected channels:** If a channel is not connected (e.g., TUI is closed), messages are not queued for it — only active connections receive output
- **Channel metadata:** Each message in the session records which channel it originated from, enabling the Assistant to be aware of the source context

> **Open Design Questions for Multi-Channel:**
> - **Message ordering:** If a user sends messages from Slack and Web simultaneously, the Orchestrator must serialize them into a single agent input queue. The simplest approach is arrival-order (first message received wins), with the second queued behind the first's response.
> - **Response broadcast:** Responses should be sent to all currently connected channels for the session. The original message's `channelType` is recorded so the Assistant can tailor responses if needed.
> - **Cross-channel context:** If a user sends from Slack, the response appears in both Slack and Web UI. Both channels show the full conversation history, not just their own messages.

> **Identity Linking Required:** Cross-channel session sharing depends on an identity linking mechanism that maps channel-specific identities (OIDC subject IDs, Slack user IDs) to a single canonical user ID (UUID-based). This mechanism is not yet designed.
>
> **Design Considerations:**
> - Users should have a UUID-based `canonical-id` as their primary identity
> - The system must resolve channel identity → canonical-id efficiently without scanning all users' identity files
> - An index or lookup table (e.g., `data/state/identity-index.json` mapping `{channel-type}/{channel-id}` → `canonical-id`) is needed for O(1) resolution
> - Until identity linking is implemented, each channel identity operates as a separate user context

**MVP Behavior:** Each channel identity (`oidc-subject-id`, `slack-user-id`, TUI `--user` flag) maps to an independent user context with separate sessions, memory, and preferences.

### 17.2 Identity Resolution

| Channel | User Identity | Context Resolution |
|---|---|---|
| **TUI** | CLI flag (`--user`) — no authentication, host-trust assumed | User context specified by flag (admin use) |
| **Web** | Authenticated user (OIDC subject ID) | User context or group context (via room selection) |
| **Slack DM** | Slack user ID | User context (`users/{slack-user-id}`) |
| **Slack Channel** | Slack user ID (sender) | Group context (`groups/{slack-channel-id}`) |

### 17.3 Privilege Model

| Capability | Admin User | Standard User (own context) | Standard User (other context) |
|---|---|---|---|
| Send messages | ✅ Any context | ✅ Own context | ❌ |
| Read own session data | ✅ | ✅ | ❌ |
| Write to session data/ | ✅ | ✅ (via tools) | ❌ |
| Read user/group memory | ✅ Any | ✅ Own | ❌ |
| Write user/group memory | ✅ Any | ✅ Own | ❌ |
| Install user skills | ✅ | ✅ (own user) | ❌ |
| Install group skills | ✅ | ❌ | ❌ |
| Manage own tasks | ✅ | ✅ | ❌ |
| Manage all tasks | ✅ | ❌ | ❌ |
| Manage mount allowlist | ✅ | ❌ | ❌ |
| Register new groups | ✅ | ❌ | ❌ |
| Deactivate groups | ✅ | ❌ | ❌ |
| Manage providers | ✅ | ❌ | ❌ |
| Modify system config | ✅ | ❌ | ❌ |
| Force-restart containers | ✅ | ❌ | ❌ |
| View system status | ✅ (full) | ✅ (own sessions, active model, container count) | ❌ |

**Group Lifecycle:**
- **Slack channels:** Groups are auto-created when the bot is first mentioned in a Slack channel. The group ID is the Slack channel ID.
- **Web UI:** Groups (rooms) are created by admin users through the Web UI. Standard users can join existing groups.
- **Deactivation:** Admin users can deactivate groups. Deactivated groups retain their data but no longer accept new messages.

---

## 18. Observability

### 18.1 Logging

- **Structured logging** with JSON output from all components
- **Correlation IDs** — Every request gets a unique ID (UUID v4) that propagates through all containers and IPC calls. For gRPC: carried in the `x-correlation-id` metadata header. For file-based IPC: included as a `correlationId` field in the JSON payload. All log entries include the correlation ID for end-to-end tracing.
- **Container logs** — Captured and persisted to `data/logs/{container-id}/`
- **Log levels** — Configurable per component (debug, info, warn, error)

### 18.2 Metrics

| Metric | Source |
|---|---|
| Container spawn time | Orchestrator |
| Container resource usage (CPU, memory) | Container runtime |
| LLM request latency | LLM Gateway |
| Token usage per request/session/user | LLM Gateway |
| Tool execution duration | Tool containers |
| Delegation round-trip latency | Orchestrator |
| Message throughput per channel | Gateways |
| Active sessions count (by status) | Orchestrator |
| Memory store size (per user/group) | Orchestrator |
| Task execution success/failure rate | Scheduler |
| LLM request queue depth | LLM Gateway |

**Metrics Exposure:**
Metrics are collected by the Orchestrator and exposed via a local HTTP endpoint (`/metrics`) in Prometheus exposition format. This endpoint is only accessible on `localhost` (not exposed to containers or external networks). For deployments that don't use Prometheus, metrics are also written to structured log output (JSON) for log-based monitoring.

Future: Grafana dashboard templates, StatsD export option.

### 18.3 Health Checks

The Orchestrator actively monitors all long-running containers via gRPC health probes. This is a pull-based model — the Orchestrator initiates health checks rather than waiting for containers to report status — enabling faster detection of failures.

**Health Check Flow:**

```
Orchestrator health check loop (every orchestrator.poll_intervals.health_check ms)
  │
  ▼
For each long-running container (gateways, LLM Gateway, active Assistant agents):
  ├── Send gRPC HealthCheck request to container (via ContainerHealth service, §12.3)
  ├── Container responds with current status and metrics
  │
  ▼
Evaluate response:
  ├── Healthy: reset failure counter, update last_healthy timestamp
  ├── Unhealthy/Timeout: increment failure counter
  │     ├── If failure_count >= threshold (default: 3):
  │     │     ├── Log error with container details
  │     │     ├── Restart container
  │     │     └── Optionally notify admin channel
  │     └── Else: continue monitoring
```

**Health Status Reporting:**

```protobuf
message HealthCheckRequest {
  string container_id = 1;
}

message HealthCheckResponse {
  string status = 1;           // "healthy", "degraded", "unhealthy"
  string message = 2;          // Human-readable status message
  int64 uptime_seconds = 3;    // Container uptime
  map<string, string> metrics = 4;  // Container-specific metrics
}
```

**Configuration:**

```yaml
# In config.yaml
orchestrator:
  health_check:
    interval: 30000            # ms between health checks
    timeout: 5000              # ms to wait for response
    failure_threshold: 3       # consecutive failures before restart
    notify_channel: "log"      # "log" | "slack" (future)
```

---

## 19. Development Roadmap

### Phase 1 — Core Platform (MVP)

| Component | Deliverables |
|---|---|
| **Orchestrator** | Container lifecycle, session management, IPC broker, policy engine, web proxy for tool containers |
| **Agent Core** | Assistant agent with delegation model, Worker agent for tool execution |
| **LLM Gateway** | Anthropic + OpenAI providers, streaming, token tracking, credential injection |
| **Tool System** | `read_file`, `write_file`, `edit_file`, `list_dir`, `grep`, `find`, `exec`, `web_search`, `web_fetch`, `message` (all containerized). Mount allowlist enforcement. |
| **TUI** | Interactive chat, model selection, session management |
| **Data Layer** | Filesystem-based memory, JSONL sessions (tree-structured), session archival (inactivity-based, no Memory Agent extraction — Phase 2) |
| **Security** | Container isolation, credential management, seccomp profiles, mount allowlist, path validation |
| **Configuration** | YAML config, environment variable overrides, providers.yaml |
| **Skills** | Agent Skills loader, skill manifest injection, 2-3 bundled skills |
| **Scheduling** | Session archival timer, system maintenance tasks. No heartbeats, no user-scheduled tasks (Phase 2). |
| **Container Runtime** | Docker runtime. Apple Containers support (experimental). |

### Phase 2 — Multi-Channel & Multi-Agent

| Component | Deliverables |
|---|---|
| **Slack Gateway** | Per-workspace containers, DM and channel support, trigger matching |
| **Web UI** | React chat interface, session management, OIDC authentication, multi-modal input |
| **Researcher Agent** | Long-running background research tasks |
| **Coder Agent** | Code analysis/generation with exec access, user project mounts |
| **Memory Agent** | Session archival memory extraction, explicit "remember" requests |
| **Heartbeat Agent** | Lightweight agent for scheduled heartbeat prompt execution |
| **Session Archival** | Memory Agent extraction on archival (upgrade from Phase 1 timer-only) |
| **MCP Support** | MCP server discovery and containerized execution |
| **Task Scheduler** | Full cron + interval + one-shot support, heartbeat system, natural language task creation |
| **Skill Installation** | Third-party skill installation from GitHub, skill addon layer system |
| **Additional providers** | OpenRouter, Ollama, LM Studio, Google, Mistral, Groq, custom endpoints |

### Phase 3 — Advanced Security & Operations

| Component | Deliverables |
|---|---|
| **Credential proxy** | Vault-backed secrets, automatic rotation |
| **Network policies** | Configurable per-container egress rules (Phase 1/2 provides binary network on/off per container type) |
| **Audit logging** | Comprehensive audit trail with searchable UI, retention policies (Phase 1/2 provides structured logging and IPC request logging) |
| **Session branching** | Tree-structured sessions with fork/navigate |
| **Context compaction** | Automatic summarization when approaching token limits |
| **Memory backends** | SQLite backend, semantic search exploration |
| **Container pooling** | Pre-warmed containers for faster agent startup |

---

## 20. Resolved Architecture Decisions

| Decision | Resolution | Rationale |
|---|---|---|
| **Primary IPC transport** | gRPC over Unix Domain Sockets | Single-host deployment. UDS avoids network exposure. gRPC provides typed contracts and bidirectional streaming. Both Go and TypeScript have mature libraries. |
| **Event bus** | In-process Go channels | Single-instance deployment. No external infrastructure dependency. Go channels provide the concurrency primitives needed. |
| **Session storage format** | Abstract interface, JSONL (tree) initial implementation | Human-readable, proven in pi-agent. Abstract interface allows migration to SQLite or other backends later without changing agent code. |
| **Memory backend (v2)** | SQLite FTS5 (future) | Lowest operational overhead for structured + full-text search. Upgrade path to vector DB when semantic search is needed. |
| **Container image strategy** | Common base image + per-role layers + per-skill addon layers | Common Node.js 22 base minimizes storage and build time. Role layers add agent/tool/gateway specific deps. Skill addon layers add skill-specific packages (e.g., `gh` CLI). |
| **Auth for Web UI** | OIDC (OpenID Connect) with auto-user creation | Supports any OIDC provider (Google, GitHub, Okta, Keycloak, self-hosted). Users are auto-created on first successful authentication. More robust than API keys, more portable than reverse proxy auth. |
| **Agent-to-agent communication** | Via Orchestrator only | Simpler security is better security. Maintains the invariant that the Orchestrator brokers all communication. Slightly higher latency is an acceptable trade-off for a much simpler security model. |
| **Task scheduling** | Unified scheduler with three task categories | Heartbeats (cadence-based recurring), scheduled tasks (user-requested with specific timing), and system tasks (internal maintenance) all use the same scheduling engine. Heartbeats are translated from cadence levels to cron expressions with random jitter. All tasks stored in `scheduler.json`. See §15 for full specification. |
| **Skill installation** | User-directed install from GitHub repos | Users ask the assistant to install a skill from a GitHub URL. The system clones, validates, and installs. This is a weak security pattern (arbitrary repo execution risk) — will be hardened in future versions with signing, registries, or content scanning. |
| **Session archival** | Inactivity timeout (default 24h) + user request + Memory Agent | Sessions are archived after 24 hours of no user messages (configurable). A Memory Agent runs on archival to extract memorable content into long-term memory. Users can also archive manually. |
| **Agent routing model** | No Router Agent — Assistant is the entry point, delegates to agents | The Assistant has a `delegate` meta-tool for dispatching to specialized agents: Worker (general tasks), Researcher (deep research), Coder (code). This keeps the Assistant's context limited to user messages, delegation calls, and responses. Utility agents (Memory, Heartbeat) are standalone, triggered by the Orchestrator directly. |
| **Assistant delegation model** | `delegate` meta-tool with Worker agent for general tasks | The Assistant has a single `delegate` tool for dispatching work to specialized agents (Worker, Researcher, Coder). The delegate tool call and response appear in the session context, but only at the high level — never raw tool outputs or execution traces from the delegated agent. The Assistant must pass sufficient context in the delegation request since delegated agents have no access to conversation history. A Worker agent handles general-purpose delegated tasks that don't warrant Researcher or Coder. |
| **Memory Agent dual invocation** | Orchestrator-triggered (archival) + Assistant-delegated (user requests) | Memory extraction on session archival is a system concern — the Orchestrator triggers it automatically. Explicit "remember this" requests from users go through the Assistant's `delegate` tool (with `memory` as a target agent). This dual-invocation model avoids giving the Orchestrator content-interpretation responsibilities while allowing system-initiated memory extraction. |

### 20.1 Open Questions

All initial open questions have been resolved. The following questions are **deferred until after initial implementation** — documented here with initial thinking for future reference.

| Question | Status | Initial Thinking |
|---|---|---|
| **Skill security hardening** | Deferred | Review-before-install workflow combined with security scanning: pattern matching in skill code for malicious patterns, plus a security-focused agent running in a locked-down isolated container to analyze skill content before installation. |
| **Memory deduplication** | Deferred | Semantic similarity check against existing memories before writing new ones. Possibly a separate deduplication step that periodically consolidates the memory store, merging near-duplicate entries. |
| **Container pooling strategy** | Deferred | Pre-warm a pool of tool containers and agent containers to reduce startup latency. Need to determine pool size, eviction policy, and whether pool is per-agent-role or shared. Critical for Coder agent workflows that spawn many tool containers per turn. |
| **Heartbeat active hours** | Deferred | Heartbeats currently run 24/7. Consider adding an optional `active_hours` field to HEARTBEAT.md (e.g., `09:00-22:00`) to avoid running costly operations during off-hours. Low priority — single-digit users means cost is manageable. |
| **Concurrent delegation** | Deferred | Can the Assistant issue multiple `delegate` calls in parallel (e.g., ask Worker to search while Coder analyzes code)? Standard LLM tool-calling supports parallel tool calls, but the Orchestrator would need to handle concurrent container spawns for the same session. Worth exploring for Phase 2. |
| **Planner Agent** | Deferred | A dedicated agent for decomposing complex multi-step tasks into executable plans and coordinating other agents. Would use a high-reasoning model and have access to Read and message tools (to dispatch to other agents). Key open questions: How does the Planner interact with the Assistant? Does it replace the Assistant for complex tasks, or does the Assistant delegate to it? How does it track plan execution state across multiple agent invocations? How does it handle plan failures and replanning? Consider whether the Assistant with extended context and better models might subsume this role. |
| **Per-user resource quotas** | Deferred | In multi-user deployments, different users may need different resource limits (LLM token budgets, container quotas, session limits). Current config is global. |
| **HEARTBEAT.md parsing** | Deferred | HEARTBEAT.md uses a custom markdown format with cadence and prompt fields. No formal grammar or validation is specified. Need to define parse error behavior and multi-line prompt handling. |
| **Web UI group management** | Deferred | How are groups (rooms) created in the Web UI? What is the data flow for group creation, member management, and group discovery? §17.3 defines admin-only creation but the Web UI flow is unspecified. |

---

## Appendix A: Comparison with Reference Projects

| Feature | nanobot | nanoclaw | pi-agent | baaaht |
|---|---|---|---|---|
| **Language** | Python | TypeScript | TypeScript | Go + TypeScript |
| **Containerization** | Optional Docker | Container-first (Apple/Docker) | Optional sandbox extension | Container-first (mandatory) |
| **Security boundary** | Application-level | OS-level | Extension-based | OS-level (structural) |
| **Credential isolation** | In process memory | Mounted env vars | Auth file | LLM Gateway proxy (never in agents) |
| **Multi-user** | No | Per-group isolation | No | Per-user + per-group isolation |
| **Agent architecture** | Single agent + subagents | Single agent | Single agent + extensions | Delegation model: Assistant uses `delegate` tool to dispatch to Worker, Researcher, Coder + utility agents (memory, heartbeat) |
| **LLM providers** | 8+ (via LiteLLM) | Claude only | 15+ (custom abstraction) | 15+ (custom abstraction) |
| **Session branching** | No | No | Yes (tree JSONL) | Yes (tree JSONL) |
| **Skills** | Custom YAML format | MCP tools | Agent Skills (SKILL.md) | Agent Skills + MCP |
| **TUI** | CLI (Typer) | No | Bubbletea-equivalent (custom) | Bubbletea (Go) |
| **Scheduling** | Cron + interval | Cron + interval + one-shot | Cron + events | Cron + interval + one-shot + heartbeats (cadence-based) |
| **Web UI** | No | No | React (pi-web-ui) | React |
| **Slack** | No | No | Yes (pi-mom) | Yes (per-workspace containers) |
| **IPC** | In-process (async) | File-based | In-process (events) | gRPC (all real-time) + file-based (async) |
| **Data persistence** | In-memory + flat files | Flat files (JSON) | JSONL sessions + flat files | JSONL sessions + markdown memory + JSON state |
| **Observability** | Basic logging | Structured logs | Structured logs + metrics | Structured logs + Prometheus metrics + correlation IDs |

---

## Appendix B: Key Design Decisions Log

| Decision | Rationale |
|---|---|
| **Go for orchestrator, TypeScript for agents** | Go excels at systems-level concerns (container management, concurrency, binary distribution). TypeScript has the strongest LLM ecosystem (SDKs, MCP, Agent Skills). |
| **Credential proxy via LLM Gateway** | Prevents prompt injection from extracting API keys. Agents never see credentials. |
| **Tree-structured JSONL sessions** | Enables branching/forking without data duplication. Human-readable. Proven at scale in pi-agent. |
| **Both Agent Skills and MCP** | Agent Skills for procedural knowledge (instructions, prompts). MCP for tool-providing services. Different use cases, complementary. |
| **No WhatsApp** | Reduces scope. Slack covers the multi-user channel requirement. WhatsApp can be added as a skill/MCP later if needed. |
| **Single-instance self-hosted** | Simplifies security model (no multi-tenant concerns). Users own their data. Can scale up later if needed. |
| **Abstract memory interface** | Filesystem is the right starting point (simple, debuggable). The interface allows upgrading to SQLite/vector DB without changing agent code. |
| **gRPC as primary IPC** | Type-safe, bidirectional streaming, well-supported in both Go and TypeScript. Unix Domain Sockets (local-only socket files) avoid network exposure. Consolidating to a single real-time protocol simplifies debugging, monitoring, and security. File-based IPC retained only for async fire-and-forget operations. |
| **OIDC for Web UI auth** | Standard protocol supported by every major identity provider. Auto-user creation on first login eliminates manual user management. More portable than reverse proxy auth (doesn't require external nginx/Caddy configuration). |
| **Skills owned by users/groups, not agents** | Skills are per-user or per-group data, not agent container state. Prevents a skill installed for one user from leaking to another. Orchestrator mediates access to skill content. |
| **Unified task scheduler** | All scheduled work (heartbeats, user-requested tasks, system tasks) uses a single scheduling engine. Heartbeats are a UX layer — cadence levels are translated to cron expressions with jitter. Tasks are created via natural language through the Assistant. See §15 for full specification. |
| **No Router Agent** | The Router Agent added a classification hop without sufficient benefit. The Assistant handles all inbound messages and uses its `delegate` tool to dispatch to specialized agents. Reduces container churn and latency for common interactions. Utility agents (Memory, Heartbeat) are triggered directly by the Orchestrator, not via routing. |
| **Heartbeat as separate agent** | Heartbeat is a separate agent rather than a Worker invoked with a heartbeat prompt because: (1) it uses a cheaper model (Haiku vs Sonnet), reducing cost for frequent recurring tasks; (2) it has distinct identity documents (SOUL.md, IDENTITY.md) tailored for monitoring behavior; (3) it has a narrower tool set (no write tools) as a safety constraint; (4) separating it allows independent model selection and configuration without affecting Worker behavior. |
| **`delegate` meta-tool** | The Assistant has a single `delegate` tool rather than being fully tool-free. LLMs need a tool-call mechanism to signal delegation — structured output parsing was considered but tool-calling is more reliable and natural. The delegate tool is not a traditional tool (no file/network/exec access) — it's a dispatch primitive. Its call/response appears in context, but delegated agent internals (tool traces) do not. |
| **Session archival with Memory Agent** | 24-hour inactivity default balances cleanup with user convenience. Running a Memory Agent on archival ensures valuable user context isn't lost when sessions go cold. Configurable per deployment. |
| **Skill install from GitHub** | Pragmatic starting point — users tell the agent to install from a repo URL. Weak security but acceptable for self-hosted single-digit user deployments. Explicitly marked for future hardening. |

---

*This PRD is a living document. For the latest updates, refer to the repository.*
