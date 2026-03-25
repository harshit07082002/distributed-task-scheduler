package scheduler

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Config holds scheduler configuration
type Config struct {
	PollInterval    time.Duration
	ShutdownTimeout time.Duration
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() Config {
	return Config{
		PollInterval:    100 * time.Millisecond,
		ShutdownTimeout: 30 * time.Second,
	}
}

// Scheduler is the main orchestrator
type Scheduler struct {
	config Config
	worker *Worker
	store  TaskStore
	log    *slog.Logger

	mu       sync.RWMutex
	cronJobs map[string]*CronSchedule
	running  bool
	quit     chan struct{}
	done     chan struct{}

	onTaskComplete []func(task *Task, result TaskResult)
	onTaskFail     []func(task *Task, err error)
}

// TaskStore interface for persisting tasks
type TaskStore interface {
	Save(task *Task) error
	Get(id string) (*Task, error)
	GetAll() ([]*Task, error)
	Delete(id string) error
	GetByStatus(status TaskStatus) ([]*Task, error)
	// Enqueue adds a task to the scheduled queue (score = ScheduledAt)
	Enqueue(task *Task) error
	// PopDue atomically removes and returns the earliest task due by now; nil if none
	PopDue(now time.Time) (*Task, error)
}

// New creates a new Scheduler instance
func New(cfg Config, store TaskStore, log *slog.Logger) *Scheduler {
	return &Scheduler{
		config:   cfg,
		worker:   NewWorker(log),
		store:    store,
		log:      log,
		cronJobs: make(map[string]*CronSchedule),
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Register adds a handler for a task type
func (s *Scheduler) Register(taskType string, fn HandlerFunc) {
	s.worker.Register(taskType, fn)
}

// OnTaskComplete adds a hook called when a task succeeds
func (s *Scheduler) OnTaskComplete(fn func(task *Task, result TaskResult)) {
	s.onTaskComplete = append(s.onTaskComplete, fn)
}

// OnTaskFail adds a hook called when a task fails permanently
func (s *Scheduler) OnTaskFail(fn func(task *Task, err error)) {
	s.onTaskFail = append(s.onTaskFail, fn)
}

// Submit creates and saves a new task to the store
func (s *Scheduler) Submit(name, taskType string, payload map[string]any, opts ...TaskOption) (*Task, error) {
	task := &Task{
		ID:           uuid.New().String(),
		Name:         name,
		Type:         taskType,
		Payload:      payload,
		Priority:     PriorityNormal,
		Status:       StatusPending,
		ScheduledAt:  time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		RetryBackoff: 2.0,
	}

	for _, opt := range opts {
		opt(task)
	}

	if task.IsRecurring && task.CronExpr != "" {
		cron, err := ParseCron(task.CronExpr)
		if err != nil {
			return nil, fmt.Errorf("invalid cron expression: %w", err)
		}
		s.mu.Lock()
		s.cronJobs[task.ID] = cron
		s.mu.Unlock()

		next := cron.Next(time.Now())
		task.ScheduledAt = next
		task.NextRunAt = &next
	}

	if err := s.checkDependencies(task); err != nil {
		return nil, err
	}

	if err := s.store.Save(task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}
	if err := s.store.Enqueue(task); err != nil {
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}

	s.log.Info("task submitted",
		"task_id", task.ID,
		"task_name", task.Name,
		"task_type", task.Type,
		"scheduled_at", task.ScheduledAt,
		"priority", task.Priority,
	)

	return task, nil
}

// Cancel marks a task as cancelled
func (s *Scheduler) Cancel(id string) error {
	task, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if task.Status == StatusRunning {
		return fmt.Errorf("cannot cancel a running task")
	}
	task.Status = StatusCancelled
	task.UpdatedAt = time.Now()
	return s.store.Save(task)
}

// GetTask retrieves a task by ID
func (s *Scheduler) GetTask(id string) (*Task, error) {
	return s.store.Get(id)
}

// ListTasks returns all tasks, optionally filtered by status
func (s *Scheduler) ListTasks(status ...TaskStatus) ([]*Task, error) {
	if len(status) == 0 {
		return s.store.GetAll()
	}
	return s.store.GetByStatus(status[0])
}

// Stats returns scheduler statistics
func (s *Scheduler) Stats() map[string]any {
	all, _ := s.store.GetAll()
	counts := map[TaskStatus]int{}
	for _, t := range all {
		counts[t.Status]++
	}
	return map[string]any{
		"total_tasks": len(all),
		"pending":     counts[StatusPending],
		"running":     counts[StatusRunning],
		"completed":   counts[StatusCompleted],
		"failed":      counts[StatusFailed],
		"retrying":    counts[StatusRetrying],
		"cancelled":   counts[StatusCancelled],
	}
}

// Start begins the scheduler dispatch loop
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}
	s.running = true
	s.mu.Unlock()

	s.worker.Start()
	s.recoverPendingTasks()

	go s.dispatchLoop()
	go s.resultLoop()

	s.log.Info("scheduler started", "poll_interval", s.config.PollInterval)
	return nil
}

// Stop gracefully shuts down the scheduler
func (s *Scheduler) Stop() {
	s.log.Info("scheduler shutting down...")
	close(s.quit)
	<-s.done
	s.worker.Stop()
	s.log.Info("scheduler stopped")
}

// dispatchLoop polls Redis on every tick
func (s *Scheduler) dispatchLoop() {
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.quit:
			close(s.done)
			return
		case <-ticker.C:
			s.dispatch()
		}
	}
}

// dispatch atomically pops the earliest due task from the queue and sends it to the worker
func (s *Scheduler) dispatch() {
	task, err := s.store.PopDue(time.Now())
	if err != nil {
		s.log.Error("failed to pop due task", "error", err)
		return
	}
	if task == nil {
		return
	}

	now := time.Now()
	task.Status = StatusRunning
	task.StartedAt = &now
	task.WorkerID = workerID
	task.UpdatedAt = now
	if err := s.store.Save(task); err != nil {
		s.log.Error("failed to mark task as running", "task_id", task.ID, "error", err)
		_ = s.store.Enqueue(task) // put back on failure
		return
	}

	s.worker.Submit(task)
}

// resultLoop processes results from the worker
func (s *Scheduler) resultLoop() {
	for result := range s.worker.Results() {
		s.handleResult(result)
	}
}

func (s *Scheduler) handleResult(result TaskResult) {
	task, err := s.store.Get(result.TaskID)
	if err != nil {
		s.log.Error("failed to retrieve task after execution", "task_id", result.TaskID)
		return
	}

	now := time.Now()
	task.WorkerID = result.WorkerID
	task.UpdatedAt = now

	if result.Success {
		task.Status = StatusCompleted
		task.CompletedAt = &now
		task.Result = result.Result

		if task.IsRecurring {
			s.reschedule(task)
		}

		for _, hook := range s.onTaskComplete {
			hook(task, result)
		}
	} else {
		if task.CanRetry() {
			task.RetryCount++
			task.Status = StatusPending
			retryAt := now.Add(task.nextRetryDelay())
			task.ScheduledAt = retryAt
			task.Error = result.Error.Error()

			s.log.Info("scheduling retry",
				"task_id", task.ID,
				"attempt", task.RetryCount,
				"max", task.MaxRetries,
				"retry_at", retryAt,
			)

			_ = s.store.Save(task)
			_ = s.store.Enqueue(task)
			return
		}

		task.Status = StatusFailed
		task.Error = result.Error.Error()
		task.CompletedAt = &now

		for _, hook := range s.onTaskFail {
			hook(task, result.Error)
		}
	}

	_ = s.store.Save(task)
}

// reschedule saves the next run of a recurring task to Redis
func (s *Scheduler) reschedule(task *Task) {
	s.mu.RLock()
	cron, ok := s.cronJobs[task.ID]
	s.mu.RUnlock()
	if !ok {
		return
	}

	next := cron.Next(time.Now())
	newTask := *task
	newTask.ID = uuid.New().String()
	newTask.Status = StatusPending
	newTask.ScheduledAt = next
	newTask.NextRunAt = &next
	newTask.StartedAt = nil
	newTask.CompletedAt = nil
	newTask.WorkerID = ""
	newTask.Error = ""
	newTask.Result = nil
	newTask.RetryCount = 0
	newTask.CreatedAt = time.Now()
	newTask.UpdatedAt = time.Now()

	s.mu.Lock()
	s.cronJobs[newTask.ID] = cron
	s.mu.Unlock()

	_ = s.store.Save(&newTask)
	_ = s.store.Enqueue(&newTask)

	s.log.Info("recurring task rescheduled",
		"task_id", newTask.ID,
		"original_id", task.ID,
		"next_run", next,
	)
}

// recoverPendingTasks resets any running tasks to pending on startup
func (s *Scheduler) recoverPendingTasks() {
	pending, _ := s.store.GetByStatus(StatusPending)
	retrying, _ := s.store.GetByStatus(StatusRetrying)
	running, _ := s.store.GetByStatus(StatusRunning)

	tasks := append(append(pending, retrying...), running...)
	for _, t := range tasks {
		if t.Status == StatusRunning {
			t.Status = StatusPending
			t.StartedAt = nil
			_ = s.store.Save(t)
		}
		if t.IsRecurring && t.CronExpr != "" {
			if cron, err := ParseCron(t.CronExpr); err == nil {
				s.mu.Lock()
				s.cronJobs[t.ID] = cron
				s.mu.Unlock()
			}
		}
		_ = s.store.Enqueue(t)
	}

	if len(tasks) > 0 {
		s.log.Info("recovered tasks from store", "count", len(tasks))
	}
}

// checkDependencies verifies all dependencies are completed
func (s *Scheduler) checkDependencies(task *Task) error {
	for _, depID := range task.DependsOn {
		dep, err := s.store.Get(depID)
		if err != nil {
			return fmt.Errorf("dependency %s not found: %w", depID, err)
		}
		if dep.Status != StatusCompleted {
			return fmt.Errorf("dependency %s is not completed (status: %s)", depID, dep.Status)
		}
	}
	return nil
}
