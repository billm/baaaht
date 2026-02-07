package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/baaaht/orchestrator/internal/config"
	"github.com/baaaht/orchestrator/internal/logger"
	"github.com/baaaht/orchestrator/pkg/types"
)

// Scheduler manages task execution with a worker pool
type Scheduler struct {
	mu           sync.RWMutex
	queue        *Queue
	workers      []*worker
	cfg          config.SchedulerConfig
	logger       *logger.Logger
	closed       bool
	wg           sync.WaitGroup
	cancelCtx    context.CancelFunc
	taskCount    int64
	successCount int64
	failureCount int64
}

// worker represents a single worker in the worker pool
type worker struct {
	id         int
	scheduler  *Scheduler
	taskChan   chan *Task
	quitChan   chan struct{}
	working    atomic.Bool
	currentMu  sync.RWMutex
	currentTask *Task
}

// New creates a new task scheduler with the specified configuration
func New(cfg config.SchedulerConfig, log *logger.Logger) (*Scheduler, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Validate configuration
	if cfg.QueueSize <= 0 {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "queue size must be positive")
	}
	if cfg.Workers <= 0 {
		return nil, types.NewError(types.ErrCodeInvalidArgument, "worker count must be positive")
	}

	s := &Scheduler{
		queue:     NewQueue(cfg.QueueSize),
		workers:   make([]*worker, cfg.Workers),
		cfg:       cfg,
		logger:    log.With("component", "scheduler"),
		closed:    false,
		taskCount: 0,
	}

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelCtx = cancel

	// Initialize workers
	for i := 0; i < cfg.Workers; i++ {
		s.workers[i] = &worker{
			id:        i + 1,
			scheduler: s,
			taskChan:  make(chan *Task, 1),
			quitChan:  make(chan struct{}),
		}
	}

	// Start workers
	for _, w := range s.workers {
		s.wg.Add(1)
		go w.run(ctx)
	}

	s.logger.Info("Scheduler initialized",
		"workers", cfg.Workers,
		"queue_size", cfg.QueueSize,
		"max_retries", cfg.MaxRetries,
		"task_timeout", cfg.TaskTimeout)

	return s, nil
}

// NewDefault creates a new scheduler with default configuration
func NewDefault(log *logger.Logger) (*Scheduler, error) {
	cfg := config.DefaultSchedulerConfig()
	return New(cfg, log)
}

// Submit submits a task to be executed. Returns the task ID.
func (s *Scheduler) Submit(ctx context.Context, handler TaskHandler, opts ...TaskOption) (types.ID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return "", types.NewError(types.ErrCodeUnavailable, "scheduler is closed")
	}

	// Apply options to create task
	task := &Task{
		Handler:    handler,
		Status:     TaskStatusPending,
		CreatedAt:  types.NewTimestamp(),
		Retries:    0,
		MaxRetries: s.cfg.MaxRetries,
		Timeout:    s.cfg.TaskTimeout,
	}

	for _, opt := range opts {
		opt(task)
	}

	// Ensure task has ID
	if task.ID.IsEmpty() {
		task.ID = types.GenerateID()
	}

	// Enqueue task
	if err := s.queue.Enqueue(task); err != nil {
		return "", types.WrapError(types.ErrCodeInternal, "failed to enqueue task", err)
	}

	atomic.AddInt64(&s.taskCount, 1)

	s.logger.Debug("Task submitted",
		"task_id", task.ID,
		"name", task.Name,
		"priority", task.Priority)

	return task.ID, nil
}

// SubmitWithResult submits a task and waits for its result
func (s *Scheduler) SubmitWithResult(ctx context.Context, handler TaskHandler, opts ...TaskOption) (*TaskResult, error) {
	taskID, err := s.Submit(ctx, handler, opts...)
	if err != nil {
		return nil, err
	}

	return s.WaitForResult(ctx, taskID)
}

// WaitForResult waits for a task to complete and returns its result
func (s *Scheduler) WaitForResult(ctx context.Context, taskID types.ID) (*TaskResult, error) {
	resultChan := make(chan *TaskResult, 1)
	errChan := make(chan error, 1)

	// Start goroutine to poll for task completion
	go func() {
		for {
			task, err := s.GetTask(ctx, taskID)
			if err != nil {
				errChan <- err
				return
			}

			task.mu.RLock()
			status := task.Status
			var taskErr error
			if task.Error != "" {
				taskErr = fmt.Errorf(task.Error)
			}
			startedAt := task.StartedAt
			completedAt := task.CompletedAt
			task.mu.RUnlock()

			if status == TaskStatusCompleted || status == TaskStatusFailed || status == TaskStatusCanceled {
				result := &TaskResult{
					TaskID:     taskID,
					Status:     status,
					Error:      taskErr,
					StartedAt:  *startedAt,
					FinishedAt: *completedAt,
				}
				if !startedAt.IsZero() && !completedAt.IsZero() {
					result.Duration = completedAt.Sub(startedAt.Time)
				}
				resultChan <- result
				return
			}

			// Wait a bit before checking again
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}
	}()

	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, types.WrapError(types.ErrCodeCanceled, "wait for result canceled", ctx.Err())
	}
}

// Cancel cancels a pending or running task
func (s *Scheduler) Cancel(ctx context.Context, taskID types.ID) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return types.NewError(types.ErrCodeUnavailable, "scheduler is closed")
	}

	// Try to remove from queue first
	if err := s.queue.Remove(taskID); err == nil {
		s.logger.Debug("Task canceled (queued)", "task_id", taskID)
		return nil
	}

	// Task might be running, try to cancel it through the worker
	for _, w := range s.workers {
		w.currentMu.RLock()
		if w.currentTask != nil && w.currentTask.ID == taskID {
			// Task is running - we can't directly cancel it, but we can mark it
			w.currentTask.mu.Lock()
			w.currentTask.Status = TaskStatusCanceled
			w.currentTask.mu.Unlock()
			w.currentMu.RUnlock()
			s.logger.Debug("Task canceled (running)", "task_id", taskID)
			return nil
		}
		w.currentMu.RUnlock()
	}

	return types.NewError(types.ErrCodeNotFound, "task not found: "+taskID.String())
}

// GetTask retrieves a task by ID
func (s *Scheduler) GetTask(ctx context.Context, taskID types.ID) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed && !s.isTaskInQueue(taskID) {
		return nil, types.NewError(types.ErrCodeUnavailable, "scheduler is closed")
	}

	// Check queue first
	task, err := s.queue.Get(taskID)
	if err == nil {
		return task, nil
	}

	// Check running tasks
	for _, w := range s.workers {
		w.currentMu.RLock()
		if w.currentTask != nil && w.currentTask.ID == taskID {
			w.currentMu.RUnlock()
			return w.currentTask, nil
		}
		w.currentMu.RUnlock()
	}

	return nil, types.NewError(types.ErrCodeNotFound, "task not found: "+taskID.String())
}

// isTaskInQueue checks if a task is still in the queue
func (s *Scheduler) isTaskInQueue(taskID types.ID) bool {
	task, err := s.queue.Get(taskID)
	return err == nil && task != nil
}

// ListTasks returns all tasks matching the filter
func (s *Scheduler) ListTasks(ctx context.Context, filter *TaskFilter) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "scheduler is closed")
	}

	var result []*Task

	// Add queued tasks
	for _, task := range s.queue.List() {
		if s.matchesFilter(task, filter) {
			result = append(result, task)
		}
	}

	// Add running tasks
	for _, w := range s.workers {
		w.currentMu.RLock()
		if w.currentTask != nil && s.matchesFilter(w.currentTask, filter) {
			result = append(result, w.currentTask)
		}
		w.currentMu.RUnlock()
	}

	return result, nil
}

// matchesFilter checks if a task matches the given filter
func (s *Scheduler) matchesFilter(task *Task, filter *TaskFilter) bool {
	if filter == nil {
		return true
	}

	task.mu.RLock()
	defer task.mu.RUnlock()

	if filter.Status != nil && task.Status != *filter.Status {
		return false
	}

	if filter.Priority != nil && task.Priority != *filter.Priority {
		return false
	}

	if filter.SessionID != nil && (task.SessionID == nil || *task.SessionID != *filter.SessionID) {
		return false
	}

	if filter.Name != nil && task.Name != *filter.Name {
		return false
	}

	return true
}

// Stats returns scheduler statistics
func (s *Scheduler) Stats() SchedulerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := SchedulerStats{
		TotalTasks:     atomic.LoadInt64(&s.taskCount),
		SuccessCount:   atomic.LoadInt64(&s.successCount),
		FailureCount:   atomic.LoadInt64(&s.failureCount),
		QueueSize:      s.queue.Size(),
		WorkerCount:    len(s.workers),
		ActiveWorkers:  0,
		QueueStats:     s.queue.Stats(),
	}

	for _, w := range s.workers {
		if w.working.Load() {
			stats.ActiveWorkers++
		}
	}

	return stats
}

// Close gracefully shuts down the scheduler
func (s *Scheduler) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.logger.Info("Scheduler shutting down...")

	// Stop accepting new tasks
	s.closed = true

	// Close queue
	s.queue.Close()

	// Cancel all workers
	s.cancelCtx()

	// Wait for all workers to finish current tasks
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	// Wait for graceful shutdown or timeout
	shutdownTimeout := s.cfg.QueueTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
	}

	select {
	case <-done:
		s.logger.Info("Scheduler shut down gracefully")
		return nil
	case <-time.After(shutdownTimeout):
		s.logger.Warn("Scheduler shutdown timeout")
		return types.NewError(types.ErrCodeTimeout, "scheduler shutdown timeout")
	}
}

// run is the main worker loop
func (w *worker) run(ctx context.Context) {
	defer w.scheduler.wg.Done()

	w.scheduler.logger.Debug("Worker started", "worker_id", w.id)

	for {
		select {
		case <-ctx.Done():
			w.scheduler.logger.Debug("Worker stopping", "worker_id", w.id)
			return

		case <-w.quitChan:
			w.scheduler.logger.Debug("Worker quitting", "worker_id", w.id)
			return

		default:
			// Try to get a task from the queue
			taskCtx, taskCancel := context.WithTimeout(ctx, w.scheduler.cfg.TaskTimeout)
			task, err := w.scheduler.queue.Dequeue(taskCtx)
			taskCancel()

			if err != nil {
				if ctx.Err() != nil {
					// Context canceled, exit
					return
				}
				// Other error, continue
				continue
			}

			// Execute task
			w.executeTask(ctx, task)
		}
	}
}

// executeTask executes a single task
func (w *worker) executeTask(ctx context.Context, task *Task) {
	w.working.Store(true)
	w.currentMu.Lock()
	w.currentTask = task
	w.currentMu.Unlock()

	startTime := types.NewTimestamp()
	task.mu.Lock()
	task.Status = TaskStatusRunning
	task.StartedAt = &startTime
	task.mu.Unlock()

	w.scheduler.logger.Debug("Task started",
		"task_id", task.ID,
		"name", task.Name,
		"worker_id", w.id)

	// Create task context with timeout
	var taskCtx context.Context
	var taskCancel context.CancelFunc

	if task.Timeout > 0 {
		taskCtx, taskCancel = context.WithTimeout(ctx, task.Timeout)
	} else {
		taskCtx, taskCancel = context.WithTimeout(ctx, w.scheduler.cfg.TaskTimeout)
	}
	defer taskCancel()

	// Execute handler
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("task panic: %v", r)
				w.scheduler.logger.Error("Task panic",
					"task_id", task.ID,
					"name", task.Name,
					"panic", r)
			}
		}()
		err = task.Handler(taskCtx, task)
	}()

	// Record completion
	finishTime := types.NewTimestamp()
	task.mu.Lock()
	task.CompletedAt = &finishTime

	if err != nil {
		task.Error = err.Error()

		// Check if we should retry
		if task.Retries < task.MaxRetries {
			task.Retries++
			task.Status = TaskStatusQueued
			task.Error = ""
			task.mu.Unlock()

			w.scheduler.logger.Debug("Task retrying",
				"task_id", task.ID,
				"name", task.Name,
				"retry", task.Retries,
				"max_retries", task.MaxRetries)

			// Re-enqueue for retry
			if enqueueErr := w.scheduler.queue.Enqueue(task); enqueueErr != nil {
				w.scheduler.logger.Error("Failed to re-enqueue task for retry",
					"task_id", task.ID,
					"error", enqueueErr)
				task.mu.Lock()
				task.Status = TaskStatusFailed
				task.mu.Unlock()
				atomic.AddInt64(&w.scheduler.failureCount, 1)
			}
		} else {
			task.Status = TaskStatusFailed
			task.mu.Unlock()
			atomic.AddInt64(&w.scheduler.failureCount, 1)

			w.scheduler.logger.Error("Task failed",
				"task_id", task.ID,
				"name", task.Name,
				"error", err)
		}
	} else {
		task.Status = TaskStatusCompleted
		task.mu.Unlock()
		atomic.AddInt64(&w.scheduler.successCount, 1)

		w.scheduler.logger.Debug("Task completed",
			"task_id", task.ID,
			"name", task.Name)
	}

	w.currentMu.Lock()
	w.currentTask = nil
	w.currentMu.Unlock()
	w.working.Store(false)
}

// TaskOption configures a task during submission
type TaskOption func(*Task)

// WithTaskID sets a custom task ID
func WithTaskID(id types.ID) TaskOption {
	return func(t *Task) {
		t.ID = id
	}
}

// WithTaskName sets the task name
func WithTaskName(name string) TaskOption {
	return func(t *Task) {
		t.Name = name
	}
}

// WithTaskPriority sets the task priority
func WithTaskPriority(priority TaskPriority) TaskOption {
	return func(t *Task) {
		t.Priority = priority
	}
}

// WithTaskSessionID associates the task with a session
func WithTaskSessionID(sessionID types.ID) TaskOption {
	return func(t *Task) {
		t.SessionID = &sessionID
	}
}

// WithTaskMetadata adds metadata to the task
func WithTaskMetadata(key string, value interface{}) TaskOption {
	return func(t *Task) {
		if t.Metadata == nil {
			t.Metadata = make(map[string]interface{})
		}
		t.Metadata[key] = value
	}
}

// WithTaskTimeout sets a custom timeout for the task
func WithTaskTimeout(timeout time.Duration) TaskOption {
	return func(t *Task) {
		t.Timeout = timeout
	}
}

// WithTaskMaxRetries sets the maximum number of retries for the task
func WithTaskMaxRetries(maxRetries int) TaskOption {
	return func(t *Task) {
		t.MaxRetries = maxRetries
	}
}

// SchedulerStats represents statistics about the scheduler
type SchedulerStats struct {
	TotalTasks    int64       `json:"total_tasks"`
	SuccessCount  int64       `json:"success_count"`
	FailureCount  int64       `json:"failure_count"`
	QueueSize     int         `json:"queue_size"`
	WorkerCount   int         `json:"worker_count"`
	ActiveWorkers int         `json:"active_workers"`
	QueueStats    QueueStats  `json:"queue_stats"`
}

// TaskFilter defines filters for querying tasks
type TaskFilter struct {
	Status    *TaskStatus   `json:"status,omitempty"`
	Priority  *TaskPriority `json:"priority,omitempty"`
	SessionID *types.ID     `json:"session_id,omitempty"`
	Name      *string       `json:"name,omitempty"`
}

// global scheduler instance
var (
	globalScheduler *Scheduler
	globalOnce      sync.Once
)

// InitGlobal initializes the global scheduler with the specified configuration
func InitGlobal(cfg config.SchedulerConfig, log *logger.Logger) error {
	var initErr error
	globalOnce.Do(func() {
		scheduler, err := New(cfg, log)
		if err != nil {
			initErr = err
			return
		}
		globalScheduler = scheduler
	})
	return initErr
}

// Global returns the global scheduler instance
func Global() *Scheduler {
	if globalScheduler == nil {
		// Initialize with default settings if not already initialized
		scheduler, err := NewDefault(nil)
		if err != nil {
			return nil
		}
		globalScheduler = scheduler
	}
	return globalScheduler
}

// SetGlobal sets the global scheduler instance
func SetGlobal(s *Scheduler) {
	globalScheduler = s
	globalOnce = sync.Once{}
}

// Submit is a convenience function that submits a task to the global scheduler
func Submit(ctx context.Context, handler TaskHandler, opts ...TaskOption) (types.ID, error) {
	return Global().Submit(ctx, handler, opts...)
}

// SubmitWithResult is a convenience function that submits a task to the global scheduler and waits for result
func SubmitWithResult(ctx context.Context, handler TaskHandler, opts ...TaskOption) (*TaskResult, error) {
	return Global().SubmitWithResult(ctx, handler, opts...)
}
