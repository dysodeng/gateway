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

	result, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	cfg := result.Config

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

func TestRouteAuthConfig_ResolveMode(t *testing.T) {
	tests := []struct {
		name        string
		authCfg     RouteAuthConfig
		requestPath string
		prefix      string
		wantMode    string
	}{
		{
			name:        "无规则时使用默认 mode=required",
			authCfg:     RouteAuthConfig{Mode: "required"},
			requestPath: "/api/v1/users/profile",
			prefix:      "/api/v1/users",
			wantMode:    "required",
		},
		{
			name:        "无规则时使用默认 mode=optional",
			authCfg:     RouteAuthConfig{Mode: "optional"},
			requestPath: "/api/v1/users/profile",
			prefix:      "/api/v1/users",
			wantMode:    "optional",
		},
		{
			name: "精确匹配命中规则",
			authCfg: RouteAuthConfig{
				Mode: "required",
				Rules: []AuthRuleConfig{
					{Path: "/login", Mode: "none"},
				},
			},
			requestPath: "/api/v1/users/login",
			prefix:      "/api/v1/users",
			wantMode:    "none",
		},
		{
			name: "精确匹配未命中，使用默认 mode",
			authCfg: RouteAuthConfig{
				Mode: "required",
				Rules: []AuthRuleConfig{
					{Path: "/login", Mode: "none"},
				},
			},
			requestPath: "/api/v1/users/profile",
			prefix:      "/api/v1/users",
			wantMode:    "required",
		},
		{
			name: "通配符匹配命中",
			authCfg: RouteAuthConfig{
				Mode: "required",
				Rules: []AuthRuleConfig{
					{Path: "/public/*", Mode: "none"},
				},
			},
			requestPath: "/api/v1/users/public/avatar",
			prefix:      "/api/v1/users",
			wantMode:    "none",
		},
		{
			name: "通配符匹配精确路径（无子路径）",
			authCfg: RouteAuthConfig{
				Mode: "required",
				Rules: []AuthRuleConfig{
					{Path: "/public/*", Mode: "none"},
				},
			},
			requestPath: "/api/v1/users/public",
			prefix:      "/api/v1/users",
			wantMode:    "none",
		},
		{
			name: "多条规则按顺序匹配，首条命中",
			authCfg: RouteAuthConfig{
				Mode: "required",
				Rules: []AuthRuleConfig{
					{Path: "/feed", Mode: "optional"},
					{Path: "/feed", Mode: "none"},
				},
			},
			requestPath: "/api/v1/users/feed",
			prefix:      "/api/v1/users",
			wantMode:    "optional",
		},
		{
			name:        "向后兼容: 未配置 mode 且 optional=false 等同 required",
			authCfg:     RouteAuthConfig{Optional: false},
			requestPath: "/api/v1/users/profile",
			prefix:      "/api/v1/users",
			wantMode:    "required",
		},
		{
			name:        "向后兼容: 未配置 mode 且 optional=true 等同 optional",
			authCfg:     RouteAuthConfig{Optional: true},
			requestPath: "/api/v1/users/profile",
			prefix:      "/api/v1/users",
			wantMode:    "optional",
		},
		{
			name:        "请求路径等于前缀时子路径为 /",
			authCfg:     RouteAuthConfig{Mode: "required"},
			requestPath: "/api/v1/users",
			prefix:      "/api/v1/users",
			wantMode:    "required",
		},
		{
			name: "请求路径等于前缀时可匹配 / 规则",
			authCfg: RouteAuthConfig{
				Mode: "required",
				Rules: []AuthRuleConfig{
					{Path: "/", Mode: "none"},
				},
			},
			requestPath: "/api/v1/users",
			prefix:      "/api/v1/users",
			wantMode:    "none",
		},
		{
			name:        "无效 mode 值规范化为 required",
			authCfg:     RouteAuthConfig{Mode: "invalid"},
			requestPath: "/api/v1/users/profile",
			prefix:      "/api/v1/users",
			wantMode:    "required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.authCfg.ResolveMode(tt.requestPath, tt.prefix)
			if got != tt.wantMode {
				t.Errorf("ResolveMode() = %q, want %q", got, tt.wantMode)
			}
		})
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

	result, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	cfg := result.Config

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
