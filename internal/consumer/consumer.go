package consumer

import (
	"context"
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

	store        db.TaskStore
	log          *slog.Logger
	rateLimit    int
	ratePeriodMs int

	// Rate limiter state.
	mu       sync.Mutex
	tokens   int
	lastTick time.Time

	// Tracks in-flight process goroutines for graceful shutdown.
	wg sync.WaitGroup
}

// New creates a Consumer with rate limiting configuration.
func New(store db.TaskStore, rateLimit, ratePeriodMs int, log *slog.Logger) *Consumer {
	return &Consumer{
		store:        store,
		log:          log,
		rateLimit:    rateLimit,
		ratePeriodMs: ratePeriodMs,
		tokens:       rateLimit,
		lastTick:     time.Now(),
	}
}

// SubmitTask handles incoming tasks from the producer.
// The producer already persisted the task in DB with "received" state;
// the consumer uses the provided ID to update the same row.
func (c *Consumer) SubmitTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error) {
	if !c.acquireToken() {
		c.log.Warn("rate limit exceeded, rejecting task",
			"id", req.Id,
			"type", req.Type,
			"value", req.Value,
		)
		return &pb.TaskResponse{Accepted: false}, nil
	}

	metrics.TasksReceived.Inc()

	// Process asynchronously so gRPC response returns quickly.
	// WaitGroup ensures graceful shutdown waits for in-flight tasks.
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.process(req.Id, req.Type, req.Value)
	}()

	return &pb.TaskResponse{Accepted: true}, nil
}

func (c *Consumer) process(id int64, taskType, taskValue int32) {
	ctx := context.Background()
	now := func() float64 {
		return float64(time.Now().UnixMilli()) / 1000.0
	}

	// Set state to "processing".
	if err := c.store.UpdateTaskState(ctx, db.UpdateTaskStateParams{
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
	if err := c.store.UpdateTaskState(ctx, db.UpdateTaskStateParams{
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
	totalSum, err := c.store.SumValueByType(ctx, taskType)
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

// Wait blocks until all in-flight process goroutines complete.
// Call this during graceful shutdown after stopping the gRPC server.
func (c *Consumer) Wait() {
	c.wg.Wait()
}

// StartStateTracker periodically queries the DB for task state counts
// and updates the tasks_by_state Prometheus gauge. Runs until ctx is cancelled.
func (c *Consumer) StartStateTracker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, state := range []string{
				string(db.TaskStateReceived),
				string(db.TaskStateProcessing),
				string(db.TaskStateDone),
			} {
				count, err := c.store.CountTasksByState(ctx, state)
				if err != nil {
					c.log.Error("count tasks by state failed", "state", state, "error", err)
					continue
				}
				metrics.TasksByState.WithLabelValues(state).Set(float64(count))
			}
		}
	}
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
