package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
)

// mockDiscovery 用于测试的静态服务发现
type mockDiscovery struct {
	instances map[string][]discovery.ServiceInstance
}

func (m *mockDiscovery) GetInstances(name string) ([]discovery.ServiceInstance, error) {
	return m.instances[name], nil
}

func (m *mockDiscovery) Watch(_ string, _ func([]discovery.ServiceInstance)) error {
	return nil
}

func (m *mockDiscovery) Stop() error { return nil }

// TestServerRouteAndProxy 验证请求经过完整管线（中间件→路由→代理）到达后端
func TestServerRouteAndProxy(t *testing.T) {
	// 启动模拟后端
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend-ok"))
	}))
	defer backend.Close()

	// 解析后端地址
	host, port := parseHostPort(t, backend.URL)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen:          ":0",
			ShutdownTimeout: 5 * time.Second,
		},
		Health: config.HealthConfig{Path: "/health"},
		Metrics: config.MetricsConfig{Enabled: false},
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST"},
		},
		Routes: []config.RouteConfig{
			{
				Name:         "test-api",
				Prefix:       "/api/",
				StripPrefix:  true,
				Service:      "test-svc",
				Type:         "http",
				Timeout:      5 * time.Second,
				LoadBalancer: "round_robin",
			},
		},
	}

	disc := &mockDiscovery{
		instances: map[string][]discovery.ServiceInstance{
			"test-svc": {
				{ID: "1", Name: "test-svc", Host: host, Port: port, Weight: 1},
			},
		},
	}

	srv := New(cfg, disc)

	// 使用 httptest 直接测试 handler，无需真正监听端口
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()

	// 测试正常路由转发
	resp, err := http.Get(ts.URL + "/api/hello")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 200，实际 %d", resp.StatusCode)
	}
	if string(body) != "backend-ok" {
		t.Errorf("期望响应 'backend-ok'，实际 '%s'", string(body))
	}

	// 测试健康检查端点
	resp2, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("健康检查请求失败: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("健康检查期望 200，实际 %d", resp2.StatusCode)
	}

	// 测试未匹配路由返回 404
	resp3, err := http.Get(ts.URL + "/unknown")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("未匹配路由期望 404，实际 %d", resp3.StatusCode)
	}
}

// parseHostPort 从 URL 中解析 host 和 port
func parseHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("解析后端地址失败: %v", err)
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("解析端口失败: %v", err)
	}
	return host, port
}

// TestServerMaxRequestBodySize 验证请求体大小限制
func TestServerMaxRequestBodySize(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Write(body)
	}))
	defer backend.Close()

	host, port := parseHostPort(t, backend.URL)

	maxSize := int64(10) // 限制 10 字节
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen:             ":0",
			MaxRequestBodySize: maxSize,
			ShutdownTimeout:    5 * time.Second,
		},
		Health:  config.HealthConfig{Path: "/health"},
		Metrics: config.MetricsConfig{Enabled: false},
		Routes: []config.RouteConfig{
			{
				Name:         "test-api",
				Prefix:       "/api/",
				Service:      "test-svc",
				Type:         "http",
				LoadBalancer: "round_robin",
			},
		},
	}

	disc := &mockDiscovery{
		instances: map[string][]discovery.ServiceInstance{
			"test-svc": {{ID: "1", Host: host, Port: port, Weight: 1}},
		},
	}

	srv := New(cfg, disc)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()

	// 小请求体应通过
	resp, err := http.Post(ts.URL+"/api/test", "text/plain", strings.NewReader("short"))
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("小请求体期望 200，实际 %d", resp.StatusCode)
	}

	// 超大请求体应被拒绝
	bigBody := strings.NewReader(strings.Repeat("x", 100))
	resp2, err := http.Post(ts.URL+"/api/test", "text/plain", bigBody)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	resp2.Body.Close()
	// MaxBytesReader 会导致后端读取失败，返回 413 或 502
	if resp2.StatusCode == http.StatusOK {
		t.Errorf("超大请求体不应返回 200")
	}
}
