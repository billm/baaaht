package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/policy"
)

// TestInitPolicyEnforcer tests that policy is loaded from YAML during orchestrator initialization
func TestInitPolicyEnforcer(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a test policy file
	policyContent := `
id: test-policy
name: Test Policy
description: A test policy for unit testing
mode: strict
quotas:
  max_cpus: 4000000000
  max_memory: 8589934592
  max_pids: 1024
mounts:
  allow_bind_mounts: false
  allow_volumes: true
  allow_tmpfs: true
  max_tmpfs_size: 268435456
network:
  allow_network: true
  allow_host_network: false
images:
  allow_latest_tag: false
security:
  allow_privileged: false
  require_non_root: false
  read_only_rootfs: false
`

	policyPath := filepath.Join(tmpDir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte(policyContent), 0644); err != nil {
		t.Fatalf("failed to write policy file: %v", err)
	}

	// Create config with policy config path
	policyCfg := config.DefaultPolicyConfig()
	policyCfg.ConfigPath = policyPath
	policyCfg.ReloadOnChanges = false

	cfg := config.Config{
		Runtime:      config.DefaultRuntimeConfig(),
		Docker:       config.DefaultDockerConfig(),
		APIServer:    config.DefaultAPIServerConfig(),
		Logging:      config.DefaultLoggingConfig(),
		Session:      config.DefaultSessionConfig(),
		Event:        config.DefaultEventConfig(),
		IPC:          config.DefaultIPCConfig(),
		Scheduler:    config.DefaultSchedulerConfig(),
		Credentials:  config.DefaultCredentialsConfig(),
		Policy:       policyCfg,
		Memory:       config.DefaultMemoryConfig(),
		Metrics:      config.DefaultMetricsConfig(),
		Tracing:      config.DefaultTracingConfig(),
		Orchestrator: config.DefaultOrchestratorConfig(),
		GRPC:         config.DefaultGRPCConfig(),
	}

	// Create a test logger
	log, err := logger.New(cfg.Logging)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	// Create orchestrator
	orch, err := New(cfg, log)
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}

	// Initialize orchestrator
	ctx := context.Background()
	if err := orch.Initialize(ctx); err != nil {
		t.Fatalf("failed to initialize orchestrator: %v", err)
	}
	defer orch.Close()

	// Verify the policy enforcer was initialized
	if orch.policyEnforcer == nil {
		t.Fatal("policy enforcer was not initialized")
	}

	// Verify the policy was loaded from the file
	policy, err := orch.policyEnforcer.GetPolicy(ctx)
	if err != nil {
		t.Fatalf("failed to get policy: %v", err)
	}

	if policy.ID != "test-policy" {
		t.Errorf("policy ID mismatch: got %s, want test-policy", policy.ID)
	}

	if policy.Name != "Test Policy" {
		t.Errorf("policy name mismatch: got %s, want Test Policy", policy.Name)
	}

	if policy.Mode != "strict" {
		t.Errorf("policy mode mismatch: got %s, want strict", policy.Mode)
	}

	if policy.Quotas.MaxCPUs == nil {
		t.Error("expected MaxCPUs to be set")
	} else if *policy.Quotas.MaxCPUs != 4000000000 {
		t.Errorf("MaxCPUs mismatch: got %d, want %d", *policy.Quotas.MaxCPUs, 4000000000)
	}
}

// TestInitPolicyEnforcerWithInvalidPath tests initialization with an invalid policy path
func TestInitPolicyEnforcerWithInvalidPath(t *testing.T) {
	// Create config with non-existent policy path
	policyCfg := config.DefaultPolicyConfig()
	policyCfg.ConfigPath = "/nonexistent/path/to/policy.yaml"

	cfg := config.Config{
		Runtime:      config.DefaultRuntimeConfig(),
		Docker:       config.DefaultDockerConfig(),
		APIServer:    config.DefaultAPIServerConfig(),
		Logging:      config.DefaultLoggingConfig(),
		Session:      config.DefaultSessionConfig(),
		Event:        config.DefaultEventConfig(),
		IPC:          config.DefaultIPCConfig(),
		Scheduler:    config.DefaultSchedulerConfig(),
		Credentials:  config.DefaultCredentialsConfig(),
		Policy:       policyCfg,
		Memory:       config.DefaultMemoryConfig(),
		Metrics:      config.DefaultMetricsConfig(),
		Tracing:      config.DefaultTracingConfig(),
		Orchestrator: config.DefaultOrchestratorConfig(),
		GRPC:         config.DefaultGRPCConfig(),
	}

	// Create a test logger
	log, err := logger.New(cfg.Logging)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	// Create orchestrator
	orch, err := New(cfg, log)
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}

	// Initialize orchestrator - should fail due to invalid policy path
	ctx := context.Background()
	err = orch.Initialize(ctx)
	if err == nil {
		orch.Close()
		t.Fatal("expected error when initializing with invalid policy path, got nil")
	}
}

// TestInitPolicyEnforcerWithInvalidYAML tests initialization with invalid YAML content
func TestInitPolicyEnforcerWithInvalidYAML(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create an invalid policy file
	policyPath := filepath.Join(tmpDir, "invalid-policy.yaml")
	if err := os.WriteFile(policyPath, []byte("invalid: yaml: content: ["), 0644); err != nil {
		t.Fatalf("failed to write policy file: %v", err)
	}

	// Create config with invalid policy path
	policyCfg := config.DefaultPolicyConfig()
	policyCfg.ConfigPath = policyPath

	cfg := config.Config{
		Runtime:      config.DefaultRuntimeConfig(),
		Docker:       config.DefaultDockerConfig(),
		APIServer:    config.DefaultAPIServerConfig(),
		Logging:      config.DefaultLoggingConfig(),
		Session:      config.DefaultSessionConfig(),
		Event:        config.DefaultEventConfig(),
		IPC:          config.DefaultIPCConfig(),
		Scheduler:    config.DefaultSchedulerConfig(),
		Credentials:  config.DefaultCredentialsConfig(),
		Policy:       policyCfg,
		Memory:       config.DefaultMemoryConfig(),
		Metrics:      config.DefaultMetricsConfig(),
		Tracing:      config.DefaultTracingConfig(),
		Orchestrator: config.DefaultOrchestratorConfig(),
		GRPC:         config.DefaultGRPCConfig(),
	}

	// Create a test logger
	log, err := logger.New(cfg.Logging)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	// Create orchestrator
	orch, err := New(cfg, log)
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}

	// Initialize orchestrator - should fail due to invalid YAML
	ctx := context.Background()
	err = orch.Initialize(ctx)
	if err == nil {
		orch.Close()
		t.Fatal("expected error when initializing with invalid YAML, got nil")
	}
}

// TestInitPolicyEnforcerWithoutConfigPath tests that a default policy file is created when it doesn't exist
func TestInitPolicyEnforcerWithoutConfigPath(t *testing.T) {
	// Create a temporary directory for a non-existent policy path
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "nonexistent-policy.yaml")

	// Verify the file doesn't exist initially
	if _, err := os.Stat(nonExistentPath); !os.IsNotExist(err) {
		t.Fatalf("expected policy file to not exist, but it does: %s", nonExistentPath)
	}

	// Use default policy config but with a non-existent path
	policyCfg := config.DefaultPolicyConfig()
	policyCfg.ConfigPath = nonExistentPath

	cfg := config.Config{
		Runtime:      config.DefaultRuntimeConfig(),
		Docker:       config.DefaultDockerConfig(),
		APIServer:    config.DefaultAPIServerConfig(),
		Logging:      config.DefaultLoggingConfig(),
		Session:      config.DefaultSessionConfig(),
		Event:        config.DefaultEventConfig(),
		IPC:          config.DefaultIPCConfig(),
		Scheduler:    config.DefaultSchedulerConfig(),
		Credentials:  config.DefaultCredentialsConfig(),
		Policy:       policyCfg,
		Memory:       config.DefaultMemoryConfig(),
		Metrics:      config.DefaultMetricsConfig(),
		Tracing:      config.DefaultTracingConfig(),
		Orchestrator: config.DefaultOrchestratorConfig(),
		GRPC:         config.DefaultGRPCConfig(),
	}

	// Create a test logger
	log, err := logger.New(cfg.Logging)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	// Create orchestrator
	orch, err := New(cfg, log)
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}
	defer orch.Close()

	// Initialize orchestrator - should succeed by creating default policy file
	ctx := context.Background()
	if err := orch.Initialize(ctx); err != nil {
		t.Fatalf("expected successful initialization with default policy file creation, got error: %v", err)
	}

	// Verify the policy file was created
	if _, err := os.Stat(nonExistentPath); os.IsNotExist(err) {
		t.Errorf("default policy file was not created at: %s", nonExistentPath)
	}

	// Verify the policy enforcer was initialized
	if orch.policyEnforcer == nil {
		t.Fatal("policy enforcer was not initialized")
	}

	// Verify the loaded policy matches the default policy
	loadedPolicy, err := orch.policyEnforcer.GetPolicy(ctx)
	if err != nil {
		t.Fatalf("failed to get policy: %v", err)
	}

	defaultPolicy := policy.DefaultPolicy()
	if loadedPolicy.ID != defaultPolicy.ID {
		t.Errorf("policy ID mismatch: got %s, want %s", loadedPolicy.ID, defaultPolicy.ID)
	}
	if loadedPolicy.Name != defaultPolicy.Name {
		t.Errorf("policy name mismatch: got %s, want %s", loadedPolicy.Name, defaultPolicy.Name)
	}
	if loadedPolicy.Mode != defaultPolicy.Mode {
		t.Errorf("policy mode mismatch: got %s, want %s", loadedPolicy.Mode, defaultPolicy.Mode)
	}
}
