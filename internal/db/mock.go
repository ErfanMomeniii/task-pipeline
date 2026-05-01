package db

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/erfanmomeniii/task-pipeline/internal/models"
)

// MockStore is a test mock implementing TaskStore.
type MockStore struct {
	mu    sync.Mutex
	tasks map[int64]*Task
	nextID atomic.Int64

	// Optional error injection.
	InsertErr        error
	UpdateErr        error
	UpdateErrOnCall  int // fail on Nth UpdateTaskState call (1-based, 0 = use UpdateErr)
	updateCallCount  int
	SumValueErr      error
	SumValueResult   int64
	CountByStateErr  error
	CountUnprocErr   error
	ListStaleErr     error
}

// NewMockStore creates a MockStore ready for testing.
func NewMockStore() *MockStore {
	return &MockStore{tasks: make(map[int64]*Task)}
}

func (m *MockStore) InsertTask(_ context.Context, arg InsertTaskParams) (Task, error) {
	if m.InsertErr != nil {
		return Task{}, m.InsertErr
	}

	id := m.nextID.Add(1)
	t := Task{
		ID:             id,
		Type:           arg.Type,
		Value:          arg.Value,
		State:          arg.State,
		CreationTime:   arg.CreationTime,
		LastUpdateTime: arg.LastUpdateTime,
	}

	m.mu.Lock()
	m.tasks[id] = &t
	m.mu.Unlock()

	return t, nil
}

func (m *MockStore) UpdateTaskState(_ context.Context, arg UpdateTaskStateParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.updateCallCount++
	if m.UpdateErrOnCall > 0 && m.updateCallCount == m.UpdateErrOnCall {
		return m.UpdateErr
	}
	if m.UpdateErrOnCall == 0 && m.UpdateErr != nil {
		return m.UpdateErr
	}

	if t, ok := m.tasks[arg.ID]; ok {
		t.State = arg.State
		t.LastUpdateTime = arg.LastUpdateTime
	}
	return nil
}

func (m *MockStore) GetTask(_ context.Context, id int64) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if t, ok := m.tasks[id]; ok {
		return *t, nil
	}
	return Task{}, nil
}

func (m *MockStore) CountTasksByState(_ context.Context, state string) (int64, error) {
	if m.CountByStateErr != nil {
		return 0, m.CountByStateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	for _, t := range m.tasks {
		if t.State == state {
			count++
		}
	}
	return count, nil
}

func (m *MockStore) CountUnprocessed(_ context.Context) (int64, error) {
	if m.CountUnprocErr != nil {
		return 0, m.CountUnprocErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	for _, t := range m.tasks {
		if t.State != string(models.TaskStateDone) {
			count++
		}
	}
	return count, nil
}

func (m *MockStore) SumValueByType(_ context.Context, type_ int32) (int64, error) {
	if m.SumValueErr != nil {
		return 0, m.SumValueErr
	}
	if m.SumValueResult != 0 {
		return m.SumValueResult, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var sum int64
	for _, t := range m.tasks {
		if t.Type == type_ && t.State == string(models.TaskStateDone) {
			sum += int64(t.Value)
		}
	}
	return sum, nil
}

func (m *MockStore) CountDoneByType(_ context.Context, type_ int32) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	for _, t := range m.tasks {
		if t.Type == type_ && t.State == string(models.TaskStateDone) {
			count++
		}
	}
	return count, nil
}

func (m *MockStore) ListStaleTasks(_ context.Context, limit int32) ([]Task, error) {
	if m.ListStaleErr != nil {
		return nil, m.ListStaleErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var results []Task
	for _, t := range m.tasks {
		if t.State == string(models.TaskStateStale) {
			results = append(results, *t)
			if int32(len(results)) >= limit {
				break
			}
		}
	}
	return results, nil
}

// GetAll returns all tasks (test helper).
func (m *MockStore) GetAll() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, *t)
	}
	return tasks
}
