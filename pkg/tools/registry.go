package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/tools/builtin"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// =============================================================================
// Tool Interface
// =============================================================================

// Tool defines the interface for tool implementations.
//
// Different tools (file operations, shell commands, web tools, etc.) implement
// this interface to provide a unified API for tool execution and management.
type Tool interface {
	// Name returns the unique name of the tool.
	Name() string

	// Type returns the category of tool (file, shell, web, etc.).
	Type() ToolType

	// Description returns a description of what the tool does.
	Description() string

	// Execute executes the tool with the given parameters.
	//
	// The context can be used to cancel the execution. The parameters map
	// contains the tool-specific parameters. Returns the execution result.
	Execute(ctx context.Context, parameters map[string]string) (*ToolExecutionResult, error)

	// Validate validates the tool parameters before execution.
	//
	// Returns an error if the parameters are invalid or missing required fields.
	Validate(parameters map[string]string) error

	// Definition returns the tool's definition.
	//
	// This includes metadata about the tool, its parameters, security policy,
	// and resource limits.
	Definition() ToolDefinition

	// Status returns the current status of the tool.
	Status() types.Status

	// IsAvailable returns true if the tool is available for execution.
	IsAvailable() bool

	// Enabled returns true if the tool is enabled.
	Enabled() bool

	// SetEnabled enables or disables the tool.
	SetEnabled(enabled bool)

	// Stats returns usage statistics for the tool.
	Stats() ToolUsageStats

	// LastUsed returns the time the tool was last used, if ever.
	LastUsed() *time.Time

	// Close cleans up tool resources.
	Close() error
}

// ToolFactory is a function that creates a Tool instance.
//
// Factory functions are registered with the registry and used to create
// tool instances of specific types.
type ToolFactory func(def ToolDefinition) (Tool, error)

// =============================================================================
// Registry
// =============================================================================

// Registry manages tool implementations with registration and lookup.
//
// It provides a centralized way to register, retrieve, and manage tool instances.
// Tools are created on-demand using registered factories and cached for reuse.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]ToolFactory
	tools     map[string]Tool
	logger    *logger.Logger
	closed    bool
}

// NewRegistry creates a new tool registry.
func NewRegistry(log *logger.Logger) *Registry {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			// If we can't create a logger, continue without one
			log = &logger.Logger{}
		}
	}

	return &Registry{
		factories: make(map[string]ToolFactory),
		tools:     make(map[string]Tool),
		logger:    log.With("component", "tool_registry"),
		closed:    false,
	}
}

// RegisterTool registers a tool factory for a given tool name.
//
// If a factory already exists for the name, it will be overwritten.
func (r *Registry) RegisterTool(toolName string, factory ToolFactory) error {
	if toolName == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}
	if factory == nil {
		return types.NewError(types.ErrCodeInvalidArgument, "tool factory cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "tool registry is closed")
	}

	r.factories[toolName] = factory
	r.logger.Debug("Tool factory registered", "name", toolName)

	return nil
}

// UnregisterTool removes a tool factory from the registry.
//
// If a tool instance exists, it will be closed before removal.
func (r *Registry) UnregisterTool(toolName string) error {
	if toolName == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "tool registry is closed")
	}

	// Close and remove any existing tool instance
	if tool, ok := r.tools[toolName]; ok {
		if err := tool.Close(); err != nil {
			r.logger.Warn("Failed to close tool during unregistration", "name", toolName, "error", err)
		}
		delete(r.tools, toolName)
	}

	delete(r.factories, toolName)
	r.logger.Debug("Tool factory unregistered", "name", toolName)

	return nil
}

// GetTool retrieves or creates a tool instance for the given name.
//
// If an instance already exists, it will be returned.
// If no instance exists, a new one will be created using the registered factory
// with a default tool definition.
func (r *Registry) GetTool(toolName string) (Tool, error) {
	if toolName == "" {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "tool registry is closed")
	}

	// Check if we already have an instance
	if tool, ok := r.tools[toolName]; ok {
		return tool, nil
	}

	// Get the factory
	factory, ok := r.factories[toolName]
	if !ok {
		return nil, types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("no factory registered for tool: %s", toolName))
	}

	// Create a default tool definition for the tool instance
	def := ToolDefinition{
		Name:    toolName,
		Enabled: true,
	}

	// Create a new tool instance
	tool, err := factory(def)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal,
			fmt.Sprintf("failed to create tool: %s", toolName), err)
	}

	// Cache the instance
	r.tools[toolName] = tool
	r.logger.Debug("Tool instance created", "name", toolName)

	return tool, nil
}

// GetToolWithDefinition retrieves or creates a tool instance with a specific definition.
//
// If an instance already exists with the same definition, it will be returned.
// If the definition differs, a new instance will be created.
func (r *Registry) GetToolWithDefinition(toolName string, def ToolDefinition) (Tool, error) {
	if toolName == "" {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "tool registry is closed")
	}

	// Check if we already have an instance with matching definition
	if tool, ok := r.tools[toolName]; ok {
		existingDef := tool.Definition()
		if definitionsMatch(existingDef, def) {
			return tool, nil
		}
		// Definition changed, close the old instance
		if err := tool.Close(); err != nil {
			r.logger.Warn("Failed to close tool with old definition", "name", toolName, "error", err)
		}
		delete(r.tools, toolName)
	}

	// Get the factory
	factory, ok := r.factories[toolName]
	if !ok {
		return nil, types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("no factory registered for tool: %s", toolName))
	}

	// Create a new tool instance with the provided definition
	tool, err := factory(def)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal,
			fmt.Sprintf("failed to create tool: %s", toolName), err)
	}

	// Cache the instance
	r.tools[toolName] = tool
	r.logger.Debug("Tool instance created with definition", "name", toolName)

	return tool, nil
}

// GetExistingTool returns an existing tool instance without creating a new one.
//
// Returns nil if no instance exists.
func (r *Registry) GetExistingTool(toolName string) (Tool, error) {
	if toolName == "" {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "tool registry is closed")
	}

	tool, ok := r.tools[toolName]
	if !ok {
		return nil, types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("no tool instance found: %s", toolName))
	}

	return tool, nil
}

// HasTool returns true if a factory is registered for the given tool name.
func (r *Registry) HasTool(toolName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.factories[toolName]
	return ok
}

// HasToolInstance returns true if a tool instance exists for the given name.
func (r *Registry) HasToolInstance(toolName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.tools[toolName]
	return ok
}

// ListTools returns a list of registered tool names.
func (r *Registry) ListTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// ListInstances returns a list of tool names that have active instances.
func (r *Registry) ListInstances() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := make([]string, 0, len(r.tools))
	for name := range r.tools {
		instances = append(instances, name)
	}
	return instances
}

// ListToolsByType returns a list of tool names of the specified type.
func (r *Registry) ListToolsByType(toolType ToolType) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name, factory := range r.factories {
		// Create a temporary tool to check its type
		def := ToolDefinition{Name: name, Type: toolType}
		tool, err := factory(def)
		if err == nil && tool.Type() == toolType {
			names = append(names, name)
			// Close the temporary tool
			tool.Close()
		}
	}
	return names
}

// GetToolDefinition returns the definition for a tool instance.
func (r *Registry) GetToolDefinition(toolName string) (ToolDefinition, error) {
	tool, err := r.GetTool(toolName)
	if err != nil {
		return ToolDefinition{}, err
	}
	return tool.Definition(), nil
}

// ExecuteTool executes a tool by name with the given parameters.
func (r *Registry) ExecuteTool(ctx context.Context, toolName string, parameters map[string]string) (*ToolExecutionResult, error) {
	tool, err := r.GetTool(toolName)
	if err != nil {
		return nil, err
	}

	if !tool.Enabled() {
		return nil, types.NewError(types.ErrCodePermissionDenied,
			fmt.Sprintf("tool is disabled: %s", toolName))
	}

	if !tool.IsAvailable() {
		return nil, types.NewError(types.ErrCodeUnavailable,
			fmt.Sprintf("tool is not available: %s", toolName))
	}

	return tool.Execute(ctx, parameters)
}

// CloseTool closes a specific tool instance.
func (r *Registry) CloseTool(toolName string) error {
	if toolName == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "tool registry is closed")
	}

	tool, ok := r.tools[toolName]
	if !ok {
		return types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("no tool instance found: %s", toolName))
	}

	err := tool.Close()
	delete(r.tools, toolName)

	if err != nil {
		return types.WrapError(types.ErrCodeInternal,
			fmt.Sprintf("failed to close tool: %s", toolName), err)
	}

	r.logger.Debug("Tool instance closed", "name", toolName)
	return nil
}

// Close closes all tool instances and clears the registry.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	var errs []string

	// Close all tool instances
	for toolName, tool := range r.tools {
		if err := tool.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", toolName, err))
		}
	}

	// Clear the registry
	r.tools = make(map[string]Tool)
	r.factories = make(map[string]ToolFactory)
	r.closed = true

	if len(errs) > 0 {
		return types.NewError(types.ErrCodeInternal,
			fmt.Sprintf("failed to close some tools: %v", errs))
	}

	r.logger.Debug("Registry closed")
	return nil
}

// IsClosed returns true if the registry is closed.
func (r *Registry) IsClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.closed
}

// String returns a string representation of the registry.
func (r *Registry) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return fmt.Sprintf("ToolRegistry{Factories: %d, Instances: %d}",
		len(r.factories), len(r.tools))
}

// GetStats returns statistics for all tool instances.
func (r *Registry) GetStats() map[string]ToolUsageStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]ToolUsageStats)
	for name, tool := range r.tools {
		stats[name] = tool.Stats()
	}
	return stats
}

// =============================================================================
// Helper Functions
// =============================================================================

// definitionsMatch checks if two tool definitions are equivalent.
func definitionsMatch(a, b ToolDefinition) bool {
	if a.Name != b.Name {
		return false
	}
	if a.Type != b.Type {
		return false
	}
	if a.Enabled != b.Enabled {
		return false
	}
	if a.ContainerImage != b.ContainerImage {
		return false
	}
	// For simplicity, we'll check key fields
	// A more thorough implementation would compare all relevant fields
	return true
}

// =============================================================================
// Global Registry
// =============================================================================

// globalRegistry is the default global registry instance
var globalRegistry *Registry
var globalRegistryOnce sync.Once

// GetGlobalRegistry returns the global tool registry.
//
// The registry is initialized on first call.
func GetGlobalRegistry() *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewRegistry(nil)
	})
	return globalRegistry
}

// SetGlobalRegistry sets the global tool registry instance.
func SetGlobalRegistry(registry *Registry) {
	globalRegistry = registry
	globalRegistryOnce = sync.Once{}
}

// RegisterTool registers a tool factory with the global registry.
func RegisterTool(toolName string, factory ToolFactory) error {
	return GetGlobalRegistry().RegisterTool(toolName, factory)
}

// GetTool retrieves a tool from the global registry.
func GetTool(toolName string) (Tool, error) {
	return GetGlobalRegistry().GetTool(toolName)
}

// ExecuteTool executes a tool using the global registry.
func ExecuteTool(ctx context.Context, toolName string, parameters map[string]string) (*ToolExecutionResult, error) {
	return GetGlobalRegistry().ExecuteTool(ctx, toolName, parameters)
}

// ListTools returns a list of all registered tool names from the global registry.
func ListTools() []string {
	return GetGlobalRegistry().ListTools()
}

// =============================================================================
// Built-in Tool Definitions
// =============================================================================

// builtinToolFactories contains the factory functions for all built-in tools.
// These will be replaced by actual implementations in phase 4.
var builtinToolFactories = map[string]ToolFactory{
	// File tools
	"read_file":  builtinFileToolFactory,
	"write_file": builtinFileToolFactory,
	"edit_file":  builtinFileToolFactory,
	"list_dir":   builtinFileToolFactory,
	"grep":       builtinFileToolFactory,
	"find":       builtinFileToolFactory,
	// Shell tool
	"exec": builtinShellToolFactory,
	// Web tools
	"web_search": builtinWebToolFactory,
	"web_fetch":  builtinWebToolFactory,
	// Messaging tool
	"message": builtinMessageToolFactory,
}

// builtinToolDefinitions contains the definitions for all built-in tools.
var builtinToolDefinitions = map[string]ToolDefinition{
	// File tools
	"read_file": {
		Name:        "read_file",
		DisplayName: "Read File",
		Type:        ToolTypeFile,
		Description: "Read the contents of a file",
		Parameters: []ToolParameter{
			{
				Name:         "path",
				Description:  "Path to the file to read",
				Type:         ParameterTypeFilePath,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      true,
			ReadOnlyFilesystem:   true,
			AllowNetwork:         false,
			AllowIPC:             false,
			MaxConcurrent:        10,
		},
		Enabled: true,
	},
	"write_file": {
		Name:        "write_file",
		DisplayName: "Write File",
		Type:        ToolTypeFile,
		Description: "Write content to a file",
		Parameters: []ToolParameter{
			{
				Name:         "path",
				Description:  "Path to the file to write",
				Type:         ParameterTypeFilePath,
				Required:     true,
			},
			{
				Name:         "content",
				Description:  "Content to write to the file",
				Type:         ParameterTypeString,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      true,
			ReadOnlyFilesystem:   false,
			AllowNetwork:         false,
			AllowIPC:             false,
			MaxConcurrent:        10,
		},
		Enabled: true,
	},
	"edit_file": {
		Name:        "edit_file",
		DisplayName: "Edit File",
		Type:        ToolTypeFile,
		Description: "Edit a file by replacing text",
		Parameters: []ToolParameter{
			{
				Name:         "path",
				Description:  "Path to the file to edit",
				Type:         ParameterTypeFilePath,
				Required:     true,
			},
			{
				Name:         "old_text",
				Description:  "Text to replace",
				Type:         ParameterTypeString,
				Required:     true,
			},
			{
				Name:         "new_text",
				Description:  "Replacement text",
				Type:         ParameterTypeString,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      true,
			ReadOnlyFilesystem:   false,
			AllowNetwork:         false,
			AllowIPC:             false,
			MaxConcurrent:        10,
		},
		Enabled: true,
	},
	"list_dir": {
		Name:        "list_dir",
		DisplayName: "List Directory",
		Type:        ToolTypeFile,
		Description: "List the contents of a directory",
		Parameters: []ToolParameter{
			{
				Name:         "path",
				Description:  "Path to the directory to list",
				Type:         ParameterTypeDirectoryPath,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      true,
			ReadOnlyFilesystem:   true,
			AllowNetwork:         false,
			AllowIPC:             false,
			MaxConcurrent:        10,
		},
		Enabled: true,
	},
	"grep": {
		Name:        "grep",
		DisplayName: "Grep",
		Type:        ToolTypeFile,
		Description: "Search for text in files using grep",
		Parameters: []ToolParameter{
			{
				Name:         "pattern",
				Description:  "Regular expression pattern to search for",
				Type:         ParameterTypeString,
				Required:     true,
			},
			{
				Name:         "path",
				Description:  "Path to search in",
				Type:         ParameterTypeString,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      true,
			ReadOnlyFilesystem:   true,
			AllowNetwork:         false,
			AllowIPC:             false,
			MaxConcurrent:        10,
		},
		Enabled: true,
	},
	"find": {
		Name:        "find",
		DisplayName: "Find",
		Type:        ToolTypeFile,
		Description: "Find files by name or pattern",
		Parameters: []ToolParameter{
			{
				Name:         "pattern",
				Description:  "File name pattern to search for",
				Type:         ParameterTypeString,
				Required:     true,
			},
			{
				Name:         "path",
				Description:  "Path to search in",
				Type:         ParameterTypeDirectoryPath,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      true,
			ReadOnlyFilesystem:   true,
			AllowNetwork:         false,
			AllowIPC:             false,
			MaxConcurrent:        10,
		},
		Enabled: true,
	},
	// Shell tool
	"exec": {
		Name:        "exec",
		DisplayName: "Execute Shell Command",
		Type:        ToolTypeShell,
		Description: "Execute a shell command in an isolated container",
		Parameters: []ToolParameter{
			{
				Name:         "command",
				Description:  "Shell command to execute",
				Type:         ParameterTypeString,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      false,
			AllowNetwork:         false,
			AllowIPC:             false,
			MaxConcurrent:        5,
			BlockedCommands:      []string{"rm -rf /", "mkfs", "dd if=/dev/zero"},
		},
		Enabled: true,
	},
	// Web tools
	"web_search": {
		Name:        "web_search",
		DisplayName: "Web Search",
		Type:        ToolTypeWeb,
		Description: "Search the web for information",
		Parameters: []ToolParameter{
			{
				Name:         "query",
				Description:  "Search query",
				Type:         ParameterTypeString,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      false,
			AllowNetwork:         true,
			AllowIPC:             false,
			MaxConcurrent:        5,
		},
		Enabled: true,
	},
	"web_fetch": {
		Name:        "web_fetch",
		DisplayName: "Web Fetch",
		Type:        ToolTypeWeb,
		Description: "Fetch a URL and return its content",
		Parameters: []ToolParameter{
			{
				Name:         "url",
				Description:  "URL to fetch",
				Type:         ParameterTypeString,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      false,
			AllowNetwork:         true,
			AllowIPC:             false,
			MaxConcurrent:        5,
		},
		Enabled: true,
	},
	// Messaging tool
	"message": {
		Name:        "message",
		DisplayName: "Message",
		Type:        ToolTypeMessage,
		Description: "Send a message through the orchestrator",
		Parameters: []ToolParameter{
			{
				Name:         "content",
				Description:  "Message content",
				Type:         ParameterTypeString,
				Required:     true,
			},
		},
		SecurityPolicy: ToolSecurityPolicy{
			AllowFilesystem:      false,
			AllowNetwork:         false,
			AllowIPC:             true,
			MaxConcurrent:        20,
		},
		Enabled: true,
	},
}

// builtinFileToolFactory creates file tools using the builtin implementation.
func builtinFileToolFactory(def ToolDefinition) (Tool, error) {
	return builtin.FileToolFactory(def)
}

// builtinShellToolFactory creates shell tools using the builtin implementation.
func builtinShellToolFactory(def ToolDefinition) (Tool, error) {
	return builtin.ShellToolFactory(def)
}

// builtinWebToolFactory creates web tools using the builtin implementation.
func builtinWebToolFactory(def ToolDefinition) (Tool, error) {
	return builtin.WebToolFactory(def)
}

// builtinMessageToolFactory creates message tools using the builtin implementation.
func builtinMessageToolFactory(def ToolDefinition) (Tool, error) {
	return builtin.MessageToolFactory(def)
}

// builtinToolPlaceholder is a placeholder implementation for built-in tools.
// This allows the tools to be registered while their full implementations
// are being developed in phase 4.
type builtinToolPlaceholder struct {
	name       string
	toolType   ToolType
	definition ToolDefinition
	enabled    bool
	closed     bool
}

func (b *builtinToolPlaceholder) Name() string {
	return b.name
}

func (b *builtinToolPlaceholder) Type() ToolType {
	return b.toolType
}

func (b *builtinToolPlaceholder) Description() string {
	return b.definition.Description
}

func (b *builtinToolPlaceholder) Execute(ctx context.Context, parameters map[string]string) (*ToolExecutionResult, error) {
	if b.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "tool is closed")
	}
	// Placeholder - actual execution will be implemented in phase 4
	return nil, types.NewError(types.ErrCodeInternal,
		fmt.Sprintf("tool %s is not yet implemented", b.name))
}

func (b *builtinToolPlaceholder) Validate(parameters map[string]string) error {
	// Placeholder - basic validation
	for _, param := range b.definition.Parameters {
		if param.Required {
			if _, ok := parameters[param.Name]; !ok {
				return types.NewError(types.ErrCodeInvalidArgument,
					fmt.Sprintf("missing required parameter: %s", param.Name))
			}
		}
	}
	return nil
}

func (b *builtinToolPlaceholder) Definition() ToolDefinition {
	return b.definition
}

func (b *builtinToolPlaceholder) Status() types.Status {
	return types.StatusRunning
}

func (b *builtinToolPlaceholder) IsAvailable() bool {
	return !b.closed
}

func (b *builtinToolPlaceholder) Enabled() bool {
	return b.enabled && !b.closed
}

func (b *builtinToolPlaceholder) SetEnabled(enabled bool) {
	b.enabled = enabled
}

func (b *builtinToolPlaceholder) Stats() ToolUsageStats {
	return ToolUsageStats{}
}

func (b *builtinToolPlaceholder) LastUsed() *time.Time {
	return nil
}

func (b *builtinToolPlaceholder) Close() error {
	b.closed = true
	return nil
}

// RegisterBuiltinTools registers all built-in tool factories with the global registry.
//
// This function should be called during application initialization to ensure
// all built-in tools are available for use. The actual tool implementations
// will be provided in phase 4 of the implementation.
func RegisterBuiltinTools() error {
	reg := GetGlobalRegistry()

	for name, factory := range builtinToolFactories {
		if err := reg.RegisterTool(name, factory); err != nil {
			return types.WrapError(types.ErrCodeInternal,
				fmt.Sprintf("failed to register built-in tool: %s", name), err)
		}
	}

	return nil
}

// GetBuiltinToolDefinitions returns a map of all built-in tool definitions.
func GetBuiltinToolDefinitions() map[string]ToolDefinition {
	defs := make(map[string]ToolDefinition, len(builtinToolDefinitions))
	for k, v := range builtinToolDefinitions {
		defs[k] = v
	}
	return defs
}

// ListBuiltinTools returns a list of all built-in tool names.
func ListBuiltinTools() []string {
	names := make([]string, 0, len(builtinToolDefinitions))
	for name := range builtinToolDefinitions {
		names = append(names, name)
	}
	return names
}
