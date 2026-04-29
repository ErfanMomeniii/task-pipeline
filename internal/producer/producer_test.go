package producer

import (
	"bytes"
	"context"
	"log/slog"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/erfanmomeniii/task-pipeline/internal/db"
	pb "github.com/erfanmomeniii/task-pipeline/proto"
	"google.golang.org/grpc"
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
	if !isContextErr(err) {
		t.Errorf("Run() returned %v, want context error", err)
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
	if task.State != string(db.TaskStateReceived) {
		t.Errorf("state = %q, want received", task.State)
	}
	if task.Type < 0 || task.Type > 9 {
		t.Errorf("type = %d, out of range", task.Type)
	}
	if task.Value < 0 || task.Value > 99 {
		t.Errorf("value = %d, out of range", task.Value)
	}

	// Should have called gRPC.
	if mock.calls != 1 {
		t.Errorf("gRPC calls = %d, want 1", mock.calls)
	}
}

func TestProduce_BacklogLimitReached(t *testing.T) {
	store := db.NewMockStore()
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	// Pre-fill store with 5 unprocessed tasks.
	for range 5 {
		store.InsertTask(context.Background(), db.InsertTaskParams{
			Type: 0, Value: 10, State: string(db.TaskStateReceived),
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
	if mock.calls != 0 {
		t.Errorf("gRPC calls = %d, want 0 (backlog full)", mock.calls)
	}

	// Store should still have only 5 tasks.
	if len(store.GetAll()) != 5 {
		t.Errorf("tasks = %d, want 5", len(store.GetAll()))
	}
}

func TestProduce_GRPCFailure_TaskStillPersisted(t *testing.T) {
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

	// Task should still be inserted in DB with "received" state.
	tasks := store.GetAll()
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].State != string(db.TaskStateReceived) {
		t.Errorf("state = %q, want received", tasks[0].State)
	}
}

// mockGRPCClient implements pb.TaskServiceClient for testing.
type mockGRPCClient struct {
	accepted bool
	err      error
	calls    int
}

func (m *mockGRPCClient) SubmitTask(_ context.Context, _ *pb.TaskRequest, _ ...grpc.CallOption) (*pb.TaskResponse, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &pb.TaskResponse{Accepted: m.accepted}, nil
}

func isContextErr(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}
