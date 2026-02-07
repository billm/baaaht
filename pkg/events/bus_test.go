package events

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// mockEventHandler is a test handler that records events
type mockEventHandler struct {
	mu          sync.Mutex
	events      []types.Event
	callCount   int32
	canHandle   bool
	canHandleFn func(types.EventType) bool
	handleFn    func(context.Context, types.Event) error
}

func newMockEventHandler() *mockEventHandler {
	return &mockEventHandler{
		events:    make([]types.Event, 0),
		canHandle: true,
		handleFn:  nil,
	}
}

func (m *mockEventHandler) Handle(ctx context.Context, event types.Event) error {
	atomic.AddInt32(&m.callCount, 1)
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()

	if m.handleFn != nil {
		return m.handleFn(ctx, event)
	}
	return nil
}

func (m *mockEventHandler) CanHandle(eventType types.EventType) bool {
	if m.canHandleFn != nil {
		return m.canHandleFn(eventType)
	}
	return m.canHandle
}

func (m *mockEventHandler) getEventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockEventHandler) getEvents() []types.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]types.Event{}, m.events...)
}

func (m *mockEventHandler) reset() {
	m.mu.Lock()
	m.events = make([]types.Event, 0)
	atomic.StoreInt32(&m.callCount, 0)
	m.mu.Unlock()
}

// TestNewEventBus tests creating a new event bus
func TestNewEventBus(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	bus, err := New(log)
	if err != nil {
		t.Fatalf("Failed to create event bus: %v", err)
	}

	if bus == nil {
		t.Fatal("Expected non-nil bus")
	}

	if bus.closed {
		t.Error("Expected bus to be open")
	}

	if err := bus.Close(); err != nil {
		t.Errorf("Failed to close bus: %v", err)
	}
}

// TestNewEventBusWithNilLogger tests creating a bus with nil logger
func TestNewEventBusWithNilLogger(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("Failed to create event bus with nil logger: %v", err)
	}

	if bus == nil {
		t.Fatal("Expected non-nil bus")
	}

	if err := bus.Close(); err != nil {
		t.Errorf("Failed to close bus: %v", err)
	}
}

// TestSubscribe tests subscribing to events
func TestSubscribe(t *testing.T) {
	bus, log := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe to all container events
	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}

	subID, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	if subID == "" {
		t.Error("Expected non-empty subscription ID")
	}

	// Verify subscription exists
	stats := bus.Stats()
	if stats.TotalSubscriptions != 1 {
		t.Errorf("Expected 1 subscription, got %d", stats.TotalSubscriptions)
	}
	if stats.ActiveSubscriptions != 1 {
		t.Errorf("Expected 1 active subscription, got %d", stats.ActiveSubscriptions)
	}

	log.Debug("Subscribe test passed")
}

// TestSubscribeWithTopic tests subscribing with a topic
func TestSubscribeWithTopic(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe to container topic
	subID, err := bus.SubscribeWithTopic(ctx, types.TopicContainer, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe with topic: %v", err)
	}

	if subID == "" {
		t.Error("Expected non-empty subscription ID")
	}

	stats := bus.Stats()
	if stats.TotalSubscriptions != 1 {
		t.Errorf("Expected 1 subscription, got %d", stats.TotalSubscriptions)
	}
}

// TestSubscribeAllTopic tests subscribing to all events
func TestSubscribeAllTopic(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe to all events
	subID, err := bus.SubscribeWithTopic(ctx, types.TopicAll, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe to all topics: %v", err)
	}

	if subID == "" {
		t.Error("Expected non-empty subscription ID")
	}

	// Publish different event types
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	event1 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}
	event2 := types.Event{
		Type:   types.EventTypeSessionCreated,
		Source: "test",
	}

	if err := bus.Publish(ctx, event1); err != nil {
		t.Fatalf("Failed to publish event1: %v", err)
	}
	if err := bus.Publish(ctx, event2); err != nil {
		t.Fatalf("Failed to publish event2: %v", err)
	}

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	count := handler.getEventCount()
	if count != 2 {
		t.Errorf("Expected handler to receive 2 events, got %d", count)
	}
}

// TestUnsubscribe tests unsubscribing from events
func TestUnsubscribe(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe
	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}
	subID, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Unsubscribe
	if err := bus.Unsubscribe(subID); err != nil {
		t.Fatalf("Failed to unsubscribe: %v", err)
	}

	// Verify subscription removed
	stats := bus.Stats()
	if stats.TotalSubscriptions != 0 {
		t.Errorf("Expected 0 subscriptions after unsubscribe, got %d", stats.TotalSubscriptions)
	}

	// Try to unsubscribe again (should fail)
	if err := bus.Unsubscribe(subID); err == nil {
		t.Error("Expected error when unsubscribing non-existent subscription")
	}
}

// TestPublish tests publishing events
func TestPublish(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe
	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}
	_, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish event
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
		Data:   map[string]interface{}{"container_id": "abc123"},
	}

	if err := bus.Publish(ctx, event); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}

	events := handler.getEvents()
	if events[0].Type != types.EventTypeContainerCreated {
		t.Errorf("Expected event type %s, got %s", types.EventTypeContainerCreated, events[0].Type)
	}
}

// TestPublishSync tests synchronous publishing
func TestPublishSync(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe
	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}
	_, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish synchronously
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	if err := bus.PublishSync(ctx, event); err != nil {
		t.Fatalf("Failed to publish sync: %v", err)
	}

	// Event should be delivered immediately
	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event immediately, got %d", count)
	}
}

// TestPublishMultipleHandlers tests publishing to multiple handlers
func TestPublishMultipleHandlers(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler1 := newMockEventHandler()
	handler2 := newMockEventHandler()
	handler3 := newMockEventHandler()

	// Subscribe all handlers
	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}

	_, err := bus.Subscribe(ctx, filter, handler1)
	if err != nil {
		t.Fatalf("Failed to subscribe handler1: %v", err)
	}
	_, err = bus.Subscribe(ctx, filter, handler2)
	if err != nil {
		t.Fatalf("Failed to subscribe handler2: %v", err)
	}
	_, err = bus.Subscribe(ctx, filter, handler3)
	if err != nil {
		t.Fatalf("Failed to subscribe handler3: %v", err)
	}

	// Publish event
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	if err := bus.PublishSync(ctx, event); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// All handlers should receive the event
	if handler1.getEventCount() != 1 {
		t.Errorf("Expected handler1 to receive 1 event, got %d", handler1.getEventCount())
	}
	if handler2.getEventCount() != 1 {
		t.Errorf("Expected handler2 to receive 1 event, got %d", handler2.getEventCount())
	}
	if handler3.getEventCount() != 1 {
		t.Errorf("Expected handler3 to receive 1 event, got %d", handler3.getEventCount())
	}
}

// TestEventFilter tests event filtering
func TestEventFilter(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()

	handler1 := newMockEventHandler()
	handler2 := newMockEventHandler()

	// Handler1 subscribes to container.created
	type1 := types.EventTypeContainerCreated
	filter1 := types.EventFilter{Type: &type1}
	_, err := bus.Subscribe(ctx, filter1, handler1)
	if err != nil {
		t.Fatalf("Failed to subscribe handler1: %v", err)
	}

	// Handler2 subscribes to container.stopped
	type2 := types.EventTypeContainerStopped
	filter2 := types.EventFilter{Type: &type2}
	_, err = bus.Subscribe(ctx, filter2, handler2)
	if err != nil {
		t.Fatalf("Failed to subscribe handler2: %v", err)
	}

	// Publish container.created event
	event1 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}
	if err := bus.PublishSync(ctx, event1); err != nil {
		t.Fatalf("Failed to publish event1: %v", err)
	}

	// Publish container.stopped event
	event2 := types.Event{
		Type:   types.EventTypeContainerStopped,
		Source: "test",
	}
	if err := bus.PublishSync(ctx, event2); err != nil {
		t.Fatalf("Failed to publish event2: %v", err)
	}

	// Handler1 should only receive container.created
	if handler1.getEventCount() != 1 {
		t.Errorf("Expected handler1 to receive 1 event, got %d", handler1.getEventCount())
	}
	events1 := handler1.getEvents()
	if events1[0].Type != types.EventTypeContainerCreated {
		t.Errorf("Expected handler1 to receive container.created, got %s", events1[0].Type)
	}

	// Handler2 should only receive container.stopped
	if handler2.getEventCount() != 1 {
		t.Errorf("Expected handler2 to receive 1 event, got %d", handler2.getEventCount())
	}
	events2 := handler2.getEvents()
	if events2[0].Type != types.EventTypeContainerStopped {
		t.Errorf("Expected handler2 to receive container.stopped, got %s", events2[0].Type)
	}
}

// TestSessionIDFilter tests filtering by session ID
func TestSessionIDFilter(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe to events for specific session
	sessionID := types.GenerateID()
	filter := types.EventFilter{SessionID: &sessionID}

	_, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish event with matching session ID
	event1 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
		Metadata: types.EventMetadata{
			SessionID: &sessionID,
		},
	}

	// Publish event with different session ID
	otherSessionID := types.GenerateID()
	event2 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
		Metadata: types.EventMetadata{
			SessionID: &otherSessionID,
		},
	}

	if err := bus.PublishSync(ctx, event1); err != nil {
		t.Fatalf("Failed to publish event1: %v", err)
	}
	if err := bus.PublishSync(ctx, event2); err != nil {
		t.Fatalf("Failed to publish event2: %v", err)
	}

	// Handler should only receive event with matching session ID
	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}
}

// TestLabelFilter tests filtering by labels
func TestLabelFilter(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe to events with specific label
	filter := types.EventFilter{
		Labels: map[string]string{"environment": "production"},
	}

	_, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish event with matching labels
	event1 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
		Metadata: types.EventMetadata{
			Labels: map[string]string{"environment": "production", "region": "us-east"},
		},
	}

	// Publish event without matching labels
	event2 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
		Metadata: types.EventMetadata{
			Labels: map[string]string{"environment": "staging"},
		},
	}

	if err := bus.PublishSync(ctx, event1); err != nil {
		t.Fatalf("Failed to publish event1: %v", err)
	}
	if err := bus.PublishSync(ctx, event2); err != nil {
		t.Fatalf("Failed to publish event2: %v", err)
	}

	// Handler should only receive event with matching label
	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}
}

// TestPriorityFilter tests filtering by priority
func TestPriorityFilter(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe to high priority events
	priority := types.PriorityHigh
	filter := types.EventFilter{Priority: &priority}

	_, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish high priority event
	event1 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
		Metadata: types.EventMetadata{
			Priority: types.PriorityHigh,
		},
	}

	// Publish normal priority event
	event2 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
		Metadata: types.EventMetadata{
			Priority: types.PriorityNormal,
		},
	}

	if err := bus.PublishSync(ctx, event1); err != nil {
		t.Fatalf("Failed to publish event1: %v", err)
	}
	if err := bus.PublishSync(ctx, event2); err != nil {
		t.Fatalf("Failed to publish event2: %v", err)
	}

	// Handler should only receive high priority event
	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}
}

// TestSourceFilter tests filtering by source
func TestSourceFilter(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Subscribe to events from specific source
	source := "orchestrator"
	filter := types.EventFilter{Source: &source}

	_, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish event from orchestrator
	event1 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "orchestrator",
	}

	// Publish event from different source
	event2 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "agent",
	}

	if err := bus.PublishSync(ctx, event1); err != nil {
		t.Fatalf("Failed to publish event1: %v", err)
	}
	if err := bus.PublishSync(ctx, event2); err != nil {
		t.Fatalf("Failed to publish event2: %v", err)
	}

	// Handler should only receive event from orchestrator
	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}
}

// TestCanHandleFilter tests the CanHandle filter
func TestCanHandleFilter(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	// Make handler only handle container events
	handler.canHandleFn = func(eventType types.EventType) bool {
		return eventType == types.EventTypeContainerCreated
	}

	// Subscribe without type filter
	filter := types.EventFilter{}
	_, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish container event
	event1 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	// Publish session event
	event2 := types.Event{
		Type:   types.EventTypeSessionCreated,
		Source: "test",
	}

	if err := bus.PublishSync(ctx, event1); err != nil {
		t.Fatalf("Failed to publish event1: %v", err)
	}
	if err := bus.PublishSync(ctx, event2); err != nil {
		t.Fatalf("Failed to publish event2: %v", err)
	}

	// Handler should only handle container event due to CanHandle
	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}
}

// TestHandlerError tests error handling in handlers
func TestHandlerError(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()

	// Handler that returns an error
	handler := newMockEventHandler()
	handler.handleFn = func(ctx context.Context, event types.Event) error {
		return fmt.Errorf("handler error")
	}

	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}

	_, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish event - should not fail even if handler fails
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	// PublishSync returns error if handler fails
	err = bus.PublishSync(ctx, event)
	if err == nil {
		t.Error("Expected error from failed handler")
	}

	// Event should still be delivered
	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event despite error, got %d", count)
	}
}

// TestConcurrentPublish tests concurrent publishing
func TestConcurrentPublish(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}

	_, err := bus.Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish concurrently
	const numGoroutines = 100
	const eventsPerGoroutine = 10

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := types.Event{
					Type:   types.EventTypeContainerCreated,
					Source: fmt.Sprintf("goroutine-%d", id),
					Data:   map[string]interface{}{"index": j},
				}
				if err := bus.Publish(ctx, event); err != nil {
					t.Errorf("Failed to publish: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Wait for async processing
	time.Sleep(500 * time.Millisecond)

	// Verify all events were received
	expectedCount := numGoroutines * eventsPerGoroutine
	count := handler.getEventCount()
	if count != expectedCount {
		t.Errorf("Expected %d events, got %d", expectedCount, count)
	}
}

// TestClosedBus tests operations on a closed bus
func TestClosedBus(t *testing.T) {
	bus, _ := setupTestBus(t)

	// Close the bus
	if err := bus.Close(); err != nil {
		t.Fatalf("Failed to close bus: %v", err)
	}

	ctx := context.Background()
	handler := newMockEventHandler()

	// Try to subscribe on closed bus
	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}
	_, err := bus.Subscribe(ctx, filter, handler)
	if err == nil {
		t.Error("Expected error when subscribing to closed bus")
	}

	// Try to publish on closed bus
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}
	err = bus.Publish(ctx, event)
	if err == nil {
		t.Error("Expected error when publishing to closed bus")
	}

	// Close again should fail
	if err := bus.Close(); err == nil {
		t.Error("Expected error when closing already closed bus")
	}
}

// TestStats tests bus statistics
func TestStats(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()

	// Initial stats
	stats := bus.Stats()
	if stats.TotalSubscriptions != 0 {
		t.Errorf("Expected 0 total subscriptions, got %d", stats.TotalSubscriptions)
	}

	// Add subscriptions
	handler1 := newMockEventHandler()
	handler2 := newMockEventHandler()

	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}

	bus.Subscribe(ctx, filter, handler1)
	bus.Subscribe(ctx, filter, handler2)

	stats = bus.Stats()
	if stats.TotalSubscriptions != 2 {
		t.Errorf("Expected 2 total subscriptions, got %d", stats.TotalSubscriptions)
	}
	if stats.ActiveSubscriptions != 2 {
		t.Errorf("Expected 2 active subscriptions, got %d", stats.ActiveSubscriptions)
	}
}

// TestEventIDGeneration tests automatic event ID generation
func TestEventIDGeneration(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}
	bus.Subscribe(ctx, filter, handler)

	// Publish event without ID
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	if err := bus.PublishSync(ctx, event); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Check that ID was generated
	events := handler.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].ID == "" {
		t.Error("Expected event ID to be generated")
	}
}

// TestEventTimestampGeneration tests automatic timestamp generation
func TestEventTimestampGeneration(t *testing.T) {
	bus, _ := setupTestBus(t)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}
	bus.Subscribe(ctx, filter, handler)

	// Publish event without timestamp
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	if err := bus.PublishSync(ctx, event); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Check that timestamp was generated
	events := handler.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Timestamp.IsZero() {
		t.Error("Expected timestamp to be generated")
	}
}

// TestGlobalBus tests global bus functions
func TestGlobalBus(t *testing.T) {
	// Reset global bus
	globalBus = nil
	globalBusOnce = sync.Once{}

	ctx := context.Background()
	handler := newMockEventHandler()

	// Get global bus (should create one)
	bus := Global()
	if bus == nil {
		t.Fatal("Expected non-nil global bus")
	}

	// Subscribe using global function
	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}
	subID, err := Subscribe(ctx, filter, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe using global function: %v", err)
	}

	if subID == "" {
		t.Error("Expected non-empty subscription ID")
	}

	// Publish using global function
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	if err := Publish(ctx, event); err != nil {
		t.Fatalf("Failed to publish using global function: %v", err)
	}

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}

	// Clean up
	bus.Close()
	globalBus = nil
	globalBusOnce = sync.Once{}
}

// BenchmarkPublish benchmarks event publishing
func BenchmarkPublish(b *testing.B) {
	bus, _ := setupTestBus(b)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}
	bus.Subscribe(ctx, filter, handler)

	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := bus.Publish(ctx, event); err != nil {
			b.Fatalf("Failed to publish: %v", err)
		}
	}

	// Wait for all events to be processed
	bus.Close()
}

// BenchmarkPublishSync benchmarks synchronous event publishing
func BenchmarkPublishSync(b *testing.B) {
	bus, _ := setupTestBus(b)
	defer bus.Close()

	ctx := context.Background()
	handler := newMockEventHandler()

	eventType := types.EventTypeContainerCreated
	filter := types.EventFilter{Type: &eventType}
	bus.Subscribe(ctx, filter, handler)

	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := bus.PublishSync(ctx, event); err != nil {
			b.Fatalf("Failed to publish sync: %v", err)
		}
	}
}

// Helper functions

func setupTestBus(t testing.TB) (*Bus, *logger.Logger) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	bus, err := New(log)
	if err != nil {
		t.Fatalf("Failed to create event bus: %v", err)
	}

	return bus, log
}
