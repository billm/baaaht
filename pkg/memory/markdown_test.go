package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSerializeToMarkdown(t *testing.T) {
	mem := &types.Memory{
		ID:      types.NewID("mem123"),
		Scope:   types.MemoryScopeUser,
		OwnerID: "user456",
		Type:    types.MemoryTypeFact,
		Topic:   "preferences",
		Title:   "User Prefers Dark Mode",
		Content: "The user prefers dark mode in their applications.",
		Metadata: types.MemoryMetadata{
			Labels:     map[string]string{"category": "ui"},
			Tags:       []string{"ui", "preferences"},
			Source:     "manual",
			Importance: 5,
			Verified:   true,
			Confidence: 0.9,
			Version:    1,
		},
		CreatedAt: types.NewTimestampFromTime(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)),
		UpdatedAt: types.NewTimestampFromTime(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)),
	}

	result, err := SerializeToMarkdown(mem)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	// Verify JSON frontmatter exists
	assert.True(t, strings.HasPrefix(result, "```json\n"))
	assert.Contains(t, result, "\n```\n\n")

	// Verify title heading
	assert.Contains(t, result, "# User Prefers Dark Mode\n\n")

	// Verify content
	assert.Contains(t, result, "The user prefers dark mode in their applications.")

	// Verify metadata in JSON
	assert.Contains(t, result, `"id": "mem123"`)
	assert.Contains(t, result, `"scope": "user"`)
	assert.Contains(t, result, `"owner_id": "user456"`)
	assert.Contains(t, result, `"type": "fact"`)
	assert.Contains(t, result, `"topic": "preferences"`)
	assert.Contains(t, result, `"category": "ui"`)
	assert.Contains(t, result, `"source": "manual"`)
	assert.Contains(t, result, `"importance": 5`)
	assert.Contains(t, result, `"verified": true`)
	assert.Contains(t, result, `"confidence": 0.9`)
	assert.Contains(t, result, `"version": 1`)
}

func TestSerializeToMarkdownWithOptionalFields(t *testing.T) {
	sourceID := types.NewID("session789")
	pastTime := types.NewTimestampFromTime(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	futureTime := types.NewTimestampFromTime(time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC))

	mem := &types.Memory{
		ID:        types.NewID("mem456"),
		Scope:     types.MemoryScopeGroup,
		OwnerID:   "group123",
		Type:      types.MemoryTypeContext,
		Topic:     "project",
		Title:     "Project Info",
		Content:   "Important project information",
		AccessedAt: &pastTime,
		ExpiresAt:  &futureTime,
		Metadata: types.MemoryMetadata{
			Source:    "session",
			SourceID:  &sourceID,
			Importance: 8,
		},
		CreatedAt: pastTime,
		UpdatedAt: pastTime,
	}

	result, err := SerializeToMarkdown(mem)
	require.NoError(t, err)

	// Verify optional fields are included
	assert.Contains(t, result, `"accessed_at":`)
	assert.Contains(t, result, `"expires_at":`)
	assert.Contains(t, result, `"source_id": "session789"`)
}

func TestSerializeToMarkdownMinimal(t *testing.T) {
	mem := &types.Memory{
		ID:        types.NewID("mem789"),
		Scope:     types.MemoryScopeUser,
		OwnerID:   "user123",
		Type:      types.MemoryTypeDecision,
		Topic:     "",
		Title:     "Simple Decision",
		Content:   "Made a decision",
		CreatedAt: types.NewTimestamp(),
		UpdatedAt: types.NewTimestamp(),
	}

	result, err := SerializeToMarkdown(mem)
	require.NoError(t, err)

	// Verify basic structure
	assert.True(t, strings.HasPrefix(result, "```json\n"))
	assert.Contains(t, result, "# Simple Decision\n\n")
	assert.Contains(t, result, "Made a decision")

	// Empty topic should still work
	assert.Contains(t, result, `"topic": ""`)
}

func TestSerializeToMarkdownNil(t *testing.T) {
	_, err := SerializeToMarkdown(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory cannot be nil")
}

func TestDeserializeFromMarkdown(t *testing.T) {
	markdown := "```json\n" +
		"{\n" +
		"  \"id\": \"mem123\",\n" +
		"  \"scope\": \"user\",\n" +
		"  \"owner_id\": \"user456\",\n" +
		"  \"type\": \"fact\",\n" +
		"  \"topic\": \"preferences\",\n" +
		"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"labels\": {\n" +
		"    \"category\": \"ui\"\n" +
		"  },\n" +
		"  \"tags\": [\"ui\", \"preferences\"],\n" +
		"  \"source\": \"manual\",\n" +
		"  \"importance\": 5,\n" +
		"  \"verified\": true,\n" +
		"  \"confidence\": 0.9,\n" +
		"  \"version\": 1\n" +
		"}\n" +
		"```\n\n" +
		"# User Prefers Dark Mode\n\n" +
		"The user prefers dark mode in their applications.\n"

	mem, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeUser, "user456")
	require.NoError(t, err)
	require.NotNil(t, mem)

	assert.Equal(t, "mem123", mem.ID.String())
	assert.Equal(t, types.MemoryScopeUser, mem.Scope)
	assert.Equal(t, "user456", mem.OwnerID)
	assert.Equal(t, types.MemoryTypeFact, mem.Type)
	assert.Equal(t, "preferences", mem.Topic)
	assert.Equal(t, "User Prefers Dark Mode", mem.Title)
	assert.Equal(t, "The user prefers dark mode in their applications.", mem.Content)
	assert.Equal(t, "ui", mem.Metadata.Labels["category"])
	assert.Equal(t, []string{"ui", "preferences"}, mem.Metadata.Tags)
	assert.Equal(t, "manual", mem.Metadata.Source)
	assert.Equal(t, 5, mem.Metadata.Importance)
	assert.True(t, mem.Metadata.Verified)
	assert.Equal(t, 0.9, mem.Metadata.Confidence)
	assert.Equal(t, 1, mem.Metadata.Version)
}

func TestDeserializeFromMarkdownWithOptionalFields(t *testing.T) {
	markdown := "```json\n" +
		"{\n" +
		"  \"id\": \"mem456\",\n" +
		"  \"type\": \"context\",\n" +
		"  \"topic\": \"project\",\n" +
		"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"accessed_at\": \"2024-01-02T12:00:00Z\",\n" +
		"  \"expires_at\": \"2025-12-31T23:59:59Z\",\n" +
		"  \"source\": \"session\",\n" +
		"  \"source_id\": \"session789\",\n" +
		"  \"importance\": 8\n" +
		"}\n" +
		"```\n\n" +
		"# Project Info\n\n" +
		"Important project information\n"

	mem, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeGroup, "group123")
	require.NoError(t, err)
	require.NotNil(t, mem)

	assert.NotNil(t, mem.AccessedAt)
	assert.NotNil(t, mem.ExpiresAt)
	assert.Equal(t, "session", mem.Metadata.Source)
	assert.NotNil(t, mem.Metadata.SourceID)
	assert.Equal(t, "session789", mem.Metadata.SourceID.String())
	assert.Equal(t, 8, mem.Metadata.Importance)
}

func TestDeserializeFromMarkdownMinimal(t *testing.T) {
	markdown := "```json\n" +
		"{\n" +
		"  \"id\": \"mem789\",\n" +
		"  \"type\": \"decision\",\n" +
		"  \"topic\": \"\",\n" +
		"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
		"}\n" +
		"```\n\n" +
		"# Simple Decision\n\n" +
		"Made a decision\n"

	mem, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeUser, "user123")
	require.NoError(t, err)
	require.NotNil(t, mem)

	assert.Equal(t, "mem789", mem.ID.String())
	assert.Equal(t, types.MemoryTypeDecision, mem.Type)
	assert.Equal(t, "", mem.Topic)
	assert.Equal(t, "Simple Decision", mem.Title)
	assert.Equal(t, "Made a decision", mem.Content)
}

func TestDeserializeFromMarkdownEmpty(t *testing.T) {
	_, err := DeserializeFromMarkdown([]byte(""), types.MemoryScopeUser, "user123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory data cannot be empty")
}

func TestDeserializeFromMarkdownInvalidFormat(t *testing.T) {
	// Missing opening ```json
	markdown := "# Just a heading\nSome content\n"

	_, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeUser, "user123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing metadata")
}

func TestDeserializeFromMarkdownMissingClosing(t *testing.T) {
	// Missing closing ```
	markdown := " ```json\n" +
		"{\n" +
		"  \"id\": \"mem123\",\n" +
		"  \"type\": \"fact\",\n" +
		"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
		"}\n" +
		"\n" +
		"# Title\n" +
		"Content\n"

	_, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeUser, "user123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing closing metadata")
}

func TestDeserializeFromMarkdownInvalidJSON(t *testing.T) {
	markdown := " ```json\n" +
		"{ invalid json }\n" +
		"```\n\n" +
		"# Title\n" +
		"Content\n"

	_, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeUser, "user123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse memory metadata")
}

func TestDeserializeFromMarkdownMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		markdown string
		wantErr string
	}{
		{
			name: "missing id",
			markdown: " ```json\n" +
				"{\n" +
				"  \"type\": \"fact\",\n" +
				"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
				"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
				"}\n" +
				"```\n\n" +
				"# Title\n" +
				"Content\n",
			wantErr: "missing required field: id",
		},
		{
			name: "missing type",
			markdown: " ```json\n" +
				"{\n" +
				"  \"id\": \"mem123\",\n" +
				"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
				"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
				"}\n" +
				"```\n\n" +
				"# Title\n" +
				"Content\n",
			wantErr: "missing required field: type",
		},
		{
			name: "missing created_at",
			markdown: " ```json\n" +
				"{\n" +
				"  \"id\": \"mem123\",\n" +
				"  \"type\": \"fact\",\n" +
				"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
				"}\n" +
				"```\n\n" +
				"# Title\n" +
				"Content\n",
			wantErr: "missing required field: created_at",
		},
		{
			name: "missing updated_at",
			markdown: " ```json\n" +
				"{\n" +
				"  \"id\": \"mem123\",\n" +
				"  \"type\": \"fact\",\n" +
				"  \"created_at\": \"2024-01-01T12:00:00Z\"\n" +
				"}\n" +
				"```\n\n" +
				"# Title\n" +
				"Content\n",
			wantErr: "missing required field: updated_at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DeserializeFromMarkdown([]byte(tt.markdown), types.MemoryScopeUser, "user123")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestDeserializeFromMarkdownInvalidTimestamp(t *testing.T) {
	markdown := " ```json\n" +
		"{\n" +
		"  \"id\": \"mem123\",\n" +
		"  \"type\": \"fact\",\n" +
		"  \"created_at\": \"invalid-timestamp\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
		"}\n" +
		"```\n\n" +
		"# Title\n" +
		"Content\n"

	_, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeUser, "user123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid created_at timestamp")
}

func TestRoundTripSerialization(t *testing.T) {
	original := &types.Memory{
		ID:      types.GenerateID(),
		Scope:   types.MemoryScopeGroup,
		OwnerID: "group456",
		Type:    types.MemoryTypePreference,
		Topic:   "settings",
		Title:   "Group Notification Settings",
		Content: "The group prefers email notifications over push notifications for non-urgent messages.",
		Metadata: types.MemoryMetadata{
			Labels:     map[string]string{"category": "notifications", "priority": "medium"},
			Tags:       []string{"settings", "notifications", "email"},
			Source:     "session",
			Importance: 7,
			Verified:   true,
			Confidence: 0.85,
			Version:    2,
		},
		CreatedAt: types.NewTimestampFromTime(time.Now().Add(-1 * time.Hour)),
		UpdatedAt: types.NewTimestampFromTime(time.Now()),
	}

	// Set optional fields
	accessedAt := types.NewTimestampFromTime(time.Now().Add(-30 * time.Minute))
	original.AccessedAt = &accessedAt
	expiresAt := types.NewTimestampFromTime(time.Now().Add(30 * 24 * time.Hour))
	original.ExpiresAt = &expiresAt
	sourceID := types.GenerateID()
	original.Metadata.SourceID = &sourceID

	// Serialize
	markdown, err := SerializeToMarkdown(original)
	require.NoError(t, err)

	// Deserialize
	deserialized, err := DeserializeFromMarkdown([]byte(markdown), original.Scope, original.OwnerID)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, original.ID, deserialized.ID)
	assert.Equal(t, original.Scope, deserialized.Scope)
	assert.Equal(t, original.OwnerID, deserialized.OwnerID)
	assert.Equal(t, original.Type, deserialized.Type)
	assert.Equal(t, original.Topic, deserialized.Topic)
	assert.Equal(t, original.Title, deserialized.Title)
	assert.Equal(t, original.Content, deserialized.Content)
	assert.Equal(t, original.CreatedAt.Time.Unix(), deserialized.CreatedAt.Time.Unix())
	assert.Equal(t, original.UpdatedAt.Time.Unix(), deserialized.UpdatedAt.Time.Unix())
	assert.NotNil(t, deserialized.AccessedAt)
	assert.Equal(t, original.AccessedAt.Time.Unix(), deserialized.AccessedAt.Time.Unix())
	assert.NotNil(t, deserialized.ExpiresAt)
	assert.Equal(t, original.ExpiresAt.Time.Unix(), deserialized.ExpiresAt.Time.Unix())
	assert.Equal(t, original.Metadata.Labels, deserialized.Metadata.Labels)
	assert.Equal(t, original.Metadata.Tags, deserialized.Metadata.Tags)
	assert.Equal(t, original.Metadata.Source, deserialized.Metadata.Source)
	assert.Equal(t, original.Metadata.Importance, deserialized.Metadata.Importance)
	assert.Equal(t, original.Metadata.Verified, deserialized.Metadata.Verified)
	assert.InDelta(t, original.Metadata.Confidence, deserialized.Metadata.Confidence, 0.001)
	assert.Equal(t, original.Metadata.Version, deserialized.Metadata.Version)
	assert.NotNil(t, deserialized.Metadata.SourceID)
	assert.Equal(t, original.Metadata.SourceID, deserialized.Metadata.SourceID)
}

func TestDeserializeFromMarkdownWithMultiLineContent(t *testing.T) {
	markdown := " ```json\n" +
		"{\n" +
		"  \"id\": \"mem123\",\n" +
		"  \"type\": \"context\",\n" +
		"  \"topic\": \"project\",\n" +
		"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
		"}\n" +
		"```\n\n" +
		"# Project Documentation\n\n" +
		"This is a multi-line content block.\n\n" +
		"It has multiple paragraphs.\n\n" +
		"And even lists:\n" +
		"- Item 1\n" +
		"- Item 2\n" +
		"- Item 3\n"

	mem, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeUser, "user123")
	require.NoError(t, err)
	require.NotNil(t, mem)

	assert.Contains(t, mem.Content, "This is a multi-line content block.")
	assert.Contains(t, mem.Content, "It has multiple paragraphs.")
	assert.Contains(t, mem.Content, "And even lists:")
	assert.Contains(t, mem.Content, "- Item 1")
	assert.Contains(t, mem.Content, "- Item 2")
	assert.Contains(t, mem.Content, "- Item 3")
}

func TestDeserializeFromMarkdownNoTitle(t *testing.T) {
	markdown := " ```json\n" +
		"{\n" +
		"  \"id\": \"mem123\",\n" +
		"  \"type\": \"fact\",\n" +
		"  \"topic\": \"untitled\",\n" +
		"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
		"}\n" +
		"```\n\n" +
		"Just content without a title heading\n"

	mem, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeUser, "user123")
	require.NoError(t, err)
	require.NotNil(t, mem)

	// Should use "Untitled" as default title
	assert.Equal(t, "Untitled", mem.Title)
	assert.Equal(t, "Just content without a title heading", mem.Content)
}

func TestDeserializeFromMarkdownTitleWithExtraSpaces(t *testing.T) {
	markdown := " ```json\n" +
		"{\n" +
		"  \"id\": \"mem123\",\n" +
		"  \"type\": \"fact\",\n" +
		"  \"topic\": \"test\",\n" +
		"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
		"}\n" +
		"```\n\n" +
		"#    Title With Extra Spaces\n\n" +
		"Content here\n"

	mem, err := DeserializeFromMarkdown([]byte(markdown), types.MemoryScopeUser, "user123")
	require.NoError(t, err)

	// Title should be trimmed
	assert.Equal(t, "Title With Extra Spaces", mem.Title)
}
