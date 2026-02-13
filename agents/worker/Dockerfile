# syntax=docker/dockerfile:1
# Worker Agent Dockerfile for baaaht
#
# This is a minimal Alpine-based image for the Go worker binary.
# It's smaller than the assistant image since it doesn't need Node.js.
#
# Multi-stage build for cross-platform support

# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/worker ./cmd/worker
COPY pkg/ ./pkg/
COPY proto/ ./proto/
COPY internal/ ./internal/

# Build the worker binary
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /worker ./cmd/worker

# Runtime stage
FROM alpine:3.20

# Install ca-certificates for HTTPS requests and tini for init
RUN apk add --no-cache ca-certificates tini

WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /worker /app/worker
RUN chmod +x /app/worker

# Default orchestrator URL (can be overridden at runtime)
ENV ORCHESTRATOR_URL=host.docker.internal:50051

# Run as non-root user for security
USER 1000:1000

# Use tini as init system to handle signals properly
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/worker"]
