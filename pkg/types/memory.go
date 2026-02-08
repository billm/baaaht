package types

import "time"

// MemoryScope represents the scope of a memory entry
type MemoryScope string

const (
	MemoryScopeUser  MemoryScope = "user"
	MemoryScopeGroup MemoryScope = "group"
)

// MemoryType represents the type/category of memory
type MemoryType string

const (
	MemoryTypePreference MemoryType = "preference" // User preferences and settings
	MemoryTypeFact       MemoryType = "fact"       // Factual information about the user
	MemoryTypeContext    MemoryType = "context"    // Context about projects, tasks, etc.
	MemoryTypeDecision   MemoryType = "decision"   // Decisions made and their rationale
	MemoryTypeCustom     MemoryType = "custom"     // Custom memory types
)

// Memory represents a single memory entry stored as a markdown file
type Memory struct {
	ID        ID             `json:"id"`
	Scope     MemoryScope    `json:"scope"`
	OwnerID   string         `json:"owner_id"`    // User ID or Group ID
	Type      MemoryType     `json:"type"`
	Topic     string         `json:"topic"`       // Categorization topic
	Title     string         `json:"title"`       // Human-readable title
	Content   string         `json:"content"`     // Markdown content
	CreatedAt Timestamp      `json:"created_at"`
	UpdatedAt Timestamp      `json:"updated_at"`
	AccessedAt *Timestamp    `json:"accessed_at,omitempty"`
	ExpiresAt  *Timestamp    `json:"expires_at,omitempty"`
	Metadata   MemoryMetadata `json:"metadata"`
}

// MemoryMetadata contains additional information about a memory
type MemoryMetadata struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Source      string            `json:"source,omitempty"`      // e.g., "session", "manual"
	SourceID    *ID               `json:"source_id,omitempty"`   // Session ID if from session
	Importance  int               `json:"importance,omitempty"`  // 1-10, for sorting/filtering
	Confidence  float64           `json:"confidence,omitempty"`  // 0.0-1.0, extraction confidence
	Verified    bool              `json:"verified,omitempty"`    // Whether user has verified this memory
	Version     int               `json:"version,omitempty"`     // For tracking edits
}

// MemoryFilter defines filters for querying memories
type MemoryFilter struct {
	Scope     *MemoryScope  `json:"scope,omitempty"`
	OwnerID   *string       `json:"owner_id,omitempty"`
	Type      *MemoryType   `json:"type,omitempty"`
	Topic     *string       `json:"topic,omitempty"`
	Label     map[string]string `json:"label,omitempty"`
	Tags      []string      `json:"tags,omitempty"`
	StartTime *Timestamp    `json:"start_time,omitempty"`
	EndTime   *Timestamp    `json:"end_time,omitempty"`
	MinImportance *int      `json:"min_importance,omitempty"`
	Verified  *bool         `json:"verified,omitempty"`
}

// MemoryStats represents statistics about memories
type MemoryStats struct {
	Scope          MemoryScope `json:"scope"`
	OwnerID        string      `json:"owner_id"`
	TotalCount     int         `json:"total_count"`
	TypeCounts     map[MemoryType]int `json:"type_counts"`
	TopicCounts    map[string]int    `json:"topic_counts"`
	LastUpdated    Timestamp   `json:"last_updated"`
	Timestamp      Timestamp   `json:"timestamp"`
}

// MemoryConfig contains configuration for the memory system
type MemoryConfig struct {
	Enabled          bool              `json:"enabled"`
	StoragePath      string            `json:"storage_path"`       // Base path for memory files
	MaxMemoriesPerOwner int           `json:"max_memories_per_owner,omitempty"`
	AutoExtract      bool              `json:"auto_extract"`       // Auto-extract from archived sessions
	ExtractionConfig MemoryExtractionConfig `json:"extraction_config,omitempty"`
	Retention        MemoryRetention   `json:"retention,omitempty"`
}

// MemoryExtractionConfig contains configuration for memory extraction
type MemoryExtractionConfig struct {
	Enabled               bool          `json:"enabled"`
	MinConfidence         float64       `json:"min_confidence"`          // Minimum confidence to save
	MaxMemoriesPerSession int           `json:"max_memories_per_session"`
	Topics                []string      `json:"topics,omitempty"`         // Topics to extract
	ExcludedTopics        []string      `json:"excluded_topics,omitempty"`
}

// MemoryRetention contains retention policy for memories
type MemoryRetention struct {
	Enabled          bool          `json:"enabled"`
	MaxAge           time.Duration `json:"max_age,omitempty"`           // Maximum age before expiration
	MinAccessAge     time.Duration `json:"min_access_age,omitempty"`     // Time without access before expiration
	UnimportantMaxAge time.Duration `json:"unimportant_max_age,omitempty"` // Shorter retention for low importance
}

// MemorySearchResult represents a single search result
type MemorySearchResult struct {
	Memory    Memory `json:"memory"`
	Score     float64 `json:"score"`      // Relevance score
	Highlight string  `json:"highlight"`  // Snippet of matching content
}

// MemorySearchResults represents search results with pagination
type MemorySearchResults struct {
	Results []MemorySearchResult `json:"results"`
	Total   int                  `json:"total"`
	Offset  int                  `json:"offset"`
	Limit   int                  `json:"limit"`
}

// MemoryExport represents exported memory data
type MemoryExport struct {
	Memories     []Memory   `json:"memories"`
	ExportedAt   Timestamp  `json:"exported_at"`
	ExportedBy   string     `json:"exported_by"`
	Format       string     `json:"format"`       // e.g., "json", "markdown"
	Version      string     `json:"version"`      // Export format version
}
