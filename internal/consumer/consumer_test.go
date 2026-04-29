package consumer

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/erfanmomeniii/task-pipeline/internal/db"
	pb "github.com/erfanmomeniii/task-pipeline/proto"
)

func newTestConsumer(store db.TaskStore, rateLimit, ratePeriodMs int) *Consumer {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	return New(store, rateLimit, ratePeriodMs, log)
}

func TestSubmitTask_Accepted(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 10, 1000)

	resp, err := c.SubmitTask(context.Background(), &pb.TaskRequest{
		Id: 1, Type: 3, Value: 50,
	})
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}
	if !resp.Accepted {
		t.Error("expected task to be accepted")
	}
}

func TestSubmitTask_RateLimited(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 2, 10000) // 2 tokens, long period

	// Exhaust tokens.
	for range 2 {
		resp, err := c.SubmitTask(context.Background(), &pb.TaskRequest{
			Id: 1, Type: 0, Value: 1,
		})
		if err != nil {
			t.Fatalf("SubmitTask: %v", err)
		}
		if !resp.Accepted {
			t.Error("expected task to be accepted")
		}
	}

	// 3rd should be rejected.
	resp, err := c.SubmitTask(context.Background(), &pb.TaskRequest{
		Id: 3, Type: 0, Value: 1,
	})
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}
	if resp.Accepted {
		t.Error("expected task to be rejected (rate limited)")
	}
}

func TestProcess_StateTransitions(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100, 1000)

	// Pre-insert a task (simulates what producer does).
	task, err := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 5, Value: 10, State: string(db.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Process synchronously for testing.
	c.process(task.ID, task.Type, task.Value)

	// Verify final state is "done".
	got, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != string(db.TaskStateDone) {
		t.Errorf("state = %q, want %q", got.State, db.TaskStateDone)
	}
}

func TestProcess_SleepDuration(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100, 1000)

	task, _ := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 1, Value: 50, State: string(db.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	start := time.Now()
	c.process(task.ID, task.Type, task.Value)
	elapsed := time.Since(start)

	// Should have slept ~50ms.
	if elapsed < 40*time.Millisecond {
		t.Errorf("process took %v, expected >= 40ms", elapsed)
	}
}

func TestAcquireToken_WithinLimit(t *testing.T) {
	c := &Consumer{
		rateLimit:    3,
		ratePeriodMs: 1000,
		tokens:       3,
		lastTick:     time.Now(),
	}

	for i := range 3 {
		if !c.acquireToken() {
			t.Errorf("token %d should be acquired", i+1)
		}
	}

	if c.acquireToken() {
		t.Error("4th token should not be acquired within same period")
	}
}

func TestAcquireToken_Refill(t *testing.T) {
	c := &Consumer{
		rateLimit:    2,
		ratePeriodMs: 50,
		tokens:       0,
		lastTick:     time.Now().Add(-100 * time.Millisecond),
	}

	if !c.acquireToken() {
		t.Error("token should be acquired after period elapsed")
	}
	if !c.acquireToken() {
		t.Error("second token should be acquired after refill")
	}
	if c.acquireToken() {
		t.Error("third token should not be acquired (limit is 2)")
	}
}

func TestAcquireToken_Concurrent(t *testing.T) {
	c := &Consumer{
		rateLimit:    10,
		ratePeriodMs: 10000,
		tokens:       10,
		lastTick:     time.Now(),
	}

	var acquired int64
	var mu sync.Mutex
	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.acquireToken() {
				mu.Lock()
				acquired++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if acquired != 10 {
		t.Errorf("acquired %d tokens, want exactly 10", acquired)
	}
}

func TestNew(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := New(db.NewMockStore(), 5, 2000, log)

	if c.rateLimit != 5 {
		t.Errorf("rateLimit = %d, want 5", c.rateLimit)
	}
	if c.ratePeriodMs != 2000 {
		t.Errorf("ratePeriodMs = %d, want 2000", c.ratePeriodMs)
	}
	if c.tokens != 5 {
		t.Errorf("initial tokens = %d, want 5", c.tokens)
	}
}
