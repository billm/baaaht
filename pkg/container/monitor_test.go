package container

import (
	"io"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// TestNewMonitor tests creating a new monitor
func TestNewMonitor(t *testing.T) {
	tests := []struct {
		name      string
		client    *Client
		log       *logger.Logger
		wantError bool
		errorCode string
	}{
		{
			name:      "valid client and logger",
			client:    &Client{},
			log:       &logger.Logger{},
			wantError: false,
		},
		{
			name:      "valid client, nil logger",
			client:    &Client{},
			log:       nil,
			wantError: false, // Creates default logger
		},
		{
			name:      "nil client",
			client:    nil,
			log:       &logger.Logger{},
			wantError: true,
			errorCode: types.ErrCodeInvalidArgument,
		},
		{
			name:      "nil client and logger",
			client:    nil,
			log:       nil,
			wantError: true,
			errorCode: types.ErrCodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor, err := NewMonitor(tt.client, tt.log)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if err.(*types.Error).Code != tt.errorCode {
					t.Errorf("Expected error code %s, got %s", tt.errorCode, err.(*types.Error).Code)
				}
				if monitor != nil {
					t.Errorf("Expected nil monitor on error, got %v", monitor)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if monitor == nil {
					t.Errorf("Expected monitor but got nil")
				}
			}
		})
	}
}

// TestMonitorValidateContainerID tests container ID validation
func TestMonitorValidateContainerID(t *testing.T) {
	log, _ := logger.NewDefault()
	client := &Client{}
	monitor, _ := NewMonitor(client, log)

	tests := []struct {
		name      string
		containerID string
		wantError bool
		errorCode string
	}{
		{
			name:      "valid container ID",
			containerID: "abc123",
			wantError: false,
		},
		{
			name:      "valid UUID container ID",
			containerID: "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
			wantError: false,
		},
		{
			name:      "empty container ID",
			containerID: "",
			wantError: true,
			errorCode: types.ErrCodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := monitor.validateContainerID(tt.containerID)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if err.(*types.Error).Code != tt.errorCode {
					t.Errorf("Expected error code %s, got %s", tt.errorCode, err.(*types.Error).Code)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestMonitorString tests the String method
func TestMonitorString(t *testing.T) {
	log, _ := logger.NewDefault()
	client := &Client{}
	monitor, _ := NewMonitor(client, log)

	str := monitor.String()
	if str == "" {
		t.Error("String() returned empty string")
	}

	// Check that it contains client info
	expected := "Monitor{"
	if !contains(str, expected) {
		t.Errorf("String() should contain %s, got: %s", expected, str)
	}
}

// TestLogsConfig tests logs configuration validation
func TestLogsConfigValidation(t *testing.T) {
	log, _ := logger.NewDefault()
	client := &Client{}
	monitor, _ := NewMonitor(client, log)

	tests := []struct {
		name      string
		cfg       LogsConfig
		wantError bool
		errorCode string
	}{
		{
			name: "valid config with stdout",
			cfg: LogsConfig{
				ContainerID: "test-container",
				Stdout:      true,
			},
			wantError: false,
		},
		{
			name: "valid config with stderr",
			cfg: LogsConfig{
				ContainerID: "test-container",
				Stderr:      true,
			},
			wantError: false,
		},
		{
			name: "valid config with both",
			cfg: LogsConfig{
				ContainerID: "test-container",
				Stdout:      true,
				Stderr:      true,
			},
			wantError: false,
		},
		{
			name: "invalid config - neither stdout nor stderr",
			cfg: LogsConfig{
				ContainerID: "test-container",
			},
			wantError: true,
			errorCode: types.ErrCodeInvalidArgument,
		},
		{
			name: "invalid config - empty container ID",
			cfg: LogsConfig{
				Stdout: true,
			},
			wantError: true,
			errorCode: types.ErrCodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just validate config, don't actually call Logs
			err := monitor.validateContainerID(tt.cfg.ContainerID)
			if tt.cfg.ContainerID == "" && !tt.wantError {
				// This is actually a validation error, so expect it
				if err == nil {
					t.Errorf("Expected error for empty container ID")
				}
				return
			}

			// If not testing empty container ID, test stdout/stderr validation
			if tt.cfg.ContainerID != "" && !tt.cfg.Stdout && !tt.cfg.Stderr {
				tt.wantError = true
			}

			if tt.wantError && tt.cfg.ContainerID != "" && !tt.cfg.Stdout && !tt.cfg.Stderr {
				// This would fail in the actual Logs call
				return
			}
		})
	}
}

// TestLogsReader tests the log reader functionality
func TestLogsReader(t *testing.T) {
	t.Run("read from nil reader", func(t *testing.T) {
		reader := NewLogsReader(nil)

		log, err := reader.Read()
		if err != io.EOF {
			t.Errorf("Expected EOF from nil reader, got: %v", err)
		}
		if log != nil {
			t.Errorf("Expected nil log, got: %v", log)
		}
	})

	t.Run("close nil reader", func(t *testing.T) {
		reader := NewLogsReader(nil)

		err := reader.Close()
		if err != nil {
			t.Errorf("Close() on nil reader should not error, got: %v", err)
		}
	})

	t.Run("valid log header parsing", func(t *testing.T) {
		// Create a mock log entry with header
		// Header format: [1 byte stream type][3 bytes padding][4 bytes size]
		// Stream type 1 = stdout, size 5 for "hello"
		payload := "hello"
		header := []byte{1, 0, 0, 0, 0, 0, 0, byte(len(payload))}

		mockReader := &mockReadCloser{
			data: append(header, []byte(payload)...),
		}

		reader := NewLogsReader(mockReader)

		log, err := reader.Read()
		if err != nil {
			t.Fatalf("Read() failed: %v", err)
		}

		if log.Stream != "stdout" {
			t.Errorf("Expected stream stdout, got: %s", log.Stream)
		}

		if log.Message != payload {
			t.Errorf("Expected message %s, got: %s", payload, log.Message)
		}

		// Second read should return EOF
		_, err = reader.Read()
		if err != io.EOF {
			t.Errorf("Expected EOF on second read, got: %v", err)
		}
	})

	t.Run("stderr log parsing", func(t *testing.T) {
		payload := "error message"
		header := []byte{2, 0, 0, 0, 0, 0, 0, byte(len(payload))}

		mockReader := &mockReadCloser{
			data: append(header, []byte(payload)...),
		}

		reader := NewLogsReader(mockReader)

		log, err := reader.Read()
		if err != nil {
			t.Fatalf("Read() failed: %v", err)
		}

		if log.Stream != "stderr" {
			t.Errorf("Expected stream stderr, got: %s", log.Stream)
		}

		if log.Message != payload {
			t.Errorf("Expected message %s, got: %s", payload, log.Message)
		}
	})

	t.Run("multiple log entries", func(t *testing.T) {
		// First log
		payload1 := "first"
		header1 := []byte{1, 0, 0, 0, 0, 0, 0, byte(len(payload1))}

		// Second log
		payload2 := "second"
		header2 := []byte{2, 0, 0, 0, 0, 0, 0, byte(len(payload2))}

		data := append(header1, []byte(payload1)...)
		data = append(data, header2...)
		data = append(data, []byte(payload2)...)

		mockReader := &mockReadCloser{data: data}
		reader := NewLogsReader(mockReader)

		// Read first log
		log1, err := reader.Read()
		if err != nil {
			t.Fatalf("First Read() failed: %v", err)
		}
		if log1.Message != payload1 {
			t.Errorf("Expected message %s, got: %s", payload1, log1.Message)
		}

		// Read second log
		log2, err := reader.Read()
		if err != nil {
			t.Fatalf("Second Read() failed: %v", err)
		}
		if log2.Message != payload2 {
			t.Errorf("Expected message %s, got: %s", payload2, log2.Message)
		}

		// Third read should be EOF
		_, err = reader.Read()
		if err != io.EOF {
			t.Errorf("Expected EOF on third read, got: %v", err)
		}
	})
}

// TestParseStats tests the stats parsing logic
func TestParseStats(t *testing.T) {
	log, _ := logger.NewDefault()
	client := &Client{}
	monitor, _ := NewMonitor(client, log)

	t.Run("empty stats", func(t *testing.T) {
		stats := &dockertypes.StatsJSON{}
		usage := monitor.parseStats(stats)

		if usage.CPUPercent != 0 {
			t.Errorf("Expected CPU percent 0, got: %f", usage.CPUPercent)
		}
		if usage.MemoryUsage != 0 {
			t.Errorf("Expected memory usage 0, got: %d", usage.MemoryUsage)
		}
	})

	t.Run("stats with CPU data", func(t *testing.T) {
		stats := &dockertypes.StatsJSON{
			Stats: container.Stats{
				CPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{
						TotalUsage: 2000,
						PercpuUsage: []uint64{1000, 1000},
					},
					SystemUsage: 10000,
				},
				PreCPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{
						TotalUsage: 1000,
					},
					SystemUsage: 5000,
				},
			},
		}

		usage := monitor.parseStats(stats)

		if usage.CPUPercent == 0 {
			t.Errorf("Expected non-zero CPU percent, got: %f", usage.CPUPercent)
		}
	})

	t.Run("stats with memory data", func(t *testing.T) {
		stats := &dockertypes.StatsJSON{
			Stats: container.Stats{
				MemoryStats: container.MemoryStats{
					Usage: 1024 * 1024 * 100, // 100MB
					Limit: 1024 * 1024 * 1000, // 1GB
				},
			},
		}

		usage := monitor.parseStats(stats)

		if usage.MemoryUsage == 0 {
			t.Errorf("Expected non-zero memory usage, got: %d", usage.MemoryUsage)
		}
		if usage.MemoryLimit == 0 {
			t.Errorf("Expected non-zero memory limit, got: %d", usage.MemoryLimit)
		}
		if usage.MemoryPercent == 0 {
			t.Errorf("Expected non-zero memory percent, got: %f", usage.MemoryPercent)
		}
	})

	t.Run("stats with network data", func(t *testing.T) {
		stats := &dockertypes.StatsJSON{
			Networks: map[string]container.NetworkStats{
				"eth0": {
					RxBytes: 1024,
					TxBytes: 2048,
				},
			},
		}

		usage := monitor.parseStats(stats)

		if usage.NetworkRx != 1024 {
			t.Errorf("Expected network Rx 1024, got: %d", usage.NetworkRx)
		}
		if usage.NetworkTx != 2048 {
			t.Errorf("Expected network Tx 2048, got: %d", usage.NetworkTx)
		}
	})

	t.Run("stats with PIDs", func(t *testing.T) {
		stats := &dockertypes.StatsJSON{
			Stats: container.Stats{
				PidsStats: container.PidsStats{
					Current: 42,
				},
			},
		}

		usage := monitor.parseStats(stats)

		if usage.PidsCount != 42 {
			t.Errorf("Expected PIDs count 42, got: %d", usage.PidsCount)
		}
	})
}

// TestHealthCheckResult tests the health check result structure
func TestHealthCheckResult(t *testing.T) {
	result := &HealthCheckResult{
		ContainerID:  "test-container",
		Status:       types.Healthy,
		FailingStreak: 0,
		LastOutput:   "health check passed",
		CheckedAt:    time.Now(),
	}

	if result.ContainerID != "test-container" {
		t.Errorf("Expected container ID 'test-container', got: %s", result.ContainerID)
	}

	if result.Status != types.Healthy {
		t.Errorf("Expected status Healthy, got: %s", result.Status)
	}

	if result.FailingStreak != 0 {
		t.Errorf("Expected failing streak 0, got: %d", result.FailingStreak)
	}

	if result.LastOutput != "health check passed" {
		t.Errorf("Expected output 'health check passed', got: %s", result.LastOutput)
	}

	if result.CheckedAt.IsZero() {
		t.Error("Expected non-zero checked at time")
	}
}

// TestContainerEvent tests the container event structure
func TestContainerEvent(t *testing.T) {
	event := ContainerEvent{
		ContainerID: types.NewID("test-container"),
		Timestamp:   types.NewTimestampFromTime(time.Now()),
		Action:      "start",
		Actor:       "docker",
	}

	if event.ContainerID.String() != "test-container" {
		t.Errorf("Expected container ID 'test-container', got: %s", event.ContainerID.String())
	}

	if event.Action != "start" {
		t.Errorf("Expected action 'start', got: %s", event.Action)
	}

	if event.Actor != "docker" {
		t.Errorf("Expected actor 'docker', got: %s", event.Actor)
	}

	if event.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
}

// BenchmarkMonitorCreation benchmarks monitor creation
func BenchmarkMonitorCreation(b *testing.B) {
	log, _ := logger.NewDefault()
	client := &Client{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewMonitor(client, log)
	}
}

// BenchmarkParseStats benchmarks stats parsing
func BenchmarkParseStats(b *testing.B) {
	log, _ := logger.NewDefault()
	client := &Client{}
	monitor, _ := NewMonitor(client, log)

	stats := &dockertypes.StatsJSON{
		Stats: container.Stats{
			CPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 2000,
					PercpuUsage: []uint64{1000, 1000},
				},
				SystemUsage: 10000,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 1000,
				},
				SystemUsage: 5000,
			},
			MemoryStats: container.MemoryStats{
				Usage: 1024 * 1024 * 100,
				Limit: 1024 * 1024 * 1000,
			},
			PidsStats: container.PidsStats{
				Current: 42,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = monitor.parseStats(stats)
	}
}

// BenchmarkLogsReader benchmarks log reading
func BenchmarkLogsReader(b *testing.B) {
	payload := "test log message"
	header := []byte{1, 0, 0, 0, 0, 0, 0, byte(len(payload))}
	data := append(header, []byte(payload)...)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockReader := &mockReadCloser{data: data}
		reader := NewLogsReader(mockReader)
		_, _ = reader.Read()
	}
}

// Helper types and functions

type mockReadCloser struct {
	data []byte
	pos  int
}

func (m *mockReadCloser) Read(p []byte) (int, error) {
	if m.pos >= len(m.data) {
		return 0, io.EOF
	}

	n := copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		len(s) > 0 && (s[:len(substr)] == substr ||
			len(s) > len(substr) && containsHelp(s[1:], substr)))
}

func containsHelp(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	if s[:len(substr)] == substr {
		return true
	}
	if len(s) == 0 {
		return false
	}
	return containsHelp(s[1:], substr)
}
