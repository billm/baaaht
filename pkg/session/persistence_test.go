package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// createTestStore creates a test session persistence store
func createTestStore(t *testing.T) *Store {
	t.Helper()

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Create a temporary directory for test sessions
	tmpDir := t.TempDir()
	cfg := config.DefaultSessionConfig()
	cfg.StoragePath = filepath.Join(tmpDir, "sessions")
	cfg.PersistenceEnabled = true

	store, err := NewStore(cfg, log)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	return store
}

// TestNewStore tests creating a new session persistence store
func TestNewStore(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	if store == nil {
		t.Fatal("store is nil")
	}

	if store.IsClosed() {
		t.Fatal("store should not be closed")
	}
}

// TestNewStoreNilLogger tests creating a store with nil logger
func TestNewStoreNilLogger(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultSessionConfig()
	cfg.StoragePath = filepath.Join(tmpDir, "sessions")

	store, err := NewStore(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create store with nil logger: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("store is nil")
	}
}

// TestNewDefaultStore tests creating a store with default config
func TestNewDefaultStore(t *testing.T) {
	store, err := NewDefaultStore(nil)
	if err != nil {
		t.Fatalf("failed to create default store: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("store is nil")
	}
}

// TestStoreClose tests closing the store
func TestStoreClose(t *testing.T) {
	store := createTestStore(t)

	if err := store.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	if !store.IsClosed() {
		t.Fatal("store should be closed after Close()")
	}

	// Closing again should be idempotent
	if err := store.Close(); err != nil {
		t.Fatalf("closing already-closed store should not error: %v", err)
	}
}

// TestIsEnabled tests checking if persistence is enabled
func TestIsEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultSessionConfig()
	cfg.StoragePath = filepath.Join(tmpDir, "sessions")

	// Test with persistence disabled
	cfg.PersistenceEnabled = false
	store, err := NewStore(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if store.IsEnabled() {
		t.Fatal("persistence should be disabled")
	}

	// Test with persistence enabled
	cfg2 := config.DefaultSessionConfig()
	cfg2.StoragePath = filepath.Join(tmpDir, "sessions2")
	cfg2.PersistenceEnabled = true
	store2, err := NewStore(cfg2, nil)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store2.Close()

	if !store2.IsEnabled() {
		t.Fatal("persistence should be enabled")
	}
}

// TestConfig tests retrieving the store configuration
func TestConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultSessionConfig()
	cfg.StoragePath = filepath.Join(tmpDir, "sessions")
	cfg.PersistenceEnabled = true

	store, err := NewStore(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	retrievedCfg := store.Config()
	if retrievedCfg.StoragePath != cfg.StoragePath {
		t.Errorf("expected storage path %s, got %s", cfg.StoragePath, retrievedCfg.StoragePath)
	}
	if retrievedCfg.PersistenceEnabled != cfg.PersistenceEnabled {
		t.Errorf("expected persistence enabled %v, got %v", cfg.PersistenceEnabled, retrievedCfg.PersistenceEnabled)
	}
}

// TestStats tests retrieving store statistics
func TestStats(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats == nil {
		t.Fatal("stats is nil")
	}

	// Initially should have 0 sessions
	if stats.TotalSessions != 0 {
		t.Errorf("expected 0 sessions, got %d", stats.TotalSessions)
	}

	if !stats.PersistenceEnabled {
		t.Error("expected persistence enabled in default config")
	}

	if stats.StoragePath == "" {
		t.Error("storage path should not be empty")
	}
}

// TestGetSessionFilePath tests the session file path generation
func TestGetSessionFilePath(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ownerID := "user123"
	sessionID := "session456"

	path := store.getSessionFilePath(ownerID, sessionID)
	expected := filepath.Join(store.cfg.StoragePath, ownerID, sessionID+SessionFileExtension)

	if path != expected {
		t.Errorf("expected path %s, got %s", expected, path)
	}
}

// TestGetLockFilePath tests the lock file path generation
func TestGetLockFilePath(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	sessionFile := "/tmp/sessions/user/session123.jsonl"
	lockPath := store.getLockFilePath(sessionFile)
	expected := sessionFile + LockFileExtension

	if lockPath != expected {
		t.Errorf("expected lock path %s, got %s", expected, lockPath)
	}
}

// TestPersistedMessageConversion tests message type conversions
func TestPersistedMessageConversion(t *testing.T) {
	// Create a types.Message
	msg := types.Message{
		ID:        types.GenerateID(),
		Timestamp: types.NewTimestamp(),
		Role:      types.MessageRoleUser,
		Content:   "Hello, world!",
		Metadata: types.MessageMetadata{
			Extra: map[string]string{
				"key": "value",
			},
		},
	}

	// Convert to PersistedMessage
	persisted := toPersistedMessage(msg)

	if persisted.ID != msg.ID.String() {
		t.Errorf("expected ID %s, got %s", msg.ID.String(), persisted.ID)
	}

	if persisted.Role != msg.Role {
		t.Errorf("expected role %s, got %s", msg.Role, persisted.Role)
	}

	if persisted.Content != msg.Content {
		t.Errorf("expected content %s, got %s", msg.Content, persisted.Content)
	}

	// Convert back to Message
	converted := toMessage(persisted)

	if converted.ID.String() != msg.ID.String() {
		t.Errorf("expected ID %s, got %s", msg.ID.String(), converted.ID.String())
	}

	if converted.Role != msg.Role {
		t.Errorf("expected role %s, got %s", msg.Role, converted.Role)
	}

	if converted.Content != msg.Content {
		t.Errorf("expected content %s, got %s", msg.Content, converted.Content)
	}
}

// TestMarshalUnmarshalMessage tests JSON marshaling and unmarshaling
func TestMarshalUnmarshalMessage(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	msg := PersistedMessage{
		ID:        "test-id-123",
		Timestamp: time.Now(),
		Role:      types.MessageRoleAssistant,
		Content:   "Test response",
		Metadata: types.MessageMetadata{
			Extra: map[string]string{
				"model": "gpt-4",
			},
		},
	}

	// Marshal
	data, err := store.marshalMessage(msg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("marshaled data is empty")
	}

	// Unmarshal
	unmarshaled, err := store.unmarshalMessage(data)
	if err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	if unmarshaled.ID != msg.ID {
		t.Errorf("expected ID %s, got %s", msg.ID, unmarshaled.ID)
	}

	if unmarshaled.Content != msg.Content {
		t.Errorf("expected content %s, got %s", msg.Content, unmarshaled.Content)
	}
}

// TestAppendMessage tests appending a message to a session file
func TestAppendMessage(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "session456"

	// Create a test message
	msg := types.Message{
		ID:        types.GenerateID(),
		Timestamp: types.NewTimestamp(),
		Role:      types.MessageRoleUser,
		Content:   "Hello, world!",
		Metadata: types.MessageMetadata{
			Extra: map[string]string{
				"key": "value",
			},
		},
	}

	// Append the message
	if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
		t.Fatalf("failed to append message: %v", err)
	}

	// Verify the file was created
	sessionFile := store.getSessionFilePath(ownerID, sessionID)
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Fatal("session file was not created")
	}

	// Read the file and verify content
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("failed to read session file: %v", err)
	}

	// Verify it's valid JSONL
	lines := countLines(data)
	if lines != 1 {
		t.Errorf("expected 1 line in file, got %d", lines)
	}

	// Unmarshal and verify message
	var persistedMsg PersistedMessage
	if err := json.Unmarshal(data, &persistedMsg); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	if persistedMsg.ID != msg.ID.String() {
		t.Errorf("expected ID %s, got %s", msg.ID.String(), persistedMsg.ID)
	}

	if persistedMsg.Content != msg.Content {
		t.Errorf("expected content %s, got %s", msg.Content, persistedMsg.Content)
	}

	if persistedMsg.Role != msg.Role {
		t.Errorf("expected role %s, got %s", msg.Role, persistedMsg.Role)
	}
}

// TestAppendMessageMultiple tests appending multiple messages
func TestAppendMessageMultiple(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "session456"

	messages := []types.Message{
		{
			ID:        types.GenerateID(),
			Timestamp: types.NewTimestamp(),
			Role:      types.MessageRoleUser,
			Content:   "First message",
		},
		{
			ID:        types.GenerateID(),
			Timestamp: types.NewTimestamp(),
			Role:      types.MessageRoleAssistant,
			Content:   "Second message",
		},
		{
			ID:        types.GenerateID(),
			Timestamp: types.NewTimestamp(),
			Role:      types.MessageRoleUser,
			Content:   "Third message",
		},
	}

	// Append all messages
	for i, msg := range messages {
		if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
			t.Fatalf("failed to append message %d: %v", i, err)
		}
	}

	// Verify file has 3 lines
	sessionFile := store.getSessionFilePath(ownerID, sessionID)
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("failed to read session file: %v", err)
	}

	lines := countLines(data)
	if lines != 3 {
		t.Errorf("expected 3 lines in file, got %d", lines)
	}

	// Verify each message
	var persistedMsg PersistedMessage
	lineNum := 0
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &persistedMsg); err != nil {
			t.Fatalf("failed to unmarshal message at line %d: %v", lineNum, err)
		}
		if persistedMsg.Content != messages[lineNum].Content {
			t.Errorf("line %d: expected content %s, got %s", lineNum, messages[lineNum].Content, persistedMsg.Content)
		}
		lineNum++
	}
}

// TestAppendMessageWhenClosed tests appending when store is closed
func TestAppendMessageWhenClosed(t *testing.T) {
	store := createTestStore(t)

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "session456"

	msg := types.Message{
		ID:        types.GenerateID(),
		Timestamp: types.NewTimestamp(),
		Role:      types.MessageRoleUser,
		Content:   "Hello",
	}

	// Close the store
	if err := store.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	// Try to append - should fail
	if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err == nil {
		t.Error("expected error when appending to closed store, got nil")
	}
}

// TestAppendMessageWhenDisabled tests appending when persistence is disabled
func TestAppendMessageWhenDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultSessionConfig()
	cfg.StoragePath = filepath.Join(tmpDir, "sessions")
	cfg.PersistenceEnabled = false

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	store, err := NewStore(cfg, log)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "session456"

	msg := types.Message{
		ID:        types.GenerateID(),
		Timestamp: types.NewTimestamp(),
		Role:      types.MessageRoleUser,
		Content:   "Hello",
	}

	// Append should succeed silently (no-op)
	if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
		t.Errorf("expected no error when persistence is disabled, got %v", err)
	}

	// Verify no file was created
	sessionFile := store.getSessionFilePath(ownerID, sessionID)
	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Error("session file should not exist when persistence is disabled")
	}
}

// TestConstants tests the defined constants
func TestConstants(t *testing.T) {
	if DefaultFilePermissions != 0600 {
		t.Errorf("expected DefaultFilePermissions 0600, got %o", DefaultFilePermissions)
	}

	if DefaultDirPermissions != 0700 {
		t.Errorf("expected DefaultDirPermissions 0700, got %o", DefaultDirPermissions)
	}

	if SessionFileExtension != ".jsonl" {
		t.Errorf("expected SessionFileExtension .jsonl, got %s", SessionFileExtension)
	}

	if LockFileExtension != ".lock" {
		t.Errorf("expected LockFileExtension .lock, got %s", LockFileExtension)
	}
}

// countLines counts the number of lines in data
func countLines(data []byte) int {
	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	return count
}

// splitLines splits data into lines
func splitLines(data []byte) [][]byte {
	lines := [][]byte{}
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
