package types

import "time"

// SkillScope represents the scope of a skill
type SkillScope string

const (
	SkillScopeUser  SkillScope = "user"
	SkillScopeGroup SkillScope = "group"
)

// SkillState represents the state of a skill
type SkillState string

const (
	SkillStateUnknown  SkillState = "unknown"
	SkillStateLoaded   SkillState = "loaded"
	SkillStateUnloaded SkillState = "unloaded"
	SkillStateError    SkillState = "error"
	SkillStateDisabled SkillState = "disabled"
)

// SkillAgentType represents the agent types that can use a skill
type SkillAgentType string

const (
	SkillAgentTypeAssistant SkillAgentType = "assistant"
	SkillAgentTypeWorker    SkillAgentType = "worker"
	SkillAgentTypeAll       SkillAgentType = "all"
)

// SkillSource represents the source of a skill
type SkillSource string

const (
	SkillSourceFilesystem SkillSource = "filesystem"
	SkillSourceGitHub     SkillSource = "github"
	SkillSourceBuiltin    SkillSource = "builtin"
)

// SkillCapabilityType represents the type of capability a skill provides
type SkillCapabilityType string

const (
	SkillCapabilityTypeTool     SkillCapabilityType = "tool"
	SkillCapabilityTypePrompt   SkillCapabilityType = "prompt"
	SkillCapabilityTypeBehavior SkillCapabilityType = "behavior"
)

// SkillCapability represents a capability provided by a skill
type SkillCapability struct {
	Type        SkillCapabilityType `json:"type"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Config      map[string]any      `json:"config,omitempty"`
}

// Skill represents a single skill loaded from a SKILL.md file
type Skill struct {
	ID          ID             `json:"id"`
	Scope       SkillScope     `json:"scope"`
	OwnerID     string         `json:"owner_id"`    // User ID or Group ID
	State       SkillState     `json:"state"`
	Source      SkillSource    `json:"source"`
	Name        string         `json:"name"`        // Skill name (usually filename or directory)
	DisplayName string         `json:"display_name"` // Human-readable name
	Description string         `json:"description"`  // Short description
	Version     string         `json:"version"`      // Skill version
	Author      string         `json:"author"`       // Skill author
	AgentTypes  []SkillAgentType `json:"agent_types"` // Agents that can use this skill
	Capabilities []SkillCapability `json:"capabilities"` // Capabilities provided by this skill
	FilePath    string         `json:"file_path"`    // Path to SKILL.md file
	Repository  string         `json:"repository,omitempty"` // GitHub repo if from GitHub
	Content     string         `json:"content"`      // Raw markdown content
	CreatedAt   Timestamp      `json:"created_at"`
	UpdatedAt   Timestamp      `json:"updated_at"`
	ActivatedAt *Timestamp     `json:"activated_at,omitempty"`
	Metadata    SkillMetadata  `json:"metadata"`
}

// SkillMetadata contains additional information about a skill
type SkillMetadata struct {
	Labels        map[string]string `json:"labels,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	Category      string            `json:"category,omitempty"`      // e.g., "development", "productivity"
	Keywords      []string          `json:"keywords,omitempty"`      // Search keywords
	RequiredAPIs  []string          `json:"required_apis,omitempty"` // APIs required by this skill
	Dependencies  []string          `json:"dependencies,omitempty"`  // Other skills this depends on
	LoadCount     int               `json:"load_count,omitempty"`    // Number of times loaded
	LastUsedAt    *Timestamp        `json:"last_used_at,omitempty"`  // When this skill was last used
	ErrorCount    int               `json:"error_count,omitempty"`   // Number of loading errors
	LastError     string            `json:"last_error,omitempty"`    // Last error message
	Verified      bool              `json:"verified,omitempty"`      // Whether user has verified this skill
}

// SkillFilter defines filters for querying skills
type SkillFilter struct {
	Scope      *SkillScope      `json:"scope,omitempty"`
	OwnerID    *string          `json:"owner_id,omitempty"`
	State      *SkillState      `json:"state,omitempty"`
	Source     *SkillSource     `json:"source,omitempty"`
	AgentType  *SkillAgentType  `json:"agent_type,omitempty"`
	Category   *string          `json:"category,omitempty"`
	Label      map[string]string `json:"label,omitempty"`
	Tags       []string         `json:"tags,omitempty"`
	MinLoadCount *int           `json:"min_load_count,omitempty"`
	Verified   *bool            `json:"verified,omitempty"`
	StartTime  *Timestamp       `json:"start_time,omitempty"`
	EndTime    *Timestamp       `json:"end_time,omitempty"`
}

// SkillStats represents statistics about skills
type SkillStats struct {
	Scope           SkillScope           `json:"scope"`
	OwnerID         string               `json:"owner_id"`
	TotalCount      int                  `json:"total_count"`
	StateCounts     map[SkillState]int   `json:"state_counts"`
	SourceCounts    map[SkillSource]int  `json:"source_counts"`
	CategoryCounts  map[string]int       `json:"category_counts"`
	LastUpdated     Timestamp            `json:"last_updated"`
	Timestamp       Timestamp            `json:"timestamp"`
}

// SkillConfig contains configuration for the skills system
type SkillConfig struct {
	Enabled            bool                `json:"enabled"`
	StoragePath        string              `json:"storage_path"`         // Base path for skill files
	MaxSkillsPerOwner  int                 `json:"max_skills_per_owner,omitempty"`
	AutoLoad           bool                `json:"auto_load"`            // Auto-load skills from directories
	LoadConfig         SkillLoadConfig     `json:"load_config,omitempty"`
	GitHubConfig       SkillGitHubConfig   `json:"github_config,omitempty"`
	Retention          SkillRetention      `json:"retention,omitempty"`
}

// SkillLoadConfig contains configuration for skill loading
type SkillLoadConfig struct {
	Enabled              bool     `json:"enabled"`
	SkillPaths           []string `json:"skill_paths,omitempty"`           // Paths to scan for skills
	Recursive            bool     `json:"recursive"`                       // Scan directories recursively
	WatchChanges         bool     `json:"watch_changes"`                   // Watch for file changes
	MaxLoadErrors        int      `json:"max_load_errors,omitempty"`       // Max errors before stopping
	ExcludedPatterns     []string `json:"excluded_patterns,omitempty"`     // File patterns to exclude
	RequiredCapabilities []string `json:"required_capabilities,omitempty"` // Required capabilities
}

// SkillGitHubConfig contains configuration for GitHub skill installation
type SkillGitHubConfig struct {
	Enabled           bool     `json:"enabled"`
	APIEndpoint       string   `json:"api_endpoint,omitempty"`        // GitHub API endpoint
	MaxRepoSkills     int      `json:"max_repo_skills,omitempty"`     // Max skills per repo
	AllowedOrgs       []string `json:"allowed_orgs,omitempty"`        // Allowed GitHub organizations
	AllowedRepos      []string `json:"allowed_repos,omitempty"`       // Allowed repositories (format: owner/repo)
	Token             string   `json:"token,omitempty"`               // GitHub auth token
	AutoUpdate        bool     `json:"auto_update"`                   // Auto-update skills from GitHub
	UpdateInterval    time.Duration `json:"update_interval,omitempty"` // Update check interval
}

// SkillRetention contains retention policy for skills
type SkillRetention struct {
	Enabled              bool          `json:"enabled"`
	MaxAge               time.Duration `json:"max_age,omitempty"`               // Maximum age before cleanup
	UnusedMaxAge         time.Duration `json:"unused_max_age,omitempty"`         // Time unused before cleanup
	ErrorMaxAge          time.Duration `json:"error_max_age,omitempty"`          // Time in error state before cleanup
	MinLoadCount         int           `json:"min_load_count,omitempty"`         // Minimum loads to keep
	PreserveVerified     bool          `json:"preserve_verified,omitempty"`      // Don't delete verified skills
}

// SkillSearchResult represents a single search result
type SkillSearchResult struct {
	Skill    Skill   `json:"skill"`
	Score    float64 `json:"score"`     // Relevance score
	Matched  string  `json:"matched"`   // Matched field (name, description, tags)
}

// SkillSearchResults represents search results with pagination
type SkillSearchResults struct {
	Results []SkillSearchResult `json:"results"`
	Total   int                 `json:"total"`
	Offset  int                 `json:"offset"`
	Limit   int                 `json:"limit"`
}

// SkillExport represents exported skill data
type SkillExport struct {
	Skills     []Skill    `json:"skills"`
	ExportedAt Timestamp  `json:"exported_at"`
	ExportedBy string     `json:"exported_by"`
	Format     string     `json:"format"`    // e.g., "json", "markdown"
	Version    string     `json:"version"`   // Export format version
}

// SkillLoadEvent represents an event during skill loading
type SkillLoadEvent struct {
	SkillID     ID          `json:"skill_id"`
	EventType   string      `json:"event_type"`   // "loaded", "unloaded", "error"
	State       SkillState  `json:"state"`
	Error       string      `json:"error,omitempty"`
	Timestamp   Timestamp   `json:"timestamp"`
	Duration    time.Duration `json:"duration"` // Load duration
}
