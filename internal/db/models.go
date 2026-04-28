package db

import (
	"database/sql/driver"
	"fmt"
)

type TaskState string

const (
	TaskStateReceived   TaskState = "received"
	TaskStateProcessing TaskState = "processing"
	TaskStateDone       TaskState = "done"
)

func (e *TaskState) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = TaskState(s)
	case string:
		*e = TaskState(s)
	default:
		return fmt.Errorf("unsupported scan type for TaskState: %T", src)
	}
	return nil
}

type NullTaskState struct {
	TaskState TaskState `json:"task_state"`
	Valid     bool      `json:"valid"` // Valid is true if TaskState is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullTaskState) Scan(value interface{}) error {
	if value == nil {
		ns.TaskState, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.TaskState.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullTaskState) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.TaskState), nil
}

type Task struct {
	ID             int64   `json:"id"`
	Type           int32   `json:"type"`
	Value          int32   `json:"value"`
	State          string  `json:"state"`
	CreationTime   float64 `json:"creation_time"`
	LastUpdateTime float64 `json:"last_update_time"`
}
