package events

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// mockRouteHandler is a test handler that records events
type mockRouteHandler struct {
	mu          sync.Mutex
	events      []types.Event
	callCount   int32
	canHandle   bool
	canHandleFn func(types.EventType) bool
	handleFn    func(context.Context, types.Event) error
}

func newMockRouteHandler() *mockRouteHandler {
	return &mockRouteHandler{
		events:    make([]types.Event, 0),
		canHandle: true,
		handleFn:  nil,
	}
}

func (m *mockRouteHandler) Handle(ctx context.Context, event types.Event) error {
	atomic.AddInt32(&m.callCount, 1)
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()

	if m.handleFn != nil {
		return m.handleFn(ctx, event)
	}
	return nil
}

func (m *mockRouteHandler) CanHandle(eventType types.EventType) bool {
	if m.canHandleFn != nil {
		return m.canHandleFn(eventType)
	}
	return m.canHandle
}

func (m *mockRouteHandler) getEventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockRouteHandler) getEvents() []types.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]types.Event{}, m.events...)
}

func (m *mockRouteHandler) reset() {
	m.mu.Lock()
	m.events = make([]types.Event, 0)
	atomic.StoreInt32(&m.callCount, 0)
	m.mu.Unlock()
}

// TestNewRouter tests creating a new router
func TestNewRouter(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	router, err := NewRouter(log)
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	if router == nil {
		t.Fatal("Expected non-nil router")
	}

	if router.closed {
		t.Error("Expected router to be open")
	}

	stats := router.Stats()
	if stats.TotalRoutes != 0 {
		t.Errorf("Expected 0 total routes, got %d", stats.TotalRoutes)
	}

	if err := router.Close(); err != nil {
		t.Errorf("Failed to close router: %v", err)
	}
}

// TestNewRouterWithNilLogger tests creating a router with nil logger
func TestNewRouterWithNilLogger(t *testing.T) {
	router, err := NewRouter(nil)
	if err != nil {
		t.Fatalf("Failed to create router with nil logger: %v", err)
	}

	if router == nil {
		t.Fatal("Expected non-nil router")
	}

	if err := router.Close(); err != nil {
		t.Errorf("Failed to close router: %v", err)
	}
}

// TestAddRoute tests adding routes
func TestAddRoute(t *testing.T) {
	router, log := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add exact route
	routeID, err := router.AddRoute(ctx, "container.created", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	if routeID == "" {
		t.Error("Expected non-empty route ID")
	}

	stats := router.Stats()
	if stats.TotalRoutes != 1 {
		t.Errorf("Expected 1 total route, got %d", stats.TotalRoutes)
	}
	if stats.ExactRoutes != 1 {
		t.Errorf("Expected 1 exact route, got %d", stats.ExactRoutes)
	}

	log.Debug("AddRoute test passed")
}

// TestAddRouteWithWildcard tests adding wildcard routes
func TestAddRouteWithWildcard(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add wildcard route
	routeID, err := router.AddRoute(ctx, "container.*", handler)
	if err != nil {
		t.Fatalf("Failed to add wildcard route: %v", err)
	}

	if routeID == "" {
		t.Error("Expected non-empty route ID")
	}

	stats := router.Stats()
	if stats.TotalRoutes != 1 {
		t.Errorf("Expected 1 total route, got %d", stats.TotalRoutes)
	}
	if stats.WildcardRoutes != 1 {
		t.Errorf("Expected 1 wildcard route, got %d", stats.WildcardRoutes)
	}
}

// TestAddRouteWithCatchAll tests adding catch-all wildcard route
func TestAddRouteWithCatchAll(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add catch-all route
	routeID, err := router.AddRoute(ctx, "*", handler)
	if err != nil {
		t.Fatalf("Failed to add catch-all route: %v", err)
	}

	if routeID == "" {
		t.Error("Expected non-empty route ID")
	}

	stats := router.Stats()
	if stats.WildcardRoutes != 1 {
		t.Errorf("Expected 1 wildcard route, got %d", stats.WildcardRoutes)
	}
}

// TestAddRouteWithPriority tests adding routes with priority
func TestAddRouteWithPriority(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler1 := newMockRouteHandler()
	handler2 := newMockRouteHandler()

	// Add routes with different priorities
	_, err := router.AddRouteWithPriority(ctx, "container.created", handler1, 10)
	if err != nil {
		t.Fatalf("Failed to add route with priority: %v", err)
	}

	_, err = router.AddRouteWithPriority(ctx, "container.created", handler2, 20)
	if err != nil {
		t.Fatalf("Failed to add route with priority: %v", err)
	}

	stats := router.Stats()
	if stats.TotalRoutes != 1 {
		// Same pattern should replace
		t.Errorf("Expected 1 total route (same pattern), got %d", stats.TotalRoutes)
	}
}

// TestAddRouteInvalidPattern tests adding routes with invalid patterns
func TestAddRouteInvalidPattern(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Test empty pattern
	_, err := router.AddRoute(ctx, "", handler)
	if err == nil {
		t.Error("Expected error for empty pattern")
	}

	// Test invalid wildcard position
	_, err = router.AddRoute(ctx, "container.*.created", handler)
	if err == nil {
		t.Error("Expected error for invalid wildcard position")
	}

	// Test empty segment
	_, err = router.AddRoute(ctx, "container..created", handler)
	if err == nil {
		t.Error("Expected error for empty segment")
	}
}

// TestAddRouteNilHandler tests adding route with nil handler
func TestAddRouteNilHandler(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()

	_, err := router.AddRoute(ctx, "container.created", nil)
	if err == nil {
		t.Error("Expected error for nil handler")
	}
}

// TestRemoveRoute tests removing routes
func TestRemoveRoute(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add route
	routeID, err := router.AddRoute(ctx, "container.created", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Remove route
	if err := router.RemoveRoute(routeID); err != nil {
		t.Fatalf("Failed to remove route: %v", err)
	}

	stats := router.Stats()
	if stats.TotalRoutes != 0 {
		t.Errorf("Expected 0 total routes after removal, got %d", stats.TotalRoutes)
	}

	// Try to remove again (should fail)
	if err := router.RemoveRoute(routeID); err == nil {
		t.Error("Expected error when removing non-existent route")
	}
}

// TestRouteExactMatch tests routing with exact pattern match
func TestRouteExactMatch(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add exact route
	_, err := router.AddRoute(ctx, "container.created", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Route event
	event := types.Event{
		ID:     types.GenerateID(),
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	if err := router.Route(ctx, event); err != nil {
		t.Fatalf("Failed to route event: %v", err)
	}

	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}

	events := handler.getEvents()
	if events[0].Type != types.EventTypeContainerCreated {
		t.Errorf("Expected event type %s, got %s", types.EventTypeContainerCreated, events[0].Type)
	}
}

// TestRouteWildcard tests routing with wildcard patterns
func TestRouteWildcard(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add wildcard route for all container events
	_, err := router.AddRoute(ctx, "container.*", handler)
	if err != nil {
		t.Fatalf("Failed to add wildcard route: %v", err)
	}

	// Route multiple container events
	events := []types.Event{
		{Type: types.EventTypeContainerCreated, Source: "test"},
		{Type: types.EventTypeContainerStarted, Source: "test"},
		{Type: types.EventTypeContainerStopped, Source: "test"},
	}

	for _, event := range events {
		if err := router.Route(ctx, event); err != nil {
			t.Fatalf("Failed to route event: %v", err)
		}
	}

	count := handler.getEventCount()
	if count != 3 {
		t.Errorf("Expected handler to receive 3 events, got %d", count)
	}
}

// TestRouteCatchAll tests routing with catch-all pattern
func TestRouteCatchAll(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add catch-all route
	_, err := router.AddRoute(ctx, "*", handler)
	if err != nil {
		t.Fatalf("Failed to add catch-all route: %v", err)
	}

	// Route various event types
	events := []types.Event{
		{Type: types.EventTypeContainerCreated, Source: "test"},
		{Type: types.EventTypeSessionCreated, Source: "test"},
		{Type: types.EventTypeTaskCreated, Source: "test"},
	}

	for _, event := range events {
		if err := router.Route(ctx, event); err != nil {
			t.Fatalf("Failed to route event: %v", err)
		}
	}

	count := handler.getEventCount()
	if count != 3 {
		t.Errorf("Expected handler to receive 3 events, got %d", count)
	}
}

// TestRouteNoMatch tests routing when no handlers match
func TestRouteNoMatch(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add route for container events only
	_, err := router.AddRoute(ctx, "container.*", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Route session event (should not match)
	event := types.Event{
		Type:   types.EventTypeSessionCreated,
		Source: "test",
	}

	if err := router.Route(ctx, event); err != nil {
		t.Fatalf("Failed to route event: %v", err)
	}

	count := handler.getEventCount()
	if count != 0 {
		t.Errorf("Expected handler to receive 0 events, got %d", count)
	}
}

// TestRouteMultipleHandlers tests routing to multiple handlers
func TestRouteMultipleHandlers(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler1 := newMockRouteHandler()
	handler2 := newMockRouteHandler()

	// Add two routes with same pattern (note: they won't both match since exact routes are unique)
	// Let's add different patterns instead
	_, err := router.AddRoute(ctx, "container.created", handler1)
	if err != nil {
		t.Fatalf("Failed to add route1: %v", err)
	}

	_, err = router.AddRoute(ctx, "container.*", handler2)
	if err != nil {
		t.Fatalf("Failed to add route2: %v", err)
	}

	// Route event
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	if err := router.Route(ctx, event); err != nil {
		t.Fatalf("Failed to route event: %v", err)
	}

	// Both handlers should receive the event
	if handler1.getEventCount() != 1 {
		t.Errorf("Expected handler1 to receive 1 event, got %d", handler1.getEventCount())
	}
	if handler2.getEventCount() != 1 {
		t.Errorf("Expected handler2 to receive 1 event, got %d", handler2.getEventCount())
	}
}

// TestSetRouteActive tests activating/deactivating routes
func TestSetRouteActive(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add route
	routeID, err := router.AddRoute(ctx, "container.created", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Deactivate route
	if err := router.SetRouteActive(routeID, false); err != nil {
		t.Fatalf("Failed to deactivate route: %v", err)
	}

	// Route event
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	if err := router.Route(ctx, event); err != nil {
		t.Fatalf("Failed to route event: %v", err)
	}

	// Handler should not receive event
	count := handler.getEventCount()
	if count != 0 {
		t.Errorf("Expected handler to receive 0 events when deactivated, got %d", count)
	}

	// Activate route
	if err := router.SetRouteActive(routeID, true); err != nil {
		t.Fatalf("Failed to activate route: %v", err)
	}

	// Route event again
	if err := router.Route(ctx, event); err != nil {
		t.Fatalf("Failed to route event: %v", err)
	}

	// Handler should receive event now
	count = handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event when activated, got %d", count)
	}
}

// TestSetRouteMetadata tests setting route metadata
func TestSetRouteMetadata(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add route
	routeID, err := router.AddRoute(ctx, "container.created", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Set metadata
	metadata := map[string]string{
		"description": "Container creation events",
		"owner":       "orchestrator",
	}

	if err := router.SetRouteMetadata(routeID, metadata); err != nil {
		t.Fatalf("Failed to set metadata: %v", err)
	}

	// Get route info
	info, err := router.GetRoute(routeID)
	if err != nil {
		t.Fatalf("Failed to get route: %v", err)
	}

	if info.Metadata["description"] != "Container creation events" {
		t.Errorf("Expected metadata description 'Container creation events', got %s", info.Metadata["description"])
	}
	if info.Metadata["owner"] != "orchestrator" {
		t.Errorf("Expected metadata owner 'orchestrator', got %s", info.Metadata["owner"])
	}
}

// TestGetRoute tests retrieving route information
func TestGetRoute(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add route
	routeID, err := router.AddRouteWithPriority(ctx, "container.created", handler, 10)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Get route
	info, err := router.GetRoute(routeID)
	if err != nil {
		t.Fatalf("Failed to get route: %v", err)
	}

	if info.ID != routeID {
		t.Errorf("Expected route ID %s, got %s", routeID, info.ID)
	}
	if info.Pattern != "container.created" {
		t.Errorf("Expected pattern 'container.created', got %s", info.Pattern)
	}
	if !info.Active {
		t.Error("Expected route to be active")
	}
	if info.Priority != 10 {
		t.Errorf("Expected priority 10, got %d", info.Priority)
	}
}

// TestGetRouteNotFound tests getting non-existent route
func TestGetRouteNotFound(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	_, err := router.GetRoute("non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent route")
	}
}

// TestListRoutes tests listing all routes
func TestListRoutes(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler1 := newMockRouteHandler()
	handler2 := newMockRouteHandler()

	// Add routes
	router.AddRoute(ctx, "container.created", handler1)
	router.AddRoute(ctx, "session.*", handler2)

	// List routes
	routes := router.ListRoutes()

	if len(routes) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(routes))
	}
}

// TestRouterStats tests router statistics
func TestRouterStats(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler1 := newMockRouteHandler()
	handler2 := newMockRouteHandler()

	// Initial stats
	stats := router.Stats()
	if stats.TotalRoutes != 0 {
		t.Errorf("Expected 0 total routes, got %d", stats.TotalRoutes)
	}

	// Add routes
	router.AddRoute(ctx, "container.created", handler1)
	router.AddRoute(ctx, "session.*", handler2)

	stats = router.Stats()
	if stats.TotalRoutes != 2 {
		t.Errorf("Expected 2 total routes, got %d", stats.TotalRoutes)
	}
	if stats.ExactRoutes != 1 {
		t.Errorf("Expected 1 exact route, got %d", stats.ExactRoutes)
	}
	if stats.WildcardRoutes != 1 {
		t.Errorf("Expected 1 wildcard route, got %d", stats.WildcardRoutes)
	}
	if stats.ActiveRoutes != 2 {
		t.Errorf("Expected 2 active routes, got %d", stats.ActiveRoutes)
	}

	// Deactivate one route
	routeID, _ := router.AddRoute(ctx, "task.created", handler1)
	router.SetRouteActive(routeID, false)

	stats = router.Stats()
	if stats.InactiveRoutes != 1 {
		t.Errorf("Expected 1 inactive route, got %d", stats.InactiveRoutes)
	}
}

// TestRouterCanHandleFilter tests the CanHandle filter in routing
func TestRouterCanHandleFilter(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Make handler only handle container events
	handler.canHandleFn = func(eventType types.EventType) bool {
		return eventType == types.EventTypeContainerCreated
	}

	// Add catch-all route
	_, err := router.AddRoute(ctx, "*", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Route container event
	event1 := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	// Route session event
	event2 := types.Event{
		Type:   types.EventTypeSessionCreated,
		Source: "test",
	}

	if err := router.Route(ctx, event1); err != nil {
		t.Fatalf("Failed to route event1: %v", err)
	}
	if err := router.Route(ctx, event2); err != nil {
		t.Fatalf("Failed to route event2: %v", err)
	}

	// Handler should only handle container event due to CanHandle
	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}
}

// TestRouterHandlerError tests error handling in route handlers
func TestRouterHandlerError(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()

	// Handler that returns an error
	handler := newMockRouteHandler()
	handler.handleFn = func(ctx context.Context, event types.Event) error {
		return fmt.Errorf("handler error")
	}

	_, err := router.AddRoute(ctx, "container.created", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Route event - should return error
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	err = router.Route(ctx, event)
	if err == nil {
		t.Error("Expected error from failed handler")
	}

	// Event should still be delivered
	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event despite error, got %d", count)
	}
}

// TestClosedRouter tests operations on a closed router
func TestClosedRouter(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Close the router
	if err := router.Close(); err != nil {
		t.Fatalf("Failed to close router: %v", err)
	}

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Try to add route on closed router
	_, err := router.AddRoute(ctx, "container.created", handler)
	if err == nil {
		t.Error("Expected error when adding route to closed router")
	}

	// Try to route on closed router
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}
	err = router.Route(ctx, event)
	if err == nil {
		t.Error("Expected error when routing on closed router")
	}

	// Close again should fail
	if err := router.Close(); err == nil {
		t.Error("Expected error when closing already closed router")
	}
}

// TestConcurrentRouting tests concurrent routing operations
func TestConcurrentRouting(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add wildcard route
	_, err := router.AddRoute(ctx, "container.*", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Route concurrently
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
				if err := router.Route(ctx, event); err != nil {
					t.Errorf("Failed to route event: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all events were received
	expectedCount := numGoroutines * eventsPerGoroutine
	count := handler.getEventCount()
	if count != expectedCount {
		t.Errorf("Expected %d events, got %d", expectedCount, count)
	}
}

// TestGlobalRouter tests global router functions
func TestGlobalRouter(t *testing.T) {
	// Reset global router
	globalRouter = nil
	globalRouterOnce = sync.Once{}

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Get global router (should create one)
	router := GlobalRouter()
	if router == nil {
		t.Fatal("Expected non-nil global router")
	}

	// Add route using global function
	routeID, err := AddRouteToGlobal(ctx, "container.created", handler)
	if err != nil {
		t.Fatalf("Failed to add route using global function: %v", err)
	}

	if routeID == "" {
		t.Error("Expected non-empty route ID")
	}

	// Route using global function
	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	if err := RouteEvent(ctx, event); err != nil {
		t.Fatalf("Failed to route using global function: %v", err)
	}

	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}

	// Clean up
	router.Close()
	globalRouter = nil
	globalRouterOnce = sync.Once{}
}

// TestTopicHierarchy tests hierarchical topic matching
func TestTopicHierarchy(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	// Add route for nested topic
	_, err := router.AddRoute(ctx, "container.lifecycle.created", handler)
	if err != nil {
		t.Fatalf("Failed to add route: %v", err)
	}

	// Route matching event
	event := types.Event{
		Type:   "container.lifecycle.created",
		Source: "test",
	}

	if err := router.Route(ctx, event); err != nil {
		t.Fatalf("Failed to route event: %v", err)
	}

	count := handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to receive 1 event, got %d", count)
	}

	// Route non-matching event (different level)
	event2 := types.Event{
		Type:   "container.created",
		Source: "test",
	}

	if err := router.Route(ctx, event2); err != nil {
		t.Fatalf("Failed to route event: %v", err)
	}

	// Handler should still have only 1 event
	count = handler.getEventCount()
	if count != 1 {
		t.Errorf("Expected handler to still have 1 event, got %d", count)
	}
}

// BenchmarkRoute benchmarks event routing
func BenchmarkRoute(b *testing.B) {
	router, _ := setupTestRouter(b)
	defer router.Close()

	ctx := context.Background()
	handler := newMockRouteHandler()

	router.AddRoute(ctx, "container.*", handler)

	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := router.Route(ctx, event); err != nil {
			b.Fatalf("Failed to route: %v", err)
		}
	}
}

// BenchmarkRouteWithMultipleHandlers benchmarks routing with multiple handlers
func BenchmarkRouteWithMultipleHandlers(b *testing.B) {
	router, _ := setupTestRouter(b)
	defer router.Close()

	ctx := context.Background()

	// Add multiple handlers
	for i := 0; i < 10; i++ {
		handler := newMockRouteHandler()
		router.AddRoute(ctx, "container.*", handler)
	}

	event := types.Event{
		Type:   types.EventTypeContainerCreated,
		Source: "test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := router.Route(ctx, event); err != nil {
			b.Fatalf("Failed to route: %v", err)
		}
	}
}

// Helper functions

func setupTestRouter(t testing.TB) (*Router, *logger.Logger) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	router, err := NewRouter(log)
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	return router, log
}
