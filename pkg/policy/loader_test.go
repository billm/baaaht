package policy

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadPolicyFromFile tests loading a valid policy from a YAML file
func TestLoadPolicyFromFile(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a valid policy file
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

	// Load the policy
	policy, err := LoadFromFile(policyPath)
	if err != nil {
		t.Fatalf("failed to load policy from file: %v", err)
	}

	// Verify the policy was loaded correctly
	if policy.ID != "test-policy" {
		t.Errorf("policy ID mismatch: got %s, want test-policy", policy.ID)
	}

	if policy.Name != "Test Policy" {
		t.Errorf("policy name mismatch: got %s, want Test Policy", policy.Name)
	}

	if policy.Mode != EnforcementModeStrict {
		t.Errorf("policy mode mismatch: got %s, want %s", policy.Mode, EnforcementModeStrict)
	}

	if policy.Quotas.MaxCPUs == nil {
		t.Error("expected MaxCPUs to be set")
	} else if *policy.Quotas.MaxCPUs != 4000000000 {
		t.Errorf("MaxCPUs mismatch: got %d, want %d", *policy.Quotas.MaxCPUs, 4000000000)
	}

	if policy.Quotas.MaxMemory == nil {
		t.Error("expected MaxMemory to be set")
	} else if *policy.Quotas.MaxMemory != 8589934592 {
		t.Errorf("MaxMemory mismatch: got %d, want %d", *policy.Quotas.MaxMemory, 8589934592)
	}
}

// TestLoadPolicyFromFileNotFound tests loading a policy from a non-existent file
func TestLoadPolicyFromFileNotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/to/policy.yaml")
	if err == nil {
		t.Fatal("expected error when loading non-existent file, got nil")
	}
}

// TestLoadPolicyFromFileInvalidPath tests loading with invalid file paths
func TestLoadPolicyFromFileInvalidPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "wrong extension - .txt",
			path:    "/tmp/policy.txt",
			wantErr: true,
		},
		{
			name:    "wrong extension - .json",
			path:    "/tmp/policy.json",
			wantErr: true,
		},
		{
			name:    "no extension",
			path:    "/tmp/policy",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFromFile(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFromFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLoadPolicyFromFileEmptyFile tests loading an empty policy file
func TestLoadPolicyFromFileEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	emptyPath := filepath.Join(tmpDir, "empty.yaml")

	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	_, err := LoadFromFile(emptyPath)
	if err == nil {
		t.Fatal("expected error when loading empty file, got nil")
	}
}

// TestLoadPolicyFromFileInvalidYAML tests loading a file with invalid YAML syntax
func TestLoadPolicyFromFileInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidYAML := `
id: test-policy
name: Test
quotas:
  max_cpus: [invalid
`

	if err := os.WriteFile(invalidPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write invalid YAML file: %v", err)
	}

	_, err := LoadFromFile(invalidPath)
	if err == nil {
		t.Fatal("expected error when loading invalid YAML, got nil")
	}
}

// TestLoadPolicyFromFileDefaults tests that defaults are applied correctly
func TestLoadPolicyFromFileDefaults(t *testing.T) {
	tmpDir := t.TempDir()

	// Minimal policy with no optional fields
	policyContent := `
name: Minimal Policy
quotas:
  max_cpus: 1000000000
`

	policyPath := filepath.Join(tmpDir, "minimal.yaml")
	if err := os.WriteFile(policyPath, []byte(policyContent), 0644); err != nil {
		t.Fatalf("failed to write minimal policy file: %v", err)
	}

	policy, err := LoadFromFile(policyPath)
	if err != nil {
		t.Fatalf("failed to load minimal policy: %v", err)
	}

	// Check defaults
	if policy.ID != "default" {
		t.Errorf("expected default ID 'default', got %s", policy.ID)
	}

	if policy.Mode != EnforcementModeStrict {
		t.Errorf("expected default mode %s, got %s", EnforcementModeStrict, policy.Mode)
	}
}

// TestLoadPolicyFromFileWithEnvVars tests environment variable interpolation
func TestLoadPolicyFromFileWithEnvVars(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_POLICY_ID", "env-policy")
	os.Setenv("TEST_POLICY_NAME", "Env Test Policy")
	defer os.Unsetenv("TEST_POLICY_ID")
	defer os.Unsetenv("TEST_POLICY_NAME")

	tmpDir := t.TempDir()

	policyContent := `
id: ${TEST_POLICY_ID}
name: ${TEST_POLICY_NAME}
description: Policy with env vars
quotas:
  max_cpus: 1000000000
`

	policyPath := filepath.Join(tmpDir, "env-policy.yaml")
	if err := os.WriteFile(policyPath, []byte(policyContent), 0644); err != nil {
		t.Fatalf("failed to write policy file: %v", err)
	}

	policy, err := LoadFromFile(policyPath)
	if err != nil {
		t.Fatalf("failed to load policy with env vars: %v", err)
	}

	if policy.ID != "env-policy" {
		t.Errorf("expected interpolated ID 'env-policy', got %s", policy.ID)
	}

	if policy.Name != "Env Test Policy" {
		t.Errorf("expected interpolated name 'Env Test Policy', got %s", policy.Name)
	}
}

// TestLoadPolicyFromFileEnvVarWithDefault tests env var interpolation with defaults
func TestLoadPolicyFromFileEnvVarWithDefault(t *testing.T) {
	// Don't set the env var, should use default
	_ = os.Unsetenv("NONEXISTENT_VAR")

	tmpDir := t.TempDir()

	policyContent := `
id: test-policy
name: ${NONEXISTENT_VAR:-Default Name}
quotas:
  max_cpus: 1000000000
`

	policyPath := filepath.Join(tmpDir, "default-policy.yaml")
	if err := os.WriteFile(policyPath, []byte(policyContent), 0644); err != nil {
		t.Fatalf("failed to write policy file: %v", err)
	}

	policy, err := LoadFromFile(policyPath)
	if err != nil {
		t.Fatalf("failed to load policy with default env var: %v", err)
	}

	if policy.Name != "Default Name" {
		t.Errorf("expected default name 'Default Name', got %s", policy.Name)
	}
}

// TestLoadFromBytes tests loading policy from bytes
func TestLoadFromBytes(t *testing.T) {
	policyContent := `
id: bytes-policy
name: Bytes Policy
quotas:
  max_cpus: 2000000000
`

	policy, err := LoadFromBytes([]byte(policyContent))
	if err != nil {
		t.Fatalf("failed to load policy from bytes: %v", err)
	}

	if policy.ID != "bytes-policy" {
		t.Errorf("policy ID mismatch: got %s, want bytes-policy", policy.ID)
	}
}

// TestLoadPolicyFromFileWithLists tests loading policy with list fields
func TestLoadPolicyFromFileWithLists(t *testing.T) {
	tmpDir := t.TempDir()

	policyContent := `
id: lists-policy
name: Lists Policy
quotas:
  max_cpus: 1000000000
mounts:
  allow_bind_mounts: true
  allowed_bind_sources:
    - /allowed/*
    - /safe/*
  denied_bind_sources:
    - /etc/*
  allow_volumes: true
  allowed_volumes:
    - data
    - cache
network:
  allowed_networks:
    - 192.168.0.0/16
    - 10.0.0.0/8
images:
  allowed_images:
    - docker.io/library/*
    - gcr.io/distroless/*
  denied_images:
    - "*:latest"
security:
  allowed_capabilities:
    - CAP_NET_BIND_SERVICE
  add_capabilities:
    - CAP_CHOWN
`

	policyPath := filepath.Join(tmpDir, "lists.yaml")
	if err := os.WriteFile(policyPath, []byte(policyContent), 0644); err != nil {
		t.Fatalf("failed to write policy file: %v", err)
	}

	policy, err := LoadFromFile(policyPath)
	if err != nil {
		t.Fatalf("failed to load policy with lists: %v", err)
	}

	// Check mounts lists
	if len(policy.Mounts.AllowedBindSources) != 2 {
		t.Errorf("expected 2 allowed bind sources, got %d", len(policy.Mounts.AllowedBindSources))
	}

	if len(policy.Mounts.DeniedBindSources) != 1 {
		t.Errorf("expected 1 denied bind source, got %d", len(policy.Mounts.DeniedBindSources))
	}

	// Check network lists
	if len(policy.Network.AllowedNetworks) != 2 {
		t.Errorf("expected 2 allowed networks, got %d", len(policy.Network.AllowedNetworks))
	}

	// Check image lists
	if len(policy.Images.AllowedImages) != 2 {
		t.Errorf("expected 2 allowed images, got %d", len(policy.Images.AllowedImages))
	}

	if len(policy.Images.DeniedImages) != 1 {
		t.Errorf("expected 1 denied image, got %d", len(policy.Images.DeniedImages))
	}

	// Check security lists
	if len(policy.Security.AllowedCapabilities) != 1 {
		t.Errorf("expected 1 allowed capability, got %d", len(policy.Security.AllowedCapabilities))
	}

	if len(policy.Security.AddCapabilities) != 1 {
		t.Errorf("expected 1 added capability, got %d", len(policy.Security.AddCapabilities))
	}
}

// TestLoadPolicyFromFileInvalidPolicy tests loading a file with invalid policy data
func TestLoadPolicyFromFileInvalidPolicy(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		content     string
		description string
	}{
		{
			name: "invalid enforcement mode",
			content: `
id: test
name: Test
mode: invalid_mode
quotas:
  max_cpus: 1000000000
`,
			description: "policy with invalid enforcement mode",
		},
		{
			name: "min CPU exceeds max CPU",
			content: `
id: test
name: Test
quotas:
  min_cpus: 8000000000
  max_cpus: 4000000000
`,
			description: "policy with min CPU > max CPU",
		},
		{
			name: "min memory exceeds max memory",
			content: `
id: test
name: Test
quotas:
  min_memory: 17179869184
  max_memory: 1073741824
`,
			description: "policy with min memory > max memory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyPath := filepath.Join(tmpDir, tt.name+".yaml")
			if err := os.WriteFile(policyPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write policy file: %v", err)
			}

			_, err := LoadFromFile(policyPath)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tt.description)
			}
		})
	}
}

// TestLoadPolicyFromFileYmlExtension tests loading a policy with .yml extension
func TestLoadPolicyFromFileYmlExtension(t *testing.T) {
	tmpDir := t.TempDir()

	policyContent := `
id: yml-policy
name: YML Policy
quotas:
  max_cpus: 1000000000
`

	policyPath := filepath.Join(tmpDir, "policy.yml")
	if err := os.WriteFile(policyPath, []byte(policyContent), 0644); err != nil {
		t.Fatalf("failed to write policy file: %v", err)
	}

	policy, err := LoadFromFile(policyPath)
	if err != nil {
		t.Fatalf("failed to load .yml policy file: %v", err)
	}

	if policy.ID != "yml-policy" {
		t.Errorf("policy ID mismatch: got %s, want yml-policy", policy.ID)
	}
}

// TestCreateDefaultPolicyFile tests that a default policy file is created when it doesn't exist
func TestCreateDefaultPolicyFile(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "nonexistent.yaml")

	// Verify the file doesn't exist
	if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
		t.Fatalf("expected file to not exist, but it does: %s", policyPath)
	}

	// Load the policy (should create default file)
	policy, err := LoadFromFile(policyPath)
	if err != nil {
		t.Fatalf("failed to load policy: %v", err)
	}

	// Verify the file was created
	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		t.Fatal("default policy file was not created")
	}

	// Verify the policy matches the default
	defaultPolicy := DefaultPolicy()
	if policy.ID != defaultPolicy.ID {
		t.Errorf("policy ID mismatch: got %s, want %s", policy.ID, defaultPolicy.ID)
	}

	if policy.Name != defaultPolicy.Name {
		t.Errorf("policy name mismatch: got %s, want %s", policy.Name, defaultPolicy.Name)
	}

	if policy.Mode != defaultPolicy.Mode {
		t.Errorf("policy mode mismatch: got %s, want %s", policy.Mode, defaultPolicy.Mode)
	}

	// Verify quotas
	if policy.Quotas.MaxCPUs == nil || defaultPolicy.Quotas.MaxCPUs == nil {
		t.Error("expected MaxCPUs to be set")
	} else if *policy.Quotas.MaxCPUs != *defaultPolicy.Quotas.MaxCPUs {
		t.Errorf("MaxCPUs mismatch: got %d, want %d", *policy.Quotas.MaxCPUs, *defaultPolicy.Quotas.MaxCPUs)
	}

	// Verify mounts
	if policy.Mounts.AllowBindMounts != defaultPolicy.Mounts.AllowBindMounts {
		t.Errorf("AllowBindMounts mismatch: got %v, want %v", policy.Mounts.AllowBindMounts, defaultPolicy.Mounts.AllowBindMounts)
	}

	// Verify network
	if policy.Network.AllowNetwork != defaultPolicy.Network.AllowNetwork {
		t.Errorf("AllowNetwork mismatch: got %v, want %v", policy.Network.AllowNetwork, defaultPolicy.Network.AllowNetwork)
	}

	// Verify security
	if policy.Security.AllowPrivileged != defaultPolicy.Security.AllowPrivileged {
		t.Errorf("AllowPrivileged mismatch: got %v, want %v", policy.Security.AllowPrivileged, defaultPolicy.Security.AllowPrivileged)
	}
}
