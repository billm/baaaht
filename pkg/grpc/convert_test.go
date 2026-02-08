package grpc

import (
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// TestConvert_SessionState tests session state conversion
func TestConvert_SessionState(t *testing.T) {
	tests := []struct {
		name     string
		proto    proto.SessionState
		expected types.SessionState
	}{
		{"initializing", proto.SessionState_SESSION_STATE_INITIALIZING, types.SessionStateInitializing},
		{"active", proto.SessionState_SESSION_STATE_ACTIVE, types.SessionStateActive},
		{"idle", proto.SessionState_SESSION_STATE_IDLE, types.SessionStateIdle},
		{"closing", proto.SessionState_SESSION_STATE_CLOSING, types.SessionStateClosing},
		{"closed", proto.SessionState_SESSION_STATE_CLOSED, types.SessionStateClosed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protoToSessionState(tt.proto)
			if got != tt.expected {
				t.Errorf("protoToSessionState() = %v, want %v", got, tt.expected)
			}

			// Test round-trip conversion
			back := sessionStateToProto(got)
			if back != tt.proto {
				t.Errorf("round-trip: sessionStateToProto(protoToSessionState(%v)) = %v, want %v", tt.proto, back, tt.proto)
			}
		})
	}
}

// TestConvert_Status tests status conversion
func TestConvert_Status(t *testing.T) {
	tests := []struct {
		name     string
		proto    proto.Status
		expected types.Status
	}{
		{"unknown", proto.Status_STATUS_UNKNOWN, types.StatusUnknown},
		{"starting", proto.Status_STATUS_STARTING, types.StatusStarting},
		{"running", proto.Status_STATUS_RUNNING, types.StatusRunning},
		{"stopping", proto.Status_STATUS_STOPPING, types.StatusStopping},
		{"stopped", proto.Status_STATUS_STOPPED, types.StatusStopped},
		{"error", proto.Status_STATUS_ERROR, types.StatusError},
		{"terminated", proto.Status_STATUS_TERMINATED, types.StatusTerminated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protoToStatus(tt.proto)
			if got != tt.expected {
				t.Errorf("protoToStatus() = %v, want %v", got, tt.expected)
			}

			// Test round-trip conversion
			back := statusToProto(got)
			if back != tt.proto {
				t.Errorf("round-trip: statusToProto(protoToStatus(%v)) = %v, want %v", tt.proto, back, tt.proto)
			}
		})
	}
}

// TestConvert_Health tests health conversion
func TestConvert_Health(t *testing.T) {
	tests := []struct {
		name     string
		proto    proto.Health
		expected types.Health
	}{
		{"unknown", proto.Health_HEALTH_UNKNOWN, types.HealthUnknown},
		{"healthy", proto.Health_HEALTH_HEALTHY, types.Healthy},
		{"unhealthy", proto.Health_HEALTH_UNHEALTHY, types.Unhealthy},
		{"degraded", proto.Health_HEALTH_DEGRADED, types.Degraded},
		{"checking", proto.Health_HEALTH_CHECKING, types.HealthChecking},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protoToHealth(tt.proto)
			if got != tt.expected {
				t.Errorf("protoToHealth() = %v, want %v", got, tt.expected)
			}

			// Test round-trip conversion
			back := healthToProto(got)
			if back != tt.proto {
				t.Errorf("round-trip: healthToProto(protoToHealth(%v)) = %v, want %v", tt.proto, back, tt.proto)
			}
		})
	}
}

// TestConvert_Timestamp tests timestamp conversion
func TestConvert_Timestamp(t *testing.T) {
	now := time.Now()
	ts := types.NewTimestampFromTime(now)

	// Convert to proto and back
	pbTs := timestampToProto(&ts)
	got := protoToTimestamp(pbTs)

	if got == nil {
		t.Fatal("protoToTimestamp() returned nil")
	}

	// Allow 1 second difference due to protobuf timestamp precision
	diff := got.Time.Sub(now).Abs()
	if diff > time.Second {
		t.Errorf("timestamp conversion lost precision: diff = %v", diff)
	}
}

// TestConvert_Timestamp_Nil tests nil timestamp conversion
func TestConvert_Timestamp_Nil(t *testing.T) {
	got := protoToTimestamp(nil)
	if got != nil {
		t.Errorf("protoToTimestamp(nil) = %v, want nil", got)
	}

	got2 := timestampToProto(nil)
	if got2 != nil {
		t.Errorf("timestampToProto(nil) = %v, want nil", got2)
	}
}

// TestConvert_ContainerState tests container state conversion
func TestConvert_ContainerState(t *testing.T) {
	tests := []struct {
		name     string
		proto    proto.ContainerState
		expected types.ContainerState
	}{
		{"created", proto.ContainerState_CONTAINER_STATE_CREATED, types.ContainerStateCreated},
		{"running", proto.ContainerState_CONTAINER_STATE_RUNNING, types.ContainerStateRunning},
		{"paused", proto.ContainerState_CONTAINER_STATE_PAUSED, types.ContainerStatePaused},
		{"restarting", proto.ContainerState_CONTAINER_STATE_RESTARTING, types.ContainerStateRestarting},
		{"exited", proto.ContainerState_CONTAINER_STATE_EXITED, types.ContainerStateExited},
		{"removing", proto.ContainerState_CONTAINER_STATE_REMOVING, types.ContainerStateRemoving},
		{"unknown", proto.ContainerState_CONTAINER_STATE_UNKNOWN, types.ContainerStateUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protoToContainerState(tt.proto)
			if got != tt.expected {
				t.Errorf("protoToContainerState() = %v, want %v", got, tt.expected)
			}

			// Test round-trip conversion
			back := containerStateToProto(got)
			if back != tt.proto {
				t.Errorf("round-trip: containerStateToProto(protoToContainerState(%v)) = %v, want %v", tt.proto, back, tt.proto)
			}
		})
	}
}

// TestConvert_MountType tests mount type conversion
func TestConvert_MountType(t *testing.T) {
	tests := []struct {
		name     string
		proto    proto.MountType
		expected types.MountType
	}{
		{"bind", proto.MountType_MOUNT_TYPE_BIND, types.MountTypeBind},
		{"volume", proto.MountType_MOUNT_TYPE_VOLUME, types.MountTypeVolume},
		{"tmpfs", proto.MountType_MOUNT_TYPE_TMPFS, types.MountTypeTmpfs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protoToMountType(tt.proto)
			if got != tt.expected {
				t.Errorf("protoToMountType() = %v, want %v", got, tt.expected)
			}

			// Test round-trip conversion
			back := mountTypeToProto(got)
			if back != tt.proto {
				t.Errorf("round-trip: mountTypeToProto(protoToMountType(%v)) = %v, want %v", tt.proto, back, tt.proto)
			}
		})
	}
}

// TestConvert_Container tests full container conversion
func TestConvert_Container(t *testing.T) {
	now := time.Now()
	exitCode := 127

	original := types.Container{
		ID:        types.GenerateID(),
		Name:      "test-container",
		Image:     "ubuntu:latest",
		State:     types.ContainerStateRunning,
		Status:    types.StatusRunning,
		Health:    types.Healthy,
		CreatedAt: types.NewTimestampFromTime(now.Add(-1 * time.Hour)),
		StartedAt: (*types.Timestamp)(&[]types.Timestamp{types.NewTimestampFromTime(now.Add(-59 * time.Minute))}[0]),
		ExitCode:  &exitCode,
		Config: types.ContainerConfig{
			Image:      "ubuntu:latest",
			Command:    []string{"/bin/sh"},
			Env:        map[string]string{"PATH": "/usr/bin"},
			WorkingDir: "/workspace",
		},
		SessionID: types.GenerateID(),
	}

	// Convert to proto and back
	pb := containerToProto(original)
	got := protoToContainer(pb)

	// Verify key fields
	if got.ID != original.ID {
		t.Errorf("ID mismatch: got %v, want %v", got.ID, original.ID)
	}
	if got.Name != original.Name {
		t.Errorf("Name mismatch: got %v, want %v", got.Name, original.Name)
	}
	if got.Image != original.Image {
		t.Errorf("Image mismatch: got %v, want %v", got.Image, original.Image)
	}
	if got.State != original.State {
		t.Errorf("State mismatch: got %v, want %v", got.State, original.State)
	}
	if got.Status != original.Status {
		t.Errorf("Status mismatch: got %v, want %v", got.Status, original.Status)
	}
	if got.Health != original.Health {
		t.Errorf("Health mismatch: got %v, want %v", got.Health, original.Health)
	}
	if got.SessionID != original.SessionID {
		t.Errorf("SessionID mismatch: got %v, want %v", got.SessionID, original.SessionID)
	}

	// Verify optional fields
	if got.StartedAt == nil {
		t.Error("StartedAt is nil, want non-nil")
	}
	if got.ExitCode == nil {
		t.Error("ExitCode is nil, want non-nil")
	} else if *got.ExitCode != exitCode {
		t.Errorf("ExitCode mismatch: got %v, want %v", *got.ExitCode, exitCode)
	}
}

// TestConvert_ContainerConfig tests container config conversion
func TestConvert_ContainerConfig(t *testing.T) {
	timeout := 30 * time.Second

	original := types.ContainerConfig{
		Image:          "alpine:latest",
		Command:        []string{"/bin/sh", "-c"},
		Args:           []string{"echo hello"},
		Env:            map[string]string{"FOO": "bar", "BAZ": "qux"},
		WorkingDir:     "/app",
		Labels:         map[string]string{"env": "test"},
		ReadOnlyRootfs: true,
		RemoveOnStop:   false,
		NetworkMode:    "bridge",
		Networks:       []string{"bridge", "host"},
		RestartPolicy: types.RestartPolicy{
			Name:              "always",
			MaximumRetryCount: 3,
			Timeout:           &timeout,
		},
	}

	// Add mounts and ports
	original.Mounts = []types.Mount{
		{Type: types.MountTypeBind, Source: "/host", Target: "/container", ReadOnly: false},
		{Type: types.MountTypeVolume, Source: "data", Target: "/data", ReadOnly: true},
	}
	original.Ports = []types.PortBinding{
		{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
	}

	// Convert to proto and back
	pb := containerConfigToProto(original)
	got := protoToContainerConfig(pb)

	// Verify all fields
	if got.Image != original.Image {
		t.Errorf("Image mismatch: got %v, want %v", got.Image, original.Image)
	}
	if len(got.Command) != len(original.Command) {
		t.Errorf("Command length mismatch: got %d, want %d", len(got.Command), len(original.Command))
	}
	if len(got.Env) != len(original.Env) {
		t.Errorf("Env length mismatch: got %d, want %d", len(got.Env), len(original.Env))
	}
	if got.Env["FOO"] != original.Env["FOO"] {
		t.Errorf("Env[FOO] mismatch: got %v, want %v", got.Env["FOO"], original.Env["FOO"])
	}
	if got.WorkingDir != original.WorkingDir {
		t.Errorf("WorkingDir mismatch: got %v, want %v", got.WorkingDir, original.WorkingDir)
	}
	if got.ReadOnlyRootfs != original.ReadOnlyRootfs {
		t.Errorf("ReadOnlyRootfs mismatch: got %v, want %v", got.ReadOnlyRootfs, original.ReadOnlyRootfs)
	}
	if got.RemoveOnStop != original.RemoveOnStop {
		t.Errorf("RemoveOnStop mismatch: got %v, want %v", got.RemoveOnStop, original.RemoveOnStop)
	}

	// Verify restart policy
	if got.RestartPolicy.Name != original.RestartPolicy.Name {
		t.Errorf("RestartPolicy.Name mismatch: got %v, want %v", got.RestartPolicy.Name, original.RestartPolicy.Name)
	}
	if got.RestartPolicy.MaximumRetryCount != original.RestartPolicy.MaximumRetryCount {
		t.Errorf("RestartPolicy.MaximumRetryCount mismatch: got %v, want %v", got.RestartPolicy.MaximumRetryCount, original.RestartPolicy.MaximumRetryCount)
	}
	if got.RestartPolicy.Timeout == nil {
		t.Error("RestartPolicy.Timeout is nil, want non-nil")
	} else if *got.RestartPolicy.Timeout != timeout {
		t.Errorf("RestartPolicy.Timeout mismatch: got %v, want %v", *got.RestartPolicy.Timeout, timeout)
	}
}

// TestConvert_Mount tests mount conversion
func TestConvert_Mount(t *testing.T) {
	tests := []struct {
		name     string
		original types.Mount
	}{
		{
			name: "bind mount",
			original: types.Mount{
				Type:     types.MountTypeBind,
				Source:   "/host/path",
				Target:   "/container/path",
				ReadOnly: false,
			},
		},
		{
			name: "volume mount",
			original: types.Mount{
				Type:     types.MountTypeVolume,
				Source:   "my-volume",
				Target:   "/data",
				ReadOnly: true,
			},
		},
		{
			name: "tmpfs mount",
			original: types.Mount{
				Type:     types.MountTypeTmpfs,
				Source:   "tmpfs",
				Target:   "/tmp",
				ReadOnly: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := mountToProto(tt.original)
			got := protoToMount(pb)

			if got.Type != tt.original.Type {
				t.Errorf("Type mismatch: got %v, want %v", got.Type, tt.original.Type)
			}
			if got.Source != tt.original.Source {
				t.Errorf("Source mismatch: got %v, want %v", got.Source, tt.original.Source)
			}
			if got.Target != tt.original.Target {
				t.Errorf("Target mismatch: got %v, want %v", got.Target, tt.original.Target)
			}
			if got.ReadOnly != tt.original.ReadOnly {
				t.Errorf("ReadOnly mismatch: got %v, want %v", got.ReadOnly, tt.original.ReadOnly)
			}
		})
	}
}

// TestConvert_PortBinding tests port binding conversion
func TestConvert_PortBinding(t *testing.T) {
	original := types.PortBinding{
		ContainerPort: 8080,
		HostPort:      8080,
		Protocol:      "tcp",
		HostIP:        "0.0.0.0",
	}

	pb := portBindingToProto(original)
	got := protoToPortBinding(pb)

	if got.ContainerPort != original.ContainerPort {
		t.Errorf("ContainerPort mismatch: got %v, want %v", got.ContainerPort, original.ContainerPort)
	}
	if got.HostPort != original.HostPort {
		t.Errorf("HostPort mismatch: got %v, want %v", got.HostPort, original.HostPort)
	}
	if got.Protocol != original.Protocol {
		t.Errorf("Protocol mismatch: got %v, want %v", got.Protocol, original.Protocol)
	}
	if got.HostIP != original.HostIP {
		t.Errorf("HostIP mismatch: got %v, want %v", got.HostIP, original.HostIP)
	}
}

// TestConvert_RestartPolicy tests restart policy conversion
func TestConvert_RestartPolicy(t *testing.T) {
	timeout := 10 * time.Second

	original := types.RestartPolicy{
		Name:              "on-failure",
		MaximumRetryCount: 5,
		Timeout:           &timeout,
	}

	pb := restartPolicyToProto(original)
	got := protoToRestartPolicy(pb)

	if got.Name != original.Name {
		t.Errorf("Name mismatch: got %v, want %v", got.Name, original.Name)
	}
	if got.MaximumRetryCount != original.MaximumRetryCount {
		t.Errorf("MaximumRetryCount mismatch: got %v, want %v", got.MaximumRetryCount, original.MaximumRetryCount)
	}
	if got.Timeout == nil {
		t.Fatal("Timeout is nil, want non-nil")
	}
	if *got.Timeout != timeout {
		t.Errorf("Timeout mismatch: got %v, want %v", *got.Timeout, timeout)
	}
}

// TestConvert_RestartPolicy_NoTimeout tests restart policy without timeout
func TestConvert_RestartPolicy_NoTimeout(t *testing.T) {
	original := types.RestartPolicy{
		Name:              "always",
		MaximumRetryCount: 0,
		Timeout:           nil,
	}

	pb := restartPolicyToProto(original)
	got := protoToRestartPolicy(pb)

	if got.Timeout != nil {
		t.Errorf("Timeout should be nil, got %v", got.Timeout)
	}
}

// TestConvert_ResourceLimits tests resource limits conversion
func TestConvert_ResourceLimits(t *testing.T) {
	pidsLimit := int64(100)

	original := types.ResourceLimits{
		NanoCPUs:    1000000000, // 1 CPU
		MemoryBytes: 1024 * 1024 * 1024, // 1GB
		MemorySwap:  2 * 1024 * 1024 * 1024, // 2GB
		PidsLimit:   &pidsLimit,
	}

	pb := resourceLimitsToProto(original)
	got := protoToResourceLimits(pb)

	if got.NanoCPUs != original.NanoCPUs {
		t.Errorf("NanoCPUs mismatch: got %v, want %v", got.NanoCPUs, original.NanoCPUs)
	}
	if got.MemoryBytes != original.MemoryBytes {
		t.Errorf("MemoryBytes mismatch: got %v, want %v", got.MemoryBytes, original.MemoryBytes)
	}
	if got.MemorySwap != original.MemorySwap {
		t.Errorf("MemorySwap mismatch: got %v, want %v", got.MemorySwap, original.MemorySwap)
	}
	if got.PidsLimit == nil {
		t.Fatal("PidsLimit is nil, want non-nil")
	}
	if *got.PidsLimit != pidsLimit {
		t.Errorf("PidsLimit mismatch: got %v, want %v", *got.PidsLimit, pidsLimit)
	}
}

// TestConvert_ResourceUsage tests resource usage conversion
func TestConvert_ResourceUsage(t *testing.T) {
	original := types.ResourceUsage{
		CPUPercent:    50.5,
		MemoryUsage:   512 * 1024 * 1024,
		MemoryLimit:   1024 * 1024 * 1024,
		MemoryPercent: 50.0,
		NetworkRx:     1024 * 1024,
		NetworkTx:     2048 * 1024,
		BlockRead:     4096,
		BlockWrite:    8192,
		PidsCount:     10,
	}

	pb := resourceUsageToProto(original)
	got := protoToResourceUsage(pb)

	if got.CPUPercent != original.CPUPercent {
		t.Errorf("CPUPercent mismatch: got %v, want %v", got.CPUPercent, original.CPUPercent)
	}
	if got.MemoryUsage != original.MemoryUsage {
		t.Errorf("MemoryUsage mismatch: got %v, want %v", got.MemoryUsage, original.MemoryUsage)
	}
	if got.MemoryLimit != original.MemoryLimit {
		t.Errorf("MemoryLimit mismatch: got %v, want %v", got.MemoryLimit, original.MemoryLimit)
	}
	if got.MemoryPercent != original.MemoryPercent {
		t.Errorf("MemoryPercent mismatch: got %v, want %v", got.MemoryPercent, original.MemoryPercent)
	}
	if got.NetworkRx != original.NetworkRx {
		t.Errorf("NetworkRx mismatch: got %v, want %v", got.NetworkRx, original.NetworkRx)
	}
	if got.NetworkTx != original.NetworkTx {
		t.Errorf("NetworkTx mismatch: got %v, want %v", got.NetworkTx, original.NetworkTx)
	}
	if got.BlockRead != original.BlockRead {
		t.Errorf("BlockRead mismatch: got %v, want %v", got.BlockRead, original.BlockRead)
	}
	if got.BlockWrite != original.BlockWrite {
		t.Errorf("BlockWrite mismatch: got %v, want %v", got.BlockWrite, original.BlockWrite)
	}
	if got.PidsCount != original.PidsCount {
		t.Errorf("PidsCount mismatch: got %v, want %v", got.PidsCount, original.PidsCount)
	}
}

// TestConvert_Session tests full session conversion
func TestConvert_Session(t *testing.T) {
	now := time.Now()
	maxDuration := 24 * time.Hour
	idleTimeout := 30 * time.Minute

	original := types.Session{
		ID:        types.GenerateID(),
		State:     types.SessionStateActive,
		Status:    types.StatusRunning,
		CreatedAt: types.NewTimestampFromTime(now.Add(-1 * time.Hour)),
		UpdatedAt: types.NewTimestampFromTime(now),
		Metadata: types.SessionMetadata{
			Name:        "test-session",
			Description: "A test session",
			OwnerID:     "test-user",
			Labels:      map[string]string{"env": "test"},
			Tags:        []string{"test", "example"},
		},
		Context: types.SessionContext{
			Preferences: types.UserPreferences{
				Timezone: "UTC",
				Language: "en",
			},
			Config: types.SessionConfig{
				MaxContainers: 5,
				MaxDuration:   maxDuration,
				IdleTimeout:   idleTimeout,
			},
		},
		Containers: []types.ID{types.GenerateID(), types.GenerateID()},
	}

	// Convert to proto
	pb := sessionToProto(&original)

	// Verify conversion succeeded
	if pb == nil {
		t.Fatal("sessionToProto() returned nil")
	}

	if pb.Id != string(original.ID) {
		t.Errorf("Id mismatch: got %v, want %v", pb.Id, string(original.ID))
	}
	if pb.Metadata.Name != original.Metadata.Name {
		t.Errorf("Metadata.Name mismatch: got %v, want %v", pb.Metadata.Name, original.Metadata.Name)
	}
	if len(pb.ContainerIds) != len(original.Containers) {
		t.Errorf("ContainerIds length mismatch: got %d, want %d", len(pb.ContainerIds), len(original.Containers))
	}
}

// TestConvert_Session_NoDataLoss verifies no data loss in round-trip conversion
func TestConvert_Session_NoDataLoss(t *testing.T) {
	// Create a comprehensive session with all fields populated
	now := time.Now()
	maxDuration := 24 * time.Hour
	idleTimeout := 30 * time.Minute
	pidsLimit := int64(100)

	original := types.Session{
		ID:        types.GenerateID(),
		State:     types.SessionStateActive,
		Status:    types.StatusRunning,
		CreatedAt: types.NewTimestampFromTime(now.Add(-1 * time.Hour)),
		UpdatedAt: types.NewTimestampFromTime(now),
		ExpiresAt: (*types.Timestamp)(&[]types.Timestamp{types.NewTimestampFromTime(now.Add(23 * time.Hour))}[0]),
		Metadata: types.SessionMetadata{
			Name:        "test-session",
			Description: "A comprehensive test session",
			OwnerID:     "test-user",
			Labels:      map[string]string{"env": "test", "team": "platform"},
			Tags:        []string{"test", "example", "comprehensive"},
		},
		Context: types.SessionContext{
			Messages: []types.Message{
				{
					ID:        types.GenerateID(),
					Timestamp: types.NewTimestampFromTime(now.Add(-30 * time.Minute)),
					Role:      types.MessageRoleUser,
					Content:   "Hello, test!",
					Metadata: types.MessageMetadata{
						ToolName: "test-tool",
						Extra:    map[string]string{"key": "value"},
					},
				},
			},
			Preferences: types.UserPreferences{
				Timezone:      "UTC",
				Language:      "en",
				Theme:         "dark",
				Notifications: true,
			},
			Config: types.SessionConfig{
				MaxContainers: 5,
				MaxDuration:   maxDuration,
				IdleTimeout:   idleTimeout,
				ResourceLimits: types.ResourceLimits{
					NanoCPUs:    2000000000,
					MemoryBytes: 2 * 1024 * 1024 * 1024,
					MemorySwap:  4 * 1024 * 1024 * 1024,
					PidsLimit:   &pidsLimit,
				},
				AllowedImages: []string{"ubuntu:latest", "alpine:latest"},
				Policy: types.SessionPolicy{
					AllowNetwork:     true,
					AllowedNetworks:  []string{"bridge", "host"},
					AllowVolumeMount: true,
					AllowedPaths:     []string{"/tmp", "/data"},
					RequireApproval:  false,
				},
			},
		},
		Containers: []types.ID{types.GenerateID(), types.GenerateID(), types.GenerateID()},
	}

	// Convert to proto and verify all fields are preserved
	pb := sessionToProto(&original)

	// Verify all fields
	if pb.Id != string(original.ID) {
		t.Errorf("ID data loss: got %v, want %v", pb.Id, string(original.ID))
	}
	if pb.Metadata.Name != original.Metadata.Name {
		t.Errorf("Metadata.Name data loss: got %v, want %v", pb.Metadata.Name, original.Metadata.Name)
	}
	if pb.Metadata.Description != original.Metadata.Description {
		t.Errorf("Metadata.Description data loss: got %v, want %v", pb.Metadata.Description, original.Metadata.Description)
	}
	if pb.Metadata.OwnerId != original.Metadata.OwnerID {
		t.Errorf("Metadata.OwnerID data loss: got %v, want %v", pb.Metadata.OwnerId, original.Metadata.OwnerID)
	}
	if len(pb.Metadata.Labels) != len(original.Metadata.Labels) {
		t.Errorf("Metadata.Labels data loss: got %d, want %d", len(pb.Metadata.Labels), len(original.Metadata.Labels))
	}
	if len(pb.Metadata.Tags) != len(original.Metadata.Tags) {
		t.Errorf("Metadata.Tags data loss: got %d, want %d", len(pb.Metadata.Tags), len(original.Metadata.Tags))
	}
	if len(pb.ContainerIds) != len(original.Containers) {
		t.Errorf("ContainerIds data loss: got %d, want %d", len(pb.ContainerIds), len(original.Containers))
	}
	if len(pb.Context.Messages) != len(original.Context.Messages) {
		t.Errorf("Context.Messages data loss: got %d, want %d", len(pb.Context.Messages), len(original.Context.Messages))
	}
	if pb.Context.Preferences.Timezone != original.Context.Preferences.Timezone {
		t.Errorf("Preferences.Timezone data loss: got %v, want %v", pb.Context.Preferences.Timezone, original.Context.Preferences.Timezone)
	}
	if pb.Context.Config.MaxContainers != int32(original.Context.Config.MaxContainers) {
		t.Errorf("Config.MaxContainers data loss: got %v, want %v", pb.Context.Config.MaxContainers, original.Context.Config.MaxContainers)
	}
	if pb.Context.Config.MaxDurationNs != original.Context.Config.MaxDuration.Nanoseconds() {
		t.Errorf("Config.MaxDuration data loss: got %v, want %v", pb.Context.Config.MaxDurationNs, original.Context.Config.MaxDuration.Nanoseconds())
	}
	if pb.Context.Config.IdleTimeoutNs != original.Context.Config.IdleTimeout.Nanoseconds() {
		t.Errorf("Config.IdleTimeout data loss: got %v, want %v", pb.Context.Config.IdleTimeoutNs, original.Context.Config.IdleTimeout.Nanoseconds())
	}
	if len(pb.Context.Config.AllowedImages) != len(original.Context.Config.AllowedImages) {
		t.Errorf("Config.AllowedImages data loss: got %d, want %d", len(pb.Context.Config.AllowedImages), len(original.Context.Config.AllowedImages))
	}
	if pb.Context.Config.Policy.AllowNetwork != original.Context.Config.Policy.AllowNetwork {
		t.Errorf("Policy.AllowNetwork data loss: got %v, want %v", pb.Context.Config.Policy.AllowNetwork, original.Context.Config.Policy.AllowNetwork)
	}
}

// TestConvert_NilValues tests that nil values are handled correctly
func TestConvert_NilValues(t *testing.T) {
	// Test nil container
	var nilContainer *proto.Container
	got := protoToContainer(nilContainer)
	if got.ID != "" {
		t.Errorf("protoToContainer(nil) should return empty container, got ID=%v", got.ID)
	}

	// Test nil container config
	gotConfig := protoToContainerConfig(nil)
	if gotConfig.Image != "" {
		t.Errorf("protoToContainerConfig(nil) should return empty config, got Image=%v", gotConfig.Image)
	}

	// Test nil mount
	gotMount := protoToMount(nil)
	if gotMount.Source != "" {
		t.Errorf("protoToMount(nil) should return empty mount, got Source=%v", gotMount.Source)
	}

	// Test nil port binding
	gotPort := protoToPortBinding(nil)
	if gotPort.Protocol != "" {
		t.Errorf("protoToPortBinding(nil) should return empty port binding, got Protocol=%v", gotPort.Protocol)
	}

	// Test nil restart policy
	gotPolicy := protoToRestartPolicy(nil)
	if gotPolicy.Name != "" {
		t.Errorf("protoToRestartPolicy(nil) should return empty policy, got Name=%v", gotPolicy.Name)
	}
}

// TestConvert_EmptyValues tests that empty slices and maps are handled correctly
func TestConvert_EmptyValues(t *testing.T) {
	config := types.ContainerConfig{
		Image: "test",
		// Empty slices and maps
		Command: []string{},
		Args:    []string{},
		Env:     map[string]string{},
		Labels:  map[string]string{},
		Mounts:  []types.Mount{},
		Ports:   []types.PortBinding{},
	}

	pb := containerConfigToProto(config)
	got := protoToContainerConfig(pb)

	if len(got.Command) != 0 {
		t.Errorf("Command should be empty, got %d elements", len(got.Command))
	}
	if len(got.Env) != 0 {
		t.Errorf("Env should be empty, got %d elements", len(got.Env))
	}
	if len(got.Mounts) != 0 {
		t.Errorf("Mounts should be empty, got %d elements", len(got.Mounts))
	}
}

// TestConvert_TimestampPrecision tests timestamp conversion precision
func TestConvert_TimestampPrecision(t *testing.T) {
	// Test with nanosecond precision
	now := time.Now()
	ts := types.NewTimestampFromTime(now)

	pb := timestampToProto(&ts)
	back := protoToTimestamp(pb)

	// Check that we're within 1 microsecond (protobuf timestamp precision)
	diff := back.Time.Sub(now).Abs()
	if diff > time.Microsecond {
		t.Errorf("Timestamp precision loss: diff = %v", diff)
	}
}
