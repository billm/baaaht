package provider

import "context"

// StreamingEventType represents the type of streaming event
type StreamingEventType string

const (
	StreamingEventStreamStart    StreamingEventType = "stream.start"
	StreamingEventStreamChunk    StreamingEventType = "stream.chunk"
	StreamingEventStreamEnd      StreamingEventType = "stream.end"
	StreamingEventStreamError    StreamingEventType = "stream.error"
	StreamingEventTokenDelta     StreamingEventType = "token.delta"
	StreamingEventContentDelta   StreamingEventType = "content.delta"
	StreamingEventToolUseStart   StreamingEventType = "tool_use.start"
	StreamingEventToolUseChunk   StreamingEventType = "tool_use.chunk"
	StreamingEventToolUseEnd     StreamingEventType = "tool_use.end"
)

// StreamingEvent represents a streaming event
type StreamingEvent struct {
	ID        string                 `json:"id"`
	Type      StreamingEventType     `json:"type"`
	StreamID  string                 `json:"stream_id"`
	Provider  Provider               `json:"provider"`
	Model     Model                  `json:"model"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Metadata  StreamingEventMetadata `json:"metadata,omitempty"`
}

// StreamingEventMetadata contains additional information about a streaming event
type StreamingEventMetadata struct {
	RequestID    string            `json:"request_id,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	CorrelationID string           `json:"correlation_id,omitempty"`
	Timestamp    int64             `json:"timestamp,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// StreamingEventHandler handles streaming events
type StreamingEventHandler interface {
	// Handle processes a streaming event
	Handle(ctx context.Context, event StreamingEvent) error

	// CanHandle returns true if the handler can process the event type
	CanHandle(eventType StreamingEventType) bool
}

// StreamingEventFunc is a function adapter for StreamingEventHandler
type StreamingEventFunc func(ctx context.Context, event StreamingEvent) error

// Handle implements StreamingEventHandler
func (f StreamingEventFunc) Handle(ctx context.Context, event StreamingEvent) error {
	return f(ctx, event)
}

// CanHandle implements StreamingEventHandler (always returns true for StreamingEventFunc)
func (f StreamingEventFunc) CanHandle(eventType StreamingEventType) bool {
	return true
}

// StreamingEventFilter defines a filter for streaming events
type StreamingEventFilter struct {
	Type         *StreamingEventType `json:"type,omitempty"`
	StreamID     *string             `json:"stream_id,omitempty"`
	Provider     *Provider           `json:"provider,omitempty"`
	Model        *Model              `json:"model,omitempty"`
	SessionID    *string             `json:"session_id,omitempty"`
	CorrelationID *string            `json:"correlation_id,omitempty"`
	Labels       map[string]string   `json:"labels,omitempty"`
}

// Matches returns true if the event matches the filter
func (f *StreamingEventFilter) Matches(event StreamingEvent) bool {
	if f.Type != nil && event.Type != *f.Type {
		return false
	}
	if f.StreamID != nil && event.StreamID != *f.StreamID {
		return false
	}
	if f.Provider != nil && event.Provider != *f.Provider {
		return false
	}
	if f.Model != nil && event.Model != *f.Model {
		return false
	}
	if f.SessionID != nil && event.Metadata.SessionID != *f.SessionID {
		return false
	}
	if f.CorrelationID != nil && event.Metadata.CorrelationID != *f.CorrelationID {
		return false
	}
	if len(f.Labels) > 0 {
		for k, v := range f.Labels {
			if event.Metadata.Labels == nil || event.Metadata.Labels[k] != v {
				return false
			}
		}
	}
	return true
}

// StreamingEventMiddleware processes streaming events before they reach handlers
type StreamingEventMiddleware interface {
	// Process processes a streaming event and returns the modified event or an error
	Process(ctx context.Context, event StreamingEvent) (StreamingEvent, error)
}

// StreamingMiddlewareFunc is a function adapter for StreamingEventMiddleware
type StreamingMiddlewareFunc func(ctx context.Context, event StreamingEvent) (StreamingEvent, error)

// Process implements StreamingEventMiddleware
func (f StreamingMiddlewareFunc) Process(ctx context.Context, event StreamingEvent) (StreamingEvent, error) {
	return f(ctx, event)
}

// StreamingSubscription represents a subscription to streaming events
type StreamingSubscription struct {
	ID       string                   `json:"id"`
	Filter   StreamingEventFilter     `json:"filter"`
	Handler  StreamingEventHandler    `json:"-"`
	Active   bool                     `json:"active"`
	Provider Provider                 `json:"provider"`
}

// StreamHandle represents a handle to an active stream
type StreamHandle struct {
	ID       string     `json:"id"`
	Provider Provider    `json:"provider"`
	Model    Model       `json:"model"`
	Active   bool        `json:"active"`
	Chunks   <-chan *CompletionChunk `json:"-"`
}

// Close closes the stream handle
func (h *StreamHandle) Close() error {
	h.Active = false
	return nil
}

// StreamingCallback is a callback function for streaming events
type StreamingCallback func(chunk *CompletionChunk) error

// StreamingContext contains context for a streaming operation
type StreamingContext struct {
	RequestID    string
	Provider     Provider
	Model        Model
	CancelFunc   context.CancelFunc
	Handler      StreamingEventHandler
	Metadata     map[string]interface{}
}

// WithCancel returns a child context with a cancel function
func (c *StreamingContext) WithCancel() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	c.CancelFunc = cancel
	return ctx, cancel
}
// StreamingOptions configures streaming behavior
type StreamingOptions struct {
	// BufferSize is the size of the channel buffer for chunks
	BufferSize int

	// IncludeUsage specifies whether to include token usage in chunks
	IncludeUsage bool

	// IncludeRaw specifies whether to include raw provider responses
	IncludeRaw bool

	// Callback is called for each chunk
	Callback StreamingCallback

	// Middleware is applied to each event
	Middleware []StreamingEventMiddleware
}

// DefaultStreamingOptions returns default streaming options
func DefaultStreamingOptions() *StreamingOptions {
	return &StreamingOptions{
		BufferSize:   10,
		IncludeUsage: true,
		IncludeRaw:   false,
		Middleware:   nil,
	}
}

// Merge merges the given options with defaults
func (o *StreamingOptions) Merge(other *StreamingOptions) *StreamingOptions {
	if other == nil {
		return o
	}
	result := *o
	if other.BufferSize > 0 {
		result.BufferSize = other.BufferSize
	}
	if other.Callback != nil {
		result.Callback = other.Callback
	}
	if other.IncludeUsage {
		result.IncludeUsage = other.IncludeUsage
	}
	if other.IncludeRaw {
		result.IncludeRaw = other.IncludeRaw
	}
	if len(other.Middleware) > 0 {
		result.Middleware = append(result.Middleware, other.Middleware...)
	}
	return &result
}
