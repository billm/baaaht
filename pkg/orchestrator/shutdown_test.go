package orchestrator

import (
	"context"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// testHelper is an interface that both *testing.T and *testing.B implement
type testHelper interface {
	Helper()
	Fatalf(format string, args ...interface{})
}

// helper function to create a test orchestrator
func createTestOrchestratorForShutdown(t testHelper) *Orchestrator {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	log, err := logger.New(config.DefaultLoggingConfig())
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	orch, err := New(*cfg, log)
	if err != nil {
		t.Fatalf("failed to create test orchestrator: %v", err)
	}

	return orch
}

func TestNewShutdownManager(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	timeout := 10 * time.Second
	sm := NewShutdownManager(orch, timeout, orch.logger)

	if sm == nil {
		t.Fatal("NewShutdownManager returned nil")
	}

	if sm.State() != ShutdownStateRunning {
		t.Errorf("expected state %s, got %s", ShutdownStateRunning, sm.State())
	}

	if sm.IsShuttingDown() {
		t.Error("expected IsShuttingDown to be false initially")
	}

	if sm.IsComplete() {
		t.Error("expected IsComplete to be false initially")
	}
}

func TestShutdownManagerStartStop(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	// Test start
	sm.Start()

	// Multiple starts should be safe
	sm.Start()
	sm.Start()

	// Test stop
	sm.Stop()

	// Multiple stops should be safe
	sm.Stop()
	sm.Stop()
}

func TestShutdownManagerShutdown(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	ctx := context.Background()
	err := sm.Shutdown(ctx, "test shutdown")

	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	if sm.State() != ShutdownStateComplete {
		t.Errorf("expected state %s, got %s", ShutdownStateComplete, sm.State())
	}

	if !sm.IsComplete() {
		t.Error("expected IsComplete to be true after shutdown")
	}

	if sm.ShutdownReason() != "test shutdown" {
		t.Errorf("expected reason 'test shutdown', got '%s'", sm.ShutdownReason())
	}
}

func TestShutdownManagerShutdownTwice(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	ctx := context.Background()

	// First shutdown
	err := sm.Shutdown(ctx, "first shutdown")
	if err != nil {
		t.Errorf("first shutdown failed: %v", err)
	}

	// Second shutdown should fail
	err = sm.Shutdown(ctx, "second shutdown")
	if err == nil {
		t.Error("expected error for second shutdown, got nil")
	}

	if !types.IsErrCode(err, types.ErrCodeFailedPrecondition) {
		t.Errorf("expected ErrCodeFailedPrecondition, got %v", types.GetErrorCode(err))
	}
}

func TestShutdownManagerHooks(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	// Track hook execution
	var mu sync.Mutex
	var executedHooks []string

	hook1 := func(ctx context.Context) error {
		mu.Lock()
		executedHooks = append(executedHooks, "hook1")
		mu.Unlock()
		return nil
	}

	hook2 := func(ctx context.Context) error {
		mu.Lock()
		executedHooks = append(executedHooks, "hook2")
		mu.Unlock()
		return nil
	}

	hook3 := func(ctx context.Context) error {
		mu.Lock()
		executedHooks = append(executedHooks, "hook3")
		mu.Unlock()
		return nil
	}

	sm.AddHook(hook1)
	sm.AddHook(hook2)
	sm.AddHook(hook3)

	ctx := context.Background()
	err := sm.Shutdown(ctx, "test with hooks")
	if err != nil {
		t.Errorf("Shutdown with hooks failed: %v", err)
	}

	// Check all hooks were executed
	mu.Lock()
	defer mu.Unlock()

	if len(executedHooks) != 6 {
		t.Errorf("expected 6 hooks executed (3 hooks x 2 phases), got %d", len(executedHooks))
	}
}

func TestShutdownManagerHookError(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	errorHook := func(ctx context.Context) error {
		return types.NewError(types.ErrCodeInternal, "hook error")
	}

	sm.AddHook(errorHook)

	ctx := context.Background()
	err := sm.Shutdown(ctx, "test with error hook")

	// Shutdown should still succeed even with hook errors
	if err != nil {
		t.Errorf("Shutdown should succeed despite hook errors, got: %v", err)
	}

	// State should still be complete
	if sm.State() != ShutdownStateComplete {
		t.Errorf("expected state %s, got %s", ShutdownStateComplete, sm.State())
	}
}

func TestShutdownManagerHookTimeout(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 200*time.Millisecond, orch.logger)

	slowHook := func(ctx context.Context) error {
		// Sleep longer than the hook timeout
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	sm.AddHook(slowHook)

	ctx := context.Background()
	err := sm.Shutdown(ctx, "test with slow hook")

	// Shutdown should complete despite slow hook (hooks have 200ms timeout)
	if err != nil {
		t.Logf("Shutdown with slow hook result: %v", err)
	}
}

func TestShutdownManagerShutdownAndWait(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	ctx := context.Background()
	err := sm.ShutdownAndWait(ctx, "test wait")

	if err != nil {
		t.Errorf("ShutdownAndWait failed: %v", err)
	}

	if !sm.IsComplete() {
		t.Error("expected shutdown to be complete")
	}
}

func TestShutdownManagerWaitCompletion(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	// Start shutdown in background
	go func() {
		ctx := context.Background()
		_ = sm.Shutdown(ctx, "async shutdown")
	}()

	// Wait for completion
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := sm.WaitCompletion(ctx)
	if err != nil {
		t.Errorf("WaitCompletion failed: %v", err)
	}

	if !sm.IsComplete() {
		t.Error("expected shutdown to be complete")
	}
}

func TestShutdownManagerContextCanceled(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Try to wait for completion with canceled context
	err := sm.WaitCompletion(ctx)
	if err == nil {
		t.Error("expected error for canceled context")
	}

	if !types.IsErrCode(err, types.ErrCodeCanceled) {
		t.Errorf("expected ErrCodeCanceled, got %v", types.GetErrorCode(err))
	}
}

func TestShutdownManagerString(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	sm.AddHook(func(ctx context.Context) error { return nil })
	sm.AddHook(func(ctx context.Context) error { return nil })

	str := sm.String()
	if str == "" {
		t.Error("String() returned empty string")
	}
}

func TestShutdownStateString(t *testing.T) {
	states := []ShutdownState{
		ShutdownStateRunning,
		ShutdownStateInitiated,
		ShutdownStateStopping,
		ShutdownStateComplete,
	}

	for _, state := range states {
		if state.String() == "" {
			t.Errorf("State %s String() returned empty", state)
		}
	}
}

func TestShutdownGracefully(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)

	timeout := 5 * time.Second
	err := ShutdownGracefully(orch, timeout)

	if err != nil {
		t.Errorf("ShutdownGracefully failed: %v", err)
	}

	if !orch.IsClosed() {
		t.Error("expected orchestrator to be closed")
	}
}

func TestShutdownGracefullyNilOrchestrator(t *testing.T) {
	err := ShutdownGracefully(nil, 5*time.Second)

	if err == nil {
		t.Error("expected error for nil orchestrator")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
		t.Errorf("expected ErrCodeInvalidArgument, got %v", types.GetErrorCode(err))
	}
}

func TestShutdownWithSignalHandling(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm, err := ShutdownWithSignalHandling(orch, 5*time.Second, orch.logger)
	if err != nil {
		t.Fatalf("ShutdownWithSignalHandling failed: %v", err)
	}

	if sm == nil {
		t.Fatal("ShutdownWithSignalHandling returned nil manager")
	}

	// Clean up
	sm.Stop()
}

func TestShutdownWithSignalHandlingNilOrchestrator(t *testing.T) {
	sm, err := ShutdownWithSignalHandling(nil, 5*time.Second, nil)

	if err == nil {
		t.Error("expected error for nil orchestrator")
	}

	if sm != nil {
		t.Error("expected nil manager for nil orchestrator")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
		t.Errorf("expected ErrCodeInvalidArgument, got %v", types.GetErrorCode(err))
	}
}

func TestShutdownOnContextCancel(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	sm, err := ShutdownOnContextCancel(orch, ctx, orch.logger)
	if err != nil {
		t.Fatalf("ShutdownOnContextCancel failed: %v", err)
	}

	if sm == nil {
		t.Fatal("ShutdownOnContextCancel returned nil manager")
	}

	// Cancel the context to trigger shutdown
	cancel()

	// Wait for shutdown to complete
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer waitCancel()

	err = sm.WaitCompletion(waitCtx)
	if err != nil {
		t.Errorf("WaitCompletion after context cancel failed: %v", err)
	}

	if !sm.IsComplete() {
		t.Error("expected shutdown to be complete after context cancel")
	}
}

func TestShutdownOnContextCancelNilOrchestrator(t *testing.T) {
	ctx := context.Background()

	sm, err := ShutdownOnContextCancel(nil, ctx, nil)

	if err == nil {
		t.Error("expected error for nil orchestrator")
	}

	if sm != nil {
		t.Error("expected nil manager for nil orchestrator")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
		t.Errorf("expected ErrCodeInvalidArgument, got %v", types.GetErrorCode(err))
	}
}

func TestShutdownManagerSignalHandlingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping signal handling integration test in short mode")
	}

	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)
	sm.Start()

	// Track if shutdown was triggered
	shutdownTriggered := false
	sm.AddHook(func(ctx context.Context) error {
		shutdownTriggered = true
		return nil
	})

	// Send a SIGTERM signal to ourselves
	go func() {
		time.Sleep(100 * time.Millisecond)
		process, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Logf("Failed to find process: %v", err)
			return
		}
		if err := process.Signal(syscall.SIGTERM); err != nil {
			t.Logf("Failed to send signal: %v", err)
		}
	}()

	// Wait for shutdown to complete
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := sm.WaitCompletion(ctx)
	if err != nil {
		t.Errorf("WaitCompletion failed: %v", err)
	}

	if !shutdownTriggered {
		t.Error("expected shutdown hooks to be triggered")
	}

	if !sm.IsComplete() {
		t.Error("expected shutdown to be complete")
	}
}

func TestShutdownManagerConcurrentShutdown(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)
	sm.Start()

	var wg sync.WaitGroup
	results := make(chan error, 3)

	// Try to trigger shutdown from multiple goroutines
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := context.Background()
			err := sm.Shutdown(ctx, "concurrent")
			results <- err // always send result (nil or error)
		}(i)
	}

	wg.Wait()
	close(results)

	// Should have at least one success and some failures
	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		}
	}

	// At least one should succeed
	if successCount == 0 {
		t.Error("expected at least one shutdown to succeed")
	}

	// Shutdown should be complete
	if !sm.IsComplete() {
		t.Error("expected shutdown to be complete")
	}
}

func TestShutdownManagerHookPanic(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	panicHook := func(ctx context.Context) error {
		panic("hook panic")
	}

	sm.AddHook(panicHook)

	// Add a non-panicking hook to verify execution continues
	normalHookCalled := false
	normalHook := func(ctx context.Context) error {
		normalHookCalled = true
		return nil
	}
	sm.AddHook(normalHook)

	ctx := context.Background()

	// Recover from panic in the test
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Recovered from panic: %v", r)
			}
		}()
		_ = sm.Shutdown(ctx, "panic test")
	}()

	// Normal hook should still be called
	if !normalHookCalled {
		t.Error("expected normal hook to be called even after panic")
	}
}

// TestShutdownManagerStartDoesNotTriggerShutdown verifies that calling Start()
// does NOT initiate shutdown — the manager should remain in Running state.
// This catches the bug where waitForShutdown() called ShutdownAndWait() (which
// initiates shutdown) instead of WaitCompletion() (which only waits).
func TestShutdownManagerStartDoesNotTriggerShutdown(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	// Register a hook so we can detect if shutdown runs
	hookCalled := make(chan struct{}, 1)
	sm.AddHook(func(ctx context.Context) error {
		hookCalled <- struct{}{}
		return nil
	})

	// Start signal handling — this should NOT trigger shutdown
	sm.Start()
	defer sm.Stop()

	// Give it some time — if Start() triggers shutdown, it would happen quickly
	time.Sleep(200 * time.Millisecond)

	// Verify state is still Running
	if sm.State() != ShutdownStateRunning {
		t.Errorf("expected state %s after Start(), got %s", ShutdownStateRunning, sm.State())
	}

	if sm.IsShuttingDown() {
		t.Error("shutdown manager should not be shutting down after Start()")
	}

	if sm.IsComplete() {
		t.Error("shutdown manager should not be complete after Start()")
	}

	// Verify hooks were NOT called
	select {
	case <-hookCalled:
		t.Error("shutdown hook should not have been called after Start()")
	default:
		// Good — no hook was called
	}
}

// TestShutdownManagerWaitCompletionBlocksUntilSignal verifies that WaitCompletion
// blocks after Start() and only returns when shutdown is actually triggered.
// This mirrors the expected main.go lifecycle pattern.
func TestShutdownManagerWaitCompletionBlocksUntilSignal(t *testing.T) {
	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)
	sm.Start()

	waitReturned := make(chan error, 1)

	// WaitCompletion should block until shutdown is triggered
	go func() {
		waitReturned <- sm.WaitCompletion(context.Background())
	}()

	// Verify WaitCompletion is still blocking after 200ms
	select {
	case <-waitReturned:
		t.Fatal("WaitCompletion returned immediately — should block until shutdown is triggered")
	case <-time.After(200 * time.Millisecond):
		// Good — WaitCompletion is blocking as expected
	}

	// Now trigger shutdown explicitly
	go func() {
		_ = sm.Shutdown(context.Background(), "test trigger")
	}()

	// WaitCompletion should now return
	select {
	case err := <-waitReturned:
		if err != nil {
			t.Errorf("WaitCompletion returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitCompletion did not return after shutdown was triggered")
	}

	if !sm.IsComplete() {
		t.Error("expected shutdown to be complete")
	}
}

// TestShutdownManagerMainLifecyclePattern tests the exact pattern used in main.go:
// Start() -> WaitCompletion() -> Stop(), verifying it only completes on signal.
func TestShutdownManagerMainLifecyclePattern(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping lifecycle pattern test in short mode")
	}

	orch := createTestOrchestratorForShutdown(t)
	defer orch.Close()

	sm := NewShutdownManager(orch, 5*time.Second, orch.logger)

	// Add hooks before Start (mirrors the fixed main.go order)
	hookExecuted := false
	sm.AddHook(func(ctx context.Context) error {
		hookExecuted = true
		return nil
	})

	sm.Start()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// This mirrors waitForShutdown() in main.go
		_ = sm.WaitCompletion(context.Background())
		sm.Stop()
	}()

	// Confirm it's still blocking
	select {
	case <-done:
		t.Fatal("lifecycle completed without a signal — this is the original bug")
	case <-time.After(200 * time.Millisecond):
		// Expected: still running
	}

	// Simulate SIGTERM
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to find process: %v", err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}

	// Should complete now
	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("lifecycle did not complete after SIGTERM")
	}

	if !sm.IsComplete() {
		t.Error("expected shutdown to be complete")
	}

	if !hookExecuted {
		t.Error("expected shutdown hook to be executed")
	}
}

// Benchmark tests

func BenchmarkShutdownManagerNew(b *testing.B) {
	orch := createTestOrchestratorForShutdown(b)
	defer orch.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewShutdownManager(orch, 5*time.Second, orch.logger)
	}
}

func BenchmarkShutdownManagerShutdown(b *testing.B) {
	orch := createTestOrchestratorForShutdown(b)
	defer orch.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm := NewShutdownManager(orch, 5*time.Second, orch.logger)
		ctx := context.Background()
		_ = sm.Shutdown(ctx, "benchmark")
	}
}

func BenchmarkShutdownManagerWithHooks(b *testing.B) {
	orch := createTestOrchestratorForShutdown(b)
	defer orch.Close()

	hook := func(ctx context.Context) error {
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm := NewShutdownManager(orch, 5*time.Second, orch.logger)
		sm.AddHook(hook)
		sm.AddHook(hook)
		sm.AddHook(hook)
		ctx := context.Background()
		_ = sm.Shutdown(ctx, "benchmark")
	}
}

func BenchmarkShutdownGracefully(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		orch := createTestOrchestratorForShutdown(b)
		_ = ShutdownGracefully(orch, 5*time.Second)
	}
}
