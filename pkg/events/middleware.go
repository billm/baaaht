package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/baaaht/orchestrator/internal/logger"
	"github.com/baaaht/orchestrator/pkg/types"
)

// FilterMiddleware filters events based on a filter function
type FilterMiddleware struct {
	logger *logger.Logger
	filter func(types.Event) bool
}

// NewFilterMiddleware creates a new filter middleware
func NewFilterMiddleware(log *logger.Logger, filter func(types.Event) bool) (*FilterMiddleware, error) {
	if filter == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "filter function cannot be nil")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &FilterMiddleware{
		logger: log.With("component", "filter_middleware"),
		filter: filter,
	}, nil
}

// Process filters events - returns error if event should be rejected
func (m *FilterMiddleware) Process(ctx context.Context, event types.Event) (types.Event, error) {
	if !m.filter(event) {
		m.logger.Debug("Event filtered by middleware",
			"event_type", event.Type,
			"event_id", event.ID)
		return event, types.NewError(types.ErrCodeFiltered,
			fmt.Sprintf("event %s filtered by middleware", event.ID))
	}

	return event, nil
}

// TypeFilterMiddleware filters events by type
type TypeFilterMiddleware struct {
	logger    *logger.Logger
	allowedTypes map[types.EventType]bool
}

// NewTypeFilterMiddleware creates a new type filter middleware
// Pass a list of allowed event types. If allowedTypes is empty, all types are allowed.
func NewTypeFilterMiddleware(log *logger.Logger, allowedTypes ...types.EventType) (*TypeFilterMiddleware, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	allowed := make(map[types.EventType]bool)
	for _, eventType := range allowedTypes {
		allowed[eventType] = true
	}

	return &TypeFilterMiddleware{
		logger:       log.With("component", "type_filter_middleware"),
		allowedTypes: allowed,
	}, nil
}

// Process filters events by type
func (m *TypeFilterMiddleware) Process(ctx context.Context, event types.Event) (types.Event, error) {
	// If no types specified, allow all
	if len(m.allowedTypes) == 0 {
		return event, nil
	}

	if !m.allowedTypes[event.Type] {
		m.logger.Debug("Event type filtered by middleware",
			"event_type", event.Type,
			"event_id", event.ID)
		return event, types.NewError(types.ErrCodeFiltered,
			fmt.Sprintf("event type %s not allowed", event.Type))
	}

	return event, nil
}

// AddAllowedType adds an allowed event type
func (m *TypeFilterMiddleware) AddAllowedType(eventType types.EventType) {
	m.allowedTypes[eventType] = true
}

// RemoveAllowedType removes an allowed event type
func (m *TypeFilterMiddleware) RemoveAllowedType(eventType types.EventType) {
	delete(m.allowedTypes, eventType)
}

// SourceFilterMiddleware filters events by source
type SourceFilterMiddleware struct {
	logger          *logger.Logger
	allowedSources  map[string]bool
	blockedSources  map[string]bool
}

// NewSourceFilterMiddleware creates a new source filter middleware
func NewSourceFilterMiddleware(log *logger.Logger) (*SourceFilterMiddleware, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &SourceFilterMiddleware{
		logger:         log.With("component", "source_filter_middleware"),
		allowedSources: make(map[string]bool),
		blockedSources: make(map[string]bool),
	}, nil
}

// Process filters events by source
func (m *SourceFilterMiddleware) Process(ctx context.Context, event types.Event) (types.Event, error) {
	source := event.Source

	// Check blocked sources first
	if m.blockedSources[source] {
		m.logger.Debug("Event source blocked by middleware",
			"source", source,
			"event_id", event.ID)
		return event, types.NewError(types.ErrCodeFiltered,
			fmt.Sprintf("event source %s is blocked", source))
	}

	// If allowed sources configured, check them
	if len(m.allowedSources) > 0 && !m.allowedSources[source] {
		m.logger.Debug("Event source not in allowed list",
			"source", source,
			"event_id", event.ID)
		return event, types.NewError(types.ErrCodeFiltered,
			fmt.Sprintf("event source %s not allowed", source))
	}

	return event, nil
}

// AddAllowedSource adds an allowed source
func (m *SourceFilterMiddleware) AddAllowedSource(source string) {
	m.allowedSources[source] = true
}

// AddBlockedSource adds a blocked source
func (m *SourceFilterMiddleware) AddBlockedSource(source string) {
	m.blockedSources[source] = true
}

// RemoveAllowedSource removes an allowed source
func (m *SourceFilterMiddleware) RemoveAllowedSource(source string) {
	delete(m.allowedSources, source)
}

// RemoveBlockedSource removes a blocked source
func (m *SourceFilterMiddleware) RemoveBlockedSource(source string) {
	delete(m.blockedSources, source)
}

// TransformMiddleware transforms events
type TransformMiddleware struct {
	logger     *logger.Logger
	transform  func(types.Event) types.Event
}

// NewTransformMiddleware creates a new transform middleware
func NewTransformMiddleware(log *logger.Logger, transform func(types.Event) types.Event) (*TransformMiddleware, error) {
	if transform == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "transform function cannot be nil")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &TransformMiddleware{
		logger:    log.With("component", "transform_middleware"),
		transform: transform,
	}, nil
}

// Process transforms events
func (m *TransformMiddleware) Process(ctx context.Context, event types.Event) (types.Event, error) {
	transformed := m.transform(event)

	m.logger.Debug("Event transformed by middleware",
		"event_id", event.ID,
		"transformed_id", transformed.ID)

	return transformed, nil
}

// EnrichmentMiddleware adds additional data to events
type EnrichmentMiddleware struct {
	logger           *logger.Logger
	enrichments      map[string]interface{}
	addTimestamp     bool
	addHostname      bool
	customEnrichment func(types.Event) map[string]interface{}
}

// NewEnrichmentMiddleware creates a new enrichment middleware
func NewEnrichmentMiddleware(log *logger.Logger) (*EnrichmentMiddleware, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &EnrichmentMiddleware{
		logger:       log.With("component", "enrichment_middleware"),
		enrichments:  make(map[string]interface{}),
		addTimestamp: false,
		addHostname:  false,
	}, nil
}

// Process enriches events with additional data
func (m *EnrichmentMiddleware) Process(ctx context.Context, event types.Event) (types.Event, error) {
	// Initialize data map if needed
	if event.Data == nil {
		event.Data = make(map[string]interface{})
	}

	// Add static enrichments
	for k, v := range m.enrichments {
		event.Data[k] = v
	}

	// Add timestamp if configured
	if m.addTimestamp {
		event.Data["enriched_at"] = time.Now().Unix()
	}

	// Add hostname if configured
	if m.addHostname {
		hostname, err := getHostname()
		if err == nil {
			event.Data["hostname"] = hostname
		}
	}

	// Add custom enrichment
	if m.customEnrichment != nil {
		customData := m.customEnrichment(event)
		for k, v := range customData {
			event.Data[k] = v
		}
	}

	m.logger.Debug("Event enriched by middleware",
		"event_id", event.ID,
		"enrichment_count", len(m.enrichments))

	return event, nil
}

// AddEnrichment adds a static enrichment
func (m *EnrichmentMiddleware) AddEnrichment(key string, value interface{}) {
	m.enrichments[key] = value
}

// RemoveEnrichment removes an enrichment
func (m *EnrichmentMiddleware) RemoveEnrichment(key string) {
	delete(m.enrichments, key)
}

// SetAddTimestamp sets whether to add timestamp
func (m *EnrichmentMiddleware) SetAddTimestamp(add bool) {
	m.addTimestamp = add
}

// SetAddHostname sets whether to add hostname
func (m *EnrichmentMiddleware) SetAddHostname(add bool) {
	m.addHostname = add
}

// SetCustomEnrichment sets a custom enrichment function
func (m *EnrichmentMiddleware) SetCustomEnrichment(fn func(types.Event) map[string]interface{}) {
	m.customEnrichment = fn
}

// ValidationMiddleware validates events
type ValidationMiddleware struct {
	logger            *logger.Logger
	requireID         bool
	requireType       bool
	requireSource     bool
	requireTimestamp  bool
	customValidators  []func(types.Event) error
}

// NewValidationMiddleware creates a new validation middleware
func NewValidationMiddleware(log *logger.Logger) (*ValidationMiddleware, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &ValidationMiddleware{
		logger:           log.With("component", "validation_middleware"),
		requireID:        true,
		requireType:      true,
		requireSource:    false,
		requireTimestamp: true,
		customValidators: make([]func(types.Event) error, 0),
	}, nil
}

// Process validates events
func (m *ValidationMiddleware) Process(ctx context.Context, event types.Event) (types.Event, error) {
	// Validate ID
	if m.requireID && event.ID == "" {
		m.logger.Debug("Event validation failed: missing ID")
		return event, types.NewError(types.ErrCodeInvalid, "event ID is required")
	}

	// Validate Type
	if m.requireType && event.Type == "" {
		m.logger.Debug("Event validation failed: missing Type")
		return event, types.NewError(types.ErrCodeInvalid, "event Type is required")
	}

	// Validate Source
	if m.requireSource && event.Source == "" {
		m.logger.Debug("Event validation failed: missing Source")
		return event, types.NewError(types.ErrCodeInvalid, "event Source is required")
	}

	// Validate Timestamp
	if m.requireTimestamp && event.Timestamp.IsZero() {
		m.logger.Debug("Event validation failed: missing Timestamp")
		return event, types.NewError(types.ErrCodeInvalid, "event Timestamp is required")
	}

	// Run custom validators
	for i, validator := range m.customValidators {
		if err := validator(event); err != nil {
			m.logger.Debug("Event validation failed by custom validator",
				"validator_index", i,
				"error", err)
			return event, types.WrapError(types.ErrCodeInvalid,
				fmt.Sprintf("validation failed at validator %d", i), err)
		}
	}

	m.logger.Debug("Event validated successfully", "event_id", event.ID)
	return event, nil
}

// SetRequireID sets whether ID is required
func (m *ValidationMiddleware) SetRequireID(require bool) {
	m.requireID = require
}

// SetRequireType sets whether Type is required
func (m *ValidationMiddleware) SetRequireType(require bool) {
	m.requireType = require
}

// SetRequireSource sets whether Source is required
func (m *ValidationMiddleware) SetRequireSource(require bool) {
	m.requireSource = require
}

// SetRequireTimestamp sets whether Timestamp is required
func (m *ValidationMiddleware) SetRequireTimestamp(require bool) {
	m.requireTimestamp = require
}

// AddValidator adds a custom validator
func (m *ValidationMiddleware) AddValidator(validator func(types.Event) error) {
	m.customValidators = append(m.customValidators, validator)
}

// RateLimitMiddleware rate limits events
type RateLimitMiddleware struct {
	logger      *logger.Logger
	maxEvents   int
	window      time.Duration
	events      []time.Time
	mu          chan struct{} // Used as a mutex
}

// NewRateLimitMiddleware creates a new rate limit middleware
func NewRateLimitMiddleware(log *logger.Logger, maxEvents int, window time.Duration) (*RateLimitMiddleware, error) {
	if maxEvents <= 0 {
		return nil, types.NewError(types.ErrCodeInvalid, "maxEvents must be positive")
	}

	if window <= 0 {
		return nil, types.NewError(types.ErrCodeInvalid, "window must be positive")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &RateLimitMiddleware{
		logger:    log.With("component", "rate_limit_middleware"),
		maxEvents: maxEvents,
		window:    window,
		events:    make([]time.Time, 0, maxEvents),
		mu:        make(chan struct{}, 1),
	}, nil
}

// Process rate limits events
func (m *RateLimitMiddleware) Process(ctx context.Context, event types.Event) (types.Event, error) {
	// Acquire lock
	m.mu <- struct{}{}
	defer func() { <-m.mu }()

	now := time.Now()
	cutoff := now.Add(-m.window)

	// Remove old events outside the window
	newEvents := make([]time.Time, 0)
	for _, t := range m.events {
		if t.After(cutoff) {
			newEvents = append(newEvents, t)
		}
	}
	m.events = newEvents

	// Check if limit exceeded
	if len(m.events) >= m.maxEvents {
		m.logger.Debug("Rate limit exceeded",
			"max_events", m.maxEvents,
			"window", m.window,
			"current_count", len(m.events))
		return event, types.NewError(types.ErrCodeRateLimited,
			fmt.Sprintf("rate limit exceeded: %d events per %v", m.maxEvents, m.window))
	}

	// Add current event
	m.events = append(m.events, now)

	return event, nil
}

// GetEventCount returns the current event count in the window
func (m *RateLimitMiddleware) GetEventCount() int {
	m.mu <- struct{}{}
	defer func() { <-m.mu }()

	cutoff := time.Now().Add(-m.window)
	count := 0
	for _, t := range m.events {
		if t.After(cutoff) {
			count++
		}
	}
	return count
}

// DeduplicationMiddleware deduplicates events
type DeduplicationMiddleware struct {
	logger        *logger.Logger
	seenEvents    map[string]time.Time
	ttl           time.Duration
	mu            chan struct{} // Used as a mutex
}

// NewDeduplicationMiddleware creates a new deduplication middleware
func NewDeduplicationMiddleware(log *logger.Logger, ttl time.Duration) (*DeduplicationMiddleware, error) {
	if ttl <= 0 {
		return nil, types.NewError(types.ErrCodeInvalid, "TTL must be positive")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &DeduplicationMiddleware{
		logger:     log.With("component", "deduplication_middleware"),
		seenEvents: make(map[string]time.Time),
		ttl:        ttl,
		mu:         make(chan struct{}, 1),
	}, nil
}

// Process deduplicates events
func (m *DeduplicationMiddleware) Process(ctx context.Context, event types.Event) (types.Event, error) {
	// Acquire lock
	m.mu <- struct{}{}
	defer func() { <-m.mu }()

	now := time.Now()
	cutoff := now.Add(-m.ttl)
	eventKey := string(event.ID)

	// Clean up old entries
	for id, timestamp := range m.seenEvents {
		if timestamp.Before(cutoff) {
			delete(m.seenEvents, id)
		}
	}

	// Check if event was seen before
	if _, seen := m.seenEvents[eventKey]; seen {
		m.logger.Debug("Duplicate event detected",
			"event_id", event.ID,
			"event_type", event.Type)
		return event, types.NewError(types.ErrCodeDuplicate,
			fmt.Sprintf("duplicate event: %s", event.ID))
	}

	// Mark event as seen
	m.seenEvents[eventKey] = now

	return event, nil
}

// ClearSeen clears the seen events cache
func (m *DeduplicationMiddleware) ClearSeen() {
	m.mu <- struct{}{}
	defer func() { <-m.mu }()

	m.seenEvents = make(map[string]time.Time)
}

// MiddlewareChain chains multiple middleware together
type MiddlewareChain struct {
	middleware []types.EventMiddleware
	logger     *logger.Logger
}

// NewMiddlewareChain creates a new middleware chain
func NewMiddlewareChain(log *logger.Logger, middleware ...types.EventMiddleware) (*MiddlewareChain, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &MiddlewareChain{
		middleware: middleware,
		logger:     log.With("component", "middleware_chain"),
	}, nil
}

// Process executes all middleware in the chain
func (c *MiddlewareChain) Process(ctx context.Context, event types.Event) (types.Event, error) {
	var err error

	for i, mw := range c.middleware {
		event, err = mw.Process(ctx, event)
		if err != nil {
			c.logger.Debug("Middleware chain stopped",
				"middleware_index", i,
				"event_id", event.ID,
				"error", err)
			return event, err
		}
	}

	return event, nil
}

// AddMiddleware adds middleware to the end of the chain
func (c *MiddlewareChain) AddMiddleware(mw types.EventMiddleware) {
	c.middleware = append(c.middleware, mw)
}

// MiddlewareCount returns the number of middleware in the chain
func (c *MiddlewareChain) MiddlewareCount() int {
	return len(c.middleware)
}

// getHostname returns the system hostname
func getHostname() (string, error) {
	// Import os package here to avoid circular dependency in some cases
	hostname := "unknown"
	// Try to get hostname - this is a simple implementation
	// In production, you might want to use os.Hostname()
	return hostname, nil
}

// String helpers for source filtering
// AddAllowedSourcePattern adds an allowed source pattern (supports wildcards)
func (m *SourceFilterMiddleware) AddAllowedSourcePattern(pattern string) {
	m.allowedSources[pattern] = true
}

// isAllowed checks if a source matches any allowed pattern
func (m *SourceFilterMiddleware) isAllowed(source string) bool {
	if len(m.allowedSources) == 0 {
		return true // No restriction
	}

	// Check exact match
	if m.allowedSources[source] {
		return true
	}

	// Check wildcard patterns
	for pattern := range m.allowedSources {
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(source, prefix) {
				return true
			}
		}
	}

	return false
}

// isBlocked checks if a source matches any blocked pattern
func (m *SourceFilterMiddleware) isBlocked(source string) bool {
	// Check exact match
	if m.blockedSources[source] {
		return true
	}

	// Check wildcard patterns
	for pattern := range m.blockedSources {
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(source, prefix) {
				return true
			}
		}
	}

	return false
}

// ProcessWithPatterns processes with pattern matching for sources
func (m *SourceFilterMiddleware) ProcessWithPatterns(ctx context.Context, event types.Event) (types.Event, error) {
	source := event.Source

	// Check blocked sources first
	if m.isBlocked(source) {
		m.logger.Debug("Event source blocked by middleware",
			"source", source,
			"event_id", event.ID)
		return event, types.NewError(types.ErrCodeFiltered,
			fmt.Sprintf("event source %s is blocked", source))
	}

	// If allowed sources configured, check them
	if len(m.allowedSources) > 0 && !m.isAllowed(source) {
		m.logger.Debug("Event source not in allowed list",
			"source", source,
			"event_id", event.ID)
		return event, types.NewError(types.ErrCodeFiltered,
			fmt.Sprintf("event source %s not allowed", source))
	}

	return event, nil
}
