package provider

import (
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
