package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

func TestLoadFromFile(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid minimal config",
			content: `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`,
			wantErr: false,
		},
		{
			name: "valid full config",
			content: `
docker:
  host: unix:///var/run/docker.sock
  tls_cert: /path/to/cert.pem
  tls_key: /path/to/key.pem
  tls_ca_cert: /path/to/ca.pem
  tls_verify: true
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
  max_connections: 1000
  tls_enabled: true
  tls_cert: /path/to/server.crt
  tls_key: /path/to/server.key
logging:
  level: debug
  format: json
  output: stdout
  syslog_facility: local0
  rotation_enabled: true
  max_size: 100
  max_backups: 3
  max_age: 28
  compress: true
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
  persistence_enabled: true
  storage_path: /tmp/sessions
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
  persistence_enabled: true
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
  enable_auth: true
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  reload_on_changes: true
  reload_interval: 1m
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: true
  port: 9090
  path: /metrics
  report_interval: 15s
tracing:
  enabled: true
  sample_rate: 0.1
  exporter: stdout
  exporter_endpoint: http://localhost:4318
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
  enable_profiling: true
  profiling_port: 6060
`,
			wantErr: false,
		},
		{
			name: "file not found",
			content: `
this content doesn't matter as the file won't exist
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name != "file not found" {
				// Create a temporary config file
				configPath := filepath.Join(tmpDir, tt.name+".yaml")
				if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
					t.Fatalf("failed to create test config file: %v", err)
				}

				cfg, err := LoadFromFile(configPath)
				if (err != nil) != tt.wantErr {
					t.Errorf("LoadFromFile() error = %v, wantErr %v", err, tt.wantErr)
					return
				}

				if !tt.wantErr {
					// Verify some basic values
					if cfg.Docker.Host != "unix:///var/run/docker.sock" {
						t.Errorf("Docker.Host = %v, want unix:///var/run/docker.sock", cfg.Docker.Host)
					}
					if cfg.APIServer.Port != 8080 {
						t.Errorf("APIServer.Port = %v, want 8080", cfg.APIServer.Port)
					}
					if cfg.Logging.Level != "info" && tt.name == "valid minimal config" {
						t.Errorf("Logging.Level = %v, want info", cfg.Logging.Level)
					}
				}
			} else {
				// Test file not found error
				nonExistentPath := filepath.Join(tmpDir, "does-not-exist.yaml")
				_, err := LoadFromFile(nonExistentPath)
				if err == nil {
					t.Error("LoadFromFile() expected error for non-existent file, got nil")
				}
			}
		})
	}
}

func TestLoadFromFileInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file with invalid YAML
	configPath := filepath.Join(tmpDir, "invalid.yaml")
	invalidContent := `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44
  timeout: 30s
# Missing closing quote above, invalid YAML
`
	if err := os.WriteFile(configPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	_, err := LoadFromFile(configPath)
	if err == nil {
		t.Error("LoadFromFile() expected error for invalid YAML, got nil")
	}
}

func TestYAMLValidation(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		filename    string
		content     string
		wantErrCode string
		wantErrMsg  string
	}{
		{
			name:        "invalid file extension",
			filename:    "config.txt",
			content:     "docker:\n  host: unix:///var/run/docker.sock\n",
			wantErrCode: "INVALID_ARGUMENT",
			wantErrMsg:  "must have .yaml or .yml extension",
		},
		{
			name:        "invalid file extension json",
			filename:    "config.json",
			content:     "{}",
			wantErrCode: "INVALID_ARGUMENT",
			wantErrMsg:  "must have .yaml or .yml extension",
		},
		{
			name:        "empty file",
			filename:    "empty.yaml",
			content:     "",
			wantErrCode: "INVALID",
			wantErrMsg:  "configuration file is empty",
		},
		{
			name:        "whitespace only file",
			filename:    "whitespace.yaml",
			content:     "   \n\t\n  \n",
			wantErrCode: "INVALID",
			wantErrMsg:  "contains only whitespace",
		},
		{
			name:     "invalid YAML syntax - unclosed quote",
			filename: "unclosed-quote.yaml",
			content: `
docker:
  host: "unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`,
			wantErrCode: "INVALID",
			wantErrMsg:  "invalid YAML syntax",
		},
		{
			name:     "invalid YAML syntax - invalid indentation",
			filename: "bad-indent.yaml",
			content: `
docker:
host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`,
			wantErrCode: "INVALID",
			wantErrMsg:  "",
		},
		{
			name:     "invalid YAML syntax - tab character",
			filename: "tab-character.yaml",
			content: `
docker:
	host: unix:///var/run/docker.sock
	api_version: "1.44"
	timeout: 30s
	max_retries: 3
	retry_delay: 1s
api_server:
	host: 0.0.0.0
	port: 8080
	read_timeout: 30s
	write_timeout: 30s
	idle_timeout: 60s
logging:
	level: info
	format: json
	output: stdout
session:
	timeout: 30m
	max_sessions: 100
	cleanup_interval: 5m
	idle_timeout: 10m
event:
	queue_size: 10000
	workers: 4
	buffer_size: 1000
	timeout: 5s
	retry_attempts: 3
	retry_delay: 100ms
ipc:
	socket_path: /tmp/baaaht-ipc.sock
	buffer_size: 65536
	timeout: 30s
	max_connections: 100
scheduler:
	queue_size: 1000
	workers: 2
	max_retries: 3
	retry_delay: 1s
	task_timeout: 5m
	queue_timeout: 1m
credentials:
	store_path: /tmp/credentials
	encryption_enabled: true
	key_rotation_days: 90
	max_credential_age: 365
policy:
	config_path: /tmp/policies.yaml
	enforcement_mode: strict
	default_quota_cpu: 1000000000
	default_quota_memory: 1073741824
metrics:
	enabled: false
	port: 9090
	path: /metrics
tracing:
	enabled: false
	sample_rate: 0.1
	exporter: stdout
orchestrator:
	shutdown_timeout: 30s
	health_check_interval: 30s
	graceful_stop_timeout: 10s
`,
			wantErrCode: "INVALID",
			wantErrMsg:  "",
		},
		{
			name:     "invalid YAML syntax - colon in value",
			filename: "colon-value.yaml",
			content: `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`,
			wantErrCode: "",
			wantErrMsg:  "",
		},
		{
			name:        "no content",
			filename:    "no-content.yaml",
			content:     "# Just a comment\n# Another comment\n",
			wantErrCode: "INVALID",
			wantErrMsg:  "contains no valid YAML content",
		},
		{
			name:        "valid extension .yml",
			filename:    "valid.yml",
			content:     "docker:\n  host: unix:///var/run/docker.sock\n  api_version: \"1.44\"\n  timeout: 30s\n  max_retries: 3\n  retry_delay: 1s\napi_server:\n  host: 0.0.0.0\n  port: 8080\n  read_timeout: 30s\n  write_timeout: 30s\n  idle_timeout: 60s\nlogging:\n  level: info\n  format: json\n  output: stdout\nsession:\n  timeout: 30m\n  max_sessions: 100\n  cleanup_interval: 5m\n  idle_timeout: 10m\nevent:\n  queue_size: 10000\n  workers: 4\n  buffer_size: 1000\n  timeout: 5s\n  retry_attempts: 3\n  retry_delay: 100ms\nipc:\n  socket_path: /tmp/baaaht-ipc.sock\n  buffer_size: 65536\n  timeout: 30s\n  max_connections: 100\nscheduler:\n  queue_size: 1000\n  workers: 2\n  max_retries: 3\n  retry_delay: 1s\n  task_timeout: 5m\n  queue_timeout: 1m\ncredentials:\n  store_path: /tmp/credentials\n  encryption_enabled: true\n  key_rotation_days: 90\n  max_credential_age: 365\npolicy:\n  config_path: /tmp/policies.yaml\n  enforcement_mode: strict\n  default_quota_cpu: 1000000000\n  default_quota_memory: 1073741824\nmetrics:\n  enabled: false\n  port: 9090\n  path: /metrics\ntracing:\n  enabled: false\n  sample_rate: 0.1\n  exporter: stdout\norchestrator:\n  shutdown_timeout: 30s\n  health_check_interval: 30s\n  graceful_stop_timeout: 10s\n",
			wantErrCode: "",
			wantErrMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, tt.filename)

			// Create the test file
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to create test config file: %v", err)
			}

			// Try to load the configuration
			cfg, err := LoadFromFile(configPath)

			// Check if we expect an error
			if tt.wantErrCode != "" {
				if err == nil {
					t.Errorf("LoadFromFile() expected error with code %q, got nil", tt.wantErrCode)
					return
				}

				// Check error code
				if gotCode := types.GetErrorCode(err); gotCode != tt.wantErrCode {
					t.Errorf("LoadFromFile() error code = %q, want %q", gotCode, tt.wantErrCode)
				}

				// Check error message (partial match)
				if tt.wantErrMsg != "" {
					errMsg := err.Error()
					if !strings.Contains(errMsg, tt.wantErrMsg) {
						t.Errorf("LoadFromFile() error message = %q, want it to contain %q", errMsg, tt.wantErrMsg)
					}
				}

				// Verify config is nil when error occurs
				if cfg != nil {
					t.Error("LoadFromFile() config should be nil when error occurs")
				}
			} else {
				// No error expected
				if err != nil {
					t.Errorf("LoadFromFile() unexpected error: %v", err)
					return
				}

				// Verify config is not nil
				if cfg == nil {
					t.Error("LoadFromFile() config should not be nil when no error occurs")
				}
			}
		})
	}
}

func TestLoadFromFileInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file with valid YAML but invalid config
	configPath := filepath.Join(tmpDir, "invalid-config.yaml")
	invalidContent := `
docker:
  host: ""  # Empty host should fail validation
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`
	if err := os.WriteFile(configPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	_, err := LoadFromFile(configPath)
	if err == nil {
		t.Error("LoadFromFile() expected error for invalid config (empty docker host), got nil")
	}
}

func TestLoadFromFileDurationParsing(t *testing.T) {
	tmpDir := t.TempDir()

	// Test various duration formats
	configPath := filepath.Join(tmpDir, "durations.yaml")
	content := `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 500ms
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 1m
  write_timeout: 2m
  idle_timeout: 1h
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	// Verify durations were parsed correctly
	expectedDurations := map[time.Duration]string{
		cfg.Docker.Timeout:        "30s",
		cfg.Docker.RetryDelay:     "500ms",
		cfg.APIServer.ReadTimeout: "1m0s",
		cfg.APIServer.WriteTimeout: "2m0s",
		cfg.APIServer.IdleTimeout:  "1h0m0s",
	}

	for duration, expected := range expectedDurations {
		if duration.String() != expected {
			t.Errorf("Duration = %v, want %v", duration.String(), expected)
		}
	}
}

func TestEnvVarInterpolation(t *testing.T) {
	// Set test environment variables
	tests := []struct {
		name    string
		setEnv  map[string]string
		content string
		verify  func(t *testing.T, cfg *Config)
	}{
		{
			name: "simple environment variable interpolation",
			setEnv: map[string]string{
				"DOCKER_HOST":      "unix:///var/run/docker.sock",
				"API_SERVER_HOST":  "0.0.0.0",
				"LOG_LEVEL":        "debug",
				"CREDENTIALS_PATH": "/tmp/credentials",
			},
			content: `
docker:
  host: ${DOCKER_HOST}
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: ${API_SERVER_HOST}
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: ${LOG_LEVEL}
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: ${CREDENTIALS_PATH}
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`,
			verify: func(t *testing.T, cfg *Config) {
				if cfg.Docker.Host != "unix:///var/run/docker.sock" {
					t.Errorf("Docker.Host = %v, want unix:///var/run/docker.sock", cfg.Docker.Host)
				}
				if cfg.APIServer.Host != "0.0.0.0" {
					t.Errorf("APIServer.Host = %v, want 0.0.0.0", cfg.APIServer.Host)
				}
				if cfg.Logging.Level != "debug" {
					t.Errorf("Logging.Level = %v, want debug", cfg.Logging.Level)
				}
				if cfg.Credentials.StorePath != "/tmp/credentials" {
					t.Errorf("Credentials.StorePath = %v, want /tmp/credentials", cfg.Credentials.StorePath)
				}
			},
		},
		{
			name: "environment variable with default value",
			setEnv: map[string]string{
				"DOCKER_HOST": "unix:///var/run/docker.sock",
				// LOG_LEVEL not set, should use default
			},
			content: `
docker:
  host: ${DOCKER_HOST}
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: ${LOG_LEVEL:-info}
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`,
			verify: func(t *testing.T, cfg *Config) {
				if cfg.Docker.Host != "unix:///var/run/docker.sock" {
					t.Errorf("Docker.Host = %v, want unix:///var/run/docker.sock", cfg.Docker.Host)
				}
				if cfg.Logging.Level != "info" {
					t.Errorf("Logging.Level = %v, want info (default)", cfg.Logging.Level)
				}
			},
		},
		{
			name: "multiple env vars in single value",
			setEnv: map[string]string{
				"PROTOCOL": "unix",
				"SOCK_PATH": "/var/run/docker.sock",
			},
			content: `
docker:
  host: ${PROTOCOL}://${SOCK_PATH}
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`,
			verify: func(t *testing.T, cfg *Config) {
				if cfg.Docker.Host != "unix:///var/run/docker.sock" {
					t.Errorf("Docker.Host = %v, want unix:///var/run/docker.sock", cfg.Docker.Host)
				}
			},
		},
		{
			name: "env var overrides default value",
			setEnv: map[string]string{
				"LOG_LEVEL": "warn",
			},
			content: `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: ${LOG_LEVEL:-info}
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`,
			verify: func(t *testing.T, cfg *Config) {
				if cfg.Logging.Level != "warn" {
					t.Errorf("Logging.Level = %v, want warn (env var override)", cfg.Logging.Level)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, tt.name+".yaml")

			// Set environment variables for this test
			for k, v := range tt.setEnv {
				os.Setenv(k, v)
			}
			defer func() {
				// Clean up environment variables
				for k := range tt.setEnv {
					os.Unsetenv(k)
				}
			}()

			// Write the config file
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to create test config file: %v", err)
			}

			cfg, err := LoadFromFile(configPath)
			if err != nil {
				t.Fatalf("LoadFromFile() error = %v", err)
			}

			tt.verify(t, cfg)
		})
	}
}

func TestReloadConfig(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	t.Run("reloads configuration from file", func(t *testing.T) {
		// Create initial config file
		configPath := filepath.Join(tmpDir, "reload-test.yaml")
		initialContent := `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`
		if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}

		// Load initial config
		initialConfig, err := LoadFromFile(configPath)
		if err != nil {
			t.Fatalf("failed to load initial config: %v", err)
		}

		// Verify initial values
		if initialConfig.Logging.Level != "info" {
			t.Errorf("initial Logging.Level = %s, want info", initialConfig.Logging.Level)
		}
		if initialConfig.APIServer.Port != 8080 {
			t.Errorf("initial APIServer.Port = %d, want 8080", initialConfig.APIServer.Port)
		}

		// Create reloader
		reloader := NewReloader(configPath, initialConfig)

		// Track callback invocations
		callbackCalled := false
		var receivedConfig *Config

		reloader.AddCallback(func(ctx context.Context, cfg *Config) error {
			callbackCalled = true
			receivedConfig = cfg
			return nil
		})

		// Update config file with new values
		updatedContent := `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 9090
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: debug
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`
		if err := os.WriteFile(configPath, []byte(updatedContent), 0644); err != nil {
			t.Fatalf("failed to update config file: %v", err)
		}

		// Trigger reload
		ctx := context.Background()
		if err := reloader.Reload(ctx); err != nil {
			t.Fatalf("Reload() error = %v", err)
		}

		// Verify callback was called
		if !callbackCalled {
			t.Error("callback was not called after reload")
		}

		// Verify config was updated
		newConfig := reloader.GetConfig()
		if newConfig.Logging.Level != "debug" {
			t.Errorf("after reload Logging.Level = %s, want debug", newConfig.Logging.Level)
		}
		if newConfig.APIServer.Port != 9090 {
			t.Errorf("after reload APIServer.Port = %d, want 9090", newConfig.APIServer.Port)
		}

		// Verify callback received the new config
		if receivedConfig == nil {
			t.Error("callback did not receive config")
		} else {
			if receivedConfig.Logging.Level != "debug" {
				t.Errorf("callback received Logging.Level = %s, want debug", receivedConfig.Logging.Level)
			}
		}
	})

	t.Run("handles reload errors gracefully", func(t *testing.T) {
		// Create initial config file
		configPath := filepath.Join(tmpDir, "reload-error-test.yaml")
		initialContent := `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`
		if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}

		// Load initial config
		initialConfig, err := LoadFromFile(configPath)
		if err != nil {
			t.Fatalf("failed to load initial config: %v", err)
		}

		// Create reloader
		reloader := NewReloader(configPath, initialConfig)

		// Delete the config file to cause an error on reload
		os.Remove(configPath)

		// Trigger reload - should return error
		ctx := context.Background()
		err = reloader.Reload(ctx)
		if err == nil {
			t.Error("Reload() expected error for missing file, got nil")
		}

		// Verify state is back to idle
		if reloader.State() != ReloadStateIdle {
			t.Errorf("after failed reload, State = %s, want idle", reloader.State())
		}

		// Verify original config is still intact
		currentConfig := reloader.GetConfig()
		if currentConfig.Logging.Level != "info" {
			t.Errorf("after failed reload, Logging.Level = %s, want original value info", currentConfig.Logging.Level)
		}
	})

	t.Run("prevents concurrent reloads", func(t *testing.T) {
		// Create initial config file
		configPath := filepath.Join(tmpDir, "reload-concurrent-test.yaml")
		initialContent := `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`
		if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}

		// Load initial config
		initialConfig, err := LoadFromFile(configPath)
		if err != nil {
			t.Fatalf("failed to load initial config: %v", err)
		}

		// Create reloader
		reloader := NewReloader(configPath, initialConfig)

		// Add a slow callback
		reloadStarted := make(chan struct{})
		reloadComplete := make(chan struct{})

		reloader.AddCallback(func(ctx context.Context, cfg *Config) error {
			close(reloadStarted)
			// Wait a bit to simulate slow callback
			time.Sleep(100 * time.Millisecond)
			close(reloadComplete)
			return nil
		})

		// Start first reload in background
		ctx := context.Background()
		firstErr := make(chan error, 1)
		go func() {
			firstErr <- reloader.Reload(ctx)
		}()

		// Wait for first reload to start
		<-reloadStarted

		// Try second reload while first is in progress
		secondErr := reloader.Reload(ctx)

		// Second reload should return nil (skip, no error)
		if secondErr != nil {
			t.Errorf("concurrent Reload() should return nil, got %v", secondErr)
		}

		// Wait for first reload to complete
		<-reloadComplete
		if err := <-firstErr; err != nil {
			t.Errorf("first Reload() error = %v", err)
		}
	})

	t.Run("stops and starts signal handling", func(t *testing.T) {
		// Create initial config file
		configPath := filepath.Join(tmpDir, "reload-startstop-test.yaml")
		initialContent := `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`
		if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}

		// Load initial config
		initialConfig, err := LoadFromFile(configPath)
		if err != nil {
			t.Fatalf("failed to load initial config: %v", err)
		}

		// Create reloader
		reloader := NewReloader(configPath, initialConfig)

		// Start the reloader
		reloader.Start()

		// Verify started
		if reloader.State() != ReloadStateIdle {
			t.Errorf("after Start(), State = %s, want idle", reloader.State())
		}

		// Stop the reloader
		reloader.Stop()

		// Verify stopped
		if reloader.State() != ReloadStateStopped {
			t.Errorf("after Stop(), State = %s, want stopped", reloader.State())
		}

		// Starting again should be idempotent
		reloader.Start()
		if reloader.State() != ReloadStateIdle {
			t.Errorf("after second Start(), State = %s, want idle", reloader.State())
		}

		reloader.Stop()
	})

	t.Run("callback error prevents config update", func(t *testing.T) {
		// Create initial config file
		configPath := filepath.Join(tmpDir, "reload-callback-error-test.yaml")
		initialContent := `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`
		if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}

		// Load initial config
		initialConfig, err := LoadFromFile(configPath)
		if err != nil {
			t.Fatalf("failed to load initial config: %v", err)
		}

		// Create reloader
		reloader := NewReloader(configPath, initialConfig)

		// Add a callback that returns an error
		reloader.AddCallback(func(ctx context.Context, cfg *Config) error {
			return types.NewError(types.ErrCodeInternal, "callback failed intentionally")
		})

		// Update config file
		updatedContent := `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 9090
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: debug
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
`
		if err := os.WriteFile(configPath, []byte(updatedContent), 0644); err != nil {
			t.Fatalf("failed to update config file: %v", err)
		}

		// Trigger reload - should fail due to callback error
		ctx := context.Background()
		err = reloader.Reload(ctx)
		if err == nil {
			t.Error("Reload() expected error from callback, got nil")
		}

		// Verify config was NOT updated
		currentConfig := reloader.GetConfig()
		if currentConfig.Logging.Level != "info" {
			t.Errorf("after failed callback, Logging.Level = %s, want original info", currentConfig.Logging.Level)
		}
		if currentConfig.APIServer.Port != 8080 {
			t.Errorf("after failed callback, APIServer.Port = %d, want original 8080", currentConfig.APIServer.Port)
		}
	})
}


func TestPartialYAMLConfigs(t *testing.T) {
tmpDir := t.TempDir()

tests := []struct {
name    string
content string
verify  func(t *testing.T, cfg *Config)
}{
{
name: "partial memory config with storage_path",
content: `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
memory:
  storage_path: /custom/memory/path
  enabled: true
`,
verify: func(t *testing.T, cfg *Config) {
if cfg.Memory.StoragePath != "/custom/memory/path" {
t.Errorf("Memory.StoragePath = %v, want /custom/memory/path", cfg.Memory.StoragePath)
}
// UserMemoryPath and GroupMemoryPath should be derived from the configured StoragePath
expectedUserPath := "/custom/memory/path/users"
if cfg.Memory.UserMemoryPath != expectedUserPath {
t.Errorf("Memory.UserMemoryPath = %v, want %v", cfg.Memory.UserMemoryPath, expectedUserPath)
}
expectedGroupPath := "/custom/memory/path/groups"
if cfg.Memory.GroupMemoryPath != expectedGroupPath {
t.Errorf("Memory.GroupMemoryPath = %v, want %v", cfg.Memory.GroupMemoryPath, expectedGroupPath)
}
if cfg.Memory.Enabled != true {
t.Errorf("Memory.Enabled = %v, want true", cfg.Memory.Enabled)
}
// Other fields should get defaults
if cfg.Memory.MaxFileSize == 0 {
t.Error("Memory.MaxFileSize should have default value, got 0")
}
if cfg.Memory.FileFormat == "" {
t.Error("Memory.FileFormat should have default value, got empty string")
}
},
},
{
name: "partial memory config with only enabled",
content: `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
memory:
  enabled: true
  max_file_size: 2048
`,
verify: func(t *testing.T, cfg *Config) {
if cfg.Memory.Enabled != true {
t.Errorf("Memory.Enabled = %v, want true", cfg.Memory.Enabled)
}
if cfg.Memory.MaxFileSize != 2048 {
t.Errorf("Memory.MaxFileSize = %v, want 2048", cfg.Memory.MaxFileSize)
}
// Other fields should get defaults
if cfg.Memory.StoragePath == "" {
t.Error("Memory.StoragePath should have default value, got empty string")
}
if cfg.Memory.FileFormat == "" {
t.Error("Memory.FileFormat should have default value, got empty string")
}
},
},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
configPath := filepath.Join(tmpDir, tt.name+".yaml")
if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
t.Fatalf("failed to create test config file: %v", err)
}

cfg, err := LoadFromFile(configPath)
if err != nil {
t.Fatalf("LoadFromFile() error = %v", err)
}

tt.verify(t, cfg)
})
}
}

func TestEnvVarInterpolationForNewFields(t *testing.T) {
tmpDir := t.TempDir()

tests := []struct {
name   string
setEnv map[string]string
content string
verify func(t *testing.T, cfg *Config)
}{
{
name: "runtime fields interpolation",
setEnv: map[string]string{
"RUNTIME_SOCKET":   "/tmp/runtime.sock",
"RUNTIME_TLS_CERT": "/tmp/cert.pem",
"RUNTIME_TLS_KEY":  "/tmp/key.pem",
"RUNTIME_TLS_CA":   "/tmp/ca.pem",
},
content: `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
runtime:
  type: docker
  socket_path: ${RUNTIME_SOCKET}
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
  tls_enabled: true
  tls_cert_path: ${RUNTIME_TLS_CERT}
  tls_key_path: ${RUNTIME_TLS_KEY}
  tls_ca_path: ${RUNTIME_TLS_CA}
`,
verify: func(t *testing.T, cfg *Config) {
if cfg.Runtime.SocketPath != "/tmp/runtime.sock" {
t.Errorf("Runtime.SocketPath = %v, want /tmp/runtime.sock", cfg.Runtime.SocketPath)
}
if cfg.Runtime.TLSCertPath != "/tmp/cert.pem" {
t.Errorf("Runtime.TLSCertPath = %v, want /tmp/cert.pem", cfg.Runtime.TLSCertPath)
}
if cfg.Runtime.TLSKeyPath != "/tmp/key.pem" {
t.Errorf("Runtime.TLSKeyPath = %v, want /tmp/key.pem", cfg.Runtime.TLSKeyPath)
}
if cfg.Runtime.TLSCAPath != "/tmp/ca.pem" {
t.Errorf("Runtime.TLSCAPath = %v, want /tmp/ca.pem", cfg.Runtime.TLSCAPath)
}
},
},
{
name: "memory fields interpolation",
setEnv: map[string]string{
"MEMORY_STORAGE":  "/var/memory",
"MEMORY_USER":     "/var/memory/user",
"MEMORY_GROUP":    "/var/memory/group",
"MEMORY_FORMAT":   "markdown",
},
content: `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
memory:
  storage_path: ${MEMORY_STORAGE}
  user_memory_path: ${MEMORY_USER}
  group_memory_path: ${MEMORY_GROUP}
  enabled: true
  max_file_size: 1024
  file_format: ${MEMORY_FORMAT}
`,
verify: func(t *testing.T, cfg *Config) {
if cfg.Memory.StoragePath != "/var/memory" {
t.Errorf("Memory.StoragePath = %v, want /var/memory", cfg.Memory.StoragePath)
}
if cfg.Memory.UserMemoryPath != "/var/memory/user" {
t.Errorf("Memory.UserMemoryPath = %v, want /var/memory/user", cfg.Memory.UserMemoryPath)
}
if cfg.Memory.GroupMemoryPath != "/var/memory/group" {
t.Errorf("Memory.GroupMemoryPath = %v, want /var/memory/group", cfg.Memory.GroupMemoryPath)
}
if cfg.Memory.FileFormat != "markdown" {
t.Errorf("Memory.FileFormat = %v, want markdown", cfg.Memory.FileFormat)
}
},
},
{
name: "grpc socket_path interpolation",
setEnv: map[string]string{
"GRPC_SOCKET": "/tmp/grpc.sock",
},
content: `
docker:
  host: unix:///var/run/docker.sock
  api_version: "1.44"
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
api_server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: info
  format: json
  output: stdout
session:
  timeout: 30m
  max_sessions: 100
  cleanup_interval: 5m
  idle_timeout: 10m
event:
  queue_size: 10000
  workers: 4
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms
ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536
  timeout: 30s
  max_connections: 100
scheduler:
  queue_size: 1000
  workers: 2
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m
credentials:
  store_path: /tmp/credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365
policy:
  config_path: /tmp/policies.yaml
  enforcement_mode: strict
  default_quota_cpu: 1000000000
  default_quota_memory: 1073741824
metrics:
  enabled: false
  port: 9090
  path: /metrics
tracing:
  enabled: false
  sample_rate: 0.1
  exporter: stdout
orchestrator:
  shutdown_timeout: 30s
  health_check_interval: 30s
  graceful_stop_timeout: 10s
grpc:
  socket_path: ${GRPC_SOCKET}
  max_recv_msg_size: 4194304
  max_send_msg_size: 4194304
  timeout: 30s
  max_connections: 1000
`,
verify: func(t *testing.T, cfg *Config) {
if cfg.GRPC.SocketPath != "/tmp/grpc.sock" {
t.Errorf("GRPC.SocketPath = %v, want /tmp/grpc.sock", cfg.GRPC.SocketPath)
}
},
},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
configPath := filepath.Join(tmpDir, tt.name+".yaml")

// Set environment variables
for k, v := range tt.setEnv {
os.Setenv(k, v)
}
defer func() {
for k := range tt.setEnv {
os.Unsetenv(k)
}
}()

if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
t.Fatalf("failed to create test config file: %v", err)
}

cfg, err := LoadFromFile(configPath)
if err != nil {
t.Fatalf("LoadFromFile() error = %v", err)
}

tt.verify(t, cfg)
})
}
}
