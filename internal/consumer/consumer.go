package consumer

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/metrics"
	"github.com/erfanmomeniii/task-pipeline/internal/models"
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

	// Bounded worker pool: limits concurrent processing goroutines.
	// Uses a buffered channel as a semaphore (stdlib, no external deps).
	sem chan struct{}

	// Tracks in-flight process goroutines for graceful shutdown.
	wg sync.WaitGroup
}

// New creates a Consumer with rate limiting and bounded concurrency.
// maxWorkers controls the maximum number of concurrent processing goroutines.
func New(store db.TaskStore, rateLimit, ratePeriodMs, maxWorkers int, log *slog.Logger) *Consumer {
	if maxWorkers <= 0 {
		maxWorkers = rateLimit // sensible default: match rate limit
	}
	if ratePeriodMs <= 0 {
		ratePeriodMs = 1000 // default to 1 second
	}
	return &Consumer{
		store:        store,
		log:          log,
		rateLimit:    rateLimit,
		ratePeriodMs: ratePeriodMs,
		tokens:       rateLimit,
		lastTick:     time.Now(),
		sem:          make(chan struct{}, maxWorkers),
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
	// Semaphore bounds concurrency; WaitGroup ensures graceful shutdown.
	// Use context.WithoutCancel to prevent gRPC request context cancellation
	// from aborting DB calls in the background goroutine.
	asyncCtx := context.WithoutCancel(ctx)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.sem <- struct{}{}        // acquire worker slot
		defer func() { <-c.sem }() // release worker slot
		c.process(asyncCtx, req.Id, req.Type, req.Value)
	}()

	return &pb.TaskResponse{Accepted: true}, nil
}

func (c *Consumer) process(ctx context.Context, id int64, taskType, taskValue int32) {
	// Set state to "processing".
	if err := c.store.UpdateTaskState(ctx, db.UpdateTaskStateParams{
		ID:             id,
		State:          string(models.TaskStateProcessing),
		LastUpdateTime: models.NowUnixSec(),
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
		State:          string(models.TaskStateDone),
		LastUpdateTime: models.NowUnixSec(),
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
		"duration_ms", duration*1000,
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
				string(models.TaskStateReceived),
				string(models.TaskStateProcessing),
				string(models.TaskStateDone),
				string(models.TaskStateStale),
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
// Tokens refill proportionally to elapsed time, capped at rateLimit.
func (c *Consumer) acquireToken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(c.lastTick)
	period := time.Duration(c.ratePeriodMs) * time.Millisecond

	if elapsed >= period {
		// Calculate how many full periods elapsed and refill proportionally.
		periods := int(elapsed / period)
		c.tokens += periods * c.rateLimit
		if c.tokens > c.rateLimit {
			c.tokens = c.rateLimit // cap at max
		}
		// Advance lastTick by exact number of periods (avoids drift).
		c.lastTick = c.lastTick.Add(time.Duration(periods) * period)
	}

	if c.tokens > 0 {
		c.tokens--
		return true
	}

	return false
}
