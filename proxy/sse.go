package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
)

// SSEProxy SSE 透明代理，支持 keepalive 心跳和客户端重连配置
type SSEProxy struct {
	retry     int           // 客户端重连间隔（毫秒），0 表示不发送
	keepalive time.Duration // keepalive 注释行发送间隔，0 表示不发送
}

// NewSSEProxy 创建 SSE 代理实例
func NewSSEProxy() *SSEProxy {
	return &SSEProxy{}
}

// Configure 配置 SSE 代理参数
func (p *SSEProxy) Configure(cfg *config.SSEConfig) {
	if cfg == nil {
		return
	}
	p.retry = cfg.Retry
	p.keepalive = cfg.Keepalive
}

// Forward 将 SSE 请求转发到后端并实时推送给客户端
func (p *SSEProxy) Forward(w http.ResponseWriter, r *http.Request, instance *discovery.ServiceInstance, stripPrefix bool, prefix string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式传输", http.StatusInternalServerError)
		return
	}

	path := r.URL.Path
	if stripPrefix && prefix != "" {
		path = strings.TrimPrefix(path, prefix)
		if path == "" || path[0] != '/' {
			path = "/" + path
		}
	}

	backendURL := "http://" + instance.Addr() + path
	if r.URL.RawQuery != "" {
		backendURL += "?" + r.URL.RawQuery
	}

	backendReq, err := http.NewRequestWithContext(r.Context(), r.Method, backendURL, nil)
	if err != nil {
		slog.Error("SSE 创建请求失败", "error", err)
		http.Error(w, "内部错误", http.StatusInternalServerError)
		return
	}

	// 复制客户端请求头
	for key, vals := range r.Header {
		for _, v := range vals {
			backendReq.Header.Add(key, v)
		}
	}

	resp, err := http.DefaultClient.Do(backendReq)
	if err != nil {
		slog.Error("SSE 后端请求失败", "error", err)
		http.Error(w, "后端服务不可用", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// 发送 retry 字段，配置客户端重连间隔
	if p.retry > 0 {
		fmt.Fprintf(w, "retry: %d\n\n", p.retry)
		flusher.Flush()
	}

	// 启动 keepalive 心跳
	done := make(chan struct{})
	defer close(done)
	if p.keepalive > 0 {
		go func() {
			ticker := time.NewTicker(p.keepalive)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					_, err := fmt.Fprint(w, ": keepalive\n\n")
					if err != nil {
						return
					}
					flusher.Flush()
				case <-done:
					return
				case <-r.Context().Done():
					return
				}
			}
		}()
	}

	// 流式转发并实时刷新
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				slog.Error("SSE 读取错误", "error", err)
			}
			return
		}
	}
}
