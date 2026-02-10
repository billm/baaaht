# Baaaht Monorepo Makefile

.PHONY: all build test clean lint docker-build docker-run help build-orchestrator proto-gen proto-clean orchestrator tui

# Variables
BUILD_DIR=bin
DOCKER_IMAGE=baaaht/orchestrator
DOCKER_TAG=latest

# Proto variables
PROTO_DIR=proto
PROTO_FILES=$(wildcard $(PROTO_DIR)/*.proto)
PROTO_GO_FILES=$(patsubst $(PROTO_DIR)/%.proto,$(PROTO_DIR)/%.pb.go,$(PROTO_FILES))
PROTO_GRPC_GO_FILES=$(patsubst $(PROTO_DIR)/%.proto,$(PROTO_DIR)/%_grpc.pb.go,$(PROTO_FILES))

# Available tools (binaries to build)
TOOLS=$(BUILD_DIR)/orchestrator $(BUILD_DIR)/tui

# Go build flags
GO_FLAGS=-v

all: build

## build: Build all tool binaries
build: $(TOOLS)

## build-orchestrator: Build the orchestrator binary
orchestrator: $(BUILD_DIR)/orchestrator

$(BUILD_DIR)/orchestrator: $(PROTO_GO_FILES) $(PROTO_GRPC_GO_FILES)
	@echo "Building orchestrator..."
	@mkdir -p $(BUILD_DIR)
	go build $(GO_FLAGS) -o $(BUILD_DIR)/orchestrator ./cmd/orchestrator

## build-tui: Build the TUI binary
tui: $(BUILD_DIR)/tui

$(BUILD_DIR)/tui: $(PROTO_GO_FILES) $(PROTO_GRPC_GO_FILES)
	@echo "Building TUI..."
	@mkdir -p $(BUILD_DIR)
	go build $(GO_FLAGS) -o $(BUILD_DIR)/tui ./cmd/tui

## test: Run all tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## test-unit: Run unit tests only
test-unit:
	@echo "Running unit tests..."
	go test -v ./pkg/...

## test-integration: Run integration tests
test-integration:
	@echo "Running integration tests..."
	go test -v ./tests/integration/...

## clean: Clean build artifacts
clean: proto-clean
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	go clean

## lint: Run linters
lint:
	@echo "Running linters..."
	go vet ./...
	staticcheck ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

## mod-tidy: Tidy Go modules
mod-tidy:
	@echo "Tidying modules..."
	go mod tidy
	go mod verify

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f Dockerfile .

## docker-run: Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run --rm -it $(DOCKER_IMAGE):$(DOCKER_TAG)

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download

## proto-gen: Generate Go code from Protocol Buffer definitions
proto-gen: $(PROTO_GO_FILES) $(PROTO_GRPC_GO_FILES)

$(PROTO_DIR)/%.pb.go: $(PROTO_DIR)/%.proto
	@echo "Generating $<..."
	@protoc --go_out=. --go_opt=paths=source_relative $<

$(PROTO_DIR)/%_grpc.pb.go: $(PROTO_DIR)/%.proto
	@echo "Generating gRPC stubs for $<..."
	@protoc --go-grpc_out=. --go-grpc_opt=paths=source_relative $<

## proto-clean: Remove generated Protocol Buffer Go files
proto-clean:
	@echo "Cleaning generated proto files..."
	@rm -f $(PROTO_DIR)/*.pb.go

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
