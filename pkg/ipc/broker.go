package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Broker manages inter-container communication
type Broker struct {
	mu              sync.RWMutex
	socket          *Socket
	messageQueue    chan *types.IPCMessage
	handlers        map[string]MessageHandler
	sessionMessages map[types.ID][]*types.IPCMessage // Session-scoped message history
	logger          *logger.Logger
	cfg             config.IPCConfig
	closed          bool
	wg              sync.WaitGroup
	closeCh         chan struct{}
	stats           BrokerStats
}

// MessageHandler handles IPC messages
type MessageHandler interface {
	// HandleMessage processes an IPC message
	HandleMessage(ctx context.Context, msg *types.IPCMessage) error
}

// MessageHandlerFunc is a function adapter for MessageHandler
type MessageHandlerFunc func(ctx context.Context, msg *types.IPCMessage) error

// HandleMessage implements MessageHandler
func (f MessageHandlerFunc) HandleMessage(ctx context.Context, msg *types.IPCMessage) error {
	return f(ctx, msg)
}

// New creates a new IPC broker
func New(cfg config.IPCConfig, log *logger.Logger) (*Broker, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Create socket config
	socketCfg := SocketConfig{
		Path:          cfg.SocketPath,
		MaxConnections: cfg.MaxConnections,
		BufferSize:     cfg.BufferSize,
		Timeout:        cfg.Timeout,
		EnableAuth:     cfg.EnableAuth,
	}

	// Create socket
	socket, err := NewSocket(cfg.SocketPath, socketCfg, log)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create socket", err)
	}

	b := &Broker{
		socket:          socket,
		messageQueue:    make(chan *types.IPCMessage, 1000),
		handlers:        make(map[string]MessageHandler),
		sessionMessages: make(map[types.ID][]*types.IPCMessage),
		logger:          log.With("component", "ipc_broker"),
		cfg:             cfg,
		closed:          false,
		closeCh:         make(chan struct{}),
		stats: BrokerStats{
			MessagesSent:     0,
			MessagesReceived: 0,
			MessagesFailed:   0,
			ActiveHandlers:   0,
		},
	}

	// Start the message processing goroutine
	b.wg.Add(1)
	go b.processMessages()

	b.logger.Info("IPC broker initialized",
		"socket_path", cfg.SocketPath,
		"buffer_size", cfg.BufferSize,
		"timeout", cfg.Timeout.String(),
		"max_connections", cfg.MaxConnections,
		"auth_enabled", cfg.EnableAuth)

	return b, nil
}

// NewDefault creates a new IPC broker with default configuration
func NewDefault(log *logger.Logger) (*Broker, error) {
	cfg := config.DefaultIPCConfig()
	return New(cfg, log)
}

// Start starts the IPC broker
func (b *Broker) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return types.NewError(types.ErrCodeUnavailable, "broker is closed")
	}
	b.mu.Unlock()

	// Start listening on the socket
	if err := b.socket.Listen(ctx); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to start socket", err)
	}

	b.logger.Info("IPC broker started")
	return nil
}

// Send sends a message from one container to another
func (b *Broker) Send(ctx context.Context, msg *types.IPCMessage) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "broker is closed")
	}
	b.mu.RUnlock()

	// Validate message
	if err := b.validateMessage(msg); err != nil {
		return types.WrapError(types.ErrCodeInvalid, "invalid message", err)
	}

	// Set timestamp if not set
	if msg.Timestamp.IsZero() {
		msg.Timestamp = types.Timestamp(time.Now())
	}

	// Generate ID if not set
	if msg.ID.IsEmpty() {
		msg.ID = types.GenerateID()
	}

	// Serialize message
	data, err := json.Marshal(msg)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to serialize message", err)
	}

	// Send to target
	if err := b.socket.Send(ctx, msg.Target, data); err != nil {
		b.mu.Lock()
		b.stats.MessagesFailed++
		b.mu.Unlock()
		return types.WrapError(types.ErrCodeInternal, "failed to send message", err)
	}

	// Store in session history if session ID is present
	if !msg.Metadata.SessionID.IsEmpty() {
		b.storeMessage(msg)
	}

	b.mu.Lock()
	b.stats.MessagesSent++
	b.mu.Unlock()

	b.logger.Debug("Message sent",
		"message_id", msg.ID,
		"source", msg.Source,
		"target", msg.Target,
		"type", msg.Type,
		"session_id", msg.Metadata.SessionID)

	return nil
}

// Broadcast sends a message to all connected containers
func (b *Broker) Broadcast(ctx context.Context, msg *types.IPCMessage) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "broker is closed")
	}
	b.mu.RUnlock()

	// Validate message
	if err := b.validateMessage(msg); err != nil {
		return types.WrapError(types.ErrCodeInvalid, "invalid message", err)
	}

	// Set timestamp if not set
	if msg.Timestamp.IsZero() {
		msg.Timestamp = types.Timestamp(time.Now())
	}

	// Generate ID if not set
	if msg.ID.IsEmpty() {
		msg.ID = types.GenerateID()
	}

	// Serialize message
	data, err := json.Marshal(msg)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to serialize message", err)
	}

	// Broadcast to all
	if err := b.socket.Broadcast(ctx, data); err != nil {
		b.mu.Lock()
		b.stats.MessagesFailed++
		b.mu.Unlock()
		return types.WrapError(types.ErrCodeInternal, "failed to broadcast message", err)
	}

	b.mu.Lock()
	b.stats.MessagesSent++
	b.mu.Unlock()

	b.logger.Debug("Message broadcast",
		"message_id", msg.ID,
		"source", msg.Source,
		"type", msg.Type,
		"session_id", msg.Metadata.SessionID)

	return nil
}

// RegisterHandler registers a message handler for a specific message type
func (b *Broker) RegisterHandler(msgType string, handler MessageHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return types.NewError(types.ErrCodeUnavailable, "broker is closed")
	}

	if handler == nil {
		return types.NewError(types.ErrCodeInvalid, "handler cannot be nil")
	}

	b.handlers[msgType] = handler
	b.stats.ActiveHandlers++

	b.logger.Debug("Handler registered", "message_type", msgType)
	return nil
}

// UnregisterHandler removes a message handler
func (b *Broker) UnregisterHandler(msgType string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return types.NewError(types.ErrCodeUnavailable, "broker is closed")
	}

	if _, exists := b.handlers[msgType]; !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("handler not found for type: %s", msgType))
	}

	delete(b.handlers, msgType)
	b.stats.ActiveHandlers--

	b.logger.Debug("Handler unregistered", "message_type", msgType)
	return nil
}

// Receive processes an incoming message
func (b *Broker) Receive(ctx context.Context, data []byte) error {
	// Deserialize message
	var msg types.IPCMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return types.WrapError(types.ErrCodeInvalid, "failed to deserialize message", err)
	}

	// Validate message
	if err := b.validateMessage(&msg); err != nil {
		return types.WrapError(types.ErrCodeInvalid, "invalid message", err)
	}

	// Enqueue for processing
	select {
	case b.messageQueue <- &msg:
		b.mu.Lock()
		b.stats.MessagesReceived++
		b.mu.Unlock()
		return nil
	case <-ctx.Done():
		return types.WrapError(types.ErrCodeCanceled, "receive canceled", ctx.Err())
	case <-time.After(5 * time.Second):
		return types.NewError(types.ErrCodeTimeout, "message queue full")
	}
}

// processMessages processes messages from the queue
func (b *Broker) processMessages() {
	defer b.wg.Done()

	for {
		select {
		case msg := <-b.messageQueue:
			b.handleMessage(context.Background(), msg)
		case <-b.closeCh:
			// Drain remaining messages
			for {
				select {
				case msg := <-b.messageQueue:
					b.handleMessage(context.Background(), msg)
				default:
					return
				}
			}
		}
	}
}

// handleMessage handles a single message
func (b *Broker) handleMessage(ctx context.Context, msg *types.IPCMessage) {
	// Find handler for this message type
	b.mu.RLock()
	handler, exists := b.handlers[msg.Type]
	b.mu.RUnlock()

	if !exists {
		b.logger.Warn("No handler for message type",
			"message_id", msg.ID,
			"type", msg.Type,
			"source", msg.Source)
		return
	}

	// Create timeout context
	handlerCtx, cancel := context.WithTimeout(ctx, b.cfg.Timeout)
	defer cancel()

	// Handle the message
	if err := handler.HandleMessage(handlerCtx, msg); err != nil {
		b.logger.Error("Handler failed",
			"message_id", msg.ID,
			"type", msg.Type,
			"source", msg.Source,
			"error", err)
		b.mu.Lock()
		b.stats.MessagesFailed++
		b.mu.Unlock()
		return
	}

	b.logger.Debug("Message handled",
		"message_id", msg.ID,
		"type", msg.Type,
		"source", msg.Source)
}

// GetSessionMessages retrieves message history for a session
func (b *Broker) GetSessionMessages(ctx context.Context, sessionID types.ID) ([]*types.IPCMessage, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "broker is closed")
	}

	messages, exists := b.sessionMessages[sessionID]
	if !exists {
		return []*types.IPCMessage{}, nil
	}

	// Return copies to prevent modification
	result := make([]*types.IPCMessage, len(messages))
	for i, msg := range messages {
		msgCopy := *msg
		result[i] = &msgCopy
	}

	return result, nil
}

// ClearSessionMessages clears message history for a session
func (b *Broker) ClearSessionMessages(ctx context.Context, sessionID types.ID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return types.NewError(types.ErrCodeUnavailable, "broker is closed")
	}

	delete(b.sessionMessages, sessionID)

	b.logger.Debug("Session messages cleared", "session_id", sessionID)
	return nil
}

// validateMessage validates a message
func (b *Broker) validateMessage(msg *types.IPCMessage) error {
	if msg.Source.IsEmpty() {
		return fmt.Errorf("source cannot be empty")
	}
	if msg.Target.IsEmpty() {
		return fmt.Errorf("target cannot be empty")
	}
	if msg.Type == "" {
		return fmt.Errorf("type cannot be empty")
	}
	return nil
}

// storeMessage stores a message in session history
func (b *Broker) storeMessage(msg *types.IPCMessage) {
	if msg.Metadata.SessionID.IsEmpty() {
		return
	}

	sessionID := msg.Metadata.SessionID
	b.sessionMessages[sessionID] = append(b.sessionMessages[sessionID], msg)

	// Limit history size (keep last 1000 messages per session)
	const maxHistory = 1000
	if len(b.sessionMessages[sessionID]) > maxHistory {
		b.sessionMessages[sessionID] = b.sessionMessages[sessionID][len(b.sessionMessages[sessionID])-maxHistory:]
	}
}

// Close closes the IPC broker
func (b *Broker) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "broker already closed")
	}
	b.closed = true
	b.mu.Unlock()

	// Stop message processor
	close(b.closeCh)

	// Wait for goroutines to finish
	b.wg.Wait()

	// Close message queue
	close(b.messageQueue)

	// Close socket
	if err := b.socket.Close(); err != nil {
		return err
	}

	b.logger.Info("IPC broker closed")
	return nil
}

// Stats returns broker statistics
func (b *Broker) Stats() BrokerStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := b.stats
	stats.QueueSize = len(b.messageQueue)
	stats.SocketStats = b.socket.Stats()
	return stats
}

// String returns a string representation of the broker
func (b *Broker) String() string {
	stats := b.Stats()
	return fmt.Sprintf("Broker{Sent: %d, Received: %d, Failed: %d, Handlers: %d, QueueSize: %d}",
		stats.MessagesSent, stats.MessagesReceived, stats.MessagesFailed,
		stats.ActiveHandlers, stats.QueueSize)
}

// BrokerStats represents broker statistics
type BrokerStats struct {
	MessagesSent     int64         `json:"messages_sent"`
	MessagesReceived int64         `json:"messages_received"`
	MessagesFailed   int64         `json:"messages_failed"`
	ActiveHandlers   int           `json:"active_handlers"`
	QueueSize        int           `json:"queue_size"`
	SocketStats      SocketStats   `json:"socket_stats"`
}

// String returns a string representation of the stats
func (s BrokerStats) String() string {
	return fmt.Sprintf("BrokerStats{Sent: %d, Received: %d, Failed: %d, Handlers: %d, Queue: %d, Socket: %s}",
		s.MessagesSent, s.MessagesReceived, s.MessagesFailed,
		s.ActiveHandlers, s.QueueSize, s.SocketStats.String())
}

// global broker instance
var (
	globalBroker     *Broker
	globalBrokerOnce sync.Once
)

// InitGlobal initializes the global IPC broker
func InitGlobal(cfg config.IPCConfig, log *logger.Logger) error {
	var initErr error
	globalBrokerOnce.Do(func() {
		broker, err := New(cfg, log)
		if err != nil {
			initErr = err
			return
		}
		globalBroker = broker
	})
	return initErr
}

// Global returns the global IPC broker instance
func Global() *Broker {
	if globalBroker == nil {
		log, err := logger.NewDefault()
		if err != nil {
			panic(fmt.Sprintf("failed to create default logger: %v", err))
		}
		broker, err := NewDefault(log)
		if err != nil {
			panic(fmt.Sprintf("failed to create global broker: %v", err))
		}
		globalBroker = broker
	}
	return globalBroker
}

// SetGlobal sets the global IPC broker instance
func SetGlobal(broker *Broker) {
	globalBroker = broker
	globalBrokerOnce = sync.Once{}
}
