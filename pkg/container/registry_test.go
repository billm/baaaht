package container

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// mockRuntime is a mock implementation of the Runtime interface for testing
type mockRuntime struct {
	runtimeType string
	closed      bool
}

func (m *mockRuntime) Create(ctx context.Context, cfg CreateConfig) (*CreateResult, error) {
	return nil, nil
}

func (m *mockRuntime) Start(ctx context.Context, cfg StartConfig) error {
	return nil
}

func (m *mockRuntime) Stop(ctx context.Context, cfg StopConfig) error {
	return nil
}

func (m *mockRuntime) Restart(ctx context.Context, cfg RestartConfig) error {
	return nil
}

func (m *mockRuntime) Destroy(ctx context.Context, cfg DestroyConfig) error {
	return nil
}

func (m *mockRuntime) Pause(ctx context.Context, containerID string) error {
	return nil
}

func (m *mockRuntime) Unpause(ctx context.Context, containerID string) error {
	return nil
}

func (m *mockRuntime) Kill(ctx context.Context, cfg KillConfig) error {
	return nil
}

func (m *mockRuntime) Wait(ctx context.Context, containerID string) (int, error) {
	return 0, nil
}

func (m *mockRuntime) Status(ctx context.Context, containerID string) (types.ContainerState, error) {
	return types.ContainerStateRunning, nil
}

func (m *mockRuntime) IsRunning(ctx context.Context, containerID string) (bool, error) {
	return true, nil
}

func (m *mockRuntime) HealthCheck(ctx context.Context, containerID string) (*HealthCheckResult, error) {
	return &HealthCheckResult{Status: types.Healthy}, nil
}

func (m *mockRuntime) HealthCheckWithRetry(ctx context.Context, containerID string, maxAttempts int, interval time.Duration) (*HealthCheckResult, error) {
	return &HealthCheckResult{Status: types.Healthy}, nil
}

func (m *mockRuntime) Stats(ctx context.Context, containerID string) (*types.ContainerStats, error) {
	return &types.ContainerStats{}, nil
}

func (m *mockRuntime) StatsStream(ctx context.Context, containerID string, interval time.Duration) (<-chan *types.ContainerStats, <-chan error) {
	statsCh := make(chan *types.ContainerStats)
	errCh := make(chan error)
	go func() {
		close(statsCh)
		close(errCh)
	}()
	return statsCh, errCh
}

func (m *mockRuntime) Logs(ctx context.Context, cfg LogsConfig) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockRuntime) LogsLines(ctx context.Context, cfg LogsConfig) ([]types.ContainerLog, error) {
	return nil, nil
}

func (m *mockRuntime) EventsStream(ctx context.Context, containerID string) (<-chan EventsMessage, <-chan error) {
	eventCh := make(chan EventsMessage)
	errCh := make(chan error)
	go func() {
		close(eventCh)
		close(errCh)
	}()
	return eventCh, errCh
}

func (m *mockRuntime) PullImage(ctx context.Context, image string, timeout time.Duration) error {
	return nil
}

func (m *mockRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	return true, nil
}

func (m *mockRuntime) Client() interface{} {
	return nil
}

func (m *mockRuntime) Type() string {
	return m.runtimeType
}

func (m *mockRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	return &RuntimeInfo{
		Type:     m.runtimeType,
		Version:  "1.0.0",
		Platform: "test",
	}, nil
}

func (m *mockRuntime) Close() error {
	m.closed = true
	return nil
}

// TestNewRegistry tests creating a new registry
func TestNewRegistry(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	reg := NewRegistry(log)
	if reg == nil {
		t.Fatal("NewRegistry returned nil")
	}

	if reg.factories == nil {
		t.Error("factories map is not initialized")
	}

	if reg.runtimes == nil {
		t.Error("runtimes map is not initialized")
	}

	if len(reg.ListRuntimes()) != 0 {
		t.Errorf("expected 0 registered runtimes, got %d", len(reg.ListRuntimes()))
	}
}

// TestNewRegistryNilLogger tests creating a registry with nil logger
func TestNewRegistryNilLogger(t *testing.T) {
	reg := NewRegistry(nil)
	if reg == nil {
		t.Fatal("NewRegistry with nil logger returned nil")
	}
	if reg.logger == nil {
		t.Error("logger should not be nil after NewRegistry with nil logger")
	}
}

// TestRegisterRuntime tests registering a runtime factory
func TestRegisterRuntime(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "test"}, nil
	}

	err := reg.RegisterRuntime("test", factory)
	if err != nil {
		t.Fatalf("Failed to register runtime: %v", err)
	}

	if !reg.HasRuntime("test") {
		t.Error("runtime was not registered")
	}

	types := reg.ListRuntimes()
	if len(types) != 1 {
		t.Errorf("expected 1 registered runtime, got %d", len(types))
	}

	if types[0] != "test" {
		t.Errorf("expected runtime type 'test', got '%s'", types[0])
	}
}

// TestRegisterRuntimeErrors tests error cases for RegisterRuntime
func TestRegisterRuntimeErrors(t *testing.T) {
	reg := NewRegistry(nil)

	// Test empty runtime type
	err := reg.RegisterRuntime("", func(cfg RuntimeConfig) (Runtime, error) {
		return nil, nil
	})
	if err == nil {
		t.Error("expected error when registering with empty runtime type")
	}

	// Test nil factory
	err = reg.RegisterRuntime("test", nil)
	if err == nil {
		t.Error("expected error when registering with nil factory")
	}
}

// TestUnregisterRuntime tests unregistering a runtime factory
func TestUnregisterRuntime(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "test"}, nil
	}

	// Register first
	err := reg.RegisterRuntime("test", factory)
	if err != nil {
		t.Fatalf("Failed to register runtime: %v", err)
	}

	// Verify registered
	if !reg.HasRuntime("test") {
		t.Error("runtime was not registered")
	}

	// Unregister
	err = reg.UnregisterRuntime("test")
	if err != nil {
		t.Fatalf("Failed to unregister runtime: %v", err)
	}

	// Verify unregistered
	if reg.HasRuntime("test") {
		t.Error("runtime was not unregistered")
	}
}

// TestUnregisterRuntimeErrors tests error cases for UnregisterRuntime
func TestUnregisterRuntimeErrors(t *testing.T) {
	reg := NewRegistry(nil)

	// Test empty runtime type
	err := reg.UnregisterRuntime("")
	if err == nil {
		t.Error("expected error when unregistering with empty runtime type")
	}
}

// TestGetRuntime tests getting a runtime instance
func TestGetRuntime(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "test"}, nil
	}

	// Register factory
	err := reg.RegisterRuntime("test", factory)
	if err != nil {
		t.Fatalf("Failed to register runtime: %v", err)
	}

	// Get runtime
	rt, err := reg.GetRuntime("test", RuntimeConfig{})
	if err != nil {
		t.Fatalf("Failed to get runtime: %v", err)
	}

	if rt == nil {
		t.Fatal("runtime is nil")
	}

	if rt.Type() != "test" {
		t.Errorf("expected runtime type 'test', got '%s'", rt.Type())
	}

	// Check that instance is cached
	if !reg.HasRuntimeInstance("test") {
		t.Error("runtime instance should be cached")
	}

	// Get again - should return same instance
	rt2, err := reg.GetRuntime("test", RuntimeConfig{})
	if err != nil {
		t.Fatalf("Failed to get runtime again: %v", err)
	}

	if rt != rt2 {
		t.Error("expected same runtime instance on second call")
	}
}

// TestGetRuntimeErrors tests error cases for GetRuntime
func TestGetRuntimeErrors(t *testing.T) {
	reg := NewRegistry(nil)

	// Test empty runtime type
	_, err := reg.GetRuntime("", RuntimeConfig{})
	if err == nil {
		t.Error("expected error when getting with empty runtime type")
	}

	// Test unregistered runtime
	_, err = reg.GetRuntime("nonexistent", RuntimeConfig{})
	if err == nil {
		t.Error("expected error when getting unregistered runtime")
	}
}

// TestGetRuntimeByType tests getting a runtime by RuntimeType enum
func TestGetRuntimeByType(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "docker"}, nil
	}

	// Register factory
	err := reg.RegisterRuntime("docker", factory)
	if err != nil {
		t.Fatalf("Failed to register runtime: %v", err)
	}

	// Get runtime by type
	rt, err := reg.GetRuntimeByType(types.RuntimeTypeDocker, RuntimeConfig{})
	if err != nil {
		t.Fatalf("Failed to get runtime by type: %v", err)
	}

	if rt.Type() != "docker" {
		t.Errorf("expected runtime type 'docker', got '%s'", rt.Type())
	}
}

// TestGetExistingRuntime tests getting an existing runtime instance
func TestGetExistingRuntime(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "test"}, nil
	}

	// Register factory
	err := reg.RegisterRuntime("test", factory)
	if err != nil {
		t.Fatalf("Failed to register runtime: %v", err)
	}

	// Try to get before instance exists
	_, err = reg.GetExistingRuntime("test")
	if err == nil {
		t.Error("expected error when getting non-existent instance")
	}

	// Create instance
	_, err = reg.GetRuntime("test", RuntimeConfig{})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	// Now get existing
	rt, err := reg.GetExistingRuntime("test")
	if err != nil {
		t.Fatalf("Failed to get existing runtime: %v", err)
	}

	if rt.Type() != "test" {
		t.Errorf("expected runtime type 'test', got '%s'", rt.Type())
	}
}

// TestHasRuntime tests HasRuntime method
func TestHasRuntime(t *testing.T) {
	reg := NewRegistry(nil)

	if reg.HasRuntime("test") {
		t.Error("expected false for unregistered runtime")
	}

	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return nil, nil
	}

	err := reg.RegisterRuntime("test", factory)
	if err != nil {
		t.Fatalf("Failed to register runtime: %v", err)
	}

	if !reg.HasRuntime("test") {
		t.Error("expected true for registered runtime")
	}
}

// TestHasRuntimeInstance tests HasRuntimeInstance method
func TestHasRuntimeInstance(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "test"}, nil
	}

	err := reg.RegisterRuntime("test", factory)
	if err != nil {
		t.Fatalf("Failed to register runtime: %v", err)
	}

	if reg.HasRuntimeInstance("test") {
		t.Error("expected false before instance created")
	}

	_, err = reg.GetRuntime("test", RuntimeConfig{})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	if !reg.HasRuntimeInstance("test") {
		t.Error("expected true after instance created")
	}
}

// TestListRuntimes tests ListRuntimes method
func TestListRuntimes(t *testing.T) {
	reg := NewRegistry(nil)

	types := reg.ListRuntimes()
	if len(types) != 0 {
		t.Errorf("expected 0 types, got %d", len(types))
	}

	// Register multiple runtimes
	for i := 0; i < 3; i++ {
		runtimeType := fmt.Sprintf("test%d", i)
		factory := func(cfg RuntimeConfig) (Runtime, error) {
			return nil, nil
		}
		err := reg.RegisterRuntime(runtimeType, factory)
		if err != nil {
			t.Fatalf("Failed to register runtime: %v", err)
		}
	}

	types = reg.ListRuntimes()
	if len(types) != 3 {
		t.Errorf("expected 3 types, got %d", len(types))
	}
}

// TestListInstances tests ListInstances method
func TestListInstances(t *testing.T) {
	reg := NewRegistry(nil)

	instances := reg.ListInstances()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(instances))
	}

	// Register and create runtimes
	for i := 0; i < 3; i++ {
		runtimeType := fmt.Sprintf("test%d", i)
		factory := func(cfg RuntimeConfig) (Runtime, error) {
			return &mockRuntime{runtimeType: runtimeType}, nil
		}
		err := reg.RegisterRuntime(runtimeType, factory)
		if err != nil {
			t.Fatalf("Failed to register runtime: %v", err)
		}

		_, err = reg.GetRuntime(runtimeType, RuntimeConfig{})
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}
	}

	instances = reg.ListInstances()
	if len(instances) != 3 {
		t.Errorf("expected 3 instances, got %d", len(instances))
	}
}

// TestCloseRuntime tests closing a specific runtime instance
func TestCloseRuntime(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "test"}, nil
	}

	// Register and create
	err := reg.RegisterRuntime("test", factory)
	if err != nil {
		t.Fatalf("Failed to register runtime: %v", err)
	}

	_, err = reg.GetRuntime("test", RuntimeConfig{})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	// Close the runtime
	err = reg.CloseRuntime("test")
	if err != nil {
		t.Fatalf("Failed to close runtime: %v", err)
	}

	// Verify instance is removed
	if reg.HasRuntimeInstance("test") {
		t.Error("runtime instance should be removed after close")
	}
}

// TestCloseRuntimeErrors tests error cases for CloseRuntime
func TestCloseRuntimeErrors(t *testing.T) {
	reg := NewRegistry(nil)

	// Test empty runtime type
	err := reg.CloseRuntime("")
	if err == nil {
		t.Error("expected error when closing with empty runtime type")
	}

	// Test closing non-existent runtime
	err = reg.CloseRuntime("nonexistent")
	if err == nil {
		t.Error("expected error when closing non-existent runtime")
	}
}

// TestClose tests closing the registry
func TestClose(t *testing.T) {
	reg := NewRegistry(nil)

	// Register and create multiple runtimes
	for i := 0; i < 3; i++ {
		runtimeType := fmt.Sprintf("test%d", i)
		factory := func(rt string) RuntimeFactory {
			return func(cfg RuntimeConfig) (Runtime, error) {
				return &mockRuntime{runtimeType: rt}, nil
			}
		}(runtimeType)

		err := reg.RegisterRuntime(runtimeType, factory)
		if err != nil {
			t.Fatalf("Failed to register runtime: %v", err)
		}

		_, err = reg.GetRuntime(runtimeType, RuntimeConfig{})
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}
	}

	// Close registry
	err := reg.Close()
	if err != nil {
		t.Fatalf("Failed to close registry: %v", err)
	}

	// Verify all instances are removed
	instances := reg.ListInstances()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances after close, got %d", len(instances))
	}
}

// TestRegistryString tests String method
func TestRegistryString(t *testing.T) {
	reg := NewRegistry(nil)

	str := reg.String()
	if str == "" {
		t.Error("String returned empty string")
	}

	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "test"}, nil
	}

	err := reg.RegisterRuntime("test", factory)
	if err != nil {
		t.Fatalf("Failed to register runtime: %v", err)
	}

	_, err = reg.GetRuntime("test", RuntimeConfig{})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	str = reg.String()
	if str == "" {
		t.Error("String returned empty string after adding runtime")
	}
}

// TestGlobalRegistry tests global registry functions
func TestGlobalRegistry(t *testing.T) {
	// Clear any previous global state
	globalRegistry = nil
	globalRegistryOnce = *(new(sync.Once))

	// Get global registry
	reg := GetGlobalRegistry()
	if reg == nil {
		t.Fatal("GetGlobalRegistry returned nil")
	}

	// Should return same instance on subsequent calls
	reg2 := GetGlobalRegistry()
	if reg != reg2 {
		t.Error("GetGlobalRegistry should return same instance")
	}

	// Test global RegisterRuntime
	factory := func(cfg RuntimeConfig) (Runtime, error) {
		return &mockRuntime{runtimeType: "test"}, nil
	}

	RegisterRuntime("test", factory)

	// Test global GetRuntimeFactory
	f, ok := GetRuntimeFactory("test")
	if !ok {
		t.Error("GetRuntimeFactory failed")
	}
	if f == nil {
		t.Error("GetRuntimeFactory returned nil factory")
	}

	// Test global ListRuntimes
	types := ListRuntimes()
	if len(types) != 1 {
		t.Errorf("expected 1 runtime type, got %d", len(types))
	}
}
