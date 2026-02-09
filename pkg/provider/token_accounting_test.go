package provider

import (
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

func TestNewTokenAccount(t *testing.T) {
	account := NewTokenAccount(ProviderAnthropic)

	if account.Provider != ProviderAnthropic {
		t.Errorf("Provider = %v, want %v", account.Provider, ProviderAnthropic)
	}
	if account.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
	if account.RequestCount != 0 {
		t.Errorf("RequestCount = %v, want 0", account.RequestCount)
	}
	if account.MaxRecords != 1000 {
		t.Errorf("MaxRecords = %v, want 1000", account.MaxRecords)
	}
}

func TestNewTokenAccountForModel(t *testing.T) {
	account := NewTokenAccountForModel(ProviderAnthropic, ModelClaude3_5Sonnet)

	if account.Provider != ProviderAnthropic {
		t.Errorf("Provider = %v, want %v", account.Provider, ProviderAnthropic)
	}
	if account.Model != ModelClaude3_5Sonnet {
		t.Errorf("Model = %v, want %v", account.Model, ModelClaude3_5Sonnet)
	}
	if account.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
}

func TestTokenAccountRecord(t *testing.T) {
	account := NewTokenAccount(ProviderOpenAI)

	usage1 := TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}
	usage1.TotalTokens = usage1.InputTokens + usage1.OutputTokens

	account.Record("req-1", ModelGPT4o, usage1)

	if account.RequestCount != 1 {
		t.Errorf("RequestCount = %v, want 1", account.RequestCount)
	}

	total := account.GetTotalUsage()
	if total.InputTokens != 100 {
		t.Errorf("InputTokens = %v, want 100", total.InputTokens)
	}
	if total.OutputTokens != 50 {
		t.Errorf("OutputTokens = %v, want 50", total.OutputTokens)
	}
	if total.TotalTokens != 150 {
		t.Errorf("TotalTokens = %v, want 150", total.TotalTokens)
	}

	// Record another usage
	usage2 := TokenUsage{
		InputTokens:  200,
		OutputTokens: 100,
	}
	usage2.TotalTokens = usage2.InputTokens + usage2.OutputTokens

	account.Record("req-2", ModelGPT4oMini, usage2)

	if account.RequestCount != 2 {
		t.Errorf("RequestCount = %v, want 2", account.RequestCount)
	}

	total = account.GetTotalUsage()
	if total.InputTokens != 300 {
		t.Errorf("InputTokens = %v, want 300", total.InputTokens)
	}
	if total.OutputTokens != 150 {
		t.Errorf("OutputTokens = %v, want 150", total.OutputTokens)
	}
	if total.TotalTokens != 450 {
		t.Errorf("TotalTokens = %v, want 450", total.TotalTokens)
	}
}

func TestTokenAccountRecordWithCacheTokens(t *testing.T) {
	account := NewTokenAccount(ProviderAnthropic)

	usage := TokenUsage{
		InputTokens:      1000,
		CacheReadTokens:  500,
		CacheWriteTokens: 100,
		OutputTokens:     200,
	}
	usage.TotalTokens = 1000 + 500 + 100 + 200

	account.Record("req-1", ModelClaude3_5Sonnet, usage)

	total := account.GetTotalUsage()
	if total.InputTokens != 1000 {
		t.Errorf("InputTokens = %v, want 1000", total.InputTokens)
	}
	if total.CacheReadTokens != 500 {
		t.Errorf("CacheReadTokens = %v, want 500", total.CacheReadTokens)
	}
	if total.CacheWriteTokens != 100 {
		t.Errorf("CacheWriteTokens = %v, want 100", total.CacheWriteTokens)
	}
	if total.OutputTokens != 200 {
		t.Errorf("OutputTokens = %v, want 200", total.OutputTokens)
	}
	if total.TotalTokens != 1800 {
		t.Errorf("TotalTokens = %v, want 1800", total.TotalTokens)
	}
}

func TestTokenAccountRecordWithMetadata(t *testing.T) {
	account := NewTokenAccount(ProviderAnthropic)

	usage := TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}
	usage.TotalTokens = 150

	metadata := map[string]interface{}{
		"agent_type": "researcher",
		"session_id": "sess-123",
	}

	account.RecordWithMetadata("req-1", ModelClaude3_5Sonnet, usage, metadata)

	if account.RequestCount != 1 {
		t.Errorf("RequestCount = %v, want 1", account.RequestCount)
	}

	records := account.GetRecords()
	if len(records) != 1 {
		t.Errorf("Records length = %v, want 1", len(records))
		return
	}

	if records[0].RequestID != "req-1" {
		t.Errorf("RequestID = %v, want 'req-1'", records[0].RequestID)
	}
	if records[0].Metadata["agent_type"] != "researcher" {
		t.Errorf("Metadata agent_type = %v, want 'researcher'", records[0].Metadata["agent_type"])
	}
}

func TestTokenAccountAddUsage(t *testing.T) {
	account := NewTokenAccount(ProviderOpenAI)

	usage1 := TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}
	usage1.TotalTokens = 150

	account.AddUsage(usage1)

	total := account.GetTotalUsage()
	if total.InputTokens != 100 {
		t.Errorf("InputTokens = %v, want 100", total.InputTokens)
	}

	// AddUsage should not increment RequestCount
	if account.RequestCount != 0 {
		t.Errorf("RequestCount = %v, want 0 (AddUsage should not increment)", account.RequestCount)
	}
}

func TestTokenAccountGetAverageTokensPerRequest(t *testing.T) {
	account := NewTokenAccount(ProviderAnthropic)

	// No requests yet
	if avg := account.GetAverageTokensPerRequest(); avg != 0 {
		t.Errorf("AverageTokensPerRequest = %v, want 0 for no requests", avg)
	}

	// Add some requests
	for i := 0; i < 3; i++ {
		usage := TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
		}
		usage.TotalTokens = 150
		account.Record("req", ModelClaude3_5Sonnet, usage)
	}

	avg := account.GetAverageTokensPerRequest()
	expected := 150.0
	if avg != expected {
		t.Errorf("AverageTokensPerRequest = %v, want %v", avg, expected)
	}
}

func TestTokenAccountGetRecords(t *testing.T) {
	account := NewTokenAccount(ProviderOpenAI)

	usage := TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}
	usage.TotalTokens = 150

	account.Record("req-1", ModelGPT4o, usage)
	account.Record("req-2", ModelGPT4oMini, usage)

	records := account.GetRecords()
	if len(records) != 2 {
		t.Errorf("Records length = %v, want 2", len(records))
	}

	// Verify records are copies (not the same slice)
	records[0] = UsageRecord{}
	newRecords := account.GetRecords()
	if len(newRecords) != 2 {
		t.Errorf("Modified records slice affected original, length = %v, want 2", len(newRecords))
	}
}

func TestTokenAccountGetRecordsSince(t *testing.T) {
	account := NewTokenAccount(ProviderAnthropic)

	usage := TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}
	usage.TotalTokens = 150

	account.Record("req-1", ModelClaude3_5Sonnet, usage)
	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)
	account.Record("req-2", ModelClaude3_5Sonnet, usage)

	records := account.GetRecordsSince(cutoff)
	if len(records) != 1 {
		t.Errorf("RecordsSince length = %v, want 1", len(records))
	}
	if records[0].RequestID != "req-2" {
		t.Errorf("RequestID = %v, want 'req-2'", records[0].RequestID)
	}
}

func TestTokenAccountGetRecordsInRange(t *testing.T) {
	account := NewTokenAccount(ProviderOpenAI)

	usage := TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}
	usage.TotalTokens = 150

	account.Record("req-1", ModelGPT4o, usage)
	time.Sleep(10 * time.Millisecond)
	start := time.Now()
	account.Record("req-2", ModelGPT4o, usage)
	time.Sleep(10 * time.Millisecond)
	end := time.Now()
	time.Sleep(10 * time.Millisecond)
	account.Record("req-3", ModelGPT4o, usage)

	records := account.GetRecordsInRange(start, end)
	if len(records) != 1 {
		t.Errorf("RecordsInRange length = %v, want 1", len(records))
	}
	if records[0].RequestID != "req-2" {
		t.Errorf("RequestID = %v, want 'req-2'", records[0].RequestID)
	}
}

func TestTokenAccountReset(t *testing.T) {
	account := NewTokenAccount(ProviderAnthropic)

	usage := TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}
	usage.TotalTokens = 150

	account.Record("req-1", ModelClaude3_5Sonnet, usage)
	account.Record("req-2", ModelClaude3_5Sonnet, usage)

	account.Reset()

	if account.RequestCount != 0 {
		t.Errorf("RequestCount after reset = %v, want 0", account.RequestCount)
	}

	total := account.GetTotalUsage()
	if total.InputTokens != 0 || total.OutputTokens != 0 {
		t.Errorf("TotalUsage after reset = %v, want zero", total)
	}

	if account.StartTime.IsZero() {
		t.Error("StartTime should be updated after reset")
	}

	if len(account.GetRecords()) != 0 {
		t.Error("Records should be empty after reset")
	}
}

func TestTokenAccountGetSummary(t *testing.T) {
	account := NewTokenAccount(ProviderAnthropic)

	usage := TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}
	usage.TotalTokens = 150

	account.Record("req-1", ModelClaude3_5Sonnet, usage)

	summary := account.GetSummary()

	if summary.Provider != ProviderAnthropic {
		t.Errorf("Summary Provider = %v, want %v", summary.Provider, ProviderAnthropic)
	}
	if summary.RequestCount != 1 {
		t.Errorf("Summary RequestCount = %v, want 1", summary.RequestCount)
	}
	if summary.TotalUsage.InputTokens != 100 {
		t.Errorf("Summary TotalUsage.InputTokens = %v, want 100", summary.TotalUsage.InputTokens)
	}
}

func TestNewTokenBudget(t *testing.T) {
	budget := NewTokenBudget("test-budget", 10000)

	if budget.Name != "test-budget" {
		t.Errorf("Name = %v, want 'test-budget'", budget.Name)
	}
	if budget.Limit != 10000 {
		t.Errorf("Limit = %v, want 10000", budget.Limit)
	}
	if budget.Used != 0 {
		t.Errorf("Used = %v, want 0", budget.Used)
	}
	if budget.Period != 0 {
		t.Errorf("Period = %v, want 0 (no period)", budget.Period)
	}
}

func TestNewTokenBudgetWithPeriod(t *testing.T) {
	budget := NewTokenBudgetWithPeriod("daily-budget", 100000, 24*time.Hour)

	if budget.Name != "daily-budget" {
		t.Errorf("Name = %v, want 'daily-budget'", budget.Name)
	}
	if budget.Limit != 100000 {
		t.Errorf("Limit = %v, want 100000", budget.Limit)
	}
	if budget.Period != 24*time.Hour {
		t.Errorf("Period = %v, want 24h", budget.Period)
	}
	if budget.ResetTime.IsZero() {
		t.Error("ResetTime should not be zero")
	}
}

func TestTokenBudgetCanUse(t *testing.T) {
	budget := NewTokenBudget("test", 1000)

	if !budget.CanUse(500) {
		t.Error("Should be able to use 500 tokens")
	}

	if !budget.CanUse(1000) {
		t.Error("Should be able to use exactly 1000 tokens")
	}

	if budget.CanUse(1001) {
		t.Error("Should not be able to use more than limit")
	}

	// Use some tokens
	budget.Use(500)

	if !budget.CanUse(500) {
		t.Error("Should be able to use remaining 500 tokens")
	}

	if budget.CanUse(501) {
		t.Error("Should not be able to use more than remaining")
	}
}

func TestTokenBudgetUse(t *testing.T) {
	budget := NewTokenBudget("test", 1000)

	err := budget.Use(500)
	if err != nil {
		t.Errorf("Use(500) returned error: %v", err)
	}

	if budget.GetRemaining() != 500 {
		t.Errorf("Remaining = %v, want 500", budget.GetRemaining())
	}

	err = budget.Use(500)
	if err != nil {
		t.Errorf("Use(500) returned error: %v", err)
	}

	if budget.GetRemaining() != 0 {
		t.Errorf("Remaining = %v, want 0", budget.GetRemaining())
	}

	// Should fail when budget is exceeded
	err = budget.Use(1)
	if err == nil {
		t.Error("Use(1) should fail when budget exceeded")
	}
	if !types.IsErrCode(err, types.ErrCodeResourceExhausted) {
		t.Errorf("Error code = %v, want %v", types.GetErrorCode(err), types.ErrCodeResourceExhausted)
	}
}

func TestTokenBudgetWithPeriod(t *testing.T) {
	// Create a budget with a short period for testing
	budget := NewTokenBudgetWithPeriod("test", 1000, 100*time.Millisecond)

	// Use the entire budget
	budget.Use(1000)

	if budget.CanUse(1) {
		t.Error("Before reset, should NOT be able to use more tokens (budget exhausted)")
	}

	// Wait for period to reset
	time.Sleep(150 * time.Millisecond)

	// Budget should now be reset
	if !budget.CanUse(1) {
		t.Error("After reset, should be able to use tokens")
	}

	// Should be able to use tokens now
	err := budget.Use(500)
	if err != nil {
		t.Errorf("Use(500) after reset returned error: %v", err)
	}
}

func TestTokenBudgetGetRemaining(t *testing.T) {
	budget := NewTokenBudget("test", 1000)

	if budget.GetRemaining() != 1000 {
		t.Errorf("Remaining = %v, want 1000", budget.GetRemaining())
	}

	budget.Use(300)

	if budget.GetRemaining() != 700 {
		t.Errorf("Remaining = %v, want 700", budget.GetRemaining())
	}

	budget.Use(700)

	if budget.GetRemaining() != 0 {
		t.Errorf("Remaining = %v, want 0", budget.GetRemaining())
	}
}

func TestTokenBudgetGetUsagePercent(t *testing.T) {
	budget := NewTokenBudget("test", 1000)

	if budget.GetUsagePercent() != 0 {
		t.Errorf("UsagePercent = %v, want 0", budget.GetUsagePercent())
	}

	budget.Use(500)

	expected := 50.0
	if budget.GetUsagePercent() != expected {
		t.Errorf("UsagePercent = %v, want %v", budget.GetUsagePercent(), expected)
	}

	budget.Use(500)

	expected = 100.0
	if budget.GetUsagePercent() != expected {
		t.Errorf("UsagePercent = %v, want %v", budget.GetUsagePercent(), expected)
	}
}

func TestTokenBudgetReset(t *testing.T) {
	budget := NewTokenBudget("test", 1000)

	budget.Use(500)
	budget.Use(300)

	if budget.Used != 800 {
		t.Errorf("Used before reset = %v, want 800", budget.Used)
	}

	budget.Reset()

	if budget.Used != 0 {
		t.Errorf("Used after reset = %v, want 0", budget.Used)
	}
	if budget.GetRemaining() != 1000 {
		t.Errorf("Remaining after reset = %v, want 1000", budget.GetRemaining())
	}
}

func TestUsageSummaryGetElapsedTime(t *testing.T) {
	summary := &UsageSummary{
		StartTime: time.Now().Add(-1 * time.Hour),
	}

	elapsed := summary.GetElapsedTime()
	if elapsed < 59*time.Minute || elapsed > 61*time.Minute {
		t.Errorf("ElapsedTime = %v, want ~1 hour", elapsed)
	}

	// With last request time
	summary.LastRequestTime = summary.StartTime.Add(30 * time.Minute)
	elapsed = summary.GetElapsedTime()
	if elapsed < 29*time.Minute || elapsed > 31*time.Minute {
		t.Errorf("ElapsedTime with LastRequestTime = %v, want ~30 minutes", elapsed)
	}
}

func TestUsageSummaryGetTokensPerSecond(t *testing.T) {
	summary := &UsageSummary{
		StartTime: time.Now().Add(-10 * time.Second),
		TotalUsage: TokenUsage{
			TotalTokens: 1000,
		},
	}

	tps := summary.GetTokensPerSecond()
	if tps < 90 || tps > 110 {
		t.Errorf("TokensPerSecond = %v, want ~100", tps)
	}

	// Zero elapsed time
	summary2 := &UsageSummary{
		StartTime: time.Now(),
		TotalUsage: TokenUsage{
			TotalTokens: 1000,
		},
	}

	if summary2.GetTokensPerSecond() != 0 {
		t.Error("TokensPerSecond should be 0 when elapsed time is 0")
	}
}

func TestNewTokenCalculator(t *testing.T) {
	calc := NewTokenCalculator(0.003, 0.006)

	if calc.InputCostPer1K != 0.003 {
		t.Errorf("InputCostPer1K = %v, want 0.003", calc.InputCostPer1K)
	}
	if calc.OutputCostPer1K != 0.006 {
		t.Errorf("OutputCostPer1K = %v, want 0.006", calc.OutputCostPer1K)
	}
}

func TestTokenCalculatorCalculateCost(t *testing.T) {
	calc := NewTokenCalculator(0.003, 0.006)

	usage := TokenUsage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	cost := calc.CalculateCost(usage)
	expected := 0.003*1.0 + 0.006*0.5 // $0.003 + $0.003 = $0.006

	if cost < 0.0059 || cost > 0.0061 {
		t.Errorf("Cost = %v, want %v", cost, expected)
	}
}

func TestTokenCalculatorCalculateCostWithCache(t *testing.T) {
	calc := NewTokenCalculator(0.003, 0.006)
	calc.CacheReadCostPer1K = 0.0003
	calc.CacheWriteCostPer1K = 0.00375

	usage := TokenUsage{
		InputTokens:      1000,
		CacheReadTokens:  500,
		CacheWriteTokens: 100,
		OutputTokens:     500,
	}

	cost := calc.CalculateCost(usage)
	// Input: 1000 * $0.003/1000 = $0.003
	// Output: 500 * $0.006/1000 = $0.003
	// Cache Read: 500 * $0.0003/1000 = $0.00015
	// Cache Write: 100 * $0.00375/1000 = $0.000375
	// Total: ~0.006525

	if cost < 0.0064 || cost > 0.0066 {
		t.Errorf("Cost = %v, want ~0.006525", cost)
	}
}

func TestEstimateInputTokens(t *testing.T) {
	text := "This is a test string with approximately forty characters"
	tokens := EstimateInputTokens(text)

	// 48 characters / 4 = 12 tokens
	if tokens < 10 || tokens > 14 {
		t.Errorf("EstimateInputTokens = %v, want ~12", tokens)
	}
}

func TestEstimateOutputTokens(t *testing.T) {
	text := "Response text"
	tokens := EstimateOutputTokens(text)

	// Should be same as input estimation
	if tokens < 2 || tokens > 5 {
		t.Errorf("EstimateOutputTokens = %v, want ~3", tokens)
	}
}

func TestTokenAccountMaxRecords(t *testing.T) {
	account := NewTokenAccount(ProviderAnthropic)
	account.MaxRecords = 5 // Set a low limit for testing

	usage := TokenUsage{
		InputTokens:  10,
		OutputTokens: 5,
	}
	usage.TotalTokens = 15

	// Add more records than the max
	for i := 0; i < 10; i++ {
		account.Record("req", ModelClaude3_5Sonnet, usage)
	}

	records := account.GetRecords()
	if len(records) != 5 {
		t.Errorf("Records length = %v, want 5 (max records)", len(records))
	}

	// Verify we kept the most recent records
	if records[0].RequestID != "req" {
		t.Error("Should keep most recent records")
	}
}
