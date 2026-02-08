package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// HandlerChain chains multiple handlers together, executing them in sequence
// If any handler returns an error, the chain stops and returns the error
type HandlerChain struct {
	handlers []types.EventHandler
	logger   *logger.Logger
}

// NewHandlerChain creates a new handler chain
func NewHandlerChain(log *logger.Logger, handlers ...types.EventHandler) (*HandlerChain, error) {
	if len(handlers) == 0 {
		return nil, types.NewError(types.ErrCodeInvalid, "at least one handler required")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	return &HandlerChain{
		handlers: handlers,
		logger:   log.With("component", "handler_chain"),
	}, nil
}

// Handle executes all handlers in the chain
func (c *HandlerChain) Handle(ctx context.Context, event types.Event) error {
	for i, handler := range c.handlers {
		if !handler.CanHandle(event.Type) {
			c.logger.Debug("Handler skipped in chain",
				"handler_index", i,
				"event_type", event.Type)
			continue
		}

		c.logger.Debug("Executing handler in chain",
			"handler_index", i,
			"event_type", event.Type,
			"event_id", event.ID)

		if err := handler.Handle(ctx, event); err != nil {
			return types.WrapError(types.ErrCodeHandlerFailed,
				fmt.Sprintf("handler at index %d failed", i), err)
		}
	}

	return nil
}

// CanHandle returns true if any handler in the chain can handle the event
func (c *HandlerChain) CanHandle(eventType types.EventType) bool {
	for _, handler := range c.handlers {
		if handler.CanHandle(eventType) {
			return true
		}
	}
	return false
}

// AddHandler adds a handler to the end of the chain
func (c *HandlerChain) AddHandler(handler types.EventHandler) error {
	if handler == nil {
		return types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}
	c.handlers = append(c.handlers, handler)
	return nil
}

// RemoveHandler removes a handler from the chain by index
func (c *HandlerChain) RemoveHandler(index int) error {
	if index < 0 || index >= len(c.handlers) {
		return types.NewError(types.ErrCodeInvalid, fmt.Sprintf("invalid handler index: %d", index))
	}
	c.handlers = append(c.handlers[:index], c.handlers[index+1:]...)
	return nil
}

// HandlerCount returns the number of handlers in the chain
func (c *HandlerChain) HandlerCount() int {
	return len(c.handlers)
}

// AsyncHandler handles events asynchronously in a goroutine
type AsyncHandler struct {
	handler types.EventHandler
	logger  *logger.Logger
	timeout time.Duration
}

// NewAsyncHandler creates a new async handler
func NewAsyncHandler(log *logger.Logger, handler types.EventHandler, timeout time.Duration) (*AsyncHandler, error) {
	if handler == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}

	return &AsyncHandler{
		handler: handler,
		logger:  log.With("component", "async_handler"),
		timeout: timeout,
	}, nil
}

// Handle executes the handler asynchronously
func (h *AsyncHandler) Handle(ctx context.Context, event types.Event) error {
	// Create a channel for the result
	resultCh := make(chan error, 1)

	// Execute handler in goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- types.NewError(types.ErrCodeInternal,
					fmt.Sprintf("handler panic: %v", r))
			}
		}()
		resultCh <- h.handler.Handle(ctx, event)
	}()

	// Wait for result or timeout
	select {
	case err := <-resultCh:
		return err
	case <-time.After(h.timeout):
		return types.NewError(types.ErrCodeTimeout,
			fmt.Sprintf("async handler timed out after %v", h.timeout))
	case <-ctx.Done():
		return types.WrapError(types.ErrCodeCanceled, "async handler canceled", ctx.Err())
	}
}

// CanHandle delegates to the wrapped handler
func (h *AsyncHandler) CanHandle(eventType types.EventType) bool {
	return h.handler.CanHandle(eventType)
}

// RetryHandler retries a handler on failure
type RetryHandler struct {
	handler    types.EventHandler
	logger     *logger.Logger
	maxRetries int
	backoff    time.Duration
}

// NewRetryHandler creates a new retry handler
func NewRetryHandler(log *logger.Logger, handler types.EventHandler, maxRetries int, backoff time.Duration) (*RetryHandler, error) {
	if handler == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	if maxRetries <= 0 {
		maxRetries = 3 // Default retries
	}

	if backoff == 0 {
		backoff = time.Second // Default backoff
	}

	return &RetryHandler{
		handler:    handler,
		logger:     log.With("component", "retry_handler"),
		maxRetries: maxRetries,
		backoff:    backoff,
	}, nil
}

// Handle executes the handler with retry logic
func (h *RetryHandler) Handle(ctx context.Context, event types.Event) error {
	var lastErr error

	for attempt := 0; attempt <= h.maxRetries; attempt++ {
		if attempt > 0 {
			h.logger.Debug("Retrying handler",
				"attempt", attempt,
				"max_retries", h.maxRetries,
				"event_type", event.Type,
				"event_id", event.ID,
				"last_error", lastErr)

			// Wait before retry
			select {
			case <-time.After(h.backoff * time.Duration(attempt)):
			case <-ctx.Done():
				return types.WrapError(types.ErrCodeCanceled, "retry handler canceled during backoff", ctx.Err())
			}
		}

		err := h.handler.Handle(ctx, event)
		if err == nil {
			if attempt > 0 {
				h.logger.Info("Handler succeeded after retry",
					"attempt", attempt,
					"event_type", event.Type,
					"event_id", event.ID)
			}
			return nil
		}

		lastErr = err

		// Check if error is not retryable
		if types.IsErrCode(err, types.ErrCodeInvalid) ||
			types.IsErrCode(err, types.ErrCodeNotFound) ||
			types.IsErrCode(err, types.ErrCodePermission) {
			h.logger.Debug("Non-retryable error, aborting retries",
				"error_code", types.GetErrorCode(err),
				"event_type", event.Type)
			return err
		}
	}

	return types.WrapError(types.ErrCodeHandlerFailed,
		fmt.Sprintf("handler failed after %d retries", h.maxRetries), lastErr)
}

// CanHandle delegates to the wrapped handler
func (h *RetryHandler) CanHandle(eventType types.EventType) bool {
	return h.handler.CanHandle(eventType)
}

// TimeoutHandler adds a timeout to handler execution
type TimeoutHandler struct {
	handler types.EventHandler
	logger  *logger.Logger
	timeout time.Duration
}

// NewTimeoutHandler creates a new timeout handler
func NewTimeoutHandler(log *logger.Logger, handler types.EventHandler, timeout time.Duration) (*TimeoutHandler, error) {
	if handler == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}

	return &TimeoutHandler{
		handler: handler,
		logger:  log.With("component", "timeout_handler"),
		timeout: timeout,
	}, nil
}

// Handle executes the handler with a timeout
func (h *TimeoutHandler) Handle(ctx context.Context, event types.Event) error {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	resultCh := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- types.NewError(types.ErrCodeInternal,
					fmt.Sprintf("handler panic: %v", r))
			}
		}()
		resultCh <- h.handler.Handle(ctx, event)
	}()

	select {
	case err := <-resultCh:
		return err
	case <-ctx.Done():
		return types.NewError(types.ErrCodeTimeout,
			fmt.Sprintf("handler timed out after %v", h.timeout))
	}
}

// CanHandle delegates to the wrapped handler
func (h *TimeoutHandler) CanHandle(eventType types.EventType) bool {
	return h.handler.CanHandle(eventType)
}

// LoggingHandler logs event handling
type LoggingHandler struct {
	handler  types.EventHandler
	logger   *logger.Logger
	logLevel string
}

// NewLoggingHandler creates a new logging handler
func NewLoggingHandler(log *logger.Logger, handler types.EventHandler, logLevel string) (*LoggingHandler, error) {
	if handler == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	if logLevel == "" {
		logLevel = "info"
	}

	return &LoggingHandler{
		handler:  handler,
		logger:   log.With("component", "logging_handler"),
		logLevel: logLevel,
	}, nil
}

// Handle executes the handler with logging
func (h *LoggingHandler) Handle(ctx context.Context, event types.Event) error {
	startTime := time.Now()

	switch h.logLevel {
	case "debug":
		h.logger.Debug("Handling event",
			"event_type", event.Type,
			"event_id", event.ID,
			"source", event.Source)
	case "info":
		h.logger.Info("Handling event",
			"event_type", event.Type,
			"event_id", event.ID,
			"source", event.Source)
	}

	err := h.handler.Handle(ctx, event)

	duration := time.Since(startTime)

	if err != nil {
		h.logger.Error("Handler failed",
			"event_type", event.Type,
			"event_id", event.ID,
			"duration", duration,
			"error", err)
	} else {
		switch h.logLevel {
		case "debug":
			h.logger.Debug("Handler succeeded",
				"event_type", event.Type,
				"event_id", event.ID,
				"duration", duration)
		case "info":
			h.logger.Info("Handler succeeded",
				"event_type", event.Type,
				"event_id", event.ID,
				"duration", duration)
		}
	}

	return err
}

// CanHandle delegates to the wrapped handler
func (h *LoggingHandler) CanHandle(eventType types.EventType) bool {
	return h.handler.CanHandle(eventType)
}

// FilterHandler filters events before handling them
type FilterHandler struct {
	handler types.EventHandler
	logger  *logger.Logger
	filter  func(types.Event) bool
}

// NewFilterHandler creates a new filter handler
func NewFilterHandler(log *logger.Logger, handler types.EventHandler, filter func(types.Event) bool) (*FilterHandler, error) {
	if handler == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

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

	return &FilterHandler{
		handler: handler,
		logger:  log.With("component", "filter_handler"),
		filter:  filter,
	}, nil
}

// Handle executes the handler only if the filter returns true
func (h *FilterHandler) Handle(ctx context.Context, event types.Event) error {
	if !h.filter(event) {
		h.logger.Debug("Event filtered out",
			"event_type", event.Type,
			"event_id", event.ID)
		return nil
	}

	return h.handler.Handle(ctx, event)
}

// CanHandle delegates to the wrapped handler
func (h *FilterHandler) CanHandle(eventType types.EventType) bool {
	return h.handler.CanHandle(eventType)
}

// BufferedHandler buffers events and handles them in batches
type BufferedHandler struct {
	handler       types.EventHandler
	logger        *logger.Logger
	buffer        []types.Event
	mu            sync.Mutex
	batchSize     int
	flushInterval time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewBufferedHandler creates a new buffered handler
func NewBufferedHandler(log *logger.Logger, handler types.EventHandler, batchSize int, flushInterval time.Duration) (*BufferedHandler, error) {
	if handler == nil {
		return nil, types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	if flushInterval == 0 {
		flushInterval = 5 * time.Second // Default flush interval
	}

	h := &BufferedHandler{
		handler:       handler,
		logger:        log.With("component", "buffered_handler"),
		buffer:        make([]types.Event, 0, batchSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
	}

	// Start flush goroutine
	h.wg.Add(1)
	go h.flushLoop()

	return h, nil
}

// Handle adds the event to the buffer
func (h *BufferedHandler) Handle(ctx context.Context, event types.Event) error {
	h.mu.Lock()
	h.buffer = append(h.buffer, event)
	shouldFlush := len(h.buffer) >= h.batchSize
	h.mu.Unlock()

	if shouldFlush {
		return h.Flush(ctx)
	}

	return nil
}

// CanHandle delegates to the wrapped handler
func (h *BufferedHandler) CanHandle(eventType types.EventType) bool {
	return h.handler.CanHandle(eventType)
}

// Flush flushes the buffer by handling all buffered events
func (h *BufferedHandler) Flush(ctx context.Context) error {
	h.mu.Lock()
	if len(h.buffer) == 0 {
		h.mu.Unlock()
		return nil
	}

	// Copy buffer and create new one
	events := make([]types.Event, len(h.buffer))
	copy(events, h.buffer)
	h.buffer = make([]types.Event, 0, h.batchSize)
	h.mu.Unlock()

	h.logger.Debug("Flushing buffered events", "count", len(events))

	// Handle all events
	var errs []error
	for _, event := range events {
		if err := h.handler.Handle(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return types.WrapError(types.ErrCodePartialFailure,
			fmt.Sprintf("%d event(s) failed during flush", len(errs)),
			fmt.Errorf("%v", errs))
	}

	return nil
}

// flushLoop periodically flushes the buffer
func (h *BufferedHandler) flushLoop() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_ = h.Flush(ctx)
			cancel()
		case <-h.stopCh:
			// Final flush
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_ = h.Flush(ctx)
			cancel()
			return
		}
	}
}

// Close closes the buffered handler
func (h *BufferedHandler) Close() error {
	close(h.stopCh)
	h.wg.Wait()

	// Final flush
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.Flush(ctx)
}

// BufferedSize returns the current buffer size
func (h *BufferedHandler) BufferedSize() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.buffer)
}
