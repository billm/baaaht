package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConfigPrecedence(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	t.Run("defaults are used when nothing else is specified", func(t *testing.T) {
		// Set test config path to a non-existent file
		nonExistentPath := filepath.Join(tmpDir, "nonexistent.yaml")
		SetTestConfigPath(nonExistentPath)
		defer SetTestConfigPath("")

		// Clear all relevant environment variables
		envVars := []string{
			"DOCKER_HOST", "API_SERVER_HOST", "API_SERVER_PORT",
			"LOG_LEVEL", "LOG_FORMAT", "SESSION_TIMEOUT", "MAX_SESSIONS",
			"EVENT_QUEUE_SIZE", "EVENT_WORKERS", "IPC_SOCKET_PATH",
			"SCHEDULER_QUEUE_SIZE", "SCHEDULER_WORKERS", "CREDENTIAL_STORE_PATH",
			"POLICY_CONFIG_PATH", "METRICS_ENABLED", "METRICS_PORT",
			"TRACE_ENABLED", "SHUTDOWN_TIMEOUT",
		}
		for _, env := range envVars {
			os.Unsetenv(env)
		}

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		// Verify default values are used
		defaultDocker := DefaultDockerConfig()
		if cfg.Docker.Host != defaultDocker.Host {
			t.Errorf("Docker.Host = %s, want default %s", cfg.Docker.Host, defaultDocker.Host)
		}
		if cfg.Docker.Timeout != defaultDocker.Timeout {
			t.Errorf("Docker.Timeout = %v, want default %v", cfg.Docker.Timeout, defaultDocker.Timeout)
		}

		defaultAPIServer := DefaultAPIServerConfig()
		if cfg.APIServer.Host != defaultAPIServer.Host {
			t.Errorf("APIServer.Host = %s, want default %s", cfg.APIServer.Host, defaultAPIServer.Host)
		}
		if cfg.APIServer.Port != defaultAPIServer.Port {
			t.Errorf("APIServer.Port = %d, want default %d", cfg.APIServer.Port, defaultAPIServer.Port)
		}

		defaultLogging := DefaultLoggingConfig()
		if cfg.Logging.Level != defaultLogging.Level {
			t.Errorf("Logging.Level = %s, want default %s", cfg.Logging.Level, defaultLogging.Level)
		}
	})

	t.Run("YAML overrides defaults", func(t *testing.T) {
		// Create a YAML config file
		configPath := filepath.Join(tmpDir, "config-override.yaml")
		yamlContent := `
docker:
  host: unix:///var/run/yaml/docker.sock
  api_version: "1.44"
  timeout: 45s
  max_retries: 5
  retry_delay: 1s
api_server:
  host: 127.0.0.1
  port: 9090
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: debug
  format: text
  output: stdout
session:
  timeout: 1h
  max_sessions: 50
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
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Clear environment variables
		envVars := []string{
			"DOCKER_HOST", "API_SERVER_HOST", "API_SERVER_PORT",
			"LOG_LEVEL", "LOG_FORMAT", "SESSION_TIMEOUT", "MAX_SESSIONS",
		}
		for _, env := range envVars {
			os.Unsetenv(env)
		}

		// Set test config path
		SetTestConfigPath(configPath)
		defer SetTestConfigPath("")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		// Verify YAML values override defaults
		defaultDocker := DefaultDockerConfig()
		if cfg.Docker.Host == defaultDocker.Host {
			t.Errorf("Docker.Host = %s (default), should be overridden by YAML", cfg.Docker.Host)
		}
		if cfg.Docker.Host != "unix:///var/run/yaml/docker.sock" {
			t.Errorf("Docker.Host = %s, want unix:///var/run/yaml/docker.sock", cfg.Docker.Host)
		}
		if cfg.Docker.Timeout != 45*time.Second {
			t.Errorf("Docker.Timeout = %v, want 45s", cfg.Docker.Timeout)
		}
		if cfg.Docker.MaxRetries != 5 {
			t.Errorf("Docker.MaxRetries = %d, want 5", cfg.Docker.MaxRetries)
		}

		defaultAPIServer := DefaultAPIServerConfig()
		if cfg.APIServer.Port == defaultAPIServer.Port {
			t.Errorf("APIServer.Port = %d (default), should be overridden by YAML", cfg.APIServer.Port)
		}
		if cfg.APIServer.Port != 9090 {
			t.Errorf("APIServer.Port = %d, want 9090", cfg.APIServer.Port)
		}

		defaultLogging := DefaultLoggingConfig()
		if cfg.Logging.Level == defaultLogging.Level {
			t.Errorf("Logging.Level = %s (default), should be overridden by YAML", cfg.Logging.Level)
		}
		if cfg.Logging.Level != "debug" {
			t.Errorf("Logging.Level = %s, want debug", cfg.Logging.Level)
		}
		if cfg.Logging.Format != "text" {
			t.Errorf("Logging.Format = %s, want text", cfg.Logging.Format)
		}
	})

	t.Run("environment variables override YAML", func(t *testing.T) {
		// Create a YAML config file
		configPath := filepath.Join(tmpDir, "config-env-override.yaml")
		yamlContent := `
docker:
  host: unix:///var/run/yaml/docker.sock
  api_version: "1.44"
  timeout: 45s
  max_retries: 5
  retry_delay: 1s
api_server:
  host: 127.0.0.1
  port: 9090
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: debug
  format: text
  output: stdout
session:
  timeout: 1h
  max_sessions: 50
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
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Set environment variables to override YAML values
		os.Setenv("DOCKER_HOST", "unix:///var/run/env/docker.sock")
		defer os.Unsetenv("DOCKER_HOST")
		os.Setenv("API_SERVER_PORT", "8888")
		defer os.Unsetenv("API_SERVER_PORT")
		os.Setenv("LOG_LEVEL", "warn")
		defer os.Unsetenv("LOG_LEVEL")

		// Set test config path
		SetTestConfigPath(configPath)
		defer SetTestConfigPath("")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		// Verify env vars override YAML values
		if cfg.Docker.Host != "unix:///var/run/env/docker.sock" {
			t.Errorf("Docker.Host = %s, want env var value unix:///var/run/env/docker.sock", cfg.Docker.Host)
		}
		if cfg.APIServer.Port != 8888 {
			t.Errorf("APIServer.Port = %d, want env var value 8888", cfg.APIServer.Port)
		}
		if cfg.Logging.Level != "warn" {
			t.Errorf("Logging.Level = %s, want env var value warn", cfg.Logging.Level)
		}

		// Verify values from YAML are still used when env var is not set
		if cfg.APIServer.Host != "127.0.0.1" {
			t.Errorf("APIServer.Host = %s, want YAML value 127.0.0.1", cfg.APIServer.Host)
		}
		if cfg.Logging.Format != "text" {
			t.Errorf("Logging.Format = %s, want YAML value text", cfg.Logging.Format)
		}
	})

	t.Run("CLI overrides override environment variables", func(t *testing.T) {
		// Create a YAML config file
		configPath := filepath.Join(tmpDir, "config-cli-override.yaml")
		yamlContent := `
docker:
  host: unix:///var/run/yaml/docker.sock
  api_version: "1.44"
  timeout: 45s
  max_retries: 5
  retry_delay: 1s
api_server:
  host: 127.0.0.1
  port: 9090
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: debug
  format: text
  output: stdout
session:
  timeout: 1h
  max_sessions: 50
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
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Set environment variables to override YAML values
		os.Setenv("DOCKER_HOST", "unix:///var/run/env/docker.sock")
		defer os.Unsetenv("DOCKER_HOST")
		os.Setenv("API_SERVER_PORT", "8888")
		defer os.Unsetenv("API_SERVER_PORT")
		os.Setenv("LOG_LEVEL", "warn")
		defer os.Unsetenv("LOG_LEVEL")

		// Set test config path
		SetTestConfigPath(configPath)
		defer SetTestConfigPath("")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		// First verify env vars are applied (override YAML)
		if cfg.Docker.Host != "unix:///var/run/env/docker.sock" {
			t.Errorf("Before CLI override: Docker.Host = %s, want env var value", cfg.Docker.Host)
		}
		if cfg.APIServer.Port != 8888 {
			t.Errorf("Before CLI override: APIServer.Port = %d, want env var value", cfg.APIServer.Port)
		}
		if cfg.Logging.Level != "warn" {
			t.Errorf("Before CLI override: Logging.Level = %s, want env var value", cfg.Logging.Level)
		}

		// Apply CLI-style overrides (this is what main.go does)
		cfg.ApplyOverrides(OverrideOptions{
			DockerHost:    "unix:///var/run/cli/docker.sock",
			APIServerPort: 9999,
			LogLevel:      "error",
		})

		// Verify CLI overrides override env vars (and thus YAML)
		if cfg.Docker.Host != "unix:///var/run/cli/docker.sock" {
			t.Errorf("After CLI override: Docker.Host = %s, want cli value unix:///var/run/cli/docker.sock", cfg.Docker.Host)
		}
		if cfg.APIServer.Port != 9999 {
			t.Errorf("After CLI override: APIServer.Port = %d, want cli value 9999", cfg.APIServer.Port)
		}
		if cfg.Logging.Level != "error" {
			t.Errorf("After CLI override: Logging.Level = %s, want cli value error", cfg.Logging.Level)
		}

		// Verify values from YAML are still used when not overridden
		if cfg.APIServer.Host != "127.0.0.1" {
			t.Errorf("APIServer.Host = %s, want YAML value 127.0.0.1 (not overridden)", cfg.APIServer.Host)
		}
		if cfg.Logging.Format != "text" {
			t.Errorf("Logging.Format = %s, want YAML value text (not overridden)", cfg.Logging.Format)
		}
	})

	t.Run("full precedence chain: defaults < YAML < env vars < CLI", func(t *testing.T) {
		// Create a YAML config file
		configPath := filepath.Join(tmpDir, "config-full-chain.yaml")
		yamlContent := `
docker:
  host: unix:///var/run/yaml/docker.sock
  api_version: "1.44"
  timeout: 45s
  max_retries: 5
  retry_delay: 1s
api_server:
  host: 127.0.0.1
  port: 9090
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
logging:
  level: debug
  format: text
  output: stdout
session:
  timeout: 1h
  max_sessions: 50
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
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Set environment variables
		os.Setenv("DOCKER_HOST", "unix:///var/run/env/docker.sock")
		defer os.Unsetenv("DOCKER_HOST")
		os.Setenv("API_SERVER_PORT", "8888")
		defer os.Unsetenv("API_SERVER_PORT")

		// Set test config path
		SetTestConfigPath(configPath)
		defer SetTestConfigPath("")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		// Before CLI overrides: verify env vars override YAML
		if cfg.Docker.Host != "unix:///var/run/env/docker.sock" {
			t.Errorf("Docker.Host = %s, want env var value (overrides YAML)", cfg.Docker.Host)
		}

		// Apply CLI overrides
		cfg.ApplyOverrides(OverrideOptions{
			DockerHost: "unix:///var/run/cli/docker.sock",
		})

		// After CLI override: verify CLI overrides env vars
		if cfg.Docker.Host != "unix:///var/run/cli/docker.sock" {
			t.Errorf("Docker.Host = %s, want CLI value (overrides env var)", cfg.Docker.Host)
		}

		// Verify a value that comes from YAML (not overridden by env var or CLI)
		defaultLogging := DefaultLoggingConfig()
		if cfg.Logging.Level == defaultLogging.Level {
			t.Errorf("Logging.Level should be from YAML (debug), not default")
		}
		if cfg.Logging.Level != "debug" {
			t.Errorf("Logging.Level = %s, want YAML value debug (not overridden)", cfg.Logging.Level)
		}

		// Verify a value that comes from defaults (not in YAML, not overridden)
		if cfg.Session.MaxSessions != 50 {
			t.Errorf("Session.MaxSessions = %d, want YAML value 50", cfg.Session.MaxSessions)
		}
	})
}

func TestLoadWithYAML(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	t.Run("loads config from default YAML file when it exists", func(t *testing.T) {
		// Create a valid YAML config file
		configPath := filepath.Join(tmpDir, "config.yaml")
		yamlContent := `
docker:
  host: unix:///var/run/custom/docker.sock
  timeout: 45s
  max_retries: 5

api_server:
  host: 127.0.0.1
  port: 9090
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s

logging:
  level: debug
  format: text

session:
  timeout: 1h
  max_sessions: 50

event:
  queue_size: 5000
  workers: 2

ipc:
  socket_path: /tmp/test-ipc.sock
  buffer_size: 32768

scheduler:
  queue_size: 500
  workers: 1

credentials:
  store_path: /tmp/test-credentials
  key_rotation_days: 90

policy:
  config_path: /tmp/test-policy.yaml
  enforcement_mode: strict

metrics:
  port: 9090

tracing:
  sample_rate: 0.1

orchestrator:
  shutdown_timeout: 30s
`
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Set test config path
		SetTestConfigPath(configPath)
		defer SetTestConfigPath("")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		// Verify values from YAML were loaded
		if cfg.Docker.Host != "unix:///var/run/custom/docker.sock" {
			t.Errorf("Docker.Host = %s, want unix:///var/run/custom/docker.sock", cfg.Docker.Host)
		}
		if cfg.Docker.Timeout != 45*time.Second {
			t.Errorf("Docker.Timeout = %v, want 45s", cfg.Docker.Timeout)
		}
		if cfg.Docker.MaxRetries != 5 {
			t.Errorf("Docker.MaxRetries = %d, want 5", cfg.Docker.MaxRetries)
		}
		if cfg.APIServer.Host != "127.0.0.1" {
			t.Errorf("APIServer.Host = %s, want 127.0.0.1", cfg.APIServer.Host)
		}
		if cfg.APIServer.Port != 9090 {
			t.Errorf("APIServer.Port = %d, want 9090", cfg.APIServer.Port)
		}
		if cfg.Logging.Level != "debug" {
			t.Errorf("Logging.Level = %s, want debug", cfg.Logging.Level)
		}
		if cfg.Logging.Format != "text" {
			t.Errorf("Logging.Format = %s, want text", cfg.Logging.Format)
		}
		if cfg.Session.Timeout != 1*time.Hour {
			t.Errorf("Session.Timeout = %v, want 1h", cfg.Session.Timeout)
		}
		if cfg.Session.MaxSessions != 50 {
			t.Errorf("Session.MaxSessions = %d, want 50", cfg.Session.MaxSessions)
		}
	})

	t.Run("uses defaults when YAML file does not exist", func(t *testing.T) {
		// Set test config path to a non-existent file
		nonExistentPath := filepath.Join(tmpDir, "nonexistent.yaml")
		SetTestConfigPath(nonExistentPath)
		defer SetTestConfigPath("")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		// Verify default values are used
		defaultDocker := DefaultDockerConfig()
		if cfg.Docker.Host != defaultDocker.Host {
			t.Errorf("Docker.Host = %s, want default %s", cfg.Docker.Host, defaultDocker.Host)
		}
		if cfg.Docker.Timeout != defaultDocker.Timeout {
			t.Errorf("Docker.Timeout = %v, want default %v", cfg.Docker.Timeout, defaultDocker.Timeout)
		}

		defaultAPIServer := DefaultAPIServerConfig()
		if cfg.APIServer.Host != defaultAPIServer.Host {
			t.Errorf("APIServer.Host = %s, want default %s", cfg.APIServer.Host, defaultAPIServer.Host)
		}
		if cfg.APIServer.Port != defaultAPIServer.Port {
			t.Errorf("APIServer.Port = %d, want default %d", cfg.APIServer.Port, defaultAPIServer.Port)
		}
	})

	t.Run("returns error when YAML file is invalid", func(t *testing.T) {
		// Create an invalid YAML config file
		configPath := filepath.Join(tmpDir, "invalid.yaml")
		invalidYAML := `
docker:
  host: unix:///var/run/docker.sock
  invalid_yaml: [
`
		if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Set test config path
		SetTestConfigPath(configPath)
		defer SetTestConfigPath("")

		_, err := Load()
		if err == nil {
			t.Error("Load() expected error for invalid YAML, got nil")
		}
	})

	t.Run("loads partial config with defaults applied", func(t *testing.T) {
		// Create a partial YAML config (docker host not specified)
		configPath := filepath.Join(tmpDir, "partial-config.yaml")
		partialConfig := `
logging:
  level: debug
api_server:
  port: 9090
`
		if err := os.WriteFile(configPath, []byte(partialConfig), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Set test config path
		SetTestConfigPath(configPath)
		defer SetTestConfigPath("")

		// Should succeed with defaults applied
		cfg, err := Load()
		if err != nil {
			t.Errorf("Load() failed with partial config: %v", err)
		}

		// Verify defaults were applied
		if cfg.Docker.Host != DefaultDockerHost {
			t.Errorf("Expected default docker host %q, got %q", DefaultDockerHost, cfg.Docker.Host)
		}
		if cfg.Docker.Timeout == 0 {
			t.Error("Expected docker timeout to have default value, got 0")
		}
		// Verify YAML overrides were applied
		if cfg.Logging.Level != "debug" {
			t.Errorf("Expected log level 'debug', got %q", cfg.Logging.Level)
		}
		if cfg.APIServer.Port != 9090 {
			t.Errorf("Expected API port 9090, got %d", cfg.APIServer.Port)
		}
	})
}

func TestLoadWithEnvVarOverride(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a YAML config file
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
docker:
  host: unix:///var/run/yaml/docker.sock
  timeout: 45s
  max_retries: 3

api_server:
  host: 0.0.0.0
  port: 9090
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s

logging:
  level: info
  format: json

session:
  timeout: 30m
  max_sessions: 100

event:
  queue_size: 10000
  workers: 4

ipc:
  socket_path: /tmp/baaaht-ipc.sock
  buffer_size: 65536

scheduler:
  queue_size: 1000
  workers: 2

credentials:
  store_path: /tmp/test-credentials
  key_rotation_days: 90

policy:
  config_path: /tmp/test-policy.yaml
  enforcement_mode: strict

metrics:
  port: 9090

tracing:
  sample_rate: 0.1

orchestrator:
  shutdown_timeout: 30s
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set test config path
	SetTestConfigPath(configPath)
	defer SetTestConfigPath("")

	// Set environment variables to override YAML values
	tests := []struct {
		name          string
		envVar        string
		envValue      string
		fieldChecker  func(*Config) interface{}
		expectedValue interface{}
	}{
		{
			name:     "DOCKER_HOST overrides YAML docker.host",
			envVar:   "DOCKER_HOST",
			envValue: "unix:///var/run/env/docker.sock",
			fieldChecker: func(cfg *Config) interface{} {
				return cfg.Docker.Host
			},
			expectedValue: "unix:///var/run/env/docker.sock",
		},
		{
			name:     "API_SERVER_PORT overrides YAML api_server.port",
			envVar:   "API_SERVER_PORT",
			envValue: "8888",
			fieldChecker: func(cfg *Config) interface{} {
				return cfg.APIServer.Port
			},
			expectedValue: 8888,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable
			os.Setenv(tt.envVar, tt.envValue)
			defer os.Unsetenv(tt.envVar)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}

			actual := tt.fieldChecker(cfg)
			if actual != tt.expectedValue {
				t.Errorf("After env var override, got %v (type %T), want %v (type %T)",
					actual, actual, tt.expectedValue, tt.expectedValue)
			}
		})
	}
}

func TestLoadWithPath(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	t.Run("empty path delegates to Load()", func(t *testing.T) {
		nonExistentPath := filepath.Join(t.TempDir(), "nonexistent.yaml")
		SetTestConfigPath(nonExistentPath)
		defer SetTestConfigPath("")

		previousDockerHost, hadDockerHost := os.LookupEnv("DOCKER_HOST")
		os.Unsetenv("DOCKER_HOST")
		defer func() {
			if hadDockerHost {
				os.Setenv("DOCKER_HOST", previousDockerHost)
				return
			}
			os.Unsetenv("DOCKER_HOST")
		}()

		cfgWithPath, err := LoadWithPath("")
		if err != nil {
			t.Fatalf("LoadWithPath(\"\") error = %v, want nil", err)
		}

		cfgFromLoad, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		if !reflect.DeepEqual(cfgWithPath, cfgFromLoad) {
			t.Error("LoadWithPath(\"\") did not delegate equivalently to Load()")
		}
	})

	t.Run("explicit path loads from file", func(t *testing.T) {
		// Create a config file with a distinctive value
		configPath := filepath.Join(tmpDir, "custom-config.yaml")
		yamlContent := `
docker:
  host: unix:///var/run/custom/docker.sock
  api_version: "1.44"
  timeout: 45s
  max_retries: 5
  retry_delay: 1s

api_server:
  host: 127.0.0.1
  port: 7777
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s

logging:
  level: debug
  format: text
  output: stdout

session:
  timeout: 1h
  max_sessions: 200
  cleanup_interval: 5m
  idle_timeout: 10m

event:
  queue_size: 5000
  workers: 2
  buffer_size: 1000
  timeout: 5s
  retry_attempts: 3
  retry_delay: 100ms

ipc:
  socket_path: /tmp/custom-ipc.sock
  buffer_size: 32768
  timeout: 30s
  max_connections: 100

scheduler:
  queue_size: 500
  workers: 1
  max_retries: 3
  retry_delay: 1s
  task_timeout: 5m
  queue_timeout: 1m

credentials:
  store_path: /tmp/custom-credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365

policy:
  config_path: /tmp/custom-policy.yaml
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
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Load with explicit path
		cfg, err := LoadWithPath(configPath)
		if err != nil {
			t.Fatalf("LoadWithPath(%q) error = %v, want nil", configPath, err)
		}

		// Verify it loaded values from the file
		if cfg.Docker.Host != "unix:///var/run/custom/docker.sock" {
			t.Errorf("Docker.Host = %q, want \"unix:///var/run/custom/docker.sock\"", cfg.Docker.Host)
		}
		if cfg.APIServer.Port != 7777 {
			t.Errorf("APIServer.Port = %d, want 7777", cfg.APIServer.Port)
		}
		if cfg.Session.MaxSessions != 200 {
			t.Errorf("Session.MaxSessions = %d, want 200", cfg.Session.MaxSessions)
		}
	})

	t.Run("missing explicit file returns error", func(t *testing.T) {
		// Try to load from a non-existent file
		nonExistentPath := filepath.Join(tmpDir, "does-not-exist.yaml")
		cfg, err := LoadWithPath(nonExistentPath)
		if err == nil {
			t.Fatal("LoadWithPath(nonexistent) error = nil, want error")
		}
		if cfg != nil {
			t.Error("LoadWithPath(nonexistent) returned non-nil config")
		}

		// Verify error is a "not found" error
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("LoadWithPath(nonexistent) error = %q, want it to contain \"not found\"", err.Error())
		}
	})

	t.Run("explicit path with env override", func(t *testing.T) {
		// Create a config file
		configPath := filepath.Join(tmpDir, "env-override.yaml")
		yamlContent := `
docker:
  host: unix:///var/run/yaml/docker.sock
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
  store_path: /tmp/test-credentials
  encryption_enabled: true
  key_rotation_days: 90
  max_credential_age: 365

policy:
  config_path: /tmp/test-policy.yaml
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
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Set environment variable
		os.Setenv("DOCKER_HOST", "unix:///var/run/env/docker.sock")
		defer os.Unsetenv("DOCKER_HOST")

		// Load with explicit path
		cfg, err := LoadWithPath(configPath)
		if err != nil {
			t.Fatalf("LoadWithPath(%q) error = %v, want nil", configPath, err)
		}

		// Verify env var takes precedence
		if cfg.Docker.Host != "unix:///var/run/env/docker.sock" {
			t.Errorf("Docker.Host = %q, want \"unix:///var/run/env/docker.sock\" (from env)", cfg.Docker.Host)
		}
	})

	t.Run("invalid file path returns error", func(t *testing.T) {
		// Try to load from a file with invalid extension
		invalidPath := filepath.Join(tmpDir, "config.txt")
		cfg, err := LoadWithPath(invalidPath)
		if err == nil {
			t.Fatal("LoadWithPath(invalid extension) error = nil, want error")
		}
		if cfg != nil {
			t.Error("LoadWithPath(invalid extension) returned non-nil config")
		}
	})
}

func TestValidateDockerConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  DockerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid docker config",
			config: DockerConfig{
				Host:       "unix:///var/run/docker.sock",
				APIVersion: "1.44",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			wantErr: false,
		},
		{
			name: "empty host",
			config: DockerConfig{
				Host:       "",
				APIVersion: "1.44",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			wantErr: true,
			errMsg:  "host cannot be empty",
		},
		{
			name: "zero timeout",
			config: DockerConfig{
				Host:       "unix:///var/run/docker.sock",
				APIVersion: "1.44",
				Timeout:    0,
				MaxRetries: 3,
			},
			wantErr: true,
			errMsg:  "timeout must be positive",
		},
		{
			name: "negative timeout",
			config: DockerConfig{
				Host:       "unix:///var/run/docker.sock",
				APIVersion: "1.44",
				Timeout:    -5 * time.Second,
				MaxRetries: 3,
			},
			wantErr: true,
			errMsg:  "timeout must be positive",
		},
		{
			name: "negative max retries",
			config: DockerConfig{
				Host:       "unix:///var/run/docker.sock",
				APIVersion: "1.44",
				Timeout:    30 * time.Second,
				MaxRetries: -1,
			},
			wantErr: true,
			errMsg:  "max retries cannot be negative",
		},
		{
			name: "valid with all TLS fields",
			config: DockerConfig{
				Host:       "unix:///var/run/docker.sock",
				TLSCert:    "/path/to/cert.pem",
				TLSKey:     "/path/to/key.pem",
				TLSCACert:  "/path/to/ca.pem",
				TLSVerify:  true,
				APIVersion: "1.44",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker:    tt.config,
				APIServer: DefaultAPIServerConfig(),
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateAPIServerConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  APIServerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid api server config",
			config: APIServerConfig{
				Host:         "0.0.0.0",
				Port:         8080,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "port too low",
			config: APIServerConfig{
				Host:         "0.0.0.0",
				Port:         0,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "port too high",
			config: APIServerConfig{
				Host:         "0.0.0.0",
				Port:         70000,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "zero read timeout",
			config: APIServerConfig{
				Host:         "0.0.0.0",
				Port:         8080,
				ReadTimeout:  0,
				WriteTimeout: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "read timeout must be positive",
		},
		{
			name: "zero write timeout",
			config: APIServerConfig{
				Host:         "0.0.0.0",
				Port:         8080,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 0,
			},
			wantErr: true,
			errMsg:  "write timeout must be positive",
		},
		{
			name: "valid with TLS enabled",
			config: APIServerConfig{
				Host:           "0.0.0.0",
				Port:           8443,
				TLSEnabled:     true,
				TLSCert:        "/path/to/cert.pem",
				TLSKey:         "/path/to/key.pem",
				ReadTimeout:    30 * time.Second,
				WriteTimeout:   30 * time.Second,
				IdleTimeout:    60 * time.Second,
				MaxConnections: 1000,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker:    DefaultDockerConfig(),
				APIServer: tt.config,
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateLoggingConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  LoggingConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid debug level",
			config: LoggingConfig{
				Level:  "debug",
				Format: "json",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "valid info level",
			config: LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "valid warn level",
			config: LoggingConfig{
				Level:  "warn",
				Format: "text",
				Output: "stderr",
			},
			wantErr: false,
		},
		{
			name: "valid error level",
			config: LoggingConfig{
				Level:  "error",
				Format: "json",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "invalid log level",
			config: LoggingConfig{
				Level:  "invalid",
				Format: "json",
				Output: "stdout",
			},
			wantErr: true,
			errMsg:  "invalid log level",
		},
		{
			name: "invalid log format",
			config: LoggingConfig{
				Level:  "info",
				Format: "invalid",
				Output: "stdout",
			},
			wantErr: true,
			errMsg:  "invalid log format",
		},
		{
			name: "valid json format",
			config: LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "valid text format",
			config: LoggingConfig{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   tt.config,
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateSessionConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  SessionConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid session config",
			config: SessionConfig{
				Timeout:     30 * time.Minute,
				MaxSessions: 100,
			},
			wantErr: false,
		},
		{
			name: "zero timeout",
			config: SessionConfig{
				Timeout:     0,
				MaxSessions: 100,
			},
			wantErr: true,
			errMsg:  "timeout must be positive",
		},
		{
			name: "negative timeout",
			config: SessionConfig{
				Timeout:     -5 * time.Minute,
				MaxSessions: 100,
			},
			wantErr: true,
			errMsg:  "timeout must be positive",
		},
		{
			name: "zero max sessions",
			config: SessionConfig{
				Timeout:     30 * time.Minute,
				MaxSessions: 0,
			},
			wantErr: true,
			errMsg:  "max sessions must be positive",
		},
		{
			name: "negative max sessions",
			config: SessionConfig{
				Timeout:     30 * time.Minute,
				MaxSessions: -10,
			},
			wantErr: true,
			errMsg:  "max sessions must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   DefaultLoggingConfig(),
				Session:   tt.config,
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateEventConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  EventConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid event config",
			config: EventConfig{
				QueueSize: 10000,
				Workers:   4,
			},
			wantErr: false,
		},
		{
			name: "zero queue size",
			config: EventConfig{
				QueueSize: 0,
				Workers:   4,
			},
			wantErr: true,
			errMsg:  "queue size must be positive",
		},
		{
			name: "zero workers",
			config: EventConfig{
				QueueSize: 10000,
				Workers:   0,
			},
			wantErr: true,
			errMsg:  "workers must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     tt.config,
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateIPCConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  IPCConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid ipc config",
			config: IPCConfig{
				SocketPath: "/tmp/baaaht-ipc.sock",
				BufferSize: 65536,
			},
			wantErr: false,
		},
		{
			name: "empty socket path",
			config: IPCConfig{
				SocketPath: "",
				BufferSize: 65536,
			},
			wantErr: true,
			errMsg:  "socket path cannot be empty",
		},
		{
			name: "zero buffer size",
			config: IPCConfig{
				SocketPath: "/tmp/baaaht-ipc.sock",
				BufferSize: 0,
			},
			wantErr: true,
			errMsg:  "buffer size must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       tt.config,
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateSchedulerConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  SchedulerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid scheduler config",
			config: SchedulerConfig{
				QueueSize: 1000,
				Workers:   2,
			},
			wantErr: false,
		},
		{
			name: "zero queue size",
			config: SchedulerConfig{
				QueueSize: 0,
				Workers:   2,
			},
			wantErr: true,
			errMsg:  "queue size must be positive",
		},
		{
			name: "zero workers",
			config: SchedulerConfig{
				QueueSize: 1000,
				Workers:   0,
			},
			wantErr: true,
			errMsg:  "workers must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: tt.config,
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateCredentialsConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  CredentialsConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid credentials config",
			config: CredentialsConfig{
				StorePath:       "/tmp/credentials",
				KeyRotationDays: 90,
			},
			wantErr: false,
		},
		{
			name: "empty store path",
			config: CredentialsConfig{
				StorePath:       "",
				KeyRotationDays: 90,
			},
			wantErr: true,
			errMsg:  "store path cannot be empty",
		},
		{
			name: "zero key rotation days",
			config: CredentialsConfig{
				StorePath:       "/tmp/credentials",
				KeyRotationDays: 0,
			},
			wantErr: true,
			errMsg:  "key rotation days must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:     DefaultLoggingConfig(),
				Session:     DefaultSessionConfig(),
				Event:       DefaultEventConfig(),
				IPC:         DefaultIPCConfig(),
				Scheduler:   DefaultSchedulerConfig(),
				Credentials: tt.config,
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidatePolicyConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  PolicyConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid strict mode",
			config: PolicyConfig{
				ConfigPath:      "/tmp/policies.yaml",
				EnforcementMode: "strict",
			},
			wantErr: false,
		},
		{
			name: "valid permissive mode",
			config: PolicyConfig{
				ConfigPath:      "/tmp/policies.yaml",
				EnforcementMode: "permissive",
			},
			wantErr: false,
		},
		{
			name: "valid disabled mode",
			config: PolicyConfig{
				ConfigPath:      "/tmp/policies.yaml",
				EnforcementMode: "disabled",
			},
			wantErr: false,
		},
		{
			name: "empty config path",
			config: PolicyConfig{
				ConfigPath:      "",
				EnforcementMode: "strict",
			},
			wantErr: true,
			errMsg:  "config path cannot be empty",
		},
		{
			name: "invalid enforcement mode",
			config: PolicyConfig{
				ConfigPath:      "/tmp/policies.yaml",
				EnforcementMode: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid enforcement mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy:       tt.config,
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateMetricsConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  MetricsConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid metrics config",
			config: MetricsConfig{
				Port: 9090,
			},
			wantErr: false,
		},
		{
			name: "port too low",
			config: MetricsConfig{
				Port: 0,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "port too high",
			config: MetricsConfig{
				Port: 70000,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      tt.config,
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateTracingConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  TracingConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid sample rate 0.1",
			config: TracingConfig{
				SampleRate: 0.1,
			},
			wantErr: false,
		},
		{
			name: "valid sample rate 0",
			config: TracingConfig{
				SampleRate: 0,
			},
			wantErr: false,
		},
		{
			name: "valid sample rate 1",
			config: TracingConfig{
				SampleRate: 1.0,
			},
			wantErr: false,
		},
		{
			name: "negative sample rate",
			config: TracingConfig{
				SampleRate: -0.1,
			},
			wantErr: true,
			errMsg:  "sample rate must be between 0 and 1",
		},
		{
			name: "sample rate greater than 1",
			config: TracingConfig{
				SampleRate: 1.5,
			},
			wantErr: true,
			errMsg:  "sample rate must be between 0 and 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      tt.config,
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateOrchestratorConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  OrchestratorConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid orchestrator config",
			config: OrchestratorConfig{
				ShutdownTimeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "zero shutdown timeout",
			config: OrchestratorConfig{
				ShutdownTimeout: 0,
			},
			wantErr: true,
			errMsg:  "shutdown timeout must be positive",
		},
		{
			name: "negative shutdown timeout",
			config: OrchestratorConfig{
				ShutdownTimeout: -10 * time.Second,
			},
			wantErr: true,
			errMsg:  "shutdown timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: tt.config,
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestConfigStringMethods(t *testing.T) {
	t.Run("DockerConfig.String()", func(t *testing.T) {
		cfg := DockerConfig{
			Host:       "unix:///var/run/docker.sock",
			APIVersion: "1.44",
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		}
		str := cfg.String()
		if !strings.Contains(str, "unix:///var/run/docker.sock") {
			t.Errorf("String() should contain host, got %s", str)
		}
		if !strings.Contains(str, "1.44") {
			t.Errorf("String() should contain API version, got %s", str)
		}
	})

	t.Run("APIServerConfig.String()", func(t *testing.T) {
		cfg := APIServerConfig{
			Host:       "0.0.0.0",
			Port:       8080,
			TLSEnabled: true,
		}
		str := cfg.String()
		if !strings.Contains(str, "0.0.0.0") {
			t.Errorf("String() should contain host, got %s", str)
		}
		if !strings.Contains(str, "8080") {
			t.Errorf("String() should contain port, got %s", str)
		}
		if !strings.Contains(str, "true") {
			t.Errorf("String() should contain TLS enabled, got %s", str)
		}
	})

	t.Run("LoggingConfig.String()", func(t *testing.T) {
		cfg := LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		}
		str := cfg.String()
		if !strings.Contains(str, "info") {
			t.Errorf("String() should contain level, got %s", str)
		}
		if !strings.Contains(str, "json") {
			t.Errorf("String() should contain format, got %s", str)
		}
	})

	t.Run("SessionConfig.String()", func(t *testing.T) {
		cfg := SessionConfig{
			Timeout:            30 * time.Minute,
			MaxSessions:        100,
			PersistenceEnabled: true,
		}
		str := cfg.String()
		if !strings.Contains(str, "30m0s") {
			t.Errorf("String() should contain timeout, got %s", str)
		}
		if !strings.Contains(str, "100") {
			t.Errorf("String() should contain max sessions, got %s", str)
		}
	})

	t.Run("EventConfig.String()", func(t *testing.T) {
		cfg := EventConfig{
			QueueSize:          10000,
			Workers:            4,
			PersistenceEnabled: true,
		}
		str := cfg.String()
		if !strings.Contains(str, "10000") {
			t.Errorf("String() should contain queue size, got %s", str)
		}
		if !strings.Contains(str, "4") {
			t.Errorf("String() should contain workers, got %s", str)
		}
	})

	t.Run("IPCConfig.String()", func(t *testing.T) {
		cfg := IPCConfig{
			SocketPath: "/tmp/baaaht-ipc.sock",
			BufferSize: 65536,
			EnableAuth: true,
		}
		str := cfg.String()
		if !strings.Contains(str, "/tmp/baaaht-ipc.sock") {
			t.Errorf("String() should contain socket path, got %s", str)
		}
		if !strings.Contains(str, "65536") {
			t.Errorf("String() should contain buffer size, got %s", str)
		}
	})

	t.Run("SchedulerConfig.String()", func(t *testing.T) {
		cfg := SchedulerConfig{
			QueueSize:  1000,
			Workers:    2,
			MaxRetries: 3,
		}
		str := cfg.String()
		if !strings.Contains(str, "1000") {
			t.Errorf("String() should contain queue size, got %s", str)
		}
		if !strings.Contains(str, "2") {
			t.Errorf("String() should contain workers, got %s", str)
		}
	})

	t.Run("CredentialsConfig.String() redacts store path", func(t *testing.T) {
		cfg := CredentialsConfig{
			StorePath:         "/tmp/credentials",
			EncryptionEnabled: true,
		}
		str := cfg.String()
		// Store path should be redacted
		if strings.Contains(str, "/tmp/credentials") {
			t.Errorf("String() should redact store path, got %s", str)
		}
		if !strings.Contains(str, "[REDACTED]") {
			t.Errorf("String() should contain [REDACTED], got %s", str)
		}
	})

	t.Run("PolicyConfig.String()", func(t *testing.T) {
		cfg := PolicyConfig{
			ConfigPath:      "/tmp/policies.yaml",
			EnforcementMode: "strict",
		}
		str := cfg.String()
		if !strings.Contains(str, "/tmp/policies.yaml") {
			t.Errorf("String() should contain config path, got %s", str)
		}
		if !strings.Contains(str, "strict") {
			t.Errorf("String() should contain enforcement mode, got %s", str)
		}
	})

	t.Run("MetricsConfig.String()", func(t *testing.T) {
		cfg := MetricsConfig{
			Enabled: true,
			Port:    9090,
			Path:    "/metrics",
		}
		str := cfg.String()
		if !strings.Contains(str, "9090") {
			t.Errorf("String() should contain port, got %s", str)
		}
		if !strings.Contains(str, "/metrics") {
			t.Errorf("String() should contain path, got %s", str)
		}
	})

	t.Run("TracingConfig.String()", func(t *testing.T) {
		cfg := TracingConfig{
			Enabled:    true,
			SampleRate: 0.1,
			Exporter:   "stdout",
		}
		str := cfg.String()
		if !strings.Contains(str, "0.10") {
			t.Errorf("String() should contain sample rate, got %s", str)
		}
		if !strings.Contains(str, "stdout") {
			t.Errorf("String() should contain exporter, got %s", str)
		}
	})

	t.Run("OrchestratorConfig.String()", func(t *testing.T) {
		cfg := OrchestratorConfig{
			ShutdownTimeout: 30 * time.Second,
			EnableProfiling: true,
			ProfilingPort:   6060,
		}
		str := cfg.String()
		if !strings.Contains(str, "30s") {
			t.Errorf("String() should contain shutdown timeout, got %s", str)
		}
		if !strings.Contains(str, "6060") {
			t.Errorf("String() should contain profiling port, got %s", str)
		}
	})
}

func TestConfigAddressMethods(t *testing.T) {
	cfg := &Config{
		Docker: DefaultDockerConfig(),
		APIServer: APIServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
		Logging:   DefaultLoggingConfig(),
		Session:   DefaultSessionConfig(),
		Event:     DefaultEventConfig(),
		IPC:       DefaultIPCConfig(),
		Scheduler: DefaultSchedulerConfig(),
		Credentials: CredentialsConfig{
			StorePath:       "/tmp/credentials",
			KeyRotationDays: 90,
		},
		Policy: PolicyConfig{
			ConfigPath:      "/tmp/policies.yaml",
			EnforcementMode: "strict",
		},
		Metrics: MetricsConfig{
			Port: 9090,
		},
		Tracing: DefaultTracingConfig(),
		Orchestrator: OrchestratorConfig{
			ProfilingPort: 6060,
		},
	}

	t.Run("APIAddress()", func(t *testing.T) {
		addr := cfg.APIAddress()
		expected := "127.0.0.1:8080"
		if addr != expected {
			t.Errorf("APIAddress() = %s, want %s", addr, expected)
		}
	})

	t.Run("MetricsAddress()", func(t *testing.T) {
		addr := cfg.MetricsAddress()
		expected := ":9090"
		if addr != expected {
			t.Errorf("MetricsAddress() = %s, want %s", addr, expected)
		}
	})

	t.Run("ProfilingAddress()", func(t *testing.T) {
		addr := cfg.ProfilingAddress()
		expected := ":6060"
		if addr != expected {
			t.Errorf("ProfilingAddress() = %s, want %s", addr, expected)
		}
	})
}

func TestApplyOverrides(t *testing.T) {
	cfg := &Config{
		Docker: DefaultDockerConfig(),
		APIServer: APIServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Session:      DefaultSessionConfig(),
		Event:        DefaultEventConfig(),
		IPC:          DefaultIPCConfig(),
		Scheduler:    DefaultSchedulerConfig(),
		Credentials:  CredentialsConfig{StorePath: "/tmp/credentials", KeyRotationDays: 90},
		Policy:       PolicyConfig{ConfigPath: "/tmp/policies.yaml", EnforcementMode: "strict"},
		Metrics:      DefaultMetricsConfig(),
		Tracing:      DefaultTracingConfig(),
		Orchestrator: DefaultOrchestratorConfig(),
	}

	t.Run("apply all overrides", func(t *testing.T) {
		overrides := OverrideOptions{
			DockerHost:    "unix:///var/run/override/docker.sock",
			APIServerHost: "127.0.0.1",
			APIServerPort: 9090,
			LogLevel:      "debug",
			LogFormat:     "text",
			LogOutput:     "stderr",
		}

		cfg.ApplyOverrides(overrides)

		if cfg.Docker.Host != "unix:///var/run/override/docker.sock" {
			t.Errorf("Docker.Host = %s, want unix:///var/run/override/docker.sock", cfg.Docker.Host)
		}
		if cfg.APIServer.Host != "127.0.0.1" {
			t.Errorf("APIServer.Host = %s, want 127.0.0.1", cfg.APIServer.Host)
		}
		if cfg.APIServer.Port != 9090 {
			t.Errorf("APIServer.Port = %d, want 9090", cfg.APIServer.Port)
		}
		if cfg.Logging.Level != "debug" {
			t.Errorf("Logging.Level = %s, want debug", cfg.Logging.Level)
		}
		if cfg.Logging.Format != "text" {
			t.Errorf("Logging.Format = %s, want text", cfg.Logging.Format)
		}
		if cfg.Logging.Output != "stderr" {
			t.Errorf("Logging.Output = %s, want stderr", cfg.Logging.Output)
		}
	})

	t.Run("apply partial overrides", func(t *testing.T) {
		// Reset config
		cfg.Docker.Host = "unix:///var/run/docker.sock"
		cfg.APIServer.Host = "0.0.0.0"
		cfg.APIServer.Port = 8080
		cfg.Logging.Level = "info"
		cfg.Logging.Format = "json"
		cfg.Logging.Output = "stdout"

		overrides := OverrideOptions{
			DockerHost: "unix:///var/run/override/docker.sock",
		}

		cfg.ApplyOverrides(overrides)

		if cfg.Docker.Host != "unix:///var/run/override/docker.sock" {
			t.Errorf("Docker.Host = %s, want unix:///var/run/override/docker.sock", cfg.Docker.Host)
		}
		// Other values should remain unchanged
		if cfg.APIServer.Host != "0.0.0.0" {
			t.Errorf("APIServer.Host = %s, want 0.0.0.0 (unchanged)", cfg.APIServer.Host)
		}
		if cfg.Logging.Level != "info" {
			t.Errorf("Logging.Level = %s, want info (unchanged)", cfg.Logging.Level)
		}
	})

	t.Run("apply empty overrides", func(t *testing.T) {
		// Reset config
		cfg.Docker.Host = "unix:///var/run/docker.sock"
		cfg.APIServer.Host = "0.0.0.0"
		cfg.APIServer.Port = 8080
		cfg.Logging.Level = "info"
		cfg.Logging.Format = "json"

		var overrides OverrideOptions

		cfg.ApplyOverrides(overrides)

		// All values should remain unchanged
		if cfg.Docker.Host != "unix:///var/run/docker.sock" {
			t.Errorf("Docker.Host = %s, want unix:///var/run/docker.sock (unchanged)", cfg.Docker.Host)
		}
		if cfg.APIServer.Host != "0.0.0.0" {
			t.Errorf("APIServer.Host = %s, want 0.0.0.0 (unchanged)", cfg.APIServer.Host)
		}
		if cfg.Logging.Level != "info" {
			t.Errorf("Logging.Level = %s, want info (unchanged)", cfg.Logging.Level)
		}
	})
}

func TestConfigFullString(t *testing.T) {
	cfg := &Config{
		Docker: DockerConfig{
			Host:       "unix:///var/run/docker.sock",
			APIVersion: "1.44",
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		APIServer: APIServerConfig{
			Host:       "0.0.0.0",
			Port:       8080,
			TLSEnabled: false,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Session: SessionConfig{
			Timeout:            30 * time.Minute,
			MaxSessions:        100,
			PersistenceEnabled: false,
		},
		Event: EventConfig{
			QueueSize:          10000,
			Workers:            4,
			PersistenceEnabled: false,
		},
		IPC: IPCConfig{
			SocketPath: "/tmp/baaaht-ipc.sock",
			BufferSize: 65536,
			EnableAuth: true,
		},
		Scheduler: SchedulerConfig{
			QueueSize:  1000,
			Workers:    2,
			MaxRetries: 3,
		},
		Credentials: CredentialsConfig{
			StorePath:         "/tmp/credentials",
			EncryptionEnabled: true,
		},
		Policy: PolicyConfig{
			ConfigPath:      "/tmp/policies.yaml",
			EnforcementMode: "strict",
		},
		Metrics: MetricsConfig{
			Enabled: false,
			Port:    9090,
			Path:    "/metrics",
		},
		Tracing: TracingConfig{
			Enabled:    false,
			SampleRate: 0.1,
			Exporter:   "stdout",
		},
		Orchestrator: OrchestratorConfig{
			ShutdownTimeout: 30 * time.Second,
			EnableProfiling: false,
			ProfilingPort:   6060,
		},
	}

	str := cfg.String()

	// Verify all sections are included
	sections := []string{
		"DockerConfig", "APIServerConfig", "LoggingConfig", "SessionConfig",
		"EventConfig", "IPCConfig", "SchedulerConfig", "CredentialsConfig",
		"PolicyConfig", "MetricsConfig", "TracingConfig", "OrchestratorConfig",
	}

	for _, section := range sections {
		if !strings.Contains(str, section) {
			t.Errorf("String() should contain %s, got %s", section, str)
		}
	}

	// Verify sensitive data is redacted
	if strings.Contains(str, "/tmp/credentials") {
		t.Errorf("String() should redact credentials store path, got %s", str)
	}

	// Verify non-sensitive data is present
	if !strings.Contains(str, "unix:///var/run/docker.sock") {
		t.Errorf("String() should contain docker host, got %s", str)
	}
}

func TestLLMValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  LLMConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid LLM config when disabled",
			config: LLMConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid LLM config when enabled with valid provider",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "anthropic",
				DefaultModel:          "anthropic/claude-sonnet-4-20250514",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "sk-test-key",
						BaseURL: "https://api.anthropic.com",
						Enabled: true,
						Models:  []string{"anthropic/claude-sonnet-4-20250514"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "enabled but missing container image",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "",
				DefaultProvider:       "anthropic",
				DefaultModel:          "anthropic/claude-sonnet-4-20250514",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "sk-test-key",
						Enabled: true,
					},
				},
			},
			wantErr: true,
			errMsg:  "container image cannot be empty",
		},
		{
			name: "enabled but missing default provider",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "",
				DefaultModel:          "anthropic/claude-sonnet-4-20250514",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "sk-test-key",
						Enabled: true,
					},
				},
			},
			wantErr: true,
			errMsg:  "default provider cannot be empty",
		},
		{
			name: "enabled but missing default model",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "anthropic",
				DefaultModel:          "",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "sk-test-key",
						Enabled: true,
					},
				},
			},
			wantErr: true,
			errMsg:  "default model cannot be empty",
		},
		{
			name: "enabled but timeout is zero",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "anthropic",
				DefaultModel:          "anthropic/claude-sonnet-4-20250514",
				Timeout:               0,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "sk-test-key",
						Enabled: true,
					},
				},
			},
			wantErr: true,
			errMsg:  "timeout must be positive",
		},
		{
			name: "enabled but max concurrent requests is zero",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "anthropic",
				DefaultModel:          "anthropic/claude-sonnet-4-20250514",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 0,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "sk-test-key",
						Enabled: true,
					},
				},
			},
			wantErr: true,
			errMsg:  "max concurrent requests must be positive",
		},
		{
			name: "enabled but default provider not in providers",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "openai",
				DefaultModel:          "openai/gpt-4o",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "sk-test-key",
						Enabled: true,
					},
				},
			},
			wantErr: true,
			errMsg:  "default provider 'openai' not found",
		},
		{
			name: "enabled provider without API key",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "anthropic",
				DefaultModel:          "anthropic/claude-sonnet-4-20250514",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "",
						Enabled: true,
					},
				},
			},
			wantErr: true,
			errMsg:  "no API key configured",
		},
		{
			name: "disabled provider without API key is valid",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "anthropic",
				DefaultModel:          "anthropic/claude-sonnet-4-20250514",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "sk-test-key",
						Enabled: true,
					},
					"openai": {
						Name:    "openai",
						APIKey:  "",
						Enabled: false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "provider with empty name",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "emptyprovider",
				DefaultModel:          "anthropic/claude-sonnet-4-20250514",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"emptyprovider": {
						Name:    "",
						APIKey:  "sk-test-key",
						Enabled: true,
					},
				},
			},
			wantErr: true,
			errMsg:  "empty name",
		},
		{
			name: "valid LLM config with multiple providers",
			config: LLMConfig{
				Enabled:               true,
				ContainerImage:        "baaaht/llm-gateway:latest",
				DefaultProvider:       "anthropic",
				DefaultModel:          "anthropic/claude-sonnet-4-20250514",
				Timeout:               120 * time.Second,
				MaxConcurrentRequests: 10,
				Providers: map[string]LLMProviderConfig{
					"anthropic": {
						Name:    "anthropic",
						APIKey:  "sk-ant-test-key",
						BaseURL: "https://api.anthropic.com",
						Enabled: true,
						Models:  []string{"anthropic/claude-sonnet-4-20250514"},
					},
					"openai": {
						Name:    "openai",
						APIKey:  "sk-openai-test-key",
						BaseURL: "https://api.openai.com/v1",
						Enabled: true,
						Models:  []string{"openai/gpt-4o"},
					},
				},
				RateLimits: map[string]int{
					"anthropic": 60,
					"openai":    60,
				},
				FallbackChains: map[string][]string{
					"anthropic/*": {"openai"},
					"openai/*":    {"anthropic"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore environment variables for API key tests
			var savedAnthropicKey, savedOpenAIKey string
			if tt.name == "enabled provider without API key" {
				savedAnthropicKey = os.Getenv("ANTHROPIC_API_KEY")
				savedOpenAIKey = os.Getenv("OPENAI_API_KEY")
				os.Unsetenv("ANTHROPIC_API_KEY")
				os.Unsetenv("OPENAI_API_KEY")
				defer func() {
					if savedAnthropicKey != "" {
						os.Setenv("ANTHROPIC_API_KEY", savedAnthropicKey)
					}
					if savedOpenAIKey != "" {
						os.Setenv("OPENAI_API_KEY", savedOpenAIKey)
					}
				}()
			}

			cfg := &Config{
				Docker: DefaultDockerConfig(),
				APIServer: APIServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Logging:   DefaultLoggingConfig(),
				Session:   DefaultSessionConfig(),
				Event:     DefaultEventConfig(),
				IPC:       DefaultIPCConfig(),
				Scheduler: DefaultSchedulerConfig(),
				Credentials: CredentialsConfig{
					StorePath:       "/tmp/credentials",
					KeyRotationDays: 90,
				},
				Policy: PolicyConfig{
					ConfigPath:      "/tmp/policies.yaml",
					EnforcementMode: "strict",
				},
				Metrics:      DefaultMetricsConfig(),
				Tracing:      DefaultTracingConfig(),
				Orchestrator: DefaultOrchestratorConfig(),
				Runtime:      DefaultRuntimeConfig(),
				Memory:       DefaultMemoryConfig(),
				GRPC:         DefaultGRPCConfig(),
				LLM:          tt.config,
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}
