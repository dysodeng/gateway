package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/dysodeng/gateway/config"
)

// computeHMACSHA256 计算 HMAC-SHA256 签名
// data 为待签名数据，secret 为密钥
func computeHMACSHA256(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// NewRequestSign 创建请求签名验证中间件
// 验证流程：检查请求头 -> 验证时间戳有效期 -> 验证 HMAC-SHA256 签名
func NewRequestSign(cfg config.RequestSignConfig) Middleware {
	// 签名验证未启用时直接透传
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	// 设置默认请求头名称
	signHeader := cfg.SignHeader
	if signHeader == "" {
		signHeader = "X-Signature"
	}
	timestampHeader := cfg.TimestampHeader
	if timestampHeader == "" {
		timestampHeader = "X-Timestamp"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从请求头获取签名和时间戳
			signature := r.Header.Get(signHeader)
			timestampStr := r.Header.Get(timestampHeader)

			// 检查必要请求头是否存在
			if signature == "" || timestampStr == "" {
				http.Error(w, "缺少签名或时间戳请求头", http.StatusForbidden)
				return
			}

			// 解析时间戳
			timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
			if err != nil {
				http.Error(w, "时间戳格式错误", http.StatusForbidden)
				return
			}

			// 验证时间戳是否在有效期内
			now := time.Now().Unix()
			expire := int64(cfg.Expire)
			if expire <= 0 {
				expire = 300 // 默认 5 分钟有效期
			}
			if abs(now-timestamp) > expire {
				http.Error(w, "请求已过期", http.StatusForbidden)
				return
			}

			// 读取请求体用于签名验证
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "读取请求体失败", http.StatusForbidden)
				return
			}
			// 恢复请求体，供后续 Handler 使用
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			// 计算期望签名：HMAC-SHA256(body + timestamp)
			expectedSig := computeHMACSHA256(string(bodyBytes)+timestampStr, cfg.Secret)

			// 使用恒定时间比较防止时序攻击
			if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
				http.Error(w, "签名验证失败", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// abs 返回 int64 的绝对值
func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
