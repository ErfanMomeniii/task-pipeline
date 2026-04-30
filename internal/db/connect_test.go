package db

import (
	"net/url"
	"testing"
)

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

func TestDSN_SpecialCharsInPassword(t *testing.T) {
	got := DSN("localhost", 5432, "admin", "p@ss:w/rd%20!", "mydb", "disable")

	// Parse the DSN to verify credentials are correctly escaped.
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("DSN produced invalid URL: %v", err)
	}

	if u.User.Username() != "admin" {
		t.Errorf("username = %q, want %q", u.User.Username(), "admin")
	}
	pass, _ := u.User.Password()
	if pass != "p@ss:w/rd%20!" {
		t.Errorf("password = %q, want %q", pass, "p@ss:w/rd%20!")
	}
	if u.Hostname() != "localhost" {
		t.Errorf("host = %q, want %q", u.Hostname(), "localhost")
	}
}
