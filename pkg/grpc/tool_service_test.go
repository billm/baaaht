package grpc

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/proto"
)

// mockToolServiceDeps implements ToolServiceDependencies for testing
type mockToolServiceDeps struct{}

func (m *mockToolServiceDeps) EventBus() *events.Bus {
	return nil // Event bus is optional for tests
}

// Test helper to create a test service
func newTestToolService(t *testing.T) *ToolService {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return NewToolService(&mockToolServiceDeps{}, log)
}

// =============================================================================
// Test Tool Registration
// =============================================================================

func TestToolService_RegisterTool(t *testing.T) {
	service := newTestToolService(t)

	t.Run("valid_registration", func(t *testing.T) {
		req := &proto.RegisterToolRequest{
			Definition: &proto.ToolDefinition{
				Name:        "test-tool",
				DisplayName: "Test Tool",
				Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
				Description: "A test tool",
				Version:     "1.0.0",
				Enabled:     true,
				Parameters: []*proto.ToolParameter{
					{
						Name:        "param1",
						Type:        proto.ParameterType_PARAMETER_TYPE_STRING,
						Required:    true,
						Description: "First parameter",
					},
				},
			},
			Force: false,
		}

		resp, err := service.RegisterTool(context.Background(), req)
		if err != nil {
			t.Fatalf("RegisterTool failed: %v", err)
		}

		if resp.Name == "" {
			t.Error("Expected non-empty name")
		}
		if resp.Tool == nil {
			t.Error("Expected tool in response")
		}
		if resp.Tool.Name != "test-tool" {
			t.Errorf("Expected tool name 'test-tool', got '%s'", resp.Tool.Name)
		}
	})

	t.Run("empty_definition_returns_error", func(t *testing.T) {
		req := &proto.RegisterToolRequest{
			Definition: nil,
		}

		_, err := service.RegisterTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for nil definition")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		req := &proto.RegisterToolRequest{
			Definition: &proto.ToolDefinition{
				Name: "",
			},
		}

		_, err := service.RegisterTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("duplicate_registration_without_force_returns_error", func(t *testing.T) {
		req := &proto.RegisterToolRequest{
			Definition: &proto.ToolDefinition{
				Name:        "duplicate-tool",
				DisplayName: "Duplicate Tool",
				Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			},
		}

		// First registration should succeed
		_, err := service.RegisterTool(context.Background(), req)
		if err != nil {
			t.Fatalf("First registration failed: %v", err)
		}

		// Second registration without force should fail
		_, err = service.RegisterTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for duplicate registration")
		}
		if status.Code(err) != codes.AlreadyExists {
			t.Errorf("Expected AlreadyExists, got %v", status.Code(err))
		}
	})

	t.Run("duplicate_registration_with_force_succeeds", func(t *testing.T) {
		req := &proto.RegisterToolRequest{
			Definition: &proto.ToolDefinition{
				Name:        "force-tool",
				DisplayName: "Force Tool",
				Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			},
			Force: false,
		}

		// First registration
		_, err := service.RegisterTool(context.Background(), req)
		if err != nil {
			t.Fatalf("First registration failed: %v", err)
		}

		// Second registration with force should succeed
		req.Force = true
		req.Definition.DisplayName = "Updated Force Tool"
		resp, err := service.RegisterTool(context.Background(), req)
		if err != nil {
			t.Errorf("Registration with force failed: %v", err)
		}
		if resp.Tool.DisplayName != "Updated Force Tool" {
			t.Errorf("Expected updated display name, got '%s'", resp.Tool.DisplayName)
		}
	})
}

func TestToolService_UnregisterTool(t *testing.T) {
	service := newTestToolService(t)

	// First register a tool
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:        "test-unregister-tool",
			DisplayName: "Test Unregister Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	t.Run("valid_unregistration", func(t *testing.T) {
		req := &proto.UnregisterToolRequest{
			Name:  "test-unregister-tool",
			Force: false,
		}

		resp, err := service.UnregisterTool(context.Background(), req)
		if err != nil {
			t.Fatalf("UnregisterTool failed: %v", err)
		}

		if !resp.Success {
			t.Error("Expected success=true")
		}
		if resp.Message == "" {
			t.Error("Expected non-empty message")
		}
	})

	t.Run("unregister_nonexistent_tool", func(t *testing.T) {
		req := &proto.UnregisterToolRequest{
			Name:  "non-existent-tool",
			Force: false,
		}

		_, err := service.UnregisterTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent tool")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		req := &proto.UnregisterToolRequest{
			Name: "",
		}

		_, err := service.UnregisterTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestToolService_UpdateTool(t *testing.T) {
	service := newTestToolService(t)

	// Register a tool first
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:        "test-update-tool",
			DisplayName: "Original Name",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			Description: "Original description",
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	t.Run("valid_update", func(t *testing.T) {
		req := &proto.UpdateToolRequest{
			Name: "test-update-tool",
			Definition: &proto.ToolDefinition{
				Name:        "test-update-tool",
				DisplayName: "Updated Name",
				Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
				Description: "Updated description",
			},
		}

		resp, err := service.UpdateTool(context.Background(), req)
		if err != nil {
			t.Fatalf("UpdateTool failed: %v", err)
		}

		if resp.Tool == nil {
			t.Error("Expected tool in response")
		}
		if resp.Tool.DisplayName != "Updated Name" {
			t.Errorf("Expected updated display name 'Updated Name', got '%s'", resp.Tool.DisplayName)
		}
	})

	t.Run("update_nonexistent_tool", func(t *testing.T) {
		req := &proto.UpdateToolRequest{
			Name: "non-existent-tool",
			Definition: &proto.ToolDefinition{
				Name: "non-existent-tool",
			},
		}

		_, err := service.UpdateTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent tool")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		req := &proto.UpdateToolRequest{
			Name: "",
			Definition: &proto.ToolDefinition{
				Name: "test",
			},
		}

		_, err := service.UpdateTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("nil_definition_returns_error", func(t *testing.T) {
		req := &proto.UpdateToolRequest{
			Name:       "test-update-tool",
			Definition: nil,
		}

		_, err := service.UpdateTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for nil definition")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestToolService_EnableTool(t *testing.T) {
	service := newTestToolService(t)

	// Register a disabled tool
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:        "test-enable-tool",
			DisplayName: "Test Enable Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			Enabled:     false,
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	t.Run("enable_tool", func(t *testing.T) {
		req := &proto.EnableToolRequest{
			Name: "test-enable-tool",
		}

		resp, err := service.EnableTool(context.Background(), req)
		if err != nil {
			t.Fatalf("EnableTool failed: %v", err)
		}

		if resp.Name != "test-enable-tool" {
			t.Errorf("Expected name 'test-enable-tool', got '%s'", resp.Name)
		}
		if !resp.Enabled {
			t.Error("Expected enabled=true")
		}
	})

	t.Run("enable_nonexistent_tool", func(t *testing.T) {
		req := &proto.EnableToolRequest{
			Name: "non-existent-tool",
		}

		_, err := service.EnableTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent tool")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		req := &proto.EnableToolRequest{
			Name: "",
		}

		_, err := service.EnableTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestToolService_DisableTool(t *testing.T) {
	service := newTestToolService(t)

	// Register an enabled tool
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:        "test-disable-tool",
			DisplayName: "Test Disable Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			Enabled:     true,
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	t.Run("disable_tool", func(t *testing.T) {
		req := &proto.DisableToolRequest{
			Name:                    "test-disable-tool",
			CancelActiveExecutions:  false,
		}

		resp, err := service.DisableTool(context.Background(), req)
		if err != nil {
			t.Fatalf("DisableTool failed: %v", err)
		}

		if resp.Name != "test-disable-tool" {
			t.Errorf("Expected name 'test-disable-tool', got '%s'", resp.Name)
		}
		if !resp.Disabled {
			t.Error("Expected disabled=true")
		}
	})

	t.Run("disable_nonexistent_tool", func(t *testing.T) {
		req := &proto.DisableToolRequest{
			Name: "non-existent-tool",
		}

		_, err := service.DisableTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent tool")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		req := &proto.DisableToolRequest{
			Name: "",
		}

		_, err := service.DisableTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

// =============================================================================
// Test Tool Discovery
// =============================================================================

func TestToolService_ListTools(t *testing.T) {
	service := newTestToolService(t)

	// Register some tools
	tools := []struct {
		name        string
		displayName string
		toolType    proto.ToolType
		enabled     bool
	}{
		{"tool1", "Tool 1", proto.ToolType_TOOL_TYPE_BUILT_IN, true},
		{"tool2", "Tool 2", proto.ToolType_TOOL_TYPE_CONTAINER, true},
		{"tool3", "Tool 3", proto.ToolType_TOOL_TYPE_BUILT_IN, false},
	}

	for _, tool := range tools {
		req := &proto.RegisterToolRequest{
			Definition: &proto.ToolDefinition{
				Name:        tool.name,
				DisplayName: tool.displayName,
				Type:        tool.toolType,
				Enabled:     tool.enabled,
			},
		}
		_, err := service.RegisterTool(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to register tool %s: %v", tool.name, err)
		}
	}

	t.Run("list_all_tools", func(t *testing.T) {
		req := &proto.ListToolsRequest{
			Filter: nil,
		}

		resp, err := service.ListTools(context.Background(), req)
		if err != nil {
			t.Fatalf("ListTools failed: %v", err)
		}

		if resp.TotalCount != 3 {
			t.Errorf("Expected 3 tools, got %d", resp.TotalCount)
		}
		if len(resp.Tools) != 3 {
			t.Errorf("Expected 3 tools in list, got %d", len(resp.Tools))
		}
	})

	t.Run("list_tools_with_type_filter", func(t *testing.T) {
		req := &proto.ListToolsRequest{
			Filter: &proto.ToolFilter{
				Type: proto.ToolType_TOOL_TYPE_BUILT_IN,
			},
		}

		resp, err := service.ListTools(context.Background(), req)
		if err != nil {
			t.Fatalf("ListTools with filter failed: %v", err)
		}

		if resp.TotalCount != 2 {
			t.Errorf("Expected 2 BUILT_IN tools, got %d", resp.TotalCount)
		}
	})

	t.Run("list_tools_with_enabled_filter", func(t *testing.T) {
		req := &proto.ListToolsRequest{
			Filter: &proto.ToolFilter{
				Enabled: true,
			},
		}

		resp, err := service.ListTools(context.Background(), req)
		if err != nil {
			t.Fatalf("ListTools with enabled filter failed: %v", err)
		}

		if resp.TotalCount != 2 {
			t.Errorf("Expected 2 enabled tools, got %d", resp.TotalCount)
		}
	})
}

func TestToolService_GetTool(t *testing.T) {
	service := newTestToolService(t)

	// Register a tool
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:        "test-get-tool",
			DisplayName: "Test Get Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	t.Run("get_tool", func(t *testing.T) {
		req := &proto.GetToolRequest{
			Name: "test-get-tool",
		}

		resp, err := service.GetTool(context.Background(), req)
		if err != nil {
			t.Fatalf("GetTool failed: %v", err)
		}

		if resp.Tool == nil {
			t.Error("Expected tool in response")
		}
		if resp.Tool.Name != "test-get-tool" {
			t.Errorf("Expected tool name 'test-get-tool', got '%s'", resp.Tool.Name)
		}
	})

	t.Run("get_nonexistent_tool", func(t *testing.T) {
		req := &proto.GetToolRequest{
			Name: "non-existent-tool",
		}

		_, err := service.GetTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent tool")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		req := &proto.GetToolRequest{
			Name: "",
		}

		_, err := service.GetTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestToolService_GetToolDefinition(t *testing.T) {
	service := newTestToolService(t)

	// Register a tool
	definition := &proto.ToolDefinition{
		Name:        "test-def-tool",
		DisplayName: "Test Definition Tool",
		Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
		Description: "A test tool for getting definitions",
		Version:     "2.0.0",
		Parameters: []*proto.ToolParameter{
			{
				Name:     "input",
				Type:     proto.ParameterType_PARAMETER_TYPE_STRING,
				Required: true,
			},
		},
	}
	regReq := &proto.RegisterToolRequest{
		Definition: definition,
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	t.Run("get_tool_definition", func(t *testing.T) {
		req := &proto.GetToolDefinitionRequest{
			Name: "test-def-tool",
		}

		resp, err := service.GetToolDefinition(context.Background(), req)
		if err != nil {
			t.Fatalf("GetToolDefinition failed: %v", err)
		}

		if resp.Definition == nil {
			t.Error("Expected definition in response")
		}
		if resp.Definition.Name != "test-def-tool" {
			t.Errorf("Expected definition name 'test-def-tool', got '%s'", resp.Definition.Name)
		}
		if len(resp.Definition.Parameters) != 1 {
			t.Errorf("Expected 1 parameter, got %d", len(resp.Definition.Parameters))
		}
	})

	t.Run("get_definition_for_nonexistent_tool", func(t *testing.T) {
		req := &proto.GetToolDefinitionRequest{
			Name: "non-existent-tool",
		}

		_, err := service.GetToolDefinition(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent tool")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		req := &proto.GetToolDefinitionRequest{
			Name: "",
		}

		_, err := service.GetToolDefinition(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

// =============================================================================
// Test Tool Execution
// =============================================================================

func TestToolService_ExecuteTool(t *testing.T) {
	service := newTestToolService(t)

	// Register a tool first
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:        "test-exec-tool",
			DisplayName: "Test Execution Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			Enabled:     true,
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	t.Run("valid_tool_execution", func(t *testing.T) {
		req := &proto.ExecuteToolRequest{
			ToolName:    "test-exec-tool",
			SessionId:   "test-session-123",
			Parameters:  map[string]string{"key": "value"},
			TimeoutNs:   30000000000, // 30 seconds
			CorrelationId: "test-correlation-456",
		}

		resp, err := service.ExecuteTool(context.Background(), req)
		if err != nil {
			t.Fatalf("ExecuteTool failed: %v", err)
		}

		if resp.ExecutionId == "" {
			t.Error("Expected non-empty execution_id")
		}
		if resp.Execution == nil {
			t.Error("Expected execution in response")
		}
		if resp.Execution.ToolName != "test-exec-tool" {
			t.Errorf("Expected tool_name 'test-exec-tool', got '%s'", resp.Execution.ToolName)
		}
	})

	t.Run("execute_nonexistent_tool", func(t *testing.T) {
		req := &proto.ExecuteToolRequest{
			ToolName: "non-existent-tool",
		}

		_, err := service.ExecuteTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent tool")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("execute_disabled_tool", func(t *testing.T) {
		// Register a disabled tool
		regReq := &proto.RegisterToolRequest{
			Definition: &proto.ToolDefinition{
				Name:    "disabled-tool",
				Type:    proto.ToolType_TOOL_TYPE_BUILT_IN,
				Enabled: false,
			},
		}
		_, err := service.RegisterTool(context.Background(), regReq)
		if err != nil {
			t.Fatalf("Failed to register disabled tool: %v", err)
		}

		req := &proto.ExecuteToolRequest{
			ToolName: "disabled-tool",
		}

		_, err = service.ExecuteTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for disabled tool")
		}
		if status.Code(err) != codes.FailedPrecondition {
			t.Errorf("Expected FailedPrecondition, got %v", status.Code(err))
		}
	})

	t.Run("empty_tool_name_returns_error", func(t *testing.T) {
		req := &proto.ExecuteToolRequest{
			ToolName: "",
		}

		_, err := service.ExecuteTool(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty tool_name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestToolService_CancelExecution(t *testing.T) {
	service := newTestToolService(t)

	// Register a tool and create an execution
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:    "test-cancel-tool",
			Type:    proto.ToolType_TOOL_TYPE_BUILT_IN,
			Enabled: true,
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	execReq := &proto.ExecuteToolRequest{
		ToolName: "test-cancel-tool",
	}
	execResp, err := service.ExecuteTool(context.Background(), execReq)
	if err != nil {
		t.Fatalf("Failed to create execution: %v", err)
	}

	t.Run("cancel_execution", func(t *testing.T) {
		req := &proto.CancelExecutionRequest{
			ExecutionId: execResp.ExecutionId,
			Reason:      "test cancellation",
			Force:       false,
		}

		resp, err := service.CancelExecution(context.Background(), req)
		if err != nil {
			t.Fatalf("CancelExecution failed: %v", err)
		}

		if !resp.Cancelled {
			t.Error("Expected cancelled=true")
		}
		if resp.Status != proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED {
			t.Errorf("Expected CANCELLED status, got %v", resp.Status)
		}
	})

	t.Run("cancel_nonexistent_execution", func(t *testing.T) {
		req := &proto.CancelExecutionRequest{
			ExecutionId: "non-existent-execution",
		}

		_, err := service.CancelExecution(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent execution")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_execution_id_returns_error", func(t *testing.T) {
		req := &proto.CancelExecutionRequest{
			ExecutionId: "",
		}

		_, err := service.CancelExecution(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty execution_id")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestToolService_GetExecutionStatus(t *testing.T) {
	service := newTestToolService(t)

	// Register a tool and create an execution
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:    "test-status-tool",
			Type:    proto.ToolType_TOOL_TYPE_BUILT_IN,
			Enabled: true,
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	execReq := &proto.ExecuteToolRequest{
		ToolName:  "test-status-tool",
		SessionId: "test-session",
	}
	execResp, err := service.ExecuteTool(context.Background(), execReq)
	if err != nil {
		t.Fatalf("Failed to create execution: %v", err)
	}

	t.Run("get_execution_status", func(t *testing.T) {
		req := &proto.GetExecutionStatusRequest{
			ExecutionId: execResp.ExecutionId,
		}

		resp, err := service.GetExecutionStatus(context.Background(), req)
		if err != nil {
			t.Fatalf("GetExecutionStatus failed: %v", err)
		}

		if resp.Execution == nil {
			t.Error("Expected execution in response")
		}
		if resp.Execution.ExecutionId != execResp.ExecutionId {
			t.Errorf("Expected execution_id %s, got %s", execResp.ExecutionId, resp.Execution.ExecutionId)
		}
		if resp.Execution.ToolName != "test-status-tool" {
			t.Errorf("Expected tool_name 'test-status-tool', got '%s'", resp.Execution.ToolName)
		}
	})

	t.Run("get_status_for_nonexistent_execution", func(t *testing.T) {
		req := &proto.GetExecutionStatusRequest{
			ExecutionId: "non-existent-execution",
		}

		_, err := service.GetExecutionStatus(context.Background(), req)
		if err == nil {
			t.Error("Expected error for non-existent execution")
		}
		if status.Code(err) != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", status.Code(err))
		}
	})

	t.Run("empty_execution_id_returns_error", func(t *testing.T) {
		req := &proto.GetExecutionStatusRequest{
			ExecutionId: "",
		}

		_, err := service.GetExecutionStatus(context.Background(), req)
		if err == nil {
			t.Error("Expected error for empty execution_id")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestToolService_ListExecutions(t *testing.T) {
	service := newTestToolService(t)

	// Register a tool
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:    "test-list-exec-tool",
			Type:    proto.ToolType_TOOL_TYPE_BUILT_IN,
			Enabled: true,
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Create some executions
	for i := 0; i < 3; i++ {
		execReq := &proto.ExecuteToolRequest{
			ToolName:  "test-list-exec-tool",
			SessionId: "test-session",
		}
		_, err := service.ExecuteTool(context.Background(), execReq)
		if err != nil {
			t.Fatalf("Failed to create execution: %v", err)
		}
	}

	t.Run("list_all_executions", func(t *testing.T) {
		req := &proto.ListExecutionsRequest{
			Filter: nil,
		}

		resp, err := service.ListExecutions(context.Background(), req)
		if err != nil {
			t.Fatalf("ListExecutions failed: %v", err)
		}

		if resp.TotalCount != 3 {
			t.Errorf("Expected 3 executions, got %d", resp.TotalCount)
		}
		if len(resp.Executions) != 3 {
			t.Errorf("Expected 3 executions in list, got %d", len(resp.Executions))
		}
	})

	t.Run("list_executions_with_tool_filter", func(t *testing.T) {
		req := &proto.ListExecutionsRequest{
			Filter: &proto.ExecutionFilter{
				ToolName: "test-list-exec-tool",
			},
		}

		resp, err := service.ListExecutions(context.Background(), req)
		if err != nil {
			t.Fatalf("ListExecutions with filter failed: %v", err)
		}

		if resp.TotalCount != 3 {
			t.Errorf("Expected 3 executions for tool, got %d", resp.TotalCount)
		}
	})

	t.Run("list_executions_with_session_filter", func(t *testing.T) {
		req := &proto.ListExecutionsRequest{
			Filter: &proto.ExecutionFilter{
				SessionId: "test-session",
			},
		}

		resp, err := service.ListExecutions(context.Background(), req)
		if err != nil {
			t.Fatalf("ListExecutions with session filter failed: %v", err)
		}

		if resp.TotalCount != 3 {
			t.Errorf("Expected 3 executions for session, got %d", resp.TotalCount)
		}
	})
}

// =============================================================================
// Test Health and Status
// =============================================================================

func TestToolService_HealthCheck(t *testing.T) {
	service := newTestToolService(t)

	t.Run("health_check_returns_healthy", func(t *testing.T) {
		resp, err := service.HealthCheck(context.Background(), &emptypb.Empty{})
		if err != nil {
			t.Fatalf("HealthCheck failed: %v", err)
		}

		if resp.Health != proto.Health_HEALTH_HEALTHY {
			t.Errorf("Expected HEALTHY status, got %v", resp.Health)
		}
		if resp.Version == "" {
			t.Error("Expected non-empty version")
		}
		if len(resp.Subsystems) == 0 {
			t.Error("Expected subsystems in response")
		}
	})
}

func TestToolService_GetServiceStatus(t *testing.T) {
	service := newTestToolService(t)

	t.Run("get_service_status", func(t *testing.T) {
		resp, err := service.GetServiceStatus(context.Background(), &emptypb.Empty{})
		if err != nil {
			t.Fatalf("GetServiceStatus failed: %v", err)
		}

		if resp.Status != proto.Status_STATUS_RUNNING {
			t.Errorf("Expected RUNNING status, got %v", resp.Status)
		}
		if resp.StartedAt == nil {
			t.Error("Expected started_at in response")
		}
		if resp.Uptime == nil {
			t.Error("Expected uptime in response")
		}
	})
}

func TestToolService_GetStats(t *testing.T) {
	service := newTestToolService(t)

	// Register a tool and create some executions
	regReq := &proto.RegisterToolRequest{
		Definition: &proto.ToolDefinition{
			Name:    "test-stats-tool",
			Type:    proto.ToolType_TOOL_TYPE_BUILT_IN,
			Enabled: true,
		},
	}
	_, err := service.RegisterTool(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Create executions to generate stats
	for i := 0; i < 3; i++ {
		execReq := &proto.ExecuteToolRequest{
			ToolName: "test-stats-tool",
		}
		_, err := service.ExecuteTool(context.Background(), execReq)
		if err != nil {
			t.Fatalf("Failed to create execution: %v", err)
		}
	}

	t.Run("get_all_stats", func(t *testing.T) {
		req := &proto.GetStatsRequest{
			ToolName: "",
		}

		resp, err := service.GetStats(context.Background(), req)
		if err != nil {
			t.Fatalf("GetStats failed: %v", err)
		}

		if resp.ServiceStats == nil {
			t.Error("Expected service_stats in response")
		}
		if len(resp.ToolStats) == 0 {
			t.Error("Expected tool_stats in response")
		}
	})

	t.Run("get_stats_for_specific_tool", func(t *testing.T) {
		req := &proto.GetStatsRequest{
			ToolName: "test-stats-tool",
		}

		resp, err := service.GetStats(context.Background(), req)
		if err != nil {
			t.Fatalf("GetStats for specific tool failed: %v", err)
		}

		if len(resp.ToolStats) != 1 {
			t.Errorf("Expected 1 tool stat, got %d", len(resp.ToolStats))
		}
		if resp.ToolStats[0].ToolName != "test-stats-tool" {
			t.Errorf("Expected tool name 'test-stats-tool', got '%s'", resp.ToolStats[0].ToolName)
		}
	})
}

// =============================================================================
// Test Close
// =============================================================================

func TestToolService_Close(t *testing.T) {
	service := newTestToolService(t)

	t.Run("close_without_streams", func(t *testing.T) {
		err := service.Close()
		if err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})
}

func TestToolService_String(t *testing.T) {
	service := newTestToolService(t)

	t.Run("string_representation", func(t *testing.T) {
		str := service.String()
		if str == "" {
			t.Error("Expected non-empty string representation")
		}
	})
}

// =============================================================================
// Test Registries
// =============================================================================

func TestToolRegistry(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	registry := NewToolRegistry(log)

	t.Run("register_and_get_tool", func(t *testing.T) {
		info := &ToolInfo{
			DisplayName: "Test Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			Version:     "1.0.0",
			UsageStats:  &ToolUsageStatsInfo{},
		}
		toolName := "test-tool"

		err := registry.Register(toolName, info, false)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		retrieved, err := registry.Get(toolName)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if retrieved.DisplayName != "Test Tool" {
			t.Errorf("Expected display name 'Test Tool', got '%s'", retrieved.DisplayName)
		}
	})

	t.Run("register_duplicate_tool", func(t *testing.T) {
		info := &ToolInfo{
			DisplayName: "Duplicate Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			UsageStats:  &ToolUsageStatsInfo{},
		}
		toolName := "duplicate-tool"

		err := registry.Register(toolName, info, false)
		if err != nil {
			t.Fatalf("First register failed: %v", err)
		}

		err = registry.Register(toolName, info, false)
		if err == nil {
			t.Error("Expected error when registering duplicate tool")
		}
	})

	t.Run("list_tools", func(t *testing.T) {
		registry := NewToolRegistry(log)

		// Register multiple tools
		for i := 0; i < 3; i++ {
			info := &ToolInfo{
				DisplayName: "Tool",
				Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
				UsageStats:  &ToolUsageStatsInfo{},
			}
			name := fmt.Sprintf("tool-%d", i)
			_ = registry.Register(name, info, false)
		}

		tools := registry.List()
		if len(tools) != 3 {
			t.Errorf("Expected 3 tools, got %d", len(tools))
		}
	})

	t.Run("unregister_tool", func(t *testing.T) {
		info := &ToolInfo{
			DisplayName: "Unregister Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			UsageStats:  &ToolUsageStatsInfo{},
		}
		toolName := "unregister-test"

		_ = registry.Register(toolName, info, false)
		err := registry.Unregister(toolName, false)
		if err != nil {
			t.Fatalf("Unregister failed: %v", err)
		}

		_, err = registry.Get(toolName)
		if err == nil {
			t.Error("Expected error when getting unregistered tool")
		}
	})

	t.Run("enable_and_disable_tool", func(t *testing.T) {
		info := &ToolInfo{
			DisplayName: "Enable Disable Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			Enabled:     true,
			UsageStats:  &ToolUsageStatsInfo{},
		}
		toolName := "enable-disable-test"

		_ = registry.Register(toolName, info, false)

		// Disable
		err := registry.Disable(toolName)
		if err != nil {
			t.Fatalf("Disable failed: %v", err)
		}

		tool, _ := registry.Get(toolName)
		tool.mu.RLock()
		enabled := tool.Enabled
		state := tool.State
		tool.mu.RUnlock()

		if enabled {
			t.Error("Expected tool to be disabled")
		}
		if state != proto.ToolState_TOOL_STATE_DISABLED {
			t.Errorf("Expected DISABLED state, got %v", state)
		}

		// Enable
		err = registry.Enable(toolName)
		if err != nil {
			t.Fatalf("Enable failed: %v", err)
		}

		tool, _ = registry.Get(toolName)
		tool.mu.RLock()
		enabled = tool.Enabled
		state = tool.State
		tool.mu.RUnlock()

		if !enabled {
			t.Error("Expected tool to be enabled")
		}
		if state != proto.ToolState_TOOL_STATE_AVAILABLE {
			t.Errorf("Expected AVAILABLE state, got %v", state)
		}
	})

	t.Run("record_execution", func(t *testing.T) {
		info := &ToolInfo{
			DisplayName: "Stats Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			UsageStats:  &ToolUsageStatsInfo{},
		}
		toolName := "stats-tool"

		_ = registry.Register(toolName, info, false)

		// Record successful execution
		err := registry.RecordExecution(toolName, 1000000000, true)
		if err != nil {
			t.Fatalf("RecordExecution failed: %v", err)
		}

		tool, _ := registry.Get(toolName)
		tool.UsageStats.mu.RLock()
		totalExecutions := tool.UsageStats.TotalExecutions
		successfulExecutions := tool.UsageStats.SuccessfulExecutions
		avgDuration := tool.UsageStats.AvgDurationNs
		tool.UsageStats.mu.RUnlock()

		if totalExecutions != 1 {
			t.Errorf("Expected 1 total execution, got %d", totalExecutions)
		}
		if successfulExecutions != 1 {
			t.Errorf("Expected 1 successful execution, got %d", successfulExecutions)
		}
		if avgDuration != 1000000000 {
			t.Errorf("Expected avg duration 1000000000, got %d", avgDuration)
		}

		// Record failed execution
		err = registry.RecordExecution(toolName, 2000000000, false)
		if err != nil {
			t.Fatalf("RecordExecution (failed) failed: %v", err)
		}

		tool, _ = registry.Get(toolName)
		tool.UsageStats.mu.RLock()
		totalExecutions = tool.UsageStats.TotalExecutions
		failedExecutions := tool.UsageStats.FailedExecutions
		avgDuration = tool.UsageStats.AvgDurationNs
		tool.UsageStats.mu.RUnlock()

		if totalExecutions != 2 {
			t.Errorf("Expected 2 total executions, got %d", totalExecutions)
		}
		if failedExecutions != 1 {
			t.Errorf("Expected 1 failed execution, got %d", failedExecutions)
		}
		expectedAvg := (1000000000 + 2000000000) / 2
		if avgDuration != expectedAvg {
			t.Errorf("Expected avg duration %d, got %d", expectedAvg, avgDuration)
		}
	})

	t.Run("update_tool", func(t *testing.T) {
		info := &ToolInfo{
			DisplayName: "Update Tool",
			Type:        proto.ToolType_TOOL_TYPE_BUILT_IN,
			UsageStats:  &ToolUsageStatsInfo{},
		}
		toolName := "update-test"

		_ = registry.Register(toolName, info, false)

		newDefinition := &proto.ToolDefinition{
			Name:        toolName,
			DisplayName: "Updated Tool",
			Type:        proto.ToolType_TOOL_TYPE_CONTAINER,
		}

		err := registry.Update(toolName, newDefinition)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		tool, _ := registry.Get(toolName)
		tool.mu.RLock()
		def := tool.Definition
		tool.mu.RUnlock()

		if def.DisplayName != "Updated Tool" {
			t.Errorf("Expected display name 'Updated Tool', got '%s'", def.DisplayName)
		}
	})
}

func TestExecutionRegistry(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	registry := NewExecutionRegistry(log)

	t.Run("add_and_get_execution", func(t *testing.T) {
		execution := &ExecutionInfo{
			ToolName:  "test-tool",
			SessionID: "test-session",
			Status:    proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_PENDING,
		}

		err := registry.Add(execution)
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}

		if execution.ExecutionID == "" {
			t.Error("Expected execution_id to be generated")
		}

		retrieved, err := registry.Get(execution.ExecutionID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if retrieved.ToolName != "test-tool" {
			t.Errorf("Expected tool_name 'test-tool', got '%s'", retrieved.ToolName)
		}
	})

	t.Run("list_all_executions", func(t *testing.T) {
		registry := NewExecutionRegistry(log)

		// Add multiple executions
		for i := 0; i < 3; i++ {
			execution := &ExecutionInfo{
				ToolName:  fmt.Sprintf("tool-%d", i),
				SessionID: "test-session",
			}
			_ = registry.Add(execution)
		}

		executions := registry.List()
		if len(executions) != 3 {
			t.Errorf("Expected 3 executions, got %d", len(executions))
		}
	})

	t.Run("list_by_tool", func(t *testing.T) {
		registry := NewExecutionRegistry(log)

		// Add executions for different tools
		for i := 0; i < 2; i++ {
			execution := &ExecutionInfo{
				ToolName:  "tool-a",
				SessionID: "test-session",
			}
			_ = registry.Add(execution)
		}

		execution := &ExecutionInfo{
			ToolName:  "tool-b",
			SessionID: "test-session",
		}
		_ = registry.Add(execution)

		// List for tool-a
		executions := registry.ListByTool("tool-a")
		if len(executions) != 2 {
			t.Errorf("Expected 2 executions for tool-a, got %d", len(executions))
		}

		// List for tool-b
		executions = registry.ListByTool("tool-b")
		if len(executions) != 1 {
			t.Errorf("Expected 1 execution for tool-b, got %d", len(executions))
		}
	})

	t.Run("update_status", func(t *testing.T) {
		execution := &ExecutionInfo{
			ToolName:  "status-tool",
			SessionID: "test-session",
		}
		_ = registry.Add(execution)

		// Update to running
		err := registry.UpdateStatus(execution.ExecutionID, proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING)
		if err != nil {
			t.Fatalf("UpdateStatus to RUNNING failed: %v", err)
		}

		exec, _ := registry.Get(execution.ExecutionID)
		exec.mu.RLock()
		status := exec.Status
		startedAt := exec.StartedAt
		exec.mu.RUnlock()

		if status != proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING {
			t.Errorf("Expected RUNNING status, got %v", status)
		}
		if startedAt == nil {
			t.Error("Expected started_at to be set")
		}

		// Update to completed
		err = registry.UpdateStatus(execution.ExecutionID, proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_COMPLETED)
		if err != nil {
			t.Fatalf("UpdateStatus to COMPLETED failed: %v", err)
		}

		exec, _ = registry.Get(execution.ExecutionID)
		exec.mu.RLock()
		status = exec.Status
		completedAt := exec.CompletedAt
		durationNs := exec.DurationNs
		exec.mu.RUnlock()

		if status != proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_COMPLETED {
			t.Errorf("Expected COMPLETED status, got %v", status)
		}
		if completedAt == nil {
			t.Error("Expected completed_at to be set")
		}
		if durationNs == 0 {
			t.Error("Expected duration_ns to be set")
		}
	})

	t.Run("cancel_execution", func(t *testing.T) {
		execution := &ExecutionInfo{
			ToolName:  "cancel-tool",
			SessionID: "test-session",
		}
		_ = registry.Add(execution)

		// Update to running first
		_ = registry.UpdateStatus(execution.ExecutionID, proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING)

		err := registry.Cancel(execution.ExecutionID)
		if err != nil {
			t.Fatalf("Cancel failed: %v", err)
		}

		exec, _ := registry.Get(execution.ExecutionID)
		exec.mu.RLock()
		status := exec.Status
		completedAt := exec.CompletedAt
		exec.mu.RUnlock()

		if status != proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_CANCELLED {
			t.Errorf("Expected CANCELLED status, got %v", status)
		}
		if completedAt == nil {
			t.Error("Expected completed_at to be set when cancelled")
		}
	})

	t.Run("get_nonexistent_execution", func(t *testing.T) {
		_, err := registry.Get("non-existent-execution")
		if err == nil {
			t.Error("Expected error for non-existent execution")
		}
	})

	t.Run("update_status_for_nonexistent_execution", func(t *testing.T) {
		err := registry.UpdateStatus("non-existent-execution", proto.ToolExecutionStatus_TOOL_EXECUTION_STATUS_RUNNING)
		if err == nil {
			t.Error("Expected error for non-existent execution")
		}
	})

	t.Run("cancel_nonexistent_execution", func(t *testing.T) {
		err := registry.Cancel("non-existent-execution")
		if err == nil {
			t.Error("Expected error for non-existent execution")
		}
	})
}
