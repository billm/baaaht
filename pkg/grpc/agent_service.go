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
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/skills"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// AgentServiceDependencies represents the agent service dependencies
type AgentServiceDependencies interface {
	EventBus() *events.Bus
	SessionManager() *session.Manager
}

// AgentInfo holds information about a registered agent
type AgentInfo struct {
	ID            string
	Name          string
	Type          string
	State         string
	Metadata      map[string]interface{}
	Capabilities  *proto.AgentCapabilities
	RegisteredAt  time.Time
	LastHeartbeat time.Time
	ActiveTasks   map[string]*TaskInfo
	mu            sync.RWMutex
}

// TaskInfo holds information about a task
type TaskInfo struct {
	ID          string
	Name        string
	SessionID   string
	Type        string
	State       string
	Priority    string
	Config      *proto.TaskConfig
	Result      *proto.TaskResult
	Error       *proto.TaskError
	Progress    float32
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	mu          sync.RWMutex
}

// AgentRegistry manages agent registration and task tracking
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*AgentInfo
	tasks  map[string]*TaskInfo
	logger *logger.Logger
}

// NewAgentRegistry creates a new agent registry
func NewAgentRegistry(log *logger.Logger) *AgentRegistry {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	return &AgentRegistry{
		agents: make(map[string]*AgentInfo),
		tasks:  make(map[string]*TaskInfo),
		logger: log.With("component", "agent_registry"),
	}
}

// Register registers a new agent
func (r *AgentRegistry) Register(agentID string, info *AgentInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[agentID]; exists {
		return types.NewError(types.ErrCodeAlreadyExists, "agent already registered")
	}

	info.ID = agentID
	info.RegisteredAt = time.Now()
	info.LastHeartbeat = time.Now()
	info.State = "idle"
	info.ActiveTasks = make(map[string]*TaskInfo)

	r.agents[agentID] = info

	r.logger.Info("Agent registered", "agent_id", agentID, "name", info.Name, "type", info.Type)

	return nil
}

// Unregister removes an agent
func (r *AgentRegistry) Unregister(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "agent not found")
	}

	agent.mu.Lock()
	agent.State = "terminated"
	agent.mu.Unlock()

	delete(r.agents, agentID)

	r.logger.Info("Agent unregistered", "agent_id", agentID)

	return nil
}

// Get retrieves an agent by ID
func (r *AgentRegistry) Get(agentID string) (*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, "agent not found")
	}

	return agent, nil
}

// List returns all registered agents
func (r *AgentRegistry) List() []*AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}

	return agents
}

// UpdateHeartbeat updates the last heartbeat time for an agent
func (r *AgentRegistry) UpdateHeartbeat(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "agent not found")
	}

	agent.mu.Lock()
	agent.LastHeartbeat = time.Now()
	agent.mu.Unlock()

	return nil
}

// AddTask adds a task to an agent
func (r *AgentRegistry) AddTask(agentID string, task *TaskInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "agent not found")
	}

	task.ID = string(types.GenerateID())
	task.CreatedAt = time.Now()
	task.State = "pending"

	r.tasks[task.ID] = task

	agent.mu.Lock()
	agent.ActiveTasks[task.ID] = task
	agent.State = "busy"
	agent.mu.Unlock()

	r.logger.Info("Task added", "task_id", task.ID, "agent_id", agentID, "type", task.Type)

	return nil
}

// GetTask retrieves a task by ID
func (r *AgentRegistry) GetTask(taskID string) (*TaskInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, exists := r.tasks[taskID]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, "task not found")
	}

	return task, nil
}

// ListTasks returns all tasks for an agent
func (r *AgentRegistry) ListTasks(agentID string) []*TaskInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return nil
	}

	tasks := make([]*TaskInfo, 0, len(agent.ActiveTasks))
	agent.mu.RLock()
	for _, task := range agent.ActiveTasks {
		tasks = append(tasks, task)
	}
	agent.mu.RUnlock()

	return tasks
}

// AgentService implements the gRPC AgentService interface
type AgentService struct {
	proto.UnimplementedAgentServiceServer
	deps         AgentServiceDependencies
	registry     *AgentRegistry
	skillsLoader *skills.Loader
	logger       *logger.Logger
	mu           sync.RWMutex

	// Track active streams for graceful shutdown
	streams map[interface{}]context.CancelFunc

	// Track agent streams by agent ID for message routing
	agentStreams   map[string]proto.AgentService_StreamAgentServer
	agentSendLocks map[string]*sync.Mutex

	// Track pending routed responses keyed by agentID:correlationID
	pendingResponses map[string]pendingResponseRoute
}

type pendingResponseRoute struct {
	sessionID types.ID
	handler   MessageResponseHandler
}

// MessageResponseHandler handles responses from agents
type MessageResponseHandler interface {
	SendResponseToSession(sessionID types.ID, message types.Message) error
}

// NewAgentService creates a new agent service
func NewAgentService(deps AgentServiceDependencies, log *logger.Logger, skillsLoader *skills.Loader) *AgentService {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	service := &AgentService{
		deps:             deps,
		registry:         NewAgentRegistry(log),
		skillsLoader:     skillsLoader,
		logger:           log.With("component", "agent_service"),
		streams:          make(map[interface{}]context.CancelFunc),
		agentStreams:     make(map[string]proto.AgentService_StreamAgentServer),
		agentSendLocks:   make(map[string]*sync.Mutex),
		pendingResponses: make(map[string]pendingResponseRoute),
	}

	if skillsLoader != nil {
		service.logger.Info("Agent service initialized with skills loader")
	}

	return service
}

func pendingResponseKey(agentID, correlationID string) string {
	return fmt.Sprintf("%s:%s", agentID, correlationID)
}

func (s *AgentService) sendToAgentStream(agentID string, resp *proto.StreamAgentResponse) error {
	s.mu.RLock()
	stream, exists := s.agentStreams[agentID]
	sendLock := s.agentSendLocks[agentID]
	s.mu.RUnlock()

	if !exists || stream == nil || sendLock == nil {
		return types.NewError(types.ErrCodeUnavailable, "agent stream not available")
	}

	sendLock.Lock()
	defer sendLock.Unlock()

	if err := stream.Send(resp); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to send message to agent", err)
	}

	return nil
}

func (s *AgentService) HasAgentStream(agentID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream, exists := s.agentStreams[agentID]
	return exists && stream != nil
}

// GetRegistry returns the agent registry
func (s *AgentService) GetRegistry() *AgentRegistry {
	return s.registry
}

// GetSkillsLoader returns the skills loader (may be nil)
func (s *AgentService) GetSkillsLoader() *skills.Loader {
	return s.skillsLoader
}

// =============================================================================
// Skills Management
// =============================================================================

// ActivateSkillsForAgent activates skills for a specific agent
func (s *AgentService) ActivateSkillsForAgent(ctx context.Context, agentID string, agentType string) error {
	if s.skillsLoader == nil {
		s.logger.Debug("Skills loader not available, skipping skill activation", "agent_id", agentID)
		return nil
	}

	s.logger.Info("Activating skills for agent", "agent_id", agentID, "agent_type", agentType)

	// Activate skills for the agent type
	// The scope is typically "user" for individual agents
	skillsScope := types.SkillScopeUser
	count, err := s.skillsLoader.ActivateByOwner(ctx, skillsScope, agentID)
	if err != nil {
		s.logger.Error("Failed to activate skills for agent", "agent_id", agentID, "error", err)
		return err
	}

	// Get active skills for the agent
	activeSkills, err := s.skillsLoader.GetActiveSkills(ctx, skillsScope, agentID)
	if err != nil {
		s.logger.Error("Failed to get active skills for agent", "agent_id", agentID, "error", err)
		return err
	}
	s.logger.Info("Skills activated for agent", "agent_id", agentID, "agent_type", agentType, "activated_count", count, "skill_count", len(activeSkills))

	return nil
}

// GetActiveSkillsForAgent returns the active skills for a specific agent
func (s *AgentService) GetActiveSkillsForAgent(ctx context.Context, agentID string) ([]*types.Skill, error) {
	if s.skillsLoader == nil {
		return []*types.Skill{}, nil
	}

	scope := types.SkillScopeUser
	activeSkills, err := s.skillsLoader.GetActiveSkills(ctx, scope, agentID)
	if err != nil {
		return nil, err
	}
	return activeSkills, nil
}

// =============================================================================
// Agent Registration RPCs
// =============================================================================

// Register registers an agent with the orchestrator
func (s *AgentService) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	s.logger.Debug("Register called", "name", req.Name, "type", req.Type.String())

	if req.Name == "" {
		return nil, errInvalidArgument("name is required")
	}

	// Generate agent ID
	agentID := string(types.GenerateID())

	// Create agent info
	info := &AgentInfo{
		Name:         req.Name,
		Type:         protoToAgentType(req.Type),
		Metadata:     protoToAgentMetadata(req.Metadata),
		Capabilities: req.Capabilities,
	}

	// Register agent
	if err := s.registry.Register(agentID, info); err != nil {
		s.logger.Error("Failed to register agent", "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Get registered agent
	agent, err := s.registry.Get(agentID)
	if err != nil {
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Agent registered", "agent_id", agentID, "name", req.Name)

	// Activate skills for this agent if skills loader is available
	if s.skillsLoader != nil {
		agentType := protoToAgentType(req.Type)
		if err := s.ActivateSkillsForAgent(ctx, agentID, agentType); err != nil {
			s.logger.Warn("Failed to activate skills for agent", "agent_id", agentID, "error", err)
			// Don't fail registration if skill activation fails
		}
	}

	// Publish event to event bus
	if s.deps != nil {
		bus := s.deps.EventBus()
		if bus != nil {
			event := types.Event{
				Type:      "agent.registered",
				Source:    "agent.grpc",
				Timestamp: types.NewTimestamp(),
				Metadata: types.EventMetadata{
					Priority: types.PriorityNormal,
				},
				Data: map[string]interface{}{
					"agent_id": agentID,
					"name":     req.Name,
					"type":     req.Type.String(),
				},
			}
			if err := bus.Publish(ctx, event); err != nil {
				s.logger.Warn("Failed to publish agent registered event", "error", err)
			}
		}
	}

	return &proto.RegisterResponse{
		AgentId:           agentID,
		Agent:             s.agentInfoToProto(agent),
		RegistrationToken: agentID, // Use agent ID as token for simplicity
	}, nil
}

// Unregister unregisters an agent from the orchestrator
func (s *AgentService) Unregister(ctx context.Context, req *proto.UnregisterRequest) (*proto.UnregisterResponse, error) {
	s.logger.Debug("Unregister called", "agent_id", req.AgentId)

	if req.AgentId == "" {
		return nil, errInvalidArgument("agent_id is required")
	}

	if err := s.registry.Unregister(req.AgentId); err != nil {
		s.logger.Error("Failed to unregister agent", "agent_id", req.AgentId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	message := "Agent unregistered"
	if req.Reason != "" {
		message = fmt.Sprintf("Agent unregistered: %s", req.Reason)
	}

	s.logger.Info("Agent unregistered", "agent_id", req.AgentId, "reason", req.Reason)

	// Publish event to event bus
	if s.deps != nil {
		bus := s.deps.EventBus()
		if bus != nil {
			event := types.Event{
				Type:      "agent.unregistered",
				Source:    "agent.grpc",
				Timestamp: types.NewTimestamp(),
				Metadata: types.EventMetadata{
					Priority: types.PriorityNormal,
				},
				Data: map[string]interface{}{
					"agent_id": req.AgentId,
					"reason":   req.Reason,
				},
			}
			if err := bus.Publish(ctx, event); err != nil {
				s.logger.Warn("Failed to publish agent unregistered event", "error", err)
			}
		}
	}

	return &proto.UnregisterResponse{
		Success: true,
		Message: message,
	}, nil
}

// Heartbeat sends a heartbeat to keep the agent connection alive
func (s *AgentService) Heartbeat(ctx context.Context, req *proto.HeartbeatRequest) (*proto.HeartbeatResponse, error) {
	s.logger.Debug("Heartbeat called", "agent_id", req.AgentId)

	if req.AgentId == "" {
		return nil, errInvalidArgument("agent_id is required")
	}

	if err := s.registry.UpdateHeartbeat(req.AgentId); err != nil {
		s.logger.Error("Failed to update heartbeat", "agent_id", req.AgentId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// TODO: Return pending tasks for the agent
	var pendingTasks []string

	return &proto.HeartbeatResponse{
		Timestamp:    timestamppb.Now(),
		PendingTasks: pendingTasks,
	}, nil
}

// =============================================================================
// Task Execution RPCs
// =============================================================================

// ExecuteTask executes a task on the agent
func (s *AgentService) ExecuteTask(ctx context.Context, req *proto.ExecuteTaskRequest) (*proto.ExecuteTaskResponse, error) {
	s.logger.Debug("ExecuteTask called", "agent_id", req.AgentId, "session_id", req.SessionId)

	if req.AgentId == "" {
		return nil, errInvalidArgument("agent_id is required")
	}
	if req.SessionId == "" {
		return nil, errInvalidArgument("session_id is required")
	}
	if req.Config == nil {
		return nil, errInvalidArgument("config is required")
	}

	// Create task info
	task := &TaskInfo{
		Name:      req.Config.Command,
		SessionID: req.SessionId,
		Type:      protoToTaskType(req.Type),
		Priority:  protoToTaskPriority(req.Priority),
		Config:    req.Config,
		Progress:  0.0,
	}

	// Add task to registry
	if err := s.registry.AddTask(req.AgentId, task); err != nil {
		s.logger.Error("Failed to create task", "agent_id", req.AgentId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Task created", "task_id", task.ID, "agent_id", req.AgentId)

	// Publish event to event bus
	if s.deps != nil {
		bus := s.deps.EventBus()
		if bus != nil {
			event := types.Event{
				Type:      "task.created",
				Source:    "agent.grpc",
				Timestamp: types.NewTimestamp(),
				Metadata: types.EventMetadata{
					Priority: types.PriorityNormal,
				},
				Data: map[string]interface{}{
					"task_id":    task.ID,
					"agent_id":   req.AgentId,
					"session_id": req.SessionId,
					"type":       req.Type.String(),
				},
			}
			if err := bus.Publish(ctx, event); err != nil {
				s.logger.Warn("Failed to publish task created event", "error", err)
			}
		}
	}

	return &proto.ExecuteTaskResponse{
		TaskId: task.ID,
		Task:   s.taskInfoToProto(task),
	}, nil
}

// StreamTask establishes a bidirectional stream for task execution with real-time updates
func (s *AgentService) StreamTask(stream proto.AgentService_StreamTaskServer) error {
	ctx := stream.Context()
	s.logger.Debug("StreamTask started")

	// Track this stream for cleanup
	_, cancel := context.WithCancel(ctx)
	defer cancel()

	s.mu.Lock()
	s.streams[stream] = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.streams, stream)
		s.mu.Unlock()
	}()

	var taskID string
	var firstMessage = true

	// Receive loop
	for {
		req, err := stream.Recv()
		if err != nil {
			if firstMessage {
				s.logger.Debug("StreamTask ended without receiving any message")
				return err
			}
			s.logger.Debug("StreamTask receive loop ended", "task_id", taskID, "error", err)
			return nil
		}

		// Handle heartbeat
		if req.GetHeartbeat() != nil {
			if err := stream.Send(&proto.StreamTaskResponse{
				Payload: &proto.StreamTaskResponse_Heartbeat{&emptypb.Empty{}},
			}); err != nil {
				s.logger.Error("Failed to send heartbeat response", "error", err)
				return err
			}
			continue
		}

		// First message must contain task_id
		if firstMessage {
			if req.TaskId == "" {
				return errInvalidArgument("first message must contain task_id")
			}
			taskID = req.TaskId
			firstMessage = false
			s.logger.Debug("StreamTask established for task", "task_id", taskID)

			// Validate task exists
			_, err := s.registry.GetTask(taskID)
			if err != nil {
				s.logger.Error("Task not found", "task_id", taskID, "error", err)
				return grpcErrorFromTypesError(err)
			}
		}

		// Handle task input
		if req.GetInput() != nil {
			input := req.GetInput()

			// Send output response (echo for now)
			resp := &proto.StreamTaskResponse{
				Payload: &proto.StreamTaskResponse_Output{
					Output: &proto.TaskOutput{
						Data:       input.Data,
						Text:       string(input.Data),
						StreamType: "stdout",
					},
				},
			}
			if err := stream.Send(resp); err != nil {
				s.logger.Error("Failed to send output", "task_id", taskID, "error", err)
				return err
			}
		}

		// Handle cancel command
		if req.GetCancel() != nil {
			// Update task state to cancelled
			task, err := s.registry.GetTask(taskID)
			if err == nil {
				task.mu.Lock()
				task.State = "cancelled"
				now := time.Now()
				task.CompletedAt = &now
				task.mu.Unlock()

				// Send complete response
				resp := &proto.StreamTaskResponse{
					Payload: &proto.StreamTaskResponse_Complete{
						Complete: &proto.TaskComplete{
							TaskId:      taskID,
							CompletedAt: timestamppb.Now(),
						},
					},
				}
				if err := stream.Send(resp); err != nil {
					s.logger.Error("Failed to send cancel response", "task_id", taskID, "error", err)
					return err
				}
			}
			s.logger.Info("Task cancelled", "task_id", taskID, "reason", req.GetCancel().Reason)
			return nil
		}
	}
}

// CancelTask cancels a running task
func (s *AgentService) CancelTask(ctx context.Context, req *proto.CancelTaskRequest) (*proto.CancelTaskResponse, error) {
	s.logger.Debug("CancelTask called", "task_id", req.TaskId)

	if req.TaskId == "" {
		return nil, errInvalidArgument("task_id is required")
	}

	task, err := s.registry.GetTask(req.TaskId)
	if err != nil {
		s.logger.Error("Failed to get task for cancellation", "task_id", req.TaskId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	task.mu.Lock()
	task.State = "cancelled"
	now := time.Now()
	task.CompletedAt = &now
	task.mu.Unlock()

	s.logger.Info("Task cancelled", "task_id", req.TaskId, "reason", req.Reason)

	return &proto.CancelTaskResponse{
		TaskId:    req.TaskId,
		State:     taskStateToProto(task.State),
		Cancelled: true,
	}, nil
}

// ListTasks lists all tasks on the agent
func (s *AgentService) ListTasks(ctx context.Context, req *proto.ListTasksRequest) (*proto.ListTasksResponse, error) {
	s.logger.Debug("ListTasks called", "agent_id", req.AgentId)

	if req.AgentId == "" {
		return nil, errInvalidArgument("agent_id is required")
	}

	tasks := s.registry.ListTasks(req.AgentId)

	pbTasks := make([]*proto.Task, len(tasks))
	for i, task := range tasks {
		pbTasks[i] = s.taskInfoToProto(task)
	}

	return &proto.ListTasksResponse{
		Tasks:      pbTasks,
		TotalCount: int32(len(tasks)),
	}, nil
}

// GetTaskStatus retrieves the status of a task
func (s *AgentService) GetTaskStatus(ctx context.Context, req *proto.GetTaskStatusRequest) (*proto.GetTaskStatusResponse, error) {
	s.logger.Debug("GetTaskStatus called", "task_id", req.TaskId)

	if req.TaskId == "" {
		return nil, errInvalidArgument("task_id is required")
	}

	task, err := s.registry.GetTask(req.TaskId)
	if err != nil {
		s.logger.Error("Failed to get task status", "task_id", req.TaskId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	return &proto.GetTaskStatusResponse{
		Task: s.taskInfoToProto(task),
	}, nil
}

// =============================================================================
// Communication RPCs
// =============================================================================

// StreamAgent establishes a bidirectional stream for agent communication
func (s *AgentService) StreamAgent(stream proto.AgentService_StreamAgentServer) error {
	ctx := stream.Context()
	s.logger.Debug("StreamAgent started")

	// Track this stream for cleanup
	_, cancel := context.WithCancel(ctx)
	defer cancel()

	s.mu.Lock()
	s.streams[stream] = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.streams, stream)
		s.mu.Unlock()
	}()

	var agentID string
	var firstMessage = true

	// Receive loop
	for {
		req, err := stream.Recv()
		if err != nil {
			if firstMessage {
				s.logger.Debug("StreamAgent ended without receiving any message")
				return err
			}
			s.logger.Debug("StreamAgent receive loop ended", "agent_id", agentID, "error", err)
			return nil
		}

		// Handle heartbeat
		if req.GetHeartbeat() != nil {
			if !firstMessage {
				if err := s.sendToAgentStream(agentID, &proto.StreamAgentResponse{
					Payload: &proto.StreamAgentResponse_Heartbeat{&emptypb.Empty{}},
				}); err != nil {
					s.logger.Error("Failed to send heartbeat response", "error", err)
					return err
				}
				continue
			}

			if err := stream.Send(&proto.StreamAgentResponse{
				Payload: &proto.StreamAgentResponse_Heartbeat{&emptypb.Empty{}},
			}); err != nil {
				s.logger.Error("Failed to send heartbeat response", "error", err)
				return err
			}
			continue
		}

		// First message must contain agent_id
		if firstMessage {
			if req.AgentId == "" {
				return errInvalidArgument("first message must contain agent_id")
			}
			agentID = req.AgentId
			firstMessage = false
			s.logger.Debug("StreamAgent established for agent", "agent_id", agentID)

			// Validate agent exists
			_, err := s.registry.Get(agentID)
			if err != nil {
				s.logger.Error("Agent not found", "agent_id", agentID, "error", err)
				return grpcErrorFromTypesError(err)
			}

			// Register this stream for the agent
			s.mu.Lock()
			s.agentStreams[agentID] = stream
			s.agentSendLocks[agentID] = &sync.Mutex{}
			s.mu.Unlock()

			defer func() {
				s.mu.Lock()
				delete(s.agentStreams, agentID)
				delete(s.agentSendLocks, agentID)
				for key := range s.pendingResponses {
					if len(key) >= len(agentID)+1 && key[:len(agentID)+1] == agentID+":" {
						delete(s.pendingResponses, key)
					}
				}
				s.mu.Unlock()
			}()
		}

		// Handle message from agent (e.g., assistant response)
		if req.GetMessage() != nil {
			msg := req.GetMessage()

			s.logger.Debug("Received agent message",
				"agent_id", agentID,
				"message_id", msg.Id,
				"type", msg.Type.String())

			if msg.Metadata != nil && msg.Metadata.CorrelationId != "" {
				pendingKey := pendingResponseKey(agentID, msg.Metadata.CorrelationId)

				s.mu.Lock()
				pendingRoute, exists := s.pendingResponses[pendingKey]
				if exists {
					delete(s.pendingResponses, pendingKey)
				}
				s.mu.Unlock()

				if !exists {
					s.logger.Warn("Discarding unsolicited agent response",
						"agent_id", agentID,
						"correlation_id", msg.Metadata.CorrelationId)
				} else {
					sessionID := types.ID(msg.Metadata.SessionId)
					if sessionID != pendingRoute.sessionID {
						s.logger.Warn("Discarding agent response with mismatched session",
							"agent_id", agentID,
							"expected_session", pendingRoute.sessionID,
							"actual_session", sessionID,
							"correlation_id", msg.Metadata.CorrelationId)
					} else if dataMsg := msg.GetDataMessage(); dataMsg != nil {
						content := string(dataMsg.Data)

						responseMsg := types.Message{
							ID:        types.GenerateID(),
							Timestamp: types.NewTimestampFromTime(time.Now()),
							Role:      types.MessageRoleAssistant,
							Content:   content,
						}

						if mgr := s.deps.SessionManager(); mgr != nil {
							if err := mgr.AddMessage(ctx, sessionID, responseMsg); err != nil {
								s.logger.Error("Failed to add assistant response to session", "session_id", sessionID, "error", err)
							}
						}

						if pendingRoute.handler != nil {
							if err := pendingRoute.handler.SendResponseToSession(sessionID, responseMsg); err != nil {
								s.logger.Error("Failed to send response to TUI", "session_id", sessionID, "error", err)
							}
						}
					}
				}
			}

			// Send acknowledgment
			resp := &proto.StreamAgentResponse{
				Payload: &proto.StreamAgentResponse_Message{
					Message: msg,
				},
			}
			if err := s.sendToAgentStream(agentID, resp); err != nil {
				s.logger.Error("Failed to send message response", "agent_id", agentID, "error", err)
				return err
			}
		}
	}
}

// SendMessage sends a message to the agent
func (s *AgentService) SendMessage(ctx context.Context, req *proto.AgentSendMessageRequest) (*proto.AgentSendMessageResponse, error) {
	s.logger.Debug("SendMessage called", "agent_id", req.AgentId)

	if req.AgentId == "" {
		return nil, errInvalidArgument("agent_id is required")
	}
	if req.Message == nil {
		return nil, errInvalidArgument("message is required")
	}

	// Validate agent exists
	_, err := s.registry.Get(req.AgentId)
	if err != nil {
		s.logger.Error("Agent not found", "agent_id", req.AgentId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Generate message ID if not set
	messageID := req.Message.Id
	if messageID == "" {
		messageID = string(types.GenerateID())
	}

	s.logger.Debug("Message sent", "message_id", messageID, "agent_id", req.AgentId)

	return &proto.AgentSendMessageResponse{
		MessageId: messageID,
		Timestamp: timestamppb.Now(),
	}, nil
}

// RouteMessageToAgent routes a user message to a specific agent
func (s *AgentService) RouteMessageToAgent(ctx context.Context, agentID string, sessionID types.ID, message types.Message, handler MessageResponseHandler) error {
	s.logger.Debug("RouteMessageToAgent called", "agent_id", agentID, "session_id", sessionID, "message_id", message.ID)

	if message.ID.IsEmpty() {
		message.ID = types.GenerateID()
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = types.NewTimestampFromTime(time.Now())
	}

	correlationID := string(message.ID)

	// Store the pending response route for this agent+correlation combination
	pendingKey := pendingResponseKey(agentID, correlationID)
	s.mu.Lock()
	s.pendingResponses[pendingKey] = pendingResponseRoute{
		sessionID: sessionID,
		handler:   handler,
	}
	s.mu.Unlock()

	// Create AgentMessage with DataMessage payload containing the user message
	agentMsg := &proto.AgentMessage{
		Id:        string(message.ID),
		Type:      proto.MessageType_MESSAGE_TYPE_DATA,
		Timestamp: timestamppb.New(message.Timestamp.Time),
		SourceId:  "orchestrator",
		TargetId:  agentID,
		Payload: &proto.AgentMessage_DataMessage{
			DataMessage: &proto.DataMessage{
				ContentType: "text/plain",
				Data:        []byte(message.Content),
			},
		},
		Metadata: &proto.AgentMessageMetadata{
			SessionId:     string(sessionID),
			CorrelationId: correlationID,
		},
	}

	// Send message to agent via stream
	resp := &proto.StreamAgentResponse{
		Payload: &proto.StreamAgentResponse_Message{
			Message: agentMsg,
		},
	}

	if err := s.sendToAgentStream(agentID, resp); err != nil {
		s.mu.Lock()
		delete(s.pendingResponses, pendingKey)
		s.mu.Unlock()
		s.logger.Error("Failed to send message to agent", "agent_id", agentID, "error", err)
		return err
	}

	s.logger.Debug("Sent message to agent", "agent_id", agentID, "session_id", sessionID)
	return nil
}

// =============================================================================
// Health and Status RPCs
// =============================================================================

// HealthCheck returns the health status of the agent service
func (s *AgentService) HealthCheck(ctx context.Context, req *emptypb.Empty) (*proto.AgentHealthCheckResponse, error) {
	s.logger.Debug("HealthCheck called")

	// Check if registry is initialized
	if s.registry == nil {
		return &proto.AgentHealthCheckResponse{
			Status: statusToProto(types.StatusError),
		}, nil
	}

	// TODO: Get version from build info
	version := "dev"

	// List all subsystems
	subsystems := []string{
		"agent_registry",
		"task_manager",
	}

	return &proto.AgentHealthCheckResponse{
		Status:     statusToProto(types.StatusRunning),
		Version:    version,
		Subsystems: subsystems,
	}, nil
}

// GetStatus returns the current status of the agent service
func (s *AgentService) GetStatus(ctx context.Context, req *emptypb.Empty) (*proto.AgentStatusResponse, error) {
	s.logger.Debug("GetStatus called")

	agents := s.registry.List()

	// Count active tasks
	activeTasks := 0
	for _, agent := range agents {
		agent.mu.RLock()
		activeTasks += len(agent.ActiveTasks)
		agent.mu.RUnlock()
	}

	// For uptime, assume service started at creation time
	startedAt := time.Now()
	uptime := time.Since(startedAt)

	return &proto.AgentStatusResponse{
		Status:      statusToProto(types.StatusRunning),
		State:       proto.AgentState_AGENT_STATE_IDLE,
		ActiveTasks: int32(activeTasks),
		StartedAt:   timestamppb.New(startedAt),
		Uptime:      timestamppb.New(startedAt.Add(uptime)),
	}, nil
}

// GetCapabilities returns the capabilities of the agent service
func (s *AgentService) GetCapabilities(ctx context.Context, req *emptypb.Empty) (*proto.CapabilitiesResponse, error) {
	s.logger.Debug("GetCapabilities called")

	capabilities := &proto.AgentCapabilities{
		SupportedTasks:     []string{"code_execution", "file_operation", "tool_execution"},
		SupportedTools:     []string{"bash", "python", "file_reader"},
		MaxConcurrentTasks: 10,
		ResourceLimits: &proto.ResourceLimits{
			NanoCpus:    1000000000,         // 1 CPU
			MemoryBytes: 1024 * 1024 * 1024, // 1GB
		},
		SupportsStreaming:    true,
		SupportsCancellation: true,
		SupportedProtocols:   []string{"grpc", "uds"},
	}

	return &proto.CapabilitiesResponse{
		Capabilities: capabilities,
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// agentInfoToProto converts AgentInfo to protobuf Agent
func (s *AgentService) agentInfoToProto(info *AgentInfo) *proto.Agent {
	info.mu.RLock()
	defer info.mu.RUnlock()

	pb := &proto.Agent{
		Id:            info.ID,
		Name:          info.Name,
		Type:          agentTypeToProto(info.Type),
		State:         agentStateToProto(info.State),
		Status:        statusToProto(types.StatusRunning),
		RegisteredAt:  timestamppb.New(info.RegisteredAt),
		LastHeartbeat: timestamppb.New(info.LastHeartbeat),
		Metadata:      agentMetadataToProto(info.Metadata),
		Capabilities:  info.Capabilities,
		ActiveTasks:   make([]string, 0, len(info.ActiveTasks)),
	}

	for taskID := range info.ActiveTasks {
		pb.ActiveTasks = append(pb.ActiveTasks, taskID)
	}

	return pb
}

// taskInfoToProto converts TaskInfo to protobuf Task
func (s *AgentService) taskInfoToProto(task *TaskInfo) *proto.Task {
	task.mu.RLock()
	defer task.mu.RUnlock()

	pb := &proto.Task{
		Id:        task.ID,
		Name:      task.Name,
		SessionId: task.SessionID,
		Type:      taskTypeToProto(task.Type),
		State:     taskStateToProto(task.State),
		Priority:  taskPriorityToProto(task.Priority),
		CreatedAt: timestamppb.New(task.CreatedAt),
		Config:    task.Config,
		Result:    task.Result,
		Error:     task.Error,
		Progress:  float64(task.Progress),
	}

	if task.StartedAt != nil {
		pb.StartedAt = timestamppb.New(*task.StartedAt)
	}

	if task.CompletedAt != nil {
		pb.CompletedAt = timestamppb.New(*task.CompletedAt)
	}

	return pb
}

// Close closes all active streams
func (s *AgentService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Closing agent service streams", "count", len(s.streams))

	for stream, cancel := range s.streams {
		cancel()
		delete(s.streams, stream)
	}

	return nil
}

// String returns a string representation of the service
func (s *AgentService) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return fmt.Sprintf("AgentService{active_streams: %d, registered_agents: %d}",
		len(s.streams), len(s.registry.List()))
}
