# Assistant Agent

Primary conversational agent that handles user interactions directly and delegates tool-requiring work to specialized agents (Worker, Researcher, Coder).

## Overview

The Assistant Agent is the user's primary interface to the baaaht platform. It maintains clean conversation context by delegating all tool-requiring operations to specialized agents:

- **Worker Agent**: File operations, web searches, and basic tasks
- **Researcher Agent**: Long-running deep research with source synthesis (Phase 2)
- **Coder Agent**: Code analysis, generation, review, and execution (Phase 2)

## Architecture

The Assistant has no direct tool access—its only tool is `delegate` for dispatching workloads to specialized agents. This design keeps the Assistant's context clean and focused on conversation, making it more efficient and reducing token costs.

### Communication

- **Orchestrator**: gRPC over Unix Domain Sockets for registration, heartbeat, and message streaming
- **LLM Gateway**: HTTP/HTTPS for LLM API communication with streaming support
- **Specialized Agents**: Delegation via Orchestrator's agent-to-agent messaging

### Session Management

The Assistant maintains session context across multiple delegations, ensuring conversation history is preserved even when work is dispatched to other agents.

## Development

### Prerequisites

- Node.js 22+
- npm or yarn
- Protocol Buffers compiler (protoc) with TypeScript plugin

### Installation

```bash
npm install
```

### Building

```bash
npm run build
```

### Running Locally

```bash
# Development mode with ts-node
npm run dev

# Production mode
npm run build
npm start
```

### Testing

```bash
# All tests
npm test

# Unit tests only
npm run test:unit

# Integration tests only
npm run test:integration
```

### Type Checking

```bash
npm run typecheck
```

### Linting

```bash
npm run lint
npm run lint:fix
```

## Project Structure

```
agents/assistant/
├── src/
│   ├── agent.ts              # Main Agent class
│   ├── bootstrap.ts          # Agent bootstrap and initialization
│   ├── lifecycle.ts          # Lifecycle management (start/stop)
│   ├── shutdown.ts           # Graceful shutdown handler
│   ├── orchestrator/         # gRPC client for Orchestrator communication
│   │   ├── grpc-client.ts    # Base gRPC client wrapper
│   │   ├── registration.ts   # Agent registration logic
│   │   └── stream-client.ts  # Bidirectional streaming client
│   ├── llm/                  # LLM Gateway client
│   │   ├── gateway-client.ts # HTTP client with streaming support
│   │   ├── stream-parser.ts  # SSE chunk parser
│   │   └── types.ts          # LLM-related types
│   ├── session/              # Session context management
│   │   ├── manager.ts        # Session state manager
│   │   ├── context-window.ts # Context window management
│   │   └── types.ts          # Session-related types
│   ├── tools/                # Tool implementations
│   │   ├── delegate.ts       # Delegate tool interface
│   │   ├── worker-delegation.ts    # Worker delegation logic
│   │   ├── researcher-delegation.ts # Researcher delegation (Phase 2)
│   │   └── coder-delegation.ts      # Coder delegation (Phase 2)
│   └── proto/                # Generated TypeScript types from protobuf
│       ├── agent.ts
│       ├── llm.ts
│       └── common.ts
├── tests/
│   ├── unit/                 # Unit tests
│   └── integration/          # Integration tests
└── package.json
```

## Configuration

The Assistant Agent is configured via environment variables:

- `ORCHESTRATOR_ADDRESS`: Orchestrator gRPC address (default: `unix:///tmp/orchestrator.sock`)
- `LLM_GATEWAY_URL`: LLM Gateway URL (default: `http://localhost:8080`)
- `AGENT_NAME`: Agent name (default: `assistant`)
- `LOG_LEVEL`: Logging level (default: `info`)

## License

MIT

## Copyright

2026 baaaht project
