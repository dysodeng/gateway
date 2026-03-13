package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTracing_InjectsTraceID(t *testing.T) {
	// 设置测试用的 TracerProvider（始终采样）
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(nil)

	var traceID string
	handler := NewTracing()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID = r.Header.Get("X-Trace-Id")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if traceID == "" {
		t.Error("期望 X-Trace-Id 被设置，但为空")
	}
	if len(traceID) != 32 {
		t.Errorf("Trace ID 长度 = %d, 期望 32", len(traceID))
	}
}
