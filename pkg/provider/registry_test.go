package provider

import (
	"context"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	tests := []struct {
		name        string
		cfg         RegistryConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration",
			cfg: RegistryConfig{
				DefaultProvider:   ProviderAnthropic,
				Providers:         map[Provider]ProviderConfig{},
				FailoverEnabled:   true,
				FailoverThreshold: 3,
			},
			expectError: false,
		},
		{
			name: "empty configuration with defaults",
			cfg: RegistryConfig{
				Providers: map[Provider]ProviderConfig{},
			},
			expectError: false,
		},
		{
			name: "invalid default provider",
			cfg: RegistryConfig{
				DefaultProvider: Provider("invalid"),
				Providers:       map[Provider]ProviderConfig{},
			},
			expectError: true,
			errorMsg:    "invalid default provider",
		},
		{
			name: "provider config mismatch",
			cfg: RegistryConfig{
				Providers: map[Provider]ProviderConfig{
					ProviderAnthropic: {
						Provider: ProviderOpenAI, // Mismatch
					},
				},
			},
			expectError: true,
			errorMsg:    "provider config mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, err := logger.NewDefault()
			require.NoError(t, err)

			registry, err := NewRegistry(tt.cfg, log)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, registry)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, registry)
				assert.False(t, registry.IsClosed())
			}
		})
	}
}

func TestNewDefaultRegistry(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	registry, err := NewDefaultRegistry(log)
	require.NoError(t, err)
	require.NotNil(t, registry)

	assert.False(t, registry.IsClosed())
	assert.Equal(t, ProviderAnthropic, registry.defaultProvider)
	assert.True(t, registry.cfg.FailoverEnabled)
}

func TestRegistry_Register(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("register nil provider", func(t *testing.T) {
		err := registry.Register(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider cannot be nil")
	})

	t.Run("register provider successfully", func(t *testing.T) {
		// Create a mock provider
		mockProvider := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Test Provider",
			status: ProviderStatusAvailable,
		}

		err := registry.Register(ctx, mockProvider)
		assert.NoError(t, err)

		// Verify provider is registered
		retrieved, err := registry.Get(ctx, ProviderAnthropic)
		assert.NoError(t, err)
		assert.Equal(t, mockProvider, retrieved)
	})

	t.Run("register duplicate provider", func(t *testing.T) {
		mockProvider := &mockLLMProvider{
			id:     ProviderOpenAI,
			name:   "OpenAI",
			status: ProviderStatusAvailable,
		}

		err := registry.Register(ctx, mockProvider)
		assert.NoError(t, err)

		// Try to register again
		err = registry.Register(ctx, mockProvider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("register after close", func(t *testing.T) {
		registry2, err := NewRegistry(RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}, log)
		require.NoError(t, err)

		registry2.Close()

		mockProvider := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Test",
			status: ProviderStatusAvailable,
		}

		err = registry2.Register(ctx, mockProvider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "registry is closed")
	})
}

func TestRegistry_RegisterFromConfig(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := DefaultConfig()
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("invalid provider ID", func(t *testing.T) {
		providerCfg := ProviderConfig{
			Provider: Provider("invalid"),
		}

		err := registry.RegisterFromConfig(ctx, providerCfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid provider ID")
	})

	t.Run("missing API key", func(t *testing.T) {
		providerCfg := ProviderConfig{
			Provider: ProviderAnthropic,
			// Missing APIKey
		}

		err := registry.RegisterFromConfig(ctx, providerCfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API key is required")
	})
}

func TestRegistry_Get(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	// Register a test provider
	mockProvider := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Test Provider",
		status: ProviderStatusAvailable,
	}
	err = registry.Register(ctx, mockProvider)
	require.NoError(t, err)

	t.Run("get existing provider", func(t *testing.T) {
		retrieved, err := registry.Get(ctx, ProviderAnthropic)
		assert.NoError(t, err)
		assert.Equal(t, mockProvider, retrieved)
	})

	t.Run("get non-existent provider", func(t *testing.T) {
		retrieved, err := registry.Get(ctx, ProviderOpenAI)
		assert.Error(t, err)
		assert.Nil(t, retrieved)
		assert.Contains(t, err.Error(), "provider not found")
	})

	t.Run("get with invalid provider ID", func(t *testing.T) {
		retrieved, err := registry.Get(ctx, Provider("invalid"))
		assert.Error(t, err)
		assert.Nil(t, retrieved)
		assert.Contains(t, err.Error(), "invalid provider ID")
	})

	t.Run("get after close", func(t *testing.T) {
		registry2, err := NewRegistry(RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}, log)
		require.NoError(t, err)
		registry2.Close()

		_, err = registry2.Get(ctx, ProviderAnthropic)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "registry is closed")
	})
}

func TestRegistry_GetDefault(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("get default provider when set", func(t *testing.T) {
		cfg := RegistryConfig{
			DefaultProvider: ProviderAnthropic,
			Providers:       map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		ctx := context.Background()

		mockProvider := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Default Provider",
			status: ProviderStatusAvailable,
		}
		err = registry.Register(ctx, mockProvider)
		require.NoError(t, err)

		retrieved, err := registry.GetDefault(ctx)
		assert.NoError(t, err)
		assert.Equal(t, mockProvider, retrieved)
	})

	t.Run("get default provider when not registered", func(t *testing.T) {
		cfg := RegistryConfig{
			DefaultProvider: ProviderOpenAI,
			Providers:       map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		ctx := context.Background()

		_, err = registry.GetDefault(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "default provider not found")
	})
}

func TestRegistry_SetDefault(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		DefaultProvider: ProviderAnthropic,
		Providers:       map[Provider]ProviderConfig{},
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	// Register providers
	mockAnthropic := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Anthropic",
		status: ProviderStatusAvailable,
	}
	mockOpenAI := &mockLLMProvider{
		id:     ProviderOpenAI,
		name:   "OpenAI",
		status: ProviderStatusAvailable,
	}

	err = registry.Register(ctx, mockAnthropic)
	require.NoError(t, err)
	err = registry.Register(ctx, mockOpenAI)
	require.NoError(t, err)

	t.Run("set default to valid provider", func(t *testing.T) {
		err := registry.SetDefault(ctx, ProviderOpenAI)
		assert.NoError(t, err)

		// Verify default changed
		defaultProvider, err := registry.GetDefault(ctx)
		assert.NoError(t, err)
		assert.Equal(t, mockOpenAI, defaultProvider)
	})

	t.Run("set default to non-existent provider", func(t *testing.T) {
		// Use a valid provider type that isn't registered
		// Since we've registered Anthropic and OpenAI, let's try to create a new registry
		// with only Anthropic registered, then try to set OpenAI as default
		newRegistry, err := NewRegistry(RegistryConfig{
			DefaultProvider: ProviderAnthropic,
			Providers:       map[Provider]ProviderConfig{},
		}, log)
		require.NoError(t, err)

		mockAnthropic := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Anthropic",
			status: ProviderStatusAvailable,
		}
		err = newRegistry.Register(ctx, mockAnthropic)
		require.NoError(t, err)

		// Now try to set default to OpenAI (valid type, but not registered)
		err = newRegistry.SetDefault(ctx, ProviderOpenAI)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider not found")
	})

	t.Run("set default with invalid provider ID", func(t *testing.T) {
		err := registry.SetDefault(ctx, Provider(""))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid provider ID")
	})
}

func TestRegistry_List(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("list empty registry", func(t *testing.T) {
		providers, err := registry.List(ctx)
		assert.NoError(t, err)
		assert.Empty(t, providers)
	})

	t.Run("list with providers", func(t *testing.T) {
		mockProvider1 := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Anthropic",
			status: ProviderStatusAvailable,
		}
		mockProvider2 := &mockLLMProvider{
			id:     ProviderOpenAI,
			name:   "OpenAI",
			status: ProviderStatusAvailable,
		}

		err := registry.Register(ctx, mockProvider1)
		require.NoError(t, err)
		err = registry.Register(ctx, mockProvider2)
		require.NoError(t, err)

		providers, err := registry.List(ctx)
		assert.NoError(t, err)
		assert.Len(t, providers, 2)
	})
}

func TestRegistry_ListAvailable(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	// Register providers with different statuses
	mockAvailable := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Available",
		status: ProviderStatusAvailable,
	}
	mockUnavailable := &mockLLMProvider{
		id:     ProviderOpenAI,
		name:   "Unavailable",
		status: ProviderStatusUnavailable,
	}

	err = registry.Register(ctx, mockAvailable)
	require.NoError(t, err)
	err = registry.Register(ctx, mockUnavailable)
	require.NoError(t, err)

	providers, err := registry.ListAvailable(ctx)
	assert.NoError(t, err)
	assert.Len(t, providers, 1)
	assert.Equal(t, mockAvailable, providers[0])
}

func TestRegistry_Unregister(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("unregister existing provider", func(t *testing.T) {
		mockProvider := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Test",
			status: ProviderStatusAvailable,
		}

		err := registry.Register(ctx, mockProvider)
		require.NoError(t, err)

		err = registry.Unregister(ctx, ProviderAnthropic)
		assert.NoError(t, err)

		// Verify provider is removed
		_, err = registry.Get(ctx, ProviderAnthropic)
		assert.Error(t, err)
	})

	t.Run("unregister non-existent provider", func(t *testing.T) {
		err := registry.Unregister(ctx, ProviderOpenAI)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider not found")
	})
}

func TestRegistry_Close(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	// Register some providers
	mockProvider1 := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Anthropic",
		status: ProviderStatusAvailable,
		closed: false,
	}
	mockProvider2 := &mockLLMProvider{
		id:     ProviderOpenAI,
		name:   "OpenAI",
		status: ProviderStatusAvailable,
		closed: false,
	}

	err = registry.Register(ctx, mockProvider1)
	require.NoError(t, err)
	err = registry.Register(ctx, mockProvider2)
	require.NoError(t, err)

	// Close registry
	err = registry.Close()
	assert.NoError(t, err)
	assert.True(t, registry.IsClosed())

	// Verify all providers are closed
	assert.True(t, mockProvider1.closed)
	assert.True(t, mockProvider2.closed)

	// Double close should be safe
	err = registry.Close()
	assert.NoError(t, err)
}

func TestRegistry_Stats(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		Providers: map[Provider]ProviderConfig{},
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	ctx := context.Background()

	// Register a mock provider with token accounting
	mockProvider := &mockLLMProvider{
		id:           ProviderAnthropic,
		name:         "Test",
		status:       ProviderStatusAvailable,
		tokenAccount: NewTokenAccount(ProviderAnthropic),
	}

	// Record some usage
	mockProvider.tokenAccount.Record(types.GenerateID().String(), ModelClaude3_5Sonnet, TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150, // Total is not auto-computed
	})

	err = registry.Register(ctx, mockProvider)
	require.NoError(t, err)

	stats, err := registry.Stats(ctx)
	assert.NoError(t, err)
	assert.Len(t, stats, 1)

	anthropicStats := stats[ProviderAnthropic]
	assert.Equal(t, ProviderAnthropic, anthropicStats.Provider)
	assert.Equal(t, ProviderStatusAvailable, anthropicStats.Status)
	assert.Equal(t, int64(150), anthropicStats.TotalTokens)
}

func TestRegistry_InitializeFromConfig(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("initialize with valid config", func(t *testing.T) {
		cfg := RegistryConfig{
			Providers: map[Provider]ProviderConfig{
				ProviderAnthropic: {
					Provider: ProviderAnthropic,
					Enabled:  false, // Disabled to avoid API key requirement
				},
			},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		ctx := context.Background()
		err = registry.InitializeFromConfig(ctx)
		assert.NoError(t, err)

		// No providers should be registered since we disabled them
		providers, err := registry.List(ctx)
		assert.NoError(t, err)
		assert.Empty(t, providers)
	})
}

func TestGlobalRegistry(t *testing.T) {
	t.Run("global registry singleton", func(t *testing.T) {
		// Reset global registry
		SetGlobal(nil)

		reg1 := Global()
		reg2 := Global()

		assert.Same(t, reg1, reg2)
	})

	t.Run("set global registry", func(t *testing.T) {
		log, err := logger.NewDefault()
		require.NoError(t, err)

		cfg := RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		SetGlobal(registry)

		retrieved := Global()
		assert.Same(t, registry, retrieved)

		// Reset for other tests
		SetGlobal(nil)
	})
}

// mockLLMProvider is a mock implementation of LLMProvider for testing
type mockLLMProvider struct {
	id                  Provider
	name                string
	status              ProviderStatus
	closed              bool
	tokenAccount        *TokenAccount
	supportsAllModels   bool
}

func (m *mockLLMProvider) Provider() Provider {
	return m.id
}

func (m *mockLLMProvider) Name() string {
	return m.name
}

func (m *mockLLMProvider) Status() ProviderStatus {
	return m.status
}

func (m *mockLLMProvider) IsAvailable() bool {
	return m.status.IsAvailable()
}

func (m *mockLLMProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if !m.IsAvailable() {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider not available")
	}
	return &CompletionResponse{
		ID:         types.GenerateID().String(),
		Model:      req.Model,
		Content:    "Test response",
		StopReason: StopReasonEndTurn,
		Usage: TokenUsage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}, nil
}

func (m *mockLLMProvider) CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan *CompletionChunk, error) {
	if !m.IsAvailable() {
		return nil, NewProviderError(ErrCodeProviderNotAvailable, "provider not available")
	}
	ch := make(chan *CompletionChunk, 1)
	ch <- &CompletionChunk{
		ID:      types.GenerateID().String(),
		Model:   req.Model,
		Content: "Test response",
	}
	close(ch)
	return ch, nil
}

func (m *mockLLMProvider) SupportsModel(model Model) bool {
	if m.supportsAllModels {
		return true
	}
	// Default behavior - support Anthropic models for Anthropic provider
	if m.id == ProviderAnthropic {
		switch model {
		case ModelClaude3_5Sonnet, ModelClaude3_5SonnetNew, ModelClaude3Opus, ModelClaude3Sonnet, ModelClaude3Haiku:
			return true
		}
	}
	// Support OpenAI models for OpenAI provider
	if m.id == ProviderOpenAI {
		switch model {
		case ModelGPT4o, ModelGPT4oMini, ModelGPT4Turbo, ModelGPT4, ModelGPT35Turbo:
			return true
		}
	}
	return false
}

func (m *mockLLMProvider) ModelInfo(model Model) (*ModelInfo, error) {
	return &ModelInfo{
		ID:          model,
		Provider:    m.id,
		Name:        string(model),
		ContextSize: 200000,
		Capabilities: ModelCapabilities{
			Streaming:       true,
			FunctionCalling: true,
		},
	}, nil
}

func (m *mockLLMProvider) ListModels() ([]Model, error) {
	return []Model{ModelClaude3_5Sonnet}, nil
}

func (m *mockLLMProvider) Close() error {
	m.closed = true
	m.status = ProviderStatusUnavailable
	return nil
}

func (m *mockLLMProvider) GetTokenAccount() *TokenAccount {
	if m.tokenAccount == nil {
		m.tokenAccount = NewTokenAccount(m.id)
	}
	return m.tokenAccount
}

func TestRegistry_GetProviderByModel(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("get provider for Anthropic model", func(t *testing.T) {
		cfg := RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		anthropic := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Anthropic",
			status: ProviderStatusAvailable,
		}

		err = registry.Register(ctx, anthropic)
		require.NoError(t, err)

		provider, err := registry.GetProviderByModel(ctx, ModelClaude3_5Sonnet)
		assert.NoError(t, err)
		assert.Equal(t, anthropic, provider)
	})

	t.Run("get provider for OpenAI model", func(t *testing.T) {
		cfg := RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		openai := &mockLLMProvider{
			id:     ProviderOpenAI,
			name:   "OpenAI",
			status: ProviderStatusAvailable,
		}

		err = registry.Register(ctx, openai)
		require.NoError(t, err)

		provider, err := registry.GetProviderByModel(ctx, ModelGPT4o)
		assert.NoError(t, err)
		assert.Equal(t, openai, provider)
	})

	t.Run("empty model returns error", func(t *testing.T) {
		cfg := RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		anthropic := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Anthropic",
			status: ProviderStatusAvailable,
		}

		err = registry.Register(ctx, anthropic)
		require.NoError(t, err)

		_, err = registry.GetProviderByModel(ctx, Model(""))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "model cannot be empty")
	})

	t.Run("unknown model returns error", func(t *testing.T) {
		cfg := RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		anthropic := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Anthropic",
			status: ProviderStatusAvailable,
		}

		err = registry.Register(ctx, anthropic)
		require.NoError(t, err)

		_, err = registry.GetProviderByModel(ctx, Model("unknown-model"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no available provider found")
	})

	t.Run("provider unavailable returns error", func(t *testing.T) {
		cfg := RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		anthropic := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Anthropic",
			status: ProviderStatusUnavailable,
		}

		err = registry.Register(ctx, anthropic)
		require.NoError(t, err)

		_, err = registry.GetProviderByModel(ctx, ModelClaude3_5Sonnet)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no available provider found")
	})

	t.Run("closed registry returns error", func(t *testing.T) {
		cfg := RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)
		registry.Close()

		_, err = registry.GetProviderByModel(ctx, ModelClaude3_5Sonnet)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "registry is closed")
	})
}

func TestRegistry_Config(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("get config returns expected values", func(t *testing.T) {
		expectedCfg := RegistryConfig{
			DefaultProvider:      ProviderOpenAI,
			FailoverEnabled:      true,
			FailoverThreshold:    5,
			CircuitBreakerTimeout: 120 * time.Second,
			Providers: map[Provider]ProviderConfig{
				ProviderAnthropic: {
					Provider: ProviderAnthropic,
					Enabled:  true,
					Priority: 10,
				},
			},
		}

		registry, err := NewRegistry(expectedCfg, log)
		require.NoError(t, err)

		actualCfg := registry.Config()

		assert.Equal(t, expectedCfg.DefaultProvider, actualCfg.DefaultProvider)
		assert.Equal(t, expectedCfg.FailoverEnabled, actualCfg.FailoverEnabled)
		assert.Equal(t, expectedCfg.FailoverThreshold, actualCfg.FailoverThreshold)
		assert.Equal(t, expectedCfg.CircuitBreakerTimeout, actualCfg.CircuitBreakerTimeout)
		assert.Contains(t, actualCfg.Providers, ProviderAnthropic)
	})

	t.Run("config is independent of registry state", func(t *testing.T) {
		cfg := RegistryConfig{
			DefaultProvider: ProviderAnthropic,
			Providers:       map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		// Get config
		retrievedCfg := registry.Config()

		// Modify retrieved config (should not affect registry)
		retrievedCfg.DefaultProvider = ProviderOpenAI

		// Get config again
		cfg2 := registry.Config()

		// Should still be Anthropic
		assert.Equal(t, ProviderAnthropic, cfg2.DefaultProvider)
	})
}

func TestRegistry_ResetAllFailover(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	ctx := context.Background()

	cfg := RegistryConfig{
		Providers:       map[Provider]ProviderConfig{},
		FailoverEnabled: true,
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	// Register multiple providers
	anthropic := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Anthropic",
		status: ProviderStatusAvailable,
	}
	openai := &mockLLMProvider{
		id:     ProviderOpenAI,
		name:   "OpenAI",
		status: ProviderStatusAvailable,
	}

	err = registry.Register(ctx, anthropic)
	require.NoError(t, err)
	err = registry.Register(ctx, openai)
	require.NoError(t, err)

	// Record failures for all providers
	registry.RecordProviderFailure(ProviderAnthropic, "error 1")
	registry.RecordProviderFailure(ProviderAnthropic, "error 2")
	registry.RecordProviderFailure(ProviderOpenAI, "error 1")

	// Verify failures are recorded
	state1 := registry.GetFailoverState(ProviderAnthropic)
	state2 := registry.GetFailoverState(ProviderOpenAI)
	assert.Equal(t, 2, state1.ConsecutiveFailures)
	assert.Equal(t, 1, state2.ConsecutiveFailures)

	// Reset all
	registry.ResetAllFailover()

	// Verify all states are reset
	state1 = registry.GetFailoverState(ProviderAnthropic)
	state2 = registry.GetFailoverState(ProviderOpenAI)
	assert.Equal(t, 0, state1.ConsecutiveFailures)
	assert.Equal(t, 0, state2.ConsecutiveFailures)
	assert.Empty(t, state1.LastFailureReason)
	assert.Empty(t, state2.LastFailureReason)
}

func TestRegistry_FailoverWithPriority(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("failover selects higher priority backup", func(t *testing.T) {
		cfg := RegistryConfig{
			DefaultProvider:   ProviderAnthropic,
			Providers: map[Provider]ProviderConfig{
				ProviderAnthropic: {Provider: ProviderAnthropic, Priority: 10},
				ProviderOpenAI:    {Provider: ProviderOpenAI, Priority: 5},  // Higher priority (lower number)
			},
			FailoverEnabled:   true,
			FailoverThreshold: 3,
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		primary := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Primary",
			status: ProviderStatusUnavailable,
		}
		backup1 := &mockLLMProvider{
			id:     ProviderOpenAI,
			name:   "Backup1",
			status: ProviderStatusAvailable,
		}
		backup2 := &mockLLMProvider{
			id:     Provider("other"),
			name:   "Backup2",
			status: ProviderStatusAvailable,
		}
		backup2.supportsAllModels = true

		err = registry.Register(ctx, primary)
		require.NoError(t, err)
		err = registry.Register(ctx, backup1)
		require.NoError(t, err)
		err = registry.Register(ctx, backup2)
		require.NoError(t, err)

		// With failover, should select the higher priority backup (OpenAI with priority 5)
		provider, failover, err := registry.GetWithFailover(ctx, ProviderAnthropic)
		assert.NoError(t, err)
		assert.True(t, failover)
		assert.Equal(t, backup1, provider) // OpenAI has higher priority (lower number)
	})

	t.Run("failover with all backups unavailable returns error", func(t *testing.T) {
		cfg := RegistryConfig{
			DefaultProvider:   ProviderAnthropic,
			Providers:         map[Provider]ProviderConfig{},
			FailoverEnabled:   true,
			FailoverThreshold: 3,
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		primary := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Primary",
			status: ProviderStatusUnavailable,
		}
		backup := &mockLLMProvider{
			id:     ProviderOpenAI,
			name:   "Backup",
			status: ProviderStatusUnavailable, // Also unavailable
		}

		err = registry.Register(ctx, primary)
		require.NoError(t, err)
		err = registry.Register(ctx, backup)
		require.NoError(t, err)

		provider, failover, err := registry.GetWithFailover(ctx, ProviderAnthropic)
		assert.Error(t, err)
		assert.False(t, failover)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "all providers are unavailable")
	})
}

func TestRegistry_FailoverWithCircuitBreaker(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("circuit breaker prevents provider selection", func(t *testing.T) {
		cfg := RegistryConfig{
			DefaultProvider:      ProviderAnthropic,
			Providers:            map[Provider]ProviderConfig{},
			FailoverEnabled:      true,
			FailoverThreshold:    2,
			CircuitBreakerTimeout: 100 * time.Millisecond,
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		primary := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Primary",
			status: ProviderStatusAvailable,
		}
		backup := &mockLLMProvider{
			id:     ProviderOpenAI,
			name:   "Backup",
			status: ProviderStatusAvailable,
		}

		err = registry.Register(ctx, primary)
		require.NoError(t, err)
		err = registry.Register(ctx, backup)
		require.NoError(t, err)

		// Trigger circuit breaker on primary
		registry.RecordProviderFailure(ProviderAnthropic, "error 1")
		registry.RecordProviderFailure(ProviderAnthropic, "error 2")

		// Circuit should be open now
		state := registry.GetFailoverState(ProviderAnthropic)
		assert.True(t, state.CircuitOpen)

		// Request with failover should use backup
		provider, failover, err := registry.GetWithFailover(ctx, ProviderAnthropic)
		assert.NoError(t, err)
		assert.True(t, failover)
		assert.Equal(t, backup, provider)
	})

	t.Run("circuit expires after timeout", func(t *testing.T) {
		cfg := RegistryConfig{
			DefaultProvider:      ProviderAnthropic,
			Providers:            map[Provider]ProviderConfig{},
			FailoverEnabled:      true,
			FailoverThreshold:    2,
			CircuitBreakerTimeout: 50 * time.Millisecond,
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		primary := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Primary",
			status: ProviderStatusAvailable,
		}

		err = registry.Register(ctx, primary)
		require.NoError(t, err)

		// Trigger circuit breaker
		registry.RecordProviderFailure(ProviderAnthropic, "error 1")
		registry.RecordProviderFailure(ProviderAnthropic, "error 2")

		// Circuit should be open
		state := registry.GetFailoverState(ProviderAnthropic)
		assert.True(t, state.CircuitOpen)

		// Wait for circuit to expire
		time.Sleep(60 * time.Millisecond)

		// Circuit should be closed now
		state = registry.GetFailoverState(ProviderAnthropic)
		assert.False(t, state.CircuitOpen)
	})
}
