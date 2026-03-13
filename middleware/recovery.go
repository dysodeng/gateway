package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// NewRecovery 创建 panic 恢复中间件，防止单个请求的 panic 导致服务崩溃
func NewRecovery() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					slog.Error("请求处理发生 panic",
						"error", err,
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)
					http.Error(w, "内部服务器错误", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
