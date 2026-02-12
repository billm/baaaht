package integration

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/orchestrator"
	"github.com/billm/baaaht/orchestrator/pkg/policy"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/require"
)

// TestPolicyEnforcementFlow tests the full policy enforcement flow from YAML to container creation
func TestPolicyEnforcementFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Log("=== Testing Policy Enforcement Flow ===")

	// Step 1: Create a test policy YAML file
	t.Log("=== Step 1: Creating test policy YAML file ===")

	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "test-policy.yaml")

	// Create a restrictive policy that disallows latest tag and sets CPU/memory limits
	policyYAML := `
id: test-policy
name: Test Policy
description: Policy for integration testing
mode: strict

quotas:
  max_cpus: 2000000000  # 2 CPUs
  max_memory: 5368709120  # 5GB

images:
  allow_latest_tag: false
`

	err = os.WriteFile(policyPath, []byte(policyYAML), 0644)
	require.NoError(t, err, "Failed to write policy file")

	t.Logf("Policy file created: %s", policyPath)

	// Step 2: Load config and set policy path
	t.Log("=== Step 2: Loading config with policy path ===")

	cfg, err := config.Load()
	require.NoError(t, err, "Failed to load config")

	// Override paths for testing
	cfg.Credentials.StorePath = filepath.Join(tmpDir, "credentials")
	cfg.Session.StoragePath = filepath.Join(tmpDir, "sessions")
	cfg.IPC.SocketPath = filepath.Join(tmpDir, "ipc.sock")
	cfg.Policy.ConfigPath = policyPath
	cfg.LLM.Enabled = false

	// Step 3: Bootstrap orchestrator with policy enforcement
	t.Log("=== Step 3: Bootstrapping orchestrator with policy enforcer ===")

	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:            *cfg,
		Logger:            log,
		Version:           "test-policy-1.0.0",
		ShutdownTimeout:   10 * time.Second,
		EnableHealthCheck: false,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	orch := result.Orchestrator

	defer func() {
		t.Log("=== Cleanup: Shutting down orchestrator ===")
		_ = orch.Close()
	}()

	// Verify policy enforcer is initialized
	policyEnforcer := orch.PolicyEnforcer()
	require.NotNil(t, policyEnforcer, "Policy enforcer should not be nil")

	// Verify policy was loaded
	currentPolicy, err := policyEnforcer.GetPolicy(ctx)
	require.NoError(t, err, "Failed to get current policy")
	require.Equal(t, "test-policy", currentPolicy.ID)
	require.Equal(t, policy.EnforcementModeStrict, currentPolicy.Mode)
	t.Logf("Policy loaded: id=%s, mode=%s", currentPolicy.ID, currentPolicy.Mode)

	// Step 4: Create a test session
	t.Log("=== Step 4: Creating test session ===")

	sessionMgr := orch.SessionManager()
	sessionID, err := sessionMgr.Create(ctx, types.SessionMetadata{
		Name:    "policy-test-session",
		OwnerID: "policy-test-user",
	}, types.SessionConfig{
		MaxContainers: 5,
	})
	require.NoError(t, err, "Failed to create session")
	t.Logf("Session created: %s", sessionID)

	// Step 5: Test container creation with violating configuration
	t.Log("=== Step 5: Testing container creation with violating configuration ===")

	dockerClient := orch.DockerClient()
	creator, err := container.NewCreator(dockerClient, log)
	require.NoError(t, err, "Failed to create container creator")

	// Set the policy enforcer on the creator
	creator.SetEnforcer(policyEnforcer)

	// Try to create container with latest tag (should fail)
	containerNameLatest := "policy-test-latest-" + time.Now().Format("20060102150405")
	createCfgLatest := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest", // This violates the policy (no latest tag allowed)
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels:  make(map[string]string),
		},
		Name:        containerNameLatest,
		SessionID:   sessionID,
		AutoPull:    false, // Don't pull to avoid network dependency
		PullTimeout: 1 * time.Minute,
	}

	_, err = creator.Create(ctx, createCfgLatest)
	require.Error(t, err, "Container creation with latest tag should fail")

	// Verify it's a permission error
	var customErr *types.Error
	require.ErrorAs(t, err, &customErr, "Error should be a types.Error")
	require.Equal(t, types.ErrCodePermission, customErr.Code, "Error code should indicate permission error")
	require.Contains(t, err.Error(), "policy", "Error should mention policy")
	t.Log("Container creation with latest tag correctly failed with policy error")

	// Step 6: Test container creation with CPU quota violation
	t.Log("=== Step 6: Testing container creation with CPU quota violation ===")

	containerNameCPU := "policy-test-cpu-" + time.Now().Format("20060102150405")
	maxCPUs := int64(4000000000) // 4 CPUs - exceeds max_cpus of 2
	createCfgCPU := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:3.18",
			Command: []string{"sleep", "300"},
			Resources: types.ResourceLimits{
				NanoCPUs: maxCPUs,
			},
			Env:    make(map[string]string),
			Labels: make(map[string]string),
		},
		Name:        containerNameCPU,
		SessionID:   sessionID,
		AutoPull:    false,
		PullTimeout: 1 * time.Minute,
	}

	_, err = creator.Create(ctx, createCfgCPU)
	require.Error(t, err, "Container creation with CPU quota violation should fail")
	t.Log("Container creation with CPU quota violation correctly failed")

	// Step 7: Test container creation with compliant configuration
	t.Log("=== Step 7: Testing container creation with compliant configuration ===")

	containerNameCompliant := "policy-test-compliant-" + time.Now().Format("20060102150405")
	createCfgCompliant := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:3.18", // Specific tag, not latest
			Command: []string{"sleep", "300"},
			Resources: types.ResourceLimits{
				NanoCPUs:    1000000000, // 1 CPU - within limit
				MemoryBytes: 1073741824, // 1GB - within limit
			},
			NetworkMode: "none",
			Env:         make(map[string]string),
			Labels:      make(map[string]string),
		},
		Name:        containerNameCompliant,
		SessionID:   sessionID,
		AutoPull:    false,
		PullTimeout: 5 * time.Minute,
	}
	createCfgCompliant.Config.Labels["baaaht.session_id"] = sessionID.String()
	createCfgCompliant.Config.Labels["baaaht.managed"] = "true"

	createResult, err := creator.Create(ctx, createCfgCompliant)
	require.NoError(t, err, "Container creation with compliant configuration should succeed")
	require.NotNil(t, createResult, "Create result should not be nil")
	require.NotEmpty(t, createResult.ContainerID, "Container ID should not be empty")

	t.Logf("Container created successfully with compliant config: %s", createResult.ContainerID)

	// Step 8: Start and verify the container
	t.Log("=== Step 8: Starting and verifying the container ===")

	lifecycleMgr, err := container.NewLifecycleManager(dockerClient, log)
	require.NoError(t, err, "Failed to create lifecycle manager")

	startCfg := container.StartConfig{
		ContainerID: createResult.ContainerID,
		Name:        containerNameCompliant,
	}

	err = lifecycleMgr.Start(ctx, startCfg)
	require.NoError(t, err, "Failed to start container")
	t.Logf("Container started: %s", containerNameCompliant)

	// Verify container is running
	status, err := lifecycleMgr.Status(ctx, createResult.ContainerID)
	require.NoError(t, err, "Failed to get container status")
	require.Equal(t, types.ContainerStateRunning, status, "Container should be running")
	t.Log("Container verified running")

	// Step 9: Cleanup
	t.Log("=== Step 9: Cleaning up container ===")

	stopTimeout := 10 * time.Second
	stopCfg := container.StopConfig{
		ContainerID: createResult.ContainerID,
		Name:        containerNameCompliant,
		Timeout:     &stopTimeout,
	}

	err = lifecycleMgr.Stop(ctx, stopCfg)
	require.NoError(t, err, "Failed to stop container")
	t.Log("Container stopped")

	destroyCfg := container.DestroyConfig{
		ContainerID:   createResult.ContainerID,
		Name:          containerNameCompliant,
		Force:         true,
		RemoveVolumes: true,
	}

	err = lifecycleMgr.Destroy(ctx, destroyCfg)
	require.NoError(t, err, "Failed to destroy container")
	t.Log("Container destroyed")

	// Test complete
	t.Log("=== Policy Enforcement Flow Test Complete ===")
	t.Log("All steps passed successfully:")
	t.Log("  1. Policy YAML file created")
	t.Log("  2. Config loaded with policy path")
	t.Log("  3. Orchestrator bootstrapped with policy enforcer")
	t.Log("  4. Session created")
	t.Log("  5. Container with latest tag correctly rejected")
	t.Log("  6. Container with CPU quota violation correctly rejected")
	t.Log("  7. Container with compliant config created successfully")
	t.Log("  8. Container started and verified")
	t.Log("  9. Container cleaned up")
}

// TestPolicyPermissiveMode tests policy enforcement in permissive mode
func TestPolicyPermissiveMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Log("=== Testing Policy Permissive Mode ===")

	// Create a permissive policy file
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "permissive-policy.yaml")

	policyYAML := `
id: permissive-policy
name: Permissive Policy
mode: permissive

quotas:
  max_cpus: 2000000000

images:
  allow_latest_tag: false
`

	err = os.WriteFile(policyPath, []byte(policyYAML), 0644)
	require.NoError(t, err, "Failed to write policy file")

	// Load config
	cfg, err := config.Load()
	require.NoError(t, err, "Failed to load config")

	cfg.Credentials.StorePath = filepath.Join(tmpDir, "credentials")
	cfg.Session.StoragePath = filepath.Join(tmpDir, "sessions")
	cfg.IPC.SocketPath = filepath.Join(tmpDir, "ipc.sock")
	cfg.Policy.ConfigPath = policyPath

	// Bootstrap orchestrator
	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:            *cfg,
		Logger:            log,
		Version:           "test-permissive-1.0.0",
		ShutdownTimeout:   10 * time.Second,
		EnableHealthCheck: false,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	orch := result.Orchestrator

	defer func() {
		_ = orch.Close()
	}()

	policyEnforcer := orch.PolicyEnforcer()
	require.NotNil(t, policyEnforcer, "Policy enforcer should not be nil")

	// Create session
	sessionMgr := orch.SessionManager()
	sessionID, err := sessionMgr.Create(ctx, types.SessionMetadata{
		Name:    "permissive-test-session",
		OwnerID: "permissive-test-user",
	}, types.SessionConfig{})
	require.NoError(t, err, "Failed to create session")

	// Create container creator with enforcer
	dockerClient := orch.DockerClient()
	creator, err := container.NewCreator(dockerClient, log)
	require.NoError(t, err, "Failed to create container creator")
	creator.SetEnforcer(policyEnforcer)

	// In permissive mode, violations should be logged but not rejected
	containerName := "permissive-test-" + time.Now().Format("20060102150405")
	createCfg := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:3.18",
			Command: []string{"sleep", "300"},
			Resources: types.ResourceLimits{
				NanoCPUs: 4000000000, // Exceeds max_cpus of 2
			},
			Env:    make(map[string]string),
			Labels: make(map[string]string),
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	// This should succeed in permissive mode (but log a warning)
	createResult, err := creator.Create(ctx, createCfg)
	require.NoError(t, err, "In permissive mode, container creation should succeed despite violations")
	require.NotNil(t, createResult, "Create result should not be nil")

	t.Logf("Container created in permissive mode: %s", createResult.ContainerID)

	// Cleanup
	lifecycleMgr, err := container.NewLifecycleManager(dockerClient, log)
	require.NoError(t, err, "Failed to create lifecycle manager")
	require.NotNil(t, lifecycleMgr, "Lifecycle manager should not be nil")
	err = lifecycleMgr.Destroy(ctx, container.DestroyConfig{
		ContainerID:   createResult.ContainerID,
		Name:          containerName,
		Force:         true,
		RemoveVolumes: true,
	})
	require.NoError(t, err, "Failed to destroy container")

	t.Log("Permissive mode test passed")
}

// TestPolicyDisabledMode tests policy enforcement in disabled mode
func TestPolicyDisabledMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Log("=== Testing Policy Disabled Mode ===")

	// Create a disabled policy file
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "disabled-policy.yaml")

	policyYAML := `
id: disabled-policy
name: Disabled Policy
mode: disabled

quotas:
  max_cpus: 1000000000

images:
  allow_latest_tag: false
`

	err = os.WriteFile(policyPath, []byte(policyYAML), 0644)
	require.NoError(t, err, "Failed to write policy file")

	// Load config
	cfg, err := config.Load()
	require.NoError(t, err, "Failed to load config")

	cfg.Credentials.StorePath = filepath.Join(tmpDir, "credentials")
	cfg.Session.StoragePath = filepath.Join(tmpDir, "sessions")
	cfg.IPC.SocketPath = filepath.Join(tmpDir, "ipc.sock")
	cfg.Policy.ConfigPath = policyPath

	// Bootstrap orchestrator
	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:            *cfg,
		Logger:            log,
		Version:           "test-disabled-1.0.0",
		ShutdownTimeout:   10 * time.Second,
		EnableHealthCheck: false,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	orch := result.Orchestrator

	defer func() {
		_ = orch.Close()
	}()

	policyEnforcer := orch.PolicyEnforcer()
	require.NotNil(t, policyEnforcer, "Policy enforcer should not be nil")

	// Create session
	sessionMgr := orch.SessionManager()
	sessionID, err := sessionMgr.Create(ctx, types.SessionMetadata{
		Name:    "disabled-test-session",
		OwnerID: "disabled-test-user",
	}, types.SessionConfig{})
	require.NoError(t, err, "Failed to create session")

	// Create container creator with enforcer
	dockerClient := orch.DockerClient()
	creator, err := container.NewCreator(dockerClient, log)
	require.NoError(t, err, "Failed to create container creator")
	creator.SetEnforcer(policyEnforcer)

	// In disabled mode, everything should be allowed
	containerName := "disabled-test-" + time.Now().Format("20060102150405")
	createCfg := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:3.18",
			Command: []string{"sleep", "300"},
			Resources: types.ResourceLimits{
				NanoCPUs: 8000000000, // Way exceeds max_cpus of 1
			},
			Env:    make(map[string]string),
			Labels: make(map[string]string),
		},
		Name:        containerName,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	// This should succeed in disabled mode (no enforcement)
	createResult, err := creator.Create(ctx, createCfg)
	require.NoError(t, err, "In disabled mode, container creation should succeed with no restrictions")
	require.NotNil(t, createResult, "Create result should not be nil")

	t.Logf("Container created in disabled mode: %s", createResult.ContainerID)

	// Cleanup
	lifecycleMgr, err := container.NewLifecycleManager(dockerClient, log)
	require.NoError(t, err, "Failed to create lifecycle manager")
	require.NotNil(t, lifecycleMgr, "Lifecycle manager should not be nil")

	err = lifecycleMgr.Destroy(ctx, container.DestroyConfig{
		ContainerID:   createResult.ContainerID,
		Name:          containerName,
		Force:         true,
		RemoveVolumes: true,
	})
	require.NoError(t, err, "Failed to destroy container")

	t.Log("Disabled mode test passed")
}

// TestPolicyHotReload tests policy hot-reload during orchestrator operation
func TestPolicyHotReload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Log("=== Testing Policy Hot-Reload ===")

	// Step 1: Create initial policy YAML file
	t.Log("=== Step 1: Creating initial policy YAML file ===")

	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "hotreload-policy.yaml")

	// Initial policy: allow specific images, allow latest tag
	initialPolicyYAML := `
id: hotreload-policy
name: Hot Reload Test Policy
description: Policy for testing hot-reload functionality
mode: strict

quotas:
  max_cpus: 4000000000  # 4 CPUs
  max_memory: 8589934592  # 8GB

images:
  allow_latest_tag: true  # Initially allow latest tag
`

	err = os.WriteFile(policyPath, []byte(initialPolicyYAML), 0644)
	require.NoError(t, err, "Failed to write initial policy file")
	t.Logf("Initial policy file created: %s", policyPath)

	// Step 2: Load config and set policy path
	t.Log("=== Step 2: Loading config with policy path ===")

	cfg, err := config.Load()
	require.NoError(t, err, "Failed to load config")

	// Override paths for testing
	cfg.Credentials.StorePath = filepath.Join(tmpDir, "credentials")
	cfg.Session.StoragePath = filepath.Join(tmpDir, "sessions")
	cfg.IPC.SocketPath = filepath.Join(tmpDir, "ipc.sock")
	cfg.Policy.ConfigPath = policyPath
	// Ensure hot-reload is enabled (should be default, but be explicit)
	cfg.Policy.ReloadOnChanges = true
	cfg.LLM.Enabled = false

	// Step 3: Bootstrap orchestrator with policy enforcement
	t.Log("=== Step 3: Bootstrapping orchestrator with policy hot-reload ===")

	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:            *cfg,
		Logger:            log,
		Version:           "test-hotreload-1.0.0",
		ShutdownTimeout:   10 * time.Second,
		EnableHealthCheck: false,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	orch := result.Orchestrator

	defer func() {
		t.Log("=== Cleanup: Shutting down orchestrator ===")
		_ = orch.Close()
	}()

	// Verify policy enforcer is initialized
	policyEnforcer := orch.PolicyEnforcer()
	require.NotNil(t, policyEnforcer, "Policy enforcer should not be nil")

	// Verify policy reloader is initialized
	policyReloader := orch.PolicyReloader()
	require.NotNil(t, policyReloader, "Policy reloader should not be nil")
	t.Log("Policy reloader initialized successfully")

	// Verify initial policy was loaded
	currentPolicy, err := policyEnforcer.GetPolicy(ctx)
	require.NoError(t, err, "Failed to get current policy")
	require.Equal(t, "hotreload-policy", currentPolicy.ID)
	require.True(t, currentPolicy.Images.AllowLatestTag, "Initial policy should allow latest tag")
	t.Logf("Initial policy loaded: id=%s, allow_latest=%v", currentPolicy.ID, currentPolicy.Images.AllowLatestTag)

	// Step 4: Create a test session
	t.Log("=== Step 4: Creating test session ===")

	sessionMgr := orch.SessionManager()
	sessionID, err := sessionMgr.Create(ctx, types.SessionMetadata{
		Name:    "hotreload-test-session",
		OwnerID: "hotreload-test-user",
	}, types.SessionConfig{
		MaxContainers: 5,
	})
	require.NoError(t, err, "Failed to create session")
	t.Logf("Session created: %s", sessionID)

	// Step 5: Create container allowed by initial policy (with latest tag)
	t.Log("=== Step 5: Creating container allowed by initial policy ===")

	dockerClient := orch.DockerClient()
	creator, err := container.NewCreator(dockerClient, log)
	require.NoError(t, err, "Failed to create container creator")

	// Set the policy enforcer on the creator
	creator.SetEnforcer(policyEnforcer)

	containerName1 := "hotreload-test-1-" + time.Now().Format("20060102150405")
	createCfg1 := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest", // Allowed by initial policy
			Command: []string{"sleep", "300"},
			NetworkMode: "none",
			Env:         make(map[string]string),
			Labels:      make(map[string]string),
		},
		Name:        containerName1,
		SessionID:   sessionID,
		AutoPull:    false,
		PullTimeout: 5 * time.Minute,
	}
	createCfg1.Config.Labels["baaaht.session_id"] = sessionID.String()
	createCfg1.Config.Labels["baaaht.managed"] = "true"

	createResult1, err := creator.Create(ctx, createCfg1)
	require.NoError(t, err, "Container creation with latest tag should succeed with initial policy")
	require.NotNil(t, createResult1, "Create result should not be nil")
	t.Logf("Container created successfully with initial policy: %s", createResult1.ContainerID)

	// Cleanup container
	defer func() {
		lifecycleMgr, err := container.NewLifecycleManager(dockerClient, log)
		if err != nil {
			t.Logf("Failed to create lifecycle manager for cleanup: %v", err)
			return
		}
		err = lifecycleMgr.Destroy(ctx, container.DestroyConfig{
			ContainerID:   createResult1.ContainerID,
			Name:          containerName1,
			Force:         true,
			RemoveVolumes: true,
		})
		if err != nil {
			t.Logf("Failed to destroy container during cleanup: %v", err)
		}
	}()

	// Step 6: Modify policy YAML file (disallow latest tag)
	t.Log("=== Step 6: Modifying policy YAML file ===")

	updatedPolicyYAML := `
id: hotreload-policy
name: Hot Reload Test Policy
description: Policy for testing hot-reload functionality (updated)
mode: strict

quotas:
  max_cpus: 4000000000  # 4 CPUs
  max_memory: 8589934592  # 8GB

images:
  allow_latest_tag: false  # Now disallow latest tag
`

	err = os.WriteFile(policyPath, []byte(updatedPolicyYAML), 0644)
	require.NoError(t, err, "Failed to write updated policy file")
	t.Logf("Policy file updated: %s", policyPath)

	// Step 7: Trigger reload by sending SIGHUP to self
	t.Log("=== Step 7: Triggering policy reload via SIGHUP ===")

	// Get the current process
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err, "Failed to find current process")

	// Send SIGHUP to trigger reload
	err = process.Signal(syscall.SIGHUP)
	require.NoError(t, err, "Failed to send SIGHUP signal")
	t.Log("SIGHUP signal sent to trigger reload")

	// Step 8: Wait for reload to complete
	t.Log("=== Step 8: Waiting for policy reload ===")

	// Wait up to 5 seconds for the policy to be reloaded
	reloadCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	reloaded := false
	for i := 0; i < 50; i++ {
		updatedPolicy, err := policyEnforcer.GetPolicy(reloadCtx)
		require.NoError(t, err, "Failed to get policy during reload check")

		if !updatedPolicy.Images.AllowLatestTag {
			reloaded = true
			t.Logf("Policy reloaded successfully: allow_latest=%v", updatedPolicy.Images.AllowLatestTag)
			break
		}

		// Wait a bit before checking again
		time.Sleep(100 * time.Millisecond)
	}

	require.True(t, reloaded, "Policy was not reloaded within timeout")

	// Verify policy reloader state
	require.Equal(t, policy.ReloadStateIdle, policyReloader.State(), "Reloader should be in idle state after reload")
	require.False(t, policyReloader.IsReloading(), "Reloader should not be reloading")

	// Step 9: Attempt to create container that violates new policy
	t.Log("=== Step 9: Attempting to create container that violates new policy ===")

	containerName2 := "hotreload-test-2-" + time.Now().Format("20060102150405")
	createCfg2 := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:latest", // Now disallowed by updated policy
			Command: []string{"sleep", "300"},
			Env:     make(map[string]string),
			Labels:  make(map[string]string),
		},
		Name:        containerName2,
		SessionID:   sessionID,
		AutoPull:    false,
		PullTimeout: 1 * time.Minute,
	}

	_, err = creator.Create(ctx, createCfg2)
	require.Error(t, err, "Container creation with latest tag should fail after policy update")

	// Verify it's a permission error
	require.Contains(t, err.Error(), "PERMISSION", "Error should be a permission error")
	require.Contains(t, err.Error(), "policy", "Error should mention policy")
	t.Log("Container creation with latest tag correctly failed after hot-reload")

	// Step 10: Verify container with specific tag still works
	t.Log("=== Step 10: Verifying container with specific tag still works ===")

	containerName3 := "hotreload-test-3-" + time.Now().Format("20060102150405")
	createCfg3 := container.CreateConfig{
		Config: types.ContainerConfig{
			Image:   "alpine:3.18", // Specific tag, should still work
			Command: []string{"sleep", "300"},
			NetworkMode: "none",
			Env:         make(map[string]string),
			Labels:      make(map[string]string),
		},
		Name:        containerName3,
		SessionID:   sessionID,
		AutoPull:    false,
		PullTimeout: 5 * time.Minute,
	}
	createCfg3.Config.Labels["baaaht.session_id"] = sessionID.String()
	createCfg3.Config.Labels["baaaht.managed"] = "true"

	createResult3, err := creator.Create(ctx, createCfg3)
	require.NoError(t, err, "Container creation with specific tag should succeed")
	require.NotNil(t, createResult3, "Create result should not be nil")
	t.Logf("Container with specific tag created successfully: %s", createResult3.ContainerID)

	// Cleanup third container
	lifecycleMgr, _ := container.NewLifecycleManager(dockerClient, log)
	_ = lifecycleMgr.Destroy(ctx, container.DestroyConfig{
		ContainerID:   createResult3.ContainerID,
		Name:          containerName3,
		Force:         true,
		RemoveVolumes: true,
	})

	// Test complete
	t.Log("=== Policy Hot-Reload Test Complete ===")
	t.Log("All steps passed successfully:")
	t.Log("  1. Initial policy YAML file created")
	t.Log("  2. Config loaded with policy path")
	t.Log("  3. Orchestrator bootstrapped with policy hot-reload")
	t.Log("  4. Session created")
	t.Log("  5. Container with latest tag created (initial policy allowed it)")
	t.Log("  6. Policy YAML file modified (disallow latest tag)")
	t.Log("  7. SIGHUP signal sent to trigger reload")
	t.Log("  8. Policy reloaded successfully")
	t.Log("  9. Container with latest tag correctly rejected (new policy)")
	t.Log(" 10. Container with specific tag still allowed")
}
