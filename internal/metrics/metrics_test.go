package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRegisterProducer(t *testing.T) {
	// Use a fresh registry to avoid conflicts with other tests.
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

func TestRegisterConsumer(t *testing.T) {
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

	_ = reg // keep registry reference
}
