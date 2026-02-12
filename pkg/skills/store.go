package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

const (
	bytesPerKB = 1024 // Bytes per kilobyte for size conversions
)

// Store manages skill storage with filesystem persistence
type Store struct {
	skills map[types.ID]*types.Skill
	mu     sync.RWMutex
	cfg    types.SkillConfig
	logger *logger.Logger
	closed bool
}

// NewStore creates a new skill store with the specified configuration
func NewStore(cfg types.SkillConfig, log *logger.Logger) (*Store, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Ensure the storage directory exists
	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create skill storage directory", err)
	}

	store := &Store{
		skills: make(map[types.ID]*types.Skill),
		cfg:    cfg,
		logger: log.With("component", "skill_store"),
		closed: false,
	}

	// Load existing skills from disk
	if err := store.loadFromDisk(); err != nil {
		store.logger.Warn("Failed to load existing skills from disk", "error", err)
		// Continue anyway - the store will start fresh
	}

	store.logger.Info("Skill store initialized",
		"storage_path", cfg.StoragePath,
		"enabled", cfg.Enabled)
	return store, nil
}

// NewDefaultStore creates a new skill store with default configuration
func NewDefaultStore(log *logger.Logger) (*Store, error) {
	cfg := DefaultSkillConfig()
	return NewStore(cfg, log)
}

// Store saves a new skill or updates an existing one
func (s *Store) Store(ctx context.Context, skill *types.Skill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "skill store is closed")
	}

	if !s.cfg.Enabled {
		return types.NewError(types.ErrCodePermission, "skill storage is disabled")
	}

	if skill.ID == "" {
		skill.ID = types.GenerateID()
	}

	if skill.Name == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "skill name is required")
	}

	if skill.DisplayName == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "skill display_name is required")
	}

	if skill.Description == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "skill description is required")
	}

	if skill.Scope == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "skill scope is required")
	}

	if skill.OwnerID == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "skill owner_id is required")
	}

	now := types.NewTimestamp()

	// Check if skill already exists
	existing, exists := s.skills[skill.ID]
	if exists {
		// Update existing skill - deep copy metadata to prevent external mutation
		existing.Name = skill.Name
		existing.DisplayName = skill.DisplayName
		existing.Description = skill.Description
		existing.Version = skill.Version
		existing.Author = skill.Author
		existing.State = skill.State
		existing.Source = skill.Source

		// Deep copy slices
		if skill.AgentTypes != nil {
			existing.AgentTypes = make([]types.SkillAgentType, len(skill.AgentTypes))
			copy(existing.AgentTypes, skill.AgentTypes)
		} else {
			existing.AgentTypes = nil
		}

		if skill.Capabilities != nil {
			existing.Capabilities = make([]types.SkillCapability, len(skill.Capabilities))
			for i, cap := range skill.Capabilities {
				existing.Capabilities[i] = cap
				// Deep copy the Config map (recursively handles nested structures)
				if cap.Config != nil {
					existing.Capabilities[i].Config = deepCopyConfigMap(cap.Config)
				}
			}
		} else {
			existing.Capabilities = nil
		}

		// Deep copy metadata fields
		if skill.Metadata.Labels != nil {
			existing.Metadata.Labels = make(map[string]string, len(skill.Metadata.Labels))
			for k, v := range skill.Metadata.Labels {
				existing.Metadata.Labels[k] = v
			}
		} else {
			existing.Metadata.Labels = nil
		}

		if skill.Metadata.Tags != nil {
			existing.Metadata.Tags = make([]string, len(skill.Metadata.Tags))
			copy(existing.Metadata.Tags, skill.Metadata.Tags)
		} else {
			existing.Metadata.Tags = nil
		}

		if skill.Metadata.Keywords != nil {
			existing.Metadata.Keywords = make([]string, len(skill.Metadata.Keywords))
			copy(existing.Metadata.Keywords, skill.Metadata.Keywords)
		} else {
			existing.Metadata.Keywords = nil
		}

		if skill.Metadata.RequiredAPIs != nil {
			existing.Metadata.RequiredAPIs = make([]string, len(skill.Metadata.RequiredAPIs))
			copy(existing.Metadata.RequiredAPIs, skill.Metadata.RequiredAPIs)
		} else {
			existing.Metadata.RequiredAPIs = nil
		}

		if skill.Metadata.Dependencies != nil {
			existing.Metadata.Dependencies = make([]string, len(skill.Metadata.Dependencies))
			copy(existing.Metadata.Dependencies, skill.Metadata.Dependencies)
		} else {
			existing.Metadata.Dependencies = nil
		}

		// Copy scalar metadata fields
		existing.Metadata.Category = skill.Metadata.Category
		existing.Metadata.LoadCount = skill.Metadata.LoadCount
		existing.Metadata.ErrorCount = skill.Metadata.ErrorCount
		existing.Metadata.LastError = skill.Metadata.LastError
		existing.Metadata.Verified = skill.Metadata.Verified

		// Copy pointer fields
		existing.ActivatedAt = skill.ActivatedAt
		existing.Metadata.LastUsedAt = skill.Metadata.LastUsedAt

		// Copy fields
		existing.Content = skill.Content
		existing.FilePath = skill.FilePath
		existing.Repository = skill.Repository

		// Use the UpdatedAt from the input skill if it's set
		if !skill.UpdatedAt.Time.IsZero() {
			existing.UpdatedAt = skill.UpdatedAt
		} else {
			existing.UpdatedAt = now
		}

		s.logger.Info("Skill updated", "id", skill.ID, "name", skill.Name)
	} else {
		// Create new skill - deep copy to prevent external mutation
		skillCopy := deepCopySkill(skill)
		skillCopy.CreatedAt = now
		skillCopy.UpdatedAt = now
		s.skills[skill.ID] = skillCopy
		s.logger.Info("Skill stored", "id", skill.ID, "name", skill.Name, "scope", skill.Scope)
	}

	// Persist to disk using the canonical in-memory object (with correct timestamps)
	skillToPersist := s.skills[skill.ID]
	if err := s.saveToDisk(skillToPersist); err != nil {
		s.logger.Error("Failed to persist skill to disk", "error", err)
		return err
	}

	return nil
}

// Get retrieves a skill by ID
func (s *Store) Get(ctx context.Context, id types.ID) (*types.Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "skill store is closed")
	}

	skill, exists := s.skills[id]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, fmt.Sprintf("skill not found: %s", id))
	}

	// Update access time asynchronously
	go s.updateAccessTime(id)

	// Return a deep copy to prevent external mutation
	return deepCopySkill(skill), nil
}

// GetByOwner retrieves all skills for a specific owner
func (s *Store) GetByOwner(ctx context.Context, scope types.SkillScope, ownerID string) ([]*types.Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "skill store is closed")
	}

	result := make([]*types.Skill, 0)
	for _, skill := range s.skills {
		if skill.Scope == scope && skill.OwnerID == ownerID {
			result = append(result, deepCopySkill(skill))
		}
	}

	return result, nil
}

// List retrieves skills matching the given filter
func (s *Store) List(ctx context.Context, filter *types.SkillFilter) ([]*types.Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "skill store is closed")
	}

	result := make([]*types.Skill, 0)
	for _, skill := range s.skills {
		if !s.matchesFilter(skill, filter) {
			continue
		}

		result = append(result, deepCopySkill(skill))
	}

	return result, nil
}

// Delete removes a skill from the store
func (s *Store) Delete(ctx context.Context, id types.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "skill store is closed")
	}

	skill, exists := s.skills[id]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("skill not found: %s", id))
	}

	// Delete from filesystem
	if err := s.deleteFromDisk(skill); err != nil {
		s.logger.Error("Failed to delete skill from disk", "error", err)
		return err
	}

	delete(s.skills, id)
	s.logger.Info("Skill deleted", "id", id)

	return nil
}

// Stats returns statistics about skills for a specific owner
func (s *Store) Stats(ctx context.Context, scope types.SkillScope, ownerID string) (*types.SkillStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "skill store is closed")
	}

	stats := &types.SkillStats{
		Scope:        scope,
		OwnerID:      ownerID,
		StateCounts:  make(map[types.SkillState]int),
		SourceCounts: make(map[types.SkillSource]int),
		CategoryCounts: make(map[string]int),
		TotalCount:   0,
		Timestamp:    types.NewTimestamp(),
	}

	for _, skill := range s.skills {
		if skill.Scope != scope || skill.OwnerID != ownerID {
			continue
		}

		stats.TotalCount++
		stats.StateCounts[skill.State]++
		stats.SourceCounts[skill.Source]++
		if skill.Metadata.Category != "" {
			stats.CategoryCounts[skill.Metadata.Category]++
		}

		if skill.UpdatedAt.Time.After(stats.LastUpdated.Time) {
			stats.LastUpdated = skill.UpdatedAt
		}
	}

	return stats, nil
}

// Close closes the skill store
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	s.logger.Info("Skill store closed")
	return nil
}

// IsClosed returns true if the store is closed
func (s *Store) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// updateAccessTime updates the last used time for a skill
func (s *Store) updateAccessTime(id types.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if skill, exists := s.skills[id]; exists {
		now := types.NewTimestamp()
		skill.Metadata.LastUsedAt = &now
	}
}

// matchesFilter checks if a skill matches the given filter
func (s *Store) matchesFilter(skill *types.Skill, filter *types.SkillFilter) bool {
	if filter == nil {
		return true
	}

	if filter.Scope != nil && skill.Scope != *filter.Scope {
		return false
	}

	if filter.OwnerID != nil && skill.OwnerID != *filter.OwnerID {
		return false
	}

	if filter.State != nil && skill.State != *filter.State {
		return false
	}

	if filter.Source != nil && skill.Source != *filter.Source {
		return false
	}

	if filter.AgentType != nil {
		// Check if skill supports the specified agent type
		match := false
		for _, at := range skill.AgentTypes {
			if at == *filter.AgentType || at == types.SkillAgentTypeAll {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	if filter.Category != nil && skill.Metadata.Category != *filter.Category {
		return false
	}

	if filter.Verified != nil && skill.Metadata.Verified != *filter.Verified {
		return false
	}

	if filter.MinLoadCount != nil && skill.Metadata.LoadCount < *filter.MinLoadCount {
		return false
	}

	if len(filter.Label) > 0 {
		match := false
		for k, v := range filter.Label {
			if skill.Metadata.Labels != nil {
				if skillVal, ok := skill.Metadata.Labels[k]; ok && skillVal == v {
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
		// Check if skill has all required tags
		tagSet := make(map[string]bool)
		for _, tag := range skill.Metadata.Tags {
			tagSet[tag] = true
		}
		for _, tag := range filter.Tags {
			if !tagSet[tag] {
				return false
			}
		}
	}

	if filter.StartTime != nil && skill.CreatedAt.Time.Before(filter.StartTime.Time) {
		return false
	}

	if filter.EndTime != nil && skill.CreatedAt.Time.After(filter.EndTime.Time) {
		return false
	}

	return true
}

// saveToDisk saves a skill to disk as a markdown file with metadata
func (s *Store) saveToDisk(skill *types.Skill) error {
	// Get the base storage path
	basePath := s.cfg.StoragePath

	// Create owner directory (sanitize owner ID to prevent path traversal)
	ownerDir := filepath.Join(basePath, sanitizeOwnerID(skill.OwnerID))
	if err := os.MkdirAll(ownerDir, 0755); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to create owner directory", err)
	}

	// Generate filename from skill ID and name
	filename := s.generateFilename(skill)
	filePath := filepath.Join(ownerDir, filename)

	// Create markdown content with frontmatter metadata
	content, err := s.serializeToMarkdown(skill)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to serialize skill", err)
	}

	// Enforce MaxSkillsPerOwner if configured (only for new skills, not updates)
	if s.cfg.MaxSkillsPerOwner > 0 {
		// Count existing skills for this owner (excluding the current skill ID)
		count := 0
		for _, existingSkill := range s.skills {
			if existingSkill.Scope == skill.Scope && existingSkill.OwnerID == skill.OwnerID && existingSkill.ID != skill.ID {
				count++
			}
		}
		if count >= s.cfg.MaxSkillsPerOwner {
			return types.NewError(types.ErrCodePermission,
				fmt.Sprintf("maximum number of skills (%d) reached for owner %s", s.cfg.MaxSkillsPerOwner, skill.OwnerID))
		}
	}

	// Write to temporary file first
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to write skill file", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return types.WrapError(types.ErrCodeInternal, "failed to rename skill file", err)
	}

	return nil
}

// deleteFromDisk deletes a skill file from disk
func (s *Store) deleteFromDisk(skill *types.Skill) error {
	filePath := s.getFilePath(skill)
	if filePath == "" {
		return nil // No file to delete
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return types.WrapError(types.ErrCodeInternal, "failed to delete skill file", err)
	}

	return nil
}

// loadFromDisk loads all skills from disk
func (s *Store) loadFromDisk() error {
	basePath := s.cfg.StoragePath

	// Load skills from base directory
	if err := s.loadSkillsFromDir(basePath); err != nil {
		s.logger.Warn("Failed to load skills from directory", "path", basePath, "error", err)
	}

	return nil
}

// loadSkillsFromDir loads skills from a specific directory
func (s *Store) loadSkillsFromDir(dirPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Process owner directory
			ownerID := entry.Name()
			ownerPath := filepath.Join(dirPath, ownerID)

			// Read skill files from owner directory
			ownerEntries, err := os.ReadDir(ownerPath)
			if err != nil {
				s.logger.Warn("Failed to read owner directory", "path", ownerPath, "error", err)
				continue
			}

			for _, fileEntry := range ownerEntries {
				if fileEntry.IsDir() {
					continue // Skip nested directories
				}

				// Only process .md files
				if !strings.HasSuffix(fileEntry.Name(), ".md") {
					continue
				}

				filePath := filepath.Join(ownerPath, fileEntry.Name())
				skill, err := s.loadSkillFromFile(filePath, ownerID)
				if err != nil {
					s.logger.Warn("Failed to load skill from file", "path", filePath, "error", err)
					continue
				}

				s.skills[skill.ID] = skill
			}
		}
	}

	s.logger.Info("Loaded skills from directory", "path", dirPath, "count", len(s.skills))
	return nil
}

// loadSkillFromFile loads a skill from a markdown file
func (s *Store) loadSkillFromFile(filePath string, ownerID string) (*types.Skill, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Determine scope from file path or default to user scope
	scope := types.SkillScopeUser
	// In a real implementation, we might determine scope from directory structure

	return s.deserializeFromMarkdown(data, scope, ownerID, filePath)
}

// generateFilename generates a filename for a skill
func (s *Store) generateFilename(skill *types.Skill) string {
	// Use ID as primary identifier, with sanitized name for readability
	sanitizedName := strings.ToLower(strings.ReplaceAll(skill.Name, " ", "-"))
	sanitizedName = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, sanitizedName)

	return fmt.Sprintf("%s__%s.md", skill.ID, sanitizedName)
}

// getFilePath returns the file path for a skill
func (s *Store) getFilePath(skill *types.Skill) string {
	if skill.FilePath != "" {
		return skill.FilePath
	}

	basePath := s.cfg.StoragePath
	filename := s.generateFilename(skill)
	return filepath.Join(basePath, sanitizeOwnerID(skill.OwnerID), filename)
}

// deepCopySkill creates a deep copy of a Skill object, including all nested slices and maps
func deepCopySkill(skill *types.Skill) *types.Skill {
	if skill == nil {
		return nil
	}

	// Create a shallow copy first
	result := *skill

	// Deep copy the Labels map
	if skill.Metadata.Labels != nil {
		result.Metadata.Labels = make(map[string]string, len(skill.Metadata.Labels))
		for k, v := range skill.Metadata.Labels {
			result.Metadata.Labels[k] = v
		}
	}

	// Deep copy the Tags slice
	if skill.Metadata.Tags != nil {
		result.Metadata.Tags = make([]string, len(skill.Metadata.Tags))
		copy(result.Metadata.Tags, skill.Metadata.Tags)
	}

	// Deep copy the Keywords slice
	if skill.Metadata.Keywords != nil {
		result.Metadata.Keywords = make([]string, len(skill.Metadata.Keywords))
		copy(result.Metadata.Keywords, skill.Metadata.Keywords)
	}

	// Deep copy the RequiredAPIs slice
	if skill.Metadata.RequiredAPIs != nil {
		result.Metadata.RequiredAPIs = make([]string, len(skill.Metadata.RequiredAPIs))
		copy(result.Metadata.RequiredAPIs, skill.Metadata.RequiredAPIs)
	}

	// Deep copy the Dependencies slice
	if skill.Metadata.Dependencies != nil {
		result.Metadata.Dependencies = make([]string, len(skill.Metadata.Dependencies))
		copy(result.Metadata.Dependencies, skill.Metadata.Dependencies)
	}

	// Deep copy the AgentTypes slice
	if skill.AgentTypes != nil {
		result.AgentTypes = make([]types.SkillAgentType, len(skill.AgentTypes))
		copy(result.AgentTypes, skill.AgentTypes)
	}

	// Deep copy the Capabilities slice
	if skill.Capabilities != nil {
		result.Capabilities = make([]types.SkillCapability, len(skill.Capabilities))
		for i, cap := range skill.Capabilities {
			result.Capabilities[i] = cap
			// Deep copy the Config map (recursively handles nested structures)
			if cap.Config != nil {
				result.Capabilities[i].Config = deepCopyConfigMap(cap.Config)
			}
		}
	}

	return &result
}

// deepCopyConfigMap creates a deep copy of a config map, handling nested structures
func deepCopyConfigMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = deepCopyValue(v)
	}
	return result
}

// deepCopyValue creates a deep copy of a value, handling common types
func deepCopyValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]interface{}:
		return deepCopyConfigMap(val)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = deepCopyValue(item)
		}
		return result
	case []string:
		result := make([]string, len(val))
		copy(result, val)
		return result
	case []int:
		result := make([]int, len(val))
		copy(result, val)
		return result
	case []float64:
		result := make([]float64, len(val))
		copy(result, val)
		return result
	default:
		// For primitive types (string, int, float64, bool, etc.), return as-is
		// These are immutable in Go, so no deep copy needed
		return v
	}
}

// serializeToMarkdown converts a skill to markdown format with frontmatter
func (s *Store) serializeToMarkdown(skill *types.Skill) (string, error) {
	return SerializeToMarkdown(skill)
}

// deserializeFromMarkdown parses a skill from markdown format
func (s *Store) deserializeFromMarkdown(data []byte, scope types.SkillScope, ownerID string, filePath string) (*types.Skill, error) {
	return ParseFromMarkdown(data, scope, ownerID, filePath)
}

// sanitizeOwnerID sanitizes an owner ID for safe filesystem usage
func sanitizeOwnerID(ownerID string) string {
	// Handle empty input explicitly
	if ownerID == "" {
		return ""
	}
	// Remove any path components that could lead to directory traversal
	ownerID = filepath.Base(ownerID)
	// filepath.Base("") returns ".", so check again
	if ownerID == "." {
		return ""
	}
	// Remove any non-alphanumeric characters (except dash, underscore, dot)
	ownerID = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, ownerID)
	return ownerID
}

// Global store instance
var (
	globalStore     *Store
	storeGlobalOnce sync.Once
)

// InitGlobal initializes the global skill store
func InitGlobal(cfg types.SkillConfig, log *logger.Logger) error {
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

// Global returns the global skill store instance
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

// SetGlobal sets the global skill store instance
func SetGlobal(s *Store) {
	globalStore = s
	storeGlobalOnce = sync.Once{}
}
