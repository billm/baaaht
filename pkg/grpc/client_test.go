package grpc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func TestNewClient(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create client config
	cfg := ClientConfig{
		DialTimeout:        DefaultDialTimeout,
		RPCTimeout:         DefaultRPCTimeout,
		MaxRecvMsgSize:     DefaultMaxRecvMsgSize,
		MaxSendMsgSize:     DefaultMaxSendMsgSize,
		ReconnectInterval:  DefaultReconnectInterval,
		ReconnectMaxAttempts: DefaultReconnectMaxAttempts,
	}

	// Create client
	client, err := NewClient(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify client state
	if client.SocketPath() != socketPath {
		t.Errorf("Expected socket path %s, got %s", socketPath, client.SocketPath())
	}

	if client.IsConnected() {
		t.Error("Expected IsConnected to be false before Dial")
	}

	t.Logf("Client created successfully: %s", client.String())
}

func TestClientDial(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create and start server
	serverCfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, serverCfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create client
	clientCfg := ClientConfig{
		DialTimeout: 5 * time.Second,
	}
	client, err := NewClient(socketPath, clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Dial the server
	if err := client.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer client.Close()

	// Verify client is connected
	if !client.IsConnected() {
		t.Error("Expected IsConnected to be true after Dial")
	}

	// Check connection state
	if client.GetState() != connectivity.Ready {
		t.Errorf("Expected state Ready, got %v", client.GetState())
	}

	t.Log("Client connected successfully")
}

func TestClientDialAlreadyConnected(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create and start server
	serverCfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, serverCfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create client
	clientCfg := ClientConfig{
		DialTimeout: 5 * time.Second,
	}
	client, err := NewClient(socketPath, clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// First dial
	if err := client.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	// Try to dial again (should fail)
	if err := client.Dial(ctx); err == nil {
		t.Error("Expected error when dialing already connected client")
	}

	t.Log("Correctly rejected duplicate dial")
}

func TestClientDialClosed(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create client
	clientCfg := ClientConfig{
		DialTimeout: 5 * time.Second,
	}
	client, err := NewClient(socketPath, clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Close the client
	if err := client.Close(); err != nil {
		t.Fatalf("Failed to close client: %v", err)
	}

	// Try to dial (should fail)
	ctx := context.Background()
	if err := client.Dial(ctx); err == nil {
		t.Error("Expected error when dialing closed client")
	}

	t.Log("Correctly rejected dial on closed client")
}

func TestClientClose(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create and start server
	serverCfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, serverCfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create client
	clientCfg := ClientConfig{
		DialTimeout: 5 * time.Second,
	}
	client, err := NewClient(socketPath, clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Dial the server
	if err := client.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	// Verify connected
	if !client.IsConnected() {
		t.Error("Expected IsConnected to be true before Close")
	}

	// Close the client
	if err := client.Close(); err != nil {
		t.Fatalf("Failed to close client: %v", err)
	}

	// Verify not connected
	if client.IsConnected() {
		t.Error("Expected IsConnected to be false after Close")
	}

	// Try to close again (should fail)
	if err := client.Close(); err == nil {
		t.Error("Expected error when closing already closed client")
	}

	t.Log("Client closed successfully")
}

func TestClientHealthCheck(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create and start server
	serverCfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, serverCfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create client
	clientCfg := ClientConfig{
		DialTimeout: 5 * time.Second,
	}
	client, err := NewClient(socketPath, clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Dial the server
	if err := client.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer client.Close()

	// Perform health check
	resp, err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("Expected SERVING status, got %v", resp.Status)
	}

	// Check stats
	stats := client.Stats()
	if stats.TotalRPCs != 1 {
		t.Errorf("Expected 1 RPC, got %d", stats.TotalRPCs)
	}

	t.Log("Health check successful")
}

func TestClientHealthCheckNotConnected(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create client (not connected)
	clientCfg := ClientConfig{
		DialTimeout: 5 * time.Second,
	}
	client, err := NewClient(socketPath, clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Try health check (should fail)
	ctx := context.Background()
	_, err = client.HealthCheck(ctx)
	if err == nil {
		t.Error("Expected error when health checking while not connected")
	}

	t.Log("Correctly rejected health check while not connected")
}

func TestClientStats(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create client
	clientCfg := ClientConfig{
		DialTimeout: 5 * time.Second,
	}
	client, err := NewClient(socketPath, clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Get initial stats
	stats := client.Stats()
	if !stats.ConnectTime.IsZero() {
		t.Error("Expected connect time to be zero before Dial")
	}
	if stats.IsConnected {
		t.Error("Expected IsConnected to be false before Dial")
	}

	// Create and start server
	serverCfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, serverCfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Dial the server
	if err := client.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer client.Close()

	// Get stats after dial
	stats = client.Stats()
	if stats.ConnectTime.IsZero() {
		t.Error("Expected connect time to be set after Dial")
	}
	if !stats.IsConnected {
		t.Error("Expected IsConnected to be true after Dial")
	}

	t.Logf("Client stats: %+v", stats)
}

func TestClientResetConnection(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create and start server
	serverCfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, serverCfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create client
	clientCfg := ClientConfig{
		DialTimeout: 5 * time.Second,
	}
	client, err := NewClient(socketPath, clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Dial the server
	if err := client.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	initialConnectTime := client.Stats().ConnectTime

	// Reset connection
	if err := client.ResetConnection(ctx); err != nil {
		t.Fatalf("Failed to reset connection: %v", err)
	}
	defer client.Close()

	// Verify reconnected
	if !client.IsConnected() {
		t.Error("Expected IsConnected to be true after reset")
	}

	// Verify connect time changed
	newConnectTime := client.Stats().ConnectTime
	if !newConnectTime.After(initialConnectTime) {
		t.Error("Expected connect time to change after reset")
	}

	t.Log("Connection reset successfully")
}

func TestNewClientConn(t *testing.T) {
	// Create a temporary socket path with a short name
	tmpDir := os.TempDir()
	socketPath := filepath.Join(tmpDir, fmt.Sprintf("grpc_test_%d.sock", time.Now().UnixNano()))
	defer os.Remove(socketPath)

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create and start server
	serverCfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, serverCfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create client connection using convenience function
	conn, err := NewClientConn(socketPath)
	if err != nil {
		t.Fatalf("Failed to create client connection: %v", err)
	}
	defer conn.Close()

	// Verify connection works
	healthClient := grpc_health_v1.NewHealthClient(conn)
	resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("Expected SERVING status, got %v", resp.Status)
	}

	t.Log("Convenience function created connection successfully")
}

func TestClientWaitForStateChange(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create and start server
	serverCfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, serverCfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create client
	clientCfg := ClientConfig{
		DialTimeout: 5 * time.Second,
	}
	client, err := NewClient(socketPath, clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Get initial state
	initialState := client.GetState()
	if initialState != connectivity.Idle {
		t.Errorf("Expected Idle state, got %v", initialState)
	}

	// Dial the server
	if err := client.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer client.Close()

	// Verify state changed to Ready
	if client.GetState() != connectivity.Ready {
		t.Errorf("Expected Ready state, got %v", client.GetState())
	}

	t.Log("State changed correctly from Idle to Ready")
}

func TestClientDefaultConfig(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create client with empty config (should use defaults)
	client, err := NewClient(socketPath, ClientConfig{}, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Verify defaults are applied
	if client.dialTimeout != DefaultDialTimeout {
		t.Errorf("Expected dial timeout %v, got %v", DefaultDialTimeout, client.dialTimeout)
	}
	if client.rpcTimeout != DefaultRPCTimeout {
		t.Errorf("Expected RPC timeout %v, got %v", DefaultRPCTimeout, client.rpcTimeout)
	}
	if client.maxRecvMsgSize != DefaultMaxRecvMsgSize {
		t.Errorf("Expected max recv msg size %d, got %d", DefaultMaxRecvMsgSize, client.maxRecvMsgSize)
	}
	if client.maxSendMsgSize != DefaultMaxSendMsgSize {
		t.Errorf("Expected max send msg size %d, got %d", DefaultMaxSendMsgSize, client.maxSendMsgSize)
	}

	t.Log("Default configuration applied correctly")
}
