package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
)

// buildSignature 计算 HMAC-SHA256 签名，用于测试辅助
// 签名内容为 body + timestamp 拼接
func buildSignature(secret, body, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body + timestamp))
	return hex.EncodeToString(mac.Sum(nil))
}

// captureBody 用于在请求签名验证后仍能读取请求体的测试 Handler
var captureBodyHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	fmt.Fprintf(w, "%s", data)
	w.WriteHeader(http.StatusOK)
})

// newRequestSignConfig 创建用于测试的请求签名配置
func newRequestSignConfig(enabled bool) config.RequestSignConfig {
	return config.RequestSignConfig{
		Enabled:         enabled,
		Algorithm:       "hmac-sha256",
		SignHeader:      "X-Signature",
		TimestampHeader: "X-Timestamp",
		Expire:          60,
		Secret:          "test-secret-key",
	}
}

// TestRequestSign_ValidSignature 验证正确签名时请求通过
func TestRequestSign_ValidSignature(t *testing.T) {
	cfg := newRequestSignConfig(true)
	handler := NewRequestSign(cfg)(okHandler)

	body := `{"user":"alice"}`
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	sig := buildSignature(cfg.Secret, body, timestamp)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(cfg.SignHeader, sig)
	req.Header.Set(cfg.TimestampHeader, timestamp)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("合法签名期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}
}

// TestRequestSign_InvalidSignature 验证签名错误时返回 403
func TestRequestSign_InvalidSignature(t *testing.T) {
	cfg := newRequestSignConfig(true)
	handler := NewRequestSign(cfg)(okHandler)

	body := `{"user":"bob"}`
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(cfg.SignHeader, "invalidsignature")
	req.Header.Set(cfg.TimestampHeader, timestamp)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("非法签名期望状态码 %d，实际 %d", http.StatusForbidden, w.Code)
	}
}

// TestRequestSign_ExpiredTimestamp 验证时间戳过期时返回 403
func TestRequestSign_ExpiredTimestamp(t *testing.T) {
	cfg := newRequestSignConfig(true)
	handler := NewRequestSign(cfg)(okHandler)

	body := `{"user":"charlie"}`
	// 使用 120 秒前的时间戳（超出 60 秒有效期）
	expiredTimestamp := fmt.Sprintf("%d", time.Now().Unix()-120)
	sig := buildSignature(cfg.Secret, body, expiredTimestamp)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(cfg.SignHeader, sig)
	req.Header.Set(cfg.TimestampHeader, expiredTimestamp)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("过期时间戳期望状态码 %d，实际 %d", http.StatusForbidden, w.Code)
	}
}

// TestRequestSign_MissingHeaders 验证缺少签名头或时间戳头时返回 403
func TestRequestSign_MissingHeaders(t *testing.T) {
	cfg := newRequestSignConfig(true)
	handler := NewRequestSign(cfg)(okHandler)

	// 缺少所有签名相关头
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("缺少请求头时期望状态码 %d，实际 %d", http.StatusForbidden, w.Code)
	}

	// 只缺少签名头
	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
	req2.Header.Set(cfg.TimestampHeader, fmt.Sprintf("%d", time.Now().Unix()))
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Errorf("缺少签名头时期望状态码 %d，实际 %d", http.StatusForbidden, w2.Code)
	}

	// 只缺少时间戳头
	body := "body"
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	sig := buildSignature(cfg.Secret, body, timestamp)
	req3 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req3.Header.Set(cfg.SignHeader, sig)
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	if w3.Code != http.StatusForbidden {
		t.Errorf("缺少时间戳头时期望状态码 %d，实际 %d", http.StatusForbidden, w3.Code)
	}
}

// TestRequestSign_Disabled 验证禁用时所有请求均通过（无需签名验证）
func TestRequestSign_Disabled(t *testing.T) {
	cfg := newRequestSignConfig(false)
	handler := NewRequestSign(cfg)(okHandler)

	// 不携带任何签名头，也应通过
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("any body"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("禁用签名验证时期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}
}
