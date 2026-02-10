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
	EnvMemoryStoragePath = "MEMORY_STORAGE_PATH"
	EnvMemoryEnabled     = "MEMORY_ENABLED"
	EnvMemoryMaxFileSize = "MEMORY_MAX_FILE_SIZE"
	EnvMemoryFileFormat  = "MEMORY_FILE_FORMAT"
	EnvSessionPersistence = "SESSION_PERSISTENCE"
	EnvSessionStoragePath = "SESSION_STORAGE_PATH"
	EnvGRPCSocketPath    = "GRPC_SOCKET_PATH"
	EnvGRPCMaxRecvMsgSize = "GRPC_MAX_RECV_MSG_SIZE"
	EnvGRPCMaxSendMsgSize = "GRPC_MAX_SEND_MSG_SIZE"
	EnvGRPCTimeout       = "GRPC_TIMEOUT"
	EnvGRPCMaxConnections = "GRPC_MAX_CONNECTIONS"
	EnvLLMEnabled        = "LLM_ENABLED"
	EnvLLMContainerImage = "LLM_CONTAINER_IMAGE"
	EnvLLMDefaultModel   = "LLM_DEFAULT_MODEL"
	EnvLLMDefaultProvider = "LLM_DEFAULT_PROVIDER"
	EnvLLMTimeout        = "LLM_TIMEOUT"
	EnvLLMMaxConcurrent  = "LLM_MAX_CONCURRENT_REQUESTS"
	EnvProviderDefaultProvider = "PROVIDER_DEFAULT"
	EnvProviderFailoverEnabled = "PROVIDER_FAILOVER_ENABLED"
	EnvProviderFailoverThreshold = "PROVIDER_FAILOVER_THRESHOLD"
	EnvAnthropicAPIKey    = "ANTHROPIC_API_KEY"
	EnvAnthropicBaseURL   = "ANTHROPIC_BASE_URL"
	EnvAnthropicTimeout   = "ANTHROPIC_TIMEOUT"
	EnvAnthropicMaxRetries = "ANTHROPIC_MAX_RETRIES"
	EnvAnthropicEnabled   = "ANTHROPIC_ENABLED"
	EnvOpenAIAPIKey       = "OPENAI_API_KEY"
	EnvOpenAIBaseURL      = "OPENAI_BASE_URL"
	EnvOpenAITimeout      = "OPENAI_TIMEOUT"
	EnvOpenAIMaxRetries   = "OPENAI_MAX_RETRIES"
	EnvOpenAIEnabled      = "OPENAI_ENABLED"
	EnvSkillsEnabled      = "SKILLS_ENABLED"
	EnvSkillsStoragePath  = "SKILLS_STORAGE_PATH"
	EnvSkillsAutoLoad     = "SKILLS_AUTO_LOAD"
	EnvSkillsMaxPerOwner  = "SKILLS_MAX_PER_OWNER"
	EnvSkillsGitHubToken  = "SKILLS_GITHUB_TOKEN"
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

	// Default Memory settings
	DefaultMemoryEnabled    = true
	DefaultMemoryMaxFileSize = 100 // KB
	DefaultMemoryFileFormat = "markdown"

	// Default gRPC settings
	DefaultGRPCSocketPath     = "/tmp/baaaht-grpc.sock"
	DefaultGRPCMaxRecvMsgSize = 100 * 1024 * 1024 // 100 MB
	DefaultGRPCMaxSendMsgSize = 100 * 1024 * 1024 // 100 MB
	DefaultGRPCTimeout        = 30 * time.Second
	DefaultGRPCMaxConnections = 100

	// Default LLM settings
	DefaultLLMEnabled            = false
	DefaultLLMContainerImage     = "baaaht/llm-gateway:latest"
	DefaultLLMDefaultProvider    = "anthropic"
	DefaultLLMDefaultModel       = "anthropic/claude-sonnet-4-20250514"
	DefaultLLMTimeout            = 120 * time.Second
	DefaultLLMMaxConcurrent      = 10
	// Default Provider settings
	DefaultProviderDefaultProvider = "anthropic"
	DefaultProviderFailoverEnabled = true
	DefaultProviderFailoverThreshold = 3
	DefaultProviderHealthCheckInterval = 30 * time.Second
	DefaultProviderCircuitBreakerTimeout = 60 * time.Second

	// Default Anthropic settings
	DefaultAnthropicBaseURL   = "https://api.anthropic.com"
	DefaultAnthropicTimeout   = 60 // seconds
	DefaultAnthropicMaxRetries = 3
	DefaultAnthropicEnabled   = true
	DefaultAnthropicPriority  = 10

	// Default OpenAI settings
	DefaultOpenAIBaseURL      = "https://api.openai.com"
	DefaultOpenAITimeout      = 60 // seconds
	DefaultOpenAIMaxRetries   = 3
	DefaultOpenAIEnabled      = true
	DefaultOpenAIPriority     = 20

	// Default Skills settings
	DefaultSkillsEnabled      = true
	DefaultSkillsMaxPerOwner  = 100
	DefaultSkillsAutoLoad     = true
	DefaultSkillsMaxLoadErrors = 10
	DefaultSkillsReloadInterval = 5 * time.Minute
	DefaultSkillsGitHubAPIEndpoint = "https://api.github.com"
	DefaultSkillsGitHubMaxRepoSkills = 50
	DefaultSkillsGitHubUpdateInterval = 24 * time.Hour
	DefaultSkillsRetentionMaxAge = 90 * 24 * time.Hour // 90 days
	DefaultSkillsRetentionUnusedMaxAge = 30 * 24 * time.Hour // 30 days
	DefaultSkillsRetentionErrorMaxAge = 7 * 24 * time.Hour // 7 days
	DefaultSkillsRetentionMinLoadCount = 0
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

// DefaultMemoryConfig returns the default memory configuration
func DefaultMemoryConfig() MemoryConfig {
	storagePath := "/var/lib/baaaht/memory" // fallback
	if configDir, err := GetConfigDir(); err == nil {
		storagePath = filepath.Join(configDir, "memory")
	}
	return MemoryConfig{
		StoragePath:     storagePath,
		UserMemoryPath:  filepath.Join(storagePath, "users"),
		GroupMemoryPath: filepath.Join(storagePath, "groups"),
		Enabled:         DefaultMemoryEnabled,
		MaxFileSize:     DefaultMemoryMaxFileSize, // KB
		FileFormat:      DefaultMemoryFileFormat,
	}
}

// DefaultGRPCConfig returns the default gRPC server configuration
func DefaultGRPCConfig() GRPCConfig {
	return GRPCConfig{
		SocketPath:     DefaultGRPCSocketPath,
		MaxRecvMsgSize: DefaultGRPCMaxRecvMsgSize,
		MaxSendMsgSize: DefaultGRPCMaxSendMsgSize,
		Timeout:        DefaultGRPCTimeout,
		MaxConnections: DefaultGRPCMaxConnections,
	}
}

// DefaultProviderConfig returns the default provider configuration
func DefaultProviderConfig() ProviderConfig {
	return ProviderConfig{
		DefaultProvider:        DefaultProviderDefaultProvider,
		FailoverEnabled:        DefaultProviderFailoverEnabled,
		FailoverThreshold:      DefaultProviderFailoverThreshold,
		HealthCheckInterval:    DefaultProviderHealthCheckInterval,
		CircuitBreakerTimeout:  DefaultProviderCircuitBreakerTimeout,
		Anthropic: ProviderSpecificConfig{
			Enabled:    DefaultAnthropicEnabled,
			BaseURL:    DefaultAnthropicBaseURL,
			Timeout:    DefaultAnthropicTimeout,
			MaxRetries: DefaultAnthropicMaxRetries,
			Priority:   DefaultAnthropicPriority,
		},
		OpenAI: ProviderSpecificConfig{
			Enabled:    DefaultOpenAIEnabled,
			BaseURL:    DefaultOpenAIBaseURL,
			Timeout:    DefaultOpenAITimeout,
			MaxRetries: DefaultOpenAIMaxRetries,
			Priority:   DefaultOpenAIPriority,
		},
	}
}

// DefaultLLMConfig returns the default LLM Gateway configuration
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		Enabled:               DefaultLLMEnabled,
		ContainerImage:        DefaultLLMContainerImage,
		Providers: map[string]LLMProviderConfig{
			"anthropic": {
				Name:    "anthropic",
				APIKey:  "", // Loaded from ANTHROPIC_API_KEY environment variable
				BaseURL: "https://api.anthropic.com",
				Enabled: true,
				Models:  []string{"anthropic/claude-sonnet-4-20250514", "anthropic/claude-3-5-sonnet-20241022"},
			},
			"openai": {
				Name:    "openai",
				APIKey:  "", // Loaded from OPENAI_API_KEY environment variable
				BaseURL: "https://api.openai.com/v1",
				Enabled: true,
				Models:  []string{"openai/gpt-4o", "openai/gpt-4o-mini", "openai/gpt-4-turbo"},
			},
		},
		DefaultModel:          DefaultLLMDefaultModel,
		DefaultProvider:       DefaultLLMDefaultProvider,
		Timeout:               DefaultLLMTimeout,
		MaxConcurrentRequests: DefaultLLMMaxConcurrent,
		RateLimits: map[string]int{
			"anthropic": 60,  // 60 requests per minute
			"openai":    60,  // 60 requests per minute
		},
		FallbackChains: map[string][]string{
			"anthropic/*": {"openai"},
			"openai/*":    {"anthropic"},
		},
	}
}

// DefaultSkillsConfig returns the default skills configuration
func DefaultSkillsConfig() SkillsConfig {
	storagePath := "/var/lib/baaaht/skills" // fallback
	if configDir, err := GetConfigDir(); err == nil {
		storagePath = filepath.Join(configDir, "skills")
	}
	return SkillsConfig{
		Enabled:           DefaultSkillsEnabled,
		StoragePath:       storagePath,
		MaxSkillsPerOwner: DefaultSkillsMaxPerOwner,
		AutoLoad:          DefaultSkillsAutoLoad,
		LoadConfig: SkillsLoadConfig{
			Enabled:          DefaultSkillsAutoLoad,
			SkillPaths:       []string{storagePath},
			Recursive:        true,
			WatchChanges:     false,
			MaxLoadErrors:    DefaultSkillsMaxLoadErrors,
			ExcludedPatterns: []string{".git", ".github", "node_modules", "vendor"},
			ReloadInterval:   DefaultSkillsReloadInterval,
		},
		GitHubConfig: SkillsGitHubConfig{
			Enabled:        false,
			APIEndpoint:    DefaultSkillsGitHubAPIEndpoint,
			MaxRepoSkills:  DefaultSkillsGitHubMaxRepoSkills,
			AllowedOrgs:    []string{},
			AllowedRepos:   []string{},
			Token:          "", // Loaded from SKILLS_GITHUB_TOKEN environment variable
			AutoUpdate:     false,
			UpdateInterval: DefaultSkillsGitHubUpdateInterval,
		},
		Retention: SkillsRetention{
			Enabled:          false,
			MaxAge:           DefaultSkillsRetentionMaxAge,
			UnusedMaxAge:     DefaultSkillsRetentionUnusedMaxAge,
			ErrorMaxAge:      DefaultSkillsRetentionErrorMaxAge,
			MinLoadCount:     DefaultSkillsRetentionMinLoadCount,
			PreserveVerified: true,
		},
	}
}
