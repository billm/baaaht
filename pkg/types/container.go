package types

import "time"

// ContainerState represents the lifecycle state of a container
type ContainerState string

const (
	ContainerStateCreated  ContainerState = "created"
	ContainerStateRunning  ContainerState = "running"
	ContainerStatePaused   ContainerState = "paused"
	ContainerStateRestarting ContainerState = "restarting"
	ContainerStateExited   ContainerState = "exited"
	ContainerStateRemoving ContainerState = "removing"
	ContainerStateUnknown  ContainerState = "unknown"
)

// Container represents a container instance managed by the orchestrator
type Container struct {
	ID          ID              `json:"id"`
	Name        string          `json:"name"`
	Image       string          `json:"image"`
	State       ContainerState  `json:"state"`
	Status      Status          `json:"status"`
	Health      Health          `json:"health"`
	CreatedAt   Timestamp       `json:"created_at"`
	StartedAt   *Timestamp      `json:"started_at,omitempty"`
	ExitedAt    *Timestamp      `json:"exited_at,omitempty"`
	ExitCode    *int            `json:"exit_code,omitempty"`
	Config      ContainerConfig `json:"config"`
	Resources   ResourceUsage   `json:"resources"`
	SessionID   ID              `json:"session_id"`
}

// ContainerConfig represents the configuration used to create a container
type ContainerConfig struct {
	Image           string            `json:"image"`
	Command         []string          `json:"command,omitempty"`
	Args            []string          `json:"args,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	WorkingDir      string            `json:"working_dir,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Mounts          []Mount           `json:"mounts,omitempty"`
	Ports           []PortBinding     `json:"ports,omitempty"`
	NetworkMode     string            `json:"network_mode,omitempty"`
	Networks        []string          `json:"networks,omitempty"`
	Resources       ResourceLimits    `json:"resources,omitempty"`
	RestartPolicy   RestartPolicy     `json:"restart_policy,omitempty"`
	ReadOnlyRootfs  bool              `json:"read_only_rootfs,omitempty"`
	RemoveOnStop    bool              `json:"remove_on_stop,omitempty"`
}

// Mount represents a filesystem mount
type Mount struct {
	Type     MountType `json:"type"`
	Source   string    `json:"source"`
	Target   string    `json:"target"`
	ReadOnly bool      `json:"read_only,omitempty"`
}

// MountType represents the type of mount
type MountType string

const (
	MountTypeBind   MountType = "bind"
	MountTypeVolume MountType = "volume"
	MountTypeTmpfs  MountType = "tmpfs"
)

// PortBinding represents a port mapping
type PortBinding struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	HostIP        string `json:"host_ip,omitempty"`
}

// ResourceLimits defines resource constraints for a container
type ResourceLimits struct {
	NanoCPUs    int64 `json:"nano_cpus,omitempty"`    // CPU in 1e-9 units
	MemoryBytes int64 `json:"memory_bytes,omitempty"` // Memory in bytes
	MemorySwap  int64 `json:"memory_swap,omitempty"`  // Memory swap in bytes
	// PidsLimit is the maximum number of processes allowed in the container
	PidsLimit *int64 `json:"pids_limit,omitempty"`
}

// ResourceUsage represents current resource usage of a container
type ResourceUsage struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   int64   `json:"memory_usage"`
	MemoryLimit   int64   `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	NetworkRx     int64   `json:"network_rx"`
	NetworkTx     int64   `json:"network_tx"`
	BlockRead     int64   `json:"block_read"`
	BlockWrite    int64   `json:"block_write"`
	PidsCount     int64   `json:"pids_count"`
}

// RestartPolicy defines the restart policy for a container
type RestartPolicy struct {
	Name              string        `json:"name"`
	MaximumRetryCount int           `json:"maximum_retry_count,omitempty"`
	Timeout           *time.Duration `json:"timeout,omitempty"`
}

// ContainerLog represents a log entry from a container
type ContainerLog struct {
	ContainerID ID        `json:"container_id"`
	Timestamp   Timestamp `json:"timestamp"`
	Stream      string    `json:"stream"` // stdout or stderr
	Message     string    `json:"message"`
}

// ContainerStats represents statistics for a container
type ContainerStats struct {
	ContainerID ID           `json:"container_id"`
	Timestamp   Timestamp    `json:"timestamp"`
	Resources   ResourceUsage `json:"resources"`
}

// ContainerFilter defines filters for querying containers
type ContainerFilter struct {
	SessionID  *ID              `json:"session_id,omitempty"`
	State      *ContainerState  `json:"state,omitempty"`
	Status     *Status          `json:"status,omitempty"`
	Label      map[string]string `json:"label,omitempty"`
}
