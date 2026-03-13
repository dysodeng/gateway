package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel"
)

// NewTracing 创建链路追踪中间件。
// 为每个请求创建 span，并将 Trace ID 注入 X-Trace-Id 请求头传递给后端。
func NewTracing() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tracer := otel.Tracer("gateway")
			ctx, span := tracer.Start(r.Context(), r.Method+" "+r.URL.Path)
			defer span.End()

			// 注入 Trace ID 到请求头，供后端服务使用
			traceID := span.SpanContext().TraceID()
			if traceID.IsValid() {
				r.Header.Set("X-Trace-Id", traceID.String())
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
