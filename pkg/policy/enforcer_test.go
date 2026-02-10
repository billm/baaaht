package policy

import (
	"context"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestEnforcer creates a policy enforcer for testing
func createTestEnforcer(t *testing.T) *Enforcer {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	enforcer, err := NewDefault(log)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	return enforcer
}

// TestNewEnforcer tests creating a new policy enforcer
func TestNewEnforcer(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	cfg := config.DefaultPolicyConfig()

	enforcer, err := New(cfg, log)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}
	defer enforcer.Close()

	if enforcer == nil {
		t.Fatal("enforcer is nil")
	}
}

// TestNewEnforcerNilLogger tests creating an enforcer with nil logger
func TestNewEnforcerNilLogger(t *testing.T) {
	cfg := config.DefaultPolicyConfig()

	enforcer, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create enforcer with nil logger: %v", err)
	}
	defer enforcer.Close()

	if enforcer == nil {
		t.Fatal("enforcer is nil")
	}
}

// TestNewDefaultEnforcer tests creating an enforcer with default config
func TestNewDefaultEnforcer(t *testing.T) {
	enforcer, err := NewDefault(nil)
	if err != nil {
		t.Fatalf("failed to create default enforcer: %v", err)
	}
	defer enforcer.Close()

	if enforcer == nil {
		t.Fatal("enforcer is nil")
	}
}

// TestSetAndGetPolicy tests setting and getting policies
func TestSetAndGetPolicy(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()

	policy := &Policy{
		ID:          "test-policy",
		Name:        "Test Policy",
		Description: "A test policy",
		Mode:        EnforcementModeStrict,
		Quotas: ResourceQuota{
			MaxCPUs:   int64Ptr(2000000000), // 2 CPUs
			MaxMemory: int64Ptr(2147483648), // 2GB
		},
	}

	err := enforcer.SetPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}

	retrieved, err := enforcer.GetPolicy(ctx)
	if err != nil {
		t.Fatalf("failed to get policy: %v", err)
	}

	if retrieved.ID != policy.ID {
		t.Errorf("policy ID mismatch: got %s, want %s", retrieved.ID, policy.ID)
	}

	if retrieved.Mode != policy.Mode {
		t.Errorf("policy mode mismatch: got %s, want %s", retrieved.Mode, policy.Mode)
	}
}

// TestSetInvalidPolicy tests setting an invalid policy
func TestSetInvalidPolicy(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()

	policy := &Policy{
		ID:   "invalid",
		Name: "Invalid Policy",
		Mode: EnforcementMode("invalid"), // Invalid mode
	}

	err := enforcer.SetPolicy(ctx, policy)
	if err == nil {
		t.Error("expected error when setting invalid policy, got nil")
	}
}

// TestSetSessionPolicy tests setting session-specific policies
func TestSetSessionPolicy(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	policy := &Policy{
		ID:   "session-policy",
		Name: "Session Policy",
		Mode: EnforcementModePermissive,
		Quotas: ResourceQuota{
			MaxCPUs: int64Ptr(8000000000), // 8 CPUs
		},
	}

	err := enforcer.SetSessionPolicy(ctx, sessionID, policy)
	if err != nil {
		t.Fatalf("failed to set session policy: %v", err)
	}

	retrieved, err := enforcer.GetSessionPolicy(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session policy: %v", err)
	}

	// Should have merged with global policy
	if retrieved.Quotas.MaxCPUs == nil {
		t.Error("expected MaxCPUs to be set in merged policy")
	}
}

// TestRemoveSessionPolicy tests removing session-specific policies
func TestRemoveSessionPolicy(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	policy := &Policy{
		ID:   "session-policy",
		Name: "Session Policy",
		Mode: EnforcementModePermissive,
	}

	err := enforcer.SetSessionPolicy(ctx, sessionID, policy)
	if err != nil {
		t.Fatalf("failed to set session policy: %v", err)
	}

	err = enforcer.RemoveSessionPolicy(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to remove session policy: %v", err)
	}

	// Should now return global policy
	retrieved, err := enforcer.GetSessionPolicy(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session policy: %v", err)
	}

	if retrieved.ID != "default" {
		t.Errorf("expected default policy, got %s", retrieved.ID)
	}
}

// TestValidateContainerConfigQuotas tests quota validation
func TestValidateContainerConfigQuotas(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	tests := []struct {
		name          string
		config        types.ContainerConfig
		wantAllowed   bool
		wantViolation string
	}{
		{
			name: "valid quotas",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Resources: types.ResourceLimits{
					NanoCPUs:    1000000000, // 1 CPU
					MemoryBytes: 1073741824, // 1GB
				},
			},
			wantAllowed: true,
		},
		{
			name: "CPU exceeds maximum",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Resources: types.ResourceLimits{
					NanoCPUs: 8000000000, // 8 CPUs
				},
			},
			wantAllowed:   false,
			wantViolation: "quota.cpu.max",
		},
		{
			name: "memory exceeds maximum",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Resources: types.ResourceLimits{
					MemoryBytes: 17179869184, // 16GB
				},
			},
			wantAllowed:   false,
			wantViolation: "quota.memory.max",
		},
		{
			name: "PIDs exceeds maximum",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Resources: types.ResourceLimits{
					PidsLimit: int64Ptr(2048),
				},
			},
			wantAllowed:   false,
			wantViolation: "quota.pids.max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := enforcer.ValidateContainerConfig(ctx, sessionID, tt.config)
			if err != nil {
				t.Fatalf("validation failed: %v", err)
			}

			if result.Allowed != tt.wantAllowed {
				t.Errorf("allowed mismatch: got %v, want %v", result.Allowed, tt.wantAllowed)
			}

			if tt.wantViolation != "" {
				found := false
				for _, v := range result.Violations {
					if v.Rule == tt.wantViolation {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected violation %s not found", tt.wantViolation)
				}
			}
		})
	}
}

// TestValidateContainerConfigMounts tests mount validation
func TestValidateContainerConfigMounts(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	// Set a policy that allows bind mounts only from specific paths
	policy := DefaultPolicy()
	policy.Mounts.AllowBindMounts = true
	policy.Mounts.AllowedBindSources = []string{"/allowed/*"}
	policy.Mounts.DeniedBindSources = []string{"/forbidden/*"}

	err := enforcer.SetPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}

	tests := []struct {
		name          string
		config        types.ContainerConfig
		wantAllowed   bool
		wantViolation string
	}{
		{
			name: "allowed bind mount",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Mounts: []types.Mount{
					{
						Type:   types.MountTypeBind,
						Source: "/allowed/data",
						Target: "/data",
					},
				},
			},
			wantAllowed: true,
		},
		{
			name: "denied bind mount",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Mounts: []types.Mount{
					{
						Type:   types.MountTypeBind,
						Source: "/forbidden/data",
						Target: "/data",
					},
				},
			},
			wantAllowed:   false,
			wantViolation: "mount.bind.source_not_allowed",
		},
		{
			name: "bind mount disabled",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Mounts: []types.Mount{
					{
						Type:   types.MountTypeBind,
						Source: "/some/path",
						Target: "/data",
					},
				},
			},
			wantAllowed:   false,
			wantViolation: "mount.bind.disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset policy for bind mount disabled test
			if tt.wantViolation == "mount.bind.disabled" {
				policy := DefaultPolicy()
				policy.Mounts.AllowBindMounts = false
				enforcer.SetPolicy(ctx, policy)
			}

			result, err := enforcer.ValidateContainerConfig(ctx, sessionID, tt.config)
			if err != nil {
				t.Fatalf("validation failed: %v", err)
			}

			if result.Allowed != tt.wantAllowed {
				t.Errorf("allowed mismatch: got %v, want %v", result.Allowed, tt.wantAllowed)
			}

			if tt.wantViolation != "" {
				found := false
				for _, v := range result.Violations {
					if v.Rule == tt.wantViolation {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected violation %s not found", tt.wantViolation)
				}
			}
		})
	}
}

// TestValidateContainerConfigImage tests image validation
func TestValidateContainerConfigImage(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	policy := DefaultPolicy()
	policy.Images.AllowLatestTag = false
	policy.Images.AllowedImages = []string{"docker.io/library/*"}
	policy.Images.DeniedImages = []string{"*:unstable"}

	err := enforcer.SetPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}

	tests := []struct {
		name          string
		image         string
		wantAllowed   bool
		wantViolation string
	}{
		{
			name:        "allowed image with tag",
			image:       "docker.io/library/nginx:1.21",
			wantAllowed: true,
		},
		{
			name:          "latest tag not allowed",
			image:         "nginx:latest",
			wantAllowed:   false,
			wantViolation: "image.latest_tag",
		},
		{
			name:          "no tag not allowed",
			image:         "nginx",
			wantAllowed:   false,
			wantViolation: "image.latest_tag",
		},
		{
			name:          "denied image",
			image:         "nginx:unstable",
			wantAllowed:   false,
			wantViolation: "image.denied",
		},
		{
			name:          "not in allowed list",
			image:         "gcr.io/distroless/base",
			wantAllowed:   false,
			wantViolation: "image.not_allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := types.ContainerConfig{
				Image: tt.image,
			}

			result, err := enforcer.ValidateContainerConfig(ctx, sessionID, config)
			if err != nil {
				t.Fatalf("validation failed: %v", err)
			}

			if result.Allowed != tt.wantAllowed {
				t.Errorf("allowed mismatch: got %v, want %v", result.Allowed, tt.wantAllowed)
			}

			if tt.wantViolation != "" {
				found := false
				for _, v := range result.Violations {
					if v.Rule == tt.wantViolation {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected violation %s not found", tt.wantViolation)
				}
			}
		})
	}
}

// TestEnforceContainerConfig tests enforcing policies on container configs
func TestEnforceContainerConfig(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	config := types.ContainerConfig{
		Image:     "nginx:1.21",
		Resources: types.ResourceLimits{
			// No quotas set
		},
	}

	enforced, err := enforcer.EnforceContainerConfig(ctx, sessionID, config)
	if err != nil {
		t.Fatalf("enforcement failed: %v", err)
	}

	// Should have default quotas applied
	if enforced.Resources.NanoCPUs == 0 {
		t.Error("expected NanoCPUs to be set")
	}
	if enforced.Resources.MemoryBytes == 0 {
		t.Error("expected MemoryBytes to be set")
	}
	if enforced.Resources.PidsLimit == nil {
		t.Error("expected PidsLimit to be set")
	}
}

// TestEnforcementModePermissive tests permissive enforcement mode
func TestEnforcementModePermissive(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	policy := DefaultPolicy()
	policy.Mode = EnforcementModePermissive
	policy.Quotas.MaxCPUs = int64Ptr(1000000000) // 1 CPU

	err := enforcer.SetPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}

	config := types.ContainerConfig{
		Image: "nginx:latest",
		Resources: types.ResourceLimits{
			NanoCPUs: 8000000000, // 8 CPUs - exceeds max
		},
	}

	result, err := enforcer.ValidateContainerConfig(ctx, sessionID, config)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Permissive mode should still allow but with violations
	if !result.Allowed {
		t.Error("permissive mode should allow config with violations")
	}

	if len(result.Violations) == 0 {
		t.Error("expected violations in permissive mode")
	}
}

// TestEnforcementModeDisabled tests disabled enforcement mode
func TestEnforcementModeDisabled(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	policy := DefaultPolicy()
	policy.Mode = EnforcementModeDisabled

	err := enforcer.SetPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}

	config := types.ContainerConfig{
		Image: "anything:anything",
		Resources: types.ResourceLimits{
			NanoCPUs: 999999999999, // Way over limit
		},
	}

	result, err := enforcer.ValidateContainerConfig(ctx, sessionID, config)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Disabled mode should allow everything
	if !result.Allowed {
		t.Error("disabled mode should allow all configs")
	}

	if len(result.Violations) > 0 {
		t.Error("disabled mode should not produce violations")
	}
}

// TestEnforcerClose tests closing the enforcer
func TestEnforcerClose(t *testing.T) {
	enforcer := createTestEnforcer(t)

	ctx := context.Background()

	err := enforcer.Close()
	if err != nil {
		t.Fatalf("failed to close enforcer: %v", err)
	}

	// Operations should fail after close
	_, err = enforcer.GetPolicy(ctx)
	if err == nil {
		t.Error("expected error when getting policy from closed enforcer")
	}
}

// TestEnforcerStats tests getting enforcer statistics
func TestEnforcerStats(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	policy := &Policy{
		ID:   "session-policy",
		Name: "Session Policy",
		Mode: EnforcementModePermissive,
	}

	err := enforcer.SetSessionPolicy(ctx, sessionID, policy)
	if err != nil {
		t.Fatalf("failed to set session policy: %v", err)
	}

	stats := enforcer.Stats(ctx)
	if stats != 1 {
		t.Errorf("expected 1 active session, got %d", stats)
	}
}

// TestPolicyMerge tests merging policies
func TestPolicyMerge(t *testing.T) {
	base := DefaultPolicy()
	override := &Policy{
		ID:   "override",
		Name: "Override Policy",
		Mode: EnforcementModePermissive,
		Quotas: ResourceQuota{
			MaxCPUs: int64Ptr(16000000000), // 16 CPUs
		},
	}

	merged := base.Merge(override)

	if merged.Quotas.MaxCPUs == nil {
		t.Error("expected MaxCPUs to be set in merged policy")
	} else if *merged.Quotas.MaxCPUs != 16000000000 {
		t.Errorf("MaxCPUs mismatch: got %d, want %d", *merged.Quotas.MaxCPUs, 16000000000)
	}

	if merged.Mode != EnforcementModePermissive {
		t.Errorf("mode mismatch: got %s, want %s", merged.Mode, EnforcementModePermissive)
	}
}

// TestValidatePolicy tests policy validation
func TestValidatePolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  *Policy
		wantErr bool
	}{
		{
			name: "valid policy",
			policy: &Policy{
				ID:   "valid",
				Name: "Valid Policy",
				Mode: EnforcementModeStrict,
			},
			wantErr: false,
		},
		{
			name: "invalid mode",
			policy: &Policy{
				ID:   "invalid",
				Name: "Invalid Policy",
				Mode: EnforcementMode("invalid"),
			},
			wantErr: true,
		},
		{
			name: "min CPU exceeds max",
			policy: &Policy{
				ID:   "invalid",
				Name: "Invalid Policy",
				Mode: EnforcementModeStrict,
				Quotas: ResourceQuota{
					MinCPUs: int64Ptr(8000000000),
					MaxCPUs: int64Ptr(4000000000),
				},
			},
			wantErr: true,
		},
		{
			name: "min memory exceeds max",
			policy: &Policy{
				ID:   "invalid",
				Name: "Invalid Policy",
				Mode: EnforcementModeStrict,
				Quotas: ResourceQuota{
					MinMemory: int64Ptr(17179869184),
					MaxMemory: int64Ptr(1073741824),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestMatchPattern tests pattern matching
func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"exact", "exact", true},
		{"*", "anything", true},
		{"prefix*", "prefixvalue", true},
		{"*suffix", "valuesuffix", true},
		{"*middle*", "premiddlepost", true},
		{"docker.io/*", "docker.io/library/nginx", true},
		{"docker.io/*", "gcr.io/distroless/base", false},
		{"*", "anything", true},
		{"", "", true},
		{"", "not-empty", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.s, func(t *testing.T) {
			got := matchPattern(tt.s, tt.pattern)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.s, tt.pattern, got, tt.want)
			}
		})
	}
}

// TestIsPathAllowed tests path allow/deny logic
func TestIsPathAllowed(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		allowed []string
		denied  []string
		want    bool
	}{
		{
			name:    "no restrictions",
			path:    "/any/path",
			allowed: []string{},
			denied:  []string{},
			want:    true,
		},
		{
			name:    "allowed pattern matches",
			path:    "/allowed/data",
			allowed: []string{"/allowed/*"},
			denied:  []string{},
			want:    true,
		},
		{
			name:    "allowed pattern does not match",
			path:    "/other/data",
			allowed: []string{"/allowed/*"},
			denied:  []string{},
			want:    false,
		},
		{
			name:    "denied takes precedence",
			path:    "/allowed/data",
			allowed: []string{"/allowed/*"},
			denied:  []string{"/allowed/data"},
			want:    false,
		},
		{
			name:    "denied pattern",
			path:    "/etc/passwd",
			allowed: []string{},
			denied:  []string{"/etc/*"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPathAllowed(tt.path, tt.allowed, tt.denied)
			if got != tt.want {
				t.Errorf("isPathAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseTmpfsSize tests tmpfs size parsing
func TestParseTmpfsSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"100", 100},
		{"1k", 1024},
		{"1m", 1024 * 1024},
		{"1g", 1024 * 1024 * 1024},
		{"512m", 512 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTmpfsSize(tt.input)
			if got != tt.want {
				t.Errorf("parseTmpfsSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestEnforcerString tests string representation
func TestEnforcerString(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	s := enforcer.String()
	if s == "" {
		t.Error("string representation should not be empty")
	}

	// Should contain mode
	if !containsSubstring(s, "mode:") {
		t.Error("string representation should contain mode")
	}
}

// BenchmarkValidateConfig benchmarks configuration validation
func BenchmarkValidateConfig(b *testing.B) {
	log, _ := logger.NewDefault()
	enforcer, _ := NewDefault(log)
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	config := types.ContainerConfig{
		Image: "nginx:1.21",
		Resources: types.ResourceLimits{
			NanoCPUs:    1000000000,
			MemoryBytes: 1073741824,
		},
		Mounts: []types.Mount{
			{
				Type:   types.MountTypeVolume,
				Source: "data",
				Target: "/data",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = enforcer.ValidateContainerConfig(ctx, sessionID, config)
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestEnforceMountMode tests enforcement of read-only mode on mounts based on allowlist
func TestEnforceMountMode(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	enforcer, err := NewDefault(log)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	// Create a policy with mount allowlist entries
	policy := DefaultPolicy()
	policy.Mode = EnforcementModeStrict
	policy.Mounts.AllowBindMounts = true
	policy.Mounts.MountAllowlist = []MountAllowlistEntry{
		{
			Path: "/readonly/data",
			Mode: MountAccessModeReadOnly,
			User: "testuser",
		},
		{
			Path: "/readwrite/data",
			Mode: MountAccessModeReadWrite,
			User: "testuser",
		},
	}

	err = enforcer.SetPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}

	tests := []struct {
		name              string
		config            types.ContainerConfig
		wantReadOnlyIndex int   // Index of mount that should be read-only, -1 if none
		wantReadWrite     bool  // Whether a mount should remain read-write
	}{
		{
			name: "enforce read-only on readonly path",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Labels: map[string]string{
					"username": "testuser",
				},
				Mounts: []types.Mount{
					{
						Type:     types.MountTypeBind,
						Source:   "/readonly/data",
						Target:   "/data",
						ReadOnly: false, // User requested read-write
					},
				},
			},
			wantReadOnlyIndex: 0,
		},
		{
			name: "allow read-write on readwrite path",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Labels: map[string]string{
					"username": "testuser",
				},
				Mounts: []types.Mount{
					{
						Type:     types.MountTypeBind,
						Source:   "/readwrite/data",
						Target:   "/data",
						ReadOnly: false,
					},
				},
			},
			wantReadOnlyIndex: -1,
			wantReadWrite:     true,
		},
		{
			name: "enforce read-only even when already set",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Labels: map[string]string{
					"username": "testuser",
				},
				Mounts: []types.Mount{
					{
						Type:     types.MountTypeBind,
						Source:   "/readonly/data",
						Target:   "/data",
						ReadOnly: true,
					},
				},
			},
			wantReadOnlyIndex: 0,
		},
		{
			name: "multiple mounts with mixed modes",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Labels: map[string]string{
					"username": "testuser",
				},
				Mounts: []types.Mount{
					{
						Type:     types.MountTypeBind,
						Source:   "/readonly/data",
						Target:   "/ro",
						ReadOnly: false,
					},
					{
						Type:     types.MountTypeBind,
						Source:   "/readwrite/data",
						Target:   "/rw",
						ReadOnly: false,
					},
				},
			},
			wantReadOnlyIndex: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enforced, err := enforcer.EnforceContainerConfig(ctx, sessionID, tt.config)
			if err != nil {
				t.Fatalf("enforcement failed: %v", err)
			}

			if tt.wantReadOnlyIndex >= 0 {
				if tt.wantReadOnlyIndex >= len(enforced.Mounts) {
					t.Errorf("mount index %d out of range", tt.wantReadOnlyIndex)
					return
				}
				if !enforced.Mounts[tt.wantReadOnlyIndex].ReadOnly {
					t.Errorf("mount at index %d should be read-only", tt.wantReadOnlyIndex)
				}
			}

			if tt.wantReadWrite {
				// Find a mount that should be read-write
				foundRW := false
				for _, m := range enforced.Mounts {
					if m.Source == "/readwrite/data" && !m.ReadOnly {
						foundRW = true
						break
					}
				}
				if !foundRW {
					t.Error("expected at least one mount to be read-write")
				}
			}
		})
	}
}

// TestEnforceMountModeDisabledMode tests that enforcement is skipped in disabled mode
func TestEnforceMountModeDisabledMode(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	enforcer, err := NewDefault(log)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}
	defer enforcer.Close()

	ctx := context.Background()
	sessionID := types.GenerateID()

	// Create a policy with mount allowlist but disabled enforcement
	policy := DefaultPolicy()
	policy.Mode = EnforcementModeDisabled
	policy.Mounts.AllowBindMounts = true
	policy.Mounts.MountAllowlist = []MountAllowlistEntry{
		{
			Path: "/readonly/data",
			Mode: MountAccessModeReadOnly,
			User: "testuser",
		},
	}

	err = enforcer.SetPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}

	config := types.ContainerConfig{
		Image: "nginx:1.21",
		Labels: map[string]string{
			"username": "testuser",
		},
		Mounts: []types.Mount{
			{
				Type:     types.MountTypeBind,
				Source:   "/readonly/data",
				Target:   "/data",
				ReadOnly: false, // User requested read-write
			},
		},
	}

	enforced, err := enforcer.EnforceContainerConfig(ctx, sessionID, config)
	if err != nil {
		t.Fatalf("enforcement failed: %v", err)
	}

	// In disabled mode, the mount should remain as the user requested
	if enforced.Mounts[0].ReadOnly {
		t.Error("in disabled mode, mount should not be forced to read-only")
	}
}

// TestSetAuditLogger tests setting a custom audit logger on the enforcer
func TestSetAuditLogger(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	log, err := logger.NewDefault()
	require.NoError(t, err)

	// Create a custom audit logger
	customAuditLogger, err := NewAuditLogger(log, "")
	require.NoError(t, err)

	// Set the custom audit logger
	err = enforcer.SetAuditLogger(customAuditLogger)
	require.NoError(t, err, "setting audit logger should succeed")

	// Get the audit logger to verify it was set
	retrievedLogger := enforcer.GetAuditLogger()
	require.NotNil(t, retrievedLogger, "audit logger should not be nil")
	assert.Same(t, customAuditLogger, retrievedLogger, "retrieved logger should be the same as the one set")
}

// TestSetAuditLoggerOnClosedEnforcer tests that setting audit logger on closed enforcer fails
func TestSetAuditLoggerOnClosedEnforcer(t *testing.T) {
	enforcer := createTestEnforcer(t)
	enforcer.Close()

	log, err := logger.NewDefault()
	require.NoError(t, err)

	customAuditLogger, err := NewAuditLogger(log, "")
	require.NoError(t, err)

	err = enforcer.SetAuditLogger(customAuditLogger)
	assert.Error(t, err, "setting audit logger on closed enforcer should fail")
	var customErr *types.Error
	assert.ErrorAs(t, err, &customErr)
	assert.Equal(t, types.ErrCodeUnavailable, customErr.Code)
}

// TestSetGroupProvider tests setting a group membership provider on the enforcer
func TestSetGroupProvider(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	// Create a mock group provider
	mockProvider := &mockGroupProvider{
		groups: map[string][]string{
			"testuser": {"developers"},
		},
	}

	// Set the group provider
	err := enforcer.SetGroupProvider(mockProvider)
	require.NoError(t, err, "setting group provider should succeed")
}

// TestSetGroupProviderOnClosedEnforcer tests that setting group provider on closed enforcer fails
func TestSetGroupProviderOnClosedEnforcer(t *testing.T) {
	enforcer := createTestEnforcer(t)
	enforcer.Close()

	mockProvider := &mockGroupProvider{}

	err := enforcer.SetGroupProvider(mockProvider)
	assert.Error(t, err, "setting group provider on closed enforcer should fail")
	var customErr *types.Error
	assert.ErrorAs(t, err, &customErr)
	assert.Equal(t, types.ErrCodeUnavailable, customErr.Code)
}

// TestGetAuditLoggerReturnsNilForNewEnforcer tests that GetAuditLogger returns the default audit logger
func TestGetAuditLoggerReturnsDefaultLogger(t *testing.T) {
	enforcer := createTestEnforcer(t)
	defer enforcer.Close()

	// Get the default audit logger
	retrievedLogger := enforcer.GetAuditLogger()
	require.NotNil(t, retrievedLogger, "audit logger should not be nil for new enforcer")
}
