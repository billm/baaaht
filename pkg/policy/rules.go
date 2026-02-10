package policy

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// EnforcementMode defines how policies are enforced
type EnforcementMode string

const (
	// EnforcementModeStrict strictly enforces all policies and rejects violations
	EnforcementModeStrict EnforcementMode = "strict"
	// EnforcementModePermissive allows violations but logs warnings
	EnforcementModePermissive EnforcementMode = "permissive"
	// EnforcementModeDisabled disables all policy enforcement
	EnforcementModeDisabled EnforcementMode = "disabled"
)

// Severity defines the severity level of a policy violation
type Severity string

const (
	// SeverityError indicates a critical violation
	SeverityError Severity = "error"
	// SeverityWarning indicates a non-critical violation
	SeverityWarning Severity = "warning"
	// SeverityInfo indicates informational notice
	SeverityInfo Severity = "info"
)

// MountAccessMode defines the access mode for a mount allowlist entry
type MountAccessMode string

const (
	// MountAccessModeReadOnly allows read-only access to the mount
	MountAccessModeReadOnly MountAccessMode = "readonly"
	// MountAccessModeReadWrite allows read-write access to the mount
	MountAccessModeReadWrite MountAccessMode = "readwrite"
	// MountAccessModeDenied explicitly denies access to the mount
	MountAccessModeDenied MountAccessMode = "denied"
)

// Policy represents a set of security rules and restrictions
type Policy struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Mode        EnforcementMode   `json:"mode" yaml:"mode"`
	Quotas      ResourceQuota     `json:"quotas" yaml:"quotas"`
	Mounts      MountPolicy       `json:"mounts" yaml:"mounts"`
	Network     NetworkPolicy     `json:"network" yaml:"network"`
	Images      ImagePolicy       `json:"images" yaml:"images"`
	Security    SecurityPolicy    `json:"security" yaml:"security"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// ResourceQuota defines resource limits
type ResourceQuota struct {
	// CPU limits in nanoseconds (1 CPU = 1,000,000,000 nanoseconds)
	MaxCPUs *int64 `json:"max_cpus,omitempty" yaml:"max_cpus,omitempty"`
	MinCPUs *int64 `json:"min_cpus,omitempty" yaml:"min_cpus,omitempty"`
	// Memory limits in bytes
	MaxMemory *int64 `json:"max_memory,omitempty" yaml:"max_memory,omitempty"`
	MinMemory *int64 `json:"min_memory,omitempty" yaml:"min_memory,omitempty"`
	// Memory swap limit in bytes (0 = disable, -1 = unlimited)
	MaxMemorySwap *int64 `json:"max_memory_swap,omitempty" yaml:"max_memory_swap,omitempty"`
	// Process limit
	MaxPids *int64 `json:"max_pids,omitempty" yaml:"max_pids,omitempty"`
}

// MountPolicy defines mount restrictions
type MountPolicy struct {
	// Allow bind mounts from host
	AllowBindMounts bool `json:"allow_bind_mounts" yaml:"allow_bind_mounts"`
	// Allowed bind mount sources (empty = all allowed)
	AllowedBindSources []string `json:"allowed_bind_sources,omitempty" yaml:"allowed_bind_sources,omitempty"`
	// Deny bind mount sources (takes precedence)
	DeniedBindSources []string `json:"denied_bind_sources,omitempty" yaml:"denied_bind_sources,omitempty"`
	// Allow volume mounts
	AllowVolumes bool `json:"allow_volumes" yaml:"allow_volumes"`
	// Allowed volume names/patterns (empty = all allowed)
	AllowedVolumes []string `json:"allowed_volumes,omitempty" yaml:"allowed_volumes,omitempty"`
	// Deny volume names/patterns (takes precedence)
	DeniedVolumes []string `json:"denied_volumes,omitempty" yaml:"denied_volumes,omitempty"`
	// Allow tmpfs mounts
	AllowTmpfs bool `json:"allow_tmpfs" yaml:"allow_tmpfs"`
	// Maximum tmpfs size in bytes
	MaxTmpfsSize *int64 `json:"max_tmpfs_size,omitempty" yaml:"max_tmpfs_size,omitempty"`
	// Read-only root filesystem enforcement
	EnforceReadOnlyRootfs bool `json:"enforce_read_only_rootfs,omitempty" yaml:"enforce_read_only_rootfs,omitempty"`
	// Mount allowlist with per-user/per-group entries
	MountAllowlist []MountAllowlistEntry `json:"mount_allowlist,omitempty" yaml:"mount_allowlist,omitempty"`
}

// MountAllowlistEntry defines a single mount allowlist entry with user/group scoping
type MountAllowlistEntry struct {
	// Path is the mount path on the host
	Path string `json:"path" yaml:"path"`
	// User is the username this entry applies to (empty = all users)
	User string `json:"user,omitempty" yaml:"user,omitempty"`
	// Group is the group name this entry applies to (empty = all groups)
	Group string `json:"group,omitempty" yaml:"group,omitempty"`
	// Mode defines the access mode for this mount
	Mode MountAccessMode `json:"mode" yaml:"mode"`
}

// NetworkPolicy defines network restrictions
type NetworkPolicy struct {
	// Allow network access
	AllowNetwork bool `json:"allow_network" yaml:"allow_network"`
	// Allowed networks (CIDR notation)
	AllowedNetworks []string `json:"allowed_networks,omitempty" yaml:"allowed_networks,omitempty"`
	// Denied networks (takes precedence)
	DeniedNetworks []string `json:"denied_networks,omitempty" yaml:"denied_networks,omitempty"`
	// Allow host networking
	AllowHostNetwork bool `json:"allow_host_network" yaml:"allow_host_network"`
}

// ImagePolicy defines container image restrictions
type ImagePolicy struct {
	// Allowed image patterns (e.g., "docker.io/library/*", "registry.example.com/*")
	AllowedImages []string `json:"allowed_images,omitempty" yaml:"allowed_images,omitempty"`
	// Denied image patterns (takes precedence)
	DeniedImages []string `json:"denied_images,omitempty" yaml:"denied_images,omitempty"`
	// Require image digest pinning
	RequireDigest bool `json:"require_digest,omitempty" yaml:"require_digest,omitempty"`
	// Allow latest tag
	AllowLatestTag bool `json:"allow_latest_tag" yaml:"allow_latest_tag"`
}

// SecurityPolicy defines security restrictions
type SecurityPolicy struct {
	// Allow privileged containers
	AllowPrivileged bool `json:"allow_privileged" yaml:"allow_privileged"`
	// Allowed Linux capabilities
	AllowedCapabilities []string `json:"allowed_capabilities,omitempty" yaml:"allowed_capabilities,omitempty"`
	// Add required capabilities
	AddCapabilities []string `json:"add_capabilities,omitempty" yaml:"add_capabilities,omitempty"`
	// Drop capabilities
	DropCapabilities []string `json:"drop_capabilities,omitempty" yaml:"drop_capabilities,omitempty"`
	// Allow running as root
	AllowRoot bool `json:"allow_root" yaml:"allow_root"`
	// Require non-root user
	RequireNonRoot bool `json:"require_non_root" yaml:"require_non_root"`
	// Allowed user IDs (UIDs)
	AllowedUIDs []int64 `json:"allowed_uids,omitempty" yaml:"allowed_uids,omitempty"`
	// Allowed group IDs (GIDs)
	AllowedGIDs []int64 `json:"allowed_gids,omitempty" yaml:"allowed_gids,omitempty"`
	// Read-only root filesystem
	ReadOnlyRootfs bool `json:"read_only_rootfs" yaml:"read_only_rootfs"`
}

// Violation represents a policy violation
type Violation struct {
	Rule      string `json:"rule" yaml:"rule"`
	Message   string `json:"message" yaml:"message"`
	Severity  string `json:"severity" yaml:"severity"`   // error, warning, info
	Component string `json:"component" yaml:"component"` // quota, mount, network, image, security
}

// ValidationResult represents the result of a policy validation
type ValidationResult struct {
	Allowed    bool        `json:"allowed" yaml:"allowed"`
	Violations []Violation `json:"violations,omitempty" yaml:"violations,omitempty"`
	Warnings   []Violation `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// Merge merges another policy into this one, with the other policy taking precedence
func (p *Policy) Merge(other *Policy) *Policy {
	if other == nil {
		return p
	}

	merged := *p

	// Merge quotas
	if other.Quotas.MaxCPUs != nil {
		merged.Quotas.MaxCPUs = other.Quotas.MaxCPUs
	}
	if other.Quotas.MinCPUs != nil {
		merged.Quotas.MinCPUs = other.Quotas.MinCPUs
	}
	if other.Quotas.MaxMemory != nil {
		merged.Quotas.MaxMemory = other.Quotas.MaxMemory
	}
	if other.Quotas.MinMemory != nil {
		merged.Quotas.MinMemory = other.Quotas.MinMemory
	}
	if other.Quotas.MaxMemorySwap != nil {
		merged.Quotas.MaxMemorySwap = other.Quotas.MaxMemorySwap
	}
	if other.Quotas.MaxPids != nil {
		merged.Quotas.MaxPids = other.Quotas.MaxPids
	}

	// Merge mount policy
	merged.Mounts.AllowBindMounts = other.Mounts.AllowBindMounts
	if len(other.Mounts.AllowedBindSources) > 0 {
		merged.Mounts.AllowedBindSources = other.Mounts.AllowedBindSources
	}
	if len(other.Mounts.DeniedBindSources) > 0 {
		merged.Mounts.DeniedBindSources = other.Mounts.DeniedBindSources
	}
	merged.Mounts.AllowVolumes = other.Mounts.AllowVolumes
	if len(other.Mounts.AllowedVolumes) > 0 {
		merged.Mounts.AllowedVolumes = other.Mounts.AllowedVolumes
	}
	if len(other.Mounts.DeniedVolumes) > 0 {
		merged.Mounts.DeniedVolumes = other.Mounts.DeniedVolumes
	}
	merged.Mounts.AllowTmpfs = other.Mounts.AllowTmpfs
	if other.Mounts.MaxTmpfsSize != nil {
		merged.Mounts.MaxTmpfsSize = other.Mounts.MaxTmpfsSize
	}
	merged.Mounts.EnforceReadOnlyRootfs = other.Mounts.EnforceReadOnlyRootfs
	if len(other.Mounts.MountAllowlist) > 0 {
		merged.Mounts.MountAllowlist = other.Mounts.MountAllowlist
	}

	// Merge network policy
	merged.Network.AllowNetwork = other.Network.AllowNetwork
	if len(other.Network.AllowedNetworks) > 0 {
		merged.Network.AllowedNetworks = other.Network.AllowedNetworks
	}
	if len(other.Network.DeniedNetworks) > 0 {
		merged.Network.DeniedNetworks = other.Network.DeniedNetworks
	}
	merged.Network.AllowHostNetwork = other.Network.AllowHostNetwork

	// Merge image policy
	if len(other.Images.AllowedImages) > 0 {
		merged.Images.AllowedImages = other.Images.AllowedImages
	}
	if len(other.Images.DeniedImages) > 0 {
		merged.Images.DeniedImages = other.Images.DeniedImages
	}
	merged.Images.RequireDigest = other.Images.RequireDigest
	merged.Images.AllowLatestTag = other.Images.AllowLatestTag

	// Merge security policy
	merged.Security.AllowPrivileged = other.Security.AllowPrivileged
	if len(other.Security.AllowedCapabilities) > 0 {
		merged.Security.AllowedCapabilities = other.Security.AllowedCapabilities
	}
	if len(other.Security.AddCapabilities) > 0 {
		merged.Security.AddCapabilities = other.Security.AddCapabilities
	}
	if len(other.Security.DropCapabilities) > 0 {
		merged.Security.DropCapabilities = other.Security.DropCapabilities
	}
	merged.Security.AllowRoot = other.Security.AllowRoot
	merged.Security.RequireNonRoot = other.Security.RequireNonRoot
	if len(other.Security.AllowedUIDs) > 0 {
		merged.Security.AllowedUIDs = other.Security.AllowedUIDs
	}
	if len(other.Security.AllowedGIDs) > 0 {
		merged.Security.AllowedGIDs = other.Security.AllowedGIDs
	}
	merged.Security.ReadOnlyRootfs = other.Security.ReadOnlyRootfs

	// Update mode
	merged.Mode = other.Mode

	return &merged
}

// Validate validates the policy configuration itself
func (p *Policy) Validate() error {
	// Validate mode
	switch p.Mode {
	case EnforcementModeStrict, EnforcementModePermissive, EnforcementModeDisabled:
		// Valid modes
	default:
		return fmt.Errorf("invalid enforcement mode: %s", p.Mode)
	}

	// Validate quotas
	if p.Quotas.MinCPUs != nil && p.Quotas.MaxCPUs != nil {
		if *p.Quotas.MinCPUs > *p.Quotas.MaxCPUs {
			return fmt.Errorf("min CPUs cannot exceed max CPUs")
		}
	}
	if p.Quotas.MinMemory != nil && p.Quotas.MaxMemory != nil {
		if *p.Quotas.MinMemory > *p.Quotas.MaxMemory {
			return fmt.Errorf("min memory cannot exceed max memory")
		}
	}
	if p.Quotas.MaxPids != nil && *p.Quotas.MaxPids < 1 {
		return fmt.Errorf("max PIDs must be at least 1")
	}

	// Validate mount policy
	if p.Mounts.MaxTmpfsSize != nil && *p.Mounts.MaxTmpfsSize < 0 {
		return fmt.Errorf("max tmpfs size cannot be negative")
	}

	// Validate image patterns
	for _, pattern := range p.Images.AllowedImages {
		if _, _, _, err := parseImagePattern(pattern); err != nil {
			return fmt.Errorf("invalid allowed image pattern %q: %w", pattern, err)
		}
	}
	for _, pattern := range p.Images.DeniedImages {
		if _, _, _, err := parseImagePattern(pattern); err != nil {
			return fmt.Errorf("invalid denied image pattern %q: %w", pattern, err)
		}
	}

	return nil
}

// DefaultPolicy returns a policy with default restrictive settings
func DefaultPolicy() *Policy {
	return &Policy{
		ID:          "default",
		Name:        "Default Policy",
		Description: "Default security policy with reasonable restrictions",
		Mode:        EnforcementModeStrict,
		Quotas: ResourceQuota{
			MaxCPUs:       int64Ptr(4000000000), // 4 CPUs
			MaxMemory:     int64Ptr(8589934592), // 8GB
			MaxMemorySwap: int64Ptr(8589934592), // 8GB
			MaxPids:       int64Ptr(1024),
		},
		Mounts: MountPolicy{
			AllowBindMounts:       false,
			AllowVolumes:          true,
			AllowTmpfs:            true,
			MaxTmpfsSize:          int64Ptr(268435456), // 256MB
			EnforceReadOnlyRootfs: false,
		},
		Network: NetworkPolicy{
			AllowNetwork:     true,
			AllowHostNetwork: false,
		},
		Images: ImagePolicy{
			AllowLatestTag: false,
		},
		Security: SecurityPolicy{
			AllowPrivileged: false,
			RequireNonRoot:  false,
			ReadOnlyRootfs:  false,
		},
	}
}

// PermissivePolicy returns a policy with permissive settings
func PermissivePolicy() *Policy {
	return &Policy{
		ID:          "permissive",
		Name:        "Permissive Policy",
		Description: "Permissive security policy for development",
		Mode:        EnforcementModePermissive,
		Quotas: ResourceQuota{
			MaxCPUs:       int64Ptr(16000000000), // 16 CPUs
			MaxMemory:     int64Ptr(17179869184), // 16GB
			MaxMemorySwap: int64Ptr(-1),          // Unlimited
			MaxPids:       int64Ptr(4096),
		},
		Mounts: MountPolicy{
			AllowBindMounts: true,
			AllowVolumes:    true,
			AllowTmpfs:      true,
		},
		Network: NetworkPolicy{
			AllowNetwork:     true,
			AllowHostNetwork: false,
		},
		Images: ImagePolicy{
			AllowLatestTag: true,
		},
		Security: SecurityPolicy{
			AllowPrivileged: false,
			RequireNonRoot:  false,
			ReadOnlyRootfs:  false,
			AllowRoot:       true,
		},
	}
}

// parseImagePattern parses an image pattern and returns the components
func parseImagePattern(pattern string) (host, repo, tag string, err error) {
	// Split by colon to separate tag
	parts := strings.SplitN(pattern, ":", 2)
	imageRef := parts[0]
	if len(parts) == 2 {
		tag = parts[1]
	}

	// Split by slash to separate host and repository
	slashParts := strings.Split(imageRef, "/")
	if len(slashParts) > 1 && strings.Contains(slashParts[0], ".") {
		host = slashParts[0]
		repo = strings.Join(slashParts[1:], "/")
	} else {
		repo = imageRef
	}

	return host, repo, tag, nil
}

// matchPattern checks if a string matches a pattern with wildcards
func matchPattern(s, pattern string) bool {
	// Fast path: exact match
	if s == pattern {
		return true
	}

	// Check for wildcard
	if !strings.Contains(pattern, "*") {
		return false
	}

	// Convert wildcard pattern to regex
	// Replace * with .* and escape other regex special chars
	regexPattern := strings.ReplaceAll(regexp.QuoteMeta(pattern), "\\*", ".*")
	matched, _ := regexp.MatchString("^"+regexPattern+"$", s)
	return matched
}

// isPathAllowed checks if a path is allowed based on allowed and denied lists
func isPathAllowed(path string, allowed, denied []string) bool {
	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check denied list first (takes precedence)
	for _, pattern := range denied {
		if matchPattern(cleanPath, pattern) || matchPattern(path, pattern) {
			return false
		}
	}

	// If no allowed list, allow all not denied
	if len(allowed) == 0 {
		return true
	}

	// Check allowed list
	for _, pattern := range allowed {
		if matchPattern(cleanPath, pattern) || matchPattern(path, pattern) {
			return true
		}
	}

	return false
}

// int64Ptr returns a pointer to an int64
func int64Ptr(i int64) *int64 {
	return &i
}
