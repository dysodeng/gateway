package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dysodeng/gateway/pkg/metrics"
)

// TestInitMetrics 验证 InitMetrics 返回非空 Handler
func TestInitMetrics(t *testing.T) {
	handler, err := metrics.InitMetrics()
	if err != nil {
		t.Fatalf("InitMetrics 返回错误: %v", err)
	}
	if handler == nil {
		t.Fatal("InitMetrics 返回了空 Handler")
	}
}

// TestRecordRequest 验证 RecordRequest 后 /metrics 端点包含 gateway_request_total 指标
func TestRecordRequest(t *testing.T) {
	handler, err := metrics.InitMetrics()
	if err != nil {
		t.Fatalf("InitMetrics 返回错误: %v", err)
	}

	// 记录一次请求
	metrics.RecordRequest("/api/test", "GET", 200, 100*time.Millisecond)

	// 请求 /metrics 端点，验证指标存在
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "gateway_request_total") {
		t.Errorf("metrics 端点响应不包含 gateway_request_total，实际响应:\n%s", bodyStr)
	}
}

// TestRecordRequestDuration 验证 RecordRequest 后 /metrics 端点包含 gateway_request_duration 指标
func TestRecordRequestDuration(t *testing.T) {
	handler, err := metrics.InitMetrics()
	if err != nil {
		t.Fatalf("InitMetrics 返回错误: %v", err)
	}

	// 记录一次请求
	metrics.RecordRequest("/api/duration", "POST", 201, 50*time.Millisecond)

	// 请求 /metrics 端点，验证持续时间指标存在
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "gateway_request_duration") {
		t.Errorf("metrics 端点响应不包含 gateway_request_duration，实际响应:\n%s", bodyStr)
	}
}

// TestActiveConnections 验证活跃连接数的增减操作
func TestActiveConnections(t *testing.T) {
	handler, err := metrics.InitMetrics()
	if err != nil {
		t.Fatalf("InitMetrics 返回错误: %v", err)
	}

	// 增加 ws 连接
	metrics.IncrActiveConn("ws")
	metrics.IncrActiveConn("ws")
	// 减少 ws 连接
	metrics.DecrActiveConn("ws")

	// 请求 /metrics 端点，验证指标存在
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "gateway_active_connections") {
		t.Errorf("metrics 端点响应不包含 gateway_active_connections，实际响应:\n%s", bodyStr)
	}
}

// TestCircuitBreakerState 验证熔断器状态指标
func TestCircuitBreakerState(t *testing.T) {
	handler, err := metrics.InitMetrics()
	if err != nil {
		t.Fatalf("InitMetrics 返回错误: %v", err)
	}

	// 设置熔断器状态
	metrics.SetCircuitBreakerState("user-service", "open")

	// 请求 /metrics 端点，验证指标存在
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "gateway_circuit_breaker_state") {
		t.Errorf("metrics 端点响应不包含 gateway_circuit_breaker_state，实际响应:\n%s", bodyStr)
	}
}
