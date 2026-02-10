package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	// Create temporary directory for testing
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
	require.NotNil(t, store)
	assert.False(t, store.IsClosed())

	// Verify directories were created
	info, err := os.Stat(cfg.UserMemoryPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	info, err = os.Stat(cfg.GroupMemoryPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	err = store.Close()
	require.NoError(t, err)
	assert.True(t, store.IsClosed())
}

func TestNewDefaultStore(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	// Use explicit config with temp dir to avoid writing to real data directory
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, store)

	// Close the store to clean up
	err = store.Close()
	require.NoError(t, err)
}

func TestStoreMemory(t *testing.T) {
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

	ctx := context.Background()

	mem := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Topic:   "preferences",
		Title:   "User Prefers Dark Mode",
		Content: "The user prefers dark mode in their applications.",
		Metadata: types.MemoryMetadata{
			Labels:     map[string]string{"category": "ui"},
			Tags:       []string{"ui", "preferences"},
			Source:     "manual",
			Importance: 5,
			Verified:   true,
		},
	}

	err = store.Store(ctx, mem)
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID, "ID should be generated")

	// Verify file was created
	filePath := filepath.Join(cfg.UserMemoryPath, mem.OwnerID, store.generateFilename(mem))
	_, err = os.Stat(filePath)
	require.NoError(t, err, "memory file should be created")
}

func TestStoreMemoryValidation(t *testing.T) {
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

	ctx := context.Background()

	tests := []struct {
		name    string
		mem     *types.Memory
		wantErr string
	}{
		{
			name: "missing title",
			mem: &types.Memory{
				Type:    types.MemoryTypeFact,
				Scope:   types.MemoryScopeUser,
				OwnerID: "user123",
				Content: "Some content",
			},
			wantErr: "memory title is required",
		},
		{
			name: "missing scope",
			mem: &types.Memory{
				Type:    types.MemoryTypeFact,
				OwnerID: "user123",
				Title:   "Test",
				Content: "Some content",
			},
			wantErr: "memory scope is required",
		},
		{
			name: "missing owner_id",
			mem: &types.Memory{
				Type:    types.MemoryTypeFact,
				Scope:   types.MemoryScopeUser,
				Title:   "Test",
				Content: "Some content",
			},
			wantErr: "memory owner_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Store(ctx, tt.mem)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestStoreMemoryDisabled(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         false,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	log, err := logger.NewDefault()
	require.NoError(t, err)

	store, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	mem := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Title:   "Test",
		Content: "Some content",
	}

	err = store.Store(ctx, mem)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory storage is disabled")
}

func TestGetMemory(t *testing.T) {
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

	ctx := context.Background()

	mem := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Topic:   "preferences",
		Title:   "User Prefers Dark Mode",
		Content: "The user prefers dark mode in their applications.",
	}

	err = store.Store(ctx, mem)
	require.NoError(t, err)

	// Get the memory
	retrieved, err := store.Get(ctx, mem.ID)
	require.NoError(t, err)
	assert.Equal(t, mem.ID, retrieved.ID)
	assert.Equal(t, mem.Title, retrieved.Title)
	assert.Equal(t, mem.Content, retrieved.Content)
	assert.Equal(t, mem.Scope, retrieved.Scope)
	assert.Equal(t, mem.OwnerID, retrieved.OwnerID)
}

func TestGetMemoryNotFound(t *testing.T) {
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

	ctx := context.Background()

	_, err = store.Get(ctx, types.GenerateID())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory not found")
}

func TestGetMemoryExpired(t *testing.T) {
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

	ctx := context.Background()

	// Create expired memory
	pastTime := types.NewTimestampFromTime(time.Now().Add(-1 * time.Hour))
	mem := &types.Memory{
		Type:      types.MemoryTypeFact,
		Scope:     types.MemoryScopeUser,
		OwnerID:   "user123",
		Title:     "Expired Memory",
		Content:   "This memory is expired",
		ExpiresAt: &pastTime,
	}

	err = store.Store(ctx, mem)
	require.NoError(t, err)

	// Try to get expired memory
	_, err = store.Get(ctx, mem.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory has expired")
}

func TestGetByOwner(t *testing.T) {
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

	ctx := context.Background()

	// Store memories for different users
	user1Mem1 := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user1",
		Title:   "User 1 Memory 1",
		Content: "Content 1",
	}
	user1Mem2 := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user1",
		Title:   "User 1 Memory 2",
		Content: "Content 2",
	}
	user2Mem := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user2",
		Title:   "User 2 Memory",
		Content: "Content 3",
	}

	err = store.Store(ctx, user1Mem1)
	require.NoError(t, err)
	err = store.Store(ctx, user1Mem2)
	require.NoError(t, err)
	err = store.Store(ctx, user2Mem)
	require.NoError(t, err)

	// Get memories for user1
	user1Memories, err := store.GetByOwner(ctx, types.MemoryScopeUser, "user1")
	require.NoError(t, err)
	assert.Len(t, user1Memories, 2)

	// Get memories for user2
	user2Memories, err := store.GetByOwner(ctx, types.MemoryScopeUser, "user2")
	require.NoError(t, err)
	assert.Len(t, user2Memories, 1)

	// Get memories for non-existent user
	emptyMemories, err := store.GetByOwner(ctx, types.MemoryScopeUser, "nonexistent")
	require.NoError(t, err)
	assert.Len(t, emptyMemories, 0)
}

func TestListMemories(t *testing.T) {
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

	ctx := context.Background()

	// Store memories with different properties
	mem1 := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user1",
		Topic:   "preferences",
		Title:   "Memory 1",
		Content: "Content 1",
		Metadata: types.MemoryMetadata{
			Labels:     map[string]string{"category": "ui"},
			Tags:       []string{"ui", "important"},
			Importance: 8,
			Verified:   true,
		},
	}
	mem2 := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user1",
		Topic:   "context",
		Title:   "Memory 2",
		Content: "Content 2",
		Metadata: types.MemoryMetadata{
			Importance: 3,
			Verified:   false,
		},
	}
	mem3 := &types.Memory{
		Type:    types.MemoryTypePreference,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user1",
		Topic:   "preferences",
		Title:   "Memory 3",
		Content: "Content 3",
		Metadata: types.MemoryMetadata{
			Importance: 5,
			Verified:   true,
		},
	}

	err = store.Store(ctx, mem1)
	require.NoError(t, err)
	err = store.Store(ctx, mem2)
	require.NoError(t, err)
	err = store.Store(ctx, mem3)
	require.NoError(t, err)

	// List all memories
	allMemories, err := store.List(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, allMemories, 3)

	// Filter by topic
	preferencesFilter := &types.MemoryFilter{
		Topic: strPtr("preferences"),
	}
	preferencesMemories, err := store.List(ctx, preferencesFilter)
	require.NoError(t, err)
	assert.Len(t, preferencesMemories, 2)

	// Filter by type
	factFilter := &types.MemoryFilter{
		Type: factTypePtr(),
	}
	factMemories, err := store.List(ctx, factFilter)
	require.NoError(t, err)
	assert.Len(t, factMemories, 2)

	// Filter by min importance
	importantFilter := &types.MemoryFilter{
		MinImportance: intPtr(5),
	}
	importantMemories, err := store.List(ctx, importantFilter)
	require.NoError(t, err)
	assert.Len(t, importantMemories, 2)

	// Filter by verified
	verifiedFilter := &types.MemoryFilter{
		Verified: boolPtr(true),
	}
	verifiedMemories, err := store.List(ctx, verifiedFilter)
	require.NoError(t, err)
	assert.Len(t, verifiedMemories, 2)

	// Filter by tags
	tagFilter := &types.MemoryFilter{
		Tags: []string{"ui"},
	}
	tagMemories, err := store.List(ctx, tagFilter)
	require.NoError(t, err)
	assert.Len(t, tagMemories, 1)

	// Filter by label
	labelFilter := &types.MemoryFilter{
		Label: map[string]string{"category": "ui"},
	}
	labelMemories, err := store.List(ctx, labelFilter)
	require.NoError(t, err)
	assert.Len(t, labelMemories, 1)
}

func TestDeleteMemory(t *testing.T) {
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

	ctx := context.Background()

	mem := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Title:   "Test Memory",
		Content: "Test content",
	}

	err = store.Store(ctx, mem)
	require.NoError(t, err)

	// Verify file exists
	filePath := filepath.Join(cfg.UserMemoryPath, mem.OwnerID, store.generateFilename(mem))
	_, err = os.Stat(filePath)
	require.NoError(t, err)

	// Delete memory
	err = store.Delete(ctx, mem.ID)
	require.NoError(t, err)

	// Verify file was deleted
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err), "memory file should be deleted")

	// Verify memory is gone from store
	_, err = store.Get(ctx, mem.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory not found")
}

func TestDeleteMemoryNotFound(t *testing.T) {
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

	ctx := context.Background()

	err = store.Delete(ctx, types.GenerateID())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory not found")
}

func TestStats(t *testing.T) {
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

	ctx := context.Background()

	// Store memories
	mem1 := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user1",
		Topic:   "topic1",
		Title:   "Memory 1",
		Content: "Content 1",
	}
	mem2 := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user1",
		Topic:   "topic1",
		Title:   "Memory 2",
		Content: "Content 2",
	}
	mem3 := &types.Memory{
		Type:    types.MemoryTypePreference,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user1",
		Topic:   "topic2",
		Title:   "Memory 3",
		Content: "Content 3",
	}
	mem4 := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user2",
		Topic:   "topic1",
		Title:   "Memory 4",
		Content: "Content 4",
	}

	err = store.Store(ctx, mem1)
	require.NoError(t, err)
	err = store.Store(ctx, mem2)
	require.NoError(t, err)
	err = store.Store(ctx, mem3)
	require.NoError(t, err)
	err = store.Store(ctx, mem4)
	require.NoError(t, err)

	// Get stats for user1
	stats, err := store.Stats(ctx, types.MemoryScopeUser, "user1")
	require.NoError(t, err)
	assert.Equal(t, types.MemoryScopeUser, stats.Scope)
	assert.Equal(t, "user1", stats.OwnerID)
	assert.Equal(t, 3, stats.TotalCount)
	assert.Equal(t, 2, stats.TypeCounts[types.MemoryTypeFact])
	assert.Equal(t, 1, stats.TypeCounts[types.MemoryTypePreference])
	assert.Equal(t, 2, stats.TopicCounts["topic1"])
	assert.Equal(t, 1, stats.TopicCounts["topic2"])

	// Get stats for user2
	stats, err = store.Stats(ctx, types.MemoryScopeUser, "user2")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.TotalCount)
}

func TestLoadFromDisk(t *testing.T) {
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

	// Create first store and save memories
	store1, err := NewStore(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	mem1 := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user1",
		Topic:   "topic1",
		Title:   "Memory 1",
		Content: "Content 1",
		Metadata: types.MemoryMetadata{
			Labels:     map[string]string{"key": "value"},
			Tags:       []string{"tag1", "tag2"},
			Importance: 7,
			Verified:   true,
		},
	}

	err = store1.Store(ctx, mem1)
	require.NoError(t, err)

	err = store1.Close()
	require.NoError(t, err)

	// Create new store - should load existing memories
	store2, err := NewStore(cfg, log)
	require.NoError(t, err)
	defer store2.Close()

	// Verify memory was loaded
	retrieved, err := store2.Get(ctx, mem1.ID)
	require.NoError(t, err)
	assert.Equal(t, mem1.ID, retrieved.ID)
	assert.Equal(t, mem1.Title, retrieved.Title)
	assert.Equal(t, mem1.Content, retrieved.Content)
	assert.Equal(t, mem1.Topic, retrieved.Topic)
	assert.Equal(t, mem1.Type, retrieved.Type)
	assert.Equal(t, mem1.Scope, retrieved.Scope)
	assert.Equal(t, mem1.OwnerID, retrieved.OwnerID)
}

func TestUpdateMemory(t *testing.T) {
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

	ctx := context.Background()

	mem := &types.Memory{
		Type:    types.MemoryTypeFact,
		Scope:   types.MemoryScopeUser,
		OwnerID: "user123",
		Title:   "Original Title",
		Content: "Original content",
	}

	err = store.Store(ctx, mem)
	require.NoError(t, err)

	originalID := mem.ID
	originalCreatedAt := mem.CreatedAt

	// Update memory
	mem.Title = "Updated Title"
	mem.Content = "Updated content"
	mem.Topic = "updated-topic"

	err = store.Store(ctx, mem)
	require.NoError(t, err)

	// Verify ID and CreatedAt didn't change
	assert.Equal(t, originalID, mem.ID)
	assert.Equal(t, originalCreatedAt, mem.CreatedAt)

	// Verify update persisted
	retrieved, err := store.Get(ctx, mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", retrieved.Title)
	assert.Equal(t, "Updated content", retrieved.Content)
	assert.Equal(t, "updated-topic", retrieved.Topic)
}

func TestGroupScope(t *testing.T) {
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

	ctx := context.Background()

	groupMem := &types.Memory{
		Type:    types.MemoryTypeContext,
		Scope:   types.MemoryScopeGroup,
		OwnerID: "group1",
		Title:   "Group Project Info",
		Content: "Important project information for the group",
	}

	err = store.Store(ctx, groupMem)
	require.NoError(t, err)

	// Verify file was created in group directory
	filePath := filepath.Join(cfg.GroupMemoryPath, groupMem.OwnerID, store.generateFilename(groupMem))
	_, err = os.Stat(filePath)
	require.NoError(t, err, "group memory file should be created in groups directory")

	// Verify can retrieve
	retrieved, err := store.Get(ctx, groupMem.ID)
	require.NoError(t, err)
	assert.Equal(t, types.MemoryScopeGroup, retrieved.Scope)
	assert.Equal(t, "group1", retrieved.OwnerID)
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func factTypePtr() *types.MemoryType {
	t := types.MemoryTypeFact
	return &t
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}
