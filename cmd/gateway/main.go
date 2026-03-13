package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
	"github.com/dysodeng/gateway/pkg/logger"
	"github.com/dysodeng/gateway/pkg/metrics"
	"github.com/dysodeng/gateway/pkg/trace"
	"github.com/dysodeng/gateway/server"
)

func main() {
	configPath := flag.String("config", "gateway.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		os.Exit(1)
	}

	logger.InitLogger(cfg.Log.Level == "debug")

	// 初始化各组件，收集 shutdown 函数
	var shutdowns []func(context.Context) error

	disc := initDiscovery(cfg)
	shutdowns = append(shutdowns, func(ctx context.Context) error {
		return disc.Stop()
	})

	if cfg.Telemetry.Enabled {
		shutdown := initTelemetry(cfg)
		shutdowns = append(shutdowns, shutdown)
	}

	if cfg.Metrics.Enabled {
		shutdown := initMetrics(cfg)
		shutdowns = append(shutdowns, shutdown)
	}

	srv := server.New(cfg, disc)

	// 监听退出信号，优雅关闭
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()

		if err = srv.Shutdown(ctx); err != nil {
			slog.Error("关闭服务器失败", "error", err)
		}
		for _, fn := range shutdowns {
			if err = fn(ctx); err != nil {
				slog.Error("关闭组件失败", "error", err)
			}
		}
	}()

	if err = srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("服务器异常退出", "error", err)
		os.Exit(1)
	}

	slog.Info("网关已停止")
}

// initDiscovery 根据配置初始化服务发现
func initDiscovery(cfg *config.Config) discovery.Discovery {
	switch cfg.Discovery.Type {
	case "static":
		if cfg.Discovery.Static == nil {
			slog.Error("静态服务发现配置缺失")
			os.Exit(1)
		}
		return discovery.NewStaticDiscovery(cfg.Discovery.Static)
	case "etcd":
		if cfg.Discovery.Etcd == nil {
			slog.Error("etcd 服务发现配置缺失")
			os.Exit(1)
		}
		disc, err := discovery.NewEtcdDiscovery(cfg.Discovery.Etcd)
		if err != nil {
			slog.Error("初始化 etcd 服务发现失败", "error", err)
			os.Exit(1)
		}
		return disc
	default:
		slog.Error("不支持的服务发现类型", "type", cfg.Discovery.Type)
		os.Exit(1)
		return nil
	}
}

// initTelemetry 初始化 OpenTelemetry 链路追踪
func initTelemetry(cfg *config.Config) func(context.Context) error {
	shutdown, err := trace.InitProvider(cfg.Telemetry)
	if err != nil {
		slog.Error("初始化 OTel 链路追踪失败", "error", err)
		os.Exit(1)
	}
	return shutdown
}

// initMetrics 初始化 OpenTelemetry 指标采集
func initMetrics(cfg *config.Config) func(context.Context) error {
	shutdown, err := metrics.InitMetrics(cfg.Metrics)
	if err != nil {
		slog.Error("初始化 OTel 指标采集失败", "error", err)
		os.Exit(1)
	}
	return shutdown
}
