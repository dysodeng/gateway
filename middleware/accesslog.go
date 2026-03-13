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
