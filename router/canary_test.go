package router

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/dysodeng/gateway/config"
)

// makeRequest 创建带有指定请求头的测试请求
func makeRequest(headers map[string]string) *http.Request {
	req := &http.Request{
		Header: make(http.Header),
		URL:    &url.URL{Path: "/test"},
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

// TestCanary_HeaderMatchWeightZero 测试 Header 匹配且 weight=0 时，100% 路由到灰度服务
func TestCanary_HeaderMatchWeightZero(t *testing.T) {
	rules := []config.CanaryRuleConfig{
		{
			Service: "canary-svc",
			Weight:  0,
			Match: &config.CanaryMatch{
				Headers: map[string]string{"X-Canary": "true"},
			},
		},
	}

	req := makeRequest(map[string]string{"X-Canary": "true"})

	// 运行多次，期望每次都路由到灰度服务
	for i := 0; i < 20; i++ {
		svc := ResolveCanary(rules, "default-svc", req)
		if svc != "canary-svc" {
			t.Errorf("第 %d 次: weight=0 且 Header 匹配，期望 canary-svc，实际 %q", i+1, svc)
		}
	}
}

// TestCanary_NoMatch_DefaultService 测试请求头不匹配时，返回默认服务
func TestCanary_NoMatch_DefaultService(t *testing.T) {
	rules := []config.CanaryRuleConfig{
		{
			Service: "canary-svc",
			Weight:  0,
			Match: &config.CanaryMatch{
				Headers: map[string]string{"X-Canary": "true"},
			},
		},
	}

	// 请求中没有 X-Canary 头
	req := makeRequest(map[string]string{})

	svc := ResolveCanary(rules, "default-svc", req)
	if svc != "default-svc" {
		t.Errorf("Header 不匹配，期望 default-svc，实际 %q", svc)
	}
}

// TestCanary_WeightOnlyRule 测试无 Header 条件且 weight=100 时，所有请求都路由到灰度服务
func TestCanary_WeightOnlyRule(t *testing.T) {
	rules := []config.CanaryRuleConfig{
		{
			Service: "canary-svc",
			Weight:  100,
			Match:   nil, // 无 Header 条件
		},
	}

	req := makeRequest(map[string]string{})

	// weight=100 时，所有请求都应路由到灰度服务
	for i := 0; i < 20; i++ {
		svc := ResolveCanary(rules, "default-svc", req)
		if svc != "canary-svc" {
			t.Errorf("第 %d 次: weight=100 无 Header 条件，期望 canary-svc，实际 %q", i+1, svc)
		}
	}
}

// TestCanary_EmptyRules 测试规则列表为空时，始终返回默认服务
func TestCanary_EmptyRules(t *testing.T) {
	rules := []config.CanaryRuleConfig{}

	req := makeRequest(map[string]string{"X-Canary": "true"})

	svc := ResolveCanary(rules, "default-svc", req)
	if svc != "default-svc" {
		t.Errorf("规则为空，期望 default-svc，实际 %q", svc)
	}
}
