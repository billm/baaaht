package provider

import (
	"context"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFailoverManager(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		FailoverEnabled:      true,
		FailoverThreshold:    3,
		CircuitBreakerTimeout: 60 * time.Second,
	}

	fm := NewFailoverManager(cfg, log)
	assert.NotNil(t, fm)
	assert.False(t, fm.closed)
	assert.Equal(t, cfg, fm.cfg)
}

func TestFailoverManager_RecordFailure(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("failover disabled - does not record failures", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled: false,
		}
		fm := NewFailoverManager(cfg, log)

		fm.RecordFailure(ProviderAnthropic, "test error")

		state := fm.GetProviderState(ProviderAnthropic)
		assert.Equal(t, 0, state.ConsecutiveFailures)
		assert.False(t, state.CircuitOpen)
	})

	t.Run("failover enabled - records failures", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled:      true,
			FailoverThreshold:    3,
			CircuitBreakerTimeout: 60 * time.Second,
		}
		fm := NewFailoverManager(cfg, log)

		// Record 2 failures - should not open circuit
		fm.RecordFailure(ProviderAnthropic, "error 1")
		fm.RecordFailure(ProviderAnthropic, "error 2")

		state := fm.GetProviderState(ProviderAnthropic)
		assert.Equal(t, 2, state.ConsecutiveFailures)
		assert.False(t, state.CircuitOpen)
	})

	t.Run("failover threshold reached - circuit opens", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled:      true,
			FailoverThreshold:    3,
			CircuitBreakerTimeout: 100 * time.Millisecond,
		}
		fm := NewFailoverManager(cfg, log)

		// Record 3 failures - should open circuit
		fm.RecordFailure(ProviderOpenAI, "error 1")
		fm.RecordFailure(ProviderOpenAI, "error 2")
		fm.RecordFailure(ProviderOpenAI, "error 3")

		state := fm.GetProviderState(ProviderOpenAI)
		assert.Equal(t, 3, state.ConsecutiveFailures)
		assert.True(t, state.CircuitOpen)
		assert.NotNil(t, state.LastFailureTime)
		assert.Equal(t, "error 3", state.LastFailureReason)
	})
}

func TestFailoverManager_RecordSuccess(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		FailoverEnabled:      true,
		FailoverThreshold:    3,
		CircuitBreakerTimeout: 60 * time.Second,
	}
	fm := NewFailoverManager(cfg, log)

	t.Run("success after failure resets count", func(t *testing.T) {
		// Record some failures
		fm.RecordFailure(ProviderAnthropic, "error")
		fm.RecordFailure(ProviderAnthropic, "error")

		state := fm.GetProviderState(ProviderAnthropic)
		assert.Equal(t, 2, state.ConsecutiveFailures)

		// Record success
		fm.RecordSuccess(ProviderAnthropic)

		state = fm.GetProviderState(ProviderAnthropic)
		assert.Equal(t, 0, state.ConsecutiveFailures)
		assert.False(t, state.CircuitOpen)
		assert.Empty(t, state.LastFailureReason)
	})
}

func TestFailoverManager_IsProviderAvailable(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	t.Run("failover disabled - always available", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled: false,
		}
		fm := NewFailoverManager(cfg, log)

		// Even after failures, should return true
		fm.RecordFailure(ProviderAnthropic, "error")
		assert.True(t, fm.IsProviderAvailable(ProviderAnthropic))
	})

	t.Run("provider with no failures is available", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled: true,
		}
		fm := NewFailoverManager(cfg, log)

		assert.True(t, fm.IsProviderAvailable(ProviderAnthropic))
	})

	t.Run("provider with open circuit is unavailable", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled:      true,
			FailoverThreshold:    2,
			CircuitBreakerTimeout: 100 * time.Millisecond,
		}
		fm := NewFailoverManager(cfg, log)

		// Open the circuit
		fm.RecordFailure(ProviderOpenAI, "error")
		fm.RecordFailure(ProviderOpenAI, "error")

		assert.False(t, fm.IsProviderAvailable(ProviderOpenAI))
	})

	t.Run("circuit expires after timeout", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled:      true,
			FailoverThreshold:    2,
			CircuitBreakerTimeout: 10 * time.Millisecond,
		}
		fm := NewFailoverManager(cfg, log)

		// Open the circuit
		fm.RecordFailure(ProviderAnthropic, "error")
		fm.RecordFailure(ProviderAnthropic, "error")

		assert.False(t, fm.IsProviderAvailable(ProviderAnthropic))

		// Wait for circuit to expire
		time.Sleep(15 * time.Millisecond)

		assert.True(t, fm.IsProviderAvailable(ProviderAnthropic))
	})
}

func TestFailoverManager_ShouldAttemptProvider(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		FailoverEnabled:      true,
		FailoverThreshold:    2,
		CircuitBreakerTimeout: 50 * time.Millisecond,
	}
	fm := NewFailoverManager(cfg, log)

	t.Run("provider with no failures can be attempted", func(t *testing.T) {
		assert.True(t, fm.ShouldAttemptProvider(ProviderAnthropic))
	})

	t.Run("provider with open circuit cannot be attempted", func(t *testing.T) {
		fm.RecordFailure(ProviderOpenAI, "error")
		fm.RecordFailure(ProviderOpenAI, "error")

		assert.False(t, fm.ShouldAttemptProvider(ProviderOpenAI))
	})

	t.Run("provider can be attempted after circuit timeout", func(t *testing.T) {
		fm.RecordFailure(ProviderAnthropic, "error")
		fm.RecordFailure(ProviderAnthropic, "error")

		assert.False(t, fm.ShouldAttemptProvider(ProviderAnthropic))

		// Wait for circuit timeout
		time.Sleep(60 * time.Millisecond)

		assert.True(t, fm.ShouldAttemptProvider(ProviderAnthropic))
	})
}

func TestFailoverManager_GetProviderState(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		FailoverEnabled:      true,
		FailoverThreshold:    3,
		CircuitBreakerTimeout: 60 * time.Second,
	}
	fm := NewFailoverManager(cfg, log)

	t.Run("state for unknown provider", func(t *testing.T) {
		state := fm.GetProviderState(ProviderAnthropic)
		assert.Equal(t, ProviderAnthropic, state.Provider)
		assert.Equal(t, 0, state.ConsecutiveFailures)
		assert.False(t, state.CircuitOpen)
	})

	t.Run("state after failures", func(t *testing.T) {
		fm.RecordFailure(ProviderOpenAI, "test error")
		fm.RecordFailure(ProviderOpenAI, "test error")

		state := fm.GetProviderState(ProviderOpenAI)
		assert.Equal(t, ProviderOpenAI, state.Provider)
		assert.Equal(t, 2, state.ConsecutiveFailures)
		assert.False(t, state.CircuitOpen)
		assert.Equal(t, "test error", state.LastFailureReason)
		assert.NotNil(t, state.LastFailureTime)
	})
}

func TestFailoverManager_Reset(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		FailoverEnabled: true,
	}
	fm := NewFailoverManager(cfg, log)

	// Record failures
	fm.RecordFailure(ProviderAnthropic, "error")
	fm.RecordFailure(ProviderAnthropic, "error")

	// Reset
	fm.Reset(ProviderAnthropic)

	state := fm.GetProviderState(ProviderAnthropic)
	assert.Equal(t, 0, state.ConsecutiveFailures)
	assert.False(t, state.CircuitOpen)
	assert.Empty(t, state.LastFailureReason)
}

func TestFailoverManager_ResetAll(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		FailoverEnabled: true,
	}
	fm := NewFailoverManager(cfg, log)

	// Record failures for multiple providers
	fm.RecordFailure(ProviderAnthropic, "error")
	fm.RecordFailure(ProviderOpenAI, "error")

	// Reset all
	fm.ResetAll()

	state1 := fm.GetProviderState(ProviderAnthropic)
	state2 := fm.GetProviderState(ProviderOpenAI)

	assert.Equal(t, 0, state1.ConsecutiveFailures)
	assert.Equal(t, 0, state2.ConsecutiveFailures)
}

func TestFailoverManager_Close(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	cfg := RegistryConfig{
		FailoverEnabled: true,
	}
	fm := NewFailoverManager(cfg, log)

	// Record some failures
	fm.RecordFailure(ProviderAnthropic, "error")

	// Close
	err = fm.Close()
	assert.NoError(t, err)
	assert.True(t, fm.closed)

	// Operations after close should be no-ops
	fm.RecordFailure(ProviderOpenAI, "error")
	state := fm.GetProviderState(ProviderOpenAI)
	assert.Equal(t, 0, state.ConsecutiveFailures)

	// Double close should be safe
	err = fm.Close()
	assert.NoError(t, err)
}

func TestFailoverManager_SelectProviderWithFailover(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("failover disabled - returns primary", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled: false,
		}
		fm := NewFailoverManager(cfg, log)

		primary := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Primary",
			status: ProviderStatusAvailable,
		}

		provider, failover, err := fm.SelectProviderWithFailover(
			ctx,
			ProviderAnthropic,
			func(p Provider) (LLMProvider, error) {
				if p == ProviderAnthropic {
					return primary, nil
				}
				return nil, NewProviderError(ErrCodeProviderNotFound, "not found")
			},
			func() ([]LLMProvider, error) {
				return []LLMProvider{primary}, nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, primary, provider)
		assert.False(t, failover)
	})

	t.Run("failover enabled - primary available", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled: true,
			Providers: map[Provider]ProviderConfig{
				ProviderAnthropic: {Priority: 10},
				ProviderOpenAI:    {Priority: 20},
			},
		}
		fm := NewFailoverManager(cfg, log)

		primary := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Primary",
			status: ProviderStatusAvailable,
		}

		provider, failover, err := fm.SelectProviderWithFailover(
			ctx,
			ProviderAnthropic,
			func(p Provider) (LLMProvider, error) {
				if p == ProviderAnthropic {
					return primary, nil
				}
				return nil, NewProviderError(ErrCodeProviderNotFound, "not found")
			},
			func() ([]LLMProvider, error) {
				return []LLMProvider{primary}, nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, primary, provider)
		assert.False(t, failover)
	})

	t.Run("primary unavailable - failover to backup", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled: true,
			Providers: map[Provider]ProviderConfig{
				ProviderAnthropic: {Priority: 10},
				ProviderOpenAI:    {Priority: 20},
			},
		}
		fm := NewFailoverManager(cfg, log)

		primary := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Primary",
			status: ProviderStatusUnavailable,
		}
		backup := &mockLLMProvider{
			id:     ProviderOpenAI,
			name:   "Backup",
			status: ProviderStatusAvailable,
		}

		provider, failover, err := fm.SelectProviderWithFailover(
			ctx,
			ProviderAnthropic,
			func(p Provider) (LLMProvider, error) {
				if p == ProviderAnthropic {
					return primary, nil
				}
				if p == ProviderOpenAI {
					return backup, nil
				}
				return nil, NewProviderError(ErrCodeProviderNotFound, "not found")
			},
			func() ([]LLMProvider, error) {
				return []LLMProvider{primary, backup}, nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, backup, provider)
		assert.True(t, failover)
	})

	t.Run("all providers unavailable", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled: true,
		}
		fm := NewFailoverManager(cfg, log)

		primary := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Primary",
			status: ProviderStatusUnavailable,
		}

		_, _, err := fm.SelectProviderWithFailover(
			ctx,
			ProviderAnthropic,
			func(p Provider) (LLMProvider, error) {
				return primary, nil
			},
			func() ([]LLMProvider, error) {
				return []LLMProvider{primary}, nil
			},
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no available backup providers")
	})

	t.Run("primary not found - failover to backup", func(t *testing.T) {
		cfg := RegistryConfig{
			FailoverEnabled: true,
			Providers: map[Provider]ProviderConfig{
				ProviderOpenAI: {Priority: 20},
			},
		}
		fm := NewFailoverManager(cfg, log)

		backup := &mockLLMProvider{
			id:     ProviderOpenAI,
			name:   "Backup",
			status: ProviderStatusAvailable,
		}

		provider, failover, err := fm.SelectProviderWithFailover(
			ctx,
			ProviderAnthropic,
			func(p Provider) (LLMProvider, error) {
				if p == ProviderAnthropic {
					return nil, NewProviderError(ErrCodeProviderNotFound, "not found")
				}
				if p == ProviderOpenAI {
					return backup, nil
				}
				return nil, NewProviderError(ErrCodeProviderNotFound, "not found")
			},
			func() ([]LLMProvider, error) {
				return []LLMProvider{backup}, nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, backup, provider)
		assert.True(t, failover)
	})
}

func TestFailoverState_IsHealthy(t *testing.T) {
	t.Run("healthy state", func(t *testing.T) {
		state := FailoverState{
			Provider:            ProviderAnthropic,
			ConsecutiveFailures: 0,
			CircuitOpen:         false,
		}
		assert.True(t, state.IsHealthy())
	})

	t.Run("unhealthy state - circuit open", func(t *testing.T) {
		state := FailoverState{
			Provider:            ProviderAnthropic,
			ConsecutiveFailures: 3,
			CircuitOpen:         true,
		}
		assert.False(t, state.IsHealthy())
	})
}

func TestFailoverState_String(t *testing.T) {
	t.Run("healthy string", func(t *testing.T) {
		state := FailoverState{
			Provider:            ProviderAnthropic,
			ConsecutiveFailures: 0,
			CircuitOpen:         false,
		}
		assert.Contains(t, state.String(), "healthy")
	})

	t.Run("circuit open string", func(t *testing.T) {
		state := FailoverState{
			Provider:            ProviderOpenAI,
			ConsecutiveFailures: 3,
			CircuitOpen:         true,
		}
		assert.Contains(t, state.String(), "CIRCUIT OPEN")
	})

	t.Run("failures string", func(t *testing.T) {
		state := FailoverState{
			Provider:            ProviderAnthropic,
			ConsecutiveFailures: 2,
			CircuitOpen:         false,
		}
		assert.Contains(t, state.String(), "failures")
	})
}

func TestRegistry_GetWithFailover(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("primary available - no failover", func(t *testing.T) {
		cfg := RegistryConfig{
			DefaultProvider:   ProviderAnthropic,
			Providers:         map[Provider]ProviderConfig{},
			FailoverEnabled:   true,
			FailoverThreshold: 3,
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		// Register primary and backup
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

		provider, failover, err := registry.GetWithFailover(ctx, ProviderAnthropic)
		assert.NoError(t, err)
		assert.Equal(t, primary, provider)
		assert.False(t, failover)
	})

	t.Run("primary unavailable - failover to backup", func(t *testing.T) {
		cfg := RegistryConfig{
			DefaultProvider:   ProviderAnthropic,
			Providers:         map[Provider]ProviderConfig{},
			FailoverEnabled:   true,
			FailoverThreshold: 3,
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)

		// Register unavailable primary and available backup
		primary := &mockLLMProvider{
			id:     ProviderAnthropic,
			name:   "Primary",
			status: ProviderStatusUnavailable,
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

		provider, failover, err := registry.GetWithFailover(ctx, ProviderAnthropic)
		assert.NoError(t, err)
		assert.Equal(t, backup, provider)
		assert.True(t, failover)
	})

	t.Run("closed registry returns error", func(t *testing.T) {
		cfg := RegistryConfig{
			Providers: map[Provider]ProviderConfig{},
		}
		registry, err := NewRegistry(cfg, log)
		require.NoError(t, err)
		registry.Close()

		_, _, err = registry.GetWithFailover(ctx, ProviderAnthropic)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "registry is closed")
	})
}

func TestRegistry_ProviderFailureTracking(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	ctx := context.Background()

	cfg := RegistryConfig{
		DefaultProvider:   ProviderAnthropic,
		Providers:         map[Provider]ProviderConfig{},
		FailoverEnabled:   true,
		FailoverThreshold: 2,
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	provider := &mockLLMProvider{
		id:     ProviderAnthropic,
		name:   "Test",
		status: ProviderStatusAvailable,
	}
	err = registry.Register(ctx, provider)
	require.NoError(t, err)

	t.Run("record failure updates state", func(t *testing.T) {
		registry.RecordProviderFailure(ProviderAnthropic, "test error")

		state := registry.GetFailoverState(ProviderAnthropic)
		assert.Equal(t, 1, state.ConsecutiveFailures)
		assert.Equal(t, "test error", state.LastFailureReason)
	})

	t.Run("record success resets failures", func(t *testing.T) {
		registry.RecordProviderFailure(ProviderAnthropic, "error")
		registry.RecordProviderSuccess(ProviderAnthropic)

		state := registry.GetFailoverState(ProviderAnthropic)
		assert.Equal(t, 0, state.ConsecutiveFailures)
		assert.Empty(t, state.LastFailureReason)
	})

	t.Run("check provider health", func(t *testing.T) {
		registry.RecordProviderFailure(ProviderAnthropic, "error")
		registry.RecordProviderFailure(ProviderAnthropic, "error")

		// Circuit should be open
		assert.False(t, registry.IsProviderHealthy(ProviderAnthropic))

		// Reset
		registry.ResetProviderFailover(ProviderAnthropic)

		// Should be healthy again
		assert.True(t, registry.IsProviderHealthy(ProviderAnthropic))
	})
}

func TestRegistry_GetAllFailoverStates(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	ctx := context.Background()

	cfg := RegistryConfig{
		Providers:       map[Provider]ProviderConfig{},
		FailoverEnabled: true,
	}
	registry, err := NewRegistry(cfg, log)
	require.NoError(t, err)

	// Register providers
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

	// Record some failures
	registry.RecordProviderFailure(ProviderAnthropic, "error 1")
	registry.RecordProviderFailure(ProviderOpenAI, "error 2")

	states := registry.GetAllFailoverStates()
	assert.Len(t, states, 2)

	assert.Contains(t, states, ProviderAnthropic)
	assert.Contains(t, states, ProviderOpenAI)
	assert.Equal(t, 1, states[ProviderAnthropic].ConsecutiveFailures)
	assert.Equal(t, 1, states[ProviderOpenAI].ConsecutiveFailures)
}
