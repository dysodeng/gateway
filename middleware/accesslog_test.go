package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAccessLog_LogsRequest 验证访问日志中间件不破坏正常请求流程，
// 并能正确捕获响应状态码
func TestAccessLog_LogsRequest(t *testing.T) {
	// 测试正常请求返回 200
	t.Run("正常请求返回200", func(t *testing.T) {
		handler := NewAccessLog()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, w.Code)
		}
	})

	// 测试 statusWriter 正确捕获非 200 状态码
	t.Run("正确捕获非200状态码", func(t *testing.T) {
		capturedCode := 0
		handler := NewAccessLog()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			// 从包装的 ResponseWriter 中读取已捕获的状态码
			if sw, ok := w.(*statusWriter); ok {
				capturedCode = sw.statusCode
			}
		}))

		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("期望响应状态码 %d，实际 %d", http.StatusNotFound, w.Code)
		}
		if capturedCode != http.StatusNotFound {
			t.Errorf("statusWriter 期望捕获状态码 %d，实际 %d", http.StatusNotFound, capturedCode)
		}
	})

	// 测试未显式调用 WriteHeader 时默认状态码为 200
	t.Run("默认状态码为200", func(t *testing.T) {
		handler := NewAccessLog()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 不显式调用 WriteHeader
			_, _ = w.Write([]byte("ok"))
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期望默认状态码 %d，实际 %d", http.StatusOK, w.Code)
		}
	})
}
