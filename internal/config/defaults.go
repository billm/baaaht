package config

import (
	"os"
	"path/filepath"
	"time"
)

// testConfigPath is an override for the default config path used in testing
// If set, GetDefaultConfigPath will return this value instead of the standard path
var testConfigPath string

// SetTestConfigPath sets a custom config path for testing purposes
// This should only be called from tests
func SetTestConfigPath(path string) {
	testConfigPath = path
}

// GetConfigDir returns the baaaht configuration directory
// Uses ~/.config/baaaht/ on Unix systems
func GetConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "baaaht"), nil
}

// GetDefaultConfigPath returns the default config file path
func GetDefaultConfigPath() (string, error) {
	// If test config path is set, use it
	if testConfigPath != "" {
		return testConfigPath, nil
	}

	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.yaml"), nil
}

const (
	// Environment variable names
	EnvDockerHost        = "DOCKER_HOST"
	EnvDockerTLSCert     = "DOCKER_TLS_CERT"
	EnvDockerTLSKey      = "DOCKER_TLS_KEY"
	EnvDockerTLSCACert   = "DOCKER_TLS_CA_CERT"
	EnvDockerTLSVerify   = "DOCKER_TLS_VERIFY"
	EnvAPIServerHost     = "API_SERVER_HOST"
	EnvAPIServerPort     = "API_SERVER_PORT"
	EnvLogLevel          = "LOG_LEVEL"
	EnvLogFormat         = "LOG_FORMAT"
	EnvSessionTimeout    = "SESSION_TIMEOUT"
	EnvMaxSessions       = "MAX_SESSIONS"
	EnvEventQueueSize    = "EVENT_QUEUE_SIZE"
	EnvEventWorkers      = "EVENT_WORKERS"
	EnvIPCSocketPath     = "IPC_SOCKET_PATH"
	EnvSchedulerQueue    = "SCHEDULER_QUEUE_SIZE"
	EnvSchedulerWorkers  = "SCHEDULER_WORKERS"
	EnvCredStorePath     = "CREDENTIAL_STORE_PATH"
	EnvPolicyConfigPath  = "POLICY_CONFIG_PATH"
	EnvMetricsEnabled    = "METRICS_ENABLED"
	EnvMetricsPort       = "METRICS_PORT"
	EnvTraceEnabled      = "TRACE_ENABLED"
	EnvShutdownTimeout   = "SHUTDOWN_TIMEOUT"
	EnvRuntimeType       = "CONTAINER_RUNTIME"
	EnvRuntimeSocketPath = "CONTAINER_RUNTIME_SOCKET"
	EnvRuntimeTimeout    = "CONTAINER_RUNTIME_TIMEOUT"
	EnvRuntimeTLSCert    = "CONTAINER_RUNTIME_TLS_CERT"
	EnvRuntimeTLSKey     = "CONTAINER_RUNTIME_TLS_KEY"
	EnvRuntimeTLSCA      = "CONTAINER_RUNTIME_TLS_CA"
	EnvRuntimeTLSEnabled = "CONTAINER_RUNTIME_TLS_ENABLED"
)

const (
	// Default API Server settings
	DefaultAPIServerHost = "0.0.0.0"
	DefaultAPIServerPort = 8080

	// Default Docker settings
	DefaultDockerHost = "unix:///var/run/docker.sock"

	// Default Logging settings
	DefaultLogLevel  = "info"
	DefaultLogFormat = "json"

	// Default Session settings
	DefaultSessionTimeout = 30 * time.Minute
	DefaultMaxSessions    = 100

	// Default Event settings
	DefaultEventQueueSize = 10000
	DefaultEventWorkers   = 4

	// Default IPC settings
	DefaultIPCSocketPath = "/tmp/baaaht-ipc.sock"

	// Default Scheduler settings
	DefaultSchedulerQueueSize = 1000
	DefaultSchedulerWorkers   = 2

	// Default Credentials settings
	DefaultCredStorePath = "" // Will be set to ~/.config/baaaht/credentials via getConfigDir()

	// Default Policy settings
	DefaultPolicyConfigPath = "" // Will be set to ~/.config/baaaht/policies.yaml via getConfigDir()

	// Default Metrics settings
	DefaultMetricsEnabled = false
	DefaultMetricsPort    = 9090

	// Default Tracing settings
	DefaultTraceEnabled = false

	// Default Orchestrator settings
	DefaultShutdownTimeout = 30 * time.Second

	// Default Runtime settings
	DefaultRuntimeType    = "auto"
	DefaultRuntimeTimeout = 30 * time.Second
)

// DefaultDockerConfig returns the default Docker configuration
func DefaultDockerConfig() DockerConfig {
	return DockerConfig{
		Host:       DefaultDockerHost,
		TLSCert:    "",
		TLSKey:     "",
		TLSCACert:  "",
		TLSVerify:  false,
		APIVersion: "1.44",
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		RetryDelay: 1 * time.Second,
	}
}

// DefaultAPIServerConfig returns the default API server configuration
func DefaultAPIServerConfig() APIServerConfig {
	return APIServerConfig{
		Host:           DefaultAPIServerHost,
		Port:           DefaultAPIServerPort,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxConnections: 1000,
		TLSEnabled:     false,
		TLSCert:        "",
		TLSKey:         "",
	}
}

// DefaultLoggingConfig returns the default logging configuration
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:           DefaultLogLevel,
		Format:          DefaultLogFormat,
		Output:          "stdout",
		SyslogFacility:  "local0",
		RotationEnabled: true,
		MaxSize:         100, // MB
		MaxBackups:      3,
		MaxAge:          28, // days
		Compress:        true,
	}
}

// DefaultSessionConfig returns the default session configuration
func DefaultSessionConfig() SessionConfig {
	storagePath := "/var/lib/baaaht/sessions" // fallback
	if configDir, err := GetConfigDir(); err == nil {
		storagePath = filepath.Join(configDir, "sessions")
	}
	return SessionConfig{
		Timeout:            DefaultSessionTimeout,
		MaxSessions:        DefaultMaxSessions,
		CleanupInterval:    5 * time.Minute,
		IdleTimeout:        10 * time.Minute,
		PersistenceEnabled: true,
		StoragePath:        storagePath,
	}
}

// DefaultEventConfig returns the default event system configuration
func DefaultEventConfig() EventConfig {
	return EventConfig{
		QueueSize:          DefaultEventQueueSize,
		Workers:            DefaultEventWorkers,
		BufferSize:         1000,
		Timeout:            5 * time.Second,
		RetryAttempts:      3,
		RetryDelay:         100 * time.Millisecond,
		PersistenceEnabled: false,
	}
}

// DefaultIPCConfig returns the default IPC configuration
func DefaultIPCConfig() IPCConfig {
	return IPCConfig{
		SocketPath:     DefaultIPCSocketPath,
		BufferSize:     65536,
		Timeout:        30 * time.Second,
		MaxConnections: 100,
		EnableAuth:     true,
	}
}

// DefaultSchedulerConfig returns the default scheduler configuration
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		QueueSize:    DefaultSchedulerQueueSize,
		Workers:      DefaultSchedulerWorkers,
		MaxRetries:   3,
		RetryDelay:   1 * time.Second,
		TaskTimeout:  5 * time.Minute,
		QueueTimeout: 1 * time.Minute,
	}
}

// DefaultCredentialsConfig returns the default credentials configuration
func DefaultCredentialsConfig() CredentialsConfig {
	storePath := "/var/lib/baaaht/credentials" // fallback
	if configDir, err := GetConfigDir(); err == nil {
		storePath = filepath.Join(configDir, "credentials")
	}
	return CredentialsConfig{
		StorePath:         storePath,
		EncryptionEnabled: true,
		KeyRotationDays:   90,
		MaxCredentialAge:  365, // days
	}
}

// DefaultPolicyConfig returns the default policy configuration
func DefaultPolicyConfig() PolicyConfig {
	configPath := "/etc/baaaht/policies.yaml" // fallback
	if configDir, err := GetConfigDir(); err == nil {
		configPath = filepath.Join(configDir, "policies.yaml")
	}
	return PolicyConfig{
		ConfigPath:         configPath,
		ReloadOnChanges:    true,
		ReloadInterval:     1 * time.Minute,
		EnforcementMode:    "strict",
		DefaultQuotaCPU:    1000000000, // 1 CPU in nanoseconds
		DefaultQuotaMemory: 1073741824, // 1GB in bytes
	}
}

// DefaultMetricsConfig returns the default metrics configuration
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Enabled:        DefaultMetricsEnabled,
		Port:           DefaultMetricsPort,
		Path:           "/metrics",
		ReportInterval: 15 * time.Second,
	}
}

// DefaultTracingConfig returns the default tracing configuration
func DefaultTracingConfig() TracingConfig {
	return TracingConfig{
		Enabled:          DefaultTraceEnabled,
		SampleRate:       0.1, // 10%
		Exporter:         "stdout",
		ExporterEndpoint: "",
	}
}

// DefaultOrchestratorConfig returns the default orchestrator configuration
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		ShutdownTimeout:     DefaultShutdownTimeout,
		HealthCheckInterval: 30 * time.Second,
		GracefulStopTimeout: 10 * time.Second,
		EnableProfiling:     false,
		ProfilingPort:       6060,
	}
}

// DefaultRuntimeConfig returns the default container runtime configuration
func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		Type:        DefaultRuntimeType,
		SocketPath:  "", // Auto-detected based on platform
		Timeout:     DefaultRuntimeTimeout,
		MaxRetries:  3,
		RetryDelay:  1 * time.Second,
		TLSEnabled:  false,
		TLSCertPath: "",
		TLSKeyPath:  "",
		TLSCAPath:   "",
	}
}
