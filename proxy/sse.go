package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
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
	target := &url.URL{
		Scheme: "http",
		Host:   instance.Addr(),
	}

	proxy := &httputil.ReverseProxy{
		// FlushInterval = -1 表示每次写入后立即 Flush，专为 SSE/流式场景设计
		FlushInterval: -1,
		Transport: &http.Transport{
			DisableCompression: true, // 禁用 gzip，避免解码缓冲
		},
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			if stripPrefix && prefix != "" {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
				if req.URL.Path == "" || req.URL.Path[0] != '/' {
					req.URL.Path = "/" + req.URL.Path
				}
			}
			// 禁止后端返回压缩内容
			req.Header.Del("Accept-Encoding")
		},
		ModifyResponse: func(resp *http.Response) error {
			return p.injectSSEHeaders(resp)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("SSE 后端请求失败", "error", err)
			http.Error(w, "后端服务不可用", http.StatusBadGateway)
		},
	}

	// 启动 keepalive 心跳（需要在 proxy.ServeHTTP 之前设置，但 keepalive 需要 flusher）
	// keepalive 通过 ModifyResponse 后在独立 goroutine 中发送
	if p.keepalive > 0 {
		if flusher, ok := w.(http.Flusher); ok {
			done := make(chan struct{})
			defer close(done)
			go p.sendKeepalive(w, flusher, done, r.Context().Done())
		}
	}

	proxy.ServeHTTP(w, r)
}

// injectSSEHeaders 在响应头中注入 retry 字段
func (p *SSEProxy) injectSSEHeaders(resp *http.Response) error {
	if p.retry > 0 {
		// 将 retry 信息注入到响应体前面
		original := resp.Body
		resp.Body = &retryPrefixReader{
			prefix: []byte(fmt.Sprintf("retry: %d\n\n", p.retry)),
			reader: original,
		}
	}
	return nil
}

// sendKeepalive 定期发送 keepalive 注释行
func (p *SSEProxy) sendKeepalive(w http.ResponseWriter, flusher http.Flusher, done <-chan struct{}, ctxDone <-chan struct{}) {
	ticker := time.NewTicker(p.keepalive)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-done:
			return
		case <-ctxDone:
			return
		}
	}
}

// retryPrefixReader 在原始 reader 前面插入 retry 前缀
type retryPrefixReader struct {
	prefix []byte
	reader io.ReadCloser
	offset int
}

func (r *retryPrefixReader) Read(p []byte) (int, error) {
	if r.offset < len(r.prefix) {
		n := copy(p, r.prefix[r.offset:])
		r.offset += n
		return n, nil
	}
	return r.reader.Read(p)
}

func (r *retryPrefixReader) Close() error {
	return r.reader.Close()
}
