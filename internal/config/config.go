package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Config represents the complete configuration for the orchestrator
type Config struct {
	Docker       DockerConfig       `json:"docker" yaml:"docker"`
	APIServer    APIServerConfig    `json:"api_server" yaml:"api_server"`
	Logging      LoggingConfig      `json:"logging" yaml:"logging"`
	Session      SessionConfig      `json:"session" yaml:"session"`
	Event        EventConfig        `json:"event" yaml:"event"`
	IPC          IPCConfig          `json:"ipc" yaml:"ipc"`
	Scheduler    SchedulerConfig    `json:"scheduler" yaml:"scheduler"`
	Credentials  CredentialsConfig  `json:"credentials" yaml:"credentials"`
	Policy       PolicyConfig       `json:"policy" yaml:"policy"`
	Metrics      MetricsConfig      `json:"metrics" yaml:"metrics"`
	Tracing      TracingConfig      `json:"tracing" yaml:"tracing"`
	Orchestrator OrchestratorConfig `json:"orchestrator" yaml:"orchestrator"`
}

// DockerConfig contains Docker client configuration
type DockerConfig struct {
	Host       string        `json:"host" yaml:"host"`
	TLSCert    string        `json:"tls_cert,omitempty" yaml:"tls_cert,omitempty"`
	TLSKey     string        `json:"tls_key,omitempty" yaml:"tls_key,omitempty"`
	TLSCACert  string        `json:"tls_ca_cert,omitempty" yaml:"tls_ca_cert,omitempty"`
	TLSVerify  bool          `json:"tls_verify" yaml:"tls_verify"`
	APIVersion string        `json:"api_version" yaml:"api_version"`
	Timeout    time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetries int           `json:"max_retries" yaml:"max_retries"`
	RetryDelay time.Duration `json:"retry_delay" yaml:"retry_delay"`
}

// APIServerConfig contains API server configuration
type APIServerConfig struct {
	Host           string        `json:"host" yaml:"host"`
	Port           int           `json:"port" yaml:"port"`
	ReadTimeout    time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout   time.Duration `json:"write_timeout" yaml:"write_timeout"`
	IdleTimeout    time.Duration `json:"idle_timeout" yaml:"idle_timeout"`
	MaxConnections int           `json:"max_connections" yaml:"max_connections"`
	TLSEnabled     bool          `json:"tls_enabled" yaml:"tls_enabled"`
	TLSCert        string        `json:"tls_cert,omitempty" yaml:"tls_cert,omitempty"`
	TLSKey         string        `json:"tls_key,omitempty" yaml:"tls_key,omitempty"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level            string `json:"level" yaml:"level"`            // debug, info, warn, error
	Format           string `json:"format" yaml:"format"`           // json, text
	Output           string `json:"output" yaml:"output"`           // stdout, stderr, syslog, file path
	SyslogFacility   string `json:"syslog_facility" yaml:"syslog_facility"`
	RotationEnabled  bool   `json:"rotation_enabled" yaml:"rotation_enabled"`
	MaxSize          int    `json:"max_size" yaml:"max_size"`         // MB
	MaxBackups       int    `json:"max_backups" yaml:"max_backups"`
	MaxAge           int    `json:"max_age" yaml:"max_age"`          // days
	Compress         bool   `json:"compress" yaml:"compress"`
}

// SessionConfig contains session management configuration
type SessionConfig struct {
	Timeout            time.Duration `json:"timeout" yaml:"timeout"`
	MaxSessions        int           `json:"max_sessions" yaml:"max_sessions"`
	CleanupInterval    time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
	IdleTimeout        time.Duration `json:"idle_timeout" yaml:"idle_timeout"`
	PersistenceEnabled bool          `json:"persistence_enabled" yaml:"persistence_enabled"`
	StoragePath        string        `json:"storage_path" yaml:"storage_path"`
}

// EventConfig contains event system configuration
type EventConfig struct {
	QueueSize          int           `json:"queue_size" yaml:"queue_size"`
	Workers            int           `json:"workers" yaml:"workers"`
	BufferSize         int           `json:"buffer_size" yaml:"buffer_size"`
	Timeout            time.Duration `json:"timeout" yaml:"timeout"`
	RetryAttempts      int           `json:"retry_attempts" yaml:"retry_attempts"`
	RetryDelay         time.Duration `json:"retry_delay" yaml:"retry_delay"`
	PersistenceEnabled bool          `json:"persistence_enabled" yaml:"persistence_enabled"`
}

// IPCConfig contains IPC configuration
type IPCConfig struct {
	SocketPath     string        `json:"socket_path" yaml:"socket_path"`
	BufferSize     int           `json:"buffer_size" yaml:"buffer_size"`
	Timeout        time.Duration `json:"timeout" yaml:"timeout"`
	MaxConnections int           `json:"max_connections" yaml:"max_connections"`
	EnableAuth     bool          `json:"enable_auth" yaml:"enable_auth"`
}

// SchedulerConfig contains task scheduler configuration
type SchedulerConfig struct {
	QueueSize    int           `json:"queue_size" yaml:"queue_size"`
	Workers      int           `json:"workers" yaml:"workers"`
	MaxRetries   int           `json:"max_retries" yaml:"max_retries"`
	RetryDelay   time.Duration `json:"retry_delay" yaml:"retry_delay"`
	TaskTimeout  time.Duration `json:"task_timeout" yaml:"task_timeout"`
	QueueTimeout time.Duration `json:"queue_timeout" yaml:"queue_timeout"`
}

// CredentialsConfig contains credential management configuration
type CredentialsConfig struct {
	StorePath         string `json:"store_path" yaml:"store_path"`
	EncryptionEnabled bool   `json:"encryption_enabled" yaml:"encryption_enabled"`
	KeyRotationDays   int    `json:"key_rotation_days" yaml:"key_rotation_days"`
	MaxCredentialAge  int    `json:"max_credential_age" yaml:"max_credential_age"` // days
}

// PolicyConfig contains policy engine configuration
type PolicyConfig struct {
	ConfigPath          string        `json:"config_path" yaml:"config_path"`
	ReloadOnChanges     bool          `json:"reload_on_changes" yaml:"reload_on_changes"`
	ReloadInterval      time.Duration `json:"reload_interval" yaml:"reload_interval"`
	EnforcementMode     string        `json:"enforcement_mode" yaml:"enforcement_mode"` // strict, permissive, disabled
	DefaultQuotaCPU     int64         `json:"default_quota_cpu" yaml:"default_quota_cpu"`
	DefaultQuotaMemory  int64         `json:"default_quota_memory" yaml:"default_quota_memory"`
}

// MetricsConfig contains metrics configuration
type MetricsConfig struct {
	Enabled         bool          `json:"enabled" yaml:"enabled"`
	Port            int           `json:"port" yaml:"port"`
	Path            string        `json:"path" yaml:"path"`
	ReportInterval  time.Duration `json:"report_interval" yaml:"report_interval"`
}

// TracingConfig contains distributed tracing configuration
type TracingConfig struct {
	Enabled          bool    `json:"enabled" yaml:"enabled"`
	SampleRate       float64 `json:"sample_rate" yaml:"sample_rate"`
	Exporter         string  `json:"exporter" yaml:"exporter"` // stdout, jaeger, otlp
	ExporterEndpoint string  `json:"exporter_endpoint,omitempty" yaml:"exporter_endpoint,omitempty"`
}

// OrchestratorConfig contains orchestrator-specific configuration
type OrchestratorConfig struct {
	ShutdownTimeout       time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout"`
	HealthCheckInterval   time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
	GracefulStopTimeout   time.Duration `json:"graceful_stop_timeout" yaml:"graceful_stop_timeout"`
	EnableProfiling       bool          `json:"enable_profiling" yaml:"enable_profiling"`
	ProfilingPort         int           `json:"profiling_port" yaml:"profiling_port"`
}

// Load creates a new Config by loading defaults and overriding with environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Docker:       DefaultDockerConfig(),
		APIServer:    DefaultAPIServerConfig(),
		Logging:      DefaultLoggingConfig(),
		Session:      DefaultSessionConfig(),
		Event:        DefaultEventConfig(),
		IPC:          DefaultIPCConfig(),
		Scheduler:    DefaultSchedulerConfig(),
		Credentials:  DefaultCredentialsConfig(),
		Policy:       DefaultPolicyConfig(),
		Metrics:      DefaultMetricsConfig(),
		Tracing:      DefaultTracingConfig(),
		Orchestrator: DefaultOrchestratorConfig(),
	}

	// Load Docker configuration
	if v := os.Getenv(EnvDockerHost); v != "" {
		cfg.Docker.Host = v
	}
	cfg.Docker.TLSCert = os.Getenv(EnvDockerTLSCert)
	cfg.Docker.TLSKey = os.Getenv(EnvDockerTLSKey)
	cfg.Docker.TLSCACert = os.Getenv(EnvDockerTLSCACert)
	if v := os.Getenv(EnvDockerTLSVerify); v != "" {
		cfg.Docker.TLSVerify = strings.ToLower(v) == "true" || v == "1"
	}

	// Load API Server configuration
	if v := os.Getenv(EnvAPIServerHost); v != "" {
		cfg.APIServer.Host = v
	}
	if v := os.Getenv(EnvAPIServerPort); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.APIServer.Port = port
		}
	}

	// Load Logging configuration
	if v := os.Getenv(EnvLogLevel); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv(EnvLogFormat); v != "" {
		cfg.Logging.Format = v
	}

	// Load Session configuration
	if v := os.Getenv(EnvSessionTimeout); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Session.Timeout = d
		}
	}
	if v := os.Getenv(EnvMaxSessions); v != "" {
		if max, err := strconv.Atoi(v); err == nil {
			cfg.Session.MaxSessions = max
		}
	}

	// Load Event configuration
	if v := os.Getenv(EnvEventQueueSize); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.Event.QueueSize = size
		}
	}
	if v := os.Getenv(EnvEventWorkers); v != "" {
		if workers, err := strconv.Atoi(v); err == nil {
			cfg.Event.Workers = workers
		}
	}

	// Load IPC configuration
	if v := os.Getenv(EnvIPCSocketPath); v != "" {
		cfg.IPC.SocketPath = v
	}

	// Load Scheduler configuration
	if v := os.Getenv(EnvSchedulerQueue); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.Scheduler.QueueSize = size
		}
	}
	if v := os.Getenv(EnvSchedulerWorkers); v != "" {
		if workers, err := strconv.Atoi(v); err == nil {
			cfg.Scheduler.Workers = workers
		}
	}

	// Load Credentials configuration
	if v := os.Getenv(EnvCredStorePath); v != "" {
		cfg.Credentials.StorePath = v
	}

	// Load Policy configuration
	if v := os.Getenv(EnvPolicyConfigPath); v != "" {
		cfg.Policy.ConfigPath = v
	}

	// Load Metrics configuration
	if v := os.Getenv(EnvMetricsEnabled); v != "" {
		cfg.Metrics.Enabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv(EnvMetricsPort); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Metrics.Port = port
		}
	}

	// Load Tracing configuration
	if v := os.Getenv(EnvTraceEnabled); v != "" {
		cfg.Tracing.Enabled = strings.ToLower(v) == "true" || v == "1"
	}

	// Load Orchestrator configuration
	if v := os.Getenv(EnvShutdownTimeout); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Orchestrator.ShutdownTimeout = d
		}
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks the configuration for validity
func (c *Config) Validate() error {
	// Validate Docker configuration
	if c.Docker.Host == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "docker host cannot be empty")
	}
	if c.Docker.Timeout <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "docker timeout must be positive")
	}
	if c.Docker.MaxRetries < 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "docker max retries cannot be negative")
	}

	// Validate API Server configuration
	if c.APIServer.Port < 1 || c.APIServer.Port > 65535 {
		return types.NewError(types.ErrCodeInvalidArgument, "api server port must be between 1 and 65535")
	}
	if c.APIServer.ReadTimeout <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "api server read timeout must be positive")
	}
	if c.APIServer.WriteTimeout <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "api server write timeout must be positive")
	}

	// Validate Logging configuration
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid log level: %s (must be debug, info, warn, or error)", c.Logging.Level))
	}
	validLogFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validLogFormats[c.Logging.Format] {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid log format: %s (must be json or text)", c.Logging.Format))
	}

	// Validate Session configuration
	if c.Session.Timeout <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "session timeout must be positive")
	}
	if c.Session.MaxSessions <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "max sessions must be positive")
	}

	// Validate Event configuration
	if c.Event.QueueSize <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "event queue size must be positive")
	}
	if c.Event.Workers <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "event workers must be positive")
	}

	// Validate IPC configuration
	if c.IPC.SocketPath == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "ipc socket path cannot be empty")
	}
	if c.IPC.BufferSize <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "ipc buffer size must be positive")
	}

	// Validate Scheduler configuration
	if c.Scheduler.QueueSize <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "scheduler queue size must be positive")
	}
	if c.Scheduler.Workers <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "scheduler workers must be positive")
	}

	// Validate Credentials configuration
	if c.Credentials.StorePath == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "credential store path cannot be empty")
	}
	if c.Credentials.KeyRotationDays <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "key rotation days must be positive")
	}

	// Validate Policy configuration
	if c.Policy.ConfigPath == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "policy config path cannot be empty")
	}
	validEnforcementModes := map[string]bool{
		"strict":      true,
		"permissive":  true,
		"disabled":    true,
	}
	if !validEnforcementModes[c.Policy.EnforcementMode] {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid enforcement mode: %s (must be strict, permissive, or disabled)", c.Policy.EnforcementMode))
	}

	// Validate Metrics configuration
	if c.Metrics.Port < 1 || c.Metrics.Port > 65535 {
		return types.NewError(types.ErrCodeInvalidArgument, "metrics port must be between 1 and 65535")
	}

	// Validate Tracing configuration
	if c.Tracing.SampleRate < 0 || c.Tracing.SampleRate > 1 {
		return types.NewError(types.ErrCodeInvalidArgument, "trace sample rate must be between 0 and 1")
	}

	// Validate Orchestrator configuration
	if c.Orchestrator.ShutdownTimeout <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "shutdown timeout must be positive")
	}

	return nil
}

// APIAddress returns the API server address in host:port format
func (c *Config) APIAddress() string {
	return fmt.Sprintf("%s:%d", c.APIServer.Host, c.APIServer.Port)
}

// MetricsAddress returns the metrics server address in host:port format
func (c *Config) MetricsAddress() string {
	return fmt.Sprintf(":%d", c.Metrics.Port)
}

// ProfilingAddress returns the profiling server address in host:port format
func (c *Config) ProfilingAddress() string {
	return fmt.Sprintf(":%d", c.Orchestrator.ProfilingPort)
}

// String returns a string representation of the configuration (sensitive data is hidden)
func (c *Config) String() string {
	return fmt.Sprintf("Config{Docker: %s, API: %s, Logging: %s, Session: %s, Event: %s, IPC: %s, Scheduler: %s, Credentials: %s, Policy: %s, Metrics: %s, Tracing: %s, Orchestrator: %s}",
		c.Docker.String(),
		c.APIServer.String(),
		c.Logging.String(),
		c.Session.String(),
		c.Event.String(),
		c.IPC.String(),
		c.Scheduler.String(),
		c.Credentials.String(),
		c.Policy.String(),
		c.Metrics.String(),
		c.Tracing.String(),
		c.Orchestrator.String(),
	)
}

func (c DockerConfig) String() string {
	return fmt.Sprintf("DockerConfig{Host: %s, APIVersion: %s, Timeout: %s, MaxRetries: %d}",
		c.Host, c.APIVersion, c.Timeout, c.MaxRetries)
}

func (c APIServerConfig) String() string {
	return fmt.Sprintf("APIServerConfig{Host: %s, Port: %d, TLSEnabled: %v}",
		c.Host, c.Port, c.TLSEnabled)
}

func (c LoggingConfig) String() string {
	return fmt.Sprintf("LoggingConfig{Level: %s, Format: %s, Output: %s}",
		c.Level, c.Format, c.Output)
}

func (c SessionConfig) String() string {
	return fmt.Sprintf("SessionConfig{Timeout: %s, MaxSessions: %d, PersistenceEnabled: %v}",
		c.Timeout, c.MaxSessions, c.PersistenceEnabled)
}

func (c EventConfig) String() string {
	return fmt.Sprintf("EventConfig{QueueSize: %d, Workers: %d, PersistenceEnabled: %v}",
		c.QueueSize, c.Workers, c.PersistenceEnabled)
}

func (c IPCConfig) String() string {
	return fmt.Sprintf("IPCConfig{SocketPath: %s, BufferSize: %d, EnableAuth: %v}",
		c.SocketPath, c.BufferSize, c.EnableAuth)
}

func (c SchedulerConfig) String() string {
	return fmt.Sprintf("SchedulerConfig{QueueSize: %d, Workers: %d, MaxRetries: %d}",
		c.QueueSize, c.Workers, c.MaxRetries)
}

func (c CredentialsConfig) String() string {
	return fmt.Sprintf("CredentialsConfig{StorePath: %s, EncryptionEnabled: %v}",
		"[REDACTED]", c.EncryptionEnabled)
}

func (c PolicyConfig) String() string {
	return fmt.Sprintf("PolicyConfig{ConfigPath: %s, EnforcementMode: %s}",
		c.ConfigPath, c.EnforcementMode)
}

func (c MetricsConfig) String() string {
	return fmt.Sprintf("MetricsConfig{Enabled: %v, Port: %d, Path: %s}",
		c.Enabled, c.Port, c.Path)
}

func (c TracingConfig) String() string {
	return fmt.Sprintf("TracingConfig{Enabled: %v, SampleRate: %.2f, Exporter: %s}",
		c.Enabled, c.SampleRate, c.Exporter)
}

func (c OrchestratorConfig) String() string {
	return fmt.Sprintf("OrchestratorConfig{ShutdownTimeout: %s, EnableProfiling: %v, ProfilingPort: %d}",
		c.ShutdownTimeout, c.EnableProfiling, c.ProfilingPort)
}
