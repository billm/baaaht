name: docker-build-all
description: Build all baaaht Docker images (orchestrator, worker, LLM gateway, agents)
disable-model-invocation: true

# Docker Build All

Build all Docker images for the baaaht platform in the correct order.

## Usage

```
/docker-build-all
```

## What it does

Builds images in dependency order:
1. Agent base image
2. LLM Gateway
3. Assistant agent
4. Worker agent
5. Orchestrator

## Commands

```bash
# Build all images
make agent-images-build

# Or build individually:
make base-image-build          # Agent base
make llm-gateway-docker        # LLM Gateway
make assistant-image-build     # Assistant
make worker-image-build        # Worker

# Build orchestrator (Go binary, optional container)
make docker-build
```

## Environment Variables

- `OWNER` - Docker registry owner (default: billm)
- `DOCKER_TAG` - Orchestrator image tag
- `AGENT_IMAGE_TAG` - Agent image tag

## Examples

```bash
# Build with custom owner
OWNER=myorg /docker-build-all

# Build for push to GHCR
make agent-images-build agent-images-push
```
