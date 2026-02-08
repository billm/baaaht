package config

import (
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
