package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionPersistence tests the full session persistence flow including:
// - Creating sessions with messages
// - Persisting to JSONL format
// - Restoring after restart
// - Adding messages after restoration
// - Human-readable JSONL format
// - Per-user directories
func TestSessionPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping persistence test in short mode")
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	// Create a temporary directory for persistence
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "sessions")

	t.Log("=== Step 1: Create session manager with persistence enabled ===")

	cfg := config.SessionConfig{
		StoragePath:        storagePath,
		PersistenceEnabled: true,
		MaxSessions:        100,
		IdleTimeout:        30 * time.Minute,
		Timeout:            24 * time.Hour,
	}

	manager1, err := session.New(cfg, log)
	require.NoError(t, err, "Failed to create session manager")
	require.NotNil(t, manager1, "Session manager should not be nil")
	defer func() {
		if !t.Failed() {
			manager1.Close()
		}
	}()

	t.Logf("Session manager created with storage path: %s", storagePath)

	// Step 2: Create a session and add messages
	t.Log("=== Step 2: Create session and add messages ===")

	sessionMetadata := types.SessionMetadata{
		Name:        "persistence-test-session",
		Description: "Testing session persistence",
		OwnerID:     "test-user-1",
		Labels: map[string]string{
			"test":        "persistence",
			"integration": "true",
		},
	}

	sessionConfig := types.SessionConfig{
		MaxContainers: 10,
		MaxDuration:   1 * time.Hour,
		IdleTimeout:   30 * time.Minute,
	}

	sessionID, err := manager1.Create(ctx, sessionMetadata, sessionConfig)
	require.NoError(t, err, "Failed to create session")
	require.False(t, sessionID.IsEmpty(), "Session ID should not be empty")

	t.Logf("Session created: %s", sessionID)

	// Add multiple messages to the session
	messages := []types.Message{
		{
			Role:    types.MessageRoleUser,
			Content: "Hello, this is a test message",
			Metadata: types.MessageMetadata{
				Extra: map[string]string{
					"source": "integration-test",
				},
			},
		},
		{
			Role:    types.MessageRoleAssistant,
			Content: "Hello! I received your message.",
			Metadata: types.MessageMetadata{
				Extra: map[string]string{
					"model": "test-model",
				},
			},
		},
		{
			Role:    types.MessageRoleUser,
			Content: "Can you help me with a task?",
		},
		{
			Role:    types.MessageRoleAssistant,
			Content: "Of course! What would you like me to help you with?",
		},
		{
			Role:    types.MessageRoleUser,
			Content: "I need to test session persistence.",
		},
	}

	for i, msg := range messages {
		err := manager1.AddMessage(ctx, sessionID, msg)
		require.NoError(t, err, "Failed to add message %d", i)
		t.Logf("Message %d added: role=%s, content=%s", i+1, msg.Role, msg.Content)
	}

	// Verify messages are in memory
	retrievedSession, err := manager1.Get(ctx, sessionID)
	require.NoError(t, err, "Failed to retrieve session")
	require.Equal(t, len(messages), len(retrievedSession.Context.Messages), "Message count should match")

	t.Logf("Session has %d messages in memory", len(retrievedSession.Context.Messages))

	// Step 3: Verify JSONL file exists and is valid
	t.Log("=== Step 3: Verify JSONL file exists and is valid ===")

	// Expected file path: <storagePath>/<ownerID>/<sessionID>.jsonl
	expectedFilePath := filepath.Join(storagePath, sessionMetadata.OwnerID, sessionID.String()+".jsonl")

	fileInfo, err := os.Stat(expectedFilePath)
	require.NoError(t, err, "JSONL file should exist")
	require.Greater(t, fileInfo.Size(), int64(0), "JSONL file should not be empty")

	t.Logf("JSONL file verified: %s (size: %d bytes)", expectedFilePath, fileInfo.Size())

	// Verify JSONL format is human-readable
	fileContent, err := os.ReadFile(expectedFilePath)
	require.NoError(t, err, "Failed to read JSONL file")

	lines := strings.Split(string(fileContent), "\n")
	// Remove empty last line if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	require.Equal(t, len(messages), len(lines), "JSONL file should have one line per message")

	t.Logf("JSONL file has %d lines (one per message)", len(lines))

	// Verify each line is valid JSON and human-readable
	// Use a local type since PersistedMessage is not exported
	type persistedMessage struct {
		ID        string    `json:"id"`
		Timestamp time.Time `json:"timestamp"`
		Role      string    `json:"role"`
		Content   string    `json:"content"`
	}

	for i, line := range lines {
		require.NotEmpty(t, line, "Line %d should not be empty", i)

		// Verify it's valid JSON
		var persistedMsg persistedMessage
		err := json.Unmarshal([]byte(line), &persistedMsg)
		require.NoError(t, err, "Line %d should be valid JSON", i)

		// Verify message content matches
		require.Equal(t, messages[i].Content, persistedMsg.Content, "Message %d content should match", i)
		require.Equal(t, string(messages[i].Role), persistedMsg.Role, "Message %d role should match", i)

		// Verify the line is human-readable (contains expected fields)
		assert.Contains(t, line, `"id":`, "Line should contain id field")
		assert.Contains(t, line, `"timestamp":`, "Line should contain timestamp field")
		assert.Contains(t, line, `"role":`, "Line should contain role field")
		assert.Contains(t, line, `"content":`, "Line should contain content field")

		t.Logf("Line %d: valid JSON with id=%s, role=%s", i+1, persistedMsg.ID, persistedMsg.Role)
	}

	// Step 4: Close and recreate session manager (simulate restart)
	t.Log("=== Step 4: Close and recreate session manager (simulate restart) ===")

	err = manager1.Close()
	require.NoError(t, err, "Failed to close session manager")
	t.Log("Session manager closed")

	// Create a new session manager with the same config
	manager2, err := session.New(cfg, log)
	require.NoError(t, err, "Failed to create second session manager")
	defer manager2.Close()

	t.Log("New session manager created")

	// Verify no sessions in memory yet (before restore)
	sessionsBeforeRestore, err := manager2.List(ctx, nil)
	require.NoError(t, err, "Failed to list sessions before restore")
	require.Equal(t, 0, len(sessionsBeforeRestore), "Should have no sessions before restore")

	t.Log("No sessions in memory before restore")

	// Step 5: Restore sessions from persistence
	t.Log("=== Step 5: Restore sessions from persistence ===")

	err = manager2.RestoreSessions(ctx)
	require.NoError(t, err, "Failed to restore sessions")

	t.Log("Sessions restored from persistence")

	// Step 6: Verify session was restored
	t.Log("=== Step 6: Verify session was restored ===")

	sessionsAfterRestore, err := manager2.List(ctx, nil)
	require.NoError(t, err, "Failed to list sessions after restore")
	require.Equal(t, 1, len(sessionsAfterRestore), "Should have 1 restored session")

	restoredSession := sessionsAfterRestore[0]
	require.Equal(t, sessionID, restoredSession.ID, "Restored session ID should match")
	require.Equal(t, sessionMetadata.OwnerID, restoredSession.Metadata.OwnerID, "Restored owner ID should match")
	require.Equal(t, types.SessionStateIdle, restoredSession.State, "Restored session should be in idle state")

	t.Logf("Session restored: id=%s, state=%s, messages=%d",
		restoredSession.ID, restoredSession.State, len(restoredSession.Context.Messages))

	// Verify all messages were restored
	require.Equal(t, len(messages), len(restoredSession.Context.Messages), "All messages should be restored")

	for i, msg := range restoredSession.Context.Messages {
		require.Equal(t, messages[i].Content, msg.Content, "Message %d content should match", i)
		require.Equal(t, messages[i].Role, msg.Role, "Message %d role should match", i)
		t.Logf("Message %d restored: role=%s, content=%s", i+1, msg.Role, msg.Content)
	}

	// Step 7: Add more messages after restoration
	t.Log("=== Step 7: Add more messages after restoration ===")

	newMessages := []types.Message{
		{
			Role:    types.MessageRoleAssistant,
			Content: "I can help you test persistence!",
		},
		{
			Role:    types.MessageRoleUser,
			Content: "Great, thank you!",
		},
	}

	for i, msg := range newMessages {
		err := manager2.AddMessage(ctx, sessionID, msg)
		require.NoError(t, err, "Failed to add new message %d", i)
		t.Logf("New message %d added after restoration: role=%s, content=%s", i+1, msg.Role, msg.Content)
	}

	// Verify messages were added and persisted
	retrievedSession2, err := manager2.Get(ctx, sessionID)
	require.NoError(t, err, "Failed to retrieve session after adding new messages")

	expectedTotalMessages := len(messages) + len(newMessages)
	require.Equal(t, expectedTotalMessages, len(retrievedSession2.Context.Messages), "Should have all messages including new ones")

	t.Logf("Session now has %d total messages", len(retrievedSession2.Context.Messages))

	// Verify JSONL file was updated with new messages
	fileContent2, err := os.ReadFile(expectedFilePath)
	require.NoError(t, err, "Failed to read updated JSONL file")

	lines2 := strings.Split(string(fileContent2), "\n")
	if len(lines2) > 0 && lines2[len(lines2)-1] == "" {
		lines2 = lines2[:len(lines2)-1]
	}

	require.Equal(t, expectedTotalMessages, len(lines2), "JSONL file should have all messages including new ones")

	t.Logf("JSONL file now has %d lines (all messages persisted)", len(lines2))

	// Step 8: Test multiple users (per-user directories)
	t.Log("=== Step 8: Test multiple users with per-user directories ===")

	// Create a session for a different user
	sessionMetadata2 := types.SessionMetadata{
		Name:        "persistence-test-session-2",
		Description: "Testing multiple users",
		OwnerID:     "test-user-2",
		Labels: map[string]string{
			"test": "multi-user",
		},
	}

	sessionID2, err := manager2.Create(ctx, sessionMetadata2, sessionConfig)
	require.NoError(t, err, "Failed to create second session")

	// Add a message to the second session
	err = manager2.AddMessage(ctx, sessionID2, types.Message{
		Role:    types.MessageRoleUser,
		Content: "Message from user 2",
	})
	require.NoError(t, err, "Failed to add message to second session")

	// Verify per-user directory structure
	userDir1 := filepath.Join(storagePath, "test-user-1")
	userDir2 := filepath.Join(storagePath, "test-user-2")

	_, err = os.Stat(userDir1)
	require.NoError(t, err, "User 1 directory should exist")

	_, err = os.Stat(userDir2)
	require.NoError(t, err, "User 2 directory should exist")

	// Verify session files are in correct directories
	expectedFile1 := filepath.Join(userDir1, sessionID.String()+".jsonl")
	expectedFile2 := filepath.Join(userDir2, sessionID2.String()+".jsonl")

	_, err = os.Stat(expectedFile1)
	require.NoError(t, err, "Session 1 file should exist in user 1 directory")

	_, err = os.Stat(expectedFile2)
	require.NoError(t, err, "Session 2 file should exist in user 2 directory")

	t.Logf("Per-user directories verified: %s and %s", userDir1, userDir2)

	// Verify total session count
	sessions, err := manager2.List(ctx, nil)
	require.NoError(t, err, "Failed to list all sessions")
	require.Equal(t, 2, len(sessions), "Should have 2 sessions")

	t.Logf("Total sessions: %d", len(sessions))

	// Test complete
	t.Log("=== Session Persistence Test Complete ===")
	t.Log("All steps passed successfully:")
	t.Log("  1. Session manager created with persistence enabled")
	t.Log("  2. Session created and messages added")
	t.Log("  3. JSONL file verified (exists, valid, human-readable)")
	t.Log("  4. Session manager closed and recreated")
	t.Log("  5. Sessions restored from persistence")
	t.Log("  6. Session and messages verified after restoration")
	t.Log("  7. New messages added and persisted after restoration")
	t.Log("  8. Per-user directories verified for multiple users")
}

// TestSessionPersistenceConcurrentAccess tests concurrent writes to the same session
func TestSessionPersistenceConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent access test in short mode")
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	// Create a temporary directory for persistence
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "sessions")

	cfg := config.SessionConfig{
		StoragePath:        storagePath,
		PersistenceEnabled: true,
		MaxSessions:        100,
	}

	manager, err := session.New(cfg, log)
	require.NoError(t, err)
	defer manager.Close()

	// Create a session
	sessionMetadata := types.SessionMetadata{
		Name:    "concurrent-test-session",
		OwnerID: "test-user-concurrent",
	}

	sessionID, err := manager.Create(ctx, sessionMetadata, types.SessionConfig{})
	require.NoError(t, err)

	// Add messages concurrently
	numGoroutines := 10
	messagesPerGoroutine := 5

	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := types.Message{
					Role:    types.MessageRoleUser,
					Content: fmt.Sprintf("Concurrent message %d-%d", index, j),
				}
				if err := manager.AddMessage(ctx, sessionID, msg); err != nil {
					errChan <- err
					return
				}
			}
			errChan <- nil
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		err := <-errChan
		require.NoError(t, err, "Goroutine should complete without error")
	}

	// Verify all messages were persisted
	retrievedSession, err := manager.Get(ctx, sessionID)
	require.NoError(t, err)

	expectedMessages := numGoroutines * messagesPerGoroutine
	require.Equal(t, expectedMessages, len(retrievedSession.Context.Messages),
		"All concurrent messages should be persisted")

	t.Logf("All %d concurrent messages persisted successfully", expectedMessages)
}

// TestSessionPersistenceDisabled tests that persistence is skipped when disabled
func TestSessionPersistenceDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping disabled persistence test in short mode")
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	// Create a temporary directory for persistence
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "sessions")

	cfg := config.SessionConfig{
		StoragePath:        storagePath,
		PersistenceEnabled: false, // Disabled
		MaxSessions:        100,
	}

	manager, err := session.New(cfg, log)
	require.NoError(t, err)
	defer manager.Close()

	// Create a session
	sessionMetadata := types.SessionMetadata{
		Name:    "disabled-persistence-test",
		OwnerID: "test-user-disabled",
	}

	sessionID, err := manager.Create(ctx, sessionMetadata, types.SessionConfig{})
	require.NoError(t, err)

	// Add a message
	err = manager.AddMessage(ctx, sessionID, types.Message{
		Role:    types.MessageRoleUser,
		Content: "This should not be persisted",
	})
	require.NoError(t, err)

	// Verify JSONL file was NOT created
	expectedFilePath := filepath.Join(storagePath, sessionMetadata.OwnerID, sessionID.String()+".jsonl")
	_, err = os.Stat(expectedFilePath)
	require.True(t, os.IsNotExist(err), "JSONL file should not exist when persistence is disabled")

	// Close and recreate manager
	manager.Close()

	manager2, err := session.New(cfg, log)
	require.NoError(t, err)
	defer manager2.Close()

	// Restore should succeed but not restore anything
	err = manager2.RestoreSessions(ctx)
	require.NoError(t, err, "Restore should succeed even with persistence disabled")

	// Verify no sessions were restored
	sessions, err := manager2.List(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, 0, len(sessions), "No sessions should be restored when persistence is disabled")

	t.Log("Persistence disabled test passed: no files created, no sessions restored")
}
