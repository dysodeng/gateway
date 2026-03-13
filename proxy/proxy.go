package proxy

import (
	"net/http"
	"strings"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
)

// Dispatcher 代理调度器，根据协议类型分发请求到对应的代理
type Dispatcher struct {
	httpProxy *HTTPProxy
	wsProxy   *WebSocketProxy
	sseProxy  *SSEProxy
}

// NewDispatcher 创建代理调度器
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		httpProxy: NewHTTPProxy(),
		wsProxy:   NewWebSocketProxy(),
		sseProxy:  NewSSEProxy(),
	}
}

// Forward 根据请求类型分发到对应的代理处理
func (d *Dispatcher) Forward(w http.ResponseWriter, r *http.Request, route *config.RouteConfig, instance *discovery.ServiceInstance) {
	if isWebSocketRequest(r) {
		d.wsProxy.Forward(w, r, instance)
		return
	}

	if route.Type == "sse" {
		d.sseProxy.Forward(w, r, instance, route.StripPrefix, route.Prefix)
		return
	}

	d.httpProxy.Forward(w, r, instance, route.StripPrefix, route.Prefix)
}

// isWebSocketRequest 检查是否为 WebSocket 升级请求。
// Connection 头可能包含多个逗号分隔的值（如 "keep-alive, Upgrade"），需要逐一检查。
func isWebSocketRequest(r *http.Request) bool {
	for _, v := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(v), "upgrade") {
			return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		}
	}
	return false
}
