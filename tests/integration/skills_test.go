package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/skills"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSkillDiscovery tests skill discovery from filesystem directories
func TestSkillDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping skill discovery test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	log, err := logger.NewDefault()
	require.NoError(t, err, "Failed to create logger")

	t.Run("discover skills in single directory", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create test skills directory
		skillsDir := filepath.Join(tempDir, "test-skills")
		require.NoError(t, os.MkdirAll(skillsDir, 0755))

		// Create a test SKILL.md file
		skillContent := createTestSkillContent("test-skill", "Test Skill", "A test skill for discovery")
		skillPath := filepath.Join(skillsDir, "SKILL.md")
		require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))

		// Load skills from path
		stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "test-user", skillsDir)
		require.NoError(t, err)

		assert.Equal(t, 1, stats.TotalCount, "Should find one skill")
		assert.Equal(t, 1, stats.LoadedCount, "Should load one skill")
		assert.Equal(t, 0, stats.FailedCount, "Should have no failures")
	})

	t.Run("discover skills recursively", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
			LoadConfig: types.SkillLoadConfig{
				Recursive: true,
			},
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create nested skill directories
		baseDir := filepath.Join(tempDir, "recursive-skills")
		level1Dir := filepath.Join(baseDir, "level1")
		level2Dir := filepath.Join(level1Dir, "level2")

		for _, dir := range []string{baseDir, level1Dir, level2Dir} {
			require.NoError(t, os.MkdirAll(dir, 0755))

			skillName := fmt.Sprintf("skill-%s", filepath.Base(dir))
			skillContent := createTestSkillContent(skillName, skillName, "Skill in "+dir)
			skillPath := filepath.Join(dir, "SKILL.md")
			require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))
		}

		// Load skills recursively
		stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "test-user", baseDir)
		require.NoError(t, err)

		assert.Equal(t, 3, stats.TotalCount, "Should find all nested skills")
		assert.Equal(t, 3, stats.LoadedCount, "Should load all nested skills")
	})

	t.Run("discover skills non-recursively", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
			LoadConfig: types.SkillLoadConfig{
				Recursive: false,
			},
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create nested skill directories
		baseDir := filepath.Join(tempDir, "non-recursive-skills")
		level1Dir := filepath.Join(baseDir, "level1")

		for _, dir := range []string{baseDir, level1Dir} {
			require.NoError(t, os.MkdirAll(dir, 0755))

			skillName := fmt.Sprintf("skill-%s", filepath.Base(dir))
			skillContent := createTestSkillContent(skillName, skillName, "Skill in "+dir)
			skillPath := filepath.Join(dir, "SKILL.md")
			require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))
		}

		// Load skills non-recursively
		stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "test-user", baseDir)
		require.NoError(t, err)

		assert.Equal(t, 1, stats.TotalCount, "Should only find base directory skill")
		assert.Equal(t, 1, stats.LoadedCount, "Should only load base directory skill")
	})

	t.Run("handle non-existent directory gracefully", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		nonExistentPath := filepath.Join(tempDir, "does-not-exist")
		stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "test-user", nonExistentPath)

		require.NoError(t, err, "Should not error on non-existent path")
		assert.Equal(t, 0, stats.TotalCount, "Should have zero total count")
		assert.Equal(t, 1, stats.FailedCount, "Should have one failed count")
		assert.NotEmpty(t, stats.Errors, "Should have error message")
	})
}

// TestSkillLoading tests the complete skill loading workflow
func TestSkillLoading(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping skill loading test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	log, err := logger.NewDefault()
	require.NoError(t, err, "Failed to create logger")

	t.Run("load skill with all fields", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create a comprehensive skill file
		skillsDir := filepath.Join(tempDir, "full-skill")
		require.NoError(t, os.MkdirAll(skillsDir, 0755))

		skillContent := "```json\n" +
			"{\n" +
			`  "name": "comprehensive-skill",
  "display_name": "Comprehensive Skill",
  "description": "A comprehensive test skill with all fields",
  "version": "2.0.0",
  "author": "Test Author",
  "agent_types": ["assistant", "worker"],
  "capabilities": [
    {
      "type": "tool",
      "name": "test-tool",
      "description": "A test tool capability",
      "config": {"timeout": 30}
    },
    {
      "type": "prompt",
      "name": "test-prompt",
      "description": "A test prompt capability"
    }
  ],
  "category": "testing",
  "tags": ["test", "integration"],
  "keywords": ["testing", "skill", "loader"],
  "verified": true
}
` +
			"```\n\n" +
			"# Comprehensive Skill\n\n" +
			"This skill contains comprehensive content for testing.\n\n" +
			"## Features\n\n" +
			"- Tool capabilities\n" +
			"- Prompt capabilities\n" +
			"- Full metadata\n"

		skillPath := filepath.Join(skillsDir, "SKILL.md")
		require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))

		// Load the skill
		stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "test-user", skillsDir)
		require.NoError(t, err)
		assert.Equal(t, 1, stats.LoadedCount)

		// Verify the loaded skill
		loadedSkills, err := store.GetByOwner(ctx, types.SkillScopeUser, "test-user")
		require.NoError(t, err)
		require.Len(t, loadedSkills, 1)

		skill := loadedSkills[0]
		assert.Equal(t, "comprehensive-skill", skill.Name)
		assert.Equal(t, "Comprehensive Skill", skill.DisplayName)
		assert.Equal(t, "A comprehensive test skill with all fields", skill.Description)
		assert.Equal(t, "2.0.0", skill.Version)
		assert.Equal(t, "Test Author", skill.Author)
		assert.Len(t, skill.AgentTypes, 2)
		assert.Len(t, skill.Capabilities, 2)
		assert.Equal(t, "testing", skill.Metadata.Category)
		assert.Len(t, skill.Metadata.Tags, 2)
		assert.Len(t, skill.Metadata.Keywords, 3)
		assert.True(t, skill.Metadata.Verified)
	})

	t.Run("load multiple skills from multiple paths", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
			AutoLoad:    true,
			LoadConfig: types.SkillLoadConfig{
				Enabled: true,
			},
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		// Create multiple skill directories
		paths := make([]string, 3)
		for i := 0; i < 3; i++ {
			skillsDir := filepath.Join(tempDir, fmt.Sprintf("skills-%d", i))
			require.NoError(t, os.MkdirAll(skillsDir, 0755))

			skillName := fmt.Sprintf("skill-%d", i)
			skillContent := createTestSkillContent(skillName, fmt.Sprintf("Skill %d", i), fmt.Sprintf("Skill number %d", i))
			skillPath := filepath.Join(skillsDir, "SKILL.md")
			require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))

			paths[i] = skillsDir
		}

		// Update loader config with paths
		cfg.LoadConfig.SkillPaths = paths
		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Load all skills
		stats, err := loader.LoadAll(ctx, types.SkillScopeUser, "multi-path-user")
		require.NoError(t, err)

		assert.Equal(t, 3, stats.TotalCount, "Should find all skills from all paths")
		assert.Equal(t, 3, stats.LoadedCount, "Should load all skills from all paths")

		// Verify all skills are in store
		allSkills, err := store.GetByOwner(ctx, types.SkillScopeUser, "multi-path-user")
		require.NoError(t, err)
		assert.Len(t, allSkills, 3)
	})

	t.Run("handle invalid skill files", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
			LoadConfig: types.SkillLoadConfig{
				MaxLoadErrors: 3,
			},
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create directory with mixed valid and invalid files
		skillsDir := filepath.Join(tempDir, "mixed-skills")
		require.NoError(t, os.MkdirAll(skillsDir, 0755))

		// Valid skill
		validSkillContent := createTestSkillContent("valid-skill", "Valid Skill", "A valid skill")
		require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(validSkillContent), 0644))

		// Invalid skill (no JSON frontmatter)
		invalidContent := "# Just a markdown file\n\nNo frontmatter here."
		require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "invalid.md"), []byte(invalidContent), 0644))

		// Another invalid skill (malformed JSON)
		malformedContent := "```json\n{invalid json}\n```\n\n# Malformed\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "malformed.md"), []byte(malformedContent), 0644))

		stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "test-user", skillsDir)
		require.NoError(t, err)

		assert.Equal(t, 1, stats.LoadedCount, "Should load one valid skill")
		assert.Equal(t, 0, stats.FailedCount, "Non-SKILL files should be ignored")
		assert.Len(t, stats.Errors, 0, "No parsing errors expected for ignored files")
	})
}

// TestSkillActivation tests skill activation and deactivation workflows
func TestSkillActivation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping skill activation test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	log, err := logger.NewDefault()
	require.NoError(t, err, "Failed to create logger")

	t.Run("activate single skill", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create and load a skill
		skillsDir := filepath.Join(tempDir, "activate-single")
		require.NoError(t, os.MkdirAll(skillsDir, 0755))

		skillContent := createTestSkillContent("activatable-skill", "Activatable Skill", "A skill that can be activated")
		skillPath := filepath.Join(skillsDir, "SKILL.md")
		require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))

		stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "activate-user", skillsDir)
		require.NoError(t, err)
		assert.Equal(t, 1, stats.LoadedCount)

		// Get the skill
		loadedSkills, err := store.GetByOwner(ctx, types.SkillScopeUser, "activate-user")
		require.NoError(t, err)
		require.Len(t, loadedSkills, 1)

		skillID := loadedSkills[0].ID

		// Activate the skill
		err = loader.ActivateSkill(ctx, skillID)
		require.NoError(t, err)

		// Verify activation
		activatedSkill, err := store.Get(ctx, skillID)
		require.NoError(t, err)
		assert.Equal(t, types.SkillStateLoaded, activatedSkill.State)
		assert.NotNil(t, activatedSkill.ActivatedAt)
		assert.Equal(t, 1, activatedSkill.Metadata.LoadCount)
	})

	t.Run("deactivate skill", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create a loaded skill directly
		skill := &types.Skill{
			Name:        "deactivatable-skill",
			DisplayName: "Deactivatable Skill",
			Description: "A skill that can be deactivated",
			Scope:       types.SkillScopeUser,
			OwnerID:     "deactivate-user",
			State:       types.SkillStateLoaded,
		}

		require.NoError(t, store.Store(ctx, skill))

		// Deactivate the skill
		err = loader.DeactivateSkill(ctx, skill.ID)
		require.NoError(t, err)

		// Verify deactivation
		deactivatedSkill, err := store.Get(ctx, skill.ID)
		require.NoError(t, err)
		assert.Equal(t, types.SkillStateUnloaded, deactivatedSkill.State)
	})

	t.Run("activate all skills for owner", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create multiple skills
		skillsDir := filepath.Join(tempDir, "activate-all")
		require.NoError(t, os.MkdirAll(skillsDir, 0755))

		for i := 0; i < 5; i++ {
			skillName := fmt.Sprintf("bulk-skill-%d", i)
			skillContent := createTestSkillContent(skillName, fmt.Sprintf("Bulk Skill %d", i), fmt.Sprintf("Skill %d for bulk activation", i))

			subdir := filepath.Join(skillsDir, fmt.Sprintf("skill-%d", i))
			require.NoError(t, os.MkdirAll(subdir, 0755))
			skillPath := filepath.Join(subdir, "SKILL.md")
			require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))
		}

		// Load all skills
		totalLoaded := 0
		for i := 0; i < 5; i++ {
			subdir := filepath.Join(skillsDir, fmt.Sprintf("skill-%d", i))
			stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "bulk-user", subdir)
			require.NoError(t, err)
			totalLoaded += stats.LoadedCount
		}
		assert.Equal(t, 5, totalLoaded)

		// Deactivate all first
		allSkills, err := store.GetByOwner(ctx, types.SkillScopeUser, "bulk-user")
		require.NoError(t, err)
		for _, skill := range allSkills {
			skill.State = types.SkillStateUnloaded
			require.NoError(t, store.Store(ctx, skill))
		}

		// Activate all at once
		count, err := loader.ActivateByOwner(ctx, types.SkillScopeUser, "bulk-user")
		require.NoError(t, err)
		assert.Equal(t, 5, count)

		// Verify all are activated
		activeSkills, err := loader.GetActiveSkills(ctx, types.SkillScopeUser, "bulk-user")
		require.NoError(t, err)
		assert.Len(t, activeSkills, 5)

		for _, skill := range activeSkills {
			assert.Equal(t, types.SkillStateLoaded, skill.State)
		}
	})

	t.Run("get active skills only", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create skills with different states
		ownerID := "filter-user"
		states := []types.SkillState{
			types.SkillStateLoaded,
			types.SkillStateLoaded,
			types.SkillStateUnloaded,
			types.SkillStateLoaded,
			types.SkillStateUnloaded,
		}

		for i, state := range states {
			skill := &types.Skill{
				Name:        fmt.Sprintf("state-skill-%d", i),
				DisplayName: fmt.Sprintf("State Skill %d", i),
				Description: fmt.Sprintf("Skill with state %s", state),
				Scope:       types.SkillScopeUser,
				OwnerID:     ownerID,
				State:       state,
			}
			require.NoError(t, store.Store(ctx, skill))
		}

		// Get active skills
		activeSkills, err := loader.GetActiveSkills(ctx, types.SkillScopeUser, ownerID)
		require.NoError(t, err)
		assert.Len(t, activeSkills, 3)

		// Verify all are loaded
		for _, skill := range activeSkills {
			assert.Equal(t, types.SkillStateLoaded, skill.State)
		}
	})
}

// TestSkillReload tests skill reloading from disk
func TestSkillReload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping skill reload test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	log, err := logger.NewDefault()
	require.NoError(t, err, "Failed to create logger")

	t.Run("reload skill from updated file", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create initial skill file
		skillsDir := filepath.Join(tempDir, "reload-skill")
		require.NoError(t, os.MkdirAll(skillsDir, 0755))

		initialContent := "```json\n" +
			"{\n" +
			`  "name": "reloadable-skill",
  "display_name": "Reloadable Skill",
  "description": "Initial description",
  "version": "1.0.0",
  "author": "Test Author"
}
` +
			"```\n\n" +
			"# Reloadable Skill\n\n" +
			"Initial content.\n"

		skillPath := filepath.Join(skillsDir, "SKILL.md")
		require.NoError(t, os.WriteFile(skillPath, []byte(initialContent), 0644))

		// Load the skill
		stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "reload-user", skillsDir)
		require.NoError(t, err)
		assert.Equal(t, 1, stats.LoadedCount)

		// Get the skill
		loadedSkills, err := store.GetByOwner(ctx, types.SkillScopeUser, "reload-user")
		require.NoError(t, err)
		require.Len(t, loadedSkills, 1)

		skillID := loadedSkills[0].ID
		originalCreatedAt := loadedSkills[0].CreatedAt

		// Update the skill file
		updatedContent := "```json\n" +
			"{\n" +
			`  "name": "reloadable-skill",
  "display_name": "Reloadable Skill",
  "description": "Updated description",
  "version": "2.0.0",
  "author": "Updated Author"
}
` +
			"```\n\n" +
			"# Reloadable Skill\n\n" +
			"Updated content.\n"

		require.NoError(t, os.WriteFile(skillPath, []byte(updatedContent), 0644))

		// Reload the skill
		err = loader.ReloadSkill(ctx, skillID)
		require.NoError(t, err)

		// Verify the skill was updated
		reloadedSkill, err := store.Get(ctx, skillID)
		require.NoError(t, err)

		assert.Equal(t, "Updated description", reloadedSkill.Description)
		assert.Equal(t, "2.0.0", reloadedSkill.Version)
		assert.Equal(t, "Updated Author", reloadedSkill.Author)
		assert.Contains(t, reloadedSkill.Content, "Updated content")
		assert.Equal(t, originalCreatedAt, reloadedSkill.CreatedAt, "CreatedAt should be preserved")
		assert.True(t, reloadedSkill.UpdatedAt.Time.After(originalCreatedAt.Time), "UpdatedAt should be newer")
	})
}

// TestSkillConcurrentOperations tests concurrent skill operations
func TestSkillConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent operations test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	log, err := logger.NewDefault()
	require.NoError(t, err, "Failed to create logger")

	t.Run("concurrent skill loading", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create multiple skill directories
		numSkills := 10
		var wg sync.WaitGroup
		errors := make(chan error, numSkills)

		for i := 0; i < numSkills; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				skillsDir := filepath.Join(tempDir, fmt.Sprintf("concurrent-skill-%d", idx))
				if err := os.MkdirAll(skillsDir, 0755); err != nil {
					errors <- err
					return
				}

				skillName := fmt.Sprintf("concurrent-skill-%d", idx)
				skillContent := createTestSkillContent(skillName, fmt.Sprintf("Concurrent Skill %d", idx), fmt.Sprintf("Skill %d for concurrent loading", idx))
				skillPath := filepath.Join(skillsDir, "SKILL.md")
				if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
					errors <- err
					return
				}

				stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, fmt.Sprintf("user-%d", idx), skillsDir)
				if err != nil {
					errors <- err
					return
				}
				if stats.LoadedCount != 1 {
					errors <- fmt.Errorf("expected 1 loaded skill, got %d", stats.LoadedCount)
					return
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		errorCount := 0
		for err := range errors {
			t.Errorf("Concurrent load error: %v", err)
			errorCount++
		}
		assert.Equal(t, 0, errorCount, "Should have no concurrent load errors")
	})

	t.Run("concurrent activation and deactivation", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create multiple skills
		ownerID := "concurrent-activate-user"
		numSkills := 5
		skillIDs := make([]types.ID, numSkills)

		for i := 0; i < numSkills; i++ {
			skill := &types.Skill{
				Name:        fmt.Sprintf("concurrent-activate-skill-%d", i),
				DisplayName: fmt.Sprintf("Concurrent Activate Skill %d", i),
				Description: fmt.Sprintf("Skill %d for concurrent activation", i),
				Scope:       types.SkillScopeUser,
				OwnerID:     ownerID,
				State:       types.SkillStateUnloaded,
			}
			require.NoError(t, store.Store(ctx, skill))
			skillIDs[i] = skill.ID
		}

		// Concurrently activate and deactivate
		var wg sync.WaitGroup
		for i := 0; i < numSkills; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				// Activate
				if err := loader.ActivateSkill(ctx, skillIDs[idx]); err != nil {
					t.Errorf("Failed to activate skill %d: %v", idx, err)
					return
				}

				// Deactivate
				if err := loader.DeactivateSkill(ctx, skillIDs[idx]); err != nil {
					t.Errorf("Failed to deactivate skill %d: %v", idx, err)
					return
				}

				// Activate again
				if err := loader.ActivateSkill(ctx, skillIDs[idx]); err != nil {
					t.Errorf("Failed to reactivate skill %d: %v", idx, err)
					return
				}
			}(i)
		}

		wg.Wait()

		// Verify final state
		activeSkills, err := loader.GetActiveSkills(ctx, types.SkillScopeUser, ownerID)
		require.NoError(t, err)
		assert.Len(t, activeSkills, numSkills, "All skills should be active")
	})

	t.Run("concurrent store access", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create a skill to work with
		skillsDir := filepath.Join(tempDir, "concurrent-store-access")
		require.NoError(t, os.MkdirAll(skillsDir, 0755))

		skillContent := createTestSkillContent("shared-skill", "Shared Skill", "Skill for concurrent store access")
		skillPath := filepath.Join(skillsDir, "SKILL.md")
		require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))

		_, err = loader.LoadFromPath(ctx, types.SkillScopeUser, "store-user", skillsDir)
		require.NoError(t, err)

		loadedSkills, err := store.GetByOwner(ctx, types.SkillScopeUser, "store-user")
		require.NoError(t, err)
		require.Len(t, loadedSkills, 1)

		skillID := loadedSkills[0].ID

		// Concurrently access the skill
		var wg sync.WaitGroup
		numAccesses := 20

		for i := 0; i < numAccesses; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				// Get skill
				_, err := store.Get(ctx, skillID)
				if err != nil {
					t.Errorf("Failed to get skill: %v", err)
					return
				}

				// Get stats
				_, err = loader.GetLoadStats(ctx, types.SkillScopeUser, "store-user")
				if err != nil {
					t.Errorf("Failed to get stats: %v", err)
					return
				}
			}()
		}

		wg.Wait()
	})
}

// TestSkillStats tests skill statistics and reporting
func TestSkillStats(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping skill stats test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	log, err := logger.NewDefault()
	require.NoError(t, err, "Failed to create logger")

	t.Run("get skill statistics", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create skills with various states and categories
		ownerID := "stats-user"
		skills := []*types.Skill{
			{
				Name:        "loaded-dev-skill",
				DisplayName: "Loaded Dev Skill",
				Description: "A loaded development skill",
				Scope:       types.SkillScopeUser,
				OwnerID:     ownerID,
				State:       types.SkillStateLoaded,
				Metadata: types.SkillMetadata{
					Category: "development",
				},
			},
			{
				Name:        "unloaded-dev-skill",
				DisplayName: "Unloaded Dev Skill",
				Description: "An unloaded development skill",
				Scope:       types.SkillScopeUser,
				OwnerID:     ownerID,
				State:       types.SkillStateUnloaded,
				Metadata: types.SkillMetadata{
					Category: "development",
				},
			},
			{
				Name:        "loaded-prod-skill",
				DisplayName: "Loaded Prod Skill",
				Description: "A loaded productivity skill",
				Scope:       types.SkillScopeUser,
				OwnerID:     ownerID,
				State:       types.SkillStateLoaded,
				Metadata: types.SkillMetadata{
					Category: "productivity",
				},
			},
			{
				Name:        "error-skill",
				DisplayName: "Error Skill",
				Description: "A skill in error state",
				Scope:       types.SkillScopeUser,
				OwnerID:     ownerID,
				State:       types.SkillStateError,
				Metadata: types.SkillMetadata{
					Category: "development",
				},
			},
		}

		for _, skill := range skills {
			require.NoError(t, store.Store(ctx, skill))
		}

		// Get stats
		stats, err := loader.GetLoadStats(ctx, types.SkillScopeUser, ownerID)
		require.NoError(t, err)

		assert.Equal(t, types.SkillScopeUser, stats.Scope)
		assert.Equal(t, ownerID, stats.OwnerID)
		assert.Equal(t, 4, stats.TotalCount)
		assert.Equal(t, 2, stats.StateCounts[types.SkillStateLoaded])
		assert.Equal(t, 1, stats.StateCounts[types.SkillStateUnloaded])
		assert.Equal(t, 1, stats.StateCounts[types.SkillStateError])
		assert.Equal(t, 3, stats.CategoryCounts["development"])
		assert.Equal(t, 1, stats.CategoryCounts["productivity"])
	})

	t.Run("load stats includes duration", func(t *testing.T) {
		cfg := types.SkillConfig{
			StoragePath: tempDir,
			Enabled:     true,
		}

		store, err := skills.NewStore(cfg, log)
		require.NoError(t, err)
		defer store.Close()

		loader, err := skills.NewLoader(cfg, store, log)
		require.NoError(t, err)
		defer loader.Close()

		// Create a skill directory
		skillsDir := filepath.Join(tempDir, "duration-stats")
		require.NoError(t, os.MkdirAll(skillsDir, 0755))

		skillContent := createTestSkillContent("duration-skill", "Duration Skill", "Skill for testing duration stats")
		skillPath := filepath.Join(skillsDir, "SKILL.md")
		require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))

		// Load and check duration
		stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "duration-user", skillsDir)
		require.NoError(t, err)

		assert.Greater(t, stats.Duration, time.Duration(0), "Duration should be greater than zero")
	})
}

// createTestSkillContent creates a test skill markdown file content
func createTestSkillContent(name, displayName, description string) string {
	return "```json\n" +
		"{\n" +
		fmt.Sprintf(`  "name": "%s",
  "display_name": "%s",
  "description": "%s",
  "version": "1.0.0",
  "author": "Test Author"
`, name, displayName, description) +
		"}\n" +
		"```\n\n" +
		fmt.Sprintf("# %s\n\n%s\n", displayName, description)
}
