package grpc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/ipc"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// ServiceDependencies represents the orchestrator service dependencies
type ServiceDependencies interface {
	SessionManager() *session.Manager
	EventBus() *events.Bus
	IPCBroker() *ipc.Broker
	AgentService() *AgentService
}

// OrchestratorService implements the gRPC OrchestratorService interface
type OrchestratorService struct {
	proto.UnimplementedOrchestratorServiceServer
	deps   ServiceDependencies
	logger *logger.Logger
	mu     sync.RWMutex

	// Track active streams for graceful shutdown
	streams map[interface{}]context.CancelFunc

	// Track StreamMessages streams by session ID for message routing
	sessionStreams map[types.ID]*sessionStreamState
}

type sessionStreamState struct {
	stream proto.OrchestratorService_StreamMessagesServer
	mu     sync.Mutex
}

// NewOrchestratorService creates a new orchestrator service
func NewOrchestratorService(deps ServiceDependencies, log *logger.Logger) *OrchestratorService {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			// If we can't create a logger, create a no-op fallback
			log = &logger.Logger{}
		}
	}

	return &OrchestratorService{
		deps:           deps,
		logger:         log.With("component", "orchestrator_service"),
		streams:        make(map[interface{}]context.CancelFunc),
		sessionStreams: make(map[types.ID]*sessionStreamState),
	}
}

func (s *OrchestratorService) sendToSessionStream(sessionID types.ID, resp *proto.StreamMessageResponse) error {
	s.mu.RLock()
	streamState, exists := s.sessionStreams[sessionID]
	s.mu.RUnlock()

	if !exists || streamState == nil || streamState.stream == nil {
		return types.NewError(types.ErrCodeNotFound, "no active stream for session")
	}

	streamState.mu.Lock()
	defer streamState.mu.Unlock()

	if err := streamState.stream.Send(resp); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to send response", err)
	}

	return nil
}

func streamErrorResponse(message string) *proto.StreamMessageResponse {
	ts := types.NewTimestamp()
	return &proto.StreamMessageResponse{
		Payload: &proto.StreamMessageResponse_Event{
			Event: &proto.Event{
				Id:        string(types.GenerateID()),
				Type:      proto.EventType_EVENT_TYPE_CONTAINER_ERROR,
				Source:    "orchestrator",
				Timestamp: timestampToProto(&ts),
				Data:      map[string]string{"error": message},
			},
		},
	}
}

// =============================================================================
// Session Management RPCs
// =============================================================================

// CreateSession creates a new session with the specified metadata and configuration
func (s *OrchestratorService) CreateSession(ctx context.Context, req *proto.CreateSessionRequest) (*proto.CreateSessionResponse, error) {
	s.logger.Debug("CreateSession called", "name", req.Metadata.Name)

	mgr := s.deps.SessionManager()
	if mgr == nil {
		return nil, errInternal("session manager not available")
	}

	// Convert protobuf to types
	metadata := protoToSessionMetadata(req.Metadata)
	config := protoToSessionConfig(req.Config)

	// Create session
	sessionID, err := mgr.Create(ctx, metadata, config)
	if err != nil {
		s.logger.Error("Failed to create session", "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Get the created session
	sess, err := mgr.Get(ctx, sessionID)
	if err != nil {
		s.logger.Error("Failed to retrieve created session", "session_id", sessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Session created", "session_id", sessionID, "name", metadata.Name)

	// Publish event to event bus
	bus := s.deps.EventBus()
	if bus != nil {
		event := types.Event{
			Type:      "session.created",
			Source:    "orchestrator.grpc",
			Timestamp: types.NewTimestamp(),
			Metadata: types.EventMetadata{
				SessionID: &sessionID,
				Priority:  types.PriorityNormal,
			},
			Data: map[string]interface{}{
				"session_id": string(sessionID),
				"name":       metadata.Name,
				"owner_id":   metadata.OwnerID,
			},
		}
		if err := bus.Publish(ctx, event); err != nil {
			s.logger.Warn("Failed to publish session created event", "error", err)
		}
	}

	return &proto.CreateSessionResponse{
		SessionId: string(sessionID),
		Session:   sessionToProto(sess),
	}, nil
}

// GetSession retrieves a session by its ID
func (s *OrchestratorService) GetSession(ctx context.Context, req *proto.GetSessionRequest) (*proto.GetSessionResponse, error) {
	s.logger.Debug("GetSession called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return nil, errInvalidArgument("session_id is required")
	}

	mgr := s.deps.SessionManager()
	if mgr == nil {
		return nil, errInternal("session manager not available")
	}

	sessionID := types.ID(req.SessionId)
	sess, err := mgr.Get(ctx, sessionID)
	if err != nil {
		s.logger.Error("Failed to get session", "session_id", sessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	return &proto.GetSessionResponse{
		Session: sessionToProto(sess),
	}, nil
}

// ListSessions retrieves all sessions matching the specified filter
func (s *OrchestratorService) ListSessions(ctx context.Context, req *proto.ListSessionsRequest) (*proto.ListSessionsResponse, error) {
	s.logger.Debug("ListSessions called")

	mgr := s.deps.SessionManager()
	if mgr == nil {
		return nil, errInternal("session manager not available")
	}

	filter := protoToSessionFilter(req.Filter)
	sessions, err := mgr.List(ctx, filter)
	if err != nil {
		s.logger.Error("Failed to list sessions", "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	pbSessions := make([]*proto.Session, len(sessions))
	for i, sess := range sessions {
		pbSessions[i] = sessionToProto(sess)
	}

	return &proto.ListSessionsResponse{
		Sessions:   pbSessions,
		TotalCount: int32(len(sessions)),
	}, nil
}

// UpdateSession updates a session's metadata or context
func (s *OrchestratorService) UpdateSession(ctx context.Context, req *proto.UpdateSessionRequest) (*proto.UpdateSessionResponse, error) {
	s.logger.Debug("UpdateSession called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return nil, errInvalidArgument("session_id is required")
	}

	mgr := s.deps.SessionManager()
	if mgr == nil {
		return nil, errInternal("session manager not available")
	}

	sessionID := types.ID(req.SessionId)

	// Update metadata if provided
	if req.Metadata != nil {
		metadata := protoToSessionMetadata(req.Metadata)
		if err := mgr.Update(ctx, sessionID, metadata); err != nil {
			s.logger.Error("Failed to update session metadata", "session_id", sessionID, "error", err)
			return nil, grpcErrorFromTypesError(err)
		}
	}

	// Get the updated session
	sess, err := mgr.Get(ctx, sessionID)
	if err != nil {
		s.logger.Error("Failed to retrieve updated session", "session_id", sessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Session updated", "session_id", sessionID)

	return &proto.UpdateSessionResponse{
		Session: sessionToProto(sess),
	}, nil
}

// CloseSession closes a session and releases its resources
func (s *OrchestratorService) CloseSession(ctx context.Context, req *proto.CloseSessionRequest) (*proto.CloseSessionResponse, error) {
	s.logger.Debug("CloseSession called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return nil, errInvalidArgument("session_id is required")
	}

	mgr := s.deps.SessionManager()
	if mgr == nil {
		return nil, errInternal("session manager not available")
	}

	sessionID := types.ID(req.SessionId)

	// Get current state before closing (for logging)
	sess, err := mgr.Get(ctx, sessionID)
	if err != nil {
		s.logger.Error("Failed to get session for closing", "session_id", sessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Close the session
	if err := mgr.CloseSession(ctx, sessionID); err != nil {
		s.logger.Error("Failed to close session", "session_id", sessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Session closed", "session_id", sessionID, "reason", req.Reason)

	// Retrieve the updated session to get the new state
	sess, err = mgr.Get(ctx, sessionID)
	if err != nil {
		s.logger.Error("Failed to retrieve session after closing", "session_id", sessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Publish event to event bus
	bus := s.deps.EventBus()
	if bus != nil {
		event := types.Event{
			Type:      "session.closed",
			Source:    "orchestrator.grpc",
			Timestamp: types.NewTimestamp(),
			Metadata: types.EventMetadata{
				SessionID: &sessionID,
				Priority:  types.PriorityNormal,
			},
			Data: map[string]interface{}{
				"session_id": string(sessionID),
				"reason":     req.Reason,
				"state":      string(sessionStateToProto(sess.State)),
			},
		}
		if err := bus.Publish(ctx, event); err != nil {
			s.logger.Warn("Failed to publish session closed event", "error", err)
		}
	}

	return &proto.CloseSessionResponse{
		SessionId: string(sessionID),
		State:     sessionStateToProto(sess.State),
	}, nil
}

// =============================================================================
// Message Handling RPCs
// =============================================================================

// SendMessage sends a message to a session
func (s *OrchestratorService) SendMessage(ctx context.Context, req *proto.SendMessageRequest) (*proto.SendMessageResponse, error) {
	s.logger.Debug("SendMessage called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return nil, errInvalidArgument("session_id is required")
	}
	if req.Message == nil {
		return nil, errInvalidArgument("message is required")
	}

	mgr := s.deps.SessionManager()
	if mgr == nil {
		return nil, errInternal("session manager not available")
	}

	sessionID := types.ID(req.SessionId)
	message := protoToMessage(req.Message)

	// Generate ID and timestamp before adding to avoid race conditions
	// (AddMessage would generate these internally, but we need them for the response)
	if message.ID.IsEmpty() {
		message.ID = types.GenerateID()
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = types.NewTimestampFromTime(time.Now())
	}

	// Add message to session
	if err := mgr.AddMessage(ctx, sessionID, message); err != nil {
		s.logger.Error("Failed to send message", "session_id", sessionID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	messageID := string(message.ID)

	s.logger.Debug("Message sent", "session_id", sessionID, "message_id", messageID)

	// Publish event to event bus
	bus := s.deps.EventBus()
	if bus != nil {
		event := types.Event{
			Type:      "message.sent",
			Source:    "orchestrator.grpc",
			Timestamp: types.NewTimestamp(),
			Metadata: types.EventMetadata{
				SessionID: &sessionID,
				Priority:  types.PriorityNormal,
			},
			Data: map[string]interface{}{
				"message_id": messageID,
				"role":       string(message.Role),
				"session_id": string(sessionID),
			},
		}
		if err := bus.Publish(ctx, event); err != nil {
			s.logger.Warn("Failed to publish message sent event", "error", err)
		}
	}

	return &proto.SendMessageResponse{
		MessageId: messageID,
		SessionId: string(sessionID),
		Timestamp: timestampToProto(&message.Timestamp),
	}, nil
}

// StreamMessages establishes a bidirectional stream for real-time message exchange
func (s *OrchestratorService) StreamMessages(stream proto.OrchestratorService_StreamMessagesServer) error {
	ctx := stream.Context()
	s.logger.Debug("StreamMessages started")

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

	var sessionID types.ID
	var firstMessage = true

	// Receive loop
	for {
		req, err := stream.Recv()
		if err != nil {
			if firstMessage {
				s.logger.Debug("StreamMessages ended without receiving any message")
				return err
			}
			// Client closed stream
			s.logger.Debug("StreamMessages receive loop ended", "session_id", sessionID, "error", err)
			return nil
		}

		// Handle heartbeat
		if req.GetHeartbeat() != nil {
			// Send heartbeat response
			if err := stream.Send(&proto.StreamMessageResponse{
				Payload: &proto.StreamMessageResponse_Heartbeat{&emptypb.Empty{}},
			}); err != nil {
				s.logger.Error("Failed to send heartbeat response", "error", err)
				return err
			}
			continue
		}

		// First message must contain session_id
		if firstMessage {
			if req.GetSessionId() == "" {
				return errInvalidArgument("first message must contain session_id")
			}
			sessionID = types.ID(req.GetSessionId())
			firstMessage = false
			s.logger.Debug("StreamMessages established for session", "session_id", sessionID)

			// Validate session exists and is active
			mgr := s.deps.SessionManager()
			if err := mgr.ValidateSession(streamCtx, sessionID); err != nil {
				s.logger.Error("Session validation failed", "session_id", sessionID, "error", err)
				return grpcErrorFromTypesError(err)
			}

			// Register this stream for the session
			s.mu.Lock()
			s.sessionStreams[sessionID] = &sessionStreamState{stream: stream}
			s.mu.Unlock()

			defer func() {
				s.mu.Lock()
				delete(s.sessionStreams, sessionID)
				s.mu.Unlock()
			}()
		}

		// Process message
		if req.GetMessage() != nil {
			message := protoToMessage(req.GetMessage())
			mgr := s.deps.SessionManager()

			// Generate ID and timestamp before adding to avoid race conditions
			// (AddMessage would generate these internally, but we need them for the response)
			if message.ID.IsEmpty() {
				message.ID = types.GenerateID()
			}
			if message.Timestamp.IsZero() {
				message.Timestamp = types.NewTimestampFromTime(time.Now())
			}

			// Add message to session
			if err := mgr.AddMessage(streamCtx, sessionID, message); err != nil {
				s.logger.Error("Failed to add message in stream", "session_id", sessionID, "error", err)
				_ = s.sendToSessionStream(sessionID, streamErrorResponse(err.Error()))
				continue
			}

			// Route user messages to the assistant agent
			if message.Role == types.MessageRoleUser {
				s.logger.Debug("Routing user message to assistant", "session_id", sessionID, "message_id", message.ID)
				if err := s.routeToAssistant(streamCtx, sessionID, message); err != nil {
					s.logger.Error("Failed to route message to assistant", "session_id", sessionID, "error", err)
					_ = s.sendToSessionStream(sessionID, streamErrorResponse(err.Error()))
				}
			}
		}
	}
}

// routeToAssistant routes a user message to the assistant agent
func (s *OrchestratorService) routeToAssistant(ctx context.Context, sessionID types.ID, message types.Message) error {
	// Get agent service from dependencies
	agentSvc := s.deps.AgentService()
	if agentSvc == nil {
		return types.NewError(types.ErrCodeUnavailable, "agent service not available")
	}

	// Find assistant agent
	registry := agentSvc.GetRegistry()
	agents := registry.List()

	var assistantID string
	for _, agent := range agents {
		if !agentSvc.HasAgentStream(agent.ID) {
			continue
		}

		isAssistantName := strings.EqualFold(agent.Name, "assistant")
		isAssistantRole := false
		if labelsAny, ok := agent.Metadata["labels"]; ok {
			switch labels := labelsAny.(type) {
			case map[string]string:
				isAssistantRole = strings.EqualFold(labels["role"], "assistant")
			case map[string]interface{}:
				if role, ok := labels["role"].(string); ok {
					isAssistantRole = strings.EqualFold(role, "assistant")
				}
			}
		}

		if isAssistantName || isAssistantRole {
			assistantID = agent.ID
			break
		}
	}

	if assistantID == "" {
		return types.NewError(types.ErrCodeNotFound, "assistant agent not found")
	}

	s.logger.Debug("Found assistant agent", "agent_id", assistantID)

	// Route message to assistant agent through AgentService
	// The assistant needs to process the message and send a response back
	if err := agentSvc.RouteMessageToAgent(ctx, assistantID, sessionID, message, s); err != nil {
		return err
	}

	return nil
}

// SendResponseToSession sends an assistant response back to the TUI stream for a session
func (s *OrchestratorService) SendResponseToSession(sessionID types.ID, message types.Message) error {
	resp := &proto.StreamMessageResponse{
		Payload: &proto.StreamMessageResponse_Message{
			Message: messageToProto(message),
		},
	}

	if err := s.sendToSessionStream(sessionID, resp); err != nil {
		s.logger.Error("Failed to send response to TUI", "session_id", sessionID, "error", err)
		return err
	}

	s.logger.Debug("Sent assistant response to TUI", "session_id", sessionID, "message_id", message.ID)
	return nil
}

// =============================================================================
// Container Operations RPCs
// =============================================================================

// CreateContainer creates a new container within a session
func (s *OrchestratorService) CreateContainer(ctx context.Context, req *proto.CreateContainerRequest) (*proto.CreateContainerResponse, error) {
	s.logger.Debug("CreateContainer called", "session_id", req.SessionId)

	if req.SessionId == "" {
		return nil, errInvalidArgument("session_id is required")
	}
	if req.Config == nil {
		return nil, errInvalidArgument("config is required")
	}

	// Container creation requires the docker client which is not yet exposed
	// For now, return unimplemented
	return nil, status.Error(codes.Unimplemented, "container operations not yet implemented")
}

// GetContainer retrieves a container by its ID
func (s *OrchestratorService) GetContainer(ctx context.Context, req *proto.GetContainerRequest) (*proto.GetContainerResponse, error) {
	return nil, status.Error(codes.Unimplemented, "container operations not yet implemented")
}

// ListContainers retrieves all containers matching the specified filter
func (s *OrchestratorService) ListContainers(ctx context.Context, req *proto.ListContainersRequest) (*proto.ListContainersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "container operations not yet implemented")
}

// StopContainer stops a running container
func (s *OrchestratorService) StopContainer(ctx context.Context, req *proto.StopContainerRequest) (*proto.StopContainerResponse, error) {
	return nil, status.Error(codes.Unimplemented, "container operations not yet implemented")
}

// RemoveContainer removes a container from the system
func (s *OrchestratorService) RemoveContainer(ctx context.Context, req *proto.RemoveContainerRequest) (*proto.RemoveContainerResponse, error) {
	return nil, status.Error(codes.Unimplemented, "container operations not yet implemented")
}

// =============================================================================
// Event Subscription RPCs
// =============================================================================

// SubscribeEvents subscribes to system events matching the specified filter
func (s *OrchestratorService) SubscribeEvents(req *proto.SubscribeEventsRequest, stream proto.OrchestratorService_SubscribeEventsServer) error {
	ctx := stream.Context()
	s.logger.Debug("SubscribeEvents called")

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
		if req.Filter.Type != proto.EventType_EVENT_TYPE_UNSPECIFIED {
			eventType := types.EventType(eventTypeToString(req.Filter.Type))
			filter.Type = &eventType
		}
		if req.Filter.Source != "" {
			filter.Source = &req.Filter.Source
		}
		if req.Filter.SessionId != "" {
			sessionID := types.ID(req.Filter.SessionId)
			filter.SessionID = &sessionID
		}
		if req.Filter.ContainerId != "" {
			containerID := types.ID(req.Filter.ContainerId)
			filter.ContainerID = &containerID
		}
	}

	// Subscribe to events
	subID, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		s.logger.Error("Failed to subscribe to events", "error", err)
		return grpcErrorFromTypesError(err)
	}
	defer bus.Unsubscribe(subID)

	s.logger.Info("Event subscription created", "subscription_id", subID)

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

			pbEvent := eventToProto(event)
			if err := stream.Send(pbEvent); err != nil {
				s.logger.Error("Failed to send event", "subscription_id", subID, "error", err)
				return err
			}
		}
	}
}

// eventStreamHandler is an EventHandler that sends events to a channel
type eventStreamHandler struct {
	eventCh chan<- types.Event
}

func (h *eventStreamHandler) Handle(ctx context.Context, event types.Event) error {
	select {
	case h.eventCh <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *eventStreamHandler) CanHandle(eventType types.EventType) bool {
	return true
}

// =============================================================================
// Health and Status RPCs
// =============================================================================

// HealthCheck returns the health status of the orchestrator
func (s *OrchestratorService) HealthCheck(ctx context.Context, req *emptypb.Empty) (*proto.HealthCheckResponse, error) {
	s.logger.Debug("HealthCheck called")

	// Check session manager
	mgr := s.deps.SessionManager()
	var health proto.Health
	if mgr != nil {
		health = proto.Health_HEALTH_HEALTHY
	} else {
		health = proto.Health_HEALTH_UNHEALTHY
	}

	// TODO: Add version from build info
	version := "dev"

	// List all subsystems
	subsystems := []string{
		"session_manager",
		"event_bus",
		"ipc_broker",
	}

	return &proto.HealthCheckResponse{
		Health:     health,
		Version:    version,
		Subsystems: subsystems,
	}, nil
}

// GetStatus returns the current status of the orchestrator
func (s *OrchestratorService) GetStatus(ctx context.Context, req *emptypb.Empty) (*proto.StatusResponse, error) {
	s.logger.Debug("GetStatus called")

	mgr := s.deps.SessionManager()
	if mgr == nil {
		return nil, errInternal("session manager not available")
	}

	// Get session stats
	active, idle, _, _ := mgr.Stats(ctx)

	// Calculate active sessions and containers
	activeSessions := active + idle
	activeContainers := 0

	// TODO: Get container count from container manager
	// For now, return 0

	// Calculate uptime (assuming service started at server creation time)
	startedAt := time.Now() // TODO: Get actual start time from server
	uptime := time.Since(startedAt)

	startedTs := types.NewTimestampFromTime(startedAt)
	// Represent uptime as a duration anchored at the Unix epoch (seconds/nanos encode the duration)
	uptimeTs := types.NewTimestampFromTime(time.Unix(0, 0).Add(uptime))

	return &proto.StatusResponse{
		Status:           proto.Status_STATUS_RUNNING,
		ActiveSessions:   int32(activeSessions),
		ActiveContainers: int32(activeContainers),
		StartedAt:        timestampToProto(&startedTs),
		Uptime:           timestampToProto(&uptimeTs),
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// eventTypeToString converts protobuf EventType to string used by event system
func eventTypeToString(et proto.EventType) string {
	switch et {
	case proto.EventType_EVENT_TYPE_CONTAINER_CREATED:
		return "container.created"
	case proto.EventType_EVENT_TYPE_CONTAINER_STARTED:
		return "container.started"
	case proto.EventType_EVENT_TYPE_CONTAINER_STOPPED:
		return "container.stopped"
	case proto.EventType_EVENT_TYPE_CONTAINER_REMOVED:
		return "container.removed"
	case proto.EventType_EVENT_TYPE_CONTAINER_ERROR:
		return "container.error"
	case proto.EventType_EVENT_TYPE_SESSION_CREATED:
		return "session.created"
	case proto.EventType_EVENT_TYPE_SESSION_UPDATED:
		return "session.updated"
	case proto.EventType_EVENT_TYPE_SESSION_CLOSED:
		return "session.closed"
	case proto.EventType_EVENT_TYPE_TASK_CREATED:
		return "task.created"
	case proto.EventType_EVENT_TYPE_TASK_UPDATED:
		return "task.updated"
	case proto.EventType_EVENT_TYPE_TASK_COMPLETED:
		return "task.completed"
	case proto.EventType_EVENT_TYPE_TASK_FAILED:
		return "task.failed"
	case proto.EventType_EVENT_TYPE_IPC_MESSAGE:
		return "ipc.message"
	case proto.EventType_EVENT_TYPE_POLICY_VIOLATION:
		return "policy.violation"
	case proto.EventType_EVENT_TYPE_RESOURCE_THRESHOLD:
		return "resource.threshold"
	case proto.EventType_EVENT_TYPE_SYSTEM_STARTUP:
		return "system.startup"
	case proto.EventType_EVENT_TYPE_SYSTEM_SHUTDOWN:
		return "system.shutdown"
	default:
		return "unknown"
	}
}

// Close closes all active streams
func (s *OrchestratorService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Closing orchestrator service streams", "count", len(s.streams))

	for stream, cancel := range s.streams {
		cancel()
		delete(s.streams, stream)
	}

	return nil
}

// String returns a string representation of the service
func (s *OrchestratorService) String() string {
	return fmt.Sprintf("OrchestratorService{active_streams: %d}", len(s.streams))
}
