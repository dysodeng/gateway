package middleware

import "net/http"

// Middleware 中间件函数类型，接收一个 Handler 返回包装后的 Handler
type Middleware func(next http.Handler) http.Handler

// Chain 将多个中间件串联成链。
// 中间件按提供的顺序执行：Chain(m1, m2, m3)(handler) => m1(m2(m3(handler)))
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
