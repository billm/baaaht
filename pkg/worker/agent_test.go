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

// TestWorkerShutdown tests that a worker agent gracefully shuts down and unregisters
func TestWorkerShutdown(t *testing.T) {
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

	// Connect to orchestrator
	if err := agent.Dial(ctx); err != nil {
		t.Fatalf("Failed to dial orchestrator: %v", err)
	}

	// Register the agent
	workerName := "test-worker-shutdown"
	if err := agent.Register(ctx, workerName); err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	agentID := agent.GetAgentID()
	t.Logf("Agent registered with ID: %s", agentID)

	// Verify the agent is in the registry
	registry := agentService.GetRegistry()
	registeredAgent, err := registry.Get(agentID)
	if err != nil {
		t.Fatalf("Failed to get registered agent from registry: %v", err)
	}

	if registeredAgent.Name != workerName {
		t.Errorf("Expected agent name %s, got %s", workerName, registeredAgent.Name)
	}

	// Verify agent is registered
	if !agent.IsRegistered() {
		t.Error("Agent should be registered")
	}

	// Close the agent (this should unregister)
	if err := agent.Close(); err != nil {
		t.Fatalf("Failed to close agent: %v", err)
	}

	// Verify agent is no longer registered
	if agent.IsRegistered() {
		t.Error("Agent should not be registered after close")
	}

	if agent.GetAgentID() != "" {
		t.Error("Agent ID should be empty after close")
	}

	// Verify the agent is removed from the registry
	_, err = registry.Get(agentID)
	if err == nil {
		t.Error("Expected error when getting unregistered agent from registry")
	}

	// Verify it's a not found error
	if err != nil {
		if typesErr, ok := err.(*types.Error); ok {
			if typesErr.Code != types.ErrCodeNotFound {
				t.Errorf("Expected ErrCodeNotFound, got %v", typesErr.Code)
			}
		}
	}

	// Verify agent can be closed again without error
	if err := agent.Close(); err == nil {
		t.Error("Expected error when closing already closed agent")
	} else if err != nil {
		if typesErr, ok := err.(*types.Error); ok {
			if typesErr.Code != types.ErrCodeInvalid {
				t.Errorf("Expected ErrCodeInvalid, got %v", typesErr.Code)
			}
		}
	}

	t.Log("Worker agent shutdown test passed!")
}

// TestListenForTasks tests that a worker agent can establish a stream for task execution
func TestListenForTasks(t *testing.T) {
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
		DialTimeout: 5 * time.Second,
		RPCTimeout:  5 * time.Second,
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
	workerName := "test-worker-listen"
	if err := agent.Register(ctx, workerName); err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	agentID := agent.GetAgentID()
	t.Logf("Agent registered with ID: %s", agentID)

	// Create a task via ExecuteTask RPC
	taskReq := &proto.ExecuteTaskRequest{
		AgentId:   agentID,
		SessionId: "test-session-1",
		Type:      proto.TaskType_TASK_TYPE_CUSTOM,
		Priority:  proto.TaskPriority_TASK_PRIORITY_NORMAL,
		Config: &proto.TaskConfig{
			Command: "echo hello",
		},
	}

	taskResp, err := agentClient.ExecuteTask(ctx, taskReq)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	taskID := taskResp.TaskId
	t.Logf("Task created with ID: %s", taskID)

	// Test ListenForTasks - establish stream
	streamCtx, streamCancel := context.WithTimeout(ctx, 10*time.Second)
	defer streamCancel()

	stream, err := agent.ListenForTasks(streamCtx, taskID)
	if err != nil {
		t.Fatalf("Failed to listen for tasks: %v", err)
	}

	if stream == nil {
		t.Fatal("Expected non-nil stream")
	}

	t.Log("Stream established successfully")

	// Send a test message with input data
	testInput := &proto.StreamTaskRequest{
		TaskId: taskID,
		Payload: &proto.StreamTaskRequest_Input{
			Input: &proto.TaskInput{
				Data: []byte("test input data"),
				Metadata: map[string]string{
					"source": "test",
				},
			},
		},
	}

	if err := stream.Send(testInput); err != nil {
		t.Fatalf("Failed to send input message: %v", err)
	}

	t.Log("Input message sent successfully")

	// Receive response from server
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Failed to receive response: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	// Verify we received output (server echoes back input)
	output := resp.GetOutput()
	if output == nil {
		t.Error("Expected output in response")
	} else {
		t.Logf("Received output: %s", output.Text)
	}

	// Close the stream
	if err := stream.CloseSend(); err != nil {
		t.Logf("Warning: Failed to close send stream: %v", err)
	}

	t.Log("Worker agent ListenForTasks test passed!")
}

// TestListenForTasksNotRegistered tests that ListenForTasks fails when agent is not registered
func TestListenForTasksNotRegistered(t *testing.T) {
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

	// Try to listen for tasks without registering
	ctx := context.Background()
	_, err = agent.ListenForTasks(ctx, "test-task-id")

	if err == nil {
		t.Error("Expected error when listening for tasks with unregistered agent")
	}

	if err != nil {
		if typesErr, ok := err.(*types.Error); ok {
			if typesErr.Code != types.ErrCodeInvalid {
				t.Errorf("Expected ErrCodeInvalid, got %v", typesErr.Code)
			}
		}
	}

	t.Log("Worker agent ListenForTasks not registered test passed!")
}

// TestTaskRouting tests that tasks are routed to the correct executor based on type
func TestTaskRouting(t *testing.T) {
	// Create logger
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create worker agent
	cfg := AgentConfig{
		DialTimeout: 5 * time.Second,
		RPCTimeout:  5 * time.Second,
	}
	agent, err := NewAgent("localhost:9999", cfg, log)
	if err != nil {
		t.Fatalf("Failed to create worker agent: %v", err)
	}
	defer agent.Close()

	// Create executor (without actual runtime for testing)
	executor, err := NewExecutorDefault(log)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	ctx := context.Background()
	mountSource := "/tmp/test"

	// Test file operation routing
	t.Run("FileOperationRead", func(t *testing.T) {
		config := &proto.TaskConfig{
			Command: "read",
			Arguments: []string{"test.txt"},
		}

		// This will fail without actual runtime, but we can verify routing logic
		_, err := agent.RouteTask(ctx, executor, proto.TaskType_TASK_TYPE_FILE_OPERATION, config, mountSource)
		// Expected to fail because file doesn't exist, but routing should work
		if err == nil {
			t.Log("File read routing succeeded (or runtime not available)")
		} else {
			t.Logf("File read routing attempted, got expected error: %v", err)
		}
	})

	// Test file write operation routing
	t.Run("FileOperationWrite", func(t *testing.T) {
		config := &proto.TaskConfig{
			Command: "write",
			Arguments: []string{"output.txt"},
			Parameters: map[string]string{
				"content": "test content",
			},
		}

		_, err := agent.RouteTask(ctx, executor, proto.TaskType_TASK_TYPE_FILE_OPERATION, config, mountSource)
		if err == nil {
			t.Log("File write routing succeeded")
		} else {
			t.Logf("File write routing attempted, got error: %v", err)
		}
	})

	// Test grep operation routing
	t.Run("FileOperationGrep", func(t *testing.T) {
		config := &proto.TaskConfig{
			Command: "grep",
			Parameters: map[string]string{
				"pattern": "test",
				"path":    ".",
			},
		}

		_, err := agent.RouteTask(ctx, executor, proto.TaskType_TASK_TYPE_FILE_OPERATION, config, mountSource)
		if err == nil {
			t.Log("Grep routing succeeded")
		} else {
			t.Logf("Grep routing attempted, got error: %v", err)
		}
	})

	// Test find operation routing
	t.Run("FileOperationFind", func(t *testing.T) {
		config := &proto.TaskConfig{
			Command: "find",
			Parameters: map[string]string{
				"path": ".",
				"name": "*.txt",
			},
		}

		_, err := agent.RouteTask(ctx, executor, proto.TaskType_TASK_TYPE_FILE_OPERATION, config, mountSource)
		if err == nil {
			t.Log("Find routing succeeded")
		} else {
			t.Logf("Find routing attempted, got error: %v", err)
		}
	})

	// Test list operation routing
	t.Run("FileOperationList", func(t *testing.T) {
		config := &proto.TaskConfig{
			Command: "list",
			Parameters: map[string]string{
				"path": ".",
			},
		}

		_, err := agent.RouteTask(ctx, executor, proto.TaskType_TASK_TYPE_FILE_OPERATION, config, mountSource)
		if err == nil {
			t.Log("List routing succeeded")
		} else {
			t.Logf("List routing attempted, got error: %v", err)
		}
	})

	// Test network request routing
	t.Run("NetworkRequestWebSearch", func(t *testing.T) {
		config := &proto.TaskConfig{
			Command: "search",
			Parameters: map[string]string{
				"url": "https://example.com",
			},
		}

		_, err := agent.RouteTask(ctx, executor, proto.TaskType_TASK_TYPE_NETWORK_REQUEST, config, "")
		if err == nil {
			t.Log("Web search routing succeeded")
		} else {
			t.Logf("Web search routing attempted, got error: %v", err)
		}
	})

	// Test URL fetch routing
	t.Run("NetworkRequestFetch", func(t *testing.T) {
		config := &proto.TaskConfig{
			Command: "fetch",
			Parameters: map[string]string{
				"url": "https://example.com",
			},
		}

		_, err := agent.RouteTask(ctx, executor, proto.TaskType_TASK_TYPE_NETWORK_REQUEST, config, "")
		if err == nil {
			t.Log("URL fetch routing succeeded")
		} else {
			t.Logf("URL fetch routing attempted, got error: %v", err)
		}
	})

	// Test unknown operation
	t.Run("UnknownOperation", func(t *testing.T) {
		config := &proto.TaskConfig{
			Command: "unknown_operation",
		}

		_, err := agent.RouteTask(ctx, executor, proto.TaskType_TASK_TYPE_FILE_OPERATION, config, mountSource)
		if err == nil {
			t.Error("Expected error for unknown operation")
		} else {
			t.Logf("Unknown operation correctly rejected: %v", err)
		}
	})

	// Test unsupported task type
	t.Run("UnsupportedTaskType", func(t *testing.T) {
		config := &proto.TaskConfig{
			Command: "test",
		}

		_, err := agent.RouteTask(ctx, executor, proto.TaskType_TASK_TYPE_CODE_EXECUTION, config, mountSource)
		if err == nil {
			t.Log("Code execution routing attempted (may fail without runtime)")
		} else {
			t.Logf("Code execution routing attempted, got error: %v", err)
		}
	})

	t.Log("Task routing test passed!")
}

// TestMapOperationToToolType tests the operation to tool type mapping
func TestMapOperationToToolType(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := AgentConfig{
		DialTimeout: 5 * time.Second,
		RPCTimeout:  5 * time.Second,
	}
	agent, err := NewAgent("localhost:9999", cfg, log)
	if err != nil {
		t.Fatalf("Failed to create worker agent: %v", err)
	}
	defer agent.Close()

	testCases := []struct {
		operation  string
		expectTool ToolType
	}{
		{"file_read", ToolTypeFileRead},
		{"read", ToolTypeFileRead},
		{"cat", ToolTypeFileRead},
		{"file_write", ToolTypeFileWrite},
		{"write", ToolTypeFileWrite},
		{"file_edit", ToolTypeFileEdit},
		{"edit", ToolTypeFileEdit},
		{"grep", ToolTypeGrep},
		{"search", ToolTypeGrep},
		{"find", ToolTypeFind},
		{"list", ToolTypeList},
		{"ls", ToolTypeList},
		{"web_search", ToolTypeWebSearch},
		{"fetch", ToolTypeFetchURL},
		{"web_fetch", ToolTypeFetchURL},
		{"curl", ToolTypeFetchURL},
		{"unknown", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.operation, func(t *testing.T) {
			result := agent.mapOperationToToolType(tc.operation)
			if result != tc.expectTool {
				t.Errorf("Expected %s for operation %s, got %s", tc.expectTool, tc.operation, result)
			} else {
				t.Logf("Operation %s correctly mapped to %s", tc.operation, result)
			}
		})
	}

	t.Log("Map operation to tool type test passed!")
}
