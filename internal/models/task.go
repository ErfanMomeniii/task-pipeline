package models

// State represents the processing state of a task.
type State string

const (
	StateReceived   State = "received"
	StateProcessing State = "processing"
	StateDone       State = "done"
)

// Task represents a unit of work flowing through the pipeline.
type Task struct {
	ID             int64
	Type           int32
	Value          int32
	State          State
	CreationTime   float64
	LastUpdateTime float64
}
