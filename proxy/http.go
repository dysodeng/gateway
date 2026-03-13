package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/dysodeng/gateway/discovery"
)

// HTTPProxy HTTP 反向代理
type HTTPProxy struct{}

// NewHTTPProxy 创建 HTTP 反向代理实例
func NewHTTPProxy() *HTTPProxy {
	return &HTTPProxy{}
}

// Forward 将请求转发到后端服务实例
func (p *HTTPProxy) Forward(w http.ResponseWriter, r *http.Request, instance *discovery.ServiceInstance, stripPrefix bool, prefix string) {
	target := &url.URL{
		Scheme: "http",
		Host:   instance.Addr(),
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			if stripPrefix && prefix != "" {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
				if req.URL.Path == "" {
					req.URL.Path = "/"
				}
			}
		},
	}

	proxy.ServeHTTP(w, r)
}
