// Package metrics 提供基于 OpenTelemetry OTLP 协议的指标采集功能。
// 注册网关核心指标：请求总数、请求时延、活跃连接数、熔断器状态。
package metrics

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dysodeng/gateway/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// 全局 meter 实例，由 InitMetrics 初始化后供各函数使用
var (
	globalMeter metric.Meter

	// requestCounter gateway_request_total 请求计数器（标签：route, status_code, method）
	requestCounter metric.Int64Counter

	// requestDuration gateway_request_duration 请求时延直方图（秒，标签：route, status_code, method）
	requestDuration metric.Float64Histogram

	// activeConnections gateway_active_connections 活跃连接数（标签：type）
	activeConnections metric.Int64UpDownCounter

	// circuitBreakerState gateway_circuit_breaker_state 熔断器状态（标签：service, state）
	circuitBreakerState metric.Int64UpDownCounter

	// initOnce 保证 InitMetrics 只真正初始化一次 meter 实例（线程安全）
	initOnce sync.Once
	initErr  error

	// globalProvider 保存 MeterProvider 引用，用于 shutdown
	globalProvider *sdkmetric.MeterProvider
)

// InitMetrics 初始化 OpenTelemetry MeterProvider（使用 OTLP exporter），
// 注册所有网关核心指标，并返回 shutdown 函数用于优雅关闭时刷新指标数据。
// 可多次调用，但 meter 实例仅初始化一次（线程安全）。
func InitMetrics(cfg config.MetricsConfig) (shutdown func(context.Context) error, err error) {
	initOnce.Do(func() {
		ctx := context.Background()

		// 根据协议创建 OTLP metric exporter
		var exporter sdkmetric.Exporter
		switch cfg.Exporter.Protocol {
		case "http":
			exporter, initErr = otlpmetrichttp.New(ctx,
				otlpmetrichttp.WithEndpoint(cfg.Exporter.Endpoint),
				otlpmetrichttp.WithInsecure(),
			)
		default: // "grpc" 或默认协议
			exporter, initErr = otlpmetricgrpc.New(ctx,
				otlpmetricgrpc.WithEndpoint(cfg.Exporter.Endpoint),
				otlpmetricgrpc.WithInsecure(),
			)
		}
		if initErr != nil {
			initErr = fmt.Errorf("创建 OTLP metric exporter 失败: %w", initErr)
			return
		}

		// 使用 PeriodicReader 定期推送指标
		reader := sdkmetric.NewPeriodicReader(exporter)

		// 构建 MeterProvider
		globalProvider = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(reader),
		)

		// 获取全局 meter 实例，Scope 名称使用模块路径
		globalMeter = globalProvider.Meter("github.com/dysodeng/gateway")

		// 注册 gateway_request_total 计数器
		requestCounter, initErr = globalMeter.Int64Counter(
			"gateway_request_total",
			metric.WithDescription("网关处理的请求总数"),
		)
		if initErr != nil {
			initErr = fmt.Errorf("创建 gateway_request_total 失败: %w", initErr)
			return
		}

		// 注册 gateway_request_duration 直方图（单位：秒）
		requestDuration, initErr = globalMeter.Float64Histogram(
			"gateway_request_duration",
			metric.WithDescription("网关请求处理时延（秒）"),
			metric.WithUnit("s"),
		)
		if initErr != nil {
			initErr = fmt.Errorf("创建 gateway_request_duration 失败: %w", initErr)
			return
		}

		// 注册 gateway_active_connections UpDownCounter
		activeConnections, initErr = globalMeter.Int64UpDownCounter(
			"gateway_active_connections",
			metric.WithDescription("当前活跃连接数（按连接类型区分）"),
		)
		if initErr != nil {
			initErr = fmt.Errorf("创建 gateway_active_connections 失败: %w", initErr)
			return
		}

		// 注册 gateway_circuit_breaker_state UpDownCounter
		circuitBreakerState, initErr = globalMeter.Int64UpDownCounter(
			"gateway_circuit_breaker_state",
			metric.WithDescription("熔断器状态（按服务和状态区分）"),
		)
		if initErr != nil {
			initErr = fmt.Errorf("创建 gateway_circuit_breaker_state 失败: %w", initErr)
			return
		}
	})

	if initErr != nil {
		return nil, initErr
	}

	return globalProvider.Shutdown, nil
}

// RecordRequest 记录一次 HTTP 请求的总数及时延。
// route：路由路径，method：HTTP 方法，statusCode：响应状态码，duration：请求耗时。
func RecordRequest(route, method string, statusCode int, duration time.Duration) {
	if requestCounter == nil || requestDuration == nil {
		return
	}

	ctx := context.Background()

	// 公共属性集合
	attrs := attribute.NewSet(
		attribute.String("route", route),
		attribute.String("method", method),
		attribute.String("status_code", strconv.Itoa(statusCode)),
	)

	// 增加请求计数
	requestCounter.Add(ctx, 1, metric.WithAttributeSet(attrs))

	// 记录时延（转换为秒）
	requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributeSet(attrs))
}

// IncrActiveConn 增加指定类型的活跃连接数。
// connType 取值：ws（WebSocket）、sse（Server-Sent Events）、http（普通 HTTP）。
func IncrActiveConn(connType string) {
	if activeConnections == nil {
		return
	}
	activeConnections.Add(
		context.Background(),
		1,
		metric.WithAttributes(attribute.String("type", connType)),
	)
}

// DecrActiveConn 减少指定类型的活跃连接数。
// connType 取值：ws（WebSocket）、sse（Server-Sent Events）、http（普通 HTTP）。
func DecrActiveConn(connType string) {
	if activeConnections == nil {
		return
	}
	activeConnections.Add(
		context.Background(),
		-1,
		metric.WithAttributes(attribute.String("type", connType)),
	)
}

// SetCircuitBreakerState 更新熔断器状态指标。
// service：服务名称，state：状态值（closed/open/half_open）。
// 每次调用将对应 (service, state) 组合的计数器加 1，
// 适用于记录状态变更事件；若需精确跟踪当前状态，
// 调用方应在切换前对旧状态执行相反操作（Add -1）。
func SetCircuitBreakerState(service, state string) {
	if circuitBreakerState == nil {
		return
	}
	circuitBreakerState.Add(
		context.Background(),
		1,
		metric.WithAttributes(
			attribute.String("service", service),
			attribute.String("state", state),
		),
	)
}
