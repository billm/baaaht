package grpc

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/ipc"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// mockGatewayServiceDeps implements GatewayServiceDependencies for testing
type mockGatewayServiceDeps struct {
	mgr *session.Manager
	bus *events.Bus
	broker *ipc.Broker
}

func (m *mockGatewayServiceDeps) SessionManager() *session.Manager {
	return m.mgr
}

func (m *mockGatewayServiceDeps) EventBus() *events.Bus {
	return m.bus
}

func (m *mockGatewayServiceDeps) IPCBroker() *ipc.Broker {
	return m.broker // Can be nil for tests
}

func setupGatewayService(t *testing.T) (*GatewayService, *mockGatewayServiceDeps, context.Context) {
	ctx := context.Background()

	// Create logger
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create session manager
	mgr, err := session.NewDefault(log)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Create event bus
	bus, err := events.New(log)
	if err != nil {
		t.Fatalf("Failed to create event bus: %v", err)
	}

	// Create dependencies
	deps := &mockGatewayServiceDeps{
		mgr: mgr,
		bus: bus,
	}

	// Create service
	svc := NewGatewayService(deps, log)

	return svc, deps, ctx
}

// Test GatewayRegistry

func TestGatewayRegistry(t *testing.T) {
	t.Run("Add and Get", func(t *testing.T) {
		log, _ := logger.NewDefault()
		registry := NewGatewayRegistry(log)

		info := &GatewaySessionInfo{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}

		sessionID := "test-session-id"
		err := registry.Add(sessionID, info)
		if err != nil {
			t.Fatalf("Failed to add session: %v", err)
		}

		retrieved, err := registry.Get(sessionID)
		if err != nil {
			t.Fatalf("Failed to get session: %v", err)
		}

		if retrieved.ID != sessionID {
			t.Errorf("Expected session ID %s, got %s", sessionID, retrieved.ID)
		}
	})

	t.Run("Add duplicate fails", func(t *testing.T) {
		log, _ := logger.NewDefault()
		registry := NewGatewayRegistry(log)

		info := &GatewaySessionInfo{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}

		sessionID := "test-session-id"
		_ = registry.Add(sessionID, info)

		err := registry.Add(sessionID, info)
		if err == nil {
			t.Error("Expected error when adding duplicate session")
		}
	})

	t.Run("Get non-existent fails", func(t *testing.T) {
		log, _ := logger.NewDefault()
		registry := NewGatewayRegistry(log)

		_, err := registry.Get("non-existent")
		if err == nil {
			t.Error("Expected error when getting non-existent session")
		}
	})

	t.Run("Remove", func(t *testing.T) {
		log, _ := logger.NewDefault()
		registry := NewGatewayRegistry(log)

		info := &GatewaySessionInfo{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}

		sessionID := "test-session-id"
		_ = registry.Add(sessionID, info)

		err := registry.Remove(sessionID)
		if err != nil {
			t.Fatalf("Failed to remove session: %v", err)
		}

		_, err = registry.Get(sessionID)
		if err == nil {
			t.Error("Expected error when getting removed session")
		}
	})

	t.Run("Stream count management", func(t *testing.T) {
		log, _ := logger.NewDefault()
		registry := NewGatewayRegistry(log)

		info := &GatewaySessionInfo{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}

		sessionID := "test-session-id"
		_ = registry.Add(sessionID, info)

		_ = registry.IncrementStreamCount(sessionID)
		_ = registry.IncrementStreamCount(sessionID)

		retrieved, _ := registry.Get(sessionID)
		if retrieved.ActiveStreams != 2 {
			t.Errorf("Expected 2 active streams, got %d", retrieved.ActiveStreams)
		}

		_ = registry.DecrementStreamCount(sessionID)

		retrieved, _ = registry.Get(sessionID)
		if retrieved.ActiveStreams != 1 {
			t.Errorf("Expected 1 active stream, got %d", retrieved.ActiveStreams)
		}
	})
}

// Test GatewayService RPCs

func TestGatewayService_CreateGatewaySession(t *testing.T) {
	svc, _, ctx := setupGatewayService(t)

	t.Run("Valid session creation", func(t *testing.T) {
		req := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name:    "test-gateway-session",
				UserId:  "user-123",
				Labels:  map[string]string{"env": "test"},
			},
			Config: &proto.GatewaySessionConfig{
				TimeoutNs:      3600000000000, // 1 hour
				EnableStreaming: true,
			},
		}

		resp, err := svc.CreateGatewaySession(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create gateway session: %v", err)
		}

		if resp.SessionId == "" {
			t.Error("Expected non-empty session ID")
		}

		if resp.Session == nil {
			t.Error("Expected session in response")
		}

		if resp.Session.State != proto.GatewaySessionState_GATEWAY_SESSION_STATE_ACTIVE {
			t.Errorf("Expected state ACTIVE, got %v", resp.Session.State)
		}

		if resp.Session.OrchestratorSessionId == "" {
			t.Error("Expected orchestrator session ID to be set")
		}
	})

	t.Run("Create multiple sessions", func(t *testing.T) {
		req1 := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "session-1",
			},
		}

		req2 := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "session-2",
			},
		}

		resp1, err1 := svc.CreateGatewaySession(ctx, req1)
		resp2, err2 := svc.CreateGatewaySession(ctx, req2)

		if err1 != nil || err2 != nil {
			t.Fatalf("Failed to create sessions: %v, %v", err1, err2)
		}

		if resp1.SessionId == resp2.SessionId {
			t.Error("Expected different session IDs")
		}
	})
}

func TestGatewayService_GetGatewaySession(t *testing.T) {
	svc, _, ctx := setupGatewayService(t)

	t.Run("Get existing session", func(t *testing.T) {
		// Create a session first
		createReq := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}

		createResp, err := svc.CreateGatewaySession(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Get the session
		getReq := &proto.GetGatewaySessionRequest{
			SessionId: createResp.SessionId,
		}

		getResp, err := svc.GetGatewaySession(ctx, getReq)
		if err != nil {
			t.Fatalf("Failed to get session: %v", err)
		}

		if getResp.Session.Id != createResp.SessionId {
			t.Errorf("Expected session ID %s, got %s", createResp.SessionId, getResp.Session.Id)
		}

		if getResp.Session.Metadata.Name != "test-session" {
			t.Errorf("Expected name 'test-session', got %s", getResp.Session.Metadata.Name)
		}
	})

	t.Run("Get non-existent session fails", func(t *testing.T) {
		getReq := &proto.GetGatewaySessionRequest{
			SessionId: "non-existent",
		}

		_, err := svc.GetGatewaySession(ctx, getReq)
		if err == nil {
			t.Error("Expected error when getting non-existent session")
		}
	})

	t.Run("Get with empty session ID fails", func(t *testing.T) {
		getReq := &proto.GetGatewaySessionRequest{
			SessionId: "",
		}

		_, err := svc.GetGatewaySession(ctx, getReq)
		if err == nil {
			t.Error("Expected error when getting session with empty ID")
		}
	})
}

func TestGatewayService_ListGatewaySessions(t *testing.T) {
	svc, _, ctx := setupGatewayService(t)

	t.Run("List all sessions", func(t *testing.T) {
		// Create multiple sessions
		for i := 0; i < 3; i++ {
			createReq := &proto.CreateGatewaySessionRequest{
				Metadata: &proto.GatewaySessionMetadata{
					Name:   fmt.Sprintf("session-%d", i),
					UserId: "user-123",
				},
			}
			_, _ = svc.CreateGatewaySession(ctx, createReq)
		}

		listReq := &proto.ListGatewaySessionsRequest{}
		listResp, err := svc.ListGatewaySessions(ctx, listReq)

		if err != nil {
			t.Fatalf("Failed to list sessions: %v", err)
		}

		if listResp.TotalCount < 3 {
			t.Errorf("Expected at least 3 sessions, got %d", listResp.TotalCount)
		}

		if len(listResp.Sessions) != int(listResp.TotalCount) {
			t.Error("Sessions count mismatch")
		}
	})

	t.Run("List with filter by state", func(t *testing.T) {
		// Create a session
		createReq := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "filtered-session",
			},
		}
		createResp, _ := svc.CreateGatewaySession(ctx, createReq)

		// List with state filter
		listReq := &proto.ListGatewaySessionsRequest{
			Filter: &proto.GatewaySessionFilter{
				State: proto.GatewaySessionState_GATEWAY_SESSION_STATE_ACTIVE,
			},
		}
		listResp, err := svc.ListGatewaySessions(ctx, listReq)

		if err != nil {
			t.Fatalf("Failed to list sessions with filter: %v", err)
		}

		// Check that our created session is in the list
		found := false
		for _, sess := range listResp.Sessions {
			if sess.Id == createResp.SessionId {
				found = true
				break
			}
		}

		if !found {
			t.Error("Created session not found in filtered list")
		}
	})

	t.Run("List with filter by user_id", func(t *testing.T) {
		// Create sessions for different users
		createReq1 := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name:   "user1-session",
				UserId: "user-1",
			},
		}
		createResp1, _ := svc.CreateGatewaySession(ctx, createReq1)

		createReq2 := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name:   "user2-session",
				UserId: "user-2",
			},
		}
		_, _ = svc.CreateGatewaySession(ctx, createReq2)

		// List with user filter
		listReq := &proto.ListGatewaySessionsRequest{
			Filter: &proto.GatewaySessionFilter{
				UserId: "user-1",
			},
		}
		listResp, err := svc.ListGatewaySessions(ctx, listReq)

		if err != nil {
			t.Fatalf("Failed to list sessions with user filter: %v", err)
		}

		// Check that only user-1 sessions are returned
		for _, sess := range listResp.Sessions {
			if sess.Metadata.UserId != "user-1" {
				t.Errorf("Expected only user-1 sessions, got user %s", sess.Metadata.UserId)
			}
		}

		// Check that our session is in the list
		found := false
		for _, sess := range listResp.Sessions {
			if sess.Id == createResp1.SessionId {
				found = true
				break
			}
		}

		if !found {
			t.Error("Created user-1 session not found in filtered list")
		}
	})
}

func TestGatewayService_CloseGatewaySession(t *testing.T) {
	svc, _, ctx := setupGatewayService(t)

	t.Run("Close existing session", func(t *testing.T) {
		// Create a session
		createReq := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}
		createResp, _ := svc.CreateGatewaySession(ctx, createReq)

		// Close the session
		closeReq := &proto.CloseGatewaySessionRequest{
			SessionId: createResp.SessionId,
			Reason:    "Test close",
		}

		closeResp, err := svc.CloseGatewaySession(ctx, closeReq)
		if err != nil {
			t.Fatalf("Failed to close session: %v", err)
		}

		if closeResp.State != proto.GatewaySessionState_GATEWAY_SESSION_STATE_CLOSED {
			t.Errorf("Expected state CLOSED, got %v", closeResp.State)
		}

		// Verify session is no longer accessible
		getReq := &proto.GetGatewaySessionRequest{
			SessionId: createResp.SessionId,
		}
		_, err = svc.GetGatewaySession(ctx, getReq)
		if err == nil {
			t.Error("Expected error when getting closed session")
		}
	})

	t.Run("Close non-existent session fails", func(t *testing.T) {
		closeReq := &proto.CloseGatewaySessionRequest{
			SessionId: "non-existent",
		}

		_, err := svc.CloseGatewaySession(ctx, closeReq)
		if err == nil {
			t.Error("Expected error when closing non-existent session")
		}
	})

	t.Run("Close with empty session ID fails", func(t *testing.T) {
		closeReq := &proto.CloseGatewaySessionRequest{
			SessionId: "",
		}

		_, err := svc.CloseGatewaySession(ctx, closeReq)
		if err == nil {
			t.Error("Expected error when closing session with empty ID")
		}
	})
}

func TestGatewayService_GatewaySendMessage(t *testing.T) {
	svc, _, ctx := setupGatewayService(t)

	t.Run("Send message to existing session", func(t *testing.T) {
		// Create a session
		createReq := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}
		createResp, _ := svc.CreateGatewaySession(ctx, createReq)

		// Send a message
		sendReq := &proto.GatewaySendMessageRequest{
			SessionId: createResp.SessionId,
			Message: &proto.GatewayMessage{
				Id:        string(types.GenerateID()),
				Timestamp: timestamppb.Now(),
				Role:      proto.GatewayMessageRole_GATEWAY_MESSAGE_ROLE_USER,
				Content:   "Hello, gateway!",
				Metadata: &proto.GatewayMessageMetadata{
					SessionId: createResp.SessionId,
				},
			},
		}

		sendResp, err := svc.GatewaySendMessage(ctx, sendReq)
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}

		if sendResp.MessageId == "" {
			t.Error("Expected non-empty message ID")
		}

		if sendResp.SessionId != createResp.SessionId {
			t.Errorf("Expected session ID %s, got %s", createResp.SessionId, sendResp.SessionId)
		}
	})

	t.Run("Send message to non-existent session fails", func(t *testing.T) {
		sendReq := &proto.GatewaySendMessageRequest{
			SessionId: "non-existent",
			Message: &proto.GatewayMessage{
				Role:    proto.GatewayMessageRole_GATEWAY_MESSAGE_ROLE_USER,
				Content: "Hello!",
			},
		}

		_, err := svc.GatewaySendMessage(ctx, sendReq)
		if err == nil {
			t.Error("Expected error when sending to non-existent session")
		}
	})

	t.Run("Send with empty session ID fails", func(t *testing.T) {
		sendReq := &proto.GatewaySendMessageRequest{
			SessionId: "",
			Message: &proto.GatewayMessage{
				Role:    proto.GatewayMessageRole_GATEWAY_MESSAGE_ROLE_USER,
				Content: "Hello!",
			},
		}

		_, err := svc.GatewaySendMessage(ctx, sendReq)
		if err == nil {
			t.Error("Expected error when sending with empty session ID")
		}
	})

	t.Run("Send with nil message fails", func(t *testing.T) {
		createReq := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}
		createResp, _ := svc.CreateGatewaySession(ctx, createReq)

		sendReq := &proto.GatewaySendMessageRequest{
			SessionId: createResp.SessionId,
			Message:   nil,
		}

		_, err := svc.GatewaySendMessage(ctx, sendReq)
		if err == nil {
			t.Error("Expected error when sending nil message")
		}
	})
}

func TestGatewayService_HealthCheck(t *testing.T) {
	svc, _, ctx := setupGatewayService(t)

	t.Run("HealthCheck returns healthy", func(t *testing.T) {
		req := &emptypb.Empty{}
		resp, err := svc.HealthCheck(ctx, req)

		if err != nil {
			t.Fatalf("HealthCheck failed: %v", err)
		}

		if resp.Health != proto.Health_HEALTH_HEALTHY {
			t.Errorf("Expected health HEALTHY, got %v", resp.Health)
		}

		if resp.Version == "" {
			t.Error("Expected non-empty version")
		}

		if len(resp.Subsystems) == 0 {
			t.Error("Expected non-empty subsystems list")
		}
	})
}

func TestGatewayService_GetStatus(t *testing.T) {
	svc, _, ctx := setupGatewayService(t)

	t.Run("GetStatus returns running status", func(t *testing.T) {
		req := &emptypb.Empty{}
		resp, err := svc.GetStatus(ctx, req)

		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}

		if resp.Status != proto.Status_STATUS_RUNNING {
			t.Errorf("Expected status RUNNING, got %v", resp.Status)
		}

		if resp.StartedAt == nil {
			t.Error("Expected non-nil StartedAt")
		}

		if resp.Uptime == nil {
			t.Error("Expected non-nil Uptime")
		}
	})

	t.Run("GetStatus reflects active sessions", func(t *testing.T) {
		// Create a session
		createReq := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}
		_, _ = svc.CreateGatewaySession(ctx, createReq)

		// Get status
		req := &emptypb.Empty{}
		resp, _ := svc.GetStatus(ctx, req)

		if resp.ActiveSessions < 1 {
			t.Errorf("Expected at least 1 active session, got %d", resp.ActiveSessions)
		}
	})
}

func TestGatewayService_StreamResponses(t *testing.T) {
	svc, _, ctx := setupGatewayService(t)

	t.Run("StreamResponses sends chunks", func(t *testing.T) {
		// Create a session
		createReq := &proto.CreateGatewaySessionRequest{
			Metadata: &proto.GatewaySessionMetadata{
				Name: "test-session",
			},
		}
		createResp, _ := svc.CreateGatewaySession(ctx, createReq)

		// Create a mock server stream
		mockStream := &mockGatewayStreamResponsesServer{
			ctx: ctx,
			responses: make([]*proto.StreamResponsesResponse, 0, 10),
		}

		// Call StreamResponses
		req := &proto.StreamResponsesRequest{
			SessionId: createResp.SessionId,
			Options: &proto.StreamOptions{
				ChunkSize: 5,
			},
		}

		err := svc.StreamResponses(req, mockStream)
		if err != nil {
			t.Fatalf("StreamResponses failed: %v", err)
		}

		// Verify we received responses
		if len(mockStream.responses) == 0 {
			t.Error("Expected to receive at least one response")
		}

		// Verify we received chunks and status updates
		foundChunk := false
		foundStatus := false
		for _, resp := range mockStream.responses {
			if resp.GetChunk() != nil {
				foundChunk = true
			}
			if resp.GetStatus() != nil {
				foundStatus = true
			}
		}

		if !foundChunk {
			t.Error("Expected to receive at least one chunk")
		}
		if !foundStatus {
			t.Error("Expected to receive status updates")
		}
	})

	t.Run("StreamResponses with non-existent session fails", func(t *testing.T) {
		mockStream := &mockGatewayStreamResponsesServer{
			ctx: ctx,
			responses: make([]*proto.StreamResponsesResponse, 0),
		}

		req := &proto.StreamResponsesRequest{
			SessionId: "non-existent",
		}

		err := svc.StreamResponses(req, mockStream)
		if err == nil {
			t.Error("Expected error when streaming for non-existent session")
		}
	})
}

func TestGatewayService_Close(t *testing.T) {
	svc, _, _ := setupGatewayService(t)

	t.Run("Close closes all streams", func(t *testing.T) {
		// Create a mock stream and add it to the service
		mockStream := &mockGatewayStreamResponsesServer{
			ctx: context.Background(),
			responses: make([]*proto.StreamResponsesResponse, 0),
		}

		_, cancel := context.WithCancel(context.Background())
		svc.mu.Lock()
		svc.streams[mockStream] = cancel
		svc.mu.Unlock()

		// Close the service
		err := svc.Close()
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Verify stream was removed
		svc.mu.RLock()
		streamCount := len(svc.streams)
		svc.mu.RUnlock()

		if streamCount != 0 {
			t.Errorf("Expected 0 streams after close, got %d", streamCount)
		}
	})
}

func TestGatewayService_String(t *testing.T) {
	svc, _, _ := setupGatewayService(t)

	t.Run("String returns representation", func(t *testing.T) {
		str := svc.String()
		if str == "" {
			t.Error("Expected non-empty string representation")
		}
	})
}

// Mock implementations for testing

type mockGatewayStreamResponsesServer struct {
	proto.GatewayService_StreamResponsesServer
	ctx        context.Context
	responses  []*proto.StreamResponsesResponse
	sendError  error
}

func (m *mockGatewayStreamResponsesServer) Context() context.Context {
	return m.ctx
}

func (m *mockGatewayStreamResponsesServer) Send(resp *proto.StreamResponsesResponse) error {
	if m.sendError != nil {
		return m.sendError
	}
	m.responses = append(m.responses, resp)
	return nil
}

func (m *mockGatewayStreamResponsesServer) SetHeader(md metadata.MD) error {
	return nil
}

func (m *mockGatewayStreamResponsesServer) SendHeader(md metadata.MD) error {
	return nil
}

func (m *mockGatewayStreamResponsesServer) SetTrailer(md metadata.MD) {}

func (m *mockGatewayStreamResponsesServer) SendMsg(msg interface{}) error {
	return nil
}

func (m *mockGatewayStreamResponsesServer) RecvMsg(msg interface{}) error {
	return nil
}
