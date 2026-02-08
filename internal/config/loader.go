package config

import (
	"os"
	"regexp"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"gopkg.in/yaml.v3"
)

// envVarPattern matches ${VAR_NAME} and ${VAR_NAME:-default}
var envVarPattern = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)(:-([^}]*))?\}`)

// interpolateEnvVars replaces environment variable placeholders with their values
// Supports ${VAR_NAME} and ${VAR_NAME:-default_value} syntax
func interpolateEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract the variable name and default value
		parts := envVarPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match // No match found, return original
		}

		varName := parts[1]
		defaultValue := ""
		if len(parts) >= 4 && parts[3] != "" {
			defaultValue = parts[3]
		}

		// Get the environment variable value
		if value := os.Getenv(varName); value != "" {
			return value
		}

		// Return default value if env var is not set
		return defaultValue
	})
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, types.WrapError(types.ErrCodeNotFound, "configuration file not found", err)
		}
		return nil, types.WrapError(types.ErrCodeInvalidArgument, "failed to read configuration file", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalid, "failed to parse YAML configuration", err)
	}

	// Interpolate environment variables in all string fields
	interpolateEnvVarsInConfig(&cfg)

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalid, "configuration validation failed", err)
	}

	return &cfg, nil
}

// interpolateEnvVarsInConfig interpolates environment variables in all string fields
func interpolateEnvVarsInConfig(cfg *Config) {
	// Docker config
	cfg.Docker.Host = interpolateEnvVars(cfg.Docker.Host)
	cfg.Docker.TLSCert = interpolateEnvVars(cfg.Docker.TLSCert)
	cfg.Docker.TLSKey = interpolateEnvVars(cfg.Docker.TLSKey)
	cfg.Docker.TLSCACert = interpolateEnvVars(cfg.Docker.TLSCACert)
	cfg.Docker.APIVersion = interpolateEnvVars(cfg.Docker.APIVersion)

	// API Server config
	cfg.APIServer.Host = interpolateEnvVars(cfg.APIServer.Host)
	cfg.APIServer.TLSCert = interpolateEnvVars(cfg.APIServer.TLSCert)
	cfg.APIServer.TLSKey = interpolateEnvVars(cfg.APIServer.TLSKey)

	// Logging config
	cfg.Logging.Level = interpolateEnvVars(cfg.Logging.Level)
	cfg.Logging.Format = interpolateEnvVars(cfg.Logging.Format)
	cfg.Logging.Output = interpolateEnvVars(cfg.Logging.Output)
	cfg.Logging.SyslogFacility = interpolateEnvVars(cfg.Logging.SyslogFacility)

	// Session config
	cfg.Session.StoragePath = interpolateEnvVars(cfg.Session.StoragePath)

	// IPC config
	cfg.IPC.SocketPath = interpolateEnvVars(cfg.IPC.SocketPath)

	// Credentials config
	cfg.Credentials.StorePath = interpolateEnvVars(cfg.Credentials.StorePath)

	// Policy config
	cfg.Policy.ConfigPath = interpolateEnvVars(cfg.Policy.ConfigPath)
	cfg.Policy.EnforcementMode = interpolateEnvVars(cfg.Policy.EnforcementMode)

	// Metrics config
	cfg.Metrics.Path = interpolateEnvVars(cfg.Metrics.Path)

	// Tracing config
	cfg.Tracing.Exporter = interpolateEnvVars(cfg.Tracing.Exporter)
	cfg.Tracing.ExporterEndpoint = interpolateEnvVars(cfg.Tracing.ExporterEndpoint)
}
