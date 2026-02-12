package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/logger"
)

// mockGroupProvider is a mock implementation of GroupMembershipProvider for testing
type mockGroupProvider struct {
	groups map[string][]string
	err    error
}

// GetUserGroups returns the list of group names for a given user
func (m *mockGroupProvider) GetUserGroups(ctx context.Context, username string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	if groups, ok := m.groups[username]; ok {
		return groups, nil
	}
	return []string{}, nil
}

// createTestResolver creates a mount allowlist resolver for testing
func createTestResolver(t *testing.T) *MountAllowlistResolver {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	policy := &Policy{
		ID:          "test-policy",
		Name:        "Test Policy",
		Description: "A test policy",
		Mode:        EnforcementModeStrict,
		Mounts: MountPolicy{
			MountAllowlist: []MountAllowlistEntry{
				{
					Path: "/home/shared/data",
					Mode: MountAccessModeReadOnly,
				},
				{
					Path: "/home/alice/projects",
					User: "alice",
					Mode: MountAccessModeReadWrite,
				},
				{
					Path: "/shared/team-data",
					Group: "developers",
					Mode: MountAccessModeReadWrite,
				},
			},
		},
	}

	resolver, err := NewMountAllowlistResolver(policy, log)
	if err != nil {
		t.Fatalf("failed to create resolver: %v", err)
	}

	return resolver
}

// TestGroupMembership tests the group membership provider interface
func TestGroupMembership(t *testing.T) {
	t.Run("SetGroupProvider sets provider correctly", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		provider := &mockGroupProvider{
			groups: map[string][]string{
				"alice": {"developers", "admins"},
				"bob":   {"developers"},
			},
		}

		err := resolver.SetGroupProvider(provider)
		if err != nil {
			t.Fatalf("failed to set group provider: %v", err)
		}
	})

	t.Run("SetGroupProvider fails when resolver is closed", func(t *testing.T) {
		resolver := createTestResolver(t)
		resolver.Close()

		provider := &mockGroupProvider{
			groups: map[string][]string{},
		}

		err := resolver.SetGroupProvider(provider)
		if err == nil {
			t.Fatal("expected error when setting provider on closed resolver")
		}
	})

	t.Run("GetUserGroups returns correct groups", func(t *testing.T) {
		provider := &mockGroupProvider{
			groups: map[string][]string{
				"alice":  {"developers", "admins"},
				"bob":    {"developers"},
				"charlie": {"ops", "admins"},
			},
		}

		ctx := context.Background()

		groups, err := provider.GetUserGroups(ctx, "alice")
		if err != nil {
			t.Fatalf("failed to get groups: %v", err)
		}

		if len(groups) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(groups))
		}

		if !containsString(groups, "developers") {
			t.Error("expected 'developers' in groups")
		}

		if !containsString(groups, "admins") {
			t.Error("expected 'admins' in groups")
		}
	})

	t.Run("GetUserGroups returns empty list for unknown user", func(t *testing.T) {
		provider := &mockGroupProvider{
			groups: map[string][]string{
				"alice": {"developers"},
			},
		}

		ctx := context.Background()

		groups, err := provider.GetUserGroups(ctx, "unknown")
		if err != nil {
			t.Fatalf("failed to get groups: %v", err)
		}

		if len(groups) != 0 {
			t.Fatalf("expected 0 groups for unknown user, got %d", len(groups))
		}
	})

	t.Run("GetUserGroups returns error when provider fails", func(t *testing.T) {
		expectedErr := errors.New("provider error")
		provider := &mockGroupProvider{
			err: expectedErr,
		}

		ctx := context.Background()

		groups, err := provider.GetUserGroups(ctx, "alice")
		if err != expectedErr {
			t.Fatalf("expected error '%v', got '%v'", expectedErr, err)
		}

		if groups != nil {
			t.Fatal("expected nil groups on error")
		}
	})
}

// TestMountAllowlistResolver tests the resolver functionality
func TestMountAllowlistResolver(t *testing.T) {
	t.Run("NewMountAllowlistResolver creates resolver", func(t *testing.T) {
		log, err := logger.NewDefault()
		if err != nil {
			t.Fatalf("failed to create logger: %v", err)
		}

		policy := DefaultPolicy()
		resolver, err := NewMountAllowlistResolver(policy, log)
		if err != nil {
			t.Fatalf("failed to create resolver: %v", err)
		}
		defer resolver.Close()

		if resolver == nil {
			t.Fatal("resolver is nil")
		}
	})

	t.Run("NewMountAllowlistResolver with nil policy uses default", func(t *testing.T) {
		log, err := logger.NewDefault()
		if err != nil {
			t.Fatalf("failed to create logger: %v", err)
		}

		resolver, err := NewMountAllowlistResolver(nil, log)
		if err != nil {
			t.Fatalf("failed to create resolver with nil policy: %v", err)
		}
		defer resolver.Close()

		if resolver == nil {
			t.Fatal("resolver is nil")
		}
	})

	t.Run("NewDefaultMountAllowlistResolver creates resolver with default policy", func(t *testing.T) {
		resolver, err := NewDefaultMountAllowlistResolver(nil)
		if err != nil {
			t.Fatalf("failed to create default resolver: %v", err)
		}
		defer resolver.Close()

		if resolver == nil {
			t.Fatal("resolver is nil")
		}
	})

	t.Run("GetPolicy returns policy copy", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()
		policy, err := resolver.GetPolicy(ctx)
		if err != nil {
			t.Fatalf("failed to get policy: %v", err)
		}

		if policy == nil {
			t.Fatal("policy is nil")
		}

		if policy.ID != "test-policy" {
			t.Errorf("expected policy ID 'test-policy', got '%s'", policy.ID)
		}
	})

	t.Run("GetPolicy fails when resolver is closed", func(t *testing.T) {
		resolver := createTestResolver(t)
		resolver.Close()

		ctx := context.Background()
		_, err := resolver.GetPolicy(ctx)
		if err == nil {
			t.Fatal("expected error when getting policy from closed resolver")
		}
	})

	t.Run("SetPolicy updates policy", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()
		newPolicy := &Policy{
			ID:          "new-policy",
			Name:        "New Policy",
			Description: "A new test policy",
			Mode:        EnforcementModePermissive,
			Mounts: MountPolicy{
				MountAllowlist: []MountAllowlistEntry{
					{
						Path: "/new/path",
						Mode: MountAccessModeReadOnly,
					},
				},
			},
		}

		err := resolver.SetPolicy(ctx, newPolicy)
		if err != nil {
			t.Fatalf("failed to set policy: %v", err)
		}

		policy, err := resolver.GetPolicy(ctx)
		if err != nil {
			t.Fatalf("failed to get policy: %v", err)
		}

		if policy.ID != "new-policy" {
			t.Errorf("expected policy ID 'new-policy', got '%s'", policy.ID)
		}
	})

	t.Run("SetPolicy fails when resolver is closed", func(t *testing.T) {
		resolver := createTestResolver(t)
		resolver.Close()

		ctx := context.Background()
		newPolicy := DefaultPolicy()

		err := resolver.SetPolicy(ctx, newPolicy)
		if err == nil {
			t.Fatal("expected error when setting policy on closed resolver")
		}
	})

	t.Run("Close is idempotent", func(t *testing.T) {
		resolver := createTestResolver(t)

		err := resolver.Close()
		if err != nil {
			t.Fatalf("failed to close resolver: %v", err)
		}

		err = resolver.Close()
		if err != nil {
			t.Fatalf("close should be idempotent: %v", err)
		}
	})
}

// TestResolveMountAccess tests mount access resolution
func TestResolveMountAccess(t *testing.T) {
	t.Run("Default entry allows access for any user", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()

		mode, err := resolver.ResolveMountAccess(ctx, "/home/shared/data", "anyuser")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}

		if mode != MountAccessModeReadOnly {
			t.Errorf("expected readonly mode, got '%s'", mode)
		}
	})

	t.Run("User-specific entry takes precedence", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()

		mode, err := resolver.ResolveMountAccess(ctx, "/home/alice/projects", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}

		if mode != MountAccessModeReadWrite {
			t.Errorf("expected readwrite mode, got '%s'", mode)
		}
	})

	t.Run("Group-specific entry with provider allows access", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		provider := &mockGroupProvider{
			groups: map[string][]string{
				"bob": {"developers"},
			},
		}

		err := resolver.SetGroupProvider(provider)
		if err != nil {
			t.Fatalf("failed to set group provider: %v", err)
		}

		ctx := context.Background()

		mode, err := resolver.ResolveMountAccess(ctx, "/shared/team-data", "bob")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}

		if mode != MountAccessModeReadWrite {
			t.Errorf("expected readwrite mode, got '%s'", mode)
		}
	})

	t.Run("Unknown path returns denied", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()

		mode, err := resolver.ResolveMountAccess(ctx, "/unknown/path", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}

		if mode != MountAccessModeDenied {
			t.Errorf("expected denied mode, got '%s'", mode)
		}
	})

	t.Run("ResolveMountAccess fails when resolver is closed", func(t *testing.T) {
		resolver := createTestResolver(t)
		resolver.Close()

		ctx := context.Background()

		_, err := resolver.ResolveMountAccess(ctx, "/home/shared/data", "alice")
		if err == nil {
			t.Fatal("expected error when resolving mount access on closed resolver")
		}
	})
}

// TestGetAllowedMounts tests retrieving allowed mounts for a user
func TestGetAllowedMounts(t *testing.T) {
	t.Run("Returns all default entries for user without provider", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()

		mounts, err := resolver.GetAllowedMounts(ctx, "bob")
		if err != nil {
			t.Fatalf("failed to get allowed mounts: %v", err)
		}

		// Should only get the default entry
		if len(mounts) != 1 {
			t.Fatalf("expected 1 mount, got %d", len(mounts))
		}

		if mounts[0].Path != "/home/shared/data" {
			t.Errorf("expected path '/home/shared/data', got '%s'", mounts[0].Path)
		}
	})

	t.Run("Returns user-specific and default entries", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()

		mounts, err := resolver.GetAllowedMounts(ctx, "alice")
		if err != nil {
			t.Fatalf("failed to get allowed mounts: %v", err)
		}

		// Should get default entry and alice's entry
		if len(mounts) != 2 {
			t.Fatalf("expected 2 mounts, got %d", len(mounts))
		}
	})

	t.Run("Returns group-specific entries with provider", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		provider := &mockGroupProvider{
			groups: map[string][]string{
				"bob": {"developers"},
			},
		}

		err := resolver.SetGroupProvider(provider)
		if err != nil {
			t.Fatalf("failed to set group provider: %v", err)
		}

		ctx := context.Background()

		mounts, err := resolver.GetAllowedMounts(ctx, "bob")
		if err != nil {
			t.Fatalf("failed to get allowed mounts: %v", err)
		}

		// Should get default entry and developers entry
		if len(mounts) != 2 {
			t.Fatalf("expected 2 mounts, got %d", len(mounts))
		}
	})

	t.Run("GetAllowedMounts fails when resolver is closed", func(t *testing.T) {
		resolver := createTestResolver(t)
		resolver.Close()

		ctx := context.Background()

		_, err := resolver.GetAllowedMounts(ctx, "alice")
		if err == nil {
			t.Fatal("expected error when getting allowed mounts from closed resolver")
		}
	})
}

// TestValidatePath tests path validation
func TestValidatePath(t *testing.T) {
	t.Run("Allowed path returns no error", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()

		err := resolver.ValidatePath(ctx, "/home/shared/data", "alice")
		if err != nil {
			t.Errorf("expected no error for allowed path: %v", err)
		}
	})

	t.Run("Denied path returns error", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()

		err := resolver.ValidatePath(ctx, "/unknown/path", "alice")
		if err == nil {
			t.Error("expected error for denied path")
		}
	})
}

// TestStats tests resolver statistics
func TestStats(t *testing.T) {
	t.Run("Returns correct statistics", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		ctx := context.Background()

		total, user, group := resolver.Stats(ctx)

		if total != 3 {
			t.Errorf("expected 3 total entries, got %d", total)
		}

		if user != 1 {
			t.Errorf("expected 1 user entry, got %d", user)
		}

		if group != 1 {
			t.Errorf("expected 1 group entry, got %d", group)
		}
	})
}

// TestString tests string representation
func TestString(t *testing.T) {
	t.Run("Returns valid string representation", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		str := resolver.String()
		if str == "" {
			t.Error("string representation is empty")
		}

		// Check that it contains expected information
		// The string should contain "MountAllowlistResolver"
		if len(str) < 10 {
			t.Error("string representation should be longer")
		}
	})
}

// TestGroupMountSharing tests group mount sharing between multiple users
func TestGroupMountSharing(t *testing.T) {
	t.Run("Multiple users in same group can access group mount", func(t *testing.T) {
		resolver := createTestResolver(t)
		defer resolver.Close()

		// Set up a provider with multiple users in the developers group
		provider := &mockGroupProvider{
			groups: map[string][]string{
				"alice":   {"developers", "admins"},
				"bob":     {"developers"},
				"charlie": {"developers", "ops"},
			},
		}

		err := resolver.SetGroupProvider(provider)
		if err != nil {
			t.Fatalf("failed to set group provider: %v", err)
		}

		ctx := context.Background()

		// All three users should be able to access the developers group mount
		for _, username := range []string{"alice", "bob", "charlie"} {
			mode, err := resolver.ResolveMountAccess(ctx, "/shared/team-data", username)
			if err != nil {
				t.Fatalf("failed to resolve mount access for %s: %v", username, err)
			}

			if mode != MountAccessModeReadWrite {
				t.Errorf("%s expected readwrite mode, got '%s'", username, mode)
			}
		}
	})

	t.Run("Users in different groups have different access", func(t *testing.T) {
		// Create a custom resolver with multiple group mounts
		log, err := logger.NewDefault()
		if err != nil {
			t.Fatalf("failed to create logger: %v", err)
		}

		policy := &Policy{
			ID:          "test-policy",
			Name:        "Test Policy",
			Description: "A test policy for group access",
			Mode:        EnforcementModeStrict,
			Mounts: MountPolicy{
				MountAllowlist: []MountAllowlistEntry{
					{
						Path:  "/shared/dev-data",
						Group: "developers",
						Mode:  MountAccessModeReadWrite,
					},
					{
						Path:  "/shared/ops-data",
						Group: "ops",
						Mode:  MountAccessModeReadOnly,
					},
				},
			},
		}

		resolver, err := NewMountAllowlistResolver(policy, log)
		if err != nil {
			t.Fatalf("failed to create resolver: %v", err)
		}
		defer resolver.Close()

		// Set up users with different group memberships
		provider := &mockGroupProvider{
			groups: map[string][]string{
				"alice":   {"developers"},
				"bob":     {"developers", "ops"},
				"charlie": {"ops"},
			},
		}

		err = resolver.SetGroupProvider(provider)
		if err != nil {
			t.Fatalf("failed to set group provider: %v", err)
		}

		ctx := context.Background()

		// alice can access developers mount but not ops mount
		mode, err := resolver.ResolveMountAccess(ctx, "/shared/dev-data", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access for alice: %v", err)
		}
		if mode != MountAccessModeReadWrite {
			t.Errorf("alice expected readwrite for developers mount, got '%s'", mode)
		}

		mode, err = resolver.ResolveMountAccess(ctx, "/shared/ops-data", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access for alice: %v", err)
		}
		if mode != MountAccessModeDenied {
			t.Errorf("alice expected denied for ops mount, got '%s'", mode)
		}

		// bob can access both mounts
		mode, err = resolver.ResolveMountAccess(ctx, "/shared/dev-data", "bob")
		if err != nil {
			t.Fatalf("failed to resolve mount access for bob: %v", err)
		}
		if mode != MountAccessModeReadWrite {
			t.Errorf("bob expected readwrite for developers mount, got '%s'", mode)
		}

		mode, err = resolver.ResolveMountAccess(ctx, "/shared/ops-data", "bob")
		if err != nil {
			t.Fatalf("failed to resolve mount access for bob: %v", err)
		}
		if mode != MountAccessModeReadOnly {
			t.Errorf("bob expected readonly for ops mount, got '%s'", mode)
		}

		// charlie can access ops mount but not developers mount
		mode, err = resolver.ResolveMountAccess(ctx, "/shared/dev-data", "charlie")
		if err != nil {
			t.Fatalf("failed to resolve mount access for charlie: %v", err)
		}
		if mode != MountAccessModeDenied {
			t.Errorf("charlie expected denied for developers mount, got '%s'", mode)
		}

		mode, err = resolver.ResolveMountAccess(ctx, "/shared/ops-data", "charlie")
		if err != nil {
			t.Fatalf("failed to resolve mount access for charlie: %v", err)
		}
		if mode != MountAccessModeReadOnly {
			t.Errorf("charlie expected readonly for ops mount, got '%s'", mode)
		}
	})

	t.Run("GetAllowedMounts returns correct group mounts for each user", func(t *testing.T) {
		// Create a custom resolver with multiple group mounts
		log, err := logger.NewDefault()
		if err != nil {
			t.Fatalf("failed to create logger: %v", err)
		}

		policy := &Policy{
			ID:          "test-policy",
			Name:        "Test Policy",
			Description: "A test policy for group access",
			Mode:        EnforcementModeStrict,
			Mounts: MountPolicy{
				MountAllowlist: []MountAllowlistEntry{
					{
						Path:  "/shared/dev-data",
						Group: "developers",
						Mode:  MountAccessModeReadWrite,
					},
					{
						Path:  "/shared/ops-data",
						Group: "ops",
						Mode:  MountAccessModeReadOnly,
					},
				},
			},
		}

		resolver, err := NewMountAllowlistResolver(policy, log)
		if err != nil {
			t.Fatalf("failed to create resolver: %v", err)
		}
		defer resolver.Close()

		// Set up users with different group memberships
		provider := &mockGroupProvider{
			groups: map[string][]string{
				"alice":   {"developers"},
				"bob":     {"developers", "ops"},
				"charlie": {"ops"},
			},
		}

		err = resolver.SetGroupProvider(provider)
		if err != nil {
			t.Fatalf("failed to set group provider: %v", err)
		}

		ctx := context.Background()

		// alice should only get developers mount
		aliceMounts, err := resolver.GetAllowedMounts(ctx, "alice")
		if err != nil {
			t.Fatalf("failed to get allowed mounts for alice: %v", err)
		}
		if len(aliceMounts) != 1 {
			t.Errorf("alice expected 1 mount, got %d", len(aliceMounts))
		}
		if len(aliceMounts) > 0 && aliceMounts[0].Path != "/shared/dev-data" {
			t.Errorf("alice expected /shared/dev-data, got '%s'", aliceMounts[0].Path)
		}

		// bob should get both mounts
		bobMounts, err := resolver.GetAllowedMounts(ctx, "bob")
		if err != nil {
			t.Fatalf("failed to get allowed mounts for bob: %v", err)
		}
		if len(bobMounts) != 2 {
			t.Errorf("bob expected 2 mounts, got %d", len(bobMounts))
		}

		// charlie should only get ops mount
		charlieMounts, err := resolver.GetAllowedMounts(ctx, "charlie")
		if err != nil {
			t.Fatalf("failed to get allowed mounts for charlie: %v", err)
		}
		if len(charlieMounts) != 1 {
			t.Errorf("charlie expected 1 mount, got %d", len(charlieMounts))
		}
		if len(charlieMounts) > 0 && charlieMounts[0].Path != "/shared/ops-data" {
			t.Errorf("charlie expected /shared/ops-data, got '%s'", charlieMounts[0].Path)
		}
	})
}

func TestWildcardPathMatching(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	policy := &Policy{
		ID:   "wildcard-test-policy",
		Name: "Wildcard Test Policy",
		Mode: EnforcementModeStrict,
		Mounts: MountPolicy{
			MountAllowlist: []MountAllowlistEntry{
				{
					Path: "/home/*/.ssh",
					Mode: MountAccessModeDenied,
				},
				{
					Path: "/home/*/projects",
					User: "alice",
					Mode: MountAccessModeReadWrite,
				},
				{
					Path: "/data/*/shared",
					Group: "engineering",
					Mode: MountAccessModeReadOnly,
				},
				{
					Path: "/tmp/*",
					Mode: MountAccessModeReadWrite,
				},
			},
		},
	}

	resolver, err := NewMountAllowlistResolver(policy, log)
	if err != nil {
		t.Fatalf("failed to create resolver: %v", err)
	}
	defer resolver.Close()

	// Set up group provider
	groupProvider := &mockGroupProvider{
		groups: map[string][]string{
			"alice": {"engineering", "developers"},
			"bob":   {"engineering"},
		},
	}
	if err := resolver.SetGroupProvider(groupProvider); err != nil {
		t.Fatalf("failed to set group provider: %v", err)
	}

	ctx := context.Background()

	t.Run("Wildcard denies SSH directories for all users", func(t *testing.T) {
		// Test alice's SSH directory
		mode, err := resolver.ResolveMountAccess(ctx, "/home/alice/.ssh", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeDenied {
			t.Errorf("expected denied for /home/alice/.ssh, got %v", mode)
		}

		// Test bob's SSH directory
		mode, err = resolver.ResolveMountAccess(ctx, "/home/bob/.ssh", "bob")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeDenied {
			t.Errorf("expected denied for /home/bob/.ssh, got %v", mode)
		}

		// Test any other user's SSH directory
		mode, err = resolver.ResolveMountAccess(ctx, "/home/charlie/.ssh", "charlie")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeDenied {
			t.Errorf("expected denied for /home/charlie/.ssh, got %v", mode)
		}
	})

	t.Run("Wildcard matches user-specific projects directory", func(t *testing.T) {
		// Alice should have read-write access to her projects
		mode, err := resolver.ResolveMountAccess(ctx, "/home/alice/projects", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeReadWrite {
			t.Errorf("expected readwrite for alice's /home/alice/projects, got %v", mode)
		}

		// Bob should not have access to his projects (no allowlist entry for bob)
		mode, err = resolver.ResolveMountAccess(ctx, "/home/bob/projects", "bob")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeDenied {
			t.Errorf("expected denied for bob's /home/bob/projects, got %v", mode)
		}
	})

	t.Run("Wildcard matches group-specific shared directories", func(t *testing.T) {
		// Alice (member of engineering) should have read-only access to shared dirs
		mode, err := resolver.ResolveMountAccess(ctx, "/data/project1/shared", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeReadOnly {
			t.Errorf("expected readonly for alice's /data/project1/shared, got %v", mode)
		}

		// Bob (also member of engineering) should have read-only access
		mode, err = resolver.ResolveMountAccess(ctx, "/data/project2/shared", "bob")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeReadOnly {
			t.Errorf("expected readonly for bob's /data/project2/shared, got %v", mode)
		}

		// Charlie (not in engineering) should not have access
		mode, err = resolver.ResolveMountAccess(ctx, "/data/project1/shared", "charlie")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeDenied {
			t.Errorf("expected denied for charlie's /data/project1/shared, got %v", mode)
		}
	})

	t.Run("Wildcard matches default tmp directories", func(t *testing.T) {
		// Any user should have read-write access to tmp directories
		mode, err := resolver.ResolveMountAccess(ctx, "/tmp/workdir", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeReadWrite {
			t.Errorf("expected readwrite for /tmp/workdir, got %v", mode)
		}

		mode, err = resolver.ResolveMountAccess(ctx, "/tmp/scratch", "bob")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeReadWrite {
			t.Errorf("expected readwrite for /tmp/scratch, got %v", mode)
		}
	})

	t.Run("Wildcard does not match partial paths", func(t *testing.T) {
		// /home/.ssh should not match /home/*/.ssh
		mode, err := resolver.ResolveMountAccess(ctx, "/home/.ssh", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeDenied {
			t.Errorf("expected denied for /home/.ssh (no wildcard match), got %v", mode)
		}

		// /home/alice/projects/subfolder should not match /home/*/projects
		mode, err = resolver.ResolveMountAccess(ctx, "/home/alice/projects/subfolder", "alice")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		if mode != MountAccessModeDenied {
			t.Errorf("expected denied for /home/alice/projects/subfolder, got %v", mode)
		}
	})

	t.Run("Path cleaning works correctly with wildcards", func(t *testing.T) {
		// Test that path traversal doesn't bypass wildcard matching
		mode, err := resolver.ResolveMountAccess(ctx, "/home/alice/../bob/.ssh", "bob")
		if err != nil {
			t.Fatalf("failed to resolve mount access: %v", err)
		}
		// After cleaning, this becomes /home/bob/.ssh, which should be denied
		if mode != MountAccessModeDenied {
			t.Errorf("expected denied for cleaned path /home/bob/.ssh, got %v", mode)
		}
	})
}

