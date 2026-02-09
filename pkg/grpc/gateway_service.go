package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/ipc"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// GatewayServiceDependencies represents the gateway service dependencies
type GatewayServiceDependencies interface {
	SessionManager() *session.Manager
	EventBus() *events.Bus
	IPCBroker() *ipc.Broker
}

// GatewaySessionInfo holds information about a gateway session
type GatewaySessionInfo struct {
	ID                      string
	State                   proto.GatewaySessionState
	Metadata                *proto.GatewaySessionMetadata
	Config                  *proto.GatewaySessionConfig
	OrchestratorSessionID   string
	CreatedAt               types.Timestamp
	UpdatedAt               types.Timestamp
	ExpiresAt               *types.Timestamp
	ClosedAt                *types.Timestamp
	ActiveStreams           int
	mu                      sync.RWMutex
}

// GatewayRegistry manages gateway sessions
type GatewayRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*GatewaySessionInfo
	logger   *logger.Logger
}

// NewGatewayRegistry creates a new gateway registry
func NewGatewayRegistry(log *logger.Logger) *GatewayRegistry {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	return &GatewayRegistry{
		sessions: make(map[string]*GatewaySessionInfo),
		logger:   log.With("component", "gateway_registry"),
	}
}

// Add adds a gateway session to the registry
func (r *GatewayRegistry) Add(sessionID string, info *GatewaySessionInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[sessionID]; exists {
		return types.NewError(types.ErrCodeAlreadyExists, "gateway session already exists")
	}

	info.ID = sessionID
	info.CreatedAt = types.NewTimestamp()
	info.UpdatedAt = types.NewTimestamp()
	info.State = proto.GatewaySessionState_GATEWAY_SESSION_STATE_ACTIVE
	info.ActiveStreams = 0

	r.sessions[sessionID] = info

	r.logger.Info("Gateway session registered", "session_id", sessionID)

	return nil
}

// Get retrieves a gateway session by ID
func (r *GatewayRegistry) Get(sessionID string) (*GatewaySessionInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, "gateway session not found")
	}

	return sess, nil
}

// Remove removes a gateway session
func (r *GatewayRegistry) Remove(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "gateway session not found")
	}

	sess.mu.Lock()
	sess.State = proto.GatewaySessionState_GATEWAY_SESSION_STATE_CLOSED
	closedAt := types.NewTimestamp()
	sess.ClosedAt = &closedAt
	sess.mu.Unlock()

	delete(r.sessions, sessionID)

	r.logger.Info("Gateway session removed", "session_id", sessionID)

	return nil
}

// List returns all gateway sessions
func (r *GatewayRegistry) List() []*GatewaySessionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sessions := make([]*GatewaySessionInfo, 0, len(r.sessions))
	for _, sess := range r.sessions {
		sessions = append(sessions, sess)
	}

	return sessions
}

// UpdateState updates the state of a gateway session
func (r *GatewayRegistry) UpdateState(sessionID string, state proto.GatewaySessionState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "gateway session not found")
	}

	sess.mu.Lock()
	sess.State = state
	sess.UpdatedAt = types.NewTimestamp()
	sess.mu.Unlock()

	return nil
}

// IncrementStreamCount increments the active stream count for a session
func (r *GatewayRegistry) IncrementStreamCount(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "gateway session not found")
	}

	sess.mu.Lock()
	sess.ActiveStreams++
	sess.UpdatedAt = types.NewTimestamp()
	sess.mu.Unlock()

	return nil
}

// DecrementStreamCount decrements the active stream count for a session
func (r *GatewayRegistry) DecrementStreamCount(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "gateway session not found")
	}

	sess.mu.Lock()
	if sess.ActiveStreams > 0 {
		sess.ActiveStreams--
	}
	sess.UpdatedAt = types.NewTimestamp()
	sess.mu.Unlock()

	return nil
}

// GatewayService implements the gRPC GatewayService interface
type GatewayService struct {
	proto.UnimplementedGatewayServiceServer
	deps     GatewayServiceDependencies
	registry *GatewayRegistry
	logger   *logger.Logger
	mu       sync.RWMutex

	// Track active streams for graceful shutdown
	streams map[interface{}]context.CancelFunc

	// Track server start time for uptime calculation
	startedAt time.Time
}

// NewGatewayService creates a new gateway service
func NewGatewayService(deps GatewayServiceDependencies, log *logger.Logger) *GatewayService {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	return &GatewayService{
		deps:      deps,
		registry:  NewGatewayRegistry(log),
		logger:    log.With("component", "gateway_service"),
		streams:   make(map[interface{}]context.CancelFunc),
		startedAt: time.Now(),
	}
}

// GetRegistry returns the gateway registry
func (s *GatewayService) GetRegistry() *GatewayRegistry {
	return s.registry
}

// =============================================================================
// Session Management RPCs
// =============================================================================

// CreateGatewaySession creates a new gateway session for a user
func (s *GatewayService) CreateGatewaySession(ctx context.Context, req *proto.CreateGatewaySessionRequest) (*proto.CreateGatewaySessionResponse, error) {
	s.logger.Debug("CreateGatewaySession called", "name", req.Metadata.Name)

	mgr := s.deps.SessionManager()
	if mgr == nil {
		return nil, errInternal("session manager not available")
	}

	// Create an orchestrator session for this gateway session
	orchSessionMetadata := protoToGatewaySessionMetadataToSessionMetadata(req.Metadata)
	orchSessionConfig := protoToGatewaySessionConfigToSessionConfig(req.Config)

	orchSessionID, err := mgr.Create(ctx, orchSessionMetadata, orchSessionConfig)
	if err != nil {
		s.logger.Error("Failed to create orchestrator session", "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Generate gateway session ID
	gatewaySessionID := string(types.GenerateID())

	// Create gateway session info
	info := &GatewaySessionInfo{
		Metadata:              req.Metadata,
		Config:                req.Config,
		OrchestratorSessionID: string(orchSessionID),
	}

	if err := s.registry.Add(gatewaySessionID, info); err != nil {
		// Cleanup orchestrator session
		_ = mgr.CloseSession(ctx, orchSessionID)
		s.logger.Error("Failed to register gateway session", "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Gateway session created", "session_id", gatewaySessionID, "name", req.Metadata.Name)

	// Publish event to event bus
	bus := s.deps.EventBus()
	if bus != nil {
		event := types.Event{
			Type:      "gateway.session.created",
			Source:    "gateway.grpc",
			Timestamp: types.NewTimestamp(),
			Metadata: types.EventMetadata{
				SessionID: &orchSessionID,
				Priority:  types.PriorityNormal,
			},
			Data: map[string]interface{}{
				"gateway_session_id": gatewaySessionID,
				"orch_session_id":     string(orchSessionID),
				"name":                req.Metadata.Name,
				"user_id":             req.Metadata.UserId,
			},
		}
		if err := bus.Publish(ctx, event); err != nil {
			s.logger.Warn("Failed to publish gateway session created event", "error", err)
		}
	}

	// Get the orchestrator session
	orchSession, err := mgr.Get(ctx, orchSessionID)
	if err != nil {
		s.logger.Error("Failed to retrieve created orchestrator session", "session_id", orchSessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Build gateway session response
	gatewaySession := &proto.GatewaySession{
		Id:        gatewaySessionID,
		State:     proto.GatewaySessionState_GATEWAY_SESSION_STATE_ACTIVE,
		Status:    statusToProto(orchSession.Status),
		CreatedAt: timestampToProto(&info.CreatedAt),
		UpdatedAt: timestampToProto(&info.UpdatedAt),
		Metadata:  req.Metadata,
		Config:    req.Config,
		OrchestratorSessionId: string(orchSessionID),
	}

	return &proto.CreateGatewaySessionResponse{
		SessionId: gatewaySessionID,
		Session:   gatewaySession,
	}, nil
}

// GetGatewaySession retrieves a gateway session by its ID
func (s *GatewayService) GetGatewaySession(ctx context.Context, req *proto.GetGatewaySessionRequest) (*proto.GetGatewaySessionResponse, error) {
	s.logger.Debug("GetGatewaySession called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return nil, errInvalidArgument("session_id is required")
	}

	info, err := s.registry.Get(req.SessionId)
	if err != nil {
		s.logger.Error("Failed to get gateway session", "session_id", req.SessionId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Get orchestrator session for status
	mgr := s.deps.SessionManager()
	orchSessionID := types.ID(info.OrchestratorSessionID)
	orchSession, err := mgr.Get(ctx, orchSessionID)
	if err != nil {
		s.logger.Error("Failed to get orchestrator session", "session_id", orchSessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	info.mu.RLock()
	session := &proto.GatewaySession{
		Id:                      info.ID,
		State:                   info.State,
		Status:                  statusToProto(orchSession.Status),
		CreatedAt:               timestampToProto(&info.CreatedAt),
		UpdatedAt:               timestampToProto(&info.UpdatedAt),
		Metadata:                info.Metadata,
		Config:                  info.Config,
		OrchestratorSessionId:   info.OrchestratorSessionID,
	}
	info.mu.RUnlock()

	if info.ExpiresAt != nil {
		info.mu.RLock()
		session.ExpiresAt = timestampToProto(info.ExpiresAt)
		info.mu.RUnlock()
	}
	if info.ClosedAt != nil {
		info.mu.RLock()
		session.ClosedAt = timestampToProto(info.ClosedAt)
		info.mu.RUnlock()
	}

	return &proto.GetGatewaySessionResponse{
		Session: session,
	}, nil
}

// ListGatewaySessions retrieves all gateway sessions matching the specified filter
func (s *GatewayService) ListGatewaySessions(ctx context.Context, req *proto.ListGatewaySessionsRequest) (*proto.ListGatewaySessionsResponse, error) {
	s.logger.Debug("ListGatewaySessions called")

	allSessions := s.registry.List()

	// Apply filter
	var filteredSessions []*GatewaySessionInfo
	if req.Filter != nil {
		for _, sess := range allSessions {
			sess.mu.RLock()
			match := true

			// Filter by state
			if req.Filter.State != proto.GatewaySessionState_GATEWAY_SESSION_STATE_UNSPECIFIED {
				if sess.State != req.Filter.State {
					match = false
				}
			}

			// Filter by status (need to get from orchestrator session)
			if match && req.Filter.Status != proto.Status_STATUS_UNSPECIFIED {
				mgr := s.deps.SessionManager()
				orchSessionID := types.ID(sess.OrchestratorSessionID)
				orchSession, err := mgr.Get(ctx, orchSessionID)
				if err == nil && statusToProto(orchSession.Status) != req.Filter.Status {
					match = false
				}
			}

			// Filter by user_id
			if match && req.Filter.UserId != "" {
				if sess.Metadata != nil && sess.Metadata.UserId != req.Filter.UserId {
					match = false
				}
			}

			// Filter by labels (match all)
			if match && len(req.Filter.Labels) > 0 {
				if sess.Metadata == nil {
					match = false
				} else {
					for k, v := range req.Filter.Labels {
						if sess.Metadata.Labels == nil || sess.Metadata.Labels[k] != v {
							match = false
							break
						}
					}
				}
			}
			sess.mu.RUnlock()

			if match {
				filteredSessions = append(filteredSessions, sess)
			}
		}
	} else {
		filteredSessions = allSessions
	}

	// Convert to protobuf
	pbSessions := make([]*proto.GatewaySession, len(filteredSessions))
	for i, sess := range filteredSessions {
		sess.mu.RLock()

		// Get orchestrator session for status
		mgr := s.deps.SessionManager()
		orchSessionID := types.ID(sess.OrchestratorSessionID)
		orchSession, _ := mgr.Get(ctx, orchSessionID)

		var sessionStatus proto.Status
		if orchSession != nil {
			sessionStatus = statusToProto(orchSession.Status)
		}

		pbSessions[i] = &proto.GatewaySession{
			Id:                      sess.ID,
			State:                   sess.State,
			Status:                  sessionStatus,
			CreatedAt:               timestampToProto(&sess.CreatedAt),
			UpdatedAt:               timestampToProto(&sess.UpdatedAt),
			Metadata:                sess.Metadata,
			Config:                  sess.Config,
			OrchestratorSessionId:   sess.OrchestratorSessionID,
		}
		sess.mu.RUnlock()
	}

	return &proto.ListGatewaySessionsResponse{
		Sessions:   pbSessions,
		TotalCount: int32(len(pbSessions)),
	}, nil
}

// CloseGatewaySession closes a gateway session
func (s *GatewayService) CloseGatewaySession(ctx context.Context, req *proto.CloseGatewaySessionRequest) (*proto.CloseGatewaySessionResponse, error) {
	s.logger.Debug("CloseGatewaySession called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return nil, errInvalidArgument("session_id is required")
	}

	info, err := s.registry.Get(req.SessionId)
	if err != nil {
		s.logger.Error("Failed to get gateway session", "session_id", req.SessionId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Close the orchestrator session
	mgr := s.deps.SessionManager()
	orchSessionID := types.ID(info.OrchestratorSessionID)
	if err := mgr.CloseSession(ctx, orchSessionID); err != nil {
		s.logger.Error("Failed to close orchestrator session", "session_id", orchSessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Remove gateway session
	if err := s.registry.Remove(req.SessionId); err != nil {
		s.logger.Error("Failed to remove gateway session", "session_id", req.SessionId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Gateway session closed", "session_id", req.SessionId, "reason", req.Reason)

	// Publish event to event bus
	bus := s.deps.EventBus()
	if bus != nil {
		event := types.Event{
			Type:      "gateway.session.closed",
			Source:    "gateway.grpc",
			Timestamp: types.NewTimestamp(),
			Metadata: types.EventMetadata{
				SessionID: &orchSessionID,
				Priority:  types.PriorityNormal,
			},
			Data: map[string]interface{}{
				"gateway_session_id": req.SessionId,
				"orch_session_id":     string(orchSessionID),
				"reason":              req.Reason,
			},
		}
		if err := bus.Publish(ctx, event); err != nil {
			s.logger.Warn("Failed to publish gateway session closed event", "error", err)
		}
	}

	return &proto.CloseGatewaySessionResponse{
		SessionId: req.SessionId,
		State:     proto.GatewaySessionState_GATEWAY_SESSION_STATE_CLOSED,
	}, nil
}

// =============================================================================
// Message Handling RPCs
// =============================================================================

// GatewaySendMessage sends a message to a gateway session
func (s *GatewayService) GatewaySendMessage(ctx context.Context, req *proto.GatewaySendMessageRequest) (*proto.GatewaySendMessageResponse, error) {
	s.logger.Debug("GatewaySendMessage called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return nil, errInvalidArgument("session_id is required")
	}
	if req.Message == nil {
		return nil, errInvalidArgument("message is required")
	}

	info, err := s.registry.Get(req.SessionId)
	if err != nil {
		s.logger.Error("Failed to get gateway session", "session_id", req.SessionId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	mgr := s.deps.SessionManager()
	orchSessionID := types.ID(info.OrchestratorSessionID)

	// Convert gateway message to orchestrator message
	message := protoToGatewayMessage(req.Message)

	// Generate ID and timestamp before adding to avoid race conditions
	// (AddMessage would generate these internally, but we need them for the response)
	if message.ID.IsEmpty() {
		message.ID = types.GenerateID()
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = types.NewTimestampFromTime(time.Now())
	}

	// Add message to session
	if err := mgr.AddMessage(ctx, orchSessionID, message); err != nil {
		s.logger.Error("Failed to send message", "session_id", req.SessionId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	messageID := string(message.ID)

	s.logger.Debug("Message sent to gateway session", "session_id", req.SessionId, "message_id", messageID)

	return &proto.GatewaySendMessageResponse{
		MessageId:  messageID,
		SessionId:  req.SessionId,
		Timestamp:  timestampToProto(&message.Timestamp),
	}, nil
}

// StreamChat establishes a bidirectional stream for real-time chat with response streaming
func (s *GatewayService) StreamChat(stream proto.GatewayService_StreamChatServer) error {
	ctx := stream.Context()
	s.logger.Debug("StreamChat started")

	// Track this stream for cleanup
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.mu.Lock()
	s.streams[stream] = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.streams, stream)
		s.mu.Unlock()
	}()

	var sessionID string
	var gatewaySessionInfo *GatewaySessionInfo
	var firstMessage = true

	// Receive loop
	for {
		req, err := stream.Recv()
		if err != nil {
			if firstMessage {
				s.logger.Debug("StreamChat ended without receiving any message")
				return err
			}
			// Client closed stream
			if gatewaySessionInfo != nil {
				s.logger.Debug("StreamChat receive loop ended", "session_id", sessionID)
				s.registry.DecrementStreamCount(sessionID)
			}
			return nil
		}

		// Handle heartbeat
		if req.GetHeartbeat() != nil {
			// Send heartbeat response
			if err := stream.Send(&proto.StreamChatResponse{
				Payload: &proto.StreamChatResponse_Heartbeat{&emptypb.Empty{}},
			}); err != nil {
				s.logger.Error("Failed to send heartbeat response", "error", err)
				return err
			}
			continue
		}

		// First message must contain session_id
		if firstMessage {
			if req.SessionId == "" {
				return errInvalidArgument("first message must contain session_id")
			}
			sessionID = req.SessionId
			firstMessage = false

			// Validate gateway session exists
			info, err := s.registry.Get(sessionID)
			if err != nil {
				s.logger.Error("Gateway session not found", "session_id", sessionID, "error", err)
				return grpcErrorFromTypesError(err)
			}
			gatewaySessionInfo = info

			// Increment stream count
			_ = s.registry.IncrementStreamCount(sessionID)

			s.logger.Debug("StreamChat established for gateway session", "session_id", sessionID)
		}

		// Handle cancel
		if req.GetCancel() != nil {
			s.logger.Debug("StreamChat cancel received", "session_id", sessionID, "reason", req.GetCancel().Reason)
			return nil
		}

		// Process message
		if req.GetMessage() != nil {
			mgr := s.deps.SessionManager()
			orchSessionID := types.ID(gatewaySessionInfo.OrchestratorSessionID)

			// Convert gateway message to orchestrator message
			message := protoToGatewayMessage(req.GetMessage())

			// Add message to orchestrator session
			if err := mgr.AddMessage(streamCtx, orchSessionID, message); err != nil {
				s.logger.Error("Failed to add message in stream", "session_id", sessionID, "error", err)
				// Send error as status
				resp := &proto.StreamChatResponse{
					Payload: &proto.StreamChatResponse_Status{
						Status: &proto.StreamStatus{
							State:   proto.StreamState_STREAM_STATE_ERROR,
							Message: err.Error(),
						},
					},
				}
				_ = stream.Send(resp)
				continue
			}

			// Echo message back as acknowledgment
			resp := &proto.StreamChatResponse{
				Payload: &proto.StreamChatResponse_Chunk{
					Chunk: &proto.ResponseChunk{
						ChunkId:  string(types.GenerateID()),
						Sequence: 0,
						Text:     message.Content,
						IsFinal:  true,
						Metadata: &proto.ChunkMetadata{
							Timestamp:   timestampToProto(&message.Timestamp),
							ContentType: "text/plain",
						},
					},
				},
			}
			if err := stream.Send(resp); err != nil {
				s.logger.Error("Failed to send chat response", "session_id", sessionID, "error", err)
				return err
			}
		}
	}
}

// StreamResponses establishes a server-side stream for receiving response chunks
func (s *GatewayService) StreamResponses(req *proto.StreamResponsesRequest, stream proto.GatewayService_StreamResponsesServer) error {
	ctx := stream.Context()
	s.logger.Debug("StreamResponses called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return errInvalidArgument("session_id is required")
	}

	// Validate gateway session exists
	_, err := s.registry.Get(req.SessionId)
	if err != nil {
		s.logger.Error("Gateway session not found", "session_id", req.SessionId, "error", err)
		return grpcErrorFromTypesError(err)
	}

	// Track this stream for cleanup
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.mu.Lock()
	s.streams[stream] = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.streams, stream)
		s.mu.Unlock()
		s.registry.DecrementStreamCount(req.SessionId)
	}()

	// Increment stream count
	_ = s.registry.IncrementStreamCount(req.SessionId)

	// Send initial status
	startedAt := types.NewTimestamp()
	if err := stream.Send(&proto.StreamResponsesResponse{
		Payload: &proto.StreamResponsesResponse_Status{
			Status: &proto.StreamStatus{
				State:     proto.StreamState_STREAM_STATE_ACTIVE,
				StartedAt: timestampToProto(&startedAt),
			},
		},
	}); err != nil {
		s.logger.Error("Failed to send initial status", "session_id", req.SessionId, "error", err)
		return err
	}

	// Simulate streaming response chunks
	// In a real implementation, this would subscribe to LLM response events
	// from the orchestrator and stream them as they arrive
	chunkID := string(types.GenerateID())
	responseText := "This is a simulated response from the gateway service."
	chunkSize := 10
	if req.Options != nil && req.Options.ChunkSize > 0 {
		chunkSize = int(req.Options.ChunkSize)
	}

	for i := 0; i < len(responseText); i += chunkSize {
		select {
		case <-streamCtx.Done():
			s.logger.Debug("StreamResponses context done", "session_id", req.SessionId)
			return nil
		default:
		}

		end := i + chunkSize
		if end > len(responseText) {
			end = len(responseText)
		}

		chunk := &proto.ResponseChunk{
			ChunkId:  chunkID,
			Sequence: int32(i / chunkSize),
			Text:     responseText[i:end],
			IsDelta:  true,
			IsFinal:  end >= len(responseText),
			Metadata: &proto.ChunkMetadata{
				Timestamp:    timestampToProtoValue(types.NewTimestamp()),
				ContentType:  "text/plain",
				TotalBytes:   int64(len(responseText)),
				Progress:     float64(end) / float64(len(responseText)),
			},
		}

		if err := stream.Send(&proto.StreamResponsesResponse{
			Payload: &proto.StreamResponsesResponse_Chunk{
				Chunk: chunk,
			},
		}); err != nil {
			s.logger.Error("Failed to send response chunk", "session_id", req.SessionId, "error", err)
			return err
		}

		// Small delay between chunks
		time.Sleep(50 * time.Millisecond)
	}

	// Send final status
	endedAt := types.NewTimestamp()
	if err := stream.Send(&proto.StreamResponsesResponse{
		Payload: &proto.StreamResponsesResponse_Status{
			Status: &proto.StreamStatus{
				State:      proto.StreamState_STREAM_STATE_COMPLETE,
				BytesSent:  int64(len(responseText)),
				ChunksSent: int64(len(responseText)/chunkSize + 1),
				StartedAt:  timestampToProto(&startedAt),
				EndedAt:    timestampToProto(&endedAt),
			},
		},
	}); err != nil {
		s.logger.Error("Failed to send final status", "session_id", req.SessionId, "error", err)
		return err
	}

	s.logger.Debug("StreamResponses completed", "session_id", req.SessionId)

	return nil
}

// =============================================================================
// Event Subscription RPCs
// =============================================================================

// SubscribeToEvents subscribes to gateway events for a session
func (s *GatewayService) SubscribeToEvents(req *proto.SubscribeToEventsRequest, stream proto.GatewayService_SubscribeToEventsServer) error {
	ctx := stream.Context()
	s.logger.Debug("SubscribeToEvents called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return errInvalidArgument("session_id is required")
	}

	// Validate gateway session exists
	_, err := s.registry.Get(req.SessionId)
	if err != nil {
		s.logger.Error("Gateway session not found", "session_id", req.SessionId, "error", err)
		return grpcErrorFromTypesError(err)
	}

	bus := s.deps.EventBus()
	if bus == nil {
		return errInternal("event bus not available")
	}

	// Create event channel
	eventCh := make(chan types.Event, 100)
	defer close(eventCh)

	// Create event handler that sends events to the channel
	handler := &eventStreamHandler{
		eventCh: eventCh,
	}

	// Build filter from request
	var filter types.EventFilter
	if req.Filter != nil {
		// Filter by session
		sessionID := types.ID(req.SessionId)
		filter.SessionID = &sessionID
	}

	// Subscribe to events
	subID, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		s.logger.Error("Failed to subscribe to events", "error", err)
		return grpcErrorFromTypesError(err)
	}
	defer bus.Unsubscribe(subID)

	s.logger.Info("Gateway event subscription created", "subscription_id", subID, "session_id", req.SessionId)

	// Send events to stream
	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("Event subscription ended by context", "subscription_id", subID)
			return nil
		case event, ok := <-eventCh:
			if !ok {
				s.logger.Debug("Event channel closed", "subscription_id", subID)
				return nil
			}

			// Convert orchestrator event to gateway event
			gatewayEvent := &proto.GatewayEvent{
				Id:        string(event.ID),
				Type:      eventTypeToGatewayEventType(event.Type),
				Source:    event.Source,
				Timestamp: timestampToProto(&event.Timestamp),
				Metadata: &proto.GatewayEventMetadata{
					SessionId: req.SessionId,
					Priority:  priorityToProto(string(event.Metadata.Priority)),
				},
			}

			// Convert data map
			if event.Data != nil {
				gatewayEvent.Data = make(map[string]string)
				for k, v := range event.Data {
					gatewayEvent.Data[k] = fmt.Sprintf("%v", v)
				}
			}

			// Convert labels
			if event.Metadata.Labels != nil {
				gatewayEvent.Metadata.Labels = event.Metadata.Labels
			}

			if err := stream.Send(gatewayEvent); err != nil {
				s.logger.Error("Failed to send gateway event", "subscription_id", subID, "error", err)
				return err
			}
		}
	}
}

// =============================================================================
// Health and Status RPCs
// =============================================================================

// HealthCheck returns the health status of the gateway
func (s *GatewayService) HealthCheck(ctx context.Context, req *emptypb.Empty) (*proto.GatewayHealthCheckResponse, error) {
	s.logger.Debug("HealthCheck called")

	// Check dependencies
	mgr := s.deps.SessionManager()
	bus := s.deps.EventBus()
	var health proto.Health
	if mgr != nil && bus != nil {
		health = proto.Health_HEALTH_HEALTHY
	} else {
		health = proto.Health_HEALTH_UNHEALTHY
	}

	// TODO: Add version from build info
	version := "dev"

	subsystems := []string{
		"session_manager",
		"event_bus",
		"gateway_registry",
	}

	return &proto.GatewayHealthCheckResponse{
		Health:    health,
		Version:   version,
		Subsystems: subsystems,
	}, nil
}

// GetStatus returns the current status of the gateway
func (s *GatewayService) GetStatus(ctx context.Context, req *emptypb.Empty) (*proto.GatewayStatusResponse, error) {
	s.logger.Debug("GetStatus called")

	s.mu.RLock()
	startedAt := s.startedAt
	s.mu.RUnlock()

	// Get session stats
	sessions := s.registry.List()
	activeSessions := 0
	activeStreams := 0

	for _, sess := range sessions {
		sess.mu.RLock()
		if sess.State == proto.GatewaySessionState_GATEWAY_SESSION_STATE_ACTIVE ||
		   sess.State == proto.GatewaySessionState_GATEWAY_SESSION_STATE_IDLE {
			activeSessions++
		}
		activeStreams += sess.ActiveStreams
		sess.mu.RUnlock()
	}

	uptime := time.Since(startedAt)
	startedTs := types.NewTimestampFromTime(startedAt)
	uptimeTs := types.NewTimestampFromTime(startedAt.Add(uptime))

	return &proto.GatewayStatusResponse{
		Status:         proto.Status_STATUS_RUNNING,
		ActiveSessions: int32(activeSessions),
		ActiveStreams:  int32(activeStreams),
		StartedAt:      timestampToProto(&startedTs),
		Uptime:         timestampToProto(&uptimeTs),
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// eventTypeToGatewayEventType converts orchestrator event type to gateway event type
func eventTypeToGatewayEventType(eventType types.EventType) proto.GatewayEventType {
	switch eventType {
	case "session.created":
		return proto.GatewayEventType_GATEWAY_EVENT_TYPE_SESSION_CREATED
	case "session.updated":
		return proto.GatewayEventType_GATEWAY_EVENT_TYPE_SESSION_UPDATED
	case "session.closed":
		return proto.GatewayEventType_GATEWAY_EVENT_TYPE_SESSION_CLOSED
	default:
		return proto.GatewayEventType_GATEWAY_EVENT_TYPE_UNSPECIFIED
	}
}

// protoToGatewaySessionMetadataToSessionMetadata converts gateway session metadata to orchestrator session metadata
func protoToGatewaySessionMetadataToSessionMetadata(m *proto.GatewaySessionMetadata) types.SessionMetadata {
	if m == nil {
		return types.SessionMetadata{}
	}

	return types.SessionMetadata{
		Name:        m.Name,
		Description: m.Description,
		OwnerID:     m.UserId,
		Labels:      m.Labels,
		Tags:        m.Tags,
	}
}

// protoToGatewaySessionConfigToSessionConfig converts gateway session config to orchestrator session config
func protoToGatewaySessionConfigToSessionConfig(c *proto.GatewaySessionConfig) types.SessionConfig {
	if c == nil {
		return types.SessionConfig{}
	}

	cfg := types.SessionConfig{}

	if c.TimeoutNs > 0 {
		cfg.MaxDuration = time.Duration(c.TimeoutNs)
	}
	if c.IdleTimeoutNs > 0 {
		cfg.IdleTimeout = time.Duration(c.IdleTimeoutNs)
	}

	return cfg
}

// protoToGatewayMessage converts protobuf GatewayMessage to types.Message
func protoToGatewayMessage(m *proto.GatewayMessage) types.Message {
	if m == nil {
		return types.Message{}
	}

	msg := types.Message{
		ID:        types.ID(m.Id),
		Role:      protoToGatewayMessageRole(m.Role),
		Content:   m.Content,
		Metadata:  protoToGatewayMessageMetadata(m.Metadata),
	}

	if m.Timestamp != nil {
		ts := protoToTimestamp(m.Timestamp)
		if ts != nil {
			msg.Timestamp = *ts
		}
	}

	return msg
}

// protoToGatewayMessageRole converts protobuf GatewayMessageRole to types.MessageRole
func protoToGatewayMessageRole(role proto.GatewayMessageRole) types.MessageRole {
	switch role {
	case proto.GatewayMessageRole_GATEWAY_MESSAGE_ROLE_USER:
		return types.MessageRoleUser
	case proto.GatewayMessageRole_GATEWAY_MESSAGE_ROLE_SYSTEM:
		return types.MessageRoleSystem
	case proto.GatewayMessageRole_GATEWAY_MESSAGE_ROLE_ASSISTANT:
		return types.MessageRoleAssistant
	default:
		return types.MessageRoleUser
	}
}

// protoToGatewayMessageMetadata converts protobuf GatewayMessageMetadata to types.MessageMetadata
func protoToGatewayMessageMetadata(m *proto.GatewayMessageMetadata) types.MessageMetadata {
	if m == nil {
		return types.MessageMetadata{}
	}

	meta := types.MessageMetadata{
		ToolName:   m.Model,
		ToolCallID: m.CorrelationId,
		Extra:      m.Extra,
	}

	if m.SessionId != "" {
		sessionID := types.ID(m.SessionId)
		meta.ContainerID = &sessionID
	}

	return meta
}

// Close closes all active streams
func (s *GatewayService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Closing gateway service streams", "count", len(s.streams))

	for stream, cancel := range s.streams {
		cancel()
		delete(s.streams, stream)
	}

	return nil
}

// String returns a string representation of the service
func (s *GatewayService) String() string {
	s.mu.RLock()
	activeStreams := len(s.streams)
	s.mu.RUnlock()

	return fmt.Sprintf("GatewayService{active_streams: %d}", activeStreams)
}

// =============================================================================
// GatewaySessionState conversion functions
// =============================================================================

// gatewaySessionStateToProto converts proto.GatewaySessionState to itself (identity function for consistency)
func gatewaySessionStateToProto(state proto.GatewaySessionState) proto.GatewaySessionState {
	return state
}

// timestampToProtoValue converts types.Timestamp value to protobuf timestamp
func timestampToProtoValue(ts types.Timestamp) *timestamppb.Timestamp {
	return timestamppb.New(ts.Time)
}
