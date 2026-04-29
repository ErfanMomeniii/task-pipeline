package models

// TaskState represents the processing state of a task in the pipeline.
type TaskState string

const (
	TaskStateReceived   TaskState = "received"
	TaskStateProcessing TaskState = "processing"
	TaskStateDone       TaskState = "done"
)
