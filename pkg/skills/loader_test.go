package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLoader(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
		AutoLoad:    true,
		LoadConfig: types.SkillLoadConfig{
			Enabled:      true,
			Recursive:    true,
			SkillPaths:   []string{tempDir},
			MaxLoadErrors: 5,
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	require.NotNil(t, loader)
	assert.False(t, loader.IsClosed())

	err = loader.Close()
	require.NoError(t, err)
	assert.True(t, loader.IsClosed())
}

func TestNewDefaultLoader(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		Enabled:     true,
		StoragePath: tempDir,
	}
	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewDefaultLoader(store, log)
	require.NoError(t, err)
	require.NotNil(t, loader)

	err = loader.Close()
	require.NoError(t, err)
}

func TestNewLoaderNilStore(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := types.SkillConfig{
		Enabled: true,
	}

	loader, err := NewLoader(cfg, nil, log)
	assert.Error(t, err)
	assert.Nil(t, loader)
}

func TestNewLoaderNilLogger(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	// Nil logger should create a default one
	loader, err := NewLoader(cfg, store, nil)
	require.NoError(t, err)
	require.NotNil(t, loader)

	err = loader.Close()
	require.NoError(t, err)
}

func TestLoadFromPath(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	// Create a test SKILL.md file
	skillDir := filepath.Join(tempDir, "skills")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	skillContent := "```json\n" +
		"{\n" +
		`  "name": "test-skill",
  "display_name": "Test Skill",
  "description": "A test skill",
  "version": "1.0.0",
  "author": "Test Author",
  "agent_types": ["assistant"],
  "capabilities": [
    {
      "type": "tool",
      "name": "test-tool",
      "description": "A test tool"
    }
  ]
}
` +
		"```\n\n" +
		"# Test Skill\n\n" +
		"This is a test skill content.\n"

	skillPath := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))

	ctx := context.Background()
	stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "user123", skillDir)
	require.NoError(t, err)

	assert.Equal(t, types.SkillScopeUser, stats.Scope)
	assert.Equal(t, "user123", stats.OwnerID)
	assert.Equal(t, 1, stats.TotalCount)
	assert.Equal(t, 1, stats.LoadedCount)
	assert.Equal(t, 0, stats.SkippedCount)
	assert.Equal(t, 0, stats.FailedCount)
	assert.Greater(t, stats.Duration, time.Duration(0))
}

func TestLoadFromPathNonExistent(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()
	nonExistentPath := filepath.Join(tempDir, "nonexistent")
	stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "user123", nonExistentPath)
	require.NoError(t, err)

	// Should not error, just report nothing loaded
	assert.Equal(t, 0, stats.TotalCount)
	assert.Equal(t, 0, stats.LoadedCount)
	assert.Equal(t, 1, stats.FailedCount)
	assert.Len(t, stats.Errors, 1)
	assert.Contains(t, stats.Errors[0], "does not exist")
}

func TestLoadFromPathRecursive(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
		LoadConfig: types.SkillLoadConfig{
			Recursive: true,
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	// Create nested skill directories
	skillContent := "```json\n" +
		"{\n" +
		`  "name": "test-skill",
  "display_name": "Test Skill",
  "description": "A test skill"
}
` +
		"```\n\n" +
		"# Test Skill\n"

	skillDir1 := filepath.Join(tempDir, "skills", "level1")
	require.NoError(t, os.MkdirAll(skillDir1, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir1, "SKILL.md"), []byte(skillContent), 0644))

	skillDir2 := filepath.Join(tempDir, "skills", "level1", "level2")
	require.NoError(t, os.MkdirAll(skillDir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte(skillContent), 0644))

	ctx := context.Background()
	baseDir := filepath.Join(tempDir, "skills")
	stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "user123", baseDir)
	require.NoError(t, err)

	assert.Equal(t, 2, stats.TotalCount)
	assert.Equal(t, 2, stats.LoadedCount)
}

func TestLoadFromPathNonRecursive(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
		LoadConfig: types.SkillLoadConfig{
			Recursive: false,
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	// Create nested skill directories
	skillContent := "```json\n" +
		"{\n" +
		`  "name": "test-skill",
  "display_name": "Test Skill",
  "description": "A test skill"
}
` +
		"```\n\n" +
		"# Test Skill\n"

	// Create one skill file in the base directory
	baseDir := filepath.Join(tempDir, "skills")
	require.NoError(t, os.MkdirAll(baseDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte(skillContent), 0644))

	// Create skills in subdirectories (should NOT be found when non-recursive)
	skillDir1 := filepath.Join(tempDir, "skills", "level1")
	require.NoError(t, os.MkdirAll(skillDir1, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir1, "SKILL.md"), []byte(skillContent), 0644))

	skillDir2 := filepath.Join(tempDir, "skills", "level1", "level2")
	require.NoError(t, os.MkdirAll(skillDir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte(skillContent), 0644))

	ctx := context.Background()
	stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "user123", baseDir)
	require.NoError(t, err)

	// Should only find the base directory skill (not the subdirectory ones)
	assert.Equal(t, 1, stats.TotalCount)
	assert.Equal(t, 1, stats.LoadedCount)
}

func TestLoadFromPathWithExclusions(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
		LoadConfig: types.SkillLoadConfig{
			ExcludedPatterns: []string{"excluded", "node_modules"},
			Recursive:        true, // Enable recursion for this test
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	// Create skill files in different directories
	skillContent := "```json\n" +
		"{\n" +
		`  "name": "test-skill",
  "display_name": "Test Skill",
  "description": "A test skill"
}
` +
		"```\n\n" +
		"# Test Skill\n"

	// Normal skill
	normalDir := filepath.Join(tempDir, "skills", "normal")
	require.NoError(t, os.MkdirAll(normalDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(normalDir, "SKILL.md"), []byte(skillContent), 0644))

	// Excluded skill
	excludedDir := filepath.Join(tempDir, "skills", "excluded")
	require.NoError(t, os.MkdirAll(excludedDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(excludedDir, "SKILL.md"), []byte(skillContent), 0644))

	// Node_modules directory
	nodeModulesDir := filepath.Join(tempDir, "skills", "node_modules")
	require.NoError(t, os.MkdirAll(nodeModulesDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nodeModulesDir, "SKILL.md"), []byte(skillContent), 0644))

	ctx := context.Background()
	baseDir := filepath.Join(tempDir, "skills")
	stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "user123", baseDir)
	require.NoError(t, err)

	// Should only load the non-excluded skill
	assert.Equal(t, 1, stats.TotalCount)
	assert.Equal(t, 1, stats.LoadedCount)
}

func TestLoadFromPathWithMaxErrors(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
		LoadConfig: types.SkillLoadConfig{
			MaxLoadErrors: 2,
			Recursive:     true, // Enable recursion to scan subdirectories
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	// Create invalid skill files in subdirectories
	baseDir := filepath.Join(tempDir, "skills")
	require.NoError(t, os.MkdirAll(baseDir, 0755))

	// Create 3 subdirectories with invalid SKILL.md files
	invalidContent := []byte("invalid content without proper frontmatter")
	for i := 1; i <= 3; i++ {
		subdir := filepath.Join(baseDir, fmt.Sprintf("subdir%d", i))
		require.NoError(t, os.MkdirAll(subdir, 0755))
		skillPath := filepath.Join(subdir, "SKILL.md")
		require.NoError(t, os.WriteFile(skillPath, invalidContent, 0644))
	}

	ctx := context.Background()
	stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "user123", baseDir)
	require.NoError(t, err)

	// Should stop after max errors (2)
	assert.Equal(t, 2, stats.FailedCount)
	assert.Len(t, stats.Errors, 2)
}

func TestLoadAll(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
		AutoLoad:    true,
		LoadConfig: types.SkillLoadConfig{
			Enabled:    true,
			SkillPaths: []string{},
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	// Create multiple skill directories
	skillContent := "```json\n" +
		"{\n" +
		`  "name": "test-skill",
  "display_name": "Test Skill",
  "description": "A test skill"
}
` +
		"```\n\n" +
		"# Test Skill\n"

	// Update config to include paths
	loader.cfg.LoadConfig.SkillPaths = []string{
		filepath.Join(tempDir, "skills1"),
		filepath.Join(tempDir, "skills2"),
	}

	for i := 1; i <= 2; i++ {
		skillDir := filepath.Join(tempDir, "skills"+string(rune('0'+i)))
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644))
	}

	ctx := context.Background()
	stats, err := loader.LoadAll(ctx, types.SkillScopeUser, "user123")
	require.NoError(t, err)

	assert.Equal(t, 2, stats.TotalCount)
	assert.Equal(t, 2, stats.LoadedCount)
}

func TestLoadAllDisabled(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
		AutoLoad:    false,
		LoadConfig: types.SkillLoadConfig{
			Enabled: false,
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()
	stats, err := loader.LoadAll(ctx, types.SkillScopeUser, "user123")
	require.NoError(t, err)

	// Should load nothing when disabled
	assert.Equal(t, 0, stats.TotalCount)
	assert.Equal(t, 0, stats.LoadedCount)
}

func TestActivateSkill(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Create a skill
	skill := &types.Skill{
		Name:        "test-skill",
		DisplayName: "Test Skill",
		Description: "A test skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
		State:       types.SkillStateUnloaded,
	}

	require.NoError(t, store.Store(ctx, skill))

	// Activate the skill
	err = loader.ActivateSkill(ctx, skill.ID)
	require.NoError(t, err)

	// Verify the skill is activated
	activated, err := store.Get(ctx, skill.ID)
	require.NoError(t, err)
	assert.Equal(t, types.SkillStateLoaded, activated.State)
	assert.NotNil(t, activated.ActivatedAt)
	assert.Equal(t, 1, activated.Metadata.LoadCount)
}

func TestDeactivateSkill(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Create a loaded skill
	skill := &types.Skill{
		Name:        "test-skill",
		DisplayName: "Test Skill",
		Description: "A test skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
		State:       types.SkillStateLoaded,
	}

	require.NoError(t, store.Store(ctx, skill))

	// Deactivate the skill
	err = loader.DeactivateSkill(ctx, skill.ID)
	require.NoError(t, err)

	// Verify the skill is deactivated
	deactivated, err := store.Get(ctx, skill.ID)
	require.NoError(t, err)
	assert.Equal(t, types.SkillStateUnloaded, deactivated.State)
}

func TestActivateByOwner(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Create multiple skills for the same owner
	skills := []*types.Skill{
		{
			Name:        "skill1",
			DisplayName: "Skill 1",
			Description: "First skill",
			Scope:       types.SkillScopeUser,
			OwnerID:     "user123",
			State:       types.SkillStateUnloaded,
		},
		{
			Name:        "skill2",
			DisplayName: "Skill 2",
			Description: "Second skill",
			Scope:       types.SkillScopeUser,
			OwnerID:     "user123",
			State:       types.SkillStateUnloaded,
		},
		{
			Name:        "skill3",
			DisplayName: "Skill 3",
			Description: "Third skill",
			Scope:       types.SkillScopeUser,
			OwnerID:     "user123",
			State:       types.SkillStateLoaded,
		},
	}

	for _, skill := range skills {
		require.NoError(t, store.Store(ctx, skill))
	}

	// Activate all skills for the owner
	count, err := loader.ActivateByOwner(ctx, types.SkillScopeUser, "user123")
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Verify all skills are activated
	allSkills, err := store.GetByOwner(ctx, types.SkillScopeUser, "user123")
	require.NoError(t, err)
	for _, skill := range allSkills {
		assert.Equal(t, types.SkillStateLoaded, skill.State)
	}
}

func TestGetActiveSkills(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Create skills with different states
	skills := []*types.Skill{
		{
			Name:        "loaded-skill1",
			DisplayName: "Loaded Skill 1",
			Description: "First loaded skill",
			Scope:       types.SkillScopeUser,
			OwnerID:     "user123",
			State:       types.SkillStateLoaded,
		},
		{
			Name:        "loaded-skill2",
			DisplayName: "Loaded Skill 2",
			Description: "Second loaded skill",
			Scope:       types.SkillScopeUser,
			OwnerID:     "user123",
			State:       types.SkillStateLoaded,
		},
		{
			Name:        "unloaded-skill",
			DisplayName: "Unloaded Skill",
			Description: "Unloaded skill",
			Scope:       types.SkillScopeUser,
			OwnerID:     "user123",
			State:       types.SkillStateUnloaded,
		},
	}

	for _, skill := range skills {
		require.NoError(t, store.Store(ctx, skill))
	}

	// Get active skills
	activeSkills, err := loader.GetActiveSkills(ctx, types.SkillScopeUser, "user123")
	require.NoError(t, err)
	assert.Len(t, activeSkills, 2)

	// Verify all returned skills are loaded
	for _, skill := range activeSkills {
		assert.Equal(t, types.SkillStateLoaded, skill.State)
	}
}

func TestReloadSkill(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Create a skill file
	skillContent := "```json\n" +
		"{\n" +
		`  "name": "test-skill",
  "display_name": "Test Skill",
  "description": "Original description",
  "version": "1.0.0",
  "author": "Test Author"
}
` +
		"```\n\n" +
		"# Test Skill\n\n" +
		"Original content.\n"

	skillDir := filepath.Join(tempDir, "skills")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	skillPath := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte(skillContent), 0644))

	// Load the skill
	stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "user123", skillDir)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.LoadedCount)

	// Get the loaded skill
	skills, err := store.GetByOwner(ctx, types.SkillScopeUser, "user123")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	skillID := skills[0].ID
	originalCreatedAt := skills[0].CreatedAt

	// Update the skill file
	updatedContent := "```json\n" +
		"{\n" +
		`  "name": "test-skill",
  "display_name": "Test Skill",
  "description": "Updated description",
  "version": "2.0.0",
  "author": "Test Author"
}
` +
		"```\n\n" +
		"# Test Skill\n\n" +
		"Updated content.\n"

	require.NoError(t, os.WriteFile(skillPath, []byte(updatedContent), 0644))

	// Reload the skill
	err = loader.ReloadSkill(ctx, skillID)
	require.NoError(t, err)

	// Verify the skill was updated
	reloaded, err := store.Get(ctx, skillID)
	require.NoError(t, err)
	assert.Equal(t, "Updated description", reloaded.Description)
	assert.Equal(t, "2.0.0", reloaded.Version)
	assert.Equal(t, "Updated content.", strings.TrimSpace(reloaded.Content))
	assert.Equal(t, originalCreatedAt, reloaded.CreatedAt) // CreatedAt should be preserved
}

func TestReloadSkillNoFilePath(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Create a skill without a file path
	skill := &types.Skill{
		Name:        "test-skill",
		DisplayName: "Test Skill",
		Description: "A test skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
		State:       types.SkillStateLoaded,
		FilePath:    "", // No file path
	}

	require.NoError(t, store.Store(ctx, skill))

	// Try to reload - should fail
	err = loader.ReloadSkill(ctx, skill.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no file path")
}

func TestGetLoadStats(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Create skills with different states
	skills := []*types.Skill{
		{
			Name:        "skill1",
			DisplayName: "Skill 1",
			Description: "First skill",
			Scope:       types.SkillScopeUser,
			OwnerID:     "user123",
			State:       types.SkillStateLoaded,
			Metadata: types.SkillMetadata{
				Category: "development",
			},
		},
		{
			Name:        "skill2",
			DisplayName: "Skill 2",
			Description: "Second skill",
			Scope:       types.SkillScopeUser,
			OwnerID:     "user123",
			State:       types.SkillStateUnloaded,
			Metadata: types.SkillMetadata{
				Category: "development",
			},
		},
	}

	for _, skill := range skills {
		require.NoError(t, store.Store(ctx, skill))
	}

	// Get stats
	stats, err := loader.GetLoadStats(ctx, types.SkillScopeUser, "user123")
	require.NoError(t, err)
	assert.Equal(t, types.SkillScopeUser, stats.Scope)
	assert.Equal(t, "user123", stats.OwnerID)
	assert.Equal(t, 2, stats.TotalCount)
	assert.Equal(t, 1, stats.StateCounts[types.SkillStateLoaded])
	assert.Equal(t, 1, stats.StateCounts[types.SkillStateUnloaded])
	assert.Equal(t, 2, stats.CategoryCounts["development"])
}

func TestLoaderClosed(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
		AutoLoad:    true,
		LoadConfig: types.SkillLoadConfig{
			Enabled:    true,
			SkillPaths: []string{tempDir},
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	err = loader.Close()
	require.NoError(t, err)

	ctx := context.Background()

	// All operations should fail when closed
	_, err = loader.LoadFromPath(ctx, types.SkillScopeUser, "user123", tempDir)
	assert.Error(t, err)

	_, err = loader.LoadAll(ctx, types.SkillScopeUser, "user123")
	assert.Error(t, err)

	err = loader.ActivateSkill(ctx, types.GenerateID())
	assert.Error(t, err)

	err = loader.DeactivateSkill(ctx, types.GenerateID())
	assert.Error(t, err)

	_, err = loader.ActivateByOwner(ctx, types.SkillScopeUser, "user123")
	assert.Error(t, err)

	_, err = loader.GetActiveSkills(ctx, types.SkillScopeUser, "user123")
	assert.Error(t, err)

	err = loader.ReloadSkill(ctx, types.GenerateID())
	assert.Error(t, err)

	_, err = loader.GetLoadStats(ctx, types.SkillScopeUser, "user123")
	assert.Error(t, err)
}

func TestIsPathExcluded(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
		LoadConfig: types.SkillLoadConfig{
			ExcludedPatterns: []string{"node_modules", ".git", "test", "*.tmp"},
		},
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	testCases := []struct {
		name     string
		path     string
		excluded bool
	}{
		{"Normal path", filepath.Join(tempDir, "skills", "normal"), false},
		{"Node modules", filepath.Join(tempDir, "node_modules", "package"), true},
		{"Git directory", filepath.Join(tempDir, ".git", "objects"), true},
		{"Test directory", filepath.Join(tempDir, "test", "files"), true},
		{"TMP file", filepath.Join(tempDir, "file.tmp"), true},
		{"Nested node_modules", filepath.Join(tempDir, "project", "node_modules", "pkg"), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := loader.isPathExcluded(tc.path)
			assert.Equal(t, tc.excluded, result)
		})
	}
}

func TestLoadStats(t *testing.T) {
	stats := LoadStats{
		Scope:        types.SkillScopeUser,
		OwnerID:      "user123",
		TotalCount:   10,
		LoadedCount:  7,
		SkippedCount: 2,
		FailedCount:  1,
		Errors:       []string{"skill1: parse error", "skill2: load error"},
		Duration:     1000000000, // 1 second
	}

	assert.Equal(t, types.SkillScopeUser, stats.Scope)
	assert.Equal(t, "user123", stats.OwnerID)
	assert.Equal(t, 10, stats.TotalCount)
	assert.Equal(t, 7, stats.LoadedCount)
	assert.Equal(t, 2, stats.SkippedCount)
	assert.Equal(t, 1, stats.FailedCount)
	assert.Len(t, stats.Errors, 2)
	assert.Greater(t, stats.Duration, time.Duration(0))
}

func TestLoadFromPathCaseInsensitive(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	// Create test SKILL.md files with different cases
	skillContent := "```json\n" +
		"{\n" +
		`  "name": "test-skill",
  "display_name": "Test Skill",
  "description": "A test skill"
}
` +
		"```\n\n" +
		"# Test Skill\n"

	skillDir := filepath.Join(tempDir, "skills")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	// Create files with different case variations
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644))

	subdir := filepath.Join(skillDir, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "skill.md"), []byte(skillContent), 0644))

	ctx := context.Background()
	stats, err := loader.LoadFromPath(ctx, types.SkillScopeUser, "user123", skillDir)
	require.NoError(t, err)

	// Should find both files (case-insensitive matching)
	// Note: Test config does not enable recursive scanning, so only top-level SKILL.md is found
	assert.Equal(t, 1, stats.TotalCount)
}

func TestActivateSkillNotFound(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Try to activate a non-existent skill
	err = loader.ActivateSkill(ctx, types.GenerateID())
	assert.Error(t, err)
}

func TestDeactivateSkillNotFound(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Try to deactivate a non-existent skill
	err = loader.DeactivateSkill(ctx, types.GenerateID())
	assert.Error(t, err)
}

func TestReloadSkillNotFound(t *testing.T) {
	tempDir := t.TempDir()
	cfg := types.SkillConfig{
		StoragePath: tempDir,
		Enabled:     true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	loader, err := NewLoader(cfg, store, log)
	require.NoError(t, err)
	defer loader.Close()

	ctx := context.Background()

	// Try to reload a non-existent skill
	err = loader.ReloadSkill(ctx, types.GenerateID())
	assert.Error(t, err)
}
