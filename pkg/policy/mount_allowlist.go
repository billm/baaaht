package policy

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// GroupMembershipProvider defines an interface for looking up user group memberships
// This allows the resolver to be decoupled from the actual group membership source
type GroupMembershipProvider interface {
	// GetUserGroups returns the list of group names for a given user
	GetUserGroups(ctx context.Context, username string) ([]string, error)
}

// MountAllowlistResolver resolves mount allowlists for users and groups
// with proper inheritance and access mode enforcement
type MountAllowlistResolver struct {
	mu       sync.RWMutex
	policy   *Policy
	logger   *logger.Logger
	closed   bool
	groupProvider GroupMembershipProvider
}

// NewMountAllowlistResolver creates a new mount allowlist resolver
func NewMountAllowlistResolver(policy *Policy, log *logger.Logger) (*MountAllowlistResolver, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	if policy == nil {
		policy = DefaultPolicy()
	}

	r := &MountAllowlistResolver{
		policy:   policy,
		logger:   log.With("component", "mount_allowlist_resolver"),
		closed:   false,
		groupProvider: nil, // Can be set later with SetGroupProvider
	}

	r.logger.Info("Mount allowlist resolver initialized",
		"policy_id", policy.ID,
		"policy_name", policy.Name,
		"allowlist_entries", len(policy.Mounts.MountAllowlist))

	return r, nil
}

// NewDefault creates a new mount allowlist resolver with default policy
func NewDefaultMountAllowlistResolver(log *logger.Logger) (*MountAllowlistResolver, error) {
	policy := DefaultPolicy()
	return NewMountAllowlistResolver(policy, log)
}

// SetPolicy updates the policy used by the resolver
func (r *MountAllowlistResolver) SetPolicy(ctx context.Context, policy *Policy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "resolver is closed")
	}

	// Validate policy
	if err := policy.Validate(); err != nil {
		return types.WrapError(types.ErrCodeInvalidArgument, "invalid policy", err)
	}

	r.policy = policy

	r.logger.Info("Policy updated",
		"policy_id", policy.ID,
		"name", policy.Name,
		"allowlist_entries", len(policy.Mounts.MountAllowlist))

	return nil
}

// GetPolicy returns the current policy
func (r *MountAllowlistResolver) GetPolicy(ctx context.Context) (*Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "resolver is closed")
	}

	// Return a copy
	policy := *r.policy
	return &policy, nil
}

// SetGroupProvider sets the group membership provider
func (r *MountAllowlistResolver) SetGroupProvider(provider GroupMembershipProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "resolver is closed")
	}

	r.groupProvider = provider
	r.logger.Debug("Group membership provider set")
	return nil
}

// ResolveMountAccess checks if a path is allowed for a user/group
// and returns the access mode (readonly, readwrite, or denied)
func (r *MountAllowlistResolver) ResolveMountAccess(ctx context.Context, path, username string) (MountAccessMode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return MountAccessModeDenied, types.NewError(types.ErrCodeUnavailable, "resolver is closed")
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Get user's groups
	var userGroups []string
	if r.groupProvider != nil && username != "" {
		groups, err := r.groupProvider.GetUserGroups(ctx, username)
		if err != nil {
			r.logger.Warn("Failed to get user groups", "user", username, "error", err)
			// Continue without group information
		} else {
			userGroups = groups
		}
	}

	// Find the best matching entry
	// Priority: user-specific > group-specific > default (no user/group)
	var bestMatch *MountAllowlistEntry
	var bestMatchPriority int // 3 = user-specific, 2 = group-specific, 1 = default

	for _, entry := range r.policy.Mounts.MountAllowlist {
		// Check if path matches
		if !matchPattern(cleanPath, entry.Path) && !matchPattern(path, entry.Path) {
			continue
		}

		// Determine entry priority
		priority := 0
		if entry.User != "" && entry.User == username {
			priority = 3 // User-specific match
		} else if entry.Group != "" && containsString(userGroups, entry.Group) {
			priority = 2 // Group-specific match
		} else if entry.User == "" && entry.Group == "" {
			priority = 1 // Default match
		} else {
			continue // No match
		}

		// Skip if we already have a higher priority match
		if bestMatch != nil && priority <= bestMatchPriority {
			continue
		}

		bestMatch = &entry
		bestMatchPriority = priority
	}

	// If no match found, default to denied
	if bestMatch == nil {
		r.logger.Debug("Path not found in allowlist, access denied",
			"path", cleanPath,
			"user", username)
		return MountAccessModeDenied, nil
	}

	r.logger.Debug("Path access resolved",
		"path", cleanPath,
		"user", username,
		"mode", bestMatch.Mode,
		"match_type", bestMatchPriority)

	return bestMatch.Mode, nil
}

// GetAllowedMounts returns all allowed mounts for a user
func (r *MountAllowlistResolver) GetAllowedMounts(ctx context.Context, username string) ([]MountAllowlistEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "resolver is closed")
	}

	// Get user's groups
	var userGroups []string
	if r.groupProvider != nil && username != "" {
		groups, err := r.groupProvider.GetUserGroups(ctx, username)
		if err != nil {
			r.logger.Warn("Failed to get user groups", "user", username, "error", err)
		} else {
			userGroups = groups
		}
	}

	var allowedMounts []MountAllowlistEntry

	for _, entry := range r.policy.Mounts.MountAllowlist {
		// Skip denied entries
		if entry.Mode == MountAccessModeDenied {
			continue
		}

		// Check if entry applies to user
		applies := false
		if entry.User != "" {
			if entry.User == username {
				applies = true
			}
		} else if entry.Group != "" {
			if containsString(userGroups, entry.Group) {
				applies = true
			}
		} else {
			// Default entry (no user or group)
			applies = true
		}

		if applies {
			allowedMounts = append(allowedMounts, entry)
		}
	}

	return allowedMounts, nil
}

// ValidatePath validates a path against the allowlist for a user
// Returns an error if the path is not allowed
func (r *MountAllowlistResolver) ValidatePath(ctx context.Context, path, username string) error {
	mode, err := r.ResolveMountAccess(ctx, path, username)
	if err != nil {
		return err
	}

	if mode == MountAccessModeDenied {
		return types.NewError(types.ErrCodePermission,
			fmt.Sprintf("path %s is not in the mount allowlist for user %s", path, username))
	}

	return nil
}

// Close closes the resolver
func (r *MountAllowlistResolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	r.logger.Info("Mount allowlist resolver closed")
	return nil
}

// Stats returns statistics about the resolver
func (r *MountAllowlistResolver) Stats(ctx context.Context) (totalEntries int, userEntries int, groupEntries int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	totalEntries = len(r.policy.Mounts.MountAllowlist)
	for _, entry := range r.policy.Mounts.MountAllowlist {
		if entry.User != "" {
			userEntries++
		}
		if entry.Group != "" {
			groupEntries++
		}
	}
	return
}

// String returns a string representation of the resolver
func (r *MountAllowlistResolver) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total, user, group := r.Stats(context.Background())
	return fmt.Sprintf("MountAllowlistResolver{policy: %s, entries: %d (user: %d, group: %d)}",
		r.policy.Name, total, user, group)
}

// containsString checks if a string slice contains a specific string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
