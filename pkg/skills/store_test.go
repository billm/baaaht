package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	require.NotNil(t, store)

	defer store.Close()

	assert.False(t, store.IsClosed())
}

func TestNewDefaultStore(t *testing.T) {
	store, err := NewDefaultStore(nil)
	require.NoError(t, err)
	require.NotNil(t, store)

	defer store.Close()

	assert.False(t, store.IsClosed())
}

func TestStoreSkill(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	skill := &types.Skill{
		Name:        "test-skill",
		DisplayName: "Test Skill",
		Description: "A test skill for unit testing",
		Version:     "1.0.0",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
		State:       types.SkillStateLoaded,
		Source:      types.SkillSourceFilesystem,
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAssistant},
		Capabilities: []types.SkillCapability{
			{
				Type:        types.SkillCapabilityTypeTool,
				Name:        "test_tool",
				Description: "A test tool",
			},
		},
		Metadata: types.SkillMetadata{
			Category: "testing",
			Tags:     []string{"test", "unit-test"},
		},
	}

	err = store.Store(ctx, skill)
	require.NoError(t, err)

	// Verify ID was generated
	assert.NotEmpty(t, skill.ID)

	// Verify file was created
	filePath := filepath.Join(tempDir, sanitizeOwnerID(skill.OwnerID), store.generateFilename(skill))
	_, err = os.Stat(filePath)
	require.NoError(t, err)
}

func TestStoreSkillValidation(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	tests := []struct {
		name    string
		skill   *types.Skill
		wantErr string
	}{
		{
			name: "missing name",
			skill: &types.Skill{
				DisplayName: "Test",
				Description: "Test description",
				Scope:       types.SkillScopeUser,
				OwnerID:     "user123",
			},
			wantErr: "skill name is required",
		},
		{
			name: "missing display_name",
			skill: &types.Skill{
				Name:        "test",
				Description: "Test description",
				Scope:       types.SkillScopeUser,
				OwnerID:     "user123",
			},
			wantErr: "skill display_name is required",
		},
		{
			name: "missing description",
			skill: &types.Skill{
				Name:        "test",
				DisplayName: "Test",
				Scope:       types.SkillScopeUser,
				OwnerID:     "user123",
			},
			wantErr: "skill description is required",
		},
		{
			name: "missing scope",
			skill: &types.Skill{
				Name:        "test",
				DisplayName: "Test",
				Description: "Test description",
				OwnerID:     "user123",
			},
			wantErr: "skill scope is required",
		},
		{
			name: "missing owner_id",
			skill: &types.Skill{
				Name:        "test",
				DisplayName: "Test",
				Description: "Test description",
				Scope:       types.SkillScopeUser,
			},
			wantErr: "skill owner_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Store(ctx, tt.skill)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestGetSkill(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	skill := &types.Skill{
		Name:        "get-test",
		DisplayName: "Get Test Skill",
		Description: "Testing get functionality",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
	}

	err = store.Store(ctx, skill)
	require.NoError(t, err)

	// Get the skill
	retrieved, err := store.Get(ctx, skill.ID)
	require.NoError(t, err)
	assert.Equal(t, skill.ID, retrieved.ID)
	assert.Equal(t, skill.Name, retrieved.Name)
	assert.Equal(t, skill.DisplayName, retrieved.DisplayName)
	assert.Equal(t, skill.Description, retrieved.Description)

	// Try to get non-existent skill
	_, err = store.Get(ctx, types.NewID("nonexistent"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill not found")
}

func TestGetByOwner(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Store skills for two different owners
	skill1 := &types.Skill{
		Name:        "skill1",
		DisplayName: "Skill 1",
		Description: "First skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
	}

	skill2 := &types.Skill{
		Name:        "skill2",
		DisplayName: "Skill 2",
		Description: "Second skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
	}

	skill3 := &types.Skill{
		Name:        "skill3",
		DisplayName: "Skill 3",
		Description: "Third skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user2",
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
	}

	err = store.Store(ctx, skill1)
	require.NoError(t, err)
	err = store.Store(ctx, skill2)
	require.NoError(t, err)
	err = store.Store(ctx, skill3)
	require.NoError(t, err)

	// Get skills for user1
	skills, err := store.GetByOwner(ctx, types.SkillScopeUser, "user1")
	require.NoError(t, err)
	assert.Len(t, skills, 2)

	// Get skills for user2
	skills, err = store.GetByOwner(ctx, types.SkillScopeUser, "user2")
	require.NoError(t, err)
	assert.Len(t, skills, 1)
}

func TestListSkills(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Store skills with different properties
	skill1 := &types.Skill{
		Name:        "loaded-skill",
		DisplayName: "Loaded Skill",
		Description: "A loaded skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		State:       types.SkillStateLoaded,
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
		Metadata: types.SkillMetadata{
			Category: "development",
			Tags:     []string{"dev", "coding"},
		},
	}

	skill2 := &types.Skill{
		Name:        "disabled-skill",
		DisplayName: "Disabled Skill",
		Description: "A disabled skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		State:       types.SkillStateDisabled,
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
		Metadata: types.SkillMetadata{
			Category: "development",
		},
	}

	skill3 := &types.Skill{
		Name:        "error-skill",
		DisplayName: "Error Skill",
		Description: "A skill in error state",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		State:       types.SkillStateError,
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
		Metadata: types.SkillMetadata{
			Category: "utility",
		},
	}

	err = store.Store(ctx, skill1)
	require.NoError(t, err)
	err = store.Store(ctx, skill2)
	require.NoError(t, err)
	err = store.Store(ctx, skill3)
	require.NoError(t, err)

	// Filter by state
	loadedState := types.SkillStateLoaded
	filter := &types.SkillFilter{
		Scope:    &[]types.SkillScope{types.SkillScopeUser}[0],
		OwnerID:  &[]string{"user1"}[0],
		State:    &loadedState,
	}

	skills, err := store.List(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, skills, 1)
	assert.Equal(t, "loaded-skill", skills[0].Name)

	// Filter by category
	category := "development"
	filter = &types.SkillFilter{
		Scope:    &[]types.SkillScope{types.SkillScopeUser}[0],
		OwnerID:  &[]string{"user1"}[0],
		Category: &category,
	}

	skills, err = store.List(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, skills, 2)

	// Filter by tags
	filter = &types.SkillFilter{
		Scope:   &[]types.SkillScope{types.SkillScopeUser}[0],
		OwnerID: &[]string{"user1"}[0],
		Tags:    []string{"coding"},
	}

	skills, err = store.List(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, skills, 1)
}

func TestDeleteSkill(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	skill := &types.Skill{
		Name:        "delete-test",
		DisplayName: "Delete Test Skill",
		Description: "Testing delete functionality",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
	}

	err = store.Store(ctx, skill)
	require.NoError(t, err)

	// Verify skill exists
	_, err = store.Get(ctx, skill.ID)
	require.NoError(t, err)

	// Delete the skill
	err = store.Delete(ctx, skill.ID)
	require.NoError(t, err)

	// Verify skill is gone
	_, err = store.Get(ctx, skill.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill not found")

	// Verify file was deleted
	filePath := filepath.Join(tempDir, sanitizeOwnerID(skill.OwnerID), store.generateFilename(skill))
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}

func TestStats(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Store skills
	skill1 := &types.Skill{
		Name:        "skill1",
		DisplayName: "Skill 1",
		Description: "First skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		State:       types.SkillStateLoaded,
		Source:      types.SkillSourceFilesystem,
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
		Metadata: types.SkillMetadata{
			Category: "dev",
		},
	}

	skill2 := &types.Skill{
		Name:        "skill2",
		DisplayName: "Skill 2",
		Description: "Second skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		State:       types.SkillStateLoaded,
		Source:      types.SkillSourceGitHub,
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
		Metadata: types.SkillMetadata{
			Category: "dev",
		},
	}

	skill3 := &types.Skill{
		Name:        "skill3",
		DisplayName: "Skill 3",
		Description: "Third skill",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		State:       types.SkillStateDisabled,
		Source:      types.SkillSourceFilesystem,
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
		Metadata: types.SkillMetadata{
			Category: "utility",
		},
	}

	err = store.Store(ctx, skill1)
	require.NoError(t, err)
	err = store.Store(ctx, skill2)
	require.NoError(t, err)
	err = store.Store(ctx, skill3)
	require.NoError(t, err)

	// Get stats
	stats, err := store.Stats(ctx, types.SkillScopeUser, "user1")
	require.NoError(t, err)

	assert.Equal(t, types.SkillScopeUser, stats.Scope)
	assert.Equal(t, "user1", stats.OwnerID)
	assert.Equal(t, 3, stats.TotalCount)
	assert.Equal(t, 2, stats.StateCounts[types.SkillStateLoaded])
	assert.Equal(t, 1, stats.StateCounts[types.SkillStateDisabled])
	assert.Equal(t, 2, stats.SourceCounts[types.SkillSourceFilesystem])
	assert.Equal(t, 1, stats.SourceCounts[types.SkillSourceGitHub])
	assert.Equal(t, 2, stats.CategoryCounts["dev"])
	assert.Equal(t, 1, stats.CategoryCounts["utility"])
}

func TestUpdateSkill(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	skill := &types.Skill{
		Name:        "update-test",
		DisplayName: "Update Test Skill",
		Description: "Original description",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
	}

	err = store.Store(ctx, skill)
	require.NoError(t, err)

	// Update the skill
	skill.DisplayName = "Updated Test Skill"
	skill.Description = "Updated description"
	skill.Metadata.Tags = []string{"updated"}

	err = store.Store(ctx, skill)
	require.NoError(t, err)

	// Verify the update
	retrieved, err := store.Get(ctx, skill.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Test Skill", retrieved.DisplayName)
	assert.Equal(t, "Updated description", retrieved.Description)
	assert.Contains(t, retrieved.Metadata.Tags, "updated")
}

func TestCloseStore(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)

	assert.False(t, store.IsClosed())

	err = store.Close()
	require.NoError(t, err)

	assert.True(t, store.IsClosed())

	// Close again should be idempotent
	err = store.Close()
	require.NoError(t, err)
}

func TestStoreWhenClosed(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)

	err = store.Close()
	require.NoError(t, err)

	ctx := context.Background()
	skill := &types.Skill{
		Name:        "test",
		DisplayName: "Test",
		Description: "Test",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
	}

	err = store.Store(ctx, skill)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill store is closed")
}

func TestDeepCopySkill(t *testing.T) {
	skill := &types.Skill{
		Name:        "original",
		DisplayName: "Original",
		Description: "Original description",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAssistant, types.SkillAgentTypeWorker},
		Capabilities: []types.SkillCapability{
			{Type: types.SkillCapabilityTypeTool, Name: "tool1"},
		},
		Metadata: types.SkillMetadata{
			Labels:       map[string]string{"key": "value"},
			Tags:         []string{"tag1", "tag2"},
			Keywords:     []string{"kw1"},
			RequiredAPIs: []string{"api1"},
			Dependencies: []string{"dep1"},
		},
	}

	copy := deepCopySkill(skill)

	// Verify they are equal
	assert.Equal(t, skill.Name, copy.Name)
	assert.Equal(t, skill.Metadata.Labels["key"], copy.Metadata.Labels["key"])

	// Modify the copy
	copy.Name = "modified"
	copy.Metadata.Labels["key"] = "modified"
	copy.Metadata.Tags[0] = "modified"

	// Verify original is unchanged
	assert.Equal(t, "original", skill.Name)
	assert.Equal(t, "value", skill.Metadata.Labels["key"])
	assert.Equal(t, "tag1", skill.Metadata.Tags[0])
}

func TestLoadFromDisk(t *testing.T) {
	tempDir := t.TempDir()

	// Create a skill file manually
	ownerDir := filepath.Join(tempDir, "user123")
	err := os.MkdirAll(ownerDir, 0755)
	require.NoError(t, err)

	skillContent := "```json\n" +
		"{\n" +
		"  \"id\": \"loaded-skill-123\",\n" +
		"  \"name\": \"loaded-skill\",\n" +
		"  \"display_name\": \"Loaded Skill\",\n" +
		"  \"description\": \"A skill loaded from disk\",\n" +
		"  \"version\": \"1.0.0\",\n" +
		"  \"state\": \"loaded\",\n" +
		"  \"source\": \"filesystem\"\n" +
		"}\n" +
		"```\n\n" +
		"# Loaded Skill\n\n" +
		"Content of the loaded skill.\n"

	skillPath := filepath.Join(ownerDir, "loaded-skill-123__loaded-skill.md")
	err = os.WriteFile(skillPath, []byte(skillContent), 0644)
	require.NoError(t, err)

	// Create store and load from disk
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	// Verify skill was loaded
	ctx := context.Background()
	skill, err := store.Get(ctx, types.NewID("loaded-skill-123"))
	require.NoError(t, err)
	assert.Equal(t, "loaded-skill", skill.Name)
	assert.Equal(t, "Loaded Skill", skill.DisplayName)
	assert.Equal(t, "user123", skill.OwnerID)
}

func TestGlobalStore(t *testing.T) {
	// Reset global store
	SetGlobal(nil)

	// First call should create default store
	store := Global()
	assert.NotNil(t, store)
	assert.False(t, store.IsClosed())

	// Second call should return the same instance
	store2 := Global()
	assert.Same(t, store, store2)

	// Close and reset
	store.Close()
	SetGlobal(nil)

	// Create a new store with custom config
	cfg := DefaultSkillConfig()
	newStore, err := NewStore(cfg, nil)
	require.NoError(t, err)

	SetGlobal(newStore)

	// Global should return the custom store
	store3 := Global()
	assert.Same(t, newStore, store3)

	store3.Close()
	SetGlobal(nil)
}

func TestSanitizeOwnerID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user123", "user123"},
		{"user-123", "user-123"},
		{"user_123", "user_123"},
		{"user.123", "user.123"},
		{"user@123", "user-123"},
		{"user/123", "user-123"},
		{"user\\123", "user-123"},
		{"../malicious", "malicious"},
		{"../../etc/passwd", "passwd"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeOwnerID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStoreWhenDisabled(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir
	cfg.Enabled = false

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	skill := &types.Skill{
		Name:        "test",
		DisplayName: "Test",
		Description: "Test",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
	}

	err = store.Store(ctx, skill)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill storage is disabled")
}

func TestMatchesFilter(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultSkillConfig()
	cfg.StoragePath = tempDir

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	skill := &types.Skill{
		Name:        "filter-test",
		DisplayName: "Filter Test",
		Description: "Testing filter functionality",
		Scope:       types.SkillScopeUser,
		OwnerID:     "user1",
		State:       types.SkillStateLoaded,
		Source:      types.SkillSourceFilesystem,
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAssistant, types.SkillAgentTypeWorker},
		Metadata: types.SkillMetadata{
			Category: "development",
			Tags:     []string{"test", "filter"},
			Labels:   map[string]string{"env": "test"},
			Verified: true,
		},
		CreatedAt: types.NewTimestampFromTime(time.Now().Add(-24 * time.Hour)),
	}

	// Test matching filters
	userScope := types.SkillScopeUser
	user1 := "user1"
	loadedState := types.SkillStateLoaded
	filesystemSource := types.SkillSourceFilesystem
	devCategory := "development"
	assistantAgent := types.SkillAgentTypeAssistant
	verified := true

	tests := []struct {
		name   string
		filter *types.SkillFilter
		match  bool
	}{
		{
			name:   "nil filter",
			filter: nil,
			match:  true,
		},
		{
			name: "matching scope",
			filter: &types.SkillFilter{
				Scope: &userScope,
			},
			match: true,
		},
		{
			name: "non-matching scope",
			filter: &types.SkillFilter{
				Scope: &[]types.SkillScope{types.SkillScopeGroup}[0],
			},
			match: false,
		},
		{
			name: "matching owner",
			filter: &types.SkillFilter{
				OwnerID: &user1,
			},
			match: true,
		},
		{
			name: "non-matching owner",
			filter: &types.SkillFilter{
				OwnerID: &[]string{"user2"}[0],
			},
			match: false,
		},
		{
			name: "matching state",
			filter: &types.SkillFilter{
				State: &loadedState,
			},
			match: true,
		},
		{
			name: "non-matching state",
			filter: &types.SkillFilter{
				State: &[]types.SkillState{types.SkillStateDisabled}[0],
			},
			match: false,
		},
		{
			name: "matching source",
			filter: &types.SkillFilter{
				Source: &filesystemSource,
			},
			match: true,
		},
		{
			name: "matching agent type",
			filter: &types.SkillFilter{
				AgentType: &assistantAgent,
			},
			match: true,
		},
		{
			name: "non-matching agent type",
			filter: &types.SkillFilter{
				AgentType: &[]types.SkillAgentType{types.SkillAgentTypeWorker}[0],
			},
			match: true, // Still matches because skill has both assistant and worker
		},
		{
			name: "matching category",
			filter: &types.SkillFilter{
				Category: &devCategory,
			},
			match: true,
		},
		{
			name: "matching verified",
			filter: &types.SkillFilter{
				Verified: &verified,
			},
			match: true,
		},
		{
			name: "non-matching verified",
			filter: &types.SkillFilter{
				Verified: &[]bool{false}[0],
			},
			match: false,
		},
		{
			name: "matching labels",
			filter: &types.SkillFilter{
				Label: map[string]string{"env": "test"},
			},
			match: true,
		},
		{
			name: "non-matching labels",
			filter: &types.SkillFilter{
				Label: map[string]string{"env": "prod"},
			},
			match: false,
		},
		{
			name: "matching tags",
			filter: &types.SkillFilter{
				Tags: []string{"test"},
			},
			match: true,
		},
		{
			name: "non-matching tags",
			filter: &types.SkillFilter{
				Tags: []string{"notfound"},
			},
			match: false,
		},
		{
			name: "multiple matching tags",
			filter: &types.SkillFilter{
				Tags: []string{"test", "filter"},
			},
			match: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.matchesFilter(skill, tt.filter)
			assert.Equal(t, tt.match, result)
		})
	}
}
