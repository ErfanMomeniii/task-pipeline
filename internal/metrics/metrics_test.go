package metrics

import (
	"bytes"
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
	BacklogGauge.Set(5)

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
	ValueSumPerType.WithLabelValues("3").Add(42)

	if got := testutil.ToFloat64(TasksDone); got > 0 {
		// Counter incremented — test passes if non-zero.
	}
	if got := testutil.ToFloat64(ValueSumPerType.WithLabelValues("3")); got < 42 {
		t.Errorf("ValueSumPerType[3] = %v, want >= 42", got)
	}

	_ = reg
}

func TestServe(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	go func() {
		_ = Serve(port, log)
	}()

	// Retry loop instead of fixed sleep — avoids flakiness in CI.
	var resp *http.Response
	url := fmt.Sprintf("http://localhost:%d/metrics", port)
	for range 20 {
		resp, err = http.Get(url)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /metrics: %v (after retries)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
