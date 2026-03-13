package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dysodeng/gateway/discovery"
)

// TestHTTPProxy_Forward 测试 HTTP 反向代理基本转发功能
func TestHTTPProxy_Forward(t *testing.T) {
	// 创建模拟后端服务器
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	// 从后端 URL 解析 host 和 port
	host, port := parseHostPort(t, backend.URL)

	instance := &discovery.ServiceInstance{
		Host: host,
		Port: port,
	}

	// 创建网关请求和响应记录器
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	proxy := NewHTTPProxy()
	proxy.Forward(rec, req, instance, false, "")

	resp := rec.Result()
	defer resp.Body.Close()

	// 验证状态码
	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 %d，实际得到 %d", http.StatusOK, resp.StatusCode)
	}

	// 验证响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	if string(body) != "hello from backend" {
		t.Errorf("期望响应体 %q，实际得到 %q", "hello from backend", string(body))
	}
}

// TestHTTPProxy_StripPrefix 测试 HTTP 反向代理前缀剥离功能
func TestHTTPProxy_StripPrefix(t *testing.T) {
	// 记录后端收到的请求路径
	var receivedPath string

	// 创建模拟后端服务器
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// 从后端 URL 解析 host 和 port
	host, port := parseHostPort(t, backend.URL)

	instance := &discovery.ServiceInstance{
		Host: host,
		Port: port,
	}

	// 模拟客户端请求 /api/v1/users/123，剥离前缀 /api/v1/users
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/123", nil)
	rec := httptest.NewRecorder()

	proxy := NewHTTPProxy()
	proxy.Forward(rec, req, instance, true, "/api/v1/users")

	// 验证后端收到的路径为 /123
	if receivedPath != "/123" {
		t.Errorf("期望后端收到路径 %q，实际得到 %q", "/123", receivedPath)
	}
}
