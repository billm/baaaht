package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Extractor analyzes sessions and extracts memorable content
type Extractor struct {
	mu     sync.RWMutex
	cfg    types.MemoryExtractionConfig
	logger *logger.Logger
	store  *Store
	closed bool
}

// NewExtractor creates a new memory extractor
func NewExtractor(cfg types.MemoryExtractionConfig, store *Store, log *logger.Logger) (*Extractor, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	if store == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "memory store is required")
	}

	e := &Extractor{
		cfg:    cfg,
		logger: log.With("component", "memory_extractor"),
		store:  store,
		closed: false,
	}

	e.logger.Info("Memory extractor initialized",
		"enabled", cfg.Enabled,
		"min_confidence", cfg.MinConfidence,
		"max_memories_per_session", cfg.MaxMemoriesPerSession)

	return e, nil
}

// NewDefaultExtractor creates a new memory extractor with default configuration
func NewDefaultExtractor(store *Store, log *logger.Logger) (*Extractor, error) {
	cfg := types.MemoryExtractionConfig{
		Enabled:               true,
		MinConfidence:         0.6,
		MaxMemoriesPerSession: 10,
		Topics:                []string{},
		ExcludedTopics:        []string{},
	}
	return NewExtractor(cfg, store, log)
}

// ExtractFromSession analyzes a session and extracts memorable content
func (e *Extractor) ExtractFromSession(ctx context.Context, session *types.Session) ([]*types.Memory, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "memory extractor is closed")
	}

	if !e.cfg.Enabled {
		e.logger.Debug("Memory extraction is disabled")
		return nil, nil
	}

	if session == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "session is required")
	}

	e.logger.Info("Extracting memories from session",
		"session_id", session.ID,
		"owner_id", session.Metadata.OwnerID,
		"message_count", len(session.Context.Messages))

	// Analyze session for memorable content
	candidates := e.analyzeSession(session)

	// Filter by confidence and limits
	filtered := e.filterCandidates(candidates)

	// Store memories
	var stored []*types.Memory
	for _, mem := range filtered {
		mem.Metadata.Source = "session"
		mem.Metadata.SourceID = &session.ID

		if err := e.store.Store(ctx, mem); err != nil {
			e.logger.Error("Failed to store extracted memory",
				"memory_id", mem.ID,
				"title", mem.Title,
				"error", err)
			continue
		}

		stored = append(stored, mem)
		e.logger.Info("Memory extracted and stored",
			"memory_id", mem.ID,
			"title", mem.Title,
			"type", mem.Type,
			"confidence", mem.Metadata.Confidence)
	}

	e.logger.Info("Memory extraction complete",
		"session_id", session.ID,
		"extracted_count", len(stored),
		"candidate_count", len(candidates))

	return stored, nil
}

// ExtractFromMessage extracts memories from a single message
func (e *Extractor) ExtractFromMessage(ctx context.Context, sessionID types.ID, message types.Message, ownerID string, scope types.MemoryScope) ([]*types.Memory, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "memory extractor is closed")
	}

	if !e.cfg.Enabled {
		return nil, nil
	}

	e.logger.Debug("Extracting memories from message",
		"session_id", sessionID,
		"message_id", message.ID,
		"role", message.Role)

	// Analyze message for memorable content
	candidates := e.analyzeMessage(sessionID, message, ownerID, scope)

	// Filter by confidence and limits
	filtered := e.filterCandidates(candidates)

	// Store memories
	var stored []*types.Memory
	for _, mem := range filtered {
		mem.Metadata.Source = "session"
		mem.Metadata.SourceID = &sessionID

		if err := e.store.Store(ctx, mem); err != nil {
			e.logger.Error("Failed to store extracted memory",
				"memory_id", mem.ID,
				"title", mem.Title,
				"error", err)
			continue
		}

		stored = append(stored, mem)
	}

	return stored, nil
}

// analyzeSession analyzes a session and extracts candidate memories
func (e *Extractor) analyzeSession(session *types.Session) []*types.Memory {
	var candidates []*types.Memory

	// Extract from user preferences
	if session.Context.Preferences.Timezone != "" {
		candidates = append(candidates, &types.Memory{
			Type:    types.MemoryTypePreference,
			Scope:   types.MemoryScopeUser,
			OwnerID: session.Metadata.OwnerID,
			Topic:   "preferences",
			Title:   "User Timezone Preference",
			Content: fmt.Sprintf("User's timezone is %s", session.Context.Preferences.Timezone),
			Metadata: types.MemoryMetadata{
				Labels:     map[string]string{"category": "preferences"},
				Tags:       []string{"timezone", "preferences"},
				Importance: 3,
				Confidence: 0.9,
			},
		})
	}

	if session.Context.Preferences.Language != "" {
		candidates = append(candidates, &types.Memory{
			Type:    types.MemoryTypePreference,
			Scope:   types.MemoryScopeUser,
			OwnerID: session.Metadata.OwnerID,
			Topic:   "preferences",
			Title:   "User Language Preference",
			Content: fmt.Sprintf("User's preferred language is %s", session.Context.Preferences.Language),
			Metadata: types.MemoryMetadata{
				Labels:     map[string]string{"category": "preferences"},
				Tags:       []string{"language", "preferences"},
				Importance: 3,
				Confidence: 0.9,
			},
		})
	}

	if session.Context.Preferences.Theme != "" {
		candidates = append(candidates, &types.Memory{
			Type:    types.MemoryTypePreference,
			Scope:   types.MemoryScopeUser,
			OwnerID: session.Metadata.OwnerID,
			Topic:   "preferences",
			Title:   "User Theme Preference",
			Content: fmt.Sprintf("User's preferred theme is %s", session.Context.Preferences.Theme),
			Metadata: types.MemoryMetadata{
				Labels:     map[string]string{"category": "preferences"},
				Tags:       []string{"theme", "ui", "preferences"},
				Importance: 2,
				Confidence: 0.9,
			},
		})
	}

	// Extract from metadata
	for key, value := range session.Metadata.Labels {
		candidates = append(candidates, &types.Memory{
			Type:    types.MemoryTypeContext,
			Scope:   types.MemoryScopeUser,
			OwnerID: session.Metadata.OwnerID,
			Topic:   "metadata",
			Title:   fmt.Sprintf("Session Label: %s", key),
			Content: fmt.Sprintf("Session has label '%s' with value '%s'", key, value),
			Metadata: types.MemoryMetadata{
				Labels:     map[string]string{"label_key": key},
				Tags:       []string{"metadata", "labels"},
				Importance: 2,
				Confidence: 0.7,
			},
		})
	}

	// Extract from messages
	for _, msg := range session.Context.Messages {
		messageMemories := e.analyzeMessage(session.ID, msg, session.Metadata.OwnerID, types.MemoryScopeUser)
		candidates = append(candidates, messageMemories...)
	}

	return candidates
}

// analyzeMessage analyzes a message and extracts candidate memories
func (e *Extractor) analyzeMessage(sessionID types.ID, message types.Message, ownerID string, scope types.MemoryScope) []*types.Memory {
	var candidates []*types.Memory

	content := strings.ToLower(message.Content)

	// Skip empty or very short messages
	if len(strings.TrimSpace(message.Content)) < 10 {
		return candidates
	}

	// Check for preference patterns
	preferencePatterns := map[string]string{
		"i prefer":     "preference",
		"i like":       "preference",
		"i don't like": "preference",
		"i always":     "preference",
		"i never":      "preference",
		"i usually":    "preference",
		"i want":       "preference",
		"i need":       "preference",
	}

	for pattern, topic := range preferencePatterns {
		if strings.Contains(content, pattern) {
			candidates = append(candidates, &types.Memory{
				Type:    types.MemoryTypePreference,
				Scope:   scope,
				OwnerID: ownerID,
				Topic:   topic,
				Title:   e.generateTitle(message.Content, "Preference"),
				Content: message.Content,
				Metadata: types.MemoryMetadata{
					Labels:     map[string]string{"pattern": pattern},
					Tags:       []string{"preference", "extracted"},
					Importance: 5,
					Confidence: 0.65,
				},
			})
			break // Only extract one preference per message
		}
	}

	// Check for decision patterns
	decisionPatterns := map[string]string{
		"i decided":    "decision",
		"we decided":   "decision",
		"my decision":  "decision",
		"chosen":       "decision",
		"going with":   "decision",
		"settled on":   "decision",
		"agreed on":    "decision",
		"conclusion":   "decision",
	}

	for pattern, topic := range decisionPatterns {
		if strings.Contains(content, pattern) {
			candidates = append(candidates, &types.Memory{
				Type:    types.MemoryTypeDecision,
				Scope:   scope,
				OwnerID: ownerID,
				Topic:   topic,
				Title:   e.generateTitle(message.Content, "Decision"),
				Content: message.Content,
				Metadata: types.MemoryMetadata{
					Labels:     map[string]string{"pattern": pattern},
					Tags:       []string{"decision", "extracted"},
					Importance: 7,
					Confidence: 0.7,
				},
			})
			break
		}
	}

	// Check for factual information patterns
	factPatterns := map[string]string{
		"my name is":     "identity",
		"i work at":      "work",
		"i am a":         "identity",
		"my email is":    "contact",
		"my phone is":    "contact",
		"located in":     "location",
		"based in":       "location",
		"my project is":  "project",
		"working on":     "project",
	}

	for pattern, topic := range factPatterns {
		if strings.Contains(content, pattern) {
			candidates = append(candidates, &types.Memory{
				Type:    types.MemoryTypeFact,
				Scope:   scope,
				OwnerID: ownerID,
				Topic:   topic,
				Title:   e.generateTitle(message.Content, toTitleCase(topic)),
				Content: message.Content,
				Metadata: types.MemoryMetadata{
					Labels:     map[string]string{"pattern": pattern},
					Tags:       []string{"fact", topic, "extracted"},
					Importance: 6,
					Confidence: 0.75,
				},
			})
			break
		}
	}

	// Check for context information
	contextPatterns := map[string]string{
		"working on":     "project",
		"building":       "project",
		"developing":     "project",
		"my goal":        "goal",
		"trying to":      "goal",
		"want to achieve": "goal",
	}

	for pattern, topic := range contextPatterns {
		if strings.Contains(content, pattern) {
			candidates = append(candidates, &types.Memory{
				Type:    types.MemoryTypeContext,
				Scope:   scope,
				OwnerID: ownerID,
				Topic:   topic,
				Title:   e.generateTitle(message.Content, toTitleCase(topic)),
				Content: message.Content,
				Metadata: types.MemoryMetadata{
					Labels:     map[string]string{"pattern": pattern},
					Tags:       []string{"context", topic, "extracted"},
					Importance: 5,
					Confidence: 0.6,
				},
			})
			break
		}
	}

	return candidates
}

// filterCandidates filters candidate memories by confidence and configuration limits
func (e *Extractor) filterCandidates(candidates []*types.Memory) []*types.Memory {
	var filtered []*types.Memory

	// Filter by minimum confidence
	for _, mem := range candidates {
		if mem.Metadata.Confidence >= e.cfg.MinConfidence {
			// Filter by topic if configured
			if e.shouldIncludeTopic(mem.Topic) {
				filtered = append(filtered, mem)
			}
		}
	}

	// Sort by confidence and importance
	// For now, just limit to max memories per session
	if e.cfg.MaxMemoriesPerSession > 0 && len(filtered) > e.cfg.MaxMemoriesPerSession {
		// Simple truncation - could be improved with proper sorting
		filtered = filtered[:e.cfg.MaxMemoriesPerSession]
	}

	return filtered
}

// shouldIncludeTopic checks if a topic should be included based on configuration
func (e *Extractor) shouldIncludeTopic(topic string) bool {
	// Check excluded topics first
	for _, excluded := range e.cfg.ExcludedTopics {
		if topic == excluded {
			return false
		}
	}

	// If no specific topics configured, include all
	if len(e.cfg.Topics) == 0 {
		return true
	}

	// Check if topic is in the allowed list
	for _, allowed := range e.cfg.Topics {
		if topic == allowed {
			return true
		}
	}

	return false
}

// generateTitle generates a title from content
func (e *Extractor) generateTitle(content, suffix string) string {
	// Take first 50 characters and add suffix
	title := strings.TrimSpace(content)
	if len(title) > 50 {
		title = title[:47] + "..."
	}
	return fmt.Sprintf("%s - %s", title, suffix)
}

// Close closes the extractor
func (e *Extractor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}

	e.closed = true
	e.logger.Info("Memory extractor closed")
	return nil
}

// IsClosed returns true if the extractor is closed
func (e *Extractor) IsClosed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closed
}

// UpdateConfig updates the extractor configuration
func (e *Extractor) UpdateConfig(cfg types.MemoryExtractionConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return types.NewError(types.ErrCodeUnavailable, "memory extractor is closed")
	}

	e.cfg = cfg
	e.logger.Info("Memory extractor configuration updated",
		"enabled", cfg.Enabled,
		"min_confidence", cfg.MinConfidence,
		"max_memories_per_session", cfg.MaxMemoriesPerSession)

	return nil
}

// GetConfig returns the current extractor configuration
func (e *Extractor) GetConfig() types.MemoryExtractionConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

// toTitleCase converts a string to title case
func toTitleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
