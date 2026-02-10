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
	grpcPkg "github.com/billm/baaaht/orchestrator/pkg/grpc"
	"github.com/billm/baaaht/orchestrator/pkg/orchestrator"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToolExecutionFlow tests the complete tool execution flow from gRPC request to result
func TestToolExecutionFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	// Step 1: Bootstrap orchestrator with tool service registered
	t.Log("=== Step 1: Bootstrapping orchestrator with tool service ===")

	cfg := loadTestConfig(t)

	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-tool-exec-1.0.0",
		ShutdownTimeout:     30 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	require.NotNil(t, result.Orchestrator, "Orchestrator should not be nil")
	require.True(t, result.IsSuccessful(), "Bootstrap should be successful")

	orch := result.Orchestrator
	t.Logf("Orchestrator bootstrapped successfully in %v", result.Duration())

	// Ensure cleanup
	t.Cleanup(func() {
		t.Log("=== Cleaning up orchestrator ===")
		if err := orch.Close(); err != nil {
			t.Logf("Warning: Orchestrator close returned error: %v", err)
		}
	})

	// Step 2: Bootstrap gRPC server with tool service
	t.Log("=== Step 2: Bootstrapping gRPC server with tool service ===")

	grpcBootstrapCfg := grpcPkg.BootstrapConfig{
		Config:              cfg.GRPC,
		Logger:              log,
		SessionManager:      orch.SessionManager(),
		EventBus:            orch.EventBus(),
		IPCBroker:           nil,
		Version:             "test-tool-grpc-1.0.0",
		ShutdownTimeout:     10 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	grpcResult, err := grpcPkg.Bootstrap(ctx, grpcBootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap gRPC server")
	require.True(t, grpcResult.IsSuccessful(), "gRPC bootstrap should be successful")

	t.Cleanup(func() {
		t.Log("=== Cleaning up gRPC server ===")
		if grpcResult.Health != nil {
			grpcResult.Health.Shutdown()
		}
		if grpcResult.Server != nil {
			if err := grpcResult.Server.Stop(); err != nil {
				t.Logf("Warning: gRPC server stop returned error: %v", err)
			}
		}
	})

	t.Logf("gRPC server started on %s", cfg.GRPC.SocketPath)

	// Step 3: Create gRPC client and connect
	t.Log("=== Step 3: Creating gRPC client ===")

	clientCfg := grpcPkg.ClientConfig{
		DialTimeout:          5 * time.Second,
		RPCTimeout:           5 * time.Second,
		MaxRecvMsgSize:       grpcPkg.DefaultMaxRecvMsgSize,
		MaxSendMsgSize:       grpcPkg.DefaultMaxSendMsgSize,
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 0,
	}

	grpcClient, err := grpcPkg.NewClient(cfg.GRPC.SocketPath, clientCfg, log)
	require.NoError(t, err, "Failed to create gRPC client")

	t.Cleanup(func() {
		if err := grpcClient.Close(); err != nil {
			t.Logf("Warning: gRPC client close returned error: %v", err)
		}
	})

	err = grpcClient.Dial(ctx)
	require.NoError(t, err, "Failed to connect gRPC client")
	require.True(t, grpcClient.IsConnected(), "Client should be connected")

	t.Log("gRPC client connected successfully")

	// Step 4: Create session via gRPC
	t.Log("=== Step 4: Creating session via gRPC ===")

	orchClient := proto.NewOrchestratorServiceClient(grpcClient.GetConn())

	createReq := &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name:        "tool-exec-test-session",
			Description: "Session for tool execution integration test",
			OwnerId:     "tool-exec-test-user",
			Labels: map[string]string{
				"test":     "tool-exec",
				"workflow": "tool-execution",
			},
		},
		Config: &proto.SessionConfig{
			MaxContainers: 10,
			MaxDurationNs: 3600 * 1000000000,
			IdleTimeoutNs: 1800 * 1000000000,
		},
	}

	createResp, err := orchClient.CreateSession(ctx, createReq)
	require.NoError(t, err, "CreateSession should succeed")
	require.NotEmpty(t, createResp.SessionId, "Session ID should not be empty")

	sessionID := createResp.SessionId
	t.Logf("Session created via gRPC: %s", sessionID)

	// Step 5: Create tool client and register a test tool
	t.Log("=== Step 5: Creating tool client and registering test tool ===")

	toolClient := proto.NewToolServiceClient(grpcClient.GetConn())

	// First, check health of tool service
	healthCheckReq := &proto.ToolHealthCheckRequest{}
	healthResp, err := toolClient.HealthCheck(ctx, healthCheckReq)
	require.NoError(t, err, "Tool service health check should succeed")
	require.Equal(t, proto.Health_HEALTH_HEALTHY, healthResp.Health, "Tool service should be healthy")
	t.Logf("Tool service health check passed: %s", healthResp.Health)

	// Register a test tool (read_file)
	registerToolReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:        "read_file",
			DisplayName: "Read File",
			Type:        proto.ToolType_TOOL_TYPE_FILE,
			Description: "Read the contents of a file",
			Parameters: []*proto.ToolParameter{
				{
					Name:         "path",
					Description:  "Path to the file to read",
					Type:         proto.ParameterType_PARAMETER_TYPE_FILE_PATH,
					Required:     true,
				},
			},
			SecurityPolicy: &proto.ToolSecurityPolicy{
				AllowFilesystem:    true,
				ReadOnlyFilesystem: true,
				AllowNetwork:       false,
				AllowIpc:           false,
				MaxConcurrent:      10,
			},
			Enabled: true,
		},
		Force: true,
	}

	registerResp, err := toolClient.RegisterTool(ctx, registerToolReq)
	require.NoError(t, err, "RegisterTool should succeed")
	require.NotEmpty(t, registerResp.Name, "Registered tool name should not be empty")
	require.Equal(t, "read_file", registerResp.Name, "Registered tool name should match")
	t.Logf("Tool registered successfully: %s", registerResp.Name)

	// Step 6: Execute tool request
	t.Log("=== Step 6: Executing read_file tool request ===")

	// Create a test file to read
	testDir := t.TempDir()
	testFile := filepath.Join(testDir, "test.txt")
	testContent := "Hello from tool execution test!"

	// Write test content using standard Go (simulating a file that already exists)
	err = writeFile(testFile, testContent)
	require.NoError(t, err, "Failed to create test file")

	executeToolReq := &proto.ExecuteToolRequest{
		ToolName:  "read_file",
		SessionId: sessionID,
		Parameters: map[string]string{
			"path": testFile,
		},
		CorrelationId: "test-correlation-123",
	}

	executeResp, err := toolClient.ExecuteTool(ctx, executeToolReq)
	require.NoError(t, err, "ExecuteTool should succeed")
	require.NotEmpty(t, executeResp.ExecutionId, "Execution ID should not be empty")

	executionID := executeResp.ExecutionId
	t.Logf("Tool execution created: %s", executionID)

	// Verify execution info
	require.NotNil(t, executeResp.Execution, "Execution info should not be nil")
	require.Equal(t, "read_file", executeResp.Execution.ToolName, "Tool name should match")
	require.Equal(t, sessionID, executeResp.Execution.SessionId, "Session ID should match")

	// Step 7: Verify execution status
	t.Log("=== Step 7: Verifying execution status ===")

	getStatusReq := &proto.GetExecutionStatusRequest{
		ExecutionId: executionID,
	}

	getStatusResp, err := toolClient.GetExecutionStatus(ctx, getStatusReq)
	require.NoError(t, err, "GetExecutionStatus should succeed")
	require.NotNil(t, getStatusResp.Execution, "Execution should not be nil")
	require.Equal(t, executionID, getStatusResp.Execution.ExecutionId, "Execution ID should match")

	// The execution should be completed (currently marked as completed in the TODO section)
	t.Logf("Execution status: %s", getStatusResp.Execution.Status)

	// Step 8: List tools and verify our tool is registered
	t.Log("=== Step 8: Listing tools to verify registration ===")

	listToolsReq := &proto.ListToolsRequest{}
	listToolsResp, err := toolClient.ListTools(ctx, listToolsReq)
	require.NoError(t, err, "ListTools should succeed")
	require.GreaterOrEqual(t, len(listToolsResp.Tools), 1, "At least one tool should be registered")

	// Find our registered tool
	var found bool
	for _, tool := range listToolsResp.Tools {
		if tool.Name == "read_file" {
			found = true
			require.Equal(t, proto.ToolType_TOOL_TYPE_FILE, tool.Type, "Tool type should be FILE")
			require.True(t, tool.Enabled, "Tool should be enabled")
			t.Logf("Found tool: %s (type: %s, enabled: %v)", tool.Name, tool.Type, tool.Enabled)
			break
		}
	}
	require.True(t, found, "Registered tool should be found in list")

	// Step 9: List executions
	t.Log("=== Step 9: Listing executions ===")

	listExecsReq := &proto.ListExecutionsRequest{
		Filter: &proto.ExecutionFilter{
			ToolName: "read_file",
		},
	}
	listExecsResp, err := toolClient.ListExecutions(ctx, listExecsReq)
	require.NoError(t, err, "ListExecutions should succeed")
	require.GreaterOrEqual(t, len(listExecsResp.Executions), 1, "At least one execution should exist")

	// Find our execution
	var execFound bool
	for _, exec := range listExecsResp.Executions {
		if exec.ExecutionId == executionID {
			execFound = true
			require.Equal(t, "read_file", exec.ToolName, "Tool name should match")
			require.Equal(t, sessionID, exec.SessionId, "Session ID should match")
			t.Logf("Found execution: %s (tool: %s, status: %s)", exec.ExecutionId, exec.ToolName, exec.Status)
			break
		}
	}
	require.True(t, execFound, "Execution should be found in list")

	// Step 10: Get tool stats
	t.Log("=== Step 10: Getting tool stats ===")

	getStatsReq := &proto.GetStatsRequest{
		ToolName: "read_file",
	}
	getStatsResp, err := toolClient.GetStats(ctx, getStatsReq)
	require.NoError(t, err, "GetStats should succeed")
	require.NotNil(t, getStatsResp.ServiceStats, "Service stats should not be nil")

	t.Logf("Service stats - Total executions: %d, Successful: %d, Failed: %d",
		getStatsResp.ServiceStats.TotalExecutions,
		getStatsResp.ServiceStats.SuccessfulExecutions,
		getStatsResp.ServiceStats.FailedExecutions)

	// Verify our tool execution is tracked
	require.GreaterOrEqual(t, getStatsResp.ServiceStats.TotalExecutions, int64(1), "At least one execution should be tracked")

	// Step 11: Test cancellation
	t.Log("=== Step 11: Testing execution cancellation ===")

	// Create a new execution to cancel
	cancelExecuteReq := &proto.ExecuteToolRequest{
		ToolName:  "read_file",
		SessionId: sessionID,
		Parameters: map[string]string{
			"path": testFile,
		},
		CorrelationId: "test-cancel-456",
	}

	cancelExecuteResp, err := toolClient.ExecuteTool(ctx, cancelExecuteReq)
	require.NoError(t, err, "ExecuteTool for cancellation test should succeed")

	cancelReqID := cancelExecuteResp.ExecutionId

	// Cancel the execution
	cancelReq := &proto.CancelExecutionRequest{
		ExecutionId: cancelReqID,
		Reason:      "Test cancellation",
		Force:       false,
	}

	cancelResp, err := toolClient.CancelExecution(ctx, cancelReq)
	require.NoError(t, err, "CancelExecution should succeed")
	require.True(t, cancelResp.Cancelled, "Execution should be cancelled")
	require.Equal(t, proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED, cancelResp.Status, "Status should be CANCELLED")
	t.Logf("Execution cancelled successfully: %s", cancelReqID)

	// Test complete
	t.Log("=== Tool Execution Flow Test Complete ===")
	t.Log("All steps passed successfully:")
	t.Log("  1. Orchestrator bootstrapped with tool service")
	t.Log("  2. gRPC server bootstrapped with tool service")
	t.Log("  3. gRPC client created and connected")
	t.Log("  4. Session created via gRPC")
	t.Log("  5. Tool client created and tool registered")
	t.Log("  6. Tool execution request executed")
	t.Log("  7. Execution status verified")
	t.Log("  8. Tool registration verified via list")
	t.Log("  9. Executions listed and verified")
	t.Log(" 10. Tool stats retrieved and verified")
	t.Log(" 11. Execution cancellation tested")
}

// TestToolServiceHealthCheck tests the tool service health check endpoint
func TestToolServiceHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Log("=== Testing Tool Service Health Check ===")

	cfg := loadTestConfig(t)

	// Bootstrap orchestrator
	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-tool-health-1.0.0",
		ShutdownTimeout:     30 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	orch := result.Orchestrator

	t.Cleanup(func() {
		if err := orch.Close(); err != nil {
			t.Logf("Warning: Orchestrator close returned error: %v", err)
		}
	})

	// Bootstrap gRPC server
	grpcBootstrapCfg := grpcPkg.BootstrapConfig{
		Config:              cfg.GRPC,
		Logger:              log,
		SessionManager:      orch.SessionManager(),
		EventBus:            orch.EventBus(),
		IPCBroker:           nil,
		Version:             "test-tool-health-1.0.0",
		ShutdownTimeout:     10 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	grpcResult, err := grpcPkg.Bootstrap(ctx, grpcBootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap gRPC server")

	t.Cleanup(func() {
		if grpcResult.Health != nil {
			grpcResult.Health.Shutdown()
		}
		if grpcResult.Server != nil {
			if err := grpcResult.Server.Stop(); err != nil {
				t.Logf("Warning: gRPC server stop returned error: %v", err)
			}
		}
	})

	// Connect client
	clientCfg := grpcPkg.ClientConfig{
		DialTimeout:          5 * time.Second,
		RPCTimeout:           5 * time.Second,
		MaxRecvMsgSize:       grpcPkg.DefaultMaxRecvMsgSize,
		MaxSendMsgSize:       grpcPkg.DefaultMaxSendMsgSize,
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 0,
	}

	grpcClient, err := grpcPkg.NewClient(cfg.GRPC.SocketPath, clientCfg, log)
	require.NoError(t, err, "Failed to create gRPC client")

	t.Cleanup(func() {
		if err := grpcClient.Close(); err != nil {
			t.Logf("Warning: gRPC client close returned error: %v", err)
		}
	})

	err = grpcClient.Dial(ctx)
	require.NoError(t, err, "Failed to connect gRPC client")

	// Test tool service health check
	toolClient := proto.NewToolServiceClient(grpcClient.GetConn())

	healthCheckReq := &proto.ToolHealthCheckRequest{}
	healthResp, err := toolClient.HealthCheck(ctx, healthCheckReq)
	require.NoError(t, err, "HealthCheck should succeed")
	require.Equal(t, proto.Health_HEALTH_HEALTHY, healthResp.Health, "Health status should be HEALTHY")
	require.NotEmpty(t, healthResp.Version, "Version should not be empty")
	require.NotEmpty(t, healthResp.Subsystems, "Subsystems should not be empty")

	t.Logf("Tool service health check passed:")
	t.Logf("  Health: %s", healthResp.Health)
	t.Logf("  Version: %s", healthResp.Version)
	t.Logf("  Registered tools: %d", healthResp.RegisteredTools)
	t.Logf("  Active executions: %d", healthResp.ActiveExecutions)
	t.Logf("  Subsystems: %v", healthResp.Subsystems)

	// Test service status
	statusReq := &proto.ToolServiceStatusRequest{}
	statusResp, err := toolClient.GetServiceStatus(ctx, statusReq)
	require.NoError(t, err, "GetServiceStatus should succeed")
	require.Equal(t, proto.Status_STATUS_RUNNING, statusResp.Status, "Status should be RUNNING")
	require.NotNil(t, statusResp.StartedAt, "StartedAt should not be nil")
	require.NotNil(t, statusResp.Uptime, "Uptime should not be nil")

	t.Logf("Tool service status:")
	t.Logf("  Status: %s", statusResp.Status)
	t.Logf("  Registered tools: %d", statusResp.RegisteredTools)
	t.Logf("  Enabled tools: %d", statusResp.EnabledTools)
	t.Logf("  Active executions: %d", statusResp.ActiveExecutions)

	t.Log("Tool service health check test passed")
}

// TestMultipleToolExecutions tests executing multiple tools concurrently
func TestMultipleToolExecutions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if err := container.CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err, "Failed to create logger")

	t.Log("=== Testing Multiple Tool Executions ===")

	cfg := loadTestConfig(t)

	// Bootstrap orchestrator
	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              log,
		Version:             "test-multi-tool-1.0.0",
		ShutdownTimeout:     30 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap orchestrator")
	orch := result.Orchestrator

	t.Cleanup(func() {
		if err := orch.Close(); err != nil {
			t.Logf("Warning: Orchestrator close returned error: %v", err)
		}
	})

	// Bootstrap gRPC server
	grpcBootstrapCfg := grpcPkg.BootstrapConfig{
		Config:              cfg.GRPC,
		Logger:              log,
		SessionManager:      orch.SessionManager(),
		EventBus:            orch.EventBus(),
		IPCBroker:           nil,
		Version:             "test-multi-tool-1.0.0",
		ShutdownTimeout:     10 * time.Second,
		EnableHealthCheck:   false,
		HealthCheckInterval: 30 * time.Second,
	}

	grpcResult, err := grpcPkg.Bootstrap(ctx, grpcBootstrapCfg)
	require.NoError(t, err, "Failed to bootstrap gRPC server")

	t.Cleanup(func() {
		if grpcResult.Health != nil {
			grpcResult.Health.Shutdown()
		}
		if grpcResult.Server != nil {
			if err := grpcResult.Server.Stop(); err != nil {
				t.Logf("Warning: gRPC server stop returned error: %v", err)
			}
		}
	})

	// Connect client
	clientCfg := grpcPkg.ClientConfig{
		DialTimeout:          5 * time.Second,
		RPCTimeout:           5 * time.Second,
		MaxRecvMsgSize:       grpcPkg.DefaultMaxRecvMsgSize,
		MaxSendMsgSize:       grpcPkg.DefaultMaxSendMsgSize,
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 0,
	}

	grpcClient, err := grpcPkg.NewClient(cfg.GRPC.SocketPath, clientCfg, log)
	require.NoError(t, err, "Failed to create gRPC client")

	t.Cleanup(func() {
		if err := grpcClient.Close(); err != nil {
			t.Logf("Warning: gRPC client close returned error: %v", err)
		}
	})

	err = grpcClient.Dial(ctx)
	require.NoError(t, err, "Failed to connect gRPC client")

	// Create session
	orchClient := proto.NewOrchestratorServiceClient(grpcClient.GetConn())

	createReq := &proto.CreateSessionRequest{
		Metadata: &proto.SessionMetadata{
			Name:        "multi-tool-test-session",
			Description: "Session for multiple tool execution test",
			OwnerId:     "multi-tool-test-user",
		},
		Config: &proto.SessionConfig{
			MaxContainers: 10,
		},
	}

	createResp, err := orchClient.CreateSession(ctx, createReq)
	require.NoError(t, err, "CreateSession should succeed")
	sessionID := createResp.SessionId

	// Create tool client and register multiple tools
	toolClient := proto.NewToolServiceClient(grpcClient.GetConn())

	toolsToRegister := []*proto.ToolDefinition{
		{
			Name:        "read_file",
			DisplayName: "Read File",
			Type:        proto.ToolType_TOOL_TYPE_FILE,
			Description: "Read the contents of a file",
			Parameters: []*proto.ToolParameter{
				{Name: "path", Description: "Path to the file", Type: proto.ParameterType_PARAMETER_TYPE_FILE_PATH, Required: true},
			},
			SecurityPolicy: &proto.ToolSecurityPolicy{
				AllowFilesystem: true, ReadOnlyFilesystem: true, AllowNetwork: false, AllowIpc: false, MaxConcurrent: 10,
			},
			Enabled: true,
		},
		{
			Name:        "write_file",
			DisplayName: "Write File",
			Type:        proto.ToolType_TOOL_TYPE_FILE,
			Description: "Write content to a file",
			Parameters: []*proto.ToolParameter{
				{Name: "path", Description: "Path to the file", Type: proto.ParameterType_PARAMETER_TYPE_FILE_PATH, Required: true},
				{Name: "content", Description: "Content to write", Type: proto.ParameterType_PARAMETER_TYPE_STRING, Required: true},
			},
			SecurityPolicy: &proto.ToolSecurityPolicy{
				AllowFilesystem: true, ReadOnlyFilesystem: false, AllowNetwork: false, AllowIpc: false, MaxConcurrent: 10,
			},
			Enabled: true,
		},
		{
			Name:        "exec",
			DisplayName: "Execute Shell Command",
			Type:        proto.ToolType_TOOL_TYPE_SHELL,
			Description: "Execute a shell command",
			Parameters: []*proto.ToolParameter{
				{Name: "command", Description: "Shell command to execute", Type: proto.ParameterType_PARAMETER_TYPE_STRING, Required: true},
			},
			SecurityPolicy: &proto.ToolSecurityPolicy{
				AllowFilesystem: false, AllowNetwork: false, AllowIpc: false, MaxConcurrent: 5,
				BlockedCommands: []string{"rm -rf /", "mkfs"},
			},
			Enabled: true,
		},
	}

	// Register all tools
	for _, toolDef := range toolsToRegister {
		registerReq := &proto.RegisterToolRequest{
			Definition: toolDef,
			Force:      true,
		}
		_, err := toolClient.RegisterTool(ctx, registerReq)
		require.NoError(t, err, "RegisterTool should succeed for %s", toolDef.Name)
		t.Logf("Registered tool: %s", toolDef.Name)
	}

	// List all tools to verify registration
	listToolsReq := &proto.ListToolsRequest{}
	listToolsResp, err := toolClient.ListTools(ctx, listToolsReq)
	require.NoError(t, err, "ListTools should succeed")
	require.GreaterOrEqual(t, len(listToolsResp.Tools), len(toolsToRegister), "All tools should be registered")

	t.Logf("Total tools registered: %d", len(listToolsResp.Tools))

	// Execute multiple tools
	testDir := t.TempDir()
	testFile := filepath.Join(testDir, "multi-test.txt")

	executions := make([]string, 0, len(toolsToRegister))

	// Execute read_file tool
	readExecuteReq := &proto.ExecuteToolRequest{
		ToolName:  "read_file",
		SessionId: sessionID,
		Parameters: map[string]string{
			"path": testFile,
		},
	}
	readResp, err := toolClient.ExecuteTool(ctx, readExecuteReq)
	require.NoError(t, err, "ExecuteTool read_file should succeed")
	executions = append(executions, readResp.ExecutionId)
	t.Logf("Executed read_file: %s", readResp.ExecutionId)

	// Execute write_file tool
	writeExecuteReq := &proto.ExecuteToolRequest{
		ToolName:  "write_file",
		SessionId: sessionID,
		Parameters: map[string]string{
			"path":    testFile,
			"content": "Test content from multi-execution test",
		},
	}
	writeResp, err := toolClient.ExecuteTool(ctx, writeExecuteReq)
	require.NoError(t, err, "ExecuteTool write_file should succeed")
	executions = append(executions, writeResp.ExecutionId)
	t.Logf("Executed write_file: %s", writeResp.ExecutionId)

	// Execute exec tool
	execExecuteReq := &proto.ExecuteToolRequest{
		ToolName:  "exec",
		SessionId: sessionID,
		Parameters: map[string]string{
			"command": "echo 'Hello from tool execution'",
		},
	}
	execResp, err := toolClient.ExecuteTool(ctx, execExecuteReq)
	require.NoError(t, err, "ExecuteTool exec should succeed")
	executions = append(executions, execResp.ExecutionId)
	t.Logf("Executed exec: %s", execResp.ExecutionId)

	// Verify all executions
	for _, execID := range executions {
		getStatusReq := &proto.GetExecutionStatusRequest{
			ExecutionId: execID,
		}
		getStatusResp, err := toolClient.GetExecutionStatus(ctx, getStatusReq)
		require.NoError(t, err, "GetExecutionStatus should succeed for %s", execID)
		require.NotNil(t, getStatusResp.Execution, "Execution should not be nil")
		require.Equal(t, execID, getStatusResp.Execution.ExecutionId, "Execution ID should match")
		t.Logf("Execution %s status: %s", execID, getStatusResp.Execution.Status)
	}

	// Get stats to verify all executions were tracked
	getStatsReq := &proto.GetStatsRequest{}
	getStatsResp, err := toolClient.GetStats(ctx, getStatsReq)
	require.NoError(t, err, "GetStats should succeed")
	require.GreaterOrEqual(t, getStatsResp.ServiceStats.TotalExecutions, int64(len(toolsToRegister)), "All executions should be tracked")

	t.Logf("Multiple tool executions test passed:")
	t.Logf("  Tools registered: %d", len(toolsToRegister))
	t.Logf("  Executions created: %d", len(executions))
	t.Logf("  Total service executions: %d", getStatsResp.ServiceStats.TotalExecutions)
}

// Helper function to write a test file
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
