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

const workerID = "worker"

// Worker is a single task execution unit
type Worker struct {
	taskCh   chan *Task
	resultCh chan TaskResult
	handlers map[string]HandlerFunc
	mu       sync.RWMutex
	wg       sync.WaitGroup
	log      *slog.Logger
}

// NewWorker creates a single worker
func NewWorker(log *slog.Logger) *Worker {
	return &Worker{
		taskCh:   make(chan *Task, 64),
		resultCh: make(chan TaskResult, 64),
		handlers: make(map[string]HandlerFunc),
		log:      log,
	}
}

// Register adds a handler for the given task type
func (w *Worker) Register(taskType string, fn HandlerFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[taskType] = fn
	w.log.Info("handler registered", "task_type", taskType)
}

// Start launches the worker goroutine
func (w *Worker) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.log.Info("worker started", "worker_id", workerID)
		for task := range w.taskCh {
			w.resultCh <- w.execute(task)
		}
		w.log.Info("worker stopped", "worker_id", workerID)
	}()
}

// Submit sends a task to the worker
func (w *Worker) Submit(task *Task) {
	w.taskCh <- task
}

// Results returns the results channel
func (w *Worker) Results() <-chan TaskResult {
	return w.resultCh
}

// Stop gracefully shuts down the worker
func (w *Worker) Stop() {
	close(w.taskCh)
	w.wg.Wait()
	close(w.resultCh)
}

func (w *Worker) execute(task *Task) TaskResult {
	start := time.Now()

	w.mu.RLock()
	handler, ok := w.handlers[task.Type]
	w.mu.RUnlock()

	if !ok {
		return TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    fmt.Errorf("no handler registered for task type %q", task.Type),
			Duration: time.Since(start),
			WorkerID: workerID,
		}
	}

	ctx := context.Background()
	var cancel context.CancelFunc
	if task.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, task.Timeout)
		defer cancel()
	}

	ctx = context.WithValue(ctx, contextKeyTaskID, task.ID)
	ctx = context.WithValue(ctx, contextKeyWorkerID, workerID)

	w.log.Info("executing task",
		"task_id", task.ID,
		"task_type", task.Type,
		"worker_id", workerID,
		"priority", task.Priority,
	)

	result, err := handler(ctx, task)
	duration := time.Since(start)

	if err != nil {
		w.log.Warn("task failed",
			"task_id", task.ID,
			"worker_id", workerID,
			"error", err,
			"duration", duration,
		)
		return TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err,
			Duration: duration,
			WorkerID: workerID,
		}
	}

	w.log.Info("task completed",
		"task_id", task.ID,
		"worker_id", workerID,
		"duration", duration,
	)

	return TaskResult{
		TaskID:   task.ID,
		Success:  true,
		Result:   result,
		Duration: duration,
		WorkerID: workerID,
	}
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
