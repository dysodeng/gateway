package proxy

import (
	"errors"
	"net/http"
	"testing"
)

// TestShouldRetry_5xx_GET 测试 GET 请求遇到 5xx 错误时应重试
func TestShouldRetry_5xx_GET(t *testing.T) {
	result := ShouldRetry(http.MethodGet, 502, nil, []string{"5xx"})
	if !result {
		t.Errorf("期望 GET + 502 + [\"5xx\"] 返回 true，实际得到 false")
	}
}

// TestShouldRetry_5xx_POST 测试 POST 请求（非幂等）不应重试
func TestShouldRetry_5xx_POST(t *testing.T) {
	result := ShouldRetry(http.MethodPost, 502, nil, []string{"5xx"})
	if result {
		t.Errorf("期望 POST + 502 + [\"5xx\"] 返回 false（非幂等方法），实际得到 true")
	}
}

// TestShouldRetry_Timeout 测试 GET 请求遇到超时错误时应重试
func TestShouldRetry_Timeout(t *testing.T) {
	result := ShouldRetry(http.MethodGet, 0, errors.New("请求超时"), []string{"timeout"})
	if !result {
		t.Errorf("期望 GET + error + [\"timeout\"] 返回 true，实际得到 false")
	}
}

// TestShouldRetry_200 测试 GET 请求 200 响应不触发 5xx 重试
func TestShouldRetry_200(t *testing.T) {
	result := ShouldRetry(http.MethodGet, 200, nil, []string{"5xx"})
	if result {
		t.Errorf("期望 GET + 200 + [\"5xx\"] 返回 false，实际得到 true")
	}
}

// TestShouldRetry_NoConditions 测试无重试条件时不应重试
func TestShouldRetry_NoConditions(t *testing.T) {
	result := ShouldRetry(http.MethodGet, 502, nil, []string{})
	if result {
		t.Errorf("期望 GET + 502 + [] 返回 false（无条件），实际得到 true")
	}
}
