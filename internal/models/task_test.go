package models

import "testing"

func TestTaskStateConstants(t *testing.T) {
	tests := []struct {
		state TaskState
		want  string
	}{
		{TaskStateReceived, "received"},
		{TaskStateProcessing, "processing"},
		{TaskStateDone, "done"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.want {
			t.Errorf("TaskState = %q, want %q", tt.state, tt.want)
		}
	}
}

func TestTask_Fields(t *testing.T) {
	task := Task{
		ID:             1,
		Type:           5,
		Value:          42,
		State:          string(TaskStateReceived),
		CreationTime:   1000.0,
		LastUpdateTime: 1000.0,
	}

	if task.ID != 1 {
		t.Errorf("ID = %d, want 1", task.ID)
	}
	if task.State != "received" {
		t.Errorf("State = %q, want received", task.State)
	}
}
