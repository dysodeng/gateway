package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dysodeng/gateway/discovery"
	"github.com/gorilla/websocket"
)

// TestWebSocketProxy_BiDirectional 测试 WebSocket 双向透传功能
func TestWebSocketProxy_BiDirectional(t *testing.T) {
	// 创建模拟后端 WebSocket echo 服务器
	backendUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := backendUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("后端升级 WebSocket 失败: %v", err)
			return
		}
		defer conn.Close()

		// echo 服务：收到消息原样返回
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if err := conn.WriteMessage(msgType, msg); err != nil {
				break
			}
		}
	}))
	defer backend.Close()

	// 解析后端地址
	host, port := parseHostPort(t, backend.URL)

	instance := &discovery.ServiceInstance{
		Host: host,
		Port: port,
	}

	// 创建网关 WebSocket 代理服务器
	wsProxy := NewWebSocketProxy()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsProxy.Forward(w, r, instance)
	}))
	defer gateway.Close()

	// 将 http:// 替换为 ws:// 以建立 WebSocket 连接
	gatewayWsURL := "ws" + strings.TrimPrefix(gateway.URL, "http") + "/echo"

	// 客户端连接到网关
	clientConn, _, err := websocket.DefaultDialer.Dial(gatewayWsURL, nil)
	if err != nil {
		t.Fatalf("客户端连接网关失败: %v", err)
	}
	defer clientConn.Close()

	// 发送测试消息
	testMsg := "hello websocket"
	if err := clientConn.WriteMessage(websocket.TextMessage, []byte(testMsg)); err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}

	// 设置读取超时并验证 echo 返回
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msgType, received, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Errorf("期望消息类型 %d，实际得到 %d", websocket.TextMessage, msgType)
	}
	if string(received) != testMsg {
		t.Errorf("期望收到 %q，实际得到 %q", testMsg, string(received))
	}
}
