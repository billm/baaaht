package provider

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

const (
	// Default provider settings
	DefaultProviderTimeout    = 60 // seconds
	DefaultProviderMaxRetries = 3
	DefaultProviderEnabled    = true
	DefaultProviderPriority   = 100

	// Default Anthropic settings
	DefaultAnthropicBaseURL = "https://api.anthropic.com"
	DefaultAnthropicModel   = ModelClaude3_5Sonnet

	// Default OpenAI settings
	DefaultOpenAIBaseURL = "https://api.openai.com"
	DefaultOpenAIModel   = ModelGPT4o
)

// RegistryConfig represents the provider configuration for the orchestrator
type RegistryConfig struct {
	// DefaultProvider specifies the default provider to use
	DefaultProvider Provider `json:"default_provider" yaml:"default_provider"`

	// Providers is a map of provider configurations keyed by provider name
	Providers map[Provider]ProviderConfig `json:"providers" yaml:"providers"`

	// FailoverEnabled specifies whether automatic failover is enabled
	FailoverEnabled bool `json:"failover_enabled" yaml:"failover_enabled"`

	// FailoverThreshold specifies the number of consecutive failures before failover
	FailoverThreshold int `json:"failover_threshold" yaml:"failover_threshold"`

	// HealthCheckInterval specifies how often to check provider health
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`

	// CircuitBreakerTimeout specifies how long to wait before rechecking a failed provider
	CircuitBreakerTimeout time.Duration `json:"circuit_breaker_timeout" yaml:"circuit_breaker_timeout"`
}

// DefaultConfig returns the default provider configuration
func DefaultConfig() RegistryConfig {
	providers := make(map[Provider]ProviderConfig)
	providers[ProviderAnthropic] = DefaultAnthropicConfig()
	providers[ProviderOpenAI] = DefaultOpenAIConfig()

	return RegistryConfig{
		DefaultProvider:      ProviderAnthropic,
		Providers:            providers,
		FailoverEnabled:      true,
		FailoverThreshold:    3,
		HealthCheckInterval:  30 * time.Second,
		CircuitBreakerTimeout: 60 * time.Second,
	}
}

// DefaultAnthropicConfig returns the default Anthropic provider configuration
func DefaultAnthropicConfig() ProviderConfig {
	return ProviderConfig{
		Provider:   ProviderAnthropic,
		BaseURL:    DefaultAnthropicBaseURL,
		Timeout:    DefaultProviderTimeout,
		MaxRetries: DefaultProviderMaxRetries,
		Models: []Model{
			ModelClaude3_5Sonnet,
			ModelClaude3_5SonnetNew,
			ModelClaude3Opus,
			ModelClaude3Sonnet,
			ModelClaude3Haiku,
		},
		Enabled:  DefaultProviderEnabled,
		Priority: 10, // Higher priority (lower number)
		Metadata: map[string]interface{}{
			"name":        "Anthropic",
			"description": "Anthropic Claude API",
			"supports_prompt_caching": true,
			"supports_function_calling": true,
			"supports_vision": true,
		},
	}
}

// DefaultOpenAIConfig returns the default OpenAI provider configuration
func DefaultOpenAIConfig() ProviderConfig {
	return ProviderConfig{
		Provider:   ProviderOpenAI,
		BaseURL:    DefaultOpenAIBaseURL,
		Timeout:    DefaultProviderTimeout,
		MaxRetries: DefaultProviderMaxRetries,
		Models: []Model{
			ModelGPT4o,
			ModelGPT4oMini,
			ModelGPT4Turbo,
			ModelGPT4,
			ModelGPT35Turbo,
		},
		Enabled:  DefaultProviderEnabled,
		Priority: 20, // Lower priority than Anthropic
		Metadata: map[string]interface{}{
			"name":        "OpenAI",
			"description": "OpenAI GPT API",
			"supports_function_calling": true,
			"supports_vision": true,
			"supports_json_mode": true,
		},
	}
}

// Validate validates the provider configuration
func (c *RegistryConfig) Validate() error {
	// Validate default provider
	if c.DefaultProvider == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "default provider cannot be empty")
	}
	if !c.DefaultProvider.IsValid() {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid default provider: %s", c.DefaultProvider))
	}

	// Validate provider configurations
	for provider, cfg := range c.Providers {
		if !provider.IsValid() {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("invalid provider: %s", provider))
		}
		if cfg.Provider != provider {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("provider config key mismatch: %s != %s", provider, cfg.Provider))
		}
		if cfg.BaseURL == "" {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("provider %s: base URL cannot be empty", provider))
		}
		if cfg.Timeout <= 0 {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("provider %s: timeout must be positive", provider))
		}
		if cfg.MaxRetries < 0 {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("provider %s: max retries cannot be negative", provider))
		}
		if len(cfg.Models) == 0 {
			return types.NewError(types.ErrCodeInvalidArgument,
				fmt.Sprintf("provider %s: must have at least one model", provider))
		}
	}

	// Validate default provider exists
	if _, exists := c.Providers[c.DefaultProvider]; !exists {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("default provider %s is not configured", c.DefaultProvider))
	}

	// Validate failover settings
	if c.FailoverEnabled && c.FailoverThreshold <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "failover threshold must be positive when failover is enabled")
	}

	// Validate health check settings
	if c.HealthCheckInterval <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "health check interval must be positive")
	}

	// Validate circuit breaker settings
	if c.CircuitBreakerTimeout <= 0 {
		return types.NewError(types.ErrCodeInvalidArgument, "circuit breaker timeout must be positive")
	}

	return nil
}

// ApplyDefaults applies defaults to any missing fields in the provider configuration
func (c *RegistryConfig) ApplyDefaults() {
	// If providers map is empty, initialize with defaults
	if len(c.Providers) == 0 {
		defaultCfg := DefaultConfig()
		c.Providers = defaultCfg.Providers
	}

	// Apply defaults for Anthropic if not present
	if _, exists := c.Providers[ProviderAnthropic]; !exists {
		c.Providers[ProviderAnthropic] = DefaultAnthropicConfig()
	}

	// Apply defaults for OpenAI if not present
	if _, exists := c.Providers[ProviderOpenAI]; !exists {
		c.Providers[ProviderOpenAI] = DefaultOpenAIConfig()
	}

	// Apply field-by-field defaults for each provider
	for provider, cfg := range c.Providers {
		if cfg.Timeout == 0 {
			cfg.Timeout = DefaultProviderTimeout
		}
		if cfg.MaxRetries == 0 {
			cfg.MaxRetries = DefaultProviderMaxRetries
		}
		if cfg.Priority == 0 {
			cfg.Priority = DefaultProviderPriority
		}
		if cfg.Metadata == nil {
			cfg.Metadata = make(map[string]interface{})
		}
		c.Providers[provider] = cfg
	}

	// Apply defaults for failover settings
	if c.FailoverThreshold == 0 {
		c.FailoverThreshold = 3
	}
	if c.HealthCheckInterval == 0 {
		c.HealthCheckInterval = 30 * time.Second
	}
	if c.CircuitBreakerTimeout == 0 {
		c.CircuitBreakerTimeout = 60 * time.Second
	}
}

// String returns a string representation of the configuration
func (c *RegistryConfig) String() string {
	enabledProviders := []string{}
	for _, cfg := range c.Providers {
		if cfg.Enabled {
			enabledProviders = append(enabledProviders, cfg.Provider.String())
		}
	}
	return fmt.Sprintf("ProviderConfig{DefaultProvider: %s, EnabledProviders: [%s], FailoverEnabled: %v}",
		c.DefaultProvider, strings.Join(enabledProviders, ", "), c.FailoverEnabled)
}

// GetProviderConfig returns the configuration for a specific provider
func (c *RegistryConfig) GetProviderConfig(provider Provider) (ProviderConfig, error) {
	cfg, exists := c.Providers[provider]
	if !exists {
		return ProviderConfig{}, NewProviderError(ErrCodeProviderNotFound,
			fmt.Sprintf("provider %s not found in configuration", provider))
	}
	return cfg, nil
}

// GetEnabledProviders returns all enabled providers sorted by priority
func (c *RegistryConfig) GetEnabledProviders() []ProviderConfig {
	providers := make([]ProviderConfig, 0)
	for _, cfg := range c.Providers {
		if cfg.Enabled {
			providers = append(providers, cfg)
		}
	}
	// Sort by priority (lower number = higher priority)
	for i := 0; i < len(providers); i++ {
		for j := i + 1; j < len(providers); j++ {
			if providers[j].Priority < providers[i].Priority {
				providers[i], providers[j] = providers[j], providers[i]
			}
		}
	}
	return providers
}

// LoadFromEnv loads provider configuration from environment variables
func (c *RegistryConfig) LoadFromEnv() {
	// Default provider
	if v := os.Getenv("PROVIDER_DEFAULT"); v != "" {
		c.DefaultProvider = Provider(v)
	}

	// Failover settings
	if v := os.Getenv("PROVIDER_FAILOVER_ENABLED"); v != "" {
		c.FailoverEnabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("PROVIDER_FAILOVER_THRESHOLD"); v != "" {
		if threshold, err := parseNonNegativeInt(v); err == nil {
			c.FailoverThreshold = threshold
		}
	}

	// Anthropic settings
	if anthropicCfg, exists := c.Providers[ProviderAnthropic]; exists {
		if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
			anthropicCfg.APIKey = v
		}
		if v := os.Getenv("ANTHROPIC_BASE_URL"); v != "" {
			anthropicCfg.BaseURL = v
		}
		if v := os.Getenv("ANTHROPIC_TIMEOUT"); v != "" {
			if timeout, err := parseNonNegativeInt(v); err == nil {
				anthropicCfg.Timeout = timeout
			}
		}
		if v := os.Getenv("ANTHROPIC_MAX_RETRIES"); v != "" {
			if retries, err := parseNonNegativeInt(v); err == nil {
				anthropicCfg.MaxRetries = retries
			}
		}
		if v := os.Getenv("ANTHROPIC_ENABLED"); v != "" {
			anthropicCfg.Enabled = strings.ToLower(v) == "true" || v == "1"
		}
		c.Providers[ProviderAnthropic] = anthropicCfg
	}

	// OpenAI settings
	if openaiCfg, exists := c.Providers[ProviderOpenAI]; exists {
		if v := os.Getenv("OPENAI_API_KEY"); v != "" {
			openaiCfg.APIKey = v
		}
		if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
			openaiCfg.BaseURL = v
		}
		if v := os.Getenv("OPENAI_TIMEOUT"); v != "" {
			if timeout, err := parseNonNegativeInt(v); err == nil {
				openaiCfg.Timeout = timeout
			}
		}
		if v := os.Getenv("OPENAI_MAX_RETRIES"); v != "" {
			if retries, err := parseNonNegativeInt(v); err == nil {
				openaiCfg.MaxRetries = retries
			}
		}
		if v := os.Getenv("OPENAI_ENABLED"); v != "" {
			openaiCfg.Enabled = strings.ToLower(v) == "true" || v == "1"
		}
		c.Providers[ProviderOpenAI] = openaiCfg
	}
}

// LoadAPIKeysFromCredentials loads API keys from the credentials store
func (c *RegistryConfig) LoadAPIKeysFromCredentials(credStorePath string) error {
	// This will be implemented when the credentials integration is complete
	// For now, API keys can be set via environment variables
	return nil
}

// parseNonNegativeInt parses a non-negative integer from a string
func parseNonNegativeInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	if err != nil {
		return 0, err
	}
	if i < 0 {
		return 0, fmt.Errorf("value must be non-negative")
	}
	return i, nil
}

// ApplyOverrides applies configuration overrides from the main config system
// This allows provider settings to be overridden from the main config file
func (c *RegistryConfig) ApplyOverrides(opts config.ProviderOverrideOptions) error {
	// Apply provider-specific overrides
	for providerKey, override := range opts.Providers {
		provider := Provider(providerKey)
		if cfg, exists := c.Providers[provider]; exists {
			if override.Enabled != nil {
				cfg.Enabled = *override.Enabled
			}
			if override.BaseURL != "" {
				cfg.BaseURL = override.BaseURL
			}
			if override.Timeout != "" {
				if timeout, err := parseNonNegativeInt(override.Timeout); err == nil {
					cfg.Timeout = timeout
				}
			}
			if override.MaxRetries != nil {
				cfg.MaxRetries = *override.MaxRetries
			}
			if override.APIKey != "" {
				cfg.APIKey = override.APIKey
			}
			c.Providers[provider] = cfg
		}
	}

	// Apply global settings
	if opts.DefaultProvider != "" {
		c.DefaultProvider = Provider(opts.DefaultProvider)
	}
	if opts.FailoverEnabled != nil {
		c.FailoverEnabled = *opts.FailoverEnabled
	}
	if opts.FailoverThreshold != nil {
		c.FailoverThreshold = *opts.FailoverThreshold
	}

	return nil
}
