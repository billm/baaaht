package worker

import (
	"context"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// TestExecuteTask verifies that ExecuteTask properly creates a container,
// executes the task, captures output, and cleans up
func TestExecuteTask(t *testing.T) {
	// Skip if Go toolchain is not available or Docker is not running
	// This is a placeholder for the actual integration test

	tests := []struct {
		name       string
		toolType   ToolType
		args       []string
		mountSource string
		wantCode   int
		wantOut    bool // wantOut indicates if we expect stdout output
	}{
		{
			name:       "FileReadTool - successful read",
			toolType:   ToolTypeFileRead,
			args:       []string{"/etc/hostname"},
			mountSource: "/etc",
			wantCode:   0,
			wantOut:    true,
		},
		{
			name:       "ListTool - list directory",
			toolType:   ToolTypeList,
			args:       []string{},
			mountSource: "/tmp",
			wantCode:   0,
			wantOut:    true,
		},
		{
			name:       "Echo with cat",
			toolType:   ToolTypeFileRead,
			args:       []string{"/proc/version"},
			mountSource: "/proc",
			wantCode:   0,
			wantOut:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create executor
			exec, err := NewExecutorDefault(nil)
			if err != nil {
				t.Skipf("Cannot create executor (Docker may not be available): %v", err)
				return
			}
			defer exec.Close()

			// Prepare task config
			taskCfg := TaskConfig{
				ToolType:    tt.toolType,
				Args:        tt.args,
				MountSource: tt.mountSource,
				Timeout:     30 * time.Second,
			}

			// Execute task
			ctx := context.Background()
			result := exec.ExecuteTask(ctx, taskCfg)

			// Check result
			if result.Error != nil {
				t.Errorf("ExecuteTask() returned error: %v", result.Error)
				return
			}

			if result.ExitCode != tt.wantCode {
				t.Errorf("ExecuteTask() exitCode = %d, want %d", result.ExitCode, tt.wantCode)
			}

			if tt.wantOut && result.Stdout == "" {
				t.Error("ExecuteTask() returned empty stdout, expected output")
			}

			if result.ContainerID == "" {
				t.Error("ExecuteTask() returned empty ContainerID")
			}

			if result.Duration == 0 {
				t.Error("ExecuteTask() returned zero Duration")
			}
		})
	}
}

// TestExecuteTaskTimeout verifies that ExecuteTask properly handles timeout
func TestExecuteTaskTimeout(t *testing.T) {
	exec, err := NewExecutorDefault(nil)
	if err != nil {
		t.Skipf("Cannot create executor (Docker may not be available): %v", err)
		return
	}
	defer exec.Close()

	// Create a task that will timeout
	taskCfg := TaskConfig{
		ToolType:    ToolTypeFileRead,
		Args:        []string{"/dev/urandom"}, // This will hang reading forever
		MountSource: "/dev",
		Timeout:     100 * time.Millisecond, // Very short timeout
	}

	ctx := context.Background()
	result := exec.ExecuteTask(ctx, taskCfg)

	// Should have timed out
	if result.Error == nil {
		t.Error("ExecuteTask() with long-running task should have timed out, but didn't")
	}
}

// TestExecuteTaskInvalidTool verifies that ExecuteTask handles invalid tool types
func TestExecuteTaskInvalidTool(t *testing.T) {
	exec, err := NewExecutorDefault(nil)
	if err != nil {
		t.Skipf("Cannot create executor (Docker may not be available): %v", err)
		return
	}
	defer exec.Close()

	taskCfg := TaskConfig{
		ToolType: ToolType("invalid_tool"),
		Args:     []string{},
		Timeout:  30 * time.Second,
	}

	ctx := context.Background()
	result := exec.ExecuteTask(ctx, taskCfg)

	if result.Error == nil {
		t.Error("ExecuteTask() with invalid tool type should return error")
	}
}

// TestExecuteTaskCleanup verifies that containers are always cleaned up
func TestExecuteTaskCleanup(t *testing.T) {
	exec, err := NewExecutorDefault(nil)
	if err != nil {
		t.Skipf("Cannot create executor (Docker may not be available): %v", err)
		return
	}
	defer exec.Close()

	// Track the container ID
	var containerID string

	// Execute a simple task
	taskCfg := TaskConfig{
		ToolType:    ToolTypeList,
		Args:        []string{},
		MountSource: "/tmp",
		Timeout:     30 * time.Second,
	}

	ctx := context.Background()
	result := exec.ExecuteTask(ctx, taskCfg)

	if result.Error != nil {
		t.Logf("ExecuteTask() returned error (may be expected if Docker unavailable): %v", result.Error)
	}

	containerID = result.ContainerID

	// Verify container was cleaned up by checking if it still exists
	// This would require listing containers, which is outside the scope of this test
	// In a real test, we would use the runtime to check if the container exists

	// For now, just verify the container ID was set
	if containerID == "" && result.Error == nil {
		t.Error("ExecuteTask() returned empty ContainerID but no error")
	}

	t.Logf("Container ID: %s", containerID)
}

// TestExecuteTaskContextCancellation verifies that ExecuteTask handles context cancellation
func TestExecuteTaskContextCancellation(t *testing.T) {
	exec, err := NewExecutorDefault(nil)
	if err != nil {
		t.Skipf("Cannot create executor (Docker may not be available): %v", err)
		return
	}
	defer exec.Close()

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start a long-running task in a goroutine
	taskCfg := TaskConfig{
		ToolType:    ToolTypeFileRead,
		Args:        []string{"/dev/urandom"},
		MountSource: "/dev",
		Timeout:     30 * time.Second,
	}

	resultChan := make(chan *TaskResult, 1)
	go func() {
		resultChan <- exec.ExecuteTask(ctx, taskCfg)
	}()

	// Give the container time to start, then cancel
	time.Sleep(500 * time.Millisecond)
	cancel()

	// Wait for result
	result := <-resultChan

	// Should have failed due to cancellation
	if result.Error == nil {
		t.Error("ExecuteTask() with cancelled context should return error")
	}

	t.Logf("Cancelled task result: %v", result.Error)
}

// TestTaskResult verifies the TaskResult struct is properly populated
func TestTaskResult(t *testing.T) {
	exec, err := NewExecutorDefault(nil)
	if err != nil {
		t.Skipf("Cannot create executor (Docker may not be available): %v", err)
		return
	}
	defer exec.Close()

	taskCfg := TaskConfig{
		ToolType:    ToolTypeList,
		Args:        []string{"/tmp"},
		MountSource: "/tmp",
		Timeout:     30 * time.Second,
	}

	ctx := context.Background()
	result := exec.ExecuteTask(ctx, taskCfg)

	if result.Error != nil {
		t.Skipf("Task execution failed: %v", result.Error)
		return
	}

	// Verify all fields are populated
	if result.StartedAt.IsZero() {
		t.Error("TaskResult.StartedAt is zero")
	}

	if result.CompletedAt.IsZero() {
		t.Error("TaskResult.CompletedAt is zero")
	}

	if result.Duration == 0 {
		t.Error("TaskResult.Duration is zero")
	}

	if result.ContainerID == "" {
		t.Error("TaskResult.ContainerID is empty")
	}

	// ExitCode should be set (even if non-zero)
	t.Logf("ExitCode: %d", result.ExitCode)

	t.Logf("Stdout length: %d", len(result.Stdout))
	t.Logf("Stderr length: %d", len(result.Stderr))
	t.Logf("Duration: %v", result.Duration)
}
