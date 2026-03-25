package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// HandlerFunc is the function signature for task handlers
type HandlerFunc func(ctx context.Context, task *Task) (map[string]any, error)

// Worker represents a single task execution unit
type Worker struct {
	id       string
	taskCh   <-chan *Task
	resultCh chan<- TaskResult
	handlers map[string]HandlerFunc
	quit     chan struct{}
	wg       *sync.WaitGroup
	log      *slog.Logger
}

func newWorker(id string, taskCh <-chan *Task, resultCh chan<- TaskResult, handlers map[string]HandlerFunc, wg *sync.WaitGroup, log *slog.Logger) *Worker {
	return &Worker{
		id:       id,
		taskCh:   taskCh,
		resultCh: resultCh,
		handlers: handlers,
		quit:     make(chan struct{}),
		wg:       wg,
		log:      log,
	}
}

func (w *Worker) start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.log.Info("worker started", "worker_id", w.id)
		for {
			select {
			case task, ok := <-w.taskCh:
				if !ok {
					w.log.Info("worker shutting down", "worker_id", w.id)
					return
				}
				result := w.execute(task)
				w.resultCh <- result
			case <-w.quit:
				w.log.Info("worker received quit signal", "worker_id", w.id)
				return
			}
		}
	}()
}

func (w *Worker) execute(task *Task) TaskResult {
	start := time.Now()

	handler, ok := w.handlers[task.Type]
	if !ok {
		return TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    fmt.Errorf("no handler registered for task type %q", task.Type),
			Duration: time.Since(start),
			WorkerID: w.id,
		}
	}

	// Set up context with optional timeout
	ctx := context.Background()
	var cancel context.CancelFunc
	if task.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, task.Timeout)
		defer cancel()
	}

	// Add task info to context
	ctx = context.WithValue(ctx, contextKeyTaskID, task.ID)
	ctx = context.WithValue(ctx, contextKeyWorkerID, w.id)

	w.log.Info("executing task",
		"task_id", task.ID,
		"task_type", task.Type,
		"worker_id", w.id,
		"priority", task.Priority,
	)

	result, err := handler(ctx, task)
	duration := time.Since(start)

	if err != nil {
		w.log.Warn("task failed",
			"task_id", task.ID,
			"worker_id", w.id,
			"error", err,
			"duration", duration,
		)
		return TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err,
			Duration: duration,
			WorkerID: w.id,
		}
	}

	w.log.Info("task completed",
		"task_id", task.ID,
		"worker_id", w.id,
		"duration", duration,
	)

	return TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Result:   result,
		Duration: duration,
		WorkerID: w.id,
	}
}

// WorkerPool manages a pool of workers
type WorkerPool struct {
	workers  []*Worker
	taskCh   chan *Task
	resultCh chan TaskResult
	handlers map[string]HandlerFunc
	mu       sync.RWMutex
	wg       sync.WaitGroup
	log      *slog.Logger
	size     int
}

// NewWorkerPool creates a new worker pool with the given concurrency
func NewWorkerPool(size int, log *slog.Logger) *WorkerPool {
	wp := &WorkerPool{
		taskCh:   make(chan *Task, size*2),
		resultCh: make(chan TaskResult, size*2),
		handlers: make(map[string]HandlerFunc),
		log:      log,
		size:     size,
	}
	return wp
}

// Register adds a handler for the given task type
func (wp *WorkerPool) Register(taskType string, fn HandlerFunc) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.handlers[taskType] = fn
	wp.log.Info("handler registered", "task_type", taskType)
}

// Start launches all workers
func (wp *WorkerPool) Start() {
	wp.mu.RLock()
	handlers := wp.handlers
	wp.mu.RUnlock()

	for i := 0; i < wp.size; i++ {
		id := fmt.Sprintf("worker-%03d", i+1)
		w := newWorker(id, wp.taskCh, wp.resultCh, handlers, &wp.wg, wp.log)
		wp.workers = append(wp.workers, w)
		w.start()
	}
	wp.log.Info("worker pool started", "size", wp.size)
}

// Submit sends a task to an available worker
func (wp *WorkerPool) Submit(task *Task) {
	wp.taskCh <- task
}

// Results returns the results channel
func (wp *WorkerPool) Results() <-chan TaskResult {
	return wp.resultCh
}

// Stop gracefully shuts down the worker pool
func (wp *WorkerPool) Stop() {
	close(wp.taskCh)
	wp.wg.Wait()
	close(wp.resultCh)
}

// ActiveWorkers returns the number of configured workers
func (wp *WorkerPool) ActiveWorkers() int {
	return len(wp.workers)
}

// context keys
type contextKey string

const (
	contextKeyTaskID   contextKey = "task_id"
	contextKeyWorkerID contextKey = "worker_id"
)

// TaskIDFromContext extracts the task ID from a context
func TaskIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyTaskID).(string)
	return v
}

// WorkerIDFromContext extracts the worker ID from a context
func WorkerIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyWorkerID).(string)
	return v
}
