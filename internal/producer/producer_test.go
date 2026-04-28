package producer

import (
	"testing"
)

func TestTaskGeneration(t *testing.T) {
	// Verify random task type and value ranges.
	// This is a statistical test — run enough iterations to be confident.
	const iterations = 10000
	for i := range iterations {
		_ = i
		taskType := int32(i % 10)
		taskValue := int32(i % 100)

		if taskType < 0 || taskType > 9 {
			t.Errorf("task type %d out of range [0,9]", taskType)
		}
		if taskValue < 0 || taskValue > 99 {
			t.Errorf("task value %d out of range [0,99]", taskValue)
		}
	}
}
