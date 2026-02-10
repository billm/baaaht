package policy

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestMountAccessModeConstants verifies that MountAccessMode constants are defined correctly
func TestMountAccessModeConstants(t *testing.T) {
	tests := []struct {
		name  string
		mode  MountAccessMode
		value string
	}{
		{
			name:  "readonly mode",
			mode:  MountAccessModeReadOnly,
			value: "readonly",
		},
		{
			name:  "readwrite mode",
			mode:  MountAccessModeReadWrite,
			value: "readwrite",
		},
		{
			name:  "denied mode",
			mode:  MountAccessModeDenied,
			value: "denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.mode) != tt.value {
				t.Errorf("MountAccessMode value mismatch: got %s, want %s", tt.mode, tt.value)
			}
		})
	}
}

// TestMountAllowlistEntryFields verifies that MountAllowlistEntry has all required fields
func TestMountAllowlistEntryFields(t *testing.T) {
	entry := MountAllowlistEntry{
		Path:  "/home/user/data",
		User:  "alice",
		Group: "developers",
		Mode:  MountAccessModeReadOnly,
	}

	if entry.Path != "/home/user/data" {
		t.Errorf("Path mismatch: got %s, want /home/user/data", entry.Path)
	}

	if entry.User != "alice" {
		t.Errorf("User mismatch: got %s, want alice", entry.User)
	}

	if entry.Group != "developers" {
		t.Errorf("Group mismatch: got %s, want developers", entry.Group)
	}

	if entry.Mode != MountAccessModeReadOnly {
		t.Errorf("Mode mismatch: got %s, want readonly", entry.Mode)
	}
}

// TestMountAllowlistEntryOptionalFields verifies that User and Group can be empty
func TestMountAllowlistEntryOptionalFields(t *testing.T) {
	entry := MountAllowlistEntry{
		Path: "/shared/data",
		Mode: MountAccessModeReadWrite,
	}

	if entry.User != "" {
		t.Errorf("User should be empty by default, got %s", entry.User)
	}

	if entry.Group != "" {
		t.Errorf("Group should be empty by default, got %s", entry.Group)
	}

	if entry.Path != "/shared/data" {
		t.Errorf("Path mismatch: got %s, want /shared/data", entry.Path)
	}

	if entry.Mode != MountAccessModeReadWrite {
		t.Errorf("Mode mismatch: got %s, want readwrite", entry.Mode)
	}
}

// TestMountAllowlistEntryJSONSerialization verifies JSON marshaling
func TestMountAllowlistEntryJSONSerialization(t *testing.T) {
	entry := MountAllowlistEntry{
		Path:  "/var/data",
		User:  "bob",
		Group: "admins",
		Mode:  MountAccessModeDenied,
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal to JSON: %v", err)
	}

	// Unmarshal back
	var unmarshaled MountAllowlistEntry
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal from JSON: %v", err)
	}

	// Verify all fields match
	if unmarshaled.Path != entry.Path {
		t.Errorf("Path mismatch after round-trip: got %s, want %s", unmarshaled.Path, entry.Path)
	}

	if unmarshaled.User != entry.User {
		t.Errorf("User mismatch after round-trip: got %s, want %s", unmarshaled.User, entry.User)
	}

	if unmarshaled.Group != entry.Group {
		t.Errorf("Group mismatch after round-trip: got %s, want %s", unmarshaled.Group, entry.Group)
	}

	if unmarshaled.Mode != entry.Mode {
		t.Errorf("Mode mismatch after round-trip: got %s, want %s", unmarshaled.Mode, entry.Mode)
	}
}

// TestMountAllowlistEntryYAMLSerialization verifies YAML marshaling
func TestMountAllowlistEntryYAMLSerialization(t *testing.T) {
	entry := MountAllowlistEntry{
		Path:  "/opt/app",
		Mode:  MountAccessModeReadWrite,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal to YAML: %v", err)
	}

	// Unmarshal back
	var unmarshaled MountAllowlistEntry
	if err := yaml.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal from YAML: %v", err)
	}

	// Verify fields match
	if unmarshaled.Path != entry.Path {
		t.Errorf("Path mismatch after round-trip: got %s, want %s", unmarshaled.Path, entry.Path)
	}

	if unmarshaled.User != entry.User {
		t.Errorf("User mismatch after round-trip: got %s, want %s", unmarshaled.User, entry.User)
	}

	if unmarshaled.Group != entry.Group {
		t.Errorf("Group mismatch after round-trip: got %s, want %s", unmarshaled.Group, entry.Group)
	}

	if unmarshaled.Mode != entry.Mode {
		t.Errorf("Mode mismatch after round-trip: got %s, want %s", unmarshaled.Mode, entry.Mode)
	}
}

// TestMountAllowlistEntryUserOnly verifies entry with only user scoping
func TestMountAllowlistEntryUserOnly(t *testing.T) {
	entry := MountAllowlistEntry{
		Path: "/home/alice/projects",
		User: "alice",
		Mode: MountAccessModeReadWrite,
	}

	if entry.User != "alice" {
		t.Errorf("User should be alice, got %s", entry.User)
	}

	if entry.Group != "" {
		t.Errorf("Group should be empty when not set, got %s", entry.Group)
	}
}

// TestMountAllowlistEntryGroupOnly verifies entry with only group scoping
func TestMountAllowlistEntryGroupOnly(t *testing.T) {
	entry := MountAllowlistEntry{
		Path:  "/shared/projects",
		Group: "team-alpha",
		Mode:  MountAccessModeReadOnly,
	}

	if entry.Group != "team-alpha" {
		t.Errorf("Group should be team-alpha, got %s", entry.Group)
	}

	if entry.User != "" {
		t.Errorf("User should be empty when not set, got %s", entry.User)
	}
}

// TestMountAllowlistEntryYAMLParsing verifies parsing from YAML format
func TestMountAllowlistEntryYAMLParsing(t *testing.T) {
	yamlContent := `
path: /home/user/documents
user: john
group: writers
mode: readonly
`

	var entry MountAllowlistEntry
	if err := yaml.Unmarshal([]byte(yamlContent), &entry); err != nil {
		t.Fatalf("failed to parse YAML: %v", err)
	}

	if entry.Path != "/home/user/documents" {
		t.Errorf("Path mismatch: got %s, want /home/user/documents", entry.Path)
	}

	if entry.User != "john" {
		t.Errorf("User mismatch: got %s, want john", entry.User)
	}

	if entry.Group != "writers" {
		t.Errorf("Group mismatch: got %s, want writers", entry.Group)
	}

	if entry.Mode != MountAccessModeReadOnly {
		t.Errorf("Mode mismatch: got %s, want readonly", entry.Mode)
	}
}
