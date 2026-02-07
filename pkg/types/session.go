package types

import "time"

// SessionState represents the lifecycle state of a session
type SessionState string

const (
	SessionStateInitializing SessionState = "initializing"
	SessionStateActive       SessionState = "active"
	SessionStateIdle         SessionState = "idle"
	SessionStateClosing      SessionState = "closing"
	SessionStateClosed       SessionState = "closed"
)

// Session represents a user session with conversation context
type Session struct {
	ID          ID                `json:"id"`
	State       SessionState      `json:"state"`
	Status      Status            `json:"status"`
	CreatedAt   Timestamp         `json:"created_at"`
	UpdatedAt   Timestamp         `json:"updated_at"`
	ExpiresAt   *Timestamp        `json:"expires_at,omitempty"`
	ClosedAt    *Timestamp        `json:"closed_at,omitempty"`
	Metadata    SessionMetadata   `json:"metadata"`
	Context     SessionContext    `json:"context"`
	Containers  []ID              `json:"containers,omitempty"`
}

// SessionMetadata contains descriptive information about a session
type SessionMetadata struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	OwnerID     string            `json:"owner_id,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

// SessionContext holds the conversation and operational context
type SessionContext struct {
	// Conversation history and state
	Messages      []Message       `json:"messages,omitempty"`
	CurrentTaskID *ID             `json:"current_task_id,omitempty"`
	TaskHistory   []ID            `json:"task_history,omitempty"`

	// User preferences and settings
	Preferences   UserPreferences `json:"preferences,omitempty"`

	// Session-specific configuration
	Config        SessionConfig   `json:"config,omitempty"`

	// Resource usage tracking
	ResourceUsage ResourceUsage   `json:"resource_usage,omitempty"`
}

// Message represents a message in the conversation
type Message struct {
	ID        ID              `json:"id"`
	Timestamp Timestamp       `json:"timestamp"`
	Role      MessageRole     `json:"role"`
	Content   string          `json:"content"`
	Metadata  MessageMetadata `json:"metadata,omitempty"`
}

// MessageRole represents the role of the message sender
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleSystem    MessageRole = "system"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
)

// MessageMetadata contains additional information about a message
type MessageMetadata struct {
	ContainerID *ID               `json:"container_id,omitempty"`
	ToolName    string            `json:"tool_name,omitempty"`
	ToolCallID  string            `json:"tool_call_id,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// UserPreferences contains user-specific preferences
type UserPreferences struct {
	Timezone     string `json:"timezone,omitempty"`
	Language     string `json:"language,omitempty"`
	Theme        string `json:"theme,omitempty"`
	Notifications bool   `json:"notifications,omitempty"`
}

// SessionConfig contains session-specific configuration
type SessionConfig struct {
	MaxContainers    int               `json:"max_containers,omitempty"`
	MaxDuration      time.Duration     `json:"max_duration,omitempty"`
	IdleTimeout      time.Duration     `json:"idle_timeout,omitempty"`
	ResourceLimits   ResourceLimits    `json:"resource_limits,omitempty"`
	AllowedImages    []string          `json:"allowed_images,omitempty"`
	Policy           SessionPolicy     `json:"policy,omitempty"`
}

// SessionPolicy defines security policies for a session
type SessionPolicy struct {
	AllowNetwork     bool              `json:"allow_network"`
	AllowedNetworks  []string          `json:"allowed_networks,omitempty"`
	AllowVolumeMount bool              `json:"allow_volume_mount"`
	AllowedPaths     []string          `json:"allowed_paths,omitempty"`
	RequireApproval  bool              `json:"require_approval,omitempty"`
}

// SessionFilter defines filters for querying sessions
type SessionFilter struct {
	State    *SessionState     `json:"state,omitempty"`
	Status   *Status           `json:"status,omitempty"`
	OwnerID  *string           `json:"owner_id,omitempty"`
	Label    map[string]string `json:"label,omitempty"`
}

// SessionStats represents statistics about a session
type SessionStats struct {
	SessionID       ID        `json:"session_id"`
	MessageCount    int       `json:"message_count"`
	ContainerCount  int       `json:"container_count"`
	TaskCount       int       `json:"task_count"`
	DurationSeconds int64     `json:"duration_seconds"`
	Timestamp       Timestamp `json:"timestamp"`
}
