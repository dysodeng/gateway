package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
)

func TestServer_Reload(t *testing.T) {
	disc := &mockDiscovery{
		instances: map[string][]discovery.ServiceInstance{
			"test-svc": {{Host: "127.0.0.1", Port: 9999, Weight: 1}},
		},
	}

	cfg := &config.Config{
		Server: config.ServerConfig{Listen: ":0"},
		CORS:   config.CORSConfig{AllowedOrigins: []string{"*"}},
		Health: config.HealthConfig{Path: "/health"},
		Routes: []config.RouteConfig{
			{
				Name:         "test",
				Prefix:       "/api/test",
				Service:      "test-svc",
				Timeout:      5 * time.Second,
				LoadBalancer: "round_robin",
			},
		},
	}

	srv := New(cfg, disc)

	// 初始请求 — /api/test 应匹配（后端不可达返回 502，但不是 404）
	req := httptest.NewRequest(http.MethodGet, "/api/test/hello", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Error("初始配置下 /api/test 不应返回 404")
	}

	// Reload：改变路由前缀
	newCfg := *cfg
	newCfg.Routes = []config.RouteConfig{
		{
			Name:         "test-v2",
			Prefix:       "/api/v2/test",
			Service:      "test-svc",
			Timeout:      5 * time.Second,
			LoadBalancer: "round_robin",
		},
	}
	srv.Reload(&newCfg)

	// 旧路由应返回 404
	req = httptest.NewRequest(http.MethodGet, "/api/test/hello", nil)
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Reload 后旧路由应返回 404，实际: %d", w.Code)
	}

	// 新路由应匹配
	req = httptest.NewRequest(http.MethodGet, "/api/v2/test/hello", nil)
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Error("Reload 后新路由不应返回 404")
	}
}
