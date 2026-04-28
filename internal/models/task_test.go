package models

import "testing"

func TestStateConstants(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateReceived, "received"},
		{StateProcessing, "processing"},
		{StateDone, "done"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.want {
			t.Errorf("got %q, want %q", tt.state, tt.want)
		}
	}
}
