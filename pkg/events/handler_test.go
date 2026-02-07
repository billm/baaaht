package events

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// setupTestHandlerLogger creates a logger for handler tests
func setupTestHandlerLogger(t *testing.T) (*logger.Logger, context.Context) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return log, context.Background()
}

// TestNewHandlerChain tests creating a handler chain
func TestNewHandlerChain(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler1 := newMockEventHandler()
	handler2 := newMockEventHandler()

	chain, err := NewHandlerChain(log, handler1, handler2)
	if err != nil {
		t.Fatalf("Failed to create handler chain: %v", err)
	}

	if chain == nil {
		t.Fatal("Expected non-nil chain")
	}

	if chain.HandlerCount() != 2 {
		t.Errorf("Expected 2 handlers, got %d", chain.HandlerCount())
	}

	// Test handling an event
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	if err := chain.Handle(ctx, event); err != nil {
		t.Errorf("Failed to handle event: %v", err)
	}

	if handler1.getEventCount() != 1 {
		t.Errorf("Expected handler1 to receive 1 event, got %d", handler1.getEventCount())
	}

	if handler2.getEventCount() != 1 {
		t.Errorf("Expected handler2 to receive 1 event, got %d", handler2.getEventCount())
	}
}

// TestNewHandlerChainEmpty tests creating a chain with no handlers
func TestNewHandlerChainEmpty(t *testing.T) {
	log, _ := setupTestHandlerLogger(t)

	_, err := NewHandlerChain(log)
	if err == nil {
		t.Error("Expected error when creating chain with no handlers")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestHandlerChainCanHandle tests CanHandle for handler chain
func TestHandlerChainCanHandle(t *testing.T) {
	log, _ := setupTestHandlerLogger(t)

	// Create handlers that can handle different event types
	handler1 := newMockEventHandler()
	handler1.canHandle = false

	handler2 := newMockEventHandler()
	handler2.canHandle = true

	chain, err := NewHandlerChain(log, handler1, handler2)
	if err != nil {
		t.Fatalf("Failed to create handler chain: %v", err)
	}

	if !chain.CanHandle(types.EventTypeContainerCreated) {
		t.Error("Expected chain to handle event type (handler2 should accept)")
	}
}

// TestHandlerChainAddHandler tests adding a handler to the chain
func TestHandlerChainAddHandler(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler1 := newMockEventHandler()
	chain, err := NewHandlerChain(log, handler1)
	if err != nil {
		t.Fatalf("Failed to create handler chain: %v", err)
	}

	handler2 := newMockEventHandler()
	if err := chain.AddHandler(handler2); err != nil {
		t.Errorf("Failed to add handler: %v", err)
	}

	if chain.HandlerCount() != 2 {
		t.Errorf("Expected 2 handlers, got %d", chain.HandlerCount())
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	if err := chain.Handle(ctx, event); err != nil {
		t.Errorf("Failed to handle event: %v", err)
	}

	if handler2.getEventCount() != 1 {
		t.Errorf("Expected handler2 to receive 1 event, got %d", handler2.getEventCount())
	}
}

// TestHandlerChainRemoveHandler tests removing a handler from the chain
func TestHandlerChainRemoveHandler(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler1 := newMockEventHandler()
	handler2 := newMockEventHandler()
	chain, err := NewHandlerChain(log, handler1, handler2)
	if err != nil {
		t.Fatalf("Failed to create handler chain: %v", err)
	}

	if err := chain.RemoveHandler(0); err != nil {
		t.Errorf("Failed to remove handler: %v", err)
	}

	if chain.HandlerCount() != 1 {
		t.Errorf("Expected 1 handler after removal, got %d", chain.HandlerCount())
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	if err := chain.Handle(ctx, event); err != nil {
		t.Errorf("Failed to handle event: %v", err)
	}

	if handler1.getEventCount() != 0 {
		t.Errorf("Expected handler1 to receive 0 events, got %d", handler1.getEventCount())
	}

	if handler2.getEventCount() != 1 {
		t.Errorf("Expected handler2 to receive 1 event, got %d", handler2.getEventCount())
	}
}

// TestHandlerChainError tests error propagation in chain
func TestHandlerChainError(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler1 := newMockEventHandler()
	handler2 := newMockEventHandler()

	expectedErr := errors.New("handler error")
	handler2.handleFn = func(ctx context.Context, event types.Event) error {
		return expectedErr
	}

	chain, err := NewHandlerChain(log, handler1, handler2)
	if err != nil {
		t.Fatalf("Failed to create handler chain: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	err = chain.Handle(ctx, event)
	if err == nil {
		t.Fatal("Expected error from chain")
	}

	if !types.IsErrCode(err, types.ErrCodeHandlerFailed) {
		t.Errorf("Expected ErrCodeHandlerFailed, got %v", types.GetErrorCode(err))
	}

	// Handler 1 should have executed, handler 2 should have failed
	if handler1.getEventCount() != 1 {
		t.Errorf("Expected handler1 to receive 1 event, got %d", handler1.getEventCount())
	}
}

// TestAsyncHandler tests async handler
func TestAsyncHandler(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler := newMockEventHandler()
	asyncHandler, err := NewAsyncHandler(log, handler, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create async handler: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	if err := asyncHandler.Handle(ctx, event); err != nil {
		t.Errorf("Failed to handle event: %v", err)
	}

	// Give async handler time to complete
	time.Sleep(100 * time.Millisecond)

	if handler.getEventCount() != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", handler.getEventCount())
	}
}

// TestAsyncHandlerTimeout tests async handler timeout
func TestAsyncHandlerTimeout(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler := newMockEventHandler()
	handler.handleFn = func(ctx context.Context, event types.Event) error {
		time.Sleep(2 * time.Second)
		return nil
	}

	asyncHandler, err := NewAsyncHandler(log, handler, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create async handler: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	err = asyncHandler.Handle(ctx, event)
	if err == nil {
		t.Error("Expected timeout error")
	}

	if !types.IsErrCode(err, types.ErrCodeTimeout) {
		t.Errorf("Expected ErrCodeTimeout, got %v", types.GetErrorCode(err))
	}
}

// TestRetryHandler tests retry handler
func TestRetryHandler(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	attempts := int32(0)
	handler := newMockEventHandler()
	handler.handleFn = func(ctx context.Context, event types.Event) error {
		atomic.AddInt32(&attempts, 1)
		if atomic.LoadInt32(&attempts) < 3 {
			return types.NewError(types.ErrCodeInternal, "temporary error")
		}
		return nil
	}

	retryHandler, err := NewRetryHandler(log, handler, 5, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create retry handler: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	if err := retryHandler.Handle(ctx, event); err != nil {
		t.Errorf("Failed to handle event after retries: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

// TestRetryHandlerNonRetryable tests that non-retryable errors are not retried
func TestRetryHandlerNonRetryable(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	attempts := int32(0)
	handler := newMockEventHandler()
	handler.handleFn = func(ctx context.Context, event types.Event) error {
		atomic.AddInt32(&attempts, 1)
		return types.NewError(types.ErrCodeInvalid, "invalid event")
	}

	retryHandler, err := NewRetryHandler(log, handler, 5, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create retry handler: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	err = retryHandler.Handle(ctx, event)
	if err == nil {
		t.Error("Expected error from handler")
	}

	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("Expected 1 attempt (non-retryable), got %d", attempts)
	}
}

// TestRetryHandlerMaxRetries tests that max retries is respected
func TestRetryHandlerMaxRetries(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	attempts := int32(0)
	handler := newMockEventHandler()
	handler.handleFn = func(ctx context.Context, event types.Event) error {
		atomic.AddInt32(&attempts, 1)
		return types.NewError(types.ErrCodeInternal, "persistent error")
	}

	retryHandler, err := NewRetryHandler(log, handler, 3, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create retry handler: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	err = retryHandler.Handle(ctx, event)
	if err == nil {
		t.Error("Expected error after max retries")
	}

	if atomic.LoadInt32(&attempts) != 4 { // 1 initial + 3 retries
		t.Errorf("Expected 4 attempts (1 + 3 retries), got %d", attempts)
	}
}

// TestTimeoutHandler tests timeout handler
func TestTimeoutHandler(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler := newMockEventHandler()
	timeoutHandler, err := NewTimeoutHandler(log, handler, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create timeout handler: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	if err := timeoutHandler.Handle(ctx, event); err != nil {
		t.Errorf("Failed to handle event: %v", err)
	}

	if handler.getEventCount() != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", handler.getEventCount())
	}
}

// TestTimeoutHandlerTimeout tests timeout handler timeout
func TestTimeoutHandlerTimeout(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler := newMockEventHandler()
	handler.handleFn = func(ctx context.Context, event types.Event) error {
		time.Sleep(2 * time.Second)
		return nil
	}

	timeoutHandler, err := NewTimeoutHandler(log, handler, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create timeout handler: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	err = timeoutHandler.Handle(ctx, event)
	if err == nil {
		t.Error("Expected timeout error")
	}

	if !types.IsErrCode(err, types.ErrCodeTimeout) {
		t.Errorf("Expected ErrCodeTimeout, got %v", types.GetErrorCode(err))
	}
}

// TestLoggingHandler tests logging handler
func TestLoggingHandler(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler := newMockEventHandler()
	loggingHandler, err := NewLoggingHandler(log, handler, "info")
	if err != nil {
		t.Fatalf("Failed to create logging handler: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "test-source",
		Timestamp: types.Timestamp(time.Now()),
	}

	if err := loggingHandler.Handle(ctx, event); err != nil {
		t.Errorf("Failed to handle event: %v", err)
	}

	if handler.getEventCount() != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", handler.getEventCount())
	}
}

// TestLoggingHandlerError tests logging handler with error
func TestLoggingHandlerError(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	expectedErr := errors.New("handler error")
	handler := newMockEventHandler()
	handler.handleFn = func(ctx context.Context, event types.Event) error {
		return expectedErr
	}

	loggingHandler, err := NewLoggingHandler(log, handler, "debug")
	if err != nil {
		t.Fatalf("Failed to create logging handler: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "test-source",
		Timestamp: types.Timestamp(time.Now()),
	}

	err = loggingHandler.Handle(ctx, event)
	if err == nil {
		t.Error("Expected error from handler")
	}

	if err != expectedErr {
		t.Errorf("Expected original error, got %v", err)
	}
}

// TestFilterHandler tests filter handler
func TestFilterHandler(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler := newMockEventHandler()

	// Filter only container.created events
	filter := func(event types.Event) bool {
		return event.Type == types.EventTypeContainerCreated
	}

	filterHandler, err := NewFilterHandler(log, handler, filter)
	if err != nil {
		t.Fatalf("Failed to create filter handler: %v", err)
	}

	// Event that passes filter
	event1 := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	// Event that doesn't pass filter
	event2 := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerStopped,
		Timestamp: types.Timestamp(time.Now()),
	}

	if err := filterHandler.Handle(ctx, event1); err != nil {
		t.Errorf("Failed to handle event1: %v", err)
	}

	if err := filterHandler.Handle(ctx, event2); err != nil {
		t.Errorf("Failed to handle event2: %v", err)
	}

	if handler.getEventCount() != 1 {
		t.Errorf("Expected handler to receive 1 event (filtered), got %d", handler.getEventCount())
	}
}

// TestBufferedHandler tests buffered handler
func TestBufferedHandler(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler := newMockEventHandler()
	bufferedHandler, err := NewBufferedHandler(log, handler, 5, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create buffered handler: %v", err)
	}
	defer bufferedHandler.Close()

	// Add events below batch size
	for i := 0; i < 3; i++ {
		event := types.Event{
			ID:        types.GenerateID(),
			Type:      types.EventTypeContainerCreated,
			Timestamp: types.Timestamp(time.Now()),
		}
		if err := bufferedHandler.Handle(ctx, event); err != nil {
			t.Errorf("Failed to handle event %d: %v", i, err)
		}
	}

	if bufferedHandler.BufferedSize() != 3 {
		t.Errorf("Expected buffer size 3, got %d", bufferedHandler.BufferedSize())
	}

	// Flush manually
	if err := bufferedHandler.Flush(ctx); err != nil {
		t.Errorf("Failed to flush: %v", err)
	}

	if bufferedHandler.BufferedSize() != 0 {
		t.Errorf("Expected buffer size 0 after flush, got %d", bufferedHandler.BufferedSize())
	}

	// Give handler time to process
	time.Sleep(50 * time.Millisecond)

	if handler.getEventCount() != 3 {
		t.Errorf("Expected handler to receive 3 events, got %d", handler.getEventCount())
	}
}

// TestBufferedHandlerAutoFlush tests buffered handler auto-flush on batch size
func TestBufferedHandlerAutoFlush(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler := newMockEventHandler()
	bufferedHandler, err := NewBufferedHandler(log, handler, 3, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create buffered handler: %v", err)
	}
	defer bufferedHandler.Close()

	// Add events to trigger auto-flush
	for i := 0; i < 5; i++ {
		event := types.Event{
			ID:        types.GenerateID(),
			Type:      types.EventTypeContainerCreated,
			Timestamp: types.Timestamp(time.Now()),
		}
		if err := bufferedHandler.Handle(ctx, event); err != nil {
			t.Errorf("Failed to handle event %d: %v", i, err)
		}
	}

	// Give handler time to process
	time.Sleep(100 * time.Millisecond)

	// First 3 should have been auto-flushed
	if handler.getEventCount() < 3 {
		t.Errorf("Expected handler to receive at least 3 events, got %d", handler.getEventCount())
	}
}

// TestBufferedHandlerPeriodicFlush tests buffered handler periodic flush
func TestBufferedHandlerPeriodicFlush(t *testing.T) {
	log, ctx := setupTestHandlerLogger(t)

	handler := newMockEventHandler()
	bufferedHandler, err := NewBufferedHandler(log, handler, 100, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create buffered handler: %v", err)
	}
	defer bufferedHandler.Close()

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	if err := bufferedHandler.Handle(ctx, event); err != nil {
		t.Errorf("Failed to handle event: %v", err)
	}

	// Wait for periodic flush
	time.Sleep(300 * time.Millisecond)

	if handler.getEventCount() != 1 {
		t.Errorf("Expected handler to receive 1 event after periodic flush, got %d", handler.getEventCount())
	}
}

// TestNewAsyncHandlerNilHandler tests async handler with nil handler
func TestNewAsyncHandlerNilHandler(t *testing.T) {
	log, _ := setupTestHandlerLogger(t)

	_, err := NewAsyncHandler(log, nil, 5*time.Second)
	if err == nil {
		t.Error("Expected error when creating async handler with nil handler")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestNewRetryHandlerNilHandler tests retry handler with nil handler
func TestNewRetryHandlerNilHandler(t *testing.T) {
	log, _ := setupTestHandlerLogger(t)

	_, err := NewRetryHandler(log, nil, 3, 1*time.Second)
	if err == nil {
		t.Error("Expected error when creating retry handler with nil handler")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestNewTimeoutHandlerNilHandler tests timeout handler with nil handler
func TestNewTimeoutHandlerNilHandler(t *testing.T) {
	log, _ := setupTestHandlerLogger(t)

	_, err := NewTimeoutHandler(log, nil, 5*time.Second)
	if err == nil {
		t.Error("Expected error when creating timeout handler with nil handler")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestNewLoggingHandlerNilHandler tests logging handler with nil handler
func TestNewLoggingHandlerNilHandler(t *testing.T) {
	log, _ := setupTestHandlerLogger(t)

	_, err := NewLoggingHandler(log, nil, "info")
	if err == nil {
		t.Error("Expected error when creating logging handler with nil handler")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestNewFilterHandlerNilHandler tests filter handler with nil handler
func TestNewFilterHandlerNilHandler(t *testing.T) {
	log, _ := setupTestHandlerLogger(t)

	filter := func(event types.Event) bool { return true }

	_, err := NewFilterHandler(log, nil, filter)
	if err == nil {
		t.Error("Expected error when creating filter handler with nil handler")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestNewFilterHandlerNilFilter tests filter handler with nil filter
func TestNewFilterHandlerNilFilter(t *testing.T) {
	log, _ := setupTestHandlerLogger(t)

	handler := newMockEventHandler()

	_, err := NewFilterHandler(log, handler, nil)
	if err == nil {
		t.Error("Expected error when creating filter handler with nil filter")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestNewBufferedHandlerNilHandler tests buffered handler with nil handler
func TestNewBufferedHandlerNilHandler(t *testing.T) {
	log, _ := setupTestHandlerLogger(t)

	_, err := NewBufferedHandler(log, nil, 10, 1*time.Second)
	if err == nil {
		t.Error("Expected error when creating buffered handler with nil handler")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// BenchmarkHandlerChain benchmarks handler chain execution
func BenchmarkHandlerChain(b *testing.B) {
	log, _ := setupTestHandlerLogger(b)

	handler1 := newMockEventHandler()
	handler2 := newMockEventHandler()
	handler3 := newMockEventHandler()

	chain, err := NewHandlerChain(log, handler1, handler2, handler3)
	if err != nil {
		b.Fatalf("Failed to create handler chain: %v", err)
	}

	ctx := context.Background()
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = chain.Handle(ctx, event)
	}
}

// BenchmarkRetryHandler benchmarks retry handler
func BenchmarkRetryHandler(b *testing.B) {
	log, _ := setupTestHandlerLogger(b)

	handler := newMockEventHandler()
	retryHandler, err := NewRetryHandler(log, handler, 0, 0)
	if err != nil {
		b.Fatalf("Failed to create retry handler: %v", err)
	}

	ctx := context.Background()
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = retryHandler.Handle(ctx, event)
	}
}

// BenchmarkTimeoutHandler benchmarks timeout handler
func BenchmarkTimeoutHandler(b *testing.B) {
	log, _ := setupTestHandlerLogger(b)

	handler := newMockEventHandler()
	timeoutHandler, err := NewTimeoutHandler(log, handler, 30*time.Second)
	if err != nil {
		b.Fatalf("Failed to create timeout handler: %v", err)
	}

	ctx := context.Background()
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = timeoutHandler.Handle(ctx, event)
	}
}
