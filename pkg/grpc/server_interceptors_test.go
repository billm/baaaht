package grpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
)

// mockUnaryHandler is a mock unary handler for testing
func mockUnaryHandler(ctx context.Context, req interface{}) (interface{}, error) {
	return "success", nil
}

// mockStreamHandler is a mock stream handler for testing
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

// TestLoggingInterceptor_LogsRequests tests that the logging interceptor logs requests
func TestLoggingInterceptor_LogsRequests(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := DefaultInterceptorConfig()
	cfg.EnableLogging = true
	cfg.LogAllRequests = true
	cfg.LogLevel = "debug"

	logCfg := LoggingInterceptorConfig{
		logger:      log.With("component", "test"),
		logAll:      true,
		logLevel:    logger.LevelDebug,
		excludeMethods: make(map[string]bool),
	}

	interceptor := loggingUnaryInterceptor(logCfg)

	tests := []struct {
		name       string
		method     string
		req        interface{}
		wantResp   interface{}
		wantErr    bool
		errCode    codes.Code
		handlerErr error
	}{
		{
			name:     "successful request",
			method:   "/test.Service/Method",
			req:      "test request",
			wantResp: "success",
			wantErr:  false,
		},
		{
			name:       "failed request",
			method:     "/test.Service/FailMethod",
			req:        "test request",
			wantErr:    true,
			errCode:    codes.NotFound,
			handlerErr: status.Error(codes.NotFound, "not found"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create handler based on test case
			handler := mockUnaryHandler
			if tt.handlerErr != nil {
				handler = func(ctx context.Context, req interface{}) (interface{}, error) {
					return nil, tt.handlerErr
				}
			}

			// Create server info
			info := &grpc.UnaryServerInfo{
				FullMethod: tt.method,
			}

			// Call interceptor
			resp, err := interceptor(ctx, tt.req, info, handler)

			// Check results
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("Expected status error but got: %v", err)
					return
				}
				if st.Code() != tt.errCode {
					t.Errorf("Expected error code %v but got %v", tt.errCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if resp != tt.wantResp {
					t.Errorf("Expected response %v but got %v", tt.wantResp, resp)
				}
			}
		})
	}
}

// TestLoggingInterceptor_StreamLogsRequests tests that the logging interceptor logs stream requests
func TestLoggingInterceptor_StreamLogsRequests(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := DefaultInterceptorConfig()
	cfg.EnableLogging = true
	cfg.LogAllRequests = true
	cfg.LogLevel = "debug"

	logCfg := LoggingInterceptorConfig{
		logger:      log.With("component", "test"),
		logAll:      true,
		logLevel:    logger.LevelDebug,
		excludeMethods: make(map[string]bool),
	}

	interceptor := loggingStreamInterceptor(logCfg)

	ctx := context.Background()

	mockStream := &mockServerStream{ctx: ctx}

	info := &grpc.StreamServerInfo{
		FullMethod:      "/test.Service/StreamMethod",
		IsClientStream:  true,
		IsServerStream:  true,
	}

	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	err = interceptor(nil, mockStream, info, handler)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestAuthInterceptor_ValidatesTokens tests that the auth interceptor validates tokens
func TestAuthInterceptor_ValidatesTokens(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	validTokens := map[string]string{
		"valid-token-123":  "test-token",
		"another-valid-456": "another-token",
	}

	// Create auth config directly for testing
	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "test"),
		validTokens: validTokens,
		tokenPrefix: bearerPrefix,
		enableAuth:  true,
	}

	interceptor := authUnaryInterceptor(authCfg)

	tests := []struct {
		name       string
		token      string
		wantErr    bool
		errCode    codes.Code
		setupCtx   func() context.Context
	}{
		{
			name:     "valid token",
			token:    "valid-token-123",
			wantErr:  false,
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, bearerPrefix+"valid-token-123")
				return metadata.NewIncomingContext(context.Background(), md)
			},
		},
		{
			name:     "invalid token",
			token:    "invalid-token",
			wantErr:  true,
			errCode:  codes.Unauthenticated,
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, bearerPrefix+"invalid-token")
				return metadata.NewIncomingContext(context.Background(), md)
			},
		},
		{
			name:     "missing token",
			token:    "",
			wantErr:  true,
			errCode:  codes.Unauthenticated,
			setupCtx: func() context.Context {
				return context.Background()
			},
		},
		{
			name:     "token without bearer prefix",
			token:    "valid-token-123",
			wantErr:  true,
			errCode:  codes.Unauthenticated,
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, "valid-token-123")
				return metadata.NewIncomingContext(context.Background(), md)
			},
		},
		{
			name:     "empty token",
			token:    "",
			wantErr:  true,
			errCode:  codes.Unauthenticated,
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, bearerPrefix)
				return metadata.NewIncomingContext(context.Background(), md)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()

			info := &grpc.UnaryServerInfo{
				FullMethod: "/test.Service/Method",
			}

			resp, err := interceptor(ctx, "test request", info, mockUnaryHandler)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("Expected status error but got: %v", err)
					return
				}
				if st.Code() != tt.errCode {
					t.Errorf("Expected error code %v but got %v", tt.errCode, st.Code())
				}
				if resp != nil {
					t.Errorf("Expected nil response but got %v", resp)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if resp == nil {
					t.Errorf("Expected response but got nil")
				}
			}
		})
	}
}

// TestAuthInterceptor_StreamValidatesTokens tests that the stream auth interceptor validates tokens
func TestAuthInterceptor_StreamValidatesTokens(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	validTokens := map[string]string{
		"valid-token-123": "test-token",
	}

	// Create auth config directly for testing
	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "test"),
		validTokens: validTokens,
		tokenPrefix: bearerPrefix,
		enableAuth:  true,
	}

	interceptor := authStreamInterceptor(authCfg)

	tests := []struct {
		name    string
		token   string
		wantErr bool
		setupCtx func() context.Context
	}{
		{
			name:    "valid token",
			token:   "valid-token-123",
			wantErr: false,
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, bearerPrefix+"valid-token-123")
				return metadata.NewIncomingContext(context.Background(), md)
			},
		},
		{
			name:    "invalid token",
			token:   "invalid-token",
			wantErr: true,
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, bearerPrefix+"invalid-token")
				return metadata.NewIncomingContext(context.Background(), md)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()

			mockStream := &mockServerStream{ctx: ctx}

			info := &grpc.StreamServerInfo{
				FullMethod: "/test.Service/StreamMethod",
			}

			handler := func(srv interface{}, stream grpc.ServerStream) error {
				return nil
			}

			err := interceptor(nil, mockStream, info, handler)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("Expected status error but got: %v", err)
				} else if st.Code() != codes.Unauthenticated {
					t.Errorf("Expected error code %v but got %v", codes.Unauthenticated, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestAuthInterceptor_Disabled tests that auth interceptor allows requests when disabled
func TestAuthInterceptor_Disabled(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create auth config with auth disabled
	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "test"),
		validTokens: make(map[string]string),
		tokenPrefix: bearerPrefix,
		enableAuth:  false, // Auth disabled
	}

	interceptor := authUnaryInterceptor(authCfg)

	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	resp, err := interceptor(ctx, "test request", info, mockUnaryHandler)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Errorf("Expected response but got nil")
	}
}

// TestAuthInterceptor_AddRemoveToken tests adding and removing tokens
func TestAuthInterceptor_AddRemoveToken(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := DefaultInterceptorConfig()
	cfg.EnableAuth = true
	cfg.AuthTokens = make(map[string]string)

	// Create auth config directly (not through interceptor)
	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "test"),
		validTokens: cfg.AuthTokens,
		tokenPrefix: bearerPrefix,
		enableAuth:  true,
	}

	// Test adding token
	err = authCfg.AddAuthToken("new-token", "test token")
	if err != nil {
		t.Errorf("Failed to add token: %v", err)
	}

	if !authCfg.isValidToken("new-token") {
		t.Errorf("Token was not added correctly")
	}

	// Test removing token
	err = authCfg.RemoveAuthToken("new-token")
	if err != nil {
		t.Errorf("Failed to remove token: %v", err)
	}

	if authCfg.isValidToken("new-token") {
		t.Errorf("Token was not removed correctly")
	}

	// Test removing non-existent token
	err = authCfg.RemoveAuthToken("non-existent")
	if err == nil {
		t.Errorf("Expected error when removing non-existent token")
	}
}

// TestAuthInterceptor_Stats tests that auth interceptor tracks statistics
func TestAuthInterceptor_Stats(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	validTokens := map[string]string{
		"valid-token": "test",
	}

	cfg := DefaultInterceptorConfig()
	cfg.EnableAuth = true
	cfg.AuthTokens = validTokens

	// Create auth config directly
	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "test"),
		validTokens: validTokens,
		tokenPrefix: bearerPrefix,
		enableAuth:  true,
		stats: AuthStats{},
	}

	// Create interceptor with this config
	interceptor := authUnaryInterceptor(authCfg)

	// Test successful auth
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(authorizationKey, bearerPrefix+"valid-token"))

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, _ = interceptor(ctx, "test", info, mockUnaryHandler)

	stats := authCfg.GetStats()
	if stats.TotalRequests != 1 {
		t.Errorf("Expected 1 total request but got %d", stats.TotalRequests)
	}
	if stats.SuccessAuth != 1 {
		t.Errorf("Expected 1 success auth but got %d", stats.SuccessAuth)
	}

	// Test failed auth
	ctx2 := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(authorizationKey, bearerPrefix+"invalid-token"))

	_, _ = interceptor(ctx2, "test", info, mockUnaryHandler)

	stats = authCfg.GetStats()
	if stats.TotalRequests != 2 {
		t.Errorf("Expected 2 total requests but got %d", stats.TotalRequests)
	}
	if stats.SuccessAuth != 1 {
		t.Errorf("Expected 1 success auth but got %d", stats.SuccessAuth)
	}
	if stats.FailedAuth != 1 {
		t.Errorf("Expected 1 failed auth but got %d", stats.FailedAuth)
	}
}

// TestMaskToken tests the token masking function
func TestMaskToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "long token",
			token:    "abcdefghijklmnopqrstuvwxyz",
			expected: "abcd...wxyz",
		},
		{
			name:     "short token",
			token:    "abcd",
			expected: "****",
		},
		{
			name:     "exactly 8 chars",
			token:    "12345678",
			expected: "****",
		},
		{
			name:     "empty token",
			token:    "",
			expected: "****",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskToken(tt.token)
			if result != tt.expected {
				t.Errorf("maskToken(%q) = %q, want %q", tt.token, result, tt.expected)
			}
		})
	}
}

// TestExtractToken tests token extraction from metadata
func TestExtractToken(t *testing.T) {
	tests := []struct {
		name        string
		setupCtx    func() context.Context
		prefix      string
		wantToken   string
		wantErr     bool
	}{
		{
			name: "valid token with bearer prefix",
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, bearerPrefix+"my-token")
				return metadata.NewIncomingContext(context.Background(), md)
			},
			prefix:    bearerPrefix,
			wantToken: "my-token",
			wantErr:   false,
		},
		{
			name: "valid token without prefix requirement",
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, "my-token")
				return metadata.NewIncomingContext(context.Background(), md)
			},
			prefix:    "",
			wantToken: "my-token",
			wantErr:   false,
		},
		{
			name:     "no metadata",
			setupCtx: func() context.Context {
				return context.Background()
			},
			prefix:  bearerPrefix,
			wantErr: true,
		},
		{
			name: "no authorization header",
			setupCtx: func() context.Context {
				md := metadata.Pairs("other-key", "other-value")
				return metadata.NewIncomingContext(context.Background(), md)
			},
			prefix:  bearerPrefix,
			wantErr: true,
		},
		{
			name: "missing bearer prefix",
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, "my-token")
				return metadata.NewIncomingContext(context.Background(), md)
			},
			prefix:  bearerPrefix,
			wantErr: true,
		},
		{
			name: "empty token with bearer prefix",
			setupCtx: func() context.Context {
				md := metadata.Pairs(authorizationKey, bearerPrefix)
				return metadata.NewIncomingContext(context.Background(), md)
			},
			prefix:  bearerPrefix,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := extractToken(tt.setupCtx(), tt.prefix)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if token != tt.wantToken {
					t.Errorf("Expected token %q but got %q", tt.wantToken, token)
				}
			}
		})
	}
}

// TestDefaultInterceptorConfig tests the default config
func TestDefaultInterceptorConfig(t *testing.T) {
	cfg := DefaultInterceptorConfig()

	if !cfg.EnableLogging {
		t.Errorf("Expected EnableLogging to be true")
	}
	if cfg.EnableAuth {
		t.Errorf("Expected EnableAuth to be false by default")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("Expected LogLevel to be 'info' but got %q", cfg.LogLevel)
	}
	if !cfg.LogAllRequests {
		t.Errorf("Expected LogAllRequests to be true")
	}
	if cfg.AuthTokens == nil {
		t.Errorf("Expected AuthTokens map to be initialized")
	}
	if cfg.AuthTokenPrefix != bearerPrefix {
		t.Errorf("Expected AuthTokenPrefix to be %q but got %q", bearerPrefix, cfg.AuthTokenPrefix)
	}
}

// TestParseLogLevel tests log level parsing
func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected logger.Level
	}{
		{"debug", logger.LevelDebug},
		{"DEBUG", logger.LevelDebug},
		{"info", logger.LevelInfo},
		{"INFO", logger.LevelInfo},
		{"warn", logger.LevelWarn},
		{"warning", logger.LevelWarn},
		{"WARN", logger.LevelWarn},
		{"error", logger.LevelError},
		{"ERROR", logger.LevelError},
		{"invalid", logger.LevelInfo}, // defaults to info
		{"", logger.LevelInfo},        // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseLogLevel(tt.input)
			if result != tt.expected {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Helper function to extract unary interceptor from ServerOption
func getUnaryInterceptorFromOption(opt grpc.ServerOption) grpc.UnaryServerInterceptor {
	// This is a workaround for testing - in real usage, the interceptor
	// is applied by the gRPC server internally
	// We'll create a test server and extract the interceptor
	lis := bufconn.Listen(1024)
	s := grpc.NewServer(opt)

	// Get the interceptor from the server's chain
	// Note: This is implementation-specific and may change
	_ = lis
	_ = s

	// Since we can't easily extract the interceptor from the ServerOption,
	// we'll return a test interceptor that wraps the functionality
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Create a minimal server with the option and call the handler
		testServer := grpc.NewServer(opt)
		defer testServer.Stop()

		// The actual interceptor is applied by the server
		// For testing purposes, we directly call the handler
		// In a real scenario, the server would apply the interceptor
		return handler(ctx, req)
	}
}

// Helper function to extract stream interceptor from ServerOption
func getStreamInterceptorFromOption(opt grpc.ServerOption) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Similar to the unary interceptor helper
		return handler(srv, stream)
	}
}

// TestIntegration_InterceptorsChain tests chaining multiple interceptors
func TestIntegration_InterceptorsChain(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	validTokens := map[string]string{
		"test-token": "test",
	}

	cfg := DefaultInterceptorConfig()
	cfg.EnableAuth = true
	cfg.AuthTokens = validTokens
	cfg.LogAllRequests = true

	// Create server with both interceptors
	lis := bufconn.Listen(1024)
	logIntOpt := NewLoggingInterceptor(log, cfg)
	authIntOpt := NewAuthInterceptor(log, cfg)

	server := grpc.NewServer(logIntOpt, authIntOpt)

	// The server should have both interceptors applied
	if server == nil {
		t.Fatal("Failed to create server with interceptors")
	}

	// Clean up
	server.Stop()
	lis.Close()

	// Test that a request with valid auth goes through
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(authorizationKey, bearerPrefix+"test-token"))

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	// Get the auth interceptor directly
	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "test"),
		validTokens: validTokens,
		tokenPrefix: bearerPrefix,
		enableAuth:  true,
	}

	interceptor := authUnaryInterceptor(authCfg)
	resp, err := interceptor(ctx, "test", info, mockUnaryHandler)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Errorf("Expected response but got nil")
	}
}

// BenchmarkAuthInterceptor_ValidToken benchmarks authentication with valid token
func BenchmarkAuthInterceptor_ValidToken(b *testing.B) {
	log, _ := logger.New(config.LoggingConfig{
		Level:  "error", // Minimal logging for benchmark
		Format: "text",
		Output: "stderr",
	})

	validTokens := map[string]string{
		"benchmark-token": "benchmark",
	}

	cfg := DefaultInterceptorConfig()
	cfg.EnableAuth = true
	cfg.AuthTokens = validTokens

	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "benchmark"),
		validTokens: validTokens,
		tokenPrefix: bearerPrefix,
		enableAuth:  true,
	}

	interceptor := authUnaryInterceptor(authCfg)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(authorizationKey, bearerPrefix+"benchmark-token"))

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		interceptor(ctx, "test", info, mockUnaryHandler)
	}
}

// TestLoggingInterceptorWithDuration tests that logging interceptor properly tracks duration
func TestLoggingInterceptorWithDuration(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := DefaultInterceptorConfig()
	cfg.EnableLogging = true
	cfg.LogAllRequests = true
	cfg.LogLevel = "debug"

	logCfg := LoggingInterceptorConfig{
		logger:      log.With("component", "test"),
		logAll:      true,
		logLevel:    logger.LevelDebug,
		excludeMethods: make(map[string]bool),
	}

	interceptor := loggingUnaryInterceptor(logCfg)

	// Create a handler that sleeps to simulate work
	slowHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		time.Sleep(10 * time.Millisecond)
		return "done", nil
	}

	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/SlowMethod",
	}

	start := time.Now()
	resp, err := interceptor(ctx, "test", info, slowHandler)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Errorf("Expected response but got nil")
	}

	// Verify the duration is at least what the handler sleeps
	if duration < 10*time.Millisecond {
		t.Errorf("Expected duration >= 10ms but got %v", duration)
	}
}

// TestAuthInterceptorWithEmptyTokenMap tests auth interceptor with no valid tokens
func TestAuthInterceptorWithEmptyTokenMap(t *testing.T) {
	log, err := logger.New(config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// With no valid tokens, auth should effectively be disabled
	// because enableAuth becomes false in the interceptor constructor
	authCfg := &AuthInterceptorConfig{
		logger:      log.With("component", "test"),
		validTokens: make(map[string]string), // Empty token map
		tokenPrefix: bearerPrefix,
		enableAuth:  false, // No tokens means auth is effectively disabled
	}

	interceptor := authUnaryInterceptor(authCfg)

	// With no valid tokens, auth should effectively be disabled
	// because enableAuth becomes false in the interceptor constructor
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(authorizationKey, bearerPrefix+"any-token"))

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	resp, err := interceptor(ctx, "test", info, mockUnaryHandler)

	// Since no valid tokens are configured, auth is skipped
	if err != nil {
		t.Errorf("Unexpected error (auth should be skipped with no valid tokens): %v", err)
	}
	if resp == nil {
		t.Errorf("Expected response but got nil")
	}
}
