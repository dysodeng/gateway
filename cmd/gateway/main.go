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

// application 封装网关应用的生命周期管理
type application struct {
	configPath string
	cfg        *config.Config
	loadResult *config.LoadResult
	server     *server.Server
	discovery  discovery.Discovery
	shutdowns  []func(context.Context) error
}

func main() {
	configPath := flag.String("config", "gateway.yaml", "配置文件路径")
	flag.Parse()
	newApplication(*configPath).run()
}

// newApplication 创建应用实例
func newApplication(configPath string) *application {
	return &application{configPath: configPath}
}

// run 编排应用生命周期：初始化 → 等待退出信号 → 启动服务（阻塞）
func (app *application) run() {
	app.initialize()
	app.serve()
	app.waitForInterruptSignal()
}

// initialize 加载配置并初始化所有组件
func (app *application) initialize() {
	result, err := config.Load(app.configPath)
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		os.Exit(1)
	}
	app.cfg = result.Config
	app.loadResult = result

	logger.InitLogger(app.cfg.Log.Debug)
	slog.Info("配置加载完成", "source", result.Source, "path", result.SourcePath)

	app.discovery = app.initDiscovery()
	app.shutdowns = append(app.shutdowns, func(ctx context.Context) error {
		return app.discovery.Stop()
	})

	if app.cfg.Telemetry.Enabled {
		shutdown := app.initTelemetry()
		app.shutdowns = append(app.shutdowns, shutdown)
	}

	if app.cfg.Metrics.Enabled {
		shutdown := app.initMetrics()
		app.shutdowns = append(app.shutdowns, shutdown)
	}

	app.printStartupInfo()
}

// serve 创建服务器并启动 HTTP 服务
func (app *application) serve() {
	app.server = server.New(app.cfg, app.discovery)

	// 如果配置来自配置中心，启动热更新监听
	if app.loadResult.WatchSource != nil {
		watcher := config.NewWatcher(app.loadResult.WatchSource, func(newCfg *config.Config) {
			app.server.Reload(newCfg)
		})
		watchCtx, watchCancel := context.WithCancel(context.Background())
		if err := watcher.Start(watchCtx); err != nil {
			slog.Error("启动配置监听失败", "error", err)
			watchCancel()
		} else {
			slog.Info("配置热更新已启用", "source", app.loadResult.Source, "path", app.loadResult.SourcePath)
			app.shutdowns = append(app.shutdowns, func(ctx context.Context) error {
				watchCancel()
				return nil
			})
		}
	}

	go func() {
		if err := app.server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("服务器异常退出", "error", err)
			os.Exit(1)
		}
	}()
}

// waitForInterruptSignal 监听系统信号并优雅关闭所有组件
func (app *application) waitForInterruptSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), app.cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := app.server.Shutdown(ctx); err != nil {
		slog.Error("关闭服务器失败", "error", err)
	}
	for _, fn := range app.shutdowns {
		if err := fn(ctx); err != nil {
			slog.Error("关闭组件失败", "error", err)
		}
	}
}

// initDiscovery 根据配置初始化服务发现
func (app *application) initDiscovery() discovery.Discovery {
	switch app.cfg.Discovery.Type {
	case "static":
		if app.cfg.Discovery.Static == nil {
			slog.Error("静态服务发现配置缺失")
			os.Exit(1)
		}
		slog.Info("服务发现已初始化", "type", "static", "services", len(app.cfg.Discovery.Static.Services))
		return discovery.NewStaticDiscovery(app.cfg.Discovery.Static)
	case "etcd":
		if app.cfg.Discovery.Etcd == nil {
			slog.Error("etcd 服务发现配置缺失")
			os.Exit(1)
		}
		disc, err := discovery.NewEtcdDiscovery(app.cfg.Discovery.Etcd)
		if err != nil {
			slog.Error("初始化 etcd 服务发现失败", "error", err)
			os.Exit(1)
		}
		slog.Info("服务发现已初始化", "type", "etcd", "endpoints", app.cfg.Discovery.Etcd.Endpoints)
		return disc
	default:
		slog.Error("不支持的服务发现类型", "type", app.cfg.Discovery.Type)
		os.Exit(1)
		return nil
	}
}

// initTelemetry 初始化 OpenTelemetry 链路追踪
func (app *application) initTelemetry() func(context.Context) error {
	shutdown, err := trace.InitProvider(app.cfg.Telemetry)
	if err != nil {
		slog.Error("初始化 OTel 链路追踪失败", "error", err)
		os.Exit(1)
	}
	slog.Info("链路追踪已启用", "protocol", app.cfg.Telemetry.Exporter.Protocol, "endpoint", app.cfg.Telemetry.Exporter.Endpoint)
	return shutdown
}

// initMetrics 初始化 OpenTelemetry 指标采集
func (app *application) initMetrics() func(context.Context) error {
	shutdown, err := metrics.InitMetrics(app.cfg.Metrics)
	if err != nil {
		slog.Error("初始化 OTel 指标采集失败", "error", err)
		os.Exit(1)
	}
	slog.Info("指标采集已启用", "protocol", app.cfg.Metrics.Exporter.Protocol, "endpoint", app.cfg.Metrics.Exporter.Endpoint)
	return shutdown
}

// printStartupInfo 打印启动摘要
func (app *application) printStartupInfo() {
	slog.Info("路由已加载", "count", len(app.cfg.Routes))
	for _, route := range app.cfg.Routes {
		routeType := route.Type
		if routeType == "" {
			routeType = "http"
		}
		attrs := []any{
			"name", route.Name,
			"prefix", route.Prefix,
			"service", route.Service,
			"type", routeType,
			"lb", route.LoadBalancer,
			"timeout", route.Timeout,
		}
		if route.Middleware.Auth != nil {
			attrs = append(attrs, "auth", route.Middleware.Auth.Scheme)
			if route.Middleware.Auth.Optional {
				attrs = append(attrs, "auth_optional", true)
			}
		}
		if route.Middleware.RateLimit != nil && route.Middleware.RateLimit.Enabled {
			attrs = append(attrs, "rate_limit", route.Middleware.RateLimit.QPS)
		}
		if route.Middleware.Rewrite != nil {
			attrs = append(attrs, "rewrite", true)
		}
		slog.Info("  路由", attrs...)
	}
}
