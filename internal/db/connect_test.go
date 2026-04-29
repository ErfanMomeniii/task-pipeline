package db

import "testing"

func TestDSN(t *testing.T) {
	got := DSN("myhost", 5433, "user", "pass", "mydb", "require")
	want := "postgres://user:pass@myhost:5433/mydb?sslmode=require"
	if got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestDSN_Defaults(t *testing.T) {
	got := DSN("localhost", 5432, "taskpipeline", "taskpipeline", "taskpipeline", "disable")
	want := "postgres://taskpipeline:taskpipeline@localhost:5432/taskpipeline?sslmode=disable"
	if got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}
