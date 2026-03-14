package config

import (
	"context"
	"testing"
	"time"
)

// mockWatchSource 支持 Watch 的 mock 配置源
type mockWatchSource struct {
	data    []byte
	err     error
	watchCh chan []byte
}

func (m *mockWatchSource) Load() ([]byte, error)                          { return m.data, m.err }
func (m *mockWatchSource) Type() string                                   { return "mock" }
func (m *mockWatchSource) Watch(_ context.Context) (<-chan []byte, error) { return m.watchCh, nil }

func TestWatcher_ReceivesUpdate(t *testing.T) {
	watchCh := make(chan []byte, 1)
	src := &mockWatchSource{watchCh: watchCh}

	var received *Config
	callback := func(cfg *Config) {
		received = cfg
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWatcher(src, callback)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 模拟配置变更
	watchCh <- []byte("server:\n  listen: \":9999\"\nroutes: []\n")

	time.Sleep(100 * time.Millisecond)

	if received == nil {
		t.Fatal("回调未触发")
	}
	if received.Server.Listen != ":9999" {
		t.Errorf("Server.Listen = %q, want %q", received.Server.Listen, ":9999")
	}
}

func TestWatcher_InvalidYAML(t *testing.T) {
	watchCh := make(chan []byte, 1)
	src := &mockWatchSource{watchCh: watchCh}

	callCount := 0
	callback := func(cfg *Config) {
		callCount++
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWatcher(src, callback)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 发送无效 YAML
	watchCh <- []byte("{{invalid yaml")
	time.Sleep(100 * time.Millisecond)

	if callCount != 0 {
		t.Errorf("无效 YAML 不应触发回调，实际触发了 %d 次", callCount)
	}
}
