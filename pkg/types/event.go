package types

import "context"

// EventType represents the type of event
type EventType string

const (
	EventTypeContainerCreated   EventType = "container.created"
	EventTypeContainerStarted   EventType = "container.started"
	EventTypeContainerStopped   EventType = "container.stopped"
	EventTypeContainerRemoved   EventType = "container.removed"
	EventTypeContainerError     EventType = "container.error"
	EventTypeSessionCreated     EventType = "session.created"
	EventTypeSessionUpdated     EventType = "session.updated"
	EventTypeSessionClosed      EventType = "session.closed"
	EventTypeTaskCreated       EventType = "task.created"
	EventTypeTaskUpdated       EventType = "task.updated"
	EventTypeTaskCompleted     EventType = "task.completed"
	EventTypeTaskFailed        EventType = "task.failed"
	EventTypeIPCMessage        EventType = "ipc.message"
	EventTypePolicyViolation   EventType = "policy.violation"
	EventTypeResourceThreshold EventType = "resource.threshold"
	EventTypeSystemStartup     EventType = "system.startup"
	EventTypeSystemShutdown    EventType = "system.shutdown"
)

// Event represents a system event
type Event struct {
	ID        ID                 `json:"id"`
	Type      EventType          `json:"type"`
	Source    string             `json:"source"`
	Timestamp Timestamp          `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Metadata  EventMetadata      `json:"metadata,omitempty"`
}

// EventMetadata contains additional information about an event
type EventMetadata struct {
	CorrelationID *ID              `json:"correlation_id,omitempty"`
	SessionID     *ID              `json:"session_id,omitempty"`
	ContainerID   *ID              `json:"container_id,omitempty"`
	UserID        string           `json:"user_id,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Priority      Priority         `json:"priority,omitempty"`
}

// Priority represents the priority level of an event
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// EventHandler handles events
type EventHandler interface {
	// Handle processes an event
	Handle(ctx context.Context, event Event) error

	// CanHandle returns true if the handler can process the event type
	CanHandle(eventType EventType) bool
}

// EventFunc is a function adapter for EventHandler
type EventFunc func(ctx context.Context, event Event) error

// Handle implements EventHandler
func (f EventFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// CanHandle implements EventHandler (always returns true for EventFunc)
func (f EventFunc) CanHandle(eventType EventType) bool {
	return true
}

// EventFilter defines a filter for events
type EventFilter struct {
	Type       *EventType        `json:"type,omitempty"`
	Source     *string           `json:"source,omitempty"`
	SessionID  *ID               `json:"session_id,omitempty"`
	ContainerID *ID              `json:"container_id,omitempty"`
	Priority   *Priority         `json:"priority,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	StartTime  *Timestamp        `json:"start_time,omitempty"`
	EndTime    *Timestamp        `json:"end_time,omitempty"`
}

// EventMiddleware processes events before they reach handlers
type EventMiddleware interface {
	// Process processes an event and returns the modified event or an error
	Process(ctx context.Context, event Event) (Event, error)
}

// MiddlewareFunc is a function adapter for EventMiddleware
type MiddlewareFunc func(ctx context.Context, event Event) (Event, error)

// Process implements EventMiddleware
func (f MiddlewareFunc) Process(ctx context.Context, event Event) (Event, error) {
	return f(ctx, event)
}

// EventSubscription represents a subscription to events
type EventSubscription struct {
	ID       ID               `json:"id"`
	Filter   EventFilter      `json:"filter"`
	Handler  EventHandler     `json:"-"`
	Active   bool             `json:"active"`
	CreatedAt Timestamp       `json:"created_at"`
}

// IPCMessage represents an inter-process communication message
type IPCMessage struct {
	ID        ID              `json:"id"`
	Source    ID              `json:"source"`      // Container ID
	Target    ID              `json:"target"`      // Container ID or orchestrator
	Type      string          `json:"type"`        // Message type (e.g., "request", "response", "notification")
	Payload   []byte          `json:"payload"`     // Message data
	Metadata  IPCMetadata     `json:"metadata"`
	Timestamp Timestamp       `json:"timestamp"`
}

// IPCMetadata contains metadata for IPC messages
type IPCMetadata struct {
	CorrelationID string            `json:"correlation_id,omitempty"`
	SessionID     ID                `json:"session_id,omitempty"`
	MessageID     string            `json:"message_id,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

// EventTopic represents a topic for event routing
type EventTopic string

const (
	TopicAll       EventTopic = "*"
	TopicContainer EventTopic = "container"
	TopicSession   EventTopic = "session"
	TopicTask      EventTopic = "task"
	TopicIPC       EventTopic = "ipc"
	TopicPolicy    EventTopic = "policy"
	TopicSystem    EventTopic = "system"
)
