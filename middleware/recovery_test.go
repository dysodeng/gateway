package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRecovery_NoPanic 验证正常请求不受恢复中间件影响，返回 200
func TestRecovery_NoPanic(t *testing.T) {
	handler := NewRecovery()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}
}

// TestRecovery_CatchesPanic 验证发生 panic 的请求被捕获并返回 500，不会导致服务崩溃
func TestRecovery_CatchesPanic(t *testing.T) {
	handler := NewRecovery()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("模拟 panic 错误")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()

	// 此处不应 panic，中间件应捕获并返回 500
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusInternalServerError, w.Code)
	}
}
