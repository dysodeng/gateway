package middleware

import (
	"net/http"

	"github.com/dysodeng/gateway/config"
)

// NewRewrite 创建请求重写中间件
// 负责在请求转发前对请求头进行注入或移除操作
// strip_prefix 由代理层处理，此中间件专注于请求头的操作
func NewRewrite(cfg config.RouteRewriteConfig) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 移除指定的请求头
			for _, header := range cfg.RemoveHeaders {
				r.Header.Del(header)
			}

			// 注入自定义请求头（若已存在则覆盖）
			for key, value := range cfg.AddHeaders {
				r.Header.Set(key, value)
			}

			next.ServeHTTP(w, r)
		})
	}
}
