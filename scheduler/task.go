package scheduler

import (
	"time"
)

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusRetrying  TaskStatus = "retrying"
	StatusCancelled TaskStatus = "cancelled"
)

// Priority defines task execution priority
type Priority int

const (
	PriorityLow    Priority = 1
	PriorityNormal Priority = 5
	PriorityHigh   Priority = 10
)

// Task represents a unit of work to be executed
type Task struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Payload     map[string]any    `json:"payload"`
	Priority    Priority          `json:"priority"`
	Status      TaskStatus        `json:"status"`
	ScheduledAt time.Time         `json:"scheduled_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Error       string            `json:"error,omitempty"`
	Result      map[string]any    `json:"result,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`

	// Retry configuration
	MaxRetries     int           `json:"max_retries"`
	RetryCount     int           `json:"retry_count"`
	RetryDelay     time.Duration `json:"retry_delay"`
	RetryBackoff   float64       `json:"retry_backoff"` // multiplier per retry

	// Cron configuration
	CronExpr    string     `json:"cron_expr,omitempty"`
	NextRunAt   *time.Time `json:"next_run_at,omitempty"`
	IsRecurring bool       `json:"is_recurring"`

	// Timeout
	Timeout time.Duration `json:"timeout,omitempty"`

	// Dependencies
	DependsOn []string `json:"depends_on,omitempty"`

	// Worker assignment
	WorkerID string `json:"worker_id,omitempty"`
}

// TaskOption is a functional option for configuring tasks
type TaskOption func(*Task)

// WithPriority sets task priority
func WithPriority(p Priority) TaskOption {
	return func(t *Task) { t.Priority = p }
}

// WithRetry configures retry behavior
func WithRetry(maxRetries int, delay time.Duration, backoff float64) TaskOption {
	return func(t *Task) {
		t.MaxRetries = maxRetries
		t.RetryDelay = delay
		t.RetryBackoff = backoff
	}
}

// WithCron sets a cron expression for recurring tasks
func WithCron(expr string) TaskOption {
	return func(t *Task) {
		t.CronExpr = expr
		t.IsRecurring = true
	}
}

// WithSchedule sets a future execution time
func WithSchedule(at time.Time) TaskOption {
	return func(t *Task) { t.ScheduledAt = at }
}

// WithTimeout sets a task timeout
func WithTimeout(d time.Duration) TaskOption {
	return func(t *Task) { t.Timeout = d }
}

// WithTags adds tags to a task
func WithTags(tags ...string) TaskOption {
	return func(t *Task) { t.Tags = append(t.Tags, tags...) }
}

// WithDependencies sets task dependencies
func WithDependencies(ids ...string) TaskOption {
	return func(t *Task) { t.DependsOn = ids }
}

// WithMetadata adds metadata to a task
func WithMetadata(key, value string) TaskOption {
	return func(t *Task) {
		if t.Metadata == nil {
			t.Metadata = make(map[string]string)
		}
		t.Metadata[key] = value
	}
}

// nextRetryDelay calculates the delay before the next retry attempt
func (t *Task) nextRetryDelay() time.Duration {
	if t.RetryBackoff <= 0 {
		t.RetryBackoff = 2.0
	}
	delay := t.RetryDelay
	for i := 0; i < t.RetryCount; i++ {
		delay = time.Duration(float64(delay) * t.RetryBackoff)
	}
	return delay
}

// CanRetry returns true if the task has remaining retry attempts
func (t *Task) CanRetry() bool {
	return t.RetryCount < t.MaxRetries
}

// TaskResult holds the outcome of a task execution
type TaskResult struct {
	TaskID    string
	Success   bool
	Result    map[string]any
	Error     error
	Duration  time.Duration
	WorkerID  string
}
