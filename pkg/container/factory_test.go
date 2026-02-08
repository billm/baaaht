package container

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// TestFactory is a comprehensive test for the runtime factory functionality
func TestFactory(t *testing.T) {
	// Test initializing runtime factories
	t.Run("InitializeRuntimes", func(t *testing.T) {
		log, err := logger.NewDefault()
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		// Clear global state
		globalRegistry = nil
		globalRegistryOnce = *(new(sync.Once))

		// Initialize runtimes
		if err := InitializeRuntimes(log); err != nil {
			t.Fatalf("Failed to initialize runtimes: %v", err)
		}

		reg := GetGlobalRegistry()

		// Check that Docker and Apple are registered
		if !reg.HasRuntime(string(types.RuntimeTypeDocker)) {
			t.Error("Docker runtime not registered")
		}

		if !reg.HasRuntime(string(types.RuntimeTypeAppleContainers)) {
			t.Error("Apple Containers runtime not registered")
		}

		// Check that we have 2 runtimes registered
		types := reg.ListRuntimes()
		if len(types) != 2 {
			t.Errorf("expected 2 registered runtimes, got %d", len(types))
		}
	})

	// Test creating a runtime with explicit docker type
	t.Run("DockerRuntime", func(t *testing.T) {
		ctx := context.Background()
		log, err := logger.NewDefault()
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 10 * time.Second,
		}

		rt, err := NewRuntime(ctx, cfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
			return
		}

		if rt == nil {
			t.Fatal("NewRuntime returned nil runtime")
		}

		if rt.Type() != string(types.RuntimeTypeDocker) {
			t.Errorf("expected runtime type 'docker', got '%s'", rt.Type())
		}

		// Verify it's a DockerRuntime
		if drt, ok := rt.(*DockerRuntime); !ok {
			t.Error("runtime is not a *DockerRuntime")
		} else {
			if drt.IsClosed() {
				t.Error("Docker runtime is closed immediately after creation")
			}
		}

		// Close the runtime
		if err := rt.Close(); err != nil {
			t.Errorf("Failed to close runtime: %v", err)
		}
	})

	// Test creating a runtime with invalid type
	t.Run("InvalidRuntimeType", func(t *testing.T) {
		ctx := context.Background()
		log, err := logger.NewDefault()
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := RuntimeConfig{
			Type:    "invalid_runtime_type",
			Logger:  log,
			Timeout: 10 * time.Second,
		}

		_, err = NewRuntime(ctx, cfg)
		if err == nil {
			t.Error("expected error when creating runtime with invalid type")
		}

		if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
			t.Errorf("expected ErrCodeInvalidArgument, got %v", err)
		}
	})

	// Test creating a runtime with auto-detection
	t.Run("AutoRuntimeDetection", func(t *testing.T) {
		ctx := context.Background()
		log, err := logger.NewDefault()
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := RuntimeConfig{
			Type:    string(types.RuntimeTypeAuto),
			Logger:  log,
			Timeout: 10 * time.Second,
		}

		rt, err := NewRuntime(ctx, cfg)
		if err != nil {
			t.Logf("NewRuntime with auto failed (may be expected if Docker not available): %v", err)
			// This is not necessarily a failure - Docker might not be available
			return
		}

		if rt == nil {
			t.Fatal("NewRuntime returned nil runtime")
		}

		// Verify the runtime type is either docker or apple
		rtType := rt.Type()
		if rtType != string(types.RuntimeTypeDocker) && rtType != string(types.RuntimeTypeAppleContainers) {
			t.Errorf("expected runtime type 'docker' or 'apple', got '%s'", rtType)
		}

		// Close the runtime
		if err := rt.Close(); err != nil {
			t.Errorf("Failed to close runtime: %v", err)
		}
	})

	// Test getting runtime from registry
	t.Run("GetRuntimeFromRegistry", func(t *testing.T) {
		ctx := context.Background()
		log, err := logger.NewDefault()
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		// Clear global state
		globalRegistry = nil
		globalRegistryOnce = *(new(sync.Once))

		cfg := RuntimeConfig{
			Type:    string(types.RuntimeTypeDocker),
			Logger:  log,
			Timeout: 10 * time.Second,
		}

		rt, err := GetRuntimeFromRegistry(ctx, string(types.RuntimeTypeDocker), cfg)
		if err != nil {
			t.Skipf("Docker runtime not available: %v", err)
			return
		}

		if rt == nil {
			t.Fatal("GetRuntimeFromRegistry returned nil runtime")
		}

		if rt.Type() != string(types.RuntimeTypeDocker) {
			t.Errorf("expected runtime type 'docker', got '%s'", rt.Type())
		}

		// Close the runtime
		if err := rt.Close(); err != nil {
			t.Errorf("Failed to close runtime: %v", err)
		}
	})
}

// TestNewRuntimeAuto tests creating a runtime with auto-detection
func TestNewRuntimeAuto(t *testing.T) {
	ctx := context.Background()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := RuntimeConfig{
		Type:    string(types.RuntimeTypeAuto),
		Logger:  log,
		Timeout: 10 * time.Second,
	}

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Logf("NewRuntime with auto failed (may be expected if Docker not available): %v", err)
		// This is not necessarily a failure - Docker might not be available
		return
	}

	if rt == nil {
		t.Fatal("NewRuntime returned nil runtime")
	}

	// Verify the runtime type is either docker or apple
	rtType := rt.Type()
	if rtType != string(types.RuntimeTypeDocker) && rtType != string(types.RuntimeTypeAppleContainers) {
		t.Errorf("expected runtime type 'docker' or 'apple', got '%s'", rtType)
	}

	// Close the runtime
	if err := rt.Close(); err != nil {
		t.Errorf("Failed to close runtime: %v", err)
	}
}

// TestNewRuntimeDocker tests creating a Docker runtime explicitly
func TestNewRuntimeDocker(t *testing.T) {
	ctx := context.Background()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := RuntimeConfig{
		Type:    string(types.RuntimeTypeDocker),
		Logger:  log,
		Timeout: 10 * time.Second,
	}

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Skipf("Docker runtime not available: %v", err)
		return
	}

	if rt == nil {
		t.Fatal("NewRuntime returned nil runtime")
	}

	if rt.Type() != string(types.RuntimeTypeDocker) {
		t.Errorf("expected runtime type 'docker', got '%s'", rt.Type())
	}

	// Verify it's a DockerRuntime
	if drt, ok := rt.(*DockerRuntime); !ok {
		t.Error("runtime is not a *DockerRuntime")
	} else {
		if drt.IsClosed() {
			t.Error("Docker runtime is closed immediately after creation")
		}
	}

	// Close the runtime
	if err := rt.Close(); err != nil {
		t.Errorf("Failed to close runtime: %v", err)
	}
}

// TestNewRuntimeInvalid tests creating a runtime with invalid type
func TestNewRuntimeInvalid(t *testing.T) {
	ctx := context.Background()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := RuntimeConfig{
		Type:    "invalid_runtime_type",
		Logger:  log,
		Timeout: 10 * time.Second,
	}

	_, err = NewRuntime(ctx, cfg)
	if err == nil {
		t.Error("expected error when creating runtime with invalid type")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
		t.Errorf("expected ErrCodeInvalidArgument, got %v", err)
	}
}

// TestNewRuntimeNilLogger tests creating a runtime with nil logger
func TestNewRuntimeNilLogger(t *testing.T) {
	ctx := context.Background()

	cfg := RuntimeConfig{
		Type:    string(types.RuntimeTypeDocker),
		Logger:  nil,
		Timeout: 10 * time.Second,
	}

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Skipf("Docker runtime not available: %v", err)
		return
	}

	if rt == nil {
		t.Fatal("NewRuntime returned nil runtime")
	}

	// Close the runtime
	if err := rt.Close(); err != nil {
		t.Errorf("Failed to close runtime: %v", err)
	}
}

// TestNewRuntimeNoTimeout tests creating a runtime with zero timeout
func TestNewRuntimeNoTimeout(t *testing.T) {
	ctx := context.Background()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := RuntimeConfig{
		Type:    string(types.RuntimeTypeDocker),
		Logger:  log,
		Timeout: 0, // Zero timeout - should use default
	}

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Skipf("Docker runtime not available: %v", err)
		return
	}

	if rt == nil {
		t.Fatal("NewRuntime returned nil runtime")
	}

	// Close the runtime
	if err := rt.Close(); err != nil {
		t.Errorf("Failed to close runtime: %v", err)
	}
}

// TestNewRuntimeDefault tests creating a runtime with default settings
func TestNewRuntimeDefault(t *testing.T) {
	ctx := context.Background()

	rt, err := NewRuntimeDefault(ctx)
	if err != nil {
		t.Skipf("No runtime available: %v", err)
		return
	}

	if rt == nil {
		t.Fatal("NewRuntimeDefault returned nil runtime")
	}

	// Close the runtime
	if err := rt.Close(); err != nil {
		t.Errorf("Failed to close runtime: %v", err)
	}
}

// TestInitializeRuntimes tests initializing runtime factories
func TestInitializeRuntimes(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Clear global state
	globalRegistry = nil
	globalRegistryOnce = *(new(sync.Once))

	// Initialize runtimes
	if err := InitializeRuntimes(log); err != nil {
		t.Fatalf("Failed to initialize runtimes: %v", err)
	}

	reg := GetGlobalRegistry()

	// Check that Docker and Apple are registered
	if !reg.HasRuntime(string(types.RuntimeTypeDocker)) {
		t.Error("Docker runtime not registered")
	}

	if !reg.HasRuntime(string(types.RuntimeTypeAppleContainers)) {
		t.Error("Apple Containers runtime not registered")
	}

	// Check that we have 2 runtimes registered
	types := reg.ListRuntimes()
	if len(types) != 2 {
		t.Errorf("expected 2 registered runtimes, got %d", len(types))
	}
}

// TestInitializeRuntimesNilLogger tests initializing with nil logger
func TestInitializeRuntimesNilLogger(t *testing.T) {
	// Clear global state
	globalRegistry = nil
	globalRegistryOnce = *(new(sync.Once))

	// Initialize runtimes with nil logger
	if err := InitializeRuntimes(nil); err != nil {
		t.Fatalf("Failed to initialize runtimes with nil logger: %v", err)
	}

	reg := GetGlobalRegistry()

	// Check that runtimes are still registered
	if !reg.HasRuntime(string(types.RuntimeTypeDocker)) {
		t.Error("Docker runtime not registered")
	}
}

// TestGetRuntimeFromRegistry tests getting a runtime from the registry
func TestGetRuntimeFromRegistry(t *testing.T) {
	ctx := context.Background()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Clear global state
	globalRegistry = nil
	globalRegistryOnce = *(new(sync.Once))

	cfg := RuntimeConfig{
		Type:    string(types.RuntimeTypeDocker),
		Logger:  log,
		Timeout: 10 * time.Second,
	}

	rt, err := GetRuntimeFromRegistry(ctx, string(types.RuntimeTypeDocker), cfg)
	if err != nil {
		t.Skipf("Docker runtime not available: %v", err)
		return
	}

	if rt == nil {
		t.Fatal("GetRuntimeFromRegistry returned nil runtime")
	}

	if rt.Type() != string(types.RuntimeTypeDocker) {
		t.Errorf("expected runtime type 'docker', got '%s'", rt.Type())
	}

	// Close the runtime
	if err := rt.Close(); err != nil {
		t.Errorf("Failed to close runtime: %v", err)
	}
}

// TestCreateRuntimeWithConfig tests creating a runtime with config helper
func TestCreateRuntimeWithConfig(t *testing.T) {
	ctx := context.Background()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	rt, err := CreateRuntimeWithConfig(ctx, string(types.RuntimeTypeDocker), log)
	if err != nil {
		t.Skipf("Docker runtime not available: %v", err)
		return
	}

	if rt == nil {
		t.Fatal("CreateRuntimeWithConfig returned nil runtime")
	}

	// Close the runtime
	if err := rt.Close(); err != nil {
		t.Errorf("Failed to close runtime: %v", err)
	}
}

// TestRegisterCustomRuntime tests registering a custom runtime
func TestRegisterCustomRuntime(t *testing.T) {
	// Clear global state
	globalRegistry = nil
	globalRegistryOnce = *(new(sync.Once))

	// Register a custom runtime
	customFactory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "custom"}, nil
	}

	if err := RegisterCustomRuntime("custom", customFactory); err != nil {
		t.Fatalf("Failed to register custom runtime: %v", err)
	}

	reg := GetGlobalRegistry()

	if !reg.HasRuntime("custom") {
		t.Error("Custom runtime not registered")
	}

	// Get the custom runtime
	rt, err := reg.GetRuntime("custom", RuntimeConfig{})
	if err != nil {
		t.Fatalf("Failed to get custom runtime: %v", err)
	}

	if rt.Type() != "custom" {
		t.Errorf("expected runtime type 'custom', got '%s'", rt.Type())
	}
}

// TestAvailableRuntimes tests listing available runtimes
func TestAvailableRuntimes(t *testing.T) {
	ctx := context.Background()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Clear global state
	globalRegistry = nil
	globalRegistryOnce = *(new(sync.Once))

	// Initialize runtimes
	_ = InitializeRuntimes(log)

	available := AvailableRuntimes(ctx)

	// We should have at least Docker if it's available
	dockerAvailable := false
	for _, rt := range available {
		if rt == string(types.RuntimeTypeDocker) {
			dockerAvailable = true
			break
		}
	}

	// Check if Docker is actually available on the system
	expectedDockerAvailable := IsDockerAvailable(ctx)
	if dockerAvailable != expectedDockerAvailable {
		t.Errorf("Docker availability mismatch: got %v, expected %v", dockerAvailable, expectedDockerAvailable)
	}
}

// TestDetectBestRuntime tests detecting the best runtime
func TestDetectBestRuntime(t *testing.T) {
	ctx := context.Background()

	// Clear global state
	globalRegistry = nil
	globalRegistryOnce = *(new(sync.Once))

	bestRuntime := DetectBestRuntime(ctx)

	// Best runtime should be either docker, apple, or auto (if none available)
	if bestRuntime != types.RuntimeTypeDocker &&
		bestRuntime != types.RuntimeTypeAppleContainers &&
		bestRuntime != types.RuntimeTypeAuto {
		t.Errorf("unexpected best runtime type: %s", bestRuntime)
	}

	t.Logf("Best runtime detected: %s", bestRuntime)
}

// TestRuntimeConfigToDockerConfig tests converting RuntimeConfig to DockerConfig
func TestRuntimeConfigToDockerConfig(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	tests := []struct {
		name     string
		cfg      RuntimeConfig
		wantHost string
	}{
		{
			name: "default host",
			cfg: RuntimeConfig{
				Type:   string(types.RuntimeTypeDocker),
				Logger: log,
			},
			wantHost: config.DefaultDockerConfig().Host,
		},
		{
			name: "custom host",
			cfg: RuntimeConfig{
				Type:     string(types.RuntimeTypeDocker),
				Logger:   log,
				Endpoint: "tcp://localhost:2375",
			},
			wantHost: "tcp://localhost:2375",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := NewRuntime(context.Background(), tt.cfg)
			if err != nil {
				t.Skipf("Docker runtime not available: %v", err)
				return
			}

			if rt == nil {
				t.Fatal("NewRuntime returned nil")
			}

			// Get the Docker client and check the host
			if drt, ok := rt.(*DockerRuntime); ok {
				client := drt.DockerClient()
				if client == nil {
					t.Fatal("Docker client is nil")
				}
				cfg := client.Config()
				if cfg.Host != tt.wantHost {
					t.Errorf("expected host %s, got %s", tt.wantHost, cfg.Host)
				}
			} else {
				t.Error("runtime is not a *DockerRuntime")
			}

			// Close the runtime
			_ = rt.Close()
		})
	}
}

// TestRuntimeConfigWithOptions tests RuntimeConfig with options
func TestRuntimeConfigWithOptions(t *testing.T) {
	ctx := context.Background()
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := RuntimeConfig{
		Type:    string(types.RuntimeTypeDocker),
		Logger:  log,
		Timeout: 10 * time.Second,
		Options: map[string]interface{}{
			"tls_cert":   "/path/to/cert.pem",
			"tls_key":    "/path/to/key.pem",
			"tls_ca":     "/path/to/ca.pem",
			"tls_verify": true,
		},
	}

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Skipf("Docker runtime not available: %v", err)
		return
	}

	if rt == nil {
		t.Fatal("NewRuntime returned nil runtime")
	}

	// Verify TLS settings were applied
	if drt, ok := rt.(*DockerRuntime); ok {
		client := drt.DockerClient()
		if client == nil {
			t.Fatal("Docker client is nil")
		}
		dockerCfg := client.Config()
		if dockerCfg.TLSCert != "/path/to/cert.pem" {
			t.Errorf("expected TLS cert /path/to/cert.pem, got %s", dockerCfg.TLSCert)
		}
		if dockerCfg.TLSKey != "/path/to/key.pem" {
			t.Errorf("expected TLS key /path/to/key.pem, got %s", dockerCfg.TLSKey)
		}
		if dockerCfg.TLSCACert != "/path/to/ca.pem" {
			t.Errorf("expected TLS CA /path/to/ca.pem, got %s", dockerCfg.TLSCACert)
		}
		if !dockerCfg.TLSVerify {
			t.Error("expected TLS verify to be true")
		}
	} else {
		t.Error("runtime is not a *DockerRuntime")
	}

	// Close the runtime
	if err := rt.Close(); err != nil {
		t.Errorf("Failed to close runtime: %v", err)
	}
}

// TestNewRuntimeWithPackageLevelRegistration tests that NewRuntime works with
// custom runtimes registered via the package-level RegisterRuntime function
func TestNewRuntimeWithPackageLevelRegistration(t *testing.T) {
	ctx := context.Background()

	// Save original registry state to restore after test
	runtimeRegistryMu.Lock()
	originalRegistry := make(map[string]RuntimeFactory)
	for k, v := range runtimeRegistry {
		originalRegistry[k] = v
	}
	runtimeRegistryMu.Unlock()

	// Clean up after test
	defer func() {
		runtimeRegistryMu.Lock()
		runtimeRegistry = originalRegistry
		runtimeRegistryMu.Unlock()
	}()

	// Register a custom runtime using package-level RegisterRuntime
	customType := "test-custom-runtime"
	RegisterRuntime(customType, func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: customType}, nil
	})

	// Verify the factory was registered
	factory, ok := GetRuntimeFactory(customType)
	if !ok {
		t.Fatal("Custom runtime factory not registered")
	}
	if factory == nil {
		t.Fatal("Custom runtime factory is nil")
	}

	// Create a runtime using NewRuntime with the custom type
	cfg := RuntimeConfig{
		Type:    customType,
		Timeout: 10 * time.Second,
	}

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime failed with custom type: %v", err)
	}

	if rt == nil {
		t.Fatal("NewRuntime returned nil runtime")
	}

	// Verify we got the correct runtime type
	if rt.Type() != customType {
		t.Errorf("expected runtime type %q, got %q", customType, rt.Type())
	}

	// Verify it's a mockRuntime
	if _, ok := rt.(*mockRuntime); !ok {
		t.Error("runtime is not a *mockRuntime")
	}

	// Test that an unregistered custom type returns an error
	unregisteredType := "unregistered-type"
	cfg.Type = unregisteredType

	_, err = NewRuntime(ctx, cfg)
	if err == nil {
		t.Error("expected error for unregistered runtime type, got nil")
	}

	// Verify error is ErrCodeInvalidArgument
	if typedErr, ok := err.(*types.Error); ok {
		if typedErr.Code != types.ErrCodeInvalidArgument {
			t.Errorf("expected error code %s, got %s", types.ErrCodeInvalidArgument, typedErr.Code)
		}
	} else {
		t.Errorf("expected *types.Error, got %T", err)
	}
}
