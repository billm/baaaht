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
	"github.com/billm/baaaht/orchestrator/pkg/types"

	dockertypes "github.com/docker/docker/api/types"
)

// Monitor handles container monitoring operations (health checks, stats, logs)
type Monitor struct {
	client *Client
	logger *logger.Logger
	mu     sync.RWMutex
}

// NewMonitor creates a new container monitor
func NewMonitor(client *Client, log *logger.Logger) (*Monitor, error) {
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

	return &Monitor{
		client: client,
		logger: log.With("component", "container_monitor"),
	}, nil
}

// HealthCheckResult represents the result of a container health check
type HealthCheckResult struct {
	ContainerID string
	Status      types.Health
	FailingStreak int
	LastOutput   string
	CheckedAt    time.Time
}

// HealthCheck performs a health check on a container
func (m *Monitor) HealthCheck(ctx context.Context, containerID string) (*HealthCheckResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.validateContainerID(containerID); err != nil {
		return nil, err
	}

	m.logger.Debug("Performing health check", "container_id", containerID)

	timeoutCtx, cancel := m.client.WithTimeout(ctx)
	defer cancel()

	containerJSON, err := m.client.cli.ContainerInspect(timeoutCtx, containerID)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to inspect container for health check", err)
	}

	result := &HealthCheckResult{
		ContainerID: containerID,
		CheckedAt:   time.Now(),
	}

	// Parse Docker health status
	if containerJSON.State.Health == nil {
		// Container has no healthcheck configured, infer from state
		if containerJSON.State.Running {
			result.Status = types.Healthy
		} else if containerJSON.State.OOMKilled {
			result.Status = types.Unhealthy
		} else if containerJSON.State.ExitCode != 0 {
			result.Status = types.Unhealthy
		} else {
			result.Status = types.HealthUnknown
		}
	} else {
		// Parse healthcheck status
		switch containerJSON.State.Health.Status {
		case "healthy":
			result.Status = types.Healthy
		case "unhealthy":
			result.Status = types.Unhealthy
		case "starting":
			result.Status = types.HealthChecking
		default:
			result.Status = types.HealthUnknown
		}

		result.FailingStreak = containerJSON.State.Health.FailingStreak
		if len(containerJSON.State.Health.Log) > 0 {
			lastLog := containerJSON.State.Health.Log[len(containerJSON.State.Health.Log)-1]
			result.LastOutput = lastLog.Output
		}
	}

	m.logger.Debug("Health check completed",
		"container_id", containerID,
		"status", result.Status,
		"failing_streak", result.FailingStreak)

	return result, nil
}

// HealthCheckWithRetry performs a health check with retries
func (m *Monitor) HealthCheckWithRetry(ctx context.Context, containerID string, maxAttempts int, interval time.Duration) (*HealthCheckResult, error) {
	m.logger.Info("Starting health check with retry",
		"container_id", containerID,
		"max_attempts", maxAttempts,
		"interval", interval)

	var lastResult *HealthCheckResult
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			m.logger.Debug("Retrying health check",
				"container_id", containerID,
				"attempt", attempt+1,
				"max_attempts", maxAttempts)

			select {
			case <-ctx.Done():
				return nil, types.WrapError(types.ErrCodeTimeout, "health check canceled during retry delay", ctx.Err())
			case <-time.After(interval):
			}
		}

		result, err := m.HealthCheck(ctx, containerID)
		if err != nil {
			lastErr = err
			continue
		}

		lastResult = result

		// If healthy, return immediately
		if result.Status == types.Healthy {
			m.logger.Info("Container is healthy",
				"container_id", containerID,
				"attempt", attempt+1)
			return result, nil
		}

		// If unhealthy and not checking, return immediately
		if result.Status == types.Unhealthy {
			m.logger.Warn("Container is unhealthy",
				"container_id", containerID,
				"failing_streak", result.FailingStreak)
			return result, nil
		}
	}

	// Return the last result or error
	if lastResult != nil {
		return lastResult, nil
	}
	return nil, types.WrapError(types.ErrCodeTimeout, "health check failed after all attempts", lastErr)
}

// Stats retrieves resource usage statistics for a container
func (m *Monitor) Stats(ctx context.Context, containerID string) (*types.ContainerStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.validateContainerID(containerID); err != nil {
		return nil, err
	}

	m.logger.Debug("Retrieving container stats", "container_id", containerID)

	timeoutCtx, cancel := m.client.WithTimeout(ctx)
	defer cancel()

	stats, err := m.client.cli.ContainerStats(timeoutCtx, containerID, false)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to retrieve container stats", err)
	}
	defer stats.Body.Close()

	var dockerStats dockertypes.StatsJSON
	if err := json.NewDecoder(stats.Body).Decode(&dockerStats); err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to decode container stats", err)
	}

	// Parse stats
	resourceUsage := m.parseStats(&dockerStats)

	containerStats := &types.ContainerStats{
		ContainerID: types.ID(containerID),
		Timestamp:   types.NewTimestampFromTime(time.Now()),
		Resources:   resourceUsage,
	}

	m.logger.Debug("Stats retrieved",
		"container_id", containerID,
		"cpu_percent", resourceUsage.CPUPercent,
		"memory_percent", resourceUsage.MemoryPercent)

	return containerStats, nil
}

// StatsStream returns a channel that receives stats updates
func (m *Monitor) StatsStream(ctx context.Context, containerID string, interval time.Duration) (<-chan *types.ContainerStats, <-chan error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statsCh := make(chan *types.ContainerStats, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(statsCh)
		defer close(errCh)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case <-ticker.C:
				stats, err := m.Stats(ctx, containerID)
				if err != nil {
					errCh <- err
					return
				}

				select {
				case statsCh <- stats:
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				}
			}
		}
	}()

	return statsCh, errCh
}

// LogsConfig holds configuration for retrieving container logs
type LogsConfig struct {
	ContainerID string
	Tail        string // Number of lines to show from the end of the logs
	Since       string // Only return logs since this time (RFC3339, timestamp, or duration)
	Until       string // Only return logs until this time (RFC3339, timestamp, or duration)
	Follow      bool   // Follow log output
	Timestamps  bool   // Add timestamps to every log line
	Stdout      bool   // Return logs from stdout
	Stderr      bool   // Return logs from stderr
}

// Logs retrieves logs from a container
func (m *Monitor) Logs(ctx context.Context, cfg LogsConfig) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.validateContainerID(cfg.ContainerID); err != nil {
		return nil, err
	}

	if !cfg.Stdout && !cfg.Stderr {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "at least one of Stdout or Stderr must be enabled")
	}

	m.logger.Debug("Retrieving container logs",
		"container_id", cfg.ContainerID,
		"follow", cfg.Follow,
		"tail", cfg.Tail)

	timeoutCtx, cancel := m.client.WithTimeout(ctx)

	options := types.ContainerLogsOptions{
		ShowStdout: cfg.Stdout,
		ShowStderr: cfg.Stderr,
		Follow:     cfg.Follow,
		Tail:       cfg.Tail,
		Since:      cfg.Since,
		Until:      cfg.Until,
		Timestamps: cfg.Timestamps,
	}

	reader, err := m.client.cli.ContainerLogs(timeoutCtx, cfg.ContainerID, options)
	cancel()

	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to retrieve container logs", err)
	}

	return reader, nil
}

// LogsReader wraps a log reader and parses log lines
type LogsReader struct {
	reader io.ReadCloser
}

// NewLogsReader creates a new logs reader
func NewLogsReader(reader io.ReadCloser) *LogsReader {
	return &LogsReader{reader: reader}
}

// Read reads a log entry
func (r *LogsReader) Read() (*types.ContainerLog, error) {
	if r.reader == nil {
		return nil, io.EOF
	}

	// Docker log format is:
	// [8-byte header][payload]
	// Header: [1 byte stream type][3 bytes padding][4 bytes payload size]
	// Stream type: 1 = stdout, 2 = stderr

	header := make([]byte, 8)
	_, err := io.ReadFull(r.reader, header)
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read log header: %w", err)
	}

	// Parse header
	streamType := header[0]
	payloadSize := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])

	// Read payload
	payload := make([]byte, payloadSize)
	_, err = io.ReadFull(r.reader, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to read log payload: %w", err)
	}

	// Determine stream
	var stream string
	switch streamType {
	case 1:
		stream = "stdout"
	case 2:
		stream = "stderr"
	default:
		stream = "unknown"
	}

	return &types.ContainerLog{
		Timestamp: types.NewTimestampFromTime(time.Now()),
		Stream:    stream,
		Message:   string(payload),
	}, nil
}

// Close closes the underlying reader
func (r *LogsReader) Close() error {
	if r.reader != nil {
		return r.reader.Close()
	}
	return nil
}

// LogsLines reads logs and returns them as lines
func (m *Monitor) LogsLines(ctx context.Context, cfg LogsConfig) ([]types.ContainerLog, error) {
	reader, err := m.Logs(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	logReader := NewLogsReader(reader)
	var logs []types.ContainerLog

	for {
		log, err := logReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, types.WrapError(types.ErrCodeInternal, "failed to read log entry", err)
		}

		log.ContainerID = types.ID(cfg.ContainerID)
		logs = append(logs, *log)
	}

	return logs, nil
}

// EventsStream returns a stream of container events
func (m *Monitor) EventsStream(ctx context.Context, containerID string) (<-chan dockertypes.ContainerEvent, <-chan error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	eventCh := make(chan dockertypes.ContainerEvent, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		timeoutCtx, cancel := m.client.WithTimeout(ctx)
		defer cancel()

		// Build filters for the specific container
		filters := map[string][]string{
			"container": {containerID},
			"event": {
				"create",
				"start",
				"stop",
				"die",
				"destroy",
				"pause",
				"unpause",
				"restart",
				"oom",
				"health_status",
			},
		}

		eventsReader, err := m.client.cli.Events(timeoutCtx, types.EventsOptions{
			Filters: filters,
		})
		if err != nil {
			errCh <- types.WrapError(types.ErrCodeInternal, "failed to stream events", err)
			return
		}
		defer eventsReader.Close()

		decoder := json.NewDecoder(eventsReader)

		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
				var event types.EventsMessage
				if err := decoder.Decode(&event); err != nil {
					if err == io.EOF {
						return
					}
					errCh <- types.WrapError(types.ErrCodeInternal, "failed to decode event", err)
					return
				}

				containerEvent := dockertypes.ContainerEvent{
					ContainerID: types.ID(containerID),
					Timestamp:   types.NewTimestampFromTime(time.Unix(event.Time, 0)),
					Action:      event.Action,
					Actor: event.Actor.ID,
				}

				select {
				case eventCh <- containerEvent:
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				}
			}
		}
	}()

	return eventCh, errCh
}

// parseStats parses Docker stats into our ResourceUsage format
func (m *Monitor) parseStats(stats *dockertypes.StatsJSON) types.ResourceUsage {
	usage := types.ResourceUsage{}

	// Calculate CPU percentage
	if stats.PreCPUStats.CPUUsage.TotalUsage > 0 && stats.CPUStats.CPUUsage.TotalUsage > 0 {
		cpuDelta := stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage
		systemDelta := stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage

		if systemDelta > 0 {
			usage.CPUPercent = (float64(cpuDelta) / float64(systemDelta)) * float64(len(stats.CPUStats.CPUUsage.PercpuUsage)) * 100.0
		}
	}

	// Parse memory usage
	if stats.MemoryStats.Usage > 0 {
		usage.MemoryUsage = int64(stats.MemoryStats.Usage)
	}

	// Get memory limit
	if stats.MemoryStats.Limit > 0 {
		usage.MemoryLimit = int64(stats.MemoryStats.Limit)
		usage.MemoryPercent = (float64(usage.MemoryUsage) / float64(usage.MemoryLimit)) * 100.0
	}

	// Parse network stats
	if stats.Networks != nil {
		for _, netStats := range stats.Networks {
			usage.NetworkRx += int64(netStats.RxBytes)
			usage.NetworkTx += int64(netStats.TxBytes)
		}
	}

	// Parse block I/O stats
	for _, blkStat := range stats.BlkioStats.IoServiceBytesRecursive {
		if strings.ToLower(blkStat.Op) == "read" || strings.ToLower(blkStat.Op) == "total" {
			usage.BlockRead += int64(blkStat.Value)
		}
		if strings.ToLower(blkStat.Op) == "write" || strings.ToLower(blkStat.Op) == "total" {
			usage.BlockWrite += int64(blkStat.Value)
		}
	}

	// Get PIDs count
	if stats.PidsStats.Current > 0 {
		usage.PidsCount = int64(stats.PidsStats.Current)
	}

	return usage
}

// validateContainerID validates that a container ID is not empty
func (m *Monitor) validateContainerID(containerID string) error {
	if containerID == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "container ID cannot be empty")
	}
	return nil
}

// String returns a string representation of the Monitor
func (m *Monitor) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return fmt.Sprintf("Monitor{Client: %v}", m.client)
}

// ContainerEvent represents a container lifecycle event
type ContainerEvent struct {
	ContainerID types.ID
	Timestamp   types.Timestamp
	Action      string
	Actor       string
}
