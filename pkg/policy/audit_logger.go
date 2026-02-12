package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
)

// AuditEventType represents the type of audit event
type AuditEventType string

const (
	// AuditEventTypeMountViolation is logged when a mount violation is detected
	AuditEventTypeMountViolation AuditEventType = "mount_violation"
	// AuditEventTypeMountDenied is logged when a mount is explicitly denied
	AuditEventTypeMountDenied AuditEventType = "mount_denied"
	// AuditEventTypeMountAllowed is logged when a mount is allowed (for audit trail)
	AuditEventTypeMountAllowed AuditEventType = "mount_allowed"
)

// AuditEvent represents a security-related audit event
type AuditEvent struct {
	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`
	// Type is the type of audit event
	Type AuditEventType `json:"type"`
	// Severity is the severity level of the event
	Severity string `json:"severity"`
	// User is the username associated with the event
	User string `json:"user,omitempty"`
	// Group is the group name associated with the event
	Group string `json:"group,omitempty"`
	// SessionID is the session identifier
	SessionID string `json:"session_id,omitempty"`
	// ContainerID is the container identifier
	ContainerID string `json:"container_id,omitempty"`
	// Path is the mount path that was accessed
	Path string `json:"path"`
	// Mode is the requested access mode
	Mode MountAccessMode `json:"mode"`
	// Decision is the enforcement decision (allowed, denied, violation)
	Decision string `json:"decision"`
	// Reason explains why the decision was made
	Reason string `json:"reason,omitempty"`
	// PolicyID is the ID of the policy that was evaluated
	PolicyID string `json:"policy_id,omitempty"`
	// PolicyName is the name of the policy that was evaluated
	PolicyName string `json:"policy_name,omitempty"`
	// SourceIP is the IP address of the requester (if applicable)
	SourceIP string `json:"source_ip,omitempty"`
	// AdditionalContext holds any extra context information
	AdditionalContext map[string]string `json:"additional_context,omitempty"`
}

// AuditLogger handles structured audit logging for security events
type AuditLogger struct {
	logger    *logger.Logger
	mu        sync.RWMutex
	auditFile *os.File
	auditPath string
	closed    bool
}

// NewAuditLogger creates a new audit logger with the specified configuration
func NewAuditLogger(log *logger.Logger, auditPath string) (*AuditLogger, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, fmt.Errorf("failed to create default logger: %w", err)
		}
	}

	al := &AuditLogger{
		logger:    log.With("component", "audit_logger"),
		auditPath: auditPath,
		closed:    false,
	}

	// Initialize audit file if path is provided
	if auditPath != "" {
		al.mu.Lock()
		if err := al.initAuditFileLocked(); err != nil {
			al.mu.Unlock()
			return nil, fmt.Errorf("failed to initialize audit file: %w", err)
		}
		al.mu.Unlock()
	}

	al.logger.Info("Audit logger initialized", "audit_path", auditPath)
	return al, nil
}

// NewDefaultAuditLogger creates a new audit logger with default settings (no file output)
func NewDefaultAuditLogger(log *logger.Logger) (*AuditLogger, error) {
	return NewAuditLogger(log, "")
}

// initAuditFile initializes the audit log file
// Note: This method does NOT acquire a lock. The caller must hold the appropriate lock.
func (al *AuditLogger) initAuditFileLocked() error {
	if al.auditPath == "" {
		return nil
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(al.auditPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create audit log directory: %w", err)
	}

	// Open file in append mode, create if doesn't exist
	file, err := os.OpenFile(al.auditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return fmt.Errorf("failed to open audit log file: %w", err)
	}

	al.auditFile = file
	return nil
}

// LogEvent logs an audit event
func (al *AuditLogger) LogEvent(event AuditEvent) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.closed {
		return fmt.Errorf("audit logger is closed")
	}

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// Marshal event to JSON for structured logging
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	// Log to the underlying logger
	al.logger.Info("audit_event",
		"type", string(event.Type),
		"severity", event.Severity,
		"user", event.User,
		"group", event.Group,
		"session_id", event.SessionID,
		"container_id", event.ContainerID,
		"path", event.Path,
		"mode", string(event.Mode),
		"decision", event.Decision,
		"reason", event.Reason,
		"policy_id", event.PolicyID,
		"policy_name", event.PolicyName,
		"source_ip", event.SourceIP,
		"raw_event", string(eventJSON),
	)

	// Write to audit file if configured
	if al.auditFile != nil {
		// Write raw JSON line to audit file
		if _, err := al.auditFile.Write(append(eventJSON, '\n')); err != nil {
			al.logger.Error("Failed to write audit event to file", "error", err)
			return fmt.Errorf("failed to write audit event to file: %w", err)
		}
		// Sync to ensure data is written
		if err := al.auditFile.Sync(); err != nil {
			al.logger.Error("Failed to sync audit file", "error", err)
		}
	}

	return nil
}

// LogMountViolation logs a mount violation event
func (al *AuditLogger) LogMountViolation(user, group, path string, mode MountAccessMode, reason string, metadata map[string]string) error {
	event := AuditEvent{
		Type:       AuditEventTypeMountViolation,
		Severity:   "error",
		User:       user,
		Group:      group,
		Path:       path,
		Mode:       mode,
		Decision:   "denied",
		Reason:     reason,
		AdditionalContext: metadata,
	}

	return al.LogEvent(event)
}

// LogMountDenied logs a mount denied event (explicit deny)
func (al *AuditLogger) LogMountDenied(user, group, path string, mode MountAccessMode, reason string, metadata map[string]string) error {
	event := AuditEvent{
		Type:       AuditEventTypeMountDenied,
		Severity:   "warning",
		User:       user,
		Group:      group,
		Path:       path,
		Mode:       mode,
		Decision:   "denied",
		Reason:     reason,
		AdditionalContext: metadata,
	}

	return al.LogEvent(event)
}

// LogMountAllowed logs a mount allowed event (for audit trail)
func (al *AuditLogger) LogMountAllowed(user, group, path string, mode MountAccessMode, metadata map[string]string) error {
	event := AuditEvent{
		Type:       AuditEventTypeMountAllowed,
		Severity:   "info",
		User:       user,
		Group:      group,
		Path:       path,
		Mode:       mode,
		Decision:   "allowed",
		AdditionalContext: metadata,
	}

	return al.LogEvent(event)
}

// WithPolicyInfo adds policy information to the audit event context
func (al *AuditLogger) WithPolicyInfo(policyID, policyName string, event AuditEvent) AuditEvent {
	event.PolicyID = policyID
	event.PolicyName = policyName
	return event
}

// WithSessionInfo adds session information to the audit event context
func (al *AuditLogger) WithSessionInfo(sessionID, containerID string, event AuditEvent) AuditEvent {
	event.SessionID = sessionID
	event.ContainerID = containerID
	return event
}

// Close closes the audit logger and any open files
func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.closed {
		return nil
	}

	al.closed = true

	if al.auditFile != nil {
		if err := al.auditFile.Close(); err != nil {
			return fmt.Errorf("failed to close audit file: %w", err)
		}
		al.auditFile = nil
	}

	al.logger.Info("Audit logger closed")
	return nil
}

// SetAuditPath changes the audit log file path (closes existing file if any)
func (al *AuditLogger) SetAuditPath(path string) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	// Close existing file if open
	if al.auditFile != nil {
		if err := al.auditFile.Close(); err != nil {
			return fmt.Errorf("failed to close existing audit file: %w", err)
		}
		al.auditFile = nil
	}

	al.auditPath = path

	// Initialize new file if path is not empty
	if path != "" {
		if err := al.initAuditFileLocked(); err != nil {
			return fmt.Errorf("failed to initialize new audit file: %w", err)
		}
	}

	al.logger.Info("Audit log path updated", "audit_path", path)
	return nil
}

// GetAuditPath returns the current audit log file path
func (al *AuditLogger) GetAuditPath() string {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return al.auditPath
}
