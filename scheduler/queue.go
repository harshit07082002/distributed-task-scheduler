package scheduler

import (
	"container/heap"
	"sync"
	"time"
)

// taskHeap implements heap.Interface for priority-based task ordering
type taskHeap []*Task

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool {
	// Higher priority first; ties broken by ScheduledAt (earlier first)
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority
	}
	return h[i].ScheduledAt.Before(h[j].ScheduledAt)
}

func (h taskHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *taskHeap) Push(x any) {
	*h = append(*h, x.(*Task))
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	task := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return task
}

// TaskQueue is a thread-safe priority queue for tasks
type TaskQueue struct {
	mu      sync.Mutex
	heap    taskHeap
	notify  chan struct{}
	closed  bool
}

// NewTaskQueue creates a new empty task queue
func NewTaskQueue() *TaskQueue {
	q := &TaskQueue{
		heap:   make(taskHeap, 0),
		notify: make(chan struct{}, 1),
	}
	heap.Init(&q.heap)
	return q
}

// Push adds a task to the queue
func (q *TaskQueue) Push(task *Task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	heap.Push(&q.heap, task)
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Pop removes and returns the next ready task (nil if none available)
func (q *TaskQueue) Pop() *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	for q.heap.Len() > 0 {
		top := q.heap[0]
		if top.ScheduledAt.After(now) {
			break // Not yet ready
		}
		// Check if it's a ready task
		task := heap.Pop(&q.heap).(*Task)
		return task
	}
	return nil
}

// Peek returns the next scheduled time without removing the task
func (q *TaskQueue) Peek() *time.Time {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.heap.Len() == 0 {
		return nil
	}
	t := q.heap[0].ScheduledAt
	return &t
}

// Len returns the number of tasks in the queue
func (q *TaskQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.heap.Len()
}

// Notify returns a channel that receives a signal when tasks are added
func (q *TaskQueue) Notify() <-chan struct{} {
	return q.notify
}

// Close shuts down the queue
func (q *TaskQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	close(q.notify)
}

// Drain returns all pending tasks (for shutdown/persistence)
func (q *TaskQueue) Drain() []*Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	tasks := make([]*Task, len(q.heap))
	copy(tasks, q.heap)
	return tasks
}
