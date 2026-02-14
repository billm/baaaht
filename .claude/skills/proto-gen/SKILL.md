name: proto-gen
description: Generate Go and TypeScript code from protobuf definitions
disable-model-invocation: true

# Proto Generation

Regenerate all protobuf code (Go and TypeScript) from .proto definitions.

## Usage

```
/proto-gen
```

## What it does

1. Generates Go code from all `.proto` files in `proto/`
2. Generates TypeScript code for the assistant agent
3. Validates generated files exist

## Commands

```bash
# Generate Go protobuf files
make proto-gen

# Generate TypeScript for assistant agent
cd agents/assistant && npm run proto

# Verify generation
ls -la proto/*.pb.go proto/*_grpc.pb.go
```

## Prerequisites

- `protoc` installed
- Go protoc plugins: `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`
- Go gRPC plugins: `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`
- TypeScript plugin: `npm install` in `agents/assistant/`
