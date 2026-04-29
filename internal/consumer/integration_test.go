package consumer_test

import (
	"context"
	"net"
	"testing"
	"time"

	"bytes"
	"log/slog"

	"github.com/erfanmomeniii/task-pipeline/internal/consumer"
	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/models"
	pb "github.com/erfanmomeniii/task-pipeline/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestGRPCRoundTrip starts an in-process gRPC server with the consumer,
// connects a client, submits a task, and verifies the full flow.
func TestGRPCRoundTrip(t *testing.T) {
	store := db.NewMockStore()
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := consumer.New(store, 10, 1000, 10, log)

	// Start gRPC server on a random port.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterTaskServiceServer(srv, c)

	go srv.Serve(lis)
	defer srv.GracefulStop()

	// Connect a gRPC client.
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewTaskServiceClient(conn)

	// Pre-insert task in DB (simulating what producer does).
	task, err := store.InsertTask(context.Background(), db.InsertTaskParams{
		Type: 3, Value: 10, State: string(models.TaskStateReceived),
		CreationTime: 1000, LastUpdateTime: 1000,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Submit via gRPC.
	resp, err := client.SubmitTask(context.Background(), &pb.TaskRequest{
		Id:    task.ID,
		Type:  task.Type,
		Value: task.Value,
	})
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}
	if !resp.Accepted {
		t.Fatal("expected task to be accepted")
	}

	// Wait for async processing to complete.
	c.Wait()

	// Verify task reached "done" state in DB.
	got, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.State != string(models.TaskStateDone) {
		t.Errorf("state = %q, want %q", got.State, models.TaskStateDone)
	}
}

// TestGRPCRoundTrip_RateLimited verifies that the consumer rejects tasks
// over gRPC when the rate limit is exceeded.
func TestGRPCRoundTrip_RateLimited(t *testing.T) {
	store := db.NewMockStore()
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := consumer.New(store, 1, 10000, 10, log) // 1 token, long period

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterTaskServiceServer(srv, c)

	go srv.Serve(lis)
	defer func() {
		srv.GracefulStop()
		c.Wait()
	}()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewTaskServiceClient(conn)

	// First task: accepted.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp1, err := client.SubmitTask(ctx, &pb.TaskRequest{Id: 1, Type: 0, Value: 1})
	if err != nil {
		t.Fatalf("SubmitTask 1: %v", err)
	}
	if !resp1.Accepted {
		t.Error("first task should be accepted")
	}

	// Second task: rejected (rate limited).
	resp2, err := client.SubmitTask(ctx, &pb.TaskRequest{Id: 2, Type: 0, Value: 1})
	if err != nil {
		t.Fatalf("SubmitTask 2: %v", err)
	}
	if resp2.Accepted {
		t.Error("second task should be rejected (rate limited)")
	}
}
