package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "text", "info")
	log.Info("hello", "key", "value")

	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", out)
	}
	if !strings.Contains(out, "key=value") {
		t.Errorf("expected 'key=value' in output, got: %s", out)
	}
}

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "json", "debug")
	log.Debug("test-msg")

	out := buf.String()
	if !strings.Contains(out, `"msg":"test-msg"`) {
		t.Errorf("expected JSON msg field, got: %s", out)
	}
}

func TestNew_WarnLevel(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "text", "warn")
	log.Info("should-not-appear")
	log.Warn("should-appear")

	out := buf.String()
	if strings.Contains(out, "should-not-appear") {
		t.Error("info message should be filtered at warn level")
	}
	if !strings.Contains(out, "should-appear") {
		t.Error("warn message should appear at warn level")
	}
}

func TestNew_DefaultLevel(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "text", "unknown-level")
	log.Info("hello")

	if !strings.Contains(buf.String(), "hello") {
		t.Error("unknown level should default to info")
	}
}

func TestNew_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "text", "error")
	log.Info("should-not-appear")

	if buf.Len() != 0 {
		t.Errorf("expected no output for info at error level, got: %s", buf.String())
	}
}
