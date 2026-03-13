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
	pkglog "github.com/dysodeng/gateway/pkg/log"
	"github.com/dysodeng/gateway/pkg/trace"
	"github.com/dysodeng/gateway/server"
)

func main() {
	configPath := flag.String("config", "gateway.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		os.Exit(1)
	}

	// 初始化日志
	if err = pkglog.InitLogger(cfg.Log); err != nil {
		slog.Error("初始化日志失败", "error", err)
		os.Exit(1)
	}

	// 初始化服务发现
	var disc discovery.Discovery
	switch cfg.Discovery.Type {
	case "static":
		if cfg.Discovery.Static == nil {
			slog.Error("静态服务发现配置缺失")
			os.Exit(1)
		}
		disc = discovery.NewStaticDiscovery(cfg.Discovery.Static)
	default:
		slog.Error("不支持的服务发现类型", "type", cfg.Discovery.Type)
		os.Exit(1)
	}

	// 初始化 OpenTelemetry 链路追踪
	var shutdownTracer func(context.Context) error
	if cfg.Telemetry.Enabled {
		shutdown, err := trace.InitProvider(cfg.Telemetry)
		if err != nil {
			slog.Error("初始化 OTel 链路追踪失败", "error", err)
			os.Exit(1)
		}
		shutdownTracer = shutdown
	}

	srv := server.New(cfg, disc)

	// 优雅关闭
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()

		// 停止接受新连接，等待在途请求完成
		if err = srv.Shutdown(ctx); err != nil {
			slog.Error("关闭服务器失败", "error", err)
		}

		// 停止服务发现
		if disc != nil {
			if err = disc.Stop(); err != nil {
				slog.Error("停止服务发现失败", "error", err)
			}
		}

		// 刷新 OTel 数据
		if shutdownTracer != nil {
			if err = shutdownTracer(ctx); err != nil {
				slog.Error("关闭 OTel 失败", "error", err)
			}
		}
	}()

	if err = srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("服务器异常退出", "error", err)
		os.Exit(1)
	}

	slog.Info("网关已停止")
}
