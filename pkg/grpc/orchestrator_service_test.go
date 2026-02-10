package grpc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/ipc"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// mockServer implements the Server interface for testing
type mockServer struct {
	sessionMgr *session.Manager
	eventBus   *events.Bus
	ipcBroker  *ipc.Broker
}

func (m *mockServer) SessionManager() *session.Manager {
	return m.sessionMgr
}

func (m *mockServer) EventBus() *events.Bus {
	return m.eventBus
}

func (m *mockServer) IPCBroker() *ipc.Broker {
	return m.ipcBroker // Can be nil for tests
}

// setupTestServer creates a test server with initialized dependencies
func setupTestServer(t *testing.T) *mockServer {
	t.Helper()

	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create session manager
	sessionCfg := config.DefaultSessionConfig()
	sessionCfg.StoragePath = t.TempDir()
	sessionMgr, err := session.New(sessionCfg, log)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Create event bus
	eventBus, err := events.New(log)
	if err != nil {
		t.Fatalf("Failed to create event bus: %v", err)
	}

	return &mockServer{
		sessionMgr: sessionMgr,
		eventBus:   eventBus,
	}
}

// teardownTestServer cleans up test resources
func teardownTestServer(t *testing.T, srv *mockServer) {
	t.Helper()

	if srv.sessionMgr != nil {
		_ = srv.sessionMgr.Close()
	}
	if srv.eventBus != nil {
		_ = srv.eventBus.Close()
	}
}

// TestOrchestratorService_CreateSession tests the CreateSession RPC
func TestOrchestratorService_CreateSession(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)

	ctx := context.Background()

	tests := []struct {
		name    string
		req     *proto.CreateSessionRequest
		wantErr bool
	}{
		{
			name: "valid session creation",
			req: &proto.CreateSessionRequest{
				Metadata: &proto.SessionMetadata{
					Name:    "test-session",
					OwnerId: "test-user",
				},
				Config: &proto.SessionConfig{
					MaxContainers: 5,
				},
			},
			wantErr: false,
		},
		{
			name: "session with labels",
			req: &proto.CreateSessionRequest{
				Metadata: &proto.SessionMetadata{
					Name:    "labeled-session",
					OwnerId: "test-user",
					Labels:  map[string]string{"env": "test", "team": "platform"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.CreateSession(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resp == nil {
					t.Error("CreateSession() returned nil response")
					return
				}

				if resp.SessionId == "" {
					t.Error("CreateSession() returned empty session_id")
				}

				if resp.Session == nil {
					t.Error("CreateSession() returned nil session")
				}

				if resp.Session != nil && resp.Session.Id != resp.SessionId {
					t.Errorf("CreateSession() session.ID mismatch: got %v, want %v", resp.Session.Id, resp.SessionId)
				}
			}
		})
	}
}

// TestOrchestratorService_GetSession tests the GetSession RPC
func TestOrchestratorService_GetSession(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx := context.Background()

	// Create a session first
	createResp, err := service.CreateSession(ctx, &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name:    "test-session",
			OwnerId: "test-user",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	tests := []struct {
		name    string
		req     *proto.GetSessionRequest
		wantErr bool
	}{
		{
			name: "valid session retrieval",
			req: &proto.GetSessionRequest{
				SessionId: createResp.SessionId,
			},
			wantErr: false,
		},
		{
			name: "non-existent session",
			req: &proto.GetSessionRequest{
				SessionId: "non-existent-id",
			},
			wantErr: true,
		},
		{
			name:    "empty session ID",
			req:     &proto.GetSessionRequest{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.GetSession(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && resp != nil {
				if resp.Session == nil {
					t.Error("GetSession() returned nil session")
				}
				if resp.Session != nil && resp.Session.Id != tt.req.SessionId {
					t.Errorf("GetSession() session.ID mismatch: got %v, want %v", resp.Session.Id, tt.req.SessionId)
				}
			}
		})
	}
}

// TestOrchestratorService_ListSessions tests the ListSessions RPC
func TestOrchestratorService_ListSessions(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx := context.Background()

	// Create some sessions
	for i := 0; i < 3; i++ {
		_, err := service.CreateSession(ctx, &proto.CreateSessionRequest{
			Metadata: &proto.SessionMetadata{
				Name:    fmt.Sprintf("test-session-%d", i),
				OwnerId: "test-user",
			},
		})
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}
	}

	tests := []struct {
		name    string
		req     *proto.ListSessionsRequest
		wantMin int
		wantErr bool
	}{
		{
			name:    "list all sessions",
			req:     &proto.ListSessionsRequest{},
			wantMin: 3,
			wantErr: false,
		},
		{
			name: "list with filter (no match)",
			req: &proto.ListSessionsRequest{
				Filter: &proto.SessionFilter{
					OwnerId: "non-existent-user",
				},
			},
			wantMin: 0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.ListSessions(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("ListSessions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resp == nil {
					t.Error("ListSessions() returned nil response")
					return
				}

				if len(resp.Sessions) < tt.wantMin {
					t.Errorf("ListSessions() returned %d sessions, want at least %d", len(resp.Sessions), tt.wantMin)
				}

				if resp.TotalCount != int32(len(resp.Sessions)) {
					t.Errorf("ListSessions() TotalCount mismatch: got %d, want %d", resp.TotalCount, len(resp.Sessions))
				}
			}
		})
	}
}

// TestOrchestratorService_UpdateSession tests the UpdateSession RPC
func TestOrchestratorService_UpdateSession(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx := context.Background()

	// Create a session
	createResp, err := service.CreateSession(ctx, &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name:    "test-session",
			OwnerId: "test-user",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Update the session
	updateResp, err := service.UpdateSession(ctx, &proto.UpdateSessionRequest{
		SessionId: createResp.SessionId,
		Metadata: &proto.SessionMetadata{
			Name:    "updated-session",
			OwnerId: "test-user",
			Labels:  map[string]string{"updated": "true"},
		},
	})

	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}

	if updateResp == nil {
		t.Fatal("UpdateSession() returned nil response")
	}

	if updateResp.Session.Metadata.Name != "updated-session" {
		t.Errorf("UpdateSession() name = %v, want %v", updateResp.Session.Metadata.Name, "updated-session")
	}
}

// TestOrchestratorService_CloseSession tests the CloseSession RPC
func TestOrchestratorService_CloseSession(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx := context.Background()

	// Create a session
	createResp, err := service.CreateSession(ctx, &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name:    "test-session",
			OwnerId: "test-user",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Close the session
	closeResp, err := service.CloseSession(ctx, &proto.CloseSessionRequest{
		SessionId: createResp.SessionId,
		Reason:    "test cleanup",
	})

	if err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}

	if closeResp == nil {
		t.Fatal("CloseSession() returned nil response")
	}

	if closeResp.SessionId != createResp.SessionId {
		t.Errorf("CloseSession() session_id = %v, want %v", closeResp.SessionId, createResp.SessionId)
	}
}

// TestOrchestratorService_SendMessage tests the SendMessage RPC
func TestOrchestratorService_SendMessage(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx := context.Background()

	// Create a session
	createResp, err := service.CreateSession(ctx, &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name:    "test-session",
			OwnerId: "test-user",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Send a message
	ts := types.NewTimestamp()
	msg := &proto.Message{
		Id:        string(types.GenerateID()),
		Role:      proto.MessageRole_MESSAGE_ROLE_USER,
		Content:   "Hello, orchestrator!",
		Timestamp: timestampToProto(&ts),
	}

	sendResp, err := service.SendMessage(ctx, &proto.SendMessageRequest{
		SessionId: createResp.SessionId,
		Message:   msg,
	})

	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if sendResp == nil {
		t.Fatal("SendMessage() returned nil response")
	}

	if sendResp.SessionId != createResp.SessionId {
		t.Errorf("SendMessage() session_id = %v, want %v", sendResp.SessionId, createResp.SessionId)
	}

	if sendResp.MessageId == "" {
		t.Error("SendMessage() returned empty message_id")
	}

	// Verify message was added to session
	getResp, err := service.GetSession(ctx, &proto.GetSessionRequest{
		SessionId: createResp.SessionId,
	})
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}

	if len(getResp.Session.Context.Messages) != 1 {
		t.Errorf("Session has %d messages, want 1", len(getResp.Session.Context.Messages))
	}
}

// TestOrchestratorService_HealthCheck tests the HealthCheck RPC
func TestOrchestratorService_HealthCheck(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx := context.Background()

	resp, err := service.HealthCheck(ctx, &emptypb.Empty{})

	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}

	if resp == nil {
		t.Fatal("HealthCheck() returned nil response")
	}

	if resp.Health != proto.Health_HEALTH_HEALTHY {
		t.Errorf("HealthCheck() health = %v, want %v", resp.Health, proto.Health_HEALTH_HEALTHY)
	}

	if resp.Version == "" {
		t.Error("HealthCheck() version is empty")
	}

	if len(resp.Subsystems) == 0 {
		t.Error("HealthCheck() subsystems is empty")
	}
}

// TestOrchestratorService_GetStatus tests the GetStatus RPC
func TestOrchestratorService_GetStatus(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx := context.Background()

	resp, err := service.GetStatus(ctx, &emptypb.Empty{})

	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}

	if resp == nil {
		t.Fatal("GetStatus() returned nil response")
	}

	if resp.Status != proto.Status_STATUS_RUNNING {
		t.Errorf("GetStatus() status = %v, want %v", resp.Status, proto.Status_STATUS_RUNNING)
	}

	if resp.StartedAt == nil {
		t.Error("GetStatus() started_at is nil")
	}

	if resp.Uptime == nil {
		t.Error("GetStatus() uptime is nil")
	}
}

// TestOrchestratorService_SubscribeEvents tests the SubscribeEvents RPC
func TestOrchestratorService_SubscribeEvents(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start event subscription in a goroutine
	eventCh := make(chan *proto.Event, 10)
	errCh := make(chan error, 1)

	go func() {
		errCh <- service.SubscribeEvents(&proto.SubscribeEventsRequest{}, &mockEventStream{ctx: ctx, ch: eventCh})
	}()

	// Publish an event
	time.Sleep(100 * time.Millisecond) // Give subscription time to register

	testEvent := types.Event{
		ID:        types.GenerateID(),
		Type:      "test.event",
		Source:    "test",
		Timestamp: types.NewTimestamp(),
		Metadata: types.EventMetadata{
			Priority: "normal",
		},
	}

	srv.eventBus.Publish(ctx, testEvent)

	// Check if we received the event
	select {
	case event := <-eventCh:
		if event == nil {
			t.Error("Received nil event")
		} else if event.Type != proto.EventType_EVENT_TYPE_UNSPECIFIED && event.Source != "test" {
			t.Errorf("Event source mismatch: got %v, want 'test'", event.Source)
		}
	case <-time.After(1 * time.Second):
		t.Error("Did not receive event within timeout")
	case err := <-errCh:
		if err != nil && err != context.DeadlineExceeded {
			t.Errorf("SubscribeEvents returned error: %v", err)
		}
	}
}

// mockEventStream implements the server streaming interface for testing
type mockEventStream struct {
	proto.OrchestratorService_SubscribeEventsServer
	ctx context.Context
	ch  chan<- *proto.Event
}

func (m *mockEventStream) Context() context.Context {
	return m.ctx
}

func (m *mockEventStream) Send(event *proto.Event) error {
	select {
	case m.ch <- event:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *mockEventStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockEventStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockEventStream) SetTrailer(metadata.MD) {
}

// TestOrchestratorService_StreamMessages tests the StreamMessages bidirectional RPC
func TestOrchestratorService_StreamMessages(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create a session first
	createResp, err := service.CreateSession(ctx, &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name:    "test-session",
			OwnerId: "test-user",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create mock stream
	stream := &mockMessageStream{
		ctx:       ctx,
		sessionID: createResp.SessionId,
		messages: []*proto.StreamMessageRequest{
			{
				SessionId: createResp.SessionId,
				Payload: &proto.StreamMessageRequest_Message{
					Message: &proto.Message{
						Id:      string(types.GenerateID()),
						Role:    proto.MessageRole_MESSAGE_ROLE_USER,
						Content: "Test message",
					},
				},
			},
		},
	}

	// Run the stream
	err = service.StreamMessages(stream)
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("StreamMessages() error = %v", err)
	}

	// Check that we received responses
	if len(stream.responses) == 0 {
		t.Error("StreamMessages() did not send any responses")
	}
}

// mockMessageStream implements the bidirectional streaming interface for testing
type mockMessageStream struct {
	proto.OrchestratorService_StreamMessagesServer
	ctx          context.Context
	sessionID    string
	messages     []*proto.StreamMessageRequest
	sentCount    int
	recvCount    int
	responses    []*proto.StreamMessageResponse
	_responsesMu sync.Mutex
}

func (m *mockMessageStream) Context() context.Context {
	return m.ctx
}

func (m *mockMessageStream) Send(resp *proto.StreamMessageResponse) error {
	m._responsesMu.Lock()
	defer m._responsesMu.Unlock()
	m.responses = append(m.responses, resp)
	return nil
}

func (m *mockMessageStream) Recv() (*proto.StreamMessageRequest, error) {
	if m.sentCount >= len(m.messages) {
		return nil, context.DeadlineExceeded
	}
	msg := m.messages[m.sentCount]
	m.sentCount++
	m.recvCount++
	return msg, nil
}

func (m *mockMessageStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockMessageStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockMessageStream) SetTrailer(metadata.MD) {
}

// TestOrchestratorService_ErrorHandling tests error handling in the service
func TestOrchestratorService_ErrorHandling(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewOrchestratorService(srv, nil)
	ctx := context.Background()

	tests := []struct {
		name    string
		req     interface{}
		method  string
		wantErr bool
	}{
		{
			name: "GetSession with empty ID",
			req: &proto.GetSessionRequest{
				SessionId: "",
			},
			method:  "GetSession",
			wantErr: true,
		},
		{
			name: "CloseSession with empty ID",
			req: &proto.CloseSessionRequest{
				SessionId: "",
			},
			method:  "CloseSession",
			wantErr: true,
		},
		{
			name: "SendMessage with empty session ID",
			req: &proto.SendMessageRequest{
				SessionId: "",
				Message:   &proto.Message{},
			},
			method:  "SendMessage",
			wantErr: true,
		},
		{
			name: "SendMessage with nil message",
			req: &proto.SendMessageRequest{
				SessionId: "some-id",
				Message:   nil,
			},
			method:  "SendMessage",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error

			switch tt.method {
			case "GetSession":
				_, err = service.GetSession(ctx, tt.req.(*proto.GetSessionRequest))
			case "CloseSession":
				_, err = service.CloseSession(ctx, tt.req.(*proto.CloseSessionRequest))
			case "SendMessage":
				_, err = service.SendMessage(ctx, tt.req.(*proto.SendMessageRequest))
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("%s() error = %v, wantErr %v", tt.method, err, tt.wantErr)
			}
		})
	}
}
