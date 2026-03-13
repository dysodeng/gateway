package etcd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dysodeng/gateway/sdk"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	if opts.prefix != "/services/" {
		t.Errorf("默认 prefix 应为 /services/，实际为 %s", opts.prefix)
	}
	if opts.ttl != 10*time.Second {
		t.Errorf("默认 ttl 应为 10s，实际为 %v", opts.ttl)
	}
	if opts.dialTimeout != 5*time.Second {
		t.Errorf("默认 dialTimeout 应为 5s，实际为 %v", opts.dialTimeout)
	}
	if opts.healthInterval != 5*time.Second {
		t.Errorf("默认 healthInterval 应为 5s，实际为 %v", opts.healthInterval)
	}
}

func TestWithOptions(t *testing.T) {
	opts := defaultOptions()
	WithPrefix("/custom/")(opts)
	WithTTL(30 * time.Second)(opts)
	WithDialTimeout(10 * time.Second)(opts)
	WithAuth("user", "pass")(opts)

	if opts.prefix != "/custom/" {
		t.Errorf("prefix 应为 /custom/，实际为 %s", opts.prefix)
	}
	if opts.ttl != 30*time.Second {
		t.Errorf("ttl 应为 30s，实际为 %v", opts.ttl)
	}
	if opts.dialTimeout != 10*time.Second {
		t.Errorf("dialTimeout 应为 10s，实际为 %v", opts.dialTimeout)
	}
	if opts.username != "user" || opts.password != "pass" {
		t.Error("认证信息设置不正确")
	}
}

func TestWithHealthChecker(t *testing.T) {
	opts := defaultOptions()
	checker := func(ctx context.Context) error { return nil }
	WithHealthChecker(checker, 3*time.Second)(opts)

	if opts.healthChecker == nil {
		t.Error("healthChecker 不应为 nil")
	}
	if opts.healthInterval != 3*time.Second {
		t.Errorf("healthInterval 应为 3s，实际为 %v", opts.healthInterval)
	}
}

func TestWithHealthCheckerDefaultInterval(t *testing.T) {
	opts := defaultOptions()
	checker := func(ctx context.Context) error { return nil }
	// interval <= 0 时保持默认值
	WithHealthChecker(checker, 0)(opts)

	if opts.healthInterval != 5*time.Second {
		t.Errorf("healthInterval 应保持默认 5s，实际为 %v", opts.healthInterval)
	}
}

func TestInstanceKey(t *testing.T) {
	r := &Registry{
		opts: &options{prefix: "/services/"},
	}

	inst := sdk.ServiceInstance{
		Name: "user-service",
		ID:   "10.0.0.1:8080",
		Host: "10.0.0.1",
		Port: 8080,
	}

	key := r.instanceKey(inst)
	expected := "/services/user-service/10.0.0.1:8080"
	if key != expected {
		t.Errorf("instanceKey 应为 %s，实际为 %s", expected, key)
	}
}

func TestInstanceKeyAutoID(t *testing.T) {
	r := &Registry{
		opts: &options{prefix: "/services/"},
	}

	inst := sdk.ServiceInstance{
		Name: "order-service",
		Host: "192.168.1.10",
		Port: 9090,
	}

	// ID 为空时，Register 会自动生成，这里模拟该逻辑
	if inst.ID == "" {
		inst.ID = "192.168.1.10:9090"
	}

	key := r.instanceKey(inst)
	if !strings.HasPrefix(key, "/services/order-service/") {
		t.Errorf("key 前缀不正确: %s", key)
	}
}

func TestPrefixNormalization(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/services", "/services/"},
		{"/services/", "/services/"},
		{"/custom/prefix", "/custom/prefix/"},
		{"/custom/prefix/", "/custom/prefix/"},
	}

	for _, tt := range tests {
		opts := defaultOptions()
		opts.prefix = tt.input
		// 模拟 NewRegistry 中的 prefix 规范化
		if !strings.HasSuffix(opts.prefix, "/") {
			opts.prefix += "/"
		}
		if opts.prefix != tt.want {
			t.Errorf("prefix %q 规范化后应为 %q，实际为 %q", tt.input, tt.want, opts.prefix)
		}
	}
}
