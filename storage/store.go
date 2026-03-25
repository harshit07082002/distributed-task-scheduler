package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/example/task-scheduler/scheduler"
)

// MemoryStore is a thread-safe in-memory store with optional file persistence
type MemoryStore struct {
	mu       sync.RWMutex
	tasks    map[string]*scheduler.Task
	filePath string
}

// NewMemoryStore creates a new in-memory store
// If filePath is non-empty, tasks are persisted to disk
func NewMemoryStore(filePath string) (*MemoryStore, error) {
	s := &MemoryStore{
		tasks:    make(map[string]*scheduler.Task),
		filePath: filePath,
	}

	if filePath != "" {
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}
		if err := s.load(); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load persisted tasks: %w", err)
		}
	}

	return s, nil
}

// Save persists a task to the store
func (s *MemoryStore) Save(task *scheduler.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskCopy := *task
	s.tasks[task.ID] = &taskCopy

	if s.filePath != "" {
		return s.flush()
	}
	return nil
}

// Get retrieves a task by ID
func (s *MemoryStore) Get(id string) (*scheduler.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}
	taskCopy := *task
	return &taskCopy, nil
}

// GetAll returns all tasks
func (s *MemoryStore) GetAll() ([]*scheduler.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*scheduler.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		copy := *t
		result = append(result, &copy)
	}
	return result, nil
}

// Delete removes a task by ID
func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[id]; !ok {
		return fmt.Errorf("task %q not found", id)
	}
	delete(s.tasks, id)

	if s.filePath != "" {
		return s.flush()
	}
	return nil
}

// GetByStatus returns all tasks with the given status
func (s *MemoryStore) GetByStatus(status scheduler.TaskStatus) ([]*scheduler.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*scheduler.Task
	for _, t := range s.tasks {
		if t.Status == status {
			copy := *t
			result = append(result, &copy)
		}
	}
	return result, nil
}

// flush writes all tasks to the file (must hold write lock)
func (s *MemoryStore) flush() error {
	tasks := make([]*scheduler.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tasks: %w", err)
	}

	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("failed to write task file: %w", err)
	}
	return os.Rename(tmp, s.filePath)
}

// load reads tasks from file
func (s *MemoryStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	var tasks []*scheduler.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return fmt.Errorf("failed to unmarshal tasks: %w", err)
	}

	for _, t := range tasks {
		s.tasks[t.ID] = t
	}
	return nil
}

// Count returns the number of stored tasks
func (s *MemoryStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tasks)
}
