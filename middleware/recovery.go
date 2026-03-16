package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
)

// recoveryResponseWriter 包装 http.ResponseWriter，记录 header 是否已发送
type recoveryResponseWriter struct {
	http.ResponseWriter
	once          sync.Once
	headerWritten bool
}

// WriteHeader 记录 header 已发送
func (w *recoveryResponseWriter) WriteHeader(code int) {
	w.once.Do(func() { w.headerWritten = true })
	w.ResponseWriter.WriteHeader(code)
}

// Write 隐式触发 WriteHeader(200)
func (w *recoveryResponseWriter) Write(b []byte) (int, error) {
	w.once.Do(func() { w.headerWritten = true })
	return w.ResponseWriter.Write(b)
}

// Unwrap 支持 http.ResponseController 等标准库机制
func (w *recoveryResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// NewRecovery 创建 panic 恢复中间件，防止单个请求的 panic 导致服务崩溃
func NewRecovery() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &recoveryResponseWriter{ResponseWriter: w}
			defer func() {
				if err := recover(); err != nil {
					slog.Error("请求处理发生 panic",
						"error", err,
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)
					// 仅在 header 未发送时才尝试写入 500 响应
					if !rw.headerWritten {
						http.Error(w, "内部服务器错误", http.StatusInternalServerError)
					}
				}
			}()
			next.ServeHTTP(rw, r)
		})
	}
}
