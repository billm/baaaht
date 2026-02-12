package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Loader manages skill discovery and activation from filesystem directories
type Loader struct {
	mu     sync.RWMutex
	cfg    types.SkillConfig
	logger *logger.Logger
	store  *Store
	closed bool
}

// NewLoader creates a new skill loader
func NewLoader(cfg types.SkillConfig, store *Store, log *logger.Logger) (*Loader, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	if store == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "skill store is required")
	}

	ldr := &Loader{
		cfg:    cfg,
		logger: log.With("component", "skill_loader"),
		store:  store,
		closed: false,
	}

	ldr.logger.Info("Skill loader initialized",
		"storage_path", cfg.StoragePath,
		"enabled", cfg.Enabled,
		"auto_load", cfg.AutoLoad)

	return ldr, nil
}

// NewDefaultLoader creates a new skill loader with default configuration
func NewDefaultLoader(store *Store, log *logger.Logger) (*Loader, error) {
	cfg := DefaultSkillConfig()
	return NewLoader(cfg, store, log)
}

// LoadStats contains statistics about a skill loading operation
type LoadStats struct {
	Scope        types.SkillScope `json:"scope"`
	OwnerID      string           `json:"owner_id"`
	TotalCount   int              `json:"total_count"`
	LoadedCount  int              `json:"loaded_count"`
	SkippedCount int              `json:"skipped_count"`
	FailedCount  int              `json:"failed_count"`
	Errors       []string         `json:"errors,omitempty"`
	Duration     time.Duration    `json:"duration"`
}

// LoadFromPath loads skills from a specific directory path for a scope and owner
func (l *Loader) LoadFromPath(ctx context.Context, scope types.SkillScope, ownerID, skillPath string) (LoadStats, error) {
	startTime := time.Now()
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return LoadStats{}, types.NewError(types.ErrCodeUnavailable, "skill loader is closed")
	}
	l.mu.Unlock()

	stats := LoadStats{
		Scope:   scope,
		OwnerID: ownerID,
		Errors:  make([]string, 0),
	}

	// Validate skill path exists
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		stats.FailedCount++
		stats.Errors = append(stats.Errors, fmt.Sprintf("skill path does not exist: %s", skillPath))
		stats.Duration = time.Since(startTime)
		return stats, nil // Not an error, just no skills to load
	}

	// Scan for skill files
	skillFiles, err := l.scanSkillFiles(skillPath, l.cfg.LoadConfig.Recursive)
	if err != nil {
		stats.FailedCount++
		stats.Errors = append(stats.Errors, fmt.Sprintf("failed to scan skill files: %v", err))
		stats.Duration = time.Since(startTime)
		return stats, nil
	}

	stats.TotalCount = len(skillFiles)

	// Load each skill file
	for _, filePath := range skillFiles {
		skill, err := l.loadSkillFromFile(scope, ownerID, filePath)
		if err != nil {
			stats.FailedCount++
			stats.Errors = append(stats.Errors, fmt.Sprintf("%s: %v", filepath.Base(filePath), err))
			l.logger.Warn("Failed to load skill", "path", filePath, "error", err)

			// Check if we've exceeded max load errors
			if l.cfg.LoadConfig.MaxLoadErrors > 0 && stats.FailedCount >= l.cfg.LoadConfig.MaxLoadErrors {
				l.logger.Error("Maximum load errors reached, stopping load",
					"max_errors", l.cfg.LoadConfig.MaxLoadErrors,
					"failed_count", stats.FailedCount)
				break
			}
			continue
		}

		// Store the skill
		if err := l.store.Store(ctx, skill); err != nil {
			stats.FailedCount++
			stats.Errors = append(stats.Errors, fmt.Sprintf("%s: failed to store: %v", skill.Name, err))
			l.logger.Warn("Failed to store skill", "skill", skill.Name, "error", err)
			continue
		}

		stats.LoadedCount++
		l.logger.Info("Skill loaded", "id", skill.ID, "name", skill.Name, "scope", scope, "owner_id", ownerID)
	}

	stats.Duration = time.Since(startTime)
	l.logger.Info("Skill loading complete",
		"scope", scope,
		"owner_id", ownerID,
		"skill_path", skillPath,
		"total", stats.TotalCount,
		"loaded", stats.LoadedCount,
		"skipped", stats.SkippedCount,
		"failed", stats.FailedCount,
		"duration", stats.Duration)

	return stats, nil
}

// LoadAll loads skills from all configured skill paths for a scope and owner
func (l *Loader) LoadAll(ctx context.Context, scope types.SkillScope, ownerID string) (LoadStats, error) {
	l.mu.RLock()
	if l.closed {
		l.mu.RUnlock()
		return LoadStats{}, types.NewError(types.ErrCodeUnavailable, "skill loader is closed")
	}
	l.mu.RUnlock()

	combinedStats := LoadStats{
		Scope:   scope,
		OwnerID: ownerID,
		Errors:  make([]string, 0),
	}

	if !l.cfg.AutoLoad || !l.cfg.LoadConfig.Enabled {
		l.logger.Debug("Skill auto-load disabled", "scope", scope, "owner_id", ownerID)
		return combinedStats, nil
	}

	// Load from each configured path
	for _, skillPath := range l.cfg.LoadConfig.SkillPaths {
		stats, err := l.LoadFromPath(ctx, scope, ownerID, skillPath)
		if err != nil {
			combinedStats.Errors = append(combinedStats.Errors, err.Error())
			continue
		}
		combinedStats.TotalCount += stats.TotalCount
		combinedStats.LoadedCount += stats.LoadedCount
		combinedStats.SkippedCount += stats.SkippedCount
		combinedStats.FailedCount += stats.FailedCount
		combinedStats.Errors = append(combinedStats.Errors, stats.Errors...)
		combinedStats.Duration += stats.Duration
	}

	return combinedStats, nil
}

// ActivateSkill activates a skill by setting its state to loaded
func (l *Loader) ActivateSkill(ctx context.Context, skillID types.ID) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return types.NewError(types.ErrCodeUnavailable, "skill loader is closed")
	}

	// Get the skill from store
	skill, err := l.store.Get(ctx, skillID)
	if err != nil {
		return err
	}

	// Update skill state
	now := types.NewTimestamp()
	skill.State = types.SkillStateLoaded
	skill.UpdatedAt = now
	skill.ActivatedAt = &now
	skill.Metadata.LoadCount++

	// Store the updated skill
	if err := l.store.Store(ctx, skill); err != nil {
		return err
	}

	l.logger.Info("Skill activated", "id", skillID, "name", skill.Name)
	return nil
}

// DeactivateSkill deactivates a skill by setting its state to unloaded
func (l *Loader) DeactivateSkill(ctx context.Context, skillID types.ID) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return types.NewError(types.ErrCodeUnavailable, "skill loader is closed")
	}

	// Get the skill from store
	skill, err := l.store.Get(ctx, skillID)
	if err != nil {
		return err
	}

	// Update skill state
	skill.State = types.SkillStateUnloaded
	skill.UpdatedAt = types.NewTimestamp()

	// Store the updated skill
	if err := l.store.Store(ctx, skill); err != nil {
		return err
	}

	l.logger.Info("Skill deactivated", "id", skillID, "name", skill.Name)
	return nil
}

// ActivateByOwner activates all skills for a specific scope and owner
func (l *Loader) ActivateByOwner(ctx context.Context, scope types.SkillScope, ownerID string) (int, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return 0, types.NewError(types.ErrCodeUnavailable, "skill loader is closed")
	}

	// Get all skills for the owner
	skills, err := l.store.GetByOwner(ctx, scope, ownerID)
	if err != nil {
		return 0, err
	}

	activatedCount := 0
	now := types.NewTimestamp()

	for _, skill := range skills {
		// Update skill state
		skill.State = types.SkillStateLoaded
		skill.UpdatedAt = now
		if skill.ActivatedAt == nil {
			skill.ActivatedAt = &now
		}
		skill.Metadata.LoadCount++

		// Store the updated skill
		if err := l.store.Store(ctx, skill); err != nil {
			l.logger.Warn("Failed to activate skill", "id", skill.ID, "error", err)
			continue
		}
		activatedCount++
	}

	l.logger.Info("Bulk activation complete",
		"scope", scope,
		"owner_id", ownerID,
		"activated", activatedCount,
		"total", len(skills))

	return activatedCount, nil
}

// GetActiveSkills returns all active (loaded) skills for a scope and owner
func (l *Loader) GetActiveSkills(ctx context.Context, scope types.SkillScope, ownerID string) ([]*types.Skill, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "skill loader is closed")
	}

	// Get all skills for the owner
	skills, err := l.store.GetByOwner(ctx, scope, ownerID)
	if err != nil {
		return nil, err
	}

	// Filter for active skills
	activeSkills := make([]*types.Skill, 0)
	for _, skill := range skills {
		if skill.State == types.SkillStateLoaded {
			activeSkills = append(activeSkills, skill)
		}
	}

	return activeSkills, nil
}

// ReloadSkill reloads a skill from its file path
func (l *Loader) ReloadSkill(ctx context.Context, skillID types.ID) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return types.NewError(types.ErrCodeUnavailable, "skill loader is closed")
	}

	// Get the existing skill
	skill, err := l.store.Get(ctx, skillID)
	if err != nil {
		return err
	}

	if skill.FilePath == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "skill has no file path, cannot reload")
	}

	// Load the skill from file
	scope := skill.Scope
	ownerID := skill.OwnerID
	filePath := skill.FilePath

	updatedSkill, err := l.loadSkillFromFile(scope, ownerID, filePath)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to reload skill", err)
	}

	// Preserve the original ID, timestamps, and activation status
	updatedSkill.ID = skill.ID
	updatedSkill.CreatedAt = skill.CreatedAt
	updatedSkill.ActivatedAt = skill.ActivatedAt
	updatedSkill.Metadata.LoadCount = skill.Metadata.LoadCount
	updatedSkill.Metadata.Verified = skill.Metadata.Verified

	// Store the updated skill
	if err := l.store.Store(ctx, updatedSkill); err != nil {
		return err
	}

	l.logger.Info("Skill reloaded", "id", skillID, "name", skill.Name)
	return nil
}

// scanSkillFiles scans a directory for SKILL.md files
func (l *Loader) scanSkillFiles(dirPath string, recursive bool) ([]string, error) {
	skillFiles := make([]string, 0)

	// Check if path is excluded
	if l.isPathExcluded(dirPath) {
		return skillFiles, nil
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return skillFiles, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		fullPath := filepath.Join(dirPath, entry.Name())

		if entry.IsDir() {
			// Recursively scan subdirectories if enabled
			if recursive {
				subFiles, err := l.scanSkillFiles(fullPath, recursive)
				if err != nil {
					l.logger.Warn("Failed to scan subdirectory", "path", fullPath, "error", err)
					continue
				}
				skillFiles = append(skillFiles, subFiles...)
			}
			continue
		}

		// Check for SKILL.md file (case-insensitive)
		if strings.EqualFold(entry.Name(), "SKILL.md") || strings.EqualFold(entry.Name(), "skill.md") {
			// Check if file is excluded
			if !l.isPathExcluded(fullPath) {
				skillFiles = append(skillFiles, fullPath)
			}
		}
	}

	return skillFiles, nil
}

// isPathExcluded checks if a path matches any exclusion pattern
func (l *Loader) isPathExcluded(path string) bool {
	for _, pattern := range l.cfg.LoadConfig.ExcludedPatterns {
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
		// Also check if path contains the pattern
		if strings.Contains(path, pattern) {
			return true
		}
	}
	return false
}

// loadSkillFromFile loads a skill from a file path
func (l *Loader) loadSkillFromFile(scope types.SkillScope, ownerID, filePath string) (*types.Skill, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to read skill file", err)
	}

	skill, err := ParseFromMarkdown(data, scope, ownerID, filePath)
	if err != nil {
		return nil, err
	}

	// Set source based on file path
	if strings.Contains(filePath, ".config") || strings.Contains(filePath, "config") {
		skill.Source = types.SkillSourceBuiltin
	} else {
		skill.Source = types.SkillSourceFilesystem
	}

	return skill, nil
}

// GetLoadStats returns statistics about loaded skills for a scope and owner
func (l *Loader) GetLoadStats(ctx context.Context, scope types.SkillScope, ownerID string) (*types.SkillStats, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "skill loader is closed")
	}

	return l.store.Stats(ctx, scope, ownerID)
}

// Close closes the skill loader
func (l *Loader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	l.closed = true
	l.logger.Info("Skill loader closed")
	return nil
}

// IsClosed returns true if the loader is closed
func (l *Loader) IsClosed() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.closed
}
