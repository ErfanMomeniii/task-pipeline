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