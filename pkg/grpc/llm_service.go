package grpc

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/events"
	"github.com/billm/baaaht/orchestrator/pkg/ipc"
	"github.com/billm/baaaht/orchestrator/pkg/session"
	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/billm/baaaht/orchestrator/proto"
)

// LLMServiceDependencies represents the LLM service dependencies
type LLMServiceDependencies interface {
	SessionManager() *session.Manager
	EventBus() *events.Bus
	IPCBroker() *ipc.Broker
}

// LLMRequestInfo tracks information about an active LLM request
type LLMRequestInfo struct {
	RequestID      string
	SessionID      string
	Model          string
	Provider       string
	State          string
	CreatedAt      types.Timestamp
	StartedAt      *types.Timestamp
	CompletedAt    *types.Timestamp
	InputTokens    int32
	OutputTokens   int32
	StreamChunks   int32
	mu             sync.RWMutex
}

// LLMRegistry tracks active LLM requests
type LLMRegistry struct {
	mu       sync.RWMutex
	requests map[string]*LLMRequestInfo
	logger   *logger.Logger
}

// NewLLMRegistry creates a new LLM registry
func NewLLMRegistry(log *logger.Logger) *LLMRegistry {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	return &LLMRegistry{
		requests: make(map[string]*LLMRequestInfo),
		logger:  log.With("component", "llm_registry"),
	}
}

// Add adds a request to the registry
func (r *LLMRegistry) Add(requestID string, info *LLMRequestInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.requests[requestID]; exists {
		return types.NewError(types.ErrCodeAlreadyExists, "LLM request already exists")
	}

	info.RequestID = requestID
	info.CreatedAt = types.NewTimestamp()
	info.State = "pending"
	r.requests[requestID] = info

	r.logger.Debug("LLM request registered", "request_id", requestID, "model", info.Model)

	return nil
}

// Get retrieves a request by ID
func (r *LLMRegistry) Get(requestID string) (*LLMRequestInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	req, exists := r.requests[requestID]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, "LLM request not found")
	}

	return req, nil
}

// Remove removes a request from the registry
func (r *LLMRegistry) Remove(requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	req, exists := r.requests[requestID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "LLM request not found")
	}

	req.mu.Lock()
	req.State = "completed"
	completedAt := types.NewTimestamp()
	req.CompletedAt = &completedAt
	req.mu.Unlock()

	delete(r.requests, requestID)

	r.logger.Debug("LLM request removed", "request_id", requestID)

	return nil
}

// UpdateState updates the state of a request
func (r *LLMRegistry) UpdateState(requestID string, state string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	req, exists := r.requests[requestID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "LLM request not found")
	}

	req.mu.Lock()
	req.State = state
	if state == "processing" && req.StartedAt == nil {
		startedAt := types.NewTimestamp()
		req.StartedAt = &startedAt
	}
	req.mu.Unlock()

	return nil
}

// IncrementTokens increments the token counts for a request
func (r *LLMRegistry) IncrementTokens(requestID string, inputTokens, outputTokens int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	req, exists := r.requests[requestID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "LLM request not found")
	}

	req.mu.Lock()
	req.InputTokens += inputTokens
	req.OutputTokens += outputTokens
	req.mu.Unlock()

	return nil
}

// IncrementStreamChunks increments the stream chunk count for a request
func (r *LLMRegistry) IncrementStreamChunks(requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	req, exists := r.requests[requestID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "LLM request not found")
	}

	req.mu.Lock()
	req.StreamChunks++
	req.mu.Unlock()

	return nil
}

// LLMService implements the gRPC LLMService interface
type LLMService struct {
	proto.UnimplementedLLMServiceServer
	deps     LLMServiceDependencies
	registry *LLMRegistry
	logger   *logger.Logger
	mu       sync.RWMutex

	// Track active streams for graceful shutdown
	streams map[interface{}]context.CancelFunc

	// Track server start time for uptime calculation
	startedAt time.Time

	// Statistics
	totalRequests      int64
	totalTokensUsed    int64
}

// NewLLMService creates a new LLM service
func NewLLMService(deps LLMServiceDependencies, log *logger.Logger) *LLMService {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	return &LLMService{
		deps:      deps,
		registry:  NewLLMRegistry(log),
		logger:    log.With("component", "llm_service"),
		streams:   make(map[interface{}]context.CancelFunc),
		startedAt: time.Now(),
	}
}

// =============================================================================
// LLM Request/Response RPCs
// =============================================================================

// StreamLLM establishes a bidirectional stream for LLM request/response handling
func (s *LLMService) StreamLLM(stream proto.LLMService_StreamLLMServer) error {
	ctx := stream.Context()
	s.logger.Debug("StreamLLM started")

	// Track this stream for cleanup
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.mu.Lock()
	s.streams[stream] = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.streams, stream)
		s.mu.Unlock()
	}()

	var requestID string
	var firstMessage = true

	// Receive loop
	for {
		req, err := stream.Recv()
		if err != nil {
			// Handle io.EOF explicitly - this is a normal stream end
			if err == io.EOF {
				s.logger.Debug("StreamLLM stream ended normally (EOF)", "request_id", requestID, "firstMessage", firstMessage)
				return nil
			}
			if firstMessage {
				s.logger.Debug("StreamLLM ended without receiving any message", "error", err)
				return err
			}
			// Client closed stream with other error
			s.logger.Debug("StreamLLM receive loop ended", "request_id", requestID, "error", err)
			return nil
		}

		// Handle heartbeat
		if req.GetHeartbeat() != nil {
			// Send heartbeat response
			if err := stream.Send(&proto.StreamLLMResponse{
				Payload: &proto.StreamLLMResponse_Heartbeat{&emptypb.Empty{}},
			}); err != nil {
				s.logger.Error("Failed to send heartbeat response", "error", err)
				return err
			}
			continue
		}

		// First message must contain request
		if firstMessage {
			llmReq := req.GetRequest()
			if llmReq == nil {
				return errInvalidArgument("first message must contain LLM request")
			}

			if llmReq.RequestId == "" {
				return errInvalidArgument("request_id is required")
			}

			requestID = llmReq.RequestId
			firstMessage = false

			// Create request info
			info := &LLMRequestInfo{
				SessionID: llmReq.SessionId,
				Model:     llmReq.Model,
				Provider:  llmReq.Provider,
			}

			if llmReq.Provider == "" {
				// Derive provider from model if not specified
				info.Provider = s.deriveProviderFromModel(llmReq.Model)
			}

			if err := s.registry.Add(requestID, info); err != nil {
				s.logger.Error("Failed to register LLM request", "request_id", requestID, "error", err)
				return grpcErrorFromTypesError(err)
			}

			// Update state to processing
			_ = s.registry.UpdateState(requestID, "processing")

			s.logger.Debug("StreamLLM established", "request_id", requestID, "model", llmReq.Model)
		}

		// Process the LLM request
		llmReq := req.GetRequest()
		if llmReq != nil && requestID == llmReq.RequestId {
			// Send simulated streaming response
			if err := s.handleStreamingRequest(streamCtx, stream, llmReq); err != nil {
				s.logger.Error("Failed to handle streaming request", "request_id", requestID, "error", err)

				// Send error response
				_ = stream.Send(&proto.StreamLLMResponse{
					Payload: &proto.StreamLLMResponse_Error{
						Error: &proto.StreamError{
							RequestId:          requestID,
							Code:               "INTERNAL_ERROR",
							Message:            err.Error(),
							Retryable:          false,
							SuggestedProvider:  "",
						},
					},
				})
				continue
			}

			// Send completion response
			completedAt := types.NewTimestamp()
			resp := &proto.StreamLLMResponse{
				Payload: &proto.StreamLLMResponse_Complete{
					Complete: &proto.StreamComplete{
						RequestId: requestID,
						Response: &proto.LLMResponse{
							RequestId:   requestID,
							Content:     "This is a simulated response from the LLM service.",
							ToolCalls:   []*proto.ToolCall{},
							Usage: &proto.TokenUsage{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
							FinishReason: proto.FinishReason_FINISH_REASON_STOP,
							Provider:     s.deriveProviderFromModel(llmReq.Model),
							Model:        llmReq.Model,
							CompletedAt:  timestampToProto(&completedAt),
							Metadata:     llmReq.Metadata,
						},
					},
				},
			}
			if err := stream.Send(resp); err != nil {
				s.logger.Error("Failed to send completion response", "request_id", requestID, "error", err)
				return err
			}

			// Update registry
			_ = s.registry.UpdateState(requestID, "completed")
			_ = s.registry.Remove(requestID)

			// Update statistics
			s.mu.Lock()
			s.totalRequests++
			s.totalTokensUsed += 30
			s.mu.Unlock()
		}
	}
}

// handleStreamingRequest simulates streaming response chunks
func (s *LLMService) handleStreamingRequest(ctx context.Context, stream proto.LLMService_StreamLLMServer, req *proto.LLMRequest) error {
	requestID := req.RequestId
	responseText := "This is a simulated response from the LLM service."

	// Send initial usage
	_ = stream.Send(&proto.StreamLLMResponse{
		Payload: &proto.StreamLLMResponse_Usage{
			Usage: &proto.StreamUsage{
				RequestId: requestID,
				Usage: &proto.TokenUsage{
					InputTokens: 10,
				},
			},
		},
	})

	// Stream response in chunks
	chunkSize := 10
	for i := 0; i < len(responseText); i += chunkSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := i + chunkSize
		if end > len(responseText) {
			end = len(responseText)
		}

		isLast := end >= len(responseText)

		chunk := &proto.StreamChunk{
			RequestId: requestID,
			Content:   responseText[i:end],
			Index:     int32(i / chunkSize),
			IsLast:    isLast,
		}

		if err := stream.Send(&proto.StreamLLMResponse{
			Payload: &proto.StreamLLMResponse_Chunk{
				Chunk: chunk,
			},
		}); err != nil {
			return err
		}

		// Update registry
		_ = s.registry.IncrementStreamChunks(requestID)

		// Small delay between chunks
		time.Sleep(50 * time.Millisecond)
	}

	// Send final usage
	_ = stream.Send(&proto.StreamLLMResponse{
		Payload: &proto.StreamLLMResponse_Usage{
			Usage: &proto.StreamUsage{
				RequestId: requestID,
				Usage: &proto.TokenUsage{
					InputTokens:  10,
					OutputTokens: 20,
					TotalTokens:  30,
				},
			},
		},
	})

	return nil
}

// CompleteLLM sends a non-streaming LLM request and receives a complete response
func (s *LLMService) CompleteLLM(ctx context.Context, req *proto.CompleteLLMRequest) (*proto.CompleteLLMResponse, error) {
	llmReq := req.Request
	if llmReq == nil {
		return nil, errInvalidArgument("request is required")
	}

	if llmReq.RequestId == "" {
		return nil, errInvalidArgument("request_id is required")
	}

	requestID := llmReq.RequestId
	s.logger.Debug("CompleteLLM called", "request_id", requestID)

	// Create request info
	info := &LLMRequestInfo{
		SessionID: llmReq.SessionId,
		Model:     llmReq.Model,
		Provider:  llmReq.Provider,
	}

	if llmReq.Provider == "" {
		info.Provider = s.deriveProviderFromModel(llmReq.Model)
	}

	if err := s.registry.Add(requestID, info); err != nil {
		s.logger.Error("Failed to register LLM request", "request_id", requestID, "error", err)
		return nil, grpcErrorFromTypesError(err)
	}

	// Update state to processing
	_ = s.registry.UpdateState(requestID, "processing")

	s.logger.Debug("Processing LLM request", "request_id", requestID, "model", llmReq.Model)

	// Simulate processing delay
	time.Sleep(100 * time.Millisecond)

	// Create response
	completedAt := types.NewTimestamp()
	response := &proto.LLMResponse{
		RequestId:   requestID,
		Content:     "This is a simulated response from the LLM service.",
		ToolCalls:   []*proto.ToolCall{},
		Usage: &proto.TokenUsage{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
		FinishReason: proto.FinishReason_FINISH_REASON_STOP,
		Provider:     info.Provider,
		Model:        llmReq.Model,
		CompletedAt:  timestampToProto(&completedAt),
		Metadata:     llmReq.Metadata,
	}

	// Update registry
	_ = s.registry.UpdateState(requestID, "completed")
	_ = s.registry.Remove(requestID)

	// Update statistics
	s.mu.Lock()
	s.totalRequests++
	s.totalTokensUsed += 30
	s.mu.Unlock()

	return &proto.CompleteLLMResponse{
		Response: response,
	}, nil
}

// =============================================================================
// Configuration and Capabilities RPCs
// =============================================================================

// ListModels lists all available models for the specified provider
func (s *LLMService) ListModels(ctx context.Context, req *proto.ListModelsRequest) (*proto.ListModelsResponse, error) {
	s.logger.Debug("ListModels called", "provider", req.Provider)

	// Return simulated model list
	// In a real implementation, this would query the LLM providers
	models := []*proto.ModelInfo{
		{
			Id:   "anthropic/claude-sonnet-4-20250514",
			Name: "Claude Sonnet 4",
			Provider: "anthropic",
			Capabilities: &proto.ModelCapabilities{
				Streaming: true,
				Tools:     true,
				Vision:    true,
				Thinking:  true,
			},
			Metadata: map[string]string{
				"context_window": "200000",
				"pricing_input":  "$3.00/M tokens",
				"pricing_output": "$15.00/M tokens",
			},
		},
		{
			Id:   "anthropic/claude-3-5-sonnet-20241022",
			Name: "Claude 3.5 Sonnet",
			Provider: "anthropic",
			Capabilities: &proto.ModelCapabilities{
				Streaming: true,
				Tools:     true,
				Vision:    true,
				Thinking:  false,
			},
			Metadata: map[string]string{
				"context_window": "200000",
			},
		},
		{
			Id:   "openai/gpt-4o",
			Name: "GPT-4o",
			Provider: "openai",
			Capabilities: &proto.ModelCapabilities{
				Streaming: true,
				Tools:     true,
				Vision:    true,
				Thinking:  false,
			},
			Metadata: map[string]string{
				"context_window": "128000",
			},
		},
	}

	// Filter by provider if specified
	if req.Provider != "" {
		filtered := make([]*proto.ModelInfo, 0)
		for _, model := range models {
			if model.Provider == req.Provider {
				filtered = append(filtered, model)
			}
		}
		models = filtered
	}

	return &proto.ListModelsResponse{
		Models: models,
	}, nil
}

// GetCapabilities returns the capabilities of available LLM providers
func (s *LLMService) GetCapabilities(ctx context.Context, req *proto.GetCapabilitiesRequest) (*proto.GetCapabilitiesResponse, error) {
	s.logger.Debug("GetCapabilities called", "provider", req.Provider)

	// Return simulated provider capabilities
	providers := []*proto.ProviderCapabilities{
		{
			Name:      "anthropic",
			Available: true,
			Models: []*proto.ModelInfo{
				{
					Id:   "anthropic/claude-sonnet-4-20250514",
					Name: "Claude Sonnet 4",
					Provider: "anthropic",
					Capabilities: &proto.ModelCapabilities{
						Streaming: true,
						Tools:     true,
						Vision:    true,
						Thinking:  true,
					},
				},
			},
			Metadata: map[string]string{
				"api_version": "2023-06-01",
			},
		},
		{
			Name:      "openai",
			Available: true,
			Models: []*proto.ModelInfo{
				{
					Id:   "openai/gpt-4o",
					Name: "GPT-4o",
					Provider: "openai",
					Capabilities: &proto.ModelCapabilities{
						Streaming: true,
						Tools:     true,
						Vision:    true,
						Thinking:  false,
					},
				},
			},
			Metadata: map[string]string{
				"api_version": "v2",
			},
		},
	}

	// Filter by provider if specified
	if req.Provider != "" {
		filtered := make([]*proto.ProviderCapabilities, 0)
		for _, provider := range providers {
			if provider.Name == req.Provider {
				filtered = append(filtered, provider)
			}
		}
		providers = filtered
	}

	return &proto.GetCapabilitiesResponse{
		Providers: providers,
	}, nil
}

// =============================================================================
// Health and Status RPCs
// =============================================================================

// HealthCheck returns the health status of the LLM Gateway
func (s *LLMService) HealthCheck(ctx context.Context, req *emptypb.Empty) (*proto.LLMHealthCheckResponse, error) {
	s.logger.Debug("HealthCheck called")

	// Check dependencies
	mgr := s.deps.SessionManager()
	bus := s.deps.EventBus()
	var health proto.Health
	if mgr != nil && bus != nil {
		health = proto.Health_HEALTH_HEALTHY
	} else {
		health = proto.Health_HEALTH_UNHEALTHY
	}

	// TODO: Add version from build info
	version := "dev"

	availableProviders := []string{"anthropic", "openai"}
	unavailableProviders := []string{}

	return &proto.LLMHealthCheckResponse{
		Health:              health,
		Version:             version,
		AvailableProviders:  availableProviders,
		UnavailableProviders: unavailableProviders,
		Timestamp:           timestamppb.Now(),
	}, nil
}

// GetStatus returns the current status of the LLM Gateway
func (s *LLMService) GetStatus(ctx context.Context, req *emptypb.Empty) (*proto.LLMStatusResponse, error) {
	s.logger.Debug("GetStatus called")

	s.mu.RLock()
	startedAt := s.startedAt
	totalRequests := s.totalRequests
	totalTokensUsed := s.totalTokensUsed
	s.mu.RUnlock()

	uptime := time.Since(startedAt)
	startedTs := types.NewTimestampFromTime(startedAt)

	// Get provider statuses
	providers := map[string]*proto.ProviderStatus{
		"anthropic": {
			Available:       true,
			Health:          proto.Health_HEALTH_HEALTHY,
			ActiveRequests:  0,
			TotalRequests:   totalRequests / 2, // Approximate split
			AvgResponseTimeMs: 500.0,
			LastRequestAt:   timestamppb.Now(),
		},
		"openai": {
			Available:       true,
			Health:          proto.Health_HEALTH_HEALTHY,
			ActiveRequests:  0,
			TotalRequests:   totalRequests / 2,
			AvgResponseTimeMs: 400.0,
			LastRequestAt:   timestamppb.Now(),
		},
	}

	return &proto.LLMStatusResponse{
		Status:           proto.Status_STATUS_RUNNING,
		StartedAt:        timestampToProto(&startedTs),
		Uptime:           durationpb.New(uptime),
		ActiveRequests:   0,
		TotalRequests:    totalRequests,
		TotalTokensUsed:  totalTokensUsed,
		Providers:        providers,
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// deriveProviderFromModel derives the provider name from the model ID
func (s *LLMService) deriveProviderFromModel(model string) string {
	// Simple heuristic: provider is the first part of the model ID
	// e.g., "anthropic/claude-sonnet-4" -> "anthropic"
	for i, c := range model {
		if c == '/' {
			return model[:i]
		}
	}
	// Default fallback
	return "unknown"
}

// Close closes all active streams
func (s *LLMService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Closing LLM service streams", "count", len(s.streams))

	for stream, cancel := range s.streams {
		cancel()
		delete(s.streams, stream)
	}

	return nil
}

// String returns a string representation of the service
func (s *LLMService) String() string {
	s.mu.RLock()
	activeStreams := len(s.streams)
	totalRequests := s.totalRequests
	s.mu.RUnlock()

	return fmt.Sprintf("LLMService{active_streams: %d, total_requests: %d}", activeStreams, totalRequests)
}
