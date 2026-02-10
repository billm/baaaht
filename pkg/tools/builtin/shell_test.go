package builtin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/pkg/tools"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test NewShellTool
// =============================================================================

func TestNewShellTool(t *testing.T) {
	tests := []struct {
		name    string
		def     tools.ToolDefinition
		wantErr bool
		errCode string
	}{
		{
			name: "valid shell tool",
			def: tools.ToolDefinition{
				Name:        "exec",
				DisplayName: "Execute Shell Command",
				Type:        tools.ToolTypeShell,
				Description: "Execute a shell command",
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
		{
			name: "tool with blocked commands",
			def: tools.ToolDefinition{
				Name:        "exec",
				DisplayName: "Execute Shell Command",
				Type:        tools.ToolTypeShell,
				Description: "Execute a shell command",
				Enabled:     true,
				SecurityPolicy: tools.ToolSecurityPolicy{
					BlockedCommands: []string{"rm -rf /", "mkfs"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, err := NewShellTool(tt.def)

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
				assert.Equal(t, tools.ToolTypeShell, tool.Type())
			}
		})
	}
}

// =============================================================================
// Test Tool Interface Methods
// =============================================================================

func TestShellTool_InterfaceMethods(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "test_tool",
		DisplayName: "Test Tool",
		Type:        tools.ToolTypeShell,
		Description: "Test shell tool",
		Enabled:     true,
		SecurityPolicy: tools.ToolSecurityPolicy{
			BlockedCommands: []string{"dangerous"},
		},
	}

	tool, err := NewShellTool(def)
	require.NoError(t, err)

	// Test Name
	assert.Equal(t, "test_tool", tool.Name())

	// Test Type
	assert.Equal(t, tools.ToolTypeShell, tool.Type())

	// Test Description
	assert.Equal(t, "Test shell tool", tool.Description())

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

func TestShellTool_Validate(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "exec",
		DisplayName: "Execute Shell Command",
		Type:        tools.ToolTypeShell,
		Description: "Execute a shell command",
		Parameters: []tools.ToolParameter{
			{
				Name:     "command",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
		SecurityPolicy: tools.ToolSecurityPolicy{
			BlockedCommands: []string{"rm -rf /", "mkfs"},
		},
	}

	tool, err := NewShellTool(def)
	require.NoError(t, err)

	tests := []struct {
		name       string
		parameters map[string]string
		wantErr    bool
		errCode    string
	}{
		{
			name: "valid command",
			parameters: map[string]string{
				"command": "echo hello",
			},
			wantErr: false,
		},
		{
			name:       "missing command parameter",
			parameters: map[string]string{},
			wantErr:    true,
			errCode:    types.ErrCodeInvalidArgument,
		},
		{
			name: "empty command",
			parameters: map[string]string{
				"command": "",
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
		{
			name: "blocked command - rm -rf /",
			parameters: map[string]string{
				"command": "rm -rf /",
			},
			wantErr: true,
			errCode: types.ErrCodePermissionDenied,
		},
		{
			name: "blocked command - mkfs",
			parameters: map[string]string{
				"command": "mkfs /dev/sda1",
			},
			wantErr: true,
			errCode: types.ErrCodePermissionDenied,
		},
		{
			name: "blocked command case insensitive",
			parameters: map[string]string{
				"command": "MKFS -t ext4 /dev/sda1",
			},
			wantErr: true,
			errCode: types.ErrCodePermissionDenied,
		},
		{
			name: "safe command with mkdir",
			parameters: map[string]string{
				"command": "mkdir /tmp/test",
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

func TestShellTool_ExecCommand(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "exec",
		DisplayName: "Execute Shell Command",
		Type:        tools.ToolTypeShell,
		Description: "Execute a shell command",
		Parameters: []tools.ToolParameter{
			{
				Name:     "command",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
		SecurityPolicy: tools.ToolSecurityPolicy{
			BlockedCommands: []string{"rm -rf /"},
		},
	}

	tool, err := NewShellTool(def)
	require.NoError(t, err)

	tests := []struct {
		name         string
		command      string
		wantErr      bool
		wantExitCode int32
		checkOutput  func(string) bool
	}{
		{
			name:         "echo command",
			command:      "echo hello world",
			wantErr:      false,
			wantExitCode: 0,
			checkOutput: func(output string) bool {
				return strings.Contains(output, "hello world")
			},
		},
		{
			name:         "ls command",
			command:      "ls /",
			wantErr:      false,
			wantExitCode: 0,
			checkOutput: func(output string) bool {
				return len(output) > 0
			},
		},
		{
			name:         "pwd command",
			command:      "pwd",
			wantErr:      false,
			wantExitCode: 0,
			checkOutput: func(output string) bool {
				return len(strings.TrimSpace(output)) > 0
			},
		},
		{
			name:         "command with pipes",
			command:      "echo test | grep test",
			wantErr:      false,
			wantExitCode: 0,
			checkOutput: func(output string) bool {
				return strings.Contains(output, "test")
			},
		},
		{
			name:         "non-existent command",
			command:      "nonexistentcommand12345",
			wantErr:      false, // The tool doesn't return error, just non-zero exit
			wantExitCode: 127,   // Standard exit code for command not found
			checkOutput: func(output string) bool {
				return true // Output contains error message from shell
			},
		},
		{
			name:         "exit with custom code",
			command:      "exit 42",
			wantErr:      false,
			wantExitCode: 42,
			checkOutput: func(output string) bool {
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			parameters := map[string]string{
				"command": tt.command,
			}

			result, err := tool.Execute(ctx, parameters)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "exec", result.ToolName)
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

func TestShellTool_Stats(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "exec",
		DisplayName: "Execute Shell Command",
		Type:        tools.ToolTypeShell,
		Description: "Execute a shell command",
		Parameters: []tools.ToolParameter{
			{
				Name:     "command",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
	}

	tool, err := NewShellTool(def)
	require.NoError(t, err)

	ctx := context.Background()

	// Execute a successful command
	params := map[string]string{"command": "echo success"}
	_, err = tool.Execute(ctx, params)
	require.NoError(t, err)

	// Check stats
	stats := tool.Stats()
	assert.Equal(t, int64(1), stats.TotalExecutions)
	assert.Equal(t, int64(1), stats.SuccessfulExecutions)
	assert.Equal(t, int64(0), stats.FailedExecutions)
	assert.Greater(t, stats.TotalDuration, time.Duration(0))
	assert.Greater(t, stats.AverageDuration, time.Duration(0))
	assert.NotNil(t, stats.LastExecution)

	// Execute a failing command (validation failure)
	params = map[string]string{"command": ""}
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

func TestShellTool_ClosedTool(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "exec",
		DisplayName: "Execute Shell Command",
		Type:        tools.ToolTypeShell,
		Description: "Execute a shell command",
		Enabled:     true,
	}

	tool, err := NewShellTool(def)
	require.NoError(t, err)

	// Close the tool
	err = tool.Close()
	assert.NoError(t, err)

	// Verify closed state
	assert.False(t, tool.IsAvailable())
	assert.Equal(t, types.StatusStopped, tool.Status())

	// Try to execute with closed tool
	ctx := context.Background()
	params := map[string]string{"command": "echo test"}
	_, err = tool.Execute(ctx, params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool is closed")
}

// =============================================================================
// Test Concurrent Execution
// =============================================================================

func TestShellTool_ConcurrentExecution(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "exec",
		DisplayName: "Execute Shell Command",
		Type:        tools.ToolTypeShell,
		Description: "Execute a shell command",
		Parameters: []tools.ToolParameter{
			{
				Name:     "command",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
	}

	tool, err := NewShellTool(def)
	require.NoError(t, err)

	ctx := context.Background()

	// Run multiple commands concurrently
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			params := map[string]string{
				"command": "echo hello",
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
	assert.Equal(t, int64(numGoroutines), stats.TotalExecutions)
	assert.Equal(t, int64(numGoroutines), stats.SuccessfulExecutions)
	assert.Equal(t, int64(0), stats.FailedExecutions)
}

// =============================================================================
// Test Factory Function
// =============================================================================

func TestShellToolFactory(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "exec",
		DisplayName: "Execute Shell Command",
		Type:        tools.ToolTypeShell,
		Description: "Execute a shell command",
		Enabled:     true,
	}

	// Test factory function
	tool, err := ShellToolFactory(def)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify it implements the Tool interface
	var toolsTool tools.Tool = tool
	assert.Equal(t, "exec", toolsTool.Name())
	assert.Equal(t, tools.ToolTypeShell, toolsTool.Type())
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkShellTool_ExecCommand(b *testing.B) {
	def := tools.ToolDefinition{
		Name:        "exec",
		DisplayName: "Execute Shell Command",
		Type:        tools.ToolTypeShell,
		Description: "Execute a shell command",
		Parameters: []tools.ToolParameter{
			{
				Name:     "command",
				Type:     tools.ParameterTypeString,
				Required: true,
			},
		},
		Enabled: true,
	}

	tool, err := NewShellTool(def)
	require.NoError(b, err)

	ctx := context.Background()
	params := map[string]string{"command": "echo test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tool.Execute(ctx, params)
		if err != nil {
			b.Fatal(err)
		}
	}
}
