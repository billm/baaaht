package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/policy"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/go-connections/nat"
)

// CreateConfig holds configuration for creating a container
type CreateConfig struct {
	// Container configuration
	Config      types.ContainerConfig
	Name        string
	SessionID   types.ID
	AutoPull    bool          // Automatically pull image if not present
	PullTimeout time.Duration // Timeout for image pull operations
}

// CreateResult contains the result of a container creation operation
type CreateResult struct {
	ContainerID string
	Warnings    []string
	ImagePulled bool
}

// Creator handles container creation operations
type Creator struct {
	client   *Client
	enforcer *policy.Enforcer
	logger   *logger.Logger
	mu       sync.RWMutex
}

// NewCreator creates a new container creator
func NewCreator(client *Client, log *logger.Logger) (*Creator, error) {
	if client == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "Docker client cannot be nil")
	}
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &Creator{
		client: client,
		logger: log.With("component", "container_creator"),
	}, nil
}

// NewCreatorFromRuntime creates a new container creator from a Runtime interface
// This allows the Creator to work with any runtime implementation
func NewCreatorFromRuntime(runtime Runtime, log *logger.Logger) (*Creator, error) {
	if runtime == nil {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime cannot be nil")
	}

	// Extract the underlying client from the runtime
	client, ok := runtime.Client().(*Client)
	if !ok {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "runtime does not provide a Docker client")
	}

	return NewCreator(client, log)
}

// SetEnforcer sets the policy enforcer for the creator
func (c *Creator) SetEnforcer(enforcer *policy.Enforcer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enforcer = enforcer
}

// Enforcer returns the current policy enforcer
func (c *Creator) Enforcer() *policy.Enforcer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enforcer
}

// Create creates a new container with the specified configuration
func (c *Creator) Create(ctx context.Context, cfg CreateConfig) (*CreateResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.validateConfig(cfg); err != nil {
		return nil, err
	}

	// Validate against policy if enforcer is set
	if c.enforcer != nil {
		result, err := c.enforcer.ValidateContainerConfig(ctx, cfg.SessionID, cfg.Config)
		if err != nil {
			c.logger.Warn("Policy validation failed", "error", err)
			return nil, types.WrapError(types.ErrCodeInternal, "policy validation error", err)
		}

		// Log any warnings (non-violation issues)
		for _, warning := range result.Warnings {
			c.logger.Warn("Policy validation warning",
				"rule", warning.Rule,
				"message", warning.Message,
				"component", warning.Component)
		}

		// Log violations (these may or may not be fatal depending on mode)
		for _, violation := range result.Violations {
			if violation.Severity == "error" {
				c.logger.Warn("Policy violation detected",
					"rule", violation.Rule,
					"message", violation.Message,
					"component", violation.Component)
			} else {
				c.logger.Info("Policy validation notice",
					"rule", violation.Rule,
					"message", violation.Message,
					"component", violation.Component)
			}
		}

		// If not allowed, return error
		if !result.Allowed {
			return nil, types.NewError(types.ErrCodePermission, "container configuration violates policy")
		}
	}

	c.logger.Info("Creating container",
		"name", cfg.Name,
		"image", cfg.Config.Image,
		"session_id", cfg.SessionID)

	// Check if image exists locally, pull if needed
	imagePulled := false
	if cfg.AutoPull {
		exists, err := c.imageExists(ctx, cfg.Config.Image)
		if err != nil {
			c.logger.Warn("Failed to check image existence", "error", err)
			// Continue to attempt creation
		} else if !exists {
			c.logger.Info("Image not found locally, pulling", "image", cfg.Config.Image)
			if err := c.pullImage(ctx, cfg.Config.Image, cfg.PullTimeout); err != nil {
				return nil, types.WrapError(types.ErrCodeUnavailable, "failed to pull image", err)
			}
			imagePulled = true
			c.logger.Info("Image pulled successfully", "image", cfg.Config.Image)
		}
	}

	// Convert our config to Docker config
	containerConfig, hostConfig, networkingConfig, err := c.convertConfig(cfg.Config, cfg.Name, cfg.SessionID)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInvalidArgument, "failed to convert container config", err)
	}

	// Check if Docker client is available
	if c.client.cli == nil {
		return nil, types.NewError(types.ErrCodeInternal, "Docker client is not initialized")
	}

	// Create the container
	timeoutCtx, cancel := c.client.WithTimeout(ctx)
	defer cancel()

	resp, err := c.client.cli.ContainerCreate(
		timeoutCtx,
		containerConfig,
		hostConfig,
		networkingConfig,
		nil,
		cfg.Name,
	)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create container", err)
	}

	c.logger.Info("Container created successfully",
		"container_id", resp.ID,
		"name", cfg.Name,
		"warnings", len(resp.Warnings))

	return &CreateResult{
		ContainerID: resp.ID,
		Warnings:    resp.Warnings,
		ImagePulled: imagePulled,
	}, nil
}

// CreateWithDefaults creates a container with sensible defaults
func (c *Creator) CreateWithDefaults(ctx context.Context, image string, name string, sessionID types.ID) (*CreateResult, error) {
	cfg := CreateConfig{
		Config: types.ContainerConfig{
			Image:     image,
			Env:       make(map[string]string),
			Labels:    make(map[string]string),
			Mounts:    []types.Mount{},
			Ports:     []types.PortBinding{},
			Resources: types.ResourceLimits{},
			RestartPolicy: types.RestartPolicy{
				Name: "unless-stopped",
			},
		},
		Name:        name,
		SessionID:   sessionID,
		AutoPull:    true,
		PullTimeout: 5 * time.Minute,
	}

	// Add session label
	cfg.Config.Labels["baaaht.session_id"] = sessionID.String()
	cfg.Config.Labels["baaaht.managed"] = "true"

	return c.Create(ctx, cfg)
}

// PullImage pulls an image from the registry
func (c *Creator) PullImage(ctx context.Context, image string, timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pullImage(ctx, image, timeout)
}

// pullImage is the internal implementation of image pulling
func (c *Creator) pullImage(ctx context.Context, imageRef string, timeout time.Duration) error {
	c.logger.Info("Pulling image", "image", imageRef)

	pullTimeout := timeout
	if pullTimeout == 0 {
		pullTimeout = 5 * time.Minute
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()

	reader, err := c.client.cli.ImagePull(timeoutCtx, imageRef, image.PullOptions{})
	if err != nil {
		return types.WrapError(types.ErrCodeUnavailable, "failed to pull image", err)
	}
	defer reader.Close()

	// Parse and log pull progress
	decoder := json.NewDecoder(reader)
	for {
		var jm struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  string `json:"error,omitempty"`
		}
		if err := decoder.Decode(&jm); err != nil {
			if err == io.EOF {
				break
			}
			return types.WrapError(types.ErrCodeInternal, "failed to decode pull progress", err)
		}

		if jm.Error != "" {
			return types.NewError(types.ErrCodeInternal, "image pull error: "+jm.Error)
		}

		// Log progress updates
		if jm.Status != "" && jm.ID != "" {
			c.logger.Debug("Image pull progress", "id", jm.ID, "status", jm.Status)
		}
	}

	c.logger.Info("Image pulled successfully", "image", imageRef)
	return nil
}

// ImageExists checks if an image exists locally
func (c *Creator) ImageExists(ctx context.Context, image string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.imageExists(ctx, image)
}

// imageExists is the internal implementation of image existence check
func (c *Creator) imageExists(ctx context.Context, imageRef string) (bool, error) {
	timeoutCtx, cancel := c.client.WithTimeout(ctx)
	defer cancel()

	// List images with a filter for the specific image
	filter := filters.NewArgs()
	filter.Add("reference", imageRef)

	images, err := c.client.cli.ImageList(timeoutCtx, image.ListOptions{
		Filters: filter,
	})
	if err != nil {
		return false, types.WrapError(types.ErrCodeInternal, "failed to list images", err)
	}

	return len(images) > 0, nil
}

// validateConfig validates the container creation configuration
func (c *Creator) validateConfig(cfg CreateConfig) error {
	if cfg.Config.Image == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "container image is required")
	}
	if cfg.Name == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "container name is required")
	}
	if cfg.SessionID.IsEmpty() {
		return types.NewError(types.ErrCodeInvalidArgument, "session ID is required")
	}
	return nil
}

// convertConfig converts our ContainerConfig to Docker API types
func (c *Creator) convertConfig(cfg types.ContainerConfig, name string, sessionID types.ID) (
	*container.Config,
	*container.HostConfig,
	*network.NetworkingConfig,
	error) {
	// Build container config
	containerConfig := &container.Config{
		Image:       cfg.Image,
		Cmd:         strslice.StrSlice(cfg.Command),
		ArgsEscaped: true,
		Env:         convertEnvMap(cfg.Env),
		WorkingDir:  cfg.WorkingDir,
		Labels:      convertLabels(cfg.Labels, name, sessionID),
		StopSignal:  "SIGTERM",
		Tty:         false,
		OpenStdin:   false,
	}

	// Convert Args to Entrypoint if needed (for OCI compatibility)
	if len(cfg.Args) > 0 {
		containerConfig.Entrypoint = strslice.StrSlice(cfg.Args)
	}

	// Build host config
	hostConfig := &container.HostConfig{
		Mounts:         convertMounts(cfg.Mounts),
		PortBindings:   convertPortBindings(cfg.Ports),
		NetworkMode:    container.NetworkMode(cfg.NetworkMode),
		AutoRemove:     cfg.RemoveOnStop,
		RestartPolicy:  convertRestartPolicy(cfg.RestartPolicy),
		ReadonlyRootfs: cfg.ReadOnlyRootfs,
	}

	// Configure resource limits
	if cfg.Resources.NanoCPUs > 0 || cfg.Resources.MemoryBytes > 0 {
		hostConfig.Resources = container.Resources{
			NanoCPUs:   cfg.Resources.NanoCPUs,
			Memory:     cfg.Resources.MemoryBytes,
			MemorySwap: cfg.Resources.MemorySwap,
			PidsLimit:  cfg.Resources.PidsLimit,
		}
	}

	// Build networking config
	networkingConfig := &network.NetworkingConfig{}

	if len(cfg.Networks) > 0 {
		networkingConfig.EndpointsConfig = make(map[string]*network.EndpointSettings)
		for _, net := range cfg.Networks {
			networkingConfig.EndpointsConfig[net] = &network.EndpointSettings{}
		}
	}

	return containerConfig, hostConfig, networkingConfig, nil
}

// convertEnvMap converts a map[string]string to a []string in KEY=VALUE format
func convertEnvMap(env map[string]string) []string {
	if env == nil {
		return nil
	}

	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

// convertLabels converts our labels and adds standard baaaht labels
func convertLabels(labels map[string]string, name string, sessionID types.ID) map[string]string {
	result := make(map[string]string)

	// Add standard labels
	result["baaaht.managed"] = "true"
	result["baaaht.container_name"] = name
	result["baaaht.session_id"] = sessionID.String()
	result["baaaht.created_at"] = time.Now().Format(time.RFC3339)

	// Add custom labels
	for k, v := range labels {
		result[k] = v
	}

	return result
}

// convertMounts converts our Mount type to Docker API mount type
func convertMounts(mounts []types.Mount) []mount.Mount {
	if mounts == nil {
		return nil
	}

	result := make([]mount.Mount, 0, len(mounts))
	for _, m := range mounts {
		result = append(result, mount.Mount{
			Type:     mount.Type(string(m.Type)),
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}
	return result
}

// convertPortBindings converts our PortBinding type to Docker API port bindings
func convertPortBindings(ports []types.PortBinding) nat.PortMap {
	if ports == nil {
		return nil
	}

	result := make(nat.PortMap)
	for _, p := range ports {
		// Build port string with protocol (default to tcp if not specified)
		protocol := p.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		port := nat.Port(fmt.Sprintf("%d/%s", p.ContainerPort, protocol))

		hostIP := p.HostIP
		if hostIP == "" {
			hostIP = "0.0.0.0"
		}

		binding := nat.PortBinding{
			HostIP:   hostIP,
			HostPort: fmt.Sprintf("%d", p.HostPort),
		}

		// Check if port already has bindings
		if existing, ok := result[port]; ok {
			result[port] = append(existing, binding)
		} else {
			result[port] = []nat.PortBinding{binding}
		}
	}
	return result
}

// convertRestartPolicy converts our RestartPolicy to Docker API type
func convertRestartPolicy(rp types.RestartPolicy) container.RestartPolicy {
	var maxRetries int
	if rp.MaximumRetryCount > 0 {
		maxRetries = int(rp.MaximumRetryCount)
	}

	// Docker restart policy names
	var name container.RestartPolicyMode
	switch strings.ToLower(rp.Name) {
	case "no", "":
		name = container.RestartPolicyDisabled
	case "always":
		name = container.RestartPolicyAlways
	case "unless-stopped":
		name = container.RestartPolicyUnlessStopped
	case "on-failure":
		name = container.RestartPolicyOnFailure
	default:
		name = container.RestartPolicyDisabled
	}

	return container.RestartPolicy{
		Name:              name,
		MaximumRetryCount: maxRetries,
	}
}

// String returns a string representation of the Creator
func (c *Creator) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	enforcerStatus := "none"
	if c.enforcer != nil {
		enforcerStatus = "enabled"
	}

	return fmt.Sprintf("Creator{Client: %v, Enforcer: %s}", c.client, enforcerStatus)
}
