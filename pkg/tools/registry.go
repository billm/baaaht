package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
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
