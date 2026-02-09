package provider

import (
	"context"
	"testing"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

func TestProviderIsValid(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		want     bool
	}{
		{
			name:     "anthropic is valid",
			provider: ProviderAnthropic,
			want:     true,
		},
		{
			name:     "openai is valid",
			provider: ProviderOpenAI,
			want:     true,
		},
		{
			name:     "unknown provider is invalid",
			provider: Provider("unknown"),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.IsValid(); got != tt.want {
				t.Errorf("Provider.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviderStatusIsAvailable(t *testing.T) {
	tests := []struct {
		name   string
		status ProviderStatus
		want   bool
	}{
		{
			name:   "available is available",
			status: ProviderStatusAvailable,
			want:   true,
		},
		{
			name:   "degraded is available",
			status: ProviderStatusDegraded,
			want:   true,
		},
		{
			name:   "unavailable is not available",
			status: ProviderStatusUnavailable,
			want:   false,
		},
		{
			name:   "error is not available",
			status: ProviderStatusError,
			want:   false,
		},
		{
			name:   "unknown is not available",
			status: ProviderStatusUnknown,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsAvailable(); got != tt.want {
				t.Errorf("ProviderStatus.IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModelProviderFor(t *testing.T) {
	tests := []struct {
		name     string
		model    Model
		want     Provider
	}{
		{
			name:  "claude-3-5-sonnet is anthropic",
			model: ModelClaude3_5Sonnet,
			want:  ProviderAnthropic,
		},
		{
			name:  "claude-3-opus is anthropic",
			model: ModelClaude3Opus,
			want:  ProviderAnthropic,
		},
		{
			name:  "claude-3-haiku is anthropic",
			model: ModelClaude3Haiku,
			want:  ProviderAnthropic,
		},
		{
			name:  "gpt-4o is openai",
			model: ModelGPT4o,
			want:  ProviderOpenAI,
		},
		{
			name:  "gpt-4o-mini is openai",
			model: ModelGPT4oMini,
			want:  ProviderOpenAI,
		},
		{
			name:  "gpt-4 is openai",
			model: ModelGPT4,
			want:  ProviderOpenAI,
		},
		{
			name:  "unknown model returns empty provider",
			model: Model("unknown-model"),
			want:  Provider(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.model.ProviderFor(); got != tt.want {
				t.Errorf("Model.ProviderFor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenUsageTotal(t *testing.T) {
	tests := []struct {
		name  string
		usage TokenUsage
		want  int
	}{
		{
			name: "basic usage",
			usage: TokenUsage{
				InputTokens:  100,
				OutputTokens: 50,
			},
			want: 150,
		},
		{
			name: "usage with cache tokens",
			usage: TokenUsage{
				InputTokens:     100,
				CacheReadTokens: 20,
				CacheWriteTokens: 10,
				OutputTokens:    50,
			},
			want: 180,
		},
		{
			name:  "zero usage",
			usage: TokenUsage{},
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.usage.Total(); got != tt.want {
				t.Errorf("TokenUsage.Total() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviderStatsSuccessRate(t *testing.T) {
	tests := []struct {
		name  string
		stats ProviderStats
		want  float64
	}{
		{
			name: "100% success rate",
			stats: ProviderStats{
				TotalRequests:   10,
				SuccessRequests: 10,
			},
			want: 100.0,
		},
		{
			name: "50% success rate",
			stats: ProviderStats{
				TotalRequests:   10,
				SuccessRequests: 5,
			},
			want: 50.0,
		},
		{
			name: "0% success rate",
			stats: ProviderStats{
				TotalRequests:   10,
				SuccessRequests: 0,
			},
			want: 0.0,
		},
		{
			name:  "no requests returns 0",
			stats: ProviderStats{},
			want:  0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stats.SuccessRate(); got != tt.want {
				t.Errorf("ProviderStats.SuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviderStatsIsAvailable(t *testing.T) {
	tests := []struct {
		name  string
		stats ProviderStats
		want  bool
	}{
		{
			name: "available status is available",
			stats: ProviderStats{
				Status: ProviderStatusAvailable,
			},
			want: true,
		},
		{
			name: "degraded status is available",
			stats: ProviderStats{
				Status: ProviderStatusDegraded,
			},
			want: true,
		},
		{
			name: "unavailable status is not available",
			stats: ProviderStats{
				Status: ProviderStatusUnavailable,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stats.IsAvailable(); got != tt.want {
				t.Errorf("ProviderStats.IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewProviderError(t *testing.T) {
	err := NewProviderError(ErrCodeProviderNotAvailable, "provider is down")
	if err.Code != ErrCodeProviderNotAvailable {
		t.Errorf("error code = %v, want %v", err.Code, ErrCodeProviderNotAvailable)
	}
	if err.Message != "provider is down" {
		t.Errorf("error message = %v, want 'provider is down'", err.Message)
	}
	if err.Err != nil {
		t.Errorf("error Err should be nil, got %v", err.Err)
	}
}

func TestWrapProviderError(t *testing.T) {
	original := types.NewError(types.ErrCodeInternal, "internal error")
	wrapped := WrapProviderError(ErrCodeUpstreamError, "upstream failed", original)

	if wrapped.Code != ErrCodeUpstreamError {
		t.Errorf("wrapped error code = %v, want %v", wrapped.Code, ErrCodeUpstreamError)
	}
	if wrapped.Message != "upstream failed" {
		t.Errorf("wrapped error message = %v, want 'upstream failed'", wrapped.Message)
	}
	if wrapped.Err != original {
		t.Errorf("wrapped Err = %v, want %v", wrapped.Err, original)
	}
}

func TestCompletionRequest(t *testing.T) {
	req := &CompletionRequest{
		Model:     ModelClaude3_5Sonnet,
		MaxTokens: 1000,
		Messages: []Message{
			{
				Role:    MessageRoleUser,
				Content: "Hello, world!",
			},
		},
		Temperature: float64Ptr(0.7),
		TopP:        float64Ptr(0.9),
	}

	if req.Model != ModelClaude3_5Sonnet {
		t.Errorf("model = %v, want %v", req.Model, ModelClaude3_5Sonnet)
	}
	if req.MaxTokens != 1000 {
		t.Errorf("max_tokens = %v, want 1000", req.MaxTokens)
	}
	if len(req.Messages) != 1 {
		t.Errorf("messages length = %v, want 1", len(req.Messages))
	}
}

func TestCompletionResponse(t *testing.T) {
	resp := &CompletionResponse{
		ID:       "test-id",
		Model:    ModelClaude3_5Sonnet,
		Provider: ProviderAnthropic,
		Content:  "Hello!",
		StopReason: StopReasonEndTurn,
		Usage: TokenUsage{
			InputTokens:  10,
			OutputTokens: 20,
		},
	}

	if resp.ID != "test-id" {
		t.Errorf("id = %v, want 'test-id'", resp.ID)
	}
	if resp.Model != ModelClaude3_5Sonnet {
		t.Errorf("model = %v, want %v", resp.Model, ModelClaude3_5Sonnet)
	}
	if resp.Content != "Hello!" {
		t.Errorf("content = %v, want 'Hello!'", resp.Content)
	}
	if resp.Usage.Total() != 30 {
		t.Errorf("usage total = %v, want 30", resp.Usage.Total())
	}
}

func TestMessageRoleString(t *testing.T) {
	tests := []struct {
		role MessageRole
		want string
	}{
		{MessageRoleSystem, "system"},
		{MessageRoleUser, "user"},
		{MessageRoleAssistant, "assistant"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.role.String(); got != tt.want {
				t.Errorf("MessageRole.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStopReasonString(t *testing.T) {
	tests := []struct {
		reason StopReason
		want   string
	}{
		{StopReasonEndTurn, "end_turn"},
		{StopReasonMaxTokens, "max_tokens"},
		{StopReasonStopSequence, "stop_sequence"},
		{StopReasonToolUse, "tool_use"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.reason.String(); got != tt.want {
				t.Errorf("StopReason.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModelIsEmpty(t *testing.T) {
	if !Model("").IsEmpty() {
		t.Error("empty model should return true for IsEmpty")
	}
	if Model("some-model").IsEmpty() {
		t.Error("non-empty model should return false for IsEmpty")
	}
}

// Helper function
func float64Ptr(f float64) *float64 {
	return &f
}

func TestStreamingEventFilterMatches(t *testing.T) {
	tests := []struct {
		name   string
		filter *StreamingEventFilter
		event  StreamingEvent
		want   bool
	}{
		{
			name:   "no filter matches all",
			filter: &StreamingEventFilter{},
			event: StreamingEvent{
				Type:     StreamingEventStreamChunk,
				StreamID: "stream-123",
			},
			want: true,
		},
		{
			name: "type filter matches",
			filter: &StreamingEventFilter{
				Type: streamingEventTypePtr(StreamingEventStreamChunk),
			},
			event: StreamingEvent{
				Type:     StreamingEventStreamChunk,
				StreamID: "stream-123",
			},
			want: true,
		},
		{
			name: "type filter does not match",
			filter: &StreamingEventFilter{
				Type: streamingEventTypePtr(StreamingEventStreamStart),
			},
			event: StreamingEvent{
				Type:     StreamingEventStreamChunk,
				StreamID: "stream-123",
			},
			want: false,
		},
		{
			name: "stream ID filter matches",
			filter: &StreamingEventFilter{
				StreamID: stringPtr("stream-123"),
			},
			event: StreamingEvent{
				Type:     StreamingEventStreamChunk,
				StreamID: "stream-123",
			},
			want: true,
		},
		{
			name: "stream ID filter does not match",
			filter: &StreamingEventFilter{
				StreamID: stringPtr("stream-456"),
			},
			event: StreamingEvent{
				Type:     StreamingEventStreamChunk,
				StreamID: "stream-123",
			},
			want: false,
		},
		{
			name: "provider filter matches",
			filter: &StreamingEventFilter{
				Provider: providerPtr(ProviderAnthropic),
			},
			event: StreamingEvent{
				Type:     StreamingEventStreamChunk,
				Provider: ProviderAnthropic,
			},
			want: true,
		},
		{
			name: "provider filter does not match",
			filter: &StreamingEventFilter{
				Provider: providerPtr(ProviderOpenAI),
			},
			event: StreamingEvent{
				Type:     StreamingEventStreamChunk,
				Provider: ProviderAnthropic,
			},
			want: false,
		},
		{
			name: "model filter matches",
			filter: &StreamingEventFilter{
				Model: modelPtr(ModelClaude3_5Sonnet),
			},
			event: StreamingEvent{
				Type:  StreamingEventStreamChunk,
				Model: ModelClaude3_5Sonnet,
			},
			want: true,
		},
		{
			name: "model filter does not match",
			filter: &StreamingEventFilter{
				Model: modelPtr(ModelGPT4o),
			},
			event: StreamingEvent{
				Type:  StreamingEventStreamChunk,
				Model: ModelClaude3_5Sonnet,
			},
			want: false,
		},
		{
			name: "labels filter matches",
			filter: &StreamingEventFilter{
				Labels: map[string]string{"key": "value"},
			},
			event: StreamingEvent{
				Type: StreamingEventStreamChunk,
				Metadata: StreamingEventMetadata{
					Labels: map[string]string{"key": "value"},
				},
			},
			want: true,
		},
		{
			name: "labels filter does not match",
			filter: &StreamingEventFilter{
				Labels: map[string]string{"key": "value"},
			},
			event: StreamingEvent{
				Type: StreamingEventStreamChunk,
				Metadata: StreamingEventMetadata{
					Labels: map[string]string{"key": "other"},
				},
			},
			want: false,
		},
		{
			name: "multiple filters match",
			filter: &StreamingEventFilter{
				Type:     streamingEventTypePtr(StreamingEventStreamChunk),
				Provider: providerPtr(ProviderAnthropic),
				Model:    modelPtr(ModelClaude3_5Sonnet),
			},
			event: StreamingEvent{
				Type:     StreamingEventStreamChunk,
				Provider: ProviderAnthropic,
				Model:    ModelClaude3_5Sonnet,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Matches(tt.event); got != tt.want {
				t.Errorf("StreamingEventFilter.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStreamingEventFunc(t *testing.T) {
	called := false
	fn := StreamingEventFunc(func(ctx context.Context, event StreamingEvent) error {
		called = true
		return nil
	})

	if !fn.CanHandle(StreamingEventStreamChunk) {
		t.Error("StreamingEventFunc.CanHandle should always return true")
	}

	ctx := context.Background()
	err := fn.Handle(ctx, StreamingEvent{})
	if err != nil {
		t.Errorf("Handle returned error: %v", err)
	}
	if !called {
		t.Error("Handle was not called")
	}
}

func TestStreamingMiddlewareFunc(t *testing.T) {
	fn := StreamingMiddlewareFunc(func(ctx context.Context, event StreamingEvent) (StreamingEvent, error) {
		event.Data = map[string]interface{}{"processed": true}
		return event, nil
	})

	ctx := context.Background()
	event := StreamingEvent{}
	result, err := fn.Process(ctx, event)
	if err != nil {
		t.Errorf("Process returned error: %v", err)
	}
	if result.Data["processed"] != true {
		t.Error("Process did not modify event")
	}
}

func TestDefaultStreamingOptions(t *testing.T) {
	opts := DefaultStreamingOptions()
	if opts.BufferSize != 10 {
		t.Errorf("BufferSize = %v, want 10", opts.BufferSize)
	}
	if !opts.IncludeUsage {
		t.Error("IncludeUsage should be true by default")
	}
	if opts.IncludeRaw {
		t.Error("IncludeRaw should be false by default")
	}
}

func TestStreamingOptionsMerge(t *testing.T) {
	defaults := DefaultStreamingOptions()

	// Merge with nil
	result := defaults.Merge(nil)
	if result != defaults {
		t.Error("Merge with nil should return original")
	}

	// Merge with custom options
	custom := &StreamingOptions{
		BufferSize: 20,
		IncludeRaw: true,
	}
	result = defaults.Merge(custom)
	if result.BufferSize != 20 {
		t.Errorf("BufferSize = %v, want 20", result.BufferSize)
	}
	if !result.IncludeUsage {
		t.Error("IncludeUsage should remain true")
	}
	if !result.IncludeRaw {
		t.Error("IncludeRaw should be true")
	}
}

func TestStreamHandleClose(t *testing.T) {
	handle := &StreamHandle{
		ID:       "stream-123",
		Provider: ProviderAnthropic,
		Model:    ModelClaude3_5Sonnet,
		Active:   true,
	}

	err := handle.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
	if handle.Active {
		t.Error("Active should be false after Close")
	}
}

func TestStreamingContextWithCancel(t *testing.T) {
	ctx := &StreamingContext{
		RequestID: "req-123",
		Provider:  ProviderAnthropic,
		Model:     ModelClaude3_5Sonnet,
	}

	_, cancel := ctx.WithCancel()
	if cancel == nil {
		t.Error("WithCancel should return a cancel function")
	}
	cancel()
}

// Helper functions for tests
func streamingEventTypePtr(e StreamingEventType) *StreamingEventType {
	return &e
}

func stringPtr(s string) *string {
	return &s
}

func providerPtr(p Provider) *Provider {
	return &p
}

func modelPtr(m Model) *Model {
	return &m
}
