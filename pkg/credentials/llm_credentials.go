package credentials

import (
	"context"
	"fmt"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// llmProviderEnvVar maps provider names to their environment variable names
var llmProviderEnvVar = map[string]string{
	LLMProviderAnthropic:  "ANTHROPIC_API_KEY",
	LLMProviderOpenAI:     "OPENAI_API_KEY",
	LLMProviderOpenRouter: "OPENROUTER_API_KEY",
	LLMProviderOllama:     "", // Ollama typically runs locally without API keys
	LLMProviderLMStudio:   "", // LM Studio typically runs locally without API keys
}

// LLMCredentialManager provides LLM-specific credential management helpers
type LLMCredentialManager struct {
	store *Store
	cfg   config.LLMConfig
	logger *logger.Logger
}

// NewLLMCredentialManager creates a new LLM credential manager
func NewLLMCredentialManager(store *Store, cfg config.LLMConfig, log *logger.Logger) *LLMCredentialManager {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			// If we can't create a default logger, create one with basic stderr output
			// This ensures the manager is always safe to use
			log, _ = logger.New(config.LoggingConfig{
				Level:  "info",
				Output: "stderr",
				Format: "text",
			})
		}
	}
	return &LLMCredentialManager{
		store:  store,
		cfg:    cfg,
		logger: log.With("component", "llm_credential_manager"),
	}
}

// InjectLLMCredentials adds LLM API keys to the environment variable list
// It takes an existing environment slice and returns a new slice with LLM credentials added
func (m *LLMCredentialManager) InjectLLMCredentials(env []string) []string {
	ctx := context.Background()

	// Start with the existing environment
	result := make([]string, 0, len(env)+len(m.cfg.Providers))
	result = append(result, env...)

	// Get all providers that have stored credentials
	providers, err := m.store.ListLLMProviders(ctx)
	if err != nil {
		m.logger.Warn("Failed to list LLM providers for credential injection", "error", err)
		return result
	}

	// For each stored provider, inject the API key
	for _, provider := range providers {
		envVar, ok := llmProviderEnvVar[provider]
		if !ok || envVar == "" {
			// Provider doesn't use environment variable for credentials
			continue
		}

		// Get the API key from the store
		apiKey, err := m.store.GetLLMCredential(ctx, provider)
		if err != nil {
			m.logger.Warn("Failed to retrieve LLM credential for injection", "provider", provider, "error", err)
			continue
		}

		// Add to environment
		result = append(result, fmt.Sprintf("%s=%s", envVar, apiKey))
		m.logger.Debug("Injected LLM credential into environment", "provider", provider, "env_var", envVar)
	}

	m.logger.Info("Injected LLM credentials into environment", "count", len(providers))
	return result
}

// ValidateLLMCredentials checks that required LLM credentials exist
// Returns an error if any enabled provider is missing credentials
func (m *LLMCredentialManager) ValidateLLMCredentials() error {
	ctx := context.Background()
	var missingProviders []string

	// Check each enabled provider
	for name, providerCfg := range m.cfg.Providers {
		if !providerCfg.Enabled {
			continue
		}

		// Check if provider requires an API key
		envVar, requiresKey := llmProviderEnvVar[name]
		if !requiresKey || envVar == "" {
			// Provider doesn't use API keys (e.g., local LLMs)
			continue
		}

		// Check if credential exists in store
		hasCredential, err := m.store.HasLLMCredential(ctx, name)
		if err != nil {
			return types.WrapError(types.ErrCodeInternal, fmt.Sprintf("failed to check credential for provider: %s", name), err)
		}

		if !hasCredential {
			missingProviders = append(missingProviders, name)
		}
	}

	if len(missingProviders) > 0 {
		return types.NewError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("missing LLM credentials for enabled providers: %v", missingProviders))
	}

	m.logger.Info("LLM credentials validated", "enabled_providers", len(m.cfg.Providers))
	return nil
}

// GetProviderConfig retrieves the full configuration for a provider including credentials
// Returns a LLMProviderConfig with the APIKey field populated from the credential store
func (m *LLMCredentialManager) GetProviderConfig(provider string) (config.LLMProviderConfig, error) {
	ctx := context.Background()

	// Get provider config from LLM config
	providerCfg, exists := m.cfg.Providers[provider]
	if !exists {
		return config.LLMProviderConfig{}, types.NewError(types.ErrCodeNotFound,
			fmt.Sprintf("provider not configured: %s", provider))
	}

	// Check if provider requires an API key
	envVar, requiresKey := llmProviderEnvVar[provider]
	if requiresKey && envVar != "" {
		// Get API key from credential store
		apiKey, err := m.store.GetLLMCredential(ctx, provider)
		if err != nil {
			if types.IsErrCode(err, types.ErrCodeNotFound) {
				return config.LLMProviderConfig{}, types.NewError(types.ErrCodeNotFound,
					fmt.Sprintf("credential not found for provider: %s", provider))
			}
			return config.LLMProviderConfig{}, types.WrapError(types.ErrCodeInternal,
				fmt.Sprintf("failed to get credential for provider: %s", provider), err)
		}
		providerCfg.APIKey = apiKey
	}

	return providerCfg, nil
}

// ListConfiguredProviders returns a list of provider names that are both configured and have credentials
func (m *LLMCredentialManager) ListConfiguredProviders() ([]string, error) {
	ctx := context.Background()
	var configuredProviders []string

	// Get all providers that have stored credentials
	storedProviders, err := m.store.ListLLMProviders(ctx)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to list stored LLM providers", err)
	}

	// Filter to only include providers that are also enabled in config
	for _, provider := range storedProviders {
		if providerCfg, exists := m.cfg.Providers[provider]; exists && providerCfg.Enabled {
			configuredProviders = append(configuredProviders, provider)
		}
	}

	return configuredProviders, nil
}

// EnsureDefaultProvider checks that the default provider has credentials configured
// Returns an error if the default provider is missing credentials
func (m *LLMCredentialManager) EnsureDefaultProvider() error {
	if m.cfg.DefaultProvider == "" {
		return types.NewError(types.ErrCodeInvalidArgument, "no default provider configured")
	}

	_, err := m.GetProviderConfig(m.cfg.DefaultProvider)
	if err != nil {
		return types.WrapError(types.ErrCodeInvalidArgument,
			fmt.Sprintf("default provider %s is not properly configured", m.cfg.DefaultProvider), err)
	}

	m.logger.Info("Default provider verified", "provider", m.cfg.DefaultProvider)
	return nil
}

// GetCredentialEnvVar returns the environment variable name for a provider's API key
func GetCredentialEnvVar(provider string) (string, bool) {
	envVar, ok := llmProviderEnvVar[provider]
	return envVar, ok && envVar != ""
}
