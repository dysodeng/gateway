package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/dysodeng/gateway/discovery"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WebSocketProxy WebSocket 透明代理
type WebSocketProxy struct{}

// NewWebSocketProxy 创建 WebSocket 代理实例
func NewWebSocketProxy() *WebSocketProxy {
	return &WebSocketProxy{}
}

// Forward 在客户端和后端之间建立 WebSocket 双向透传
func (p *WebSocketProxy) Forward(w http.ResponseWriter, r *http.Request, instance *discovery.ServiceInstance) {
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
			dst.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
		writer, err := dst.NextWriter(messageType)
		if err != nil {
			return
		}
		io.Copy(writer, reader)
		writer.Close()
	}
}
