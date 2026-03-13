package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/golang-jwt/jwt/v5"
)

const testJWTSecret = "test-secret-key"

// makeJWTHandler 构造一个使用 JWT 认证的测试 Handler
func makeJWTHandler(cfg *config.JWTConfig) http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 将注入的请求头回写到响应头，方便测试断言
		for key := range r.Header {
			w.Header().Set(key, r.Header.Get(key))
		}
		w.WriteHeader(http.StatusOK)
	})
	return newJWTAuth(cfg)(next)
}

// signToken 生成 HS256 JWT 并签名
func signToken(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(testJWTSecret))
}

// TestJWTAuth_ValidToken 验证合法 JWT Token 能够正常通过并注入 claims 到请求头
func TestJWTAuth_ValidToken(t *testing.T) {
	cfg := &config.JWTConfig{
		Secret:     testJWTSecret,
		Algorithms: []string{"HS256"},
		Header:     "Authorization",
		ClaimsToHeaders: map[string]string{
			"user_id": "X-User-Id",
		},
	}

	tokenStr, err := signToken(jwt.MapClaims{
		"user_id": "123",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("生成 JWT 失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()

	makeJWTHandler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 200，实际: %d，响应: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-User-Id"); got != "123" {
		t.Errorf("期望 X-User-Id=123，实际: %q", got)
	}
}

// TestJWTAuth_InvalidToken 验证无效 JWT Token 返回 401
func TestJWTAuth_InvalidToken(t *testing.T) {
	cfg := &config.JWTConfig{
		Secret:     testJWTSecret,
		Algorithms: []string{"HS256"},
		Header:     "Authorization",
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer this.is.not.a.valid.token")
	w := httptest.NewRecorder()

	makeJWTHandler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望状态码 401，实际: %d", w.Code)
	}
}

// TestJWTAuth_MissingHeader 验证缺少 Authorization 头时返回 401
func TestJWTAuth_MissingHeader(t *testing.T) {
	cfg := &config.JWTConfig{
		Secret:     testJWTSecret,
		Algorithms: []string{"HS256"},
		Header:     "Authorization",
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	makeJWTHandler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望状态码 401，实际: %d", w.Code)
	}
}

// TestJWTAuth_ExpiredToken 验证过期 JWT Token 返回 401
func TestJWTAuth_ExpiredToken(t *testing.T) {
	cfg := &config.JWTConfig{
		Secret:     testJWTSecret,
		Algorithms: []string{"HS256"},
		Header:     "Authorization",
	}

	// exp 设置为过去时间，使 token 过期
	tokenStr, err := signToken(jwt.MapClaims{
		"user_id": "123",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("生成过期 JWT 失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()

	makeJWTHandler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望状态码 401，实际: %d，响应: %s", w.Code, w.Body.String())
	}
}

// makeAPIKeyHandler 构造一个使用 API Key 认证的测试 Handler
func makeAPIKeyHandler(cfg *config.APIKeyConfig) http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return newAPIKeyAuth(cfg)(next)
}

// TestAPIKeyAuth_Header 验证通过请求头传递 API Key 能够正常通过
func TestAPIKeyAuth_Header(t *testing.T) {
	cfg := &config.APIKeyConfig{
		Header: "X-API-Key",
		Query:  "api_key",
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "my-secret-key")
	w := httptest.NewRecorder()

	makeAPIKeyHandler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 200，实际: %d", w.Code)
	}
}

// TestAPIKeyAuth_Query 验证通过 Query 参数传递 API Key 能够正常通过
func TestAPIKeyAuth_Query(t *testing.T) {
	cfg := &config.APIKeyConfig{
		Header: "X-API-Key",
		Query:  "api_key",
	}

	req := httptest.NewRequest(http.MethodGet, "/?api_key=my-secret-key", nil)
	w := httptest.NewRecorder()

	makeAPIKeyHandler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 200，实际: %d", w.Code)
	}
}

// TestAPIKeyAuth_Missing 验证未提供 API Key 时返回 401
func TestAPIKeyAuth_Missing(t *testing.T) {
	cfg := &config.APIKeyConfig{
		Header: "X-API-Key",
		Query:  "api_key",
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	makeAPIKeyHandler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望状态码 401，实际: %d", w.Code)
	}
}

// makeOAuth2Handler 构造一个使用 OAuth2 内省认证的测试 Handler
func makeOAuth2Handler(cfg *config.OAuth2Config) http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 将注入的请求头回写到响应头，方便测试断言
		for key := range r.Header {
			w.Header().Set(key, r.Header.Get(key))
		}
		w.WriteHeader(http.StatusOK)
	})
	return newOAuth2Auth(cfg)(next)
}

// TestOAuth2Auth_Valid 验证内省端点返回 active=true 时请求通过，并注入声明到请求头
func TestOAuth2Auth_Valid(t *testing.T) {
	// 使用 httptest.NewServer 模拟 OAuth2 内省端点
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"active": true,
			"sub":    "user1",
		})
	}))
	defer mockServer.Close()

	cfg := &config.OAuth2Config{
		IntrospectEndpoint: mockServer.URL,
		ClientID:           "client-id",
		ClientSecret:       "client-secret",
		ClaimsToHeaders: map[string]string{
			"sub": "X-OAuth-Subject",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	makeOAuth2Handler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 200，实际: %d，响应: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-OAuth-Subject"); got != "user1" {
		t.Errorf("期望 X-OAuth-Subject=user1，实际: %q", got)
	}
}

// TestOAuth2Auth_Invalid 验证内省端点返回 active=false 时请求被拒绝，返回 401
func TestOAuth2Auth_Invalid(t *testing.T) {
	// 使用 httptest.NewServer 模拟返回 active=false 的内省端点
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"active": false,
		})
	}))
	defer mockServer.Close()

	cfg := &config.OAuth2Config{
		IntrospectEndpoint: mockServer.URL,
		ClientID:           "client-id",
		ClientSecret:       "client-secret",
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	w := httptest.NewRecorder()

	makeOAuth2Handler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望状态码 401，实际: %d，响应: %s", w.Code, w.Body.String())
	}
}
