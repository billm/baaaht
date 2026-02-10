package worker

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	grpcpkg "github.com/billm/baaaht/orchestrator/pkg/grpc"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// mockAgentServiceDependencies implements AgentServiceDependencies for testing
type mockAgentServiceDependencies struct {
	eventBus *events.Bus
}

func (m *mockAgentServiceDependencies) EventBus() *events.Bus {
	return m.eventBus
}

// TestWorkerRegister tests that a worker agent can successfully register with the orchestrator
func TestWorkerRegister(t *testing.T) {
	// Create event bus for agent service
	eventBus, err := events.New(nil)
	if err != nil {
		t.Fatalf("Failed to create event bus: %v", err)
	}
	defer eventBus.Close()

	// Create agent service dependencies
	deps := &mockAgentServiceDependencies{eventBus: eventBus}

	// Create logger
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create agent service
	agentService := grpcpkg.NewAgentService(deps, log)

	// Create gRPC server
	server := grpc.NewServer()
	proto.RegisterAgentServiceServer(server, agentService)

	// Start server on a random port
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(lis)
	}()
	defer func() {
		server.GracefulStop()
		if err := <-serverErr; err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Get the actual address
	addr := lis.Addr().String()

	t.Logf("Agent service listening on %s", addr)

	// Wait for server to be ready
	ctx := context.Background()
	clientConn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer clientConn.Close()

	// Verify client can connect
	agentClient := proto.NewAgentServiceClient(clientConn)
	_, err = agentClient.HealthCheck(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	// Create worker agent
	cfg := AgentConfig{
		DialTimeout:   5 * time.Second,
		RPCTimeout:    5 * time.Second,
	}
	agent, err := NewAgent(addr, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create worker agent: %v", err)
	}
	defer agent.Close()

	// Connect to orchestrator
	if err := agent.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial orchestrator: %v", err)
	}

	// Register the agent
	workerName := "test-worker-1"
	if err := agent.Register(ctx, workerName); err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	// Verify registration state
	if !agent.IsRegistered() {
		t.Error("Agent should be registered")
	}

	agentID := agent.GetAgentID()
	if agentID == "" {
		t.Error("Agent ID should not be empty")
	}

	t.Logf("Agent registered successfully with ID: %s", agentID)

	// Verify the agent is in the registry
	registry := agentService.GetRegistry()
	registeredAgent, err := registry.Get(agentID)
	if err != nil {
		t.Fatalf("Failed to get registered agent from registry: %v", err)
	}

	if registeredAgent.Name != workerName {
		t.Errorf("Expected agent name %s, got %s", workerName, registeredAgent.Name)
	}

	if registeredAgent.Type != "worker" {
		t.Errorf("Expected agent type 'worker', got '%s'", registeredAgent.Type)
	}

	if registeredAgent.State != "idle" {
		t.Errorf("Expected agent state 'idle', got '%s'", registeredAgent.State)
	}

	// Verify capabilities
	capabilities := registeredAgent.Capabilities
	if capabilities == nil {
		t.Fatal("Agent capabilities should not be nil")
	}

	if !capabilities.SupportsStreaming {
		t.Error("Agent should support streaming")
	}

	if !capabilities.SupportsCancellation {
		t.Error("Agent should support cancellation")
	}

	expectedTasks := []string{"file_operation", "network_request", "tool_execution"}
	if len(capabilities.SupportedTasks) != len(expectedTasks) {
		t.Errorf("Expected %d supported tasks, got %d", len(expectedTasks), len(capabilities.SupportedTasks))
	}

	expectedTools := []string{"file_read", "file_write", "file_edit", "grep", "find", "web_search", "web_fetch"}
	if len(capabilities.SupportedTools) != len(expectedTools) {
		t.Errorf("Expected %d supported tools, got %d", len(expectedTools), len(capabilities.SupportedTools))
	}

	t.Log("Worker agent registration test passed!")
}

// TestWorkerRegisterAlreadyRegistered tests that registering twice fails
func TestWorkerRegisterAlreadyRegistered(t *testing.T) {
	// Create logger
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create worker agent (without connecting)
	cfg := AgentConfig{
		DialTimeout: 5 * time.Second,
		RPCTimeout:  5 * time.Second,
	}
	agent, err := NewAgent("localhost:9999", cfg, log)
	if err != nil {
		t.Fatalf("Failed to create worker agent: %v", err)
	}
	defer agent.Close()

	// Manually set registered state
	agent.mu.Lock()
	agent.registered = true
	agent.mu.Unlock()

	// Try to register again
	ctx := context.Background()
	err = agent.Register(ctx, "test-worker")

	if err == nil {
		t.Error("Expected error when registering already registered agent")
	}

	if err != nil {
		if typesErr, ok := err.(*types.Error); ok {
			if typesErr.Code != types.ErrCodeInvalid {
				t.Errorf("Expected ErrCodeInvalid, got %v", typesErr.Code)
			}
		}
	}

	t.Log("Worker agent already registered test passed!")
}

// TestWorkerHeartbeat tests that heartbeats are sent successfully at expected intervals
func TestWorkerHeartbeat(t *testing.T) {
	// Create event bus for agent service
	eventBus, err := events.New(nil)
	if err != nil {
		t.Fatalf("Failed to create event bus: %v", err)
	}
	defer eventBus.Close()

	// Create agent service dependencies
	deps := &mockAgentServiceDependencies{eventBus: eventBus}

	// Create logger
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create agent service
	agentService := grpcpkg.NewAgentService(deps, log)

	// Create gRPC server
	server := grpc.NewServer()
	proto.RegisterAgentServiceServer(server, agentService)

	// Start server on a random port
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(lis)
	}()
	defer func() {
		server.GracefulStop()
		if err := <-serverErr; err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Get the actual address
	addr := lis.Addr().String()

	// Create worker agent with short heartbeat interval for testing
	cfg := AgentConfig{
		DialTimeout:       5 * time.Second,
		RPCTimeout:        5 * time.Second,
		HeartbeatInterval: 2 * time.Second, // Short interval for testing
	}
	agent, err := NewAgent(addr, cfg, log)
	if err != nil {
		t.Fatalf("Failed to create worker agent: %v", err)
	}
	defer agent.Close()

	// Connect to orchestrator
	ctx := context.Background()
	if err := agent.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial orchestrator: %v", err)
	}

	// Register the agent (this starts the heartbeat loop)
	workerName := "test-worker-heartbeat"
	if err := agent.Register(ctx, workerName); err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	agentID := agent.GetAgentID()
	t.Logf("Agent registered with ID: %s", agentID)

	// Get the registry to verify heartbeat updates
	registry := agentService.GetRegistry()

	// Get initial last heartbeat time
	registeredAgent, err := registry.Get(agentID)
	if err != nil {
		t.Fatalf("Failed to get registered agent from registry: %v", err)
	}
	initialHeartbeat := registeredAgent.LastHeartbeat
	t.Logf("Initial heartbeat: %v", initialHeartbeat)

	// Wait for at least one heartbeat to be sent
	// Heartbeat interval is 2 seconds, so wait 3 seconds to be safe
	time.Sleep(3 * time.Second)

	// Verify heartbeat was updated
	registeredAgent, err = registry.Get(agentID)
	if err != nil {
		t.Fatalf("Failed to get registered agent from registry: %v", err)
	}
	updatedHeartbeat := registeredAgent.LastHeartbeat
	t.Logf("Updated heartbeat: %v", updatedHeartbeat)

	// Check that heartbeat was updated (newer time than initial)
	if updatedHeartbeat.Before(initialHeartbeat) || updatedHeartbeat.Equal(initialHeartbeat) {
		t.Errorf("Expected heartbeat to be updated, but initial=%v, updated=%v",
			initialHeartbeat, updatedHeartbeat)
	}

	// Wait for another heartbeat cycle
	time.Sleep(3 * time.Second)

	// Verify another heartbeat was sent
	registeredAgent, err = registry.Get(agentID)
	if err != nil {
		t.Fatalf("Failed to get registered agent from registry: %v", err)
	}
	finalHeartbeat := registeredAgent.LastHeartbeat
	t.Logf("Final heartbeat: %v", finalHeartbeat)

	// Check that heartbeat was updated again
	if finalHeartbeat.Before(updatedHeartbeat) || finalHeartbeat.Equal(updatedHeartbeat) {
		t.Errorf("Expected heartbeat to be updated again, but updated=%v, final=%v",
			updatedHeartbeat, finalHeartbeat)
	}

	// Verify agent stats show successful RPCs
	stats := agent.Stats()
	if stats.TotalRPCs == 0 {
		t.Error("Expected TotalRPCs > 0, but got 0")
	}
	// Account for Register RPC + heartbeat RPCs
	// At minimum: 1 Register + 2 Heartbeats = 3
	expectedMinRPCs := int64(3)
	if stats.TotalRPCs < expectedMinRPCs {
		t.Errorf("Expected at least %d TotalRPCs, got %d", expectedMinRPCs, stats.TotalRPCs)
	}

	t.Log("Worker agent heartbeat test passed!")
}
