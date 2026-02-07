package scheduler

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/baaaht/orchestrator/pkg/types"
)

// TaskStatus represents the current status of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCanceled  TaskStatus = "canceled"
)

// TaskPriority represents the priority level of a task
type TaskPriority int

const (
	PriorityLow    TaskPriority = 0
	PriorityNormal TaskPriority = 1
	PriorityHigh   TaskPriority = 2
)

// Task represents a unit of work to be executed
type Task struct {
	ID          types.ID            `json:"id"`
	Name        string              `json:"name"`
	Handler     TaskHandler         `json:"-"`
	Priority    TaskPriority        `json:"priority"`
	Context     context.Context     `json:"-"`
	SessionID   *types.ID           `json:"session_id,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Status      TaskStatus          `json:"status"`
	CreatedAt   types.Timestamp     `json:"created_at"`
	StartedAt   *types.Timestamp    `json:"started_at,omitempty"`
	CompletedAt *types.Timestamp    `json:"completed_at,omitempty"`
	Error       string              `json:"error,omitempty"`
	Retries     int                 `json:"retries"`
	MaxRetries  int                 `json:"max_retries"`
	Timeout     time.Duration       `json:"timeout"`
	mu          sync.RWMutex       `json:"-"`
}

// TaskHandler is a function that executes a task
type TaskHandler func(ctx context.Context, task *Task) error

// TaskResult represents the result of a task execution
type TaskResult struct {
	TaskID     types.ID     `json:"task_id"`
	Status     TaskStatus   `json:"status"`
	Error      error        `json:"error,omitempty"`
	StartedAt  types.Timestamp `json:"started_at"`
	FinishedAt types.Timestamp `json:"finished_at"`
	Duration   time.Duration `json:"duration"`
}

// priorityQueue implements heap.Interface for priority-based task queue
type priorityQueue []*Task

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Higher priority value means higher priority
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	// If priorities are equal, older tasks (lower created timestamp) come first
	return pq[i].CreatedAt.Before(pq[j].CreatedAt.Time)
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x interface{}) {
	task := x.(*Task)
	*pq = append(*pq, task)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	task := old[n-1]
	*pq = old[0 : n-1]
	return task
}

// Queue represents a priority-based task queue
type Queue struct {
	mu         sync.RWMutex
	pq         priorityQueue
	tasks      map[types.ID]*Task
	notifyChan chan struct{}
	closed     bool
	maxSize    int
}

// NewQueue creates a new task queue with the specified maximum size
func NewQueue(maxSize int) *Queue {
	q := &Queue{
		pq:         make(priorityQueue, 0),
		tasks:      make(map[types.ID]*Task),
		notifyChan: make(chan struct{}, 1),
		closed:     false,
		maxSize:    maxSize,
	}
	heap.Init(&q.pq)
	return q
}

// Enqueue adds a task to the queue. Returns an error if the queue is full or closed.
func (q *Queue) Enqueue(task *Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return types.NewError(types.ErrCodeUnavailable, "queue is closed")
	}

	if q.maxSize > 0 && len(q.tasks) >= q.maxSize {
		return types.NewError(types.ErrCodeResourceExhausted,
			"queue is full")
	}

	// Ensure task has ID and timestamp
	if task.ID.IsEmpty() {
		task.ID = types.GenerateID()
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = types.NewTimestamp()
	}

	// Set status to queued
	task.Status = TaskStatusQueued

	// Add to map and heap
	q.tasks[task.ID] = task
	heap.Push(&q.pq, task)

	// Notify one waiting goroutine (non-blocking)
	select {
	case q.notifyChan <- struct{}{}:
	default:
	}

	return nil
}

// Dequeue removes and returns the highest priority task from the queue.
// Blocks until a task is available or the context is canceled.
func (q *Queue) Dequeue(ctx context.Context) (*Task, error) {
	for {
		q.mu.Lock()
		task, err := q.tryDequeue()
		q.mu.Unlock()

		if err != nil {
			// If queue is empty but not closed, wait for notification
			if types.IsErrCode(err, types.ErrCodeResourceExhausted) && !q.closed {
				select {
				case <-q.notifyChan:
					continue
				case <-ctx.Done():
					return nil, types.WrapError(types.ErrCodeCanceled, "dequeue canceled", ctx.Err())
				}
			}
			return nil, err
		}

		return task, nil
	}
}

// tryDequeue attempts to dequeue a task without blocking
func (q *Queue) tryDequeue() (*Task, error) {
	if q.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "queue is closed")
	}

	if q.pq.Len() == 0 {
		return nil, types.NewError(types.ErrCodeResourceExhausted, "queue is empty")
	}

	task := heap.Pop(&q.pq).(*Task)
	delete(q.tasks, task.ID)

	return task, nil
}

// Peek returns the highest priority task without removing it from the queue
func (q *Queue) Peek() (*Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "queue is closed")
	}

	if q.pq.Len() == 0 {
		return nil, types.NewError(types.ErrCodeNotFound, "queue is empty")
	}

	return q.pq[0], nil
}

// Remove removes a task from the queue by ID
func (q *Queue) Remove(taskID types.ID) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return types.NewError(types.ErrCodeUnavailable, "queue is closed")
	}

	task, exists := q.tasks[taskID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "task not found: "+taskID.String())
	}

	// Remove from heap (mark as deleted and rebuild)
	task.Status = TaskStatusCanceled
	delete(q.tasks, taskID)

	// Rebuild heap without the removed task
	newPQ := make(priorityQueue, 0, len(q.pq)-1)
	for _, t := range q.pq {
		if t.ID != taskID {
			newPQ = append(newPQ, t)
		}
	}
	q.pq = newPQ
	heap.Init(&q.pq)

	return nil
}

// Get retrieves a task by ID without removing it from the queue
func (q *Queue) Get(taskID types.ID) (*Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	task, exists := q.tasks[taskID]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, "task not found: "+taskID.String())
	}

	return task, nil
}

// List returns all tasks currently in the queue
func (q *Queue) List() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	tasks := make([]*Task, 0, len(q.tasks))
	for _, task := range q.tasks {
		tasks = append(tasks, task)
	}

	return tasks
}

// Size returns the current number of tasks in the queue
func (q *Queue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.tasks)
}

// IsEmpty returns true if the queue is empty
func (q *Queue) IsEmpty() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.tasks) == 0
}

// IsFull returns true if the queue has reached its maximum size
func (q *Queue) IsFull() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.maxSize > 0 && len(q.tasks) >= q.maxSize
}

// Close closes the queue and prevents further operations
func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	q.closed = true
	close(q.notifyChan)
}

// Closed returns true if the queue is closed
func (q *Queue) Closed() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.closed
}

// Clear removes all tasks from the queue
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Mark all tasks as canceled
	for _, task := range q.tasks {
		task.Status = TaskStatusCanceled
	}

	q.pq = make(priorityQueue, 0)
	q.tasks = make(map[types.ID]*Task)
	heap.Init(&q.pq)
}

// UpdatePriority updates the priority of a task in the queue
func (q *Queue) UpdatePriority(taskID types.ID, priority TaskPriority) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return types.NewError(types.ErrCodeUnavailable, "queue is closed")
	}

	task, exists := q.tasks[taskID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, "task not found: "+taskID.String())
	}

	task.mu.Lock()
	task.Priority = priority
	task.mu.Unlock()

	// Rebuild heap to reflect new priority
	heap.Init(&q.pq)

	return nil
}

// Stats returns statistics about the queue
func (q *Queue) Stats() QueueStats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := QueueStats{
		Size:     len(q.tasks),
		MaxSize:  q.maxSize,
		Closed:   q.closed,
		Priority: make(map[TaskPriority]int),
	}

	for _, task := range q.tasks {
		task.mu.RLock()
		stats.Priority[task.Priority]++
		task.mu.RUnlock()
	}

	return stats
}

// QueueStats represents statistics about the queue
type QueueStats struct {
	Size     int                      `json:"size"`
	MaxSize  int                      `json:"max_size"`
	Closed   bool                     `json:"closed"`
	Priority map[TaskPriority]int     `json:"priority,omitempty"`
}
