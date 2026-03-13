package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/dysodeng/gateway/config"
	"github.com/golang-jwt/jwt/v5"
)

// NewAuth 创建认证中间件，根据认证方案类型分发到对应的认证逻辑
// optional 为 true 时，无 token 放行，有 token 则验证
func NewAuth(scheme config.AuthSchemeConfig, optional bool) Middleware {
	switch scheme.Type {
	case "jwt":
		return newJWTAuth(scheme.JWT, optional)
	case "api_key":
		return newAPIKeyAuth(scheme.APIKey, optional)
	case "oauth2":
		return newOAuth2Auth(scheme.OAuth2, optional)
	default:
		// 未知认证类型，直接通过
		return func(next http.Handler) http.Handler { return next }
	}
}

// newJWTAuth 创建 JWT 认证中间件
func newJWTAuth(cfg *config.JWTConfig, optional bool) Middleware {
	if cfg == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从请求头获取 token
			authHeader := r.Header.Get(cfg.Header)
			if authHeader == "" {
				if optional {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "缺少认证信息", http.StatusUnauthorized)
				return
			}

			// 解析 Bearer token
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenStr == authHeader {
				http.Error(w, "认证格式错误", http.StatusUnauthorized)
				return
			}

			// 解析并验证 JWT
			token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
				// 验证签名算法是否在允许列表中
				alg := token.Method.Alg()
				allowed := false
				for _, a := range cfg.Algorithms {
					if a == alg {
						allowed = true
						break
					}
				}
				if !allowed {
					return nil, fmt.Errorf("不支持的签名算法: %s", alg)
				}

				// 根据算法类型返回对应的密钥
				switch {
				case strings.HasPrefix(alg, "HS"):
					return []byte(cfg.Secret), nil
				default:
					return nil, fmt.Errorf("不支持的密钥类型: %s", alg)
				}
			})

			if err != nil || !token.Valid {
				http.Error(w, "认证失败", http.StatusUnauthorized)
				return
			}

			// 将 claims 注入到请求头
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				for claimKey, headerKey := range cfg.ClaimsToHeaders {
					if val, exists := claims[claimKey]; exists {
						r.Header.Set(headerKey, fmt.Sprintf("%v", val))
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// newAPIKeyAuth 创建 API Key 认证中间件
func newAPIKeyAuth(cfg *config.APIKeyConfig, optional bool) Middleware {
	if cfg == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 先从 Header 中获取 API Key
			apiKey := r.Header.Get(cfg.Header)

			// 再从 Query 参数中获取
			if apiKey == "" && cfg.Query != "" {
				apiKey = r.URL.Query().Get(cfg.Query)
			}

			if apiKey == "" {
				if optional {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "缺少 API Key", http.StatusUnauthorized)
				return
			}

			// API Key 验证逻辑（此处仅检查非空，具体验证由配置中心提供的密钥列表决定）
			// 实际使用时需要与配置中心的密钥列表对比
			next.ServeHTTP(w, r)
		})
	}
}

// newOAuth2Auth 创建 OAuth2 Token 内省认证中间件 (RFC 7662)
func newOAuth2Auth(cfg *config.OAuth2Config, optional bool) Middleware {
	if cfg == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从 Authorization 头获取 Bearer token
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				if optional {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "缺少认证信息", http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenStr == authHeader {
				http.Error(w, "认证格式错误", http.StatusUnauthorized)
				return
			}

			// 调用内省端点验证 token
			introspectReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, cfg.IntrospectEndpoint,
				strings.NewReader("token="+tokenStr))
			if err != nil {
				http.Error(w, "内部错误", http.StatusInternalServerError)
				return
			}
			introspectReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			introspectReq.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)

			resp, err := http.DefaultClient.Do(introspectReq)
			if err != nil {
				http.Error(w, "认证服务不可用", http.StatusServiceUnavailable)
				return
			}
			defer resp.Body.Close()

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				http.Error(w, "认证响应解析失败", http.StatusInternalServerError)
				return
			}

			// 检查 token 是否有效
			active, _ := result["active"].(bool)
			if !active {
				http.Error(w, "Token 已失效", http.StatusUnauthorized)
				return
			}

			// 将内省结果中的字段注入到请求头
			for claimKey, headerKey := range cfg.ClaimsToHeaders {
				if val, exists := result[claimKey]; exists {
					r.Header.Set(headerKey, fmt.Sprintf("%v", val))
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
