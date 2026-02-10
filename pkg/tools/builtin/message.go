package builtin

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/ipc"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/pkg/tools"
)

// =============================================================================
// Message Tool Base
// =============================================================================

// MessageTool implements the Tool interface for messaging operations
type MessageTool struct {
	name        string
	displayName string
	definition  tools.ToolDefinition
	logger      *logger.Logger
	broker      *ipc.Broker
	sourceID    types.ID
	mu          sync.RWMutex
	enabled     bool
	stats       tools.ToolUsageStats
	lastUsed    *time.Time
	closed      bool
}

// NewMessageTool creates a new message tool with the given definition
func NewMessageTool(def tools.ToolDefinition) (*MessageTool, error) {
	if def.Name == "" {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}

	log, err := logger.NewDefault()
	if err != nil {
		log = &logger.Logger{}
	}

	// Get the global IPC broker
	broker := ipc.Global()

	// Generate a unique source ID for this tool instance
	sourceID := types.GenerateID()

	return &MessageTool{
		name:        def.Name,
		displayName: def.DisplayName,
		definition:  def,
		logger:      log.With("component", fmt.Sprintf("message_tool_%s", def.Name)),
		broker:      broker,
		sourceID:    sourceID,
		enabled:     def.Enabled,
	}, nil
}

// Name returns the tool name
func (m *MessageTool) Name() string {
	return m.name
}

// Type returns the tool type
func (m *MessageTool) Type() tools.ToolType {
	return tools.ToolTypeMessage
}

// Description returns the tool description
func (m *MessageTool) Description() string {
	return m.definition.Description
}

// Execute executes the message tool operation
func (m *MessageTool) Execute(ctx context.Context, parameters map[string]string) (*tools.ToolExecutionResult, error) {
	startTime := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "tool is closed")
	}

	// Update stats
	m.stats.TotalExecutions++
	now := time.Now()
	m.lastUsed = &now

	// Validate parameters first
	if err := m.Validate(parameters); err != nil {
		m.stats.FailedExecutions++
		return nil, err
	}

	// Execute the specific messaging operation
	var output string
	var exitCode int32
	var err error

	switch m.name {
	case "message":
		output, exitCode, err = m.sendMessage(ctx, parameters)
	default:
		m.stats.FailedExecutions++
		return nil, types.NewError(types.ErrCodeInternal, fmt.Sprintf("unknown message tool: %s", m.name))
	}

	duration := time.Since(startTime)
	m.stats.TotalDuration += duration

	if err != nil {
		m.stats.FailedExecutions++
		// Return error for execution failures
		return nil, types.WrapError(types.ErrCodeInternal, fmt.Sprintf("%s execution failed", m.name), err)
	}

	m.stats.SuccessfulExecutions++
	m.stats.AverageDuration = time.Duration(int64(m.stats.TotalDuration) / m.stats.TotalExecutions)

	return &tools.ToolExecutionResult{
		ToolName:    m.name,
		Status:      tools.ToolExecutionStatusCompleted,
		ExitCode:    exitCode,
		OutputText:  output,
		OutputData:  []byte(output),
		Duration:    duration,
		CompletedAt: time.Now(),
	}, nil
}

// Validate validates the tool parameters
func (m *MessageTool) Validate(parameters map[string]string) error {
	// Check required parameters
	for _, param := range m.definition.Parameters {
		if param.Required {
			if value, ok := parameters[param.Name]; !ok || value == "" {
				return types.NewError(types.ErrCodeInvalidArgument,
					fmt.Sprintf("missing required parameter: %s", param.Name))
			}
		}
	}

	// Perform tool-specific validation
	switch m.name {
	case "message":
		content := parameters["content"]
		if content == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "message content cannot be empty")
		}
		// Check content length (max 10KB)
		const maxContentLength = 10 * 1024
		if len(content) > maxContentLength {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("message content exceeds maximum length of %d bytes", maxContentLength))
		}
	}

	return nil
}

// Definition returns the tool definition
func (m *MessageTool) Definition() tools.ToolDefinition {
	// Update the definition with runtime info
	def := m.definition
	def.Enabled = m.Enabled()
	return def
}

// Status returns the current status
func (m *MessageTool) Status() types.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return types.StatusStopped
	}
	return types.StatusRunning
}

// IsAvailable returns true if the tool is available
func (m *MessageTool) IsAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.closed
}

// Enabled returns true if the tool is enabled
func (m *MessageTool) Enabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// SetEnabled enables or disables the tool
func (m *MessageTool) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

// Stats returns usage statistics
func (m *MessageTool) Stats() tools.ToolUsageStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

// LastUsed returns the last used time
func (m *MessageTool) LastUsed() *time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastUsed
}

// Close closes the tool
func (m *MessageTool) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// =============================================================================
// Message Operations
// =============================================================================

// sendMessage sends a message through the IPC broker
func (m *MessageTool) sendMessage(ctx context.Context, parameters map[string]string) (string, int32, error) {
	content := parameters["content"]
	target := parameters["target"]
	messageType := parameters["type"]
	sessionID := parameters["session_id"]

	// Default to orchestrator as target if not specified
	targetID := types.ID(target)
	if targetID.IsEmpty() {
		targetID = types.NewID("orchestrator")
	}

	// Default message type to "notification" if not specified
	if messageType == "" {
		messageType = "notification"
	}

	// Create IPC message
	msg := &types.IPCMessage{
		Source:  m.sourceID,
		Target:  targetID,
		Type:    messageType,
		Payload: []byte(content),
		Metadata: types.IPCMetadata{
			MessageID: types.GenerateID().String(),
			Headers:   make(map[string]string),
		},
		Timestamp: types.NewTimestampFromTime(time.Now()),
	}

	// Add session ID if provided
	if sessionID != "" {
		msg.Metadata.SessionID = types.NewID(sessionID)
	}

	// Add metadata headers
	msg.Metadata.Headers["tool_name"] = m.name
	msg.Metadata.Headers["content_length"] = fmt.Sprintf("%d", len(content))

	// Send the message through the broker
	if err := m.broker.Send(ctx, msg); err != nil {
		return "", 1, fmt.Errorf("failed to send message: %w", err)
	}

	m.logger.Debug("Message sent",
		"message_id", msg.ID,
		"target", targetID,
		"type", messageType,
		"content_length", len(content))

	return fmt.Sprintf("Message sent successfully to %s (ID: %s)", targetID, msg.Metadata.MessageID), 0, nil
}

// =============================================================================
// Factory Functions
// =============================================================================

// MessageToolFactory creates a message tool from a definition
func MessageToolFactory(def tools.ToolDefinition) (tools.Tool, error) {
	return NewMessageTool(def)
}
