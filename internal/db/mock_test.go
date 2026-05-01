package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/erfanmomeniii/task-pipeline/internal/models"
)

func TestMockStore_InsertAndGetTask(t *testing.T) {
	s := NewMockStore()

	task, err := s.InsertTask(context.Background(), InsertTaskParams{
		Type: 3, Value: 42, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	if err != nil {
		t.Fatalf("InsertTask: %v", err)
	}
	if task.ID != 1 {
		t.Errorf("ID = %d, want 1", task.ID)
	}

	got, err := s.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Type != 3 || got.Value != 42 {
		t.Errorf("GetTask returned type=%d value=%d, want 3/42", got.Type, got.Value)
	}
}

func TestMockStore_GetTask_NotFound(t *testing.T) {
	s := NewMockStore()
	got, err := s.GetTask(context.Background(), 999)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ID != 0 {
		t.Errorf("expected zero-value Task for missing ID, got ID=%d", got.ID)
	}
}

func TestMockStore_InsertErr(t *testing.T) {
	s := NewMockStore()
	s.InsertErr = fmt.Errorf("insert failed")

	_, err := s.InsertTask(context.Background(), InsertTaskParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockStore_UpdateTaskState(t *testing.T) {
	s := NewMockStore()
	task, _ := s.InsertTask(context.Background(), InsertTaskParams{
		Type: 0, Value: 10, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	err := s.UpdateTaskState(context.Background(), UpdateTaskStateParams{
		ID: task.ID, State: string(models.TaskStateProcessing), LastUpdateTime: 2000,
	})
	if err != nil {
		t.Fatalf("UpdateTaskState: %v", err)
	}

	got, _ := s.GetTask(context.Background(), task.ID)
	if got.State != string(models.TaskStateProcessing) {
		t.Errorf("state = %q, want processing", got.State)
	}
}

func TestMockStore_UpdateErr(t *testing.T) {
	s := NewMockStore()
	s.UpdateErr = fmt.Errorf("update failed")

	err := s.UpdateTaskState(context.Background(), UpdateTaskStateParams{ID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockStore_UpdateErrOnCall(t *testing.T) {
	s := NewMockStore()
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 0, Value: 1, State: "received",
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	s.UpdateErr = fmt.Errorf("fail on 2nd")
	s.UpdateErrOnCall = 2

	// 1st call succeeds.
	err := s.UpdateTaskState(context.Background(), UpdateTaskStateParams{ID: 1, State: "processing"})
	if err != nil {
		t.Fatalf("1st UpdateTaskState should succeed: %v", err)
	}

	// 2nd call fails.
	err = s.UpdateTaskState(context.Background(), UpdateTaskStateParams{ID: 1, State: "done"})
	if err == nil {
		t.Fatal("2nd UpdateTaskState should fail")
	}
}

func TestMockStore_CountTasksByState(t *testing.T) {
	s := NewMockStore()
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 0, Value: 1, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 1, Value: 2, State: string(models.TaskStateDone),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	count, err := s.CountTasksByState(context.Background(), string(models.TaskStateReceived))
	if err != nil {
		t.Fatalf("CountTasksByState: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestMockStore_CountByStateErr(t *testing.T) {
	s := NewMockStore()
	s.CountByStateErr = fmt.Errorf("db down")

	_, err := s.CountTasksByState(context.Background(), "received")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockStore_CountUnprocessed(t *testing.T) {
	s := NewMockStore()
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 0, Value: 1, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 1, Value: 2, State: string(models.TaskStateDone),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	count, err := s.CountUnprocessed(context.Background())
	if err != nil {
		t.Fatalf("CountUnprocessed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestMockStore_CountUnprocErr(t *testing.T) {
	s := NewMockStore()
	s.CountUnprocErr = fmt.Errorf("db down")

	_, err := s.CountUnprocessed(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockStore_SumValueByType(t *testing.T) {
	s := NewMockStore()
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 3, Value: 10, State: string(models.TaskStateDone),
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 3, Value: 20, State: string(models.TaskStateDone),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	sum, err := s.SumValueByType(context.Background(), 3)
	if err != nil {
		t.Fatalf("SumValueByType: %v", err)
	}
	if sum != 30 {
		t.Errorf("sum = %d, want 30", sum)
	}
}

func TestMockStore_SumValueErr(t *testing.T) {
	s := NewMockStore()
	s.SumValueErr = fmt.Errorf("query failed")

	_, err := s.SumValueByType(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockStore_SumValueResult(t *testing.T) {
	s := NewMockStore()
	s.SumValueResult = 999

	sum, _ := s.SumValueByType(context.Background(), 0)
	if sum != 999 {
		t.Errorf("sum = %d, want 999", sum)
	}
}

func TestMockStore_CountDoneByType(t *testing.T) {
	s := NewMockStore()
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 5, Value: 1, State: string(models.TaskStateDone),
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 5, Value: 2, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	count, err := s.CountDoneByType(context.Background(), 5)
	if err != nil {
		t.Fatalf("CountDoneByType: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestMockStore_ListStaleTasks(t *testing.T) {
	s := NewMockStore()
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 1, Value: 10, State: string(models.TaskStateStale),
		CreationTime: 100, LastUpdateTime: 100,
	})
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 2, Value: 20, State: string(models.TaskStateReceived),
		CreationTime: 9999, LastUpdateTime: 9999,
	})

	results, err := s.ListStaleTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListStaleTasks: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("got %d stale tasks, want 1", len(results))
	}
}

func TestMockStore_ListStaleTasks_LimitReached(t *testing.T) {
	s := NewMockStore()
	for i := 0; i < 5; i++ {
		s.InsertTask(context.Background(), InsertTaskParams{
			Type: 1, Value: int32(i), State: string(models.TaskStateStale),
			CreationTime: 100, LastUpdateTime: 100,
		})
	}

	results, err := s.ListStaleTasks(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListStaleTasks: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2 (limit)", len(results))
	}
}

func TestMockStore_ListStaleErr(t *testing.T) {
	s := NewMockStore()
	s.ListStaleErr = fmt.Errorf("db down")

	_, err := s.ListStaleTasks(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockStore_GetAll(t *testing.T) {
	s := NewMockStore()
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 0, Value: 1, State: "received",
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	s.InsertTask(context.Background(), InsertTaskParams{
		Type: 1, Value: 2, State: "done",
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	all := s.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll = %d tasks, want 2", len(all))
	}
}

func TestNew_ReturnsQueries(t *testing.T) {
	q := New(nil)
	if q == nil {
		t.Fatal("New() returned nil")
	}
}

func TestWithTx_ReturnsNewQueries(t *testing.T) {
	q := New(nil)
	q2 := q.WithTx(nil)
	if q2 == nil {
		t.Fatal("WithTx() returned nil")
	}
}

func TestTaskState_Scan_String(t *testing.T) {
	var ts TaskState
	if err := ts.Scan("received"); err != nil {
		t.Fatalf("Scan string: %v", err)
	}
	if ts != TaskStateReceived {
		t.Errorf("got %q, want %q", ts, TaskStateReceived)
	}
}

func TestTaskState_Scan_Bytes(t *testing.T) {
	var ts TaskState
	if err := ts.Scan([]byte("processing")); err != nil {
		t.Fatalf("Scan bytes: %v", err)
	}
	if ts != TaskStateProcessing {
		t.Errorf("got %q, want %q", ts, TaskStateProcessing)
	}
}

func TestTaskState_Scan_UnsupportedType(t *testing.T) {
	var ts TaskState
	if err := ts.Scan(123); err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestNullTaskState_Scan_Nil(t *testing.T) {
	var ns NullTaskState
	if err := ns.Scan(nil); err != nil {
		t.Fatalf("Scan nil: %v", err)
	}
	if ns.Valid {
		t.Error("expected Valid=false for nil")
	}
}

func TestNullTaskState_Scan_Valid(t *testing.T) {
	var ns NullTaskState
	if err := ns.Scan("done"); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !ns.Valid {
		t.Error("expected Valid=true")
	}
	if ns.TaskState != TaskStateDone {
		t.Errorf("got %q, want %q", ns.TaskState, TaskStateDone)
	}
}

func TestNullTaskState_Value_Invalid(t *testing.T) {
	ns := NullTaskState{Valid: false}
	v, err := ns.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}

func TestNullTaskState_Value_Valid(t *testing.T) {
	ns := NullTaskState{TaskState: TaskStateDone, Valid: true}
	v, err := ns.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if v != "done" {
		t.Errorf("got %v, want %q", v, "done")
	}
}
