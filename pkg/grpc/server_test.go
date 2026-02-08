package grpc

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// testHealthServer is a simple health check server for testing
type testHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
}

func (s *testHealthServer) Check(ctx context.Context, in *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

func (s *testHealthServer) Watch(in *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	return nil
}

func TestNewServer(t *testing.T) {
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

	// Create server config
	cfg := ServerConfig{
		Path:              socketPath,
		MaxRecvMsgSize:    DefaultMaxRecvMsgSize,
		MaxSendMsgSize:    DefaultMaxSendMsgSize,
		ConnectionTimeout: DefaultConnectionTimeout,
		ShutdownTimeout:   DefaultShutdownTimeout,
	}

	// Create server
	server, err := NewServer(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Verify server state
	if server.SocketPath() != socketPath {
		t.Errorf("Expected socket path %s, got %s", socketPath, server.SocketPath())
	}

	if server.IsServing() {
		t.Error("Expected IsServing to be false before Start")
	}

	t.Logf("Server created successfully: %s", server.String())
}

func TestServerStartStop(t *testing.T) {
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

	// Create server
	cfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Verify server is serving
	if !server.IsServing() {
		t.Error("Expected IsServing to be true after Start")
	}

	// Verify socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("Socket file does not exist after Start")
	}

	// Try to start again (should fail)
	if err := server.Start(ctx); err == nil {
		t.Error("Expected error when starting already started server")
	}

	// Stop server
	if err := server.Stop(); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}

	// Verify server is not serving
	if server.IsServing() {
		t.Error("Expected IsServing to be false after Stop")
	}

	// Verify socket file is removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("Socket file still exists after Stop")
	}

	// Try to stop again (should fail)
	if err := server.Stop(); err == nil {
		t.Error("Expected error when stopping already stopped server")
	}
}

func TestServerWithHealthCheck(t *testing.T) {
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

	// Create server
	cfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register health service
	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	// Start server
	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create a client connection
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", addr)
	}

	conn, err := grpc.Dial(socketPath,
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Call health check
	healthClient := grpc_health_v1.NewHealthClient(conn)
	resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("Expected SERVING status, got %v", resp.Status)
	}

	t.Log("Health check successful")
}

func TestServerConcurrentConnections(t *testing.T) {
	// Create a temporary socket path with a short name
	// Unix domain sockets have path length limits (~108 bytes)
	tmpDir := os.TempDir()
	socketPath := filepath.Join(tmpDir, fmt.Sprintf("grpc_test_%d.sock", time.Now().UnixNano()))
	// Clean up socket after test
	defer os.Remove(socketPath)

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create server
	cfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register health service
	healthServer := &testHealthServer{}
	grpc_health_v1.RegisterHealthServer(server.GetServer(), healthServer)

	// Start server
	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create multiple concurrent client connections
	numConns := 10
	errs := make(chan error, numConns)

	for i := 0; i < numConns; i++ {
		go func(idx int) {
			dialer := func(ctx context.Context, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", addr)
			}

			conn, err := grpc.Dial(socketPath,
				grpc.WithContextDialer(dialer),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
			if err != nil {
				errs <- fmt.Errorf("conn %d: failed to connect: %w", idx, err)
				return
			}
			defer conn.Close()

			// Call health check
			healthClient := grpc_health_v1.NewHealthClient(conn)
			_, err = healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
			if err != nil {
				errs <- fmt.Errorf("conn %d: health check failed: %w", idx, err)
				return
			}

			errs <- nil
		}(i)
	}

	// Wait for all connections to complete
	for i := 0; i < numConns; i++ {
		if err := <-errs; err != nil {
			t.Errorf("Connection error: %v", err)
		}
	}

	t.Logf("Successfully handled %d concurrent connections", numConns)
}

func TestServerStats(t *testing.T) {
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

	// Create server
	cfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	server, err := NewServer(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Get initial stats
	stats := server.Stats()
	if !stats.StartTime.IsZero() {
		t.Error("Expected start time to be zero before Start")
	}

	// Start server
	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Get stats after start
	stats = server.Stats()
	if stats.StartTime.IsZero() {
		t.Error("Expected start time to be set after Start")
	}
	if !stats.IsServing {
		t.Error("Expected IsServing to be true after Start")
	}

	t.Logf("Server stats: %+v", stats)
}

func TestServerRemovesExistingSocket(t *testing.T) {
	// Create a temporary socket path with a short name
	// Unix domain sockets have path length limits (~108 bytes)
	tmpDir := os.TempDir()
	socketPath := filepath.Join(tmpDir, fmt.Sprintf("grpc_sock_%d.sock", time.Now().UnixNano()))
	// Clean up socket after test
	defer os.Remove(socketPath)

	// Create an existing socket file
	file, err := os.Create(socketPath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	file.Close()

	// Verify file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("Test file should exist before creating server")
	}

	// Create logger
	logCfg := config.DefaultLoggingConfig()
	logCfg.Level = "error"
	log, err := logger.New(logCfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create server (should remove existing file)
	cfg := ServerConfig{
		Path:              socketPath,
		ConnectionTimeout: 5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	_, err = NewServer(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	ctx := context.Background()
	server, err := NewServer(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Verify socket file exists (as a socket, not regular file)
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("Socket file should exist after Start: %v", err)
	}

	if info.Mode()&os.ModeSocket == 0 {
		t.Error("Expected socket file to be a Unix domain socket")
	}

	t.Log("Server successfully replaced existing file with socket")
}
