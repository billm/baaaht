package worker

import (
	"testing"

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
