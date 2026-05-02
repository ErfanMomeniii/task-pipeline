package models

import "time"

// TaskState represents the processing state of a task in the pipeline.
type TaskState string

const (
	TaskStateReceived   TaskState = "received"
	TaskStateProcessing TaskState = "processing"
	TaskStateDone       TaskState = "done"
	TaskStateStale      TaskState = "stale"
)

// NowUnixSec returns the current time as a Unix timestamp with millisecond
// precision, expressed in seconds (e.g., 1714600000.123).
func NowUnixSec() float64 {
	return float64(time.Now().UnixMilli()) / 1000.0
}

