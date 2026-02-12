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
	"github.com/billm/baaaht/orchestrator/pkg/tools"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// ToolServiceDependencies represents the tool service dependencies
type ToolServiceDependencies interface {
	EventBus() *events.Bus
	Executor() *tools.Executor
}

// ToolInfo holds information about a registered tool
type ToolInfo struct {
	Name         string
	DisplayName  string
	Type         proto.ToolType
	State        proto.ToolState
	Status       proto.Status
	Definition   *proto.ToolDefinition
	UsageStats   *ToolUsageStatsInfo
	RegisteredAt time.Time
	UpdatedAt    time.Time
	LastUsedAt   *time.Time
	Enabled      bool
	Version      string
	mu           sync.RWMutex
}

// ToolUsageStatsInfo holds usage statistics for a tool
type ToolUsageStatsInfo struct {
	TotalExecutions     int64
	SuccessfulExecutions int64
	FailedExecutions    int64
	TimeoutExecutions   int64
	CancelledExecutions int64
	TotalDurationNs     int64
	AvgDurationNs       int64
	LastExecution       *time.Time
	mu                  sync.RWMutex
}

// ExecutionInfo holds information about a tool execution
type ExecutionInfo struct {
	ExecutionID   string
	ToolName      string
	SessionID     string
	Status        proto.ToolExecutionStatus
	Parameters    map[string]string
	CreatedAt     time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
	ExpiresAt     *time.Time
	Result        *proto.ToolExecutionResult
	ContainerID   string
	DurationNs    int64
	ErrorMessage  string
	CorrelationID string
	mu            sync.RWMutex
}

// ToolRegistry manages tool registration and tracking
type ToolRegistry struct {
	mu     sync.RWMutex
	tools  map[string]*ToolInfo
	logger *logger.Logger
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry(log *logger.Logger) *ToolRegistry {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	return &ToolRegistry{
		tools:  make(map[string]*ToolInfo),
		logger: log.With("component", "tool_registry"),
	}
}

// Register registers a new tool
func (r *ToolRegistry) Register(name string, info *ToolInfo, force bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists && !force {
		return types.NewError(types.ErrCodeAlreadyExists, "tool already registered")
	}

	info.Name = name
	now := time.Now()
	info.RegisteredAt = now
	info.UpdatedAt = now
	info.State = proto.ToolState_TOOL_STATE_REGISTERED
	info.Status = proto.Status_STATUS_RUNNING

	r.tools[name] = info

	r.logger.Info("Tool registered", "tool_name", name, "display_name", info.DisplayName, "type", info.Type.String())

	return nil
}

// Unregister removes a tool
func (r *ToolRegistry) Unregister(name string, force bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tool, exists := r.tools[name]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "tool not found")
	}

	tool.mu.Lock()
	tool.State = proto.ToolState_TOOL_STATE_DISABLED
	tool.mu.Unlock()

	delete(r.tools, name)

	r.logger.Info("Tool unregistered", "tool_name", name, "force", force)

	return nil
}

// Get retrieves a tool by name
func (r *ToolRegistry) Get(name string) (*ToolInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, "tool not found")
	}

	return tool, nil
}

// List returns all registered tools
func (r *ToolRegistry) List() []*ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]*ToolInfo, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	return tools
}

// Update updates an existing tool
func (r *ToolRegistry) Update(name string, definition *proto.ToolDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tool, exists := r.tools[name]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "tool not found")
	}

	tool.mu.Lock()
	tool.Definition = definition
	tool.UpdatedAt = time.Now()
	tool.mu.Unlock()

	r.logger.Info("Tool updated", "tool_name", name)

	return nil
}

// Enable enables a tool
func (r *ToolRegistry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tool, exists := r.tools[name]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "tool not found")
	}

	tool.mu.Lock()
	tool.Enabled = true
	tool.State = proto.ToolState_TOOL_STATE_AVAILABLE
	tool.UpdatedAt = time.Now()
	tool.mu.Unlock()

	r.logger.Info("Tool enabled", "tool_name", name)

	return nil
}

// Disable disables a tool
func (r *ToolRegistry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tool, exists := r.tools[name]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "tool not found")
	}

	tool.mu.Lock()
	tool.Enabled = false
	tool.State = proto.ToolState_TOOL_STATE_DISABLED
	tool.UpdatedAt = time.Now()
	tool.mu.Unlock()

	r.logger.Info("Tool disabled", "tool_name", name)

	return nil
}

// RecordExecution records an execution for a tool
func (r *ToolRegistry) RecordExecution(toolName string, durationNs int64, success bool) error {
	r.mu.RLock()
	tool, exists := r.tools[toolName]
	r.mu.RUnlock()

	if !exists {
		return types.NewError(types.ErrCodeNotFound, "tool not found")
	}

	tool.mu.Lock()
	defer tool.mu.Unlock()

	tool.UsageStats.mu.Lock()
	defer tool.UsageStats.mu.Unlock()

	tool.UsageStats.TotalExecutions++
	if success {
		tool.UsageStats.SuccessfulExecutions++
	} else {
		tool.UsageStats.FailedExecutions++
	}
	tool.UsageStats.TotalDurationNs += durationNs
	tool.UsageStats.AvgDurationNs = tool.UsageStats.TotalDurationNs / tool.UsageStats.TotalExecutions

	now := time.Now()
	tool.UsageStats.LastExecution = &now
	tool.LastUsedAt = &now

	return nil
}

// ExecutionRegistry manages tool execution tracking
type ExecutionRegistry struct {
	mu         sync.RWMutex
	executions map[string]*ExecutionInfo
	logger     *logger.Logger
}

// NewExecutionRegistry creates a new execution registry
func NewExecutionRegistry(log *logger.Logger) *ExecutionRegistry {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	return &ExecutionRegistry{
		executions: make(map[string]*ExecutionInfo),
		logger:     log.With("component", "execution_registry"),
	}
}

// Add adds a new execution
func (r *ExecutionRegistry) Add(execution *ExecutionInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	execution.ExecutionID = string(types.GenerateID())
	execution.CreatedAt = time.Now()
	execution.Status = proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_PENDING

	r.executions[execution.ExecutionID] = execution

	r.logger.Info("Execution added", "execution_id", execution.ExecutionID, "tool_name", execution.ToolName)

	return nil
}

// Get retrieves an execution by ID
func (r *ExecutionRegistry) Get(executionID string) (*ExecutionInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	execution, exists := r.executions[executionID]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, "execution not found")
	}

	return execution, nil
}

// List returns all executions
func (r *ExecutionRegistry) List() []*ExecutionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	executions := make([]*ExecutionInfo, 0, len(r.executions))
	for _, execution := range r.executions {
		executions = append(executions, execution)
	}

	return executions
}

// ListByTool returns all executions for a specific tool
func (r *ExecutionRegistry) ListByTool(toolName string) []*ExecutionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	executions := make([]*ExecutionInfo, 0)
	for _, execution := range r.executions {
		if execution.ToolName == toolName {
			executions = append(executions, execution)
		}
	}

	return executions
}

// UpdateStatus updates the status of an execution
func (r *ExecutionRegistry) UpdateStatus(executionID string, status proto.ToolExecutionStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	execution, exists := r.executions[executionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "execution not found")
	}

	execution.mu.Lock()
	execution.Status = status
	if status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING && execution.StartedAt == nil {
		now := time.Now()
		execution.StartedAt = &now
	}
	if status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_COMPLETED ||
	   status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_FAILED ||
	   status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_TIMEOUT ||
	   status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED {
		now := time.Now()
		execution.CompletedAt = &now
		if execution.StartedAt != nil {
			execution.DurationNs = now.Sub(*execution.StartedAt).Nanoseconds()
		}
	}
	execution.mu.Unlock()

	return nil
}

// Cancel cancels an execution
func (r *ExecutionRegistry) Cancel(executionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	execution, exists := r.executions[executionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "execution not found")
	}

	execution.mu.Lock()
	execution.Status = proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED
	now := time.Now()
	execution.CompletedAt = &now
	if execution.StartedAt != nil {
		execution.DurationNs = now.Sub(*execution.StartedAt).Nanoseconds()
	}
	execution.mu.Unlock()

	r.logger.Info("Execution cancelled", "execution_id", executionID)

	return nil
}

// ToolService implements the gRPC ToolService interface
type ToolService struct {
	proto.UnimplementedToolServiceServer
	deps              ToolServiceDependencies
	toolRegistry      *ToolRegistry
	executionRegistry *ExecutionRegistry
	executor          *tools.Executor
	logger            *logger.Logger
	mu                sync.RWMutex

	// Track active streams for graceful shutdown
	streams map[interface{}]context.CancelFunc
}

// NewToolService creates a new tool service
func NewToolService(deps ToolServiceDependencies, log *logger.Logger) *ToolService {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	return &ToolService{
		deps:              deps,
		toolRegistry:      NewToolRegistry(log),
		executionRegistry: NewExecutionRegistry(log),
		executor:          deps.Executor(),
		logger:            log.With("component", "tool_service"),
		streams:           make(map[interface{}]context.CancelFunc),
	}
}

// GetToolRegistry returns the tool registry
func (s *ToolService) GetToolRegistry() *ToolRegistry {
	return s.toolRegistry
}

// GetExecutionRegistry returns the execution registry
func (s *ToolService) GetExecutionRegistry() *ExecutionRegistry {
	return s.executionRegistry
}

// =============================================================================
// Tool Registration and Lifecycle RPCs
// =============================================================================

// RegisterTool registers a new tool with the orchestrator
func (s *ToolService) RegisterTool(ctx context.Context, req *proto.RegisterToolRequest) (*proto.RegisterToolResponse, error) {
	s.logger.Debug("RegisterTool called", "name", req.Definition.Name)

	if req.Definition == nil {
		return nil, errInvalidArgument("definition is required")
	}
	if req.Definition.Name == "" {
		return nil, errInvalidArgument("definition.name is required")
	}

	name := req.Definition.Name

	// Create tool info
	info := &ToolInfo{
		DisplayName: req.Definition.DisplayName,
		Type:        req.Definition.Type,
		Definition:  req.Definition,
		Enabled:     req.Definition.Enabled,
		Version:     req.Definition.Version,
		UsageStats: &ToolUsageStatsInfo{
			TotalExecutions:     0,
			SuccessfulExecutions: 0,
			FailedExecutions:    0,
			TimeoutExecutions:   0,
			CancelledExecutions: 0,
		},
	}

	// Register tool
	if err := s.toolRegistry.Register(name, info, req.Force); err != nil {
		s.logger.Error("Failed to register tool", "name", name, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Get registered tool
	tool, err := s.toolRegistry.Get(name)
	if err != nil {
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Tool registered", "name", name)

	// Publish event to event bus
	if s.deps != nil {
		bus := s.deps.EventBus()
		if bus != nil {
			event := types.Event{
				Type:      "tool.registered",
				Source:    "tool.grpc",
				Timestamp: types.NewTimestamp(),
				Metadata: types.EventMetadata{
					Priority: types.PriorityNormal,
				},
				Data: map[string]interface{}{
					"tool_name": name,
					"type":      req.Definition.Type.String(),
				},
			}
			if err := bus.Publish(ctx, event); err != nil {
				s.logger.Warn("Failed to publish tool registered event", "error", err)
			}
		}
	}

	return &proto.RegisterToolResponse{
		Name: name,
		Tool: s.toolInfoToProto(tool),
	}, nil
}

// UnregisterTool unregisters a tool from the orchestrator
func (s *ToolService) UnregisterTool(ctx context.Context, req *proto.UnregisterToolRequest) (*proto.UnregisterToolResponse, error) {
	s.logger.Debug("UnregisterTool called", "name", req.Name)

	if req.Name == "" {
		return nil, errInvalidArgument("name is required")
	}

	if err := s.toolRegistry.Unregister(req.Name, req.Force); err != nil {
		s.logger.Error("Failed to unregister tool", "name", req.Name, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	message := "Tool unregistered"
	s.logger.Info("Tool unregistered", "name", req.Name, "force", req.Force)

	// Publish event to event bus
	if s.deps != nil {
		bus := s.deps.EventBus()
		if bus != nil {
			event := types.Event{
				Type:      "tool.unregistered",
				Source:    "tool.grpc",
				Timestamp: types.NewTimestamp(),
				Metadata: types.EventMetadata{
					Priority: types.PriorityNormal,
				},
				Data: map[string]interface{}{
					"tool_name": req.Name,
					"force":     req.Force,
				},
			}
			if err := bus.Publish(ctx, event); err != nil {
				s.logger.Warn("Failed to publish tool unregistered event", "error", err)
			}
		}
	}

	return &proto.UnregisterToolResponse{
		Success: true,
		Message: message,
	}, nil
}

// UpdateTool updates an existing tool's definition
func (s *ToolService) UpdateTool(ctx context.Context, req *proto.UpdateToolRequest) (*proto.UpdateToolResponse, error) {
	s.logger.Debug("UpdateTool called", "name", req.Name)

	if req.Name == "" {
		return nil, errInvalidArgument("name is required")
	}
	if req.Definition == nil {
		return nil, errInvalidArgument("definition is required")
	}

	if err := s.toolRegistry.Update(req.Name, req.Definition); err != nil {
		s.logger.Error("Failed to update tool", "name", req.Name, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	tool, err := s.toolRegistry.Get(req.Name)
	if err != nil {
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Tool updated", "name", req.Name)

	return &proto.UpdateToolResponse{
		Tool: s.toolInfoToProto(tool),
	}, nil
}

// EnableTool enables a tool for execution
func (s *ToolService) EnableTool(ctx context.Context, req *proto.EnableToolRequest) (*proto.EnableToolResponse, error) {
	s.logger.Debug("EnableTool called", "name", req.Name)

	if req.Name == "" {
		return nil, errInvalidArgument("name is required")
	}

	if err := s.toolRegistry.Enable(req.Name); err != nil {
		s.logger.Error("Failed to enable tool", "name", req.Name, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Tool enabled", "name", req.Name)

	return &proto.EnableToolResponse{
		Name:    req.Name,
		Enabled: true,
	}, nil
}

// DisableTool disables a tool from execution
func (s *ToolService) DisableTool(ctx context.Context, req *proto.DisableToolRequest) (*proto.DisableToolResponse, error) {
	s.logger.Debug("DisableTool called", "name", req.Name)

	if req.Name == "" {
		return nil, errInvalidArgument("name is required")
	}

	if err := s.toolRegistry.Disable(req.Name); err != nil {
		s.logger.Error("Failed to disable tool", "name", req.Name, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	var cancelledCount int32 = 0
	if req.CancelActiveExecutions {
		// Cancel all active executions for this tool
		executions := s.executionRegistry.ListByTool(req.Name)
		for _, exec := range executions {
			exec.mu.RLock()
			status := exec.Status
			exec.mu.RUnlock()

			if status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING ||
			   status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_PENDING {
				if err := s.executionRegistry.Cancel(exec.ExecutionID); err == nil {
					cancelledCount++
				}
			}
		}
	}

	s.logger.Info("Tool disabled", "name", req.Name, "cancelled_executions", cancelledCount)

	return &proto.DisableToolResponse{
		Name:                req.Name,
		Disabled:            true,
		CancelledExecutions: cancelledCount,
	}, nil
}

// =============================================================================
// Tool Discovery RPCs
// =============================================================================

// ListTools lists all tools matching the specified filter
func (s *ToolService) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	s.logger.Debug("ListTools called")

	tools := s.toolRegistry.List()

	var filteredTools []*ToolInfo
	if req.Filter != nil {
		filteredTools = s.applyToolFilter(tools, req.Filter)
	} else {
		filteredTools = tools
	}

	pbTools := make([]*proto.ToolInstance, len(filteredTools))
	for i, tool := range filteredTools {
		pbTools[i] = s.toolInfoToProto(tool)
	}

	return &proto.ListToolsResponse{
		Tools:      pbTools,
		TotalCount: int32(len(pbTools)),
	}, nil
}

// GetTool retrieves a tool by its name
func (s *ToolService) GetTool(ctx context.Context, req *proto.GetToolRequest) (*proto.GetToolResponse, error) {
	s.logger.Debug("GetTool called", "name", req.Name)

	if req.Name == "" {
		return nil, errInvalidArgument("name is required")
	}

	tool, err := s.toolRegistry.Get(req.Name)
	if err != nil {
		s.logger.Error("Failed to get tool", "name", req.Name, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	return &proto.GetToolResponse{
		Tool: s.toolInfoToProto(tool),
	}, nil
}

// GetToolDefinition retrieves a tool's definition
func (s *ToolService) GetToolDefinition(ctx context.Context, req *proto.GetToolDefinitionRequest) (*proto.GetToolDefinitionResponse, error) {
	s.logger.Debug("GetToolDefinition called", "name", req.Name)

	if req.Name == "" {
		return nil, errInvalidArgument("name is required")
	}

	tool, err := s.toolRegistry.Get(req.Name)
	if err != nil {
		s.logger.Error("Failed to get tool", "name", req.Name, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	return &proto.GetToolDefinitionResponse{
		Definition: tool.Definition,
	}, nil
}

// =============================================================================
// Tool Execution RPCs
// =============================================================================

// ExecuteTool executes a tool with the specified parameters
func (s *ToolService) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	s.logger.Debug("ExecuteTool called", "tool_name", req.ToolName, "session_id", req.SessionId)

	if req.ToolName == "" {
		return nil, errInvalidArgument("tool_name is required")
	}

	// Validate tool exists and is enabled
	tool, err := s.toolRegistry.Get(req.ToolName)
	if err != nil {
		s.logger.Error("Tool not found", "tool_name", req.ToolName, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	tool.mu.RLock()
	enabled := tool.Enabled
	tool.mu.RUnlock()

	if !enabled {
		return nil, errFailedPrecondition("tool is disabled")
	}

	// Create execution info
	execution := &ExecutionInfo{
		ToolName:      req.ToolName,
		SessionID:     req.SessionId,
		Parameters:    req.Parameters,
		CorrelationID: req.CorrelationId,
		Status:        proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_PENDING,
	}

	// Add execution to registry
	if err := s.executionRegistry.Add(execution); err != nil {
		s.logger.Error("Failed to create execution", "tool_name", req.ToolName, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Tool execution created", "execution_id", execution.ExecutionID, "tool_name", req.ToolName)

	// Publish event to event bus
	if s.deps != nil {
		bus := s.deps.EventBus()
		if bus != nil {
			event := types.Event{
				Type:      "tool.execution.created",
				Source:    "tool.grpc",
				Timestamp: types.NewTimestamp(),
				Metadata: types.EventMetadata{
					Priority: types.PriorityNormal,
				},
				Data: map[string]interface{}{
					"execution_id": execution.ExecutionID,
					"tool_name":    req.ToolName,
					"session_id":   req.SessionId,
				},
			}
			if err := bus.Publish(ctx, event); err != nil {
				s.logger.Warn("Failed to publish tool execution created event", "error", err)
			}
		}
	}

	// Execute the tool using the executor
	execCfg := tools.ExecutionConfig{
		ToolName:    req.ToolName,
		Parameters:  req.Parameters,
		SessionID:   types.ID(req.SessionId),
		Timeout:     time.Duration(req.TimeoutNs),
		AutoCleanup: true,
	}

	// Update status to running
	if err := s.executionRegistry.UpdateStatus(execution.ExecutionID, proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING); err != nil {
		s.logger.Warn("Failed to update execution status to running", "execution_id", execution.ExecutionID, "error", err)
	}

	result, err := s.executor.Execute(ctx, execCfg)
	if err != nil {
		s.logger.Error("Tool execution failed", "execution_id", execution.ExecutionID, "tool_name", req.ToolName, "error", err)
		_ = s.executionRegistry.UpdateStatus(execution.ExecutionID, proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_FAILED)
		return nil, grpcErrorFromTypesError(err)
	}

	// Update execution with result
	status := toolExecutionStatusToProto(result.Status)
	execution.mu.Lock()
	execution.Status = status
	execution.Result = &proto.ToolExecutionResult{
		ExecutionId: result.ExecutionID,
		ToolName:    result.ToolName,
		Status:      status,
		ExitCode:    result.ExitCode,
		OutputData:  result.OutputData,
		OutputText:  result.OutputText,
		ErrorData:   result.ErrorData,
		ErrorText:   result.ErrorText,
		Metadata:    result.Metadata,
		DurationNs:  result.Duration.Nanoseconds(),
		CompletedAt: timestamppb.New(result.CompletedAt),
		ContainerId: result.ContainerID,
	}
	if execution.StartedAt != nil {
		execution.DurationNs = result.Duration.Nanoseconds()
	}
	execution.ContainerID = result.ContainerID
	execution.mu.Unlock()

	// Record successful execution
	success := result.Status == tools.ToolExecutionStatusCompleted
	_ = s.toolRegistry.RecordExecution(req.ToolName, result.Duration.Nanoseconds(), success)

	return &proto.ExecuteToolResponse{
		ExecutionId: execution.ExecutionID,
		Execution:   s.executionInfoToProto(execution),
	}, nil
}

// StreamTool establishes a bidirectional stream for tool execution with real-time output
func (s *ToolService) StreamTool(stream proto.ToolService_StreamToolServer) error {
	ctx := stream.Context()
	s.logger.Debug("StreamTool started")

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

	var executionID string
	var firstMessage = true

	// Receive loop
	for {
		req, err := stream.Recv()
		if err != nil {
			if firstMessage {
				s.logger.Debug("StreamTool ended without receiving any message")
				return err
			}
			s.logger.Debug("StreamTool receive loop ended", "execution_id", executionID, "error", err)
			return nil
		}

		// Handle heartbeat
		if req.GetHeartbeat() != nil {
			if err := stream.Send(&proto.StreamToolResponse{
				Payload: &proto.StreamToolResponse_Heartbeat{&emptypb.Empty{}},
			}); err != nil {
				s.logger.Error("Failed to send heartbeat response", "error", err)
				return err
			}
			continue
		}

		// First message must contain execution_id
		if firstMessage {
			if req.ExecutionId == "" {
				return errInvalidArgument("first message must contain execution_id")
			}
			executionID = req.ExecutionId
			firstMessage = false
			s.logger.Debug("StreamTool established for execution", "execution_id", executionID)

			// Validate execution exists
			_, err := s.executionRegistry.Get(executionID)
			if err != nil {
				s.logger.Error("Execution not found", "execution_id", executionID, "error", err)
				return grpcErrorFromTypesError(err)
			}
		}

		// Handle input
		if req.GetInput() != nil {
			input := req.GetInput()

			// Send output response (echo for now)
			resp := &proto.StreamToolResponse{
				Payload: &proto.StreamToolResponse_Output{
					Output: &proto.ToolOutput{
						Data:       input.Data,
						Text:       string(input.Data),
						StreamType: "stdout",
						Timestamp:  timestamppb.Now(),
					},
				},
			}
			if err := stream.Send(resp); err != nil {
				s.logger.Error("Failed to send output", "execution_id", executionID, "error", err)
				return err
			}
		}

		// Handle cancel command
		if req.GetCancel() != nil {
			// Cancel execution
			if err := s.executionRegistry.Cancel(executionID); err == nil {
				execution, _ := s.executionRegistry.Get(executionID)

				// Build result from execution if available
				var result *proto.ToolExecutionResult
				if execution != nil {
					result = s.buildExecutionResult(execution)
					result.Status = proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED
				} else {
					result = &proto.ToolExecutionResult{
						Status: proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED,
					}
				}

				// Send complete response
				resp := &proto.StreamToolResponse{
					Payload: &proto.StreamToolResponse_Complete{
						Complete: &proto.ToolExecutionComplete{
							ExecutionId: executionID,
							CompletedAt: timestamppb.Now(),
							Result:       result,
						},
					},
				}

				if err := stream.Send(resp); err != nil {
					s.logger.Error("Failed to send cancel response", "execution_id", executionID, "error", err)
					return err
				}
			}
			s.logger.Info("Tool execution cancelled", "execution_id", executionID, "reason", req.GetCancel().Reason)
			return nil
		}
	}
}

// CancelExecution cancels a running tool execution
func (s *ToolService) CancelExecution(ctx context.Context, req *proto.CancelExecutionRequest) (*proto.CancelExecutionResponse, error) {
	s.logger.Debug("CancelExecution called", "execution_id", req.ExecutionId)

	if req.ExecutionId == "" {
		return nil, errInvalidArgument("execution_id is required")
	}

	_, err := s.executionRegistry.Get(req.ExecutionId)
	if err != nil {
		s.logger.Error("Failed to get execution for cancellation", "execution_id", req.ExecutionId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	if err := s.executionRegistry.Cancel(req.ExecutionId); err != nil {
		s.logger.Error("Failed to cancel execution", "execution_id", req.ExecutionId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	s.logger.Info("Tool execution cancelled", "execution_id", req.ExecutionId, "reason", req.Reason, "force", req.Force)

	return &proto.CancelExecutionResponse{
		ExecutionId: req.ExecutionId,
		Status:      proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED,
		Cancelled:   true,
	}, nil
}

// GetExecutionStatus retrieves the status of a tool execution
func (s *ToolService) GetExecutionStatus(ctx context.Context, req *proto.GetExecutionStatusRequest) (*proto.GetExecutionStatusResponse, error) {
	s.logger.Debug("GetExecutionStatus called", "execution_id", req.ExecutionId)

	if req.ExecutionId == "" {
		return nil, errInvalidArgument("execution_id is required")
	}

	execution, err := s.executionRegistry.Get(req.ExecutionId)
	if err != nil {
		s.logger.Error("Failed to get execution status", "execution_id", req.ExecutionId, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	return &proto.GetExecutionStatusResponse{
		Execution: s.executionInfoToProto(execution),
	}, nil
}

// ListExecutions lists all tool executions matching the specified filter
func (s *ToolService) ListExecutions(ctx context.Context, req *proto.ListExecutionsRequest) (*proto.ListExecutionsResponse, error) {
	s.logger.Debug("ListExecutions called")

	executions := s.executionRegistry.List()

	var filteredExecs []*ExecutionInfo
	if req.Filter != nil {
		filteredExecs = s.applyExecutionFilter(executions, req.Filter)
	} else {
		filteredExecs = executions
	}

	pbExecutions := make([]*proto.ToolExecution, len(filteredExecs))
	for i, execution := range filteredExecs {
		pbExecutions[i] = s.executionInfoToProto(execution)
	}

	return &proto.ListExecutionsResponse{
		Executions: pbExecutions,
		TotalCount: int32(len(pbExecutions)),
	}, nil
}

// =============================================================================
// Health and Status RPCs
// =============================================================================

// HealthCheck returns the health status of the tool service
func (s *ToolService) HealthCheck(ctx context.Context, req *emptypb.Empty) (*proto.ToolHealthCheckResponse, error) {
	s.logger.Debug("HealthCheck called")

	// Check if registries are initialized
	var health proto.Health
	if s.toolRegistry != nil && s.executionRegistry != nil {
		health = proto.Health_HEALTH_HEALTHY
	} else {
		health = proto.Health_HEALTH_UNHEALTHY
	}

	// TODO: Get version from build info
	version := "dev"

	// List all subsystems
	subsystems := []string{
		"tool_registry",
		"execution_registry",
	}

	// Count registered tools and active executions
	tools := s.toolRegistry.List()
	activeExecutions := 0
	for _, execution := range s.executionRegistry.List() {
		execution.mu.RLock()
		status := execution.Status
		execution.mu.RUnlock()

		if status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING ||
		   status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_PENDING {
			activeExecutions++
		}
	}

	return &proto.ToolHealthCheckResponse{
		Health:           health,
		Version:          version,
		Subsystems:       subsystems,
		RegisteredTools:  int32(len(tools)),
		ActiveExecutions: int32(activeExecutions),
	}, nil
}

// GetServiceStatus returns the current status of the tool service
func (s *ToolService) GetServiceStatus(ctx context.Context, req *emptypb.Empty) (*proto.ToolServiceStatusResponse, error) {
	s.logger.Debug("GetServiceStatus called")

	tools := s.toolRegistry.List()

	// Count enabled tools and active executions
	enabledTools := 0
	for _, tool := range tools {
		tool.mu.RLock()
		if tool.Enabled {
			enabledTools++
		}
		tool.mu.RUnlock()
	}

	activeExecutions := 0
	for _, execution := range s.executionRegistry.List() {
		execution.mu.RLock()
		status := execution.Status
		execution.mu.RUnlock()

		if status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING ||
		   status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_PENDING {
			activeExecutions++
		}
	}

	// For uptime, assume service started at creation time
	startedAt := time.Now()
	uptime := time.Since(startedAt)

	return &proto.ToolServiceStatusResponse{
		Status:           proto.Status_STATUS_RUNNING,
		RegisteredTools:  int32(len(tools)),
		EnabledTools:     int32(enabledTools),
		ActiveExecutions: int32(activeExecutions),
		StartedAt:        timestamppb.New(startedAt),
		Uptime:           timestamppb.New(startedAt.Add(uptime)),
	}, nil
}

// GetStats returns statistics about tool usage
func (s *ToolService) GetStats(ctx context.Context, req *proto.GetStatsRequest) (*proto.GetStatsResponse, error) {
	s.logger.Debug("GetStats called", "tool_name", req.ToolName)

	tools := s.toolRegistry.List()

	// Calculate service stats
	var serviceStats proto.ToolServiceStats
	serviceStats.StartedAt = timestamppb.Now()

	var toolStats []*proto.ToolStats

	for _, tool := range tools {
		// Filter by tool name if specified
		if req.ToolName != "" && tool.Name != req.ToolName {
			continue
		}

		tool.UsageStats.mu.RLock()
		usageStats := &proto.ToolUsageStats{
			TotalExecutions:      tool.UsageStats.TotalExecutions,
			SuccessfulExecutions: tool.UsageStats.SuccessfulExecutions,
			FailedExecutions:     tool.UsageStats.FailedExecutions,
			TimeoutExecutions:    tool.UsageStats.TimeoutExecutions,
			CancelledExecutions:  tool.UsageStats.CancelledExecutions,
			TotalDurationNs:      tool.UsageStats.TotalDurationNs,
			AvgDurationNs:        tool.UsageStats.AvgDurationNs,
		}
		if tool.UsageStats.LastExecution != nil {
			usageStats.LastExecution = timestamppb.New(*tool.UsageStats.LastExecution)
		}
		tool.UsageStats.mu.RUnlock()

		toolStats = append(toolStats, &proto.ToolStats{
			ToolName:   tool.Name,
			UsageStats: usageStats,
		})

		// Update service stats totals
		serviceStats.TotalExecutions += usageStats.TotalExecutions
		serviceStats.SuccessfulExecutions += usageStats.SuccessfulExecutions
		serviceStats.FailedExecutions += usageStats.FailedExecutions
	}

	// Count active executions
	serviceStats.ActiveExecutions = 0
	for _, execution := range s.executionRegistry.List() {
		execution.mu.RLock()
		status := execution.Status
		execution.mu.RUnlock()

		if status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING ||
		   status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_PENDING {
			serviceStats.ActiveExecutions++
		}
	}

	return &proto.GetStatsResponse{
		ServiceStats: &serviceStats,
		ToolStats:    toolStats,
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func toolExecutionStatusToProto(status tools.ToolExecutionStatus) proto.ToolExecutionStatus {
	switch status {
	case tools.ToolExecutionStatusPending:
		return proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_PENDING
	case tools.ToolExecutionStatusRunning:
		return proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING
	case tools.ToolExecutionStatusCompleted:
		return proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_COMPLETED
	case tools.ToolExecutionStatusFailed:
		return proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_FAILED
	case tools.ToolExecutionStatusTimeout:
		return proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_TIMEOUT
	case tools.ToolExecutionStatusCancelled:
		return proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED
	default:
		return proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_UNSPECIFIED
	}
}

// toolInfoToProto converts ToolInfo to protobuf ToolInstance
func (s *ToolService) toolInfoToProto(info *ToolInfo) *proto.ToolInstance {
	info.mu.RLock()
	defer info.mu.RUnlock()

	pb := &proto.ToolInstance{
		Name:         info.Name,
		DisplayName:  info.DisplayName,
		Type:         info.Type,
		State:        info.State,
		Status:       info.Status,
		RegisteredAt: timestamppb.New(info.RegisteredAt),
		UpdatedAt:    timestamppb.New(info.UpdatedAt),
		Definition:   info.Definition,
		Enabled:      info.Enabled,
		Version:      info.Version,
	}

	if info.LastUsedAt != nil {
		pb.LastUsedAt = timestamppb.New(*info.LastUsedAt)
	}

	if info.UsageStats != nil {
		info.UsageStats.mu.RLock()
		pb.UsageStats = &proto.ToolUsageStats{
			TotalExecutions:      info.UsageStats.TotalExecutions,
			SuccessfulExecutions: info.UsageStats.SuccessfulExecutions,
			FailedExecutions:     info.UsageStats.FailedExecutions,
			TimeoutExecutions:    info.UsageStats.TimeoutExecutions,
			CancelledExecutions:  info.UsageStats.CancelledExecutions,
			TotalDurationNs:      info.UsageStats.TotalDurationNs,
			AvgDurationNs:        info.UsageStats.AvgDurationNs,
		}
		if info.UsageStats.LastExecution != nil {
			pb.UsageStats.LastExecution = timestamppb.New(*info.UsageStats.LastExecution)
		}
		info.UsageStats.mu.RUnlock()
	}

	return pb
}

// executionInfoToProto converts ExecutionInfo to protobuf ToolExecution
func (s *ToolService) executionInfoToProto(info *ExecutionInfo) *proto.ToolExecution {
	info.mu.RLock()
	defer info.mu.RUnlock()

	pb := &proto.ToolExecution{
		ExecutionId:   info.ExecutionID,
		ToolName:      info.ToolName,
		SessionId:     info.SessionID,
		Status:        info.Status,
		CreatedAt:     timestamppb.New(info.CreatedAt),
		Parameters:    info.Parameters,
		DurationNs:    info.DurationNs,
		CorrelationId: info.CorrelationID,
	}

	if info.StartedAt != nil {
		pb.StartedAt = timestamppb.New(*info.StartedAt)
	}

	if info.CompletedAt != nil {
		pb.CompletedAt = timestamppb.New(*info.CompletedAt)
	}

	if info.ExpiresAt != nil {
		pb.ExpiresAt = timestamppb.New(*info.ExpiresAt)
	}

	if info.Result != nil {
		pb.Result = info.Result
	} else if info.Status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_COMPLETED ||
		info.Status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_FAILED ||
		info.Status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_TIMEOUT ||
		info.Status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED {
		pb.Result = s.buildExecutionResult(info)
	}

	if info.ContainerID != "" {
		pb.ContainerId = info.ContainerID
	}

	if info.ErrorMessage != "" {
		pb.ErrorMessage = info.ErrorMessage
	}

	return pb
}

// buildExecutionResult builds a ToolExecutionResult from an ExecutionInfo
func (s *ToolService) buildExecutionResult(info *ExecutionInfo) *proto.ToolExecutionResult {
	result := &proto.ToolExecutionResult{
		ExecutionId: info.ExecutionID,
		ToolName:    info.ToolName,
		Status:      info.Status,
		DurationNs:  info.DurationNs,
		CompletedAt: timestamppb.Now(),
	}

	if info.CompletedAt != nil {
		result.CompletedAt = timestamppb.New(*info.CompletedAt)
	}

	if info.ContainerID != "" {
		result.ContainerId = info.ContainerID
	}

	if info.ErrorMessage != "" {
		result.ErrorText = info.ErrorMessage
	}

	return result
}

// applyToolFilter applies filter to tool list
func (s *ToolService) applyToolFilter(tools []*ToolInfo, filter *proto.ToolFilter) []*ToolInfo {
	var filtered []*ToolInfo

	for _, tool := range tools {
		tool.mu.RLock()
		match := true

		// Filter by type
		if filter.Type != proto.ToolType_TOOL_TYPE_UNSPECIFIED && tool.Type != filter.Type {
			match = false
		}

		// Filter by state
		if filter.State != proto.ToolState_TOOL_STATE_UNSPECIFIED && tool.State != filter.State {
			match = false
		}

		// Filter by enabled status
		// Note: filter.Enabled is a bool pointer in proto (optional)
		// For simplicity, we treat unset as matching all
		if filter.Enabled && !tool.Enabled {
			match = false
		}

		tool.mu.RUnlock()

		if match {
			filtered = append(filtered, tool)
		}
	}

	return filtered
}

// applyExecutionFilter applies filter to execution list
func (s *ToolService) applyExecutionFilter(executions []*ExecutionInfo, filter *proto.ExecutionFilter) []*ExecutionInfo {
	var filtered []*ExecutionInfo

	for _, execution := range executions {
		execution.mu.RLock()
		match := true

		// Filter by tool name
		if filter.ToolName != "" && execution.ToolName != filter.ToolName {
			match = false
		}

		// Filter by status
		if filter.Status != proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_UNSPECIFIED && execution.Status != filter.Status {
			match = false
		}

		// Filter by session ID
		if filter.SessionId != "" && execution.SessionID != filter.SessionId {
			match = false
		}

		// Filter by created time range
		if filter.CreatedAfter != nil {
			createdAfter := filter.CreatedAfter.AsTime()
			if execution.CreatedAt.Before(createdAfter) {
				match = false
			}
		}

		if filter.CreatedBefore != nil {
			createdBefore := filter.CreatedBefore.AsTime()
			if execution.CreatedAt.After(createdBefore) {
				match = false
			}
		}

		execution.mu.RUnlock()

		if match {
			filtered = append(filtered, execution)
		}
	}

	return filtered
}

// Close closes all active streams
func (s *ToolService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Closing tool service streams", "count", len(s.streams))

	for stream, cancel := range s.streams {
		cancel()
		delete(s.streams, stream)
	}

	return nil
}

// String returns a string representation of the service
func (s *ToolService) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := s.toolRegistry.List()
	executions := s.executionRegistry.List()

	activeExecutions := 0
	for _, execution := range executions {
		execution.mu.RLock()
		status := execution.Status
		execution.mu.RUnlock()

		if status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING ||
		   status == proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_PENDING {
			activeExecutions++
		}
	}

	return fmt.Sprintf("ToolService{active_streams: %d, registered_tools: %d, active_executions: %d}",
		len(s.streams), len(tools), activeExecutions)
}
