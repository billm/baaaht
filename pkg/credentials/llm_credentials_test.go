package credentials

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// createTestLLMCredentialManager creates a test LLM credential manager
func createTestLLMCredentialManager(t *testing.T) (*LLMCredentialManager, *Store) {
	t.Helper()

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Create a temporary directory for test credentials
	tmpDir := t.TempDir()
	cfg := config.DefaultCredentialsConfig()
	cfg.StorePath = filepath.Join(tmpDir, "credentials.json")

	store, err := NewStore(cfg, log)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// Create LLM config with test providers
	llmCfg := config.LLMConfig{
		Enabled:       true,
		DefaultModel:  "anthropic/claude-sonnet-4-20250514",
		DefaultProvider: LLMProviderAnthropic,
		Providers: map[string]config.ProviderConfig{
			LLMProviderAnthropic: {
				Name:    LLMProviderAnthropic,
				BaseURL: "https://api.anthropic.com",
				Enabled: true,
				Models:  []string{"anthropic/claude-sonnet-4-20250514"},
			},
			LLMProviderOpenAI: {
				Name:    LLMProviderOpenAI,
				BaseURL: "https://api.openai.com/v1",
				Enabled: true,
				Models:  []string{"openai/gpt-4o"},
			},
			LLMProviderOllama: {
				Name:    LLMProviderOllama,
				BaseURL: "http://localhost:11434",
				Enabled: true,
				Models:  []string{"ollama/llama3"},
			},
		},
	}

	manager := NewLLMCredentialManager(store, llmCfg, log)

	return manager, store
}

// TestNewLLMCredentialManager tests creating a new LLM credential manager
func TestNewLLMCredentialManager(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	if manager == nil {
		t.Fatal("manager is nil")
	}
	if manager.store == nil {
		t.Fatal("manager.store is nil")
	}
}

// TestLLMCredentialManager_InjectLLMCredentials tests injecting LLM credentials into environment
func TestLLMCredentialManager_InjectLLMCredentials(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	ctx := context.Background()

	// Store some LLM credentials
	err := store.StoreLLMCredential(ctx, LLMProviderAnthropic, "sk-ant-test-12345")
	if err != nil {
		t.Fatalf("failed to store Anthropic credential: %v", err)
	}
	err = store.StoreLLMCredential(ctx, LLMProviderOpenAI, "sk-openai-test-67890")
	if err != nil {
		t.Fatalf("failed to store OpenAI credential: %v", err)
	}

	// Create initial environment
	initialEnv := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/root",
	}

	// Inject LLM credentials
	resultEnv := manager.InjectLLMCredentials(initialEnv)

	// Verify environment contains the initial variables
	foundPath := false
	foundHome := false
	for _, env := range resultEnv {
		if env == "PATH=/usr/bin:/bin" {
			foundPath = true
		}
		if env == "HOME=/root" {
			foundHome = true
		}
	}
	if !foundPath {
		t.Error("PATH environment variable not found")
	}
	if !foundHome {
		t.Error("HOME environment variable not found")
	}

	// Verify environment contains LLM credentials
	foundAnthropic := false
	foundOpenAI := false
	for _, env := range resultEnv {
		if env == "ANTHROPIC_API_KEY=sk-ant-test-12345" {
			foundAnthropic = true
		}
		if env == "OPENAI_API_KEY=sk-openai-test-67890" {
			foundOpenAI = true
		}
	}
	if !foundAnthropic {
		t.Error("ANTHROPIC_API_KEY not found in environment")
	}
	if !foundOpenAI {
		t.Error("OPENAI_API_KEY not found in environment")
	}

	// Verify Ollama doesn't add an API key (it uses local-only)
	for _, env := range resultEnv {
		if key, _, found := cut(env, "="); found && key == "OLLAMA_API_KEY" {
			t.Error("OLLAMA_API_KEY should not be in environment (local-only provider)")
		}
	}
}

// cut is a helper function to split a string by the first occurrence of sep
func cut(s, sep string) (before, after string, found bool) {
	if i := indexStr(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// indexStr is a simple string index function
func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// TestLLMCredentialManager_InjectLLMCredentials_NoCredentials tests injection with no stored credentials
func TestLLMCredentialManager_InjectLLMCredentials_NoCredentials(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	initialEnv := []string{"PATH=/usr/bin"}

	// Inject with no credentials stored
	resultEnv := manager.InjectLLMCredentials(initialEnv)

	// Should only have the original environment
	if len(resultEnv) != 1 {
		t.Errorf("expected 1 environment variable, got %d", len(resultEnv))
	}
	if resultEnv[0] != "PATH=/usr/bin" {
		t.Errorf("expected PATH=/usr/bin, got %s", resultEnv[0])
	}
}

// TestLLMCredentialManager_ValidateLLMCredentials tests credential validation
func TestLLMCredentialManager_ValidateLLMCredentials(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	ctx := context.Background()

	// Initially should fail - no credentials stored
	err := manager.ValidateLLMCredentials()
	if err == nil {
		t.Error("expected validation error with no credentials, got nil")
	}

	// Store Anthropic credential
	err = store.StoreLLMCredential(ctx, LLMProviderAnthropic, "sk-ant-test")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Should still fail - OpenAI is also enabled
	err = manager.ValidateLLMCredentials()
	if err == nil {
		t.Error("expected validation error for missing OpenAI credential, got nil")
	}

	// Store OpenAI credential
	err = store.StoreLLMCredential(ctx, LLMProviderOpenAI, "sk-openai-test")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Now should pass - Anthropic and OpenAI both have credentials
	err = manager.ValidateLLMCredentials()
	if err != nil {
		t.Errorf("expected validation to pass, got error: %v", err)
	}
}

// TestLLMCredentialManager_ValidateLLMCredentials_DisabledProvider tests that disabled providers don't require credentials
func TestLLMCredentialManager_ValidateLLMCredentials_DisabledProvider(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	ctx := context.Background()

	// Disable OpenAI provider
	manager.cfg.Providers[LLMProviderOpenAI].Enabled = false

	// Store only Anthropic credential
	err := store.StoreLLMCredential(ctx, LLMProviderAnthropic, "sk-ant-test")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Should pass - OpenAI is disabled
	err = manager.ValidateLLMCredentials()
	if err != nil {
		t.Errorf("expected validation to pass with disabled provider, got error: %v", err)
	}
}

// TestLLMCredentialManager_ValidateLLMCredentials_LocalProviders tests that local providers don't require credentials
func TestLLMCredentialManager_ValidateLLMCredentials_LocalProviders(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	tmpDir := t.TempDir()
	cfg := config.DefaultCredentialsConfig()
	cfg.StorePath = filepath.Join(tmpDir, "credentials.json")

	store, err := NewStore(cfg, log)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create LLM config with only local providers
	llmCfg := config.LLMConfig{
		Enabled: true,
		Providers: map[string]config.ProviderConfig{
			LLMProviderOllama: {
				Name:    LLMProviderOllama,
				BaseURL: "http://localhost:11434",
				Enabled: true,
				Models:  []string{"ollama/llama3"},
			},
			LLMProviderLMStudio: {
				Name:    LLMProviderLMStudio,
				BaseURL: "http://localhost:1234",
				Enabled: true,
				Models:  []string{"lmstudio/llama3"},
			},
		},
	}

	manager := NewLLMCredentialManager(store, llmCfg, log)

	// Should pass - local providers don't require API keys
	err = manager.ValidateLLMCredentials()
	if err != nil {
		t.Errorf("expected validation to pass for local providers, got error: %v", err)
	}
}

// TestLLMCredentialManager_GetProviderConfig tests retrieving provider configuration
func TestLLMCredentialManager_GetProviderConfig(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	ctx := context.Background()

	// Store credential for Anthropic
	err := store.StoreLLMCredential(ctx, LLMProviderAnthropic, "sk-ant-test-key")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Get provider config
	providerCfg, err := manager.GetProviderConfig(LLMProviderAnthropic)
	if err != nil {
		t.Fatalf("failed to get provider config: %v", err)
	}

	// Verify config
	if providerCfg.Name != LLMProviderAnthropic {
		t.Errorf("expected name %s, got %s", LLMProviderAnthropic, providerCfg.Name)
	}
	if providerCfg.APIKey != "sk-ant-test-key" {
		t.Errorf("expected API key 'sk-ant-test-key', got '%s'", providerCfg.APIKey)
	}
	if providerCfg.BaseURL != "https://api.anthropic.com" {
		t.Errorf("expected base URL 'https://api.anthropic.com', got '%s'", providerCfg.BaseURL)
	}
	if !providerCfg.Enabled {
		t.Error("expected provider to be enabled")
	}
}

// TestLLMCredentialManager_GetProviderConfig_NotFound tests retrieving config for non-existent provider
func TestLLMCredentialManager_GetProviderConfig_NotFound(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	// Try to get config for non-existent provider
	_, err := manager.GetProviderConfig("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent provider, got nil")
	}
	if !types.IsErrCode(err, types.ErrCodeNotFound) {
		t.Errorf("expected ErrCodeNotFound, got: %v", err)
	}
}

// TestLLMCredentialManager_GetProviderConfig_MissingCredential tests retrieving config when credential is missing
func TestLLMCredentialManager_GetProviderConfig_MissingCredential(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	// Try to get config for provider without stored credential
	_, err := manager.GetProviderConfig(LLMProviderAnthropic)
	if err == nil {
		t.Error("expected error for missing credential, got nil")
	}
	if !types.IsErrCode(err, types.ErrCodeNotFound) {
		t.Errorf("expected ErrCodeNotFound, got: %v", err)
	}
}

// TestLLMCredentialManager_GetProviderConfig_LocalProvider tests that local providers work without credentials
func TestLLMCredentialManager_GetProviderConfig_LocalProvider(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	// Get config for Ollama (local provider, no API key needed)
	providerCfg, err := manager.GetProviderConfig(LLMProviderOllama)
	if err != nil {
		t.Fatalf("failed to get Ollama provider config: %v", err)
	}

	// Verify config
	if providerCfg.Name != LLMProviderOllama {
		t.Errorf("expected name %s, got %s", LLMProviderOllama, providerCfg.Name)
	}
	if providerCfg.APIKey != "" {
		t.Errorf("expected empty API key for local provider, got '%s'", providerCfg.APIKey)
	}
}

// TestLLMCredentialManager_ListConfiguredProviders tests listing configured providers
func TestLLMCredentialManager_ListConfiguredProviders(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	ctx := context.Background()

	// Initially no providers configured
	providers, err := manager.ListConfiguredProviders()
	if err != nil {
		t.Fatalf("failed to list configured providers: %v", err)
	}
	if len(providers) != 0 {
		t.Errorf("expected 0 configured providers, got %d", len(providers))
	}

	// Store Anthropic credential
	err = store.StoreLLMCredential(ctx, LLMProviderAnthropic, "sk-ant-test")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Should list Anthropic
	providers, err = manager.ListConfiguredProviders()
	if err != nil {
		t.Fatalf("failed to list configured providers: %v", err)
	}
	if len(providers) != 1 {
		t.Errorf("expected 1 configured provider, got %d", len(providers))
	}
	if len(providers) > 0 && providers[0] != LLMProviderAnthropic {
		t.Errorf("expected provider %s, got %s", LLMProviderAnthropic, providers[0])
	}

	// Store OpenAI credential
	err = store.StoreLLMCredential(ctx, LLMProviderOpenAI, "sk-openai-test")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Should list both providers
	providers, err = manager.ListConfiguredProviders()
	if err != nil {
		t.Fatalf("failed to list configured providers: %v", err)
	}
	if len(providers) != 2 {
		t.Errorf("expected 2 configured providers, got %d", len(providers))
	}
}

// TestLLMCredentialManager_ListConfiguredProviders_Disabled tests that disabled providers are not listed
func TestLLMCredentialManager_ListConfiguredProviders_Disabled(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	ctx := context.Background()

	// Store credentials for both providers
	err := store.StoreLLMCredential(ctx, LLMProviderAnthropic, "sk-ant-test")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}
	err = store.StoreLLMCredential(ctx, LLMProviderOpenAI, "sk-openai-test")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Disable OpenAI
	manager.cfg.Providers[LLMProviderOpenAI].Enabled = false

	// Should only list Anthropic
	providers, err := manager.ListConfiguredProviders()
	if err != nil {
		t.Fatalf("failed to list configured providers: %v", err)
	}
	if len(providers) != 1 {
		t.Errorf("expected 1 configured provider, got %d", len(providers))
	}
	if len(providers) > 0 && providers[0] != LLMProviderAnthropic {
		t.Errorf("expected provider %s, got %s", LLMProviderAnthropic, providers[0])
	}
}

// TestLLMCredentialManager_EnsureDefaultProvider tests checking default provider
func TestLLMCredentialManager_EnsureDefaultProvider(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	ctx := context.Background()

	// Should fail - no credentials
	err := manager.EnsureDefaultProvider()
	if err == nil {
		t.Error("expected error with no credentials, got nil")
	}

	// Store default provider credential
	err = store.StoreLLMCredential(ctx, LLMProviderAnthropic, "sk-ant-test")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Should pass now
	err = manager.EnsureDefaultProvider()
	if err != nil {
		t.Errorf("expected default provider check to pass, got error: %v", err)
	}
}

// TestLLMCredentialManager_EnsureDefaultProvider_NoDefault tests with no default provider configured
func TestLLMCredentialManager_EnsureDefaultProvider_NoDefault(t *testing.T) {
	manager, store := createTestLLMCredentialManager(t)
	defer store.Close()

	// Clear default provider
	manager.cfg.DefaultProvider = ""

	err := manager.EnsureDefaultProvider()
	if err == nil {
		t.Error("expected error with no default provider, got nil")
	}
}

// TestGetCredentialEnvVar tests getting environment variable name for provider
func TestGetCredentialEnvVar(t *testing.T) {
	tests := []struct {
		name          string
		provider      string
		expectedEnv   string
		expectedFound bool
	}{
		{
			name:          "Anthropic provider",
			provider:      LLMProviderAnthropic,
			expectedEnv:   "ANTHROPIC_API_KEY",
			expectedFound: true,
		},
		{
			name:          "OpenAI provider",
			provider:      LLMProviderOpenAI,
			expectedEnv:   "OPENAI_API_KEY",
			expectedFound: true,
		},
		{
			name:          "OpenRouter provider",
			provider:      LLMProviderOpenRouter,
			expectedEnv:   "OPENROUTER_API_KEY",
			expectedFound: true,
		},
		{
			name:          "Ollama provider (local, no env var)",
			provider:      LLMProviderOllama,
			expectedEnv:   "",
			expectedFound: false,
		},
		{
			name:          "LM Studio provider (local, no env var)",
			provider:      LLMProviderLMStudio,
			expectedEnv:   "",
			expectedFound: false,
		},
		{
			name:          "Unknown provider",
			provider:      "unknown",
			expectedEnv:   "",
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVar, found := GetCredentialEnvVar(tt.provider)
			if found != tt.expectedFound {
				t.Errorf("expected found=%v, got %v", tt.expectedFound, found)
			}
			if envVar != tt.expectedEnv {
				t.Errorf("expected env var '%s', got '%s'", tt.expectedEnv, envVar)
			}
		})
	}
}
