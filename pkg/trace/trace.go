package trace

import (
	"context"
	"fmt"

	"github.com/dysodeng/gateway/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitProvider 初始化 OpenTelemetry TracerProvider。
// 根据配置选择 gRPC 或 HTTP 协议的 OTLP exporter。
// 返回的 shutdown 函数应在程序退出时调用以刷新剩余 span。
func InitProvider(cfg config.TelemetryConfig) (shutdown func(context.Context) error, err error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 OTel 资源失败: %w", err)
	}

	var exporter sdktrace.SpanExporter
	switch cfg.Exporter.Protocol {
	case "http":
		exporter, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(cfg.Exporter.Endpoint),
			otlptracehttp.WithInsecure(),
		)
	default: // "grpc" 或默认协议
		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.Exporter.Endpoint),
			otlptracegrpc.WithInsecure(),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("创建 OTel exporter 失败: %w", err)
	}

	// 配置采样器
	var sampler sdktrace.Sampler
	switch cfg.Sampler.Type {
	case "always":
		sampler = sdktrace.AlwaysSample()
	case "never":
		sampler = sdktrace.NeverSample()
	default: // "ratio" 按比例采样
		sampler = sdktrace.TraceIDRatioBased(cfg.Sampler.Ratio)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)

	// 注册 W3C TraceContext 传播器，支持从上游请求头提取/注入 trace context
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}
