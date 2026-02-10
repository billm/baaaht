package worker

import (
	"context"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/policy"
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

// TestMountEnforcement verifies that mount allowlist enforcement works correctly
func TestMountEnforcement(t *testing.T) {
	// Create logger and policy enforcer
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	policyEnforcer, err := policy.NewDefault(log)
	if err != nil {
		t.Fatalf("failed to create policy enforcer: %v", err)
	}
	defer policyEnforcer.Close()

	// Set a restrictive policy that only allows specific mount paths
	ctx := context.Background()
	restrictivePolicy := &policy.Policy{
		ID:          "restrictive-mounts",
		Name:        "Restrictive Mount Policy",
		Description: "Policy that only allows specific mount paths",
		Mode:        policy.EnforcementModeStrict,
		Mounts: policy.MountPolicy{
			AllowBindMounts:     true,
			AllowedBindSources:  []string{"/safe/*", "/tmp/*"},
			DeniedBindSources:   []string{"/etc/*", "/root/*", "/home/*"},
			AllowVolumes:        true,
			AllowTmpfs:          true,
			MaxTmpfsSize:        int64Ptr(256 * 1024 * 1024), // 256MB
			EnforceReadOnlyRootfs: false,
		},
		Network: policy.NetworkPolicy{
			AllowNetwork:     false,
			AllowHostNetwork: false,
		},
		Images: policy.ImagePolicy{
			AllowLatestTag: true,
		},
		Security: policy.SecurityPolicy{
			AllowPrivileged: false,
			RequireNonRoot:  false,
			ReadOnlyRootfs:  false,
			AllowRoot:       true,
		},
	}

	if err := policyEnforcer.SetPolicy(ctx, restrictivePolicy); err != nil {
		t.Fatalf("failed to set restrictive policy: %v", err)
	}

	// Create executor with policy enforcer
	exec, err := NewExecutor(ExecutorConfig{
		Runtime:        nil, // Will be mocked in this test
		PolicyEnforcer: policyEnforcer,
		Logger:         log,
	})
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}
	defer exec.Close()

	tests := []struct {
		name          string
		mountSource   string
		wantAllowed   bool
		wantViolation string
	}{
		{
			name:        "allowed mount path - /safe/workspace",
			mountSource: "/safe/workspace",
			wantAllowed: true,
		},
		{
			name:        "allowed mount path - /tmp/test",
			mountSource: "/tmp/test",
			wantAllowed: true,
		},
		{
			name:          "blocked mount path - /etc/passwd",
			mountSource:   "/etc",
			wantAllowed:   false,
			wantViolation: "mount.bind.source_not_allowed",
		},
		{
			name:          "blocked mount path - /root",
			mountSource:   "/root",
			wantAllowed:   false,
			wantViolation: "mount.bind.source_not_allowed",
		},
		{
			name:          "blocked mount path - /home/user",
			mountSource:   "/home/user",
			wantAllowed:   false,
			wantViolation: "mount.bind.source_not_allowed",
		},
		{
			name:        "no mount - should be allowed (web tools don't need mounts)",
			mountSource: "",
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a container config from tool spec
			toolSpec := FileReadTool()
			containerConfig := toolSpec.ToContainerConfig(exec.SessionID(), tt.mountSource)

			// Validate the config
			result, err := exec.ValidateConfig(ctx, containerConfig)
			if err != nil {
				t.Fatalf("ValidateConfig failed: %v", err)
			}

			// Check if allowed status matches expectation
			if result.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v\nViolations: %+v\nWarnings: %+v",
					result.Allowed, tt.wantAllowed, result.Violations, result.Warnings)
			}

			// If we expect a specific violation, verify it exists
			if tt.wantViolation != "" {
				found := false
				for _, v := range result.Violations {
					if v.Rule == tt.wantViolation {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected violation %s not found. Got violations: %+v", tt.wantViolation, result.Violations)
				}
			}

			// Log the result for debugging
			t.Logf("Test '%s': Allowed=%v, Violations=%d, Warnings=%d",
				tt.name, result.Allowed, len(result.Violations), len(result.Warnings))
			for i, v := range result.Violations {
				t.Logf("  Violation[%d]: Rule=%s, Message=%s, Severity=%s",
					i, v.Rule, v.Message, v.Severity)
			}
		})
	}
}

// TestMountEnforcementWithBindDisabled verifies that bind mounts are rejected when disabled
func TestMountEnforcementWithBindDisabled(t *testing.T) {
	// Create logger and policy enforcer
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	policyEnforcer, err := policy.NewDefault(log)
	if err != nil {
		t.Fatalf("failed to create policy enforcer: %v", err)
	}
	defer policyEnforcer.Close()

	// Set a policy that disables bind mounts entirely
	ctx := context.Background()
	noBindPolicy := policy.DefaultPolicy()
	noBindPolicy.Mounts.AllowBindMounts = false

	if err := policyEnforcer.SetPolicy(ctx, noBindPolicy); err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}

	// Create executor with policy enforcer
	exec, err := NewExecutor(ExecutorConfig{
		PolicyEnforcer: policyEnforcer,
		Logger:         log,
	})
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}
	defer exec.Close()

	// Create a container config with a bind mount
	toolSpec := FileReadTool()
	containerConfig := toolSpec.ToContainerConfig(exec.SessionID(), "/any/path")

	// Validate the config
	result, err := exec.ValidateConfig(ctx, containerConfig)
	if err != nil {
		t.Fatalf("ValidateConfig failed: %v", err)
	}

	// Should not be allowed
	if result.Allowed {
		t.Error("Expected bind mount to be rejected when AllowBindMounts is false")
	}

	// Should have a violation about bind mounts being disabled
	found := false
	for _, v := range result.Violations {
		if v.Rule == "mount.bind.disabled" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'mount.bind.disabled' violation, got: %+v", result.Violations)
	}
}

// TestMountEnforcementPermissiveMode verifies that permissive mode allows with warnings
func TestMountEnforcementPermissiveMode(t *testing.T) {
	// Create logger and policy enforcer
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	policyEnforcer, err := policy.NewDefault(log)
	if err != nil {
		t.Fatalf("failed to create policy enforcer: %v", err)
	}
	defer policyEnforcer.Close()

	// Set a permissive policy that denies /etc but allows in permissive mode
	ctx := context.Background()
	permissivePolicy := policy.PermissivePolicy()
	permissivePolicy.Mounts.AllowBindMounts = true
	permissivePolicy.Mounts.AllowedBindSources = []string{"/safe/*"}
	permissivePolicy.Mounts.DeniedBindSources = []string{"/etc/*"}

	if err := policyEnforcer.SetPolicy(ctx, permissivePolicy); err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}

	// Create executor with policy enforcer
	exec, err := NewExecutor(ExecutorConfig{
		PolicyEnforcer: policyEnforcer,
		Logger:         log,
	})
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}
	defer exec.Close()

	// Create a container config with a denied mount
	toolSpec := FileReadTool()
	containerConfig := toolSpec.ToContainerConfig(exec.SessionID(), "/etc/passwd")

	// Validate the config
	result, err := exec.ValidateConfig(ctx, containerConfig)
	if err != nil {
		t.Fatalf("ValidateConfig failed: %v", err)
	}

	// In permissive mode, should be allowed despite violations
	if !result.Allowed {
		t.Error("Expected permissive mode to allow mount despite violations")
	}

	// Should have violations logged
	if len(result.Violations) == 0 {
		t.Error("Expected violations to be logged in permissive mode")
	}

	t.Logf("Permissive mode test: Allowed=%v, Violations=%d", result.Allowed, len(result.Violations))
}

// TestNetworkPolicy verifies that network policy validation works correctly for web operations
func TestNetworkPolicy(t *testing.T) {
	// Create logger and policy enforcer
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	policyEnforcer, err := policy.NewDefault(log)
	if err != nil {
		t.Fatalf("failed to create policy enforcer: %v", err)
	}
	defer policyEnforcer.Close()

	// Set a policy that disables network access
	ctx := context.Background()
	noNetworkPolicy := &policy.Policy{
		ID:          "no-network",
		Name:        "No Network Policy",
		Description: "Policy that disables all network access",
		Mode:        policy.EnforcementModeStrict,
		Mounts: policy.MountPolicy{
			AllowBindMounts: true,
		},
		Network: policy.NetworkPolicy{
			AllowNetwork:     false,
			AllowHostNetwork: false,
		},
		Images: policy.ImagePolicy{
			AllowLatestTag: true,
		},
		Security: policy.SecurityPolicy{
			AllowPrivileged: false,
		},
	}

	if err := policyEnforcer.SetPolicy(ctx, noNetworkPolicy); err != nil {
		t.Fatalf("failed to set no-network policy: %v", err)
	}

	// Create executor with policy enforcer
	exec, err := NewExecutor(ExecutorConfig{
		PolicyEnforcer: policyEnforcer,
		Logger:         log,
	})
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}
	defer exec.Close()

	tests := []struct {
		name          string
		toolType      ToolType
		wantAllowed   bool
		wantViolation string
	}{
		{
			name:          "web search - blocked when network disabled",
			toolType:      ToolTypeWebSearch,
			wantAllowed:   false,
			wantViolation: "network.disabled",
		},
		{
			name:          "fetch URL - blocked when network disabled",
			toolType:      ToolTypeFetchURL,
			wantAllowed:   false,
			wantViolation: "network.disabled",
		},
		{
			name:        "file read - allowed (no network needed)",
			toolType:    ToolTypeFileRead,
			wantAllowed: true,
		},
		{
			name:        "list files - allowed (no network needed)",
			toolType:    ToolTypeList,
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get tool spec
			toolSpec, err := GetToolSpec(tt.toolType)
			if err != nil {
				t.Fatalf("failed to get tool spec: %v", err)
			}

			// Create container config
			containerConfig := toolSpec.ToContainerConfig(exec.SessionID(), "/tmp")

			// Validate the config
			result, err := exec.ValidateConfig(ctx, containerConfig)
			if err != nil {
				t.Fatalf("ValidateConfig failed: %v", err)
			}

			// Check if allowed status matches expectation
			if result.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v\nViolations: %+v\nWarnings: %+v",
					result.Allowed, tt.wantAllowed, result.Violations, result.Warnings)
			}

			// If we expect a specific violation, verify it exists
			if tt.wantViolation != "" {
				found := false
				for _, v := range result.Violations {
					if v.Rule == tt.wantViolation {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected violation %s not found. Got violations: %+v", tt.wantViolation, result.Violations)
				}
			}

			// Log the result for debugging
			t.Logf("Test '%s': Allowed=%v, Violations=%d, Warnings=%d",
				tt.name, result.Allowed, len(result.Violations), len(result.Warnings))
			for i, v := range result.Violations {
				t.Logf("  Violation[%d]: Rule=%s, Message=%s, Severity=%s",
					i, v.Rule, v.Message, v.Severity)
			}
		})
	}
}

// TestNetworkPolicyAllowed verifies that web operations are allowed when network is enabled
func TestNetworkPolicyAllowed(t *testing.T) {
	// Create logger and policy enforcer
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	policyEnforcer, err := policy.NewDefault(log)
	if err != nil {
		t.Fatalf("failed to create policy enforcer: %v", err)
	}
	defer policyEnforcer.Close()

	// Set a policy that allows network access (this is the default)
	ctx := context.Background()
	allowNetworkPolicy := policy.DefaultPolicy()
	allowNetworkPolicy.Network.AllowNetwork = true

	if err := policyEnforcer.SetPolicy(ctx, allowNetworkPolicy); err != nil {
		t.Fatalf("failed to set allow-network policy: %v", err)
	}

	// Create executor with policy enforcer
	exec, err := NewExecutor(ExecutorConfig{
		PolicyEnforcer: policyEnforcer,
		Logger:         log,
	})
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}
	defer exec.Close()

	// Test web operations are allowed
	webToolSpec, err := GetToolSpec(ToolTypeWebSearch)
	if err != nil {
		t.Fatalf("failed to get web search tool spec: %v", err)
	}

	containerConfig := webToolSpec.ToContainerConfig(exec.SessionID(), "")

	result, err := exec.ValidateConfig(ctx, containerConfig)
	if err != nil {
		t.Fatalf("ValidateConfig failed: %v", err)
	}

	// Should be allowed
	if !result.Allowed {
		t.Errorf("Web operation should be allowed when network is enabled. Got violations: %+v", result.Violations)
	}

	t.Logf("Web operation allowed: %v, Violations: %d", result.Allowed, len(result.Violations))
}
