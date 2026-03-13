package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dysodeng/gateway/config"
)

// TestRateLimit_UnderLimit 验证请求数未超限时，所有请求均通过
func TestRateLimit_UnderLimit(t *testing.T) {
	globalCfg := config.RateLimitConfig{
		Storage:   "local",
		Algorithm: "sliding_window",
	}
	routeCfg := config.RouteRateLimitConfig{
		Enabled: true,
		QPS:     5,
	}
	handler := NewRateLimit(globalCfg, routeCfg, "test-route")(okHandler)

	// 发送 5 个请求，均应通过
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("第 %d 个请求期望状态码 %d，实际 %d", i+1, http.StatusOK, w.Code)
		}
	}
}

// TestRateLimit_OverLimit 验证请求数超限时，返回 429 Too Many Requests
func TestRateLimit_OverLimit(t *testing.T) {
	globalCfg := config.RateLimitConfig{
		Storage:   "local",
		Algorithm: "sliding_window",
	}
	routeCfg := config.RouteRateLimitConfig{
		Enabled: true,
		QPS:     3,
	}
	handler := NewRateLimit(globalCfg, routeCfg, "test-route-over")(okHandler)

	// 发送 3 个请求（在限制内）
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:5678"
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("第 %d 个请求期望状态码 %d，实际 %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 第 4 个请求应被限流
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5678"
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("超限请求期望状态码 %d，实际 %d", http.StatusTooManyRequests, w.Code)
	}

	// 验证响应头包含 Retry-After
	if w.Header().Get("Retry-After") == "" {
		t.Error("超限响应应包含 Retry-After 响应头")
	}
}

// TestRateLimit_TokenBucket_OverLimit 验证令牌桶算法在令牌耗尽后返回 429
func TestRateLimit_TokenBucket_OverLimit(t *testing.T) {
	globalCfg := config.RateLimitConfig{
		Storage:   "local",
		Algorithm: "token_bucket",
	}
	routeCfg := config.RouteRateLimitConfig{
		Enabled: true,
		QPS:     3,
	}
	handler := NewRateLimit(globalCfg, routeCfg, "test-route-tb")(okHandler)

	// 消耗所有 3 个令牌
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:4321"
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("第 %d 个请求期望状态码 %d，实际 %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 第 4 个请求应被限流
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("令牌桶超限请求期望状态码 %d，实际 %d", http.StatusTooManyRequests, w.Code)
	}
}

// TestRateLimit_Disabled 验证限流禁用时，所有请求均通过（不受 QPS 限制）
func TestRateLimit_Disabled(t *testing.T) {
	globalCfg := config.RateLimitConfig{
		Storage:   "local",
		Algorithm: "sliding_window",
	}
	routeCfg := config.RouteRateLimitConfig{
		Enabled: false,
		QPS:     1,
	}
	handler := NewRateLimit(globalCfg, routeCfg, "test-route-disabled")(okHandler)

	// 即使超过 QPS 限制，禁用状态下所有请求均应通过
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:9999"
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("禁用限流时第 %d 个请求期望状态码 %d，实际 %d", i+1, http.StatusOK, w.Code)
		}
	}
}
