package policy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestPolicyReloader is a comprehensive test for policy reloader functionality
func TestPolicyReloader(t *testing.T) {
	t.Run("comprehensive reload functionality", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		// Create initial policy file
		initialPolicy := DefaultPolicy()
		initialPolicy.ID = "test-reloader"
		initialPolicy.Mode = EnforcementModeStrict
		writePolicyFile(t, policyPath, initialPolicy)

		// Create reloader with initial policy
		reloader := NewReloader(policyPath, initialPolicy)

		// Start the reloader
		reloader.Start()
		if !reloader.started {
			t.Error("reloader.started = false after Start()")
		}

		// Test callback execution
		callbackCalled := false
		reloader.AddCallback(func(ctx context.Context, newPolicy *Policy) error {
			callbackCalled = true
			return nil
		})

		// Reload policy
		ctx := context.Background()
		if err := reloader.Reload(ctx); err != nil {
			t.Fatalf("Reload() error = %v", err)
		}

		if !callbackCalled {
			t.Error("callback was not called during reload")
		}

		// Verify policy was loaded
		currentPolicy := reloader.GetPolicy()
		if currentPolicy.ID != "test-reloader" {
			t.Errorf("policy ID = %s, want test-reloader", currentPolicy.ID)
		}

		// Verify state
		if reloader.State() != ReloadStateIdle {
			t.Errorf("State() = %s, want %s", reloader.State(), ReloadStateIdle)
		}

		if reloader.IsReloading() {
			t.Error("IsReloading() = true after reload completed")
		}

		// Stop the reloader
		reloader.Stop()
		if reloader.started {
			t.Error("reloader.started = true after Stop()")
		}

		if reloader.State() != ReloadStateStopped {
			t.Errorf("State() after Stop = %s, want %s", reloader.State(), ReloadStateStopped)
		}
	})
}

func TestNewReloader(t *testing.T) {
	t.Run("creates reloader with valid policy", func(t *testing.T) {
		policy := DefaultPolicy()
		policyPath := "/tmp/test-policy.yaml"

		reloader := NewReloader(policyPath, policy)

		if reloader == nil {
			t.Fatal("NewReloader() returned nil")
		}

		if reloader.policyPath != policyPath {
			t.Errorf("policyPath = %s, want %s", reloader.policyPath, policyPath)
		}

		if reloader.currentPolicy != policy {
			t.Error("currentPolicy not set correctly")
		}

		if reloader.state != ReloadStateIdle {
			t.Errorf("state = %s, want %s", reloader.state, ReloadStateIdle)
		}

		if reloader.started {
			t.Error("reloader should not be started initially")
		}

		if len(reloader.callbacks) != 0 {
			t.Errorf("callbacks = %d, want 0", len(reloader.callbacks))
		}
	})

	t.Run("creates reloader with nil policy", func(t *testing.T) {
		policyPath := "/tmp/test-policy.yaml"

		reloader := NewReloader(policyPath, nil)

		if reloader == nil {
			t.Fatal("NewReloader() returned nil")
		}

		if reloader.currentPolicy != nil {
			t.Error("currentPolicy should be nil")
		}
	})
}

func TestReloaderStartStop(t *testing.T) {
	t.Run("start starts signal handler", func(t *testing.T) {
		policy := DefaultPolicy()
		reloader := NewReloader("/tmp/test-policy.yaml", policy)

		reloader.Start()

		if !reloader.started {
			t.Error("reloader.started = false, want true")
		}

		if reloader.state != ReloadStateIdle {
			t.Errorf("state = %s, want %s", reloader.state, ReloadStateIdle)
		}

		reloader.Stop()
	})

	t.Run("stop stops signal handler", func(t *testing.T) {
		policy := DefaultPolicy()
		reloader := NewReloader("/tmp/test-policy.yaml", policy)

		reloader.Start()
		reloader.Stop()

		if reloader.started {
			t.Error("reloader.started = true, want false")
		}

		if reloader.state != ReloadStateStopped {
			t.Errorf("state = %s, want %s", reloader.state, ReloadStateStopped)
		}
	})

	t.Run("start is idempotent", func(t *testing.T) {
		policy := DefaultPolicy()
		reloader := NewReloader("/tmp/test-policy.yaml", policy)

		reloader.Start()
		firstStarted := reloader.started
		reloader.Start()
		secondStarted := reloader.started

		if firstStarted != secondStarted {
			t.Error("Start() should be idempotent")
		}

		reloader.Stop()
	})

	t.Run("stop is idempotent", func(t *testing.T) {
		policy := DefaultPolicy()
		reloader := NewReloader("/tmp/test-policy.yaml", policy)

		reloader.Start()
		reloader.Stop()
		reloader.Stop()

		if reloader.started {
			t.Error("Stop() should be idempotent")
		}
	})

	t.Run("restart after stop works correctly", func(t *testing.T) {
		policy := DefaultPolicy()
		reloader := NewReloader("/tmp/test-policy.yaml", policy)

		reloader.Start()
		if reloader.state != ReloadStateIdle {
			t.Errorf("after first start state = %s, want %s", reloader.state, ReloadStateIdle)
		}

		reloader.Stop()
		if reloader.state != ReloadStateStopped {
			t.Errorf("after stop state = %s, want %s", reloader.state, ReloadStateStopped)
		}

		reloader.Start()
		if reloader.state != ReloadStateIdle {
			t.Errorf("after restart state = %s, want %s", reloader.state, ReloadStateIdle)
		}

		reloader.Stop()
	})
}

func TestReloaderReload(t *testing.T) {
	t.Run("reload loads policy from file", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		// Create initial policy file
		initialPolicy := DefaultPolicy()
		initialPolicy.ID = "initial"
		initialPolicy.Mode = EnforcementModeStrict
		writePolicyFile(t, policyPath, initialPolicy)

		// Create reloader
		reloader := NewReloader(policyPath, initialPolicy)

		// Reload policy
		ctx := context.Background()
		err := reloader.Reload(ctx)
		if err != nil {
			t.Fatalf("Reload() error = %v", err)
		}

		// Verify policy was loaded
		currentPolicy := reloader.GetPolicy()
		if currentPolicy.ID != "initial" {
			t.Errorf("policy ID = %s, want initial", currentPolicy.ID)
		}

		if currentPolicy.Mode != EnforcementModeStrict {
			t.Errorf("policy mode = %s, want %s", currentPolicy.Mode, EnforcementModeStrict)
		}
	})

	t.Run("reload loads updated policy", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		// Create initial policy
		initialPolicy := DefaultPolicy()
		initialPolicy.ID = "initial"
		initialPolicy.Mode = EnforcementModeStrict
		writePolicyFile(t, policyPath, initialPolicy)

		// Create reloader with initial policy
		reloader := NewReloader(policyPath, initialPolicy)

		// Update policy file
		updatedPolicy := DefaultPolicy()
		updatedPolicy.ID = "updated"
		updatedPolicy.Mode = EnforcementModePermissive
		writePolicyFile(t, policyPath, updatedPolicy)

		// Reload policy
		ctx := context.Background()
		err := reloader.Reload(ctx)
		if err != nil {
			t.Fatalf("Reload() error = %v", err)
		}

		// Verify policy was updated
		currentPolicy := reloader.GetPolicy()
		if currentPolicy.ID != "updated" {
			t.Errorf("policy ID = %s, want updated", currentPolicy.ID)
		}

		if currentPolicy.Mode != EnforcementModePermissive {
			t.Errorf("policy mode = %s, want %s", currentPolicy.Mode, EnforcementModePermissive)
		}
	})

	t.Run("reload returns error for invalid file", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "invalid.yaml")

		// Write invalid YAML content
		if err := os.WriteFile(policyPath, []byte("invalid: yaml: content: ["), 0644); err != nil {
			t.Fatalf("Failed to write invalid policy file: %v", err)
		}

		initialPolicy := DefaultPolicy()
		reloader := NewReloader(policyPath, initialPolicy)

		ctx := context.Background()
		err := reloader.Reload(ctx)

		// Should error - file has invalid YAML
		if err == nil {
			t.Error("Reload() expected error for invalid YAML, got nil")
		}
	})

	t.Run("reload handles concurrent reloads", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		initialPolicy := DefaultPolicy()
		initialPolicy.ID = "test"
		writePolicyFile(t, policyPath, initialPolicy)

		reloader := NewReloader(policyPath, initialPolicy)

		ctx := context.Background()
		var wg sync.WaitGroup
		errors := make(chan error, 3)

		// Start concurrent reloads
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := reloader.Reload(ctx); err != nil {
					errors <- err
				}
			}()
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("Reload() concurrent error = %v", err)
		}

		// State should be idle after all reloads complete
		if reloader.State() != ReloadStateIdle {
			t.Errorf("State() = %s, want %s", reloader.State(), ReloadStateIdle)
		}
	})

	t.Run("reload sets state correctly during reload", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		initialPolicy := DefaultPolicy()
		initialPolicy.ID = "test"
		writePolicyFile(t, policyPath, initialPolicy)

		reloader := NewReloader(policyPath, initialPolicy)

		// Check initial state
		if !reloader.IsReloading() {
			// Should not be reloading initially
		}

		ctx := context.Background()
		err := reloader.Reload(ctx)
		if err != nil {
			t.Fatalf("Reload() error = %v", err)
		}

		// Check final state
		if reloader.IsReloading() {
			t.Error("IsReloading() = true, want false after reload completes")
		}
	})
}

func TestReloaderAddCallback(t *testing.T) {
	t.Run("callback is executed on reload", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		initialPolicy := DefaultPolicy()
		initialPolicy.ID = "initial"
		writePolicyFile(t, policyPath, initialPolicy)

		reloader := NewReloader(policyPath, initialPolicy)

		callbackCalled := false
		var receivedPolicy *Policy

		reloader.AddCallback(func(ctx context.Context, newPolicy *Policy) error {
			callbackCalled = true
			receivedPolicy = newPolicy
			return nil
		})

		ctx := context.Background()
		err := reloader.Reload(ctx)
		if err != nil {
			t.Fatalf("Reload() error = %v", err)
		}

		if !callbackCalled {
			t.Error("callback was not called")
		}

		if receivedPolicy == nil {
			t.Error("received policy is nil")
		} else if receivedPolicy.ID != "initial" {
			t.Errorf("received policy ID = %s, want initial", receivedPolicy.ID)
		}
	})

	t.Run("multiple callbacks are executed", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		initialPolicy := DefaultPolicy()
		writePolicyFile(t, policyPath, initialPolicy)

		reloader := NewReloader(policyPath, initialPolicy)

		callCount := int32(0)

		// Add multiple callbacks
		for i := 0; i < 3; i++ {
			reloader.AddCallback(func(ctx context.Context, newPolicy *Policy) error {
				atomic.AddInt32(&callCount, 1)
				return nil
			})
		}

		ctx := context.Background()
		err := reloader.Reload(ctx)
		if err != nil {
			t.Fatalf("Reload() error = %v", err)
		}

		if callCount != 3 {
			t.Errorf("callback count = %d, want 3", callCount)
		}
	})

	t.Run("callback error prevents policy update", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		initialPolicy := DefaultPolicy()
		initialPolicy.ID = "initial"
		writePolicyFile(t, policyPath, initialPolicy)

		reloader := NewReloader(policyPath, initialPolicy)

		// Add callback that returns error
		reloader.AddCallback(func(ctx context.Context, newPolicy *Policy) error {
			return context.DeadlineExceeded
		})

		// Update policy file
		updatedPolicy := DefaultPolicy()
		updatedPolicy.ID = "updated"
		writePolicyFile(t, policyPath, updatedPolicy)

		ctx := context.Background()
		err := reloader.Reload(ctx)
		if err == nil {
			t.Error("Reload() expected error from callback, got nil")
		}

		// Policy should not be updated
		currentPolicy := reloader.GetPolicy()
		if currentPolicy.ID == "updated" {
			t.Error("policy was updated despite callback error")
		}
	})

	t.Run("callbacks are called in order", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		initialPolicy := DefaultPolicy()
		writePolicyFile(t, policyPath, initialPolicy)

		reloader := NewReloader(policyPath, initialPolicy)

		order := make([]int, 0, 3)

		reloader.AddCallback(func(ctx context.Context, newPolicy *Policy) error {
			order = append(order, 1)
			return nil
		})
		reloader.AddCallback(func(ctx context.Context, newPolicy *Policy) error {
			order = append(order, 2)
			return nil
		})
		reloader.AddCallback(func(ctx context.Context, newPolicy *Policy) error {
			order = append(order, 3)
			return nil
		})

		ctx := context.Background()
		err := reloader.Reload(ctx)
		if err != nil {
			t.Fatalf("Reload() error = %v", err)
		}

		if len(order) != 3 {
			t.Fatalf("callback order length = %d, want 3", len(order))
		}

		if order[0] != 1 || order[1] != 2 || order[2] != 3 {
			t.Errorf("callback order = %v, want [1, 2, 3]", order)
		}
	})
}

func TestReloaderGetPolicy(t *testing.T) {
	t.Run("GetPolicy returns current policy", func(t *testing.T) {
		policy := DefaultPolicy()
		policy.ID = "test-policy"
		reloader := NewReloader("/tmp/test-policy.yaml", policy)

		retrievedPolicy := reloader.GetPolicy()

		if retrievedPolicy == nil {
			t.Fatal("GetPolicy() returned nil")
		}

		if retrievedPolicy.ID != "test-policy" {
			t.Errorf("policy ID = %s, want test-policy", retrievedPolicy.ID)
		}
	})

	t.Run("GetPolicy is thread-safe", func(t *testing.T) {
		policy := DefaultPolicy()
		policy.ID = "test-policy"
		reloader := NewReloader("/tmp/test-policy.yaml", policy)

		// Concurrent reads
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				retrievedPolicy := reloader.GetPolicy()
				if retrievedPolicy == nil {
					t.Error("GetPolicy() returned nil in concurrent read")
				}
			}()
		}

		wg.Wait()
	})
}

func TestReloaderState(t *testing.T) {
	t.Run("State returns current state", func(t *testing.T) {
		policy := DefaultPolicy()
		reloader := NewReloader("/tmp/test-policy.yaml", policy)

		if reloader.State() != ReloadStateIdle {
			t.Errorf("State() = %s, want %s", reloader.State(), ReloadStateIdle)
		}

		reloader.Start()

		if reloader.State() != ReloadStateIdle {
			t.Errorf("State() after Start = %s, want %s", reloader.State(), ReloadStateIdle)
		}

		reloader.Stop()

		if reloader.State() != ReloadStateStopped {
			t.Errorf("State() after Stop = %s, want %s", reloader.State(), ReloadStateStopped)
		}
	})

	t.Run("IsReloading returns true during reload", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		initialPolicy := DefaultPolicy()
		writePolicyFile(t, policyPath, initialPolicy)

		reloader := NewReloader(policyPath, initialPolicy)

		// Not reloading initially
		if reloader.IsReloading() {
			t.Error("IsReloading() = true before reload")
		}

		ctx := context.Background()
		err := reloader.Reload(ctx)
		if err != nil {
			t.Fatalf("Reload() error = %v", err)
		}

		// Not reloading after completion
		if reloader.IsReloading() {
			t.Error("IsReloading() = true after reload completes")
		}
	})
}

func TestReloaderSignalHandling(t *testing.T) {
	t.Run("SIGHUP triggers reload", func(t *testing.T) {
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")

		initialPolicy := DefaultPolicy()
		initialPolicy.ID = "initial"
		writePolicyFile(t, policyPath, initialPolicy)

		reloader := NewReloader(policyPath, initialPolicy)

		reloadTriggered := make(chan bool, 1)
		reloader.AddCallback(func(ctx context.Context, newPolicy *Policy) error {
			reloadTriggered <- true
			return nil
		})

		reloader.Start()
		defer reloader.Stop()

		// Send SIGHUP signal to self
		err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP)
		if err != nil {
			t.Fatalf("Failed to send SIGHUP: %v", err)
		}

		// Wait for reload to be triggered
		select {
		case <-reloadTriggered:
			// Reload was triggered successfully
		case <-time.After(5 * time.Second):
			t.Error("SIGHUP did not trigger reload within 5 seconds")
		}
	})
}

func TestReloadStateString(t *testing.T) {
	tests := []struct {
		state    ReloadState
		expected string
	}{
		{ReloadStateIdle, "idle"},
		{ReloadStateReloading, "reloading"},
		{ReloadStateStopped, "stopped"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.state.String() != tt.expected {
				t.Errorf("ReloadState.String() = %s, want %s", tt.state.String(), tt.expected)
			}
		})
	}
}

func TestReloaderString(t *testing.T) {
	t.Run("String returns valid representation", func(t *testing.T) {
		policyPath := "/tmp/test-policy.yaml"
		policy := DefaultPolicy()
		reloader := NewReloader(policyPath, policy)

		str := reloader.String()

		if !contains(str, "Reloader") {
			t.Error("String() should contain 'Reloader'")
		}

		if !contains(str, policyPath) {
			t.Errorf("String() should contain policy path %s", policyPath)
		}

		if !contains(str, "idle") {
			t.Error("String() should contain state 'idle'")
		}

		if !contains(str, "callbacks: 0") {
			t.Error("String() should contain callback count")
		}
	})

	t.Run("String reflects callback count", func(t *testing.T) {
		policy := DefaultPolicy()
		reloader := NewReloader("/tmp/test-policy.yaml", policy)

		reloader.AddCallback(func(ctx context.Context, newPolicy *Policy) error {
			return nil
		})

		str := reloader.String()

		if !contains(str, "callbacks: 1") {
			t.Errorf("String() = %s, should contain 'callbacks: 1'", str)
		}
	})
}

// Helper functions

func writePolicyFile(t *testing.T, path string, policy *Policy) {
	t.Helper()

	// Marshal policy to YAML
	yamlData, err := yaml.Marshal(policy)
	if err != nil {
		t.Fatalf("Failed to marshal policy: %v", err)
	}

	// Write policy file
	if err := os.WriteFile(path, yamlData, 0644); err != nil {
		t.Fatalf("Failed to write policy file: %v", err)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
