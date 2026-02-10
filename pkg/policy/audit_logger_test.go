package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
)

// TestNewAuditLogger tests creating a new audit logger
func TestNewAuditLogger(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	// Test audit logger without file output
	al, err := NewAuditLogger(log, "")
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	if al == nil {
		t.Fatal("expected audit logger to be non-nil")
	}

	if al.auditPath != "" {
		t.Errorf("expected empty audit path, got %s", al.auditPath)
	}

	if al.auditFile != nil {
		t.Error("expected audit file to be nil when no path is provided")
	}
}

// TestNewAuditLoggerWithFile tests creating an audit logger with file output
func TestNewAuditLoggerWithFile(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger with file: %v", err)
	}
	defer al.Close()

	if al.auditPath != auditPath {
		t.Errorf("expected audit path %s, got %s", auditPath, al.auditPath)
	}

	if al.auditFile == nil {
		t.Error("expected audit file to be non-nil when path is provided")
	}

	// Verify file was created
	if _, err := os.Stat(auditPath); os.IsNotExist(err) {
		t.Error("audit file was not created")
	}
}

// TestNewDefaultAuditLogger tests creating a default audit logger
func TestNewDefaultAuditLogger(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	al, err := NewDefaultAuditLogger(log)
	if err != nil {
		t.Fatalf("failed to create default audit logger: %v", err)
	}
	defer al.Close()

	if al == nil {
		t.Fatal("expected audit logger to be non-nil")
	}
}

// TestLogEvent tests logging a generic audit event
func TestLogEvent(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	event := AuditEvent{
		Type:       AuditEventTypeMountViolation,
		Severity:   "error",
		User:       "testuser",
		Group:      "testgroup",
		Path:       "/test/path",
		Mode:       MountAccessModeReadOnly,
		Decision:   "denied",
		Reason:     "path not in allowlist",
		PolicyID:   "policy-123",
		PolicyName: "Test Policy",
	}

	if err := al.LogEvent(event); err != nil {
		t.Fatalf("failed to log event: %v", err)
	}

	// Verify event was written to file
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}

	var loggedEvent AuditEvent
	if err := json.Unmarshal(data, &loggedEvent); err != nil {
		t.Fatalf("failed to unmarshal logged event: %v", err)
	}

	if loggedEvent.Type != AuditEventTypeMountViolation {
		t.Errorf("expected event type %s, got %s", AuditEventTypeMountViolation, loggedEvent.Type)
	}

	if loggedEvent.User != "testuser" {
		t.Errorf("expected user testuser, got %s", loggedEvent.User)
	}

	if loggedEvent.Path != "/test/path" {
		t.Errorf("expected path /test/path, got %s", loggedEvent.Path)
	}
}

// TestLogEventWithTimestamp tests that timestamps are set automatically
func TestLogEventWithTimestamp(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	before := time.Now().UTC()

	event := AuditEvent{
		Type:     AuditEventTypeMountViolation,
		Severity: "error",
		Path:     "/test/path",
		Mode:     MountAccessModeReadOnly,
		Decision: "denied",
	}

	if err := al.LogEvent(event); err != nil {
		t.Fatalf("failed to log event: %v", err)
	}

	after := time.Now().UTC()

	// Verify event was written to file with timestamp
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}

	var loggedEvent AuditEvent
	if err := json.Unmarshal(data, &loggedEvent); err != nil {
		t.Fatalf("failed to unmarshal logged event: %v", err)
	}

	// Verify timestamp was set in logged event
	if loggedEvent.Timestamp.IsZero() {
		t.Error("expected timestamp to be set in logged event")
	}

	// Verify timestamp is within reasonable range
	if loggedEvent.Timestamp.Before(before) || loggedEvent.Timestamp.After(after) {
		t.Errorf("timestamp %v is outside expected range [%v, %v]", loggedEvent.Timestamp, before, after)
	}
}

// TestLogMountViolation tests the LogMountViolation method
func TestLogMountViolation(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	metadata := map[string]string{
		"source_ip": "192.168.1.100",
		"attempt":   "1",
	}

	err = al.LogMountViolation("alice", "developers", "/etc/passwd", MountAccessModeReadWrite, "path not in allowlist", metadata)
	if err != nil {
		t.Fatalf("failed to log mount violation: %v", err)
	}

	// Verify event was written to file
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}

	var loggedEvent AuditEvent
	if err := json.Unmarshal(data, &loggedEvent); err != nil {
		t.Fatalf("failed to unmarshal logged event: %v", err)
	}

	// Verify all fields
	if loggedEvent.Type != AuditEventTypeMountViolation {
		t.Errorf("expected event type %s, got %s", AuditEventTypeMountViolation, loggedEvent.Type)
	}

	if loggedEvent.Severity != "error" {
		t.Errorf("expected severity error, got %s", loggedEvent.Severity)
	}

	if loggedEvent.User != "alice" {
		t.Errorf("expected user alice, got %s", loggedEvent.User)
	}

	if loggedEvent.Group != "developers" {
		t.Errorf("expected group developers, got %s", loggedEvent.Group)
	}

	if loggedEvent.Path != "/etc/passwd" {
		t.Errorf("expected path /etc/passwd, got %s", loggedEvent.Path)
	}

	if loggedEvent.Mode != MountAccessModeReadWrite {
		t.Errorf("expected mode readwrite, got %s", loggedEvent.Mode)
	}

	if loggedEvent.Decision != "denied" {
		t.Errorf("expected decision denied, got %s", loggedEvent.Decision)
	}

	if loggedEvent.Reason != "path not in allowlist" {
		t.Errorf("expected reason 'path not in allowlist', got %s", loggedEvent.Reason)
	}

	if loggedEvent.AdditionalContext["source_ip"] != "192.168.1.100" {
		t.Errorf("expected source_ip 192.168.1.100, got %s", loggedEvent.AdditionalContext["source_ip"])
	}

	if loggedEvent.AdditionalContext["attempt"] != "1" {
		t.Errorf("expected attempt 1, got %s", loggedEvent.AdditionalContext["attempt"])
	}
}

// TestLogMountDenied tests the LogMountDenied method
func TestLogMountDenied(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	err = al.LogMountDenied("bob", "admins", "/root", MountAccessModeReadWrite, "explicit deny", nil)
	if err != nil {
		t.Fatalf("failed to log mount denied: %v", err)
	}

	// Verify event was written to file
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}

	var loggedEvent AuditEvent
	if err := json.Unmarshal(data, &loggedEvent); err != nil {
		t.Fatalf("failed to unmarshal logged event: %v", err)
	}

	if loggedEvent.Type != AuditEventTypeMountDenied {
		t.Errorf("expected event type %s, got %s", AuditEventTypeMountDenied, loggedEvent.Type)
	}

	if loggedEvent.Severity != "warning" {
		t.Errorf("expected severity warning, got %s", loggedEvent.Severity)
	}

	if loggedEvent.User != "bob" {
		t.Errorf("expected user bob, got %s", loggedEvent.User)
	}

	if loggedEvent.Reason != "explicit deny" {
		t.Errorf("expected reason 'explicit deny', got %s", loggedEvent.Reason)
	}
}

// TestLogMountAllowed tests the LogMountAllowed method
func TestLogMountAllowed(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	err = al.LogMountAllowed("charlie", "users", "/home/charlie/data", MountAccessModeReadOnly, nil)
	if err != nil {
		t.Fatalf("failed to log mount allowed: %v", err)
	}

	// Verify event was written to file
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}

	var loggedEvent AuditEvent
	if err := json.Unmarshal(data, &loggedEvent); err != nil {
		t.Fatalf("failed to unmarshal logged event: %v", err)
	}

	if loggedEvent.Type != AuditEventTypeMountAllowed {
		t.Errorf("expected event type %s, got %s", AuditEventTypeMountAllowed, loggedEvent.Type)
	}

	if loggedEvent.Severity != "info" {
		t.Errorf("expected severity info, got %s", loggedEvent.Severity)
	}

	if loggedEvent.User != "charlie" {
		t.Errorf("expected user charlie, got %s", loggedEvent.User)
	}

	if loggedEvent.Decision != "allowed" {
		t.Errorf("expected decision allowed, got %s", loggedEvent.Decision)
	}
}

// TestAuditEventJSONSerialization tests JSON marshaling of audit events
func TestAuditEventJSONSerialization(t *testing.T) {
	event := AuditEvent{
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Type:        AuditEventTypeMountViolation,
		Severity:    "error",
		User:        "testuser",
		Group:       "testgroup",
		SessionID:   "session-123",
		ContainerID: "container-456",
		Path:        "/test/path",
		Mode:        MountAccessModeReadOnly,
		Decision:    "denied",
		Reason:      "test reason",
		PolicyID:    "policy-789",
		PolicyName:  "Test Policy",
		SourceIP:    "192.168.1.1",
		AdditionalContext: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	var unmarshaled AuditEvent
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if unmarshaled.Type != event.Type {
		t.Errorf("type mismatch: got %s, want %s", unmarshaled.Type, event.Type)
	}

	if unmarshaled.User != event.User {
		t.Errorf("user mismatch: got %s, want %s", unmarshaled.User, event.User)
	}

	if unmarshaled.Path != event.Path {
		t.Errorf("path mismatch: got %s, want %s", unmarshaled.Path, event.Path)
	}

	if unmarshaled.AdditionalContext["key1"] != "value1" {
		t.Errorf("additional_context key1 mismatch: got %s, want value1", unmarshaled.AdditionalContext["key1"])
	}
}

// TestMultipleEvents tests logging multiple events to the same file
func TestMultipleEvents(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	// Log multiple events
	events := []struct {
		user    string
		group   string
		path    string
		mode    MountAccessMode
		reason  string
	}{
		{"user1", "group1", "/path1", MountAccessModeReadOnly, "reason1"},
		{"user2", "group2", "/path2", MountAccessModeReadWrite, "reason2"},
		{"user3", "group3", "/path3", MountAccessModeDenied, "reason3"},
	}

	for _, e := range events {
		if err := al.LogMountViolation(e.user, e.group, e.path, e.mode, e.reason, nil); err != nil {
			t.Fatalf("failed to log event: %v", err)
		}
	}

	// Verify all events were written
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(events) {
		t.Errorf("expected %d log lines, got %d", len(events), len(lines))
	}

	// Verify each event
	for i, line := range lines {
		var loggedEvent AuditEvent
		if err := json.Unmarshal([]byte(line), &loggedEvent); err != nil {
			t.Fatalf("failed to unmarshal event at line %d: %v", i+1, err)
		}

		if loggedEvent.User != events[i].user {
			t.Errorf("line %d: expected user %s, got %s", i+1, events[i].user, loggedEvent.User)
		}

		if loggedEvent.Path != events[i].path {
			t.Errorf("line %d: expected path %s, got %s", i+1, events[i].path, loggedEvent.Path)
		}
	}
}

// TestLogEventAfterClose tests that logging after close returns an error
func TestLogEventAfterClose(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	al, err := NewDefaultAuditLogger(log)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}

	// Close the audit logger
	if err := al.Close(); err != nil {
		t.Fatalf("failed to close audit logger: %v", err)
	}

	// Try to log an event after closing
	event := AuditEvent{
		Type:     AuditEventTypeMountViolation,
		Severity: "error",
		Path:     "/test/path",
		Mode:     MountAccessModeReadOnly,
		Decision: "denied",
	}

	err = al.LogEvent(event)
	if err == nil {
		t.Error("expected error when logging after close, got nil")
	}

	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected error message to contain 'closed', got %s", err.Error())
	}
}

// TestWithPolicyInfo tests adding policy info to audit events
func TestWithPolicyInfo(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	al, err := NewDefaultAuditLogger(log)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	event := AuditEvent{
		Type:     AuditEventTypeMountViolation,
		Severity: "error",
		Path:     "/test/path",
		Mode:     MountAccessModeReadOnly,
		Decision: "denied",
	}

	event = al.WithPolicyInfo("policy-123", "Test Policy", event)

	if event.PolicyID != "policy-123" {
		t.Errorf("expected policy ID policy-123, got %s", event.PolicyID)
	}

	if event.PolicyName != "Test Policy" {
		t.Errorf("expected policy name 'Test Policy', got %s", event.PolicyName)
	}
}

// TestWithSessionInfo tests adding session info to audit events
func TestWithSessionInfo(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	al, err := NewDefaultAuditLogger(log)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	event := AuditEvent{
		Type:     AuditEventTypeMountViolation,
		Severity: "error",
		Path:     "/test/path",
		Mode:     MountAccessModeReadOnly,
		Decision: "denied",
	}

	event = al.WithSessionInfo("session-abc", "container-xyz", event)

	if event.SessionID != "session-abc" {
		t.Errorf("expected session ID session-abc, got %s", event.SessionID)
	}

	if event.ContainerID != "container-xyz" {
		t.Errorf("expected container ID container-xyz, got %s", event.ContainerID)
	}
}

// TestSetAuditPath tests changing the audit path dynamically
func TestSetAuditPath(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath1 := filepath.Join(tmpDir, "audit1.log")
	auditPath2 := filepath.Join(tmpDir, "audit2.log")

	al, err := NewAuditLogger(log, auditPath1)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	// Log to first file
	if err := al.LogMountViolation("user1", "group1", "/path1", MountAccessModeReadOnly, "test", nil); err != nil {
		t.Fatalf("failed to log event: %v", err)
	}

	// Verify first file exists
	if _, err := os.Stat(auditPath1); os.IsNotExist(err) {
		t.Error("first audit file was not created")
	}

	// Change audit path
	if err := al.SetAuditPath(auditPath2); err != nil {
		t.Fatalf("failed to set audit path: %v", err)
	}

	// Log to second file
	if err := al.LogMountViolation("user2", "group2", "/path2", MountAccessModeReadOnly, "test", nil); err != nil {
		t.Fatalf("failed to log event: %v", err)
	}

	// Verify second file exists
	if _, err := os.Stat(auditPath2); os.IsNotExist(err) {
		t.Error("second audit file was not created")
	}

	// Verify second file has content
	data, err := os.ReadFile(auditPath2)
	if err != nil {
		t.Fatalf("failed to read second audit file: %v", err)
	}

	var loggedEvent AuditEvent
	if err := json.Unmarshal(data, &loggedEvent); err != nil {
		t.Fatalf("failed to unmarshal logged event: %v", err)
	}

	if loggedEvent.User != "user2" {
		t.Errorf("expected user user2, got %s", loggedEvent.User)
	}
}

// TestGetAuditPath tests retrieving the current audit path
func TestGetAuditPath(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	if al.GetAuditPath() != auditPath {
		t.Errorf("expected audit path %s, got %s", auditPath, al.GetAuditPath())
	}

	// Test with empty path
	al2, err := NewDefaultAuditLogger(log)
	if err != nil {
		t.Fatalf("failed to create default audit logger: %v", err)
	}
	defer al2.Close()

	if al2.GetAuditPath() != "" {
		t.Errorf("expected empty audit path, got %s", al2.GetAuditPath())
	}
}

// TestCloseIsIdempotent tests that closing multiple times is safe
func TestCloseIsIdempotent(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}

	// First close
	if err := al.Close(); err != nil {
		t.Fatalf("failed to close audit logger: %v", err)
	}

	// Second close should not error
	if err := al.Close(); err != nil {
		t.Errorf("expected second close to return nil, got %v", err)
	}
}

// TestAuditLogFormat verifies the audit log output format and content
func TestAuditLogFormat(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	// Test complete event with all fields populated
	metadata := map[string]string{
		"source_ip":    "192.168.1.100",
		"attempt":      "1",
		"custom_field": "custom_value",
	}

	err = al.LogMountViolation("testuser", "testgroup", "/test/path", MountAccessModeReadWrite, "test violation", metadata)
	if err != nil {
		t.Fatalf("failed to log mount violation: %v", err)
	}

	// Read the audit file and verify format
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}

	// Verify it's valid JSON
	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("audit log is not valid JSON: %v", err)
	}

	// Verify timestamp format (should be ISO 8601 / RFC3339)
	timestamp, ok := event["timestamp"].(string)
	if !ok {
		t.Fatal("timestamp field is missing or not a string")
	}
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		t.Errorf("timestamp is not in RFC3339 format: %s, error: %v", timestamp, err)
	}

	// Verify type field
	eventType, ok := event["type"].(string)
	if !ok {
		t.Error("type field is missing or not a string")
	} else if eventType != string(AuditEventTypeMountViolation) {
		t.Errorf("expected type %s, got %s", AuditEventTypeMountViolation, eventType)
	}

	// Verify severity field
	severity, ok := event["severity"].(string)
	if !ok {
		t.Error("severity field is missing or not a string")
	} else if severity != "error" {
		t.Errorf("expected severity error, got %s", severity)
	}

	// Verify user field
	user, ok := event["user"].(string)
	if !ok {
		t.Error("user field is missing or not a string")
	} else if user != "testuser" {
		t.Errorf("expected user testuser, got %s", user)
	}

	// Verify group field
	group, ok := event["group"].(string)
	if !ok {
		t.Error("group field is missing or not a string")
	} else if group != "testgroup" {
		t.Errorf("expected group testgroup, got %s", group)
	}

	// Verify path field (required)
	path, ok := event["path"].(string)
	if !ok {
		t.Error("path field is missing or not a string")
	} else if path != "/test/path" {
		t.Errorf("expected path /test/path, got %s", path)
	}

	// Verify mode field
	mode, ok := event["mode"].(string)
	if !ok {
		t.Error("mode field is missing or not a string")
	} else if mode != string(MountAccessModeReadWrite) {
		t.Errorf("expected mode readwrite, got %s", mode)
	}

	// Verify decision field
	decision, ok := event["decision"].(string)
	if !ok {
		t.Error("decision field is missing or not a string")
	} else if decision != "denied" {
		t.Errorf("expected decision denied, got %s", decision)
	}

	// Verify reason field
	reason, ok := event["reason"].(string)
	if !ok {
		t.Error("reason field is missing or not a string")
	} else if reason != "test violation" {
		t.Errorf("expected reason 'test violation', got %s", reason)
	}

	// Verify additional_context field
	additionalContext, ok := event["additional_context"].(map[string]interface{})
	if !ok {
		t.Error("additional_context field is missing or not a map")
	} else {
		// Check metadata values
		if additionalContext["source_ip"] != "192.168.1.100" {
			t.Errorf("expected source_ip 192.168.1.100, got %v", additionalContext["source_ip"])
		}
		if additionalContext["attempt"] != "1" {
			t.Errorf("expected attempt 1, got %v", additionalContext["attempt"])
		}
		if additionalContext["custom_field"] != "custom_value" {
			t.Errorf("expected custom_field custom_value, got %v", additionalContext["custom_field"])
		}
	}

	// Verify JSON formatting is compact (no extra whitespace)
	jsonStr := string(data)
	// Trim the trailing newline (which is used to separate log entries)
	jsonStr = strings.TrimSuffix(jsonStr, "\n")
	if strings.Contains(jsonStr, "\n") {
		t.Error("audit log JSON should be single-line")
	}
	if strings.Contains(jsonStr, "  ") {
		t.Error("audit log JSON should not contain extra spaces")
	}
}

// TestAuditLogFormatWithOptionalFields verifies format with minimal required fields
func TestAuditLogFormatWithOptionalFields(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	// Test with minimal fields (no optional user/group/metadata)
	err = al.LogMountAllowed("", "", "/allowed/path", MountAccessModeReadOnly, nil)
	if err != nil {
		t.Fatalf("failed to log mount allowed: %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}

	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("audit log is not valid JSON: %v", err)
	}

	// Verify optional fields are either absent or empty
	if user, ok := event["user"].(string); ok && user != "" {
		t.Errorf("expected empty or missing user field, got %s", user)
	}

	// Verify required fields are present
	if event["path"] == nil {
		t.Error("path field is required but missing")
	}
	if event["type"] == nil {
		t.Error("type field is required but missing")
	}
	if event["decision"] == nil {
		t.Error("decision field is required but missing")
	}
}

// TestAuditLogFormatForAllEventTypes verifies format for all event types
func TestAuditLogFormatForAllEventTypes(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create default logger: %v", err)
	}

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	al, err := NewAuditLogger(log, auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer al.Close()

	testCases := []struct {
		name           string
		logFunc        func() error
		expectedType   AuditEventType
		expectedSeverity string
		expectedDecision string
	}{
		{
			name: "mount_violation",
			logFunc: func() error {
				return al.LogMountViolation("user1", "group1", "/violation/path", MountAccessModeReadWrite, "violation", nil)
			},
			expectedType:    AuditEventTypeMountViolation,
			expectedSeverity: "error",
			expectedDecision: "denied",
		},
		{
			name: "mount_denied",
			logFunc: func() error {
				return al.LogMountDenied("user2", "group2", "/denied/path", MountAccessModeReadWrite, "explicit deny", nil)
			},
			expectedType:    AuditEventTypeMountDenied,
			expectedSeverity: "warning",
			expectedDecision: "denied",
		},
		{
			name: "mount_allowed",
			logFunc: func() error {
				return al.LogMountAllowed("user3", "group3", "/allowed/path", MountAccessModeReadOnly, nil)
			},
			expectedType:    AuditEventTypeMountAllowed,
			expectedSeverity: "info",
			expectedDecision: "allowed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear the audit file for each test
			if err := os.Truncate(auditPath, 0); err != nil {
				t.Fatalf("failed to truncate audit file: %v", err)
			}

			if err := tc.logFunc(); err != nil {
				t.Fatalf("failed to log event: %v", err)
			}

			data, err := os.ReadFile(auditPath)
			if err != nil {
				t.Fatalf("failed to read audit file: %v", err)
			}

			var event map[string]interface{}
			if err := json.Unmarshal(data, &event); err != nil {
				t.Fatalf("audit log is not valid JSON: %v", err)
			}

			if event["type"] != string(tc.expectedType) {
				t.Errorf("expected type %s, got %v", tc.expectedType, event["type"])
			}

			if event["severity"] != tc.expectedSeverity {
				t.Errorf("expected severity %s, got %v", tc.expectedSeverity, event["severity"])
			}

			if event["decision"] != tc.expectedDecision {
				t.Errorf("expected decision %s, got %v", tc.expectedDecision, event["decision"])
			}
		})
	}
}
