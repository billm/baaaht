# Baaaht Monorepo Makefile

.PHONY: all build test clean lint docker-build docker-run help build-orchestrator

# Variables
BUILD_DIR=bin
DOCKER_IMAGE=baaaht/orchestrator
DOCKER_TAG=latest

# Available tools (binaries to build)
TOOLS=orchestrator

# Go build flags
GO_FLAGS=-v

all: build

## build: Build all tool binaries
build: $(TOOLS)

## build-orchestrator: Build the orchestrator binary
orchestrator:
	@echo "Building orchestrator..."
	@mkdir -p $(BUILD_DIR)
	go build $(GO_FLAGS) -o $(BUILD_DIR)/orchestrator ./cmd/orchestrator

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
clean:
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

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
