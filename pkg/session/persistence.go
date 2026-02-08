package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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

// fileLock represents an acquired file lock
type fileLock struct {
	file *os.File
	path string
}

// Constants for file locking
const (
	// DefaultLockTimeout is the default timeout for acquiring a lock
	DefaultLockTimeout = 30 * time.Second
	// LockRetryInterval is the interval between lock acquisition retries
	LockRetryInterval = 100 * time.Millisecond
)

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

// acquireLock attempts to acquire an exclusive file lock with retry logic
func (s *Store) acquireLock(lockPath string) (*fileLock, error) {
	return s.acquireLockWithMode(lockPath, syscall.LOCK_EX)
}

// acquireReadLock attempts to acquire a shared file lock with retry logic
func (s *Store) acquireReadLock(lockPath string) (*fileLock, error) {
	return s.acquireLockWithMode(lockPath, syscall.LOCK_SH)
}

// acquireLockWithMode attempts to acquire a file lock with the specified mode and retry logic
func (s *Store) acquireLockWithMode(lockPath string, lockMode int) (*fileLock, error) {
	// Ensure the parent directory exists for the lock file
	lockDir := filepath.Dir(lockPath)
	if err := os.MkdirAll(lockDir, DefaultDirPermissions); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create lock directory", err)
	}

	// Create lock file
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, DefaultFilePermissions)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create lock file", err)
	}

	// Try to acquire lock with timeout
	timeout := time.After(DefaultLockTimeout)
	ticker := time.NewTicker(LockRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			lockFile.Close()
			lockType := "exclusive"
			if lockMode&syscall.LOCK_SH != 0 {
				lockType = "shared"
			}
			errMsg := fmt.Sprintf("timeout after %s waiting to acquire %s file lock on %s", DefaultLockTimeout, lockType, lockPath)
			return nil, types.NewError(types.ErrCodeUnavailable, errMsg)
		case <-ticker.C:
			// Try to acquire flock (exclusive or shared)
			err := syscall.Flock(int(lockFile.Fd()), lockMode|syscall.LOCK_NB)
			if err == nil {
				// Lock acquired successfully
				return &fileLock{
					file: lockFile,
					path: lockPath,
				}, nil
			}
			// Lock is held by another process, continue retrying
		}
	}
}

// releaseLock releases a file lock
func (s *Store) releaseLock(lock *fileLock) error {
	if lock == nil || lock.file == nil {
		return nil
	}

	// Release the flock
	if err := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN); err != nil {
		s.logger.Warn("Failed to release file lock", "path", lock.path, "error", err)
	}

	// Close the file
	if err := lock.file.Close(); err != nil {
		s.logger.Warn("Failed to close lock file", "path", lock.path, "error", err)
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

// AppendMessage appends a message to the session file with atomic write and file locking
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
	lockPath := s.getLockFilePath(sessionFile)

	// Acquire file lock for concurrent write safety
	lock, err := s.acquireLock(lockPath)
	if err != nil {
		return err
	}
	defer s.releaseLock(lock)

	// Convert message to persisted format
	persistedMsg := toPersistedMessage(msg)

	// Marshal message to JSONL
	data, err := s.marshalMessage(persistedMsg)
	if err != nil {
		return err
	}

	// Append message directly to the session file using O_APPEND under the existing lock
	f, err := os.OpenFile(sessionFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, DefaultFilePermissions)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to open session file for append", err)
	}

	// Write new message
	if _, err := f.Write(data); err != nil {
		f.Close()
		return types.WrapError(types.ErrCodeInternal, "failed to write message", err)
	}

	// Ensure data is flushed to disk to maintain durability guarantees
	if err := f.Sync(); err != nil {
		f.Close()
		return types.WrapError(types.ErrCodeInternal, "failed to sync session file", err)
	}

	if err := f.Close(); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to close session file", err)
	}

	s.logger.Debug("Message appended to session", "owner_id", ownerID, "session_id", sessionID, "message_id", msg.ID.String())
	return nil
}

// LoadMessages loads all messages from a session file with file locking
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
	lockPath := s.getLockFilePath(sessionFile)

	// Acquire shared file lock for concurrent read safety
	lock, err := s.acquireReadLock(lockPath)
	if err != nil {
		// If lock acquisition fails, return error with context
		return nil, types.WrapError(types.ErrCodeUnavailable,
			fmt.Sprintf("failed to acquire read lock for session file at %s", sessionFile), err)
	}
	defer s.releaseLock(lock)

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

// ListSessions lists all session IDs for a given owner
func (s *Store) ListSessions(ctx context.Context, ownerID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "persistence store is closed")
	}

	if !s.cfg.PersistenceEnabled {
		// Return empty list if persistence is disabled
		return []string{}, nil
	}

	userDir := filepath.Join(s.cfg.StoragePath, ownerID)

	// Check if user directory exists
	info, err := os.Stat(userDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No directory for this user, return empty list
			return []string{}, nil
		}
		return nil, types.WrapError(types.ErrCodeInternal, "failed to access user directory", err)
	}

	if !info.IsDir() {
		return nil, types.NewError(types.ErrCodeInternal, "user path is not a directory")
	}

	// Read directory entries
	entries, err := os.ReadDir(userDir)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to read user directory", err)
	}

	// Collect session IDs from .jsonl files
	sessions := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Check if file has .jsonl extension
		if filepath.Ext(name) == SessionFileExtension {
			// Remove extension to get session ID
			sessionID := strings.TrimSuffix(name, SessionFileExtension)
			sessions = append(sessions, sessionID)
		}
	}

	s.logger.Debug("Listed sessions for owner", "owner_id", ownerID, "count", len(sessions))
	return sessions, nil
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
