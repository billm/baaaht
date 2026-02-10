package tools

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// mockTool is a mock implementation of the Tool interface for testing
type mockTool struct {
	name        string
	toolType    ToolType
	description string
	definition  ToolDefinition
	status      types.Status
	available   bool
	enabled     bool
	closed      bool
	execCount   int
	stats       ToolUsageStats
	lastUsed    *time.Time
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Type() ToolType {
	return m.toolType
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Execute(ctx context.Context, parameters map[string]string) (*ToolExecutionResult, error) {
	if m.closed {
		return nil, fmt.Errorf("tool is closed")
	}
	m.execCount++
	now := time.Now()
	m.lastUsed = &now
	m.stats.TotalExecutions++
	m.stats.SuccessfulExecutions++
	return &ToolExecutionResult{
		ExecutionID: "test-execution-id",
		ToolName:    m.name,
		Status:      ToolExecutionStatusCompleted,
		ExitCode:    0,
		OutputText:  "mock output",
		CompletedAt: time.Now(),
		Duration:    time.Millisecond * 100,
	}, nil
}

func (m *mockTool) Validate(parameters map[string]string) error {
	return nil
}

func (m *mockTool) Definition() ToolDefinition {
	return m.definition
}

func (m *mockTool) Status() types.Status {
	return m.status
}

func (m *mockTool) IsAvailable() bool {
	return m.available && !m.closed
}

func (m *mockTool) Enabled() bool {
	return m.enabled && !m.closed
}

func (m *mockTool) SetEnabled(enabled bool) {
	m.enabled = enabled
}

func (m *mockTool) Stats() ToolUsageStats {
	return m.stats
}

func (m *mockTool) LastUsed() *time.Time {
	return m.lastUsed
}

func (m *mockTool) Close() error {
	m.closed = true
	return nil
}

// newMockTool creates a new mock tool for testing
func newMockTool(name string, toolType ToolType) Tool {
	return &mockTool{
		name:        name,
		toolType:    toolType,
		description: fmt.Sprintf("Mock tool %s", name),
		definition: ToolDefinition{
			Name:    name,
			Type:    toolType,
			Enabled: true,
		},
		status:    types.StatusRunning,
		available: true,
		enabled:   true,
		stats: ToolUsageStats{
			TotalExecutions:      0,
			SuccessfulExecutions: 0,
			FailedExecutions:     0,
		},
	}
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

	if reg.tools == nil {
		t.Error("tools map is not initialized")
	}

	if len(reg.ListTools()) != 0 {
		t.Errorf("expected 0 registered tools, got %d", len(reg.ListTools()))
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

// TestRegisterTool tests registering a tool factory
func TestRegisterTool(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	if !reg.HasTool("test") {
		t.Error("tool was not registered")
	}

	tools := reg.ListTools()
	if len(tools) != 1 {
		t.Errorf("expected 1 registered tool, got %d", len(tools))
	}

	if tools[0] != "test" {
		t.Errorf("expected tool name 'test', got '%s'", tools[0])
	}
}

// TestRegisterToolErrors tests error cases for RegisterTool
func TestRegisterToolErrors(t *testing.T) {
	reg := NewRegistry(nil)

	// Test empty tool name
	err := reg.RegisterTool("", func(def ToolDefinition) (Tool, error) {
		return nil, nil
	})
	if err == nil {
		t.Error("expected error when registering with empty tool name")
	}

	// Test nil factory
	err = reg.RegisterTool("test", nil)
	if err == nil {
		t.Error("expected error when registering with nil factory")
	}
}

// TestRegisterToolClosedRegistry tests registering when registry is closed
func TestRegisterToolClosedRegistry(t *testing.T) {
	reg := NewRegistry(nil)
	reg.Close()

	factory := func(def ToolDefinition) (Tool, error) {
		return nil, nil
	}

	err := reg.RegisterTool("test", factory)
	if err == nil {
		t.Error("expected error when registering tool on closed registry")
	}
}

// TestUnregisterTool tests unregistering a tool factory
func TestUnregisterTool(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	// Register first
	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Verify registered
	if !reg.HasTool("test") {
		t.Error("tool was not registered")
	}

	// Unregister
	err = reg.UnregisterTool("test")
	if err != nil {
		t.Fatalf("Failed to unregister tool: %v", err)
	}

	// Verify unregistered
	if reg.HasTool("test") {
		t.Error("tool was not unregistered")
	}
}

// TestUnregisterToolErrors tests error cases for UnregisterTool
func TestUnregisterToolErrors(t *testing.T) {
	reg := NewRegistry(nil)

	// Test empty tool name
	err := reg.UnregisterTool("")
	if err == nil {
		t.Error("expected error when unregistering with empty tool name")
	}
}

// TestUnregisterToolWithInstance tests unregistering when an instance exists
func TestUnregisterToolWithInstance(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	// Register and create instance
	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	_, err = reg.GetTool("test")
	if err != nil {
		t.Fatalf("Failed to get tool: %v", err)
	}

	// Verify instance exists
	if !reg.HasToolInstance("test") {
		t.Error("tool instance should exist")
	}

	// Unregister should close the instance
	err = reg.UnregisterTool("test")
	if err != nil {
		t.Fatalf("Failed to unregister tool: %v", err)
	}

	// Verify instance is removed
	if reg.HasToolInstance("test") {
		t.Error("tool instance should be removed after unregistration")
	}
}

// TestGetTool tests getting a tool instance
func TestGetTool(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	// Register factory
	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Get tool
	tool, err := reg.GetTool("test")
	if err != nil {
		t.Fatalf("Failed to get tool: %v", err)
	}

	if tool == nil {
		t.Fatal("tool is nil")
	}

	if tool.Name() != "test" {
		t.Errorf("expected tool name 'test', got '%s'", tool.Name())
	}

	// Check that instance is cached
	if !reg.HasToolInstance("test") {
		t.Error("tool instance should be cached")
	}

	// Get again - should return same instance
	tool2, err := reg.GetTool("test")
	if err != nil {
		t.Fatalf("Failed to get tool again: %v", err)
	}

	if tool != tool2 {
		t.Error("expected same tool instance on second call")
	}
}

// TestGetToolErrors tests error cases for GetTool
func TestGetToolErrors(t *testing.T) {
	reg := NewRegistry(nil)

	// Test empty tool name
	_, err := reg.GetTool("")
	if err == nil {
		t.Error("expected error when getting with empty tool name")
	}

	// Test unregistered tool
	_, err = reg.GetTool("nonexistent")
	if err == nil {
		t.Error("expected error when getting unregistered tool")
	}
}

// TestGetToolClosedRegistry tests getting tool when registry is closed
func TestGetToolClosedRegistry(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	reg.Close()

	_, err = reg.GetTool("test")
	if err == nil {
		t.Error("expected error when getting tool from closed registry")
	}
}

// TestGetToolWithDefinition tests getting a tool with a specific definition
func TestGetToolWithDefinition(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		tool := newMockTool(def.Name, def.Type).(*mockTool)
		tool.definition = def
		return tool, nil
	}

	// Register factory
	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Get with custom definition
	def := ToolDefinition{
		Name:           "test",
		Type:           ToolTypeFile,
		Enabled:        true,
		ContainerImage: "test-image:latest",
	}

	tool, err := reg.GetToolWithDefinition("test", def)
	if err != nil {
		t.Fatalf("Failed to get tool with definition: %v", err)
	}

	if tool.Definition().ContainerImage != "test-image:latest" {
		t.Errorf("expected container image 'test-image:latest', got '%s'", tool.Definition().ContainerImage)
	}
}

// TestGetExistingTool tests getting an existing tool instance
func TestGetExistingTool(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	// Register factory
	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Try to get before instance exists
	_, err = reg.GetExistingTool("test")
	if err == nil {
		t.Error("expected error when getting non-existent instance")
	}

	// Create instance
	_, err = reg.GetTool("test")
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	// Now get existing
	tool, err := reg.GetExistingTool("test")
	if err != nil {
		t.Fatalf("Failed to get existing tool: %v", err)
	}

	if tool.Name() != "test" {
		t.Errorf("expected tool name 'test', got '%s'", tool.Name())
	}
}

// TestHasTool tests HasTool method
func TestHasTool(t *testing.T) {
	reg := NewRegistry(nil)

	if reg.HasTool("test") {
		t.Error("expected false for unregistered tool")
	}

	factory := func(def ToolDefinition) (Tool, error) {
		return nil, nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	if !reg.HasTool("test") {
		t.Error("expected true for registered tool")
	}
}

// TestHasToolInstance tests HasToolInstance method
func TestHasToolInstance(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	if reg.HasToolInstance("test") {
		t.Error("expected false before instance created")
	}

	_, err = reg.GetTool("test")
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	if !reg.HasToolInstance("test") {
		t.Error("expected true after instance created")
	}
}

// TestListTools tests ListTools method
func TestListTools(t *testing.T) {
	reg := NewRegistry(nil)

	tools := reg.ListTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}

	// Register multiple tools
	for i := 0; i < 3; i++ {
		toolName := fmt.Sprintf("test%d", i)
		factory := func(def ToolDefinition) (Tool, error) {
			return nil, nil
		}
		err := reg.RegisterTool(toolName, factory)
		if err != nil {
			t.Fatalf("Failed to register tool: %v", err)
		}
	}

	tools = reg.ListTools()
	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(tools))
	}
}

// TestListInstances tests ListInstances method
func TestListInstances(t *testing.T) {
	reg := NewRegistry(nil)

	instances := reg.ListInstances()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(instances))
	}

	// Register and create tools
	for i := 0; i < 3; i++ {
		toolName := fmt.Sprintf("test%d", i)
		factory := func(name string) ToolFactory {
			return func(def ToolDefinition) (Tool, error) {
				return newMockTool(name, ToolTypeFile), nil
			}
		}(toolName)

		err := reg.RegisterTool(toolName, factory)
		if err != nil {
			t.Fatalf("Failed to register tool: %v", err)
		}

		_, err = reg.GetTool(toolName)
		if err != nil {
			t.Fatalf("Failed to create tool: %v", err)
		}
	}

	instances = reg.ListInstances()
	if len(instances) != 3 {
		t.Errorf("expected 3 instances, got %d", len(instances))
	}
}

// TestGetToolDefinition tests getting tool definition
func TestGetToolDefinition(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool(def.Name, def.Type), nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	def, err := reg.GetToolDefinition("test")
	if err != nil {
		t.Fatalf("Failed to get tool definition: %v", err)
	}

	if def.Name != "test" {
		t.Errorf("expected tool name 'test', got '%s'", def.Name)
	}
}

// TestExecuteTool tests executing a tool
func TestExecuteTool(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	ctx := context.Background()
	result, err := reg.ExecuteTool(ctx, "test", map[string]string{"param1": "value1"})
	if err != nil {
		t.Fatalf("Failed to execute tool: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	if result.Status != ToolExecutionStatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", result.Status)
	}
}

// TestExecuteToolDisabled tests executing a disabled tool
func TestExecuteToolDisabled(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		tool := newMockTool("test", ToolTypeFile).(*mockTool)
		tool.enabled = false
		return tool, nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	ctx := context.Background()
	_, err = reg.ExecuteTool(ctx, "test", map[string]string{})
	if err == nil {
		t.Error("expected error when executing disabled tool")
	}
}

// TestExecuteToolUnavailable tests executing an unavailable tool
func TestExecuteToolUnavailable(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		tool := newMockTool("test", ToolTypeFile).(*mockTool)
		tool.available = false
		return tool, nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	ctx := context.Background()
	_, err = reg.ExecuteTool(ctx, "test", map[string]string{})
	if err == nil {
		t.Error("expected error when executing unavailable tool")
	}
}

// TestCloseTool tests closing a specific tool instance
func TestCloseTool(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	// Register and create
	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	_, err = reg.GetTool("test")
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	// Close the tool
	err = reg.CloseTool("test")
	if err != nil {
		t.Fatalf("Failed to close tool: %v", err)
	}

	// Verify instance is removed
	if reg.HasToolInstance("test") {
		t.Error("tool instance should be removed after close")
	}
}

// TestCloseToolErrors tests error cases for CloseTool
func TestCloseToolErrors(t *testing.T) {
	reg := NewRegistry(nil)

	// Test empty tool name
	err := reg.CloseTool("")
	if err == nil {
		t.Error("expected error when closing with empty tool name")
	}

	// Test closing non-existent tool
	err = reg.CloseTool("nonexistent")
	if err == nil {
		t.Error("expected error when closing non-existent tool")
	}
}

// TestClose tests closing the registry
func TestClose(t *testing.T) {
	reg := NewRegistry(nil)

	// Register and create multiple tools
	for i := 0; i < 3; i++ {
		toolName := fmt.Sprintf("test%d", i)
		factory := func(name string) ToolFactory {
			return func(def ToolDefinition) (Tool, error) {
				return newMockTool(name, ToolTypeFile), nil
			}
		}(toolName)

		err := reg.RegisterTool(toolName, factory)
		if err != nil {
			t.Fatalf("Failed to register tool: %v", err)
		}

		_, err = reg.GetTool(toolName)
		if err != nil {
			t.Fatalf("Failed to create tool: %v", err)
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

	// Verify registry is closed
	if !reg.IsClosed() {
		t.Error("registry should be closed")
	}
}

// TestCloseIdempotent tests that Close is idempotent
func TestCloseIdempotent(t *testing.T) {
	reg := NewRegistry(nil)

	// First close
	err := reg.Close()
	if err != nil {
		t.Fatalf("First close failed: %v", err)
	}

	// Second close should also succeed
	err = reg.Close()
	if err != nil {
		t.Fatalf("Second close failed: %v", err)
	}
}

// TestIsClosed tests IsClosed method
func TestIsClosed(t *testing.T) {
	reg := NewRegistry(nil)

	if reg.IsClosed() {
		t.Error("expected false for open registry")
	}

	reg.Close()

	if !reg.IsClosed() {
		t.Error("expected true for closed registry")
	}
}

// TestString tests String method
func TestString(t *testing.T) {
	reg := NewRegistry(nil)

	str := reg.String()
	if str == "" {
		t.Error("String returned empty string")
	}

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	_, err = reg.GetTool("test")
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	str = reg.String()
	if str == "" {
		t.Error("String returned empty string after adding tool")
	}
}

// TestGetStats tests GetStats method
func TestGetStats(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	_, err = reg.GetTool("test")
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	stats := reg.GetStats()
	if len(stats) != 1 {
		t.Errorf("expected 1 stat entry, got %d", len(stats))
	}

	if _, ok := stats["test"]; !ok {
		t.Error("expected stats entry for 'test' tool")
	}
}

// TestGlobalRegistry tests global registry functions
func TestGlobalRegistry(t *testing.T) {
	// Clear any previous global state
	globalRegistry = nil
	globalRegistryOnce = sync.Once{}

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

	// Test global RegisterTool
	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	err := RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool globally: %v", err)
	}

	// Test global GetTool
	tool, err := GetTool("test")
	if err != nil {
		t.Fatalf("Failed to get tool globally: %v", err)
	}
	if tool == nil {
		t.Error("GetTool returned nil")
	}

	// Test global ListTools
	tools := ListTools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
}

// TestSetGlobalRegistry tests setting a custom global registry
func TestSetGlobalRegistry(t *testing.T) {
	// First, reset the global registry state
	globalRegistry = nil
	globalRegistryOnce = sync.Once{}

	customReg := NewRegistry(nil)
	SetGlobalRegistry(customReg)

	// Now GetGlobalRegistry should return the custom registry since we reset the once
	// Note: GetGlobalRegistry initializes on first call, so we need to check the actual value
	if globalRegistry != customReg {
		t.Error("SetGlobalRegistry did not set the global registry")
	}

	// Reset for other tests
	globalRegistry = nil
	globalRegistryOnce = sync.Once{}
}

// TestConcurrentAccess tests concurrent access to the registry
func TestConcurrentAccess(t *testing.T) {
	reg := NewRegistry(nil)

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	err := reg.RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.HasTool("test")
			reg.ListTools()
			reg.GetTool("test")
		}()
	}

	wg.Wait()
}

// TestExecuteToolGlobal tests global ExecuteTool function
func TestExecuteToolGlobal(t *testing.T) {
	// Clear and setup global registry
	globalRegistry = nil
	globalRegistryOnce = sync.Once{}

	factory := func(def ToolDefinition) (Tool, error) {
		return newMockTool("test", ToolTypeFile), nil
	}

	err := RegisterTool("test", factory)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	ctx := context.Background()
	result, err := ExecuteTool(ctx, "test", map[string]string{"param1": "value1"})
	if err != nil {
		t.Fatalf("Failed to execute tool globally: %v", err)
	}

	if result.Status != ToolExecutionStatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", result.Status)
	}
}
