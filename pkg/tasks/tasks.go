package tasks

import (
	"fmt"
	"sync"
	"time"
)

// Status represents the state of an async task.
type Status string

const (
	Pending Status = "pending"
	Running Status = "running"
	Failed  Status = "failed"
	Done    Status = "done"
)

// Task represents an asynchronous operation invoked via MCP tools.
type Task struct {
	ID        string
	Type      string
	Status    Status
	Result    map[string]interface{}
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Manager tracks async tasks in a thread-safe manner.
type Manager struct {
	mu     sync.RWMutex
	tasks  map[string]*Task
	nextID int64
}

// NewManager creates a new task manager.
func NewManager() *Manager {
	return &Manager{
		tasks: make(map[string]*Task),
	}
}

// Create registers a new pending task and returns it.
func (m *Manager) Create(taskType string) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	task := &Task{
		ID:        fmt.Sprintf("%s-%d", taskType, m.nextID),
		Type:      taskType,
		Status:    Pending,
		Result:    make(map[string]interface{}),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.tasks[task.ID] = task
	return task
}

// Get retrieves a task by ID.
func (m *Manager) Get(id string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	return task, ok
}

// UpdateStatus changes the status of a task.
func (m *Manager) UpdateStatus(id string, status Status) (*Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	task.Status = status
	task.UpdatedAt = time.Now()
	return task, true
}

// SetResult stores the result of a completed task.
func (m *Manager) SetResult(id string, result map[string]interface{}) (*Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	task.Result = result
	task.Status = Done
	task.UpdatedAt = time.Now()
	return task, true
}

// SetError marks a task as failed with an error message.
func (m *Manager) SetError(id string, err error) (*Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	task.Status = Failed
	if err != nil {
		task.Error = err.Error()
	}
	task.UpdatedAt = time.Now()
	return task, true
}

// ToMap returns a serializable representation of a task.
func (t *Task) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":         t.ID,
		"type":       t.Type,
		"status":     string(t.Status),
		"result":     t.Result,
		"error":      t.Error,
		"created_at": t.CreatedAt.Format(time.RFC3339),
		"updated_at": t.UpdatedAt.Format(time.RFC3339),
	}
}
