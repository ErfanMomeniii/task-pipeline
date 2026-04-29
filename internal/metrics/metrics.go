package metrics

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Producer metrics.
var (
	TasksProduced = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "producer_tasks_produced_total",
		Help: "Total number of tasks produced and sent to consumer.",
	})

	BacklogGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "producer_backlog_current",
		Help: "Current number of unprocessed tasks (backlog).",
	})
)

// Shared metrics (used by both services).
var (
	TasksByState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tasks_by_state",
		Help: "Current number of tasks in each state (received, processing, done).",
	}, []string{"state"})
)

// Consumer metrics.
var (
	TasksReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "consumer_tasks_received_total",
		Help: "Total number of tasks received by consumer.",
	})

	TasksDone = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "consumer_tasks_done_total",
		Help: "Total number of tasks processed to done state.",
	})

	TasksPerType = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "consumer_tasks_per_type_total",
		Help: "Total processed tasks per task type.",
	}, []string{"type"})

	ValueSumPerType = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "consumer_value_sum_per_type_total",
		Help: "Total sum of task values per task type.",
	}, []string{"type"})

	ProcessingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "consumer_task_processing_duration_seconds",
		Help:    "Histogram of task processing duration in seconds.",
		Buckets: prometheus.DefBuckets,
	})
)

// RegisterProducer registers producer-specific metrics with the default registry.
func RegisterProducer() {
	prometheus.MustRegister(
		TasksProduced,
		BacklogGauge,
		TasksByState,
	)
}

// RegisterConsumer registers consumer-specific metrics with the default registry.
func RegisterConsumer() {
	prometheus.MustRegister(
		TasksReceived,
		TasksDone,
		TasksPerType,
		ValueSumPerType,
		ProcessingDuration,
		TasksByState,
	)
}

// Serve starts an HTTP server exposing /metrics on the given port.
// It blocks until the server returns an error.
func Serve(port int, log *slog.Logger) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	addr := fmt.Sprintf(":%d", port)
	log.Info("starting metrics server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}
