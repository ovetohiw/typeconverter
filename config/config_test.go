package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadFile(t *testing.T) {
	t.Setenv("ADDR", "")
	t.Setenv("WORKERS", "")
	t.Setenv("QUEUE_SIZE", "")
	t.Setenv("MAX_JOBS", "")
	t.Setenv("MAX_BODY_BYTES", "")
	t.Setenv("STORE", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	src := []byte(`{
		"http": {"addr": ":9090", "max_body_bytes": 2048},
		"queue": {"workers": 3, "queue_size": 10, "max_jobs": 50}
	}`)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != ":9090" || cfg.HTTP.MaxBodyBytes != 2048 {
		t.Fatalf("http: %+v", cfg.HTTP)
	}
	if cfg.Queue.Workers != 3 || cfg.Queue.QueueSize != 10 || cfg.Queue.MaxJobs != 50 {
		t.Fatalf("queue: %+v", cfg.Queue)
	}
	if cfg.Store.Path != "typeconverter.db" {
		t.Fatalf("store path %q", cfg.Store.Path)
	}
}

func TestLoadAppliesDefaultsForZeroWorkers(t *testing.T) {
	t.Setenv("ADDR", "")
	t.Setenv("WORKERS", "")
	t.Setenv("QUEUE_SIZE", "")
	t.Setenv("MAX_JOBS", "")
	t.Setenv("MAX_BODY_BYTES", "")
	t.Setenv("STORE", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"queue":{"workers":0}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := runtime.NumCPU()
	if want < 2 {
		want = 2
	}
	if cfg.Queue.Workers != want {
		t.Fatalf("workers=%d want %d", cfg.Queue.Workers, want)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Fatalf("addr %q", cfg.HTTP.Addr)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"http":{"addr":":8080"},"queue":{"workers":2}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADDR", ":7070")
	t.Setenv("WORKERS", "8")
	t.Setenv("STORE", "custom.db")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != ":7070" || cfg.Queue.Workers != 8 || cfg.Store.Path != "custom.db" {
		t.Fatalf("env override failed: %+v", cfg)
	}
}

func TestLoadOrDefaultMissingFile(t *testing.T) {
	t.Setenv("ADDR", ":6060")
	t.Setenv("STORE", "")
	cfg, err := LoadOrDefault(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != ":6060" {
		t.Fatalf("addr %q", cfg.HTTP.Addr)
	}
}

func TestLoadMissingFileError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate(t *testing.T) {
	cfg := Default()
	cfg.HTTP.Addr = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}
