package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dysodeng/gateway/config"
)

// okHandler 返回 200 的简单 Handler，用于验证请求是否透传
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// newRequest 创建带指定 RemoteAddr 的测试请求
func newRequest(remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	return req
}

// TestIPFilter_Whitelist_Allowed 验证白名单模式下，列表内 IP 可以通过
func TestIPFilter_Whitelist_Allowed(t *testing.T) {
	cfg := config.IPFilterConfig{
		Mode: "whitelist",
		List: []string{"192.168.1.1"},
	}
	handler := NewGlobalIPFilter(cfg)(okHandler)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, newRequest("192.168.1.1:12345"))

	if w.Code != http.StatusOK {
		t.Errorf("白名单内 IP 期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}
}

// TestIPFilter_Whitelist_Blocked 验证白名单模式下，列表外 IP 被拒绝（403）
func TestIPFilter_Whitelist_Blocked(t *testing.T) {
	cfg := config.IPFilterConfig{
		Mode: "whitelist",
		List: []string{"192.168.1.1"},
	}
	handler := NewGlobalIPFilter(cfg)(okHandler)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, newRequest("10.0.0.1:12345"))

	if w.Code != http.StatusForbidden {
		t.Errorf("白名单外 IP 期望状态码 %d，实际 %d", http.StatusForbidden, w.Code)
	}
}

// TestIPFilter_Blacklist_Blocked 验证黑名单模式下，列表内 IP 被拒绝（403）
func TestIPFilter_Blacklist_Blocked(t *testing.T) {
	cfg := config.IPFilterConfig{
		Mode: "blacklist",
		List: []string{"192.168.1.100"},
	}
	handler := NewGlobalIPFilter(cfg)(okHandler)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, newRequest("192.168.1.100:54321"))

	if w.Code != http.StatusForbidden {
		t.Errorf("黑名单内 IP 期望状态码 %d，实际 %d", http.StatusForbidden, w.Code)
	}
}

// TestIPFilter_Blacklist_Allowed 验证黑名单模式下，列表外 IP 可以通过
func TestIPFilter_Blacklist_Allowed(t *testing.T) {
	cfg := config.IPFilterConfig{
		Mode: "blacklist",
		List: []string{"192.168.1.100"},
	}
	handler := NewGlobalIPFilter(cfg)(okHandler)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, newRequest("10.20.30.40:11111"))

	if w.Code != http.StatusOK {
		t.Errorf("黑名单外 IP 期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}
}

// TestIPFilter_CIDR 验证 CIDR 范围匹配：10.0.0.0/8 包含 10.1.2.3，白名单模式下应通过
func TestIPFilter_CIDR(t *testing.T) {
	cfg := config.IPFilterConfig{
		Mode: "whitelist",
		List: []string{"10.0.0.0/8"},
	}
	handler := NewGlobalIPFilter(cfg)(okHandler)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, newRequest("10.1.2.3:8080"))

	if w.Code != http.StatusOK {
		t.Errorf("CIDR 范围内 IP 期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}
}

// TestIPFilter_EmptyList 验证列表为空时过滤器禁用，所有请求均通过
func TestIPFilter_EmptyList(t *testing.T) {
	cfg := config.IPFilterConfig{
		Mode: "whitelist",
		List: []string{},
	}
	handler := NewGlobalIPFilter(cfg)(okHandler)

	// 任意 IP 均应通过
	for _, addr := range []string{"1.2.3.4:80", "192.168.0.1:443", "10.0.0.1:8080"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, newRequest(addr))
		if w.Code != http.StatusOK {
			t.Errorf("空列表时 %s 期望状态码 %d，实际 %d", addr, http.StatusOK, w.Code)
		}
	}
}
