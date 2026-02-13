# Baaaht Monorepo Makefile

.PHONY: all build test clean lint docker-build docker-run help build-orchestrator proto-gen proto-clean orchestrator tui \
	worker llm-gateway llm-gateway-build llm-gateway-docker llm-gateway-clean \
	agent-images-build agent-images-push assistant-image-build assistant-image-push base-image-build base-image-push

# Variables
BUILD_DIR=bin
OWNER?=billm
DOCKER_IMAGE?=ghcr.io/$(OWNER)/baaaht/orchestrator
DOCKER_TAG?=sha-$(shell git rev-parse --short HEAD)
LLM_GATEWAY_IMAGE?=ghcr.io/$(OWNER)/baaaht/llm-gateway
LLM_GATEWAY_TAG?=sha-$(shell git rev-parse --short HEAD)
LLM_GATEWAY_DIR=llm-gateway
AGENT_BASE_IMAGE?=ghcr.io/$(OWNER)/baaaht/agent-base
ASSISTANT_IMAGE?=ghcr.io/$(OWNER)/baaaht/agent-assistant
AGENT_IMAGE_TAG?=sha-$(shell git rev-parse --short HEAD)
AGENT_IMAGE_LATEST_TAG?=latest

# Proto variables
PROTO_DIR=proto
PROTO_FILES=$(wildcard $(PROTO_DIR)/*.proto)
PROTO_GO_FILES=$(patsubst $(PROTO_DIR)/%.proto,$(PROTO_DIR)/%.pb.go,$(PROTO_FILES))
PROTO_GRPC_GO_FILES=$(patsubst $(PROTO_DIR)/%.proto,$(PROTO_DIR)/%_grpc.pb.go,$(PROTO_FILES))

# Available tools (binaries to build)
TOOLS=$(BUILD_DIR)/orchestrator $(BUILD_DIR)/tui $(BUILD_DIR)/worker

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

## worker: Build the worker binary
worker: $(BUILD_DIR)/worker

$(BUILD_DIR)/worker:
	@echo "Building worker..."
	@mkdir -p $(BUILD_DIR)
	go build $(GO_FLAGS) -o $(BUILD_DIR)/worker ./cmd/worker

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
	go test -v -short -timeout=12m ./pkg/...

## test-integration: Run integration tests
test-integration:
	@echo "Running integration tests..."
	go test -v ./tests/integration/...

## clean: Clean build artifacts
clean: proto-clean llm-gateway-clean
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
docker-build: llm-gateway-docker
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f Dockerfile .

## docker-run: Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run --rm -it $(DOCKER_IMAGE):$(DOCKER_TAG)

## llm-gateway: Build the LLM Gateway TypeScript service
llm-gateway: llm-gateway-build

## llm-gateway-build: Install deps and compile the LLM Gateway
llm-gateway-build:
	@echo "Building LLM Gateway..."
	cd $(LLM_GATEWAY_DIR) && npm ci && npx tsc

## llm-gateway-docker: Build the LLM Gateway Docker image
llm-gateway-docker:
	@echo "Building LLM Gateway Docker image..."
	docker build -t $(LLM_GATEWAY_IMAGE):$(LLM_GATEWAY_TAG) $(LLM_GATEWAY_DIR)

## base-image-build: Build the shared hardened agent base image
base-image-build:
	@echo "Building shared agent base image..."
	docker build -f agents/base/Dockerfile \
		-t $(AGENT_BASE_IMAGE):$(AGENT_IMAGE_TAG) \
		-t $(AGENT_BASE_IMAGE):$(AGENT_IMAGE_LATEST_TAG) .

## assistant-image-build: Build assistant image from the shared base image
assistant-image-build: base-image-build
	@echo "Building assistant image..."
	docker build -f agents/assistant/Dockerfile \
		--build-arg BASE_IMAGE=$(AGENT_BASE_IMAGE):$(AGENT_IMAGE_TAG) \
		-t $(ASSISTANT_IMAGE):$(AGENT_IMAGE_TAG) \
		-t $(ASSISTANT_IMAGE):$(AGENT_IMAGE_LATEST_TAG) agents/assistant

## agent-images-build: Build base and assistant images with deterministic tags
agent-images-build: assistant-image-build

## base-image-push: Push shared agent base image
base-image-push: base-image-build
	@echo "Pushing shared agent base image..."
	docker push $(AGENT_BASE_IMAGE):$(AGENT_IMAGE_TAG)
	docker push $(AGENT_BASE_IMAGE):$(AGENT_IMAGE_LATEST_TAG)

## assistant-image-push: Push assistant image
assistant-image-push: assistant-image-build
	@echo "Pushing assistant image..."
	docker push $(ASSISTANT_IMAGE):$(AGENT_IMAGE_TAG)
	docker push $(ASSISTANT_IMAGE):$(AGENT_IMAGE_LATEST_TAG)

## agent-images-push: Push base and assistant images
agent-images-push: base-image-push assistant-image-push

## llm-gateway-clean: Remove LLM Gateway build artifacts
llm-gateway-clean:
	@echo "Cleaning LLM Gateway..."
	rm -rf $(LLM_GATEWAY_DIR)/dist $(LLM_GATEWAY_DIR)/node_modules

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download

## proto-gen: Generate Go code from Protocol Buffer definitions
proto-gen: $(PROTO_GO_FILES) $(PROTO_GRPC_GO_FILES)

$(PROTO_DIR)/%.pb.go: $(PROTO_DIR)/%.proto
	@echo "Generating $<..."
	@cd $(PROTO_DIR) && protoc -I. --go_out=. --go_opt=paths=source_relative $(notdir $<)

$(PROTO_DIR)/%_grpc.pb.go: $(PROTO_DIR)/%.proto
	@echo "Generating gRPC stubs for $<..."
	@cd $(PROTO_DIR) && protoc -I. --go-grpc_out=. --go-grpc_opt=paths=source_relative $(notdir $<)

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
