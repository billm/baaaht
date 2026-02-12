package grpc

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

const (
	// DefaultMaxRecvMsgSize is the default maximum message size for receiving (in bytes)
	DefaultMaxRecvMsgSize = 1024 * 1024 * 100 // 100 MB
	// DefaultMaxSendMsgSize is the default maximum message size for sending (in bytes)
	DefaultMaxSendMsgSize = 1024 * 1024 * 100 // 100 MB
	// DefaultConnectionTimeout is the default connection timeout
	DefaultConnectionTimeout = 30 * time.Second
	// DefaultShutdownTimeout is the default graceful shutdown timeout
	DefaultShutdownTimeout = 10 * time.Second
)

// isClosedConnError checks if an error indicates a connection is already closed
func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}
	// Check for common "closed connection" error strings
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "closed network connection")
}

// Server represents a gRPC server with Unix Domain Socket transport
type Server struct {
	path              string
	listener          net.Listener
	server            *grpc.Server
	logger            *logger.Logger
	mu                sync.RWMutex
	closed            bool
	wg                sync.WaitGroup
	shutdownTimeout   time.Duration
	connectionTimeout time.Duration
	maxRecvMsgSize    int
	maxSendMsgSize    int
	interceptors      []grpc.ServerOption
	started           bool
	stats             ServerStats
}

// ServerStats represents server statistics
type ServerStats struct {
	StartTime   time.Time `json:"start_time"`
	ActiveConns int       `json:"active_connections"`
	TotalConns  int64     `json:"total_connections"`
	TotalRPCs   int64     `json:"total_rpcs"`
	IsServing   bool      `json:"is_serving"`
}

// ServerConfig contains server configuration
type ServerConfig struct {
	Path              string
	MaxRecvMsgSize    int
	MaxSendMsgSize    int
	ConnectionTimeout time.Duration
	ShutdownTimeout   time.Duration
	Interceptors      []grpc.ServerOption
}

// NewServer creates a new gRPC server with UDS transport
func NewServer(path string, cfg ServerConfig, log *logger.Logger) (*Server, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Validate and set default max message sizes
	maxRecvSize := cfg.MaxRecvMsgSize
	if maxRecvSize <= 0 {
		maxRecvSize = DefaultMaxRecvMsgSize
	}
	maxSendSize := cfg.MaxSendMsgSize
	if maxSendSize <= 0 {
		maxSendSize = DefaultMaxSendMsgSize
	}

	// Validate and set default timeouts
	connTimeout := cfg.ConnectionTimeout
	if connTimeout <= 0 {
		connTimeout = DefaultConnectionTimeout
	}
	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = DefaultShutdownTimeout
	}

	// Remove any existing file at the socket path
	// This handles stale socket files, but avoids deleting unsafe types like directories.
	if fi, err := os.Lstat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to stat existing path at socket path", err)
		}
	} else {
		mode := fi.Mode()
		// Only remove Unix sockets and regular files; refuse to delete other types.
		if mode&os.ModeSocket != 0 || mode.IsRegular() {
			if err := os.Remove(path); err != nil {
				return nil, types.WrapError(types.ErrCodeInternal, "failed to remove existing file at socket path", err)
			}
		} else {
			return nil, types.WrapError(
				types.ErrCodeInternal,
				fmt.Sprintf("existing path at socket path is of unsafe type %v; refusing to remove", mode),
				nil,
			)
		}
	}

	s := &Server{
		path:              path,
		logger:            log.With("component", "grpc_server", "socket_path", path),
		closed:            false,
		started:           false,
		shutdownTimeout:   shutdownTimeout,
		connectionTimeout: connTimeout,
		maxRecvMsgSize:    maxRecvSize,
		maxSendMsgSize:    maxSendSize,
		interceptors:      cfg.Interceptors,
		stats:             ServerStats{},
	}

	// Build server options with interceptors
	opts := s.buildServerOptions()

	// Create the gRPC server
	s.server = grpc.NewServer(opts...)

	s.logger.Info("gRPC server initialized",
		"path", path,
		"max_recv_msg_size", s.maxRecvMsgSize,
		"max_send_msg_size", s.maxSendMsgSize,
		"connection_timeout", s.connectionTimeout.String(),
		"shutdown_timeout", s.shutdownTimeout.String())

	return s, nil
}

// buildServerOptions constructs the gRPC server options
func (s *Server) buildServerOptions() []grpc.ServerOption {
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(s.maxRecvMsgSize),
		grpc.MaxSendMsgSize(s.maxSendMsgSize),
		grpc.Creds(insecure.NewCredentials()),
	}

	// Add custom interceptors
	opts = append(opts, s.interceptors...)

	return opts
}

// Start starts the gRPC server on the Unix socket
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return types.NewError(types.ErrCodeUnavailable, "server is closed")
	}
	if s.started {
		s.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "server already started")
	}
	// Mark as started (i.e., starting or already started) while holding the lock
	s.started = true
	s.mu.Unlock()

	// Create the listener
	listener, err := net.Listen("unix", s.path)
	if err != nil {
		// Roll back started state to allow retry
		s.mu.Lock()
		s.started = false
		s.mu.Unlock()
		return types.WrapError(types.ErrCodeInternal, "failed to listen on socket", err)
	}

	// Ensure containerized agents can connect to the UDS.
	// Unix socket connect requires write permission on the socket file.
	if err := os.Chmod(s.path, 0o777); err != nil {
		_ = listener.Close()
		s.mu.Lock()
		s.started = false
		s.mu.Unlock()
		return types.WrapError(types.ErrCodeInternal, "failed to set socket permissions", err)
	}

	s.mu.Lock()
	s.listener = listener
	s.stats.StartTime = time.Now()
	s.stats.IsServing = true
	s.mu.Unlock()

	s.logger.Info("gRPC server listening", "path", s.path)

	// Start serving in a goroutine
	s.wg.Add(1)
	go s.serve()

	return nil
}

// serve runs the gRPC server
func (s *Server) serve() {
	defer s.wg.Done()

	s.mu.RLock()
	server := s.server
	listener := s.listener
	s.mu.RUnlock()

	if err := server.Serve(listener); err != nil {
		s.mu.RLock()
		closed := s.closed
		s.mu.RUnlock()

		if !closed {
			s.logger.Error("gRPC server error", "error", err)
		}
	}
}

// RegisterService registers a gRPC service with the server
// This is a pass-through to grpc.Server.RegisterService
func (s *Server) RegisterService(sd *grpc.ServiceDesc, impl interface{}) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return types.NewError(types.ErrCodeUnavailable, "server is closed")
	}
	server := s.server
	s.mu.RUnlock()

	server.RegisterService(sd, impl)
	s.logger.Debug("Service registered", "service", sd.ServiceName)

	return nil
}

// GetServer returns the underlying grpc.Server for direct use
func (s *Server) GetServer() *grpc.Server {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.server
}

// Stop gracefully stops the gRPC server
func (s *Server) Stop() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return types.NewError(types.ErrCodeInvalid, "server already closed")
	}
	s.closed = true
	s.stats.IsServing = false
	s.mu.Unlock()

	s.logger.Info("Stopping gRPC server", "path", s.path)

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	// Graceful stop
	done := make(chan struct{})
	go func() {
		s.mu.RLock()
		server := s.server
		s.mu.RUnlock()
		server.GracefulStop()
		close(done)
	}()

	// Wait for graceful stop or timeout
	select {
	case <-done:
		s.logger.Info("gRPC server stopped gracefully")
	case <-ctx.Done():
		s.mu.RLock()
		server := s.server
		s.mu.RUnlock()
		s.logger.Warn("gRPC server shutdown timeout, stopping immediately")
		server.Stop()
	}

	// Close the listener
	s.mu.Lock()
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			// Ignore errors about already-closed connections (expected when gRPC
			// server already closed the listener during graceful shutdown)
			if !isClosedConnError(err) {
				s.logger.Error("Failed to close listener", "error", err)
			}
		}
	}
	s.mu.Unlock()

	// Wait for serve goroutine to finish
	s.wg.Wait()

	// Remove socket file
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("Failed to remove socket file", "path", s.path, "error", err)
	}

	s.logger.Info("gRPC server closed", "path", s.path)
	return nil
}

// Stats returns the current server statistics
func (s *Server) Stats() ServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Update active connections count if server is running
	if s.server != nil && s.stats.IsServing {
		// Note: gRPC doesn't expose active connection count directly
		// This would need to be tracked via interceptors if needed
	}

	return s.stats
}

// IsServing returns true if the server is currently serving
func (s *Server) IsServing() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats.IsServing && !s.closed
}

// String returns a string representation of the server
func (s *Server) String() string {
	stats := s.Stats()
	return fmt.Sprintf("Server{Path: %s, Started: %v, IsServing: %v, StartTime: %v}",
		s.path, s.started, stats.IsServing, stats.StartTime)
}

// SocketPath returns the Unix socket path the server is listening on
func (s *Server) SocketPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}
