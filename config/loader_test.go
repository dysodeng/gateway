package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mockSource 用于测试的 mock 配置源
type mockSource struct {
	data []byte
	err  error
}

func (m *mockSource) Load() ([]byte, error) {
	return m.data, m.err
}

func (m *mockSource) Type() string {
	return "mock"
}

func (m *mockSource) Watch(_ context.Context) (<-chan []byte, error) {
	return make(chan []byte), nil
}

func TestLoadFromSource(t *testing.T) {
	yamlData := []byte(`
server:
  listen: ":9999"
log:
  level: "warn"
routes:
  - name: "remote-route"
    prefix: "/api/remote"
    service: "remote-svc"
`)
	source := &mockSource{data: yamlData}
	cfg, err := loadFromSource(source)
	if err != nil {
		t.Fatalf("loadFromSource() error: %v", err)
	}
	if cfg.Server.Listen != ":9999" {
		t.Errorf("Server.Listen = %q, want %q", cfg.Server.Listen, ":9999")
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "warn")
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].Prefix != "/api/remote" {
		t.Errorf("Routes 未正确解析")
	}
}

func TestLoadFromSourceError(t *testing.T) {
	source := &mockSource{err: fmt.Errorf("connection refused")}
	_, err := loadFromSource(source)
	if err == nil {
		t.Fatal("loadFromSource() 应返回错误")
	}
}

func TestLoadFallbackToLocal(t *testing.T) {
	// 无 config_center 配置时应回退使用本地文件
	yaml := `
server:
  listen: ":7070"
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

	if cfg.Server.Listen != ":7070" {
		t.Errorf("Server.Listen = %q, want %q", cfg.Server.Listen, ":7070")
	}
	if result.Source != "local" {
		t.Errorf("Source = %q, want %q", result.Source, "local")
	}
	// 验证默认值仍然生效
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want default %q", cfg.Log.Level, "info")
	}
}

func TestCreateSourceNil(t *testing.T) {
	// config_center 为 nil 时返回 nil
	if s := createSource(nil); s != nil {
		t.Error("createSource(nil) 应返回 nil")
	}
	// type 为空时返回 nil
	if s := createSource(&ConfigCenterConfig{}); s != nil {
		t.Error("createSource(空type) 应返回 nil")
	}
	// etcd 类型但 etcd 配置为 nil 时返回 nil
	if s := createSource(&ConfigCenterConfig{Type: "etcd"}); s != nil {
		t.Error("createSource(etcd无配置) 应返回 nil")
	}
	// 不支持的类型返回 nil
	if s := createSource(&ConfigCenterConfig{Type: "unknown"}); s != nil {
		t.Error("createSource(unknown) 应返回 nil")
	}
}

func TestCreateSourceEtcd(t *testing.T) {
	cc := &ConfigCenterConfig{
		Type: "etcd",
		Etcd: &EtcdSourceConfig{
			Endpoints: []string{"127.0.0.1:2379"},
			Key:       "/gateway/config",
		},
	}
	s := createSource(cc)
	if s == nil {
		t.Fatal("createSource() 不应返回 nil")
	}
	if s.Type() != "etcd" {
		t.Errorf("Type() = %q, want %q", s.Type(), "etcd")
	}
}
