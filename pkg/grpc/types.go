package grpc

import (
	"context"
	"io"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// StreamServer is a generic interface for gRPC streaming servers
type StreamServer interface {
	Context() context.Context
	SendMsg(m interface{}) error
	RecvMsg(m interface{}) error
}

// EventHandler is a function that handles events from gRPC streams
type EventHandler func(ctx context.Context, event types.Event) error

// MessageHandler is a function that handles messages from gRPC streams
type MessageHandler func(ctx context.Context, msg *types.Message) error

// StreamCloser represents a stream that can be closed
type StreamCloser interface {
	CloseSend() error
}

// BidirectionalStream represents a bidirectional gRPC stream
type BidirectionalStream interface {
	StreamServer
	StreamCloser
}

// ServerStream represents a server-side streaming gRPC stream
type ServerStream interface {
	Context() context.Context
	Send(m interface{}) error
}

// ClientStream represents a client-side streaming gRPC stream
type ClientStream interface {
	Context() context.Context
	Recv() (*types.Message, error)
}

// EventStream represents a stream of events
type EventStream interface {
	Context() context.Context
	Send(event *types.Event) error
	Recv() (*types.Event, error)
	Close() error
}

// ConvertResult represents the result of a type conversion
type ConvertResult struct {
	Success bool
	Error   error
}

// Converter is a generic interface for type converters
type Converter interface {
	ToProto(interface{}) (interface{}, ConvertResult)
	FromProto(interface{}) (interface{}, ConvertResult)
}

// StreamObserver observes events on a gRPC stream
type StreamObserver struct {
	ctx    context.Context
	cancel context.CancelFunc
	events chan<- types.Event
	errors chan<- error
}

// NewStreamObserver creates a new stream observer
func NewStreamObserver(ctx context.Context, events chan<- types.Event, errors chan<- error) *StreamObserver {
	ctx, cancel := context.WithCancel(ctx)
	return &StreamObserver{
		ctx:    ctx,
		cancel: cancel,
		events: events,
		errors: errors,
	}
}

// Context returns the observer's context
func (o *StreamObserver) Context() context.Context {
	return o.ctx
}

// Cancel cancels the observer's context
func (o *StreamObserver) Cancel() {
	o.cancel()
}

// SendEvent sends an event to the observer's event channel
func (o *StreamObserver) SendEvent(event types.Event) {
	select {
	case o.events <- event:
	case <-o.ctx.Done():
	}
}

// SendError sends an error to the observer's error channel
func (o *StreamObserver) SendError(err error) {
	select {
	case o.errors <- err:
	case <-o.ctx.Done():
	}
}

// Done returns a channel that's closed when the observer is cancelled
func (o *StreamObserver) Done() <-chan struct{} {
	return o.ctx.Done()
}

// MessageIterator iterates over messages from a stream
type MessageIterator struct {
	stream ClientStream
	ctx    context.Context
}

// NewMessageIterator creates a new message iterator
func NewMessageIterator(ctx context.Context, stream ClientStream) *MessageIterator {
	return &MessageIterator{
		stream: stream,
		ctx:    ctx,
	}
}

// Next returns the next message from the stream
func (it *MessageIterator) Next() (*types.Message, error) {
	select {
	case <-it.ctx.Done():
		return nil, it.ctx.Err()
	default:
		return it.stream.Recv()
	}
}

// ForEach iterates over all messages until the stream is closed or context is cancelled
func (it *MessageIterator) ForEach(fn func(*types.Message) error) error {
	for {
		msg, err := it.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(msg); err != nil {
			return err
		}
	}
}

// Collect collects all messages from the stream into a slice
func (it *MessageIterator) Collect() ([]*types.Message, error) {
	var messages []*types.Message
	err := it.ForEach(func(msg *types.Message) error {
		messages = append(messages, msg)
		return nil
	})
	return messages, err
}

// ConversionError represents an error during type conversion
type ConversionError struct {
	FromType string
	ToType   string
	Err      error
}

// Error returns the error message
func (e *ConversionError) Error() string {
	return "conversion error: " + e.FromType + " -> " + e.ToType + ": " + e.Err.Error()
}

// Unwrap returns the underlying error
func (e *ConversionError) Unwrap() error {
	return e.Err
}

// NewConversionError creates a new conversion error
func NewConversionError(fromType, toType string, err error) *ConversionError {
	return &ConversionError{
		FromType: fromType,
		ToType:   toType,
		Err:      err,
	}
}
