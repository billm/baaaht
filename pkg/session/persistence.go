package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Constants for session persistence
const (
	// DefaultFilePermissions is the default permissions for session files
	DefaultFilePermissions os.FileMode = 0600
	// DefaultDirPermissions is the default permissions for session directories
	DefaultDirPermissions os.FileMode = 0700
	// SessionFileExtension is the file extension for session files
	SessionFileExtension = ".jsonl"
	// LockFileExtension is the file extension for lock files
	LockFileExtension = ".lock"
)

// Store manages session persistence with JSONL format and file locking
type Store struct {
	mu     sync.RWMutex
	cfg    config.SessionConfig
	logger *logger.Logger
	closed bool
}

// NewStore creates a new session persistence store with the specified configuration
func NewStore(cfg config.SessionConfig, log *logger.Logger) (*Store, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Ensure the storage directory exists
	if err := os.MkdirAll(cfg.StoragePath, DefaultDirPermissions); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create sessions directory", err)
	}

	store := &Store{
		cfg:    cfg,
		logger: log.With("component", "session_persistence"),
		closed: false,
	}

	store.logger.Info("Session persistence store initialized",
		"path", cfg.StoragePath,
		"persistence_enabled", cfg.PersistenceEnabled)

	return store, nil
}

// NewDefaultStore creates a new session persistence store with default configuration
func NewDefaultStore(log *logger.Logger) (*Store, error) {
	cfg := config.DefaultSessionConfig()
	return NewStore(cfg, log)
}

// Close closes the persistence store and releases resources
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	s.logger.Info("Session persistence store closed")
	return nil
}

// IsClosed returns true if the store is closed
func (s *Store) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// getSessionFilePath returns the file path for a given session ID and owner
func (s *Store) getSessionFilePath(ownerID, sessionID string) string {
	// Create per-user directory
	userDir := filepath.Join(s.cfg.StoragePath, ownerID)
	return filepath.Join(userDir, string(sessionID)+SessionFileExtension)
}

// getLockFilePath returns the lock file path for a given session file
func (s *Store) getLockFilePath(sessionFile string) string {
	return sessionFile + LockFileExtension
}

// ensureUserDir ensures the per-user directory exists
func (s *Store) ensureUserDir(ownerID string) error {
	userDir := filepath.Join(s.cfg.StoragePath, ownerID)
	if err := os.MkdirAll(userDir, DefaultDirPermissions); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create user directory", err)
	}
	return nil
}

// PersistedMessage represents a message as persisted in JSONL format
type PersistedMessage struct {
	ID        string              `json:"id"`
	Timestamp time.Time           `json:"timestamp"`
	Role      types.MessageRole   `json:"role"`
	Content   string              `json:"content"`
	Metadata  types.MessageMetadata `json:"metadata,omitempty"`
}

// toPersistedMessage converts a types.Message to a PersistedMessage
func toPersistedMessage(msg types.Message) PersistedMessage {
	return PersistedMessage{
		ID:        msg.ID.String(),
		Timestamp: msg.Timestamp.Time,
		Role:      msg.Role,
		Content:   msg.Content,
		Metadata:  msg.Metadata,
	}
}

// toMessage converts a PersistedMessage to a types.Message
func toMessage(msg PersistedMessage) types.Message {
	return types.Message{
		ID:        types.NewID(msg.ID),
		Timestamp: types.NewTimestampFromTime(msg.Timestamp),
		Role:      msg.Role,
		Content:   msg.Content,
		Metadata:  msg.Metadata,
	}
}

// marshalMessage marshals a message to JSONL format
func (s *Store) marshalMessage(msg PersistedMessage) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to marshal message", err)
	}
	// Add newline for JSONL format
	data = append(data, '\n')
	return data, nil
}

// unmarshalMessage unmarshals a message from JSONL format
func (s *Store) unmarshalMessage(data []byte) (PersistedMessage, error) {
	var msg PersistedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return PersistedMessage{}, types.WrapError(types.ErrCodeInternal, "failed to unmarshal message", err)
	}
	return msg, nil
}

// AppendMessage appends a message to the session file with atomic write
func (s *Store) AppendMessage(ctx context.Context, ownerID, sessionID string, msg types.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "persistence store is closed")
	}

	if !s.cfg.PersistenceEnabled {
		return nil // Silently skip if persistence is disabled
	}

	// Ensure user directory exists
	if err := s.ensureUserDir(ownerID); err != nil {
		return err
	}

	sessionFile := s.getSessionFilePath(ownerID, sessionID)

	// Convert message to persisted format
	persistedMsg := toPersistedMessage(msg)

	// Marshal message to JSONL
	data, err := s.marshalMessage(persistedMsg)
	if err != nil {
		return err
	}

	// Atomic write: create temp file, copy existing content, append new message, then rename
	tmpPath := sessionFile + ".tmp"

	// Open temp file for writing
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, DefaultFilePermissions)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create temp file", err)
	}

	// Copy existing content if file exists
	if _, err := os.Stat(sessionFile); err == nil {
		srcFile, err := os.Open(sessionFile)
		if err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return types.WrapError(types.ErrCodeInternal, "failed to open session file", err)
		}

		if _, err := tmpFile.ReadFrom(srcFile); err != nil {
			srcFile.Close()
			tmpFile.Close()
			os.Remove(tmpPath)
			return types.WrapError(types.ErrCodeInternal, "failed to copy existing content", err)
		}
		srcFile.Close()
	}

	// Append new message
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return types.WrapError(types.ErrCodeInternal, "failed to write message", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return types.WrapError(types.ErrCodeInternal, "failed to close temp file", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, sessionFile); err != nil {
		os.Remove(tmpPath)
		return types.WrapError(types.ErrCodeInternal, "failed to rename session file", err)
	}

	s.logger.Debug("Message appended to session", "owner_id", ownerID, "session_id", sessionID, "message_id", msg.ID.String())
	return nil
}

// LoadMessages loads all messages from a session file
func (s *Store) LoadMessages(ctx context.Context, ownerID, sessionID string) ([]types.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "persistence store is closed")
	}

	if !s.cfg.PersistenceEnabled {
		// Return empty list if persistence is disabled
		return []types.Message{}, nil
	}

	sessionFile := s.getSessionFilePath(ownerID, sessionID)

	// Read the file
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			// No file exists, return empty list
			return []types.Message{}, nil
		}
		return nil, types.WrapError(types.ErrCodeInternal, "failed to read session file", err)
	}

	// Parse JSONL format - each line is a JSON object
	messages := []types.Message{}
	lines := splitLines(data)

	for _, line := range lines {
		if len(line) == 0 {
			continue // Skip empty lines
		}

		// Unmarshal the persisted message
		persistedMsg, err := s.unmarshalMessage(line)
		if err != nil {
			s.logger.Warn("Failed to unmarshal message, skipping", "error", err)
			continue // Skip corrupted lines instead of failing entirely
		}

		// Convert to types.Message and append
		msg := toMessage(persistedMsg)
		messages = append(messages, msg)
	}

	s.logger.Debug("Loaded messages from session", "owner_id", ownerID, "session_id", sessionID, "count", len(messages))
	return messages, nil
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

// IsEnabled returns true if persistence is enabled
func (s *Store) IsEnabled() bool {
	return s.cfg.PersistenceEnabled
}

// Config returns the store configuration
func (s *Store) Config() config.SessionConfig {
	return s.cfg
}

// Stats returns statistics about the persistence store
func (s *Store) Stats(ctx context.Context) (*StoreStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "persistence store is closed")
	}

	// Count total session files
	totalSessions := 0
	totalMessages := 0

	err := filepath.Walk(s.cfg.StoragePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == SessionFileExtension {
			totalSessions++
			// Count lines in the file
			count, err := s.countLines(path)
			if err == nil {
				totalMessages += count
			}
		}
		return nil
	})

	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to scan storage directory", err)
	}

	return &StoreStats{
		TotalSessions:     totalSessions,
		TotalMessages:     totalMessages,
		StoragePath:       s.cfg.StoragePath,
		PersistenceEnabled: s.cfg.PersistenceEnabled,
	}, nil
}

// countLines counts the number of lines in a file
func (s *Store) countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	return count, nil
}

// StoreStats represents statistics about the persistence store
type StoreStats struct {
	TotalSessions      int    `json:"total_sessions"`
	TotalMessages      int    `json:"total_messages"`
	StoragePath        string `json:"storage_path"`
	PersistenceEnabled bool   `json:"persistence_enabled"`
}

// String returns a string representation of the stats
func (s *StoreStats) String() string {
	return fmt.Sprintf("StoreStats{TotalSessions: %d, TotalMessages: %d, StoragePath: %s, Enabled: %v}",
		s.TotalSessions, s.TotalMessages, s.StoragePath, s.PersistenceEnabled)
}

// Global store instance
var (
	globalStore     *Store
	storeGlobalOnce sync.Once
)

// InitGlobal initializes the global session persistence store
func InitGlobal(cfg config.SessionConfig, log *logger.Logger) error {
	var initErr error
	storeGlobalOnce.Do(func() {
		store, err := NewStore(cfg, log)
		if err != nil {
			initErr = err
			return
		}
		globalStore = store
	})
	return initErr
}

// Global returns the global persistence store instance
func Global() *Store {
	if globalStore == nil {
		// Initialize with default settings if not already initialized
		store, err := NewDefaultStore(nil)
		if err != nil {
			// Return a closed store on error, but cache it as the singleton
			globalStore = &Store{closed: true}
			return globalStore
		}
		globalStore = store
	}
	return globalStore
}

// SetGlobal sets the global persistence store instance
func SetGlobal(s *Store) {
	globalStore = s
	storeGlobalOnce = sync.Once{}
}
