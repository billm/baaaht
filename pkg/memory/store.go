package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

const (
	bytesPerKB = 1024 // Bytes per kilobyte for size conversions
)

// Store manages memory storage with filesystem persistence
type Store struct {
	memories map[types.ID]*types.Memory
	mu       sync.RWMutex
	cfg      config.MemoryConfig
	logger   *logger.Logger
	closed   bool
}

// NewStore creates a new memory store with the specified configuration
func NewStore(cfg config.MemoryConfig, log *logger.Logger) (*Store, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Ensure the storage directories exist
	if err := os.MkdirAll(cfg.UserMemoryPath, 0755); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create user memory directory", err)
	}
	if err := os.MkdirAll(cfg.GroupMemoryPath, 0755); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create group memory directory", err)
	}

	store := &Store{
		memories: make(map[types.ID]*types.Memory),
		cfg:      cfg,
		logger:   log.With("component", "memory_store"),
		closed:   false,
	}

	// Load existing memories from disk
	if err := store.loadFromDisk(); err != nil {
		store.logger.Warn("Failed to load existing memories from disk", "error", err)
		// Continue anyway - the store will start fresh
	}

	store.logger.Info("Memory store initialized",
		"user_path", cfg.UserMemoryPath,
		"group_path", cfg.GroupMemoryPath,
		"enabled", cfg.Enabled)
	return store, nil
}

// NewDefaultStore creates a new memory store with default configuration
func NewDefaultStore(log *logger.Logger) (*Store, error) {
	cfg := config.DefaultMemoryConfig()
	return NewStore(cfg, log)
}

// Store saves a new memory or updates an existing one
func (s *Store) Store(ctx context.Context, mem *types.Memory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "memory store is closed")
	}

	if !s.cfg.Enabled {
		return types.NewError(types.ErrCodePermission, "memory storage is disabled")
	}

	if mem.ID == "" {
		mem.ID = types.GenerateID()
	}

	if mem.Title == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "memory title is required")
	}

	if mem.Scope == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "memory scope is required")
	}

	if mem.OwnerID == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "memory owner_id is required")
	}

	now := types.NewTimestamp()

	// Check if memory already exists
	existing, exists := s.memories[mem.ID]
	if exists {
		// Update existing memory - deep copy metadata to prevent external mutation
		existing.Title = mem.Title
		existing.Content = mem.Content
		existing.Topic = mem.Topic
		existing.Type = mem.Type
		
		// Deep copy the metadata fields
		if mem.Metadata.Labels != nil {
			existing.Metadata.Labels = make(map[string]string, len(mem.Metadata.Labels))
			for k, v := range mem.Metadata.Labels {
				existing.Metadata.Labels[k] = v
			}
		} else {
			existing.Metadata.Labels = nil
		}
		
		if mem.Metadata.Tags != nil {
			existing.Metadata.Tags = make([]string, len(mem.Metadata.Tags))
			copy(existing.Metadata.Tags, mem.Metadata.Tags)
		} else {
			existing.Metadata.Tags = nil
		}
		
		// Copy scalar metadata fields
		existing.Metadata.Source = mem.Metadata.Source
		existing.Metadata.SourceID = mem.Metadata.SourceID
		existing.Metadata.Importance = mem.Metadata.Importance
		existing.Metadata.Confidence = mem.Metadata.Confidence
		existing.Metadata.Verified = mem.Metadata.Verified
		existing.Metadata.Version = mem.Metadata.Version
		
		existing.UpdatedAt = now
		if mem.ExpiresAt != nil {
			existing.ExpiresAt = mem.ExpiresAt
		}

		s.logger.Info("Memory updated", "id", mem.ID, "title", mem.Title)
	} else {
		// Create new memory - deep copy to prevent external mutation
		memCopy := deepCopyMemory(mem)
		memCopy.CreatedAt = now
		memCopy.UpdatedAt = now
		s.memories[mem.ID] = memCopy
		s.logger.Info("Memory stored", "id", mem.ID, "title", mem.Title, "scope", mem.Scope)
	}

	// Persist to disk using the canonical in-memory object
	memToPersist := mem
	if exists {
		memToPersist = existing
	}
	if err := s.saveToDisk(memToPersist); err != nil {
		s.logger.Error("Failed to persist memory to disk", "error", err)
		return err
	}

	return nil
}

// Get retrieves a memory by ID
func (s *Store) Get(ctx context.Context, id types.ID) (*types.Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "memory store is closed")
	}

	mem, exists := s.memories[id]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, fmt.Sprintf("memory not found: %s", id))
	}

	// Check if memory is expired
	if s.isExpired(mem) {
		return nil, types.NewError(types.ErrCodePermission, "memory has expired")
	}

	// Update access time asynchronously
	go s.updateAccessTime(id)

	// Return a deep copy to prevent external mutation
	return deepCopyMemory(mem), nil
}

// GetByOwner retrieves all memories for a specific owner
func (s *Store) GetByOwner(ctx context.Context, scope types.MemoryScope, ownerID string) ([]*types.Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "memory store is closed")
	}

	result := make([]*types.Memory, 0)
	for _, mem := range s.memories {
		if mem.Scope == scope && mem.OwnerID == ownerID {
			if !s.isExpired(mem) {
				result = append(result, deepCopyMemory(mem))
			}
		}
	}

	return result, nil
}

// List retrieves memories matching the given filter
func (s *Store) List(ctx context.Context, filter *types.MemoryFilter) ([]*types.Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "memory store is closed")
	}

	result := make([]*types.Memory, 0)
	for _, mem := range s.memories {
		if !s.matchesFilter(mem, filter) {
			continue
		}
		if s.isExpired(mem) {
			continue
		}

		result = append(result, deepCopyMemory(mem))
	}

	return result, nil
}

// Delete removes a memory from the store
func (s *Store) Delete(ctx context.Context, id types.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "memory store is closed")
	}

	mem, exists := s.memories[id]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("memory not found: %s", id))
	}

	// Delete from filesystem
	if err := s.deleteFromDisk(mem); err != nil {
		s.logger.Error("Failed to delete memory from disk", "error", err)
		return err
	}

	delete(s.memories, id)
	s.logger.Info("Memory deleted", "id", id)

	return nil
}

// Stats returns statistics about memories for a specific owner
func (s *Store) Stats(ctx context.Context, scope types.MemoryScope, ownerID string) (*types.MemoryStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "memory store is closed")
	}

	stats := &types.MemoryStats{
		Scope:       scope,
		OwnerID:     ownerID,
		TypeCounts:  make(map[types.MemoryType]int),
		TopicCounts: make(map[string]int),
		TotalCount:  0,
		Timestamp:   types.NewTimestamp(),
	}

	for _, mem := range s.memories {
		if mem.Scope != scope || mem.OwnerID != ownerID {
			continue
		}
		if s.isExpired(mem) {
			continue
		}

		stats.TotalCount++
		stats.TypeCounts[mem.Type]++
		stats.TopicCounts[mem.Topic]++

		if mem.UpdatedAt.Time.After(stats.LastUpdated.Time) {
			stats.LastUpdated = mem.UpdatedAt
		}
	}

	return stats, nil
}

// Close closes the memory store
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	s.logger.Info("Memory store closed")
	return nil
}

// IsClosed returns true if the store is closed
func (s *Store) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// isExpired checks if a memory has expired
func (s *Store) isExpired(mem *types.Memory) bool {
	if mem.ExpiresAt == nil {
		return false
	}
	return mem.ExpiresAt.Time.Before(time.Now())
}

// updateAccessTime updates the last accessed time for a memory
func (s *Store) updateAccessTime(id types.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if mem, exists := s.memories[id]; exists {
		now := types.NewTimestamp()
		mem.AccessedAt = &now
	}
}

// matchesFilter checks if a memory matches the given filter
func (s *Store) matchesFilter(mem *types.Memory, filter *types.MemoryFilter) bool {
	if filter == nil {
		return true
	}

	if filter.Scope != nil && mem.Scope != *filter.Scope {
		return false
	}

	if filter.OwnerID != nil && mem.OwnerID != *filter.OwnerID {
		return false
	}

	if filter.Type != nil && mem.Type != *filter.Type {
		return false
	}

	if filter.Topic != nil && mem.Topic != *filter.Topic {
		return false
	}

	if filter.Verified != nil && mem.Metadata.Verified != *filter.Verified {
		return false
	}

	if filter.MinImportance != nil && mem.Metadata.Importance < *filter.MinImportance {
		return false
	}

	if len(filter.Label) > 0 {
		match := false
		for k, v := range filter.Label {
			if mem.Metadata.Labels != nil {
				if memVal, ok := mem.Metadata.Labels[k]; ok && memVal == v {
					match = true
					break
				}
			}
		}
		if !match {
			return false
		}
	}

	if len(filter.Tags) > 0 {
		// Check if memory has all required tags
		tagSet := make(map[string]bool)
		for _, tag := range mem.Metadata.Tags {
			tagSet[tag] = true
		}
		for _, tag := range filter.Tags {
			if !tagSet[tag] {
				return false
			}
		}
	}

	if filter.StartTime != nil && mem.CreatedAt.Time.Before(filter.StartTime.Time) {
		return false
	}

	if filter.EndTime != nil && mem.CreatedAt.Time.After(filter.EndTime.Time) {
		return false
	}

	return true
}

// saveToDisk saves a memory to disk as a markdown file with metadata
func (s *Store) saveToDisk(mem *types.Memory) error {
	// Determine the base path based on scope
	var basePath string
	switch mem.Scope {
	case types.MemoryScopeUser:
		basePath = s.cfg.UserMemoryPath
	case types.MemoryScopeGroup:
		basePath = s.cfg.GroupMemoryPath
	default:
		return types.NewError(types.ErrCodeInvalidArgument, fmt.Sprintf("unknown memory scope: %s", mem.Scope))
	}

	// Create owner directory (sanitize owner ID to prevent path traversal)
	ownerDir := filepath.Join(basePath, sanitizeOwnerID(mem.OwnerID))
	if err := os.MkdirAll(ownerDir, 0755); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create owner directory", err)
	}

	// Generate filename from memory ID and title
	filename := s.generateFilename(mem)
	filePath := filepath.Join(ownerDir, filename)

	// Create markdown content with frontmatter metadata
	content, err := s.serializeToMarkdown(mem)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to serialize memory", err)
	}

	// Enforce MaxFileSize configuration (convert KB to bytes)
	if s.cfg.MaxFileSize > 0 {
		maxBytes := s.cfg.MaxFileSize * bytesPerKB
		if len(content) > maxBytes {
			return types.NewError(types.ErrCodeInvalidArgument, 
				fmt.Sprintf("memory content exceeds maximum file size of %d KB", s.cfg.MaxFileSize))
		}
	}

	// Enforce FileFormat - only markdown is supported
	// generateFilename always produces .md extension, so this validates the config
	if s.cfg.FileFormat != "" && s.cfg.FileFormat != "markdown" && s.cfg.FileFormat != "md" {
		return types.NewError(types.ErrCodeInvalidArgument, 
			fmt.Sprintf("unsupported file format: %s (only markdown/md is supported)", s.cfg.FileFormat))
	}

	// Write to temporary file first
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to write memory file", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return types.WrapError(types.ErrCodeInternal, "failed to rename memory file", err)
	}

	return nil
}

// deleteFromDisk deletes a memory file from disk
func (s *Store) deleteFromDisk(mem *types.Memory) error {
	filePath := s.getFilePath(mem)
	if filePath == "" {
		return nil // No file to delete
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return types.WrapError(types.ErrCodeInternal, "failed to delete memory file", err)
	}

	return nil
}

// loadFromDisk loads all memories from disk
func (s *Store) loadFromDisk() error {
	// Load from user memory directory
	if err := s.loadMemoriesFromDir(s.cfg.UserMemoryPath, types.MemoryScopeUser); err != nil {
		s.logger.Warn("Failed to load user memories", "error", err)
	}

	// Load from group memory directory
	if err := s.loadMemoriesFromDir(s.cfg.GroupMemoryPath, types.MemoryScopeGroup); err != nil {
		s.logger.Warn("Failed to load group memories", "error", err)
	}

	return nil
}

// loadMemoriesFromDir loads memories from a specific directory
func (s *Store) loadMemoriesFromDir(dirPath string, scope types.MemoryScope) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		ownerID := entry.Name()
		ownerPath := filepath.Join(dirPath, ownerID)

		// Read memory files from owner directory
	 ownerEntries, err := os.ReadDir(ownerPath)
		if err != nil {
			s.logger.Warn("Failed to read owner directory", "path", ownerPath, "error", err)
			continue
		}

		for _, fileEntry := range ownerEntries {
			// If it's a directory, it's a topic directory - recursively load memories
			if fileEntry.IsDir() {
				topicPath := filepath.Join(ownerPath, fileEntry.Name())
				topicEntries, err := os.ReadDir(topicPath)
				if err != nil {
					s.logger.Warn("Failed to read topic directory", "path", topicPath, "error", err)
					continue
				}

				// Load memory files from topic directory
				for _, topicFileEntry := range topicEntries {
					if topicFileEntry.IsDir() {
						continue // Skip nested directories
					}

					filePath := filepath.Join(topicPath, topicFileEntry.Name())
					mem, err := s.loadMemoryFromFile(filePath, scope, ownerID)
					if err != nil {
						s.logger.Warn("Failed to load memory from file", "path", filePath, "error", err)
						continue
					}

					s.memories[mem.ID] = mem
				}
				continue
			}

			// Load memory files from owner directory (non-organized memories)
			filePath := filepath.Join(ownerPath, fileEntry.Name())
			mem, err := s.loadMemoryFromFile(filePath, scope, ownerID)
			if err != nil {
				s.logger.Warn("Failed to load memory from file", "path", filePath, "error", err)
				continue
			}

			s.memories[mem.ID] = mem
		}
	}

	s.logger.Info("Loaded memories from directory", "path", dirPath, "count", len(s.memories))
	return nil
}

// loadMemoryFromFile loads a memory from a markdown file
func (s *Store) loadMemoryFromFile(filePath string, scope types.MemoryScope, ownerID string) (*types.Memory, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return s.deserializeFromMarkdown(data, scope, ownerID)
}

// generateFilename generates a filename for a memory
func (s *Store) generateFilename(mem *types.Memory) string {
	// Use ID as primary identifier, with sanitized title for readability
	sanitizedTitle := strings.ToLower(strings.ReplaceAll(mem.Title, " ", "-"))
	sanitizedTitle = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, sanitizedTitle)

	return fmt.Sprintf("%s__%s.md", mem.ID, sanitizedTitle)
}

// getFilePath returns the file path for a memory
// It checks both the owner directory and topic subdirectories
func (s *Store) getFilePath(mem *types.Memory) string {
	var basePath string
	switch mem.Scope {
	case types.MemoryScopeUser:
		basePath = s.cfg.UserMemoryPath
	case types.MemoryScopeGroup:
		basePath = s.cfg.GroupMemoryPath
	default:
		return ""
	}

	filename := s.generateFilename(mem)
	
	// First, check if the memory is in a topic directory
	if mem.Topic != "" {
		// Sanitize topic to prevent path traversal
		sanitizedTopic := sanitizeTopic(mem.Topic)
		topicPath := filepath.Join(basePath, sanitizeOwnerID(mem.OwnerID), sanitizedTopic)
		topicFilePath := filepath.Join(topicPath, filename)
		if _, err := os.Stat(topicFilePath); err == nil {
			return topicFilePath
		}
	}
	
	// Fall back to the owner directory (for non-organized memories)
	return filepath.Join(basePath, sanitizeOwnerID(mem.OwnerID), filename)
}

// deepCopyMemory creates a deep copy of a Memory object, including all nested slices and maps
func deepCopyMemory(mem *types.Memory) *types.Memory {
	if mem == nil {
		return nil
	}

	// Create a shallow copy first
	result := *mem

	// Deep copy the Labels map
	if mem.Metadata.Labels != nil {
		result.Metadata.Labels = make(map[string]string, len(mem.Metadata.Labels))
		for k, v := range mem.Metadata.Labels {
			result.Metadata.Labels[k] = v
		}
	}

	// Deep copy the Tags slice
	if mem.Metadata.Tags != nil {
		result.Metadata.Tags = make([]string, len(mem.Metadata.Tags))
		copy(result.Metadata.Tags, mem.Metadata.Tags)
	}

	return &result
}

// serializeToMarkdown converts a memory to markdown format with frontmatter
func (s *Store) serializeToMarkdown(mem *types.Memory) (string, error) {
	return SerializeToMarkdown(mem)
}

// deserializeFromMarkdown parses a memory from markdown format
func (s *Store) deserializeFromMarkdown(data []byte, scope types.MemoryScope, ownerID string) (*types.Memory, error) {
	return DeserializeFromMarkdown(data, scope, ownerID)
}

// Global store instance
var (
	globalStore     *Store
	storeGlobalOnce sync.Once
)

// InitGlobal initializes the global memory store
func InitGlobal(cfg config.MemoryConfig, log *logger.Logger) error {
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

// Global returns the global memory store instance
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

// SetGlobal sets the global memory store instance
func SetGlobal(s *Store) {
	globalStore = s
	storeGlobalOnce = sync.Once{}
}
