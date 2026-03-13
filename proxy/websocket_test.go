package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
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

// newEchoBackend 创建 WebSocket echo 后端服务器
func newEchoBackend(t *testing.T) (*httptest.Server, *discovery.ServiceInstance) {
	t.Helper()
	backendUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := backendUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			conn.WriteMessage(msgType, msg)
		}
	}))
	host, port := parseHostPort(t, backend.URL)
	return backend, &discovery.ServiceInstance{Host: host, Port: port}
}

// TestWebSocketProxy_Heartbeat 验证心跳 ping 帧按配置间隔发送
func TestWebSocketProxy_Heartbeat(t *testing.T) {
	backend, instance := newEchoBackend(t)
	defer backend.Close()

	wsProxy := NewWebSocketProxy()
	wsProxy.Configure(&config.WebSocketConfig{
		Heartbeat: 100 * time.Millisecond,
	})

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsProxy.Forward(w, r, instance)
	}))
	defer gateway.Close()

	gatewayWsURL := "ws" + strings.TrimPrefix(gateway.URL, "http") + "/ws"

	clientConn, _, err := websocket.DefaultDialer.Dial(gatewayWsURL, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer clientConn.Close()

	// 记录收到的 ping 帧
	pingCount := 0
	var mu sync.Mutex
	clientConn.SetPingHandler(func(appData string) error {
		mu.Lock()
		pingCount++
		mu.Unlock()
		// 回复 pong
		return clientConn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	// 在后台读取消息以触发 ping handler
	go func() {
		for {
			_, _, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// 等待足够时间收到至少 2 个 ping
	time.Sleep(350 * time.Millisecond)

	mu.Lock()
	count := pingCount
	mu.Unlock()

	if count < 2 {
		t.Errorf("期望至少收到 2 个 ping，实际收到 %d", count)
	}
}

// TestWebSocketProxy_MaxConnections 验证超过最大连接数时返回 503
func TestWebSocketProxy_MaxConnections(t *testing.T) {
	backend, instance := newEchoBackend(t)
	defer backend.Close()

	wsProxy := NewWebSocketProxy()
	wsProxy.Configure(&config.WebSocketConfig{
		MaxConnections: 1,
	})

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsProxy.Forward(w, r, instance)
	}))
	defer gateway.Close()

	gatewayWsURL := "ws" + strings.TrimPrefix(gateway.URL, "http") + "/ws"

	// 第一个连接应成功
	conn1, _, err := websocket.DefaultDialer.Dial(gatewayWsURL, nil)
	if err != nil {
		t.Fatalf("第一个连接失败: %v", err)
	}
	defer conn1.Close()

	// 第二个连接应被拒绝（503）
	_, resp, err := websocket.DefaultDialer.Dial(gatewayWsURL, nil)
	if err == nil {
		t.Fatal("期望第二个连接被拒绝，但成功了")
	}
	if resp != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("期望状态码 503，实际 %d", resp.StatusCode)
	}
}
