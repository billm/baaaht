package memory

import (
	"context"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

func TestNewSessionArchivalHandler(t *testing.T) {
	log, _ := logger.NewDefault()

	// Create store and extractor
	cfg := config.MemoryConfig{
		Enabled:         true,
		UserMemoryPath:  t.TempDir() + "/user",
		GroupMemoryPath: t.TempDir() + "/group",
	}

	store, err := NewStore(cfg, log)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	extractor, err := NewDefaultExtractor(store, log)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	t.Run("creates handler with valid parameters", func(t *testing.T) {
		handler, err := NewSessionArchivalHandler(extractor, store, log)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if handler == nil {
			t.Fatal("expected handler to be non-nil")
		}

		if handler.IsClosed() {
			t.Error("expected handler to be open")
		}

		if handler.GetTimeout() != 30*time.Second {
			t.Errorf("expected default timeout of 30s, got %v", handler.GetTimeout())
		}

		_ = handler.Close()
	})

	t.Run("creates handler with custom timeout", func(t *testing.T) {
		customTimeout := 60 * time.Second
		handler, err := NewSessionArchivalHandlerWithTimeout(extractor, store, log, customTimeout)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if handler.GetTimeout() != customTimeout {
			t.Errorf("expected timeout of %v, got %v", customTimeout, handler.GetTimeout())
		}

		_ = handler.Close()
	})

	t.Run("returns error when extractor is nil", func(t *testing.T) {
		handler, err := NewSessionArchivalHandler(nil, store, log)

		if err == nil {
			t.Error("expected error when extractor is nil")
		}

		if handler != nil {
			t.Error("expected handler to be nil")
		}
	})

	t.Run("returns error when store is nil", func(t *testing.T) {
		handler, err := NewSessionArchivalHandler(extractor, nil, log)

		if err == nil {
			t.Error("expected error when store is nil")
		}

		if handler != nil {
			t.Error("expected handler to be nil")
		}
	})

	t.Run("creates default logger when nil", func(t *testing.T) {
		handler, err := NewSessionArchivalHandler(extractor, store, nil)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if handler == nil {
			t.Fatal("expected handler to be non-nil")
		}

		_ = handler.Close()
	})
}

func TestSessionArchivalHandler_CanHandle(t *testing.T) {
	log, _ := logger.NewDefault()

	cfg := config.MemoryConfig{
		Enabled:         true,
		UserMemoryPath:  t.TempDir() + "/user",
		GroupMemoryPath: t.TempDir() + "/group",
	}

	store, _ := NewStore(cfg, log)
	extractor, _ := NewDefaultExtractor(store, log)
	handler, _ := NewSessionArchivalHandler(extractor, store, log)
	defer handler.Close()

	t.Run("returns true for session archived event", func(t *testing.T) {
		canHandle := handler.CanHandle(types.EventTypeSessionArchived)

		if !canHandle {
			t.Error("expected handler to handle EventTypeSessionArchived")
		}
	})

	t.Run("returns false for other event types", func(t *testing.T) {
		eventTypes := []types.EventType{
			types.EventTypeSessionCreated,
			types.EventTypeSessionUpdated,
			types.EventTypeSessionClosed,
			types.EventTypeContainerCreated,
			types.EventTypeTaskCreated,
		}

		for _, eventType := range eventTypes {
			if handler.CanHandle(eventType) {
				t.Errorf("expected handler to not handle %s", eventType)
			}
		}
	})
}

func TestSessionArchivalHandler_Handle(t *testing.T) {
	log, _ := logger.NewDefault()

	cfg := config.MemoryConfig{
		Enabled:         true,
		UserMemoryPath:  t.TempDir() + "/user",
		GroupMemoryPath: t.TempDir() + "/group",
	}

	store, _ := NewStore(cfg, log)
	extractor, _ := NewDefaultExtractor(store, log)
	handler, _ := NewSessionArchivalHandler(extractor, store, log)
	defer handler.Close()

	t.Run("returns error for wrong event type", func(t *testing.T) {
		event := types.Event{
			ID:        types.GenerateID(),
			Type:      types.EventTypeSessionCreated,
			Timestamp: types.NewTimestamp(),
			Source:    "test",
		}

		err := handler.Handle(context.Background(), event)

		if err == nil {
			t.Error("expected error for wrong event type")
		}
	})

	t.Run("handles nil session data gracefully", func(t *testing.T) {
		event := types.Event{
			ID:        types.GenerateID(),
			Type:      types.EventTypeSessionArchived,
			Timestamp: types.NewTimestamp(),
			Source:    "test",
			Data:      map[string]interface{}{},
		}

		err := handler.Handle(context.Background(), event)

		if err != nil {
			t.Errorf("expected no error with nil session data, got %v", err)
		}
	})

	t.Run("extracts memories from session", func(t *testing.T) {
		now := time.Now()

		session := &types.Session{
			ID:     types.GenerateID(),
			State:  types.SessionStateClosed,
			Status: types.StatusStopped,
			Metadata: types.SessionMetadata{
				Name:        "Test Session",
				Description: "Test session for memory extraction",
				OwnerID:     "test-user",
				Labels: map[string]string{
					"project": "test",
					"env":     "testing",
				},
			},
			Context: types.SessionContext{
				Messages: []types.Message{
					{
						ID:        types.GenerateID(),
						Timestamp: types.Timestamp{Time: now},
						Role:      types.MessageRoleUser,
						Content:   "I prefer using dark mode for my applications",
					},
					{
						ID:        types.GenerateID(),
						Timestamp: types.Timestamp{Time: now.Add(1 * time.Minute)},
						Role:      types.MessageRoleUser,
						Content:   "I work at Acme Corp as a software engineer",
					},
				},
				Preferences: types.UserPreferences{
					Timezone:     "America/New_York",
					Language:     "en",
					Theme:        "dark",
					Notifications: true,
				},
			},
			CreatedAt: types.Timestamp{Time: now},
			UpdatedAt: types.Timestamp{Time: now},
		}

		event := types.Event{
			ID:        types.GenerateID(),
			Type:      types.EventTypeSessionArchived,
			Timestamp: types.NewTimestamp(),
			Source:    "test",
			Data: map[string]interface{}{
				"session": session,
			},
		}

		err := handler.Handle(context.Background(), event)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify memories were extracted
		filter := &types.MemoryFilter{
			Scope:   ptrTo(types.MemoryScopeUser),
			OwnerID: ptrTo("test-user"),
		}
		memories, listErr := store.List(context.Background(), filter)

		if listErr != nil {
			t.Fatalf("failed to list memories: %v", listErr)
		}

		if len(memories) == 0 {
			t.Error("expected at least one memory to be extracted")
		}
	})

	t.Run("handles session from map", func(t *testing.T) {
		now := time.Now()

		sessionData := map[string]interface{}{
			"id":     string(types.GenerateID()),
			"state":  string(types.SessionStateClosed),
			"status": string(types.StatusStopped),
			"metadata": map[string]interface{}{
				"name":        "Map Session",
				"description": "Test session from map",
				"owner_id":    "map-user",
			},
			"context": map[string]interface{}{
				"messages": []interface{}{
					map[string]interface{}{
						"id":        string(types.GenerateID()),
						"timestamp": now.Format(time.RFC3339Nano),
						"role":      string(types.MessageRoleUser),
						"content":   "I decided to use Go for this project",
					},
				},
				"preferences": map[string]interface{}{
					"timezone":     "UTC",
					"language":     "en",
					"theme":        "light",
					"notifications": false,
				},
			},
			"created_at": now.Format(time.RFC3339Nano),
			"updated_at": now.Format(time.RFC3339Nano),
		}

		event := types.Event{
			ID:        types.GenerateID(),
			Type:      types.EventTypeSessionArchived,
			Timestamp: types.NewTimestamp(),
			Source:    "test",
			Data: map[string]interface{}{
				"session": sessionData,
			},
		}

		err := handler.Handle(context.Background(), event)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestSessionArchivalHandler_Close(t *testing.T) {
	log, _ := logger.NewDefault()

	cfg := config.MemoryConfig{
		Enabled:         true,
		UserMemoryPath:  t.TempDir() + "/user",
		GroupMemoryPath: t.TempDir() + "/group",
	}

	store, _ := NewStore(cfg, log)
	extractor, _ := NewDefaultExtractor(store, log)
	handler, _ := NewSessionArchivalHandler(extractor, store, log)

	t.Run("closes handler successfully", func(t *testing.T) {
		err := handler.Close()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !handler.IsClosed() {
			t.Error("expected handler to be closed")
		}
	})

	t.Run("returns error when handling after close", func(t *testing.T) {
		session := &types.Session{
			ID:        types.GenerateID(),
			State:     types.SessionStateClosed,
			Status:    types.StatusStopped,
			CreatedAt: types.NewTimestamp(),
			UpdatedAt: types.NewTimestamp(),
		}

		event := types.Event{
			ID:        types.GenerateID(),
			Type:      types.EventTypeSessionArchived,
			Timestamp: types.NewTimestamp(),
			Source:    "test",
			Data: map[string]interface{}{
				"session": session,
			},
		}

		err := handler.Handle(context.Background(), event)

		if err == nil {
			t.Error("expected error when handling after close")
		}
	})

	t.Run("close is idempotent", func(t *testing.T) {
		err := handler.Close()

		if err != nil {
			t.Errorf("expected no error on second close, got %v", err)
		}
	})
}

func TestSessionArchivalHandler_Timeout(t *testing.T) {
	log, _ := logger.NewDefault()

	cfg := config.MemoryConfig{
		Enabled:         true,
		UserMemoryPath:  t.TempDir() + "/user",
		GroupMemoryPath: t.TempDir() + "/group",
	}

	store, _ := NewStore(cfg, log)
	extractor, _ := NewDefaultExtractor(store, log)
	handler, _ := NewSessionArchivalHandler(extractor, store, log)
	defer handler.Close()

	t.Run("gets and sets timeout", func(t *testing.T) {
		initialTimeout := handler.GetTimeout()

		newTimeout := 60 * time.Second
		err := handler.SetTimeout(newTimeout)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if handler.GetTimeout() != newTimeout {
			t.Errorf("expected timeout to be %v, got %v", newTimeout, handler.GetTimeout())
		}

		// Restore original timeout
		_ = handler.SetTimeout(initialTimeout)
	})

	t.Run("returns error for invalid timeout", func(t *testing.T) {
		err := handler.SetTimeout(0)

		if err == nil {
			t.Error("expected error for zero timeout")
		}

		err = handler.SetTimeout(-1 * time.Second)

		if err == nil {
			t.Error("expected error for negative timeout")
		}
	})
}

func TestSessionArchivalHandler_sessionFromMap(t *testing.T) {
	log, _ := logger.NewDefault()

	cfg := config.MemoryConfig{
		Enabled:         true,
		UserMemoryPath:  t.TempDir() + "/user",
		GroupMemoryPath: t.TempDir() + "/group",
	}

	store, _ := NewStore(cfg, log)
	extractor, _ := NewDefaultExtractor(store, log)
	handler, _ := NewSessionArchivalHandler(extractor, store, log)
	defer handler.Close()

	t.Run("deserializes session from map", func(t *testing.T) {
		now := time.Now()
		sessionID := types.GenerateID()

		data := map[string]interface{}{
			"id":     string(sessionID),
			"state":  string(types.SessionStateActive),
			"status": string(types.StatusRunning),
			"metadata": map[string]interface{}{
				"name":        "Test Session",
				"description": "Test Description",
				"owner_id":    "test-owner",
				"labels": map[string]interface{}{
					"key1": "value1",
					"key2": "value2",
				},
				"tags": []interface{}{"tag1", "tag2", "tag3"},
			},
			"context": map[string]interface{}{
				"messages": []interface{}{
					map[string]interface{}{
						"id":        string(types.GenerateID()),
						"timestamp": now.Format(time.RFC3339Nano),
						"role":      string(types.MessageRoleUser),
						"content":   "Test message content",
					},
				},
				"preferences": map[string]interface{}{
					"timezone":     "America/Los_Angeles",
					"language":     "en",
					"theme":        "dark",
					"notifications": true,
				},
			},
			"created_at": now.Format(time.RFC3339Nano),
			"updated_at": now.Format(time.RFC3339Nano),
		}

		// Use reflection to call private method via test helper
		session, err := handler.sessionFromMap(data)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if session.ID != sessionID {
			t.Errorf("expected ID %s, got %s", sessionID, session.ID)
		}

		if session.State != types.SessionStateActive {
			t.Errorf("expected state %s, got %s", types.SessionStateActive, session.State)
		}

		if session.Metadata.Name != "Test Session" {
			t.Errorf("expected name 'Test Session', got '%s'", session.Metadata.Name)
		}

		if session.Metadata.OwnerID != "test-owner" {
			t.Errorf("expected owner_id 'test-owner', got '%s'", session.Metadata.OwnerID)
		}

		if len(session.Metadata.Labels) != 2 {
			t.Errorf("expected 2 labels, got %d", len(session.Metadata.Labels))
		}

		if len(session.Metadata.Tags) != 3 {
			t.Errorf("expected 3 tags, got %d", len(session.Metadata.Tags))
		}

		if len(session.Context.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(session.Context.Messages))
		}

		if session.Context.Preferences.Timezone != "America/Los_Angeles" {
			t.Errorf("expected timezone 'America/Los_Angeles', got '%s'", session.Context.Preferences.Timezone)
		}
	})
}

// Helper function for pointer to value
func ptrTo[T any](v T) *T {
	return &v
}
