// Package server 将路由、中间件、代理、服务发现等组件组装为完整的 HTTP 请求处理管线。
package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
	"github.com/dysodeng/gateway/middleware"
	"github.com/dysodeng/gateway/pkg/health"
	"github.com/dysodeng/gateway/proxy"
	"github.com/dysodeng/gateway/router"
	"github.com/dysodeng/gateway/router/loadbalancer"
)

// Server 网关 HTTP 服务器
type Server struct {
	cfg        *config.Config
	httpServer *http.Server
	discovery  discovery.Discovery
}

// New 创建网关服务器，组装完整的请求处理管线
func New(cfg *config.Config, disc discovery.Discovery) *Server {
	s := &Server{cfg: cfg, discovery: disc}

	r := router.New(cfg.Routes)
	dispatcher := proxy.NewDispatcher()

	// 构建每条路由的负载均衡器
	balancers := buildBalancers(cfg.Routes)

	// 核心处理器：路由匹配 → 后置中间件 → 代理转发
	coreHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		route, matched := r.Match(req)
		if !matched {
			http.NotFound(w, req)
			return
		}

		// 灰度决策
		serviceName := router.ResolveCanary(route.Canary, route.Service, req)

		// 获取服务实例
		instances, err := disc.GetInstances(serviceName)
		if err != nil || len(instances) == 0 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		// 负载均衡选择实例
		balancer := balancers[route.Name]
		instance, err := balancer.Select(instances, req)
		if err != nil {
			http.Error(w, "no available instance", http.StatusServiceUnavailable)
			return
		}

		// 请求体大小限制（WebSocket 和 SSE 跳过）
		if route.Type != "websocket" && route.Type != "sse" {
			maxSize := cfg.Server.MaxRequestBodySize
			if route.MaxRequestBodySize != nil {
				maxSize = *route.MaxRequestBodySize
			}
			if maxSize > 0 {
				req.Body = http.MaxBytesReader(w, req.Body, maxSize)
			}
		}

		// 应用后置中间件
		postRoute := buildPostRouteMiddleware(cfg, route)
		finalHandler := postRoute(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			dispatcher.Forward(w, req, route, instance)
		}))
		finalHandler.ServeHTTP(w, req)
	})

	// 构建前置中间件链
	preRoute := middleware.Chain(
		middleware.NewRecovery(),
		middleware.NewAccessLog(),
		middleware.NewCORS(cfg.CORS),
		middleware.NewTracing(),
		middleware.NewGlobalIPFilter(cfg.IPFilter),
	)

	// 健康检查端点绕过中间件
	checkers := buildHealthCheckers(cfg, disc)
	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Health.Path, health.Handler(checkers...))
	mux.Handle("/", preRoute(coreHandler))

	s.httpServer = &http.Server{
		Addr:    cfg.Server.Listen,
		Handler: mux,
	}

	return s
}

// Start 启动 HTTP 服务器
func (s *Server) Start() error {
	slog.Info("网关启动", "addr", s.cfg.Server.Listen)
	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅关闭 HTTP 服务器
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("网关正在关闭")
	return s.httpServer.Shutdown(ctx)
}

// buildBalancers 为每条路由创建对应的负载均衡器
func buildBalancers(routes []config.RouteConfig) map[string]loadbalancer.Balancer {
	balancers := make(map[string]loadbalancer.Balancer, len(routes))
	for _, route := range routes {
		switch route.LoadBalancer {
		case "weighted":
			balancers[route.Name] = loadbalancer.NewWeightedRoundRobin()
		case "random":
			balancers[route.Name] = loadbalancer.NewRandom()
		case "ip_hash":
			balancers[route.Name] = loadbalancer.NewIPHash()
		case "least_conn":
			balancers[route.Name] = loadbalancer.NewLeastConn()
		default:
			balancers[route.Name] = loadbalancer.NewRoundRobin()
		}
	}
	return balancers
}

// buildPostRouteMiddleware 构建路由级后置中间件链
func buildPostRouteMiddleware(cfg *config.Config, route *config.RouteConfig) middleware.Middleware {
	var mws []middleware.Middleware

	// 路由级 IP 过滤
	if route.Middleware.IPFilter != nil {
		mws = append(mws, middleware.NewRouteIPFilter(*route.Middleware.IPFilter))
	}

	// 认证
	if route.Middleware.Auth != nil {
		scheme, ok := cfg.AuthSchemes[route.Middleware.Auth.Scheme]
		if ok {
			mws = append(mws, middleware.NewAuth(scheme))
		}
	}

	// 限流
	if route.Middleware.RateLimit != nil && route.Middleware.RateLimit.Enabled {
		mws = append(mws, middleware.NewRateLimit(cfg.RateLimit, *route.Middleware.RateLimit, route.Name))
	}

	// 请求签名验证
	if route.Middleware.RequestSign != nil && route.Middleware.RequestSign.Enabled {
		mws = append(mws, middleware.NewRequestSign(cfg.RequestSign))
	}

	// 请求重写
	if route.Middleware.Rewrite != nil {
		mws = append(mws, middleware.NewRewrite(*route.Middleware.Rewrite))
	}

	if len(mws) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return middleware.Chain(mws...)
}

// buildHealthCheckers 根据配置构建健康检查器列表
func buildHealthCheckers(cfg *config.Config, disc discovery.Discovery) []health.Checker {
	var checkers []health.Checker
	for _, check := range cfg.Health.Checks {
		switch check.Name {
		case "discovery":
			pinger, _ := disc.(health.DiscoveryPinger)
			checkers = append(checkers, health.NewDiscoveryChecker(pinger))
		}
	}
	return checkers
}
