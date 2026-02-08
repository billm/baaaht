package grpc

import (
	"context"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/billm/baaaht/orchestrator/internal/logger"
)

// HealthServer implements the gRPC health checking protocol
// See https://github.com/grpc/grpc/blob/master/doc/health-checking.md
type HealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	logger   *logger.Logger
	mu       sync.RWMutex
	statuses map[string]grpc_health_v1.HealthCheckResponse_ServingStatus
	shutdown bool
}

// HealthServerConfig contains health server configuration
type HealthServerConfig struct {
	// InitialStatuses maps service names to their initial health status
	InitialStatuses map[string]grpc_health_v1.HealthCheckResponse_ServingStatus
}

// NewHealthServer creates a new health check server
func NewHealthServer(cfg HealthServerConfig, log *logger.Logger) (*HealthServer, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, wrapError(errCodeInternal, "failed to create default logger", err)
		}
	}

	// Default to empty status map if not provided
	statuses := cfg.InitialStatuses
	if statuses == nil {
		statuses = make(map[string]grpc_health_v1.HealthCheckResponse_ServingStatus)
	}

	// Set default status for empty service name (standard health check)
	if _, exists := statuses[""]; !exists {
		statuses[""] = grpc_health_v1.HealthCheckResponse_SERVING
	}

	hs := &HealthServer{
		logger:   log.With("component", "health_server"),
		statuses: statuses,
		shutdown: false,
	}

	hs.logger.Info("Health server initialized",
		"initial_statuses", len(statuses),
		"default_status", statuses[""])

	return hs, nil
}

// Check implements the health check RPC
// If the service name is empty ("") or not found, it returns the default server status
func (s *HealthServer) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	service := req.Service

	// If server is shutting down, return NOT_SERVING
	if s.shutdown {
		s.logger.Debug("Health check returned NOT_SERVING (shutdown)",
			"service", service)
		return &grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		}, nil
	}

	// Look up the service status
	// Empty service name means check the overall server status
	servingStatus, exists := s.statuses[service]
	if !exists {
		// If service not found and request was for specific service, return NOT_FOUND
		if service != "" {
			s.logger.Debug("Health check returned NOT_FOUND (unknown service)",
				"service", service)
			return nil, status.Error(codes.NotFound, "unknown service")
		}
		// For empty service name, use the default status
		servingStatus = s.statuses[""]
	}

	s.logger.Debug("Health check completed",
		"service", service,
		"status", servingStatus.String())

	return &grpc_health_v1.HealthCheckResponse{
		Status: servingStatus,
	}, nil
}

// Watch implements the health watch RPC
// It sends the current health status and then closes the stream.
// Note: This is a simplified implementation that does not stream ongoing status changes.
func (s *HealthServer) Watch(req *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	service := req.Service

	s.logger.Debug("Health watch started", "service", service)

	// Send initial status
	s.mu.RLock()
	initialStatus := s.getStatus(service)
	shutdown := s.shutdown
	s.mu.RUnlock()

	if err := stream.Send(&grpc_health_v1.HealthCheckResponse{
		Status: initialStatus,
	}); err != nil {
		s.logger.Error("Failed to send initial health status",
			"service", service,
			"error", err)
		return err
	}

	// If already shutting down, close the stream
	if shutdown {
		return nil
	}

	// Note: This implementation does not stream ongoing health status changes.
	// It sends a single response with the current status and then closes the stream.
	//
	// In a more advanced implementation, you could monitor status changes and
	// push updates to the stream when SetServingStatus or Shutdown is called,
	// for example using per-service watchers or channels.
	//
	// We currently close the stream after sending the initial status to avoid
	// blocking goroutines indefinitely while still exposing the Watch RPC.
	s.logger.Debug("Health watch stream closed", "service", service)
	return nil
}

// SetServingStatus sets the serving status of the given service
// If service is empty (""), it sets the default status for all unspecified services
func (s *HealthServer) SetServingStatus(service string, status grpc_health_v1.HealthCheckResponse_ServingStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldStatus, _ := s.statuses[service]
	s.statuses[service] = status

	s.logger.Info("Health status updated",
		"service", service,
		"old_status", oldStatus.String(),
		"new_status", status.String())
}

// SetShutdown puts the health server into shutdown mode
// All health checks will return NOT_SERVING after this call
func (s *HealthServer) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.shutdown {
		return
	}

	s.shutdown = true
	s.logger.Info("Health server shutdown")
}

// GetStatus returns the current serving status for a service
// If service is not found, returns the default status
func (s *HealthServer) GetStatus(service string) grpc_health_v1.HealthCheckResponse_ServingStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getStatus(service)
}

// getStatus must be called with the lock held
func (s *HealthServer) getStatus(service string) grpc_health_v1.HealthCheckResponse_ServingStatus {
	if s.shutdown {
		return grpc_health_v1.HealthCheckResponse_NOT_SERVING
	}

	status, exists := s.statuses[service]
	if !exists {
		// Return default status for unknown services
		status = s.statuses[""]
	}
	return status
}

// SetServing sets the service status to SERVING
func (s *HealthServer) SetServing(service string) {
	s.SetServingStatus(service, grpc_health_v1.HealthCheckResponse_SERVING)
}

// SetNotServing sets the service status to NOT_SERVING
func (s *HealthServer) SetNotServing(service string) {
	s.SetServingStatus(service, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
}

// IsServing returns true if the service is currently SERVING
func (s *HealthServer) IsServing(service string) bool {
	return s.GetStatus(service) == grpc_health_v1.HealthCheckResponse_SERVING
}

// GetAllStatuses returns a copy of all service statuses
func (s *HealthServer) GetAllStatuses() map[string]grpc_health_v1.HealthCheckResponse_ServingStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]grpc_health_v1.HealthCheckResponse_ServingStatus, len(s.statuses))
	for k, v := range s.statuses {
		result[k] = v
	}
	return result
}

// Helper function to wrap errors using the types package pattern
func wrapError(code string, message string, err error) error {
	// This matches the pattern used in server.go which uses types.WrapError
	// We'll use a simpler approach here since we're in the health package
	if err != nil {
		return &wrappedError{
			code:    code,
			message: message,
			cause:   err,
		}
	}
	return &wrappedError{
		code:    code,
		message: message,
	}
}

type wrappedError struct {
	code    string
	message string
	cause   error
}

func (e *wrappedError) Error() string {
	if e.cause != nil {
		return e.message + ": " + e.cause.Error()
	}
	return e.message
}

func (e *wrappedError) Unwrap() error {
	return e.cause
}

const (
	errCodeInternal = "INTERNAL"
)
