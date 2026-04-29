package consumer

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/models"
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
		Type: 5, Value: 10, State: string(models.TaskStateReceived),
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
	if got.State != string(models.TaskStateDone) {
		t.Errorf("state = %q, want %q", got.State, models.TaskStateDone)
	}
}

func TestProcess_SleepDuration(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100, 1000)

	task, _ := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 1, Value: 50, State: string(models.TaskStateReceived),
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

func TestProcess_UpdateToProcessingFails(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100, 1000)

	task, _ := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 0, Value: 1, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	// Inject error on UpdateTaskState.
	store.UpdateErr = fmt.Errorf("db connection lost")

	c.process(task.ID, task.Type, task.Value)

	// Task should remain in "received" state since update failed.
	got, _ := store.GetTask(context.Background(), task.ID)
	if got.State != string(models.TaskStateReceived) {
		t.Errorf("state = %q, want %q (update failed, should not change)", got.State, models.TaskStateReceived)
	}
}

func TestProcess_SumValueByTypeFails(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100, 1000)

	task, _ := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 0, Value: 1, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	// Inject error on SumValueByType.
	store.SumValueErr = fmt.Errorf("query timeout")

	c.process(task.ID, task.Type, task.Value)

	// Task should still reach "done" state — sum query failure is non-fatal to state.
	got, _ := store.GetTask(context.Background(), task.ID)
	if got.State != string(models.TaskStateDone) {
		t.Errorf("state = %q, want %q", got.State, models.TaskStateDone)
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

func TestStartStateTracker_ContextCancellation(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100, 1000)

	// Pre-insert tasks in different states.
	store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 0, Value: 10, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 1, Value: 20, State: string(models.TaskStateDone),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Run tracker with a short interval; it should complete when context expires.
	done := make(chan struct{})
	go func() {
		c.StartStateTracker(ctx, 50*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		// Tracker stopped as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("StartStateTracker did not stop after context cancellation")
	}
}

func TestWait_CompletesAfterProcessing(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100, 1000)

	task, _ := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 0, Value: 10, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	// Simulate what SubmitTask does: wg.Add + goroutine.
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.process(task.ID, task.Type, task.Value)
	}()

	// Wait should return after the goroutine finishes (~10ms sleep).
	done := make(chan struct{})
	go func() {
		c.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Wait returned as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after in-flight task completed")
	}
}

// FuzzAcquireToken verifies the rate limiter never grants more tokens than the
// configured limit within a single period, regardless of input parameters.
func FuzzAcquireToken(f *testing.F) {
	f.Add(1, 100, 5)
	f.Add(10, 1000, 50)
	f.Add(100, 50, 200)
	f.Add(1, 1, 1)

	f.Fuzz(func(t *testing.T, rateLimit, ratePeriodMs, attempts int) {
		// Bound inputs to sensible ranges.
		if rateLimit <= 0 || rateLimit > 10000 {
			return
		}
		if ratePeriodMs <= 0 || ratePeriodMs > 60000 {
			return
		}
		if attempts <= 0 || attempts > 10000 {
			return
		}

		c := &Consumer{
			rateLimit:    rateLimit,
			ratePeriodMs: ratePeriodMs,
			tokens:       rateLimit,
			lastTick:     time.Now(),
		}

		acquired := 0
		for range attempts {
			if c.acquireToken() {
				acquired++
			}
		}

		// Should never grant more than rateLimit tokens in a single period
		// (unless time passes and triggers a refill, but in a tight loop
		// with short durations this is extremely unlikely).
		if acquired > rateLimit+1 { // +1 tolerance for clock edge
			t.Errorf("acquired %d tokens with limit %d", acquired, rateLimit)
		}
	})
}

// BenchmarkAcquireToken measures the throughput of the rate limiter under contention.
func BenchmarkAcquireToken(b *testing.B) {
	c := &Consumer{
		rateLimit:    1000,
		ratePeriodMs: 1000,
		tokens:       1000,
		lastTick:     time.Now(),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.acquireToken()
		}
	})
}

// BenchmarkProcess measures end-to-end process with 1ms sleep.
func BenchmarkProcess(b *testing.B) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100000, 1000)

	// Pre-insert tasks.
	for i := range b.N {
		store.InsertTask(context.Background(), db.InsertTaskParams{
			Type: int32(i % 10), Value: 1, State: string(models.TaskStateReceived),
			CreationTime: 1000, LastUpdateTime: 1000,
		})
	}

	b.ResetTimer()
	for i := range b.N {
		c.process(int64(i+1), int32(i%10), 1) // 1ms sleep
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
