package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

	t.Run("returns error when YAML file has invalid config", func(t *testing.T) {
		// Create a valid YAML but invalid config (empty docker host)
		configPath := filepath.Join(tmpDir, "invalid-config.yaml")
		invalidConfig := `
docker:
  host: ""
  timeout: 30s
`
		if err := os.WriteFile(configPath, []byte(invalidConfig), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Set test config path
		SetTestConfigPath(configPath)
		defer SetTestConfigPath("")

		_, err := Load()
		if err == nil {
			t.Error("Load() expected error for invalid config (empty docker host), got nil")
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
