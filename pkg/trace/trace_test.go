package trace

import (
	"context"
	"testing"

	"github.com/dysodeng/gateway/config"
)

func TestInitProvider_GRPC(t *testing.T) {
	cfg := config.TelemetryConfig{
		Enabled:     true,
		ServiceName: "test-gateway",
		Exporter: config.ExporterConfig{
			Type:     "otlp",
			Protocol: "grpc",
			Endpoint: "localhost:4317",
		},
		Sampler: config.SamplerConfig{
			Type: "never",
		},
	}

	shutdown, err := InitProvider(cfg)
	if err != nil {
		t.Fatalf("InitProvider() error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}
}

func TestInitProvider_HTTP(t *testing.T) {
	cfg := config.TelemetryConfig{
		Enabled:     true,
		ServiceName: "test-gateway",
		Exporter: config.ExporterConfig{
			Type:     "otlp",
			Protocol: "http",
			Endpoint: "localhost:4318",
		},
		Sampler: config.SamplerConfig{
			Type: "never",
		},
	}

	shutdown, err := InitProvider(cfg)
	if err != nil {
		t.Fatalf("InitProvider() error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}
}
