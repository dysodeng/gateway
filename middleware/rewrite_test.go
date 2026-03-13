package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dysodeng/gateway/config"
)

// captureHeaderHandler 捕获请求头的测试 Handler，将请求头写入响应头以便断言
var captureHeaderHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 将请求中的所有头复制到响应头，方便测试断言
	for key, values := range r.Header {
		for _, v := range values {
			w.Header().Add("X-Captured-"+key, v)
		}
	}
	w.WriteHeader(http.StatusOK)
})

// TestRewrite_AddHeaders 验证请求头注入功能：add_headers 中的头应被添加到请求中
func TestRewrite_AddHeaders(t *testing.T) {
	cfg := config.RouteRewriteConfig{
		AddHeaders: map[string]string{
			"X-Service-Name": "user-service",
			"X-Gateway":      "api-gateway",
		},
	}
	handler := NewRewrite(cfg)(captureHeaderHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}

	// 验证注入的请求头已被下游 Handler 接收
	if w.Header().Get("X-Captured-X-Service-Name") != "user-service" {
		t.Errorf("期望 X-Service-Name 被注入为 user-service，实际为 %q",
			w.Header().Get("X-Captured-X-Service-Name"))
	}
	if w.Header().Get("X-Captured-X-Gateway") != "api-gateway" {
		t.Errorf("期望 X-Gateway 被注入为 api-gateway，实际为 %q",
			w.Header().Get("X-Captured-X-Gateway"))
	}
}

// TestRewrite_RemoveHeaders 验证请求头移除功能：remove_headers 中的头应从请求中移除
func TestRewrite_RemoveHeaders(t *testing.T) {
	cfg := config.RouteRewriteConfig{
		RemoveHeaders: []string{"X-Internal-Token", "X-Debug"},
	}
	handler := NewRewrite(cfg)(captureHeaderHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// 预先设置需要被移除的请求头
	req.Header.Set("X-Internal-Token", "secret-token")
	req.Header.Set("X-Debug", "true")
	req.Header.Set("X-Keep-Me", "keep-value")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}

	// 验证指定头已被移除
	if w.Header().Get("X-Captured-X-Internal-Token") != "" {
		t.Errorf("X-Internal-Token 应被移除，但实际仍存在: %q",
			w.Header().Get("X-Captured-X-Internal-Token"))
	}
	if w.Header().Get("X-Captured-X-Debug") != "" {
		t.Errorf("X-Debug 应被移除，但实际仍存在: %q",
			w.Header().Get("X-Captured-X-Debug"))
	}

	// 验证未在移除列表中的头保持不变
	if w.Header().Get("X-Captured-X-Keep-Me") != "keep-value" {
		t.Errorf("X-Keep-Me 不应被移除，期望 keep-value，实际为 %q",
			w.Header().Get("X-Captured-X-Keep-Me"))
	}
}

// TestRewrite_EmptyConfig 验证空配置时请求直接透传，不做任何修改
func TestRewrite_EmptyConfig(t *testing.T) {
	cfg := config.RouteRewriteConfig{}
	handler := NewRewrite(cfg)(captureHeaderHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Original", "original-value")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}

	// 原有头应保持不变
	if w.Header().Get("X-Captured-X-Original") != "original-value" {
		t.Errorf("空配置下原有请求头应保持不变，期望 original-value，实际为 %q",
			w.Header().Get("X-Captured-X-Original"))
	}
}
