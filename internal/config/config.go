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
	Provider     ProviderConfig     `json:"provider" yaml:"provider"`
	Skills       SkillsConfig       `json:"skills" yaml:"skills"`
	Audit        AuditConfig        `json:"audit" yaml:"audit"`
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
	StoragePath     string `json:"storage_path" yaml:"storage_path"`           // Base path for memory files
	UserMemoryPath  string `json:"user_memory_path" yaml:"user_memory_path"`   // Path for user-specific memory
	GroupMemoryPath string `json:"group_memory_path" yaml:"group_memory_path"` // Path for group-specific memory
	Enabled         bool   `json:"enabled" yaml:"enabled"`                     // Enable memory storage
	MaxFileSize     int    `json:"max_file_size" yaml:"max_file_size"`         // Maximum size of a memory file in KB
	FileFormat      string `json:"file_format" yaml:"file_format"`             // File format (markdown)
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
	Enabled               bool                         `json:"enabled" yaml:"enabled"`
	ContainerImage        string                       `json:"container_image" yaml:"container_image"`
	Providers             map[string]LLMProviderConfig `json:"providers" yaml:"providers"`
	DefaultModel          string                       `json:"default_model" yaml:"default_model"`
	DefaultProvider       string                       `json:"default_provider" yaml:"default_provider"`
	Timeout               time.Duration                `json:"timeout" yaml:"timeout"`
	MaxConcurrentRequests int                          `json:"max_concurrent_requests" yaml:"max_concurrent_requests"`
	RateLimits            map[string]int               `json:"rate_limits" yaml:"rate_limits"`         // provider -> requests per minute
	FallbackChains        map[string][]string          `json:"fallback_chains" yaml:"fallback_chains"` // model -> []provider names
}

// LLMProviderConfig contains configuration for a specific LLM provider
type LLMProviderConfig struct {
	Name    string   `json:"name" yaml:"name"`
	APIKey  string   `json:"api_key,omitempty" yaml:"api_key,omitempty"` // From environment variable
	BaseURL string   `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	Enabled bool     `json:"enabled" yaml:"enabled"`
	Models  []string `json:"models" yaml:"models"`
}

// ProviderConfig contains provider configuration
type ProviderConfig struct {
	DefaultProvider       string                 `json:"default_provider" yaml:"default_provider"`
	FailoverEnabled       bool                   `json:"failover_enabled" yaml:"failover_enabled"`
	FailoverThreshold     int                    `json:"failover_threshold" yaml:"failover_threshold"`
	HealthCheckInterval   time.Duration          `json:"health_check_interval" yaml:"health_check_interval"`
	CircuitBreakerTimeout time.Duration          `json:"circuit_breaker_timeout" yaml:"circuit_breaker_timeout"`
	Anthropic             ProviderSpecificConfig `json:"anthropic" yaml:"anthropic"`
	OpenAI                ProviderSpecificConfig `json:"openai" yaml:"openai"`
}

// ProviderSpecificConfig contains configuration for a specific provider
type ProviderSpecificConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	BaseURL    string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	Timeout    int    `json:"timeout,omitempty" yaml:"timeout,omitempty"` // seconds
	MaxRetries int    `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
	Priority   int    `json:"priority,omitempty" yaml:"priority,omitempty"`
}

// ProviderOverrideOptions contains provider-specific override options
type ProviderOverrideOptions struct {
	DefaultProvider   string
	FailoverEnabled   *bool
	FailoverThreshold *int
	Providers         map[string]ProviderSpecificOverride
}

// ProviderSpecificOverride contains override options for a specific provider
type ProviderSpecificOverride struct {
	Enabled    *bool
	BaseURL    string
	Timeout    string // duration string
	MaxRetries *int
	APIKey     string
}

// SkillsConfig contains skills system configuration
type SkillsConfig struct {
	Enabled           bool              `json:"enabled" yaml:"enabled"`
	StoragePath       string            `json:"storage_path" yaml:"storage_path"`             // Base path for skill files
	MaxSkillsPerOwner int               `json:"max_skills_per_owner" yaml:"max_skills_per_owner"` // Max skills per user/group
	AutoLoad          bool              `json:"auto_load" yaml:"auto_load"`                     // Auto-load skills from directories
	LoadConfig        SkillsLoadConfig  `json:"load_config" yaml:"load_config"`                 // Load configuration
	GitHubConfig      SkillsGitHubConfig `json:"github_config" yaml:"github_config"`           // GitHub configuration
	Retention         SkillsRetention   `json:"retention" yaml:"retention"`                     // Retention policy
}

// SkillsLoadConfig contains configuration for skill loading
type SkillsLoadConfig struct {
	Enabled              bool          `json:"enabled" yaml:"enabled"`
	SkillPaths           []string      `json:"skill_paths" yaml:"skill_paths"`                   // Paths to scan for skills
	Recursive            bool          `json:"recursive" yaml:"recursive"`                       // Scan directories recursively
	WatchChanges         bool          `json:"watch_changes" yaml:"watch_changes"`               // Watch for file changes
	MaxLoadErrors        int           `json:"max_load_errors" yaml:"max_load_errors"`           // Max errors before stopping
	ExcludedPatterns     []string      `json:"excluded_patterns" yaml:"excluded_patterns"`       // File patterns to exclude
	RequiredCapabilities []string      `json:"required_capabilities" yaml:"required_capabilities"` // Required capabilities
	ReloadInterval       time.Duration `json:"reload_interval" yaml:"reload_interval"`            // Interval for reloading skills
}

// SkillsGitHubConfig contains configuration for GitHub skill installation
type SkillsGitHubConfig struct {
	Enabled        bool     `json:"enabled" yaml:"enabled"`
	APIEndpoint    string   `json:"api_endpoint" yaml:"api_endpoint"`              // GitHub API endpoint
	MaxRepoSkills  int      `json:"max_repo_skills" yaml:"max_repo_skills"`        // Max skills per repo
	AllowedOrgs    []string `json:"allowed_orgs" yaml:"allowed_orgs"`              // Allowed GitHub organizations
	AllowedRepos   []string `json:"allowed_repos" yaml:"allowed_repos"`            // Allowed repositories (format: owner/repo)
	Token          string   `json:"token,omitempty" yaml:"token,omitempty"`        // GitHub auth token (from env)
	AutoUpdate     bool     `json:"auto_update" yaml:"auto_update"`                // Auto-update skills from GitHub
	UpdateInterval time.Duration `json:"update_interval" yaml:"update_interval"`   // Update check interval
}

// SkillsRetention contains retention policy for skills
type SkillsRetention struct {
	Enabled          bool          `json:"enabled" yaml:"enabled"`
	MaxAge           time.Duration `json:"max_age" yaml:"max_age"`                     // Maximum age before cleanup
	UnusedMaxAge     time.Duration `json:"unused_max_age" yaml:"unused_max_age"`       // Time unused before cleanup
	ErrorMaxAge      time.Duration `json:"error_max_age" yaml:"error_max_age"`         // Time in error state before cleanup
	MinLoadCount     int           `json:"min_load_count" yaml:"min_load_count"`       // Minimum loads to keep
	PreserveVerified bool          `json:"preserve_verified" yaml:"preserve_verified"` // Don't delete verified skills
}

// AuditConfig contains audit logging configuration
type AuditConfig struct {
	Enabled             bool   `json:"enabled" yaml:"enabled"`                          // Enable audit logging
	Output              string `json:"output" yaml:"output"`                            // stdout, stderr, syslog, or file path
	Format              string `json:"format" yaml:"format"`                            // json, text
	RotationEnabled     bool   `json:"rotation_enabled" yaml:"rotation_enabled"`        // Enable log rotation
	MaxSize             int    `json:"max_size" yaml:"max_size"`                        // Maximum size of audit log file in MB
	MaxBackups          int    `json:"max_backups" yaml:"max_backups"`                  // Maximum number of backup files to keep
	MaxAge              int    `json:"max_age" yaml:"max_age"`                          // Maximum age of audit log files in days
	Compress            bool   `json:"compress" yaml:"compress"`                        // Compress rotated log files
	IncludeSensitiveData bool  `json:"include_sensitive_data" yaml:"include_sensitive_data"` // Include sensitive data in audit logs
}

// applyDefaults fills in zero-valued config fields with their defaults
// This is called after loading from YAML to ensure partial configs have sensible defaults
// We apply defaults field-by-field to handle partial configurations properly
func applyDefaults(cfg *Config) {
	// Docker defaults - apply field-by-field to handle partial configs
	defaultDocker := DefaultDockerConfig()
	if cfg.Docker.Host == "" {
		cfg.Docker.Host = defaultDocker.Host
	}
	if cfg.Docker.APIVersion == "" {
		cfg.Docker.APIVersion = defaultDocker.APIVersion
	}
	if cfg.Docker.Timeout == 0 {
		cfg.Docker.Timeout = defaultDocker.Timeout
	}
	if cfg.Docker.MaxRetries == 0 {
		cfg.Docker.MaxRetries = defaultDocker.MaxRetries
	}
	if cfg.Docker.RetryDelay == 0 {
		cfg.Docker.RetryDelay = defaultDocker.RetryDelay
	}

	// APIServer defaults - apply field-by-field to handle partial configs
	defaultAPIServer := DefaultAPIServerConfig()
	if cfg.APIServer.Host == "" {
		cfg.APIServer.Host = defaultAPIServer.Host
	}
	if cfg.APIServer.Port == 0 {
		cfg.APIServer.Port = defaultAPIServer.Port
	}
	if cfg.APIServer.ReadTimeout == 0 {
		cfg.APIServer.ReadTimeout = defaultAPIServer.ReadTimeout
	}
	if cfg.APIServer.WriteTimeout == 0 {
		cfg.APIServer.WriteTimeout = defaultAPIServer.WriteTimeout
	}
	if cfg.APIServer.IdleTimeout == 0 {
		cfg.APIServer.IdleTimeout = defaultAPIServer.IdleTimeout
	}
	if cfg.APIServer.MaxConnections == 0 {
		cfg.APIServer.MaxConnections = defaultAPIServer.MaxConnections
	}

	// Logging defaults - apply field-by-field to handle partial configs
	defaultLogging := DefaultLoggingConfig()
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = defaultLogging.Level
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = defaultLogging.Format
	}
	if cfg.Logging.Output == "" {
		cfg.Logging.Output = defaultLogging.Output
	}
	if cfg.Logging.SyslogFacility == "" {
		cfg.Logging.SyslogFacility = defaultLogging.SyslogFacility
	}
	if cfg.Logging.MaxSize == 0 {
		cfg.Logging.MaxSize = defaultLogging.MaxSize
	}
	if cfg.Logging.MaxBackups == 0 {
		cfg.Logging.MaxBackups = defaultLogging.MaxBackups
	}
	if cfg.Logging.MaxAge == 0 {
		cfg.Logging.MaxAge = defaultLogging.MaxAge
	}

	// Session defaults - apply field-by-field to handle partial configs
	defaultSession := DefaultSessionConfig()
	if cfg.Session.Timeout == 0 {
		cfg.Session.Timeout = defaultSession.Timeout
	}
	if cfg.Session.MaxSessions == 0 {
		cfg.Session.MaxSessions = defaultSession.MaxSessions
	}
	if cfg.Session.CleanupInterval == 0 {
		cfg.Session.CleanupInterval = defaultSession.CleanupInterval
	}
	if cfg.Session.IdleTimeout == 0 {
		cfg.Session.IdleTimeout = defaultSession.IdleTimeout
	}
	if cfg.Session.StoragePath == "" {
		cfg.Session.StoragePath = defaultSession.StoragePath
	}

	// Event defaults - apply field-by-field to handle partial configs
	defaultEvent := DefaultEventConfig()
	if cfg.Event.QueueSize == 0 {
		cfg.Event.QueueSize = defaultEvent.QueueSize
	}
	if cfg.Event.Workers == 0 {
		cfg.Event.Workers = defaultEvent.Workers
	}
	if cfg.Event.BufferSize == 0 {
		cfg.Event.BufferSize = defaultEvent.BufferSize
	}
	if cfg.Event.Timeout == 0 {
		cfg.Event.Timeout = defaultEvent.Timeout
	}
	if cfg.Event.RetryAttempts == 0 {
		cfg.Event.RetryAttempts = defaultEvent.RetryAttempts
	}
	if cfg.Event.RetryDelay == 0 {
		cfg.Event.RetryDelay = defaultEvent.RetryDelay
	}

	// IPC defaults - apply field-by-field to handle partial configs
	defaultIPC := DefaultIPCConfig()
	if cfg.IPC.SocketPath == "" {
		cfg.IPC.SocketPath = defaultIPC.SocketPath
	}
	if cfg.IPC.BufferSize == 0 {
		cfg.IPC.BufferSize = defaultIPC.BufferSize
	}
	if cfg.IPC.Timeout == 0 {
		cfg.IPC.Timeout = defaultIPC.Timeout
	}
	if cfg.IPC.MaxConnections == 0 {
		cfg.IPC.MaxConnections = defaultIPC.MaxConnections
	}

	// Scheduler defaults - apply field-by-field to handle partial configs
	defaultScheduler := DefaultSchedulerConfig()
	if cfg.Scheduler.QueueSize == 0 {
		cfg.Scheduler.QueueSize = defaultScheduler.QueueSize
	}
	if cfg.Scheduler.Workers == 0 {
		cfg.Scheduler.Workers = defaultScheduler.Workers
	}
	if cfg.Scheduler.MaxRetries == 0 {
		cfg.Scheduler.MaxRetries = defaultScheduler.MaxRetries
	}
	if cfg.Scheduler.RetryDelay == 0 {
		cfg.Scheduler.RetryDelay = defaultScheduler.RetryDelay
	}
	if cfg.Scheduler.TaskTimeout == 0 {
		cfg.Scheduler.TaskTimeout = defaultScheduler.TaskTimeout
	}
	if cfg.Scheduler.QueueTimeout == 0 {
		cfg.Scheduler.QueueTimeout = defaultScheduler.QueueTimeout
	}

	// Credentials defaults - apply field-by-field to handle partial configs
	defaultCredentials := DefaultCredentialsConfig()
	if cfg.Credentials.StorePath == "" {
		cfg.Credentials.StorePath = defaultCredentials.StorePath
	}
	if cfg.Credentials.KeyRotationDays == 0 {
		cfg.Credentials.KeyRotationDays = defaultCredentials.KeyRotationDays
	}
	if cfg.Credentials.MaxCredentialAge == 0 {
		cfg.Credentials.MaxCredentialAge = defaultCredentials.MaxCredentialAge
	}

	// Policy defaults - apply field-by-field to handle partial configs
	defaultPolicy := DefaultPolicyConfig()
	if cfg.Policy.ConfigPath == "" {
		cfg.Policy.ConfigPath = defaultPolicy.ConfigPath
	}
	if cfg.Policy.ReloadInterval == 0 {
		cfg.Policy.ReloadInterval = defaultPolicy.ReloadInterval
	}
	if cfg.Policy.EnforcementMode == "" {
		cfg.Policy.EnforcementMode = defaultPolicy.EnforcementMode
	}
	if cfg.Policy.DefaultQuotaCPU == 0 {
		cfg.Policy.DefaultQuotaCPU = defaultPolicy.DefaultQuotaCPU
	}
	if cfg.Policy.DefaultQuotaMemory == 0 {
		cfg.Policy.DefaultQuotaMemory = defaultPolicy.DefaultQuotaMemory
	}

	// Metrics defaults - apply field-by-field to handle partial configs
	defaultMetrics := DefaultMetricsConfig()
	if cfg.Metrics.Port == 0 {
		cfg.Metrics.Port = defaultMetrics.Port
	}
	if cfg.Metrics.Path == "" {
		cfg.Metrics.Path = defaultMetrics.Path
	}
	if cfg.Metrics.ReportInterval == 0 {
		cfg.Metrics.ReportInterval = defaultMetrics.ReportInterval
	}

	// Tracing defaults - apply field-by-field to handle partial configs
	defaultTracing := DefaultTracingConfig()
	if cfg.Tracing.SampleRate == 0 {
		cfg.Tracing.SampleRate = defaultTracing.SampleRate
	}
	if cfg.Tracing.Exporter == "" {
		cfg.Tracing.Exporter = defaultTracing.Exporter
	}

	// Orchestrator defaults - apply field-by-field to handle partial configs
	defaultOrchestrator := DefaultOrchestratorConfig()
	if cfg.Orchestrator.ShutdownTimeout == 0 {
		cfg.Orchestrator.ShutdownTimeout = defaultOrchestrator.ShutdownTimeout
	}
	if cfg.Orchestrator.HealthCheckInterval == 0 {
		cfg.Orchestrator.HealthCheckInterval = defaultOrchestrator.HealthCheckInterval
	}
	if cfg.Orchestrator.GracefulStopTimeout == 0 {
		cfg.Orchestrator.GracefulStopTimeout = defaultOrchestrator.GracefulStopTimeout
	}
	if cfg.Orchestrator.ProfilingPort == 0 {
		cfg.Orchestrator.ProfilingPort = defaultOrchestrator.ProfilingPort
	}

	// Runtime defaults - apply field-by-field to handle partial configs
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

	// Memory defaults - apply field-by-field to handle partial configs
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

	// gRPC defaults - apply field-by-field to handle partial configs
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

	// LLM defaults - apply field-by-field to handle partial configs
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

	// Provider defaults - apply field-by-field to handle partial configs
	defaultProvider := DefaultProviderConfig()
	if cfg.Provider.DefaultProvider == "" {
		cfg.Provider.DefaultProvider = defaultProvider.DefaultProvider
	}
	if cfg.Provider.FailoverThreshold == 0 {
		cfg.Provider.FailoverThreshold = defaultProvider.FailoverThreshold
	}
	if cfg.Provider.HealthCheckInterval == 0 {
		cfg.Provider.HealthCheckInterval = defaultProvider.HealthCheckInterval
	}
	if cfg.Provider.CircuitBreakerTimeout == 0 {
		cfg.Provider.CircuitBreakerTimeout = defaultProvider.CircuitBreakerTimeout
	}
	// Apply provider-specific defaults
	if cfg.Provider.Anthropic.Timeout == 0 {
		cfg.Provider.Anthropic.Timeout = defaultProvider.Anthropic.Timeout
	}
	if cfg.Provider.Anthropic.MaxRetries == 0 {
		cfg.Provider.Anthropic.MaxRetries = defaultProvider.Anthropic.MaxRetries
	}
	if cfg.Provider.Anthropic.Priority == 0 {
		cfg.Provider.Anthropic.Priority = defaultProvider.Anthropic.Priority
	}
	if cfg.Provider.OpenAI.Timeout == 0 {
		cfg.Provider.OpenAI.Timeout = defaultProvider.OpenAI.Timeout
	}
	if cfg.Provider.OpenAI.MaxRetries == 0 {
		cfg.Provider.OpenAI.MaxRetries = defaultProvider.OpenAI.MaxRetries
	}
	if cfg.Provider.OpenAI.Priority == 0 {
		cfg.Provider.OpenAI.Priority = defaultProvider.OpenAI.Priority
	}

	// Skills defaults - NEW field, may not exist in older YAML files
	// Apply field-by-field to handle partial configs
	defaultSkills := DefaultSkillsConfig()
	if cfg.Skills.StoragePath == "" {
		cfg.Skills.StoragePath = defaultSkills.StoragePath
	}
	if cfg.Skills.MaxSkillsPerOwner == 0 {
		cfg.Skills.MaxSkillsPerOwner = defaultSkills.MaxSkillsPerOwner
	}
	if cfg.Skills.LoadConfig.ReloadInterval == 0 {
		cfg.Skills.LoadConfig.ReloadInterval = defaultSkills.LoadConfig.ReloadInterval
	}
	if cfg.Skills.LoadConfig.MaxLoadErrors == 0 {
		cfg.Skills.LoadConfig.MaxLoadErrors = defaultSkills.LoadConfig.MaxLoadErrors
	}
	if cfg.Skills.GitHubConfig.MaxRepoSkills == 0 {
		cfg.Skills.GitHubConfig.MaxRepoSkills = defaultSkills.GitHubConfig.MaxRepoSkills
	}
	if cfg.Skills.GitHubConfig.UpdateInterval == 0 {
		cfg.Skills.GitHubConfig.UpdateInterval = defaultSkills.GitHubConfig.UpdateInterval
	}
	if cfg.Skills.Retention.MaxAge == 0 {
		cfg.Skills.Retention.MaxAge = defaultSkills.Retention.MaxAge
	}
	if cfg.Skills.Retention.UnusedMaxAge == 0 {
		cfg.Skills.Retention.UnusedMaxAge = defaultSkills.Retention.UnusedMaxAge
	}
	if cfg.Skills.Retention.ErrorMaxAge == 0 {
		cfg.Skills.Retention.ErrorMaxAge = defaultSkills.Retention.ErrorMaxAge
  }
	// Audit defaults - NEW field, may not exist in older YAML files
	// Apply field-by-field to handle partial configs
	defaultAudit := DefaultAuditConfig()
	if cfg.Audit.Output == "" {
		cfg.Audit.Output = defaultAudit.Output
	}
	if cfg.Audit.Format == "" {
		cfg.Audit.Format = defaultAudit.Format
	}
	if cfg.Audit.MaxSize == 0 {
		cfg.Audit.MaxSize = defaultAudit.MaxSize
	}
	if cfg.Audit.MaxBackups == 0 {
		cfg.Audit.MaxBackups = defaultAudit.MaxBackups
	}
	if cfg.Audit.MaxAge == 0 {
		cfg.Audit.MaxAge = defaultAudit.MaxAge
	}
	// For completely missing audit sections in older configs, also apply
	// boolean defaults so behavior matches DefaultAuditConfig().
	// TODO: Consider using pointer bools for optional audit fields or adding an
	// explicit flag to detect missing configs more robustly. This check will break
	// if new non-boolean fields are added to the audit config.
	if cfg.Audit.Output == "" &&
		cfg.Audit.Format == "" &&
		cfg.Audit.MaxSize == 0 &&
		cfg.Audit.MaxBackups == 0 &&
		cfg.Audit.MaxAge == 0 {
		cfg.Audit.RotationEnabled = defaultAudit.RotationEnabled
		cfg.Audit.Compress = defaultAudit.Compress
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

	// Load Skills configuration
	if v := os.Getenv(EnvSkillsEnabled); v != "" {
		cfg.Skills.Enabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv(EnvSkillsStoragePath); v != "" {
		cfg.Skills.StoragePath = v
	}
	if v := os.Getenv(EnvSkillsAutoLoad); v != "" {
		cfg.Skills.AutoLoad = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv(EnvSkillsMaxPerOwner); v != "" {
		if max, err := strconv.Atoi(v); err == nil {
			cfg.Skills.MaxSkillsPerOwner = max
		}
	}
	if v := os.Getenv(EnvSkillsGitHubToken); v != "" {
		cfg.Skills.GitHubConfig.Token = v
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
			Provider:     DefaultProviderConfig(),
			Skills:       DefaultSkillsConfig(),
			Audit:        DefaultAuditConfig(),
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

	// Validate Provider configuration
	// Only validate if provider config is explicitly set (not all zero values)
	// This allows backward compatibility with configs that don't use provider features
	if c.Provider != (ProviderConfig{}) {
		validProviders := map[string]bool{
			"anthropic": true,
			"openai":    true,
		}
		if c.Provider.DefaultProvider != "" && !validProviders[c.Provider.DefaultProvider] {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("invalid default provider: %s (must be anthropic or openai)", c.Provider.DefaultProvider))
		}
		if c.Provider.FailoverEnabled && c.Provider.FailoverThreshold <= 0 {
			return types.NewError(types.ErrCodeInvalidArgument, "provider failover threshold must be positive when failover is enabled")
		}
		if c.Provider.HealthCheckInterval <= 0 {
			return types.NewError(types.ErrCodeInvalidArgument, "provider health check interval must be positive")
		}
		if c.Provider.CircuitBreakerTimeout <= 0 {
			return types.NewError(types.ErrCodeInvalidArgument, "provider circuit breaker timeout must be positive")
		}
	}

	// Validate Skills configuration
	if c.Skills.Enabled {
		if c.Skills.StoragePath == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "skills storage path cannot be empty when skills is enabled")
		}
		if c.Skills.MaxSkillsPerOwner < 0 {
			return types.NewError(types.ErrCodeInvalidArgument, "skills max skills per owner cannot be negative")
		}
		if c.Skills.LoadConfig.Enabled {
			if c.Skills.LoadConfig.MaxLoadErrors < 0 {
				return types.NewError(types.ErrCodeInvalidArgument, "skills max load errors cannot be negative")
			}
			if c.Skills.LoadConfig.ReloadInterval <= 0 {
				return types.NewError(types.ErrCodeInvalidArgument, "skills reload interval must be positive when load config is enabled")
			}
		}
		if c.Skills.GitHubConfig.Enabled {
			if c.Skills.GitHubConfig.MaxRepoSkills < 0 {
				return types.NewError(types.ErrCodeInvalidArgument, "skills github max repo skills cannot be negative")
			}
			if c.Skills.GitHubConfig.AutoUpdate && c.Skills.GitHubConfig.UpdateInterval <= 0 {
				return types.NewError(types.ErrCodeInvalidArgument, "skills github update interval must be positive when auto update is enabled")
			}
		}
		if c.Skills.Retention.Enabled {
			if c.Skills.Retention.MinLoadCount < 0 {
				return types.NewError(types.ErrCodeInvalidArgument, "skills retention min load count cannot be negative")
			}
    }
  }
	// Validate Audit configuration
	if c.Audit.Enabled {
		if c.Audit.Output == "" {
			return types.NewError(types.ErrCodeInvalidArgument, "audit output cannot be empty when audit is enabled")
		}
		validAuditFormats := map[string]bool{
			"json": true,
			"text": true,
		}
		if !validAuditFormats[c.Audit.Format] {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("invalid audit format: %s (must be json or text)", c.Audit.Format))
		}
		if c.Audit.MaxSize <= 0 {
			return types.NewError(types.ErrCodeInvalidArgument, "audit max size must be positive")
		}
		if c.Audit.MaxBackups < 0 {
			return types.NewError(types.ErrCodeInvalidArgument, "audit max backups cannot be negative")
		}
		if c.Audit.MaxAge <= 0 {
			return types.NewError(types.ErrCodeInvalidArgument, "audit max age must be positive")
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
	return fmt.Sprintf("Config{Docker: %s, API: %s, Logging: %s, Session: %s, Event: %s, IPC: %s, Scheduler: %s, Credentials: %s, Policy: %s, Metrics: %s, Tracing: %s, Orchestrator: %s, Runtime: %s, Memory: %s, GRPC: %s, LLM: %s, Provider: %s, Skills: %s, Audit: %s}",
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
		c.Provider.String(),
		c.Skills.String(),
		c.Audit.String(),
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
	return fmt.Sprintf("ProviderConfig{DefaultProvider: %s, FailoverEnabled: %v, Anthropic: {Enabled: %v, Priority: %d}, OpenAI: {Enabled: %v, Priority: %d}}",
		c.DefaultProvider, c.FailoverEnabled, c.Anthropic.Enabled, c.Anthropic.Priority, c.OpenAI.Enabled, c.OpenAI.Priority)
}

func (c SkillsConfig) String() string {
	return fmt.Sprintf("SkillsConfig{Enabled: %v, StoragePath: %s, AutoLoad: %v, MaxSkillsPerOwner: %d}",
		c.Enabled, c.StoragePath, c.AutoLoad, c.MaxSkillsPerOwner)
}

func (c SkillsLoadConfig) String() string {
	return fmt.Sprintf("SkillsLoadConfig{Enabled: %v, Recursive: %v, WatchChanges: %v, MaxLoadErrors: %d}",
		c.Enabled, c.Recursive, c.WatchChanges, c.MaxLoadErrors)
}

func (c SkillsGitHubConfig) String() string {
	return fmt.Sprintf("SkillsGitHubConfig{Enabled: %v, AutoUpdate: %v, MaxRepoSkills: %d}",
		c.Enabled, c.AutoUpdate, c.MaxRepoSkills)
}

func (c SkillsRetention) String() string {
	return fmt.Sprintf("SkillsRetention{Enabled: %v, MaxAge: %s, PreserveVerified: %v}",
		c.Enabled, c.MaxAge, c.PreserveVerified)
}

func (c AuditConfig) String() string {
	return fmt.Sprintf("AuditConfig{Enabled: %v, Output: %s, Format: %s, RotationEnabled: %v}",
		c.Enabled, c.Output, c.Format, c.RotationEnabled)
}
