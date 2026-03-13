package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// statusWriter 包装 ResponseWriter 以捕获响应状态码
type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.statusCode = code
	sw.ResponseWriter.WriteHeader(code)
}

// Flush 转发 Flush 调用，确保 SSE 等流式场景能逐条刷新
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap 允许 http.ResponseController 等机制访问底层 ResponseWriter
func (sw *statusWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}

// NewAccessLog 创建访问日志中间件，记录每个请求的方法、路径、状态码和耗时
func NewAccessLog() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(sw, r)
			slog.Info("访问日志",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.statusCode,
				"latency_ms", time.Since(start).Milliseconds(),
				"client_ip", r.RemoteAddr,
			)
		})
	}
}
