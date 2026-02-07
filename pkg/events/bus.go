package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Bus implements a publish-subscribe event bus
type Bus struct {
	mu            sync.RWMutex
	subscriptions map[types.ID]*types.EventSubscription
	handlerGroups map[types.EventType]map[types.ID]*types.EventSubscription
	logger        *logger.Logger
	closed        bool
	wg            sync.WaitGroup
	publishCh     chan *publishRequest
	closeCh       chan struct{}
}

// publishRequest represents an event publish request
type publishRequest struct {
	ctx   context.Context
	event types.Event
}

// New creates a new event bus
func New(log *logger.Logger) (*Bus, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	b := &Bus{
		subscriptions: make(map[types.ID]*types.EventSubscription),
		handlerGroups: make(map[types.EventType]map[types.ID]*types.EventSubscription),
		logger:        log.With("component", "event_bus"),
		closed:        false,
		publishCh:     make(chan *publishRequest, 1000),
		closeCh:       make(chan struct{}),
	}

	// Start the publish worker
	b.wg.Add(1)
	go b.publishWorker()

	b.logger.Info("Event bus initialized")
	return b, nil
}

// Subscribe registers a handler for events matching the filter
func (b *Bus) Subscribe(ctx context.Context, filter types.EventFilter, handler types.EventHandler) (types.ID, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return "", types.NewError(types.ErrCodeUnavailable, "event bus is closed")
	}

	if handler == nil {
		return "", types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

	// Generate subscription ID
	subID := types.GenerateID()

	subscription := &types.EventSubscription{
		ID:      subID,
		Filter:  filter,
		Handler: handler,
		Active:  true,
		CreatedAt: types.NewTimestampFromTime(time.Now()),
	}

	// Store subscription
	b.subscriptions[subID] = subscription

	// If filter specifies a type, add to handler group
	if filter.Type != nil {
		eventType := *filter.Type
		if b.handlerGroups[eventType] == nil {
			b.handlerGroups[eventType] = make(map[types.ID]*types.EventSubscription)
		}
		b.handlerGroups[eventType][subID] = subscription
	} else {
		// No type filter means subscribe to all events
		for _, group := range b.handlerGroups {
			group[subID] = subscription
		}
	}

	b.logger.Debug("Subscription created",
		"subscription_id", subID,
		"filter_type", filter.Type,
		"filter_source", filter.Source)

	return subID, nil
}

// SubscribeWithTopic registers a handler for a specific topic (event type pattern)
func (b *Bus) SubscribeWithTopic(ctx context.Context, topic types.EventTopic, handler types.EventHandler) (types.ID, error) {
	// Convert topic to filter
	var filter types.EventFilter

	if topic == types.TopicAll {
		// Empty filter matches all events
		filter = types.EventFilter{}
	} else {
		// Topic represents an event type pattern
		// For now, we'll treat it as an exact match prefix
		// The router will handle more sophisticated topic matching
		eventType := types.EventType(topic)
		filter = types.EventFilter{
			Type: &eventType,
		}
	}

	return b.Subscribe(ctx, filter, handler)
}

// Unsubscribe removes a subscription
func (b *Bus) Unsubscribe(subID types.ID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return types.NewError(types.ErrCodeUnavailable, "event bus is closed")
	}

	subscription, exists := b.subscriptions[subID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("subscription not found: %s", subID))
	}

	// Remove from handler groups
	if subscription.Filter.Type != nil {
		if group, ok := b.handlerGroups[*subscription.Filter.Type]; ok {
			delete(group, subID)
		}
	} else {
		// Remove from all groups
		for _, group := range b.handlerGroups {
			delete(group, subID)
		}
	}

	// Remove subscription
	delete(b.subscriptions, subID)

	b.logger.Debug("Subscription removed", "subscription_id", subID)
	return nil
}

// Publish sends an event to all matching subscribers
func (b *Bus) Publish(ctx context.Context, event types.Event) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "event bus is closed")
	}
	b.mu.RUnlock()

	// Validate event
	if event.ID == "" {
		event.ID = types.GenerateID()
	}
	if event.Timestamp.IsZero() {
			event.Timestamp = types.NewTimestampFromTime(time.Now())
	}

	// Send to publish worker
	select {
	case b.publishCh <- &publishRequest{ctx: ctx, event: event}:
		return nil
	case <-ctx.Done():
		return types.WrapError(types.ErrCodeCanceled, "publish canceled", ctx.Err())
	case <-time.After(5 * time.Second):
		return types.NewError(types.ErrCodeTimeout, "publish timeout - event bus buffer full")
	}
}

// PublishSync publishes an event synchronously, waiting for all handlers to complete
func (b *Bus) PublishSync(ctx context.Context, event types.Event) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "event bus is closed")
	}
	b.mu.RUnlock()

	// Validate event
	if event.ID == "" {
		event.ID = types.GenerateID()
	}
	if event.Timestamp.IsZero() {
			event.Timestamp = types.NewTimestampFromTime(time.Now())
	}

	return b.dispatchEvent(ctx, event)
}

// publishWorker processes events from the publish channel
func (b *Bus) publishWorker() {
	defer b.wg.Done()

	for {
		select {
		case req := <-b.publishCh:
			if err := b.dispatchEvent(req.ctx, req.event); err != nil {
				b.logger.Error("Failed to dispatch event",
					"event_id", req.event.ID,
					"event_type", req.event.Type,
					"error", err)
			}
		case <-b.closeCh:
			// Drain remaining events
			for {
				select {
				case req := <-b.publishCh:
					_ = b.dispatchEvent(req.ctx, req.event)
				default:
					return
				}
			}
		}
	}
}

// dispatchEvent sends an event to all matching subscribers
func (b *Bus) dispatchEvent(ctx context.Context, event types.Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var handlers []types.EventHandler
	var subIDs []types.ID

	// Find matching subscriptions
	for subID, sub := range b.subscriptions {
		if !sub.Active {
			continue
		}

		if b.matchesFilter(event, sub.Filter) && sub.Handler.CanHandle(event.Type) {
			handlers = append(handlers, sub.Handler)
			subIDs = append(subIDs, subID)
		}
	}

	if len(handlers) == 0 {
		b.logger.Debug("No handlers for event", "event_type", event.Type, "event_id", event.ID)
		return nil
	}

	b.logger.Debug("Dispatching event",
		"event_type", event.Type,
		"event_id", event.ID,
		"handler_count", len(handlers))

	// Dispatch to all handlers concurrently
	var wg sync.WaitGroup
	errCh := make(chan error, len(handlers))

	for i, handler := range handlers {
		wg.Add(1)
		go func(idx int, h types.EventHandler, sid types.ID) {
			defer wg.Done()

			// Create a timeout context for handler execution
			handlerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			if err := h.Handle(handlerCtx, event); err != nil {
				b.logger.Error("Handler failed",
					"subscription_id", sid,
					"event_id", event.ID,
					"event_type", event.Type,
					"error", err)
				errCh <- types.WrapError(types.ErrCodeHandlerFailed,
					fmt.Sprintf("handler %s failed", sid), err)
			}
		}(i, handler, subIDs[i])
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

// matchesFilter checks if an event matches the subscription filter
func (b *Bus) matchesFilter(event types.Event, filter types.EventFilter) bool {
	// Check type filter
	if filter.Type != nil && event.Type != *filter.Type {
		return false
	}

	// Check source filter
	if filter.Source != nil && event.Source != *filter.Source {
		return false
	}

	// Check session ID filter
	if filter.SessionID != nil {
		if event.Metadata.SessionID == nil || *event.Metadata.SessionID != *filter.SessionID {
			return false
		}
	}

	// Check container ID filter
	if filter.ContainerID != nil {
		if event.Metadata.ContainerID == nil || *event.Metadata.ContainerID != *filter.ContainerID {
			return false
		}
	}

	// Check priority filter
	if filter.Priority != nil && event.Metadata.Priority != *filter.Priority {
		return false
	}

	// Check label filters
	if len(filter.Labels) > 0 {
		for k, v := range filter.Labels {
			if event.Metadata.Labels == nil || event.Metadata.Labels[k] != v {
				return false
			}
		}
	}

	// Check time range
	if filter.StartTime != nil && event.Timestamp.Time.Before(filter.StartTime.Time) {
		return false
	}
	if filter.EndTime != nil && event.Timestamp.Time.After(filter.EndTime.Time) {
		return false
	}

	return true
}

// Close gracefully shuts down the event bus
func (b *Bus) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "event bus already closed")
	}
	b.closed = true
	b.mu.Unlock()

	// Signal the worker to stop
	close(b.closeCh)

	// Wait for worker to finish
	b.wg.Wait()

	// Close publish channel
	close(b.publishCh)

	b.logger.Info("Event bus closed")
	return nil
}

// Stats returns statistics about the event bus
func (b *Bus) Stats() BusStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	activeCount := 0
	for _, sub := range b.subscriptions {
		if sub.Active {
			activeCount++
		}
	}

	return BusStats{
		TotalSubscriptions: len(b.subscriptions),
		ActiveSubscriptions: activeCount,
		PendingEvents:       len(b.publishCh),
	}
}

// BusStats represents event bus statistics
type BusStats struct {
	TotalSubscriptions int `json:"total_subscriptions"`
	ActiveSubscriptions int `json:"active_subscriptions"`
	PendingEvents       int `json:"pending_events"`
}

// String returns a string representation of the stats
func (s BusStats) String() string {
	return fmt.Sprintf("BusStats{Total: %d, Active: %d, Pending: %d}",
		s.TotalSubscriptions, s.ActiveSubscriptions, s.PendingEvents)
}

// global event bus instance
var (
	globalBus     *Bus
	globalBusOnce sync.Once
)

// InitGlobal initializes the global event bus
func InitGlobal(log *logger.Logger) error {
	var initErr error
	globalBusOnce.Do(func() {
		bus, err := New(log)
		if err != nil {
			initErr = err
			return
		}
		globalBus = bus
	})
	return initErr
}

// Global returns the global event bus instance
func Global() *Bus {
	if globalBus == nil {
		log, err := logger.NewDefault()
		if err != nil {
			// Panic if we can't create a logger - this is a critical failure
			panic(fmt.Sprintf("failed to create default logger for global event bus: %v", err))
		}
		bus, err := New(log)
		if err != nil {
			panic(fmt.Sprintf("failed to create global event bus: %v", err))
		}
		globalBus = bus
	}
	return globalBus
}

// SetGlobal sets the global event bus instance
func SetGlobal(bus *Bus) {
	globalBus = bus
	globalBusOnce = sync.Once{}
}

// Publish is a convenience function that publishes to the global bus
func Publish(ctx context.Context, event types.Event) error {
	return Global().Publish(ctx, event)
}

// Subscribe is a convenience function that subscribes to the global bus
func Subscribe(ctx context.Context, filter types.EventFilter, handler types.EventHandler) (types.ID, error) {
	return Global().Subscribe(ctx, filter, handler)
}
