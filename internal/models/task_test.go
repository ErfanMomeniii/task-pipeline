package models

import (
	"testing"
	"time"
)

func TestTaskStateConstants(t *testing.T) {
	tests := []struct {
		state TaskState
		want  string
	}{
		{TaskStateReceived, "received"},
		{TaskStateProcessing, "processing"},
		{TaskStateDone, "done"},
		{TaskStateStale, "stale"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.want {
			t.Errorf("TaskState = %q, want %q", tt.state, tt.want)
		}
	}
}

func TestNowUnixSec(t *testing.T) {
	before := float64(time.Now().UnixMilli()) / 1000.0
	got := NowUnixSec()
	after := float64(time.Now().UnixMilli()) / 1000.0

	if got < before || got > after {
		t.Errorf("NowUnixSec() = %f, want between %f and %f", got, before, after)
	}
}