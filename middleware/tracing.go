package middleware

import (
	"encoding/hex"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// NewTracing 创建链路追踪中间件。
// 优先从上游请求头提取已有的 trace context（W3C traceparent 或自定义 X-Trace-Id），
// 在此基础上创建 span，并将 Trace ID 注入 X-Trace-Id 请求头传递给后端。
func NewTracing() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// 从请求头提取上游 trace context（W3C traceparent/tracestate）
			propagator := otel.GetTextMapPropagator()
			ctx = propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))

			// 如果标准传播未提取到有效 trace，尝试从 X-Trace-Id 恢复
			if !oteltrace.SpanContextFromContext(ctx).HasTraceID() {
				if remoteCtx, ok := traceIDFromHeader(r.Header); ok {
					ctx = oteltrace.ContextWithRemoteSpanContext(ctx, remoteCtx)
				}
			}

			tracer := otel.Tracer("gateway")
			ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path)
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

// traceIDFromHeader 尝试从 X-Trace-Id 请求头解析出 trace ID，构造远程 SpanContext
func traceIDFromHeader(header http.Header) (oteltrace.SpanContext, bool) {
	raw := header.Get("X-Trace-Id")
	if len(raw) != 32 {
		return oteltrace.SpanContext{}, false
	}

	b, err := hex.DecodeString(raw)
	if err != nil {
		return oteltrace.SpanContext{}, false
	}

	var traceID oteltrace.TraceID
	copy(traceID[:], b)
	if !traceID.IsValid() {
		return oteltrace.SpanContext{}, false
	}

	sc := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		TraceFlags: oteltrace.FlagsSampled,
		Remote:     true,
	})
	return sc, true
}
