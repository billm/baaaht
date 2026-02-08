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
