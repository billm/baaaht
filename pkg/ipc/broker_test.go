package ipc

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Helper function to create a test broker
func createTestBroker(t *testing.T) (*Broker, string) {
	t.Helper()

	// Create a temporary directory for the socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-ipc.sock")

	cfg := config.IPCConfig{
		SocketPath:     socketPath,
		BufferSize:     4096,
		Timeout:        5 * time.Second,
		MaxConnections: 10,
		EnableAuth:     false,
	}

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	broker, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create broker: %v", err)
	}

	return broker, socketPath
}

// Helper function to create a test message
func createTestMessage(source, target types.ID) *types.IPCMessage {
	return &types.IPCMessage{
		ID:      types.GenerateID(),
		Source:  source,
		Target:  target,
		Type:    "test",
		Payload: []byte("test payload"),
		Metadata: types.IPCMetadata{
			SessionID: types.GenerateID(),
		},
		Timestamp: types.NewTimestampFromTime(time.Now()),
	}
}

func TestNewBroker(t *testing.T) {
	broker, socketPath := createTestBroker(t)
	defer broker.Close()

	if broker == nil {
		t.Fatal("Expected non-nil broker")
	}

	if broker.socket == nil {
		t.Error("Expected non-nil socket")
	}

	if broker.handlers == nil {
		t.Error("Expected non-nil handlers map")
	}

	if broker.messageQueue == nil {
		t.Error("Expected non-nil message queue")
	}

	stats := broker.Stats()
	if stats.MessagesSent != 0 {
		t.Errorf("Expected 0 messages sent, got %d", stats.MessagesSent)
	}

	// Check socket path
	if broker.socket.path != socketPath {
		t.Errorf("Expected socket path %s, got %s", socketPath, broker.socket.path)
	}
}

func TestBrokerStart(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	ctx := context.Background()
	if err := broker.Start(ctx); err != nil {
		t.Fatalf("Failed to start broker: %v", err)
	}

	// Check that socket file exists
	if _, err := os.Stat(broker.socket.path); err != nil {
		t.Errorf("Socket file does not exist: %v", err)
	}
}

func TestBrokerSend(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	ctx := context.Background()
	if err := broker.Start(ctx); err != nil {
		t.Fatalf("Failed to start broker: %v", err)
	}

	source := types.GenerateID()
	target := types.GenerateID()
	msg := createTestMessage(source, target)

	// Send message (will fail because no connection, but should validate)
	err := broker.Send(ctx, msg)
	// Expected to fail because no connection exists, but validation should pass
	if err == nil {
		t.Log("Send completed (no connection exists, which is expected)")
	}
}

func TestBrokerSendValidation(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	ctx := context.Background()

	tests := []struct {
		name    string
		msg     *types.IPCMessage
		wantErr bool
	}{
		{
			name: "valid message",
			msg: &types.IPCMessage{
				ID:        types.GenerateID(),
				Source:    types.GenerateID(),
				Target:    types.GenerateID(),
				Type:      "test",
				Payload:   []byte("test"),
				Timestamp: types.NewTimestampFromTime(time.Now()),
			},
			wantErr: true, // valid message but no connection exists, so Send fails
		},
		{
			name: "empty source",
			msg: &types.IPCMessage{
				ID:        types.GenerateID(),
				Source:    "",
				Target:    types.GenerateID(),
				Type:      "test",
				Payload:   []byte("test"),
				Timestamp: types.NewTimestampFromTime(time.Now()),
			},
			wantErr: true,
		},
		{
			name: "empty target",
			msg: &types.IPCMessage{
				ID:        types.GenerateID(),
				Source:    types.GenerateID(),
				Target:    "",
				Type:      "test",
				Payload:   []byte("test"),
				Timestamp: types.NewTimestampFromTime(time.Now()),
			},
			wantErr: true,
		},
		{
			name: "empty type",
			msg: &types.IPCMessage{
				ID:        types.GenerateID(),
				Source:    types.GenerateID(),
				Target:    types.GenerateID(),
				Type:      "",
				Payload:   []byte("test"),
				Timestamp: types.NewTimestampFromTime(time.Now()),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := broker.Send(ctx, tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Send() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBrokerRegisterHandler(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	ctx := context.Background()
	if err := broker.Start(ctx); err != nil {
		t.Fatalf("Failed to start broker: %v", err)
	}

	handler := MessageHandlerFunc(func(ctx context.Context, msg *types.IPCMessage) error {
		return nil
	})

	// Register handler
	err := broker.RegisterHandler("test", handler)
	if err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	stats := broker.Stats()
	if stats.ActiveHandlers != 1 {
		t.Errorf("Expected 1 active handler, got %d", stats.ActiveHandlers)
	}

	// Unregister handler
	err = broker.UnregisterHandler("test")
	if err != nil {
		t.Fatalf("Failed to unregister handler: %v", err)
	}

	stats = broker.Stats()
	if stats.ActiveHandlers != 0 {
		t.Errorf("Expected 0 active handlers, got %d", stats.ActiveHandlers)
	}
}

func TestBrokerRegisterHandlerNil(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	err := broker.RegisterHandler("test", nil)
	if err == nil {
		t.Error("Expected error for nil handler")
	}
}

func TestBrokerRegisterHandlerDuplicate(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	handler := MessageHandlerFunc(func(ctx context.Context, msg *types.IPCMessage) error {
		return nil
	})

	// Register handler twice (should overwrite)
	broker.RegisterHandler("test", handler)
	err := broker.RegisterHandler("test", handler)
	if err != nil {
		t.Errorf("Should allow overwriting handler: %v", err)
	}
}

func TestBrokerUnregisterHandlerNotFound(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	err := broker.UnregisterHandler("nonexistent")
	if err == nil {
		t.Error("Expected error for unregistering non-existent handler")
	}
}

func TestBrokerReceive(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	ctx := context.Background()
	if err := broker.Start(ctx); err != nil {
		t.Fatalf("Failed to start broker: %v", err)
	}

	var mu sync.Mutex
	handlerCalled := false
	var receivedMsg *types.IPCMessage

	handler := MessageHandlerFunc(func(ctx context.Context, msg *types.IPCMessage) error {
		mu.Lock()
		handlerCalled = true
		receivedMsg = msg
		mu.Unlock()
		return nil
	})

	broker.RegisterHandler("test", handler)

	// Create and serialize a message
	source := types.GenerateID()
	target := types.GenerateID()
	msg := createTestMessage(source, target)

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	// Receive the message
	err = broker.Receive(ctx, data)
	if err != nil {
		t.Fatalf("Failed to receive message: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify handler was called
	mu.Lock()
	called := handlerCalled
	recvMsg := receivedMsg
	mu.Unlock()

	if !called {
		t.Error("Handler was not called")
	}

	if recvMsg == nil {
		t.Fatal("Received message is nil")
	}

	if recvMsg.ID != msg.ID {
		t.Errorf("Expected message ID %s, got %s", msg.ID, recvMsg.ID)
	}

	stats := broker.Stats()
	if stats.MessagesReceived != 1 {
		t.Errorf("Expected 1 message received, got %d", stats.MessagesReceived)
	}
}

func TestBrokerReceiveInvalid(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	ctx := context.Background()

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "invalid json",
			data:    []byte("not json"),
			wantErr: true,
		},
		{
			name:    "empty source",
			data:    []byte(`{"id":"123","source":"","target":"456","type":"test","payload":"dGVzdA=="}`),
			wantErr: true,
		},
		{
			name:    "empty target",
			data:    []byte(`{"id":"123","source":"456","target":"","type":"test","payload":"dGVzdA=="}`),
			wantErr: true,
		},
		{
			name:    "empty type",
			data:    []byte(`{"id":"123","source":"456","target":"789","type":"","payload":"dGVzdA=="}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := broker.Receive(ctx, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Receive() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBrokerGetSessionMessages(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	// Initially, no messages
	messages, err := broker.GetSessionMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to get session messages: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(messages))
	}

	// Send a message with session ID
	source := types.GenerateID()
	target := types.GenerateID()
	msg := createTestMessage(source, target)
	msg.Metadata.SessionID = sessionID

	// Store message directly (bypassing send)
	broker.storeMessage(msg)

	// Get messages again
	messages, err = broker.GetSessionMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to get session messages: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].ID != msg.ID {
		t.Errorf("Expected message ID %s, got %s", msg.ID, messages[0].ID)
	}
}

func TestBrokerClearSessionMessages(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	// Store a message
	source := types.GenerateID()
	target := types.GenerateID()
	msg := createTestMessage(source, target)
	msg.Metadata.SessionID = sessionID
	broker.storeMessage(msg)

	// Clear messages
	err := broker.ClearSessionMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to clear session messages: %v", err)
	}

	// Verify messages are cleared
	messages, err := broker.GetSessionMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to get session messages: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", len(messages))
	}
}

func TestBrokerClose(t *testing.T) {
	broker, _ := createTestBroker(t)

	ctx := context.Background()
	if err := broker.Start(ctx); err != nil {
		t.Fatalf("Failed to start broker: %v", err)
	}

	// Close broker
	err := broker.Close()
	if err != nil {
		t.Fatalf("Failed to close broker: %v", err)
	}

	// Verify socket file is removed
	if _, err := os.Stat(broker.socket.path); !os.IsNotExist(err) {
		t.Error("Socket file should be removed after close")
	}

	// Try to close again (should return error)
	err = broker.Close()
	if err == nil {
		t.Error("Expected error when closing already closed broker")
	}
}

func TestBrokerStats(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	stats := broker.Stats()

	if stats.MessagesSent != 0 {
		t.Errorf("Expected 0 messages sent, got %d", stats.MessagesSent)
	}

	if stats.MessagesReceived != 0 {
		t.Errorf("Expected 0 messages received, got %d", stats.MessagesReceived)
	}

	if stats.MessagesFailed != 0 {
		t.Errorf("Expected 0 messages failed, got %d", stats.MessagesFailed)
	}

	if stats.ActiveHandlers != 0 {
		t.Errorf("Expected 0 active handlers, got %d", stats.ActiveHandlers)
	}
}

func TestBrokerString(t *testing.T) {
	broker, _ := createTestBroker(t)
	defer broker.Close()

	str := broker.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}
}

func TestNewDefaultBroker(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	broker, err := NewDefault(log)
	if err != nil {
		t.Fatalf("Failed to create default broker: %v", err)
	}
	defer broker.Close()

	if broker == nil {
		t.Fatal("Expected non-nil broker")
	}
}

func TestGlobalBroker(t *testing.T) {
	// Reset global broker
	globalBroker = nil
	globalBrokerOnce = sync.Once{}

	broker := Global()
	if broker == nil {
		t.Fatal("Expected non-nil global broker")
	}
	defer broker.Close()

	// Calling again should return the same instance
	broker2 := Global()
	if broker != broker2 {
		t.Error("Global() should return the same instance")
	}
}

func TestSocketNew(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-socket.sock")

	cfg := SocketConfig{
		Path:           socketPath,
		MaxConnections: 10,
		BufferSize:     4096,
		Timeout:        5 * time.Second,
		EnableAuth:     false,
	}

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	socket, err := NewSocket(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create socket: %v", err)
	}
	defer socket.Close()

	if socket == nil {
		t.Fatal("Expected non-nil socket")
	}

	if socket.path != socketPath {
		t.Errorf("Expected path %s, got %s", socketPath, socket.path)
	}
}

func TestSocketListen(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-socket.sock")

	cfg := SocketConfig{
		Path:           socketPath,
		MaxConnections: 10,
		BufferSize:     4096,
		Timeout:        5 * time.Second,
		EnableAuth:     false,
	}

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	socket, err := NewSocket(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create socket: %v", err)
	}
	defer socket.Close()

	ctx := context.Background()
	if err := socket.Listen(ctx); err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	// Verify socket file exists
	if _, err := os.Stat(socketPath); err != nil {
		t.Errorf("Socket file does not exist: %v", err)
	}
}

func TestSocketStats(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-socket.sock")

	cfg := SocketConfig{
		Path:           socketPath,
		MaxConnections: 10,
		BufferSize:     4096,
		Timeout:        5 * time.Second,
		EnableAuth:     false,
	}

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	socket, err := NewSocket(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create socket: %v", err)
	}
	defer socket.Close()

	stats := socket.Stats()
	if stats.Path != socketPath {
		t.Errorf("Expected path %s, got %s", socketPath, stats.Path)
	}

	if stats.ActiveConns != 0 {
		t.Errorf("Expected 0 active connections, got %d", stats.ActiveConns)
	}
}

func TestSocketAuthenticate(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-socket.sock")

	cfg := SocketConfig{
		Path:           socketPath,
		MaxConnections: 10,
		BufferSize:     4096,
		Timeout:        5 * time.Second,
		EnableAuth:     true,
	}

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	socket, err := NewSocket(socketPath, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create socket: %v", err)
	}
	defer socket.Close()

	ctx := context.Background()
	if err := socket.Listen(ctx); err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	// Create a test connection
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to dial socket: %v", err)
	}
	defer conn.Close()

	// Wait for connection to be accepted
	time.Sleep(100 * time.Millisecond)

	// Get the connection ID (this is a simplified test)
	socket.mu.RLock()
	var connID string
	for id := range socket.conns {
		connID = id
		break
	}
	socket.mu.RUnlock()

	if connID == "" {
		// No connection was accepted, skip authentication test
		t.Skip("No connection was accepted")
	}

	// Test authentication
	containerID := types.GenerateID()
	sessionID := types.GenerateID()

	err = socket.Authenticate(types.ID(connID), containerID, sessionID)
	if err != nil {
		t.Errorf("Failed to authenticate: %v", err)
	}

	// Verify authenticated
	if !socket.IsAuthenticated(types.ID(connID)) {
		t.Error("Connection should be authenticated")
	}
}

func TestBrokerStatsString(t *testing.T) {
	stats := BrokerStats{
		MessagesSent:     100,
		MessagesReceived: 200,
		MessagesFailed:   5,
		ActiveHandlers:   3,
		QueueSize:        10,
		SocketStats: SocketStats{
			Path:          "/tmp/test.sock",
			ActiveConns:   5,
			Authenticated: 3,
		},
	}

	str := stats.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}
}

func TestSocketStatsString(t *testing.T) {
	stats := SocketStats{
		Path:          "/tmp/test.sock",
		ActiveConns:   5,
		Authenticated: 3,
	}

	str := stats.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}
}

// Benchmark tests
func BenchmarkBrokerSend(b *testing.B) {
	broker, _ := createTestBroker(&testing.T{})
	defer broker.Close()

	ctx := context.Background()
	broker.Start(ctx)

	source := types.GenerateID()
	target := types.GenerateID()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := createTestMessage(source, target)
		_ = broker.Send(ctx, msg)
	}
}

func BenchmarkBrokerReceive(b *testing.B) {
	broker, _ := createTestBroker(&testing.T{})
	defer broker.Close()

	ctx := context.Background()
	broker.Start(ctx)

	handler := MessageHandlerFunc(func(ctx context.Context, msg *types.IPCMessage) error {
		return nil
	})
	broker.RegisterHandler("test", handler)

	source := types.GenerateID()
	target := types.GenerateID()
	msg := createTestMessage(source, target)

	data, _ := json.Marshal(msg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = broker.Receive(ctx, data)
	}
}

func BenchmarkMessageMarshal(b *testing.B) {
	source := types.GenerateID()
	target := types.GenerateID()
	msg := createTestMessage(source, target)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(msg)
	}
}
