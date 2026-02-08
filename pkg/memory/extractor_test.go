package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExtractor(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:               true,
		MinConfidence:         0.6,
		MaxMemoriesPerSession: 10,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	require.NotNil(t, extractor)
	assert.False(t, extractor.IsClosed())

	err = extractor.Close()
	require.NoError(t, err)
	assert.True(t, extractor.IsClosed())
}

func TestNewDefaultExtractor(t *testing.T) {
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

	extractor, err := NewDefaultExtractor(store, log)
	require.NoError(t, err)
	require.NotNil(t, extractor)

	extCfg := extractor.GetConfig()
	assert.True(t, extCfg.Enabled)
	assert.Equal(t, 0.6, extCfg.MinConfidence)
	assert.Equal(t, 10, extCfg.MaxMemoriesPerSession)

	err = extractor.Close()
	require.NoError(t, err)
}

func TestNewExtractorNilStore(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	extCfg := types.MemoryExtractionConfig{
		Enabled: true,
	}

	extractor, err := NewExtractor(extCfg, nil, log)
	assert.Error(t, err)
	assert.Nil(t, extractor)
}

func TestNewExtractorNilLogger(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.MemoryConfig{
		StoragePath:     tempDir,
		UserMemoryPath:  filepath.Join(tempDir, "users"),
		GroupMemoryPath: filepath.Join(tempDir, "groups"),
		Enabled:         true,
		MaxFileSize:     100,
		FileFormat:      "markdown",
	}

	store, err := NewStore(cfg, nil)
	require.NoError(t, err)
	defer store.Close()

	extCfg := types.MemoryExtractionConfig{
		Enabled: true,
	}

	// Nil logger should create a default one
	extractor, err := NewExtractor(extCfg, store, nil)
	require.NoError(t, err)
	require.NotNil(t, extractor)

	err = extractor.Close()
	require.NoError(t, err)
}

func TestExtractFromSession(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:               true,
		MinConfidence:         0.5,
		MaxMemoriesPerSession: 10,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	session := &types.Session{
		ID:      types.GenerateID(),
		State:   types.SessionStateClosed,
		Status:  types.StatusStopped,
		CreatedAt: types.NewTimestampFromTime(time.Now()),
		UpdatedAt: types.NewTimestampFromTime(time.Now()),
		Metadata: types.SessionMetadata{
			Name:    "Test Session",
			OwnerID: "user123",
			Labels: map[string]string{
				"project": "test-project",
				"env":     "development",
			},
		},
		Context: types.SessionContext{
			Messages: []types.Message{
				{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestampFromTime(time.Now()),
					Role:      types.MessageRoleUser,
					Content:   "I prefer using dark mode in all my applications.",
				},
				{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestampFromTime(time.Now()),
					Role:      types.MessageRoleUser,
					Content:   "My name is John Doe and I work at Acme Corp.",
				},
			},
			Preferences: types.UserPreferences{
				Timezone: "America/New_York",
				Language: "en",
				Theme:    "dark",
			},
		},
	}

	ctx := context.Background()
	memories, err := extractor.ExtractFromSession(ctx, session)
	require.NoError(t, err)
	assert.Greater(t, len(memories), 0)

	// Verify memories were stored
	list, err := store.List(ctx, &types.MemoryFilter{
		Scope:   &[]types.MemoryScope{types.MemoryScopeUser}[0],
		OwnerID: &[]string{"user123"}[0],
	})
	require.NoError(t, err)
	assert.Greater(t, len(list), 0)
}

func TestExtractFromSessionDisabled(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled: false, // Disabled
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	session := &types.Session{
		ID:        types.GenerateID(),
		CreatedAt: types.NewTimestampFromTime(time.Now()),
		Metadata: types.SessionMetadata{
			OwnerID: "user123",
		},
		Context: types.SessionContext{
			Messages: []types.Message{
				{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestampFromTime(time.Now()),
					Role:      types.MessageRoleUser,
					Content:   "I prefer using dark mode.",
				},
			},
		},
	}

	ctx := context.Background()
	memories, err := extractor.ExtractFromSession(ctx, session)
	require.NoError(t, err)
	assert.Nil(t, memories) // Should return nil when disabled
}

func TestExtractFromSessionNil(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled: true,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	ctx := context.Background()
	_, err = extractor.ExtractFromSession(ctx, nil)
	assert.Error(t, err)
}

func TestExtractFromSessionClosed(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled: true,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	err = extractor.Close()
	require.NoError(t, err)

	session := &types.Session{
		ID:      types.GenerateID(),
		Metadata: types.SessionMetadata{
			OwnerID: "user123",
		},
	}

	ctx := context.Background()
	_, err = extractor.ExtractFromSession(ctx, session)
	assert.Error(t, err)
}

func TestExtractFromMessage(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:       true,
		MinConfidence: 0.5,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	sessionID := types.GenerateID()
	message := types.Message{
		ID:        types.GenerateID(),
		Timestamp: types.NewTimestampFromTime(time.Now()),
		Role:      types.MessageRoleUser,
		Content:   "I decided to use PostgreSQL for the database.",
	}

	ctx := context.Background()
	memories, err := extractor.ExtractFromMessage(ctx, sessionID, message, "user123", types.MemoryScopeUser)
	require.NoError(t, err)
	assert.Greater(t, len(memories), 0)

	// Verify memory type and content
	foundDecision := false
	for _, mem := range memories {
		if mem.Type == types.MemoryTypeDecision {
			foundDecision = true
			assert.Contains(t, mem.Content, "PostgreSQL")
			assert.Equal(t, "user123", mem.OwnerID)
		}
	}
	assert.True(t, foundDecision, "Should extract decision memory")
}

func TestExtractPreferencesFromMessage(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:       true,
		MinConfidence: 0.5,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	testCases := []struct {
		name     string
		content  string
		expected bool
	}{
		{"I prefer", "I prefer using Vim for editing", true},
		{"I like", "I like working in the morning", true},
		{"I don't like", "I don't like using GUI tools", true},
		{"I always", "I always run tests before committing", true},
		{"I never", "I never use tabs", true},
		{"No pattern", "Hello world", false},
		{"Too short", "Hi", false},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			message := types.Message{
				ID:        types.GenerateID(),
				Timestamp: types.NewTimestampFromTime(time.Now()),
				Role:      types.MessageRoleUser,
				Content:   tc.content,
			}

			memories, err := extractor.ExtractFromMessage(ctx, types.GenerateID(), message, "user123", types.MemoryScopeUser)
			require.NoError(t, err)

			if tc.expected {
				assert.Greater(t, len(memories), 0, "Should extract memory from: %s", tc.content)
			} else {
				assert.Equal(t, 0, len(memories), "Should not extract memory from: %s", tc.content)
			}
		})
	}
}

func TestExtractFactsFromMessage(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:       true,
		MinConfidence: 0.5,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	testCases := []struct {
	name    string
	content string
	topic   string
	}{
		{"My name", "My name is Alice Smith", "identity"},
		{"I work at", "I work at TechCorp Inc.", "work"},
		{"I am a", "I am a software engineer", "identity"},
		{"My email", "My email is alice@example.com", "contact"},
		{"Located in", "We are located in San Francisco", "location"},
		{"My project", "My project is called Project Alpha", "project"},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			message := types.Message{
				ID:        types.GenerateID(),
				Timestamp: types.NewTimestampFromTime(time.Now()),
				Role:      types.MessageRoleUser,
				Content:   tc.content,
			}

			memories, err := extractor.ExtractFromMessage(ctx, types.GenerateID(), message, "user123", types.MemoryScopeUser)
			require.NoError(t, err)
			assert.Greater(t, len(memories), 0)

			// Verify topic
			found := false
			for _, mem := range memories {
				if mem.Topic == tc.topic {
					found = true
					assert.Equal(t, types.MemoryTypeFact, mem.Type)
				}
			}
			assert.True(t, found, "Should find memory with topic: %s", tc.topic)
		})
	}
}

func TestExtractDecisionsFromMessage(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:       true,
		MinConfidence: 0.5,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	testCases := []struct {
	name    string
	content string
	}{
		{"I decided", "I decided to use React for the frontend"},
		{"We decided", "We decided to migrate to microservices"},
		{"My decision", "My decision is to go with PostgreSQL"},
		{"Chosen", "I have chosen TypeScript for this project"},
		{"Going with", "We're going with AWS for hosting"},
		{"Settled on", "I settled on using Docker for containers"},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			message := types.Message{
				ID:        types.GenerateID(),
				Timestamp: types.NewTimestampFromTime(time.Now()),
				Role:      types.MessageRoleUser,
				Content:   tc.content,
			}

			memories, err := extractor.ExtractFromMessage(ctx, types.GenerateID(), message, "user123", types.MemoryScopeUser)
			require.NoError(t, err)
			assert.Greater(t, len(memories), 0)

			// Verify decision type
			found := false
			for _, mem := range memories {
				if mem.Type == types.MemoryTypeDecision {
					found = true
					break
				}
			}
			assert.True(t, found, "Should extract decision memory from: %s", tc.content)
		})
	}
}

func TestFilterByMinConfidence(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:       true,
		MinConfidence: 0.8, // High threshold
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	session := &types.Session{
		ID:      types.GenerateID(),
		CreatedAt: types.NewTimestampFromTime(time.Now()),
		Metadata: types.SessionMetadata{
			OwnerID: "user123",
		},
		Context: types.SessionContext{
			Preferences: types.UserPreferences{
				Timezone: "UTC",
			},
			Messages: []types.Message{
				{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestampFromTime(time.Now()),
					Role:      types.MessageRoleUser,
					Content:   "I prefer using dark mode", // Low confidence (0.65)
				},
			},
		},
	}

	ctx := context.Background()
	memories, err := extractor.ExtractFromSession(ctx, session)
	require.NoError(t, err)

	// Should only get high confidence memories (preferences have 0.9, patterns have 0.65)
	// With MinConfidence 0.8, we should only get the timezone preference (0.9)
	for _, mem := range memories {
		assert.GreaterOrEqual(t, mem.Metadata.Confidence, 0.8)
	}
}

func TestFilterByTopic(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:       true,
		MinConfidence: 0.5,
		Topics:        []string{"preferences"}, // Only extract preferences
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	session := &types.Session{
		ID:      types.GenerateID(),
		CreatedAt: types.NewTimestampFromTime(time.Now()),
		Metadata: types.SessionMetadata{
			OwnerID: "user123",
		},
		Context: types.SessionContext{
			Messages: []types.Message{
				{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestampFromTime(time.Now()),
					Role:      types.MessageRoleUser,
					Content:   "I decided to use PostgreSQL", // decision topic
				},
				{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestampFromTime(time.Now()),
					Role:      types.MessageRoleUser,
					Content:   "I prefer using dark mode", // preference topic
				},
			},
		},
	}

	ctx := context.Background()
	memories, err := extractor.ExtractFromSession(ctx, session)
	require.NoError(t, err)

	// Should only get preference memories
	for _, mem := range memories {
		assert.Equal(t, "preferences", mem.Topic)
	}
}

func TestExcludeTopic(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:               true,
		MinConfidence:         0.5,
		MaxMemoriesPerSession: 10,
		ExcludedTopics:        []string{"metadata"},
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	session := &types.Session{
		ID:      types.GenerateID(),
		CreatedAt: types.NewTimestampFromTime(time.Now()),
		Metadata: types.SessionMetadata{
			OwnerID: "user123",
			Labels: map[string]string{
				"project": "test",
			},
		},
		Context: types.SessionContext{
			Messages: []types.Message{
				{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestampFromTime(time.Now()),
					Role:      types.MessageRoleUser,
					Content:   "I prefer using dark mode",
				},
			},
		},
	}

	ctx := context.Background()
	memories, err := extractor.ExtractFromSession(ctx, session)
	require.NoError(t, err)

	// Should not have any metadata topic memories
	for _, mem := range memories {
		assert.NotEqual(t, "metadata", mem.Topic)
	}
}

func TestMaxMemoriesPerSession(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:               true,
		MinConfidence:         0.5,
		MaxMemoriesPerSession: 3, // Limit to 3
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	session := &types.Session{
		ID:      types.GenerateID(),
		CreatedAt: types.NewTimestampFromTime(time.Now()),
		Metadata: types.SessionMetadata{
			OwnerID: "user123",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
				"label3": "value3",
			},
		},
		Context: types.SessionContext{
			Preferences: types.UserPreferences{
				Timezone: "UTC",
				Language: "en",
				Theme:    "dark",
			},
			Messages: []types.Message{
				{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestampFromTime(time.Now()),
					Role:      types.MessageRoleUser,
					Content:   "I prefer using dark mode",
				},
			},
		},
	}

	ctx := context.Background()
	memories, err := extractor.ExtractFromSession(ctx, session)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(memories), 3)
}

func TestUpdateConfig(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:       true,
		MinConfidence: 0.6,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	// Update config
	newCfg := types.MemoryExtractionConfig{
		Enabled:               false,
		MinConfidence:         0.9,
		MaxMemoriesPerSession: 5,
	}

	err = extractor.UpdateConfig(newCfg)
	require.NoError(t, err)

	// Verify config was updated
	retrievedCfg := extractor.GetConfig()
	assert.Equal(t, false, retrievedCfg.Enabled)
	assert.Equal(t, 0.9, retrievedCfg.MinConfidence)
	assert.Equal(t, 5, retrievedCfg.MaxMemoriesPerSession)
}

func TestUpdateConfigClosed(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled: true,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	err = extractor.Close()
	require.NoError(t, err)

	newCfg := types.MemoryExtractionConfig{
		Enabled: false,
	}

	err = extractor.UpdateConfig(newCfg)
	assert.Error(t, err)
}

func TestExtractFromMessageGroupScope(t *testing.T) {
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

	extCfg := types.MemoryExtractionConfig{
		Enabled:       true,
		MinConfidence: 0.5,
	}

	extractor, err := NewExtractor(extCfg, store, log)
	require.NoError(t, err)
	defer extractor.Close()

	sessionID := types.GenerateID()
	message := types.Message{
		ID:        types.GenerateID(),
		Timestamp: types.NewTimestampFromTime(time.Now()),
		Role:      types.MessageRoleUser,
		Content:   "We decided to use PostgreSQL for the group database.",
	}

	ctx := context.Background()
	memories, err := extractor.ExtractFromMessage(ctx, sessionID, message, "group123", types.MemoryScopeGroup)
	require.NoError(t, err)
	assert.Greater(t, len(memories), 0)

	// Verify scope
	for _, mem := range memories {
		assert.Equal(t, types.MemoryScopeGroup, mem.Scope)
		assert.Equal(t, "group123", mem.OwnerID)
	}
}
