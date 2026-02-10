package policy

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Enforcer enforces security policies on container configurations
type Enforcer struct {
	mu                    sync.RWMutex
	policy                *Policy
	cfg                   config.PolicyConfig
	logger                *logger.Logger
	closed                bool
	sessionPolicies       map[types.ID]*Policy // Session-specific policies
	mountAllowlistResolver *MountAllowlistResolver // Mount allowlist resolver
	auditLogger           *AuditLogger // Audit logger for security events
}

// New creates a new policy enforcer
func New(cfg config.PolicyConfig, log *logger.Logger) (*Enforcer, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Start with default policy
	policy := DefaultPolicy()

	e := &Enforcer{
		policy:          policy,
		cfg:             cfg,
		logger:          log.With("component", "policy_enforcer"),
		closed:          false,
		sessionPolicies: make(map[types.ID]*Policy),
	}

	e.logger.Info("Policy enforcer initialized",
		"mode", policy.Mode,
		"default_quota_cpu", policy.Quotas.MaxCPUs,
		"default_quota_memory", policy.Quotas.MaxMemory)

	return e, nil
}

// NewDefault creates a new policy enforcer with default configuration
func NewDefault(log *logger.Logger) (*Enforcer, error) {
	cfg := config.DefaultPolicyConfig()
	return New(cfg, log)
}

// SetPolicy sets the global policy
func (e *Enforcer) SetPolicy(ctx context.Context, policy *Policy) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return types.NewError(types.ErrCodeUnavailable, "enforcer is closed")
	}

	// Validate policy
	if err := policy.Validate(); err != nil {
		return types.WrapError(types.ErrCodeInvalidArgument, "invalid policy", err)
	}

	e.policy = policy

	e.logger.Info("Policy updated",
		"policy_id", policy.ID,
		"name", policy.Name,
		"mode", policy.Mode)

	return nil
}

// GetPolicy returns the current global policy
func (e *Enforcer) GetPolicy(ctx context.Context) (*Policy, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "enforcer is closed")
	}

	// Return a copy
	policy := *e.policy
	return &policy, nil
}

// SetSessionPolicy sets a policy for a specific session
func (e *Enforcer) SetSessionPolicy(ctx context.Context, sessionID types.ID, policy *Policy) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return types.NewError(types.ErrCodeUnavailable, "enforcer is closed")
	}

	// Validate policy
	if err := policy.Validate(); err != nil {
		return types.WrapError(types.ErrCodeInvalidArgument, "invalid policy", err)
	}

	e.sessionPolicies[sessionID] = policy

	e.logger.Debug("Session policy set",
		"session_id", sessionID,
		"policy_id", policy.ID,
		"name", policy.Name)

	return nil
}

// GetSessionPolicy returns the policy for a specific session
func (e *Enforcer) GetSessionPolicy(ctx context.Context, sessionID types.ID) (*Policy, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "enforcer is closed")
	}

	policy, exists := e.sessionPolicies[sessionID]
	if !exists {
		// Return global policy if no session policy exists
		policy := *e.policy
		return &policy, nil
	}

	// Merge session policy with global policy
	merged := e.policy.Merge(policy)
	return merged, nil
}

// RemoveSessionPolicy removes a session-specific policy
func (e *Enforcer) RemoveSessionPolicy(ctx context.Context, sessionID types.ID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return types.NewError(types.ErrCodeUnavailable, "enforcer is closed")
	}

	delete(e.sessionPolicies, sessionID)

	e.logger.Debug("Session policy removed", "session_id", sessionID)
	return nil
}

// ValidateContainerConfig validates a container configuration against the policy
func (e *Enforcer) ValidateContainerConfig(ctx context.Context, sessionID types.ID, config types.ContainerConfig) (*ValidationResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "enforcer is closed")
	}

	// Get applicable policy
	policy := e.policy
	if sessionPolicy, exists := e.sessionPolicies[sessionID]; exists {
		policy = e.policy.Merge(sessionPolicy)
	}

	// If disabled, allow everything
	if policy.Mode == EnforcementModeDisabled {
		return &ValidationResult{Allowed: true}, nil
	}

	result := &ValidationResult{Allowed: true}

	// Validate quotas
	e.validateQuotas(policy, config, result)

	// Validate mounts
	e.validateMounts(policy, config, result)

	// Validate network
	e.validateNetwork(policy, config, result)

	// Validate image
	e.validateImage(policy, config, result)

	// Validate security settings
	e.validateSecurity(policy, config, result)

	// Determine if allowed based on mode
	if policy.Mode == EnforcementModeStrict && len(result.Violations) > 0 {
		result.Allowed = false
	}

	return result, nil
}

// validateQuotas validates resource quotas
func (e *Enforcer) validateQuotas(policy *Policy, config types.ContainerConfig, result *ValidationResult) {
	if config.Resources.NanoCPUs > 0 {
		if policy.Quotas.MaxCPUs != nil && config.Resources.NanoCPUs > *policy.Quotas.MaxCPUs {
			violation := Violation{
				Rule: "quota.cpu.max",
				Message: fmt.Sprintf("CPU quota exceeds maximum: requested %.2f, maximum %.2f",
					float64(config.Resources.NanoCPUs)/1e9, float64(*policy.Quotas.MaxCPUs)/1e9),
				Severity:  string(SeverityError),
				Component: "quota",
			}
			result.Violations = append(result.Violations, violation)
		}
		if policy.Quotas.MinCPUs != nil && config.Resources.NanoCPUs < *policy.Quotas.MinCPUs {
			violation := Violation{
				Rule: "quota.cpu.min",
				Message: fmt.Sprintf("CPU quota below minimum: requested %.2f, minimum %.2f",
					float64(config.Resources.NanoCPUs)/1e9, float64(*policy.Quotas.MinCPUs)/1e9),
				Severity:  string(SeverityWarning),
				Component: "quota",
			}
			result.Violations = append(result.Violations, violation)
		}
	}

	if config.Resources.MemoryBytes > 0 {
		if policy.Quotas.MaxMemory != nil && config.Resources.MemoryBytes > *policy.Quotas.MaxMemory {
			violation := Violation{
				Rule: "quota.memory.max",
				Message: fmt.Sprintf("memory quota exceeds maximum: requested %d, maximum %d",
					config.Resources.MemoryBytes, *policy.Quotas.MaxMemory),
				Severity:  string(SeverityError),
				Component: "quota",
			}
			result.Violations = append(result.Violations, violation)
		}
		if policy.Quotas.MinMemory != nil && config.Resources.MemoryBytes < *policy.Quotas.MinMemory {
			violation := Violation{
				Rule: "quota.memory.min",
				Message: fmt.Sprintf("memory quota below minimum: requested %d, minimum %d",
					config.Resources.MemoryBytes, *policy.Quotas.MinMemory),
				Severity:  string(SeverityWarning),
				Component: "quota",
			}
			result.Violations = append(result.Violations, violation)
		}
	}

	if config.Resources.MemorySwap > 0 && policy.Quotas.MaxMemorySwap != nil {
		if config.Resources.MemorySwap > *policy.Quotas.MaxMemorySwap && *policy.Quotas.MaxMemorySwap != -1 {
			violation := Violation{
				Rule: "quota.memory_swap.max",
				Message: fmt.Sprintf("memory swap quota exceeds maximum: requested %d, maximum %d",
					config.Resources.MemorySwap, *policy.Quotas.MaxMemorySwap),
				Severity:  string(SeverityError),
				Component: "quota",
			}
			result.Violations = append(result.Violations, violation)
		}
	}

	if config.Resources.PidsLimit != nil && policy.Quotas.MaxPids != nil {
		if *config.Resources.PidsLimit > *policy.Quotas.MaxPids {
			violation := Violation{
				Rule: "quota.pids.max",
				Message: fmt.Sprintf("PIDs limit exceeds maximum: requested %d, maximum %d",
					*config.Resources.PidsLimit, *policy.Quotas.MaxPids),
				Severity:  string(SeverityError),
				Component: "quota",
			}
			result.Violations = append(result.Violations, violation)
		}
	}
}

// validateMounts validates mount configurations
func (e *Enforcer) validateMounts(policy *Policy, config types.ContainerConfig, result *ValidationResult) {
	for _, mount := range config.Mounts {
		switch mount.Type {
		case types.MountTypeBind:
			if !policy.Mounts.AllowBindMounts {
				violation := Violation{
					Rule:      "mount.bind.disabled",
					Message:   fmt.Sprintf("bind mounts are not allowed: %s -> %s", mount.Source, mount.Target),
					Severity:  string(SeverityError),
					Component: "mount",
				}
				result.Violations = append(result.Violations, violation)
				continue
			}

			// Check bind mount source against allow/deny lists
			if !isPathAllowed(mount.Source, policy.Mounts.AllowedBindSources, policy.Mounts.DeniedBindSources) {
				violation := Violation{
					Rule:      "mount.bind.source_not_allowed",
					Message:   fmt.Sprintf("bind mount source not allowed: %s", mount.Source),
					Severity:  string(SeverityError),
					Component: "mount",
				}
				result.Violations = append(result.Violations, violation)
			}

		case types.MountTypeVolume:
			if !policy.Mounts.AllowVolumes {
				violation := Violation{
					Rule:      "mount.volume.disabled",
					Message:   fmt.Sprintf("volume mounts are not allowed: %s", mount.Source),
					Severity:  string(SeverityError),
					Component: "mount",
				}
				result.Violations = append(result.Violations, violation)
				continue
			}

			// Check volume name against allow/deny lists
			if !isPathAllowed(mount.Source, policy.Mounts.AllowedVolumes, policy.Mounts.DeniedVolumes) {
				violation := Violation{
					Rule:      "mount.volume.not_allowed",
					Message:   fmt.Sprintf("volume not allowed: %s", mount.Source),
					Severity:  string(SeverityError),
					Component: "mount",
				}
				result.Violations = append(result.Violations, violation)
			}

		case types.MountTypeTmpfs:
			if !policy.Mounts.AllowTmpfs {
				violation := Violation{
					Rule:      "mount.tmpfs.disabled",
					Message:   fmt.Sprintf("tmpfs mounts are not allowed: %s", mount.Target),
					Severity:  string(SeverityError),
					Component: "mount",
				}
				result.Violations = append(result.Violations, violation)
				continue
			}

			// Check tmpfs size
			if policy.Mounts.MaxTmpfsSize != nil {
				// Extract size from source (format: "size=100m")
				size := int64(0) // Default unlimited
				for _, part := range strings.Split(mount.Source, ",") {
					if strings.HasPrefix(part, "size=") {
						sizeStr := strings.TrimPrefix(part, "size=")
						size = parseTmpfsSize(sizeStr)
						break
					}
				}

				if size > *policy.Mounts.MaxTmpfsSize {
					violation := Violation{
						Rule: "mount.tmpfs.size_exceeded",
						Message: fmt.Sprintf("tmpfs size exceeds maximum: requested %d, maximum %d",
							size, *policy.Mounts.MaxTmpfsSize),
						Severity:  string(SeverityError),
						Component: "mount",
					}
					result.Violations = append(result.Violations, violation)
				}
			}
		}
	}

	// Check read-only rootfs enforcement
	if policy.Mounts.EnforceReadOnlyRootfs && !config.ReadOnlyRootfs {
		violation := Violation{
			Rule:      "mount.readonly_rootfs.required",
			Message:   "read-only root filesystem is required but not set",
			Severity:  string(SeverityError),
			Component: "mount",
		}
		result.Violations = append(result.Violations, violation)
	}
}

// validateNetwork validates network settings
func (e *Enforcer) validateNetwork(policy *Policy, config types.ContainerConfig, result *ValidationResult) {
	// Check host networking
	if config.NetworkMode == "host" && !policy.Network.AllowHostNetwork {
		violation := Violation{
			Rule:      "network.host.disabled",
			Message:   "host networking is not allowed",
			Severity:  string(SeverityError),
			Component: "network",
		}
		result.Violations = append(result.Violations, violation)
	}

	// If network access is disabled, warn about any network configuration
	if !policy.Network.AllowNetwork && len(config.Networks) > 0 {
		violation := Violation{
			Rule:      "network.disabled",
			Message:   "network access is disabled but networks are configured",
			Severity:  string(SeverityWarning),
			Component: "network",
		}
		result.Warnings = append(result.Warnings, violation)
	}

	// Check port bindings - if network is disabled, ports shouldn't be exposed
	if !policy.Network.AllowNetwork && len(config.Ports) > 0 {
		violation := Violation{
			Rule:      "network.ports.disabled",
			Message:   "network access is disabled but ports are exposed",
			Severity:  string(SeverityWarning),
			Component: "network",
		}
		result.Warnings = append(result.Warnings, violation)
	}
}

// validateImage validates the container image
func (e *Enforcer) validateImage(policy *Policy, config types.ContainerConfig, result *ValidationResult) {
	image := config.Image

	// Check denied images first
	for _, pattern := range policy.Images.DeniedImages {
		if matchPattern(image, pattern) {
			violation := Violation{
				Rule:      "image.denied",
				Message:   fmt.Sprintf("image is denied by policy: %s matches pattern %s", image, pattern),
				Severity:  string(SeverityError),
				Component: "image",
			}
			result.Violations = append(result.Violations, violation)
			return // Short-circuit on deny
		}
	}

	// Check allowed images
	if len(policy.Images.AllowedImages) > 0 {
		allowed := false
		for _, pattern := range policy.Images.AllowedImages {
			if matchPattern(image, pattern) {
				allowed = true
				break
			}
		}
		if !allowed {
			violation := Violation{
				Rule:      "image.not_allowed",
				Message:   fmt.Sprintf("image is not in allowed list: %s", image),
				Severity:  string(SeverityError),
				Component: "image",
			}
			result.Violations = append(result.Violations, violation)
		}
	}

	// Check for latest tag
	if !policy.Images.AllowLatestTag {
		if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") {
			violation := Violation{
				Rule:      "image.latest_tag",
				Message:   "images with 'latest' tag or no tag are not allowed",
				Severity:  string(SeverityError),
				Component: "image",
			}
			result.Violations = append(result.Violations, violation)
		}
	}

	// Check for digest pinning
	if policy.Images.RequireDigest && !strings.Contains(image, "@sha256:") {
		violation := Violation{
			Rule:      "image.digest_required",
			Message:   "image digest pinning is required but not present",
			Severity:  string(SeverityError),
			Component: "image",
		}
		result.Violations = append(result.Violations, violation)
	}
}

// validateSecurity validates security settings
func (e *Enforcer) validateSecurity(policy *Policy, config types.ContainerConfig, result *ValidationResult) {
	// Check privileged mode - this is typically set via labels or capabilities
	if config.Labels != nil {
		if privileged, ok := config.Labels["privileged"]; ok && privileged == "true" {
			if !policy.Security.AllowPrivileged {
				violation := Violation{
					Rule:      "security.privileged.disabled",
					Message:   "privileged containers are not allowed",
					Severity:  string(SeverityError),
					Component: "security",
				}
				result.Violations = append(result.Violations, violation)
			}
		}
	}

	// Check user settings
	if user, ok := config.Labels["user"]; ok {
		// Check if running as root
		if user == "root" || user == "0" {
			if !policy.Security.AllowRoot {
				violation := Violation{
					Rule:      "security.root.disabled",
					Message:   "running as root is not allowed",
					Severity:  string(SeverityError),
					Component: "security",
				}
				result.Violations = append(result.Violations, violation)
			}
			if policy.Security.RequireNonRoot {
				violation := Violation{
					Rule:      "security.non_root_required",
					Message:   "running as non-root user is required",
					Severity:  string(SeverityError),
					Component: "security",
				}
				result.Violations = append(result.Violations, violation)
			}
		}
	}

	// Check read-only rootfs
	if policy.Security.ReadOnlyRootfs && !config.ReadOnlyRootfs {
		violation := Violation{
			Rule:      "security.readonly_rootfs.required",
			Message:   "read-only root filesystem is required by security policy",
			Severity:  string(SeverityError),
			Component: "security",
		}
		result.Violations = append(result.Violations, violation)
	}
}

// EnforceContainerConfig enforces policy on a container configuration and returns a modified config
func (e *Enforcer) EnforceContainerConfig(ctx context.Context, sessionID types.ID, config types.ContainerConfig) (types.ContainerConfig, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return config, types.NewError(types.ErrCodeUnavailable, "enforcer is closed")
	}

	// Get applicable policy
	policy := e.policy
	if sessionPolicy, exists := e.sessionPolicies[sessionID]; exists {
		policy = e.policy.Merge(sessionPolicy)
	}

	// If disabled, return config as-is
	if policy.Mode == EnforcementModeDisabled {
		return config, nil
	}

	// Validate first
	result, err := e.ValidateContainerConfig(ctx, sessionID, config)
	if err != nil {
		return config, err
	}

	if !result.Allowed {
		return config, types.NewError(types.ErrCodePermission, "container configuration violates policy")
	}

	// Apply policy defaults and constraints
	enforced := config

	// Apply default quotas if not set
	if enforced.Resources.NanoCPUs == 0 && policy.Quotas.MaxCPUs != nil {
		enforced.Resources.NanoCPUs = *policy.Quotas.MaxCPUs
	}
	if enforced.Resources.MemoryBytes == 0 && policy.Quotas.MaxMemory != nil {
		enforced.Resources.MemoryBytes = *policy.Quotas.MaxMemory
	}
	if enforced.Resources.PidsLimit == nil && policy.Quotas.MaxPids != nil {
		maxPids := *policy.Quotas.MaxPids
		enforced.Resources.PidsLimit = &maxPids
	}

	// Enforce read-only rootfs if required
	if policy.Security.ReadOnlyRootfs {
		enforced.ReadOnlyRootfs = true
	}
	if policy.Mounts.EnforceReadOnlyRootfs {
		enforced.ReadOnlyRootfs = true
	}

	return enforced, nil
}

// Close closes the enforcer
func (e *Enforcer) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}

	e.closed = true
	e.logger.Info("Policy enforcer closed")
	return nil
}

// Stats returns statistics about the enforcer
func (e *Enforcer) Stats(ctx context.Context) (activeSessions int) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	activeSessions = len(e.sessionPolicies)
	return
}

// parseTmpfsSize parses a tmpfs size string (e.g., "100m", "1g") to bytes
func parseTmpfsSize(s string) int64 {
	s = strings.ToLower(s)
	multiplier := int64(1)

	if strings.HasSuffix(s, "g") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "g")
	} else if strings.HasSuffix(s, "m") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "m")
	} else if strings.HasSuffix(s, "k") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "k")
	}

	var size int64
	if len(s) > 0 {
		fmt.Sscanf(s, "%d", &size)
	}

	return size * multiplier
}

// String returns a string representation of the enforcer
func (e *Enforcer) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return fmt.Sprintf("PolicyEnforcer{mode: %s, active_sessions: %d}",
		e.policy.Mode, len(e.sessionPolicies))
}
