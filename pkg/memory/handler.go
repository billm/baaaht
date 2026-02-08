package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// SessionArchivalHandler handles session archival events and extracts memories
type SessionArchivalHandler struct {
	extractor *Extractor
	store     *Store
	logger    *logger.Logger
	timeout   time.Duration
	closed    bool
	closeMu   sync.Mutex
}

// NewSessionArchivalHandler creates a new session archival handler
func NewSessionArchivalHandler(extractor *Extractor, store *Store, log *logger.Logger) (*SessionArchivalHandler, error) {
	if extractor == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "extractor cannot be nil")
	}

	if store == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "store cannot be nil")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	h := &SessionArchivalHandler{
		extractor: extractor,
		store:     store,
		logger:    log.With("component", "session_archival_handler"),
		timeout:   30 * time.Second,
		closed:    false,
	}

	h.logger.Info("Session archival handler initialized")

	return h, nil
}

// NewSessionArchivalHandlerWithTimeout creates a new session archival handler with a custom timeout
func NewSessionArchivalHandlerWithTimeout(extractor *Extractor, store *Store, log *logger.Logger, timeout time.Duration) (*SessionArchivalHandler, error) {
	h, err := NewSessionArchivalHandler(extractor, store, log)
	if err != nil {
		return nil, err
	}

	if timeout > 0 {
		h.timeout = timeout
	}

	return h, nil
}

// Handle processes a session archival event and extracts memories from the session
func (h *SessionArchivalHandler) Handle(ctx context.Context, event types.Event) error {
	h.closeMu.Lock()
	if h.closed {
		h.closeMu.Unlock()
		return types.NewError(types.ErrCodeUnavailable, "handler is closed")
	}
	h.closeMu.Unlock()

	// Validate event type
	if event.Type != types.EventTypeSessionArchived {
		return types.NewError(types.ErrCodeInvalid,
			fmt.Sprintf("unexpected event type: %s, expected: %s", event.Type, types.EventTypeSessionArchived))
	}

	h.logger.Info("Handling session archival event",
		"event_id", event.ID,
		"timestamp", event.Timestamp,
		"source", event.Source)

	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	// Extract session from event data
	session, err := h.extractSessionFromEvent(event)
	if err != nil {
		return types.WrapError(types.ErrCodeInvalid, "failed to extract session from event", err)
	}

	if session == nil {
		h.logger.Warn("Session data is nil in archival event, skipping memory extraction",
			"event_id", event.ID)
		return nil
	}

	h.logger.Info("Extracting memories from archived session",
		"session_id", session.ID,
		"owner_id", session.Metadata.OwnerID,
		"message_count", len(session.Context.Messages))

	// Extract memories from the session
	memories, err := h.extractor.ExtractFromSession(ctx, session)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to extract memories from session", err)
	}

	h.logger.Info("Memory extraction complete",
		"session_id", session.ID,
		"extracted_count", len(memories))

	return nil
}

// CanHandle returns true if the handler can process the given event type
func (h *SessionArchivalHandler) CanHandle(eventType types.EventType) bool {
	return eventType == types.EventTypeSessionArchived
}

// extractSessionFromEvent extracts a session from the event data
func (h *SessionArchivalHandler) extractSessionFromEvent(event types.Event) (*types.Session, error) {
	// Check if session data is present in the event
	sessionData, ok := event.Data["session"]
	if !ok {
		h.logger.Warn("No session data found in archival event", "event_id", event.ID)
		return nil, nil
	}

	// Handle different types of session data
	switch v := sessionData.(type) {
	case *types.Session:
		return v, nil
	case types.Session:
		return &v, nil
	case map[string]interface{}:
		// Deserialize session from map
		return h.sessionFromMap(v)
	default:
		return nil, types.NewError(types.ErrCodeInvalid,
			fmt.Sprintf("unsupported session data type: %T", sessionData))
	}
}

// sessionFromMap deserializes a session from a map
func (h *SessionArchivalHandler) sessionFromMap(data map[string]interface{}) (*types.Session, error) {
	session := &types.Session{}

	// Extract ID
	if id, ok := data["id"].(string); ok {
		session.ID = types.ID(id)
	}

	// Extract State
	if state, ok := data["state"].(string); ok {
		session.State = types.SessionState(state)
	}

	// Extract Status
	if status, ok := data["status"].(string); ok {
		session.Status = types.Status(status)
	}

	// Extract timestamps
	if created, ok := data["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
			session.CreatedAt = types.Timestamp{Time: t}
		}
	}

	if updated, ok := data["updated_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, updated); err == nil {
			session.UpdatedAt = types.Timestamp{Time: t}
		}
	}

	// Extract Metadata
	if metadata, ok := data["metadata"].(map[string]interface{}); ok {
		session.Metadata = h.extractMetadata(metadata)
	}

	// Extract Context
	if context, ok := data["context"].(map[string]interface{}); ok {
		session.Context = h.extractContext(context)
	}

	return session, nil
}

// extractMetadata extracts session metadata from a map
func (h *SessionArchivalHandler) extractMetadata(data map[string]interface{}) types.SessionMetadata {
	var metadata types.SessionMetadata

	if name, ok := data["name"].(string); ok {
		metadata.Name = name
	}

	if desc, ok := data["description"].(string); ok {
		metadata.Description = desc
	}

	if ownerID, ok := data["owner_id"].(string); ok {
		metadata.OwnerID = ownerID
	}

	if labels, ok := data["labels"].(map[string]interface{}); ok {
		metadata.Labels = make(map[string]string)
		for k, v := range labels {
			if vs, ok := v.(string); ok {
				metadata.Labels[k] = vs
			}
		}
	}

	if tags, ok := data["tags"].([]interface{}); ok {
		metadata.Tags = make([]string, 0, len(tags))
		for _, tag := range tags {
			if ts, ok := tag.(string); ok {
				metadata.Tags = append(metadata.Tags, ts)
			}
		}
	}

	return metadata
}

// extractContext extracts session context from a map
func (h *SessionArchivalHandler) extractContext(data map[string]interface{}) types.SessionContext {
	var context types.SessionContext

	// Extract messages
	if messages, ok := data["messages"].([]interface{}); ok {
		context.Messages = make([]types.Message, 0, len(messages))
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				context.Messages = append(context.Messages, h.extractMessage(msgMap))
			}
		}
	}

	// Extract preferences
	if prefs, ok := data["preferences"].(map[string]interface{}); ok {
		context.Preferences = h.extractPreferences(prefs)
	}

	return context
}

// extractMessage extracts a message from a map
func (h *SessionArchivalHandler) extractMessage(data map[string]interface{}) types.Message {
	var msg types.Message

	if id, ok := data["id"].(string); ok {
		msg.ID = types.ID(id)
	}

	if timestamp, ok := data["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			msg.Timestamp = types.Timestamp{Time: t}
		}
	}

	if role, ok := data["role"].(string); ok {
		msg.Role = types.MessageRole(role)
	}

	if content, ok := data["content"].(string); ok {
		msg.Content = content
	}

	return msg
}

// extractPreferences extracts user preferences from a map
func (h *SessionArchivalHandler) extractPreferences(data map[string]interface{}) types.UserPreferences {
	var prefs types.UserPreferences

	if timezone, ok := data["timezone"].(string); ok {
		prefs.Timezone = timezone
	}

	if language, ok := data["language"].(string); ok {
		prefs.Language = language
	}

	if theme, ok := data["theme"].(string); ok {
		prefs.Theme = theme
	}

	if notifications, ok := data["notifications"].(bool); ok {
		prefs.Notifications = notifications
	}

	return prefs
}

// Close closes the handler
func (h *SessionArchivalHandler) Close() error {
	h.closeMu.Lock()
	defer h.closeMu.Unlock()

	if h.closed {
		return nil
	}

	h.closed = true
	h.logger.Info("Session archival handler closed")
	return nil
}

// IsClosed returns true if the handler is closed
func (h *SessionArchivalHandler) IsClosed() bool {
	h.closeMu.Lock()
	defer h.closeMu.Unlock()
	return h.closed
}

// GetTimeout returns the handler timeout
func (h *SessionArchivalHandler) GetTimeout() time.Duration {
	return h.timeout
}

// SetTimeout sets the handler timeout
func (h *SessionArchivalHandler) SetTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return types.NewError(types.ErrCodeInvalid, "timeout must be positive")
	}
	h.timeout = timeout
	return nil
}
