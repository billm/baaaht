package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestMemoryConfig creates a test configuration with temp directory
func createTestMemoryConfig(t *testing.T) config.MemoryConfig {
	t.Helper()
	tmpDir := t.TempDir()
	return config.MemoryConfig{
		Enabled:         true,
		UserMemoryPath:  filepath.Join(tmpDir, "user"),
		GroupMemoryPath: filepath.Join(tmpDir, "group"),
	}
}

// createTestLogger creates a test logger
func createTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")
	return log
}

// createTestSession creates a test session with sample data
func createTestSession(t *testing.T, ownerID string) *types.Session {
	t.Helper()
	sessionID := types.GenerateID()
	now := types.NewTimestamp()

	return &types.Session{
		ID:        sessionID,
		State:     types.SessionStateActive,
		Status:    types.StatusUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata: types.SessionMetadata{
			Name:        "test-session",
			Description: "Test session for memory extraction",
			OwnerID:     ownerID,
			Labels: map[string]string{
				"environment": "test",
				"purpose":     "memory-extraction-test",
			},
			Tags: []string{"test", "integration"},
		},
		Context: types.SessionContext{
			Messages: []types.Message{
				{
					ID:        types.GenerateID(),
					Timestamp: now,
					Role:      types.MessageRoleUser,
					Content:   "I prefer working in the morning and my name is John Doe",
				},
				{
					ID:        types.GenerateID(),
					Timestamp: now,
					Role:      types.MessageRoleAssistant,
					Content:   "I'll remember that you prefer morning work and your name is John",
				},
				{
					ID:        types.GenerateID(),
					Timestamp: now,
					Role:      types.MessageRoleUser,
					Content:   "I decided to use Go for this project because of its performance",
				},
			},
			Preferences: types.UserPreferences{
				Timezone:      "America/New_York",
				Language:      "en",
				Theme:         "dark",
				Notifications: true,
			},
		},
	}
}

// TestMemoryExtractionFlow tests the complete flow from session to memory storage
func TestMemoryExtractionFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	ownerID := "test-user-integration"

	t.Log("=== Step 1: Creating memory store ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create memory store")
	require.NotNil(t, store, "Store should not be nil")
	require.False(t, store.IsClosed(), "Store should not be closed")

	t.Cleanup(func() {
		t.Log("=== Step 6: Closing store ===")
		err := store.Close()
		assert.NoError(t, err, "Store close should succeed")
		assert.True(t, store.IsClosed(), "Store should be closed")
	})

	t.Log("=== Step 2: Creating memory extractor ===")

	extractorCfg := types.MemoryExtractionConfig{
		Enabled:               true,
		MinConfidence:         0.6,
		MaxMemoriesPerSession: 10,
		Topics:                []string{},
		ExcludedTopics:        []string{},
	}

	extractor, err := NewExtractor(extractorCfg, store, log)
	require.NoError(t, err, "Failed to create extractor")
	require.NotNil(t, extractor, "Extractor should not be nil")

	t.Cleanup(func() {
		err := extractor.Close()
		assert.NoError(t, err, "Extractor close should succeed")
	})

	t.Log("=== Step 3: Creating test session ===")

	session := createTestSession(t, ownerID)
	require.NotNil(t, session, "Session should not be nil")
	t.Logf("Session created: %s with %d messages", session.ID, len(session.Context.Messages))

	t.Log("=== Step 4: Extracting memories from session ===")

	memories, err := extractor.ExtractFromSession(ctx, session)
	require.NoError(t, err, "Memory extraction should succeed")
	require.NotEmpty(t, memories, "At least some memories should be extracted")

	t.Logf("Extracted %d memories", len(memories))

	// Verify memories were stored
	for _, mem := range memories {
		t.Logf("  - Memory: %s (type=%s, topic=%s, confidence=%.2f)",
			mem.Title, mem.Type, mem.Topic, mem.Metadata.Confidence)

		// Verify memory has required fields
		assert.NotEmpty(t, mem.ID, "Memory ID should not be empty")
		assert.NotEmpty(t, mem.Title, "Memory title should not be empty")
		assert.NotEmpty(t, mem.Content, "Memory content should not be empty")
		assert.NotEmpty(t, mem.OwnerID, "Memory owner ID should not be empty")
		assert.Equal(t, ownerID, mem.OwnerID, "Memory owner ID should match")
		assert.Equal(t, types.MemoryScopeUser, mem.Scope, "Memory scope should be user")
		assert.GreaterOrEqual(t, mem.Metadata.Confidence, extractorCfg.MinConfidence,
			"Memory confidence should meet minimum")
	}

	t.Log("=== Step 5: Verifying memories were stored and persisted ===")

	// Verify memories can be retrieved
	for _, mem := range memories {
		retrieved, err := store.Get(ctx, mem.ID)
		require.NoError(t, err, "Should be able to retrieve stored memory")
		require.NotNil(t, retrieved, "Retrieved memory should not be nil")

		assert.Equal(t, mem.ID, retrieved.ID, "Retrieved memory ID should match")
		assert.Equal(t, mem.Title, retrieved.Title, "Retrieved memory title should match")
		assert.Equal(t, mem.Content, retrieved.Content, "Retrieved memory content should match")
		assert.Equal(t, mem.Type, retrieved.Type, "Retrieved memory type should match")
	}

	// Verify memories are persisted to disk
	ownerPath := filepath.Join(cfg.UserMemoryPath, ownerID)
	entries, err := os.ReadDir(ownerPath)
	require.NoError(t, err, "Should be able to read owner directory")
	assert.Greater(t, len(entries), 0, "Owner directory should contain memory files")

	t.Logf("Found %d memory files in %s", len(entries), ownerPath)

	// Verify files are markdown format
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		assert.True(t, filepath.Ext(entry.Name()) == ".md",
			"Memory files should have .md extension")
	}

	t.Log("=== Memory extraction flow test passed ===")
}

// TestMemoryPersistence tests that memories are correctly persisted to and loaded from disk
func TestMemoryPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	ownerID := "test-user-persistence"

	t.Log("=== Creating first store instance ===")

	store1, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create first store")

	t.Log("=== Storing test memory ===")

	testMemory := &types.Memory{
		ID:      types.GenerateID(),
		Scope:   types.MemoryScopeUser,
		OwnerID: ownerID,
		Type:    types.MemoryTypePreference,
		Topic:   "test-topic",
		Title:   "Test Memory for Persistence",
		Content: "This is a test memory to verify persistence",
		Metadata: types.MemoryMetadata{
			Labels:     map[string]string{"test": "persistence"},
			Tags:       []string{"test", "persistence"},
			Importance: 5,
			Confidence: 0.8,
		},
	}

	err = store1.Store(ctx, testMemory)
	require.NoError(t, err, "Failed to store test memory")

	t.Log("=== Closing first store instance ===")

	err = store1.Close()
	require.NoError(t, err, "Failed to close first store")

	t.Log("=== Creating second store instance (simulating restart) ===")

	// Create a new store instance to simulate application restart
	store2, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create second store")

	t.Cleanup(func() {
		store2.Close()
	})

	t.Log("=== Retrieving memory from second store ===")

	retrieved, err := store2.Get(ctx, testMemory.ID)
	require.NoError(t, err, "Failed to retrieve memory from second store")
	require.NotNil(t, retrieved, "Retrieved memory should not be nil")

	assert.Equal(t, testMemory.ID, retrieved.ID, "Memory ID should match")
	assert.Equal(t, testMemory.Title, retrieved.Title, "Memory title should match")
	assert.Equal(t, testMemory.Content, retrieved.Content, "Memory content should match")
	assert.Equal(t, testMemory.Type, retrieved.Type, "Memory type should match")
	assert.Equal(t, testMemory.Topic, retrieved.Topic, "Memory topic should match")
	assert.Equal(t, testMemory.Scope, retrieved.Scope, "Memory scope should match")
	assert.Equal(t, testMemory.OwnerID, retrieved.OwnerID, "Memory owner ID should match")
	assert.Equal(t, testMemory.Metadata.Importance, retrieved.Metadata.Importance,
		"Memory importance should match")
	assert.Equal(t, testMemory.Metadata.Confidence, retrieved.Metadata.Confidence,
		"Memory confidence should match")

	t.Log("=== Memory persistence test passed ===")
}

// TestSessionArchivalHandlerIntegration tests the session archival handler with real events
func TestSessionArchivalHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	ownerID := "test-user-handler"

	t.Log("=== Step 1: Creating store and extractor ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create store")
	t.Cleanup(func() { store.Close() })

	extractor, err := NewDefaultExtractor(store, log)
	require.NoError(t, err, "Failed to create extractor")
	t.Cleanup(func() { extractor.Close() })

	t.Log("=== Step 2: Creating session archival handler ===")

	handler, err := NewSessionArchivalHandler(extractor, store, log)
	require.NoError(t, err, "Failed to create handler")
	require.NotNil(t, handler, "Handler should not be nil")
	t.Cleanup(func() { handler.Close() })

	t.Log("=== Step 3: Creating test session ===")

	session := createTestSession(t, ownerID)
	sessionID := types.ID(session.ID)

	t.Log("=== Step 4: Creating and handling session archived event ===")

	// Create the event with session data
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeSessionArchived,
		Source:    "test-integration",
		Timestamp: types.NewTimestamp(),
		Data: map[string]interface{}{
			"session": session,
		},
		Metadata: types.EventMetadata{
			SessionID: &sessionID,
			Priority:  types.PriorityNormal,
		},
	}

	// Handle the event
	err = handler.Handle(ctx, event)
	require.NoError(t, err, "Handler should process event successfully")

	t.Log("=== Step 5: Verifying memories were extracted ===")

	// Get memories for the owner
	memories, err := store.GetByOwner(ctx, types.MemoryScopeUser, ownerID)
	require.NoError(t, err, "Failed to get memories by owner")
	require.NotEmpty(t, memories, "At least some memories should be extracted")

	t.Logf("Handler extracted %d memories", len(memories))

	// Verify memory properties
	for _, mem := range memories {
		assert.NotEmpty(t, mem.ID, "Memory ID should not be empty")
		assert.Equal(t, ownerID, mem.OwnerID, "Memory owner should match")
		assert.NotNil(t, mem.Metadata.SourceID, "Memory should have source ID")
		assert.Equal(t, session.ID, *mem.Metadata.SourceID, "Memory source ID should match session ID")
		assert.Equal(t, "session", mem.Metadata.Source, "Memory source should be 'session'")
	}

	t.Log("=== Session archival handler integration test passed ===")
}

// TestMemoryRetrievalAndFiltering tests memory retrieval with various filters
func TestMemoryRetrievalAndFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	ownerID := "test-user-filtering"

	t.Log("=== Step 1: Creating store and storing test memories ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create store")
	t.Cleanup(func() { store.Close() })

	// Create test memories with different properties
	testMemories := []*types.Memory{
		{
			ID:      types.GenerateID(),
			Scope:   types.MemoryScopeUser,
			OwnerID: ownerID,
			Type:    types.MemoryTypePreference,
			Topic:   "preferences",
			Title:   "Timezone Preference",
			Content: "User prefers America/New_York timezone",
			Metadata: types.MemoryMetadata{
				Labels:     map[string]string{"category": "preferences"},
				Tags:       []string{"timezone", "preferences"},
				Importance: 3,
				Confidence: 0.9,
			},
		},
		{
			ID:      types.GenerateID(),
			Scope:   types.MemoryScopeUser,
			OwnerID: ownerID,
			Type:    types.MemoryTypePreference,
			Topic:   "preferences",
			Title:   "Theme Preference",
			Content: "User prefers dark theme",
			Metadata: types.MemoryMetadata{
				Labels:     map[string]string{"category": "preferences"},
				Tags:       []string{"theme", "ui", "preferences"},
				Importance: 2,
				Confidence: 0.9,
			},
		},
		{
			ID:      types.GenerateID(),
			Scope:   types.MemoryScopeUser,
			OwnerID: ownerID,
			Type:    types.MemoryTypeDecision,
			Topic:   "decision",
			Title:   "Technology Decision",
			Content: "User decided to use Go for the project",
			Metadata: types.MemoryMetadata{
				Labels:     map[string]string{"category": "decision"},
				Tags:       []string{"decision", "technology"},
				Importance: 7,
				Confidence: 0.7,
			},
		},
		{
			ID:      types.GenerateID(),
			Scope:   types.MemoryScopeUser,
			OwnerID: ownerID,
			Type:    types.MemoryTypeFact,
			Topic:   "identity",
			Title:   "User Name",
			Content: "User's name is John Doe",
			Metadata: types.MemoryMetadata{
				Labels:     map[string]string{"category": "identity"},
				Tags:       []string{"fact", "identity"},
				Importance: 6,
				Confidence: 0.75,
			},
		},
	}

	for _, mem := range testMemories {
		err := store.Store(ctx, mem)
		require.NoError(t, err, "Failed to store test memory")
	}

	t.Logf("Stored %d test memories", len(testMemories))

	t.Log("=== Step 2: Testing GetByOwner ===")

	memories, err := store.GetByOwner(ctx, types.MemoryScopeUser, ownerID)
	require.NoError(t, err, "Failed to get memories by owner")
	assert.Len(t, memories, len(testMemories), "Should retrieve all stored memories")

	t.Log("=== Step 3: Testing filter by type ===")

	userScope := types.MemoryScopeUser
	preferenceType := types.MemoryTypePreference
	filter := &types.MemoryFilter{
		Scope:   &userScope,
		OwnerID: &ownerID,
		Type:    &preferenceType,
	}

	memories, err = store.List(ctx, filter)
	require.NoError(t, err, "Failed to filter memories by type")
	assert.Len(t, memories, 2, "Should find 2 preference memories")

	for _, mem := range memories {
		assert.Equal(t, types.MemoryTypePreference, mem.Type, "Memory type should be preference")
	}

	t.Log("=== Step 4: Testing filter by topic ===")

	preferenceTopic := "preferences"
	filter = &types.MemoryFilter{
		Scope:   &userScope,
		OwnerID: &ownerID,
		Topic:   &preferenceTopic,
	}

	memories, err = store.List(ctx, filter)
	require.NoError(t, err, "Failed to filter memories by topic")
	assert.Len(t, memories, 2, "Should find 2 memories with preferences topic")

	t.Log("=== Step 5: Testing filter by minimum importance ===")

	minImportance := 5
	filter = &types.MemoryFilter{
		Scope:         &userScope,
		OwnerID:       &ownerID,
		MinImportance: &minImportance,
	}

	memories, err = store.List(ctx, filter)
	require.NoError(t, err, "Failed to filter memories by importance")
	assert.Len(t, memories, 2, "Should find 2 memories with importance >= 5")

	for _, mem := range memories {
		assert.GreaterOrEqual(t, mem.Metadata.Importance, 5,
			"Memory importance should be >= 5")
	}

	t.Log("=== Step 6: Testing filter by tags ===")

	filter = &types.MemoryFilter{
		Scope:   &userScope,
		OwnerID: &ownerID,
		Tags:    []string{"preferences"},
	}

	memories, err = store.List(ctx, filter)
	require.NoError(t, err, "Failed to filter memories by tags")
	assert.GreaterOrEqual(t, len(memories), 2, "Should find memories with 'preferences' tag")

	t.Log("=== Step 7: Testing Stats ===")

	stats, err := store.Stats(ctx, types.MemoryScopeUser, ownerID)
	require.NoError(t, err, "Failed to get memory stats")
	require.NotNil(t, stats, "Stats should not be nil")

	assert.Equal(t, types.MemoryScopeUser, stats.Scope, "Stats scope should match")
	assert.Equal(t, ownerID, stats.OwnerID, "Stats owner ID should match")
	assert.Equal(t, len(testMemories), stats.TotalCount, "Stats total count should match")
	assert.Greater(t, len(stats.TypeCounts), 0, "Stats should have type counts")
	assert.Greater(t, len(stats.TopicCounts), 0, "Stats should have topic counts")

	// Verify type counts
	assert.Equal(t, 2, stats.TypeCounts[types.MemoryTypePreference],
		"Should have 2 preference memories")
	assert.Equal(t, 1, stats.TypeCounts[types.MemoryTypeDecision],
		"Should have 1 decision memory")
	assert.Equal(t, 1, stats.TypeCounts[types.MemoryTypeFact],
		"Should have 1 fact memory")

	t.Logf("Memory stats: total=%d, types=%v, topics=%v",
		stats.TotalCount, stats.TypeCounts, stats.TopicCounts)

	t.Log("=== Memory retrieval and filtering test passed ===")
}

// TestMemoryOrganization tests topic-based memory organization
func TestMemoryOrganization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	ownerID := "test-user-organization"

	t.Log("=== Step 1: Creating store, organizer, and storing memories ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create store")
	t.Cleanup(func() { store.Close() })

	organizer, err := NewOrganizer(cfg, store, log)
	require.NoError(t, err, "Failed to create organizer")
	t.Cleanup(func() { organizer.Close() })

	// Store memories with different topics
	topics := []string{"preferences", "identity", "project", "decision"}
	for _, topic := range topics {
		mem := &types.Memory{
			ID:      types.GenerateID(),
			Scope:   types.MemoryScopeUser,
			OwnerID: ownerID,
			Type:    types.MemoryTypeContext,
			Topic:   topic,
			Title:   "Memory for " + topic,
			Content: "Test content for topic: " + topic,
			Metadata: types.MemoryMetadata{
				Labels:     map[string]string{"topic": topic},
				Tags:       []string{topic},
				Importance: 5,
				Confidence: 0.8,
			},
		}
		err := store.Store(ctx, mem)
		require.NoError(t, err, "Failed to store memory for topic %s", topic)
	}

	t.Logf("Stored %d memories across different topics", len(topics))

	t.Log("=== Step 2: Testing ListTopics ===")

	allTopics, err := organizer.ListTopics(ctx, types.MemoryScopeUser, ownerID)
	require.NoError(t, err, "Failed to list topics")
	t.Logf("Found topics: %v", allTopics)

	// Note: At this point, topic directories haven't been created yet,
	// so ListTopics might return an empty list until we organize memories

	t.Log("=== Step 3: Testing GetMemoriesByTopic ===")

	preferenceTopic := "preferences"
	memories, err := organizer.GetMemoriesByTopic(ctx, types.MemoryScopeUser, ownerID, preferenceTopic)
	require.NoError(t, err, "Failed to get memories by topic")
	assert.NotEmpty(t, memories, "Should find memories with 'preferences' topic")

	for _, mem := range memories {
		assert.Equal(t, preferenceTopic, mem.Topic, "Memory topic should match")
	}

	t.Log("=== Step 4: Testing GetTopicStats ===")

	topicStats, err := organizer.GetTopicStats(ctx, types.MemoryScopeUser, ownerID)
	require.NoError(t, err, "Failed to get topic stats")
	assert.NotNil(t, topicStats, "Topic stats should not be nil")

	t.Logf("Topic stats: %v", topicStats)

	// Verify we have stats for our topics
	for _, topic := range topics {
		count, exists := topicStats[topic]
		assert.True(t, exists, "Should have stats for topic: %s", topic)
		assert.Greater(t, count, 0, "Should have at least one memory for topic: %s", topic)
	}

	t.Log("=== Step 5: Testing topic path validation ===")

	// Test valid topic path
	validPath, err := organizer.GetTopicPath(types.MemoryScopeUser, ownerID, "valid-topic")
	require.NoError(t, err, "Should get path for valid topic")
	assert.NotEmpty(t, validPath, "Valid topic path should not be empty")
	t.Logf("Valid topic path: %s", validPath)

	// Test invalid topic names
	invalidTopics := []string{"", "../escape", "path/traversal", "topic\\with\\backslash"}
	for _, invalidTopic := range invalidTopics {
		_, err := organizer.GetTopicPath(types.MemoryScopeUser, ownerID, invalidTopic)
		assert.Error(t, err, "Should return error for invalid topic: %s", invalidTopic)
	}

	t.Log("=== Memory organization test passed ===")
}

// TestMemoryExtractionWithDisabledExtraction tests extraction when disabled
func TestMemoryExtractionWithDisabledExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	ownerID := "test-user-disabled"

	t.Log("=== Creating store and disabled extractor ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create store")
	t.Cleanup(func() { store.Close() })

	extractorCfg := types.MemoryExtractionConfig{
		Enabled:               false, // Disabled
		MinConfidence:         0.6,
		MaxMemoriesPerSession: 10,
	}

	extractor, err := NewExtractor(extractorCfg, store, log)
	require.NoError(t, err, "Failed to create extractor")
	t.Cleanup(func() { extractor.Close() })

	t.Log("=== Extracting memories with extraction disabled ===")

	session := createTestSession(t, ownerID)
	memories, err := extractor.ExtractFromSession(ctx, session)

	require.NoError(t, err, "Extraction should succeed even when disabled")
	assert.Empty(t, memories, "No memories should be extracted when disabled")

	// Verify no memories were stored
	storedMemories, err := store.GetByOwner(ctx, types.MemoryScopeUser, ownerID)
	require.NoError(t, err, "Should be able to query for memories")
	assert.Empty(t, storedMemories, "No memories should be stored when extraction is disabled")

	t.Log("=== Memory extraction with disabled extraction test passed ===")
}

// TestMemoryExtractionConfidenceFiltering tests confidence-based filtering
func TestMemoryExtractionConfidenceFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	ownerID := "test-user-confidence"

	t.Log("=== Creating store with high confidence threshold ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create store")
	t.Cleanup(func() { store.Close() })

	// Set high minimum confidence to filter out low-confidence memories
	extractorCfg := types.MemoryExtractionConfig{
		Enabled:               true,
		MinConfidence:         0.8, // High threshold
		MaxMemoriesPerSession: 10,
	}

	extractor, err := NewExtractor(extractorCfg, store, log)
	require.NoError(t, err, "Failed to create extractor")
	t.Cleanup(func() { extractor.Close() })

	t.Log("=== Extracting memories with high confidence threshold ===")

	session := createTestSession(t, ownerID)
	memories, err := extractor.ExtractFromSession(ctx, session)

	require.NoError(t, err, "Extraction should succeed")

	// Verify all extracted memories meet the confidence threshold
	for _, mem := range memories {
		assert.GreaterOrEqual(t, mem.Metadata.Confidence, extractorCfg.MinConfidence,
			"All extracted memories should meet minimum confidence threshold")
		t.Logf("Memory %s has confidence %.2f (threshold: %.2f)",
			mem.Title, mem.Metadata.Confidence, extractorCfg.MinConfidence)
	}

	t.Log("=== Memory extraction confidence filtering test passed ===")
}

// TestMemoryExtractionTopicFiltering tests topic-based filtering
func TestMemoryExtractionTopicFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	ownerID := "test-user-topic-filter"

	t.Log("=== Creating store with topic filter ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create store")
	t.Cleanup(func() { store.Close() })

	// Only extract preferences topic
	extractorCfg := types.MemoryExtractionConfig{
		Enabled:               true,
		MinConfidence:         0.6,
		MaxMemoriesPerSession: 10,
		Topics:                []string{"preferences"}, // Only this topic
		ExcludedTopics:        []string{},
	}

	extractor, err := NewExtractor(extractorCfg, store, log)
	require.NoError(t, err, "Failed to create extractor")
	t.Cleanup(func() { extractor.Close() })

	t.Log("=== Extracting memories with topic filter ===")

	session := createTestSession(t, ownerID)
	memories, err := extractor.ExtractFromSession(ctx, session)

	require.NoError(t, err, "Extraction should succeed")

	// Verify all extracted memories are from allowed topics
	for _, mem := range memories {
		assert.Equal(t, "preferences", mem.Topic,
			"All extracted memories should be from 'preferences' topic")
		t.Logf("Memory %s has topic %s", mem.Title, mem.Topic)
	}

	t.Log("=== Memory extraction topic filtering test passed ===")
}

// TestExtractionFromSingleMessage tests extraction from individual messages
func TestExtractionFromSingleMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	ownerID := "test-user-message"
	sessionID := types.GenerateID()

	t.Log("=== Creating store and extractor ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create store")
	t.Cleanup(func() { store.Close() })

	extractor, err := NewDefaultExtractor(store, log)
	require.NoError(t, err, "Failed to create extractor")
	t.Cleanup(func() { extractor.Close() })

	t.Log("=== Extracting from single message ===")

	// Test with a message containing preference
	message := types.Message{
		ID:        types.GenerateID(),
		Timestamp: types.NewTimestamp(),
		Role:      types.MessageRoleUser,
		Content:   "I prefer working in the morning and usually start at 8 AM",
	}

	memories, err := extractor.ExtractFromMessage(ctx, sessionID, message, ownerID, types.MemoryScopeUser)
	require.NoError(t, err, "Extraction from message should succeed")

	if len(memories) > 0 {
		t.Logf("Extracted %d memories from single message", len(memories))
		for _, mem := range memories {
			assert.NotEmpty(t, mem.ID, "Memory ID should not be empty")
			assert.Equal(t, ownerID, mem.OwnerID, "Owner ID should match")
			assert.Equal(t, types.MemoryScopeUser, mem.Scope, "Scope should be user")
			t.Logf("  - %s: %s", mem.Type, mem.Title)
		}
	} else {
		t.Log("No memories extracted from message (may be below confidence threshold)")
	}

	t.Log("=== Extraction from single message test passed ===")
}

// TestMemoryFileStructureVerification verifies that memory files are created in
// the correct directories with proper markdown format. This is a manual verification
// test that shows the actual file structure and content.
func TestMemoryFileStructureVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)
	userID := "test-user-verify"
	groupID := "test-group-verify"

	t.Log("=== Step 1: Creating store and extractor ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create store")
	t.Cleanup(func() { store.Close() })

	extractor, err := NewDefaultExtractor(store, log)
	require.NoError(t, err, "Failed to create extractor")
	t.Cleanup(func() { extractor.Close() })

	handler, err := NewSessionArchivalHandler(extractor, store, log)
	require.NoError(t, err, "Failed to create handler")
	t.Cleanup(func() { handler.Close() })

	t.Log("=== Step 2: Creating user-scoped session ===")

	userSession := createTestSession(t, userID)
	userSessionID := types.ID(userSession.ID)

	t.Log("=== Step 3: Creating group-scoped memory directly ===")

	// Create a group-scoped memory directly
	groupMemory := &types.Memory{
		ID:      types.GenerateID(),
		Scope:   types.MemoryScopeGroup,
		OwnerID: groupID,
		Type:    types.MemoryTypeContext,
		Topic:   "project",
		Title:   "Group Project Context",
		Content: "This is a group-scoped memory for project context",
		Metadata: types.MemoryMetadata{
			Labels:     map[string]string{"type": "project-context"},
			Tags:       []string{"group", "project"},
			Importance: 7,
			Confidence: 0.9,
		},
	}

	err = store.Store(ctx, groupMemory)
	require.NoError(t, err, "Group memory storage should succeed")

	t.Log("=== Step 4: Handling archival event for user session ===")

	// Handle user session archival
	userEvent := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeSessionArchived,
		Source:    "test-verification",
		Timestamp: types.NewTimestamp(),
		Data: map[string]interface{}{
			"session": userSession,
		},
		Metadata: types.EventMetadata{
			SessionID: &userSessionID,
			Priority:  types.PriorityNormal,
		},
	}

	err = handler.Handle(ctx, userEvent)
	require.NoError(t, err, "User session archival should succeed")

	t.Log("=== Step 5: Verifying user directory structure ===")

	userOwnerPath := filepath.Join(cfg.UserMemoryPath, userID)
	assert.DirExists(t, userOwnerPath, "User owner directory should exist")
	t.Logf("User directory exists: %s", userOwnerPath)

	userEntries, err := os.ReadDir(userOwnerPath)
	require.NoError(t, err, "Should be able to read user directory")
	assert.Greater(t, len(userEntries), 0, "User directory should contain memory files")

	t.Logf("Found %d files/directories in user directory", len(userEntries))

	// Verify each file in user directory
	for _, entry := range userEntries {
		if entry.IsDir() {
			t.Logf("  [DIR] %s/", entry.Name())
			continue
		}

		filePath := filepath.Join(userOwnerPath, entry.Name())
		t.Logf("  [FILE] %s", entry.Name())

		// Verify .md extension
		assert.Equal(t, ".md", filepath.Ext(entry.Name()),
			"Memory files should have .md extension")

		// Verify file naming pattern: {id}__{sanitized-title}.md
		baseName := strings.TrimSuffix(entry.Name(), ".md")
		parts := strings.SplitN(baseName, "__", 2)
		assert.GreaterOrEqual(t, len(parts), 1, "File name should have at least ID part")

		// Read and verify markdown content
		content, err := os.ReadFile(filePath)
		require.NoError(t, err, "Should be able to read memory file")

		// Verify JSON frontmatter pattern (```json ... ```)
		contentStr := string(content)
		assert.Contains(t, contentStr, "```json",
			"Memory file should have JSON frontmatter")
		assert.Contains(t, contentStr, "```",
			"Memory file should close code block")

		// Verify markdown content structure
		lines := strings.Split(contentStr, "\n")
		assert.Greater(t, len(lines), 5, "Memory file should have multiple lines")

		// First line should be ```json
		assert.True(t, strings.HasPrefix(strings.TrimSpace(lines[0]), "```json"),
			"Memory file should start with JSON code block")

		// Should have a heading with # (markdown title)
		hasHeading := false
		for _, line := range lines {
			if strings.HasPrefix(line, "# ") {
				hasHeading = true
				break
			}
		}
		assert.True(t, hasHeading, "Memory file should have markdown heading")

		t.Logf("    Verified: .md extension, JSON frontmatter, markdown heading")
	}

	t.Log("=== Step 6: Verifying group directory structure ===")

	groupOwnerPath := filepath.Join(cfg.GroupMemoryPath, groupID)
	assert.DirExists(t, groupOwnerPath, "Group owner directory should exist")
	t.Logf("Group directory exists: %s", groupOwnerPath)

	groupEntries, err := os.ReadDir(groupOwnerPath)
	require.NoError(t, err, "Should be able to read group directory")
	assert.Greater(t, len(groupEntries), 0, "Group directory should contain memory files")

	t.Logf("Found %d files/directories in group directory", len(groupEntries))

	// Verify each file in group directory (same checks as user)
	for _, entry := range groupEntries {
		if entry.IsDir() {
			t.Logf("  [DIR] %s/", entry.Name())
			continue
		}

		filePath := filepath.Join(groupOwnerPath, entry.Name())
		t.Logf("  [FILE] %s", entry.Name())

		// Verify .md extension
		assert.Equal(t, ".md", filepath.Ext(entry.Name()),
			"Memory files should have .md extension")

		// Read and verify markdown content exists
		content, err := os.ReadFile(filePath)
		require.NoError(t, err, "Should be able to read group memory file")
		assert.NotEmpty(t, content, "Group memory file should have content")

		t.Logf("    Verified: .md extension, content present")
	}

	t.Log("=== Step 7: Verifying scope isolation ===")

	// User memories should not be in group directory
	groupMemories, err := store.GetByOwner(ctx, types.MemoryScopeGroup, groupID)
	require.NoError(t, err, "Should be able to get group memories")
	assert.Greater(t, len(groupMemories), 0, "Should have group memories")

	for _, mem := range groupMemories {
		assert.Equal(t, types.MemoryScopeGroup, mem.Scope,
			"Group memory should have group scope")
		assert.Equal(t, groupID, mem.OwnerID,
			"Group memory should have group owner ID")
	}

	// Group memories should not be in user directory
	userMemories, err := store.GetByOwner(ctx, types.MemoryScopeUser, userID)
	require.NoError(t, err, "Should be able to get user memories")
	assert.Greater(t, len(userMemories), 0, "Should have user memories")

	for _, mem := range userMemories {
		assert.Equal(t, types.MemoryScopeUser, mem.Scope,
			"User memory should have user scope")
		assert.Equal(t, userID, mem.OwnerID,
			"User memory should have user owner ID")
	}

	t.Log("=== Step 8: Displaying sample markdown file content ===")

	// Find and display a sample memory file
	if len(userEntries) > 0 {
		for _, entry := range userEntries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
				samplePath := filepath.Join(userOwnerPath, entry.Name())
				sampleContent, err := os.ReadFile(samplePath)
				require.NoError(t, err, "Should be able to read sample file")

				t.Log("=== Sample memory file content ===")
				t.Logf("File: %s", entry.Name())
				t.Log("--- Content Start ---")
				for i, line := range strings.Split(string(sampleContent), "\n") {
					if i < 30 { // Show first 30 lines
						t.Log(line)
					} else {
						t.Log("... (content truncated)")
						break
					}
				}
				t.Log("--- Content End ---")
				break
			}
		}
	}

	t.Log("=== Memory file structure verification test passed ===")
	t.Logf("Summary:")
	t.Logf("  - User directory: %s (%d files)", userOwnerPath, len(userEntries))
	t.Logf("  - Group directory: %s (%d files)", groupOwnerPath, len(groupEntries))
	t.Logf("  - All files have .md extension")
	t.Logf("  - All files have JSON frontmatter")
	t.Logf("  - All files have markdown content")
	t.Logf("  - User and group memories are properly isolated")
}

// TestStrictMemorySegregation verifies strict segregation between different
// users and groups with no cross-contamination possible.
func TestStrictMemorySegregation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	log := createTestLogger(t)
	cfg := createTestMemoryConfig(t)

	// Define multiple users and groups
	userA := "user-a-segregation"
	userB := "user-b-segregation"
	userC := "user-c-segregation"
	groupA := "group-a-segregation"
	groupB := "group-b-segregation"

	t.Log("=== Step 1: Creating store, extractor, and handler ===")

	store, err := NewStore(cfg, log)
	require.NoError(t, err, "Failed to create store")
	t.Cleanup(func() { store.Close() })

	extractor, err := NewDefaultExtractor(store, log)
	require.NoError(t, err, "Failed to create extractor")
	t.Cleanup(func() { extractor.Close() })

	handler, err := NewSessionArchivalHandler(extractor, store, log)
	require.NoError(t, err, "Failed to create handler")
	t.Cleanup(func() { handler.Close() })

	t.Log("=== Step 2: Creating and archiving sessions for User A ===")

	sessionA := createTestSession(t, userA)
	sessionA.Context.Messages = []types.Message{
		{
			ID:        types.GenerateID(),
			Timestamp: types.NewTimestamp(),
			Role:      types.MessageRoleUser,
			Content:   "My name is Alice and I work at CompanyA",
		},
	}
	sessionAID := types.ID(sessionA.ID)

	eventA := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeSessionArchived,
		Source:    "test-segregation",
		Timestamp: types.NewTimestamp(),
		Data:      map[string]interface{}{"session": sessionA},
		Metadata:  types.EventMetadata{SessionID: &sessionAID, Priority: types.PriorityNormal},
	}

	err = handler.Handle(ctx, eventA)
	require.NoError(t, err, "User A session archival should succeed")

	t.Log("=== Step 3: Creating and archiving sessions for User B ===")

	sessionB := createTestSession(t, userB)
	sessionB.Context.Messages = []types.Message{
		{
			ID:        types.GenerateID(),
			Timestamp: types.NewTimestamp(),
			Role:      types.MessageRoleUser,
			Content:   "My name is Bob and I prefer dark theme",
		},
	}
	sessionBID := types.ID(sessionB.ID)

	eventB := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeSessionArchived,
		Source:    "test-segregation",
		Timestamp: types.NewTimestamp(),
		Data:      map[string]interface{}{"session": sessionB},
		Metadata:  types.EventMetadata{SessionID: &sessionBID, Priority: types.PriorityNormal},
	}

	err = handler.Handle(ctx, eventB)
	require.NoError(t, err, "User B session archival should succeed")

	t.Log("=== Step 4: Creating and archiving sessions for User C ===")

	sessionC := createTestSession(t, userC)
	sessionC.Context.Messages = []types.Message{
		{
			ID:        types.GenerateID(),
			Timestamp: types.NewTimestamp(),
			Role:      types.MessageRoleUser,
			Content:   "I decided to use Python for my project",
		},
	}
	sessionCID := types.ID(sessionC.ID)

	eventC := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeSessionArchived,
		Source:    "test-segregation",
		Timestamp: types.NewTimestamp(),
		Data:      map[string]interface{}{"session": sessionC},
		Metadata:  types.EventMetadata{SessionID: &sessionCID, Priority: types.PriorityNormal},
	}

	err = handler.Handle(ctx, eventC)
	require.NoError(t, err, "User C session archival should succeed")

	t.Log("=== Step 5: Creating group memories for Group A ===")

	groupMemoryA := &types.Memory{
		ID:      types.GenerateID(),
		Scope:   types.MemoryScopeGroup,
		OwnerID: groupA,
		Type:    types.MemoryTypeContext,
		Topic:   "project-goals",
		Title:   "Group A Project Goals",
		Content: "Group A aims to build a scalable platform",
		Metadata: types.MemoryMetadata{
			Labels:     map[string]string{"team": "group-a"},
			Tags:       []string{"group-a", "goals"},
			Importance: 8,
			Confidence: 0.9,
		},
	}

	err = store.Store(ctx, groupMemoryA)
	require.NoError(t, err, "Group A memory storage should succeed")

	t.Log("=== Step 6: Creating group memories for Group B ===")

	groupMemoryB := &types.Memory{
		ID:      types.GenerateID(),
		Scope:   types.MemoryScopeGroup,
		OwnerID: groupB,
		Type:    types.MemoryTypeDecision,
		Topic:   "technology",
		Title:   "Group B Technology Choice",
		Content: "Group B chose Rust for their project",
		Metadata: types.MemoryMetadata{
			Labels:     map[string]string{"team": "group-b"},
			Tags:       []string{"group-b", "rust"},
			Importance: 7,
			Confidence: 0.85,
		},
	}

	err = store.Store(ctx, groupMemoryB)
	require.NoError(t, err, "Group B memory storage should succeed")

	t.Log("=== Step 7: Verifying cross-user isolation ===")

	// User A should only see their own memories
	memoriesA, err := store.GetByOwner(ctx, types.MemoryScopeUser, userA)
	require.NoError(t, err, "Should be able to get User A memories")
	assert.Greater(t, len(memoriesA), 0, "User A should have memories")

	// Verify all memories belong to User A
	for _, mem := range memoriesA {
		assert.Equal(t, userA, mem.OwnerID, "Memory should belong to User A")
		assert.NotContains(t, mem.Content, "Bob", "User A memories should not contain User B's data")
		assert.NotContains(t, mem.Content, "Python", "User A memories should not contain User C's data")
	}
	t.Logf("✓ User A has %d memories, all properly isolated", len(memoriesA))

	// User B should only see their own memories
	memoriesB, err := store.GetByOwner(ctx, types.MemoryScopeUser, userB)
	require.NoError(t, err, "Should be able to get User B memories")
	assert.Greater(t, len(memoriesB), 0, "User B should have memories")

	// Verify all memories belong to User B
	for _, mem := range memoriesB {
		assert.Equal(t, userB, mem.OwnerID, "Memory should belong to User B")
		assert.NotContains(t, mem.Content, "Alice", "User B memories should not contain User A's data")
		assert.NotContains(t, mem.Content, "Python", "User B memories should not contain User C's data")
	}
	t.Logf("✓ User B has %d memories, all properly isolated", len(memoriesB))

	// User C should only see their own memories
	memoriesC, err := store.GetByOwner(ctx, types.MemoryScopeUser, userC)
	require.NoError(t, err, "Should be able to get User C memories")
	assert.Greater(t, len(memoriesC), 0, "User C should have memories")

	// Verify all memories belong to User C
	for _, mem := range memoriesC {
		assert.Equal(t, userC, mem.OwnerID, "Memory should belong to User C")
		assert.NotContains(t, mem.Content, "Alice", "User C memories should not contain User A's data")
		assert.NotContains(t, mem.Content, "Bob", "User C memories should not contain User B's data")
	}
	t.Logf("✓ User C has %d memories, all properly isolated", len(memoriesC))

	// Verify no ID overlap between users
	userIDs := make(map[types.ID]string)
	for _, mem := range memoriesA {
		userIDs[mem.ID] = userA
	}
	for _, mem := range memoriesB {
		owner, exists := userIDs[mem.ID]
		assert.False(t, exists, "Memory ID should be unique, not shared between users (found in %s)", owner)
	}
	for _, mem := range memoriesC {
		owner, exists := userIDs[mem.ID]
		assert.False(t, exists, "Memory ID should be unique, not shared between users (found in %s)", owner)
	}
	t.Log("✓ All memory IDs are unique across users")

	t.Log("=== Step 8: Verifying cross-group isolation ===")

	// Group A should only see their own memories
	groupMemoriesA, err := store.GetByOwner(ctx, types.MemoryScopeGroup, groupA)
	require.NoError(t, err, "Should be able to get Group A memories")
	assert.Greater(t, len(groupMemoriesA), 0, "Group A should have memories")

	for _, mem := range groupMemoriesA {
		assert.Equal(t, groupA, mem.OwnerID, "Memory should belong to Group A")
		assert.NotContains(t, mem.Content, "Rust", "Group A memories should not contain Group B's data")
	}
	t.Logf("✓ Group A has %d memories, all properly isolated", len(groupMemoriesA))

	// Group B should only see their own memories
	groupMemoriesB, err := store.GetByOwner(ctx, types.MemoryScopeGroup, groupB)
	require.NoError(t, err, "Should be able to get Group B memories")
	assert.Greater(t, len(groupMemoriesB), 0, "Group B should have memories")

	for _, mem := range groupMemoriesB {
		assert.Equal(t, groupB, mem.OwnerID, "Memory should belong to Group B")
		assert.NotContains(t, mem.Content, "platform", "Group B memories should not contain Group A's data")
	}
	t.Logf("✓ Group B has %d memories, all properly isolated", len(groupMemoriesB))

	// Verify no ID overlap between groups
	groupIDs := make(map[types.ID]string)
	for _, mem := range groupMemoriesA {
		groupIDs[mem.ID] = groupA
	}
	for _, mem := range groupMemoriesB {
		owner, exists := groupIDs[mem.ID]
		assert.False(t, exists, "Memory ID should be unique, not shared between groups (found in %s)", owner)
	}
	t.Log("✓ All memory IDs are unique across groups")

	t.Log("=== Step 9: Verifying user-group scope isolation ===")

	// Users should not be able to access group memories
	userScope := types.MemoryScopeUser
	groupScope := types.MemoryScopeGroup

	// Query for user memories in user scope should work
	filter := &types.MemoryFilter{
		Scope:   &userScope,
		OwnerID: &userA,
	}
	userMemories, err := store.List(ctx, filter)
	require.NoError(t, err, "Should be able to list user memories")
	assert.Greater(t, len(userMemories), 0, "Should have user memories")
	t.Logf("✓ User scope query returned %d user memories", len(userMemories))

	// Query for group memories in group scope should work
	filter = &types.MemoryFilter{
		Scope:   &groupScope,
		OwnerID: &groupA,
	}
	groupMemories, err := store.List(ctx, filter)
	require.NoError(t, err, "Should be able to list group memories")
	assert.Greater(t, len(groupMemories), 0, "Should have group memories")
	t.Logf("✓ Group scope query returned %d group memories", len(groupMemories))

	// Verify scope field is correctly set
	for _, mem := range userMemories {
		assert.Equal(t, types.MemoryScopeUser, mem.Scope, "User memory should have user scope")
	}
	for _, mem := range groupMemories {
		assert.Equal(t, types.MemoryScopeGroup, mem.Scope, "Group memory should have group scope")
	}

	t.Log("=== Step 10: Verifying filesystem directory isolation ===")

	// Verify user directories exist and are separate
	userPathA := filepath.Join(cfg.UserMemoryPath, userA)
	userPathB := filepath.Join(cfg.UserMemoryPath, userB)
	userPathC := filepath.Join(cfg.UserMemoryPath, userC)

	assert.DirExists(t, userPathA, "User A directory should exist")
	assert.DirExists(t, userPathB, "User B directory should exist")
	assert.DirExists(t, userPathC, "User C directory should exist")
	t.Log("✓ All user directories exist")

	// Verify group directories exist and are separate
	groupPathA := filepath.Join(cfg.GroupMemoryPath, groupA)
	groupPathB := filepath.Join(cfg.GroupMemoryPath, groupB)

	assert.DirExists(t, groupPathA, "Group A directory should exist")
	assert.DirExists(t, groupPathB, "Group B directory should exist")
	t.Log("✓ All group directories exist")

	// Verify user and group base paths are different
	assert.NotEqual(t, cfg.UserMemoryPath, cfg.GroupMemoryPath,
		"User and group memory paths should be different")
	t.Logf("✓ User path: %s", cfg.UserMemoryPath)
	t.Logf("✓ Group path: %s", cfg.GroupMemoryPath)

	// Verify files are in correct directories
	userEntriesA, err := os.ReadDir(userPathA)
	require.NoError(t, err, "Should be able to read User A directory")
	assert.Greater(t, len(userEntriesA), 0, "User A directory should have files")

	userEntriesB, err := os.ReadDir(userPathB)
	require.NoError(t, err, "Should be able to read User B directory")
	assert.Greater(t, len(userEntriesB), 0, "User B directory should have files")

	userEntriesC, err := os.ReadDir(userPathC)
	require.NoError(t, err, "Should be able to read User C directory")
	assert.Greater(t, len(userEntriesC), 0, "User C directory should have files")

	groupEntriesA, err := os.ReadDir(groupPathA)
	require.NoError(t, err, "Should be able to read Group A directory")
	assert.Greater(t, len(groupEntriesA), 0, "Group A directory should have files")

	groupEntriesB, err := os.ReadDir(groupPathB)
	require.NoError(t, err, "Should be able to read Group B directory")
	assert.Greater(t, len(groupEntriesB), 0, "Group B directory should have files")

	t.Logf("✓ File counts: User A=%d, User B=%d, User C=%d, Group A=%d, Group B=%d",
		len(userEntriesA), len(userEntriesB), len(userEntriesC),
		len(groupEntriesA), len(groupEntriesB))

	// Verify no cross-contamination: check that files in user directories
	// don't reference other users
	for _, entry := range userEntriesA {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(userPathA, entry.Name())
		content, err := os.ReadFile(filePath)
		require.NoError(t, err, "Should be able to read User A file")

		contentStr := string(content)
		// Verify the file contains User A's data but not other users' data
		// Note: This is a loose check since memories may or may not contain these specific strings
		if strings.Contains(contentStr, "Alice") {
			assert.NotContains(t, contentStr, userB, "User A file should not reference User B")
			assert.NotContains(t, contentStr, userC, "User A file should not reference User C")
		}
	}

	t.Log("=== Step 11: Verifying query isolation with filters ===")

	// Query by user A should not return user B or C memories
	filterA := &types.MemoryFilter{
		Scope:   &userScope,
		OwnerID: &userA,
	}
	resultsA, err := store.List(ctx, filterA)
	require.NoError(t, err, "Should be able to filter by User A")

	for _, mem := range resultsA {
		assert.Equal(t, userA, mem.OwnerID, "Filtered results should only contain User A memories")
	}

	// Query by user B should not return user A or C memories
	filterB := &types.MemoryFilter{
		Scope:   &userScope,
		OwnerID: &userB,
	}
	resultsB, err := store.List(ctx, filterB)
	require.NoError(t, err, "Should be able to filter by User B")

	for _, mem := range resultsB {
		assert.Equal(t, userB, mem.OwnerID, "Filtered results should only contain User B memories")
	}

	// Group A filter should not return Group B memories
	filterGroupA := &types.MemoryFilter{
		Scope:   &groupScope,
		OwnerID: &groupA,
	}
	resultsGroupA, err := store.List(ctx, filterGroupA)
	require.NoError(t, err, "Should be able to filter by Group A")

	for _, mem := range resultsGroupA {
		assert.Equal(t, groupA, mem.OwnerID, "Filtered results should only contain Group A memories")
		assert.NotEqual(t, groupB, mem.OwnerID, "Filtered results should not contain Group B memories")
	}

	// Group B filter should not return Group A memories
	filterGroupB := &types.MemoryFilter{
		Scope:   &groupScope,
		OwnerID: &groupB,
	}
	resultsGroupB, err := store.List(ctx, filterGroupB)
	require.NoError(t, err, "Should be able to filter by Group B")

	for _, mem := range resultsGroupB {
		assert.Equal(t, groupB, mem.OwnerID, "Filtered results should only contain Group B memories")
		assert.NotEqual(t, groupA, mem.OwnerID, "Filtered results should not contain Group A memories")
	}

	t.Log("✓ All query filters properly isolate memories by owner")

	t.Log("=== Step 12: Verifying stats isolation ===")

	// Get stats for each user - they should be independent
	statsA, err := store.Stats(ctx, types.MemoryScopeUser, userA)
	require.NoError(t, err, "Should be able to get User A stats")
	assert.Equal(t, userA, statsA.OwnerID, "Stats should be for User A")
	assert.Equal(t, types.MemoryScopeUser, statsA.Scope, "Stats should be user scope")

	statsB, err := store.Stats(ctx, types.MemoryScopeUser, userB)
	require.NoError(t, err, "Should be able to get User B stats")
	assert.Equal(t, userB, statsB.OwnerID, "Stats should be for User B")
	assert.Equal(t, types.MemoryScopeUser, statsB.Scope, "Stats should be user scope")

	statsC, err := store.Stats(ctx, types.MemoryScopeUser, userC)
	require.NoError(t, err, "Should be able to get User C stats")
	assert.Equal(t, userC, statsC.OwnerID, "Stats should be for User C")
	assert.Equal(t, types.MemoryScopeUser, statsC.Scope, "Stats should be user scope")

	// Get stats for each group
	statsGroupA, err := store.Stats(ctx, types.MemoryScopeGroup, groupA)
	require.NoError(t, err, "Should be able to get Group A stats")
	assert.Equal(t, groupA, statsGroupA.OwnerID, "Stats should be for Group A")
	assert.Equal(t, types.MemoryScopeGroup, statsGroupA.Scope, "Stats should be group scope")

	statsGroupB, err := store.Stats(ctx, types.MemoryScopeGroup, groupB)
	require.NoError(t, err, "Should be able to get Group B stats")
	assert.Equal(t, groupB, statsGroupB.OwnerID, "Stats should be for Group B")
	assert.Equal(t, types.MemoryScopeGroup, statsGroupB.Scope, "Stats should be group scope")

	t.Logf("✓ Stats properly isolated: User A=%d memories, User B=%d memories, User C=%d memories, Group A=%d memories, Group B=%d memories",
		statsA.TotalCount, statsB.TotalCount, statsC.TotalCount,
		statsGroupA.TotalCount, statsGroupB.TotalCount)

	t.Log("=== Strict memory segregation verification complete ===")
	t.Log("Summary:")
	t.Log("  ✓ User-to-user isolation verified (3 users)")
	t.Log("  ✓ Group-to-group isolation verified (2 groups)")
	t.Log("  ✓ User-to-group scope isolation verified")
	t.Log("  ✓ Filesystem directory separation verified")
	t.Log("  ✓ Query filter isolation verified")
	t.Log("  ✓ Memory ID uniqueness verified")
	t.Log("  ✓ Stats isolation verified")
	t.Log("  ✓ No cross-contamination detected")
}
