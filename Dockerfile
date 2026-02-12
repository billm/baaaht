# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder
WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/orchestrator ./cmd/orchestrator

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/orchestrator /app/orchestrator

EXPOSE 50051
ENTRYPOINT ["/app/orchestrator"]
