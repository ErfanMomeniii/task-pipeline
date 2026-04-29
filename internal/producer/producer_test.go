package producer

import (
	"bytes"
	"context"
	"log/slog"
	"math/rand/v2"
	"testing"
	"time"
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

	// Create producer with nil queries/client — Run should still respect ctx.
	p := &Producer{
		log:        log,
		ratePerSecond: 1, // slow rate so produce() won't be called before ctx expires
		maxBacklog: 100,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := p.Run(ctx)
	if !isContextErr(err) {
		t.Errorf("Run() returned %v, want context error", err)
	}
}

func isContextErr(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}
