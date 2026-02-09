package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	Runtime      RuntimeConfig      `json:"runtime" yaml:"runtime"`
	Memory       MemoryConfig       `json:"memory" yaml:"memory"`
	GRPC         GRPCConfig         `json:"grpc" yaml:"grpc"`
	LLM          LLMConfig          `json:"llm" yaml:"llm"`
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
	Level           string `json:"level" yaml:"level"`   // debug, info, warn, error
	Format          string `json:"format" yaml:"format"` // json, text
	Output          string `json:"output" yaml:"output"` // stdout, stderr, syslog, file path
	SyslogFacility  string `json:"syslog_facility" yaml:"syslog_facility"`
	RotationEnabled bool   `json:"rotation_enabled" yaml:"rotation_enabled"`
	MaxSize         int    `json:"max_size" yaml:"max_size"` // MB
	MaxBackups      int    `json:"max_backups" yaml:"max_backups"`
	MaxAge          int    `json:"max_age" yaml:"max_age"` // days
	Compress        bool   `json:"compress" yaml:"compress"`
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
	ConfigPath         string        `json:"config_path" yaml:"config_path"`
	ReloadOnChanges    bool          `json:"reload_on_changes" yaml:"reload_on_changes"`
	ReloadInterval     time.Duration `json:"reload_interval" yaml:"reload_interval"`
	EnforcementMode    string        `json:"enforcement_mode" yaml:"enforcement_mode"` // strict, permissive, disabled
	DefaultQuotaCPU    int64         `json:"default_quota_cpu" yaml:"default_quota_cpu"`
	DefaultQuotaMemory int64         `json:"default_quota_memory" yaml:"default_quota_memory"`
}

// MetricsConfig contains metrics configuration
type MetricsConfig struct {
	Enabled        bool          `json:"enabled" yaml:"enabled"`
	Port           int           `json:"port" yaml:"port"`
	Path           string        `json:"path" yaml:"path"`
	ReportInterval time.Duration `json:"report_interval" yaml:"report_interval"`
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
	ShutdownTimeout     time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout"`
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
	GracefulStopTimeout time.Duration `json:"graceful_stop_timeout" yaml:"graceful_stop_timeout"`
	EnableProfiling     bool          `json:"enable_profiling" yaml:"enable_profiling"`
	ProfilingPort       int           `json:"profiling_port" yaml:"profiling_port"`
}

// RuntimeConfig contains container runtime configuration
type RuntimeConfig struct {
	Type        string        `json:"type" yaml:"type"` // auto, docker, apple
	SocketPath  string        `json:"socket_path,omitempty" yaml:"socket_path,omitempty"`
	Timeout     time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetries  int           `json:"max_retries" yaml:"max_retries"`
	RetryDelay  time.Duration `json:"retry_delay" yaml:"retry_delay"`
	TLSEnabled  bool          `json:"tls_enabled" yaml:"tls_enabled"`
	TLSCertPath string        `json:"tls_cert_path,omitempty" yaml:"tls_cert_path,omitempty"`
	TLSKeyPath  string        `json:"tls_key_path,omitempty" yaml:"tls_key_path,omitempty"`
	TLSCAPath   string        `json:"tls_ca_path,omitempty" yaml:"tls_ca_path,omitempty"`
}

// MemoryConfig contains memory storage configuration
type MemoryConfig struct {
	StoragePath        string `json:"storage_path" yaml:"storage_path"`                 // Base path for memory files
	UserMemoryPath     string `json:"user_memory_path" yaml:"user_memory_path"`         // Path for user-specific memory
	GroupMemoryPath    string `json:"group_memory_path" yaml:"group_memory_path"`       // Path for group-specific memory
	Enabled            bool   `json:"enabled" yaml:"enabled"`                           // Enable memory storage
	MaxFileSize        int    `json:"max_file_size" yaml:"max_file_size"`               // Maximum size of a memory file in KB
	FileFormat         string `json:"file_format" yaml:"file_format"`                   // File format (markdown)
}

// GRPCConfig contains gRPC server configuration
type GRPCConfig struct {
	SocketPath     string        `json:"socket_path" yaml:"socket_path"`
	MaxRecvMsgSize int           `json:"max_recv_msg_size" yaml:"max_recv_msg_size"` // bytes
	MaxSendMsgSize int           `json:"max_send_msg_size" yaml:"max_send_msg_size"` // bytes
	Timeout        time.Duration `json:"timeout" yaml:"timeout"`
	MaxConnections int           `json:"max_connections" yaml:"max_connections"`
}

// LLMConfig contains LLM Gateway configuration
type LLMConfig struct {
	Enabled                bool                       `json:"enabled" yaml:"enabled"`
	ContainerImage         string                     `json:"container_image" yaml:"container_image"`
	Providers              map[string]ProviderConfig  `json:"providers" yaml:"providers"`
	DefaultModel           string                     `json:"default_model" yaml:"default_model"`
	DefaultProvider        string                     `json:"default_provider" yaml:"default_provider"`
	Timeout                time.Duration              `json:"timeout" yaml:"timeout"`
	MaxConcurrentRequests  int                        `json:"max_concurrent_requests" yaml:"max_concurrent_requests"`
	RateLimits             map[string]int             `json:"rate_limits" yaml:"rate_limits"` // provider -> requests per minute
	FallbackChains         map[string][]string        `json:"fallback_chains" yaml:"fallback_chains"` // model -> []provider names
}

// ProviderConfig contains configuration for a specific LLM provider
type ProviderConfig struct {
	Name     string   `json:"name" yaml:"name"`
	APIKey   string   `json:"api_key,omitempty" yaml:"api_key,omitempty"` // From environment variable
	BaseURL  string   `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	Enabled  bool     `json:"enabled" yaml:"enabled"`
	Models   []string `json:"models" yaml:"models"`
}

// applyDefaults fills in zero-valued NEW config fields with their defaults
// This is called after loading from YAML to ensure partial configs have sensible defaults
// for newly added fields (Runtime, Memory, GRPC) that might not be in older YAML files.
// We apply defaults field-by-field to handle partial configurations properly.
func applyDefaults(cfg *Config) {
	// Runtime defaults - NEW field, may not exist in older YAML files
	// Apply field-by-field to handle partial configs
	defaultRuntime := DefaultRuntimeConfig()
	if cfg.Runtime.Type == "" {
		cfg.Runtime.Type = defaultRuntime.Type
	}
	if cfg.Runtime.SocketPath == "" {
		cfg.Runtime.SocketPath = defaultRuntime.SocketPath
	}
	if cfg.Runtime.Timeout == 0 {
		cfg.Runtime.Timeout = defaultRuntime.Timeout
	}
	if cfg.Runtime.MaxRetries == 0 {
		cfg.Runtime.MaxRetries = defaultRuntime.MaxRetries
	}
	if cfg.Runtime.RetryDelay == 0 {
		cfg.Runtime.RetryDelay = defaultRuntime.RetryDelay
	}
	if cfg.Runtime.TLSCertPath == "" {
		cfg.Runtime.TLSCertPath = defaultRuntime.TLSCertPath
	}
	if cfg.Runtime.TLSKeyPath == "" {
		cfg.Runtime.TLSKeyPath = defaultRuntime.TLSKeyPath
	}
	if cfg.Runtime.TLSCAPath == "" {
		cfg.Runtime.TLSCAPath = defaultRuntime.TLSCAPath
	}

	// Memory defaults - NEW field, may not exist in older YAML files
	// If the entire memory section is absent (all zero values), use full defaults
	// to preserve the default Enabled value. Otherwise, apply field-by-field to handle
	// partial configs, preserving an explicit Enabled=false.
	defaultMemory := DefaultMemoryConfig()
	if cfg.Memory == (MemoryConfig{}) {
		cfg.Memory = defaultMemory
	} else {
		// Apply field-by-field to handle partial configs
		if cfg.Memory.StoragePath == "" {
			cfg.Memory.StoragePath = defaultMemory.StoragePath
		}
		// UserMemoryPath and GroupMemoryPath should derive from the configured StoragePath
		if cfg.Memory.UserMemoryPath == "" {
			cfg.Memory.UserMemoryPath = filepath.Join(cfg.Memory.StoragePath, "users")
		}
		if cfg.Memory.GroupMemoryPath == "" {
			cfg.Memory.GroupMemoryPath = filepath.Join(cfg.Memory.StoragePath, "groups")
		}
		if cfg.Memory.MaxFileSize == 0 {
			cfg.Memory.MaxFileSize = defaultMemory.MaxFileSize
		}
		if cfg.Memory.FileFormat == "" {
			cfg.Memory.FileFormat = defaultMemory.FileFormat
		}
	}

	// gRPC defaults - NEW field, may not exist in older YAML files
	// Apply field-by-field to handle partial configs
	defaultGRPC := DefaultGRPCConfig()
	if cfg.GRPC.SocketPath == "" {
		cfg.GRPC.SocketPath = defaultGRPC.SocketPath
	}
	if cfg.GRPC.MaxRecvMsgSize == 0 {
		cfg.GRPC.MaxRecvMsgSize = defaultGRPC.MaxRecvMsgSize
	}
	if cfg.GRPC.MaxSendMsgSize == 0 {
		cfg.GRPC.MaxSendMsgSize = defaultGRPC.MaxSendMsgSize
	}
	if cfg.GRPC.Timeout == 0 {
		cfg.GRPC.Timeout = defaultGRPC.Timeout
	}
	if cfg.GRPC.MaxConnections == 0 {
		cfg.GRPC.MaxConnections = defaultGRPC.MaxConnections
	}

	// LLM defaults - NEW field, may not exist in older YAML files
	defaultLLM := DefaultLLMConfig()
	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = defaultLLM.Providers
	}
	if cfg.LLM.ContainerImage == "" {
		cfg.LLM.ContainerImage = defaultLLM.ContainerImage
	}
	if cfg.LLM.DefaultModel == "" {
		cfg.LLM.DefaultModel = defaultLLM.DefaultModel
	}
	if cfg.LLM.DefaultProvider == "" {
		cfg.LLM.DefaultProvider = defaultLLM.DefaultProvider
	}
	if cfg.LLM.Timeout == 0 {
		cfg.LLM.Timeout = defaultLLM.Timeout
	}
	if cfg.LLM.MaxConcurrentRequests == 0 {
		cfg.LLM.MaxConcurrentRequests = defaultLLM.MaxConcurrentRequests
	}
	if cfg.LLM.RateLimits == nil {
		cfg.LLM.RateLimits = defaultLLM.RateLimits
	}
	if cfg.LLM.FallbackChains == nil {
		cfg.LLM.FallbackChains = defaultLLM.FallbackChains
	}
}

// applyEnvOverrides applies environment variable overrides to the configuration.
// This is used by both Load() and the config reloader.
func applyEnvOverrides(cfg *Config) error {
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
	if v := os.Getenv(EnvSessionPersistence); v != "" {
		cfg.Session.PersistenceEnabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv(EnvSessionStoragePath); v != "" {
		cfg.Session.StoragePath = v
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

	// Load Runtime configuration
	if v := os.Getenv(EnvRuntimeType); v != "" {
		cfg.Runtime.Type = v
	}
	if v := os.Getenv(EnvRuntimeSocketPath); v != "" {
		cfg.Runtime.SocketPath = v
	}
	if v := os.Getenv(EnvRuntimeTimeout); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Runtime.Timeout = d
		}
	}
	cfg.Runtime.TLSCertPath = os.Getenv(EnvRuntimeTLSCert)
	cfg.Runtime.TLSKeyPath = os.Getenv(EnvRuntimeTLSKey)
	cfg.Runtime.TLSCAPath = os.Getenv(EnvRuntimeTLSCA)
	if v := os.Getenv(EnvRuntimeTLSEnabled); v != "" {
		cfg.Runtime.TLSEnabled = strings.ToLower(v) == "true" || v == "1"
	}

	// Load Memory configuration
	// Capture the existing storage path so we can keep any derived paths consistent
	oldStoragePath := cfg.Memory.StoragePath
	if v := os.Getenv(EnvMemoryStoragePath); v != "" {
		// Override the base storage path from the environment
		cfg.Memory.StoragePath = v

		// If user/group memory paths still point under the old storage base,
		// update them to use the new base path to avoid a mixed configuration.
		if cfg.Memory.UserMemoryPath != "" && strings.HasPrefix(cfg.Memory.UserMemoryPath, oldStoragePath) {
			cfg.Memory.UserMemoryPath = strings.Replace(cfg.Memory.UserMemoryPath, oldStoragePath, cfg.Memory.StoragePath, 1)
		}
		if cfg.Memory.GroupMemoryPath != "" && strings.HasPrefix(cfg.Memory.GroupMemoryPath, oldStoragePath) {
			cfg.Memory.GroupMemoryPath = strings.Replace(cfg.Memory.GroupMemoryPath, oldStoragePath, cfg.Memory.StoragePath, 1)
		}
	}
	if v := os.Getenv(EnvMemoryEnabled); v != "" {
		cfg.Memory.Enabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv(EnvMemoryMaxFileSize); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.Memory.MaxFileSize = size
		}
	}
	if v := os.Getenv(EnvMemoryFileFormat); v != "" {
		cfg.Memory.FileFormat = v
	}

	// Load gRPC configuration
	if v := os.Getenv(EnvGRPCSocketPath); v != "" {
		cfg.GRPC.SocketPath = v
	}
	if v := os.Getenv(EnvGRPCMaxRecvMsgSize); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.GRPC.MaxRecvMsgSize = size
		}
	}
	if v := os.Getenv(EnvGRPCMaxSendMsgSize); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.GRPC.MaxSendMsgSize = size
		}
	}
	if v := os.Getenv(EnvGRPCTimeout); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.GRPC.Timeout = d
		}
	}
	if v := os.Getenv(EnvGRPCMaxConnections); v != "" {
		if conns, err := strconv.Atoi(v); err == nil {
			cfg.GRPC.MaxConnections = conns
		}
	}

	// Load LLM configuration
	if v := os.Getenv(EnvLLMEnabled); v != "" {
		cfg.LLM.Enabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv(EnvLLMContainerImage); v != "" {
		cfg.LLM.ContainerImage = v
	}
	if v := os.Getenv(EnvLLMDefaultModel); v != "" {
		cfg.LLM.DefaultModel = v
	}
	if v := os.Getenv(EnvLLMDefaultProvider); v != "" {
		cfg.LLM.DefaultProvider = v
	}
	if v := os.Getenv(EnvLLMTimeout); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.LLM.Timeout = d
		}
	}
	if v := os.Getenv(EnvLLMMaxConcurrent); v != "" {
		if max, err := strconv.Atoi(v); err == nil {
			cfg.LLM.MaxConcurrentRequests = max
		}
	}
	// Load API keys from environment variables for configured providers
	for name, provider := range cfg.LLM.Providers {
		switch name {
		case "anthropic":
			if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
				provider.APIKey = key
				cfg.LLM.Providers[name] = provider
			}
		case "openai":
			if key := os.Getenv("OPENAI_API_KEY"); key != "" {
				provider.APIKey = key
				cfg.LLM.Providers[name] = provider
			}
		}
	}

	return nil
}

// Load creates a new Config by loading defaults and overriding with environment variables
func Load() (*Config, error) {
	var cfg *Config

	// Try to load from default config file if it exists
	configPath, err := GetDefaultConfigPath()
	if err == nil {
		if _, err := os.Stat(configPath); err == nil {
			// File exists, load from it
			cfg, err = LoadFromFile(configPath)
			if err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			// Some other error occurred while checking file
			return nil, fmt.Errorf("failed to check config file: %w", err)
		}
	}

	// If no config was loaded from file, use defaults
	if cfg == nil {
		cfg = &Config{
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
			Runtime:      DefaultRuntimeConfig(),
			Memory:       DefaultMemoryConfig(),
			GRPC:         DefaultGRPCConfig(),
			LLM:          DefaultLLMConfig(),
		}
	}

	// Apply environment variable overrides
	if err := applyEnvOverrides(cfg); err != nil {
		return nil, err
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
		"strict":     true,
		"permissive": true,
		"disabled":   true,
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

	// Validate Runtime configuration
	validRuntimeTypes := map[string]bool{
		"auto":   true,
		"docker": true,
		"apple":  true,
	}
	if !validRuntimeTypes[c.Runtime.Type] {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid runtime type: %s (must be auto, docker, or apple)", c.Runtime.Type))
	}
	if c.Runtime.Timeout <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "runtime timeout must be positive")
	}
	if c.Runtime.MaxRetries < 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "runtime max retries cannot be negative")
	}

	// Validate Memory configuration
	if c.Memory.StoragePath == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "memory storage path cannot be empty")
	}
	if c.Memory.MaxFileSize <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "memory max file size must be positive")
	}
	validFileFormats := map[string]bool{
		"markdown": true,
		"md":       true,
	}
	if c.Memory.FileFormat != "" && !validFileFormats[c.Memory.FileFormat] {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid memory file format: %s (must be markdown or md)", c.Memory.FileFormat))
	}
  
	// Validate gRPC configuration
	if c.GRPC.SocketPath == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "grpc socket path cannot be empty")
	}
	if c.GRPC.MaxRecvMsgSize <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "grpc max recv message size must be positive")
	}
	if c.GRPC.MaxSendMsgSize <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "grpc max send message size must be positive")
	}
	if c.GRPC.Timeout <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "grpc timeout must be positive")
	}
	if c.GRPC.MaxConnections <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "grpc max connections must be positive")
	}

	// Validate LLM configuration
	if c.LLM.Enabled {
		if c.LLM.ContainerImage == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "llm container image cannot be empty when llm is enabled")
		}
		if c.LLM.DefaultProvider == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "llm default provider cannot be empty when llm is enabled")
		}
		if c.LLM.DefaultModel == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "llm default model cannot be empty when llm is enabled")
		}
		if c.LLM.Timeout <= 0 {
			return types.NewError(types.ErrCodeInvalidArgument, "llm timeout must be positive")
		}
		if c.LLM.MaxConcurrentRequests <= 0 {
			return types.NewError(types.ErrCodeInvalidArgument, "llm max concurrent requests must be positive")
		}
		// Validate that default provider exists
		if _, exists := c.LLM.Providers[c.LLM.DefaultProvider]; !exists {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("llm default provider '%s' not found in providers configuration", c.LLM.DefaultProvider))
		}
		// Validate provider configurations
		for name, provider := range c.LLM.Providers {
			if provider.Name == "" {
				return types.NewError(types.ErrCodeInvalidArgument,
					fmt.Sprintf("llm provider '%s' has empty name", name))
			}
			if provider.Enabled && provider.APIKey == "" {
				// Check if API key can be loaded from environment
				envKey := ""
				switch name {
				case "anthropic":
					envKey = os.Getenv("ANTHROPIC_API_KEY")
				case "openai":
					envKey = os.Getenv("OPENAI_API_KEY")
				}
				if envKey == "" {
					return types.NewError(types.ErrCodeInvalidArgument,
						fmt.Sprintf("llm provider '%s' is enabled but no API key configured", name))
				}
			}
		}
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
	return fmt.Sprintf("Config{Docker: %s, API: %s, Logging: %s, Session: %s, Event: %s, IPC: %s, Scheduler: %s, Credentials: %s, Policy: %s, Metrics: %s, Tracing: %s, Orchestrator: %s, Runtime: %s, Memory: %s, GRPC: %s, LLM: %s}",
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
		c.Runtime.String(),
		c.Memory.String(),
		c.GRPC.String(),
		c.LLM.String(),
	)
}

// ApplyOverrides applies CLI flag-style overrides to the configuration.
// This is used by main.go to apply CLI flag values after loading from
// defaults, YAML file, and environment variables.
func (c *Config) ApplyOverrides(opts OverrideOptions) {
	// Docker overrides
	if opts.DockerHost != "" {
		c.Docker.Host = opts.DockerHost
	}

	// API Server overrides
	if opts.APIServerHost != "" {
		c.APIServer.Host = opts.APIServerHost
	}
	if opts.APIServerPort > 0 {
		c.APIServer.Port = opts.APIServerPort
	}

	// Logging overrides
	if opts.LogLevel != "" {
		c.Logging.Level = opts.LogLevel
	}
	if opts.LogFormat != "" {
		c.Logging.Format = opts.LogFormat
	}
	if opts.LogOutput != "" {
		c.Logging.Output = opts.LogOutput
	}

	// gRPC overrides
	if opts.GRPCSocketPath != "" {
		c.GRPC.SocketPath = opts.GRPCSocketPath
	}
	if opts.GRPCMaxRecvMsgSize > 0 {
		c.GRPC.MaxRecvMsgSize = opts.GRPCMaxRecvMsgSize
	}
	if opts.GRPCMaxSendMsgSize > 0 {
		c.GRPC.MaxSendMsgSize = opts.GRPCMaxSendMsgSize
	}
	if opts.GRPCTimeout != "" {
		if d, err := time.ParseDuration(opts.GRPCTimeout); err == nil {
			c.GRPC.Timeout = d
		}
	}
	if opts.GRPCMaxConnections > 0 {
		c.GRPC.MaxConnections = opts.GRPCMaxConnections
	}
}

// OverrideOptions contains override options typically set via CLI flags
type OverrideOptions struct {
	// Docker options
	DockerHost string

	// API Server options
	APIServerHost string
	APIServerPort int

	// Logging options
	LogLevel  string
	LogFormat string
	LogOutput string

	// gRPC options
	GRPCSocketPath     string
	GRPCMaxRecvMsgSize int
	GRPCMaxSendMsgSize int
	GRPCTimeout        string
	GRPCMaxConnections int
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

func (c RuntimeConfig) String() string {
	return fmt.Sprintf("RuntimeConfig{Type: %s, SocketPath: %s, Timeout: %s, MaxRetries: %d, TLSEnabled: %v}",
		c.Type, c.SocketPath, c.Timeout, c.MaxRetries, c.TLSEnabled)
}

func (c MemoryConfig) String() string {
	return fmt.Sprintf("MemoryConfig{StoragePath: %s, Enabled: %v, FileFormat: %s}",
		c.StoragePath, c.Enabled, c.FileFormat)
}

func (c GRPCConfig) String() string {
	return fmt.Sprintf("GRPCConfig{SocketPath: %s, MaxRecvMsgSize: %d, MaxSendMsgSize: %d, Timeout: %s, MaxConnections: %d}",
		c.SocketPath, c.MaxRecvMsgSize, c.MaxSendMsgSize, c.Timeout, c.MaxConnections)
}

func (c LLMConfig) String() string {
	providerCount := len(c.Providers)
	enabledProviders := 0
	for _, p := range c.Providers {
		if p.Enabled {
			enabledProviders++
		}
	}
	return fmt.Sprintf("LLMConfig{Enabled: %v, ContainerImage: %s, DefaultProvider: %s, DefaultModel: %s, Timeout: %s, Providers: %d/%d enabled}",
		c.Enabled, c.ContainerImage, c.DefaultProvider, c.DefaultModel, c.Timeout, enabledProviders, providerCount)
}

func (c ProviderConfig) String() string {
	return fmt.Sprintf("ProviderConfig{Name: %s, BaseURL: %s, Enabled: %v, Models: %d}",
		c.Name, c.BaseURL, c.Enabled, len(c.Models))
}