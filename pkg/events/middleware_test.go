package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/baaaht/orchestrator/internal/logger"
	"github.com/baaaht/orchestrator/pkg/types"
)

// setupTestMiddlewareLogger creates a logger for middleware tests
func setupTestMiddlewareLogger(t *testing.T) (*logger.Logger, context.Context) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return log, context.Background()
}

// TestFilterMiddleware tests filter middleware
func TestFilterMiddleware(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	// Filter only container.created events
	filter := func(event types.Event) bool {
		return event.Type == types.EventTypeContainerCreated
	}

	mw, err := NewFilterMiddleware(log, filter)
	if err != nil {
		t.Fatalf("Failed to create filter middleware: %v", err)
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

	result, err := mw.Process(ctx, event1)
	if err != nil {
		t.Errorf("Filter middleware should pass event1: %v", err)
	}
	if result.ID != event1.ID {
		t.Error("Event ID should not change")
	}

	_, err = mw.Process(ctx, event2)
	if err == nil {
		t.Error("Filter middleware should reject event2")
	}

	if !types.IsErrCode(err, types.ErrCodeFiltered) {
		t.Errorf("Expected ErrCodeFiltered, got %v", types.GetErrorCode(err))
	}
}

// TestFilterMiddlewareNilFilter tests filter middleware with nil filter
func TestFilterMiddlewareNilFilter(t *testing.T) {
	log, _ := setupTestMiddlewareLogger(t)

	_, err := NewFilterMiddleware(log, nil)
	if err == nil {
		t.Error("Expected error when creating filter middleware with nil filter")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestTypeFilterMiddleware tests type filter middleware
func TestTypeFilterMiddleware(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewTypeFilterMiddleware(log,
		types.EventTypeContainerCreated,
		types.EventTypeContainerStarted)
	if err != nil {
		t.Fatalf("Failed to create type filter middleware: %v", err)
	}

	// Allowed event
	event1 := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	// Not allowed event
	event2 := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerStopped,
		Timestamp: types.Timestamp(time.Now()),
	}

	result, err := mw.Process(ctx, event1)
	if err != nil {
		t.Errorf("Type filter middleware should pass event1: %v", err)
	}
	if result.ID != event1.ID {
		t.Error("Event ID should not change")
	}

	_, err = mw.Process(ctx, event2)
	if err == nil {
		t.Error("Type filter middleware should reject event2")
	}

	if !types.IsErrCode(err, types.ErrCodeFiltered) {
		t.Errorf("Expected ErrCodeFiltered, got %v", types.GetErrorCode(err))
	}
}

// TestTypeFilterMiddlewareEmpty tests type filter middleware with no types (allow all)
func TestTypeFilterMiddlewareEmpty(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewTypeFilterMiddleware(log)
	if err != nil {
		t.Fatalf("Failed to create type filter middleware: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Type filter middleware should pass all events when empty: %v", err)
	}
	if result.ID != event.ID {
		t.Error("Event ID should not change")
	}
}

// TestTypeFilterMiddlewareAddRemove tests adding and removing allowed types
func TestTypeFilterMiddlewareAddRemove(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewTypeFilterMiddleware(log, types.EventTypeContainerCreated)
	if err != nil {
		t.Fatalf("Failed to create type filter middleware: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerStarted,
		Timestamp: types.Timestamp(time.Now()),
	}

	// Should be blocked initially
	_, err = mw.Process(ctx, event)
	if err == nil {
		t.Error("Event should be blocked initially")
	}

	// Add allowed type
	mw.AddAllowedType(types.EventTypeContainerStarted)

	// Should pass now
	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Event should pass after adding type: %v", err)
	}
	if result.ID != event.ID {
		t.Error("Event ID should not change")
	}

	// Remove allowed type
	mw.RemoveAllowedType(types.EventTypeContainerStarted)

	// Should be blocked again
	_, err = mw.Process(ctx, event)
	if err == nil {
		t.Error("Event should be blocked after removing type")
	}
}

// TestSourceFilterMiddleware tests source filter middleware
func TestSourceFilterMiddleware(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewSourceFilterMiddleware(log)
	if err != nil {
		t.Fatalf("Failed to create source filter middleware: %v", err)
	}

	// Add allowed and blocked sources
	mw.AddAllowedSource("trusted-source")
	mw.AddBlockedSource("malicious-source")

	// Allowed source event
	event1 := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "trusted-source",
		Timestamp: types.Timestamp(time.Now()),
	}

	// Blocked source event
	event2 := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "malicious-source",
		Timestamp: types.Timestamp(time.Now()),
	}

	// Unknown source event (should pass when no restriction)
	event3 := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "unknown-source",
		Timestamp: types.Timestamp(time.Now()),
	}

	result, err := mw.Process(ctx, event1)
	if err != nil {
		t.Errorf("Source filter middleware should pass event1: %v", err)
	}
	if result.ID != event1.ID {
		t.Error("Event ID should not change")
	}

	_, err = mw.Process(ctx, event2)
	if err == nil {
		t.Error("Source filter middleware should reject event2")
	}

	// event3 should pass because we haven't restricted allowed sources
	result, err = mw.Process(ctx, event3)
	if err != nil {
		t.Errorf("Source filter middleware should pass event3: %v", err)
	}
	if result.ID != event3.ID {
		t.Error("Event ID should not change")
	}
}

// TestSourceFilterMiddlewareRestricted tests when allowed sources are restricted
func TestSourceFilterMiddlewareRestricted(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewSourceFilterMiddleware(log)
	if err != nil {
		t.Fatalf("Failed to create source filter middleware: %v", err)
	}

	// Only allow specific sources
	mw.AddAllowedSource("trusted-source")

	// Allowed source event
	event1 := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "trusted-source",
		Timestamp: types.Timestamp(time.Now()),
	}

	// Unknown source event
	event2 := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "unknown-source",
		Timestamp: types.Timestamp(time.Now()),
	}

	result, err := mw.Process(ctx, event1)
	if err != nil {
		t.Errorf("Source filter middleware should pass event1: %v", err)
	}
	if result.ID != event1.ID {
		t.Error("Event ID should not change")
	}

	_, err = mw.Process(ctx, event2)
	if err == nil {
		t.Error("Source filter middleware should reject event2")
	}
}

// TestTransformMiddleware tests transform middleware
func TestTransformMiddleware(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	transform := func(event types.Event) types.Event {
		// Add a prefix to the source
		event.Source = "transformed:" + event.Source
		return event
	}

	mw, err := NewTransformMiddleware(log, transform)
	if err != nil {
		t.Fatalf("Failed to create transform middleware: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "original",
		Timestamp: types.Timestamp(time.Now()),
	}

	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Transform middleware should not error: %v", err)
	}

	if result.Source != "transformed:original" {
		t.Errorf("Expected transformed source, got %s", result.Source)
	}
}

// TestTransformMiddlewareNilTransform tests transform middleware with nil transform
func TestTransformMiddlewareNilTransform(t *testing.T) {
	log, _ := setupTestMiddlewareLogger(t)

	_, err := NewTransformMiddleware(log, nil)
	if err == nil {
		t.Error("Expected error when creating transform middleware with nil transform")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestEnrichmentMiddleware tests enrichment middleware
func TestEnrichmentMiddleware(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewEnrichmentMiddleware(log)
	if err != nil {
		t.Fatalf("Failed to create enrichment middleware: %v", err)
	}

	// Add some enrichments
	mw.AddEnrichment("environment", "test")
	mw.AddEnrichment("version", "1.0.0")

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
		Data:      make(map[string]interface{}),
	}

	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Enrichment middleware should not error: %v", err)
	}

	if result.Data["environment"] != "test" {
		t.Error("Expected environment enrichment")
	}

	if result.Data["version"] != "1.0.0" {
		t.Error("Expected version enrichment")
	}
}

// TestEnrichmentMiddlewareWithTimestamp tests enrichment middleware with timestamp
func TestEnrichmentMiddlewareWithTimestamp(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewEnrichmentMiddleware(log)
	if err != nil {
		t.Fatalf("Failed to create enrichment middleware: %v", err)
	}

	mw.SetAddTimestamp(true)

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
		Data:      make(map[string]interface{}),
	}

	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Enrichment middleware should not error: %v", err)
	}

	if result.Data["enriched_at"] == nil {
		t.Error("Expected enriched_at field")
	}
}

// TestEnrichmentMiddlewareCustom tests enrichment middleware with custom enrichment
func TestEnrichmentMiddlewareCustom(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewEnrichmentMiddleware(log)
	if err != nil {
		t.Fatalf("Failed to create enrichment middleware: %v", err)
	}

	mw.SetCustomEnrichment(func(event types.Event) map[string]interface{} {
		return map[string]interface{}{
			"event_type_prefix": string(event.Type)[:5],
		}
	})

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
		Data:      make(map[string]interface{}),
	}

	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Enrichment middleware should not error: %v", err)
	}

	if result.Data["event_type_prefix"] != "cont" {
		t.Errorf("Expected custom enrichment, got %v", result.Data["event_type_prefix"])
	}
}

// TestValidationMiddleware tests validation middleware
func TestValidationMiddleware(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewValidationMiddleware(log)
	if err != nil {
		t.Fatalf("Failed to create validation middleware: %v", err)
	}

	// Valid event
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Validation middleware should pass valid event: %v", err)
	}
	if result.ID != event.ID {
		t.Error("Event ID should not change")
	}

	// Invalid event - missing ID
	invalidEvent := types.Event{
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	_, err = mw.Process(ctx, invalidEvent)
	if err == nil {
		t.Error("Validation middleware should reject event without ID")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalid) {
		t.Errorf("Expected ErrCodeInvalid, got %v", types.GetErrorCode(err))
	}
}

// TestValidationMiddlewareCustom tests validation middleware with custom validators
func TestValidationMiddlewareCustom(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewValidationMiddleware(log)
	if err != nil {
		t.Fatalf("Failed to create validation middleware: %v", err)
	}

	// Add custom validator
	mw.AddValidator(func(event types.Event) error {
		if event.Source == "" {
			return errors.New("source is required")
		}
		return nil
	})

	// Event without source
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	_, err = mw.Process(ctx, event)
	if err == nil {
		t.Error("Validation middleware should reject event without source")
	}

	// Event with source
	event.Source = "test-source"
	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Validation middleware should pass valid event: %v", err)
	}
	if result.Source != "test-source" {
		t.Error("Event source should not change")
	}
}

// TestValidationMiddlewareOptions tests validation middleware options
func TestValidationMiddlewareOptions(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewValidationMiddleware(log)
	if err != nil {
		t.Fatalf("Failed to create validation middleware: %v", err)
	}

	// Disable some requirements
	mw.SetRequireSource(false)
	mw.SetRequireTimestamp(false)

	// Event that would normally fail but passes now
	event := types.Event{
		ID:   types.GenerateID(),
		Type: types.EventTypeContainerCreated,
	}

	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Validation middleware should pass with relaxed requirements: %v", err)
	}
	if result.ID != event.ID {
		t.Error("Event ID should not change")
	}
}

// TestRateLimitMiddleware tests rate limit middleware
func TestRateLimitMiddleware(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewRateLimitMiddleware(log, 5, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create rate limit middleware: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	// Should allow 5 events
	for i := 0; i < 5; i++ {
		result, err := mw.Process(ctx, event)
		if err != nil {
			t.Errorf("Rate limit middleware should allow event %d: %v", i, err)
		}
		if result.ID != event.ID {
			t.Error("Event ID should not change")
		}
	}

	// 6th event should be rate limited
	_, err = mw.Process(ctx, event)
	if err == nil {
		t.Error("Rate limit middleware should reject 6th event")
	}

	if !types.IsErrCode(err, types.ErrCodeRateLimited) {
		t.Errorf("Expected ErrCodeRateLimited, got %v", types.GetErrorCode(err))
	}
}

// TestRateLimitMiddlewareWindow tests rate limit middleware window expiry
func TestRateLimitMiddlewareWindow(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewRateLimitMiddleware(log, 2, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create rate limit middleware: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	// Use up the limit
	for i := 0; i < 2; i++ {
		_, err := mw.Process(ctx, event)
		if err != nil {
			t.Errorf("Rate limit middleware should allow event %d: %v", i, err)
		}
	}

	// Should be rate limited
	_, err = mw.Process(ctx, event)
	if err == nil {
		t.Error("Rate limit middleware should reject event")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Rate limit middleware should allow event after window expiry: %v", err)
	}
	if result.ID != event.ID {
		t.Error("Event ID should not change")
	}
}

// TestDeduplicationMiddleware tests deduplication middleware
func TestDeduplicationMiddleware(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewDeduplicationMiddleware(log, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create deduplication middleware: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	// First event should pass
	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Deduplication middleware should pass first event: %v", err)
	}
	if result.ID != event.ID {
		t.Error("Event ID should not change")
	}

	// Duplicate event should be filtered
	_, err = mw.Process(ctx, event)
	if err == nil {
		t.Error("Deduplication middleware should reject duplicate event")
	}

	if !types.IsErrCode(err, types.ErrCodeDuplicate) {
		t.Errorf("Expected ErrCodeDuplicate, got %v", types.GetErrorCode(err))
	}
}

// TestDeduplicationMiddlewareExpiry tests deduplication middleware TTL expiry
func TestDeduplicationMiddlewareExpiry(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewDeduplicationMiddleware(log, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create deduplication middleware: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	// First event should pass
	_, err = mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Deduplication middleware should pass first event: %v", err)
	}

	// Duplicate event should be filtered
	_, err = mw.Process(ctx, event)
	if err == nil {
		t.Error("Deduplication middleware should reject duplicate event")
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Same event should pass again
	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Deduplication middleware should pass event after TTL expiry: %v", err)
	}
	if result.ID != event.ID {
		t.Error("Event ID should not change")
	}
}

// TestDeduplicationMiddlewareClear tests clearing the seen events cache
func TestDeduplicationMiddlewareClear(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	mw, err := NewDeduplicationMiddleware(log, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create deduplication middleware: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	// First event should pass
	_, err = mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Deduplication middleware should pass first event: %v", err)
	}

	// Duplicate event should be filtered
	_, err = mw.Process(ctx, event)
	if err == nil {
		t.Error("Deduplication middleware should reject duplicate event")
	}

	// Clear the cache
	mw.ClearSeen()

	// Same event should pass again
	result, err := mw.Process(ctx, event)
	if err != nil {
		t.Errorf("Deduplication middleware should pass event after clearing: %v", err)
	}
	if result.ID != event.ID {
		t.Error("Event ID should not change")
	}
}

// TestMiddlewareChain tests middleware chain
func TestMiddlewareChain(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	// Create middleware
	transformMw, _ := NewTransformMiddleware(log, func(event types.Event) types.Event {
		event.Source = "transformed:" + event.Source
		return event
	})

	enrichmentMw, _ := NewEnrichmentMiddleware(log)
	enrichmentMw.AddEnrichment("key", "value")

	chain, err := NewMiddlewareChain(log, transformMw, enrichmentMw)
	if err != nil {
		t.Fatalf("Failed to create middleware chain: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "original",
		Timestamp: types.Timestamp(time.Now()),
		Data:      make(map[string]interface{}),
	}

	result, err := chain.Process(ctx, event)
	if err != nil {
		t.Errorf("Middleware chain should not error: %v", err)
	}

	if result.Source != "transformed:original" {
		t.Errorf("Expected transformed source, got %s", result.Source)
	}

	if result.Data["key"] != "value" {
		t.Error("Expected enrichment")
	}
}

// TestMiddlewareChainStopOnError tests middleware chain stopping on error
func TestMiddlewareChainStopOnError(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	// Create middleware - second one should error
	filterMw, _ := NewFilterMiddleware(log, func(event types.Event) bool {
		return event.Source == "allowed"
	})

	enrichmentMw, _ := NewEnrichmentMiddleware(log)
	enrichmentMw.AddEnrichment("key", "value")

	chain, err := NewMiddlewareChain(log, filterMw, enrichmentMw)
	if err != nil {
		t.Fatalf("Failed to create middleware chain: %v", err)
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Source:    "blocked",
		Timestamp: types.Timestamp(time.Now()),
		Data:      make(map[string]interface{}),
	}

	_, err = chain.Process(ctx, event)
	if err == nil {
		t.Error("Middleware chain should stop on filter error")
	}

	// Enrichment should not have been applied
	if event.Data["key"] != nil {
		t.Error("Enrichment should not have been applied after filter error")
	}
}

// TestMiddlewareChainAdd tests adding middleware to chain
func TestMiddlewareChainAdd(t *testing.T) {
	log, ctx := setupTestMiddlewareLogger(t)

	chain, err := NewMiddlewareChain(log)
	if err != nil {
		t.Fatalf("Failed to create middleware chain: %v", err)
	}

	if chain.MiddlewareCount() != 0 {
		t.Errorf("Expected 0 middleware, got %d", chain.MiddlewareCount())
	}

	mw1, _ := NewEnrichmentMiddleware(log)
	mw2, _ := NewValidationMiddleware(log)

	chain.AddMiddleware(mw1)
	chain.AddMiddleware(mw2)

	if chain.MiddlewareCount() != 2 {
		t.Errorf("Expected 2 middleware, got %d", chain.MiddlewareCount())
	}

	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
		Data:      make(map[string]interface{}),
	}

	_, err = chain.Process(ctx, event)
	if err != nil {
		t.Errorf("Middleware chain should not error: %v", err)
	}
}

// BenchmarkMiddlewareChain benchmarks middleware chain
func BenchmarkMiddlewareChain(b *testing.B) {
	log, _ := setupTestMiddlewareLogger(b)

	mw1, _ := NewEnrichmentMiddleware(log)
	mw2, _ := NewValidationMiddleware(log)
	mw3, _ := NewTransformMiddleware(log, func(event types.Event) types.Event {
		return event
	})

	chain, _ := NewMiddlewareChain(log, mw1, mw2, mw3)

	ctx := context.Background()
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
		Data:      make(map[string]interface{}),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain.Process(ctx, event)
	}
}

// BenchmarkFilterMiddleware benchmarks filter middleware
func BenchmarkFilterMiddleware(b *testing.B) {
	log, _ := setupTestMiddlewareLogger(b)

	filter := func(event types.Event) bool {
		return event.Type == types.EventTypeContainerCreated
	}

	mw, _ := NewFilterMiddleware(log, filter)

	ctx := context.Background()
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mw.Process(ctx, event)
	}
}

// BenchmarkEnrichmentMiddleware benchmarks enrichment middleware
func BenchmarkEnrichmentMiddleware(b *testing.B) {
	log, _ := setupTestMiddlewareLogger(b)

	mw, _ := NewEnrichmentMiddleware(log)
	mw.AddEnrichment("key", "value")

	ctx := context.Background()
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
		Data:      make(map[string]interface{}),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mw.Process(ctx, event)
	}
}

// BenchmarkValidationMiddleware benchmarks validation middleware
func BenchmarkValidationMiddleware(b *testing.B) {
	log, _ := setupTestMiddlewareLogger(b)

	mw, _ := NewValidationMiddleware(log)

	ctx := context.Background()
	event := types.Event{
		ID:        types.GenerateID(),
		Type:      types.EventTypeContainerCreated,
		Timestamp: types.Timestamp(time.Now()),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mw.Process(ctx, event)
	}
}
