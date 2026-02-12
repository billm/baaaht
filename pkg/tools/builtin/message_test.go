package builtin

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/tools"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test NewMessageTool
// =============================================================================

func TestNewMessageTool(t *testing.T) {
	tests := []struct {
		name    string
		def     tools.ToolDefinition
		wantErr bool
		errCode string
	}{
		{
			name: "valid message tool",
			def: tools.ToolDefinition{
				Name:        "message",
				DisplayName: "Send Message",
				Type:        tools.ToolTypeMessage,
				Description: "Send a message through the orchestrator",
				Enabled:     true,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			def: tools.ToolDefinition{
				Name:    "",
				Enabled: true,
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, err := NewMessageTool(tt.def)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tool)
				if tt.errCode != "" {
					var customErr *types.Error
					assert.ErrorAs(t, err, &customErr)
					assert.Equal(t, tt.errCode, customErr.Code)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tool)
				assert.Equal(t, tt.def.Name, tool.Name())
				assert.Equal(t, tools.ToolTypeMessage, tool.Type())
			}
		})
	}
}

// =============================================================================
// Test Tool Interface Methods
// =============================================================================

func TestMessageTool_InterfaceMethods(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "message",
		DisplayName: "Send Message",
		Type:        tools.ToolTypeMessage,
		Description: "Send a message through the orchestrator",
		Enabled:     true,
	}

	tool, err := NewMessageTool(def)
	require.NoError(t, err)

	// Test Name
	assert.Equal(t, "message", tool.Name())

	// Test Type
	assert.Equal(t, tools.ToolTypeMessage, tool.Type())

	// Test Description
	assert.Equal(t, "Send a message through the orchestrator", tool.Description())

	// Test Definition
	retrievedDef := tool.Definition()
	assert.Equal(t, def.Name, retrievedDef.Name)
	assert.Equal(t, def.Type, retrievedDef.Type)

	// Test Status
	assert.Equal(t, types.StatusRunning, tool.Status())

	// Test IsAvailable
	assert.True(t, tool.IsAvailable())

	// Test Enabled
	assert.True(t, tool.Enabled())

	// Test SetEnabled
	tool.SetEnabled(false)
	assert.False(t, tool.Enabled())

	// Reset enabled for other tests
	tool.SetEnabled(true)

	// Test Stats
	stats := tool.Stats()
	assert.Equal(t, int64(0), stats.TotalExecutions)

	// Test LastUsed
	assert.Nil(t, tool.LastUsed())

	// Test Close
	err = tool.Close()
	assert.NoError(t, err)
	assert.False(t, tool.IsAvailable())
	assert.Equal(t, types.StatusStopped, tool.Status())
}

// =============================================================================
// Test Validation
// =============================================================================

func TestMessageTool_Validate(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "message",
		DisplayName: "Send Message",
		Type:        tools.ToolTypeMessage,
		Description: "Send a message through the orchestrator",
		Parameters: []tools.ToolParameter{
			{
				Name:     "content",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
	}

	tool, err := NewMessageTool(def)
	require.NoError(t, err)

	tests := []struct {
		name       string
		parameters map[string]string
		wantErr    bool
		errCode    string
	}{
		{
			name: "valid message",
			parameters: map[string]string{
				"content": "Hello, world!",
			},
			wantErr: false,
		},
		{
			name:       "missing content parameter",
			parameters: map[string]string{},
			wantErr:    true,
			errCode:    types.ErrCodeInvalidArgument,
		},
		{
			name: "empty content",
			parameters: map[string]string{
				"content": "",
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
		{
			name: "message with target",
			parameters: map[string]string{
				"content": "Test message",
				"target":  "container-123",
			},
			wantErr: false,
		},
		{
			name: "message with type",
			parameters: map[string]string{
				"content": "Test message",
				"type":    "request",
			},
			wantErr: false,
		},
		{
			name: "message with session ID",
			parameters: map[string]string{
				"content":    "Test message",
				"session_id": "session-456",
			},
			wantErr: false,
		},
		{
			name: "message exceeding max length (11KB)",
			parameters: map[string]string{
				"content": strings.Repeat("x", 11*1024),
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
		{
			name: "message at max length boundary (10KB)",
			parameters: map[string]string{
				"content": strings.Repeat("x", 10*1024),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.Validate(tt.parameters)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					var customErr *types.Error
					assert.ErrorAs(t, err, &customErr)
					assert.Equal(t, tt.errCode, customErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Test Execution
// =============================================================================

func TestMessageTool_SendMessage(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "message",
		DisplayName: "Send Message",
		Type:        tools.ToolTypeMessage,
		Description: "Send a message through the orchestrator",
		Parameters: []tools.ToolParameter{
			{
				Name:     "content",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
	}

	tool, err := NewMessageTool(def)
	require.NoError(t, err)

	tests := []struct {
		name         string
		parameters   map[string]string
		wantErr      bool
		wantExitCode int32
		checkOutput  func(string) bool
	}{
		{
			name: "simple message",
			parameters: map[string]string{
				"content": "Hello, world!",
			},
			wantErr:      false,
			wantExitCode: 0,
			checkOutput: func(output string) bool {
				return strings.Contains(output, "Message sent successfully") &&
					strings.Contains(output, "orchestrator")
			},
		},
		{
			name: "message with target",
			parameters: map[string]string{
				"content": "Test message",
				"target":  "container-abc123",
			},
			wantErr:      false,
			wantExitCode: 0,
			checkOutput: func(output string) bool {
				return strings.Contains(output, "Message sent successfully") &&
					strings.Contains(output, "container-abc123")
			},
		},
		{
			name: "message with custom type",
			parameters: map[string]string{
				"content": "Request data",
				"type":    "request",
			},
			wantErr:      false,
			wantExitCode: 0,
			checkOutput: func(output string) bool {
				return strings.Contains(output, "Message sent successfully")
			},
		},
		{
			name: "message with session ID",
			parameters: map[string]string{
				"content":    "Session message",
				"session_id": "session-xyz",
			},
			wantErr:      false,
			wantExitCode: 0,
			checkOutput: func(output string) bool {
				return strings.Contains(output, "Message sent successfully")
			},
		},
		{
			name: "message with all parameters",
			parameters: map[string]string{
				"content":    "Full message",
				"target":     "target-123",
				"type":       "notification",
				"session_id": "session-456",
			},
			wantErr:      false,
			wantExitCode: 0,
			checkOutput: func(output string) bool {
				return strings.Contains(output, "Message sent successfully")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			result, err := tool.Execute(ctx, tt.parameters)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				if err != nil && strings.Contains(err.Error(), "connection not found for container") {
					t.Skipf("Skipping message send test: IPC target unavailable: %v", err)
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "message", result.ToolName)
				assert.Equal(t, tools.ToolExecutionStatusCompleted, result.Status)
				assert.Equal(t, tt.wantExitCode, result.ExitCode)

				if tt.checkOutput != nil {
					assert.True(t, tt.checkOutput(result.OutputText), "Output check failed for: %s", result.OutputText)
				}

				assert.Greater(t, result.Duration, time.Duration(0))
				assert.False(t, result.CompletedAt.IsZero())
			}
		})
	}
}

// =============================================================================
// Test Stats
// =============================================================================

func TestMessageTool_Stats(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "message",
		DisplayName: "Send Message",
		Type:        tools.ToolTypeMessage,
		Description: "Send a message through the orchestrator",
		Parameters: []tools.ToolParameter{
			{
				Name:     "content",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
	}

	tool, err := NewMessageTool(def)
	require.NoError(t, err)

	ctx := context.Background()

	// Execute a successful message
	params := map[string]string{"content": "Test message"}
	_, err = tool.Execute(ctx, params)
	if err != nil && strings.Contains(err.Error(), "connection not found for container") {
		t.Skipf("Skipping message stats test: IPC target unavailable: %v", err)
	}
	require.NoError(t, err)

	// Check stats
	stats := tool.Stats()
	assert.Equal(t, int64(1), stats.TotalExecutions)
	assert.Equal(t, int64(1), stats.SuccessfulExecutions)
	assert.Equal(t, int64(0), stats.FailedExecutions)
	assert.Greater(t, stats.TotalDuration, time.Duration(0))
	assert.Greater(t, stats.AverageDuration, time.Duration(0))
	assert.NotNil(t, stats.LastExecution)

	// Execute a failing message (validation failure)
	params = map[string]string{"content": ""}
	_, err = tool.Execute(ctx, params)
	assert.Error(t, err)

	// Check stats after failure
	stats = tool.Stats()
	assert.Equal(t, int64(2), stats.TotalExecutions)
	assert.Equal(t, int64(1), stats.SuccessfulExecutions)
	assert.Equal(t, int64(1), stats.FailedExecutions)

	// Test LastUsed
	lastUsed := tool.LastUsed()
	assert.NotNil(t, lastUsed)
	assert.False(t, lastUsed.IsZero())
}

// =============================================================================
// Test Closed Tool
// =============================================================================

func TestMessageTool_ClosedTool(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "message",
		DisplayName: "Send Message",
		Type:        tools.ToolTypeMessage,
		Description: "Send a message through the orchestrator",
		Enabled:     true,
	}

	tool, err := NewMessageTool(def)
	require.NoError(t, err)

	// Close the tool
	err = tool.Close()
	assert.NoError(t, err)

	// Verify closed state
	assert.False(t, tool.IsAvailable())
	assert.Equal(t, types.StatusStopped, tool.Status())

	// Try to execute with closed tool
	ctx := context.Background()
	params := map[string]string{"content": "test"}
	_, err = tool.Execute(ctx, params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool is closed")
}

// =============================================================================
// Test Concurrent Execution
// =============================================================================

func TestMessageTool_ConcurrentExecution(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "message",
		DisplayName: "Send Message",
		Type:        tools.ToolTypeMessage,
		Description: "Send a message through the orchestrator",
		Parameters: []tools.ToolParameter{
			{
				Name:     "content",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
	}

	tool, err := NewMessageTool(def)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = tool.Execute(ctx, map[string]string{"content": "preflight"})
	if err != nil && strings.Contains(err.Error(), "connection not found for container") {
		t.Skipf("Skipping message concurrent test: IPC target unavailable: %v", err)
	}
	require.NoError(t, err)

	// Run multiple messages concurrently
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			params := map[string]string{
				"content": fmt.Sprintf("Message %d", idx),
			}
			_, err := tool.Execute(ctx, params)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent executions")
		}
	}

	// Verify stats
	stats := tool.Stats()
	assert.Equal(t, int64(numGoroutines+1), stats.TotalExecutions)
	assert.Equal(t, int64(numGoroutines+1), stats.SuccessfulExecutions)
	assert.Equal(t, int64(0), stats.FailedExecutions)
}

// =============================================================================
// Test Factory Function
// =============================================================================

func TestMessageToolFactory(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "message",
		DisplayName: "Send Message",
		Type:        tools.ToolTypeMessage,
		Description: "Send a message through the orchestrator",
		Enabled:     true,
	}

	// Test factory function
	tool, err := MessageToolFactory(def)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify it implements the Tool interface
	var toolsTool tools.Tool = tool
	assert.Equal(t, "message", toolsTool.Name())
	assert.Equal(t, tools.ToolTypeMessage, toolsTool.Type())
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkMessageTool_SendMessage(b *testing.B) {
	def := tools.ToolDefinition{
		Name:        "message",
		DisplayName: "Send Message",
		Type:        tools.ToolTypeMessage,
		Description: "Send a message through the orchestrator",
		Parameters: []tools.ToolParameter{
			{
				Name:     "content",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
	}

	tool, err := NewMessageTool(def)
	require.NoError(b, err)

	ctx := context.Background()
	params := map[string]string{"content": "Test message"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tool.Execute(ctx, params)
		if err != nil {
			b.Fatal(err)
		}
	}
}
