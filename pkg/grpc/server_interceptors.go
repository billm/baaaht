package grpc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

const (
	// authorizationKey is the metadata key for authorization tokens
	authorizationKey = "authorization"
	// bearerPrefix is the prefix for bearer tokens
	bearerPrefix = "Bearer "
)

// InterceptorConfig contains configuration for server interceptors
type InterceptorConfig struct {
	// EnableLogging enables the logging interceptor
	EnableLogging bool
	// EnableAuth enables the authentication interceptor
	EnableAuth bool
	// LogLevel determines the verbosity of logging (debug, info, warn, error)
	LogLevel string
	// LogAllRequests logs all requests, not just errors
	LogAllRequests bool
	// AuthTokens is a list of valid authentication tokens
	// If empty, authentication is disabled even if EnableAuth is true
	AuthTokens map[string]string
	// AuthTokenPrefix is the expected prefix for auth tokens (default: "Bearer ")
	AuthTokenPrefix string
}

// DefaultInterceptorConfig returns default interceptor configuration
func DefaultInterceptorConfig() InterceptorConfig {
	return InterceptorConfig{
		EnableLogging:   true,
		EnableAuth:      false,
		LogLevel:        "info",
		LogAllRequests:  true,
		AuthTokens:      make(map[string]string),
		AuthTokenPrefix: bearerPrefix,
	}
}

// LoggingInterceptorConfig contains specific configuration for the logging interceptor
type LoggingInterceptorConfig struct {
	logger      *logger.Logger
	logAll      bool
	logLevel    logger.Level
	excludeMethods map[string]bool
}

// AuthInterceptorConfig contains configuration for the authentication interceptor
type AuthInterceptorConfig struct {
	logger         *logger.Logger
	validTokens    map[string]string
	tokenPrefix    string
	enableAuth     bool
	mu             sync.RWMutex
	stats          AuthStats
}

// AuthStats tracks authentication statistics
type AuthStats struct {
	TotalRequests  int64 `json:"total_requests"`
	SuccessAuth    int64 `json:"success_auth"`
	FailedAuth     int64 `json:"failed_auth"`
	SkippedAuth    int64 `json:"skipped_auth"`
}

// NewLoggingInterceptor creates a unary logging interceptor
func NewLoggingInterceptor(log *logger.Logger, cfg InterceptorConfig) grpc.ServerOption {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			// If we can't create a logger, return a no-op option
			return grpc.EmptyServerOption{}
		}
	}

	logCfg := LoggingInterceptorConfig{
		logger:      log.With("component", "grpc_logging_interceptor"),
		logAll:      cfg.LogAllRequests,
		logLevel:    parseLogLevel(cfg.LogLevel),
		excludeMethods: make(map[string]bool),
	}

	return grpc.ChainUnaryInterceptor(loggingUnaryInterceptor(logCfg))
}

// NewStreamLoggingInterceptor creates a stream logging interceptor
func NewStreamLoggingInterceptor(log *logger.Logger, cfg InterceptorConfig) grpc.ServerOption {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return grpc.EmptyServerOption{}
		}
	}

	logCfg := LoggingInterceptorConfig{
		logger:      log.With("component", "grpc_logging_interceptor"),
		logAll:      cfg.LogAllRequests,
		logLevel:    parseLogLevel(cfg.LogLevel),
		excludeMethods: make(map[string]bool),
	}

	return grpc.ChainStreamInterceptor(loggingStreamInterceptor(logCfg))
}

// NewAuthInterceptor creates an authentication interceptor
func NewAuthInterceptor(log *logger.Logger, cfg InterceptorConfig) grpc.ServerOption {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return grpc.EmptyServerOption{}
		}
	}

	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "grpc_auth_interceptor"),
		validTokens: cfg.AuthTokens,
		tokenPrefix: cfg.AuthTokenPrefix,
		enableAuth:  cfg.EnableAuth && len(cfg.AuthTokens) > 0,
		stats: AuthStats{
			TotalRequests:  0,
			SuccessAuth:    0,
			FailedAuth:     0,
			SkippedAuth:    0,
		},
	}

	return grpc.ChainUnaryInterceptor(authUnaryInterceptor(authCfg))
}

// NewStreamAuthInterceptor creates a stream authentication interceptor
func NewStreamAuthInterceptor(log *logger.Logger, cfg InterceptorConfig) grpc.ServerOption {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return grpc.EmptyServerOption{}
		}
	}

	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "grpc_auth_interceptor"),
		validTokens: cfg.AuthTokens,
		tokenPrefix: cfg.AuthTokenPrefix,
		enableAuth:  cfg.EnableAuth && len(cfg.AuthTokens) > 0,
		stats: AuthStats{
			TotalRequests:  0,
			SuccessAuth:    0,
			FailedAuth:     0,
			SkippedAuth:    0,
		},
	}

	return grpc.ChainStreamInterceptor(authStreamInterceptor(authCfg))
}

// parseLogLevel converts a string log level to logger.Level
func parseLogLevel(level string) logger.Level {
	switch strings.ToLower(level) {
	case "debug":
		return logger.LevelDebug
	case "info":
		return logger.LevelInfo
	case "warn", "warning":
		return logger.LevelWarn
	case "error":
		return logger.LevelError
	default:
		return logger.LevelInfo
	}
}

// loggingUnaryInterceptor logs unary RPC calls
func loggingUnaryInterceptor(cfg LoggingInterceptorConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		method := info.FullMethod

		// Log request start
		if cfg.logLevel >= logger.LevelDebug {
			cfg.logger.Debug("RPC started",
				"method", method,
				"request_type", fmt.Sprintf("%T", req))
		}

		// Call the handler
		resp, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(start)

		// Log completion
		if err != nil {
			// Always log errors
			st, _ := status.FromError(err)
			cfg.logger.Error("RPC failed",
				"method", method,
				"code", st.Code().String(),
				"message", st.Message(),
				"duration_ms", duration.Milliseconds())
		} else if cfg.logAll || cfg.logLevel >= logger.LevelDebug {
			cfg.logger.Debug("RPC completed",
				"method", method,
				"response_type", fmt.Sprintf("%T", resp),
				"duration_ms", duration.Milliseconds())
		}

		return resp, err
	}
}

// loggingStreamInterceptor logs streaming RPC calls
func loggingStreamInterceptor(cfg LoggingInterceptorConfig) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		method := info.FullMethod

		// Log stream start
		if cfg.logLevel >= logger.LevelDebug {
			cfg.logger.Debug("Stream started",
				"method", method,
				"is_client_stream", info.IsClientStream,
				"is_server_stream", info.IsServerStream)
		}

		// Call the handler
		err := handler(srv, stream)

		// Calculate duration
		duration := time.Since(start)

		// Log completion
		if err != nil {
			// Always log errors
			st, _ := status.FromError(err)
			cfg.logger.Error("Stream failed",
				"method", method,
				"code", st.Code().String(),
				"message", st.Message(),
				"duration_ms", duration.Milliseconds())
		} else if cfg.logAll || cfg.logLevel >= logger.LevelDebug {
			cfg.logger.Debug("Stream completed",
				"method", method,
				"duration_ms", duration.Milliseconds())
		}

		return err
	}
}

// authUnaryInterceptor authenticates unary RPC calls
func authUnaryInterceptor(cfg *AuthInterceptorConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Update stats
		cfg.mu.Lock()
		cfg.stats.TotalRequests++
		cfg.mu.Unlock()

		// Check if auth is enabled
		if !cfg.enableAuth {
			cfg.mu.Lock()
			cfg.stats.SkippedAuth++
			cfg.mu.Unlock()
			return handler(ctx, req)
		}

		// Extract and validate token
		token, err := extractToken(ctx, cfg.tokenPrefix)
		if err != nil {
			cfg.mu.Lock()
			cfg.stats.FailedAuth++
			cfg.mu.Unlock()

			cfg.logger.Warn("Authentication failed",
				"method", info.FullMethod,
				"error", err.Error())

			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		// Validate token
		if !cfg.isValidToken(token) {
			cfg.mu.Lock()
			cfg.stats.FailedAuth++
			cfg.mu.Unlock()

			cfg.logger.Warn("Invalid authentication token",
				"method", info.FullMethod,
				"token_prefix", maskToken(token))

			return nil, status.Error(codes.Unauthenticated, "invalid authentication token")
		}

		// Authentication successful
		cfg.mu.Lock()
		cfg.stats.SuccessAuth++
		cfg.mu.Unlock()

		cfg.logger.Debug("Authentication successful",
			"method", info.FullMethod,
			"token_prefix", maskToken(token))

		// Call the handler
		return handler(ctx, req)
	}
}

// authStreamInterceptor authenticates streaming RPC calls
func authStreamInterceptor(cfg *AuthInterceptorConfig) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Update stats
		cfg.mu.Lock()
		cfg.stats.TotalRequests++
		cfg.mu.Unlock()

		// Check if auth is enabled
		if !cfg.enableAuth {
			cfg.mu.Lock()
			cfg.stats.SkippedAuth++
			cfg.mu.Unlock()
			return handler(srv, stream)
		}

		// Extract and validate token
		token, err := extractToken(stream.Context(), cfg.tokenPrefix)
		if err != nil {
			cfg.mu.Lock()
			cfg.stats.FailedAuth++
			cfg.mu.Unlock()

			cfg.logger.Warn("Stream authentication failed",
				"method", info.FullMethod,
				"error", err.Error())

			return status.Error(codes.Unauthenticated, err.Error())
		}

		// Validate token
		if !cfg.isValidToken(token) {
			cfg.mu.Lock()
			cfg.stats.FailedAuth++
			cfg.mu.Unlock()

			cfg.logger.Warn("Invalid stream authentication token",
				"method", info.FullMethod,
				"token_prefix", maskToken(token))

			return status.Error(codes.Unauthenticated, "invalid authentication token")
		}

		// Authentication successful
		cfg.mu.Lock()
		cfg.stats.SuccessAuth++
		cfg.mu.Unlock()

		cfg.logger.Debug("Stream authentication successful",
			"method", info.FullMethod,
			"token_prefix", maskToken(token))

		// Call the handler
		return handler(srv, stream)
	}
}

// extractToken extracts the authentication token from the context metadata
func extractToken(ctx context.Context, prefix string) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", types.NewError(types.ErrCodePermission, "no metadata provided")
	}

	values := md.Get(authorizationKey)
	if len(values) == 0 {
		return "", types.NewError(types.ErrCodePermission, "no authorization token provided")
	}

	authHeader := values[0]

	// Check if the token has the expected prefix
	if prefix != "" && !strings.HasPrefix(authHeader, prefix) {
		return "", types.NewError(types.ErrCodePermission, fmt.Sprintf("authorization token must start with %q", prefix))
	}

	// Extract token (remove prefix if present)
	token := authHeader
	if prefix != "" {
		token = strings.TrimPrefix(authHeader, prefix)
	}

	if token == "" {
		return "", types.NewError(types.ErrCodePermission, "empty authorization token")
	}

	return token, nil
}

// isValidToken checks if a token is valid
func (cfg *AuthInterceptorConfig) isValidToken(token string) bool {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	// Check if token exists in valid tokens map
	if _, exists := cfg.validTokens[token]; exists {
		return true
	}

	return false
}

// maskToken masks a token for safe logging (shows first 4 and last 4 characters)
func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// AddAuthToken adds a new authentication token to the interceptor config
func (cfg *AuthInterceptorConfig) AddAuthToken(token, description string) error {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if token == "" {
		return types.NewError(types.ErrCodeInvalid, "token cannot be empty")
	}

	cfg.validTokens[token] = description
	return nil
}

// RemoveAuthToken removes an authentication token from the interceptor config
func (cfg *AuthInterceptorConfig) RemoveAuthToken(token string) error {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if _, exists := cfg.validTokens[token]; !exists {
		return types.NewError(types.ErrCodeNotFound, "token not found")
	}

	delete(cfg.validTokens, token)
	return nil
}

// GetStats returns the current authentication statistics
func (cfg *AuthInterceptorConfig) GetStats() AuthStats {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	return cfg.stats
}

// String returns a string representation of the auth stats
func (s AuthStats) String() string {
	return fmt.Sprintf("AuthStats{Total: %d, Success: %d, Failed: %d, Skipped: %d}",
		s.TotalRequests, s.SuccessAuth, s.FailedAuth, s.SkippedAuth)
}

// ChainInterceptors chains multiple unary interceptors together into a single ServerOption.
func ChainInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.ServerOption {
	// If no interceptors are provided, return a no-op server option.
	if len(interceptors) == 0 {
		return grpc.EmptyServerOption{}
	}

	// Delegate to gRPC's built-in chaining helper.
	return grpc.ChainUnaryInterceptor(interceptors...)
}
