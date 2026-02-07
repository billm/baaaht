package ipc

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/baaaht/orchestrator/internal/logger"
	"github.com/baaaht/orchestrator/pkg/types"
)

// Socket represents a Unix domain socket for IPC communication
type Socket struct {
	path        string
	listener    net.Listener
	conns       map[string]*connection
	connCount   int
	mu          sync.RWMutex
	logger      *logger.Logger
	closed      bool
	wg          sync.WaitGroup
	acceptCh    chan net.Conn
	closeCh     chan struct{}
	maxConns    int
	bufferSize  int
	timeout     time.Duration
	enableAuth  bool
}

// connection represents an active socket connection
type connection struct {
	net.Conn
	containerID types.ID
	sessionID   types.ID
	authenticated bool
	createdAt    time.Time
	lastActive   time.Time
}

// NewSocket creates a new Unix domain socket for IPC
func NewSocket(path string, cfg SocketConfig, log *logger.Logger) (*Socket, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Remove existing socket file if it exists
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to remove existing socket file", err)
		}
	}

	s := &Socket{
		path:       path,
		conns:      make(map[string]*connection),
		logger:     log.With("component", "ipc_socket", "socket_path", path),
		closed:     false,
		acceptCh:   make(chan net.Conn, 100),
		closeCh:    make(chan struct{}),
		maxConns:   cfg.MaxConnections,
		bufferSize: cfg.BufferSize,
		timeout:    cfg.Timeout,
		enableAuth: cfg.EnableAuth,
	}

	// Start the accept goroutine
	s.wg.Add(1)
	go s.acceptLoop()

	s.logger.Info("IPC socket initialized",
		"path", path,
		"max_connections", cfg.MaxConnections,
		"buffer_size", cfg.BufferSize,
		"timeout", cfg.Timeout.String(),
		"auth_enabled", cfg.EnableAuth)

	return s, nil
}

// Listen starts listening on the Unix socket
func (s *Socket) Listen(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return types.NewError(types.ErrCodeUnavailable, "socket is closed")
	}
	s.mu.Unlock()

	// Create the listener
	listener, err := net.Listen("unix", s.path)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to listen on socket", err)
	}

	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	s.logger.Info("IPC socket listening", "path", s.path)

	// Start accepting connections
	s.wg.Add(1)
	go s.acceptConnections()

	return nil
}

// acceptLoop handles incoming connection notifications
func (s *Socket) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case conn := <-s.acceptCh:
			s.handleConnection(conn)
		case <-s.closeCh:
			return
		}
	}
}

// acceptConnections accepts new connections from the listener
func (s *Socket) acceptConnections() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.RLock()
			closed := s.closed
			s.mu.RUnlock()

			if closed {
				return
			}
			s.logger.Error("Failed to accept connection", "error", err)
			continue
		}

		// Check connection limit
		s.mu.RLock()
		connCount := s.connCount
		maxConns := s.maxConns
		s.mu.RUnlock()

		if maxConns > 0 && connCount >= maxConns {
			s.logger.Warn("Connection limit reached, rejecting connection",
				"remote_addr", conn.RemoteAddr().String(),
				"current_count", connCount,
				"max_connections", maxConns)
			conn.Close()
			continue
		}

		// Send to accept loop
		select {
		case s.acceptCh <- conn:
		case <-time.After(5 * time.Second):
			s.logger.Warn("Accept channel full, dropping connection",
				"remote_addr", conn.RemoteAddr().String())
			conn.Close()
		}
	}
}

// handleConnection handles a new connection
func (s *Socket) handleConnection(netConn net.Conn) {
	s.mu.Lock()
	s.connCount++
	connID := fmt.Sprintf("%s-%d", netConn.RemoteAddr().String(), time.Now().UnixNano())
	conn := &connection{
		Conn:         netConn,
		createdAt:    time.Now(),
		lastActive:   time.Now(),
		authenticated: !s.enableAuth, // If auth is disabled, auto-authenticate
	}
	s.conns[connID] = conn
	s.mu.Unlock()

	s.logger.Debug("Connection accepted",
		"conn_id", connID,
		"remote_addr", netConn.RemoteAddr().String(),
		"conn_count", s.connCount)

	// Set read/write deadlines
	if s.timeout > 0 {
		netConn.SetDeadline(time.Now().Add(s.timeout))
	}
}

// Send sends a message to a specific container connection
func (s *Socket) Send(ctx context.Context, containerID types.ID, data []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "socket is closed")
	}

	// Find the connection for this container
	var targetConn *connection
	for _, conn := range s.conns {
		if conn.containerID == containerID {
			targetConn = conn
			break
		}
	}

	if targetConn == nil {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("connection not found for container: %s", containerID))
	}

	// Update last active
	targetConn.lastActive = time.Now()

	// Set write deadline
	if s.timeout > 0 {
		if err := targetConn.Conn.SetWriteDeadline(time.Now().Add(s.timeout)); err != nil {
			return types.WrapError(types.ErrCodeInternal, "failed to set write deadline", err)
		}
	}

	// Write data
	_, err := targetConn.Conn.Write(data)
	if err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to write to connection", err)
	}

	return nil
}

// Broadcast sends a message to all connections
func (s *Socket) Broadcast(ctx context.Context, data []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "socket is closed")
	}

	var errs []error
	for connID, conn := range s.conns {
		// Update last active
		conn.lastActive = time.Now()

		// Set write deadline
		if s.timeout > 0 {
			if err := conn.Conn.SetWriteDeadline(time.Now().Add(s.timeout)); err != nil {
				s.logger.Error("Failed to set write deadline", "conn_id", connID, "error", err)
				continue
			}
		}

		// Write data
		if _, err := conn.Conn.Write(data); err != nil {
			s.logger.Error("Failed to write to connection", "conn_id", connID, "error", err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return types.WrapError(types.ErrCodePartialFailure,
			fmt.Sprintf("failed to broadcast to %d connection(s)", len(errs)),
			fmt.Errorf("%v", errs))
	}

	return nil
}

// Receive reads data from a specific connection
func (s *Socket) Receive(ctx context.Context, connID string) ([]byte, error) {
	s.mu.RLock()
	conn, exists := s.conns[connID]
	s.mu.RUnlock()

	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, fmt.Sprintf("connection not found: %s", connID))
	}

	// Set read deadline
	if s.timeout > 0 {
		if err := conn.Conn.SetReadDeadline(time.Now().Add(s.timeout)); err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to set read deadline", err)
		}
	}

	// Read data
	buf := make([]byte, s.bufferSize)
	n, err := conn.Conn.Read(buf)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to read from connection", err)
	}

	// Update last active
	s.mu.Lock()
	conn.lastActive = time.Now()
	s.mu.Unlock()

	return buf[:n], nil
}

// CloseConnection closes a specific connection
func (s *Socket) CloseConnection(connID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.conns[connID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("connection not found: %s", connID))
	}

	if err := conn.Conn.Close(); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to close connection", err)
	}

	delete(s.conns, connID)
	s.connCount--

	s.logger.Debug("Connection closed", "conn_id", connID, "conn_count", s.connCount)
	return nil
}

// Authenticate authenticates a connection with a container ID
func (s *Socket) Authenticate(connID, containerID, sessionID types.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.conns[string(connID)]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("connection not found: %s", connID))
	}

	conn.containerID = containerID
	conn.sessionID = sessionID
	conn.authenticated = true

	s.logger.Debug("Connection authenticated",
		"conn_id", connID,
		"container_id", containerID,
		"session_id", sessionID)

	return nil
}

// IsAuthenticated checks if a connection is authenticated
func (s *Socket) IsAuthenticated(connID types.ID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn, exists := s.conns[string(connID)]
	if !exists {
		return false
	}
	return conn.authenticated
}

// Close closes the socket and all connections
func (s *Socket) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "socket already closed")
	}
	s.closed = true
	s.mu.Unlock()

	// Stop accept loop
	close(s.closeCh)

	// Close the listener
	s.mu.Lock()
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Unlock()

	// Wait for goroutines to finish
	s.wg.Wait()

	// Close all connections
	s.mu.Lock()
	for connID, conn := range s.conns {
		if err := conn.Conn.Close(); err != nil {
			s.logger.Error("Failed to close connection",
				"conn_id", connID,
				"error", err)
		}
	}
	s.conns = make(map[string]*connection)
	s.connCount = 0
	s.mu.Unlock()

	// Remove socket file
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("Failed to remove socket file", "path", s.path, "error", err)
	}

	s.logger.Info("IPC socket closed", "path", s.path)
	return nil
}

// Stats returns socket statistics
func (s *Socket) Stats() SocketStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SocketStats{
		Path:          s.path,
		ActiveConns:   s.connCount,
		Authenticated: s.countAuthenticated(),
	}
}

// countAuthenticated returns the count of authenticated connections
func (s *Socket) countAuthenticated() int {
	count := 0
	for _, conn := range s.conns {
		if conn.authenticated {
			count++
		}
	}
	return count
}

// String returns a string representation of the socket
func (s *Socket) String() string {
	stats := s.Stats()
	return fmt.Sprintf("Socket{Path: %s, ActiveConns: %d, Authenticated: %d}",
		stats.Path, stats.ActiveConns, stats.Authenticated)
}

// SocketStats represents socket statistics
type SocketStats struct {
	Path          string `json:"path"`
	ActiveConns   int    `json:"active_connections"`
	Authenticated int    `json:"authenticated"`
}

// String returns a string representation of the stats
func (s SocketStats) String() string {
	return fmt.Sprintf("SocketStats{Path: %s, Active: %d, Authenticated: %d}",
		s.Path, s.ActiveConns, s.Authenticated)
}

// SocketConfig contains socket configuration
type SocketConfig struct {
	Path          string
	MaxConnections int
	BufferSize     int
	Timeout        time.Duration
	EnableAuth     bool
}
