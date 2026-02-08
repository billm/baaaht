package grpc

import (
	"context"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestHealth(t *testing.T) {
	t.Run("Check_DefaultServing", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		ctx := context.Background()
		req := &grpc_health_v1.HealthCheckRequest{}

		resp, err := hs.Check(ctx, req)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("Expected SERVING status, got %v", resp.Status)
		}
	})

	t.Run("Check_EmptyService", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		ctx := context.Background()
		req := &grpc_health_v1.HealthCheckRequest{Service: ""}

		resp, err := hs.Check(ctx, req)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("Expected SERVING status, got %v", resp.Status)
		}
	})

	t.Run("Check_KnownService", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{
			InitialStatuses: map[string]grpc_health_v1.HealthCheckResponse_ServingStatus{
				"test-service": grpc_health_v1.HealthCheckResponse_SERVING,
			},
		}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		ctx := context.Background()
		req := &grpc_health_v1.HealthCheckRequest{Service: "test-service"}

		resp, err := hs.Check(ctx, req)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("Expected SERVING status, got %v", resp.Status)
		}
	})

	t.Run("Check_UnknownService", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		ctx := context.Background()
		req := &grpc_health_v1.HealthCheckRequest{Service: "unknown-service"}

		_, err = hs.Check(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unknown service, got nil")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}

		if st.Code() != codes.NotFound {
			t.Errorf("Expected NotFound code, got %v", st.Code())
		}
	})

	t.Run("Check_AfterShutdown", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		// Shutdown the server
		hs.Shutdown()

		ctx := context.Background()
		req := &grpc_health_v1.HealthCheckRequest{}

		resp, err := hs.Check(ctx, req)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if resp.Status != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Errorf("Expected NOT_SERVING status after shutdown, got %v", resp.Status)
		}
	})

	t.Run("SetServingStatus", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		// Set status to NOT_SERVING
		hs.SetServingStatus("test-service", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

		ctx := context.Background()
		req := &grpc_health_v1.HealthCheckRequest{Service: "test-service"}

		resp, err := hs.Check(ctx, req)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if resp.Status != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Errorf("Expected NOT_SERVING status, got %v", resp.Status)
		}

		// Set status back to SERVING
		hs.SetServing("test-service")

		resp, err = hs.Check(ctx, req)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("Expected SERVING status, got %v", resp.Status)
		}
	})

	t.Run("SetNotServing", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{
			InitialStatuses: map[string]grpc_health_v1.HealthCheckResponse_ServingStatus{
				"test-service": grpc_health_v1.HealthCheckResponse_SERVING,
			},
		}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		// Set to NOT_SERVING using convenience method
		hs.SetNotServing("test-service")

		ctx := context.Background()
		req := &grpc_health_v1.HealthCheckRequest{Service: "test-service"}

		resp, err := hs.Check(ctx, req)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if resp.Status != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Errorf("Expected NOT_SERVING status, got %v", resp.Status)
		}
	})

	t.Run("GetStatus", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{
			InitialStatuses: map[string]grpc_health_v1.HealthCheckResponse_ServingStatus{
				"test-service": grpc_health_v1.HealthCheckResponse_SERVING,
			},
		}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		status := hs.GetStatus("test-service")
		if status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("Expected SERVING status, got %v", status)
		}

		// Check unknown service returns default
		status = hs.GetStatus("unknown-service")
		if status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("Expected default SERVING status for unknown service, got %v", status)
		}
	})

	t.Run("IsServing", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{
			InitialStatuses: map[string]grpc_health_v1.HealthCheckResponse_ServingStatus{
				"test-service": grpc_health_v1.HealthCheckResponse_SERVING,
				"down-service": grpc_health_v1.HealthCheckResponse_NOT_SERVING,
			},
		}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		if !hs.IsServing("test-service") {
			t.Error("Expected test-service to be serving")
		}

		if hs.IsServing("down-service") {
			t.Error("Expected down-service to not be serving")
		}
	})

	t.Run("GetAllStatuses", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{
			InitialStatuses: map[string]grpc_health_v1.HealthCheckResponse_ServingStatus{
				"":                                       grpc_health_v1.HealthCheckResponse_SERVING,
				"orchestrator-service":                   grpc_health_v1.HealthCheckResponse_SERVING,
				"agent-service":                          grpc_health_v1.HealthCheckResponse_SERVING,
				"gateway-service":                        grpc_health_v1.HealthCheckResponse_NOT_SERVING,
			},
		}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		statuses := hs.GetAllStatuses()

		// Verify we got all statuses
		if len(statuses) != 4 {
			t.Errorf("Expected 4 statuses, got %d", len(statuses))
		}

		// Verify specific statuses
		if statuses["orchestrator-service"] != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Error("Expected orchestrator-service to be SERVING")
		}

		if statuses["gateway-service"] != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Error("Expected gateway-service to be NOT_SERVING")
		}

		// Modifying returned map should not affect internal state
		statuses["new-service"] = grpc_health_v1.HealthCheckResponse_SERVING
		statuses2 := hs.GetAllStatuses()
		if _, exists := statuses2["new-service"]; exists {
			t.Error("Modifying returned map should not affect internal state")
		}
	})

	t.Run("Watch_SendsInitialStatus", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{
			InitialStatuses: map[string]grpc_health_v1.HealthCheckResponse_ServingStatus{
				"": grpc_health_v1.HealthCheckResponse_SERVING,
			},
		}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		// Note: Full Watch testing requires a real gRPC stream which is complex to mock
		// The Watch method is tested via integration tests with actual gRPC connections
		// Here we just verify the method exists and can be called with a mock
		req := &grpc_health_v1.HealthCheckRequest{Service: ""}

		// Create a minimal mock stream
		mockStream := &mockHealthWatchStream{}

		// Watch should complete without error (even though we can't easily test the stream content)
		err = hs.Watch(req, mockStream)
		if err != nil {
			t.Fatalf("Watch failed: %v", err)
		}
	})

	t.Run("NewHealthServer_WithNilLogger", func(t *testing.T) {
		cfg := HealthServerConfig{}
		hs, err := NewHealthServer(cfg, nil)
		if err != nil {
			t.Fatalf("Failed to create health server with nil logger: %v", err)
		}

		if hs == nil {
			t.Error("Expected health server to be created")
		}

		if hs.logger == nil {
			t.Error("Expected logger to be initialized")
		}
	})

	t.Run("Shutdown_Idempotent", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		// Shutdown multiple times should not panic
		hs.Shutdown()
		hs.Shutdown()
		hs.Shutdown()

		// Verify all checks return NOT_SERVING
		ctx := context.Background()
		req := &grpc_health_v1.HealthCheckRequest{}

		resp, err := hs.Check(ctx, req)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if resp.Status != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Errorf("Expected NOT_SERVING status after multiple shutdowns, got %v", resp.Status)
		}
	})
}

// mockHealthWatchStream is a mock implementation of grpc_health_v1.Health_WatchServer for testing
type mockHealthWatchStream struct{}

func (m *mockHealthWatchStream) Send(resp *grpc_health_v1.HealthCheckResponse) error {
	return nil
}

func (m *mockHealthWatchStream) Context() context.Context {
	return context.Background()
}

func (m *mockHealthWatchStream) SendMsg(msg interface{}) error {
	return nil
}

func (m *mockHealthWatchStream) RecvMsg(msg interface{}) error {
	return nil
}

func (m *mockHealthWatchStream) SetHeader(md metadata.MD) error {
	return nil
}

func (m *mockHealthWatchStream) SendHeader(md metadata.MD) error {
	return nil
}

func (m *mockHealthWatchStream) SetTrailer(md metadata.MD) {
}

// Additional methods required by grpc.ServerStream
func (m *mockHealthWatchStream) SetCompression(compression string) error {
	return nil
}

func TestHealth_Integration(t *testing.T) {
	t.Run("FullWorkflow", func(t *testing.T) {
		logCfg := config.DefaultLoggingConfig()
		logCfg.Level = "error"
		log, err := logger.New(logCfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		cfg := HealthServerConfig{}
		hs, err := NewHealthServer(cfg, log)
		if err != nil {
			t.Fatalf("Failed to create health server: %v", err)
		}

		ctx := context.Background()

		// 1. Initial check should be SERVING
		resp, err := hs.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		if err != nil {
			t.Fatalf("Initial check failed: %v", err)
		}
		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("Expected initial SERVING status, got %v", resp.Status)
		}

		// 2. Set a service to NOT_SERVING
		hs.SetNotServing("test-service")
		resp, err = hs.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "test-service"})
		if err != nil {
			t.Fatalf("Service check failed: %v", err)
		}
		if resp.Status != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Errorf("Expected NOT_SERVING for test-service, got %v", resp.Status)
		}

		// 3. Set service back to SERVING
		hs.SetServing("test-service")
		resp, err = hs.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "test-service"})
		if err != nil {
			t.Fatalf("Service check failed: %v", err)
		}
		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("Expected SERVING for test-service, got %v", resp.Status)
		}

		// 4. Shutdown
		hs.Shutdown()
		resp, err = hs.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		if err != nil {
			t.Fatalf("Check after shutdown failed: %v", err)
		}
		if resp.Status != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Errorf("Expected NOT_SERVING after shutdown, got %v", resp.Status)
		}
	})
}
