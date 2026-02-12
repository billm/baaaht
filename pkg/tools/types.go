package tools

import (
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// =============================================================================
// Tool Enums
// =============================================================================

// ToolType represents the category of tool
type ToolType string

const (
	ToolTypeUnspecified ToolType = "unspecified"
	ToolTypeFile        ToolType = "file"        // File operations (read, write, edit, list, grep, find)
	ToolTypeShell       ToolType = "shell"       // Shell command execution
	ToolTypeWeb         ToolType = "web"         // Web operations (search, fetch)
	ToolTypeMessage     ToolType = "message"     // Messaging operations
	ToolTypeCustom      ToolType = "custom"      // Custom user-defined tools
)

// ToolState represents the lifecycle state of a tool
type ToolState string

const (
	ToolStateUnspecified ToolState = "unspecified"
	ToolStateRegistered  ToolState = "registered"
	ToolStateAvailable   ToolState = "available"
	ToolStateBusy        ToolState = "busy"
	ToolStateDisabled    ToolState = "disabled"
	ToolStateError       ToolState = "error"
)

// ToolExecutionStatus represents the status of a tool execution
type ToolExecutionStatus string

const (
	ToolExecutionStatusUnspecified ToolExecutionStatus = "unspecified"
	ToolExecutionStatusPending     ToolExecutionStatus = "pending"
	ToolExecutionStatusRunning     ToolExecutionStatus = "running"
	ToolExecutionStatusCompleted   ToolExecutionStatus = "completed"
	ToolExecutionStatusFailed      ToolExecutionStatus = "failed"
	ToolExecutionStatusTimeout     ToolExecutionStatus = "timeout"
	ToolExecutionStatusCancelled   ToolExecutionStatus = "cancelled"
)

// =============================================================================
// Tool Definition Types
// =============================================================================

// ToolDefinition defines a tool's interface and configuration
type ToolDefinition struct {
	Name            string                `json:"name"`
	DisplayName     string                `json:"display_name"`            // Human-readable name
	Type            ToolType              `json:"type"`
	Description     string                `json:"description"`             // Description of what the tool does
	Parameters      []ToolParameter       `json:"parameters"`              // Parameters the tool accepts
	SecurityPolicy  ToolSecurityPolicy    `json:"security_policy"`         // Security constraints
	ResourceLimits  types.ResourceLimits  `json:"resource_limits"`         // Resource constraints
	Timeout         time.Duration         `json:"timeout"`                 // Default timeout
	ContainerImage  string                `json:"container_image"`         // Container image for execution
	Command         []string              `json:"command"`                 // Command to run in container
	Metadata        map[string]string     `json:"metadata"`                // Additional metadata
	Version         string                `json:"version"`                 // Tool version
	Enabled         bool                  `json:"enabled"`                 // Whether tool is enabled
}

// ToolParameter defines a parameter for a tool
type ToolParameter struct {
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Type          ParameterType          `json:"type"`
	Required      bool                   `json:"required"`
	DefaultValue  string                 `json:"default_value,omitempty"`  // Optional default value
	AllowedValues []string               `json:"allowed_values,omitempty"` // Optional enum of allowed values
	Constraints   map[string]string      `json:"constraints,omitempty"`    // Additional constraints (min, max, pattern, etc.)
}

// ParameterType represents the type of a tool parameter
type ParameterType string

const (
	ParameterTypeUnspecified     ParameterType = "unspecified"
	ParameterTypeString          ParameterType = "string"
	ParameterTypeInteger         ParameterType = "integer"
	ParameterTypeFloat           ParameterType = "float"
	ParameterTypeBoolean         ParameterType = "boolean"
	ParameterTypeArray           ParameterType = "array"
	ParameterTypeObject          ParameterType = "object"
	ParameterTypeFilePath        ParameterType = "file_path"        // Special type for file paths
	ParameterTypeDirectoryPath   ParameterType = "directory_path"   // Special type for directory paths
)

// ToolSecurityPolicy defines security constraints for tool execution
type ToolSecurityPolicy struct {
	AllowNetwork         bool     `json:"allow_network"`
	AllowedHosts         []string `json:"allowed_hosts,omitempty"`         // Allowlist for network access
	BlockedHosts         []string `json:"blocked_hosts,omitempty"`         // Blocklist for network access
	AllowFilesystem      bool     `json:"allow_filesystem"`
	AllowedPaths         []string `json:"allowed_paths,omitempty"`         // Allowlist for filesystem access (scoped mounts)
	ReadOnlyFilesystem   bool     `json:"read_only_filesystem"`
	AllowIPC             bool     `json:"allow_ipc"`
	MaxConcurrent        int32    `json:"max_concurrent"`
	AllowedUsers         []string `json:"allowed_users,omitempty"`         // Users allowed to execute tool
	BlockedCommands      []string `json:"blocked_commands,omitempty"`      // Blocked shell commands
}

// ToolConfig represents runtime configuration for tool execution
type ToolConfig struct {
	Tools                    []ToolDefinition        `json:"tools"`                     // Available tools
	DefaultRegistry          string                  `json:"default_registry"`          // Default container registry
	DefaultEnv               map[string]string       `json:"default_env"`                // Default environment variables
	DefaultResourceLimits    types.ResourceLimits    `json:"default_resource_limits"`    // Default resource limits
	DefaultTimeout           time.Duration           `json:"default_timeout"`            // Default timeout for all tools
	TelemetryEnabled         bool                    `json:"telemetry_enabled"`          // Whether telemetry is enabled
}

// =============================================================================
// Tool Execution Types
// =============================================================================

// ToolExecutionRequest represents a request to execute a tool
type ToolExecutionRequest struct {
	ToolName      string            `json:"tool_name"`
	SessionID     string            `json:"session_id"`
	ExecutionID   string            `json:"execution_id"`    // Unique ID for this execution
	Parameters    map[string]string `json:"parameters"`      // Tool parameters
	Timeout       time.Duration     `json:"timeout"`         // Optional override timeout
	CorrelationID string            `json:"correlation_id"`  // For tracking
	CreatedAt     time.Time         `json:"created_at"`
}

// ToolExecutionResult represents the result of a tool execution
type ToolExecutionResult struct {
	ExecutionID string                `json:"execution_id"`
	ToolName    string                `json:"tool_name"`
	Status      ToolExecutionStatus   `json:"status"`
	ExitCode    int32                 `json:"exit_code"`              // Exit code from container
	OutputData  []byte                `json:"output_data,omitempty"`  // Raw output data
	OutputText  string                `json:"output_text,omitempty"`  // Text output (if applicable)
	ErrorData   []byte                `json:"error_data,omitempty"`   // Raw error data
	ErrorText   string                `json:"error_text,omitempty"`   // Error message (if failed)
	Metadata    map[string]string     `json:"metadata"`               // Additional result metadata
	Duration    time.Duration         `json:"duration"`               // Execution duration
	CompletedAt time.Time             `json:"completed_at"`
	ContainerID string                `json:"container_id,omitempty"` // Container that executed the tool
}

// =============================================================================
// Tool Instance Types
// =============================================================================

// ToolInstance represents a registered tool instance
type ToolInstance struct {
	Name         string             `json:"name"`
	DisplayName  string             `json:"display_name"`
	Type         ToolType           `json:"type"`
	State        ToolState          `json:"state"`
	Status       types.Status       `json:"status"`
	RegisteredAt time.Time          `json:"registered_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	LastUsedAt   *time.Time         `json:"last_used_at,omitempty"` // Optional
	Definition   ToolDefinition     `json:"definition"`
	UsageStats   ToolUsageStats     `json:"usage_stats"`
	Enabled      bool               `json:"enabled"`
	Version      string             `json:"version"`
}

// ToolUsageStats contains usage statistics for a tool
type ToolUsageStats struct {
	TotalExecutions      int64      `json:"total_executions"`
	SuccessfulExecutions int64      `json:"successful_executions"`
	FailedExecutions     int64      `json:"failed_executions"`
	TimeoutExecutions    int64      `json:"timeout_executions"`
	CancelledExecutions  int64      `json:"cancelled_executions"`
	TotalDuration        time.Duration `json:"total_duration"`
	AverageDuration      time.Duration `json:"average_duration"`
	LastExecution        *time.Time `json:"last_execution,omitempty"` // Optional
}

// =============================================================================
// Tool Execution Types
// =============================================================================

// ToolExecution represents a tool execution instance
type ToolExecution struct {
	ExecutionID   string                      `json:"execution_id"`
	ToolName      string                      `json:"tool_name"`
	SessionID     string                      `json:"session_id,omitempty"` // Optional
	Status        ToolExecutionStatus         `json:"status"`
	CreatedAt     time.Time                   `json:"created_at"`
	StartedAt     *time.Time                  `json:"started_at,omitempty"`    // Optional
	CompletedAt   *time.Time                  `json:"completed_at,omitempty"`  // Optional
	ExpiresAt     *time.Time                  `json:"expires_at,omitempty"`    // Optional
	Parameters    map[string]string           `json:"parameters"`
	Result        *ToolExecutionResult        `json:"result,omitempty"`        // Optional
	ContainerID   string                      `json:"container_id,omitempty"`  // Optional
	Duration      time.Duration               `json:"duration"`
	ErrorMessage  string                      `json:"error_message,omitempty"` // Optional
	CorrelationID string                      `json:"correlation_id,omitempty"` // Optional
}

// =============================================================================
// Filter Types
// =============================================================================

// ToolFilter defines filters for querying tools
type ToolFilter struct {
	Type        *ToolType          `json:"type,omitempty"`        // Optional
	State       *ToolState         `json:"state,omitempty"`       // Optional
	Enabled     *bool              `json:"enabled,omitempty"`     // Optional, unset means both
	NamePattern string             `json:"name_pattern,omitempty"` // Optional, substring match
	Labels      map[string]string  `json:"labels,omitempty"`      // Optional, match all if empty
}

// ExecutionFilter defines filters for querying tool executions
type ExecutionFilter struct {
	ToolName      string               `json:"tool_name,omitempty"`      // Optional
	Status        *ToolExecutionStatus `json:"status,omitempty"`         // Optional
	SessionID     string               `json:"session_id,omitempty"`     // Optional
	CreatedAfter  *time.Time           `json:"created_after,omitempty"`  // Optional
	CreatedBefore *time.Time           `json:"created_before,omitempty"` // Optional
}

// =============================================================================
// Service Stats Types
// =============================================================================

// ToolServiceStats contains overall service statistics
type ToolServiceStats struct {
	TotalExecutions    int64     `json:"total_executions"`
	SuccessfulExecutions int64   `json:"successful_executions"`
	FailedExecutions   int64     `json:"failed_executions"`
	ActiveExecutions   int64     `json:"active_executions"`
	StartedAt          time.Time `json:"started_at"`
}

// ToolStats contains statistics for a specific tool
type ToolStats struct {
	ToolName    string         `json:"tool_name"`
	UsageStats  ToolUsageStats `json:"usage_stats"`
}

// =============================================================================
// Stream Types
// =============================================================================

// ToolInput contains input data for a streaming tool execution
type ToolInput struct {
	Data     []byte            `json:"data"`
	Metadata map[string]string `json:"metadata"`
}

// ToolOutput contains output data from a tool
type ToolOutput struct {
	Data       []byte    `json:"data"`
	Text       string    `json:"text,omitempty"`       // Optional, for text output
	StreamType string    `json:"stream_type"`          // stdout, stderr, data
	Timestamp  time.Time `json:"timestamp"`
}

// ToolExecutionProgress contains progress information for a tool execution
type ToolExecutionProgress struct {
	Percent float64            `json:"percent"` // 0.0 to 1.0
	Message string             `json:"message,omitempty"` // Optional
	Details map[string]string  `json:"details"`
	Timestamp time.Time        `json:"timestamp"`
}

// ToolExecutionError contains error information for a failed tool execution
type ToolExecutionError struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Details   []string  `json:"details"`
	OccurredAt time.Time `json:"occurred_at"`
}
