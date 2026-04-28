package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load defaults: %v", err)
	}

	if cfg.DB.Host != "localhost" {
		t.Errorf("DB.Host = %q, want localhost", cfg.DB.Host)
	}
	if cfg.DB.Port != 5432 {
		t.Errorf("DB.Port = %d, want 5432", cfg.DB.Port)
	}
	if cfg.GRPC.Port != 50051 {
		t.Errorf("GRPC.Port = %d, want 50051", cfg.GRPC.Port)
	}
	if cfg.Producer.MaxBacklog != 100 {
		t.Errorf("Producer.MaxBacklog = %d, want 100", cfg.Producer.MaxBacklog)
	}
	if cfg.Consumer.RateLimit != 10 {
		t.Errorf("Consumer.RateLimit = %d, want 10", cfg.Consumer.RateLimit)
	}
}

func TestLoad_FromFile(t *testing.T) {
	content := []byte(`
db:
  host: "dbhost"
  port: 5433
grpc:
  port: 9999
`)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load file: %v", err)
	}

	if cfg.DB.Host != "dbhost" {
		t.Errorf("DB.Host = %q, want dbhost", cfg.DB.Host)
	}
	if cfg.DB.Port != 5433 {
		t.Errorf("DB.Port = %d, want 5433", cfg.DB.Port)
	}
	if cfg.GRPC.Port != 9999 {
		t.Errorf("GRPC.Port = %d, want 9999", cfg.GRPC.Port)
	}
	// Defaults still apply for unset fields
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want info", cfg.Log.Level)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("TP_DB_HOST", "envhost")
	t.Setenv("TP_DB_PORT", "5434")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load env: %v", err)
	}

	if cfg.DB.Host != "envhost" {
		t.Errorf("DB.Host = %q, want envhost", cfg.DB.Host)
	}
	if cfg.DB.Port != 5434 {
		t.Errorf("DB.Port = %d, want 5434", cfg.DB.Port)
	}
}
