package events

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Router implements topic-based event routing
// Topics use hierarchical patterns like "container.created" or "container.*"
type Router struct {
	mu         sync.RWMutex
	routes     map[string]*routeEntry        // topic pattern -> route entry
	wildcards  map[string][]*routeEntry       // prefix -> entries with wildcards
	logger     *logger.Logger
	closed     bool
	nextRouteID types.ID
}

// routeEntry represents a single route registration
type routeEntry struct {
	ID         types.ID
	Pattern    string
	Handler    types.EventHandler
	Active     bool
	Metadata   map[string]string
	Priority   int
}

// NewRouter creates a new event router
func NewRouter(log *logger.Logger) (*Router, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	r := &Router{
		routes:    make(map[string]*routeEntry),
		wildcards: make(map[string][]*routeEntry),
		logger:    log.With("component", "event_router"),
		closed:    false,
	}

	r.logger.Info("Event router initialized")
	return r, nil
}

// AddRoute registers a handler for events matching a topic pattern
// Patterns support:
//   - Exact match: "container.created"
//   - Wildcard: "container.*" matches all container events
//   - Multi-level wildcard: "*" matches all events
func (r *Router) AddRoute(ctx context.Context, pattern string, handler types.EventHandler) (types.ID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return "", types.NewError(types.ErrCodeUnavailable, "router is closed")
	}

	if handler == nil {
		return "", types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

	if pattern == "" {
		return "", types.NewError(types.ErrCodeInvalid, "pattern cannot be empty")
	}

	// Validate pattern
	if err := r.validatePattern(pattern); err != nil {
		return "", types.WrapError(types.ErrCodeInvalid, "invalid pattern", err)
	}

	// Generate route ID
	r.nextRouteID = types.GenerateID()
	routeID := r.nextRouteID

	entry := &routeEntry{
		ID:       routeID,
		Pattern:  pattern,
		Handler:  handler,
		Active:   true,
		Metadata: make(map[string]string),
		Priority: 0,
	}

	// Add to appropriate collection
	if strings.HasSuffix(pattern, ".*") || pattern == "*" {
		// Wildcard route
		prefix := strings.TrimSuffix(pattern, ".*")
		if prefix == "" {
			prefix = "*"
		}
		r.wildcards[prefix] = append(r.wildcards[prefix], entry)
	} else {
		// Exact route
		r.routes[pattern] = entry
	}

	r.logger.Debug("Route added",
		"route_id", routeID,
		"pattern", pattern)

	return routeID, nil
}

// AddRouteWithPriority registers a handler with a priority
// Higher priority routes are matched first (default is 0)
func (r *Router) AddRouteWithPriority(ctx context.Context, pattern string, handler types.EventHandler, priority int) (types.ID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return "", types.NewError(types.ErrCodeUnavailable, "router is closed")
	}

	if handler == nil {
		return "", types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

	if pattern == "" {
		return "", types.NewError(types.ErrCodeInvalid, "pattern cannot be empty")
	}

	// Validate pattern
	if err := r.validatePattern(pattern); err != nil {
		return "", types.WrapError(types.ErrCodeInvalid, "invalid pattern", err)
	}

	// Generate route ID
	r.nextRouteID = types.GenerateID()
	routeID := r.nextRouteID

	entry := &routeEntry{
		ID:       routeID,
		Pattern:  pattern,
		Handler:  handler,
		Active:   true,
		Metadata: make(map[string]string),
		Priority: priority,
	}

	// Add to appropriate collection
	if strings.HasSuffix(pattern, ".*") || pattern == "*" {
		prefix := strings.TrimSuffix(pattern, ".*")
		if prefix == "" {
			prefix = "*"
		}
		r.wildcards[prefix] = append(r.wildcards[prefix], entry)
		// Sort wildcards by priority (descending)
		r.sortWildcardEntries(prefix)
	} else {
		r.routes[pattern] = entry
	}

	r.logger.Debug("Route added with priority",
		"route_id", routeID,
		"pattern", pattern,
		"priority", priority)

	return routeID, nil
}

// RemoveRoute removes a route by ID
func (r *Router) RemoveRoute(routeID types.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "router is closed")
	}

	// Search in exact routes
	for pattern, entry := range r.routes {
		if entry.ID == routeID {
			delete(r.routes, pattern)
			r.logger.Debug("Route removed", "route_id", routeID, "pattern", pattern)
			return nil
		}
	}

	// Search in wildcard routes
	for prefix, entries := range r.wildcards {
		for i, entry := range entries {
			if entry.ID == routeID {
				// Remove from slice
				r.wildcards[prefix] = append(entries[:i], entries[i+1:]...)
				if len(r.wildcards[prefix]) == 0 {
					delete(r.wildcards, prefix)
				}
				r.logger.Debug("Wildcard route removed", "route_id", routeID, "prefix", prefix)
				return nil
			}
		}
	}

	return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("route not found: %s", routeID))
}

// Route routes an event to matching handlers
func (r *Router) Route(ctx context.Context, event types.Event) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return types.NewError(types.ErrCodeUnavailable, "router is closed")
	}

	// Convert event type to topic string
	topic := string(event.Type)

	// Find matching handlers
	handlers := r.findHandlers(topic)

	if len(handlers) == 0 {
		r.logger.Debug("No handlers found for topic", "topic", topic, "event_id", event.ID)
		return nil
	}

	r.logger.Debug("Routing event",
		"topic", topic,
		"event_id", event.ID,
		"handler_count", len(handlers))

	// Dispatch to all handlers concurrently
	var wg sync.WaitGroup
	errCh := make(chan error, len(handlers))

	for _, entry := range handlers {
		// Check if handler can handle this event type
		if !entry.Handler.CanHandle(event.Type) {
			continue
		}

		wg.Add(1)
		go func(route *routeEntry) {
			defer wg.Done()

			if err := route.Handler.Handle(ctx, event); err != nil {
				r.logger.Error("Handler failed",
					"route_id", route.ID,
					"pattern", route.Pattern,
					"event_id", event.ID,
					"error", err)
				errCh <- types.WrapError(types.ErrCodeHandlerFailed,
					fmt.Sprintf("route %s failed", route.ID), err)
			}
		}(entry)
	}

	// Wait for all handlers to complete
	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return types.WrapError(types.ErrCodePartialFailure,
			fmt.Sprintf("%d handler(s) failed", len(errs)),
			fmt.Errorf("%v", errs))
	}

	return nil
}

// findHandlers finds all route entries matching a topic
func (r *Router) findHandlers(topic string) []*routeEntry {
	var handlers []*routeEntry

	// Check for exact match first
	if entry, ok := r.routes[topic]; ok && entry.Active {
		handlers = append(handlers, entry)
	}

	// Check wildcard patterns
	// Topic parts: "container.created" -> ["container", "created"]
	parts := strings.Split(topic, ".")

	// Check each possible prefix
	for i := len(parts); i >= 0; i-- {
		prefix := strings.Join(parts[:i], ".")

		// Check for wildcard entries at this prefix level
		if entries, ok := r.wildcards[prefix]; ok {
			for _, entry := range entries {
				if entry.Active && r.matchPattern(topic, entry.Pattern) {
					handlers = append(handlers, entry)
				}
			}
		}
	}

	// Check catch-all wildcard
	if entries, ok := r.wildcards["*"]; ok {
		for _, entry := range entries {
			if entry.Active {
				handlers = append(handlers, entry)
			}
		}
	}

	return handlers
}

// matchPattern checks if a topic matches a pattern
func (r *Router) matchPattern(topic, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(topic, prefix+".")
	}

	return topic == pattern
}

// validatePattern validates a topic pattern
func (r *Router) validatePattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("pattern cannot be empty")
	}

	if pattern == "*" {
		return nil
	}

	// Check for invalid wildcard patterns
	if strings.Contains(pattern, "*") {
		if !strings.HasSuffix(pattern, ".*") && pattern != "*" {
			return fmt.Errorf("wildcard must be at the end: %s", pattern)
		}
	}

	// Check for empty segments
	parts := strings.Split(pattern, ".")
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("pattern has empty segment: %s", pattern)
		}
		if part == "*" && len(parts) > 1 {
			// Only "*" alone or "prefix.*" are valid
			return fmt.Errorf("invalid wildcard position: %s", pattern)
		}
	}

	return nil
}

// sortWildcardEntries sorts wildcard entries by priority (descending)
func (r *Router) sortWildcardEntries(prefix string) {
	entries := r.wildcards[prefix]
	// Simple bubble sort for small slices
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].Priority < entries[j].Priority {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// SetRouteMetadata updates metadata for a route
func (r *Router) SetRouteMetadata(routeID types.ID, metadata map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Search in exact routes
	for _, entry := range r.routes {
		if entry.ID == routeID {
			entry.Metadata = metadata
			return nil
		}
	}

	// Search in wildcard routes
	for _, entries := range r.wildcards {
		for _, entry := range entries {
			if entry.ID == routeID {
				entry.Metadata = metadata
				return nil
			}
		}
	}

	return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("route not found: %s", routeID))
}

// SetRouteActive sets the active state of a route
func (r *Router) SetRouteActive(routeID types.ID, active bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Search in exact routes
	for _, entry := range r.routes {
		if entry.ID == routeID {
			entry.Active = active
			return nil
		}
	}

	// Search in wildcard routes
	for _, entries := range r.wildcards {
		for _, entry := range entries {
			if entry.ID == routeID {
				entry.Active = active
				return nil
			}
		}
	}

	return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("route not found: %s", routeID))
}

// GetRoute retrieves information about a route
func (r *Router) GetRoute(routeID types.ID) (*RouteInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Search in exact routes
	for _, entry := range r.routes {
		if entry.ID == routeID {
			return &RouteInfo{
				ID:       entry.ID,
				Pattern:  entry.Pattern,
				Active:   entry.Active,
				Priority: entry.Priority,
				Metadata: entry.Metadata,
			}, nil
		}
	}

	// Search in wildcard routes
	for _, entries := range r.wildcards {
		for _, entry := range entries {
			if entry.ID == routeID {
				return &RouteInfo{
					ID:       entry.ID,
					Pattern:  entry.Pattern,
					Active:   entry.Active,
					Priority: entry.Priority,
					Metadata: entry.Metadata,
				}, nil
			}
		}
	}

	return nil, types.NewError(types.ErrCodeNotFound, fmt.Sprintf("route not found: %s", routeID))
}

// ListRoutes returns all registered routes
func (r *Router) ListRoutes() []RouteInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var routes []RouteInfo

	// Add exact routes
	for _, entry := range r.routes {
		routes = append(routes, RouteInfo{
			ID:       entry.ID,
			Pattern:  entry.Pattern,
			Active:   entry.Active,
			Priority: entry.Priority,
			Metadata: entry.Metadata,
		})
	}

	// Add wildcard routes
	for _, entries := range r.wildcards {
		for _, entry := range entries {
			routes = append(routes, RouteInfo{
				ID:       entry.ID,
				Pattern:  entry.Pattern,
				Active:   entry.Active,
				Priority: entry.Priority,
				Metadata: entry.Metadata,
			})
		}
	}

	return routes
}

// Stats returns router statistics
func (r *Router) Stats() RouterStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exactCount := len(r.routes)
	wildcardCount := 0
	activeCount := 0

	for _, entry := range r.routes {
		if entry.Active {
			activeCount++
		}
	}

	for _, entries := range r.wildcards {
		wildcardCount += len(entries)
		for _, entry := range entries {
			if entry.Active {
				activeCount++
			}
		}
	}

	return RouterStats{
		TotalRoutes:      exactCount + wildcardCount,
		ExactRoutes:      exactCount,
		WildcardRoutes:   wildcardCount,
		ActiveRoutes:     activeCount,
		InactiveRoutes:   (exactCount + wildcardCount) - activeCount,
	}
}

// Close closes the router
func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return types.NewError(types.ErrCodeInvalid, "router already closed")
	}

	r.closed = true
	r.logger.Info("Event router closed")
	return nil
}

// RouteInfo contains information about a route
type RouteInfo struct {
	ID       types.ID            `json:"id"`
	Pattern  string              `json:"pattern"`
	Active   bool                `json:"active"`
	Priority int                 `json:"priority"`
	Metadata map[string]string   `json:"metadata,omitempty"`
}

// RouterStats contains statistics about the router
type RouterStats struct {
	TotalRoutes    int `json:"total_routes"`
	ExactRoutes    int `json:"exact_routes"`
	WildcardRoutes int `json:"wildcard_routes"`
	ActiveRoutes   int `json:"active_routes"`
	InactiveRoutes int `json:"inactive_routes"`
}

// String returns a string representation of the stats
func (s RouterStats) String() string {
	return fmt.Sprintf("RouterStats{Total: %d, Exact: %d, Wildcard: %d, Active: %d, Inactive: %d}",
		s.TotalRoutes, s.ExactRoutes, s.WildcardRoutes, s.ActiveRoutes, s.InactiveRoutes)
}

// global router instance
var (
	globalRouter     *Router
	globalRouterOnce sync.Once
)

// InitGlobalRouter initializes the global router
func InitGlobalRouter(log *logger.Logger) error {
	var initErr error
	globalRouterOnce.Do(func() {
		router, err := NewRouter(log)
		if err != nil {
			initErr = err
			return
		}
		globalRouter = router
	})
	return initErr
}

// GlobalRouter returns the global router instance
func GlobalRouter() *Router {
	if globalRouter == nil {
		log, err := logger.NewDefault()
		if err != nil {
			panic(fmt.Sprintf("failed to create logger for global router: %v", err))
		}
		router, err := NewRouter(log)
		if err != nil {
			panic(fmt.Sprintf("failed to create global router: %v", err))
		}
		globalRouter = router
	}
	return globalRouter
}

// SetGlobalRouter sets the global router instance
func SetGlobalRouter(router *Router) {
	globalRouter = router
	globalRouterOnce = sync.Once{}
}

// RouteEvent is a convenience function that routes to the global router
func RouteEvent(ctx context.Context, event types.Event) error {
	return GlobalRouter().Route(ctx, event)
}

// AddRouteToGlobal is a convenience function that adds a route to the global router
func AddRouteToGlobal(ctx context.Context, pattern string, handler types.EventHandler) (types.ID, error) {
	return GlobalRouter().AddRoute(ctx, pattern, handler)
}
