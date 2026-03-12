package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yaml := `
server:
  listen: ":9090"
  max_request_body_size: 5242880
  shutdown_timeout: 10s
log:
  level: "debug"
  output: "stdout"
discovery:
  type: "static"
  static:
    services:
      test-svc:
        - host: "127.0.0.1"
          port: 8081
          weight: 1
routes:
  - name: "test"
    prefix: "/api/test"
    service: "test-svc"
    timeout: 3s
    load_balancer: "round_robin"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Listen != ":9090" {
		t.Errorf("Server.Listen = %q, want %q", cfg.Server.Listen, ":9090")
	}
	if cfg.Server.MaxRequestBodySize != 5242880 {
		t.Errorf("MaxRequestBodySize = %d, want %d", cfg.Server.MaxRequestBodySize, 5242880)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("len(Routes) = %d, want 1", len(cfg.Routes))
	}
	if cfg.Routes[0].Prefix != "/api/test" {
		t.Errorf("Routes[0].Prefix = %q, want %q", cfg.Routes[0].Prefix, "/api/test")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	yaml := `
server:
  listen: ":8080"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.ShutdownTimeout == 0 {
		t.Error("expected default ShutdownTimeout, got 0")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level 'info', got %q", cfg.Log.Level)
	}
	if cfg.Health.Path != "/health" {
		t.Errorf("expected default health path '/health', got %q", cfg.Health.Path)
	}
}
