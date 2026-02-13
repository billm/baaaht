package worker

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// TestToolSpecs verifies that all tool specifications have valid images, commands, and mount configurations
func TestToolSpecs(t *testing.T) {
	tests := []struct {
		name    string
		tool    ToolType
		wantImg bool   // wantImg indicates if image should be non-empty
		wantCmd bool   // wantCmd indicates if command should be non-empty
		wantNet string // wantNet is the expected network mode
	}{
		{
			name:    "FileReadTool",
			tool:    ToolTypeFileRead,
			wantImg: true,
			wantCmd: true,
			wantNet: "none",
		},
		{
			name:    "FileWriteTool",
			tool:    ToolTypeFileWrite,
			wantImg: true,
			wantCmd: true,
			wantNet: "none",
		},
		{
			name:    "FileEditTool",
			tool:    ToolTypeFileEdit,
			wantImg: true,
			wantCmd: true,
			wantNet: "none",
		},
		{
			name:    "GrepTool",
			tool:    ToolTypeGrep,
			wantImg: true,
			wantCmd: true,
			wantNet: "none",
		},
		{
			name:    "FindTool",
			tool:    ToolTypeFind,
			wantImg: true,
			wantCmd: true,
			wantNet: "none",
		},
		{
			name:    "ListTool",
			tool:    ToolTypeList,
			wantImg: true,
			wantCmd: true,
			wantNet: "none",
		},
		{
			name:    "WebSearchTool",
			tool:    ToolTypeWebSearch,
			wantImg: true,
			wantCmd: true,
			wantNet: "", // Empty means default networking
		},
		{
			name:    "FetchURLTool",
			tool:    ToolTypeFetchURL,
			wantImg: true,
			wantCmd: true,
			wantNet: "", // Empty means default networking
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := GetToolSpec(tt.tool)
			if err != nil {
				t.Fatalf("GetToolSpec(%v) failed: %v", tt.tool, err)
			}

			// Validate the spec
			if err := spec.Validate(); err != nil {
				t.Errorf("ToolSpec.Validate() failed: %v", err)
			}

			// Check image
			if tt.wantImg && spec.Image == "" {
				t.Error("ToolSpec.Image is empty")
			}

			// Check command
			if tt.wantCmd && len(spec.Command) == 0 {
				t.Error("ToolSpec.Command is empty")
			}

			// Check network mode
			if spec.NetworkMode != tt.wantNet {
				t.Errorf("ToolSpec.NetworkMode = %q, want %q", spec.NetworkMode, tt.wantNet)
			}

			// Check timeout is set
			if spec.Timeout == 0 {
				t.Error("ToolSpec.Timeout is not set")
			}

			// Check mounts have valid targets (source can be empty as it's filled at runtime)
			for i, m := range spec.Mounts {
				if m.Target == "" {
					t.Errorf("ToolSpec.Mounts[%d].Target is empty", i)
				}
				if m.Type == "" {
					t.Errorf("ToolSpec.Mounts[%d].Type is empty", i)
				}
			}
		})
	}
}

// TestGetToolSpecInvalid verifies that GetToolSpec returns an error for unknown tool types
func TestGetToolSpecInvalid(t *testing.T) {
	_, err := GetToolSpec(ToolType("invalid"))
	if err == nil {
		t.Error("GetToolSpec() should return error for unknown tool type")
	}
}

// TestToolSpecToContainerConfig verifies that ToolSpec can be converted to ContainerConfig
func TestToolSpecToContainerConfig(t *testing.T) {
	spec := FileReadTool()
	sessionID := types.GenerateID()
	mountSource := "/tmp/test"

	config := spec.ToContainerConfig(sessionID, mountSource)

	// Check that fields are properly transferred
	if config.Image != spec.Image {
		t.Errorf("ToContainerConfig() Image = %q, want %q", config.Image, spec.Image)
	}

	if len(config.Command) != len(spec.Command) {
		t.Errorf("ToContainerConfig() Command length = %d, want %d", len(config.Command), len(spec.Command))
	}

	if config.WorkingDir != spec.WorkingDir {
		t.Errorf("ToContainerConfig() WorkingDir = %q, want %q", config.WorkingDir, spec.WorkingDir)
	}

	if config.NetworkMode != spec.NetworkMode {
		t.Errorf("ToContainerConfig() NetworkMode = %q, want %q", config.NetworkMode, spec.NetworkMode)
	}

	if !config.RemoveOnStop {
		t.Error("ToContainerConfig() RemoveOnStop should be true for tool containers")
	}

	// Check that mount source is filled in
	if len(config.Mounts) > 0 && len(spec.Mounts) > 0 {
		if config.Mounts[0].Source != mountSource {
			t.Errorf("ToContainerConfig() Mounts[0].Source = %q, want %q", config.Mounts[0].Source, mountSource)
		}
	}

	// Check labels
	if config.Labels["baaaht.tool_type"] != string(spec.Type) {
		t.Error("ToContainerConfig() tool_type label not set correctly")
	}
	if config.Labels["baaaht.tool_name"] != spec.Name {
		t.Error("ToContainerConfig() tool_name label not set correctly")
	}
	if config.Labels["baaaht.managed"] != "true" {
		t.Error("ToContainerConfig() managed label not set correctly")
	}
}

// TestToolSpecResourceLimits verifies that all tool specs have resource limits configured
func TestToolSpecResourceLimits(t *testing.T) {
	specs := AllToolSpecs()

	for _, spec := range specs {
		t.Run(spec.Name, func(t *testing.T) {
			if spec.Resources.NanoCPUs == 0 {
				t.Error("ToolSpec.Resources.NanoCPUs is not set")
			}
			if spec.Resources.MemoryBytes == 0 {
				t.Error("ToolSpec.Resources.MemoryBytes is not set")
			}
			if spec.Resources.PidsLimit == nil {
				t.Error("ToolSpec.Resources.PidsLimit is not set")
			}
		})
	}
}

// TestToolSpecReadOnlyFileOperations verifies that file read operations use read-only mounts
func TestToolSpecReadOnlyFileOperations(t *testing.T) {
	readOnlyTools := []ToolType{
		ToolTypeFileRead,
		ToolTypeGrep,
		ToolTypeFind,
		ToolTypeList,
	}

	for _, toolType := range readOnlyTools {
		t.Run(string(toolType), func(t *testing.T) {
			spec, err := GetToolSpec(toolType)
			if err != nil {
				t.Fatalf("GetToolSpec(%v) failed: %v", toolType, err)
			}

			if !spec.ReadOnlyRootfs {
				t.Errorf("ToolSpec.ReadOnlyRootfs should be true for %v", toolType)
			}

			for i, m := range spec.Mounts {
				if !m.ReadOnly {
					t.Errorf("ToolSpec.Mounts[%d].ReadOnly should be true for %v", i, toolType)
				}
			}
		})
	}
}

// TestToolSpecWriteOperations verifies that file write operations use writable mounts
func TestToolSpecWriteOperations(t *testing.T) {
	writeTools := []ToolType{
		ToolTypeFileWrite,
		ToolTypeFileEdit,
	}

	for _, toolType := range writeTools {
		t.Run(string(toolType), func(t *testing.T) {
			spec, err := GetToolSpec(toolType)
			if err != nil {
				t.Fatalf("GetToolSpec(%v) failed: %v", toolType, err)
			}

			// Write operations should have writable mounts
			hasWritableMount := false
			for _, m := range spec.Mounts {
				if !m.ReadOnly {
					hasWritableMount = true
					break
				}
			}

			if !hasWritableMount {
				t.Errorf("ToolSpec should have at least one writable mount for %v", toolType)
			}
		})
	}
}

// TestToolSpecWebOperations verifies that web operations don't require mounts
func TestToolSpecWebOperations(t *testing.T) {
	webTools := []ToolType{
		ToolTypeWebSearch,
		ToolTypeFetchURL,
	}

	for _, toolType := range webTools {
		t.Run(string(toolType), func(t *testing.T) {
			spec, err := GetToolSpec(toolType)
			if err != nil {
				t.Fatalf("GetToolSpec(%v) failed: %v", toolType, err)
			}

			if len(spec.Mounts) != 0 {
				t.Errorf("ToolSpec.Mounts should be empty for %v (web operation)", toolType)
			}
		})
	}
}

// TestAllToolSpecs covers all tool types
func TestAllToolSpecs(t *testing.T) {
	specs := AllToolSpecs()

	// Verify we have specs for all tool types
	expectedTools := map[ToolType]bool{
		ToolTypeFileRead:  false,
		ToolTypeFileWrite: false,
		ToolTypeFileEdit:  false,
		ToolTypeGrep:      false,
		ToolTypeFind:      false,
		ToolTypeList:      false,
		ToolTypeWebSearch: false,
		ToolTypeFetchURL:  false,
	}

	for _, spec := range specs {
		if found, ok := expectedTools[spec.Type]; !ok || found {
			t.Errorf("Unexpected or duplicate tool type: %v", spec.Type)
		}
		expectedTools[spec.Type] = true
	}

	// Check all tools are present
	for toolType, found := range expectedTools {
		if !found {
			t.Errorf("Missing tool spec for: %v", toolType)
		}
	}
}

// TestFileRead verifies that FileRead function can read file contents
// from a containerized cat command
func TestFileRead(t *testing.T) {
	// This test requires Docker to be available
	t.Run("reads file content successfully", func(t *testing.T) {
		// Create a temporary file to read
		tmpDir := t.TempDir()
		testFile := tmpDir + "/test.txt"
		testContent := "Hello, World!\nThis is a test file."

		// Write test content to the file
		if err := writeTestFile(testFile, testContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Create executor
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		// Read the file using FileRead function
		ctx := context.Background()
		content, err := FileRead(ctx, exec, tmpDir, "test.txt")
		if err != nil {
			t.Fatalf("FileRead() failed: %v", err)
		}

		// Verify content matches
		if content != testContent {
			t.Errorf("FileRead() content = %q, want %q", content, testContent)
		}
	})

	t.Run("reads file with absolute workspace path", func(t *testing.T) {
		// Create a temporary file to read
		tmpDir := t.TempDir()
		testFile := tmpDir + "/test.txt"
		testContent := "Absolute path test"

		if err := writeTestFile(testFile, testContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		// Read using absolute workspace path
		ctx := context.Background()
		content, err := FileRead(ctx, exec, tmpDir, "/workspace/test.txt")
		if err != nil {
			t.Fatalf("FileRead() with absolute path failed: %v", err)
		}

		if content != testContent {
			t.Errorf("FileRead() content = %q, want %q", content, testContent)
		}
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		tmpDir := t.TempDir()

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		_, err = FileRead(ctx, exec, tmpDir, "nonexistent.txt")
		if err == nil {
			t.Error("FileRead() should return error for non-existent file")
		}
	})

	t.Run("handles binary files gracefully", func(t *testing.T) {
		// Create a temporary binary file
		tmpDir := t.TempDir()
		testFile := tmpDir + "/binary.dat"
		testContent := string([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD})

		if err := writeTestFile(testFile, testContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		content, err := FileRead(ctx, exec, tmpDir, "binary.dat")
		if err != nil {
			t.Fatalf("FileRead() failed for binary file: %v", err)
		}

		// Binary content should be preserved (though cat may have issues with null bytes)
		if len(content) == 0 {
			t.Error("FileRead() returned empty content for binary file")
		}
	})

	t.Run("validates inputs", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()

		// Test nil executor
		_, err = FileRead(ctx, nil, "/tmp", "test.txt")
		if err == nil {
			t.Error("FileRead() should return error for nil executor")
		}

		// Test empty mount source
		_, err = FileRead(ctx, exec, "", "test.txt")
		if err == nil {
			t.Error("FileRead() should return error for empty mount source")
		}

		// Test empty file path
		_, err = FileRead(ctx, exec, "/tmp", "")
		if err == nil {
			t.Error("FileRead() should return error for empty file path")
		}
	})
}

// TestFileWrite verifies that FileWrite function can write content to files
// using a containerized write command
func TestFileWrite(t *testing.T) {
	t.Run("writes file content successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := "test.txt"
		testContent := "Hello, World!\nThis is a test file."

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		// Write the file using FileWrite function
		ctx := context.Background()
		if err := FileWrite(ctx, exec, tmpDir, testFile, testContent); err != nil {
			t.Fatalf("FileWrite() failed: %v", err)
		}

		// Verify content was written by reading it back
		content, err := os.ReadFile(tmpDir + "/" + testFile)
		if err != nil {
			t.Fatalf("Failed to read written file: %v", err)
		}

		if string(content) != testContent {
			t.Errorf("FileWrite() content = %q, want %q", string(content), testContent)
		}
	})

	t.Run("writes file with absolute workspace path", func(t *testing.T) {
		tmpDir := t.TempDir()
		testContent := "Absolute path test"

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		// Write using absolute workspace path
		ctx := context.Background()
		if err := FileWrite(ctx, exec, tmpDir, "/workspace/test.txt", testContent); err != nil {
			t.Fatalf("FileWrite() with absolute path failed: %v", err)
		}

		// Verify content
		content, err := os.ReadFile(tmpDir + "/test.txt")
		if err != nil {
			t.Fatalf("Failed to read written file: %v", err)
		}

		if string(content) != testContent {
			t.Errorf("FileWrite() content = %q, want %q", string(content), testContent)
		}
	})

	t.Run("creates parent directories if needed", func(t *testing.T) {
		tmpDir := t.TempDir()
		testContent := "Nested file content"
		targetPath := tmpDir + "/subdir/nested/file.txt"

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		// Write to a nested path that doesn't exist
		ctx := context.Background()
		if err := FileWrite(ctx, exec, tmpDir, "subdir/nested/file.txt", testContent); err != nil {
			t.Fatalf("FileWrite() to nested path failed: %v", err)
		}

		// Verify content (allow brief delay for CI filesystems)
		deadline := time.Now().Add(2 * time.Second)
		for {
			content, err := os.ReadFile(targetPath)
			if err == nil && string(content) == testContent {
				break
			}

			if time.Now().After(deadline) {
				if err != nil {
					t.Fatalf("Failed to read written file: %v", err)
				}
				t.Errorf("FileWrite() content = %q, want %q", string(content), testContent)
				break
			}

			time.Sleep(25 * time.Millisecond)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := "overwrite.txt"

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()

		// Write initial content
		initialContent := "Initial content"
		if err := FileWrite(ctx, exec, tmpDir, testFile, initialContent); err != nil {
			t.Fatalf("FileWrite() initial write failed: %v", err)
		}

		// Overwrite with new content
		newContent := "New content"
		if err := FileWrite(ctx, exec, tmpDir, testFile, newContent); err != nil {
			t.Fatalf("FileWrite() overwrite failed: %v", err)
		}

		// Verify new content
		content, err := os.ReadFile(tmpDir + "/" + testFile)
		if err != nil {
			t.Fatalf("Failed to read written file: %v", err)
		}

		if string(content) != newContent {
			t.Errorf("FileWrite() content = %q, want %q", string(content), newContent)
		}
	})

	t.Run("handles special characters in content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := "special.txt"
		testContent := "Content with $pecial <characters> & symbols\n\"quotes\" and 'apostrophes'\nBackticks: `cmd`"

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		if err := FileWrite(ctx, exec, tmpDir, testFile, testContent); err != nil {
			t.Fatalf("FileWrite() with special characters failed: %v", err)
		}

		// Verify content
		content, err := os.ReadFile(tmpDir + "/" + testFile)
		if err != nil {
			t.Fatalf("Failed to read written file: %v", err)
		}

		if string(content) != testContent {
			t.Errorf("FileWrite() content = %q, want %q", string(content), testContent)
		}
	})

	t.Run("validates inputs", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()

		// Test nil executor
		err = FileWrite(ctx, nil, "/tmp", "test.txt", "content")
		if err == nil {
			t.Error("FileWrite() should return error for nil executor")
		}

		// Test empty mount source
		err = FileWrite(ctx, exec, "", "test.txt", "content")
		if err == nil {
			t.Error("FileWrite() should return error for empty mount source")
		}

		// Test empty file path
		err = FileWrite(ctx, exec, "/tmp", "", "content")
		if err == nil {
			t.Error("FileWrite() should return error for empty file path")
		}
	})
}

// TestWebSearch verifies that WebSearch function can perform HTTP requests
// using a containerized curl command
func TestWebSearch(t *testing.T) {
	t.Run("fetches example.com successfully", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		content, err := WebSearch(ctx, exec, "https://example.com")
		if err != nil {
			t.Fatalf("WebSearch() failed: %v", err)
		}

		// Verify content contains expected HTML elements
		if !strings.Contains(content, "<html") && !strings.Contains(content, "<HTML") {
			t.Error("WebSearch() response should contain HTML content")
		}

		// Should contain some content from example.com
		if len(content) < 100 {
			t.Errorf("WebSearch() response seems too short: %d bytes", len(content))
		}
	})

	t.Run("fetches HTTP content with redirect", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		// This URL redirects (e.g., http to https)
		content, err := WebSearch(ctx, exec, "http://example.com")
		if err != nil {
			t.Fatalf("WebSearch() with redirect failed: %v", err)
		}

		// Should successfully follow redirect and get content
		if len(content) < 100 {
			t.Errorf("WebSearch() response seems too short after redirect: %d bytes", len(content))
		}
	})

	t.Run("validates inputs", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()

		// Test nil executor
		_, err = WebSearch(ctx, nil, "https://example.com")
		if err == nil {
			t.Error("WebSearch() should return error for nil executor")
		}

		// Test empty URL
		_, err = WebSearch(ctx, exec, "")
		if err == nil {
			t.Error("WebSearch() should return error for empty URL")
		}
	})

	t.Run("handles invalid URL gracefully", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		// Use an invalid URL that should fail
		_, err = WebSearch(ctx, exec, "http://this-domain-does-not-exist-12345.com")
		if err == nil {
			t.Error("WebSearch() should return error for invalid URL")
		}
	})
}

// TestFetchURL verifies that FetchURL function can fetch URL content
// using a containerized curl command
func TestFetchURL(t *testing.T) {
	t.Run("fetches simple URL successfully", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		content, err := FetchURL(ctx, exec, "https://example.com")
		if err != nil {
			t.Fatalf("FetchURL() failed: %v", err)
		}

		// Verify content contains expected HTML elements
		if !strings.Contains(content, "<html") && !strings.Contains(content, "<HTML") {
			t.Error("FetchURL() response should contain HTML content")
		}

		// Should contain some content from example.com
		if len(content) < 100 {
			t.Errorf("FetchURL() response seems too short: %d bytes", len(content))
		}
	})

	t.Run("fetches JSON content", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		// Use a simple JSON API
		content, err := FetchURL(ctx, exec, "https://httpbin.org/json")
		if err != nil {
			t.Fatalf("FetchURL() for JSON failed: %v", err)
		}

		if len(strings.TrimSpace(content)) == 0 {
			t.Error("FetchURL() response should not be empty")
		}
	})

	t.Run("validates inputs", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()

		// Test nil executor
		_, err = FetchURL(ctx, nil, "https://example.com")
		if err == nil {
			t.Error("FetchURL() should return error for nil executor")
		}

		// Test empty URL
		_, err = FetchURL(ctx, exec, "")
		if err == nil {
			t.Error("FetchURL() should return error for empty URL")
		}
	})

	t.Run("handles network errors gracefully", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		// Use an invalid URL that should fail
		_, err = FetchURL(ctx, exec, "http://this-domain-does-not-exist-12345.com")
		if err == nil {
			t.Error("FetchURL() should return error for invalid URL")
		}
	})
}

// writeTestFile is a helper function to write test content to a file
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// TestGrepAndFind verifies that grep and find tools work correctly
// This test requires Docker to be available (run with -requires_docker flag)
func TestGrepAndFind(t *testing.T) {
	t.Run("grep finds patterns in files", func(t *testing.T) {
		// Create temporary directory with test files
		tmpDir := t.TempDir()

		// Create test files with content
		testFiles := map[string]string{
			"file1.txt": "Hello World\nThis is a test\nAnother line",
			"file2.txt": "Hello Universe\nTesting is fun\nHello again",
			"subdir/file3.txt": "Hello Subdirectory\nNested content",
		}

		for path, content := range testFiles {
			fullPath := tmpDir + "/" + path
			dir := tmpDir + "/" + path[:len(path)-len(path[strings.LastIndex(path, "/")+1:])]
			if strings.Contains(path, "/") {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
			}
			if err := writeTestFile(fullPath, content); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		// Create executor
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		// Test grep for "Hello" pattern
		ctx := context.Background()
		taskCfg := TaskConfig{
			ToolType:    ToolTypeGrep,
			Args:        []string{"Hello", "."},
			MountSource: tmpDir,
		}

		result := exec.ExecuteTask(ctx, taskCfg)
		if result.Error != nil {
			t.Fatalf("Grep execution failed: %v", result.Error)
		}

		// Check that grep found the pattern
		if result.ExitCode != 0 {
			t.Errorf("Grep exit code = %d, want 0. Stderr: %s", result.ExitCode, result.Stderr)
		}

		// Verify output contains "Hello" matches
		output := result.Stdout
		if !strings.Contains(output, "Hello") {
			t.Errorf("Grep output should contain 'Hello', got: %s", output)
		}

		// Should have found at least 4 occurrences of "Hello"
		helloCount := strings.Count(output, "Hello")
		if helloCount < 4 {
			t.Errorf("Expected at least 4 'Hello' occurrences, got %d. Output: %s", helloCount, output)
		}
	})

	t.Run("grep with case insensitive flag", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test file with mixed case
		testContent := "hello HELLO HeLLo hElLo World"
		testFile := tmpDir + "/case_test.txt"
		if err := writeTestFile(testFile, testContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()

		// Test case-insensitive grep with -i flag
		taskCfg := TaskConfig{
			ToolType:    ToolTypeGrep,
			Args:        []string{"-i", "hello", "."},
			MountSource: tmpDir,
		}

		result := exec.ExecuteTask(ctx, taskCfg)
		if result.Error != nil {
			t.Fatalf("Grep execution failed: %v", result.Error)
		}

		// Should find all case variations
		output := result.Stdout
		if !strings.Contains(strings.ToLower(output), "hello") {
			t.Errorf("Case-insensitive grep should find 'hello' in any case. Output: %s", output)
		}
	})

	t.Run("find lists files recursively", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create nested directory structure
		testFiles := []string{
			"root.txt",
			"level1/file1.txt",
			"level1/level2/file2.txt",
			"level1/level2/level3/file3.txt",
			"another.txt",
		}

		for _, path := range testFiles {
			fullPath := tmpDir + "/" + path
			dir := tmpDir + "/" + path[:len(path)-len(path[strings.LastIndex(path, "/")+1:])]
			if strings.Contains(path, "/") {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
			}
			if err := writeTestFile(fullPath, "test content"); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		// Test find to list all .txt files recursively
		ctx := context.Background()
		taskCfg := TaskConfig{
			ToolType:    ToolTypeFind,
			Args:        []string{".", "-name", "*.txt"},
			MountSource: tmpDir,
		}

		result := exec.ExecuteTask(ctx, taskCfg)
		if result.Error != nil {
			t.Fatalf("Find execution failed: %v", result.Error)
		}

		// Check that find succeeded
		if result.ExitCode != 0 {
			t.Errorf("Find exit code = %d, want 0. Stderr: %s", result.ExitCode, result.Stderr)
		}

		// Verify output contains expected files
		output := result.Stdout
		expectedFiles := []string{"root.txt", "file1.txt", "file2.txt", "file3.txt", "another.txt"}

		for _, expectedFile := range expectedFiles {
			if !strings.Contains(output, expectedFile) {
				t.Errorf("Find output should contain '%s'. Output: %s", expectedFile, output)
			}
		}
	})

	t.Run("find with directory filter", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create files and directories
		if err := os.MkdirAll(tmpDir+"/dir1", 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.MkdirAll(tmpDir+"/dir2", 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := writeTestFile(tmpDir+"/dir1/file.txt", "content"); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
		if err := writeTestFile(tmpDir+"/dir2/file.txt", "content"); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		// Find files only in dir1
		ctx := context.Background()
		taskCfg := TaskConfig{
			ToolType:    ToolTypeFind,
			Args:        []string{"./dir1", "-type", "f"},
			MountSource: tmpDir,
		}

		result := exec.ExecuteTask(ctx, taskCfg)
		if result.Error != nil {
			t.Fatalf("Find execution failed: %v", result.Error)
		}

		output := result.Stdout

		// Should contain dir1/file.txt
		if !strings.Contains(output, "dir1/file.txt") || strings.Contains(output, "dir2/file.txt") {
			t.Errorf("Find should only list files in dir1. Output: %s", output)
		}
	})

	t.Run("grep handles no match gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a file without the pattern we'll search for
		testContent := "This is some content\nWithout the target pattern"
		if err := writeTestFile(tmpDir+"/test.txt", testContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()

		// Search for a pattern that doesn't exist
		taskCfg := TaskConfig{
			ToolType:    ToolTypeGrep,
			Args:        []string{"-r", "-n", "-I", "NonExistentPattern12345"},
			MountSource: tmpDir,
		}

		result := exec.ExecuteTask(ctx, taskCfg)
		if result.Error != nil {
			t.Fatalf("Grep execution failed: %v", result.Error)
		}

		// Grep returns exit code 1 when no matches found
		if result.ExitCode != 1 {
			t.Logf("Grep exit code for no match = %d (expected 1)", result.ExitCode)
		}

		// Stdout should be empty when no matches
		if result.Stdout != "" {
			t.Errorf("Grep stdout should be empty when no matches, got: %s", result.Stdout)
		}
	})
}

// TestListFiles verifies that ListFiles function can list directory contents
// using a containerized ls command
func TestListFiles(t *testing.T) {
	t.Run("lists current directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create some test files
		testFiles := []string{"file1.txt", "file2.txt", "subdir"}
		for _, name := range testFiles {
			path := tmpDir + "/" + name
			if name == "subdir" {
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
			} else {
				if err := writeTestFile(path, "test content"); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		output, err := ListFiles(ctx, exec, tmpDir, "", false)
		if err != nil {
			t.Fatalf("ListFiles() failed: %v", err)
		}

		// Verify output contains expected file names
		for _, name := range testFiles {
			if !strings.Contains(output, name) {
				t.Errorf("ListFiles() output should contain '%s'. Output: %s", name, output)
			}
		}
	})

	t.Run("lists specific directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a subdirectory with files
		subDir := tmpDir + "/testdir"
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		testFiles := []string{"a.txt", "b.txt"}
		for _, name := range testFiles {
			if err := writeTestFile(subDir+"/"+name, "content"); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		output, err := ListFiles(ctx, exec, tmpDir, "testdir", false)
		if err != nil {
			t.Fatalf("ListFiles() failed: %v", err)
		}

		// Verify output contains the files in testdir
		for _, name := range testFiles {
			if !strings.Contains(output, name) {
				t.Errorf("ListFiles() output should contain '%s'. Output: %s", name, output)
			}
		}
	})

	t.Run("lists with relative workspace path", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a test file
		if err := writeTestFile(tmpDir+"/workspace_test.txt", "content"); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		output, err := ListFiles(ctx, exec, tmpDir, ".", false)
		if err != nil {
			t.Fatalf("ListFiles() failed: %v", err)
		}

		// Verify output contains the test file
		if !strings.Contains(output, "workspace_test.txt") {
			t.Errorf("ListFiles() output should contain 'workspace_test.txt'. Output: %s", output)
		}
	})

	t.Run("lists with absolute workspace path", func(t *testing.T) {
		tmpDir := t.TempDir()

		if err := writeTestFile(tmpDir+"/absolute_test.txt", "content"); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		output, err := ListFiles(ctx, exec, tmpDir, "/workspace", false)
		if err != nil {
			t.Fatalf("ListFiles() failed: %v", err)
		}

		if !strings.Contains(output, "absolute_test.txt") {
			t.Errorf("ListFiles() output should contain 'absolute_test.txt'. Output: %s", output)
		}
	})

	t.Run("lists recursively", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create nested directory structure
		if err := os.MkdirAll(tmpDir+"/level1/level2", 0755); err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		if err := writeTestFile(tmpDir+"/root.txt", "root"); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if err := writeTestFile(tmpDir+"/level1/file1.txt", "level1"); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if err := writeTestFile(tmpDir+"/level1/level2/file2.txt", "level2"); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		output, err := ListFiles(ctx, exec, tmpDir, "", true)
		if err != nil {
			t.Fatalf("ListFiles() recursive failed: %v", err)
		}

		// Verify output contains all nested files
		expectedFiles := []string{"root.txt", "file1.txt", "file2.txt"}
		for _, name := range expectedFiles {
			if !strings.Contains(output, name) {
				t.Errorf("Recursive ListFiles() output should contain '%s'. Output: %s", name, output)
			}
		}

		// Recursive output should show directory structure
		if !strings.Contains(output, "level1") {
			t.Errorf("Recursive ListFiles() output should contain 'level1' directory. Output: %s", output)
		}
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		_, err = ListFiles(ctx, exec, tmpDir, "nonexistent_dir", false)
		if err == nil {
			t.Error("ListFiles() should return error for non-existent directory")
		}
	})

	t.Run("validates inputs", func(t *testing.T) {
		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()

		// Test nil executor
		_, err = ListFiles(ctx, nil, "/tmp", "", false)
		if err == nil {
			t.Error("ListFiles() should return error for nil executor")
		}

		// Test empty mount source
		_, err = ListFiles(ctx, exec, "", "", false)
		if err == nil {
			t.Error("ListFiles() should return error for empty mount source")
		}
	})

	t.Run("handles empty directory gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()

		exec, err := NewExecutorDefault(nil)
		if err != nil {
			t.Skipf("Cannot create executor (Docker may not be available): %v", err)
			return
		}
		defer exec.Close()

		ctx := context.Background()
		output, err := ListFiles(ctx, exec, tmpDir, "", false)
		if err != nil {
			t.Fatalf("ListFiles() failed for empty directory: %v", err)
		}

		// Should return some output (ls -la always shows at least . and ..)
		if output == "" {
			t.Error("ListFiles() should return output even for empty directory")
		}
	})
}
