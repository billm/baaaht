package skills

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// SerializeToMarkdown converts a skill to markdown format with JSON frontmatter
// The frontmatter contains all metadata in JSON format within a code block
func SerializeToMarkdown(skill *types.Skill) (string, error) {
	if skill == nil {
		return "", types.NewError(types.ErrCodeInvalidArgument, "skill cannot be nil")
	}

	metadata := map[string]interface{}{
		"id":           skill.ID.String(),
		"scope":        string(skill.Scope),
		"owner_id":     skill.OwnerID,
		"name":         skill.Name,
		"display_name": skill.DisplayName,
		"description":  skill.Description,
		"version":      skill.Version,
		"author":       skill.Author,
		"created_at":   skill.CreatedAt.Time.Format(time.RFC3339),
		"updated_at":   skill.UpdatedAt.Time.Format(time.RFC3339),
	}

	if skill.State != "" {
		metadata["state"] = string(skill.State)
	}

	if skill.Source != "" {
		metadata["source"] = string(skill.Source)
	}

	if len(skill.AgentTypes) > 0 {
		agentTypes := make([]string, len(skill.AgentTypes))
		for i, at := range skill.AgentTypes {
			agentTypes[i] = string(at)
		}
		metadata["agent_types"] = agentTypes
	}

	if len(skill.Capabilities) > 0 {
		metadata["capabilities"] = skill.Capabilities
	}

	if skill.FilePath != "" {
		metadata["file_path"] = skill.FilePath
	}

	if skill.Repository != "" {
		metadata["repository"] = skill.Repository
	}

	if skill.ActivatedAt != nil {
		metadata["activated_at"] = skill.ActivatedAt.Time.Format(time.RFC3339)
	}

	// Add optional metadata fields
	if len(skill.Metadata.Labels) > 0 {
		metadata["labels"] = skill.Metadata.Labels
	}

	if len(skill.Metadata.Tags) > 0 {
		metadata["tags"] = skill.Metadata.Tags
	}

	if skill.Metadata.Category != "" {
		metadata["category"] = skill.Metadata.Category
	}

	if len(skill.Metadata.Keywords) > 0 {
		metadata["keywords"] = skill.Metadata.Keywords
	}

	if len(skill.Metadata.RequiredAPIs) > 0 {
		metadata["required_apis"] = skill.Metadata.RequiredAPIs
	}

	if len(skill.Metadata.Dependencies) > 0 {
		metadata["dependencies"] = skill.Metadata.Dependencies
	}

	if skill.Metadata.Verified {
		metadata["verified"] = true
	}

	// Encode metadata as JSON (for frontmatter)
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return "", types.WrapError(types.ErrCodeInternal, "failed to marshal skill metadata", err)
	}

	// Build markdown content
	var sb strings.Builder
	sb.WriteString("```json\n")
	sb.WriteString(string(metadataJSON))
	sb.WriteString("\n```\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", skill.DisplayName))
	sb.WriteString(skill.Description)
	sb.WriteString("\n\n")

	// If content has additional details, add them
	if skill.Content != "" && skill.Content != skill.Description {
		sb.WriteString(skill.Content)
	}

	return sb.String(), nil
}

// ParseFromMarkdown parses a skill from markdown format with JSON frontmatter
// Expects the format: ```json\n{metadata}\n```\n\n# Title\n\ncontent
func ParseFromMarkdown(data []byte, scope types.SkillScope, ownerID string, filePath string) (*types.Skill, error) {
	if len(data) == 0 {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "skill data cannot be empty")
	}

	content := string(data)

	// Trim leading whitespace for robustness
	content = strings.TrimLeft(content, " \t\r\n")

	// Extract JSON frontmatter between ```json and ```
	var metadataJSON string
	if strings.HasPrefix(content, "```json\n") {
		endIndex := strings.Index(content, "\n```\n")
		if endIndex == -1 {
			return nil, types.NewError(types.ErrCodeInternal, "invalid skill file format: missing closing metadata")
		}
		metadataJSON = content[8:endIndex]
		content = content[endIndex+5:]
	} else {
		return nil, types.NewError(types.ErrCodeInternal, "invalid skill file format: missing metadata")
	}

	// Parse metadata
	var metadata struct {
		ID          string                  `json:"id"`
		Name        string                  `json:"name"`
		DisplayName string                  `json:"display_name"`
		Description string                  `json:"description"`
		Version     string                  `json:"version,omitempty"`
		Author      string                  `json:"author,omitempty"`
		State       string                  `json:"state,omitempty"`
		Source      string                  `json:"source,omitempty"`
		AgentTypes  []string                `json:"agent_types,omitempty"`
		Capabilities []types.SkillCapability `json:"capabilities,omitempty"`
		CreatedAt   string                  `json:"created_at,omitempty"`
		UpdatedAt   string                  `json:"updated_at,omitempty"`
		ActivatedAt string                  `json:"activated_at,omitempty"`
		Repository  string                  `json:"repository,omitempty"`
		Labels      map[string]string       `json:"labels,omitempty"`
		Tags        []string                `json:"tags,omitempty"`
		Category    string                  `json:"category,omitempty"`
		Keywords    []string                `json:"keywords,omitempty"`
		RequiredAPIs []string               `json:"required_apis,omitempty"`
		Dependencies []string               `json:"dependencies,omitempty"`
		Verified    bool                    `json:"verified,omitempty"`
	}

	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to parse skill metadata", err)
	}

	// Validate required fields
	if metadata.Name == "" {
		return nil, types.NewError(types.ErrCodeInternal, "missing required field: name")
	}
	if metadata.DisplayName == "" {
		return nil, types.NewError(types.ErrCodeInternal, "missing required field: display_name")
	}
	if metadata.Description == "" {
		return nil, types.NewError(types.ErrCodeInternal, "missing required field: description")
	}

	// Set defaults
	if metadata.Version == "" {
		metadata.Version = "1.0.0"
	}
	if metadata.State == "" {
		metadata.State = string(types.SkillStateUnknown)
	}
	if metadata.Source == "" {
		metadata.Source = string(types.SkillSourceFilesystem)
	}

	// Parse timestamps
	now := time.Now()
	var createdAt, updatedAt time.Time
	var err error

	if metadata.CreatedAt != "" {
		createdAt, err = time.Parse(time.RFC3339, metadata.CreatedAt)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "invalid created_at timestamp", err)
		}
	} else {
		createdAt = now
	}

	if metadata.UpdatedAt != "" {
		updatedAt, err = time.Parse(time.RFC3339, metadata.UpdatedAt)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "invalid updated_at timestamp", err)
		}
	} else {
		updatedAt = now
	}

	var activatedAt *types.Timestamp
	if metadata.ActivatedAt != "" {
		t, err := time.Parse(time.RFC3339, metadata.ActivatedAt)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "invalid activated_at timestamp", err)
		}
		ts := types.NewTimestampFromTime(t)
		activatedAt = &ts
	}

	// Parse agent types
	var agentTypes []types.SkillAgentType
	if len(metadata.AgentTypes) > 0 {
		agentTypes = make([]types.SkillAgentType, len(metadata.AgentTypes))
		for i, at := range metadata.AgentTypes {
			agentTypes[i] = types.SkillAgentType(at)
		}
	} else {
		// Default to all agents if not specified
		agentTypes = []types.SkillAgentType{types.SkillAgentTypeAll}
	}

	// Parse skill ID
	var skillID types.ID
	if metadata.ID != "" {
		skillID = types.NewID(metadata.ID)
	} else {
		// Generate ID from name and owner
		skillID = types.NewID(fmt.Sprintf("%s-%s-%s", scope, ownerID, metadata.Name))
	}

	// Trim content - remove title heading if present
	lines := strings.Split(content, "\n")
	contentWithoutTitle := content
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			// Remove title line and following empty line from content
			if i+1 < len(lines) && lines[i+1] == "" {
				contentWithoutTitle = strings.Join(lines[i+2:], "\n")
			} else {
				contentWithoutTitle = strings.Join(lines[i+1:], "\n")
			}
			break
		}
	}

	// Trim trailing whitespace from content
	contentWithoutTitle = strings.TrimRight(contentWithoutTitle, " \t\r\n")
	// Also trim leading whitespace from content (for cases without title heading)
	contentWithoutTitle = strings.TrimLeft(contentWithoutTitle, " \t\r\n")

	// Build skill object
	skill := &types.Skill{
		ID:          skillID,
		Scope:       scope,
		OwnerID:     ownerID,
		State:       types.SkillState(metadata.State),
		Source:      types.SkillSource(metadata.Source),
		Name:        metadata.Name,
		DisplayName: metadata.DisplayName,
		Description: metadata.Description,
		Version:     metadata.Version,
		Author:      metadata.Author,
		AgentTypes:  agentTypes,
		Capabilities: metadata.Capabilities,
		FilePath:    filePath,
		Repository:  metadata.Repository,
		Content:     contentWithoutTitle,
		CreatedAt:   types.NewTimestampFromTime(createdAt),
		UpdatedAt:   types.NewTimestampFromTime(updatedAt),
		ActivatedAt: activatedAt,
		Metadata: types.SkillMetadata{
			Labels:       metadata.Labels,
			Tags:         metadata.Tags,
			Category:     metadata.Category,
			Keywords:     metadata.Keywords,
			RequiredAPIs: metadata.RequiredAPIs,
			Dependencies: metadata.Dependencies,
			Verified:     metadata.Verified,
		},
	}

	return skill, nil
}
