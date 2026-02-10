package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrganizer(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	require.NotNil(t, organizer)
	assert.False(t, organizer.IsClosed())

	err = organizer.Close()
	require.NoError(t, err)
	assert.True(t, organizer.IsClosed())
}

func TestNewDefaultOrganizer(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	cfg := config.MemoryConfig{
		Enabled:         true,
		StoragePath:     tmpDir,
		UserMemoryPath:  filepath.Join(tmpDir, "users"),
		GroupMemoryPath: filepath.Join(tmpDir, "groups"),
	}
	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewDefaultOrganizer(store, log)
	require.NoError(t, err)
	require.NotNil(t, organizer)

	err = organizer.Close()
	require.NoError(t, err)
}

func TestNewOrganizerNilStore(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := config.MemoryConfig{
		Enabled: true,
	}

	organizer, err := NewOrganizer(cfg, nil, log)
	assert.Error(t, err)
	assert.Nil(t, organizer)
}

func TestNewOrganizerNilLogger(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	// Nil logger should create a default one
	organizer, err := NewOrganizer(cfg, store, nil)
	require.NoError(t, err)
	require.NotNil(t, organizer)

	err = organizer.Close()
	require.NoError(t, err)
}

func TestValidateTopic(t *testing.T) {
	testCases := []struct {
		name    string
		topic   string
		wantErr bool
	}{
		{"Valid topic", "preferences", false},
		{"Valid topic with hyphen", "user-preferences", false},
		{"Valid topic with underscore", "user_preferences", false},
		{"Valid topic with numbers", "topic123", false},
		{"Empty topic", "", true},
		{"Topic with path traversal", "../etc", true},
		{"Topic with forward slash", "topic/sub", true},
		{"Topic with backslash", "topic\\sub", true},
		{"Topic too long", string(make([]byte, 101)), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTopic(tc.topic)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeTopic(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"Lowercase", "Preferences", "preferences"},
		{"Spaces to hyphens", "user preferences", "user-preferences"},
		{"Multiple spaces", "user  preferences", "user-preferences"},
		{"Special characters", "user@preferences!", "user-preferences"},
		{"Already sanitized", "user-preferences", "user-preferences"},
		{"Consecutive hyphens", "user--preferences", "user-preferences"},
		{"Leading hyphens", "-preferences", "preferences"},
		{"Trailing hyphens", "preferences-", "preferences"},
		{"Empty after sanitization", "@#$", "general"},
		{"Numbers", "topic123", "topic123"},
		{"Underscores", "user_preferences", "user_preferences"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeTopic(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetTopicPath(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	tests := []struct {
		name     string
		scope    types.MemoryScope
		ownerID  string
		topic    string
		expected string
	}{
		{
			name:     "User scope topic",
			scope:    types.MemoryScopeUser,
			ownerID:  "user123",
			topic:    "preferences",
			expected: filepath.Join(tempDir, "users", "user123", "preferences"),
		},
		{
			name:     "Group scope topic",
			scope:    types.MemoryScopeGroup,
			ownerID:  "group456",
			topic:    "decisions",
			expected: filepath.Join(tempDir, "groups", "group456", "decisions"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := organizer.GetTopicPath(tt.scope, tt.ownerID, tt.topic)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, path)
		})
	}
}

func TestGetTopicPathInvalid(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	_, err = organizer.GetTopicPath(types.MemoryScopeUser, "user123", "")
	assert.Error(t, err)

	_, err = organizer.GetTopicPath(types.MemoryScopeUser, "user123", "../etc")
	assert.Error(t, err)
}

func TestCreateTopicDir(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	err = organizer.CreateTopicDir(types.MemoryScopeUser, "user123", "preferences")
	require.NoError(t, err)

	// Verify directory was created
	expectedPath := filepath.Join(tempDir, "users", "user123", "preferences")
	info, err := filepath.Abs(expectedPath)
	require.NoError(t, err)
	assert.DirExists(t, info)
}

func TestListTopics(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Initially no topics
	topics, err := organizer.ListTopics(ctx, types.MemoryScopeUser, "user123")
	require.NoError(t, err)
	assert.Empty(t, topics)

	// Create some topic directories
	require.NoError(t, organizer.CreateTopicDir(types.MemoryScopeUser, "user123", "preferences"))
	require.NoError(t, organizer.CreateTopicDir(types.MemoryScopeUser, "user123", "decisions"))
	require.NoError(t, organizer.CreateTopicDir(types.MemoryScopeUser, "user123", "facts"))

	// List topics
	topics, err = organizer.ListTopics(ctx, types.MemoryScopeUser, "user123")
	require.NoError(t, err)
	assert.Len(t, topics, 3)
	assert.Contains(t, topics, "preferences")
	assert.Contains(t, topics, "decisions")
	assert.Contains(t, topics, "facts")
}

func TestGetMemoriesByTopic(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Create memories with different topics
	mem1 := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypePreference,
		Topic:   "preferences",
		Title:   "Pref 1",
		Content: "Content 1",
		Metadata: types.MemoryMetadata{
			Importance: 5,
		},
	}

	mem2 := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypePreference,
		Topic:   "preferences",
		Title:   "Pref 2",
		Content: "Content 2",
		Metadata: types.MemoryMetadata{
			Importance: 5,
		},
	}

	mem3 := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypeDecision,
		Topic:   "decisions",
		Title:   "Decision 1",
		Content: "Content 3",
		Metadata: types.MemoryMetadata{
			Importance: 7,
		},
	}

	require.NoError(t, store.Store(ctx, mem1))
	require.NoError(t, store.Store(ctx, mem2))
	require.NoError(t, store.Store(ctx, mem3))

	// Get memories by topic "preferences"
	prefMemories, err := organizer.GetMemoriesByTopic(ctx, types.MemoryScopeUser, "user123", "preferences")
	require.NoError(t, err)
	assert.Len(t, prefMemories, 2)

	// Get memories by topic "decisions"
	decMemories, err := organizer.GetMemoriesByTopic(ctx, types.MemoryScopeUser, "user123", "decisions")
	require.NoError(t, err)
	assert.Len(t, decMemories, 1)
}

func TestOrganizeMemory(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Create a memory
	mem := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypePreference,
		Topic:   "preferences",
		Title:   "My Preference",
		Content: "I prefer dark mode",
		Metadata: types.MemoryMetadata{
			Importance: 5,
		},
	}

	require.NoError(t, store.Store(ctx, mem))

	// Organize the memory into topic directory
	err = organizer.OrganizeMemory(ctx, mem.ID)
	require.NoError(t, err)

	// Verify file was moved to topic directory
	topicPath := filepath.Join(tempDir, "users", "user123", "preferences")
	entries, err := filepath.Glob(filepath.Join(topicPath, "*.md"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestOrganizeAllMemories(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Create memories with different topics
	memories := []*types.Memory{
		{
			Scope:   types.MemoryScopeUser,
			OwnerID: "user123",
			Type:    types.MemoryTypePreference,
			Topic:   "preferences",
			Title:   "Pref 1",
			Content: "Content 1",
			Metadata: types.MemoryMetadata{
				Importance: 5,
			},
		},
		{
			Scope:   types.MemoryScopeUser,
			OwnerID: "user123",
			Type:    types.MemoryTypePreference,
			Topic:   "preferences",
			Title:   "Pref 2",
			Content: "Content 2",
			Metadata: types.MemoryMetadata{
				Importance: 5,
			},
		},
		{
			Scope:   types.MemoryScopeUser,
			OwnerID: "user123",
			Type:    types.MemoryTypeDecision,
			Topic:   "decisions",
			Title:   "Decision 1",
			Content: "Content 3",
			Metadata: types.MemoryMetadata{
				Importance: 7,
			},
		},
		{
			Scope:   types.MemoryScopeUser,
			OwnerID: "user123",
			Type:    types.MemoryTypeFact,
			Topic:   "facts",
			Title:   "Fact 1",
			Content: "Content 4",
			Metadata: types.MemoryMetadata{
				Importance: 6,
			},
		},
	}

	for _, mem := range memories {
		require.NoError(t, store.Store(ctx, mem))
	}

	// Organize all memories
	stats, err := organizer.OrganizeAllMemories(ctx, types.MemoryScopeUser, "user123")
	require.NoError(t, err)

	assert.Equal(t, 4, stats.TotalCount)
	assert.Equal(t, 4, stats.OrganizedCount)
	assert.Equal(t, 0, stats.SkippedCount)
	assert.Equal(t, 0, stats.FailedCount)

	// Verify topic directories exist
	preferencesPath := filepath.Join(tempDir, "users", "user123", "preferences")
	decisionsPath := filepath.Join(tempDir, "users", "user123", "decisions")
	factsPath := filepath.Join(tempDir, "users", "user123", "facts")

	assert.DirExists(t, preferencesPath)
	assert.DirExists(t, decisionsPath)
	assert.DirExists(t, factsPath)

	// Verify files in each directory
	prefFiles, _ := filepath.Glob(filepath.Join(preferencesPath, "*.md"))
	decFiles, _ := filepath.Glob(filepath.Join(decisionsPath, "*.md"))
	factFiles, _ := filepath.Glob(filepath.Join(factsPath, "*.md"))

	assert.Len(t, prefFiles, 2)
	assert.Len(t, decFiles, 1)
	assert.Len(t, factFiles, 1)
}

func TestGetTopicStats(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Create memories with different topics
	memories := []*types.Memory{
		{
			Scope:   types.MemoryScopeUser,
			OwnerID: "user123",
			Type:    types.MemoryTypePreference,
			Topic:   "preferences",
			Title:   "Pref 1",
			Content: "Content 1",
			Metadata: types.MemoryMetadata{
				Importance: 5,
			},
		},
		{
			Scope:   types.MemoryScopeUser,
			OwnerID: "user123",
			Type:    types.MemoryTypePreference,
			Topic:   "preferences",
			Title:   "Pref 2",
			Content: "Content 2",
			Metadata: types.MemoryMetadata{
				Importance: 5,
			},
		},
		{
			Scope:   types.MemoryScopeUser,
			OwnerID: "user123",
			Type:    types.MemoryTypeDecision,
			Topic:   "decisions",
			Title:   "Decision 1",
			Content: "Content 3",
			Metadata: types.MemoryMetadata{
				Importance: 7,
			},
		},
		{
			Scope:   types.MemoryScopeUser,
			OwnerID: "user123",
			Type:    types.MemoryTypeFact,
			Topic:   "", // Empty topic should be counted as "general"
			Title:   "Fact 1",
			Content: "Content 4",
			Metadata: types.MemoryMetadata{
				Importance: 6,
			},
		},
	}

	for _, mem := range memories {
		require.NoError(t, store.Store(ctx, mem))
	}

	// Get topic stats
	stats, err := organizer.GetTopicStats(ctx, types.MemoryScopeUser, "user123")
	require.NoError(t, err)

	assert.Equal(t, 2, stats["preferences"])
	assert.Equal(t, 1, stats["decisions"])
	assert.Equal(t, 1, stats["general"])
}

func TestRenameTopic(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	// Create a topic directory
	require.NoError(t, organizer.CreateTopicDir(types.MemoryScopeUser, "user123", "old-topic"))

	oldPath := filepath.Join(tempDir, "users", "user123", "old-topic")
	newPath := filepath.Join(tempDir, "users", "user123", "new-topic")

	// Verify old path exists
	assert.DirExists(t, oldPath)

	// Rename topic
	err = organizer.RenameTopic(types.MemoryScopeUser, "user123", "old-topic", "new-topic")
	require.NoError(t, err)

	// Verify old path doesn't exist and new path exists
	assert.NoDirExists(t, oldPath)
	assert.DirExists(t, newPath)
}

func TestRenameTopicErrors(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	// Try to rename non-existent topic
	err = organizer.RenameTopic(types.MemoryScopeUser, "user123", "nonexistent", "new-topic")
	assert.Error(t, err)

	// Create a topic directory
	require.NoError(t, organizer.CreateTopicDir(types.MemoryScopeUser, "user123", "topic1"))
	require.NoError(t, organizer.CreateTopicDir(types.MemoryScopeUser, "user123", "topic2"))

	// Try to rename to existing topic
	err = organizer.RenameTopic(types.MemoryScopeUser, "user123", "topic1", "topic2")
	assert.Error(t, err)
}

func TestDeleteTopic(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Create a memory and organize it
	mem := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypePreference,
		Topic:   "test-topic",
		Title:   "Test Memory",
		Content: "Test content",
		Metadata: types.MemoryMetadata{
			Importance: 5,
		},
	}

	require.NoError(t, store.Store(ctx, mem))
	require.NoError(t, organizer.OrganizeMemory(ctx, mem.ID))

	topicPath := filepath.Join(tempDir, "users", "user123", "test-topic")
	assert.DirExists(t, topicPath)

	// Delete the topic
	err = organizer.DeleteTopic(types.MemoryScopeUser, "user123", "test-topic")
	require.NoError(t, err)

	// Verify topic directory is gone
	assert.NoDirExists(t, topicPath)
}

func TestDeleteTopicErrors(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	// Try to delete non-existent topic
	err = organizer.DeleteTopic(types.MemoryScopeUser, "user123", "nonexistent")
	assert.Error(t, err)

	// Try to delete with invalid topic
	err = organizer.DeleteTopic(types.MemoryScopeUser, "user123", "../etc")
	assert.Error(t, err)
}

func TestMergeTopic(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Create memories with different topics
	mem1 := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypePreference,
		Topic:   "source-topic",
		Title:   "Memory 1",
		Content: "Content 1",
		Metadata: types.MemoryMetadata{
			Importance: 5,
		},
	}

	mem2 := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypePreference,
		Topic:   "source-topic",
		Title:   "Memory 2",
		Content: "Content 2",
		Metadata: types.MemoryMetadata{
			Importance: 5,
		},
	}

	mem3 := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypeDecision,
		Topic:   "target-topic",
		Title:   "Memory 3",
		Content: "Content 3",
		Metadata: types.MemoryMetadata{
			Importance: 7,
		},
	}

	require.NoError(t, store.Store(ctx, mem1))
	require.NoError(t, store.Store(ctx, mem2))
	require.NoError(t, store.Store(ctx, mem3))

	// Merge source-topic into target-topic
	mergedCount, err := organizer.MergeTopic(ctx, types.MemoryScopeUser, "user123", "source-topic", "target-topic")
	require.NoError(t, err)
	assert.Equal(t, 2, mergedCount)

	// Verify memories now have target-topic
	sourceMemories, err := organizer.GetMemoriesByTopic(ctx, types.MemoryScopeUser, "user123", "source-topic")
	require.NoError(t, err)
	assert.Len(t, sourceMemories, 0)

	targetMemories, err := organizer.GetMemoriesByTopic(ctx, types.MemoryScopeUser, "user123", "target-topic")
	require.NoError(t, err)
	assert.Len(t, targetMemories, 3)
}

func TestOrganizerClosed(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	err = organizer.Close()
	require.NoError(t, err)

	ctx := context.Background()

	// All operations should fail when closed
	_, err = organizer.GetTopicPath(types.MemoryScopeUser, "user123", "topic")
	assert.Error(t, err)

	err = organizer.CreateTopicDir(types.MemoryScopeUser, "user123", "topic")
	assert.Error(t, err)

	_, err = organizer.ListTopics(ctx, types.MemoryScopeUser, "user123")
	assert.Error(t, err)

	_, err = organizer.GetMemoriesByTopic(ctx, types.MemoryScopeUser, "user123", "topic")
	assert.Error(t, err)

	err = organizer.OrganizeMemory(ctx, types.GenerateID())
	assert.Error(t, err)

	_, err = organizer.OrganizeAllMemories(ctx, types.MemoryScopeUser, "user123")
	assert.Error(t, err)

	_, err = organizer.GetTopicStats(ctx, types.MemoryScopeUser, "user123")
	assert.Error(t, err)

	err = organizer.RenameTopic(types.MemoryScopeUser, "user123", "old", "new")
	assert.Error(t, err)

	err = organizer.DeleteTopic(types.MemoryScopeUser, "user123", "topic")
	assert.Error(t, err)

	_, err = organizer.MergeTopic(ctx, types.MemoryScopeUser, "user123", "source", "target")
	assert.Error(t, err)
}

func TestOrganizeMemoryAlreadyInTopicDir(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Create a memory
	mem := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypePreference,
		Topic:   "preferences",
		Title:   "My Preference",
		Content: "I prefer dark mode",
		Metadata: types.MemoryMetadata{
			Importance: 5,
		},
	}

	require.NoError(t, store.Store(ctx, mem))

	// Organize the memory
	err = organizer.OrganizeMemory(ctx, mem.ID)
	require.NoError(t, err)

	// Organize again - should be idempotent
	err = organizer.OrganizeMemory(ctx, mem.ID)
	require.NoError(t, err)

	// Verify only one file exists
	topicPath := filepath.Join(tempDir, "users", "user123", "preferences")
	entries, err := filepath.Glob(filepath.Join(topicPath, "*.md"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestOrganizeAllMemoriesWithEmptyTopic(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Create a memory with empty topic
	mem := &types.Memory{
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Type:    types.MemoryTypePreference,
		Topic:   "", // Empty topic
		Title:   "My Memory",
		Content: "Some content",
		Metadata: types.MemoryMetadata{
			Importance: 5,
		},
	}

	require.NoError(t, store.Store(ctx, mem))

	// Organize all memories - should use "general" for empty topic
	stats, err := organizer.OrganizeAllMemories(ctx, types.MemoryScopeUser, "user123")
	require.NoError(t, err)

	assert.Equal(t, 1, stats.TotalCount)
	assert.Equal(t, 1, stats.OrganizedCount)

	// Verify file is in "general" directory
	generalPath := filepath.Join(tempDir, "users", "user123", "general")
	assert.DirExists(t, generalPath)

	entries, err := filepath.Glob(filepath.Join(generalPath, "*.md"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestOrganizeStats(t *testing.T) {
	stats := OrganizeStats{
		Scope:          types.MemoryScopeUser,
		OwnerID:        "user123",
		TotalCount:     10,
		OrganizedCount: 7,
		SkippedCount:   2,
		FailedCount:    1,
		Errors:         []string{"memory 123: file not found"},
	}

	assert.Equal(t, types.MemoryScopeUser, stats.Scope)
	assert.Equal(t, "user123", stats.OwnerID)
	assert.Equal(t, 10, stats.TotalCount)
	assert.Equal(t, 7, stats.OrganizedCount)
	assert.Equal(t, 2, stats.SkippedCount)
	assert.Equal(t, 1, stats.FailedCount)
	assert.Len(t, stats.Errors, 1)
}

func TestCopyAndDelete(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	// Create a test file
	srcPath := filepath.Join(tempDir, "test_source.md")
	destPath := filepath.Join(tempDir, "test_dest.md")
	content := "test content for copy"

	err = os.WriteFile(srcPath, []byte(content), 0644)
	require.NoError(t, err)

	// Copy and delete
	err = organizer.copyAndDelete(srcPath, destPath)
	require.NoError(t, err)

	// Verify destination exists and source is gone
	destContent, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(destContent))

	_, err = os.Stat(srcPath)
	assert.True(t, os.IsNotExist(err))
}

func TestListTopicsGroupScope(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Create group topic directories
	require.NoError(t, organizer.CreateTopicDir(types.MemoryScopeGroup, "group456", "projects"))
	require.NoError(t, organizer.CreateTopicDir(types.MemoryScopeGroup, "group456", "decisions"))

	// List topics
	topics, err := organizer.ListTopics(ctx, types.MemoryScopeGroup, "group456")
	require.NoError(t, err)
	assert.Len(t, topics, 2)
	assert.Contains(t, topics, "projects")
	assert.Contains(t, topics, "decisions")
}

func TestGetMemoriesByTopicEmpty(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Get memories for a topic that doesn't exist
	memories, err := organizer.GetMemoriesByTopic(ctx, types.MemoryScopeUser, "user123", "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, memories)
}

func TestGetTopicStatsEmpty(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err)
	defer organizer.Close()

	ctx := context.Background()

	// Get stats for owner with no memories
	stats, err := organizer.GetTopicStats(ctx, types.MemoryScopeUser, "user123")
	require.NoError(t, err)
	assert.Empty(t, stats)
}
