package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Organizer manages topic-based memory organization
type Organizer struct {
	mu     sync.RWMutex
	cfg    config.MemoryConfig
	logger *logger.Logger
	store  *Store
	closed bool
}

// NewOrganizer creates a new memory organizer
func NewOrganizer(cfg config.MemoryConfig, store *Store, log *logger.Logger) (*Organizer, error) {
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

	org := &Organizer{
		cfg:    cfg,
		logger: log.With("component", "memory_organizer"),
		store:  store,
		closed: false,
	}

	org.logger.Info("Memory organizer initialized",
		"user_path", cfg.UserMemoryPath,
		"group_path", cfg.GroupMemoryPath)

	return org, nil
}

// NewDefaultOrganizer creates a new memory organizer with default configuration
func NewDefaultOrganizer(store *Store, log *logger.Logger) (*Organizer, error) {
	cfg := config.DefaultMemoryConfig()
	return NewOrganizer(cfg, store, log)
}

// GetTopicPath returns the directory path for a specific scope, owner, and topic
func (o *Organizer) GetTopicPath(scope types.MemoryScope, ownerID, topic string) (string, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.closed {
		return "", types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}

	if err := ValidateTopic(topic); err != nil {
		return "", err
	}

	ownerPath, err := GetOwnerPath(o.cfg, scope, ownerID)
	if err != nil {
		return "", err
	}

	sanitizedTopic := sanitizeTopic(topic)
	return filepath.Join(ownerPath, sanitizedTopic), nil
}

// ValidateTopic validates that a topic name is safe for filesystem use
func ValidateTopic(topic string) error {
	if topic == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "topic cannot be empty")
	}

	// Check for path traversal attempts
	if strings.Contains(topic, "..") || strings.Contains(topic, "/") || strings.Contains(topic, "\\") {
		return types.NewError(types.ErrCodeInvalidArgument, "topic contains invalid characters")
	}

	// Check length
	if len(topic) > 100 {
		return types.NewError(types.ErrCodeInvalidArgument, "topic name too long (max 100 characters)")
	}

	return nil
}

// sanitizeTopic sanitizes a topic name for safe filesystem usage
func sanitizeTopic(topic string) string {
	// Convert to lowercase
	topic = strings.ToLower(topic)

	// Replace spaces with hyphens
	topic = strings.ReplaceAll(topic, " ", "-")

	// Remove any non-alphanumeric characters (except dash, underscore)
	topic = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, topic)

	// Remove consecutive hyphens
	for strings.Contains(topic, "--") {
		topic = strings.ReplaceAll(topic, "--", "-")
	}

	// Trim hyphens from start and end
	topic = strings.Trim(topic, "-")

	// Ensure we don't have an empty topic after sanitization
	if topic == "" {
		topic = "general"
	}

	return topic
}

// CreateTopicDir creates the directory for a specific topic if it doesn't exist
func (o *Organizer) CreateTopicDir(scope types.MemoryScope, ownerID, topic string) error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}
	o.mu.Unlock()

	topicPath, err := o.GetTopicPath(scope, ownerID, topic)
	if err != nil {
		return err
	}

	// Create directory with appropriate permissions
	if err := os.MkdirAll(topicPath, 0755); err != nil {
		return types.WrapError(types.ErrCodeInternal, fmt.Sprintf("failed to create topic directory: %s", topic), err)
	}

	o.logger.Debug("Topic directory created", "scope", scope, "owner_id", ownerID, "topic", topic)
	return nil
}

// ListTopics returns a list of all topics for a specific scope and owner
func (o *Organizer) ListTopics(ctx context.Context, scope types.MemoryScope, ownerID string) ([]string, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}

	ownerPath, err := GetOwnerPath(o.cfg, scope, ownerID)
	if err != nil {
		return nil, err
	}

	// Check if owner directory exists
	exists, err := OwnerDirExists(o.cfg, scope, ownerID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return []string{}, nil
	}

	// Read directory entries
	entries, err := os.ReadDir(ownerPath)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to read owner directory", err)
	}

	topics := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			topics = append(topics, entry.Name())
		}
	}

	return topics, nil
}

// GetMemoriesByTopic retrieves all memories for a specific scope, owner, and topic
func (o *Organizer) GetMemoriesByTopic(ctx context.Context, scope types.MemoryScope, ownerID, topic string) ([]*types.Memory, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}

	if err := ValidateTopic(topic); err != nil {
		return nil, err
	}

	// Use the store to filter by topic
	filter := &types.MemoryFilter{
		Scope:   &scope,
		OwnerID: &ownerID,
		Topic:   &topic,
	}

	memories, err := o.store.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	return memories, nil
}

// OrganizeMemory moves a memory file to its topic-based directory
func (o *Organizer) OrganizeMemory(ctx context.Context, memoryID types.ID) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}

	// Get the memory from store
	mem, err := o.store.Get(ctx, memoryID)
	if err != nil {
		return err
	}

	// Get the source file path (current location)
	srcPath := o.store.getFilePath(mem)
	if srcPath == "" {
		return types.NewError(types.ErrCodeNotFound, "memory file path not found")
	}

	// Check if source file exists
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return types.NewError(types.ErrCodeNotFound, "memory file does not exist")
	}

	// Get the destination topic path
	topicPath, err := o.GetTopicPath(mem.Scope, mem.OwnerID, mem.Topic)
	if err != nil {
		return err
	}

	// Ensure topic directory exists
	if err := os.MkdirAll(topicPath, 0755); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create topic directory", err)
	}

	// Generate filename
	filename := o.store.generateFilename(mem)
	destPath := filepath.Join(topicPath, filename)

	// If source and destination are the same, no action needed
	if srcPath == destPath {
		o.logger.Debug("Memory already in topic directory", "memory_id", memoryID, "path", srcPath)
		return nil
	}

	// Move the file
	if err := os.Rename(srcPath, destPath); err != nil {
		// If rename fails (e.g., cross-device), try copy + delete
		if err := o.copyAndDelete(srcPath, destPath); err != nil {
			return types.WrapError(types.ErrCodeInternal, "failed to move memory file to topic directory", err)
		}
	}

	o.logger.Info("Memory organized into topic directory",
		"memory_id", memoryID,
		"topic", mem.Topic,
		"dest_path", destPath)

	return nil
}

// OrganizeAllMemories organizes all memories for a scope and owner into topic directories
func (o *Organizer) OrganizeAllMemories(ctx context.Context, scope types.MemoryScope, ownerID string) (OrganizeStats, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return OrganizeStats{}, types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}

	stats := OrganizeStats{
		Scope:   scope,
		OwnerID: ownerID,
	}

	// Get all memories for the owner
	memories, err := o.store.GetByOwner(ctx, scope, ownerID)
	if err != nil {
		return stats, err
	}

	stats.TotalCount = len(memories)

	// Organize each memory
	for _, mem := range memories {
		// Get the source file path
		srcPath := o.store.getFilePath(mem)
		if srcPath == "" {
			stats.FailedCount++
			stats.Errors = append(stats.Errors, fmt.Sprintf("memory %s: no file path", mem.ID))
			continue
		}

		// Check if source file exists
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			stats.SkippedCount++
			continue
		}

		// Get the destination topic path
		topic := mem.Topic
		if topic == "" {
			topic = "general"
		}

		topicPath, err := o.GetTopicPath(mem.Scope, mem.OwnerID, topic)
		if err != nil {
			stats.FailedCount++
			stats.Errors = append(stats.Errors, fmt.Sprintf("memory %s: %v", mem.ID, err))
			continue
		}

		// Ensure topic directory exists
		if err := os.MkdirAll(topicPath, 0755); err != nil {
			stats.FailedCount++
			stats.Errors = append(stats.Errors, fmt.Sprintf("memory %s: %v", mem.ID, err))
			continue
		}

		// Generate filename
		filename := o.store.generateFilename(mem)
		destPath := filepath.Join(topicPath, filename)

		// If source and destination are the same, skip
		if srcPath == destPath {
			stats.SkippedCount++
			continue
		}

		// Move the file
		if err := os.Rename(srcPath, destPath); err != nil {
			// If rename fails, try copy + delete
			if err := o.copyAndDelete(srcPath, destPath); err != nil {
				stats.FailedCount++
				stats.Errors = append(stats.Errors, fmt.Sprintf("memory %s: %v", mem.ID, err))
				continue
			}
		}

		stats.OrganizedCount++
	}

	o.logger.Info("Memory organization complete",
		"scope", scope,
		"owner_id", ownerID,
		"total", stats.TotalCount,
		"organized", stats.OrganizedCount,
		"skipped", stats.SkippedCount,
		"failed", stats.FailedCount)

	return stats, nil
}

// OrganizeStats contains statistics about a memory organization operation
type OrganizeStats struct {
	Scope         types.MemoryScope `json:"scope"`
	OwnerID       string            `json:"owner_id"`
	TotalCount    int               `json:"total_count"`
	OrganizedCount int              `json:"organized_count"`
	SkippedCount  int               `json:"skipped_count"`
	FailedCount   int               `json:"failed_count"`
	Errors        []string          `json:"errors,omitempty"`
}

// copyAndDelete copies a file and then deletes the original
func (o *Organizer) copyAndDelete(src, dest string) error {
	// Read source file
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Write to destination
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return err
	}

	// Delete source
	if err := os.Remove(src); err != nil {
		return err
	}

	return nil
}

// GetTopicStats returns statistics about memories grouped by topic
func (o *Organizer) GetTopicStats(ctx context.Context, scope types.MemoryScope, ownerID string) (map[string]int, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}

	// Get all memories for the owner
	memories, err := o.store.GetByOwner(ctx, scope, ownerID)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]int)
	for _, mem := range memories {
		topic := mem.Topic
		if topic == "" {
			topic = "general"
		}
		stats[topic]++
	}

	return stats, nil
}

// RenameTopic renames a topic directory for a scope and owner
func (o *Organizer) RenameTopic(scope types.MemoryScope, ownerID, oldTopic, newTopic string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}

	if err := ValidateTopic(oldTopic); err != nil {
		return err
	}

	if err := ValidateTopic(newTopic); err != nil {
		return err
	}

	oldPath, err := o.GetTopicPath(scope, ownerID, oldTopic)
	if err != nil {
		return err
	}

	newPath, err := o.GetTopicPath(scope, ownerID, newTopic)
	if err != nil {
		return err
	}

	// Check if old topic directory exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("topic directory not found: %s", oldTopic))
	}

	// Check if new topic directory already exists
	if _, err := os.Stat(newPath); err == nil {
		return types.NewError(types.ErrCodeAlreadyExists, fmt.Sprintf("topic directory already exists: %s", newTopic))
	}

	// Rename the directory
	if err := os.Rename(oldPath, newPath); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to rename topic directory", err)
	}

	o.logger.Info("Topic renamed",
		"scope", scope,
		"owner_id", ownerID,
		"old_topic", oldTopic,
		"new_topic", newTopic)

	return nil
}

// DeleteTopic removes a topic directory and all its memories for a scope and owner
func (o *Organizer) DeleteTopic(scope types.MemoryScope, ownerID, topic string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}

	if err := ValidateTopic(topic); err != nil {
		return err
	}

	topicPath, err := o.GetTopicPath(scope, ownerID, topic)
	if err != nil {
		return err
	}

	// Check if topic directory exists
	if _, err := os.Stat(topicPath); os.IsNotExist(err) {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("topic directory not found: %s", topic))
	}

	// Remove the directory and all contents
	if err := os.RemoveAll(topicPath); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to delete topic directory", err)
	}

	o.logger.Info("Topic deleted",
		"scope", scope,
		"owner_id", ownerID,
		"topic", topic)

	return nil
}

// MergeTopic merges memories from one topic into another
func (o *Organizer) MergeTopic(ctx context.Context, scope types.MemoryScope, ownerID, sourceTopic, targetTopic string) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return 0, types.NewError(types.ErrCodeUnavailable, "memory organizer is closed")
	}

	if err := ValidateTopic(sourceTopic); err != nil {
		return 0, err
	}

	if err := ValidateTopic(targetTopic); err != nil {
		return 0, err
	}

	// Get memories from source topic
	filter := &types.MemoryFilter{
		Scope:   &scope,
		OwnerID: &ownerID,
		Topic:   &sourceTopic,
	}

	memories, err := o.store.List(ctx, filter)
	if err != nil {
		return 0, err
	}

	// Update each memory's topic
	mergedCount := 0
	for _, mem := range memories {
		// Create a copy with updated topic
		updatedMem := *mem
		updatedMem.Topic = targetTopic

		// Store the updated memory
		if err := o.store.Store(ctx, &updatedMem); err != nil {
			o.logger.Warn("Failed to update memory topic",
				"memory_id", mem.ID,
				"source_topic", sourceTopic,
				"target_topic", targetTopic,
				"error", err)
			continue
		}

		mergedCount++
	}

	o.logger.Info("Topic merge complete",
		"scope", scope,
		"owner_id", ownerID,
		"source_topic", sourceTopic,
		"target_topic", targetTopic,
		"merged_count", mergedCount)

	return mergedCount, nil
}

// Close closes the organizer
func (o *Organizer) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return nil
	}

	o.closed = true
	o.logger.Info("Memory organizer closed")
	return nil
}

// IsClosed returns true if the organizer is closed
func (o *Organizer) IsClosed() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.closed
}
