package middleware

import (
	"net"
	"net/http"

	"github.com/dysodeng/gateway/config"
)

// ipFilter IP 过滤器核心逻辑
type ipFilter struct {
	mode    string // "whitelist" 或 "blacklist"
	nets    []*net.IPNet
	ips     []net.IP
	enabled bool
}

func newIPFilter(cfg config.IPFilterConfig) *ipFilter {
	f := &ipFilter{
		mode:    cfg.Mode,
		enabled: len(cfg.List) > 0,
	}
	for _, entry := range cfg.List {
		_, cidr, err := net.ParseCIDR(entry)
		if err == nil {
			f.nets = append(f.nets, cidr)
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			f.ips = append(f.ips, ip)
		}
	}
	return f
}

func (f *ipFilter) contains(ipStr string) bool {
	host, _, err := net.SplitHostPort(ipStr)
	if err != nil {
		host = ipStr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, cidr := range f.nets {
		if cidr.Contains(ip) {
			return true
		}
	}
	for _, allowed := range f.ips {
		if allowed.Equal(ip) {
			return true
		}
	}
	return false
}

func (f *ipFilter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !f.enabled {
			next.ServeHTTP(w, r)
			return
		}

		inList := f.contains(r.RemoteAddr)

		switch f.mode {
		case "whitelist":
			if !inList {
				http.Error(w, "禁止访问", http.StatusForbidden)
				return
			}
		case "blacklist":
			if inList {
				http.Error(w, "禁止访问", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// NewGlobalIPFilter 创建全局 IP 过滤中间件（Pre-Route 阶段）
func NewGlobalIPFilter(cfg config.IPFilterConfig) Middleware {
	f := newIPFilter(cfg)
	return f.middleware
}

// NewRouteIPFilter 创建路由级 IP 过滤中间件（Post-Route 阶段）
func NewRouteIPFilter(cfg config.IPFilterConfig) Middleware {
	f := newIPFilter(cfg)
	return f.middleware
}
