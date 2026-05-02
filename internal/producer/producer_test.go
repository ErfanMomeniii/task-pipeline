package producer

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/models"
	pb "github.com/erfanmomeniii/task-pipeline/proto"
)

func TestTaskValueRanges(t *testing.T) {
	for range 10000 {
		taskType := int32(rand.IntN(10))
		taskValue := int32(rand.IntN(100))

		if taskType < 0 || taskType > 9 {
			t.Errorf("task type %d out of range [0,9]", taskType)
		}
		if taskValue < 0 || taskValue > 99 {
			t.Errorf("task value %d out of range [0,99]", taskValue)
		}
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := &Producer{
		log:           log,
		ratePerSecond: 1,
		maxBacklog:    100,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := p.Run(ctx)
	if err != nil {
		t.Errorf("Run() returned %v, want nil on context cancellation", err)
	}
}

func TestProduce_InsertsAndSends(t *testing.T) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := &Producer{
		store:         store,
		client:        mock,
		log:           log,
		ratePerSecond: 10,
		maxBacklog:    100,
	}

	err := p.produce(context.Background())
	if err != nil {
		t.Fatalf("produce: %v", err)
	}

	// Should have inserted one task.
	tasks := store.GetAll()
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}

	task := tasks[0]
	if task.State != string(models.TaskStateReceived) {
		t.Errorf("state = %q, want received", task.State)
	}
	if task.Type < 0 || task.Type > 9 {
		t.Errorf("type = %d, out of range", task.Type)
	}
	if task.Value < 0 || task.Value > 99 {
		t.Errorf("value = %d, out of range", task.Value)
	}

	// Should have called gRPC.
	if mock.calls.Load() != 1 {
		t.Errorf("gRPC calls = %d, want 1", mock.calls.Load())
	}
}

func TestProduce_BacklogLimitReached(t *testing.T) {
	store := db.NewMockStore()
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	// Pre-fill store with 5 unprocessed tasks.
	for range 5 {
		store.InsertTask(context.Background(), db.InsertTaskParams{
			Type: 0, Value: 10, State: string(models.TaskStateReceived),
			CreationTime: 1000, LastUpdateTime: 1000,
		})
	}

	mock := &mockGRPCClient{accepted: true}
	p := &Producer{
		store:         store,
		client:        mock,
		log:           log,
		ratePerSecond: 10,
		maxBacklog:    5, // exactly at limit
	}

	err := p.produce(context.Background())
	if err != nil {
		t.Fatalf("produce: %v", err)
	}

	// Should NOT have produced a new task.
	if mock.calls.Load() != 0 {
		t.Errorf("gRPC calls = %d, want 0 (backlog full)", mock.calls.Load())
	}

	// Store should still have only 5 tasks.
	if len(store.GetAll()) != 5 {
		t.Errorf("tasks = %d, want 5", len(store.GetAll()))
	}
}

func TestProduce_GRPCFailure_TaskMarkedStale(t *testing.T) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{err: context.DeadlineExceeded}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := &Producer{
		store:         store,
		client:        mock,
		log:           log,
		ratePerSecond: 10,
		maxBacklog:    100,
	}

	// Should NOT return error — task is persisted, gRPC failure is logged.
	err := p.produce(context.Background())
	if err != nil {
		t.Fatalf("produce should not error on gRPC failure, got: %v", err)
	}

	// Task should be marked as "stale" after gRPC failure.
	tasks := store.GetAll()
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].State != string(models.TaskStateStale) {
		t.Errorf("state = %q, want stale", tasks[0].State)
	}
}

func TestProduce_ConsumerRejects_TaskMarkedStale(t *testing.T) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{accepted: false} // consumer rate-limits
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := &Producer{
		store:         store,
		client:        mock,
		log:           log,
		ratePerSecond: 10,
		maxBacklog:    100,
	}

	err := p.produce(context.Background())
	if err != nil {
		t.Fatalf("produce should not error when consumer rejects, got: %v", err)
	}

	// Task should be marked as "stale" after rejection.
	tasks := store.GetAll()
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].State != string(models.TaskStateStale) {
		t.Errorf("state = %q, want stale", tasks[0].State)
	}

	// gRPC was called.
	if mock.calls.Load() != 1 {
		t.Errorf("gRPC calls = %d, want 1", mock.calls.Load())
	}
}

func TestNew_CreatesProducer(t *testing.T) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := New(mock, store, 5, 50, log)
	if p == nil {
		t.Fatal("New() returned nil")
	}
	if p.ratePerSecond != 5 {
		t.Errorf("ratePerSecond = %d, want 5", p.ratePerSecond)
	}
	if p.maxBacklog != 50 {
		t.Errorf("maxBacklog = %d, want 50", p.maxBacklog)
	}
}

func TestProduce_InsertTaskFails(t *testing.T) {
	store := db.NewMockStore()
	store.InsertErr = fmt.Errorf("db write failed")
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := &Producer{
		store:         store,
		client:        mock,
		log:           log,
		ratePerSecond: 10,
		maxBacklog:    100,
	}

	err := p.produce(context.Background())
	if err == nil {
		t.Fatal("produce should return error when InsertTask fails")
	}
	if mock.calls.Load() != 0 {
		t.Errorf("gRPC calls = %d, want 0 (should not send after insert failure)", mock.calls.Load())
	}
}

func TestProduce_CountUnprocessedFails(t *testing.T) {
	store := db.NewMockStore()
	store.CountUnprocErr = fmt.Errorf("db read failed")
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := &Producer{
		store:         store,
		client:        mock,
		log:           log,
		ratePerSecond: 10,
		maxBacklog:    100,
	}

	err := p.produce(context.Background())
	if err == nil {
		t.Fatal("produce should return error when CountUnprocessed fails")
	}
}

func TestRun_RetryTickerFires(t *testing.T) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	// Insert stale task.
	now := models.NowUnixSec()
	store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 1, Value: 10, State: string(models.TaskStateStale),
		CreationTime: now, LastUpdateTime: now,
	})

	p := &Producer{
		store:         store,
		client:        mock,
		log:           log,
		ratePerSecond: 1,
		maxBacklog:    100,
		retryInterval: 20 * time.Millisecond, // fast retry for test
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	_ = p.Run(ctx)

	// retryStale should have fired and re-submitted the stale task.
	if mock.calls.Load() < 1 {
		t.Errorf("gRPC calls = %d, want >= 1 (retryStale should have fired)", mock.calls.Load())
	}
}

func TestRun_ProduceError(t *testing.T) {
	store := db.NewMockStore()
	store.CountUnprocErr = fmt.Errorf("db error")
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := &Producer{
		store:         store,
		client:        &mockGRPCClient{accepted: true},
		log:           log,
		ratePerSecond: 100, // fast ticker to trigger produce quickly
		maxBacklog:    100,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := p.Run(ctx)
	if err != nil {
		t.Errorf("Run() returned %v, want nil on context cancellation", err)
	}
}

func TestRetryStale_ListStaleError(t *testing.T) {
	store := db.NewMockStore()
	store.ListStaleErr = fmt.Errorf("db query failed")
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := &Producer{
		store:  store,
		client: mock,
		log:    log,
	}

	// Should not panic, just log the error.
	p.retryStale(context.Background())

	if mock.calls.Load() != 0 {
		t.Errorf("gRPC calls = %d, want 0", mock.calls.Load())
	}
}

func TestRetryStale_SubmitTaskError(t *testing.T) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{err: fmt.Errorf("connection refused")}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	now := models.NowUnixSec()
	store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 1, Value: 10, State: string(models.TaskStateStale),
		CreationTime: now, LastUpdateTime: now,
	})

	p := &Producer{
		store:  store,
		client: mock,
		log:    log,
	}

	// Should not panic — logs warning and stops batch.
	p.retryStale(context.Background())

	if mock.calls.Load() != 1 {
		t.Errorf("gRPC calls = %d, want 1 (attempted then stopped)", mock.calls.Load())
	}
}

func TestRetryStale_ResubmitsStaleTasks(t *testing.T) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	// Insert a task with "stale" state.
	now := models.NowUnixSec()
	store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 3, Value: 42, State: string(models.TaskStateStale),
		CreationTime: now, LastUpdateTime: now,
	})

	p := &Producer{
		store:  store,
		client: mock,
		log:    log,
	}

	p.retryStale(context.Background())

	if mock.calls.Load() != 1 {
		t.Errorf("gRPC calls = %d, want 1 (stale task re-submitted)", mock.calls.Load())
	}
}

func TestRetryStale_IgnoresReceivedTasks(t *testing.T) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	// Insert a task in "received" state — should NOT be retried (only "stale" tasks are retried).
	now := models.NowUnixSec()
	store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 3, Value: 42, State: string(models.TaskStateReceived),
		CreationTime: now, LastUpdateTime: now,
	})

	p := &Producer{
		store:  store,
		client: mock,
		log:    log,
	}

	p.retryStale(context.Background())

	if mock.calls.Load() != 0 {
		t.Errorf("gRPC calls = %d, want 0 (received tasks should not be retried)", mock.calls.Load())
	}
}

func TestRetryStale_ResetsToReceived(t *testing.T) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	now := models.NowUnixSec()
	task, _ := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 3, Value: 42, State: string(models.TaskStateStale),
		CreationTime: now, LastUpdateTime: now,
	})

	p := &Producer{
		store:  store,
		client: mock,
		log:    log,
	}

	p.retryStale(context.Background())

	// Task should be reset to "received" after successful retry.
	got, _ := store.GetTask(context.Background(), task.ID)
	if got.State != string(models.TaskStateReceived) {
		t.Errorf("state = %q, want received after successful retry", got.State)
	}
}

func BenchmarkProduce(b *testing.B) {
	store := db.NewMockStore()
	mock := &mockGRPCClient{accepted: true}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	p := &Producer{
		store:         store,
		client:        mock,
		log:           log,
		ratePerSecond: 10,
		maxBacklog:    b.N + 1,
	}

	b.ResetTimer()
	for range b.N {
		if err := p.produce(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

// mockGRPCClient implements pb.TaskServiceClient for testing.
// calls uses atomic.Int32 for thread safety in concurrent tests.
type mockGRPCClient struct {
	accepted bool
	err      error
	calls    atomic.Int32
}

func (m *mockGRPCClient) SubmitTask(_ context.Context, _ *pb.TaskRequest, _ ...grpc.CallOption) (*pb.TaskResponse, error) {
	m.calls.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	return &pb.TaskResponse{Accepted: m.accepted}, nil
}
