package metrics_test

import (
	"sync"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/pkg/metrics"
)

// testCfg 返回一个用于测试的 MetricsConfig（指向不存在的 endpoint，但不影响初始化）
func testCfg() config.MetricsConfig {
	return config.MetricsConfig{
		Enabled: true,
		Exporter: config.ExporterConfig{
			Protocol: "grpc",
			Endpoint: "localhost:4317",
		},
	}
}

// TestInitMetrics 验证 InitMetrics 返回 shutdown 函数且无错误
func TestInitMetrics(t *testing.T) {
	shutdown, err := metrics.InitMetrics(testCfg())
	if err != nil {
		t.Fatalf("InitMetrics 返回错误: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitMetrics 返回了空 shutdown 函数")
	}
}

// TestRecordRequestNoPanic 验证 RecordRequest 不会 panic
func TestRecordRequestNoPanic(t *testing.T) {
	metrics.RecordRequest("/api/test", "GET", 200, 100*time.Millisecond)
}

// TestActiveConnectionsNoPanic 验证活跃连接数增减不会 panic
func TestActiveConnectionsNoPanic(t *testing.T) {
	metrics.IncrActiveConn("ws")
	metrics.IncrActiveConn("ws")
	metrics.DecrActiveConn("ws")
}

// TestCircuitBreakerStateNoPanic 验证熔断器状态记录不会 panic
func TestCircuitBreakerStateNoPanic(t *testing.T) {
	metrics.SetCircuitBreakerState("user-service", "open")
}

// TestInitMetricsConcurrent 验证并发调用 InitMetrics 的线程安全性
func TestInitMetricsConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = metrics.InitMetrics(testCfg())
		}()
	}
	wg.Wait()
}
