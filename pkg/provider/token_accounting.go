package provider

import (
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// UsageRecord represents a single token usage record
type UsageRecord struct {
	// Timestamp when the usage occurred
	Timestamp time.Time `json:"timestamp"`

	// RequestID uniquely identifies the request
	RequestID string `json:"request_id"`

	// Provider that processed the request
	Provider Provider `json:"provider"`

	// Model that was used
	Model Model `json:"model"`

	// Usage is the token usage for this request
	Usage TokenUsage `json:"usage"`

	// Metadata contains additional information about the request
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TokenAccount tracks accumulated token usage over time
type TokenAccount struct {
	mu sync.RWMutex `json:"-"`

	// Provider is the provider this account tracks
	Provider Provider `json:"provider"`

	// Model is the model this account tracks (empty for all models)
	Model Model `json:"model,omitempty"`

	// StartTime is when accounting started
	StartTime time.Time `json:"start_time"`

	// TotalUsage is the accumulated usage
	TotalUsage TokenUsage `json:"total_usage"`

	// RequestCount is the total number of requests
	RequestCount int64 `json:"request_count"`

	// LastRequestTime is the timestamp of the last request
	LastRequestTime time.Time `json:"last_request_time,omitempty"`

	// Records contains historical usage records (up to MaxRecords)
	Records []UsageRecord `json:"records,omitempty"`

	// MaxRecords is the maximum number of records to keep
	MaxRecords int `json:"max_records,omitempty"`
}

// NewTokenAccount creates a new token account for a provider
func NewTokenAccount(provider Provider) *TokenAccount {
	return &TokenAccount{
		Provider:  provider,
		StartTime: time.Now(),
		MaxRecords: 1000, // Default max records
	}
}

// NewTokenAccountForModel creates a new token account for a specific model
func NewTokenAccountForModel(provider Provider, model Model) *TokenAccount {
	return &TokenAccount{
		Provider:  provider,
		Model:     model,
		StartTime: time.Now(),
		MaxRecords: 1000,
	}
}

// Record records a usage event
func (a *TokenAccount) Record(requestID string, model Model, usage TokenUsage) {
	a.RecordWithMetadata(requestID, model, usage, nil)
}

// RecordWithMetadata records a usage event with additional metadata
func (a *TokenAccount) RecordWithMetadata(requestID string, model Model, usage TokenUsage, metadata map[string]interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	totalDelta := usage.Total()

	a.TotalUsage.InputTokens += usage.InputTokens
	a.TotalUsage.CacheReadTokens += usage.CacheReadTokens
	a.TotalUsage.CacheWriteTokens += usage.CacheWriteTokens
	a.TotalUsage.OutputTokens += usage.OutputTokens
	a.TotalUsage.TotalTokens += totalDelta
	a.RequestCount++
	a.LastRequestTime = now

	// Add to records if we have space
	if a.MaxRecords > 0 {
		record := UsageRecord{
			Timestamp: now,
			RequestID: requestID,
			Provider:  a.Provider,
			Model:     model,
			Usage:     usage,
			Metadata:  metadata,
		}
		a.Records = append(a.Records, record)

		// Trim old records if we exceed max
		if len(a.Records) > a.MaxRecords {
			// Keep only the most recent records
			a.Records = a.Records[len(a.Records)-a.MaxRecords:]
		}
	}
}

// AddUsage adds token usage to the account
func (a *TokenAccount) AddUsage(usage TokenUsage) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.TotalUsage.InputTokens += usage.InputTokens
	a.TotalUsage.CacheReadTokens += usage.CacheReadTokens
	a.TotalUsage.CacheWriteTokens += usage.CacheWriteTokens
	a.TotalUsage.OutputTokens += usage.OutputTokens
	a.TotalUsage.TotalTokens += usage.TotalTokens
}

// GetTotalUsage returns the total token usage
func (a *TokenAccount) GetTotalUsage() TokenUsage {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.TotalUsage
}

// GetRequestCount returns the total number of requests
func (a *TokenAccount) GetRequestCount() int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.RequestCount
}

// GetAverageTokensPerRequest returns the average tokens per request
func (a *TokenAccount) GetAverageTokensPerRequest() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.RequestCount == 0 {
		return 0
	}
	return float64(a.TotalUsage.TotalTokens) / float64(a.RequestCount)
}

// GetRecords returns a copy of the usage records
func (a *TokenAccount) GetRecords() []UsageRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	records := make([]UsageRecord, len(a.Records))
	copy(records, a.Records)
	return records
}

// GetRecordsSince returns records since the given time
func (a *TokenAccount) GetRecordsSince(since time.Time) []UsageRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []UsageRecord
	for _, record := range a.Records {
		if record.Timestamp.After(since) {
			result = append(result, record)
		}
	}
	return result
}

// GetRecordsInRange returns records within the given time range
func (a *TokenAccount) GetRecordsInRange(start, end time.Time) []UsageRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []UsageRecord
	for _, record := range a.Records {
		if record.Timestamp.After(start) && record.Timestamp.Before(end) {
			result = append(result, record)
		}
	}
	return result
}

// Reset resets the account
func (a *TokenAccount) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.StartTime = time.Now()
	a.TotalUsage = TokenUsage{}
	a.RequestCount = 0
	a.LastRequestTime = time.Time{}
	a.Records = nil
}

// GetSummary returns a summary of the account
func (a *TokenAccount) GetSummary() *UsageSummary {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return &UsageSummary{
		Provider:               a.Provider,
		Model:                  a.Model,
		StartTime:              a.StartTime,
		LastRequestTime:        a.LastRequestTime,
		TotalUsage:             a.TotalUsage,
		RequestCount:           a.RequestCount,
		AverageTokensPerRequest: a.GetAverageTokensPerRequest(),
	}
}

// TokenBudget represents a token budget with limits
type TokenBudget struct {
	mu sync.RWMutex `json:"-"`

	// Name is the budget name
	Name string `json:"name"`

	// Limit is the maximum number of tokens allowed
	Limit int64 `json:"limit"`

	// Used is the number of tokens used
	Used int64 `json:"used"`

	// Period is the time period for the budget (0 for lifetime)
	Period time.Duration `json:"period,omitempty"`

	// StartTime is when the budget period started
	StartTime time.Time `json:"start_time"`

	// ResetTime is when the budget will reset
	ResetTime time.Time `json:"reset_time,omitempty"`
}

// NewTokenBudget creates a new token budget
func NewTokenBudget(name string, limit int64) *TokenBudget {
	return &TokenBudget{
		Name:      name,
		Limit:     limit,
		Used:      0,
		StartTime: time.Now(),
	}
}

// NewTokenBudgetWithPeriod creates a new token budget with a reset period
func NewTokenBudgetWithPeriod(name string, limit int64, period time.Duration) *TokenBudget {
	now := time.Now()
	return &TokenBudget{
		Name:      name,
		Limit:     limit,
		Used:      0,
		Period:    period,
		StartTime: now,
		ResetTime: now.Add(period),
	}
}

// CanUse checks if the budget allows using the given number of tokens
func (b *TokenBudget) CanUse(tokens int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if budget needs reset
	if b.Period > 0 && time.Now().After(b.ResetTime) {
		b.reset()
	}

	return (b.Used + tokens) <= b.Limit
}

// Use marks tokens as used
func (b *TokenBudget) Use(tokens int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if budget needs reset
	if b.Period > 0 && time.Now().After(b.ResetTime) {
		b.reset()
	}

	if (b.Used + tokens) > b.Limit {
		return types.NewError(types.ErrCodeResourceExhausted, "token budget exceeded")
	}

	b.Used += tokens
	return nil
}

// GetRemaining returns the remaining tokens in the budget
func (b *TokenBudget) GetRemaining() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	remaining := b.Limit - b.Used
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetUsagePercent returns the usage as a percentage (0-100)
func (b *TokenBudget) GetUsagePercent() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.Limit == 0 {
		return 0
	}
	return float64(b.Used) / float64(b.Limit) * 100
}

// Reset resets the budget
func (b *TokenBudget) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.reset()
}

// reset is the internal reset method (must be called with lock held)
func (b *TokenBudget) reset() {
	b.Used = 0
	b.StartTime = time.Now()
	if b.Period > 0 {
		b.ResetTime = b.StartTime.Add(b.Period)
	}
}

// UsageSummary provides a summary of usage statistics
type UsageSummary struct {
	Provider               Provider      `json:"provider"`
	Model                  Model         `json:"model,omitempty"`
	StartTime              time.Time     `json:"start_time"`
	LastRequestTime        time.Time     `json:"last_request_time,omitempty"`
	TotalUsage             TokenUsage    `json:"total_usage"`
	RequestCount           int64         `json:"request_count"`
	AverageTokensPerRequest float64      `json:"average_tokens_per_request"`
}

// GetElapsedTime returns the elapsed time since start
func (s *UsageSummary) GetElapsedTime() time.Duration {
	if s.LastRequestTime.IsZero() {
		return time.Since(s.StartTime)
	}
	return s.LastRequestTime.Sub(s.StartTime)
}

// GetTokensPerSecond returns the average tokens per second
func (s *UsageSummary) GetTokensPerSecond() float64 {
	elapsed := s.GetElapsedTime().Seconds()
	if elapsed < 0.001 { // Less than 1ms, treat as zero
		return 0
	}
	return float64(s.TotalUsage.TotalTokens) / elapsed
}

// TokenCalculator calculates token costs and estimates
type TokenCalculator struct {
	// InputCostPer1K is the cost per 1000 input tokens
	InputCostPer1K float64

	// OutputCostPer1K is the cost per 1000 output tokens
	OutputCostPer1K float64

	// CacheReadCostPer1K is the cost per 1000 cache read tokens
	CacheReadCostPer1K float64

	// CacheWriteCostPer1K is the cost per 1000 cache write tokens
	CacheWriteCostPer1K float64
}

// NewTokenCalculator creates a new token calculator
func NewTokenCalculator(inputCost, outputCost float64) *TokenCalculator {
	return &TokenCalculator{
		InputCostPer1K:  inputCost,
		OutputCostPer1K: outputCost,
	}
}

// CalculateCost calculates the cost for a given token usage
func (c *TokenCalculator) CalculateCost(usage TokenUsage) float64 {
	cost := 0.0

	cost += float64(usage.InputTokens) / 1000.0 * c.InputCostPer1K
	cost += float64(usage.OutputTokens) / 1000.0 * c.OutputCostPer1K

	if c.CacheReadCostPer1K > 0 {
		cost += float64(usage.CacheReadTokens) / 1000.0 * c.CacheReadCostPer1K
	}
	if c.CacheWriteCostPer1K > 0 {
		cost += float64(usage.CacheWriteTokens) / 1000.0 * c.CacheWriteCostPer1K
	}

	return cost
}

// EstimateInputTokens estimates the number of input tokens for a text
// This is a rough estimate (approximately 4 characters per token)
func EstimateInputTokens(text string) int {
	// Rough estimate: ~4 characters per token for English text
	// This is provider and model specific
	return len(text) / 4
}

// EstimateOutputTokens estimates the number of output tokens for a text
func EstimateOutputTokens(text string) int {
	return EstimateInputTokens(text)
}
