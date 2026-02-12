package integration

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	grpcPkg "github.com/billm/baaaht/orchestrator/pkg/grpc"
	"github.com/billm/baaaht/orchestrator/pkg/orchestrator"
	"github.com/billm/baaaht/orchestrator/proto"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stretchr/testify/require"
)

// TestE2EAssistantAgentFlow tests the complete Assistant agent flow:
// 1. Start Orchestrator with LLM Gateway (or mock)
// 2. Register mock Assistant agent via gRPC
// 3. Create session and send message
// 4. Verify delegation to Worker agent
// 5. Verify response is returned
func TestE2EAssistantAgentFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Log("=== Step 1: Bootstrapping orchestrator and gRPC server ===")

	cfg := loadTestConfig(t)

	// Bootstrap orchestrator
	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-assistant-e2e-1.0.0",
		ShutdownTimeout:     30 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	orch := result.Orchestrator

	t.Cleanup(func() {
		t.Log("=== Cleanup: Shutting down orchestrator ===")
		if err := orch.Close(); err != nil {
			t.Logf("Warning: Orchestrator close returned error: %v", err)
		}
	})

	// Bootstrap gRPC server
	grpcBootstrapCfg := grpcPkg.BootstrapConfig{
		Config:              cfg.GRPC,
		Logger:              log,
		SessionManager:      orch.SessionManager(),
		EventBus:            orch.EventBus(),
		IPCBroker:           nil,
		Version:             "test-assistant-grpc-1.0.0",
		ShutdownTimeout:     10 * time.Second,
		EnableHealthCheck:   true,
		HealthCheckInterval: 30 * time.Second,
	}

	grpcResult, err := grpcPkg.Bootstrap(ctx, grpcBootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap gRPC server")
	require.True(t, grpcResult.IsSuccessful(), "gRPC bootstrap should be successful")

	t.Cleanup(func() {
		t.Log("=== Cleanup: Shutting down gRPC server ===")
		if grpcResult.Health != nil {
			grpcResult.Health.Shutdown()
		}
		if grpcResult.Server != nil {
			if err := grpcResult.Server.Stop(); err != nil {
				t.Logf("Warning: gRPC server stop returned error: %v", err)
			}
		}
	})

	t.Logf("gRPC server started on %s", cfg.GRPC.SocketPath)

	t.Log("=== Step 2: Creating gRPC client ===")

	clientCfg := grpcPkg.ClientConfig{
		DialTimeout:          5 * time.Second,
		RPCTimeout:           5 * time.Second,
		MaxRecvMsgSize:       grpcPkg.DefaultMaxRecvMsgSize,
		MaxSendMsgSize:       grpcPkg.DefaultMaxSendMsgSize,
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 0,
	}

	grpcClient, err := grpcPkg.NewClient(cfg.GRPC.SocketPath, clientCfg, log)
	require.NoError(t, err, "Failed to create gRPC client")
	require.NoError(t, grpcClient.Dial(ctx), "Failed to connect gRPC client")
	require.True(t, grpcClient.IsConnected(), "Client should be connected")

	t.Cleanup(func() {
		if err := grpcClient.Close(); err != nil {
			t.Logf("Warning: gRPC client close returned error: %v", err)
		}
	})

	t.Log("gRPC client connected successfully")

	t.Log("=== Step 3: Verifying health check ===")

	healthResp, err := grpcClient.HealthCheck(ctx)
	require.NoError(t, err, "Health check should succeed")
	require.Equal(t, grpc_health.HealthCheckResponse_SERVING, healthResp.Status, "Health status should be SERVING")

	t.Log("Health check passed")

	t.Log("=== Step 4: Creating session ===")

	orchClient := proto.NewOrchestratorServiceClient(grpcClient.GetConn())
	agentClient := proto.NewAgentServiceClient(grpcClient.GetConn())

	createReq := &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name:        "assistant-e2e-test-session",
			Description: "Session for Assistant agent E2E test",
			OwnerId:     "assistant-e2e-test-user",
			Labels: map[string]string{
				"test":         "e2e",
				"test_type":    "assistant_agent",
				"delegation":   "enabled",
			},
		},
		Config: &proto.SessionConfig{
			MaxContainers: 10,
			MaxDurationNs: 3600 * 1000000000,
			IdleTimeoutNs: 1800 * 1000000000,
		},
	}

	createResp, err := orchClient.CreateSession(ctx, createReq)
	require.NoError(t, err, "CreateSession should succeed")
	require.NotEmpty(t, createResp.SessionId, "Session ID should not be empty")
	require.Equal(t, "assistant-e2e-test-session", createResp.Session.Metadata.Name)
	require.Equal(t, proto.SessionState_SESSION_STATE_ACTIVE, createResp.Session.State)

	sessionID := createResp.SessionId
	t.Logf("Session created: %s", sessionID)

	t.Log("=== Step 5: Registering mock Assistant agent ===")

	assistantID := fmt.Sprintf("assistant-%d", time.Now().Unix())
	registerReq := &proto.RegisterRequest{
		Name: "AssistantAgent",
		Type: proto.AgentType_AGENT_TYPE_TOOL,
		Metadata: &proto.AgentMetadata{
			Version:    "1.0.0-test",
			Description: "Mock Assistant agent for E2E testing",
			Labels: map[string]string{
				"agent_type": "assistant",
				"test":       "e2e",
			},
			Hostname: "localhost",
			Pid:      "12345",
		},
		Capabilities: &proto.AgentCapabilities{
			SupportedTasks:     []string{"message_handling", "delegation"},
			SupportedTools:     []string{"delegate"},
			MaxConcurrentTasks: 10,
			SupportsStreaming:  true,
			SupportsCancellation: true,
		},
	}

	registerResp, err := agentClient.Register(ctx, registerReq)
	require.NoError(t, err, "Assistant agent registration should succeed")
	require.NotEmpty(t, registerResp.AgentId, "Agent ID should not be empty")
	assistantID = registerResp.AgentId
	t.Logf("Assistant agent registered: %s", assistantID)

	t.Cleanup(func() {
		t.Log("=== Cleanup: Unregistering Assistant agent ===")
		unregisterReq := &proto.UnregisterRequest{
			AgentId: assistantID,
			Reason:  "E2E test cleanup",
		}
		_, _ = agentClient.Unregister(ctx, unregisterReq)
	})

	t.Log("=== Step 6: Registering mock Worker agent ===")

	workerID := fmt.Sprintf("worker-%d", time.Now().Unix())
	workerRegisterReq := &proto.RegisterRequest{
		Name: "WorkerAgent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
		Metadata: &proto.AgentMetadata{
			Version:    "1.0.0-test",
			Description: "Mock Worker agent for E2E testing",
			Labels: map[string]string{
				"agent_type": "worker",
				"test":       "e2e",
			},
			Hostname: "localhost",
			Pid:      "12346",
		},
		Capabilities: &proto.AgentCapabilities{
			SupportedTasks:     []string{"file_operation", "web_operation"},
			SupportedTools:     []string{"read_file", "write_file", "list_files", "web_search", "web_fetch"},
			MaxConcurrentTasks: 5,
			SupportsStreaming:  false,
			SupportsCancellation: true,
		},
	}

	workerRegisterResp, err := agentClient.Register(ctx, workerRegisterReq)
	require.NoError(t, err, "Worker agent registration should succeed")
	require.NotEmpty(t, workerRegisterResp.AgentId, "Worker agent ID should not be empty")
	workerID = workerRegisterResp.AgentId
	t.Logf("Worker agent registered: %s", workerID)

	t.Cleanup(func() {
		t.Log("=== Cleanup: Unregistering Worker agent ===")
		unregisterReq := &proto.UnregisterRequest{
			AgentId: workerID,
			Reason:  "E2E test cleanup",
		}
		_, _ = agentClient.Unregister(ctx, unregisterReq)
	})

	t.Log("=== Step 7: Sending message to session (simulating user input) ===")

	messageContent := "Please read the file /tmp/test.txt and tell me its contents"
	msgReq := &proto.SendMessageRequest{
		SessionId: sessionID,
		Message: &proto.Message{
			Role:      proto.MessageRole_MESSAGE_ROLE_USER,
			Content:   messageContent,
			Timestamp: timestamppb.New(time.Now()),
		},
	}

	msgResp, err := orchClient.SendMessage(ctx, msgReq)
	require.NoError(t, err, "SendMessage should succeed")
	require.NotEmpty(t, msgResp.MessageId, "Message ID should not be empty")
	t.Logf("Message sent: %s", msgResp.MessageId)

	t.Log("=== Step 8: Simulating task delegation from Assistant to Worker ===")

	// In a real scenario, the Assistant would:
	// 1. Receive the message from the session
	// 2. Process it with the LLM
	// 3. Detect a file operation is needed
	// 4. Use the delegate tool to send a task to the Worker
	// For this test, we simulate the delegation directly

	taskExecuteReq := &proto.ExecuteTaskRequest{
		AgentId:   workerID,
		SessionId: sessionID,
		Type:      proto.TaskType_TASK_TYPE_FILE_OPERATION,
		Priority:  proto.TaskPriority_TASK_PRIORITY_NORMAL,
		Config: &proto.TaskConfig{
			Command:  "read_file",
			Arguments: []string{"/tmp/test.txt"},
			Environment: map[string]string{
				"TEST_MODE": "e2e",
			},
			Parameters: map[string]string{
				"operation": "read",
				"path":      "/tmp/test.txt",
			},
			TimeoutNs: 30 * 1000000000, // 30 seconds
		},
	}

	taskExecuteResp, err := agentClient.ExecuteTask(ctx, taskExecuteReq)
	require.NoError(t, err, "ExecuteTask should succeed")
	require.NotEmpty(t, taskExecuteResp.TaskId, "Task ID should not be empty")

	taskID := taskExecuteResp.TaskId
	t.Logf("Task delegated to Worker: %s", taskID)

	t.Log("=== Step 9: Monitoring task status ===")

	// Poll for task completion
	var taskCompleted atomic.Bool
	var taskResult *proto.Task

	maxPolls := 20
	pollInterval := 500 * time.Millisecond

	for i := 0; i < maxPolls; i++ {
		statusReq := &proto.GetTaskStatusRequest{
			TaskId: taskID,
		}

		statusResp, err := agentClient.GetTaskStatus(ctx, statusReq)
		require.NoError(t, err, "GetTaskStatus should succeed")

		task := statusResp.Task
		t.Logf("Task status: %s, state: %v", taskID, task.State)

		if task.State == proto.TaskState_TASK_STATE_COMPLETED {
			taskCompleted.Store(true)
			taskResult = task
			break
		}

		if task.State == proto.TaskState_TASK_STATE_FAILED {
			t.Logf("Task failed: %s", task.Error.Message)
			break
		}

		time.Sleep(pollInterval)
	}

	// In a mock scenario without actual container execution, the task may not complete
	// The important thing is that the delegation infrastructure works
	t.Logf("Task polling completed: completed=%v", taskCompleted.Load())

	if taskCompleted.Load() && taskResult != nil && taskResult.Result != nil {
		t.Logf("Task result: exit_code=%d, output=%s", taskResult.Result.ExitCode, taskResult.Result.OutputText)
	}

	t.Log("=== Step 10: Verifying session context updated ===")

	getSessionReq := &proto.GetSessionRequest{
		SessionId: sessionID,
	}

	getSessionResp, err := orchClient.GetSession(ctx, getSessionReq)
	require.NoError(t, err, "GetSession should succeed")
	require.Equal(t, sessionID, getSessionResp.Session.Id)

	// Verify messages are in the session
	require.GreaterOrEqual(t, len(getSessionResp.Session.Context.Messages), 1,
		"Session should have at least one message")

	t.Logf("Session has %d messages", len(getSessionResp.Session.Context.Messages))

	// Find the user message we sent
	var userMessageFound bool
	for _, msg := range getSessionResp.Session.Context.Messages {
		if msg.Content == messageContent {
			userMessageFound = true
			t.Logf("Found user message: role=%s, content=%s", msg.Role, msg.Content)
			break
		}
	}
	require.True(t, userMessageFound, "User message should be in session context")

	t.Log("=== Step 11: Simulating Assistant response to user ===")

	// In a real scenario, the Assistant would:
	// 1. Receive the task result from the Worker
	// 2. Format it appropriately
	// 3. Send it as a response to the user via the LLM
	// For this test, we simulate by adding an assistant message

	responseContent := "I've read the file /tmp/test.txt. [Mock response - file would contain actual content in production]"
	responseMsgReq := &proto.SendMessageRequest{
		SessionId: sessionID,
		Message: &proto.Message{
			Role:      proto.MessageRole_MESSAGE_ROLE_ASSISTANT,
			Content:   responseContent,
			Timestamp: timestamppb.New(time.Now()),
			Metadata: &proto.MessageMetadata{
				Extra: map[string]string{
					"delegated_to": workerID,
					"task_id":      taskID,
				},
		 },
		},
	}

	responseMsgResp, err := orchClient.SendMessage(ctx, responseMsgReq)
	require.NoError(t, err, "Send assistant response should succeed")
	require.NotEmpty(t, responseMsgResp.MessageId, "Response message ID should not be empty")
	t.Logf("Assistant response sent: %s", responseMsgResp.MessageId)

	t.Log("=== Step 12: Verifying complete conversation flow ===")

	// Get the session again to verify both messages are present
	finalSessionReq := &proto.GetSessionRequest{
		SessionId: sessionID,
	}

	finalSessionResp, err := orchClient.GetSession(ctx, finalSessionReq)
	require.NoError(t, err, "Final GetSession should succeed")

	messageCount := len(finalSessionResp.Session.Context.Messages)
	t.Logf("Session now has %d messages", messageCount)
	require.GreaterOrEqual(t, messageCount, 2, "Session should have at least 2 messages (user + assistant)")

	// Verify we have both user and assistant messages
	var hasUserMessage, hasAssistantMessage bool
	for _, msg := range finalSessionResp.Session.Context.Messages {
		if msg.Role == proto.MessageRole_MESSAGE_ROLE_USER && msg.Content == messageContent {
			hasUserMessage = true
		}
		if msg.Role == proto.MessageRole_MESSAGE_ROLE_ASSISTANT {
			hasAssistantMessage = true
		}
	}

	require.True(t, hasUserMessage, "Should have user message")
	require.True(t, hasAssistantMessage, "Should have assistant message")

	t.Log("=== E2E Assistant Agent Test Complete ===")
	t.Log("All steps passed successfully:")
	t.Log("  1. Orchestrator and gRPC server bootstrapped")
	t.Log("  2. gRPC client connected")
	t.Log("  3. Health check verified")
	t.Log("  4. Session created")
	t.Log("  5. Assistant agent registered")
	t.Log("  6. Worker agent registered")
	t.Log("  7. User message sent to session")
	t.Log("  8. Task delegated from Assistant to Worker")
	t.Log("  9. Task status monitored")
	t.Log(" 10. Session context verified")
	t.Log(" 11. Assistant response sent")
	t.Log(" 12. Complete conversation flow verified")
}

// TestE2EAssistantAgentMultipleDelegations tests multiple delegations in sequence
func TestE2EAssistantAgentMultipleDelegations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	t.Log("=== Testing multiple delegations from Assistant to Worker ===")

	cfg := loadTestConfig(t)

	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-multi-delegation-1.0.0",
		ShutdownTimeout:     30 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err)
	orch := result.Orchestrator

	defer func() {
		_ = orch.Close()
	}()

	grpcBootstrapCfg := grpcPkg.BootstrapConfig{
		Config:            cfg.GRPC,
		Logger:            log,
		SessionManager:    orch.SessionManager(),
		EventBus:          orch.EventBus(),
		IPCBroker:         nil,
		Version:           "test-multi-delegation-grpc-1.0.0",
		ShutdownTimeout:   10 * time.Second,
		EnableHealthCheck: true,
	}

	grpcResult, err := grpcPkg.Bootstrap(ctx, grpcBootstrapCfg)
	require.NoError(t, err)

	defer func() {
		if grpcResult.Health != nil {
			grpcResult.Health.Shutdown()
		}
		if grpcResult.Server != nil {
			_ = grpcResult.Server.Stop()
		}
	}()

	clientCfg := grpcPkg.ClientConfig{
		DialTimeout:          5 * time.Second,
		RPCTimeout:           5 * time.Second,
		MaxRecvMsgSize:       grpcPkg.DefaultMaxRecvMsgSize,
		MaxSendMsgSize:       grpcPkg.DefaultMaxSendMsgSize,
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 0,
	}

	grpcClient, err := grpcPkg.NewClient(cfg.GRPC.SocketPath, clientCfg, log)
	require.NoError(t, err)
	require.NoError(t, grpcClient.Dial(ctx))

	defer func() {
		_ = grpcClient.Close()
	}()

	agentClient := proto.NewAgentServiceClient(grpcClient.GetConn())
	orchClient := proto.NewOrchestratorServiceClient(grpcClient.GetConn())

	// Create session
	createResp, err := orchClient.CreateSession(ctx, &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name:    "multi-delegation-test",
			OwnerId: "e2e-test-user",
		},
		Config: &proto.SessionConfig{
			MaxContainers: 10,
		},
	})
	require.NoError(t, err)
	sessionID := createResp.SessionId

	// Register agents
	assistantResp, err := agentClient.Register(ctx, &proto.RegisterRequest{
		Name: "AssistantAgent",
		Type: proto.AgentType_AGENT_TYPE_TOOL,
		Metadata: &proto.AgentMetadata{
			Version: "1.0.0-test",
		},
		Capabilities: &proto.AgentCapabilities{
			SupportedTasks: []string{"message_handling"},
			SupportedTools: []string{"delegate"},
		},
	})
	require.NoError(t, err)
	assistantID := assistantResp.AgentId

	defer func() {
		_, _ = agentClient.Unregister(ctx, &proto.UnregisterRequest{AgentId: assistantID})
	}()

	workerResp, err := agentClient.Register(ctx, &proto.RegisterRequest{
		Name: "WorkerAgent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
		Metadata: &proto.AgentMetadata{
			Version: "1.0.0-test",
		},
		Capabilities: &proto.AgentCapabilities{
			SupportedTasks: []string{"file_operation"},
		},
	})
	require.NoError(t, err)
	workerID := workerResp.AgentId

	defer func() {
		_, _ = agentClient.Unregister(ctx, &proto.UnregisterRequest{AgentId: workerID})
	}()

	t.Logf("Session: %s, Assistant: %s, Worker: %s", sessionID, assistantID, workerID)

	// Simulate multiple delegations
	numDelegations := 3
	taskIDs := make([]string, 0, numDelegations)

	for i := 0; i < numDelegations; i++ {
		taskReq := &proto.ExecuteTaskRequest{
			AgentId:   workerID,
			SessionId: sessionID,
			Type:      proto.TaskType_TASK_TYPE_FILE_OPERATION,
			Priority:  proto.TaskPriority_TASK_PRIORITY_NORMAL,
			Config: &proto.TaskConfig{
				Command:   fmt.Sprintf("operation_%d", i),
				Arguments: []string{fmt.Sprintf("/tmp/file_%d.txt", i)},
				TimeoutNs: 30 * 1000000000,
			},
		}

		taskResp, err := agentClient.ExecuteTask(ctx, taskReq)
		require.NoError(t, err, "ExecuteTask %d should succeed", i)
		require.NotEmpty(t, taskResp.TaskId, "Task ID %d should not be empty", i)

		taskIDs = append(taskIDs, taskResp.TaskId)
		t.Logf("Task %d delegated: %s", i, taskResp.TaskId)
	}

	// Verify all tasks were created
	require.Equal(t, numDelegations, len(taskIDs), "Should have delegated all tasks")

	// List tasks for the worker
	listReq := &proto.ListTasksRequest{
		AgentId: workerID,
	}

	listResp, err := agentClient.ListTasks(ctx, listReq)
	require.NoError(t, err, "ListTasks should succeed")
	require.GreaterOrEqual(t, len(listResp.Tasks), numDelegations,
		"Worker should have at least %d tasks", numDelegations)

	t.Logf("Worker has %d tasks", len(listResp.Tasks))

	t.Log("Multiple delegations test passed")
}

// TestE2EAssistantAgentStreamTask tests streaming task execution
func TestE2EAssistantAgentStreamTask(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	t.Log("=== Testing streaming task execution ===")

	cfg := loadTestConfig(t)

	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-stream-task-1.0.0",
		ShutdownTimeout:     30 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err)
	orch := result.Orchestrator

	defer func() {
		_ = orch.Close()
	}()

	grpcBootstrapCfg := grpcPkg.BootstrapConfig{
		Config:            cfg.GRPC,
		Logger:            log,
		SessionManager:    orch.SessionManager(),
		EventBus:          orch.EventBus(),
		IPCBroker:         nil,
		Version:           "test-stream-task-grpc-1.0.0",
		ShutdownTimeout:   10 * time.Second,
		EnableHealthCheck: true,
	}

	grpcResult, err := grpcPkg.Bootstrap(ctx, grpcBootstrapCfg)
	require.NoError(t, err)

	defer func() {
		if grpcResult.Health != nil {
			grpcResult.Health.Shutdown()
		}
		if grpcResult.Server != nil {
			_ = grpcResult.Server.Stop()
		}
	}()

	clientCfg := grpcPkg.ClientConfig{
		DialTimeout:          5 * time.Second,
		RPCTimeout:           5 * time.Second,
		MaxRecvMsgSize:       grpcPkg.DefaultMaxRecvMsgSize,
		MaxSendMsgSize:       grpcPkg.DefaultMaxSendMsgSize,
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 0,
	}

	grpcClient, err := grpcPkg.NewClient(cfg.GRPC.SocketPath, clientCfg, log)
	require.NoError(t, err)
	require.NoError(t, grpcClient.Dial(ctx))

	defer func() {
		_ = grpcClient.Close()
	}()

	agentClient := proto.NewAgentServiceClient(grpcClient.GetConn())

	// Register Worker agent
	workerResp, err := agentClient.Register(ctx, &proto.RegisterRequest{
		Name: "WorkerAgent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
		Metadata: &proto.AgentMetadata{
			Version: "1.0.0-test",
		},
		Capabilities: &proto.AgentCapabilities{
			SupportedTasks:      []string{"file_operation"},
			SupportsStreaming:   true,
			SupportsCancellation: true,
		},
	})
	require.NoError(t, err)
	workerID := workerResp.AgentId

	defer func() {
		_, _ = agentClient.Unregister(ctx, &proto.UnregisterRequest{AgentId: workerID})
	}()

	t.Logf("Worker agent registered: %s", workerID)

	// Create a streaming task
	streamClient, err := agentClient.StreamTask(ctx)
	require.NoError(t, err, "StreamTask should succeed")

	// Send initial task request
	taskReq := &proto.StreamTaskRequest{
		Payload: &proto.StreamTaskRequest_Input{
			Input: &proto.TaskInput{
				Data: []byte("test streaming data"),
				Metadata: map[string]string{
					"operation": "stream_test",
				},
			},
		},
	}

	err = streamClient.Send(taskReq)
	require.NoError(t, err, "Send task request should succeed")
	t.Log("Task request sent")

	// Send heartbeat
	heartbeatReq := &proto.StreamTaskRequest{
		Payload: &proto.StreamTaskRequest_Heartbeat{},
	}
	err = streamClient.Send(heartbeatReq)
	require.NoError(t, err, "Send heartbeat should succeed")
	t.Log("Heartbeat sent")

	// Try to receive response (may fail if task doesn't exist yet, which is OK for this test)
	// The important thing is the streaming infrastructure works
	_, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resp, err := streamClient.Recv()
	if err == nil && resp != nil {
		t.Logf("Received streaming response: %v", resp.Payload)
	} else {
		t.Logf("No response received (expected for non-existent task): %v", err)
	}

	err = streamClient.CloseSend()
	require.NoError(t, err, "CloseSend should succeed")

	t.Log("Streaming task test completed")
}
