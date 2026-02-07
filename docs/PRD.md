# PRD — baaaht

**A Secure, Container-Native, Multi-User Agentic AI Assistant Platform**

**Version:** 1.1
**Last Updated:** 2026-02-06
**Status:** Draft

> For technical architecture and system design, see [ARCHITECTURE.md](ARCHITECTURE.md).
> For phased implementation plan, see [IMPLEMENTATION.md](IMPLEMENTATION.md).

---

## 1. Problem Statement

Current self-hosted AI assistant solutions force users to choose between capability and security. Feature-rich platforms run LLM-generated code with full host access, treating security as an afterthought. Locked-down solutions sacrifice extensibility and multi-user support. No existing solution provides structural (OS-level) security isolation while supporting multiple users, multiple interaction channels, and an extensible agent/tool ecosystem.

## 2. Product Vision

**baaaht** is a self-hosted, container-native GenAI assistant platform for individuals and small teams. Every agent, tool, and gateway runs in an isolated container with minimal privileges. Security is structural — enforced at the OS level, not through application-level checks.

The platform draws on three predecessor projects:
- **nanobot** — Chat abstraction, scheduling, progressive skill loading, provider abstraction
- **nanoclaw** — Container-first isolation, per-group filesystem boundaries, file-based IPC
- **pi-agent** — Rich TUI, multi-provider LLM API (15+ providers), Agent Skills standard

### Key Differentiators

| Differentiator | Description |
|---|---|
| **Security by Isolation** | OS-level containerization as the primary security boundary |
| **Multi-Context Awareness** | 1:1 and group contexts with strict data segregation |
| **Agentic Architecture** | Assistant delegates to specialized agents, keeping its context clean |
| **Provider Agnostic** | Cloud APIs, OpenAI-compatible endpoints, and local inference — switchable via config |
| **Extensible** | Agent Skills (SKILL.md) for capability expansion; MCP support planned |
| **Proactive** | Heartbeat system for background monitoring; user-defined scheduled tasks |
| **Self-Hosted** | Single-instance deployment; user owns all data and infrastructure |

## 3. Target Users

| User | Profile | Key Use Cases |
|---|---|---|
| **Privacy-Conscious Developers** | Technical users wanting full control of data | Code assistance, research, local codebase analysis |
| **Power Users** | Comfortable with CLI; need automation | Complex workflows, scheduled tasks, proactive monitoring |
| **Small Teams** | Sharing a deployment via Slack or Web UI | Coordination, knowledge management, reporting |
| **Home Users** | Non-technical users on a managed deployment | Briefings, reminders, information retrieval |
| **Administrator** | Deploys and maintains the instance | Config, provider management, user admin, troubleshooting |

## 4. Core Capabilities

### 4.1 Conversational AI with Delegation

- Primary Assistant agent answers questions directly or delegates to specialized agents
- **Worker** — general-purpose tasks (web search, file reads, lookups)
- **Researcher** — long-running deep research with source synthesis
- **Coder** — code analysis, generation, review, and execution
- **Memory** — extracts memorable content from sessions into long-term storage
- **Heartbeat** — executes scheduled monitoring prompts
- Each agent has independent model selection, tool access, and identity

### 4.2 Multi-Channel Interaction

- **TUI** — Rich terminal interface (Go/Bubbletea) for direct interaction
- **Web UI** — Browser-based chat with session management and file upload (React)
- **Slack** — Channel and DM integration with per-workspace isolation
- All channels feed through the same Orchestrator; sessions are per-channel in MVP

### 4.3 Container-Native Security

- Every workload (agents, tools, gateways) runs in an isolated container
- Agents have no API keys — LLM calls proxied through a credential-holding gateway
- Per-user/group filesystem isolation enforced via mount policies
- No direct network access from agents or tools; web requests proxied through Orchestrator
- Read-only root filesystems, non-root execution, seccomp profiles, capability dropping

### 4.4 LLM Provider Abstraction

- Unified gateway supporting: Anthropic, OpenAI, Google, Mistral, Groq, DeepSeek, xAI, OpenRouter, Ollama, LM Studio, and any OpenAI-compatible endpoint
- Per-agent model selection (e.g., fast/cheap model for heartbeats, capable model for research)
- Automatic failover chains between providers
- Streaming support, token tracking, prompt caching

### 4.5 Extensibility

- **Agent Skills** — Markdown-based skill definitions (SKILL.md) with progressive loading
- Skills scoped per-user and per-group; installable from GitHub repos
- **MCP** (Model Context Protocol) support planned for Phase 2+
- Skill addon system for dependency management (CLI tools, packages)

### 4.6 Memory & Sessions

- Tree-structured session history supporting branching and forking
- Per-user and per-group long-term memory stores
- Automatic session archival with memory extraction after inactivity
- Human-readable persistence (JSONL sessions, markdown memory, YAML config)

### 4.7 Scheduling & Proactive Behavior

- Heartbeat system: recurring prompts at configurable cadence (hourly, daily, weekly)
- User-created scheduled tasks via natural language ("remind me at 8am", "every Monday...")
- System maintenance tasks (archival, cleanup)

### 4.8 Built-in Tools

File I/O (`read_file`, `write_file`, `edit_file`, `list_dir`, `grep`, `find`), shell execution (`exec` with safety controls), web access (`web_search`, `web_fetch` via proxy), and messaging (`message`). All tools containerized and ephemeral.

## 5. Non-Goals

- No WhatsApp integration
- No monolithic runtime — all workloads containerized; Orchestrator is the sole host process
- No agent-to-host direct filesystem access
- No agent direct internet access (proxy-only egress)
- No multi-tenant SaaS — self-hosted only
- No voice/audio interaction
- No email channel
- No native mobile app (Web UI is mobile-responsive)
- No GPU pod management (deferred)

## 6. Technical Constraints

| Constraint | Detail |
|---|---|
| **Languages** | Go (orchestrator, TUI) + TypeScript (agents, tools, gateways) |
| **Container Runtimes** | Docker (primary), Apple Containers (preferred on macOS when available) |
| **Deployment** | Single-instance, self-hosted |
| **Authentication** | OIDC for Web UI; host-trust for TUI; Slack identity for Slack |
| **Data Formats** | JSONL, YAML, JSON, Markdown — all human-readable |
| **IPC** | gRPC over Unix Domain Sockets (real-time) + file-based (async) |

## 7. Success Criteria

### Phase 1 (MVP)
- [ ] Send a message via TUI, receive a response from the Assistant
- [ ] Assistant delegates a file read to Worker, returns result to user
- [ ] All agent and tool workloads run in isolated containers
- [ ] API keys never visible to agent containers
- [ ] LLM requests work with both Anthropic and OpenAI
- [ ] Sessions persist across Orchestrator restarts
- [ ] Mount allowlist enforced — agents cannot access paths outside the allowlist
- [ ] Skills can be loaded and activated by the agent

### Phase 2
- [ ] Slack DM and channel messages trigger assistant responses
- [ ] Web UI provides chat with OIDC authentication
- [ ] Researcher and Coder agents functional with appropriate tool access
- [ ] Heartbeat system runs prompts on schedule and reports results
- [ ] Users can create scheduled tasks via natural language
- [ ] Memory Agent extracts content on session archival

### Phase 3
- [ ] Session branching and context compaction working
- [ ] Container pooling reduces tool execution latency
- [ ] Audit logging with searchable history
- [ ] SQLite/vector memory backend available

## 8. Delivery Phases

### Phase 1 — Core Platform (MVP)
Orchestrator, Assistant + Worker agents, LLM Gateway (Anthropic + OpenAI), all built-in tools (containerized), TUI, JSONL sessions, filesystem memory, YAML config, Agent Skills loader, Docker runtime, security foundations.

### Phase 2 — Multi-Channel & Multi-Agent
Slack Gateway, Web UI, Researcher/Coder/Memory/Heartbeat agents, full task scheduler (heartbeats + scheduled tasks), MCP support, skill installation from GitHub, additional LLM providers.

### Phase 3 — Advanced Security & Operations
Session branching, context compaction, container pooling, SQLite/vector memory backends, audit logging, credential rotation, configurable network policies.

---

*For architecture details, interface specifications, and implementation guidance, see [ARCHITECTURE.md](ARCHITECTURE.md).*
