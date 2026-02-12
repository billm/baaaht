package grpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
	RequestID    string
	SessionID    string
	Model        string
	Provider     string
	State        string
	CreatedAt    types.Timestamp
	StartedAt    *types.Timestamp
	CompletedAt  *types.Timestamp
	InputTokens  int32
	OutputTokens int32
	StreamChunks int32
	mu           sync.RWMutex
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
		logger:   log.With("component", "llm_registry"),
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
	deps       LLMServiceDependencies
	registry   *LLMRegistry
	logger     *logger.Logger
	http       *http.Client
	gatewayURL string
	mu         sync.RWMutex

	// Track active streams for graceful shutdown
	streams map[interface{}]context.CancelFunc

	// Track server start time for uptime calculation
	startedAt time.Time

	// Statistics
	totalRequests   int64
	totalTokensUsed int64
}

const defaultLLMGatewayURL = "http://127.0.0.1:8080"

// NewLLMService creates a new LLM service
func NewLLMService(deps LLMServiceDependencies, log *logger.Logger) *LLMService {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			log = &logger.Logger{}
		}
	}

	service := &LLMService{
		deps:     deps,
		registry: NewLLMRegistry(log),
		logger:   log.With("component", "llm_service"),
		http: &http.Client{
			Timeout: 130 * time.Second,
		},
		gatewayURL: strings.TrimRight(resolveLLMGatewayURL(), "/"),
		streams:    make(map[interface{}]context.CancelFunc),
		startedAt:  time.Now(),
	}

	service.logger.Info("LLMService proxy configured", "gateway_url", service.gatewayURL)

	return service
}

func resolveLLMGatewayURL() string {
	if url := strings.TrimSpace(os.Getenv("BAAAHT_LLM_GATEWAY_URL")); url != "" {
		return url
	}
	if url := strings.TrimSpace(os.Getenv("LLM_GATEWAY_URL")); url != "" {
		return url
	}
	return defaultLLMGatewayURL
}

func traceValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func (s *LLMService) traceFlow(stage, correlationID, sessionID, requestID string, details ...interface{}) {
	line := fmt.Sprintf("TRACE_FLOW stage=%s cid=%s sid=%s rid=%s",
		stage,
		traceValue(correlationID),
		traceValue(sessionID),
		traceValue(requestID),
	)
	if len(details) > 0 {
		s.logger.Info(line, details...)
		return
	}
	s.logger.Info(line)
}

// =============================================================================
// LLM Request/Response RPCs
// =============================================================================

// StreamLLM establishes a bidirectional stream for LLM request/response handling
func (s *LLMService) StreamLLM(stream proto.LLMService_StreamLLMServer) error {
	ctx := stream.Context()
	s.logger.Info("StreamLLM started")
	s.traceFlow("orchestrator.llm.stream.open", "", "", "")

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
			correlationID := ""
			if llmReq.Metadata != nil {
				correlationID = llmReq.Metadata.CorrelationId
			}
			s.traceFlow("orchestrator.llm.stream.recv_request", correlationID, llmReq.SessionId, llmReq.RequestId,
				"model", llmReq.Model,
				"provider", llmReq.Provider,
				"message_count", len(llmReq.Messages),
			)

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
			correlationID := ""
			if llmReq.Metadata != nil {
				correlationID = llmReq.Metadata.CorrelationId
			}
			s.traceFlow("orchestrator.llm.stream.dispatch_gateway", correlationID, llmReq.SessionId, llmReq.RequestId,
				"gateway_url", s.gatewayURL,
			)
			// Proxy streaming response via llm-gateway
			if err := s.handleStreamingRequest(streamCtx, stream, llmReq); err != nil {
				s.logger.Error("Failed to handle streaming request", "request_id", requestID, "error", err)

				// Send error response
				_ = stream.Send(&proto.StreamLLMResponse{
					Payload: &proto.StreamLLMResponse_Error{
						Error: &proto.StreamError{
							RequestId:         requestID,
							Code:              "INTERNAL_ERROR",
							Message:           err.Error(),
							Retryable:         false,
							SuggestedProvider: "",
						},
					},
				})
				continue
			}

			// Update registry
			_ = s.registry.UpdateState(requestID, "completed")
			_ = s.registry.Remove(requestID)
		}
	}
}

// handleStreamingRequest proxies streaming response chunks from llm-gateway SSE
func (s *LLMService) handleStreamingRequest(ctx context.Context, stream proto.LLMService_StreamLLMServer, req *proto.LLMRequest) error {
	startedAt := time.Now()
	correlationID := ""
	if req.Metadata != nil {
		correlationID = req.Metadata.CorrelationId
	}
	gatewayReq := s.toGatewayRequest(req, true)
	payload, err := json.Marshal(gatewayReq)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.gatewayURL+"/v1/stream", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	s.logger.Debug("Proxying StreamLLM request to gateway",
		"request_id", req.RequestId,
		"session_id", req.SessionId,
		"model", req.Model,
		"provider", req.Provider,
		"gateway_url", s.gatewayURL)
	s.logger.Info("Dispatching stream request to llm-gateway",
		"request_id", req.RequestId,
		"session_id", req.SessionId,
		"model", req.Model,
		"provider", req.Provider,
		"message_count", len(req.Messages))
	s.traceFlow("orchestrator.llm.stream.gateway_http_start", correlationID, req.SessionId, req.RequestId,
		"gateway_url", s.gatewayURL,
	)

	httpResp, err := s.http.Do(httpReq)
	if err != nil {
		s.logger.Error("Failed to call llm-gateway stream endpoint",
			"request_id", req.RequestId,
			"gateway_url", s.gatewayURL,
			"error", err)
		return err
	}
	defer httpResp.Body.Close()

	s.logger.Info("Received llm-gateway stream HTTP response",
		"request_id", req.RequestId,
		"status_code", httpResp.StatusCode,
		"elapsed_ms", time.Since(startedAt).Milliseconds())
	s.traceFlow("orchestrator.llm.stream.gateway_http_response", correlationID, req.SessionId, req.RequestId,
		"status_code", httpResp.StatusCode,
	)

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 16*1024))
		s.logger.Error("llm-gateway stream endpoint returned non-200",
			"request_id", req.RequestId,
			"status_code", httpResp.StatusCode,
			"response_body", strings.TrimSpace(string(body)))
		return fmt.Errorf("gateway stream request failed: status=%d body=%s", httpResp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(httpResp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	eventCount := 0
	chunkCount := 0
	completeCount := 0
	errorCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var event gatewayStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			s.logger.Warn("Failed to decode gateway stream event", "request_id", req.RequestId, "error", err)
			continue
		}
		eventCount++
		switch event.Type {
		case "chunk":
			chunkCount++
		case "complete":
			completeCount++
		case "error":
			errorCount++
		}

		resp, usageDelta := gatewayEventToProtoResponse(event)
		if resp == nil {
			continue
		}
		if err := stream.Send(resp); err != nil {
			return err
		}

		if _, ok := resp.Payload.(*proto.StreamLLMResponse_Chunk); ok {
			_ = s.registry.IncrementStreamChunks(req.RequestId)
		}

		if usageDelta != nil {
			_ = s.registry.IncrementTokens(req.RequestId, usageDelta.InputTokens, usageDelta.OutputTokens)
			s.mu.Lock()
			s.totalRequests++
			s.totalTokensUsed += int64(usageDelta.TotalTokens)
			s.mu.Unlock()
		}
	}

	if err := scanner.Err(); err != nil {
		s.logger.Error("Error reading llm-gateway SSE stream",
			"request_id", req.RequestId,
			"error", err)
		return err
	}

	s.logger.Info("Completed llm-gateway stream relay",
		"request_id", req.RequestId,
		"event_count", eventCount,
		"chunk_count", chunkCount,
		"complete_count", completeCount,
		"error_event_count", errorCount,
		"elapsed_ms", time.Since(startedAt).Milliseconds())
	s.traceFlow("orchestrator.llm.stream.relay_done", correlationID, req.SessionId, req.RequestId,
		"event_count", eventCount,
		"chunk_count", chunkCount,
		"elapsed_ms", time.Since(startedAt).Milliseconds(),
	)

	return nil
}

// CompleteLLM sends a non-streaming LLM request and receives a complete response
func (s *LLMService) CompleteLLM(ctx context.Context, req *proto.CompleteLLMRequest) (*proto.CompleteLLMResponse, error) {
	startedAt := time.Now()
	llmReq := req.Request
	if llmReq == nil {
		return nil, errInvalidArgument("request is required")
	}

	if llmReq.RequestId == "" {
		return nil, errInvalidArgument("request_id is required")
	}

	requestID := llmReq.RequestId
	s.logger.Info("CompleteLLM called", "request_id", requestID)
	correlationID := ""
	if llmReq.Metadata != nil {
		correlationID = llmReq.Metadata.CorrelationId
	}
	s.traceFlow("orchestrator.llm.complete.recv_request", correlationID, llmReq.SessionId, requestID,
		"model", llmReq.Model,
		"provider", llmReq.Provider,
		"message_count", len(llmReq.Messages),
	)

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

	s.logger.Debug("Proxying CompleteLLM request to gateway",
		"request_id", requestID,
		"session_id", llmReq.SessionId,
		"model", llmReq.Model,
		"provider", llmReq.Provider,
		"gateway_url", s.gatewayURL)
	s.logger.Info("Dispatching completion request to llm-gateway",
		"request_id", requestID,
		"session_id", llmReq.SessionId,
		"model", llmReq.Model,
		"provider", llmReq.Provider,
		"message_count", len(llmReq.Messages))
	s.traceFlow("orchestrator.llm.complete.gateway_http_start", correlationID, llmReq.SessionId, requestID,
		"gateway_url", s.gatewayURL,
	)

	gatewayReq := s.toGatewayRequest(llmReq, false)
	payload, err := json.Marshal(gatewayReq)
	if err != nil {
		_ = s.registry.Remove(requestID)
		return nil, grpcErrorFromTypesError(types.WrapError(types.ErrCodeInternal, "failed to encode gateway request", err))
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.gatewayURL+"/v1/completions", bytes.NewReader(payload))
	if err != nil {
		_ = s.registry.Remove(requestID)
		return nil, grpcErrorFromTypesError(types.WrapError(types.ErrCodeInternal, "failed to create gateway request", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := s.http.Do(httpReq)
	if err != nil {
		s.logger.Error("Failed to call llm-gateway completions endpoint",
			"request_id", requestID,
			"gateway_url", s.gatewayURL,
			"error", err)
		_ = s.registry.Remove(requestID)
		return nil, grpcErrorFromTypesError(types.WrapError(types.ErrCodeUnavailable, "failed to call llm-gateway", err))
	}
	defer httpResp.Body.Close()

	s.logger.Info("Received llm-gateway completion HTTP response",
		"request_id", requestID,
		"status_code", httpResp.StatusCode,
		"elapsed_ms", time.Since(startedAt).Milliseconds())
	s.traceFlow("orchestrator.llm.complete.gateway_http_response", correlationID, llmReq.SessionId, requestID,
		"status_code", httpResp.StatusCode,
	)

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 16*1024))
		s.logger.Error("llm-gateway completions endpoint returned non-200",
			"request_id", requestID,
			"status_code", httpResp.StatusCode,
			"response_body", strings.TrimSpace(string(body)))
		_ = s.registry.Remove(requestID)
		return nil, grpcErrorFromTypesError(types.NewError(types.ErrCodeInternal,
			fmt.Sprintf("llm-gateway completion failed: status=%d body=%s", httpResp.StatusCode, strings.TrimSpace(string(body)))))
	}

	var gatewayResp gatewayCompleteResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&gatewayResp); err != nil {
		_ = s.registry.Remove(requestID)
		return nil, grpcErrorFromTypesError(types.WrapError(types.ErrCodeInternal, "failed to decode llm-gateway response", err))
	}

	response := gatewayResponseToProto(gatewayResp.Response)
	if response == nil {
		s.logger.Error("llm-gateway completion response missing payload", "request_id", requestID)
		_ = s.registry.Remove(requestID)
		return nil, grpcErrorFromTypesError(types.NewError(types.ErrCodeInternal, "llm-gateway returned empty response"))
	}

	s.logger.Info("Completion response relayed from llm-gateway",
		"request_id", requestID,
		"provider", response.Provider,
		"model", response.Model,
		"total_tokens", func() int32 {
			if response.Usage == nil {
				return 0
			}
			return response.Usage.TotalTokens
		}(),
		"elapsed_ms", time.Since(startedAt).Milliseconds())
	s.traceFlow("orchestrator.llm.complete.relay_done", correlationID, llmReq.SessionId, requestID,
		"provider", response.Provider,
		"model", response.Model,
	)

	if response.Usage != nil {
		_ = s.registry.IncrementTokens(requestID, response.Usage.InputTokens, response.Usage.OutputTokens)
	}

	// Update registry
	_ = s.registry.UpdateState(requestID, "completed")
	_ = s.registry.Remove(requestID)

	// Update statistics
	s.mu.Lock()
	s.totalRequests++
	if response.Usage != nil {
		s.totalTokensUsed += int64(response.Usage.TotalTokens)
	}
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
			Id:       "anthropic/claude-sonnet-4-20250514",
			Name:     "Claude Sonnet 4",
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
			Id:       "anthropic/claude-3-5-sonnet-20241022",
			Name:     "Claude 3.5 Sonnet",
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
			Id:       "openai/gpt-4o",
			Name:     "GPT-4o",
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
					Id:       "anthropic/claude-sonnet-4-20250514",
					Name:     "Claude Sonnet 4",
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
					Id:       "openai/gpt-4o",
					Name:     "GPT-4o",
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
		Health:               health,
		Version:              version,
		AvailableProviders:   availableProviders,
		UnavailableProviders: unavailableProviders,
		Timestamp:            timestamppb.Now(),
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
			Available:         true,
			Health:            proto.Health_HEALTH_HEALTHY,
			ActiveRequests:    0,
			TotalRequests:     totalRequests / 2, // Approximate split
			AvgResponseTimeMs: 500.0,
			LastRequestAt:     timestamppb.Now(),
		},
		"openai": {
			Available:         true,
			Health:            proto.Health_HEALTH_HEALTHY,
			ActiveRequests:    0,
			TotalRequests:     totalRequests / 2,
			AvgResponseTimeMs: 400.0,
			LastRequestAt:     timestamppb.Now(),
		},
	}

	return &proto.LLMStatusResponse{
		Status:          proto.Status_STATUS_RUNNING,
		StartedAt:       timestampToProto(&startedTs),
		Uptime:          durationpb.New(uptime),
		ActiveRequests:  0,
		TotalRequests:   totalRequests,
		TotalTokensUsed: totalTokensUsed,
		Providers:       providers,
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

type gatewayCompleteResponse struct {
	Response *gatewayLLMResponse `json:"response"`
}

type gatewayLLMRequest struct {
	RequestID         string                `json:"request_id"`
	SessionID         string                `json:"session_id,omitempty"`
	Model             string                `json:"model"`
	Messages          []gatewayLLMMessage   `json:"messages"`
	Parameters        *gatewayLLMParameters `json:"parameters,omitempty"`
	Tools             []gatewayTool         `json:"tools,omitempty"`
	Provider          string                `json:"provider,omitempty"`
	FallbackProviders []string              `json:"fallback_providers,omitempty"`
	Metadata          *gatewayLLMMetadata   `json:"metadata,omitempty"`
}

type gatewayLLMMessage struct {
	Role    string            `json:"role"`
	Content string            `json:"content"`
	Extra   map[string]string `json:"extra,omitempty"`
}

type gatewayLLMParameters struct {
	MaxTokens     int32    `json:"max_tokens,omitempty"`
	Temperature   float64  `json:"temperature,omitempty"`
	TopP          float64  `json:"top_p,omitempty"`
	TopK          int32    `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
	Stream        bool     `json:"stream,omitempty"`
}

type gatewayTool struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	InputSchema *gatewayToolInputSchema `json:"input_schema,omitempty"`
}

type gatewayToolInputSchema struct {
	Type       string                         `json:"type"`
	Properties map[string]gatewayToolProperty `json:"properties,omitempty"`
	Required   []string                       `json:"required,omitempty"`
}

type gatewayToolProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type gatewayLLMMetadata struct {
	CreatedAt     string            `json:"created_at,omitempty"`
	AgentID       string            `json:"agent_id,omitempty"`
	ContainerID   string            `json:"container_id,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	TimeoutMS     int64             `json:"timeout_ms,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
}

type gatewayLLMResponse struct {
	RequestID    string              `json:"request_id"`
	Content      string              `json:"content"`
	ToolCalls    []gatewayToolCall   `json:"tool_calls"`
	Usage        *gatewayTokenUsage  `json:"usage"`
	FinishReason string              `json:"finish_reason"`
	Provider     string              `json:"provider"`
	Model        string              `json:"model"`
	CompletedAt  string              `json:"completed_at"`
	Metadata     *gatewayLLMMetadata `json:"metadata,omitempty"`
}

type gatewayToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type gatewayTokenUsage struct {
	InputTokens  int32 `json:"input_tokens"`
	OutputTokens int32 `json:"output_tokens"`
	TotalTokens  int32 `json:"total_tokens"`
}

type gatewayStreamEvent struct {
	Type              string              `json:"type"`
	RequestID         string              `json:"request_id"`
	Content           string              `json:"content,omitempty"`
	Index             int32               `json:"index,omitempty"`
	IsLast            bool                `json:"is_last,omitempty"`
	ToolCallID        string              `json:"tool_call_id,omitempty"`
	Name              string              `json:"name,omitempty"`
	ArgumentsDelta    string              `json:"arguments_delta,omitempty"`
	Usage             *gatewayTokenUsage  `json:"usage,omitempty"`
	Code              string              `json:"code,omitempty"`
	Message           string              `json:"message,omitempty"`
	Retryable         bool                `json:"retryable,omitempty"`
	SuggestedProvider string              `json:"suggested_provider,omitempty"`
	Response          *gatewayLLMResponse `json:"response,omitempty"`
}

func (s *LLMService) toGatewayRequest(req *proto.LLMRequest, stream bool) *gatewayLLMRequest {
	if req == nil {
		return nil
	}

	messages := make([]gatewayLLMMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m == nil {
			continue
		}
		messages = append(messages, gatewayLLMMessage{Role: m.Role, Content: m.Content, Extra: m.Extra})
	}

	tools := make([]gatewayTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		if t == nil {
			continue
		}
		tool := gatewayTool{Name: t.Name, Description: t.Description}
		if schema := t.InputSchema; schema != nil {
			properties := make(map[string]gatewayToolProperty, len(schema.Properties))
			for key, value := range schema.Properties {
				properties[key] = gatewayToolProperty{Type: value.Type, Description: value.Description, Enum: value.Enum}
			}
			tool.InputSchema = &gatewayToolInputSchema{Type: schema.Type, Properties: properties, Required: schema.Required}
		}
		tools = append(tools, tool)
	}

	var metadata *gatewayLLMMetadata
	if req.Metadata != nil {
		metadata = &gatewayLLMMetadata{
			AgentID:       req.Metadata.AgentId,
			ContainerID:   req.Metadata.ContainerId,
			Labels:        req.Metadata.Labels,
			TimeoutMS:     req.Metadata.TimeoutMs,
			CorrelationID: req.Metadata.CorrelationId,
		}
		if req.Metadata.CreatedAt != nil {
			metadata.CreatedAt = req.Metadata.CreatedAt.AsTime().Format(time.RFC3339Nano)
		}
	}

	params := &gatewayLLMParameters{}
	if req.Parameters != nil {
		params.MaxTokens = req.Parameters.MaxTokens
		params.Temperature = req.Parameters.Temperature
		params.TopP = req.Parameters.TopP
		params.TopK = req.Parameters.TopK
		params.StopSequences = req.Parameters.StopSequences
	}
	params.Stream = stream

	return &gatewayLLMRequest{
		RequestID:         req.RequestId,
		SessionID:         req.SessionId,
		Model:             req.Model,
		Messages:          messages,
		Parameters:        params,
		Tools:             tools,
		Provider:          req.Provider,
		FallbackProviders: req.FallbackProviders,
		Metadata:          metadata,
	}
}

func gatewayResponseToProto(resp *gatewayLLMResponse) *proto.LLMResponse {
	if resp == nil {
		return nil
	}

	toolCalls := make([]*proto.ToolCall, 0, len(resp.ToolCalls))
	for _, tc := range resp.ToolCalls {
		toolCalls = append(toolCalls, &proto.ToolCall{Id: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}

	var usage *proto.TokenUsage
	if resp.Usage != nil {
		usage = &proto.TokenUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}
	}

	var metadata *proto.LLMMetadata
	if resp.Metadata != nil {
		metadata = &proto.LLMMetadata{
			AgentId:       resp.Metadata.AgentID,
			ContainerId:   resp.Metadata.ContainerID,
			Labels:        resp.Metadata.Labels,
			TimeoutMs:     resp.Metadata.TimeoutMS,
			CorrelationId: resp.Metadata.CorrelationID,
		}
		if resp.Metadata.CreatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, resp.Metadata.CreatedAt); err == nil {
				metadata.CreatedAt = timestamppb.New(parsed)
			}
		}
	}

	result := &proto.LLMResponse{
		RequestId:    resp.RequestID,
		Content:      resp.Content,
		ToolCalls:    toolCalls,
		Usage:        usage,
		FinishReason: finishReasonFromGateway(resp.FinishReason),
		Provider:     resp.Provider,
		Model:        resp.Model,
		Metadata:     metadata,
	}

	if resp.CompletedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, resp.CompletedAt); err == nil {
			result.CompletedAt = timestamppb.New(parsed)
		}
	}

	return result
}

func gatewayEventToProtoResponse(event gatewayStreamEvent) (*proto.StreamLLMResponse, *proto.TokenUsage) {
	switch event.Type {
	case "chunk":
		return &proto.StreamLLMResponse{Payload: &proto.StreamLLMResponse_Chunk{Chunk: &proto.StreamChunk{
			RequestId: event.RequestID,
			Content:   event.Content,
			Index:     event.Index,
			IsLast:    event.IsLast,
		}}}, nil
	case "tool_call":
		return &proto.StreamLLMResponse{Payload: &proto.StreamLLMResponse_ToolCall{ToolCall: &proto.StreamToolCall{
			RequestId:      event.RequestID,
			ToolCallId:     event.ToolCallID,
			Name:           event.Name,
			ArgumentsDelta: event.ArgumentsDelta,
		}}}, nil
	case "usage":
		if event.Usage == nil {
			return nil, nil
		}
		usage := &proto.TokenUsage{InputTokens: event.Usage.InputTokens, OutputTokens: event.Usage.OutputTokens, TotalTokens: event.Usage.TotalTokens}
		return &proto.StreamLLMResponse{Payload: &proto.StreamLLMResponse_Usage{Usage: &proto.StreamUsage{RequestId: event.RequestID, Usage: usage}}}, nil
	case "error":
		return &proto.StreamLLMResponse{Payload: &proto.StreamLLMResponse_Error{Error: &proto.StreamError{
			RequestId:         event.RequestID,
			Code:              event.Code,
			Message:           event.Message,
			Retryable:         event.Retryable,
			SuggestedProvider: event.SuggestedProvider,
		}}}, nil
	case "complete":
		resp := gatewayResponseToProto(event.Response)
		if resp == nil {
			return nil, nil
		}
		return &proto.StreamLLMResponse{Payload: &proto.StreamLLMResponse_Complete{Complete: &proto.StreamComplete{RequestId: event.RequestID, Response: resp}}}, resp.Usage
	default:
		return nil, nil
	}
}

func finishReasonFromGateway(reason string) proto.FinishReason {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "stop":
		return proto.FinishReason_FINISH_REASON_STOP
	case "length":
		return proto.FinishReason_FINISH_REASON_LENGTH
	case "tool_uses", "tool_use", "tool_calls":
		return proto.FinishReason_FINISH_REASON_TOOL_USES
	case "error":
		return proto.FinishReason_FINISH_REASON_ERROR
	case "filter", "content_filter":
		return proto.FinishReason_FINISH_REASON_FILTER
	default:
		return proto.FinishReason_FINISH_REASON_UNSPECIFIED
	}
}

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
