package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/pkg/tools"
)

// =============================================================================
// File Tool Base
// =============================================================================

// FileTool implements the Tool interface for file I/O operations
type FileTool struct {
	name       string
	displayName string
	definition tools.ToolDefinition
	logger     *logger.Logger
	mu         sync.RWMutex
	enabled    bool
	stats      tools.ToolUsageStats
	lastUsed   *time.Time
	closed     bool
}

// NewFileTool creates a new file tool with the given definition
func NewFileTool(def tools.ToolDefinition) (*FileTool, error) {
	if def.Name == "" {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "tool name cannot be empty")
	}

	log, err := logger.NewDefault()
	if err != nil {
		log = &logger.Logger{}
	}

	return &FileTool{
		name:        def.Name,
		displayName: def.DisplayName,
		definition:  def,
		logger:      log.With("component", fmt.Sprintf("file_tool_%s", def.Name)),
		enabled:     def.Enabled,
	}, nil
}

// Name returns the tool name
func (f *FileTool) Name() string {
	return f.name
}

// Type returns the tool type
func (f *FileTool) Type() tools.ToolType {
	return tools.ToolTypeFile
}

// Description returns the tool description
func (f *FileTool) Description() string {
	return f.definition.Description
}

// Execute executes the file tool operation
func (f *FileTool) Execute(ctx context.Context, parameters map[string]string) (*tools.ToolExecutionResult, error) {
	startTime := time.Now()

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "tool is closed")
	}

	// Update stats
	f.stats.TotalExecutions++
	now := time.Now()
	f.lastUsed = &now

	// Validate parameters first
	if err := f.Validate(parameters); err != nil {
		f.stats.FailedExecutions++
		return nil, err
	}

	// Execute the specific file operation
	var output string
	var exitCode int32
	var err error

	switch f.name {
	case "read_file":
		output, exitCode, err = f.readFile(ctx, parameters)
	case "write_file":
		output, exitCode, err = f.writeFile(ctx, parameters)
	case "edit_file":
		output, exitCode, err = f.editFile(ctx, parameters)
	case "list_dir":
		output, exitCode, err = f.listDir(ctx, parameters)
	case "grep":
		output, exitCode, err = f.grep(ctx, parameters)
	case "find":
		output, exitCode, err = f.find(ctx, parameters)
	default:
		f.stats.FailedExecutions++
		return nil, types.NewError(types.ErrCodeInternal, fmt.Sprintf("unknown file tool: %s", f.name))
	}

	duration := time.Since(startTime)
	f.stats.TotalDuration += duration

	if err != nil {
		f.stats.FailedExecutions++
		// Return error for execution failures
		return nil, types.WrapError(types.ErrCodeInternal, fmt.Sprintf("%s execution failed", f.name), err)
	}

	f.stats.SuccessfulExecutions++
	f.stats.AverageDuration = time.Duration(int64(f.stats.TotalDuration) / f.stats.TotalExecutions)

	return &tools.ToolExecutionResult{
		ToolName:    f.name,
		Status:      tools.ToolExecutionStatusCompleted,
		ExitCode:    exitCode,
		OutputText:  output,
		OutputData:  []byte(output),
		Duration:    duration,
		CompletedAt: time.Now(),
	}, nil
}

// Validate validates the tool parameters
func (f *FileTool) Validate(parameters map[string]string) error {
	// Check required parameters
	for _, param := range f.definition.Parameters {
		if param.Required {
			if value, ok := parameters[param.Name]; !ok || value == "" {
				return types.NewError(types.ErrCodeInvalidArgument,
					fmt.Sprintf("missing required parameter: %s", param.Name))
			}
		}
	}

	// Perform tool-specific validation (without checking file existence)
	switch f.name {
	case "read_file", "write_file", "edit_file":
		// Just validate path format, not existence
		if parameters["path"] == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "file path cannot be empty")
		}
	case "list_dir", "find":
		// Just validate path format, not existence
		if parameters["path"] == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "directory path cannot be empty")
		}
	case "grep":
		if parameters["pattern"] == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "grep pattern cannot be empty")
		}
		if parameters["path"] == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "grep path cannot be empty")
		}
	}

	return nil
}

// Definition returns the tool definition
func (f *FileTool) Definition() tools.ToolDefinition {
	// Update the definition with runtime info
	def := f.definition
	def.Enabled = f.Enabled()
	return def
}

// Status returns the current status
func (f *FileTool) Status() types.Status {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.closed {
		return types.StatusStopped
	}
	return types.StatusRunning
}

// IsAvailable returns true if the tool is available
func (f *FileTool) IsAvailable() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return !f.closed
}

// Enabled returns true if the tool is enabled
func (f *FileTool) Enabled() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.enabled
}

// SetEnabled enables or disables the tool
func (f *FileTool) SetEnabled(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enabled = enabled
}

// Stats returns usage statistics
func (f *FileTool) Stats() tools.ToolUsageStats {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.stats
}

// LastUsed returns the last used time
func (f *FileTool) LastUsed() *time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastUsed
}

// Close closes the tool
func (f *FileTool) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// =============================================================================
// File Operations
// =============================================================================

// readFile reads a file and returns its contents
func (f *FileTool) readFile(ctx context.Context, parameters map[string]string) (string, int32, error) {
	path := parameters["path"]

	// Validate path exists and is a file
	if err := f.validateFilePath(path); err != nil {
		return "", 1, err
	}

	// Read file contents
	content, err := os.ReadFile(path)
	if err != nil {
		return "", 1, fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), 0, nil
}

// writeFile writes content to a file
func (f *FileTool) writeFile(ctx context.Context, parameters map[string]string) (string, int32, error) {
	path := parameters["path"]
	content := parameters["content"]

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", 1, fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", 1, fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), 0, nil
}

// editFile edits a file by replacing old_text with new_text
func (f *FileTool) editFile(ctx context.Context, parameters map[string]string) (string, int32, error) {
	path := parameters["path"]
	oldText := parameters["old_text"]
	newText := parameters["new_text"]

	if oldText == "" {
		return "", 1, types.NewError(types.ErrCodeInvalidArgument, "old_text cannot be empty")
	}

	// Validate path exists and is a file
	if err := f.validateFilePath(path); err != nil {
		return "", 1, err
	}

	// Read file contents
	content, err := os.ReadFile(path)
	if err != nil {
		return "", 1, fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)

	// Check if old text exists
	if !strings.Contains(contentStr, oldText) {
		return "", 1, fmt.Errorf("old_text not found in file")
	}

	// Replace old text with new text
	newContent := strings.ReplaceAll(contentStr, oldText, newText)

	// Write updated content
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return "", 1, fmt.Errorf("failed to write file: %w", err)
	}

	replacements := strings.Count(contentStr, oldText)
	return fmt.Sprintf("Successfully replaced %d occurrence(s) in %s", replacements, path), 0, nil
}

// listDir lists the contents of a directory
func (f *FileTool) listDir(ctx context.Context, parameters map[string]string) (string, int32, error) {
	path := parameters["path"]

	// Validate directory
	if err := f.validateDirectoryPath(path); err != nil {
		return "", 1, err
	}

	// Read directory
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", 1, fmt.Errorf("failed to read directory: %w", err)
	}

	// Build output
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Contents of %s:\n", path))
	for _, entry := range entries {
		info, _ := entry.Info()
		if info != nil {
			typeStr := "file"
			if entry.IsDir() {
				typeStr = "dir"
			}
			output.WriteString(fmt.Sprintf("  %s (%s)\n", entry.Name(), typeStr))
		} else {
			output.WriteString(fmt.Sprintf("  %s\n", entry.Name()))
		}
	}

	return output.String(), 0, nil
}

// grep searches for a pattern in files
func (f *FileTool) grep(ctx context.Context, parameters map[string]string) (string, int32, error) {
	pattern := parameters["pattern"]
	path := parameters["path"]

	// Check if path is a file or directory
	info, err := os.Stat(path)
	if err != nil {
		return "", 1, fmt.Errorf("failed to stat path: %w", err)
	}

	var results strings.Builder

	if info.IsDir() {
		// Search recursively in directory
		err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			matches, err := f.grepFile(filePath, pattern)
			if err != nil {
				// Log error but continue searching
				f.logger.Debug("Failed to grep file", "path", filePath, "error", err)
				return nil
			}
			if len(matches) > 0 {
				results.WriteString(fmt.Sprintf("%s:\n", filePath))
				for _, match := range matches {
					results.WriteString(fmt.Sprintf("  %s\n", match))
				}
			}
			return nil
		})
	} else {
		// Search single file
		matches, err := f.grepFile(path, pattern)
		if err != nil {
			return "", 1, fmt.Errorf("failed to grep file: %w", err)
		}
		if len(matches) == 0 {
			return fmt.Sprintf("No matches found for pattern: %s", pattern), 0, nil
		}
		for _, match := range matches {
			results.WriteString(fmt.Sprintf("%s\n", match))
		}
	}

	if err != nil {
		return "", 1, fmt.Errorf("failed to walk directory: %w", err)
	}

	result := results.String()
	if result == "" {
		return fmt.Sprintf("No matches found for pattern: %s", pattern), 0, nil
	}

	return result, 0, nil
}

// find finds files by pattern
func (f *FileTool) find(ctx context.Context, parameters map[string]string) (string, int32, error) {
	pattern := parameters["pattern"]
	path := parameters["path"]

	// Validate directory
	if err := f.validateDirectoryPath(path); err != nil {
		return "", 1, err
	}

	var results []string

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if filename matches pattern
		matched, err := filepath.Match(pattern, filepath.Base(filePath))
		if err != nil {
			return fmt.Errorf("invalid pattern: %w", err)
		}
		if matched {
			results = append(results, filePath)
		}

		return nil
	})

	if err != nil {
		return "", 1, fmt.Errorf("failed to walk directory: %w", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No files found matching pattern: %s", pattern), 0, nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d file(s):\n", len(results)))
	for _, result := range results {
		output.WriteString(fmt.Sprintf("  %s\n", result))
	}

	return output.String(), 0, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// validateFilePath validates that a path is a valid file
func (f *FileTool) validateFilePath(path string) error {
	if path == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "file path cannot be empty")
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check if path exists
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			// For write_file, the file may not exist yet
			if f.name == "write_file" {
				return nil
			}
			return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("file not found: %s", cleanPath))
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Check if it's a file (not directory)
	if info.IsDir() {
		return types.NewError(types.ErrCodeInvalidArgument, fmt.Sprintf("path is a directory, not a file: %s", cleanPath))
	}

	return nil
}

// validateDirectoryPath validates that a path is a valid directory
func (f *FileTool) validateDirectoryPath(path string) error {
	if path == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "directory path cannot be empty")
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check if path exists
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("directory not found: %s", cleanPath))
		}
		return fmt.Errorf("failed to stat directory: %w", err)
	}

	// Check if it's a directory
	if !info.IsDir() {
		return types.NewError(types.ErrCodeInvalidArgument, fmt.Sprintf("path is a file, not a directory: %s", cleanPath))
	}

	return nil
}

// grepFile searches for a pattern in a single file
func (f *FileTool) grepFile(filePath, pattern string) ([]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var matches []string
	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		if strings.Contains(line, pattern) {
			matches = append(matches, fmt.Sprintf("%d: %s", i+1, line))
		}
	}

	return matches, nil
}

// =============================================================================
// Factory Functions
// =============================================================================

// FileToolFactory creates a file tool from a definition
func FileToolFactory(def tools.ToolDefinition) (tools.Tool, error) {
	return NewFileTool(def)
}
