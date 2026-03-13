package proxy

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
)

// TestSSEProxy_Forward 测试 SSE 透明代理流式转发功能
func TestSSEProxy_Forward(t *testing.T) {
	const eventCount = 3

	// 创建模拟后端 SSE 服务器，发送 3 条事件
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("后端不支持 Flusher")
			return
		}

		for i := 0; i < eventCount; i++ {
			fmt.Fprintf(w, "data: hello\n\n")
			flusher.Flush()
		}
	}))
	defer backend.Close()

	// 解析后端地址
	host, port := parseHostPort(t, backend.URL)

	instance := &discovery.ServiceInstance{
		Host: host,
		Port: port,
	}

	// 创建网关 SSE 代理请求
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()

	proxy := NewSSEProxy()
	proxy.Forward(rec, req, instance, false, "")

	resp := rec.Result()
	defer resp.Body.Close()

	// 验证 Content-Type 为 text/event-stream
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("期望 Content-Type 包含 text/event-stream，实际得到 %q", contentType)
	}

	// 逐行读取并统计收到的事件数量
	received := 0
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			received++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("读取 SSE 响应体失败: %v", err)
	}

	if received != eventCount {
		t.Errorf("期望收到 %d 条事件，实际收到 %d 条", eventCount, received)
	}
}

// TestSSEProxy_RetryAndKeepalive 验证 retry 字段和 keepalive 心跳
func TestSSEProxy_RetryAndKeepalive(t *testing.T) {
	// 后端发送 1 条事件后等待一段时间再关闭，让 keepalive 有机会触发
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		fmt.Fprint(w, "data: event1\n\n")
		flusher.Flush()
		// 等待足够时间让 keepalive 触发
		time.Sleep(350 * time.Millisecond)
	}))
	defer backend.Close()

	host, port := parseHostPort(t, backend.URL)
	instance := &discovery.ServiceInstance{Host: host, Port: port}

	proxy := NewSSEProxy()
	proxy.Configure(&config.SSEConfig{
		Retry:     3000,
		Keepalive: 100 * time.Millisecond,
	})

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()

	proxy.Forward(rec, req, instance, false, "")

	resp := rec.Result()
	defer resp.Body.Close()

	body := rec.Body.String()

	// 验证 retry 字段
	if !strings.Contains(body, "retry: 3000") {
		t.Errorf("期望响应包含 'retry: 3000'，实际内容:\n%s", body)
	}

	// 验证 keepalive 注释行
	keepaliveCount := strings.Count(body, ": keepalive")
	if keepaliveCount < 2 {
		t.Errorf("期望至少 2 条 keepalive 注释，实际 %d 条，内容:\n%s", keepaliveCount, body)
	}

	// 验证事件数据仍然正确传递
	if !strings.Contains(body, "data: event1") {
		t.Errorf("期望响应包含 'data: event1'")
	}
}
