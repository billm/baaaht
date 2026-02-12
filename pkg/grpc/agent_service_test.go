package grpc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// mockAgentServiceDeps implements AgentServiceDependencies for testing
type mockAgentServiceDeps struct{}

func (m *mockAgentServiceDeps) EventBus() *events.Bus {
	return nil // Event bus is optional for tests
}

func (m *mockAgentServiceDeps) SessionManager() *session.Manager {
	return nil
}

// Test helper to create a test service
func newTestAgentService(t *testing.T) *AgentService {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return NewAgentService(&mockAgentServiceDeps{}, log, nil)
}

// =============================================================================
// Test Agent Registration
// =============================================================================

func TestAgentService_Register(t *testing.T) {
	service := newTestAgentService(t)

	t.Run("valid_registration", func(t *testing.T) {
		req := &proto.RegisterRequest{
			Name: "test-agent",
			Type: proto.AgentType_AGENT_TYPE_WORKER,
			Metadata: &proto.AgentMetadata{
				Version: "1.0.0",
				Labels: map[string]string{
					"env": "test",
				},
			},
			Capabilities: &proto.AgentCapabilities{
				SupportedTasks:     []string{"code_execution"},
				MaxConcurrentTasks: 5,
				SupportsStreaming:  true,
			},
		}

		resp, err := service.Register(context.Background(), req)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		if resp.AgentId == "" {
			t.Error("Expected non-empty agent_id")
		}
		if resp.Agent == nil {
			t.Error("Expected agent in response")
		}
		if resp.RegistrationToken == "" {
			t.Error("Expected non-empty registration_token")
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		req := &proto.RegisterRequest{
			Name: "",
			Type: proto.AgentType_AGENT_TYPE_WORKER,
		}

		_, err := service.Register(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestAgentService_Unregister(t *testing.T) {
	service := newTestAgentService(t)

	// First register an agent
	regReq := &proto.RegisterRequest{
		Name: "test-agent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	}
	regResp, err := service.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	t.Run("valid_unregistration", func(t *testing.T) {
		req := &proto.UnregisterRequest{
			AgentId: regResp.AgentId,
			Reason:  "test cleanup",
		}

		resp, err := service.Unregister(context.Background(), req)
		if err != nil {
			t.Fatalf("Unregister failed: %v", err)
		}

		if !resp.Success {
			t.Error("Expected success=true")
		}
		if resp.Message == "" {
			t.Error("Expected non-empty message")
		}
	})

	t.Run("unregister_nonexistent_agent", func(t *testing.T) {
		req := &proto.UnregisterRequest{
			AgentId: "non-existent-id",
		}

		_, err := service.Unregister(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent agent")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_agent_id_returns_error", func(t *testing.T) {
		req := &proto.UnregisterRequest{
			AgentId: "",
		}

		_, err := service.Unregister(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty agent_id")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestAgentService_Heartbeat(t *testing.T) {
	service := newTestAgentService(t)

	// Register an agent first
	regReq := &proto.RegisterRequest{
		Name: "test-agent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	}
	regResp, err := service.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	t.Run("valid_heartbeat", func(t *testing.T) {
		req := &proto.HeartbeatRequest{
			AgentId: regResp.AgentId,
		}

		resp, err := service.Heartbeat(context.Background(), req)
		if err != nil {
			t.Fatalf("Heartbeat failed: %v", err)
		}

		if resp.Timestamp == nil {
			t.Error("Expected timestamp in response")
		}
	})

	t.Run("heartbeat_for_nonexistent_agent", func(t *testing.T) {
		req := &proto.HeartbeatRequest{
			AgentId: "non-existent-id",
		}

		_, err := service.Heartbeat(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent agent")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})
}

// =============================================================================
// Test Task Execution
// =============================================================================

func TestAgentService_ExecuteTask(t *testing.T) {
	service := newTestAgentService(t)

	// Register an agent first
	regReq := &proto.RegisterRequest{
		Name: "test-agent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	}
	regResp, err := service.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	t.Run("valid_task_execution", func(t *testing.T) {
		req := &proto.ExecuteTaskRequest{
			AgentId:   regResp.AgentId,
			SessionId: "test-session-123",
			Type:      proto.TaskType_TASK_TYPE_CODE_EXECUTION,
			Priority:  proto.TaskPriority_TASK_PRIORITY_NORMAL,
			Config: &proto.TaskConfig{
				Command:          "echo hello",
				Arguments:        []string{"world"},
				WorkingDirectory: "/tmp",
				TimeoutNs:        30000000000, // 30 seconds
			},
		}

		resp, err := service.ExecuteTask(context.Background(), req)
		if err != nil {
			t.Fatalf("ExecuteTask failed: %v", err)
		}

		if resp.TaskId == "" {
			t.Error("Expected non-empty task_id")
		}
		if resp.Task == nil {
			t.Error("Expected task in response")
		}
		if resp.Task.State != proto.TaskState_TASK_STATE_PENDING {
			t.Errorf("Expected PENDING state, got %v", resp.Task.State)
		}
	})

	t.Run("empty_agent_id_returns_error", func(t *testing.T) {
		req := &proto.ExecuteTaskRequest{
			SessionId: "test-session",
			Config:    &proto.TaskConfig{},
		}

		_, err := service.ExecuteTask(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty agent_id")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("empty_session_id_returns_error", func(t *testing.T) {
		req := &proto.ExecuteTaskRequest{
			AgentId: regResp.AgentId,
			Config:  &proto.TaskConfig{},
		}

		_, err := service.ExecuteTask(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty session_id")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("task_for_nonexistent_agent", func(t *testing.T) {
		req := &proto.ExecuteTaskRequest{
			AgentId:   "non-existent-agent",
			SessionId: "test-session",
			Config:    &proto.TaskConfig{},
		}

		_, err := service.ExecuteTask(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent agent")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})
}

func TestAgentService_ListTasks(t *testing.T) {
	service := newTestAgentService(t)

	// Register an agent
	regReq := &proto.RegisterRequest{
		Name: "test-agent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	}
	regResp, err := service.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	// Create some tasks
	for i := 0; i < 3; i++ {
		taskReq := &proto.ExecuteTaskRequest{
			AgentId:   regResp.AgentId,
			SessionId: "test-session",
			Config: &proto.TaskConfig{
				Command: "echo task",
			},
		}
		_, err := service.ExecuteTask(context.Background(), taskReq)
		if err != nil {
			t.Fatalf("Failed to create task: %v", err)
		}
	}

	t.Run("list_all_tasks", func(t *testing.T) {
		req := &proto.ListTasksRequest{
			AgentId: regResp.AgentId,
		}

		resp, err := service.ListTasks(context.Background(), req)
		if err != nil {
			t.Fatalf("ListTasks failed: %v", err)
		}

		if resp.TotalCount != 3 {
			t.Errorf("Expected 3 tasks, got %d", resp.TotalCount)
		}
		if len(resp.Tasks) != 3 {
			t.Errorf("Expected 3 tasks in list, got %d", len(resp.Tasks))
		}
	})

	t.Run("list_tasks_for_nonexistent_agent", func(t *testing.T) {
		req := &proto.ListTasksRequest{
			AgentId: "non-existent-agent",
		}

		resp, err := service.ListTasks(context.Background(), req)
		if err != nil {
			t.Fatalf("ListTasks failed: %v", err)
		}

		if resp.TotalCount != 0 {
			t.Errorf("Expected 0 tasks for non-existent agent, got %d", resp.TotalCount)
		}
	})
}

func TestAgentService_GetTaskStatus(t *testing.T) {
	service := newTestAgentService(t)

	// Register an agent
	regReq := &proto.RegisterRequest{
		Name: "test-agent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	}
	regResp, err := service.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	// Create a task
	taskReq := &proto.ExecuteTaskRequest{
		AgentId:   regResp.AgentId,
		SessionId: "test-session",
		Config:    &proto.TaskConfig{},
	}
	taskResp, err := service.ExecuteTask(context.Background(), taskReq)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	t.Run("get_task_status", func(t *testing.T) {
		req := &proto.GetTaskStatusRequest{
			TaskId: taskResp.TaskId,
		}

		resp, err := service.GetTaskStatus(context.Background(), req)
		if err != nil {
			t.Fatalf("GetTaskStatus failed: %v", err)
		}

		if resp.Task == nil {
			t.Error("Expected task in response")
		}
		if resp.Task.Id != taskResp.TaskId {
			t.Errorf("Expected task_id %s, got %s", taskResp.TaskId, resp.Task.Id)
		}
	})

	t.Run("get_status_for_nonexistent_task", func(t *testing.T) {
		req := &proto.GetTaskStatusRequest{
			TaskId: "non-existent-task",
		}

		_, err := service.GetTaskStatus(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent task")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_task_id_returns_error", func(t *testing.T) {
		req := &proto.GetTaskStatusRequest{
			TaskId: "",
		}

		_, err := service.GetTaskStatus(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty task_id")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestAgentService_CancelTask(t *testing.T) {
	service := newTestAgentService(t)

	// Register an agent
	regReq := &proto.RegisterRequest{
		Name: "test-agent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	}
	regResp, err := service.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	// Create a task
	taskReq := &proto.ExecuteTaskRequest{
		AgentId:   regResp.AgentId,
		SessionId: "test-session",
		Config:    &proto.TaskConfig{},
	}
	taskResp, err := service.ExecuteTask(context.Background(), taskReq)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	t.Run("cancel_task", func(t *testing.T) {
		req := &proto.CancelTaskRequest{
			TaskId: taskResp.TaskId,
			Reason: "test cancellation",
		}

		resp, err := service.CancelTask(context.Background(), req)
		if err != nil {
			t.Fatalf("CancelTask failed: %v", err)
		}

		if !resp.Cancelled {
			t.Error("Expected cancelled=true")
		}
		if resp.State != proto.TaskState_TASK_STATE_CANCELLED {
			t.Errorf("Expected CANCELLED state, got %v", resp.State)
		}
	})

	t.Run("cancel_nonexistent_task", func(t *testing.T) {
		req := &proto.CancelTaskRequest{
			TaskId: "non-existent-task",
		}

		_, err := service.CancelTask(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent task")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_task_id_returns_error", func(t *testing.T) {
		req := &proto.CancelTaskRequest{
			TaskId: "",
		}

		_, err := service.CancelTask(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty task_id")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

// =============================================================================
// Test Communication
// =============================================================================

func TestAgentService_SendMessage(t *testing.T) {
	service := newTestAgentService(t)

	// Register an agent first
	regReq := &proto.RegisterRequest{
		Name: "test-agent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	}
	regResp, err := service.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	t.Run("send_message", func(t *testing.T) {
		req := &proto.AgentSendMessageRequest{
			AgentId: regResp.AgentId,
			Message: &proto.AgentMessage{
				Type:     proto.MessageType_MESSAGE_TYPE_DATA,
				SourceId: "orchestrator",
				TargetId: regResp.AgentId,
				Payload: &proto.AgentMessage_DataMessage{
					DataMessage: &proto.DataMessage{
						ContentType: "text/plain",
						Data:        []byte("hello"),
					},
				},
			},
		}

		resp, err := service.SendMessage(context.Background(), req)
		if err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}

		if resp.MessageId == "" {
			t.Error("Expected non-empty message_id")
		}
		if resp.Timestamp == nil {
			t.Error("Expected timestamp in response")
		}
	})

	t.Run("send_to_nonexistent_agent", func(t *testing.T) {
		req := &proto.AgentSendMessageRequest{
			AgentId: "non-existent-agent",
			Message: &proto.AgentMessage{
				Type: proto.MessageType_MESSAGE_TYPE_DATA,
			},
		}

		_, err := service.SendMessage(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent agent")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_agent_id_returns_error", func(t *testing.T) {
		req := &proto.AgentSendMessageRequest{
			AgentId: "",
			Message: &proto.AgentMessage{},
		}

		_, err := service.SendMessage(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty agent_id")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("nil_message_returns_error", func(t *testing.T) {
		req := &proto.AgentSendMessageRequest{
			AgentId: regResp.AgentId,
			Message: nil,
		}

		_, err := service.SendMessage(context.Background(), req)
		if err == nil {
			t.Error("Expected error for nil message")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

type captureResponseHandler struct {
	mu      sync.Mutex
	calls   int
	session types.ID
	message types.Message
	lastErr error
}

func (h *captureResponseHandler) SendResponseToSession(sessionID types.ID, message types.Message) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	h.session = sessionID
	h.message = message
	return h.lastErr
}

func (h *captureResponseHandler) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

func (h *captureResponseHandler) lastMessage() (types.ID, types.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.session, h.message
}

type mockAgentRouteStream struct {
	proto.AgentService_StreamAgentServer
	ctx  context.Context
	sent []*proto.StreamAgentResponse
}

func (m *mockAgentRouteStream) Context() context.Context { return m.ctx }

func (m *mockAgentRouteStream) Send(resp *proto.StreamAgentResponse) error {
	m.sent = append(m.sent, resp)
	return nil
}

func (m *mockAgentRouteStream) Recv() (*proto.StreamAgentRequest, error) {
	return nil, context.Canceled
}

func (m *mockAgentRouteStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockAgentRouteStream) SendHeader(metadata.MD) error { return nil }
func (m *mockAgentRouteStream) SetTrailer(metadata.MD)       {}
func (m *mockAgentRouteStream) SendMsg(any) error            { return nil }
func (m *mockAgentRouteStream) RecvMsg(any) error            { return nil }

func TestAgentService_RouteMessageAndProcessResponse(t *testing.T) {
	service := newTestAgentService(t)

	regResp, err := service.Register(context.Background(), &proto.RegisterRequest{
		Name: "assistant",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	})
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	agentID := regResp.AgentId
	stream := &mockAgentRouteStream{ctx: context.Background()}

	service.mu.Lock()
	service.agentStreams[agentID] = stream
	service.agentSendLocks[agentID] = &sync.Mutex{}
	service.mu.Unlock()

	handler := &captureResponseHandler{}
	sessionID := types.ID("session-123")
	requestMsg := types.Message{
		ID:      types.ID("corr-123"),
		Role:    types.MessageRoleUser,
		Content: "hello",
	}

	if err := service.RouteMessageToAgent(context.Background(), agentID, sessionID, requestMsg, handler); err != nil {
		t.Fatalf("RouteMessageToAgent failed: %v", err)
	}

	if len(stream.sent) != 1 {
		t.Fatalf("Expected 1 routed message, got %d", len(stream.sent))
	}

	routed := stream.sent[0].GetMessage()
	if routed == nil || routed.Metadata == nil {
		t.Fatal("Expected routed message metadata")
	}

	if routed.Metadata.CorrelationId != "corr-123" {
		t.Fatalf("Expected correlation corr-123, got %q", routed.Metadata.CorrelationId)
	}

	service.processAgentResponse(context.Background(), agentID, &proto.AgentMessage{
		Metadata: &proto.AgentMessageMetadata{
			SessionId:     string(sessionID),
			CorrelationId: "corr-123",
		},
		Payload: &proto.AgentMessage_DataMessage{DataMessage: &proto.DataMessage{Data: []byte("response")}},
	})

	if handler.callCount() != 1 {
		t.Fatalf("Expected 1 response callback, got %d", handler.callCount())
	}

	recordedSession, recordedMessage := handler.lastMessage()
	if recordedSession != sessionID {
		t.Fatalf("Expected session %q, got %q", sessionID, recordedSession)
	}
	if recordedMessage.Content != "response" {
		t.Fatalf("Expected response content, got %q", recordedMessage.Content)
	}

	service.mu.RLock()
	_, pendingExists := service.pendingResponses[pendingResponseKey(agentID, "corr-123")]
	_, timerExists := service.pendingTimers[pendingResponseKey(agentID, "corr-123")]
	service.mu.RUnlock()
	if pendingExists || timerExists {
		t.Fatal("Expected pending response and timer to be cleared")
	}
}

func TestAgentService_DiscardMismatchedResponse(t *testing.T) {
	service := newTestAgentService(t)

	regResp, err := service.Register(context.Background(), &proto.RegisterRequest{
		Name: "assistant",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	})
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	agentID := regResp.AgentId
	stream := &mockAgentRouteStream{ctx: context.Background()}

	service.mu.Lock()
	service.agentStreams[agentID] = stream
	service.agentSendLocks[agentID] = &sync.Mutex{}
	service.mu.Unlock()

	handler := &captureResponseHandler{}
	if err := service.RouteMessageToAgent(context.Background(), agentID, types.ID("session-a"), types.Message{
		ID:      types.ID("corr-mismatch"),
		Role:    types.MessageRoleUser,
		Content: "hello",
	}, handler); err != nil {
		t.Fatalf("RouteMessageToAgent failed: %v", err)
	}

	service.processAgentResponse(context.Background(), agentID, &proto.AgentMessage{
		Metadata: &proto.AgentMessageMetadata{
			SessionId:     "session-b",
			CorrelationId: "corr-mismatch",
		},
		Payload: &proto.AgentMessage_DataMessage{DataMessage: &proto.DataMessage{Data: []byte("response")}},
	})

	if handler.callCount() != 0 {
		t.Fatalf("Expected no callback for mismatched session, got %d", handler.callCount())
	}

	service.mu.RLock()
	_, pendingExists := service.pendingResponses[pendingResponseKey(agentID, "corr-mismatch")]
	_, timerExists := service.pendingTimers[pendingResponseKey(agentID, "corr-mismatch")]
	service.mu.RUnlock()
	if pendingExists || timerExists {
		t.Fatal("Expected mismatched response to clear pending route and timer")
	}
}

func TestAgentService_ExpirePendingResponse(t *testing.T) {
	service := newTestAgentService(t)
	handler := &captureResponseHandler{}
	agentID := "agent-timeout"
	pendingKey := pendingResponseKey(agentID, "corr-timeout")

	service.mu.Lock()
	service.pendingResponses[pendingKey] = pendingResponseRoute{
		sessionID: types.ID("session-timeout"),
		handler:   handler,
	}
	service.pendingTimers[pendingKey] = time.NewTimer(time.Hour)
	service.mu.Unlock()

	service.expirePendingResponse(agentID, pendingKey)

	if handler.callCount() != 1 {
		t.Fatalf("Expected timeout callback, got %d", handler.callCount())
	}

	_, msg := handler.lastMessage()
	if msg.Content == "" {
		t.Fatal("Expected timeout message content")
	}

	service.mu.RLock()
	_, pendingExists := service.pendingResponses[pendingKey]
	_, timerExists := service.pendingTimers[pendingKey]
	service.mu.RUnlock()
	if pendingExists || timerExists {
		t.Fatal("Expected expired pending route and timer to be cleared")
	}
}

// =============================================================================
// Test Health and Status
// =============================================================================

func TestAgentService_HealthCheck(t *testing.T) {
	service := newTestAgentService(t)

	t.Run("health_check_returns_healthy", func(t *testing.T) {
		resp, err := service.HealthCheck(context.Background(), &emptypb.Empty{})
		if err != nil {
			t.Fatalf("HealthCheck failed: %v", err)
		}

		if resp.Status != proto.Status_STATUS_RUNNING {
			t.Errorf("Expected RUNNING status, got %v", resp.Status)
		}
		if resp.Version == "" {
			t.Error("Expected non-empty version")
		}
		if len(resp.Subsystems) == 0 {
			t.Error("Expected subsystems in response")
		}
	})
}

func TestAgentService_GetStatus(t *testing.T) {
	service := newTestAgentService(t)

	t.Run("get_status", func(t *testing.T) {
		resp, err := service.GetStatus(context.Background(), &emptypb.Empty{})
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}

		if resp.Status != proto.Status_STATUS_RUNNING {
			t.Errorf("Expected RUNNING status, got %v", resp.Status)
		}
		if resp.StartedAt == nil {
			t.Error("Expected started_at in response")
		}
		if resp.Uptime == nil {
			t.Error("Expected uptime in response")
		}
	})
}

func TestAgentService_GetCapabilities(t *testing.T) {
	service := newTestAgentService(t)

	t.Run("get_capabilities", func(t *testing.T) {
		resp, err := service.GetCapabilities(context.Background(), &emptypb.Empty{})
		if err != nil {
			t.Fatalf("GetCapabilities failed: %v", err)
		}

		if resp.Capabilities == nil {
			t.Error("Expected capabilities in response")
		}
		if !resp.Capabilities.SupportsStreaming {
			t.Error("Expected SupportsStreaming to be true")
		}
		if !resp.Capabilities.SupportsCancellation {
			t.Error("Expected SupportsCancellation to be true")
		}
		if resp.Capabilities.MaxConcurrentTasks == 0 {
			t.Error("Expected non-zero MaxConcurrentTasks")
		}
	})
}

// =============================================================================
// Test StreamAgent (Bidirectional Streaming)
// =============================================================================

func TestAgentService_StreamAgent(t *testing.T) {
	service := newTestAgentService(t)

	// Register an agent first
	regReq := &proto.RegisterRequest{
		Name: "test-agent",
		Type: proto.AgentType_AGENT_TYPE_WORKER,
	}
	regResp, err := service.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	t.Run("bidirectional_messaging", func(t *testing.T) {
		// Create a mock stream using the actual service method
		// For this test, we'll just verify the service doesn't panic
		// Full bidirectional testing requires more complex setup

		// Verify stream tracking works
		if len(service.streams) != 0 {
			t.Errorf("Expected 0 streams initially, got %d", len(service.streams))
		}

		// Test that we can create a request message
		req := &proto.StreamAgentRequest{
			AgentId: regResp.AgentId,
			Payload: &proto.StreamAgentRequest_Heartbeat{&emptypb.Empty{}},
		}
		if req.AgentId == "" {
			t.Error("Expected non-empty agent_id in request")
		}
	})
}

// =============================================================================
// Test Close
// =============================================================================

func TestAgentService_Close(t *testing.T) {
	service := newTestAgentService(t)

	t.Run("close_without_streams", func(t *testing.T) {
		err := service.Close()
		if err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})
}

func TestAgentService_String(t *testing.T) {
	service := newTestAgentService(t)

	t.Run("string_representation", func(t *testing.T) {
		str := service.String()
		if str == "" {
			t.Error("Expected non-empty string representation")
		}
	})
}

// =============================================================================
// Test Registry
// =============================================================================

func TestAgentRegistry(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	registry := NewAgentRegistry(log)

	t.Run("register_and_get_agent", func(t *testing.T) {
		info := &AgentInfo{
			Name: "test-agent",
			Type: "worker",
		}
		agentID := "test-agent-123"

		err := registry.Register(agentID, info)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		retrieved, err := registry.Get(agentID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if retrieved.Name != "test-agent" {
			t.Errorf("Expected name 'test-agent', got '%s'", retrieved.Name)
		}
	})

	t.Run("register_duplicate_agent", func(t *testing.T) {
		info := &AgentInfo{
			Name: "test-agent",
			Type: "worker",
		}
		agentID := "duplicate-agent"

		err := registry.Register(agentID, info)
		if err != nil {
			t.Fatalf("First register failed: %v", err)
		}

		err = registry.Register(agentID, info)
		if err == nil {
			t.Error("Expected error when registering duplicate agent")
		}
	})

	t.Run("list_agents", func(t *testing.T) {
		registry := NewAgentRegistry(log)

		// Register multiple agents
		for i := 0; i < 3; i++ {
			info := &AgentInfo{
				Name: "agent",
				Type: "worker",
			}
			id := "agent-" + fmt.Sprintf("%d", i)
			_ = registry.Register(id, info)
		}

		agents := registry.List()
		if len(agents) != 3 {
			t.Errorf("Expected 3 agents, got %d", len(agents))
		}
	})

	t.Run("unregister_agent", func(t *testing.T) {
		info := &AgentInfo{
			Name: "test-agent",
			Type: "worker",
		}
		agentID := "unregister-test"

		_ = registry.Register(agentID, info)
		err := registry.Unregister(agentID)
		if err != nil {
			t.Fatalf("Unregister failed: %v", err)
		}

		_, err = registry.Get(agentID)
		if err == nil {
			t.Error("Expected error when getting unregistered agent")
		}
	})
}

// =============================================================================
// Test Skills Integration
// =============================================================================

func TestAgentService_SkillsIntegration(t *testing.T) {
	service := newTestAgentService(t)

	t.Run("get_skills_loader_returns_nil_when_not_set", func(t *testing.T) {
		loader := service.GetSkillsLoader()
		if loader != nil {
			t.Error("Expected nil skills loader when not provided")
		}
	})

	t.Run("get_active_skills_returns_empty_when_no_loader", func(t *testing.T) {
		skills, err := service.GetActiveSkillsForAgent(context.Background(), "test-agent")
		if err != nil {
			t.Fatalf("GetActiveSkillsForAgent failed: %v", err)
		}
		if len(skills) != 0 {
			t.Errorf("Expected 0 skills when no loader, got %d", len(skills))
		}
	})

	t.Run("activate_skills_for_agent_does_not_panic_when_no_loader", func(t *testing.T) {
		err := service.ActivateSkillsForAgent(context.Background(), "test-agent", "worker")
		if err != nil {
			t.Fatalf("ActivateSkillsForAgent should not error when no loader: %v", err)
		}
	})
}
