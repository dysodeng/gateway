package router

import (
	"net/http"
	"sort"
	"strings"

	"github.com/dysodeng/gateway/config"
)

// Router 基于最长前缀匹配的路由器
type Router struct {
	routes []config.RouteConfig // 按前缀长度降序排列
}

// New 创建路由器，路由按前缀长度降序排列以支持最长前缀匹配
func New(routes []config.RouteConfig) *Router {
	sorted := make([]config.RouteConfig, len(routes))
	copy(sorted, routes)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Prefix) > len(sorted[j].Prefix)
	})
	return &Router{routes: sorted}
}

// Match 对请求路径进行最长前缀匹配，返回匹配的路由配置
func (r *Router) Match(req *http.Request) (*config.RouteConfig, bool) {
	path := req.URL.Path
	for i := range r.routes {
		if strings.HasPrefix(path, r.routes[i].Prefix) {
			return &r.routes[i], true
		}
	}
	return nil, false
}
