package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestChain 验证中间件链按照提供顺序依次执行
func TestChain(t *testing.T) {
	// 记录中间件执行顺序
	order := make([]string, 0, 3)

	// 创建三个中间件，分别向 order 追加标记
	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "a")
			next.ServeHTTP(w, r)
		})
	}
	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "b")
			next.ServeHTTP(w, r)
		})
	}
	m3 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "c")
			next.ServeHTTP(w, r)
		})
	}

	// 链接中间件并执行请求
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := Chain(m1, m2, m3)(final)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// 验证执行顺序为 ["a", "b", "c"]
	expected := []string{"a", "b", "c"}
	if len(order) != len(expected) {
		t.Fatalf("期望执行 %d 个中间件，实际执行 %d 个", len(expected), len(order))
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("位置 %d：期望 %q，实际 %q", i, v, order[i])
		}
	}
}
