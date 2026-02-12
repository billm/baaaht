package builtin

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/pkg/tools"
)

// =============================================================================
// Shell Tool Base
// =============================================================================

// ShellTool implements the Tool interface for shell command execution
type ShellTool struct {
	name          string
	displayName   string
	definition    tools.ToolDefinition
	logger        *logger.Logger
	mu            sync.RWMutex
	enabled       bool
	stats         tools.ToolUsageStats
	lastUsed      *time.Time
	closed        bool
	blockedCmds   []string
	allowNetwork  bool
	allowFilesystem bool
}

// NewShellTool creates a new shell tool with the given definition
func NewShellTool(def tools.ToolDefinition) (*ShellTool, error) {
	if def.Name == "" {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}

	log, err := logger.NewDefault()
	if err != nil {
		log = &logger.Logger{}
	}

	// Extract blocked commands from security policy
	blockedCmds := def.SecurityPolicy.BlockedCommands
	if blockedCmds == nil {
		blockedCmds = []string{}
	}

	return &ShellTool{
		name:           def.Name,
		displayName:    def.DisplayName,
		definition:     def,
		logger:         log.With("component", fmt.Sprintf("shell_tool_%s", def.Name)),
		enabled:        def.Enabled,
		blockedCmds:    blockedCmds,
		allowNetwork:   def.SecurityPolicy.AllowNetwork,
		allowFilesystem: def.SecurityPolicy.AllowFilesystem,
	}, nil
}

// Name returns the tool name
func (s *ShellTool) Name() string {
	return s.name
}

// Type returns the tool type
func (s *ShellTool) Type() tools.ToolType {
	return tools.ToolTypeShell
}

// Description returns the tool description
func (s *ShellTool) Description() string {
	return s.definition.Description
}

// Execute executes the shell tool operation
func (s *ShellTool) Execute(ctx context.Context, parameters map[string]string) (*tools.ToolExecutionResult, error) {
	startTime := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "tool is closed")
	}

	// Update stats
	s.stats.TotalExecutions++
	now := time.Now()
	s.lastUsed = &now
	s.stats.LastExecution = &now

	// Validate parameters first
	if err := s.Validate(parameters); err != nil {
		s.stats.FailedExecutions++
		return nil, err
	}

	// Execute the specific shell operation
	var output string
	var exitCode int32
	var err error

	switch s.name {
	case "exec":
		output, exitCode, err = s.execCommand(ctx, parameters)
	default:
		s.stats.FailedExecutions++
		return nil, types.NewError(types.ErrCodeInternal, fmt.Sprintf("unknown shell tool: %s", s.name))
	}

	duration := time.Since(startTime)
	s.stats.TotalDuration += duration

	if err != nil {
		s.stats.FailedExecutions++
		// Return error for execution failures
		return nil, types.WrapError(types.ErrCodeInternal, fmt.Sprintf("%s execution failed", s.name), err)
	}

	s.stats.SuccessfulExecutions++
	s.stats.AverageDuration = time.Duration(int64(s.stats.TotalDuration) / s.stats.TotalExecutions)

	return &tools.ToolExecutionResult{
		ToolName:    s.name,
		Status:      tools.ToolExecutionStatusCompleted,
		ExitCode:    exitCode,
		OutputText:  output,
		OutputData:  []byte(output),
		Duration:    duration,
		CompletedAt: time.Now(),
	}, nil
}

// Validate validates the tool parameters
func (s *ShellTool) Validate(parameters map[string]string) error {
	// Check required parameters
	for _, param := range s.definition.Parameters {
		if param.Required {
			if value, ok := parameters[param.Name]; !ok || value == "" {
				return types.NewError(types.ErrCodeInvalidArgument,
					fmt.Sprintf("missing required parameter: %s", param.Name))
			}
		}
	}

	// Perform tool-specific validation
	switch s.name {
	case "exec":
		command := parameters["command"]
		if command == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "command cannot be empty")
		}
		// Check against blocked commands
		if err := s.checkBlockedCommands(command); err != nil {
			return err
		}
	}

	return nil
}

// Definition returns the tool definition
func (s *ShellTool) Definition() tools.ToolDefinition {
	// Update the definition with runtime info
	def := s.definition
	def.Enabled = s.Enabled()
	return def
}

// Status returns the current status
func (s *ShellTool) Status() types.Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return types.StatusStopped
	}
	return types.StatusRunning
}

// IsAvailable returns true if the tool is available
func (s *ShellTool) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.closed
}

// Enabled returns true if the tool is enabled
func (s *ShellTool) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// SetEnabled enables or disables the tool
func (s *ShellTool) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
}

// Stats returns usage statistics
func (s *ShellTool) Stats() tools.ToolUsageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// LastUsed returns the last used time
func (s *ShellTool) LastUsed() *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastUsed
}

// Close closes the tool
func (s *ShellTool) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// =============================================================================
// Shell Operations
// =============================================================================

// execCommand executes a shell command and returns its output
func (s *ShellTool) execCommand(ctx context.Context, parameters map[string]string) (string, int32, error) {
	command := parameters["command"]

	// Determine which shell to use
	shell := "/bin/sh"
	if _, err := exec.LookPath("bash"); err == nil {
		shell = "/bin/bash"
	}

	// Create the command with shell
	cmd := exec.CommandContext(ctx, shell, "-c", command)

	// Execute and capture output
	output, err := cmd.CombinedOutput()
	exitCode := int32(0)

	if err != nil {
		// Check if it's an exit error
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = int32(exitError.ExitCode())
		} else {
			exitCode = 1
		}
		// Still return output even on error
		return string(output), exitCode, nil
	}

	return string(output), exitCode, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// checkBlockedCommands checks if a command is in the blocked list
func (s *ShellTool) checkBlockedCommands(command string) error {
	commandLower := strings.ToLower(strings.TrimSpace(command))

	for _, blocked := range s.blockedCmds {
		blockedLower := strings.ToLower(strings.TrimSpace(blocked))
		if strings.Contains(commandLower, blockedLower) {
			return types.NewError(types.ErrCodePermissionDenied,
				fmt.Sprintf("command '%s' is blocked by security policy", blocked))
		}
	}

	return nil
}

// =============================================================================
// Factory Functions
// =============================================================================

// ShellToolFactory creates a shell tool from a definition
func ShellToolFactory(def tools.ToolDefinition) (tools.Tool, error) {
	return NewShellTool(def)
}
