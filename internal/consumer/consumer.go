package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/metrics"
	pb "github.com/erfanmomeniii/task-pipeline/proto"
)

// Consumer implements the gRPC TaskService and processes tasks.
type Consumer struct {
	pb.UnimplementedTaskServiceServer

	queries      *db.Queries
	log          *slog.Logger
	rateLimit    int
	ratePeriodMs int

	// Rate limiter state.
	mu       sync.Mutex
	tokens   int
	lastTick time.Time
}

// New creates a Consumer with rate limiting configuration.
func New(queries *db.Queries, rateLimit, ratePeriodMs int, log *slog.Logger) *Consumer {
	return &Consumer{
		queries:      queries,
		log:          log,
		rateLimit:    rateLimit,
		ratePeriodMs: ratePeriodMs,
		tokens:       rateLimit,
		lastTick:     time.Now(),
	}
}

// SubmitTask handles incoming tasks from the producer.
func (c *Consumer) SubmitTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error) {
	if !c.acquireToken() {
		c.log.Warn("rate limit exceeded, rejecting task",
			"type", req.Type,
			"value", req.Value,
		)
		return &pb.TaskResponse{Id: 0, Accepted: false}, nil
	}

	now := float64(time.Now().UnixMilli()) / 1000.0

	// Insert task with "received" state (or it may already exist from producer).
	// The consumer persists its own record to track processing.
	row, err := c.queries.InsertTask(ctx, db.InsertTaskParams{
		Type:           req.Type,
		Value:          req.Value,
		State:          string(db.TaskStateReceived),
		CreationTime:   now,
		LastUpdateTime: now,
	})
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}

	metrics.TasksReceived.Inc()

	// Process asynchronously so gRPC response returns quickly.
	go c.process(row.ID, req.Type, req.Value)

	return &pb.TaskResponse{Id: row.ID, Accepted: true}, nil
}

func (c *Consumer) process(id int64, taskType, taskValue int32) {
	ctx := context.Background()
	now := func() float64 {
		return float64(time.Now().UnixMilli()) / 1000.0
	}

	// Set state to "processing".
	if err := c.queries.UpdateTaskState(ctx, db.UpdateTaskStateParams{
		ID:             id,
		State:          string(db.TaskStateProcessing),
		LastUpdateTime: now(),
	}); err != nil {
		c.log.Error("update to processing failed", "id", id, "error", err)
		return
	}

	// Process: sleep for value milliseconds.
	start := time.Now()
	time.Sleep(time.Duration(taskValue) * time.Millisecond)
	duration := time.Since(start).Seconds()

	// Set state to "done".
	if err := c.queries.UpdateTaskState(ctx, db.UpdateTaskStateParams{
		ID:             id,
		State:          string(db.TaskStateDone),
		LastUpdateTime: now(),
	}); err != nil {
		c.log.Error("update to done failed", "id", id, "error", err)
		return
	}

	// Update metrics.
	typeStr := strconv.Itoa(int(taskType))
	metrics.TasksDone.Inc()
	metrics.TasksPerType.WithLabelValues(typeStr).Inc()
	metrics.ValueSumPerType.WithLabelValues(typeStr).Add(float64(taskValue))
	metrics.ProcessingDuration.Observe(duration)

	// Fetch total sum for this type (spec: final log per task).
	totalSum, err := c.queries.SumValueByType(ctx, taskType)
	if err != nil {
		c.log.Error("sum value by type failed", "type", taskType, "error", err)
		return
	}

	c.log.Info("task done",
		"id", id,
		"type", taskType,
		"value", taskValue,
		"duration_ms", taskValue,
		"total_sum_for_type", totalSum,
	)
}

// acquireToken implements a simple token bucket rate limiter.
func (c *Consumer) acquireToken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.lastTick)
	period := time.Duration(c.ratePeriodMs) * time.Millisecond

	if elapsed >= period {
		// Refill tokens for elapsed periods (don't accumulate beyond limit).
		c.tokens = c.rateLimit
		c.lastTick = time.Now()
	}

	if c.tokens > 0 {
		c.tokens--
		return true
	}

	return false
}
