package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// SerializeToMarkdown converts a memory to markdown format with JSON frontmatter
// The frontmatter contains all metadata in JSON format within a code block
func SerializeToMarkdown(mem *types.Memory) (string, error) {
	if mem == nil {
		return "", types.NewError(types.ErrCodeInvalidArgument, "memory cannot be nil")
	}

	metadata := map[string]interface{}{
		"id":         mem.ID.String(),
		"scope":      string(mem.Scope),
		"owner_id":   mem.OwnerID,
		"type":       string(mem.Type),
		"topic":      mem.Topic,
		"created_at": mem.CreatedAt.Time.Format(time.RFC3339),
		"updated_at": mem.UpdatedAt.Time.Format(time.RFC3339),
	}

	if mem.AccessedAt != nil {
		metadata["accessed_at"] = mem.AccessedAt.Time.Format(time.RFC3339)
	}

	if mem.ExpiresAt != nil {
		metadata["expires_at"] = mem.ExpiresAt.Time.Format(time.RFC3339)
	}

	if len(mem.Metadata.Labels) > 0 {
		metadata["labels"] = mem.Metadata.Labels
	}

	if len(mem.Metadata.Tags) > 0 {
		metadata["tags"] = mem.Metadata.Tags
	}

	if mem.Metadata.Source != "" {
		metadata["source"] = mem.Metadata.Source
	}

	if mem.Metadata.SourceID != nil {
		metadata["source_id"] = mem.Metadata.SourceID.String()
	}

	if mem.Metadata.Importance > 0 {
		metadata["importance"] = mem.Metadata.Importance
	}

	if mem.Metadata.Confidence > 0 {
		metadata["confidence"] = mem.Metadata.Confidence
	}

	if mem.Metadata.Verified {
		metadata["verified"] = true
	}

	if mem.Metadata.Version > 0 {
		metadata["version"] = mem.Metadata.Version
	}

	// Encode metadata as JSON (for frontmatter)
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return "", types.WrapError(types.ErrCodeInternal, "failed to marshal memory metadata", err)
	}

	// Build markdown content
	var sb strings.Builder
	sb.WriteString("```json\n")
	sb.WriteString(string(metadataJSON))
	sb.WriteString("\n```\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", mem.Title))
	sb.WriteString(mem.Content)

	return sb.String(), nil
}

// DeserializeFromMarkdown parses a memory from markdown format with JSON frontmatter
// Expects the format: ```json\n{metadata}\n```\n\n# Title\n\ncontent
func DeserializeFromMarkdown(data []byte, scope types.MemoryScope, ownerID string) (*types.Memory, error) {
	if len(data) == 0 {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "memory data cannot be empty")
	}

	content := string(data)

	// Trim leading whitespace for robustness
	content = strings.TrimLeft(content, " \t\r\n")

	// Extract JSON frontmatter between ```json and ```
	var metadataJSON string
	if strings.HasPrefix(content, "```json\n") {
		endIndex := strings.Index(content, "\n```\n")
		if endIndex == -1 {
			return nil, types.NewError(types.ErrCodeInternal, "invalid memory file format: missing closing metadata")
		}
		metadataJSON = content[8:endIndex]
		content = content[endIndex+5:]
	} else {
		return nil, types.NewError(types.ErrCodeInternal, "invalid memory file format: missing metadata")
	}

	// Parse metadata
	var metadata struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Topic      string `json:"topic"`
		CreatedAt  string `json:"created_at"`
		UpdatedAt  string `json:"updated_at"`
		AccessedAt string `json:"accessed_at,omitempty"`
		ExpiresAt  string `json:"expires_at,omitempty"`
		Labels     map[string]string `json:"labels,omitempty"`
		Tags       []string          `json:"tags,omitempty"`
		Source     string            `json:"source,omitempty"`
		SourceID   string            `json:"source_id,omitempty"`
		Importance int               `json:"importance,omitempty"`
		Confidence float64           `json:"confidence,omitempty"`
		Verified   bool              `json:"verified,omitempty"`
		Version    int               `json:"version,omitempty"`
	}

	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to parse memory metadata", err)
	}

	// Validate required fields
	if metadata.ID == "" {
		return nil, types.NewError(types.ErrCodeInternal, "missing required field: id")
	}
	if metadata.Type == "" {
		return nil, types.NewError(types.ErrCodeInternal, "missing required field: type")
	}
	if metadata.CreatedAt == "" {
		return nil, types.NewError(types.ErrCodeInternal, "missing required field: created_at")
	}
	if metadata.UpdatedAt == "" {
		return nil, types.NewError(types.ErrCodeInternal, "missing required field: updated_at")
	}

	// Parse timestamps
	createdAt, err := time.Parse(time.RFC3339, metadata.CreatedAt)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "invalid created_at timestamp", err)
	}

	updatedAt, err := time.Parse(time.RFC3339, metadata.UpdatedAt)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "invalid updated_at timestamp", err)
	}

	var accessedAt *types.Timestamp
	if metadata.AccessedAt != "" {
		t, err := time.Parse(time.RFC3339, metadata.AccessedAt)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "invalid accessed_at timestamp", err)
		}
		ts := types.NewTimestampFromTime(t)
		accessedAt = &ts
	}

	var expiresAt *types.Timestamp
	if metadata.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, metadata.ExpiresAt)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "invalid expires_at timestamp", err)
		}
		ts := types.NewTimestampFromTime(t)
		expiresAt = &ts
	}

	// Extract title from content (first # heading) and strip it from content
	title := "Untitled"
	contentWithoutTitle := content
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimPrefix(line, "# ")
			title = strings.TrimSpace(title)
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

	// Build memory object
	mem := &types.Memory{
		ID:      types.NewID(metadata.ID),
		Scope:   scope,
		OwnerID: ownerID,
		Type:    types.MemoryType(metadata.Type),
		Topic:   metadata.Topic,
		Title:   title,
		Content: contentWithoutTitle,
		Metadata: types.MemoryMetadata{
			Labels:     metadata.Labels,
			Tags:       metadata.Tags,
			Source:     metadata.Source,
			Importance: metadata.Importance,
			Confidence: metadata.Confidence,
			Verified:   metadata.Verified,
			Version:    metadata.Version,
		},
		CreatedAt: types.NewTimestampFromTime(createdAt),
		UpdatedAt: types.NewTimestampFromTime(updatedAt),
		AccessedAt: accessedAt,
		ExpiresAt:  expiresAt,
	}

	if metadata.SourceID != "" {
		sourceID := types.NewID(metadata.SourceID)
		mem.Metadata.SourceID = &sourceID
	}

	return mem, nil
}
