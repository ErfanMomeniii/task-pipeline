package metrics

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestProducerMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(TasksProduced, BacklogGauge)

	TasksProduced.Inc()
	TasksProduced.Inc()
	BacklogGauge.Set(5)

	if got := testutil.ToFloat64(TasksProduced); got != 2 {
		t.Errorf("TasksProduced = %v, want 2", got)
	}
	if got := testutil.ToFloat64(BacklogGauge); got != 5 {
		t.Errorf("BacklogGauge = %v, want 5", got)
	}
}

func TestConsumerMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(TasksReceived, TasksDone, TasksPerType, ValueSumPerType, ProcessingDuration)

	TasksReceived.Inc()
	TasksDone.Inc()
	TasksPerType.WithLabelValues("3").Inc()
	TasksPerType.WithLabelValues("3").Inc()
	ValueSumPerType.WithLabelValues("3").Add(42)

	if got := testutil.ToFloat64(TasksReceived); got != 1 {
		t.Errorf("TasksReceived = %v, want 1", got)
	}
	if got := testutil.ToFloat64(TasksDone); got != 1 {
		t.Errorf("TasksDone = %v, want 1", got)
	}
	if got := testutil.ToFloat64(TasksPerType.WithLabelValues("3")); got != 2 {
		t.Errorf("TasksPerType[3] = %v, want 2", got)
	}
	if got := testutil.ToFloat64(ValueSumPerType.WithLabelValues("3")); got != 42 {
		t.Errorf("ValueSumPerType[3] = %v, want 42", got)
	}

	_ = reg
}

func TestServe(t *testing.T) {
	// Find a free port.
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	// Start metrics server in background.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = Serve(port, log)
	}()

	// Wait briefly for server to start, then check /metrics endpoint.
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	_ = ctx
}
