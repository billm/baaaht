package tui

import (
	"context"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc/connectivity"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/proto"
)

// TestOrchestratorConnection verifies the TUI can connect to the orchestrator
// This is an integration test that requires the orchestrator to be running
func TestOrchestratorConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a logger
	logConfig := config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}
	log, err := logger.New(logConfig)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create a new orchestrator client
	cfg := ClientConfig{
		DialTimeout:          30 * time.Second,
		RPCTimeout:           10 * time.Second,
		MaxRecvMsgSize:       100 * 1024 * 1024, // 100 MB
		MaxSendMsgSize:       100 * 1024 * 1024, // 100 MB
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 0, // infinite reconnect attempts
	}

	client, err := NewOrchestratorClient("/tmp/baaaht-grpc.sock", cfg, log)
	if err != nil {
		t.Fatalf("Failed to create orchestrator client: %v", err)
	}
	defer client.Close()

	// Try to connect
	if _, err := os.Stat("/tmp/baaaht-grpc.sock"); err != nil {
		t.Skipf("Skipping integration test: orchestrator socket unavailable: %v", err)
	}
	if err := client.Dial(ctx); err != nil {
		t.Skipf("Skipping integration test: failed to connect to orchestrator: %v", err)
	}

	// Verify connection state
	state := client.GetState()
	if state != connectivity.Ready {
		t.Errorf("Expected connection state to be Ready, got %v", state)
	}

	// Test health check
	_, err = client.HealthCheck(ctx)
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}

	// Test creating a session
	createReq := &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name: "test-tui-session",
			Labels: map[string]string{
				"client": "tui-integration-test",
				"test":   "true",
			},
		},
	}
	createResp, err := client.CreateSession(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if createResp.SessionId == "" {
		t.Error("Session ID is empty")
	}

	// Test getting session status
	status, err := client.GetStatus(ctx)
	if err != nil {
		t.Errorf("Failed to get status: %v", err)
	}

	if status == nil {
		t.Error("Status is nil")
	}

	// Test closing the session
	closeReq := &proto.CloseSessionRequest{
		SessionId: createResp.SessionId,
	}
	_, err = client.CloseSession(ctx, closeReq)
	if err != nil {
		t.Errorf("Failed to close session: %v", err)
	}

	// Get stats
	stats := client.Stats()
	if !stats.IsConnected {
		t.Error("Client should still be connected after closing session")
	}

	t.Logf("Connection test successful. Session ID: %s, Stats: %+v", createResp.SessionId, stats)
}
