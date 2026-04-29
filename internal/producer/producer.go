package producer

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/metrics"
	pb "github.com/erfanmomeniii/task-pipeline/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Producer generates tasks and sends them to the consumer via gRPC.
type Producer struct {
	queries    *db.Queries
	client     pb.TaskServiceClient
	conn       *grpc.ClientConn
	log        *slog.Logger
	rateMs     int
	maxBacklog int
}

// New creates a Producer that connects to the consumer gRPC server.
func New(ctx context.Context, queries *db.Queries, grpcAddr string, rateMs, maxBacklog int, log *slog.Logger) (*Producer, error) {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", grpcAddr, err)
	}

	return &Producer{
		queries:    queries,
		client:     pb.NewTaskServiceClient(conn),
		conn:       conn,
		log:        log,
		rateMs:     rateMs,
		maxBacklog: maxBacklog,
	}, nil
}

// Run starts the production loop. It blocks until ctx is cancelled.
func (p *Producer) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(p.rateMs) * time.Millisecond)
	defer ticker.Stop()

	p.log.Info("producer loop started",
		"rate_ms", p.rateMs,
		"max_backlog", p.maxBacklog,
	)

	for {
		select {
		case <-ctx.Done():
			p.log.Info("producer loop stopped")
			return ctx.Err()
		case <-ticker.C:
			if err := p.produce(ctx); err != nil {
				p.log.Error("produce failed", "error", err)
			}
		}
	}
}

// Close shuts down the gRPC connection.
func (p *Producer) Close() error {
	return p.conn.Close()
}

func (p *Producer) produce(ctx context.Context) error {
	// Check backlog before producing.
	unprocessed, err := p.queries.CountUnprocessed(ctx)
	if err != nil {
		return fmt.Errorf("count unprocessed: %w", err)
	}
	metrics.BacklogGauge.Set(float64(unprocessed))

	if int(unprocessed) >= p.maxBacklog {
		p.log.Debug("backlog limit reached, skipping", "unprocessed", unprocessed, "max", p.maxBacklog)
		return nil
	}

	// Generate random task.
	taskType := int32(rand.IntN(10))
	taskValue := int32(rand.IntN(100))
	now := float64(time.Now().UnixMilli()) / 1000.0

	// Persist task with "received" state.
	row, err := p.queries.InsertTask(ctx, db.InsertTaskParams{
		Type:           taskType,
		Value:          taskValue,
		State:          string(db.TaskStateReceived),
		CreationTime:   now,
		LastUpdateTime: now,
	})
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	// Send to consumer via gRPC (include DB ID so consumer updates same row).
	resp, err := p.client.SubmitTask(ctx, &pb.TaskRequest{
		Id:    row.ID,
		Type:  taskType,
		Value: taskValue,
	})
	if err != nil {
		return fmt.Errorf("submit task via grpc: %w", err)
	}

	metrics.TasksProduced.Inc()

	p.log.Info("task produced",
		"id", row.ID,
		"type", taskType,
		"value", taskValue,
		"accepted", resp.Accepted,
	)

	return nil
}
