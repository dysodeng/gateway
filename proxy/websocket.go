package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WebSocketProxy WebSocket 透明代理，支持心跳检测和连接数限制
type WebSocketProxy struct {
	activeConns atomic.Int64
	maxConns    int
	heartbeat   time.Duration
}

// NewWebSocketProxy 创建 WebSocket 代理实例
func NewWebSocketProxy() *WebSocketProxy {
	return &WebSocketProxy{}
}

// Configure 配置 WebSocket 代理参数（心跳间隔和最大连接数）
func (p *WebSocketProxy) Configure(cfg *config.WebSocketConfig) {
	if cfg == nil {
		return
	}
	p.heartbeat = cfg.Heartbeat
	p.maxConns = cfg.MaxConnections
}

// Forward 在客户端和后端之间建立 WebSocket 双向透传
func (p *WebSocketProxy) Forward(w http.ResponseWriter, r *http.Request, instance *discovery.ServiceInstance) {
	// 连接数限制检查
	if p.maxConns > 0 {
		current := p.activeConns.Add(1)
		if current > int64(p.maxConns) {
			p.activeConns.Add(-1)
			http.Error(w, "连接数已达上限", http.StatusServiceUnavailable)
			return
		}
		defer p.activeConns.Add(-1)
	}

	// 升级客户端连接
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket 升级失败", "error", err)
		return
	}
	defer clientConn.Close()

	// 连接后端
	backendURL := url.URL{
		Scheme: "ws",
		Host:   instance.Addr(),
		Path:   r.URL.Path,
	}

	backendConn, _, err := websocket.DefaultDialer.Dial(backendURL.String(), nil)
	if err != nil {
		slog.Error("WebSocket 连接后端失败", "error", err, "addr", backendURL.String())
		clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "后端服务不可用"))
		return
	}
	defer backendConn.Close()

	// 双向转发
	done := make(chan struct{})

	// 启动心跳检测
	if p.heartbeat > 0 {
		pongWait := p.heartbeat * 2
		clientConn.SetPongHandler(func(string) error {
			return clientConn.SetReadDeadline(time.Now().Add(pongWait))
		})
		go func() {
			ticker := time.NewTicker(p.heartbeat)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err = clientConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
						return
					}
				case <-done:
					return
				}
			}
		}()
	}

	// 后端 → 客户端
	go func() {
		defer close(done)
		relay(backendConn, clientConn)
	}()

	// 客户端 → 后端
	relay(clientConn, backendConn)
	<-done
}

// relay 在两个 WebSocket 连接之间转发消息
func relay(src, dst *websocket.Conn) {
	for {
		messageType, reader, err := src.NextReader()
		if err != nil {
			_ = dst.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
		writer, err := dst.NextWriter(messageType)
		if err != nil {
			return
		}
		_, _ = io.Copy(writer, reader)
		_ = writer.Close()
	}
}
