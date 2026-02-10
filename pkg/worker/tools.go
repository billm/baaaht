package worker

import (
	"fmt"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// ToolType represents the type of tool operation
type ToolType string

const (
	ToolTypeFileRead  ToolType = "file_read"
	ToolTypeFileWrite ToolType = "file_write"
	ToolTypeFileEdit  ToolType = "file_edit"
	ToolTypeGrep      ToolType = "grep"
	ToolTypeFind      ToolType = "find"
	ToolTypeList      ToolType = "list"
	ToolTypeWebSearch ToolType = "web_search"
	ToolTypeFetchURL  ToolType = "fetch_url"
)

// ToolSpec defines the specification for a tool container
type ToolSpec struct {
	// Type identifies the tool
	Type ToolType

	// Name is a human-readable name for the tool
	Name string

	// Description explains what the tool does
	Description string

	// Image is the container image to use
	Image string

	// Command is the command to run in the container
	Command []string

	// Args are additional arguments that will be appended to the command
	// These are typically filled in at runtime based on user input
	Args []string

	// WorkingDir is the working directory inside the container
	WorkingDir string

	// Env are environment variables to set in the container
	Env map[string]string

	// Mounts defines the filesystem mounts needed by this tool
	// The Source field may be a template that is filled in at runtime
	Mounts []types.Mount

	// Resources defines resource limits for the tool container
	Resources types.ResourceLimits

	// ReadOnlyRootfs indicates if the container rootfs should be read-only
	ReadOnlyRootfs bool

	// Timeout is the maximum duration to wait for the tool to complete
	Timeout time.Duration

	// NetworkMode determines if the tool needs network access
	NetworkMode string
}

// GetToolSpec returns the tool specification for a given tool type
func GetToolSpec(toolType ToolType) (*ToolSpec, error) {
	switch toolType {
	case ToolTypeFileRead:
		return FileReadTool(), nil
	case ToolTypeFileWrite:
		return FileWriteTool(), nil
	case ToolTypeFileEdit:
		return FileEditTool(), nil
	case ToolTypeGrep:
		return GrepTool(), nil
	case ToolTypeFind:
		return FindTool(), nil
	case ToolTypeList:
		return ListTool(), nil
	case ToolTypeWebSearch:
		return WebSearchTool(), nil
	case ToolTypeFetchURL:
		return FetchURLTool(), nil
	default:
		return nil, fmt.Errorf("unknown tool type: %s", toolType)
	}
}

// FileReadTool returns a tool specification for reading files
// Uses a lightweight Alpine image with cat command
func FileReadTool() *ToolSpec {
	return &ToolSpec{
		Type:        ToolTypeFileRead,
		Name:        "file_read",
		Description: "Read file contents from a mounted path",
		Image:       "alpine:3.19",
		Command:     []string{"cat"},
		WorkingDir:  "/workspace",
		Env:         make(map[string]string),
		Mounts: []types.Mount{
			{
				Type:     types.MountTypeBind,
				Source:   "", // Filled in at runtime
				Target:   "/workspace",
				ReadOnly: true,
			},
		},
		Resources: types.ResourceLimits{
			NanoCPUs:    100_000_000, // 0.1 CPU
			MemoryBytes: 64 * 1024 * 1024, // 64MB
			PidsLimit:   int64Ptr(10),
		},
		ReadOnlyRootfs: true,
		Timeout:        30 * time.Second,
		NetworkMode:    "none",
	}
}

// FileWriteTool returns a tool specification for writing files
// Uses Alpine image with tee command to write content
func FileWriteTool() *ToolSpec {
	return &ToolSpec{
		Type:        ToolTypeFileWrite,
		Name:        "file_write",
		Description: "Write content to a file",
		Image:       "alpine:3.19",
		Command:     []string{"sh", "-c"},
		Args:        []string{"mkdir -p $(dirname \"$1\") && tee \"$1\" > /dev/null", "--"},
		WorkingDir:  "/workspace",
		Env:         make(map[string]string),
		Mounts: []types.Mount{
			{
				Type:     types.MountTypeBind,
				Source:   "", // Filled in at runtime
				Target:   "/workspace",
				ReadOnly: false,
			},
		},
		Resources: types.ResourceLimits{
			NanoCPUs:    100_000_000, // 0.1 CPU
			MemoryBytes: 64 * 1024 * 1024, // 64MB
			PidsLimit:   int64Ptr(10),
		},
		ReadOnlyRootfs: false,
		Timeout:        30 * time.Second,
		NetworkMode:    "none",
	}
}

// FileEditTool returns a tool specification for editing files
// Uses Alpine image with sed command for text replacement
func FileEditTool() *ToolSpec {
	return &ToolSpec{
		Type:        ToolTypeFileEdit,
		Name:        "file_edit",
		Description: "Edit file contents using sed replacement",
		Image:       "alpine:3.19",
		Command:     []string{"sed"},
		Args:        []string{"-i"}, // Edit in-place
		WorkingDir:  "/workspace",
		Env:         make(map[string]string),
		Mounts: []types.Mount{
			{
				Type:     types.MountTypeBind,
				Source:   "", // Filled in at runtime
				Target:   "/workspace",
				ReadOnly: false,
			},
		},
		Resources: types.ResourceLimits{
			NanoCPUs:    100_000_000, // 0.1 CPU
			MemoryBytes: 64 * 1024 * 1024, // 64MB
			PidsLimit:   int64Ptr(10),
		},
		ReadOnlyRootfs: false,
		Timeout:        30 * time.Second,
		NetworkMode:    "none",
	}
}

// GrepTool returns a tool specification for searching file contents
// Uses Alpine image with grep command
func GrepTool() *ToolSpec {
	return &ToolSpec{
		Type:        ToolTypeGrep,
		Name:        "grep",
		Description: "Search for patterns in files using grep",
		Image:       "alpine:3.19",
		Command:     []string{"grep"},
		Args:        []string{"-r", "-n", "-I"}, // Recursive, line numbers, skip binaries
		WorkingDir:  "/workspace",
		Env:         make(map[string]string),
		Mounts: []types.Mount{
			{
				Type:     types.MountTypeBind,
				Source:   "", // Filled in at runtime
				Target:   "/workspace",
				ReadOnly: true,
			},
		},
		Resources: types.ResourceLimits{
			NanoCPUs:    200_000_000, // 0.2 CPU
			MemoryBytes: 128 * 1024 * 1024, // 128MB
			PidsLimit:   int64Ptr(20),
		},
		ReadOnlyRootfs: true,
		Timeout:        60 * time.Second,
		NetworkMode:    "none",
	}
}

// FindTool returns a tool specification for finding files
// Uses Alpine image with find command
func FindTool() *ToolSpec {
	return &ToolSpec{
		Type:        ToolTypeFind,
		Name:        "find",
		Description: "Find files by name, type, or other attributes",
		Image:       "alpine:3.19",
		Command:     []string{"find"},
		WorkingDir:  "/workspace",
		Env:         make(map[string]string),
		Mounts: []types.Mount{
			{
				Type:     types.MountTypeBind,
				Source:   "", // Filled in at runtime
				Target:   "/workspace",
				ReadOnly: true,
			},
		},
		Resources: types.ResourceLimits{
			NanoCPUs:    200_000_000, // 0.2 CPU
			MemoryBytes: 128 * 1024 * 1024, // 128MB
			PidsLimit:   int64Ptr(20),
		},
		ReadOnlyRootfs: true,
		Timeout:        60 * time.Second,
		NetworkMode:    "none",
	}
}

// ListTool returns a tool specification for listing directory contents
// Uses Alpine image with ls command
func ListTool() *ToolSpec {
	return &ToolSpec{
		Type:        ToolTypeList,
		Name:        "list",
		Description: "List directory contents",
		Image:       "alpine:3.19",
		Command:     []string{"ls", "-la"},
		WorkingDir:  "/workspace",
		Env:         make(map[string]string),
		Mounts: []types.Mount{
			{
				Type:     types.MountTypeBind,
				Source:   "", // Filled in at runtime
				Target:   "/workspace",
				ReadOnly: true,
			},
		},
		Resources: types.ResourceLimits{
			NanoCPUs:    100_000_000, // 0.1 CPU
			MemoryBytes: 64 * 1024 * 1024, // 64MB
			PidsLimit:   int64Ptr(10),
		},
		ReadOnlyRootfs: true,
		Timeout:        30 * time.Second,
		NetworkMode:    "none",
	}
}

// WebSearchTool returns a tool specification for web search operations
// Uses curlimage/alpine with curl for making HTTP requests
func WebSearchTool() *ToolSpec {
	return &ToolSpec{
		Type:        ToolTypeWebSearch,
		Name:        "web_search",
		Description: "Perform web search using curl",
		Image:       "curlimages/curl:8.6.0",
		Command:     []string{"curl"},
		Args:        []string{"-s", "-L", "-A", "Mozilla/5.0"},
		WorkingDir:  "/tmp",
		Env:         make(map[string]string),
		Mounts:      []types.Mount{}, // No mounts needed for web operations
		Resources: types.ResourceLimits{
			NanoCPUs:    200_000_000, // 0.2 CPU
			MemoryBytes: 128 * 1024 * 1024, // 128MB
			PidsLimit:   int64Ptr(10),
		},
		ReadOnlyRootfs: true,
		Timeout:        30 * time.Second,
		NetworkMode:    "", // Empty means default networking
	}
}

// FetchURLTool returns a tool specification for fetching URL content
// Uses curlimage/alpine with curl for fetching content
func FetchURLTool() *ToolSpec {
	return &ToolSpec{
		Type:        ToolTypeFetchURL,
		Name:        "fetch_url",
		Description: "Fetch content from a URL",
		Image:       "curlimages/curl:8.6.0",
		Command:     []string{"curl"},
		Args:        []string{"-s", "-L"},
		WorkingDir:  "/tmp",
		Env:         make(map[string]string),
		Mounts:      []types.Mount{}, // No mounts needed for web operations
		Resources: types.ResourceLimits{
			NanoCPUs:    200_000_000, // 0.2 CPU
			MemoryBytes: 128 * 1024 * 1024, // 128MB
			PidsLimit:   int64Ptr(10),
		},
		ReadOnlyRootfs: true,
		Timeout:        60 * time.Second,
		NetworkMode:    "", // Empty means default networking
	}
}

// ToContainerConfig converts a ToolSpec to a ContainerConfig
// The source paths in mounts must be filled in before calling this
func (t *ToolSpec) ToContainerConfig(sessionID types.ID, mountSource string) types.ContainerConfig {
	// Fill in the mount source
	mounts := make([]types.Mount, len(t.Mounts))
	for i, m := range t.Mounts {
		mounts[i] = m
		if mounts[i].Source == "" && mountSource != "" {
			mounts[i].Source = mountSource
		}
	}

	return types.ContainerConfig{
		Image:          t.Image,
		Command:        t.Command,
		Args:           t.Args,
		Env:            t.Env,
		WorkingDir:     t.WorkingDir,
		Mounts:         mounts,
		NetworkMode:    t.NetworkMode,
		Resources:      t.Resources,
		ReadOnlyRootfs: t.ReadOnlyRootfs,
		RemoveOnStop:   true, // Always remove tool containers after execution
		Labels: map[string]string{
			"baaaht.tool_type": string(t.Type),
			"baaaht.tool_name": t.Name,
			"baaaht.managed":   "true",
		},
	}
}

// Validate checks if the tool specification is valid
func (t *ToolSpec) Validate() error {
	if t.Type == "" {
		return fmt.Errorf("tool type is required")
	}
	if t.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if t.Image == "" {
		return fmt.Errorf("tool image is required")
	}
	if len(t.Command) == 0 {
		return fmt.Errorf("tool command is required")
	}
	if t.Timeout <= 0 {
		return fmt.Errorf("tool timeout must be positive")
	}
	return nil
}

// int64Ptr returns a pointer to an int64
func int64Ptr(i int64) *int64 {
	return &i
}

// AllToolSpecs returns all available tool specifications
func AllToolSpecs() []*ToolSpec {
	return []*ToolSpec{
		FileReadTool(),
		FileWriteTool(),
		FileEditTool(),
		GrepTool(),
		FindTool(),
		ListTool(),
		WebSearchTool(),
		FetchURLTool(),
	}
}
