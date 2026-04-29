package models

// TaskState represents the processing state of a task in the pipeline.
type TaskState string

const (
	TaskStateReceived   TaskState = "received"
	TaskStateProcessing TaskState = "processing"
	TaskStateDone       TaskState = "done"
)

// Task represents a unit of work flowing through the pipeline.
type Task struct {
	ID             int64   `json:"id"`
	Type           int32   `json:"type"`
	Value          int32   `json:"value"`
	State          string  `json:"state"`
	CreationTime   float64 `json:"creation_time"`
	LastUpdateTime float64 `json:"last_update_time"`
}
