package proxy

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
	"github.com/gorilla/websocket"
)

// TestDispatcher_HTTP 测试普通 GET 请求被调度到 HTTP 代理
func TestDispatcher_HTTP(t *testing.T) {
	// 创建模拟后端 HTTP 服务器
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from http backend"))
	}))
	defer backend.Close()

	host, port := parseHostPort(t, backend.URL)
	instance := &discovery.ServiceInstance{Host: host, Port: port}

	route := &config.RouteConfig{
		Type:        "http",
		StripPrefix: false,
		Prefix:      "",
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	d := NewDispatcher()
	d.Forward(rec, req, route, instance)

	resp := rec.Result()
	defer resp.Body.Close()

	// 验证状态码
	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 %d，实际得到 %d", http.StatusOK, resp.StatusCode)
	}

	// 验证后端实际收到了请求（通过响应体确认）
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	if string(body) != "hello from http backend" {
		t.Errorf("期望响应体 %q，实际得到 %q", "hello from http backend", string(body))
	}
}

// TestDispatcher_WebSocket 测试携带标准 WebSocket 升级头的请求被调度到 WS 代理
func TestDispatcher_WebSocket(t *testing.T) {
	// 创建模拟后端 WebSocket echo 服务器
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
	defer backend.Close()

	host, port := parseHostPort(t, backend.URL)
	instance := &discovery.ServiceInstance{Host: host, Port: port}

	route := &config.RouteConfig{Type: "http"}

	// 通过真实 HTTP 服务器测试 WebSocket 调度
	d := NewDispatcher()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.Forward(w, r, route, instance)
	}))
	defer gateway.Close()

	// 建立 WebSocket 连接（标准升级头：Connection: Upgrade）
	gatewayWsURL := "ws" + strings.TrimPrefix(gateway.URL, "http") + "/ws"
	clientConn, _, err := websocket.DefaultDialer.Dial(gatewayWsURL, nil)
	if err != nil {
		t.Fatalf("连接网关 WebSocket 失败: %v", err)
	}
	defer clientConn.Close()

	// 发送消息并验证 echo 返回
	testMsg := "websocket dispatch test"
	if err := clientConn.WriteMessage(websocket.TextMessage, []byte(testMsg)); err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, received, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}
	if string(received) != testMsg {
		t.Errorf("期望收到 %q，实际得到 %q", testMsg, string(received))
	}
}

// TestDispatcher_WebSocket_MultiValueConnection 测试 Connection 头包含多个值时仍能正确识别 WebSocket 升级请求
func TestDispatcher_WebSocket_MultiValueConnection(t *testing.T) {
	// 创建模拟后端 WebSocket echo 服务器
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
	defer backend.Close()

	host, port := parseHostPort(t, backend.URL)
	instance := &discovery.ServiceInstance{Host: host, Port: port}

	route := &config.RouteConfig{Type: "http"}

	// 使用自定义 dialer 来注入多值 Connection 头（keep-alive, Upgrade）
	d := NewDispatcher()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.Forward(w, r, route, instance)
	}))
	defer gateway.Close()

	// 直接测试 isWebSocketRequest 函数对多值 Connection 头的识别
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Connection", "keep-alive, Upgrade")
	req.Header.Set("Upgrade", "websocket")

	if !isWebSocketRequest(req) {
		t.Error("期望 Connection: keep-alive, Upgrade 且 Upgrade: websocket 被识别为 WebSocket 请求")
	}

	// 同时验证标准单值 Connection 头也能正确识别
	req2 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req2.Header.Set("Connection", "Upgrade")
	req2.Header.Set("Upgrade", "websocket")
	if !isWebSocketRequest(req2) {
		t.Error("期望 Connection: Upgrade 且 Upgrade: websocket 被识别为 WebSocket 请求")
	}

	// 验证普通请求不被识别为 WebSocket
	req3 := httptest.NewRequest(http.MethodGet, "/api", nil)
	if isWebSocketRequest(req3) {
		t.Error("期望无升级头的普通请求不被识别为 WebSocket 请求")
	}
}

// TestDispatcher_SSE 测试 SSE 路由被调度到 SSE 代理
func TestDispatcher_SSE(t *testing.T) {
	// 创建模拟后端 SSE 服务器
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "不支持 Flusher", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("data: sse event\n\n"))
		flusher.Flush()
	}))
	defer backend.Close()

	host, port := parseHostPort(t, backend.URL)
	instance := &discovery.ServiceInstance{Host: host, Port: port}

	// 路由类型为 sse，应调度到 SSE 代理
	route := &config.RouteConfig{
		Type:        "sse",
		StripPrefix: false,
		Prefix:      "",
	}

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()

	d := NewDispatcher()
	d.Forward(rec, req, route, instance)

	resp := rec.Result()
	defer resp.Body.Close()

	// 验证 Content-Type 为 SSE
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("期望 Content-Type 包含 text/event-stream，实际得到 %q", contentType)
	}

	// 验证收到 SSE 事件数据
	found := false
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("期望收到至少一条 SSE 事件数据")
	}
}
