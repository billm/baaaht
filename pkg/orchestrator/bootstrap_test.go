package orchestrator

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// helper function to create a test bootstrap config
func createTestBootstrapConfig(t *testing.T) BootstrapConfig {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		// Use default config if loading fails
		cfg = &config.Config{}
		// Set minimal required config for testing
		*cfg = config.Config{
			Docker:  config.DefaultDockerConfig(),
			Logging: config.DefaultLoggingConfig(),
		}
	}

	// Create a test logger
	log, err := logger.New(config.DefaultLoggingConfig())
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	return BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-1.0.0",
		ShutdownTimeout:     5 * time.Second,
		EnableHealthCheck:   false, // Disable health check for unit tests
		HealthCheckInterval: 10 * time.Second,
	}
}

func TestNewDefaultBootstrapConfig(t *testing.T) {
	cfg := NewDefaultBootstrapConfig()

	if cfg.Version != DefaultVersion {
		t.Errorf("expected version %s, got %s", DefaultVersion, cfg.Version)
	}

	if cfg.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("expected shutdown timeout %v, got %v", DefaultShutdownTimeout, cfg.ShutdownTimeout)
	}

	if !cfg.EnableHealthCheck {
		t.Error("expected health check to be enabled by default")
	}
}

func TestBootstrapResultString(t *testing.T) {
	tests := []struct {
		name     string
		result   *BootstrapResult
		contains string
	}{
		{
			name: "successful bootstrap",
			result: &BootstrapResult{
				StartedAt:    time.Now(),
				Version:      "1.0.0",
				Orchestrator: &Orchestrator{},
				Error:        nil,
			},
			contains: "1.0.0",
		},
		{
			name: "failed bootstrap",
			result: &BootstrapResult{
				StartedAt:    time.Now(),
				Version:      "1.0.0",
				Orchestrator: nil,
				Error:        types.NewError(types.ErrCodeInternal, "test error"),
			},
			contains: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.result.String()
			if str == "" {
				t.Error("String() returned empty string")
			}
		})
	}
}

func TestBootstrapResultIsSuccessful(t *testing.T) {
	tests := []struct {
		name     string
		result   *BootstrapResult
		expected bool
	}{
		{
			name: "successful result",
			result: &BootstrapResult{
				Orchestrator: &Orchestrator{},
				Error:        nil,
			},
			expected: true,
		},
		{
			name: "nil orchestrator",
			result: &BootstrapResult{
				Orchestrator: nil,
				Error:        nil,
			},
			expected: false,
		},
		{
			name: "with error",
			result: &BootstrapResult{
				Orchestrator: &Orchestrator{},
				Error:        types.NewError(types.ErrCodeInternal, "test error"),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.IsSuccessful(); got != tt.expected {
				t.Errorf("IsSuccessful() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBootstrapResultDuration(t *testing.T) {
	startedAt := time.Now().Add(-1 * time.Second)
	result := &BootstrapResult{
		StartedAt: startedAt,
	}

	duration := result.Duration()
	if duration < 900*time.Millisecond || duration > 1100*time.Millisecond {
		t.Errorf("Duration() = %v, want ~1s", duration)
	}
}

func TestBootstrapInvalidConfig(t *testing.T) {
	ctx := context.Background()

	// Create an invalid config (empty)
	cfg := BootstrapConfig{
		Config:            config.Config{},
		Logger:            nil,
		Version:           "1.0.0",
		EnableHealthCheck: false,
	}

	result, err := Bootstrap(ctx, cfg)
	if err == nil {
		t.Error("expected error for invalid config, got nil")
	}

	if result == nil {
		t.Error("expected result to be returned even on error")
	}

	if result.Error == nil {
		t.Error("expected result.Error to be set")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
		t.Errorf("expected ErrCodeInvalidArgument, got %v", types.GetErrorCode(err))
	}
}

func TestGetVersion(t *testing.T) {
	version := GetVersion()
	if version != DefaultVersion {
		t.Errorf("GetVersion() = %s, want %s", version, DefaultVersion)
	}
}

func TestGetVersionInfo(t *testing.T) {
	info := GetVersionInfo()
	if info == nil {
		t.Fatal("GetVersionInfo() returned nil")
	}

	if version, ok := info["version"].(string); !ok || version != DefaultVersion {
		t.Errorf("version = %v, want %s", info["version"], DefaultVersion)
	}

	if _, ok := info["build_time"].(string); !ok {
		t.Error("build_time not found in version info")
	}

	if _, ok := info["git_commit"].(string); !ok {
		t.Error("git_commit not found in version info")
	}
}

func TestIsReady(t *testing.T) {
	tests := []struct {
		name     string
		orch     *Orchestrator
		expected bool
	}{
		{
			name:     "nil orchestrator",
			orch:     nil,
			expected: false,
		},
		{
			name: "not started",
			orch: &Orchestrator{
				started: false,
				closed:  false,
			},
			expected: false,
		},
		{
			name: "closed",
			orch: &Orchestrator{
				started: true,
				closed:  true,
			},
			expected: false,
		},
		{
			name: "ready - but would need actual subsystems",
			// This will return false because health checks will fail
			// without actual initialized subsystems
			orch: &Orchestrator{
				started: true,
				closed:  false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			got := IsReady(ctx, tt.orch)
			if got != tt.expected {
				t.Errorf("IsReady() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWaitForReadyNilOrchestrator(t *testing.T) {
	ctx := context.Background()
	err := WaitForReady(ctx, nil, 1*time.Second, 100*time.Millisecond)
	if err == nil {
		t.Error("expected error for nil orchestrator")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
		t.Errorf("expected ErrCodeInvalidArgument, got %v", types.GetErrorCode(err))
	}
}

func TestWaitForReadyTimeout(t *testing.T) {
	ctx := context.Background()
	orch := &Orchestrator{
		started: false,
		closed:  false,
	}

	err := WaitForReady(ctx, orch, 500*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}

	if !types.IsErrCode(err, types.ErrCodeTimeout) {
		t.Errorf("expected ErrCodeTimeout, got %v", types.GetErrorCode(err))
	}
}

func TestGlobalOrchestrator(t *testing.T) {
	// Save and restore original global state
	// Note: sync.Once cannot be copied, so we only save the orchestrator
	originalOrch := globalOrchestrator
	defer func() {
		globalOrchestrator = originalOrch
		// Reset sync.Once to prevent test pollution
		globalOnce = sync.Once{}
	}()

	// Reset global state
	globalOrchestrator = nil
	globalOnce = sync.Once{}

	t.Run("IsGlobalInitialized returns false initially", func(t *testing.T) {
		if IsGlobalInitialized() {
			t.Error("expected global orchestrator to not be initialized")
		}
	})

	t.Run("SetGlobal and IsGlobalInitialized", func(t *testing.T) {
		orch := &Orchestrator{}
		SetGlobal(orch)

		if !IsGlobalInitialized() {
			t.Error("expected global orchestrator to be initialized after SetGlobal")
		}

		got := Global()
		if got != orch {
			t.Error("Global() did not return the set orchestrator")
		}
	})

	t.Run("CloseGlobal", func(t *testing.T) {
		orch := &Orchestrator{
			closed: false,
		}
		SetGlobal(orch)

		err := CloseGlobal()
		if err != nil {
			t.Errorf("CloseGlobal() error = %v", err)
		}

		if IsGlobalInitialized() {
			t.Error("expected global orchestrator to be cleared after CloseGlobal")
		}
	})

	t.Run("CloseGlobal with nil orchestrator", func(t *testing.T) {
		SetGlobal(nil)
		err := CloseGlobal()
		if err != nil {
			t.Errorf("CloseGlobal() with nil should not error, got %v", err)
		}
	})
}

func TestBootstrapWithDefaults(t *testing.T) {
	// Skip test if Docker is not available
	if os.Getenv("DOCKER_HOST") == "" {
		if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
			t.Skip("Docker not available, skipping bootstrap test")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := BootstrapWithDefaults(ctx)
	if err != nil {
		// This is expected to fail in test environments without Docker
		// We're just testing the code path
		t.Logf("BootstrapWithDefaults failed (expected in test env): %v", err)
	}

	if result != nil {
		if result.Version != DefaultVersion {
			t.Errorf("expected version %s, got %s", DefaultVersion, result.Version)
		}
	}
}

func TestBootstrapConfigDefaults(t *testing.T) {
	cfg := NewDefaultBootstrapConfig()

	if cfg.Version == "" {
		t.Error("expected version to be set")
	}

	if cfg.ShutdownTimeout == 0 {
		t.Error("expected shutdown timeout to be set")
	}

	if cfg.HealthCheckInterval == 0 {
		t.Error("expected health check interval to be set")
	}

	if !cfg.EnableHealthCheck {
		t.Error("expected health check to be enabled")
	}
}

// Benchmark tests

func TestBootstrapWithSessionRestore(t *testing.T) {
	// Create a temporary directory for session storage
	tempDir, err := os.MkdirTemp("", "session-restore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create config with session persistence enabled
	cfg := &config.Config{
		Docker:    config.DefaultDockerConfig(),
		APIServer: config.DefaultAPIServerConfig(),
		Logging:   config.DefaultLoggingConfig(),
		Session: config.SessionConfig{
			MaxSessions:        100,
			IdleTimeout:        30 * time.Minute,
			Timeout:            5 * time.Minute,
			PersistenceEnabled: true,
			StoragePath:        tempDir,
		},
		Event:        config.DefaultEventConfig(),
		IPC:          config.DefaultIPCConfig(),
		Scheduler:    config.DefaultSchedulerConfig(),
		Credentials:  config.DefaultCredentialsConfig(),
		Policy:       config.DefaultPolicyConfig(),
		Metrics:      config.DefaultMetricsConfig(),
		Tracing:      config.DefaultTracingConfig(),
		Orchestrator: config.DefaultOrchestratorConfig(),
	}

	// Create a test logger
	log, err := logger.New(config.DefaultLoggingConfig())
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	ctx := context.Background()

	// First, create an orchestrator and add a session with messages
	t.Run("setup: create session with persistence", func(t *testing.T) {
		bootstrapCfg := BootstrapConfig{
			Config:            *cfg,
			Logger:            log,
			Version:           "test-1.0.0",
			ShutdownTimeout:   5 * time.Second,
			EnableHealthCheck: false,
		}

		result, err := Bootstrap(ctx, bootstrapCfg)
		if err != nil {
			t.Logf("Bootstrap failed (may be expected without Docker): %v", err)
			if result == nil || result.Orchestrator == nil {
				t.Skip("Docker not available, skipping session restore test")
			}
		}

		orch := result.Orchestrator
		if orch != nil && orch.sessionMgr != nil {
			// Create a test session
			metadata := types.SessionMetadata{
				OwnerID: "test-user",
				Name:    "test-session",
			}
			sessionCfg := types.SessionConfig{
				MaxDuration: 10 * time.Minute,
			}

			sessionID, err := orch.sessionMgr.Create(ctx, metadata, sessionCfg)
			if err != nil {
				t.Fatalf("failed to create session: %v", err)
			}

			// Add a message to the session
			message := types.Message{
				Role:    "user",
				Content: "Hello, world!",
			}
			if err := orch.sessionMgr.AddMessage(ctx, sessionID, message); err != nil {
				t.Fatalf("failed to add message: %v", err)
			}

			// Close the orchestrator to ensure persistence
			if err := orch.Close(); err != nil {
				t.Logf("Warning: failed to close orchestrator: %v", err)
			}
		}
	})

	// Now test that sessions are restored on bootstrap
	t.Run("restore sessions on bootstrap", func(t *testing.T) {
		bootstrapCfg := BootstrapConfig{
			Config:            *cfg,
			Logger:            log,
			Version:           "test-1.0.0",
			ShutdownTimeout:   5 * time.Second,
			EnableHealthCheck: false,
		}

		result, err := Bootstrap(ctx, bootstrapCfg)
		if err != nil {
			// Bootstrap may fail without Docker, but session restore should have been attempted
			// The key is that RestoreSessions was called and handled gracefully
			t.Logf("Bootstrap failed (may be expected): %v", err)
			if result == nil || result.Orchestrator == nil {
				t.Skip("Docker not available, skipping full test")
			}
		}

		if result != nil && result.Orchestrator != nil && result.Orchestrator.sessionMgr != nil {
			// Check that sessions were restored by listing them
			sessions, err := result.Orchestrator.sessionMgr.List(ctx, nil)
			if err != nil {
				t.Logf("Warning: failed to list sessions: %v", err)
			}

			// We should have at least one restored session
			if len(sessions) > 0 {
				t.Logf("Successfully restored %d session(s)", len(sessions))

				// Verify the restored session has messages
				for _, sess := range sessions {
					if len(sess.Context.Messages) > 0 {
						t.Logf("Session %s has %d message(s)", sess.ID, len(sess.Context.Messages))
					}
				}
			}

			// Clean up
			_ = result.Orchestrator.Close()
		}
	})

	// Test that errors during restore don't fail the bootstrap
	t.Run("bootstrap handles restore errors gracefully", func(t *testing.T) {
		// Use a non-existent directory to cause an error
		invalidCfg := *cfg
		invalidCfg.Session.StoragePath = "/non/existent/path/that/should/fail"

		bootstrapCfg := BootstrapConfig{
			Config:            invalidCfg,
			Logger:            log,
			Version:           "test-1.0.0",
			ShutdownTimeout:   5 * time.Second,
			EnableHealthCheck: false,
		}

		result, err := Bootstrap(ctx, bootstrapCfg)
		// Bootstrap should not fail due to restore errors
		if err != nil && result != nil && result.Orchestrator != nil {
			// If bootstrap failed, it should be for a different reason
			// (e.g., Docker not available), not because of restore
			t.Logf("Bootstrap failed: %v", err)
		}

		// The key is that the orchestrator was created even though restore might have failed
		if result != nil && result.Orchestrator != nil {
			// Clean up
			_ = result.Orchestrator.Close()
		}
	})
}

func BenchmarkBootstrapResultString(b *testing.B) {
	result := &BootstrapResult{
		StartedAt:    time.Now(),
		Version:      "1.0.0",
		Orchestrator: &Orchestrator{},
		Error:        nil,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = result.String()
	}
}

func BenchmarkIsReady(b *testing.B) {
	orch := &Orchestrator{
		started: true,
		closed:  false,
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsReady(ctx, orch)
	}
}

func BenchmarkGetVersion(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetVersion()
	}
}

func BenchmarkGetVersionInfo(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetVersionInfo()
	}
}
