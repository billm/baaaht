package grpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// TestLLMService_CompleteLLM tests the CompleteLLM RPC
func TestLLMService_CompleteLLM(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx := context.Background()

	tests := []struct {
		name    string
		req     *proto.CompleteLLMRequest
		wantErr bool
	}{
		{
			name: "valid LLM request",
			req: &proto.CompleteLLMRequest{
				Request: &proto.LLMRequest{
					RequestId: string(types.GenerateID()),
					SessionId: "test-session",
					Model:     "anthropic/claude-sonnet-4-20250514",
					Messages: []*proto.LLMMessage{
						{
							Role:    "user",
							Content: "Hello, LLM!",
						},
					},
					Parameters: &proto.LLMParameters{
						MaxTokens:   1000,
						Temperature: 0.7,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "request with provider specified",
			req: &proto.CompleteLLMRequest{
				Request: &proto.LLMRequest{
					RequestId: string(types.GenerateID()),
					SessionId: "test-session",
					Model:     "claude-sonnet-4-20250514",
					Provider:  "anthropic",
					Messages: []*proto.LLMMessage{
						{
							Role:    "user",
							Content: "Hello!",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "request with tools",
			req: &proto.CompleteLLMRequest{
				Request: &proto.LLMRequest{
					RequestId: string(types.GenerateID()),
					SessionId: "test-session",
					Model:     "openai/gpt-4o",
					Messages: []*proto.LLMMessage{
						{
							Role:    "user",
							Content: "What's the weather?",
						},
					},
					Tools: []*proto.Tool{
						{
							Name:        "get_weather",
							Description: "Get current weather",
							InputSchema: &proto.ToolInputSchema{
								Type: "object",
								Properties: map[string]*proto.ToolProperty{
									"location": {
										Type:        "string",
										Description: "City name",
									},
								},
								Required: []string{"location"},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.CompleteLLM(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("CompleteLLM() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resp == nil {
					t.Error("CompleteLLM() returned nil response")
					return
				}

				if resp.Response == nil {
					t.Error("CompleteLLM() returned nil response")
					return
				}

				if resp.Response.RequestId != tt.req.Request.RequestId {
					t.Errorf("CompleteLLM() request_id mismatch: got %v, want %v", resp.Response.RequestId, tt.req.Request.RequestId)
				}

				if resp.Response.Content == "" {
					t.Error("CompleteLLM() returned empty content")
				}

				if resp.Response.Usage == nil {
					t.Error("CompleteLLM() returned nil usage")
				}

				if resp.Response.Provider == "" {
					t.Error("CompleteLLM() returned empty provider")
				}
			}
		})
	}
}

// TestLLMService_CompleteLLM_ErrorHandling tests error handling in CompleteLLM
func TestLLMService_CompleteLLM_ErrorHandling(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx := context.Background()

	tests := []struct {
		name    string
		req     *proto.CompleteLLMRequest
		wantErr bool
	}{
		{
			name: "nil request",
			req: &proto.CompleteLLMRequest{
				Request: nil,
			},
			wantErr: true,
		},
		{
			name: "empty request ID",
			req: &proto.CompleteLLMRequest{
				Request: &proto.LLMRequest{
					RequestId: "",
					Model:     "test-model",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.CompleteLLM(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("CompleteLLM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLLMService_StreamLLM tests the StreamLLM bidirectional streaming RPC
func TestLLMService_StreamLLM(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create mock stream
	stream := &mockLLMStream{
		ctx: ctx,
		requests: []*proto.StreamLLMRequest{
			{
				Payload: &proto.StreamLLMRequest_Request{
					Request: &proto.LLMRequest{
						RequestId: string(types.GenerateID()),
						SessionId: "test-session",
						Model:     "anthropic/claude-sonnet-4-20250514",
						Messages: []*proto.LLMMessage{
							{
								Role:    "user",
								Content: "Hello, streaming LLM!",
							},
						},
					},
				},
			},
		},
	}

	// Run the stream in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.StreamLLM(stream)
	}()

	// Wait a bit for streaming to complete
	time.Sleep(2 * time.Second)

	// Cancel context to end the stream
	cancel()

	// Check for errors (context cancellation is expected)
	err := <-errCh
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		t.Errorf("StreamLLM() unexpected error = %v", err)
	}

	// Check that we received responses
	if len(stream.responses) == 0 {
		t.Error("StreamLLM() did not send any responses")
	}

	// Verify response types
	hasUsage := false
	hasChunk := false
	hasComplete := false
	for _, resp := range stream.responses {
		switch resp.Payload.(type) {
		case *proto.StreamLLMResponse_Usage:
			hasUsage = true
		case *proto.StreamLLMResponse_Chunk:
			hasChunk = true
		case *proto.StreamLLMResponse_Complete:
			hasComplete = true
		}
	}

	if !hasUsage {
		t.Error("StreamLLM() did not send usage update")
	}
	if !hasChunk {
		t.Error("StreamLLM() did not send content chunks")
	}
	if !hasComplete {
		t.Error("StreamLLM() did not send completion message")
	}
}

// TestLLMService_StreamLLM_Heartbeat tests heartbeat handling in StreamLLM
func TestLLMService_StreamLLM_Heartbeat(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create mock stream that sends heartbeat
	stream := &mockLLMStream{
		ctx: ctx,
		requests: []*proto.StreamLLMRequest{
			{
				Payload: &proto.StreamLLMRequest_Heartbeat{
					Heartbeat: &emptypb.Empty{},
				},
			},
		},
	}

	// Run the stream
	go func() {
		_ = service.StreamLLM(stream)
	}()

	// Wait for response
	time.Sleep(500 * time.Millisecond)
	cancel()

	if len(stream.responses) == 0 {
		t.Error("StreamLLM() did not respond to heartbeat")
	}

	// Verify heartbeat response
	isHeartbeat := false
	for _, resp := range stream.responses {
		if _, ok := resp.Payload.(*proto.StreamLLMResponse_Heartbeat); ok {
			isHeartbeat = true
			break
		}
	}

	if !isHeartbeat {
		t.Error("StreamLLM() did not send heartbeat response")
	}
}

// TestLLMService_ListModels tests the ListModels RPC
func TestLLMService_ListModels(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx := context.Background()

	tests := []struct {
		name       string
		req        *proto.ListModelsRequest
		wantMin    int
		wantErr    bool
		checkModel string // Optional: check if specific model exists
	}{
		{
			name:    "list all models",
			req:     &proto.ListModelsRequest{},
			wantMin: 3,
			wantErr: false,
		},
		{
			name: "filter by anthropic provider",
			req: &proto.ListModelsRequest{
				Provider: "anthropic",
			},
			wantMin:    2,
			wantErr:    false,
			checkModel: "anthropic/claude-sonnet-4-20250514",
		},
		{
			name: "filter by openai provider",
			req: &proto.ListModelsRequest{
				Provider: "openai",
			},
			wantMin:    1,
			wantErr:    false,
			checkModel: "openai/gpt-4o",
		},
		{
			name: "filter by non-existent provider",
			req: &proto.ListModelsRequest{
				Provider: "nonexistent",
			},
			wantMin: 0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.ListModels(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("ListModels() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resp == nil {
					t.Error("ListModels() returned nil response")
					return
				}

				if len(resp.Models) < tt.wantMin {
					t.Errorf("ListModels() returned %d models, want at least %d", len(resp.Models), tt.wantMin)
				}

				// Check if specific model exists if requested
				if tt.checkModel != "" {
					found := false
					for _, model := range resp.Models {
						if model.Id == tt.checkModel {
							found = true
							if model.Capabilities == nil {
								t.Errorf("ListModels() model %s has nil capabilities", tt.checkModel)
							}
							break
						}
					}
					if !found {
						t.Errorf("ListModels() did not find model %s", tt.checkModel)
					}
				}

				// Verify all models have required fields
				for _, model := range resp.Models {
					if model.Id == "" {
						t.Error("ListModels() returned model with empty ID")
					}
					if model.Name == "" {
						t.Error("ListModels() returned model with empty name")
					}
					if model.Provider == "" {
						t.Error("ListModels() returned model with empty provider")
					}
				}
			}
		})
	}
}

// TestLLMService_GetCapabilities tests the GetCapabilities RPC
func TestLLMService_GetCapabilities(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx := context.Background()

	tests := []struct {
		name         string
		req          *proto.GetCapabilitiesRequest
		wantMin      int
		wantErr      bool
		checkProvider string // Optional: check if specific provider exists
	}{
		{
			name:    "get all capabilities",
			req:     &proto.GetCapabilitiesRequest{},
			wantMin: 2,
			wantErr: false,
		},
		{
			name: "get anthropic capabilities",
			req: &proto.GetCapabilitiesRequest{
				Provider: "anthropic",
			},
			wantMin:      1,
			wantErr:      false,
			checkProvider: "anthropic",
		},
		{
			name: "get openai capabilities",
			req: &proto.GetCapabilitiesRequest{
				Provider: "openai",
			},
			wantMin:      1,
			wantErr:      false,
			checkProvider: "openai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.GetCapabilities(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resp == nil {
					t.Error("GetCapabilities() returned nil response")
					return
				}

				if len(resp.Providers) < tt.wantMin {
					t.Errorf("GetCapabilities() returned %d providers, want at least %d", len(resp.Providers), tt.wantMin)
				}

				// Check if specific provider exists if requested
				if tt.checkProvider != "" {
					found := false
					for _, provider := range resp.Providers {
						if provider.Name == tt.checkProvider {
							found = true
							if !provider.Available {
								t.Errorf("GetCapabilities() provider %s is not available", tt.checkProvider)
							}
							if len(provider.Models) == 0 {
								t.Errorf("GetCapabilities() provider %s has no models", tt.checkProvider)
							}
							break
						}
					}
					if !found {
						t.Errorf("GetCapabilities() did not find provider %s", tt.checkProvider)
					}
				}

				// Verify all providers have required fields
				for _, provider := range resp.Providers {
					if provider.Name == "" {
						t.Error("GetCapabilities() returned provider with empty name")
					}
				}
			}
		})
	}
}

// TestLLMService_HealthCheck tests the HealthCheck RPC
func TestLLMService_HealthCheck(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx := context.Background()

	resp, err := service.HealthCheck(ctx, &emptypb.Empty{})

	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}

	if resp == nil {
		t.Fatal("HealthCheck() returned nil response")
	}

	if resp.Health != proto.Health_HEALTH_HEALTHY {
		t.Errorf("HealthCheck() health = %v, want %v", resp.Health, proto.Health_HEALTH_HEALTHY)
	}

	if resp.Version == "" {
		t.Error("HealthCheck() version is empty")
	}

	if len(resp.AvailableProviders) == 0 {
		t.Error("HealthCheck() available_providers is empty")
	}

	if resp.Timestamp == nil {
		t.Error("HealthCheck() timestamp is nil")
	}
}

// TestLLMService_GetStatus tests the GetStatus RPC
func TestLLMService_GetStatus(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx := context.Background()

	resp, err := service.GetStatus(ctx, &emptypb.Empty{})

	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}

	if resp == nil {
		t.Fatal("GetStatus() returned nil response")
	}

	if resp.Status != proto.Status_STATUS_RUNNING {
		t.Errorf("GetStatus() status = %v, want %v", resp.Status, proto.Status_STATUS_RUNNING)
	}

	if resp.StartedAt == nil {
		t.Error("GetStatus() started_at is nil")
	}

	if resp.Uptime == nil {
		t.Error("GetStatus() uptime is nil")
	}

	if resp.Providers == nil {
		t.Error("GetStatus() providers is nil")
	}

	// Verify provider status
	for providerName, providerStatus := range resp.Providers {
		if providerName == "" {
			t.Error("GetStatus() provider has empty name")
		}
		if providerStatus == nil {
			t.Errorf("GetStatus() provider %s has nil status", providerName)
		}
	}
}

// TestLLMService_GetStatus_AfterRequests tests GetStatus after processing requests
func TestLLMService_GetStatus_AfterRequests(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx := context.Background()

	// Process some requests
	for i := 0; i < 3; i++ {
		_, err := service.CompleteLLM(ctx, &proto.CompleteLLMRequest{
			Request: &proto.LLMRequest{
				RequestId: string(types.GenerateID()),
				SessionId: "test-session",
				Model:     "anthropic/claude-sonnet-4-20250514",
				Messages: []*proto.LLMMessage{
					{
						Role:    "user",
						Content: "Test message",
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("CompleteLLM() failed: %v", err)
		}
	}

	// Check status reflects the requests
	resp, err := service.GetStatus(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}

	if resp.TotalRequests != 3 {
		t.Errorf("GetStatus() total_requests = %v, want 3", resp.TotalRequests)
	}

	if resp.TotalTokensUsed == 0 {
		t.Error("GetStatus() total_tokens_used is 0, expected non-zero")
	}
}

// TestLLMService_Close tests closing the LLM service
func TestLLMService_Close(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)

	// Close should not error
	err := service.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Close again should be safe
	err = service.Close()
	if err != nil {
		t.Errorf("Close() second call error = %v", err)
	}
}

// TestLLMService_String tests the String method
func TestLLMService_String(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)

	str := service.String()
	if str == "" {
		t.Error("String() returned empty string")
	}
}

// TestLLMRegistry tests the LLM registry operations
func TestLLMRegistry(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	registry := NewLLMRegistry(log)

	requestID := "test-request-123"
	info := &LLMRequestInfo{
		SessionID: "test-session",
		Model:     "anthropic/claude-sonnet-4-20250514",
		Provider:  "anthropic",
	}

	// Test Add
	err = registry.Add(requestID, info)
	if err != nil {
		t.Errorf("Registry.Add() error = %v", err)
	}

	// Test Add duplicate
	err = registry.Add(requestID, info)
	if err == nil {
		t.Error("Registry.Add() duplicate should return error")
	}

	// Test Get
	retrieved, err := registry.Get(requestID)
	if err != nil {
		t.Errorf("Registry.Get() error = %v", err)
	}
	if retrieved.RequestID != requestID {
		t.Errorf("Registry.Get() request_id = %v, want %v", retrieved.RequestID, requestID)
	}

	// Test Get non-existent
	_, err = registry.Get("non-existent")
	if err == nil {
		t.Error("Registry.Get() non-existent should return error")
	}

	// Test UpdateState
	err = registry.UpdateState(requestID, "processing")
	if err != nil {
		t.Errorf("Registry.UpdateState() error = %v", err)
	}
	retrieved, _ = registry.Get(requestID)
	if retrieved.State != "processing" {
		t.Errorf("Registry.Get() state = %v, want 'processing'", retrieved.State)
	}
	if retrieved.StartedAt == nil {
		t.Error("Registry.Get() StartedAt should be set after UpdateState to 'processing'")
	}

	// Test IncrementTokens
	err = registry.IncrementTokens(requestID, 10, 20)
	if err != nil {
		t.Errorf("Registry.IncrementTokens() error = %v", err)
	}
	retrieved, _ = registry.Get(requestID)
	if retrieved.InputTokens != 10 {
		t.Errorf("Registry.Get() input_tokens = %v, want 10", retrieved.InputTokens)
	}
	if retrieved.OutputTokens != 20 {
		t.Errorf("Registry.Get() output_tokens = %v, want 20", retrieved.OutputTokens)
	}

	// Test IncrementStreamChunks
	err = registry.IncrementStreamChunks(requestID)
	if err != nil {
		t.Errorf("Registry.IncrementStreamChunks() error = %v", err)
	}
	retrieved, _ = registry.Get(requestID)
	if retrieved.StreamChunks != 1 {
		t.Errorf("Registry.Get() stream_chunks = %v, want 1", retrieved.StreamChunks)
	}

	// Test Remove
	err = registry.Remove(requestID)
	if err != nil {
		t.Errorf("Registry.Remove() error = %v", err)
	}
	if retrieved.State != "completed" {
		t.Errorf("Registry.Get() state = %v, want 'completed'", retrieved.State)
	}
	if retrieved.CompletedAt == nil {
		t.Error("Registry.Get() CompletedAt should be set after Remove")
	}

	// Test Get after Remove
	_, err = registry.Get(requestID)
	if err == nil {
		t.Error("Registry.Get() after Remove should return error")
	}
}

// TestLLMService_ConcurrentRequests tests concurrent LLM requests
func TestLLMService_ConcurrentRequests(t *testing.T) {
	srv := setupTestServer(t)
	defer teardownTestServer(t, srv)

	service := NewLLMService(srv, nil)
	ctx := context.Background()

	const numRequests = 10
	var wg sync.WaitGroup
	errCh := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := service.CompleteLLM(ctx, &proto.CompleteLLMRequest{
				Request: &proto.LLMRequest{
					RequestId: string(types.GenerateID()),
					SessionId: "test-session",
					Model:     "anthropic/claude-sonnet-4-20250514",
					Messages: []*proto.LLMMessage{
						{
							Role:    "user",
							Content: "Test message",
						},
					},
				},
			})
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	for err := range errCh {
		t.Errorf("Concurrent request failed: %v", err)
	}

	// Verify all requests were tracked
	resp, err := service.GetStatus(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}

	if resp.TotalRequests != int64(numRequests) {
		t.Errorf("GetStatus() total_requests = %v, want %d", resp.TotalRequests, numRequests)
	}
}

// mockLLMStream implements the LLMService_StreamLLMServer interface for testing
type mockLLMStream struct {
	proto.LLMService_StreamLLMServer
	ctx        context.Context
	requests   []*proto.StreamLLMRequest
	sentCount  int
	responses  []*proto.StreamLLMResponse
	mu         sync.Mutex
}

func (m *mockLLMStream) Context() context.Context {
	return m.ctx
}

func (m *mockLLMStream) Send(resp *proto.StreamLLMResponse) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, resp)
	return nil
}

func (m *mockLLMStream) Recv() (*proto.StreamLLMRequest, error) {
	if m.sentCount >= len(m.requests) {
		return nil, context.DeadlineExceeded
	}
	req := m.requests[m.sentCount]
	m.sentCount++
	return req, nil
}

func (m *mockLLMStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockLLMStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockLLMStream) SetTrailer(metadata.MD) {
}
