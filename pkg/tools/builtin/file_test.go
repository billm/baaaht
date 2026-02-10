package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/pkg/tools"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test Helpers
// =============================================================================

func createTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

func createTestDir(t *testing.T, baseDir, name string) string {
	t.Helper()
	path := filepath.Join(baseDir, name)
	err := os.MkdirAll(path, 0755)
	require.NoError(t, err)
	return path
}

// =============================================================================
// Test NewFileTool
// =============================================================================

func TestNewFileTool(t *testing.T) {
	tests := []struct {
		name    string
		def     tools.ToolDefinition
		wantErr bool
		errCode string
	}{
		{
			name: "valid file tool",
			def: tools.ToolDefinition{
				Name:        "read_file",
				DisplayName: "Read File",
				Type:        tools.ToolTypeFile,
				Description: "Read a file",
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
			tool, err := NewFileTool(tt.def)

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
				assert.Equal(t, tools.ToolTypeFile, tool.Type())
			}
		})
	}
}

// =============================================================================
// Test Tool Interface Methods
// =============================================================================

func TestFileTool_InterfaceMethods(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "test_tool",
		DisplayName: "Test Tool",
		Type:        tools.ToolTypeFile,
		Description: "Test description",
		Enabled:     true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "test_tool", tool.Name())
	})

	t.Run("Type", func(t *testing.T) {
		assert.Equal(t, tools.ToolTypeFile, tool.Type())
	})

	t.Run("Description", func(t *testing.T) {
		assert.Equal(t, "Test description", tool.Description())
	})

	t.Run("Status", func(t *testing.T) {
		assert.Equal(t, types.StatusRunning, tool.Status())
	})

	t.Run("IsAvailable", func(t *testing.T) {
		assert.True(t, tool.IsAvailable())
	})

	t.Run("Enabled", func(t *testing.T) {
		assert.True(t, tool.Enabled())
	})

	t.Run("SetEnabled", func(t *testing.T) {
		tool.SetEnabled(false)
		assert.False(t, tool.Enabled())
		tool.SetEnabled(true)
		assert.True(t, tool.Enabled())
	})

	t.Run("Stats", func(t *testing.T) {
		stats := tool.Stats()
		assert.Equal(t, int64(0), stats.TotalExecutions)
	})

	t.Run("LastUsed", func(t *testing.T) {
		assert.Nil(t, tool.LastUsed())
	})

	t.Run("Close", func(t *testing.T) {
		assert.NoError(t, tool.Close())
		assert.False(t, tool.IsAvailable())
		assert.Equal(t, types.StatusStopped, tool.Status())
	})
}

// =============================================================================
// Test read_file
// =============================================================================

func TestFileTool_ReadFile(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create test file
	testContent := "Hello, World!\nThis is a test file."
	testFile := createTestFile(t, tmpDir, "test.txt", testContent)

	def := tools.ToolDefinition{
		Name:        "read_file",
		DisplayName: "Read File",
		Type:        tools.ToolTypeFile,
		Description: "Read a file",
		Parameters: []tools.ToolParameter{
			{Name: "path", Type: tools.ParameterTypeFilePath, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	tests := []struct {
		name      string
		params    map[string]string
		wantErr   bool
		wantStatus tools.ToolExecutionStatus
		checkOutput func(t *testing.T, output string)
	}{
		{
			name: "read existing file",
			params: map[string]string{
				"path": testFile,
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkOutput: func(t *testing.T, output string) {
				assert.Equal(t, testContent, output)
			},
		},
		{
			name: "read non-existent file",
			params: map[string]string{
				"path": filepath.Join(tmpDir, "nonexistent.txt"),
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
		{
			name: "missing path parameter",
			params: map[string]string{},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
		{
			name: "read directory instead of file",
			params: map[string]string{
				"path": tmpDir,
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tool.Execute(ctx, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				if result != nil {
					assert.Equal(t, tt.wantStatus, result.Status)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantStatus, result.Status)
				assert.Equal(t, int32(0), result.ExitCode)
				if tt.checkOutput != nil {
					tt.checkOutput(t, result.OutputText)
				}
			}
		})
	}
}

// =============================================================================
// Test write_file
// =============================================================================

func TestFileTool_WriteFile(t *testing.T) {
	tmpDir := t.TempDir()

	def := tools.ToolDefinition{
		Name:        "write_file",
		DisplayName: "Write File",
		Type:        tools.ToolTypeFile,
		Description: "Write a file",
		Parameters: []tools.ToolParameter{
			{Name: "path", Type: tools.ParameterTypeFilePath, Required: true},
			{Name: "content", Type: tools.ParameterTypeString, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	tests := []struct {
		name      string
		params    map[string]string
		wantErr   bool
		wantStatus tools.ToolExecutionStatus
		checkFile func(t *testing.T, path string)
	}{
		{
			name: "write new file",
			params: map[string]string{
				"path":    filepath.Join(tmpDir, "newfile.txt"),
				"content": "Hello, World!",
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkFile: func(t *testing.T, path string) {
				content, err := os.ReadFile(path)
				assert.NoError(t, err)
				assert.Equal(t, "Hello, World!", string(content))
			},
		},
		{
			name: "write file with subdirectory",
			params: map[string]string{
				"path":    filepath.Join(tmpDir, "subdir", "file.txt"),
				"content": "Nested file",
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkFile: func(t *testing.T, path string) {
				content, err := os.ReadFile(path)
				assert.NoError(t, err)
				assert.Equal(t, "Nested file", string(content))
			},
		},
		{
			name: "overwrite existing file",
			params: map[string]string{
				"path":    createTestFile(t, tmpDir, "existing.txt", "old content"),
				"content": "new content",
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkFile: func(t *testing.T, path string) {
				content, err := os.ReadFile(path)
				assert.NoError(t, err)
				assert.Equal(t, "new content", string(content))
			},
		},
		{
			name: "missing path parameter",
			params: map[string]string{
				"content": "content",
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
		{
			name: "missing content parameter",
			params: map[string]string{
				"path": filepath.Join(tmpDir, "test.txt"),
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tool.Execute(ctx, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				if result != nil {
					assert.Equal(t, tt.wantStatus, result.Status)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantStatus, result.Status)
				assert.Equal(t, int32(0), result.ExitCode)
				assert.Contains(t, result.OutputText, "Successfully wrote")
				if tt.checkFile != nil {
					tt.checkFile(t, tt.params["path"])
				}
			}
		})
	}
}

// =============================================================================
// Test edit_file
// =============================================================================

func TestFileTool_EditFile(t *testing.T) {
	tmpDir := t.TempDir()

	def := tools.ToolDefinition{
		Name:        "edit_file",
		DisplayName: "Edit File",
		Type:        tools.ToolTypeFile,
		Description: "Edit a file",
		Parameters: []tools.ToolParameter{
			{Name: "path", Type: tools.ParameterTypeFilePath, Required: true},
			{Name: "old_text", Type: tools.ParameterTypeString, Required: true},
			{Name: "new_text", Type: tools.ParameterTypeString, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	tests := []struct {
		name      string
		params    map[string]string
		wantErr   bool
		wantStatus tools.ToolExecutionStatus
		checkFile func(t *testing.T, path string)
	}{
		{
			name: "replace existing text",
			params: map[string]string{
				"path":     createTestFile(t, tmpDir, "edit.txt", "Hello World\nGoodbye World"),
				"old_text": "World",
				"new_text": "Universe",
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkFile: func(t *testing.T, path string) {
				content, err := os.ReadFile(path)
				assert.NoError(t, err)
				assert.Equal(t, "Hello Universe\nGoodbye Universe", string(content))
			},
		},
		{
			name: "old text not found",
			params: map[string]string{
				"path":     createTestFile(t, tmpDir, "edit2.txt", "Hello World"),
				"old_text": "Mars",
				"new_text": "Universe",
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
		{
			name: "empty old_text",
			params: map[string]string{
				"path":     createTestFile(t, tmpDir, "edit3.txt", "Hello"),
				"old_text": "",
				"new_text": "Goodbye",
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
		{
			name: "file not found",
			params: map[string]string{
				"path":     filepath.Join(tmpDir, "nonexistent.txt"),
				"old_text": "Hello",
				"new_text": "Goodbye",
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tool.Execute(ctx, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				if result != nil {
					assert.Equal(t, tt.wantStatus, result.Status)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantStatus, result.Status)
				assert.Contains(t, result.OutputText, "Successfully replaced")
				if tt.checkFile != nil {
					tt.checkFile(t, tt.params["path"])
				}
			}
		})
	}
}

// =============================================================================
// Test list_dir
// =============================================================================

func TestFileTool_ListDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test structure
	createTestFile(t, tmpDir, "file1.txt", "content1")
	createTestFile(t, tmpDir, "file2.txt", "content2")
	createTestDir(t, tmpDir, "subdir")

	def := tools.ToolDefinition{
		Name:        "list_dir",
		DisplayName: "List Directory",
		Type:        tools.ToolTypeFile,
		Description: "List directory contents",
		Parameters: []tools.ToolParameter{
			{Name: "path", Type: tools.ParameterTypeDirectoryPath, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	tests := []struct {
		name         string
		params       map[string]string
		wantErr      bool
		wantStatus   tools.ToolExecutionStatus
		checkOutput  func(t *testing.T, output string)
	}{
		{
			name: "list existing directory",
			params: map[string]string{
				"path": tmpDir,
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "file1.txt")
				assert.Contains(t, output, "file2.txt")
				assert.Contains(t, output, "subdir")
			},
		},
		{
			name: "list non-existent directory",
			params: map[string]string{
				"path": filepath.Join(tmpDir, "nonexistent"),
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
		{
			name: "list file instead of directory",
			params: map[string]string{
				"path": createTestFile(t, tmpDir, "notdir.txt", "content"),
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
		{
			name: "missing path parameter",
			params: map[string]string{},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tool.Execute(ctx, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				if result != nil {
					assert.Equal(t, tt.wantStatus, result.Status)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantStatus, result.Status)
				assert.Equal(t, int32(0), result.ExitCode)
				if tt.checkOutput != nil {
					tt.checkOutput(t, result.OutputText)
				}
			}
		})
	}
}

// =============================================================================
// Test grep
// =============================================================================

func TestFileTool_Grep(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test structure
	createTestFile(t, tmpDir, "file1.txt", "Hello World\nGoodbye World\nHello Mars")
	createTestFile(t, tmpDir, "file2.txt", "Python Ruby\nGo Rust")
	subdir := createTestDir(t, tmpDir, "subdir")
	createTestFile(t, subdir, "file3.txt", "Hello Universe\nGoodbye Universe")

	def := tools.ToolDefinition{
		Name:        "grep",
		DisplayName: "Grep",
		Type:        tools.ToolTypeFile,
		Description: "Search for text",
		Parameters: []tools.ToolParameter{
			{Name: "pattern", Type: tools.ParameterTypeString, Required: true},
			{Name: "path", Type: tools.ParameterTypeString, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]string
		wantErr     bool
		wantStatus  tools.ToolExecutionStatus
		checkOutput func(t *testing.T, output string)
	}{
		{
			name: "search in single file with match",
			params: map[string]string{
				"pattern": "Hello",
				"path":    filepath.Join(tmpDir, "file1.txt"),
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "Hello World")
				assert.Contains(t, output, "Hello Mars")
			},
		},
		{
			name: "search in single file no match",
			params: map[string]string{
				"pattern": "Java",
				"path":    filepath.Join(tmpDir, "file1.txt"),
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "No matches found")
			},
		},
		{
			name: "search in directory",
			params: map[string]string{
				"pattern": "Hello",
				"path":    tmpDir,
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "file1.txt")
				assert.Contains(t, output, "file3.txt")
				assert.NotContains(t, output, "file2.txt")
			},
		},
		{
			name: "empty pattern",
			params: map[string]string{
				"pattern": "",
				"path":    tmpDir,
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
		{
			name: "non-existent path",
			params: map[string]string{
				"pattern": "Hello",
				"path":    filepath.Join(tmpDir, "nonexistent"),
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tool.Execute(ctx, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				if result != nil {
					assert.Equal(t, tt.wantStatus, result.Status)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantStatus, result.Status)
				if tt.checkOutput != nil {
					tt.checkOutput(t, result.OutputText)
				}
			}
		})
	}
}

// =============================================================================
// Test find
// =============================================================================

func TestFileTool_Find(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test structure
	createTestFile(t, tmpDir, "file1.txt", "content1")
	createTestFile(t, tmpDir, "file2.txt", "content2")
	createTestFile(t, tmpDir, "other.md", "content3")
	subdir := createTestDir(t, tmpDir, "subdir")
	createTestFile(t, subdir, "file3.txt", "content4")

	def := tools.ToolDefinition{
		Name:        "find",
		DisplayName: "Find",
		Type:        tools.ToolTypeFile,
		Description: "Find files",
		Parameters: []tools.ToolParameter{
			{Name: "pattern", Type: tools.ParameterTypeString, Required: true},
			{Name: "path", Type: tools.ParameterTypeDirectoryPath, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]string
		wantErr     bool
		wantStatus  tools.ToolExecutionStatus
		checkOutput func(t *testing.T, output string)
	}{
		{
			name: "find files by pattern",
			params: map[string]string{
				"pattern": "*.txt",
				"path":    tmpDir,
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "file1.txt")
				assert.Contains(t, output, "file2.txt")
				assert.Contains(t, output, "file3.txt")
				assert.NotContains(t, output, "other.md")
			},
		},
		{
			name: "find specific file",
			params: map[string]string{
				"pattern": "other.md",
				"path":    tmpDir,
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "other.md")
			},
		},
		{
			name: "no matches found",
			params: map[string]string{
				"pattern": "*.xyz",
				"path":    tmpDir,
			},
			wantErr:     false,
			wantStatus:  tools.ToolExecutionStatusCompleted,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "No files found")
			},
		},
		{
			name: "non-existent directory",
			params: map[string]string{
				"pattern": "*.txt",
				"path":    filepath.Join(tmpDir, "nonexistent"),
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
		{
			name: "path is file not directory",
			params: map[string]string{
				"pattern": "*.txt",
				"path":    filepath.Join(tmpDir, "file1.txt"),
			},
			wantErr:    true,
			wantStatus: tools.ToolExecutionStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tool.Execute(ctx, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				if result != nil {
					assert.Equal(t, tt.wantStatus, result.Status)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantStatus, result.Status)
				if tt.checkOutput != nil {
					tt.checkOutput(t, result.OutputText)
				}
			}
		})
	}
}

// =============================================================================
// Test Validate
// =============================================================================

func TestFileTool_Validate(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		params    map[string]string
		wantErr   bool
		errContains string
	}{
		{
			name:     "read_file valid params",
			toolName: "read_file",
			params: map[string]string{
				"path": "/some/path.txt",
			},
			wantErr: false,
		},
		{
			name:     "read_file missing path",
			toolName: "read_file",
			params:   map[string]string{},
			wantErr:  true,
			errContains: "missing required parameter",
		},
		{
			name:     "write_file valid params",
			toolName: "write_file",
			params: map[string]string{
				"path":    "/some/path.txt",
				"content": "Hello",
			},
			wantErr: false,
		},
		{
			name:     "grep missing pattern",
			toolName: "grep",
			params: map[string]string{
				"path": "/some/path",
			},
			wantErr:  true,
			errContains: "grep pattern cannot be empty",
		},
		{
			name:     "grep missing path",
			toolName: "grep",
			params: map[string]string{
				"pattern": "test",
			},
			wantErr:  true,
			errContains: "missing required parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := tools.ToolDefinition{
				Name:        tt.toolName,
				DisplayName: strings.Title(tt.toolName),
				Type:        tools.ToolTypeFile,
				Description: "Test tool",
				Parameters: []tools.ToolParameter{
					{Name: "path", Type: tools.ParameterTypeFilePath, Required: true},
				},
				Enabled: true,
			}

			tool, err := NewFileTool(def)
			require.NoError(t, err)

			err = tool.Validate(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.errContains))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Test Stats
// =============================================================================

func TestFileTool_Stats(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := createTestFile(t, tmpDir, "test.txt", "Hello, World!")

	def := tools.ToolDefinition{
		Name:        "read_file",
		DisplayName: "Read File",
		Type:        tools.ToolTypeFile,
		Description: "Read a file",
		Parameters: []tools.ToolParameter{
			{Name: "path", Type: tools.ParameterTypeFilePath, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	ctx := context.Background()

	// Execute successfully
	_, err = tool.Execute(ctx, map[string]string{"path": testFile})
	require.NoError(t, err)

	// Check stats
	stats := tool.Stats()
	assert.Equal(t, int64(1), stats.TotalExecutions)
	assert.Equal(t, int64(1), stats.SuccessfulExecutions)
	assert.Equal(t, int64(0), stats.FailedExecutions)
	assert.NotNil(t, tool.LastUsed())

	// Execute with error
	_, err = tool.Execute(ctx, map[string]string{"path": "/nonexistent"})
	assert.Error(t, err)

	stats = tool.Stats()
	assert.Equal(t, int64(2), stats.TotalExecutions)
	assert.Equal(t, int64(1), stats.SuccessfulExecutions)
	assert.Equal(t, int64(1), stats.FailedExecutions)
}

// =============================================================================
// Test Definition
// =============================================================================

func TestFileTool_Definition(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "test_tool",
		DisplayName: "Test Tool",
		Type:        tools.ToolTypeFile,
		Description: "Test description",
		Parameters: []tools.ToolParameter{
			{Name: "path", Type: tools.ParameterTypeFilePath, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	retrievedDef := tool.Definition()
	assert.Equal(t, def.Name, retrievedDef.Name)
	assert.Equal(t, def.DisplayName, retrievedDef.DisplayName)
	assert.Equal(t, def.Type, retrievedDef.Type)
	assert.Equal(t, def.Description, retrievedDef.Description)
	assert.Equal(t, def.Enabled, retrievedDef.Enabled)
}

// =============================================================================
// Test FileToolFactory
// =============================================================================

func TestFileToolFactory(t *testing.T) {
	def := tools.ToolDefinition{
		Name:        "read_file",
		DisplayName: "Read File",
		Type:        tools.ToolTypeFile,
		Description: "Read a file",
		Enabled:     true,
	}

	tool, err := FileToolFactory(def)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify it implements the Tool interface
	assert.Implements(t, (*tools.Tool)(nil), tool)
}

// =============================================================================
// Test Concurrent Execution
// =============================================================================

func TestFileTool_ConcurrentExecution(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := createTestFile(t, tmpDir, "test.txt", "Hello, World!")

	def := tools.ToolDefinition{
		Name:        "read_file",
		DisplayName: "Read File",
		Type:        tools.ToolTypeFile,
		Description: "Read a file",
		Parameters: []tools.ToolParameter{
			{Name: "path", Type: tools.ParameterTypeFilePath, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(t, err)

	ctx := context.Background()
	params := map[string]string{"path": testFile}

	// Execute concurrently
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := tool.Execute(ctx, params)
			results <- err
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		assert.NoError(t, err)
	}

	// Verify stats
	stats := tool.Stats()
	assert.Equal(t, int64(numGoroutines), stats.TotalExecutions)
	assert.Equal(t, int64(numGoroutines), stats.SuccessfulExecutions)
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkFileTool_ReadFile(b *testing.B) {
	tmpDir := b.TempDir()
	testFile := createTestFile(&testing.T{}, tmpDir, "bench.txt", strings.Repeat("Hello, World!\n", 1000))

	def := tools.ToolDefinition{
		Name:        "read_file",
		DisplayName: "Read File",
		Type:        tools.ToolTypeFile,
		Description: "Read a file",
		Parameters: []tools.ToolParameter{
			{Name: "path", Type: tools.ParameterTypeFilePath, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(b, err)

	ctx := context.Background()
	params := map[string]string{"path": testFile}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tool.Execute(ctx, params)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFileTool_WriteFile(b *testing.B) {
	tmpDir := b.TempDir()

	def := tools.ToolDefinition{
		Name:        "write_file",
		DisplayName: "Write File",
		Type:        tools.ToolTypeFile,
		Description: "Write a file",
		Parameters: []tools.ToolParameter{
			{Name: "path", Type: tools.ParameterTypeFilePath, Required: true},
			{Name: "content", Type: tools.ParameterTypeString, Required: true},
		},
		Enabled: true,
	}

	tool, err := NewFileTool(def)
	require.NoError(b, err)

	ctx := context.Background()
	content := strings.Repeat("Hello, World!\n", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		params := map[string]string{
			"path":    filepath.Join(tmpDir, fmt.Sprintf("bench%d.txt", i)),
			"content": content,
		}
		_, err := tool.Execute(ctx, params)
		if err != nil {
			b.Fatal(err)
		}
	}
}
