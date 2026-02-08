package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// TestLoadMessages tests loading messages from a session file
func TestLoadMessages(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "session456"

	// First, append some messages
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

	for i, msg := range messages {
		if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
			t.Fatalf("failed to append message %d: %v", i, err)
		}
	}

	// Load messages
	loaded, err := store.LoadMessages(ctx, ownerID, sessionID)
	if err != nil {
		t.Fatalf("failed to load messages: %v", err)
	}

	if len(loaded) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(loaded))
	}

	// Verify each message
	for i, loadedMsg := range loaded {
		if loadedMsg.ID.String() != messages[i].ID.String() {
			t.Errorf("message %d: expected ID %s, got %s", i, messages[i].ID.String(), loadedMsg.ID.String())
		}
		if loadedMsg.Content != messages[i].Content {
			t.Errorf("message %d: expected content %s, got %s", i, messages[i].Content, loadedMsg.Content)
		}
		if loadedMsg.Role != messages[i].Role {
			t.Errorf("message %d: expected role %s, got %s", i, messages[i].Role, loadedMsg.Role)
		}
	}
}

// TestLoadMessagesEmptyFile tests loading from a non-existent file
func TestLoadMessagesEmptyFile(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "nonexistent"

	// Load from non-existent file
	loaded, err := store.LoadMessages(ctx, ownerID, sessionID)
	if err != nil {
		t.Fatalf("expected no error for non-existent file, got %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("expected 0 messages for non-existent file, got %d", len(loaded))
	}
}

// TestLoadMessagesWhenClosed tests loading when store is closed
func TestLoadMessagesWhenClosed(t *testing.T) {
	store := createTestStore(t)

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "session456"

	// Close the store
	if err := store.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	// Try to load - should fail
	if _, err := store.LoadMessages(ctx, ownerID, sessionID); err == nil {
		t.Error("expected error when loading from closed store, got nil")
	}
}

// TestLoadMessagesWhenDisabled tests loading when persistence is disabled
func TestLoadMessagesWhenDisabled(t *testing.T) {
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

	// Load should return empty list when persistence is disabled
	loaded, err := store.LoadMessages(ctx, ownerID, sessionID)
	if err != nil {
		t.Errorf("expected no error when persistence is disabled, got %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("expected 0 messages when persistence is disabled, got %d", len(loaded))
	}
}

// TestLoadMessagesWithCorruptedFile tests loading with corrupted data in file
func TestLoadMessagesWithCorruptedFile(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "session456"

	// Append some valid messages first
	validMsg := types.Message{
		ID:        types.GenerateID(),
		Timestamp: types.NewTimestamp(),
		Role:      types.MessageRoleUser,
		Content:   "Valid message",
	}

	if err := store.AppendMessage(ctx, ownerID, sessionID, validMsg); err != nil {
		t.Fatalf("failed to append message: %v", err)
	}

	// Manually append some corrupted data to the file
	sessionFile := store.getSessionFilePath(ownerID, sessionID)
	file, err := os.OpenFile(sessionFile, os.O_APPEND|os.O_WRONLY, DefaultFilePermissions)
	if err != nil {
		t.Fatalf("failed to open session file: %v", err)
	}
	if _, err := file.Write([]byte("this is not valid json\n")); err != nil {
		file.Close()
		t.Fatalf("failed to write corrupted data: %v", err)
	}
	file.Close()

	// Load messages - should skip the corrupted line and return only the valid one
	loaded, err := store.LoadMessages(ctx, ownerID, sessionID)
	if err != nil {
		t.Fatalf("expected no error when loading file with corrupted data, got %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("expected 1 valid message, got %d", len(loaded))
	}

	if len(loaded) > 0 && loaded[0].Content != validMsg.Content {
		t.Errorf("expected content %s, got %s", validMsg.Content, loaded[0].Content)
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

// TestConcurrentWrites tests concurrent writes to the same session file
func TestConcurrentWrites(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "session456"

	// Number of concurrent goroutines
	numGoroutines := 10
	messagesPerGoroutine := 5

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)

	// Start multiple goroutines writing to the same session
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < messagesPerGoroutine; j++ {
				msg := types.Message{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestamp(),
					Role:      types.MessageRoleUser,
					Content:   fmt.Sprintf("Message from goroutine %d, message %d", goroutineID, j),
				}

				if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
					errCh <- fmt.Errorf("goroutine %d: %w", goroutineID, err)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errCh)

	// Check for any errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		t.Fatalf("encountered %d errors during concurrent writes: %v", len(errors), errors[0])
	}

	// Verify all messages were written correctly
	loaded, err := store.LoadMessages(ctx, ownerID, sessionID)
	if err != nil {
		t.Fatalf("failed to load messages: %v", err)
	}

	expectedCount := numGoroutines * messagesPerGoroutine
	if len(loaded) != expectedCount {
		t.Errorf("expected %d messages, got %d", expectedCount, len(loaded))
	}

	// Verify no duplicate message IDs
	idMap := make(map[string]bool)
	for _, msg := range loaded {
		id := msg.ID.String()
		if idMap[id] {
			t.Errorf("duplicate message ID found: %s", id)
		}
		idMap[id] = true
	}
}

// TestConcurrentReadsAndWrites tests concurrent reads and writes to the same session file
func TestConcurrentReadsAndWrites(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"
	sessionID := "session456"

	// Start with some initial messages
	for i := 0; i < 5; i++ {
		msg := types.Message{
			ID:        types.GenerateID(),
			Timestamp: types.NewTimestamp(),
			Role:      types.MessageRoleUser,
			Content:   fmt.Sprintf("Initial message %d", i),
		}
		if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
			t.Fatalf("failed to append initial message: %v", err)
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			msg := types.Message{
				ID:        types.GenerateID(),
				Timestamp: types.NewTimestamp(),
				Role:      types.MessageRoleUser,
				Content:   fmt.Sprintf("Concurrent message %d", i),
			}
			if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
				select {
				case errCh <- fmt.Errorf("writer: %w", err):
				default:
				}
				return
			}
		}
	}()

	// Reader goroutines
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, err := store.LoadMessages(ctx, ownerID, sessionID)
				if err != nil {
					select {
					case errCh <- fmt.Errorf("reader %d: %w", readerID, err):
					default:
					}
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Check for any errors
	for err := range errCh {
		t.Fatalf("concurrent operation error: %v", err)
	}

	// Verify final state
	loaded, err := store.LoadMessages(ctx, ownerID, sessionID)
	if err != nil {
		t.Fatalf("failed to load final messages: %v", err)
	}

	expectedCount := 5 + 20 // 5 initial + 20 concurrent writes
	if len(loaded) != expectedCount {
		t.Errorf("expected %d messages, got %d", expectedCount, len(loaded))
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

// TestListSessions tests listing sessions for a user
func TestListSessions(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"

	// Initially, no sessions should exist
	sessions, err := store.ListSessions(ctx, ownerID)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}

	// Create some sessions by appending messages
	sessionIDs := []string{"session1", "session2", "session3"}
	for _, sessionID := range sessionIDs {
		msg := types.Message{
			ID:        types.GenerateID(),
			Timestamp: types.NewTimestamp(),
			Role:      types.MessageRoleUser,
			Content:   fmt.Sprintf("Message for %s", sessionID),
		}
		if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
			t.Fatalf("failed to append message for session %s: %v", sessionID, err)
		}
	}

	// List sessions again
	sessions, err = store.ListSessions(ctx, ownerID)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != len(sessionIDs) {
		t.Errorf("expected %d sessions, got %d", len(sessionIDs), len(sessions))
	}

	// Verify all session IDs are present
	sessionMap := make(map[string]bool)
	for _, sessionID := range sessions {
		sessionMap[sessionID] = true
	}
	for _, expectedID := range sessionIDs {
		if !sessionMap[expectedID] {
			t.Errorf("expected session ID %s not found in list", expectedID)
		}
	}
}

// TestListSessionsWhenClosed tests listing when store is closed
func TestListSessionsWhenClosed(t *testing.T) {
	store := createTestStore(t)

	ctx := context.Background()
	ownerID := "user123"

	// Close the store
	if err := store.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	// Try to list - should fail
	if _, err := store.ListSessions(ctx, ownerID); err == nil {
		t.Error("expected error when listing from closed store, got nil")
	}
}

// TestListSessionsWhenDisabled tests listing when persistence is disabled
func TestListSessionsWhenDisabled(t *testing.T) {
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

	// List should return empty list when persistence is disabled
	sessions, err := store.ListSessions(ctx, ownerID)
	if err != nil {
		t.Errorf("expected no error when persistence is disabled, got %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions when persistence is disabled, got %d", len(sessions))
	}
}

// TestListSessionsMultipleUsers tests listing sessions for multiple users
func TestListSessionsMultipleUsers(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create sessions for multiple users
	users := map[string][]string{
		"user1": {"session1", "session2"},
		"user2": {"session3", "session4", "session5"},
		"user3": {"session6"},
	}

	for ownerID, sessionIDs := range users {
		for _, sessionID := range sessionIDs {
			msg := types.Message{
				ID:        types.GenerateID(),
				Timestamp: types.NewTimestamp(),
				Role:      types.MessageRoleUser,
				Content:   fmt.Sprintf("Message for %s/%s", ownerID, sessionID),
			}
			if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
				t.Fatalf("failed to append message for %s/%s: %v", ownerID, sessionID, err)
			}
		}
	}

	// Verify each user's sessions
	for ownerID, expectedSessions := range users {
		sessions, err := store.ListSessions(ctx, ownerID)
		if err != nil {
			t.Fatalf("failed to list sessions for %s: %v", ownerID, err)
		}

		if len(sessions) != len(expectedSessions) {
			t.Errorf("user %s: expected %d sessions, got %d", ownerID, len(expectedSessions), len(sessions))
		}

		// Verify session IDs
		sessionMap := make(map[string]bool)
		for _, sessionID := range sessions {
			sessionMap[sessionID] = true
		}
		for _, expectedID := range expectedSessions {
			if !sessionMap[expectedID] {
				t.Errorf("user %s: expected session ID %s not found", ownerID, expectedID)
			}
		}
	}

	// Verify non-existent user returns empty list
	sessions, err := store.ListSessions(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("failed to list sessions for nonexistent user: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for nonexistent user, got %d", len(sessions))
	}
}

// TestListSessionsWithNonSessionFiles tests listing sessions when directory contains non-session files
func TestListSessionsWithNonSessionFiles(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	ownerID := "user123"

	// Create some session files
	sessionIDs := []string{"session1", "session2"}
	for _, sessionID := range sessionIDs {
		msg := types.Message{
			ID:        types.GenerateID(),
			Timestamp: types.NewTimestamp(),
			Role:      types.MessageRoleUser,
			Content:   "Test message",
		}
		if err := store.AppendMessage(ctx, ownerID, sessionID, msg); err != nil {
			t.Fatalf("failed to append message: %v", err)
		}
	}

	// Create some non-session files in the user directory
	userDir := filepath.Join(store.cfg.StoragePath, ownerID)
	otherFiles := []string{"README.txt", "data.json", ".hidden", "backup"}
	for _, filename := range otherFiles {
		path := filepath.Join(userDir, filename)
		if err := os.WriteFile(path, []byte("test content"), DefaultFilePermissions); err != nil {
			t.Fatalf("failed to create test file %s: %v", filename, err)
		}
	}

	// List sessions - should only return .jsonl files
	sessions, err := store.ListSessions(ctx, ownerID)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != len(sessionIDs) {
		t.Errorf("expected %d sessions, got %d", len(sessionIDs), len(sessions))
	}

	// Verify only session files are returned
	for _, sessionID := range sessions {
		if sessionID != "session1" && sessionID != "session2" {
			t.Errorf("unexpected session ID returned: %s", sessionID)
		}
	}
}

// TestListSessionsEmptyOwnerID tests listing sessions with empty owner ID
func TestListSessionsEmptyOwnerID(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Empty owner ID should work (returns sessions for empty string owner)
	sessions, err := store.ListSessions(ctx, "")
	if err != nil {
		t.Fatalf("failed to list sessions with empty owner ID: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for empty owner ID, got %d", len(sessions))
	}
}
