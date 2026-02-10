package grpc

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
)

// helper function to create a test bootstrap config
func createTestBootstrapConfig(t *testing.T) (BootstrapConfig, *session.Manager, *events.Bus) {
	t.Helper()

	// Create test logger
	log, err := logger.New(config.DefaultLoggingConfig())
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	// Create session manager
	sessionCfg := config.DefaultSessionConfig()
	sessionCfg.StoragePath = t.TempDir()
	sessionMgr, err := session.New(sessionCfg, log)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	// Create event bus
	eventBus, err := events.New(log)
	if err != nil {
		t.Fatalf("failed to create event bus: %v", err)
	}

	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-grpc.sock")

	grpcCfg := config.GRPCConfig{
		SocketPath:     socketPath,
		MaxRecvMsgSize: 100 * 1024 * 1024, // 100 MB
		MaxSendMsgSize: 100 * 1024 * 1024, // 100 MB
		Timeout:        30 * time.Second,
		MaxConnections: 100,
	}

	return BootstrapConfig{
		Config:              grpcCfg,
		Logger:              log,
		SessionManager:      sessionMgr,
		EventBus:            eventBus,
		Version:             "test-1.0.0",
		ShutdownTimeout:     5 * time.Second,
		EnableHealthCheck:   false, // Disable health check for unit tests
		HealthCheckInterval: 10 * time.Second,
	}, sessionMgr, eventBus
}

func TestNewDefaultBootstrapConfig(t *testing.T) {
	grpcCfg := config.DefaultGRPCConfig()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	cfg := NewDefaultBootstrapConfig(grpcCfg, log)

	if cfg.Version != DefaultVersion {
		t.Errorf("expected version %s, got %s", DefaultVersion, cfg.Version)
	}

	if cfg.ShutdownTimeout != BootstrapDefaultShutdownTimeout {
		t.Errorf("expected shutdown timeout %v, got %v", BootstrapDefaultShutdownTimeout, cfg.ShutdownTimeout)
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
				StartedAt:  time.Now(),
				Version:    "1.0.0",
				Server:     &Server{},
				SocketPath: "/tmp/test.sock",
				Error:      nil,
			},
			contains: "1.0.0",
		},
		{
			name: "failed bootstrap",
			result: &BootstrapResult{
				StartedAt:  time.Now(),
				Version:    "1.0.0",
				Server:     nil,
				SocketPath: "/tmp/test.sock",
				Error:      types.NewError(types.ErrCodeInternal, "test error"),
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
			if tt.contains != "" && !contains(str, tt.contains) {
				t.Errorf("String() should contain %s", tt.contains)
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
				Server:     &Server{},
				Error:      nil,
				SocketPath: "/tmp/test.sock",
			},
			expected: true, // Will be false because server is not serving
		},
		{
			name: "nil server",
			result: &BootstrapResult{
				Server:     nil,
				Error:      nil,
				SocketPath: "/tmp/test.sock",
			},
			expected: false,
		},
		{
			name: "with error",
			result: &BootstrapResult{
				Server:     &Server{},
				Error:      types.NewError(types.ErrCodeInternal, "test error"),
				SocketPath: "/tmp/test.sock",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.IsSuccessful(); got != tt.expected {
				t.Logf("IsSuccessful() = %v, want %v", got, tt.expected)
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

func TestBootstrapNilDependencies(t *testing.T) {
	ctx := context.Background()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-grpc.sock")

	grpcCfg := config.GRPCConfig{
		SocketPath:     socketPath,
		MaxRecvMsgSize: 100 * 1024 * 1024,
		MaxSendMsgSize: 100 * 1024 * 1024,
		Timeout:        30 * time.Second,
		MaxConnections: 100,
	}

	// Test with nil session manager
	cfg := BootstrapConfig{
		Config:            grpcCfg,
		Logger:            log,
		SessionManager:    nil,
		EventBus:          nil,
		Version:           "1.0.0",
		EnableHealthCheck: false,
	}

	result, err := Bootstrap(ctx, cfg)
	if err == nil {
		t.Error("expected error for nil dependencies, got nil")
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

func TestBootstrapSuccess(t *testing.T) {
	ctx := context.Background()
	cfg, sessionMgr, eventBus := createTestBootstrapConfig(t)
	defer sessionMgr.Close()
	defer eventBus.Close()

	result, err := Bootstrap(ctx, cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}

	if result.Server == nil {
		t.Error("expected server to be set")
	}

	if result.Health == nil {
		t.Error("expected health server to be set")
	}

	if result.SocketPath != cfg.Config.SocketPath {
		t.Errorf("expected socket path %s, got %s", cfg.Config.SocketPath, result.SocketPath)
	}

	if result.Version != cfg.Version {
		t.Errorf("expected version %s, got %s", cfg.Version, result.Version)
	}

	// Verify server is serving
	if !result.Server.IsServing() {
		t.Error("expected server to be serving")
	}

	// Verify health check service is registered
	statuses := result.Health.GetAllStatuses()
	expectedServices := []string{"orchestrator", "agent", "gateway"}
	for _, service := range expectedServices {
		if status, ok := statuses[service]; !ok {
			t.Errorf("expected service %s to be registered", service)
		} else if status.String() != "SERVING" {
			t.Errorf("expected service %s to be SERVING, got %s", service, status.String())
		}
	}

	// Cleanup
	if err := result.Server.Stop(); err != nil {
		t.Errorf("failed to stop server: %v", err)
	}
}

func TestBootstrapWithDefaults(t *testing.T) {
	ctx := context.Background()

	// Create dependencies
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	sessionCfg := config.DefaultSessionConfig()
	sessionCfg.StoragePath = t.TempDir()
	sessionMgr, err := session.New(sessionCfg, log)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	defer sessionMgr.Close()

	eventBus, err := events.New(log)
	if err != nil {
		t.Fatalf("failed to create event bus: %v", err)
	}
	defer eventBus.Close()

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-grpc.sock")

	grpcCfg := config.GRPCConfig{
		SocketPath:     socketPath,
		MaxRecvMsgSize: 100 * 1024 * 1024,
		MaxSendMsgSize: 100 * 1024 * 1024,
		Timeout:        30 * time.Second,
		MaxConnections: 100,
	}

	result, err := BootstrapWithDefaults(ctx, grpcCfg, sessionMgr, eventBus)
	if err != nil {
		t.Fatalf("BootstrapWithDefaults failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}

	if result.Server == nil {
		t.Error("expected server to be set")
	}

	// Cleanup
	if err := result.Server.Stop(); err != nil {
		t.Errorf("failed to stop server: %v", err)
	}
}

func TestIsReady(t *testing.T) {
	tests := []struct {
		name     string
		server   *Server
		expected bool
	}{
		{
			name:     "nil server",
			server:   nil,
			expected: false,
		},
		{
			name:     "not serving",
			server:   &Server{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			got := IsReady(ctx, tt.server)
			if got != tt.expected {
				t.Errorf("IsReady() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWaitForReadyNilServer(t *testing.T) {
	ctx := context.Background()
	err := WaitForReady(ctx, nil, 1*time.Second, 100*time.Millisecond)
	if err == nil {
		t.Error("expected error for nil server")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
		t.Errorf("expected ErrCodeInvalidArgument, got %v", types.GetErrorCode(err))
	}
}

func TestWaitForReadyTimeout(t *testing.T) {
	ctx := context.Background()
	server := &Server{}

	err := WaitForReady(ctx, server, 500*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}

	if !types.IsErrCode(err, types.ErrCodeTimeout) {
		t.Errorf("expected ErrCodeTimeout, got %v", types.GetErrorCode(err))
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

func TestGlobalGRPCServer(t *testing.T) {
	// Save and restore original global state
	originalServer := globalServer
	originalHealth := globalHealth
	originalResult := globalBootstrap
	defer func() {
		globalServer = originalServer
		globalHealth = originalHealth
		globalBootstrap = originalResult
		// Reset sync.Once to prevent test pollution
		globalOnce = sync.Once{}
	}()

	// Reset global state
	globalServer = nil
	globalHealth = nil
	globalBootstrap = nil
	globalOnce = sync.Once{}

	t.Run("IsGlobalInitialized returns false initially", func(t *testing.T) {
		if IsGlobalInitialized() {
			t.Error("expected global gRPC server to not be initialized")
		}
	})

	t.Run("SetGlobal and IsGlobalInitialized", func(t *testing.T) {
		server := &Server{}
		health, err := NewHealthServer(HealthServerConfig{}, nil)
		if err != nil {
			t.Fatalf("failed to create health server: %v", err)
		}
		result := &BootstrapResult{}

		SetGlobal(server, health, result)

		if !IsGlobalInitialized() {
			t.Error("expected global gRPC server to be initialized after SetGlobal")
		}

		got := Global()
		if got != server {
			t.Error("Global() did not return the set server")
		}

		gotHealth := GlobalHealth()
		if gotHealth != health {
			t.Error("GlobalHealth() did not return the set health server")
		}

		gotResult := GlobalResult()
		if gotResult != result {
			t.Error("GlobalResult() did not return the set result")
		}
	})

	t.Run("CloseGlobal with nil server", func(t *testing.T) {
		SetGlobal(nil, nil, nil)
		err := CloseGlobal()
		if err != nil {
			t.Errorf("CloseGlobal() with nil should not error, got %v", err)
		}

		if IsGlobalInitialized() {
			t.Error("expected global gRPC server to not be initialized after CloseGlobal with nil")
		}
	})
}

func TestValidateDependencies(t *testing.T) {
	tests := []struct {
		name        string
		cfg         BootstrapConfig
		expectError bool
	}{
		{
			name: "valid dependencies",
			cfg: BootstrapConfig{
				SessionManager: &session.Manager{},
				EventBus:       &events.Bus{},
			},
			expectError: false,
		},
		{
			name: "nil session manager",
			cfg: BootstrapConfig{
				SessionManager: nil,
				EventBus:       &events.Bus{},
			},
			expectError: true,
		},
		{
			name: "nil event bus",
			cfg: BootstrapConfig{
				SessionManager: &session.Manager{},
				EventBus:       nil,
			},
			expectError: true,
		},
		{
			name:        "both nil",
			cfg:         BootstrapConfig{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDependencies(tt.cfg)
			if (err != nil) != tt.expectError {
				t.Errorf("validateDependencies() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestServiceDependencies(t *testing.T) {
	sessionMgr := &session.Manager{}
	eventBus := &events.Bus{}

	deps := &serviceDependencies{
		sessionManager: sessionMgr,
		eventBus:       eventBus,
	}

	if deps.SessionManager() != sessionMgr {
		t.Error("SessionManager() did not return the set session manager")
	}

	if deps.EventBus() != eventBus {
		t.Error("EventBus() did not return the set event bus")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark tests

func BenchmarkBootstrapResultString(b *testing.B) {
	result := &BootstrapResult{
		StartedAt:  time.Now(),
		Version:    "1.0.0",
		Server:     &Server{},
		SocketPath: "/tmp/test.sock",
		Error:      nil,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = result.String()
	}
}

func BenchmarkIsReady(b *testing.B) {
	server := &Server{}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsReady(ctx, server)
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

// TestBootstrapIntegration tests the full bootstrap flow with actual gRPC client
func TestBootstrapIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	cfg, sessionMgr, eventBus := createTestBootstrapConfig(t)
	defer sessionMgr.Close()
	defer eventBus.Close()

	result, err := Bootstrap(ctx, cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	defer func() {
		if result.Server != nil {
			_ = result.Server.Stop()
		}
	}()

	// Test that we can connect to the server
	client, err := NewClient(result.SocketPath, ClientConfig{
		DialTimeout: 5 * time.Second,
	}, cfg.Logger)
	if err != nil {
		t.Fatalf("Failed to create gRPC client: %v", err)
	}
	defer client.Close()

	if err := client.Dial(ctx); err != nil {
		t.Fatalf("Failed to connect to gRPC server: %v", err)
	}

	// Test health check
	healthStatus, err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if healthStatus.Status != grpc_health.HealthCheckResponse_SERVING {
		t.Errorf("Expected SERVING status, got %s", healthStatus.Status.String())
	}

	// Close client
	if err := client.Close(); err != nil {
		t.Errorf("Failed to close client: %v", err)
	}

	// Close server
	if err := result.Server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}

	// Verify server is no longer serving
	if result.Server.IsServing() {
		t.Error("Expected server to not be serving after stop")
	}
}
