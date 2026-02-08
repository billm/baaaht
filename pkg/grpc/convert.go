package grpc

import (
	"fmt"
	"time"

	grpcStatus "google.golang.org/grpc/status"
	grpcCodes "google.golang.org/grpc/codes"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// SessionState conversion functions

// protoToSessionState converts protobuf SessionState to types.SessionState
func protoToSessionState(state proto.SessionState) types.SessionState {
	switch state {
	case proto.SessionState_SESSION_STATE_INITIALIZING:
		return types.SessionStateInitializing
	case proto.SessionState_SESSION_STATE_ACTIVE:
		return types.SessionStateActive
	case proto.SessionState_SESSION_STATE_IDLE:
		return types.SessionStateIdle
	case proto.SessionState_SESSION_STATE_CLOSING:
		return types.SessionStateClosing
	case proto.SessionState_SESSION_STATE_CLOSED:
		return types.SessionStateClosed
	default:
		return types.SessionStateInitializing
	}
}

// sessionStateToProto converts types.SessionState to protobuf SessionState
func sessionStateToProto(state types.SessionState) proto.SessionState {
	switch state {
	case types.SessionStateInitializing:
		return proto.SessionState_SESSION_STATE_INITIALIZING
	case types.SessionStateActive:
		return proto.SessionState_SESSION_STATE_ACTIVE
	case types.SessionStateIdle:
		return proto.SessionState_SESSION_STATE_IDLE
	case types.SessionStateClosing:
		return proto.SessionState_SESSION_STATE_CLOSING
	case types.SessionStateClosed:
		return proto.SessionState_SESSION_STATE_CLOSED
	default:
		return proto.SessionState_SESSION_STATE_UNSPECIFIED
	}
}

// Status conversion functions

// protoToStatus converts protobuf Status to types.Status
func protoToStatus(status proto.Status) types.Status {
	switch status {
	case proto.Status_STATUS_UNKNOWN:
		return types.StatusUnknown
	case proto.Status_STATUS_STARTING:
		return types.StatusStarting
	case proto.Status_STATUS_RUNNING:
		return types.StatusRunning
	case proto.Status_STATUS_STOPPING:
		return types.StatusStopping
	case proto.Status_STATUS_STOPPED:
		return types.StatusStopped
	case proto.Status_STATUS_ERROR:
		return types.StatusError
	case proto.Status_STATUS_TERMINATED:
		return types.StatusTerminated
	default:
		return types.StatusUnknown
	}
}

// statusToProto converts types.Status to protobuf Status
func statusToProto(status types.Status) proto.Status {
	switch status {
	case types.StatusUnknown:
		return proto.Status_STATUS_UNKNOWN
	case types.StatusStarting:
		return proto.Status_STATUS_STARTING
	case types.StatusRunning:
		return proto.Status_STATUS_RUNNING
	case types.StatusStopping:
		return proto.Status_STATUS_STOPPING
	case types.StatusStopped:
		return proto.Status_STATUS_STOPPED
	case types.StatusError:
		return proto.Status_STATUS_ERROR
	case types.StatusTerminated:
		return proto.Status_STATUS_TERMINATED
	default:
		return proto.Status_STATUS_UNSPECIFIED
	}
}

// Health conversion functions

// protoToHealth converts protobuf Health to types.Health
func protoToHealth(health proto.Health) types.Health {
	switch health {
	case proto.Health_HEALTH_UNKNOWN:
		return types.HealthUnknown
	case proto.Health_HEALTH_HEALTHY:
		return types.Healthy
	case proto.Health_HEALTH_UNHEALTHY:
		return types.Unhealthy
	case proto.Health_HEALTH_DEGRADED:
		return types.Degraded
	case proto.Health_HEALTH_CHECKING:
		return types.HealthChecking
	default:
		return types.HealthUnknown
	}
}

// healthToProto converts types.Health to protobuf Health
func healthToProto(health types.Health) proto.Health {
	switch health {
	case types.HealthUnknown:
		return proto.Health_HEALTH_UNKNOWN
	case types.Healthy:
		return proto.Health_HEALTH_HEALTHY
	case types.Unhealthy:
		return proto.Health_HEALTH_UNHEALTHY
	case types.Degraded:
		return proto.Health_HEALTH_DEGRADED
	case types.HealthChecking:
		return proto.Health_HEALTH_CHECKING
	default:
		return proto.Health_HEALTH_UNSPECIFIED
	}
}

// MessageRole conversion functions

// protoToMessageRole converts protobuf MessageRole to types.MessageRole
func protoToMessageRole(role proto.MessageRole) types.MessageRole {
	switch role {
	case proto.MessageRole_MESSAGE_ROLE_USER:
		return types.MessageRoleUser
	case proto.MessageRole_MESSAGE_ROLE_SYSTEM:
		return types.MessageRoleSystem
	case proto.MessageRole_MESSAGE_ROLE_ASSISTANT:
		return types.MessageRoleAssistant
	case proto.MessageRole_MESSAGE_ROLE_TOOL:
		return types.MessageRoleTool
	default:
		return types.MessageRoleUser
	}
}

// messageRoleToProto converts types.MessageRole to protobuf MessageRole
func messageRoleToProto(role types.MessageRole) proto.MessageRole {
	switch role {
	case types.MessageRoleUser:
		return proto.MessageRole_MESSAGE_ROLE_USER
	case types.MessageRoleSystem:
		return proto.MessageRole_MESSAGE_ROLE_SYSTEM
	case types.MessageRoleAssistant:
		return proto.MessageRole_MESSAGE_ROLE_ASSISTANT
	case types.MessageRoleTool:
		return proto.MessageRole_MESSAGE_ROLE_TOOL
	default:
		return proto.MessageRole_MESSAGE_ROLE_UNSPECIFIED
	}
}

// Timestamp conversion functions

// protoToTimestamp converts protobuf timestamp to types.Timestamp
func protoToTimestamp(ts *timestamppb.Timestamp) *types.Timestamp {
	if ts == nil {
		return nil
	}
	t := types.NewTimestampFromTime(ts.AsTime())
	return &t
}

// timestampToProto converts types.Timestamp to protobuf timestamp
func timestampToProto(ts *types.Timestamp) *timestamppb.Timestamp {
	if ts == nil {
		return nil
	}
	return timestamppb.New(ts.Time)
}

// Session conversion functions

// sessionToProto converts types.Session to protobuf Session
func sessionToProto(s *types.Session) *proto.Session {
	if s == nil {
		return nil
	}

	pb := &proto.Session{
		Id:        string(s.ID),
		State:     sessionStateToProto(s.State),
		Status:    statusToProto(s.Status),
		CreatedAt: timestampToProto(&s.CreatedAt),
		UpdatedAt: timestampToProto(&s.UpdatedAt),
		ExpiresAt: timestampToProto(s.ExpiresAt),
		ClosedAt:  timestampToProto(s.ClosedAt),
		Metadata:  sessionMetadataToProto(s.Metadata),
		Context:   sessionContextToProto(s.Context),
	}

	if s.Containers != nil {
		pb.ContainerIds = make([]string, len(s.Containers))
		for i, id := range s.Containers {
			pb.ContainerIds[i] = string(id)
		}
	}

	return pb
}

// protoToSessionMetadata converts protobuf SessionMetadata to types.SessionMetadata
func protoToSessionMetadata(m *proto.SessionMetadata) types.SessionMetadata {
	if m == nil {
		return types.SessionMetadata{}
	}

	meta := types.SessionMetadata{
		Name:        m.Name,
		Description: m.Description,
		OwnerID:     m.OwnerId,
	}

	if m.Labels != nil {
		meta.Labels = make(map[string]string)
		for k, v := range m.Labels {
			meta.Labels[k] = v
		}
	}

	if m.Tags != nil {
		meta.Tags = make([]string, len(m.Tags))
		copy(meta.Tags, m.Tags)
	}

	return meta
}

// sessionMetadataToProto converts types.SessionMetadata to protobuf SessionMetadata
func sessionMetadataToProto(m types.SessionMetadata) *proto.SessionMetadata {
	pb := &proto.SessionMetadata{
		Name:        m.Name,
		Description: m.Description,
		OwnerId:     m.OwnerID,
	}

	if m.Labels != nil {
		pb.Labels = make(map[string]string)
		for k, v := range m.Labels {
			pb.Labels[k] = v
		}
	}

	if m.Tags != nil {
		pb.Tags = make([]string, len(m.Tags))
		copy(pb.Tags, m.Tags)
	}

	return pb
}

// protoToSessionConfig converts protobuf SessionConfig to types.SessionConfig
func protoToSessionConfig(c *proto.SessionConfig) types.SessionConfig {
	if c == nil {
		return types.SessionConfig{}
	}

	cfg := types.SessionConfig{
		MaxContainers: int(c.MaxContainers),
	}

	if c.MaxDurationNs > 0 {
		cfg.MaxDuration = time.Duration(c.MaxDurationNs)
	}

	if c.IdleTimeoutNs > 0 {
		cfg.IdleTimeout = time.Duration(c.IdleTimeoutNs)
	}

	if c.ResourceLimits != nil {
		cfg.ResourceLimits = protoToResourceLimits(c.ResourceLimits)
	}

	if c.AllowedImages != nil {
		cfg.AllowedImages = make([]string, len(c.AllowedImages))
		copy(cfg.AllowedImages, c.AllowedImages)
	}

	if c.Policy != nil {
		cfg.Policy = protoToSessionPolicy(c.Policy)
	}

	return cfg
}

// sessionConfigToProto converts types.SessionConfig to protobuf SessionConfig
func sessionConfigToProto(c types.SessionConfig) *proto.SessionConfig {
	pb := &proto.SessionConfig{
		MaxContainers: int32(c.MaxContainers),
	}

	if c.MaxDuration > 0 {
		pb.MaxDurationNs = c.MaxDuration.Nanoseconds()
	}

	if c.IdleTimeout > 0 {
		pb.IdleTimeoutNs = c.IdleTimeout.Nanoseconds()
	}

	pb.ResourceLimits = resourceLimitsToProto(c.ResourceLimits)

	if c.AllowedImages != nil {
		pb.AllowedImages = make([]string, len(c.AllowedImages))
		copy(pb.AllowedImages, c.AllowedImages)
	}

	pb.Policy = sessionPolicyToProto(c.Policy)

	return pb
}

// protoToSessionPolicy converts protobuf SessionPolicy to types.SessionPolicy
func protoToSessionPolicy(p *proto.SessionPolicy) types.SessionPolicy {
	if p == nil {
		return types.SessionPolicy{}
	}

	policy := types.SessionPolicy{
		AllowNetwork:     p.AllowNetwork,
		AllowVolumeMount: p.AllowVolumeMount,
		RequireApproval:  p.RequireApproval,
	}

	if p.AllowedNetworks != nil {
		policy.AllowedNetworks = make([]string, len(p.AllowedNetworks))
		copy(policy.AllowedNetworks, p.AllowedNetworks)
	}

	if p.AllowedPaths != nil {
		policy.AllowedPaths = make([]string, len(p.AllowedPaths))
		copy(policy.AllowedPaths, p.AllowedPaths)
	}

	return policy
}

// sessionPolicyToProto converts types.SessionPolicy to protobuf SessionPolicy
func sessionPolicyToProto(p types.SessionPolicy) *proto.SessionPolicy {
	pb := &proto.SessionPolicy{
		AllowNetwork:     p.AllowNetwork,
		AllowVolumeMount: p.AllowVolumeMount,
		RequireApproval:  p.RequireApproval,
	}

	if p.AllowedNetworks != nil {
		pb.AllowedNetworks = make([]string, len(p.AllowedNetworks))
		copy(pb.AllowedNetworks, p.AllowedNetworks)
	}

	if p.AllowedPaths != nil {
		pb.AllowedPaths = make([]string, len(p.AllowedPaths))
		copy(pb.AllowedPaths, p.AllowedPaths)
	}

	return pb
}

// protoToSessionContext converts protobuf SessionContext to types.SessionContext
func protoToSessionContext(c *proto.SessionContext) types.SessionContext {
	if c == nil {
		return types.SessionContext{}
	}

	ctx := types.SessionContext{}

	if c.Messages != nil {
		ctx.Messages = make([]types.Message, len(c.Messages))
		for i, m := range c.Messages {
			ctx.Messages[i] = protoToMessage(m)
		}
	}

	if c.CurrentTaskId != "" {
		id := types.ID(c.CurrentTaskId)
		ctx.CurrentTaskID = &id
	}

	if c.TaskHistory != nil {
		ctx.TaskHistory = make([]types.ID, len(c.TaskHistory))
		for i, id := range c.TaskHistory {
			ctx.TaskHistory[i] = types.ID(id)
		}
	}

	if c.Preferences != nil {
		ctx.Preferences = protoToUserPreferences(c.Preferences)
	}

	if c.Config != nil {
		ctx.Config = protoToSessionConfig(c.Config)
	}

	if c.ResourceUsage != nil {
		ctx.ResourceUsage = protoToResourceUsage(c.ResourceUsage)
	}

	return ctx
}

// sessionContextToProto converts types.SessionContext to protobuf SessionContext
func sessionContextToProto(c types.SessionContext) *proto.SessionContext {
	pb := &proto.SessionContext{}

	if c.Messages != nil {
		pb.Messages = make([]*proto.Message, len(c.Messages))
		for i, m := range c.Messages {
			pb.Messages[i] = messageToProto(m)
		}
	}

	if c.CurrentTaskID != nil {
		pb.CurrentTaskId = string(*c.CurrentTaskID)
	}

	if c.TaskHistory != nil {
		pb.TaskHistory = make([]string, len(c.TaskHistory))
		for i, id := range c.TaskHistory {
			pb.TaskHistory[i] = string(id)
		}
	}

	pb.Preferences = userPreferencesToProto(c.Preferences)
	pb.Config = sessionConfigToProto(c.Config)
	pb.ResourceUsage = resourceUsageToProto(c.ResourceUsage)

	return pb
}

// protoToUserPreferences converts protobuf UserPreferences to types.UserPreferences
func protoToUserPreferences(p *proto.UserPreferences) types.UserPreferences {
	if p == nil {
		return types.UserPreferences{}
	}

	return types.UserPreferences{
		Timezone:      p.Timezone,
		Language:      p.Language,
		Theme:         p.Theme,
		Notifications: p.Notifications,
	}
}

// userPreferencesToProto converts types.UserPreferences to protobuf UserPreferences
func userPreferencesToProto(p types.UserPreferences) *proto.UserPreferences {
	return &proto.UserPreferences{
		Timezone:      p.Timezone,
		Language:      p.Language,
		Theme:         p.Theme,
		Notifications: p.Notifications,
	}
}

// protoToResourceLimits converts protobuf ResourceLimits to types.ResourceLimits
func protoToResourceLimits(r *proto.ResourceLimits) types.ResourceLimits {
	if r == nil {
		return types.ResourceLimits{}
	}

	limits := types.ResourceLimits{
		NanoCPUs:    r.NanoCpus,
		MemoryBytes: r.MemoryBytes,
		MemorySwap:  r.MemorySwap,
	}

	// PidsLimit is optional in proto but pointer in types
	if r.PidsLimit > 0 {
		limits.PidsLimit = &r.PidsLimit
	}

	return limits
}

// resourceLimitsToProto converts types.ResourceLimits to protobuf ResourceLimits
func resourceLimitsToProto(r types.ResourceLimits) *proto.ResourceLimits {
	pb := &proto.ResourceLimits{
		NanoCpus:    r.NanoCPUs,
		MemoryBytes: r.MemoryBytes,
		MemorySwap:  r.MemorySwap,
	}

	// PidsLimit is pointer in types but int64 in proto
	if r.PidsLimit != nil {
		pb.PidsLimit = *r.PidsLimit
	}

	return pb
}

// protoToResourceUsage converts protobuf ResourceUsage to types.ResourceUsage
func protoToResourceUsage(r *proto.ResourceUsage) types.ResourceUsage {
	if r == nil {
		return types.ResourceUsage{}
	}

	return types.ResourceUsage{
		CPUPercent:    r.CpuPercent,
		MemoryUsage:   r.MemoryUsage,
		MemoryLimit:   r.MemoryLimit,
		MemoryPercent: r.MemoryPercent,
		NetworkRx:     r.NetworkRx,
		NetworkTx:     r.NetworkTx,
		BlockRead:     r.BlockRead,
		BlockWrite:    r.BlockWrite,
		PidsCount:     r.PidsCount,
	}
}

// resourceUsageToProto converts types.ResourceUsage to protobuf ResourceUsage
func resourceUsageToProto(r types.ResourceUsage) *proto.ResourceUsage {
	return &proto.ResourceUsage{
		CpuPercent:    r.CPUPercent,
		MemoryUsage:   r.MemoryUsage,
		MemoryLimit:   r.MemoryLimit,
		MemoryPercent: r.MemoryPercent,
		NetworkRx:     r.NetworkRx,
		NetworkTx:     r.NetworkTx,
		BlockRead:     r.BlockRead,
		BlockWrite:    r.BlockWrite,
		PidsCount:     r.PidsCount,
	}
}

// Message conversion functions

// protoToMessage converts protobuf Message to types.Message
func protoToMessage(m *proto.Message) types.Message {
	if m == nil {
		return types.Message{}
	}

	msg := types.Message{
		ID:        types.ID(m.Id),
		Role:      protoToMessageRole(m.Role),
		Content:   m.Content,
		Metadata:  protoToMessageMetadata(m.Metadata),
	}

	if m.Timestamp != nil {
		ts := protoToTimestamp(m.Timestamp)
		if ts != nil {
			msg.Timestamp = *ts
		}
	}

	return msg
}

// messageToProto converts types.Message to protobuf Message
func messageToProto(m types.Message) *proto.Message {
	pb := &proto.Message{
		Id:      string(m.ID),
		Role:    messageRoleToProto(m.Role),
		Content: m.Content,
		Metadata: &proto.MessageMetadata{
			ToolName:   m.Metadata.ToolName,
			ToolCallId: m.Metadata.ToolCallID,
		},
	}

	if !m.Timestamp.IsZero() {
		pb.Timestamp = timestampToProto(&m.Timestamp)
	}

	if m.Metadata.ContainerID != nil {
		pb.Metadata.ContainerId = string(*m.Metadata.ContainerID)
	}

	if m.Metadata.Extra != nil {
		pb.Metadata.Extra = make(map[string]string)
		for k, v := range m.Metadata.Extra {
			pb.Metadata.Extra[k] = v
		}
	}

	return pb
}

// protoToMessageMetadata converts protobuf MessageMetadata to types.MessageMetadata
func protoToMessageMetadata(m *proto.MessageMetadata) types.MessageMetadata {
	if m == nil {
		return types.MessageMetadata{}
	}

	meta := types.MessageMetadata{
		ToolName:   m.ToolName,
		ToolCallID: m.ToolCallId,
	}

	if m.ContainerId != "" {
		id := types.ID(m.ContainerId)
		meta.ContainerID = &id
	}

	if m.Extra != nil {
		meta.Extra = make(map[string]string)
		for k, v := range m.Extra {
			meta.Extra[k] = v
		}
	}

	return meta
}

// SessionFilter conversion functions

// protoToSessionFilter converts protobuf SessionFilter to types.SessionFilter
func protoToSessionFilter(f *proto.SessionFilter) *types.SessionFilter {
	if f == nil {
		return nil
	}

	filter := &types.SessionFilter{}

	if f.State != proto.SessionState_SESSION_STATE_UNSPECIFIED {
		state := protoToSessionState(f.State)
		filter.State = &state
	}

	if f.Status != proto.Status_STATUS_UNSPECIFIED {
		status := protoToStatus(f.Status)
		filter.Status = &status
	}

	if f.OwnerId != "" {
		filter.OwnerID = &f.OwnerId
	}

	if f.Labels != nil {
		filter.Label = make(map[string]string)
		for k, v := range f.Labels {
			filter.Label[k] = v
		}
	}

	return filter
}

// Event conversion functions

// eventToProto converts types.Event to protobuf Event
func eventToProto(e types.Event) *proto.Event {
	pb := &proto.Event{
		Id:        string(e.ID),
		Type:      eventTypeToProto(string(e.Type)),
		Source:    e.Source,
		Timestamp: timestampToProto(&e.Timestamp),
		Metadata: &proto.EventMetadata{
			UserId:   e.Metadata.UserID,
			Priority: priorityToProto(string(e.Metadata.Priority)),
		},
	}

	// Convert optional ID fields
	if e.Metadata.CorrelationID != nil {
		pb.Metadata.CorrelationId = string(*e.Metadata.CorrelationID)
	}
	if e.Metadata.SessionID != nil {
		pb.Metadata.SessionId = string(*e.Metadata.SessionID)
	}
	if e.Metadata.ContainerID != nil {
		pb.Metadata.ContainerId = string(*e.Metadata.ContainerID)
	}

	// Convert data map from map[string]interface{} to map[string]string
	if e.Data != nil {
		pb.Data = make(map[string]string)
		for k, v := range e.Data {
			// Convert value to string
			var strVal string
			if v != nil {
				strVal = fmt.Sprintf("%v", v)
			}
			pb.Data[k] = strVal
		}
	}

	if e.Metadata.Labels != nil {
		pb.Metadata.Labels = make(map[string]string)
		for k, v := range e.Metadata.Labels {
			pb.Metadata.Labels[k] = v
		}
	}

	return pb
}

// eventTypeToProto converts types.EventType to protobuf EventType
func eventTypeToProto(et string) proto.EventType {
	// Map event type strings to protobuf enum values
	switch et {
	case "container.created":
		return proto.EventType_EVENT_TYPE_CONTAINER_CREATED
	case "container.started":
		return proto.EventType_EVENT_TYPE_CONTAINER_STARTED
	case "container.stopped":
		return proto.EventType_EVENT_TYPE_CONTAINER_STOPPED
	case "container.removed":
		return proto.EventType_EVENT_TYPE_CONTAINER_REMOVED
	case "container.error":
		return proto.EventType_EVENT_TYPE_CONTAINER_ERROR
	case "session.created":
		return proto.EventType_EVENT_TYPE_SESSION_CREATED
	case "session.updated":
		return proto.EventType_EVENT_TYPE_SESSION_UPDATED
	case "session.closed":
		return proto.EventType_EVENT_TYPE_SESSION_CLOSED
	case "task.created":
		return proto.EventType_EVENT_TYPE_TASK_CREATED
	case "task.updated":
		return proto.EventType_EVENT_TYPE_TASK_UPDATED
	case "task.completed":
		return proto.EventType_EVENT_TYPE_TASK_COMPLETED
	case "task.failed":
		return proto.EventType_EVENT_TYPE_TASK_FAILED
	case "ipc.message":
		return proto.EventType_EVENT_TYPE_IPC_MESSAGE
	case "policy.violation":
		return proto.EventType_EVENT_TYPE_POLICY_VIOLATION
	case "resource.threshold":
		return proto.EventType_EVENT_TYPE_RESOURCE_THRESHOLD
	case "system.startup":
		return proto.EventType_EVENT_TYPE_SYSTEM_STARTUP
	case "system.shutdown":
		return proto.EventType_EVENT_TYPE_SYSTEM_SHUTDOWN
	default:
		return proto.EventType_EVENT_TYPE_UNSPECIFIED
	}
}

// priorityToProto converts types.Priority to protobuf Priority
func priorityToProto(p string) proto.Priority {
	switch p {
	case "low":
		return proto.Priority_PRIORITY_LOW
	case "normal":
		return proto.Priority_PRIORITY_NORMAL
	case "high":
		return proto.Priority_PRIORITY_HIGH
	case "critical":
		return proto.Priority_PRIORITY_CRITICAL
	default:
		return proto.Priority_PRIORITY_UNSPECIFIED
	}
}

// grpcErrorFromTypesError converts a types.Error to a gRPC status error
func grpcErrorFromTypesError(err error) error {
	if err == nil {
		return nil
	}

	// Check if it's a types.Error
	if e, ok := err.(*types.Error); ok {
		// Map error codes to gRPC codes
		switch e.Code {
		case types.ErrCodeNotFound:
			return errNotFound(e.Message)
		case types.ErrCodeAlreadyExists:
			return errAlreadyExists(e.Message)
		case types.ErrCodeInvalidArgument:
			return errInvalidArgument(e.Message)
		case types.ErrCodePermissionDenied:
			return errPermissionDenied(e.Message)
		case types.ErrCodeInternal:
			return errInternal(e.Message)
		case types.ErrCodeUnavailable:
			return errUnavailable(e.Message)
		case types.ErrCodeTimeout:
			return errTimeout(e.Message)
		case types.ErrCodeFailedPrecondition:
			return errFailedPrecondition(e.Message)
		case types.ErrCodeResourceExhausted:
			return errResourceExhausted(e.Message)
		default:
			return errInternal(e.Message)
		}
	}

	// Fallback for unknown errors
	return errInternal(err.Error())
}

// Helper functions to create gRPC status errors
func errNotFound(msg string) error {
	return grpcStatus.Error(grpcCodes.NotFound, msg)
}

func errAlreadyExists(msg string) error {
	return grpcStatus.Error(grpcCodes.AlreadyExists, msg)
}

func errInvalidArgument(msg string) error {
	return grpcStatus.Error(grpcCodes.InvalidArgument, msg)
}

func errPermissionDenied(msg string) error {
	return grpcStatus.Error(grpcCodes.PermissionDenied, msg)
}

func errInternal(msg string) error {
	return grpcStatus.Error(grpcCodes.Internal, msg)
}

func errUnavailable(msg string) error {
	return grpcStatus.Error(grpcCodes.Unavailable, msg)
}

func errTimeout(msg string) error {
	return grpcStatus.Error(grpcCodes.DeadlineExceeded, msg)
}

func errFailedPrecondition(msg string) error {
	return grpcStatus.Error(grpcCodes.FailedPrecondition, msg)
}

func errResourceExhausted(msg string) error {
	return grpcStatus.Error(grpcCodes.ResourceExhausted, msg)
}
