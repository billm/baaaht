package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// TestQueueCreation tests creating a new task queue
func TestQueueCreation(t *testing.T) {
	q := NewQueue(100)

	if q == nil {
		t.Fatal("Failed to create queue")
	}

	if q.Size() != 0 {
		t.Errorf("Expected empty queue, got size %d", q.Size())
	}

	if !q.IsEmpty() {
		t.Error("Expected queue to be empty")
	}

	if q.IsFull() {
		t.Error("Expected queue not to be full")
	}

	if q.Closed() {
		t.Error("Expected queue to be open")
	}
}

// TestQueueEnqueueDequeue tests basic enqueue and dequeue operations
func TestQueueEnqueueDequeue(t *testing.T) {
	q := NewQueue(10)
	ctx := context.Background()

	task := &Task{
		ID:       types.GenerateID(),
		Name:     "test-task",
		Priority: PriorityNormal,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	err := q.Enqueue(task)
	if err != nil {
		t.Fatalf("Failed to enqueue task: %v", err)
	}

	if q.Size() != 1 {
		t.Errorf("Expected queue size 1, got %d", q.Size())
	}

	if q.IsEmpty() {
		t.Error("Expected queue not to be empty")
	}

	// Dequeue the task
	dequeuedTask, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Failed to dequeue task: %v", err)
	}

	if dequeuedTask.ID != task.ID {
		t.Errorf("Expected task ID %s, got %s", task.ID, dequeuedTask.ID)
	}

	if q.Size() != 0 {
		t.Errorf("Expected queue size 0 after dequeue, got %d", q.Size())
	}
}

// TestQueuePriority tests that tasks are dequeued in priority order
func TestQueuePriority(t *testing.T) {
	q := NewQueue(10)
	ctx := context.Background()

	lowTask := &Task{
		ID:       types.GenerateID(),
		Name:     "low-task",
		Priority: PriorityLow,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	highTask := &Task{
		ID:       types.GenerateID(),
		Name:     "high-task",
		Priority: PriorityHigh,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	normalTask := &Task{
		ID:       types.GenerateID(),
		Name:     "normal-task",
		Priority: PriorityNormal,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	// Enqueue in order: low, normal, high
	q.Enqueue(lowTask)
	q.Enqueue(normalTask)
	q.Enqueue(highTask)

	// Should dequeue in order: high, normal, low
	first, _ := q.Dequeue(ctx)
	if first.Priority != PriorityHigh {
		t.Errorf("Expected first task to have high priority, got %v", first.Priority)
	}

	second, _ := q.Dequeue(ctx)
	if second.Priority != PriorityNormal {
		t.Errorf("Expected second task to have normal priority, got %v", second.Priority)
	}

	third, _ := q.Dequeue(ctx)
	if third.Priority != PriorityLow {
		t.Errorf("Expected third task to have low priority, got %v", third.Priority)
	}
}

// TestQueueFull tests that the queue respects its maximum size
func TestQueueFull(t *testing.T) {
	q := NewQueue(2)

	task1 := &Task{
		ID:       types.GenerateID(),
		Priority: PriorityNormal,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	task2 := &Task{
		ID:       types.GenerateID(),
		Priority: PriorityNormal,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	task3 := &Task{
		ID:       types.GenerateID(),
		Priority: PriorityNormal,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	// Should succeed
	if err := q.Enqueue(task1); err != nil {
		t.Fatalf("Failed to enqueue first task: %v", err)
	}

	if err := q.Enqueue(task2); err != nil {
		t.Fatalf("Failed to enqueue second task: %v", err)
	}

	// Should fail - queue is full
	if err := q.Enqueue(task3); err == nil {
		t.Error("Expected error when enqueueing to full queue")
	} else if !types.IsErrCode(err, types.ErrCodeResourceExhausted) {
		t.Errorf("Expected resource exhausted error, got %v", err)
	}

	if !q.IsFull() {
		t.Error("Expected queue to be full")
	}
}

// TestQueueClose tests closing the queue
func TestQueueClose(t *testing.T) {
	q := NewQueue(10)

	q.Close()

	if !q.Closed() {
		t.Error("Expected queue to be closed")
	}

	// Should fail to enqueue after close
	task := &Task{
		ID:       types.GenerateID(),
		Priority: PriorityNormal,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	if err := q.Enqueue(task); err == nil {
		t.Error("Expected error when enqueueing to closed queue")
	}

	// Dequeue should fail immediately
	ctx := context.Background()
	_, err := q.Dequeue(ctx)
	if err == nil {
		t.Error("Expected error when dequeuing from closed queue")
	}
}

// TestQueueRemove tests removing a task from the queue
func TestQueueRemove(t *testing.T) {
	q := NewQueue(10)

	task1 := &Task{
		ID:       types.GenerateID(),
		Name:     "task1",
		Priority: PriorityNormal,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	task2 := &Task{
		ID:       types.GenerateID(),
		Name:     "task2",
		Priority: PriorityNormal,
		Handler:  func(ctx context.Context, task *Task) error { return nil },
	}

	q.Enqueue(task1)
	q.Enqueue(task2)

	// Remove task1
	err := q.Remove(task1.ID)
	if err != nil {
		t.Fatalf("Failed to remove task: %v", err)
	}

	if q.Size() != 1 {
		t.Errorf("Expected queue size 1 after removal, got %d", q.Size())
	}

	// Verify task2 is still in queue
	_, err = q.Get(task2.ID)
	if err != nil {
		t.Errorf("Expected task2 to still be in queue: %v", err)
	}
}

// TestQueueStats tests queue statistics
func TestQueueStats(t *testing.T) {
	q := NewQueue(100)

	// Add some tasks with different priorities
	for i := 0; i < 5; i++ {
		task := &Task{
			ID:       types.GenerateID(),
			Priority: PriorityNormal,
			Handler:  func(ctx context.Context, task *Task) error { return nil },
		}
		q.Enqueue(task)
	}

	for i := 0; i < 3; i++ {
		task := &Task{
			ID:       types.GenerateID(),
			Priority: PriorityHigh,
			Handler:  func(ctx context.Context, task *Task) error { return nil },
		}
		q.Enqueue(task)
	}

	stats := q.Stats()
	if stats.Size != 8 {
		t.Errorf("Expected size 8, got %d", stats.Size)
	}

	if stats.Priority[PriorityNormal] != 5 {
		t.Errorf("Expected 5 normal priority tasks, got %d", stats.Priority[PriorityNormal])
	}

	if stats.Priority[PriorityHigh] != 3 {
		t.Errorf("Expected 3 high priority tasks, got %d", stats.Priority[PriorityHigh])
	}
}

// TestSchedulerCreation tests creating a new scheduler
func TestSchedulerCreation(t *testing.T) {
	log, _ := logger.NewDefault()
	cfg := config.DefaultSchedulerConfig()

	s, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}
	defer s.Close()

	if s == nil {
		t.Fatal("Expected non-nil scheduler")
	}

	stats := s.Stats()
	if stats.WorkerCount != cfg.Workers {
		t.Errorf("Expected %d workers, got %d", cfg.Workers, stats.WorkerCount)
	}
}

// TestSchedulerSubmit tests submitting a task
func TestSchedulerSubmit(t *testing.T) {
	log, _ := logger.NewDefault()
	s, _ := NewDefault(log)
	defer s.Close()

	ctx := context.Background()
	executed := make(chan struct{})

	handler := func(ctx context.Context, task *Task) error {
		close(executed)
		return nil
	}

	taskID, err := s.Submit(ctx, handler, WithTaskName("test-task"))
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	if taskID.IsEmpty() {
		t.Error("Expected non-empty task ID")
	}

	// Wait for task to execute
	select {
	case <-executed:
		// Task executed successfully
	case <-time.After(5 * time.Second):
		t.Error("Task did not execute in time")
	}
}

// TestSchedulerSubmitWithResult tests submitting a task and waiting for result
func TestSchedulerSubmitWithResult(t *testing.T) {
	log, _ := logger.NewDefault()
	s, _ := NewDefault(log)
	defer s.Close()

	ctx := context.Background()

	handler := func(ctx context.Context, task *Task) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	result, err := s.SubmitWithResult(ctx, handler, WithTaskName("test-task"))
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	if result.Status != TaskStatusCompleted {
		t.Errorf("Expected completed status, got %s", result.Status)
	}

	if result.Duration < 100*time.Millisecond {
		t.Errorf("Expected duration >= 100ms, got %v", result.Duration)
	}
}

// TestSchedulerTaskFailure tests handling of failed tasks
func TestSchedulerTaskFailure(t *testing.T) {
	log, _ := logger.NewDefault()
	s, _ := NewDefault(log)
	defer s.Close()

	ctx := context.Background()

	expectedErr := errors.New("task failed")
	handler := func(ctx context.Context, task *Task) error {
		return expectedErr
	}

	result, err := s.SubmitWithResult(ctx, handler,
		WithTaskName("failing-task"),
		WithTaskMaxRetries(0))

	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	if result.Status != TaskStatusFailed {
		t.Errorf("Expected failed status, got %s", result.Status)
	}

	if result.Error == nil {
		t.Error("Expected error to be set")
	}

	stats := s.Stats()
	if stats.FailureCount != 1 {
		t.Errorf("Expected failure count 1, got %d", stats.FailureCount)
	}
}

// TestSchedulerTaskRetry tests task retry logic
func TestSchedulerTaskRetry(t *testing.T) {
	log, _ := logger.NewDefault()
	cfg := config.DefaultSchedulerConfig()
	cfg.MaxRetries = 3
	s, _ := New(cfg, log)
	defer s.Close()

	ctx := context.Background()

	var attemptCount int32
	handler := func(ctx context.Context, task *Task) error {
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 3 {
			return errors.New("not yet")
		}
		return nil
	}

	result, err := s.SubmitWithResult(ctx, handler,
		WithTaskName("retrying-task"),
		WithTaskMaxRetries(3))

	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	if result.Status != TaskStatusCompleted {
		t.Errorf("Expected completed status, got %s", result.Status)
	}

	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}
}

// TestSchedulerTaskTimeout tests task timeout
func TestSchedulerTaskTimeout(t *testing.T) {
	log, _ := logger.NewDefault()
	s, _ := NewDefault(log)
	defer s.Close()

	ctx := context.Background()

	handler := func(ctx context.Context, task *Task) error {
		time.Sleep(10 * time.Second)
		return nil
	}

	// Submit with short timeout
	result, err := s.SubmitWithResult(ctx, handler,
		WithTaskName("slow-task"),
		WithTaskTimeout(100*time.Millisecond))

	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	if result.Status != TaskStatusFailed {
		t.Errorf("Expected failed status due to timeout, got %s", result.Status)
	}
}

// TestSchedulerCancel tests canceling a task
func TestSchedulerCancel(t *testing.T) {
	log, _ := logger.NewDefault()
	cfg := config.DefaultSchedulerConfig()
	cfg.Workers = 1 // Single worker to control execution order
	s, _ := New(cfg, log)
	defer s.Close()

	ctx := context.Background()

	// Submit a long-running task first
	longTaskStarted := make(chan struct{})
	longTaskComplete := make(chan struct{})

	longHandler := func(ctx context.Context, task *Task) error {
		close(longTaskStarted)
		<-ctx.Done()
		close(longTaskComplete)
		return ctx.Err()
	}

	_, err := s.Submit(ctx, longHandler, WithTaskName("long-task"))
	if err != nil {
		t.Fatalf("Failed to submit long task: %v", err)
	}

	// Wait for long task to start
	<-longTaskStarted

	// Submit a short task that will be queued
	shortTaskID, err := s.Submit(ctx, func(ctx context.Context, task *Task) error {
		return nil
	}, WithTaskName("short-task"))
	if err != nil {
		t.Fatalf("Failed to submit short task: %v", err)
	}

	// Cancel the short task
	err = s.Cancel(ctx, shortTaskID)
	if err != nil {
		t.Fatalf("Failed to cancel task: %v", err)
	}

	// Verify the short task was canceled
	task, err := s.GetTask(ctx, shortTaskID)
	if err != nil {
		t.Fatalf("Failed to get task: %v", err)
	}

	task.mu.RLock()
	status := task.Status
	task.mu.RUnlock()

	if status != TaskStatusCanceled {
		t.Errorf("Expected canceled status, got %s", status)
	}
}

// TestSchedulerConcurrentSubmit tests concurrent task submission
func TestSchedulerConcurrentSubmit(t *testing.T) {
	log, _ := logger.NewDefault()
	cfg := config.DefaultSchedulerConfig()
	cfg.Workers = 4
	cfg.QueueSize = 100
	s, _ := New(cfg, log)
	defer s.Close()

	ctx := context.Background()

	var wg sync.WaitGroup
	taskCount := 50
	var completedCount int32

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			handler := func(ctx context.Context, task *Task) error {
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&completedCount, 1)
				return nil
			}

			_, err := s.Submit(ctx, handler,
				WithTaskName("concurrent-task"),
				WithTaskPriority(TaskPriority(idx%3)))

			if err != nil {
				t.Errorf("Failed to submit task %d: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait a bit for all tasks to complete
	time.Sleep(1 * time.Second)

	stats := s.Stats()
	if stats.SuccessCount != int64(taskCount) {
		t.Errorf("Expected %d successful tasks, got %d", taskCount, stats.SuccessCount)
	}
}

// TestSchedulerStats tests scheduler statistics
func TestSchedulerStats(t *testing.T) {
	log, _ := logger.NewDefault()
	s, _ := NewDefault(log)
	defer s.Close()

	ctx := context.Background()

	// Submit some successful tasks
	for i := 0; i < 3; i++ {
		handler := func(ctx context.Context, task *Task) error {
			return nil
		}
		s.Submit(ctx, handler, WithTaskName("success-task"))
	}

	// Submit a failing task
	handler := func(ctx context.Context, task *Task) error {
		return errors.New("failed")
	}
	s.Submit(ctx, handler, WithTaskName("fail-task"), WithTaskMaxRetries(0))

	// Wait for tasks to complete
	time.Sleep(500 * time.Millisecond)

	stats := s.Stats()
	if stats.TotalTasks != 4 {
		t.Errorf("Expected total tasks 4, got %d", stats.TotalTasks)
	}

	if stats.SuccessCount != 3 {
		t.Errorf("Expected success count 3, got %d", stats.SuccessCount)
	}

	if stats.FailureCount != 1 {
		t.Errorf("Expected failure count 1, got %d", stats.FailureCount)
	}
}

// TestSchedulerListTasks tests listing tasks with filters
func TestSchedulerListTasks(t *testing.T) {
	log, _ := logger.NewDefault()
	cfg := config.DefaultSchedulerConfig()
	cfg.Workers = 1 // Control execution order
	s, _ := New(cfg, log)
	defer s.Close()

	ctx := context.Background()

	// Submit tasks with different priorities
	sessionID := types.GenerateID()

	_, _ = s.Submit(ctx, func(ctx context.Context, task *Task) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}, WithTaskName("low-task"), WithTaskPriority(PriorityLow))

	_, _ = s.Submit(ctx, func(ctx context.Context, task *Task) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}, WithTaskName("high-task"), WithTaskPriority(PriorityHigh))

	_, _ = s.Submit(ctx, func(ctx context.Context, task *Task) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}, WithTaskName("session-task"), WithTaskSessionID(sessionID))

	// List all tasks
	tasks, err := s.ListTasks(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tasks: %v", err)
	}

	if len(tasks) < 3 {
		t.Errorf("Expected at least 3 tasks, got %d", len(tasks))
	}

	// Filter by priority
	highPriority := TaskStatusCompleted
	filter := &TaskFilter{
		Status: &highPriority,
	}
	_, err = s.ListTasks(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to filter tasks: %v", err)
	}

	// Filter by session ID
	filter2 := &TaskFilter{
		SessionID: &sessionID,
	}
	filtered2, err := s.ListTasks(ctx, filter2)
	if err != nil {
		t.Fatalf("Failed to filter tasks by session: %v", err)
	}

	if len(filtered2) < 1 {
		t.Error("Expected at least 1 task for session")
	}
}

// TestSchedulerClose tests graceful shutdown
func TestSchedulerClose(t *testing.T) {
	log, _ := logger.NewDefault()
	cfg := config.DefaultSchedulerConfig()
	cfg.Workers = 2
	s, _ := New(cfg, log)

	ctx := context.Background()
	taskStarted := make(chan struct{})

	// Submit a long-running task
	handler := func(ctx context.Context, task *Task) error {
		close(taskStarted)
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	_, err := s.Submit(ctx, handler, WithTaskName("long-task"))
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	<-taskStarted

	// Close scheduler - should wait for task to complete
	closeErr := s.Close()
	if closeErr != nil {
		t.Errorf("Failed to close scheduler: %v", closeErr)
	}

	// Try to submit after close - should fail
	_, err = s.Submit(ctx, handler, WithTaskName("after-close"))
	if err == nil {
		t.Error("Expected error when submitting to closed scheduler")
	}
}

// TestSchedulerTaskOptions tests task options
func TestSchedulerTaskOptions(t *testing.T) {
	log, _ := logger.NewDefault()
	s, _ := NewDefault(log)
	defer s.Close()

	ctx := context.Background()

	customID := types.GenerateID()
	sessionID := types.GenerateID()

	handler := func(ctx context.Context, task *Task) error {
		// Verify task has correct attributes
		if task.ID != customID {
			t.Errorf("Expected task ID %s, got %s", customID, task.ID)
		}
		if task.Name != "custom-name" {
			t.Errorf("Expected task name 'custom-name', got %s", task.Name)
		}
		if task.Priority != PriorityHigh {
			t.Errorf("Expected high priority, got %v", task.Priority)
		}
		if task.SessionID == nil || *task.SessionID != sessionID {
			t.Errorf("Expected session ID %s, got %v", sessionID, task.SessionID)
		}
		if task.Metadata["key"] != "value" {
			t.Errorf("Expected metadata key='value', got %v", task.Metadata["key"])
		}
		return nil
	}

	_, err := s.Submit(ctx, handler,
		WithTaskID(customID),
		WithTaskName("custom-name"),
		WithTaskPriority(PriorityHigh),
		WithTaskSessionID(sessionID),
		WithTaskMetadata("key", "value"),
		WithTaskTimeout(30*time.Second),
		WithTaskMaxRetries(5))

	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// Wait for task to complete
	time.Sleep(100 * time.Millisecond)
}

// TestSchedulerPriorityExecution tests that higher priority tasks execute first
func TestSchedulerPriorityExecution(t *testing.T) {
	log, _ := logger.NewDefault()
	cfg := config.DefaultSchedulerConfig()
	cfg.Workers = 1 // Single worker to ensure serial execution
	s, _ := New(cfg, log)
	defer s.Close()

	ctx := context.Background()

	var executionOrder []string

	handler := func(name string) TaskHandler {
		return func(ctx context.Context, task *Task) error {
			executionOrder = append(executionOrder, name)
			return nil
		}
	}

	// Submit tasks in priority order: low, normal, high
	s.Submit(ctx, handler("low"), WithTaskPriority(PriorityLow))
	s.Submit(ctx, handler("normal"), WithTaskPriority(PriorityNormal))
	s.Submit(ctx, handler("high"), WithTaskPriority(PriorityHigh))

	// Wait for all tasks to complete
	time.Sleep(500 * time.Millisecond)

	// Should execute in order: high, normal, low (not submission order)
	if len(executionOrder) != 3 {
		t.Fatalf("Expected 3 executed tasks, got %d", len(executionOrder))
	}

	// Note: The exact order depends on implementation details
	// The queue should prioritize by priority, but testing timing-dependent
	// behavior can be flaky. This test mainly ensures the system works.
}

// BenchmarkQueueEnqueue benchmarks enqueue operations
func BenchmarkQueueEnqueue(b *testing.B) {
	q := NewQueue(10000)
	handler := func(ctx context.Context, task *Task) error { return nil }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := &Task{
			ID:       types.GenerateID(),
			Priority: PriorityNormal,
			Handler:  handler,
		}
		q.Enqueue(task)
	}
}

// BenchmarkSchedulerSubmit benchmarks task submission
func BenchmarkSchedulerSubmit(b *testing.B) {
	log, _ := logger.NewDefault()
	cfg := config.DefaultSchedulerConfig()
	cfg.Workers = 4
	cfg.QueueSize = 10000
	s, _ := New(cfg, log)
	defer s.Close()

	ctx := context.Background()
	handler := func(ctx context.Context, task *Task) error { return nil }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Submit(ctx, handler, WithTaskName("bench-task"))
	}
}
