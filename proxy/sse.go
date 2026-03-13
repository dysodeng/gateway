package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/dysodeng/gateway/discovery"
)

// SSEProxy SSE 透明代理
type SSEProxy struct{}

// NewSSEProxy 创建 SSE 代理实例
func NewSSEProxy() *SSEProxy {
	return &SSEProxy{}
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
		if path == "" {
			path = "/"
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

	// 流式转发并实时刷新
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
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
