package consumer

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"
	"time"
)

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
		ratePeriodMs: 10000, // long period so no refill during test
		tokens:       10,
		lastTick:     time.Now(),
	}

	var acquired int64
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Launch 50 goroutines all trying to acquire tokens.
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

func TestNew(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := New(nil, 5, 2000, log)

	if c.rateLimit != 5 {
		t.Errorf("rateLimit = %d, want 5", c.rateLimit)
	}
	if c.ratePeriodMs != 2000 {
		t.Errorf("ratePeriodMs = %d, want 2000", c.ratePeriodMs)
	}
	if c.tokens != 5 {
		t.Errorf("initial tokens = %d, want 5", c.tokens)
	}
}
