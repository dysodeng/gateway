package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func setupTestTracer(t *testing.T) {
	t.Helper()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { tp.Shutdown(nil) })
}

func TestTracing_InjectsTraceID(t *testing.T) {
	setupTestTracer(t)

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

func TestTracing_PropagatesUpstreamTraceID(t *testing.T) {
	setupTestTracer(t)

	upstreamTraceID := "abcdef1234567890abcdef1234567890"

	var gotTraceID string
	handler := NewTracing()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceID = r.Header.Get("X-Trace-Id")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Trace-Id", upstreamTraceID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotTraceID != upstreamTraceID {
		t.Errorf("Trace ID = %q, 期望接续上游 %q", gotTraceID, upstreamTraceID)
	}
}

func TestTracing_PropagatesW3CTraceparent(t *testing.T) {
	setupTestTracer(t)

	// W3C traceparent 格式: version-traceId-spanId-flags
	w3cTraceID := "abcdef1234567890abcdef1234567890"
	traceparent := "00-" + w3cTraceID + "-00f067aa0ba902b7-01"

	var gotTraceID string
	handler := NewTracing()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceID = r.Header.Get("X-Trace-Id")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("traceparent", traceparent)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotTraceID != w3cTraceID {
		t.Errorf("Trace ID = %q, 期望接续 W3C traceparent %q", gotTraceID, w3cTraceID)
	}
}

func TestTracing_InvalidXTraceId_GeneratesNew(t *testing.T) {
	setupTestTracer(t)

	var gotTraceID string
	handler := NewTracing()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceID = r.Header.Get("X-Trace-Id")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Trace-Id", "invalid-trace-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotTraceID == "" || gotTraceID == "invalid-trace-id" {
		t.Errorf("期望生成新的 Trace ID，但得到 %q", gotTraceID)
	}
	if len(gotTraceID) != 32 {
		t.Errorf("Trace ID 长度 = %d, 期望 32", len(gotTraceID))
	}
}
