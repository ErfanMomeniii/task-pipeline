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
	return New(store, rateLimit, ratePeriodMs, rateLimit, log)
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

	task, err := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 5, Value: 10, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	c.process(context.Background(), task.ID, task.Type, task.Value)

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
	c.process(context.Background(), task.ID, task.Type, task.Value)
	elapsed := time.Since(start)

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

	store.UpdateErr = fmt.Errorf("db connection lost")

	c.process(context.Background(), task.ID, task.Type, task.Value)

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

	store.SumValueErr = fmt.Errorf("query timeout")

	c.process(context.Background(), task.ID, task.Type, task.Value)

	got, _ := store.GetTask(context.Background(), task.ID)
	if got.State != string(models.TaskStateDone) {
		t.Errorf("state = %q, want %q", got.State, models.TaskStateDone)
	}
}

func TestProcess_UpdateToDoneFails(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100, 1000)

	task, _ := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 0, Value: 1, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})

	// Fail on 2nd UpdateTaskState call (update to "done"), 1st (to "processing") succeeds.
	store.UpdateErr = fmt.Errorf("disk full")
	store.UpdateErrOnCall = 2

	c.process(context.Background(), task.ID, task.Type, task.Value)

	got, _ := store.GetTask(context.Background(), task.ID)
	if got.State != string(models.TaskStateProcessing) {
		t.Errorf("state = %q, want %q (done update failed)", got.State, models.TaskStateProcessing)
	}
}

func TestStartStateTracker_CountError(t *testing.T) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100, 1000)

	store.CountByStateErr = fmt.Errorf("db unavailable")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		c.StartStateTracker(ctx, 50*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		// Should complete even with errors (errors are logged and continued).
	case <-time.After(2 * time.Second):
		t.Fatal("StartStateTracker did not stop")
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

	done := make(chan struct{})
	go func() {
		c.StartStateTracker(ctx, 50*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
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

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.process(context.Background(), task.ID, task.Type, task.Value)
	}()

	done := make(chan struct{})
	go func() {
		c.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after in-flight task completed")
	}
}

func TestSemaphore_BoundsConcurrency(t *testing.T) {
	store := db.NewMockStore()
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := New(store, 100, 1000, 2, log) // maxWorkers=2

	// Insert 5 tasks with 50ms sleep each.
	for i := range 5 {
		store.InsertTask(context.Background(), db.InsertTaskParams{
			Type: int32(i % 10), Value: 50, State: string(models.TaskStateReceived),
			CreationTime: 1000, LastUpdateTime: 1000,
		})
	}

	// Submit all 5 tasks.
	for i := range 5 {
		c.SubmitTask(context.Background(), &pb.TaskRequest{
			Id: int64(i + 1), Type: int32(i % 10), Value: 50,
		})
	}

	// With 2 workers and 5 tasks of 50ms, it should take ~150ms (3 batches).
	// Without the semaphore, it would take ~50ms (all parallel).
	start := time.Now()
	c.Wait()
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("elapsed %v, expected >= 100ms (semaphore should bound concurrency to 2)", elapsed)
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

		if acquired > rateLimit {
			t.Errorf("acquired %d tokens with limit %d", acquired, rateLimit)
		}
	})
}

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

func BenchmarkProcess(b *testing.B) {
	store := db.NewMockStore()
	c := newTestConsumer(store, 100000, 1000)

	for i := range b.N {
		store.InsertTask(context.Background(), db.InsertTaskParams{
			Type: int32(i % 10), Value: 1, State: string(models.TaskStateReceived),
			CreationTime: 1000, LastUpdateTime: 1000,
		})
	}

	b.ResetTimer()
	for i := range b.N {
		c.process(context.Background(), int64(i+1), int32(i%10), 1)
	}
}

func TestNew(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := New(db.NewMockStore(), 5, 2000, 10, log)

	if c.rateLimit != 5 {
		t.Errorf("rateLimit = %d, want 5", c.rateLimit)
	}
	if c.ratePeriodMs != 2000 {
		t.Errorf("ratePeriodMs = %d, want 2000", c.ratePeriodMs)
	}
	if c.tokens != 5 {
		t.Errorf("initial tokens = %d, want 5", c.tokens)
	}
	if cap(c.sem) != 10 {
		t.Errorf("semaphore capacity = %d, want 10", cap(c.sem))
	}
}

func TestNew_DefaultMaxWorkers(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := New(db.NewMockStore(), 5, 2000, 0, log) // maxWorkers=0 → defaults to rateLimit

	if cap(c.sem) != 5 {
		t.Errorf("semaphore capacity = %d, want 5 (default to rateLimit)", cap(c.sem))
	}
}
