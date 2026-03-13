package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dysodeng/gateway/config"
)

// 构造测试用 CORS 配置
func newTestCORSConfig() config.CORSConfig {
	return config.CORSConfig{
		AllowedOrigins: []string{"https://example.com", "https://app.example.com"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:         3600,
	}
}

// TestCORS_Preflight 验证 OPTIONS 预检请求获得 CORS 头部并返回 204
func TestCORS_Preflight(t *testing.T) {
	handler := NewCORS(newTestCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 预检请求不应到达此处
		t.Error("预检请求不应透传到后端 Handler")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/resource", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusNoContent, w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin 期望 %q，实际 %q", "https://example.com", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods 头部不应为空")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Access-Control-Allow-Headers 头部不应为空")
	}
	if got := w.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("Access-Control-Max-Age 期望 %q，实际 %q", "3600", got)
	}
}

// TestCORS_NormalRequest 验证带 Origin 的普通 GET 请求获得 CORS 头部并透传到后端
func TestCORS_NormalRequest(t *testing.T) {
	backendCalled := false
	handler := NewCORS(newTestCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !backendCalled {
		t.Error("后端 Handler 应被调用")
	}
	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin 期望 %q，实际 %q", "https://example.com", got)
	}
}

// TestCORS_NoOrigin 验证无 Origin 请求头的请求不添加 CORS 头部并直接透传
func TestCORS_NoOrigin(t *testing.T) {
	backendCalled := false
	handler := NewCORS(newTestCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	// 不设置 Origin 头部
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !backendCalled {
		t.Error("后端 Handler 应被调用")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("无 Origin 请求不应设置 Access-Control-Allow-Origin，实际为 %q", got)
	}
}

// TestCORS_DisallowedOrigin 验证不在允许列表中的 Origin 不添加 CORS 头部并透传
func TestCORS_DisallowedOrigin(t *testing.T) {
	backendCalled := false
	handler := NewCORS(newTestCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "https://malicious.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !backendCalled {
		t.Error("后端 Handler 应被调用（不应阻断请求）")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("不允许的 Origin 不应设置 Access-Control-Allow-Origin，实际为 %q", got)
	}
}
