package skills

import (
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSkill(t *testing.T) {
	markdown := "```json\n" +
		"{\n" +
		"  \"id\": \"skill123\",\n" +
		"  \"name\": \"code-review\",\n" +
		"  \"display_name\": \"Code Review Assistant\",\n" +
		"  \"description\": \"Helps with code reviews by analyzing code quality and suggesting improvements.\",\n" +
		"  \"version\": \"1.0.0\",\n" +
		"  \"author\": \"DevTools Team\",\n" +
		"  \"agent_types\": [\"assistant\", \"worker\"],\n" +
		"  \"capabilities\": [\n" +
		"    {\n" +
		"      \"type\": \"tool\",\n" +
		"      \"name\": \"analyze_code\",\n" +
		"      \"description\": \"Analyzes code for potential issues\"\n" +
		"    },\n" +
		"    {\n" +
		"      \"type\": \"prompt\",\n" +
		"      \"name\": \"review_template\",\n" +
		"      \"description\": \"Provides a template for code reviews\"\n" +
		"    }\n" +
		"  ],\n" +
		"  \"created_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\",\n" +
		"  \"category\": \"development\",\n" +
		"  \"tags\": [\"code-review\", \"development\", \"quality\"],\n" +
		"  \"keywords\": [\"review\", \"code\", \"quality\", \"analysis\"]\n" +
		"}\n" +
		"```\n\n" +
		"# Code Review Assistant\n\n" +
		"An advanced skill for assisting with code reviews.\n"

	skill, err := ParseFromMarkdown([]byte(markdown), types.SkillScopeUser, "user456", "/skills/code-review/SKILL.md")
	require.NoError(t, err)
	require.NotNil(t, skill)

	assert.Equal(t, "skill123", skill.ID.String())
	assert.Equal(t, types.SkillScopeUser, skill.Scope)
	assert.Equal(t, "user456", skill.OwnerID)
	assert.Equal(t, "code-review", skill.Name)
	assert.Equal(t, "Code Review Assistant", skill.DisplayName)
	assert.Equal(t, "Helps with code reviews by analyzing code quality and suggesting improvements.", skill.Description)
	assert.Equal(t, "1.0.0", skill.Version)
	assert.Equal(t, "DevTools Team", skill.Author)
	assert.Equal(t, types.SkillStateUnknown, skill.State)
	assert.Equal(t, types.SkillSourceFilesystem, skill.Source)
	assert.Equal(t, "/skills/code-review/SKILL.md", skill.FilePath)

	assert.Len(t, skill.AgentTypes, 2)
	assert.Contains(t, skill.AgentTypes, types.SkillAgentTypeAssistant)
	assert.Contains(t, skill.AgentTypes, types.SkillAgentTypeWorker)

	assert.Len(t, skill.Capabilities, 2)
	assert.Equal(t, types.SkillCapabilityTypeTool, skill.Capabilities[0].Type)
	assert.Equal(t, "analyze_code", skill.Capabilities[0].Name)
	assert.Equal(t, "prompt", string(skill.Capabilities[1].Type))

	assert.Equal(t, "development", skill.Metadata.Category)
	assert.Contains(t, skill.Metadata.Tags, "code-review")
	assert.Contains(t, skill.Metadata.Keywords, "review")
}

func TestParseSkillMinimal(t *testing.T) {
	markdown := "```json\n" +
		"{\n" +
		"  \"name\": \"test-skill\",\n" +
		"  \"display_name\": \"Test Skill\",\n" +
		"  \"description\": \"A simple test skill\"\n" +
		"}\n" +
		"```\n\n" +
		"# Test Skill\n\n" +
		"Simple content\n"

	skill, err := ParseFromMarkdown([]byte(markdown), types.SkillScopeGroup, "group123", "/skills/test/SKILL.md")
	require.NoError(t, err)
	require.NotNil(t, skill)

	assert.Equal(t, "test-skill", skill.Name)
	assert.Equal(t, "Test Skill", skill.DisplayName)
	assert.Equal(t, "A simple test skill", skill.Description)
	assert.Equal(t, "1.0.0", skill.Version) // Default version
	assert.Equal(t, types.SkillStateUnknown, skill.State)
	assert.Equal(t, types.SkillSourceFilesystem, skill.Source)
	assert.Contains(t, skill.AgentTypes, types.SkillAgentTypeAll) // Default agent type
	assert.Equal(t, "group123", skill.OwnerID)
	assert.Equal(t, types.SkillScopeGroup, skill.Scope)
}

func TestParseSkillEmpty(t *testing.T) {
	_, err := ParseFromMarkdown([]byte(""), types.SkillScopeUser, "user123", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill data cannot be empty")
}

func TestParseSkillNil(t *testing.T) {
	_, err := ParseFromMarkdown(nil, types.SkillScopeUser, "user123", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill data cannot be empty")
}

func TestParseSkillInvalidFormat(t *testing.T) {
	// Missing opening ```json
	markdown := "# Just a heading\nSome content\n"

	_, err := ParseFromMarkdown([]byte(markdown), types.SkillScopeUser, "user123", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing metadata")
}

func TestParseSkillMissingClosing(t *testing.T) {
	// Missing closing ```
	markdown := "```json\n" +
		"{\n" +
		"  \"name\": \"test\",\n" +
		"  \"display_name\": \"Test\",\n" +
		"  \"description\": \"Test description\"\n" +
		"}\n" +
		"\n" +
		"# Title\n" +
		"Content\n"

	_, err := ParseFromMarkdown([]byte(markdown), types.SkillScopeUser, "user123", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing closing metadata")
}

func TestParseSkillInvalidJSON(t *testing.T) {
	markdown := "```json\n" +
		"{ invalid json }\n" +
		"```\n\n" +
		"# Title\n" +
		"Content\n"

	_, err := ParseFromMarkdown([]byte(markdown), types.SkillScopeUser, "user123", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse skill metadata")
}

func TestParseSkillMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		wantErr  string
	}{
		{
			name: "missing name",
			markdown: "```json\n" +
				"{\n" +
				"  \"display_name\": \"Test\",\n" +
				"  \"description\": \"Test description\"\n" +
				"}\n" +
				"```\n\n" +
				"# Title\n" +
				"Content\n",
			wantErr: "missing required field: name",
		},
		{
			name: "missing display_name",
			markdown: "```json\n" +
				"{\n" +
				"  \"name\": \"test\",\n" +
				"  \"description\": \"Test description\"\n" +
				"}\n" +
				"```\n\n" +
				"# Title\n" +
				"Content\n",
			wantErr: "missing required field: display_name",
		},
		{
			name: "missing description",
			markdown: "```json\n" +
				"{\n" +
				"  \"name\": \"test\",\n" +
				"  \"display_name\": \"Test\"\n" +
				"}\n" +
				"```\n\n" +
				"# Title\n" +
				"Content\n",
			wantErr: "missing required field: description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFromMarkdown([]byte(tt.markdown), types.SkillScopeUser, "user123", "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseSkillInvalidTimestamp(t *testing.T) {
	markdown := "```json\n" +
		"{\n" +
		"  \"name\": \"test\",\n" +
		"  \"display_name\": \"Test\",\n" +
		"  \"description\": \"Test description\",\n" +
		"  \"created_at\": \"invalid-timestamp\",\n" +
		"  \"updated_at\": \"2024-01-01T12:00:00Z\"\n" +
		"}\n" +
		"```\n\n" +
		"# Title\n" +
		"Content\n"

	_, err := ParseFromMarkdown([]byte(markdown), types.SkillScopeUser, "user123", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid created_at timestamp")
}

func TestSerializeToSkill(t *testing.T) {
	skill := &types.Skill{
		ID:          types.NewID("skill456"),
		Scope:       types.SkillScopeUser,
		OwnerID:     "user789",
		State:       types.SkillStateLoaded,
		Source:      types.SkillSourceFilesystem,
		Name:        "documentation-helper",
		DisplayName: "Documentation Helper",
		Description: "Helps generate and format documentation.",
		Version:     "2.0.0",
		Author:      "Docs Team",
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAssistant},
		Capabilities: []types.SkillCapability{
			{
				Type:        types.SkillCapabilityTypeTool,
				Name:        "generate_docs",
				Description: "Generates documentation from code",
				Config:      map[string]any{"format": "markdown"},
			},
		},
		FilePath: "/skills/docs/SKILL.md",
		Content:  "Additional documentation content here.",
		Metadata: types.SkillMetadata{
			Labels:       map[string]string{"category": "documentation"},
			Tags:         []string{"docs", "writing"},
			Category:     "productivity",
			Keywords:     []string{"documentation", "writing"},
			RequiredAPIs: []string{"openai"},
			Dependencies: []string{"markdown-formatter"},
			Verified:     true,
		},
		CreatedAt: types.NewTimestampFromTime(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)),
		UpdatedAt: types.NewTimestampFromTime(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)),
	}

	result, err := SerializeToMarkdown(skill)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	// Verify JSON frontmatter exists
	assert.True(t, strings.HasPrefix(result, "```json\n"))
	assert.Contains(t, result, "\n```\n\n")

	// Verify title heading
	assert.Contains(t, result, "# Documentation Helper\n\n")

	// Verify description
	assert.Contains(t, result, "Helps generate and format documentation.")

	// Verify metadata in JSON
	assert.Contains(t, result, `"id": "skill456"`)
	assert.Contains(t, result, `"scope": "user"`)
	assert.Contains(t, result, `"name": "documentation-helper"`)
	assert.Contains(t, result, `"display_name": "Documentation Helper"`)
	assert.Contains(t, result, `"version": "2.0.0"`)
	assert.Contains(t, result, `"author": "Docs Team"`)
	assert.Contains(t, result, `"state": "loaded"`)
	assert.Contains(t, result, `"source": "filesystem"`)
	assert.Contains(t, result, `"category": "productivity"`)
	assert.Contains(t, result, `"verified": true`)
}

func TestSerializeToSkillNil(t *testing.T) {
	_, err := SerializeToMarkdown(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill cannot be nil")
}

func TestSerializeToSkillMinimal(t *testing.T) {
	skill := &types.Skill{
		ID:          types.NewID("skill789"),
		Scope:       types.SkillScopeUser,
		OwnerID:     "user123",
		Name:        "minimal-skill",
		DisplayName: "Minimal Skill",
		Description: "A minimal skill",
		Version:     "1.0.0",
		CreatedAt:   types.NewTimestamp(),
		UpdatedAt:   types.NewTimestamp(),
	}

	result, err := SerializeToMarkdown(skill)
	require.NoError(t, err)

	// Verify basic structure
	assert.True(t, strings.HasPrefix(result, "```json\n"))
	assert.Contains(t, result, "# Minimal Skill\n\n")
	assert.Contains(t, result, "A minimal skill")
	assert.Contains(t, result, `"name": "minimal-skill"`)
}

func TestRoundTripSerialization(t *testing.T) {
	original := &types.Skill{
		ID:          types.GenerateID(),
		Scope:       types.SkillScopeGroup,
		OwnerID:     "group456",
		State:       types.SkillStateLoaded,
		Source:      types.SkillSourceGitHub,
		Name:        "team-tools",
		DisplayName: "Team Tools",
		Description: "A collection of tools for team collaboration.",
		Version:     "1.5.0",
		Author:      "Team Ops",
		AgentTypes:  []types.SkillAgentType{types.SkillAgentTypeAll},
		Capabilities: []types.SkillCapability{
			{
				Type:        types.SkillCapabilityTypeTool,
				Name:        "schedule_meeting",
				Description: "Schedules team meetings",
			},
			{
				Type:        types.SkillCapabilityTypePrompt,
				Name:        "agenda_template",
				Description: "Provides meeting agenda template",
			},
		},
		FilePath:   "/skills/team-tools/SKILL.md",
		Repository: "github.com/example/skills",
		Content:    "Detailed skill documentation goes here.",
		Metadata: types.SkillMetadata{
			Labels:       map[string]string{"team": "engineering"},
			Tags:         []string{"collaboration", "meetings", "productivity"},
			Category:     "teamwork",
			Keywords:     []string{"meetings", "scheduling", "agenda"},
			Dependencies: []string{"calendar-api"},
			Verified:     true,
			LoadCount:    5,
		},
		CreatedAt: types.NewTimestampFromTime(time.Now().Add(-24 * time.Hour)),
		UpdatedAt: types.NewTimestampFromTime(time.Now()),
	}

	activatedAt := types.NewTimestampFromTime(time.Now().Add(-12 * time.Hour))
	original.ActivatedAt = &activatedAt

	// Serialize
	markdown, err := SerializeToMarkdown(original)
	require.NoError(t, err)

	// Deserialize
	parsed, err := ParseFromMarkdown([]byte(markdown), original.Scope, original.OwnerID, original.FilePath)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, original.ID, parsed.ID)
	assert.Equal(t, original.Scope, parsed.Scope)
	assert.Equal(t, original.OwnerID, parsed.OwnerID)
	assert.Equal(t, original.State, parsed.State)
	assert.Equal(t, original.Source, parsed.Source)
	assert.Equal(t, original.Name, parsed.Name)
	assert.Equal(t, original.DisplayName, parsed.DisplayName)
	assert.Equal(t, original.Description, parsed.Description)
	assert.Equal(t, original.Version, parsed.Version)
	assert.Equal(t, original.Author, parsed.Author)
	assert.Len(t, parsed.AgentTypes, len(original.AgentTypes))
	assert.Equal(t, original.Repository, parsed.Repository)
	assert.NotNil(t, parsed.ActivatedAt)
	assert.Equal(t, original.ActivatedAt.Time.Unix(), parsed.ActivatedAt.Time.Unix())
	assert.Equal(t, original.Metadata.Category, parsed.Metadata.Category)
	assert.Equal(t, original.Metadata.Verified, parsed.Metadata.Verified)
}

func TestParseSkillNoTitle(t *testing.T) {
	markdown := "```json\n" +
		"{\n" +
		"  \"name\": \"no-title\",\n" +
		"  \"display_name\": \"No Title Skill\",\n" +
		"  \"description\": \"A skill without a title heading\"\n" +
		"}\n" +
		"```\n\n" +
		"Just content without a title heading\n"

	skill, err := ParseFromMarkdown([]byte(markdown), types.SkillScopeUser, "user123", "")
	require.NoError(t, err)
	require.NotNil(t, skill)

	// Content should still be preserved
	assert.Equal(t, "Just content without a title heading", skill.Content)
}

func TestParseSkillWithMultiLineContent(t *testing.T) {
	markdown := "```json\n" +
		"{\n" +
		"  \"name\": \"multi-line\",\n" +
		"  \"display_name\": \"Multi Line Skill\",\n" +
		"  \"description\": \"A skill with multi-line content\"\n" +
		"}\n" +
		"```\n\n" +
		"# Multi Line Skill\n\n" +
		"This is a multi-line content block.\n\n" +
		"It has multiple paragraphs.\n\n" +
		"Features include:\n" +
		"- Feature 1\n" +
		"- Feature 2\n" +
		"- Feature 3\n"

	skill, err := ParseFromMarkdown([]byte(markdown), types.SkillScopeUser, "user123", "")
	require.NoError(t, err)
	require.NotNil(t, skill)

	assert.Contains(t, skill.Content, "This is a multi-line content block.")
	assert.Contains(t, skill.Content, "It has multiple paragraphs.")
	assert.Contains(t, skill.Content, "Features include:")
	assert.Contains(t, skill.Content, "- Feature 1")
	assert.Contains(t, skill.Content, "- Feature 2")
	assert.Contains(t, skill.Content, "- Feature 3")
}
