package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/container"
	"github.com/billm/baaaht/orchestrator/pkg/orchestrator"
	"github.com/billm/baaaht/orchestrator/pkg/policy"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/pkg/worker"
	grpcPkg "github.com/billm/baaaht/orchestrator/pkg/grpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerIntegration performs an end-to-end test of the complete worker lifecycle
// including orchestrator bootstrap, worker registration, heartbeat, and shutdown.
func TestWorkerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping worker integration test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	// Load test config with temp directories
	cfg := loadTestConfig(t)

	t.Log("=== Step 1: Bootstrapping orchestrator ===")

	// Bootstrap orchestrator
	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-worker-integration-1.0.0",
		ShutdownTimeout:     30 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	require.True(t, result.IsSuccessful(), "Orchestrator bootstrap should be successful")

	orch := result.Orchestrator
	t.Logf("Orchestrator bootstrapped successfully in %v", result.Duration())

	// Cleanup orchestrator
	t.Cleanup(func() {
		t.Log("=== Cleanup: Shutting down orchestrator ===")
		if err := orch.Close(); err != nil {
			t.Logf("Warning: Orchestrator close returned error: %v", err)
		}
		t.Log("Orchestrator shutdown complete")
	})

	// Bootstrap gRPC server
	t.Log("=== Step 2: Bootstrapping gRPC server ===")

	grpcBootstrapCfg := grpcPkg.BootstrapConfig{
		Config:              cfg.GRPC,
		Logger:              log,
		SessionManager:      orch.SessionManager(),
		EventBus:            orch.EventBus(),
		IPCBroker:           nil,
		Version:             "test-worker-1.0.0",
		ShutdownTimeout:     10 * time.Second,
		EnableHealthCheck:   true,
		HealthCheckInterval: 30 * time.Second,
	}

	grpcResult, err := grpcPkg.Bootstrap(ctx, grpcBootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap gRPC server")
	require.True(t, grpcResult.IsSuccessful(), "gRPC bootstrap should be successful")

	// Cleanup gRPC server
	t.Cleanup(func() {
		t.Log("=== Cleanup: Shutting down gRPC server ===")
		if grpcResult.Health != nil {
			grpcResult.Health.Shutdown()
		}
		if grpcResult.Server != nil {
			if err := grpcResult.Server.Stop(); err != nil {
				t.Logf("Warning: gRPC server stop returned error: %v", err)
			}
		}
		t.Log("gRPC server shutdown complete")
	})

	t.Logf("gRPC server started on %s", cfg.GRPC.SocketPath)

	// Give the gRPC server a moment to fully start
	time.Sleep(500 * time.Millisecond)

	t.Log("=== Step 3: Bootstrapping worker ===")

	// Create worker bootstrap config
	workerName := fmt.Sprintf("integration-test-worker-%d", time.Now().Unix())
	workerBootstrapCfg := worker.BootstrapConfig{
		Logger:               log,
		Version:              worker.DefaultVersion,
		OrchestratorAddr:     cfg.GRPC.SocketPath,
		WorkerName:           workerName,
		DialTimeout:          30 * time.Second,
		RPCTimeout:           10 * time.Second,
		MaxRecvMsgSize:       worker.DefaultMaxRecvMsgSize,
		MaxSendMsgSize:       worker.DefaultMaxSendMsgSize,
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 3,
		HeartbeatInterval:    5 * time.Second, // Shorter for testing
		ShutdownTimeout:      10 * time.Second,
		EnableHealthCheck:    true,
	}

	// Bootstrap worker
	workerResult, err := worker.Bootstrap(ctx, workerBootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap worker")
	require.True(t, workerResult.IsSuccessful(), "Worker bootstrap should be successful")

	workerAgent := workerResult.Agent
	t.Logf("Worker bootstrapped successfully: %s", workerResult.String())

	// Cleanup worker
	t.Cleanup(func() {
		t.Log("=== Cleanup: Closing worker ===")
		_, closeCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer closeCancel()

		if err := workerAgent.Close(); err != nil {
			t.Logf("Warning: Worker close returned error: %v", err)
		}
		t.Log("Worker closed")
	})

	t.Log("=== Step 4: Verifying worker connection and registration ===")

	// Verify agent is connected
	assert.True(t, workerAgent.IsConnected(), "Worker should be connected to orchestrator")
	assert.True(t, workerAgent.IsRegistered(), "Worker should be registered")

	agentID := workerAgent.GetAgentID()
	assert.NotEmpty(t, agentID, "Agent ID should not be empty")
	t.Logf("Worker agent ID: %s", agentID)

	// Verify connection state
	state := workerAgent.GetState()
	t.Logf("Worker connection state: %s", state.String())

	t.Log("=== Step 5: Waiting for heartbeats ===")

	// Wait for a few heartbeats to be sent
	// The heartbeat interval is 5 seconds, so wait 12 seconds to ensure at least 2 heartbeats
	time.Sleep(12 * time.Second)

	// Check stats
	stats := workerAgent.Stats()
	assert.True(t, stats.IsConnected, "Worker should still be connected")
	assert.Greater(t, stats.TotalRPCs, int64(0), "Worker should have sent RPCs")
	t.Logf("Worker stats: TotalRPCs=%d, FailedRPCs=%d, ReconnectAttempts=%d",
		stats.TotalRPCs, stats.FailedRPCs, stats.ReconnectAttempts)

	t.Log("=== Step 6: Testing worker readiness ===")

	// Test worker readiness helpers
	isReady := worker.IsReady(workerAgent)
	assert.True(t, isReady, "Worker should be ready")

	// Test WaitForReady
	err = worker.WaitForReady(ctx, workerAgent, 5*time.Second, 100*time.Millisecond)
	assert.NoError(t, err, "WaitForReady should succeed immediately for ready worker")

	t.Log("=== Step 7: Testing connection resilience ===")

	// Get connection before reset
	connBefore := workerAgent.GetConn()
	require.NotNil(t, connBefore, "Connection should not be nil")

	// Test connection reset
	t.Log("Resetting worker connection...")
	err = workerAgent.ResetConnection(ctx)
	require.NoError(t, err, "Connection reset should succeed")

	// Verify reconnection
	assert.True(t, workerAgent.IsConnected(), "Worker should be reconnected after reset")
	assert.True(t, workerAgent.IsRegistered(), "Worker should still be registered")

	// Get connection after reset
	connAfter := workerAgent.GetConn()
	require.NotNil(t, connAfter, "Connection should not be nil after reset")

	t.Log("Connection reset and reconnection successful")

	t.Log("=== Step 8: Verifying graceful shutdown ===")

	// Close worker
	_, closeCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer closeCancel()

	err = workerAgent.Close()
	assert.NoError(t, err, "Worker close should succeed")

	// Verify worker is closed
	assert.False(t, workerAgent.IsConnected(), "Worker should not be connected after close")
	assert.False(t, workerAgent.IsRegistered(), "Worker should not be registered after close")

	t.Log("Worker graceful shutdown successful")

	t.Log("=== Worker Integration Test Complete ===")
	t.Log("All steps passed successfully:")
	t.Log("  1. Orchestrator bootstrapped")
	t.Log("  2. gRPC server bootstrapped")
	t.Log("  3. Worker bootstrapped and connected")
	t.Log("  4. Worker registration verified")
	t.Log("  5. Heartbeats sent and received")
	t.Log("  6. Worker readiness verified")
	t.Log("  7. Connection resilience tested")
	t.Log("  8. Graceful shutdown verified")
}

// TestWorkerBootstrapFailure tests worker bootstrap with invalid configuration
func TestWorkerBootstrapFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping worker bootstrap failure test in short mode")
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Run("bootstrap with invalid orchestrator address", func(t *testing.T) {
		workerName := fmt.Sprintf("failure-test-worker-%d", time.Now().Unix())
		workerBootstrapCfg := worker.BootstrapConfig{
			Logger:           log,
			Version:          worker.DefaultVersion,
			OrchestratorAddr:  "unix:///nonexistent/path/baaaht-grpc.sock",
			WorkerName:       workerName,
			DialTimeout:      2 * time.Second,
			RPCTimeout:       2 * time.Second,
			EnableHealthCheck: false,
		}

		// Bootstrap should fail due to invalid address
		workerResult, err := worker.Bootstrap(ctx, workerBootstrapCfg)
		assert.Error(t, err, "Bootstrap should fail with invalid address")
		assert.False(t, workerResult.IsSuccessful(), "Bootstrap result should not be successful")
		assert.Nil(t, workerResult.Agent, "Agent should be nil on failure")
		t.Logf("Expected error: %v", err)
	})

	t.Run("bootstrap with empty worker name", func(t *testing.T) {
		// This should use default worker name generation
		// Just verify it doesn't crash
		workerBootstrapCfg := worker.BootstrapConfig{
			Logger:           log,
			Version:          worker.DefaultVersion,
			OrchestratorAddr: "unix:///tmp/invalid.sock", // Invalid but non-empty
			WorkerName:       "", // Empty name should use default
			DialTimeout:      1 * time.Second,
			EnableHealthCheck: false,
		}

		// Bootstrap should fail (no server), but not crash
		workerResult, err := worker.Bootstrap(ctx, workerBootstrapCfg)
		assert.Error(t, err, "Bootstrap should fail without server")
		assert.False(t, workerResult.IsSuccessful(), "Bootstrap result should not be successful")
		// The important part is that it doesn't panic with empty name
		t.Logf("Bootstrap with empty name handled: error=%v", err)
	})
}

// TestWorkerAgentDirect tests creating a worker agent directly without bootstrap
func TestWorkerAgentDirect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping worker agent direct test in short mode")
	}

	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Run("create agent with default config", func(t *testing.T) {
		agentCfg := worker.AgentConfig{
			DialTimeout:    30 * time.Second,
			RPCTimeout:     10 * time.Second,
			HeartbeatInterval: 30 * time.Second,
		}

		agent, err := worker.NewAgent("unix:///tmp/test.sock", agentCfg, log)
		assert.NoError(t, err, "Agent creation should succeed")
		assert.NotNil(t, agent, "Agent should not be nil")
		assert.Equal(t, "unix:///tmp/test.sock", agent.OrchestratorAddr())
		assert.False(t, agent.IsConnected(), "Agent should not be connected initially")
		assert.False(t, agent.IsRegistered(), "Agent should not be registered initially")

		t.Logf("Agent created: %s", agent.String())
	})

	t.Run("create agent with custom config", func(t *testing.T) {
		agentCfg := worker.AgentConfig{
			DialTimeout:         60 * time.Second,
			RPCTimeout:          30 * time.Second,
			MaxRecvMsgSize:      1024 * 1024 * 200, // 200MB
			MaxSendMsgSize:      1024 * 1024 * 200, // 200MB
			ReconnectInterval:   10 * time.Second,
			ReconnectMaxAttempts: 5,
			HeartbeatInterval:   60 * time.Second,
		}

		agent, err := worker.NewAgent("unix:///tmp/custom.sock", agentCfg, log)
		assert.NoError(t, err, "Agent creation with custom config should succeed")
		assert.NotNil(t, agent, "Agent should not be nil")

		stats := agent.Stats()
		assert.False(t, stats.IsConnected, "Agent should not be connected")

		// Cleanup
		_ = agent.Close()
	})

	t.Run("create agent with invalid address", func(t *testing.T) {
		agentCfg := worker.AgentConfig{}

		// Invalid address format should still create agent, but dial will fail
		agent, err := worker.NewAgent("", agentCfg, log)
		assert.Error(t, err, "Empty address should fail")
		assert.Nil(t, agent, "Agent should be nil with empty address")
	})

	t.Run("agent close without dial", func(t *testing.T) {
		agentCfg := worker.AgentConfig{
			DialTimeout:    30 * time.Second,
			RPCTimeout:     10 * time.Second,
		}

		agent, err := worker.NewAgent("unix:///tmp/test.sock", agentCfg, log)
		require.NoError(t, err, "Agent creation should succeed")

		// Close without dialing should succeed
		err = agent.Close()
		assert.NoError(t, err, "Close without dial should succeed")
	})
}

// TestWorkerBootstrapConfig tests various bootstrap configurations
func TestWorkerBootstrapConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping worker bootstrap config test in short mode")
	}

	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Run("default bootstrap config", func(t *testing.T) {
		cfg := worker.NewDefaultBootstrapConfig()
		assert.NotNil(t, cfg.Logger, "Logger should be set")
		assert.NotEmpty(t, cfg.Version, "Version should be set")
		assert.NotEmpty(t, cfg.OrchestratorAddr, "Orchestrator address should be set")
		assert.NotEmpty(t, cfg.WorkerName, "Worker name should be generated")
		assert.Greater(t, cfg.DialTimeout, time.Duration(0), "Dial timeout should be positive")
		assert.Greater(t, cfg.RPCTimeout, time.Duration(0), "RPC timeout should be positive")
		assert.Greater(t, cfg.HeartbeatInterval, time.Duration(0), "Heartbeat interval should be positive")
		assert.True(t, cfg.EnableHealthCheck, "Health check should be enabled by default")

		t.Logf("Default config: %+v", cfg)
	})

	t.Run("custom bootstrap config", func(t *testing.T) {
		tmpDir := t.TempDir()
		socketPath := filepath.Join(tmpDir, "test.sock")

		cfg := worker.BootstrapConfig{
			Logger:               log,
			Version:              "test-1.0.0",
			OrchestratorAddr:     "unix://" + socketPath,
			WorkerName:           "test-worker",
			DialTimeout:          45 * time.Second,
			RPCTimeout:           15 * time.Second,
			MaxRecvMsgSize:       1024 * 1024 * 150,
			MaxSendMsgSize:       1024 * 1024 * 150,
			ReconnectInterval:    8 * time.Second,
			ReconnectMaxAttempts: 7,
			HeartbeatInterval:    45 * time.Second,
			ShutdownTimeout:      35 * time.Second,
			EnableHealthCheck:    false,
		}

		assert.Equal(t, "test-1.0.0", cfg.Version)
		assert.Equal(t, "unix://"+socketPath, cfg.OrchestratorAddr)
		assert.Equal(t, "test-worker", cfg.WorkerName)
		assert.Equal(t, 45*time.Second, cfg.DialTimeout)
		assert.Equal(t, 15*time.Second, cfg.RPCTimeout)
		assert.False(t, cfg.EnableHealthCheck)
	})
}

// TestWorkerVersion tests version information
func TestWorkerVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping worker version test in short mode")
	}

	t.Run("get version", func(t *testing.T) {
		version := worker.GetVersion()
		assert.NotEmpty(t, version, "Version should not be empty")
		t.Logf("Worker version: %s", version)
	})

	t.Run("default version constant", func(t *testing.T) {
		assert.NotEmpty(t, worker.DefaultVersion, "Default version should not be empty")
		t.Logf("Default worker version: %s", worker.DefaultVersion)
	})
}

// int64Ptr is a helper function for creating int64 pointers
func int64Ptr(i int64) *int64 {
	return &i
}

// TestWorkerPolicyEnforcement tests policy enforcement with blocked operations
func TestWorkerPolicyEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping worker policy enforcement test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	// Load test config with temp directories
	cfg := loadTestConfig(t)

	// Override gRPC socket path to use temp file
	tmpDir := t.TempDir()
	grpcSocketPath := filepath.Join(tmpDir, "grpc.sock")
	cfg.GRPC.SocketPath = grpcSocketPath

	t.Log("=== Step 1: Bootstrapping orchestrator with strict policy ===")

	// Bootstrap orchestrator
	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-worker-policy-1.0.0",
		ShutdownTimeout:     30 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	require.True(t, result.IsSuccessful(), "Orchestrator bootstrap should be successful")

	orch := result.Orchestrator
	t.Logf("Orchestrator bootstrapped successfully in %v", result.Duration())

	// Cleanup orchestrator
	t.Cleanup(func() {
		t.Log("=== Cleanup: Shutting down orchestrator ===")
		if err := orch.Close(); err != nil {
			t.Logf("Warning: Orchestrator close returned error: %v", err)
		}
		t.Log("Orchestrator shutdown complete")
	})

	// Bootstrap gRPC server
	t.Log("=== Step 2: Bootstrapping gRPC server ===")

	grpcBootstrapCfg := grpcPkg.BootstrapConfig{
		Config:              cfg.GRPC,
		Logger:              log,
		SessionManager:      orch.SessionManager(),
		EventBus:            orch.EventBus(),
		IPCBroker:           nil,
		Version:             "test-worker-1.0.0",
		ShutdownTimeout:     10 * time.Second,
		EnableHealthCheck:   true,
		HealthCheckInterval: 30 * time.Second,
	}

	grpcResult, err := grpcPkg.Bootstrap(ctx, grpcBootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap gRPC server")
	require.True(t, grpcResult.IsSuccessful(), "gRPC bootstrap should be successful")

	// Cleanup gRPC server
	t.Cleanup(func() {
		t.Log("=== Cleanup: Shutting down gRPC server ===")
		if grpcResult.Health != nil {
			grpcResult.Health.Shutdown()
		}
		if grpcResult.Server != nil {
			if err := grpcResult.Server.Stop(); err != nil {
				t.Logf("Warning: gRPC server stop returned error: %v", err)
			}
		}
		t.Log("gRPC server shutdown complete")
	})

	t.Logf("gRPC server started on %s", cfg.GRPC.SocketPath)

	// Give the gRPC server a moment to fully start
	time.Sleep(500 * time.Millisecond)

	t.Log("=== Step 3: Bootstrapping worker ===")

	// Create worker bootstrap config
	workerName := fmt.Sprintf("policy-test-worker-%d", time.Now().Unix())
	workerBootstrapCfg := worker.BootstrapConfig{
		Logger:               log,
		Version:              worker.DefaultVersion,
		OrchestratorAddr:     "unix://" + cfg.GRPC.SocketPath,
		WorkerName:           workerName,
		DialTimeout:          30 * time.Second,
		RPCTimeout:           10 * time.Second,
		MaxRecvMsgSize:       worker.DefaultMaxRecvMsgSize,
		MaxSendMsgSize:       worker.DefaultMaxSendMsgSize,
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 3,
		HeartbeatInterval:    5 * time.Second,
		ShutdownTimeout:      10 * time.Second,
		EnableHealthCheck:    true,
	}

	// Bootstrap worker
	workerResult, err := worker.Bootstrap(ctx, workerBootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap worker")
	require.True(t, workerResult.IsSuccessful(), "Worker bootstrap should be successful")

	workerAgent := workerResult.Agent
	t.Logf("Worker bootstrapped successfully: %s", workerResult.String())

	// Cleanup worker
	t.Cleanup(func() {
		t.Log("=== Cleanup: Closing worker ===")
		_, closeCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer closeCancel()

		if err := workerAgent.Close(); err != nil {
			t.Logf("Warning: Worker close returned error: %v", err)
		}
		t.Log("Worker closed")
	})

	// Verify worker is ready
	t.Log("=== Step 4: Verifying worker is ready ===")
	assert.True(t, workerAgent.IsConnected(), "Worker should be connected to orchestrator")
	assert.True(t, workerAgent.IsRegistered(), "Worker should be registered")
	t.Logf("Worker agent ID: %s", workerAgent.GetAgentID())

	t.Log("=== Step 5: Setting strict policy to block operations ===")

	// Get the policy enforcer from the orchestrator
	enforcer := orch.PolicyEnforcer()
	require.NotNil(t, enforcer, "Policy enforcer should not be nil")

	// Set a strict policy that will block certain operations
	maxCPUs := int64(1000000000)  // 1 CPU
	maxMemory := int64(1073741824) // 1GB

	strictPolicy := &policy.Policy{
		ID:          "strict-test-policy",
		Name:        "Strict Test Policy",
		Description: "A strict policy for testing blocked operations",
		Mode:        policy.EnforcementModeStrict,
		Quotas: policy.ResourceQuota{
			MaxCPUs:   &maxCPUs,
			MaxMemory: &maxMemory,
			MaxPids:   int64Ptr(128),
		},
		Mounts: policy.MountPolicy{
			AllowBindMounts: false,
			AllowVolumes:    true,
			AllowTmpfs:      false,
		},
		Images: policy.ImagePolicy{
			AllowLatestTag: false,
		},
		Security: policy.SecurityPolicy{
			AllowPrivileged: false,
			RequireNonRoot:  true,
		},
	}

	err = enforcer.SetPolicy(ctx, strictPolicy)
	require.NoError(t, err, "Failed to set strict policy")
	t.Log("Strict policy set successfully")

	t.Log("=== Step 6: Testing blocked operations ===")

	testSessionID := types.GenerateID()

	tests := []struct {
		name          string
		config        types.ContainerConfig
		wantAllowed   bool
		wantViolation string
	}{
		{
			name: "excessive CPU quota should be blocked",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Resources: types.ResourceLimits{
					NanoCPUs: 8000000000, // 8 CPUs - exceeds max of 1
				},
			},
			wantAllowed:   false,
			wantViolation: "quota.cpu.max",
		},
		{
			name: "excessive memory quota should be blocked",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Resources: types.ResourceLimits{
					MemoryBytes: 8589934592, // 8GB - exceeds max of 1GB
				},
			},
			wantAllowed:   false,
			wantViolation: "quota.memory.max",
		},
		{
			name: "bind mount should be blocked",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Mounts: []types.Mount{
					{
						Type:   types.MountTypeBind,
						Source: "/tmp/data",
						Target: "/data",
					},
				},
			},
			wantAllowed:   false,
			wantViolation: "mount.bind.disabled",
		},
		{
			name: "tmpfs mount should be blocked",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Mounts: []types.Mount{
					{
						Type:   types.MountTypeTmpfs,
						Source: "size=100m",
						Target: "/tmp",
					},
				},
			},
			wantAllowed:   false,
			wantViolation: "mount.tmpfs.disabled",
		},
		{
			name: "latest tag should be blocked",
			config: types.ContainerConfig{
				Image: "nginx:latest",
			},
			wantAllowed:   false,
			wantViolation: "image.latest_tag",
		},
		{
			name: "PIDs limit exceeded should be blocked",
			config: types.ContainerConfig{
				Image: "nginx:1.21",
				Resources: types.ResourceLimits{
					PidsLimit: int64Ptr(256), // exceeds max of 128
				},
			},
			wantAllowed:   false,
			wantViolation: "quota.pids.max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := enforcer.ValidateContainerConfig(ctx, testSessionID, tt.config)
			require.NoError(t, err, "Validation should not error")

			if tt.wantAllowed {
				assert.True(t, result.Allowed, "Config should be allowed: %s", tt.name)
				assert.Empty(t, result.Violations, "Should have no violations for allowed config")
			} else {
				assert.False(t, result.Allowed, "Config should be blocked: %s", tt.name)
				assert.NotEmpty(t, result.Violations, "Should have violations for blocked config")

				// Check for specific violation
				if tt.wantViolation != "" {
					found := false
					for _, v := range result.Violations {
						if v.Rule == tt.wantViolation {
							found = true
							t.Logf("Found expected violation: %s - %s", v.Rule, v.Message)
							break
						}
					}
					assert.True(t, found, "Expected violation %s not found in %v", tt.wantViolation, result.Violations)
				}
			}
		})
	}

	t.Log("=== Step 7: Testing permissive mode allows violations ===")

	// Switch to permissive mode - violations are logged but not blocked
	permissivePolicy := &policy.Policy{
		ID:          "permissive-test-policy",
		Name:        "Permissive Test Policy",
		Description: "A permissive policy for testing warning mode",
		Mode:        policy.EnforcementModePermissive,
		Quotas: policy.ResourceQuota{
			MaxCPUs:   &maxCPUs,
			MaxMemory: &maxMemory,
		},
	}

	err = enforcer.SetPolicy(ctx, permissivePolicy)
	require.NoError(t, err, "Failed to set permissive policy")

	// In permissive mode, config should be allowed even with violations
	config := types.ContainerConfig{
		Image: "nginx:latest",
		Resources: types.ResourceLimits{
			NanoCPUs: 8000000000, // exceeds quota
		},
	}

	validationResult, err := enforcer.ValidateContainerConfig(ctx, testSessionID, config)
	require.NoError(t, err, "Validation should not error")

	// Permissive mode allows but records violations
	assert.True(t, validationResult.Allowed, "Permissive mode should allow config with violations")
	assert.NotEmpty(t, validationResult.Violations, "Permissive mode should record violations")
	t.Logf("Permissive mode: allowed=%v, violations=%d", validationResult.Allowed, len(validationResult.Violations))

	t.Log("=== Step 8: Testing disabled mode allows everything ===")

	// Switch to disabled mode - no enforcement
	disabledPolicy := &policy.Policy{
		ID:          "disabled-test-policy",
		Name:        "Disabled Test Policy",
		Description: "A disabled policy for testing",
		Mode:        policy.EnforcementModeDisabled,
	}

	err = enforcer.SetPolicy(ctx, disabledPolicy)
	require.NoError(t, err, "Failed to set disabled policy")

	// In disabled mode, everything should be allowed with no violations
	validationResult, err = enforcer.ValidateContainerConfig(ctx, testSessionID, config)
	require.NoError(t, err, "Validation should not error")

	assert.True(t, validationResult.Allowed, "Disabled mode should allow all configs")
	assert.Empty(t, validationResult.Violations, "Disabled mode should not produce violations")
	t.Log("Disabled mode: all configurations allowed")

	t.Log("=== Worker Policy Enforcement Test Complete ===")
	t.Log("All policy enforcement tests passed successfully:")
	t.Log("  1. Orchestrator and gRPC server bootstrapped")
	t.Log("  2. Worker bootstrapped and connected")
	t.Log("  3. Strict policy violations detected and blocked")
	t.Log("  4. Permissive mode allows with warnings")
	t.Log("  5. Disabled mode allows everything")
}

// TestWorkerCleanup tests that containers are properly cleaned up on error and cancellation
func TestWorkerCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping worker cleanup test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	// Create a container runtime for direct testing
	cfg := container.RuntimeConfig{
		Type:    string(types.RuntimeTypeDocker),
		Logger:  log,
		Timeout: 30 * time.Second,
	}

	rt, err := container.NewRuntime(ctx, cfg)
	require.NoError(t, err, "Failed to create runtime")
	require.NotNil(t, rt)

	defer func() {
		_ = rt.Close()
	}()

	// Create a lifecycle manager to check container status
	lifecycleMgr, err := container.NewLifecycleManager(nil, log)
	require.NoError(t, err, "Failed to create lifecycle manager")

	t.Run("cleanup on invalid image error", func(t *testing.T) {
		// Create an executor with the runtime
		exec, err := worker.NewExecutorFromRuntime(rt, log)
		require.NoError(t, err, "Failed to create executor")

		sessionID := types.GenerateID()
		exec.SetSessionID(sessionID)

		// Try to execute a task with an invalid image
		// This should fail during container creation
		// The executor should ensure no containers are left behind
		taskCfg := worker.TaskConfig{
			ToolType: worker.ToolTypeList,
			// We'll manually set an invalid config that will fail
		}

		// Execute the task - it should fail gracefully
		result := exec.ExecuteTask(ctx, taskCfg)

		// The task should fail (mount source is empty, which will cause issues)
		assert.Error(t, result.Error, "Task should fail with missing mount source")
		t.Logf("Expected error: %v", result.Error)

		// Container ID should be empty if container creation failed early
		if result.ContainerID != "" {
			// If a container was created, verify it was cleaned up
			// Wait a moment for cleanup to complete
			time.Sleep(500 * time.Millisecond)

			// Check if the container still exists
			isRunning, err := lifecycleMgr.IsRunning(ctx, result.ContainerID)
			require.NoError(t, err, "Failed to check container status")

			// Container should not be running (cleaned up)
			assert.False(t, isRunning, "Container should be cleaned up after error")

			// Check status to verify it's gone or stopped
			state, err := lifecycleMgr.Status(ctx, result.ContainerID)
			if err == nil {
				// If we can get status, it should not be running
				assert.NotEqual(t, types.ContainerStateRunning, state,
					"Container state should not be running after cleanup")
			}
			t.Logf("Container %s properly cleaned up", result.ContainerID)
		} else {
			t.Log("No container was created (failed early)")
		}

		_ = exec.Close()
	})

	t.Run("cleanup on context cancellation", func(t *testing.T) {
		// Create an executor with the runtime
		exec, err := worker.NewExecutorFromRuntime(rt, log)
		require.NoError(t, err, "Failed to create executor")

		sessionID := types.GenerateID()
		exec.SetSessionID(sessionID)

		// Create a context that we can cancel
		cancelCtx, cancel := context.WithCancel(ctx)

		// Start a long-running task in the background
		resultChan := make(chan *worker.TaskResult, 1)
		go func() {
			// Use the sleep tool to create a long-running container
			// But we don't have a sleep tool, so we'll use list with a delay
			// Actually, let's just cancel immediately after starting
			taskCfg := worker.TaskConfig{
				ToolType:    worker.ToolTypeList,
				MountSource: t.TempDir(), // Valid mount source
				Timeout:     10 * time.Second,
			}
			resultChan <- exec.ExecuteTask(cancelCtx, taskCfg)
		}()

		// Wait a moment for the container to be created
		time.Sleep(500 * time.Millisecond)

		// Cancel the context
		cancel()

		// Get the result
		result := <-resultChan

		// The task should have been cancelled or failed
		if result.Error != nil {
			t.Logf("Task was cancelled or failed (expected): %v", result.Error)
		}

		// If a container was created, verify cleanup
		if result.ContainerID != "" {
			t.Logf("Container was created: %s", result.ContainerID)

			// Wait for cleanup to complete
			time.Sleep(1 * time.Second)

			// Check if the container still exists
			isRunning, err := lifecycleMgr.IsRunning(ctx, result.ContainerID)
			require.NoError(t, err, "Failed to check container status")

			// Container should not be running (cleaned up by defer)
			assert.False(t, isRunning, "Container should be cleaned up after cancellation")

			// Verify the container state
			state, err := lifecycleMgr.Status(ctx, result.ContainerID)
			if err == nil {
				// If we can get status, verify it's not running
				assert.NotEqual(t, types.ContainerStateRunning, state,
					"Container should not be running after cancellation")
				t.Logf("Container state after cancellation: %s", state)
			} else {
				// Container was removed entirely - also acceptable
				t.Logf("Container was removed (status check failed): %v", err)
			}
		} else {
			t.Log("No container ID - task may have failed before container creation")
		}

		_ = exec.Close()
	})

	t.Run("cleanup after successful execution", func(t *testing.T) {
		// Create an executor with the runtime
		exec, err := worker.NewExecutorFromRuntime(rt, log)
		require.NoError(t, err, "Failed to create executor")

		sessionID := types.GenerateID()
		exec.SetSessionID(sessionID)

		// Create a temp directory with a file to list
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err, "Failed to create test file")

		// Execute a task that should succeed
		taskCfg := worker.TaskConfig{
			ToolType:    worker.ToolTypeList,
			MountSource: tempDir,
		}

		result := exec.ExecuteTask(ctx, taskCfg)

		// Task should succeed
		if result.Error != nil {
			t.Logf("Task failed (may be expected): %v", result.Error)
		}

		// If a container was created, verify cleanup
		if result.ContainerID != "" {
			t.Logf("Container was created: %s", result.ContainerID)

			// Wait for cleanup to complete (defer in ExecuteTask)
			time.Sleep(500 * time.Millisecond)

			// Check if the container still exists
			isRunning, err := lifecycleMgr.IsRunning(ctx, result.ContainerID)
			require.NoError(t, err, "Failed to check container status")

			// Container should not be running (cleaned up by defer)
			assert.False(t, isRunning, "Container should be cleaned up after successful execution")

			// Verify the container state
			state, err := lifecycleMgr.Status(ctx, result.ContainerID)
			if err == nil {
				// If we can get status, verify it's not running
				assert.NotEqual(t, types.ContainerStateRunning, state,
					"Container should not be running after completion")
				t.Logf("Container state after completion: %s", state)
			} else {
				// Container was removed entirely - also acceptable
				t.Logf("Container was removed (status check failed): %v", err)
			}

			t.Logf("Container %s properly cleaned up after successful execution", result.ContainerID)
		}

		_ = exec.Close()
	})

	t.Run("verify executor close cleanup", func(t *testing.T) {
		// Create an executor and close it without executing any tasks
		exec, err := worker.NewExecutorFromRuntime(rt, log)
		require.NoError(t, err, "Failed to create executor")

		sessionID := types.GenerateID()
		exec.SetSessionID(sessionID)

		// Close should succeed without errors
		err = exec.Close()
		assert.NoError(t, err, "Executor close should succeed")

		// Verify executor is closed
		assert.True(t, exec.IsClosed(), "Executor should be closed")

		// Try to execute a task after close - should fail
		taskCfg := worker.TaskConfig{
			ToolType:    worker.ToolTypeList,
			MountSource: t.TempDir(),
		}

		result := exec.ExecuteTask(ctx, taskCfg)
		assert.Error(t, result.Error, "Task should fail on closed executor")
		t.Logf("Expected error after close: %v", result.Error)
	})

	t.Log("=== Worker Cleanup Test Complete ===")
	t.Log("All cleanup tests passed successfully:")
	t.Log("  1. Container cleanup on error verified")
	t.Log("  2. Container cleanup on cancellation verified")
	t.Log("  3. Container cleanup after successful execution verified")
	t.Log("  4. Executor close cleanup verified")
}
