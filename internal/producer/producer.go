package producer

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/metrics"
	"github.com/erfanmomeniii/task-pipeline/internal/models"
	pb "github.com/erfanmomeniii/task-pipeline/proto"
)

// Producer generates tasks and sends them to the consumer via gRPC.
type Producer struct {
	store         db.TaskStore
	client        pb.TaskServiceClient
	log           *slog.Logger
	ratePerSecond int
	maxBacklog    int

	// retryInterval controls how often stale tasks are re-submitted.
	// Defaults to 10s; tests may override via struct literal.
	retryInterval time.Duration
}

// New creates a Producer that sends tasks to the consumer via the given gRPC client.
// The caller owns the gRPC connection lifecycle.
func New(client pb.TaskServiceClient, store db.TaskStore, ratePerSecond, maxBacklog int, log *slog.Logger) *Producer {
	return &Producer{
		store:         store,
		client:        client,
		log:           log,
		ratePerSecond: ratePerSecond,
		maxBacklog:    maxBacklog,
		retryInterval: 10 * time.Second,
	}
}

// Run starts the production loop and stale task recovery. It blocks until ctx is canceled.
func (p *Producer) Run(ctx context.Context) error {
	interval := time.Second / time.Duration(p.ratePerSecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Recover stale tasks periodically.
	if p.retryInterval <= 0 {
		p.retryInterval = 10 * time.Second
	}
	retryTicker := time.NewTicker(p.retryInterval)
	defer retryTicker.Stop()

	p.log.Info("producer loop started",
		"rate_per_second", p.ratePerSecond,
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
		case <-retryTicker.C:
			p.retryStale(ctx)
		}
	}
}

// retryStale re-submits tasks explicitly marked as "stale".
// This handles tasks rejected by the consumer's rate limiter or lost due to transient failures.
func (p *Producer) retryStale(ctx context.Context) {
	stale, err := p.store.ListStaleTasks(ctx, 50)
	if err != nil {
		p.log.Error("list stale tasks failed", "error", err)
		return
	}

	for _, t := range stale {
		resp, err := p.client.SubmitTask(ctx, &pb.TaskRequest{
			Id:    t.ID,
			Type:  t.Type,
			Value: t.Value,
		})
		if err != nil {
			p.log.Warn("retry: consumer unavailable", "id", t.ID, "error", err)
			return // consumer down, stop retrying this batch
		}
		if resp.Accepted {
			// Reset to "received" so the consumer can process it normally.
			if uerr := p.store.UpdateTaskState(ctx, db.UpdateTaskStateParams{
				ID: t.ID, State: string(models.TaskStateReceived), LastUpdateTime: float64(time.Now().UnixMilli()) / 1000.0,
			}); uerr != nil {
				p.log.Error("failed to reset stale task to received", "id", t.ID, "error", uerr)
			}
		}
		p.log.Info("retry: re-submitted stale task", "id", t.ID, "accepted", resp.Accepted)
	}
}

func (p *Producer) produce(ctx context.Context) error {
	// Check backlog before producing.
	unprocessed, err := p.store.CountUnprocessed(ctx)
	if err != nil {
		return fmt.Errorf("count unprocessed: %w", err)
	}
	metrics.BacklogGauge.Set(float64(unprocessed))

	if int(unprocessed) >= p.maxBacklog {
		p.log.Warn("backlog limit reached, production paused", "unprocessed", unprocessed, "max", p.maxBacklog)
		return nil
	}

	// Generate random task.
	taskType := int32(rand.IntN(10))
	taskValue := int32(rand.IntN(100))
	now := float64(time.Now().UnixMilli()) / 1000.0

	// Persist task with "received" state.
	row, err := p.store.InsertTask(ctx, db.InsertTaskParams{
		Type:           taskType,
		Value:          taskValue,
		State:          string(models.TaskStateReceived),
		CreationTime:   now,
		LastUpdateTime: now,
	})
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	metrics.TasksProduced.Inc()

	// Send to consumer via gRPC (include DB ID so consumer updates same row).
	resp, err := p.client.SubmitTask(ctx, &pb.TaskRequest{
		Id:    row.ID,
		Type:  taskType,
		Value: taskValue,
	})
	if err != nil {
		// Mark as stale so retryStale can find it explicitly.
		if uerr := p.store.UpdateTaskState(ctx, db.UpdateTaskStateParams{
			ID: row.ID, State: string(models.TaskStateStale), LastUpdateTime: now,
		}); uerr != nil {
			p.log.Error("failed to mark task stale", "id", row.ID, "error", uerr)
		}
		p.log.Warn("consumer unavailable, task marked stale",
			"id", row.ID,
			"error", err,
		)
		return nil
	}

	if !resp.Accepted {
		// Consumer rejected (e.g. rate limit) — mark stale for retry.
		if uerr := p.store.UpdateTaskState(ctx, db.UpdateTaskStateParams{
			ID: row.ID, State: string(models.TaskStateStale), LastUpdateTime: float64(time.Now().UnixMilli()) / 1000.0,
		}); uerr != nil {
			p.log.Error("failed to mark task stale", "id", row.ID, "error", uerr)
		}
		p.log.Info("task rejected by consumer, marked stale",
			"id", row.ID,
			"type", taskType,
			"value", taskValue,
		)
		return nil
	}

	p.log.Info("task produced",
		"id", row.ID,
		"type", taskType,
		"value", taskValue,
	)

	return nil
}
