package router

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/dysodeng/gateway/config"
)

// TestRouter_Match 测试基本路由匹配逻辑，验证不同路径能匹配到正确的路由，未匹配路径返回 false
func TestRouter_Match(t *testing.T) {
	routes := []config.RouteConfig{
		{Name: "用户服务", Prefix: "/api/users", Service: "user-svc"},
		{Name: "订单服务", Prefix: "/api/orders", Service: "order-svc"},
		{Name: "通用接口", Prefix: "/api", Service: "api-svc"},
	}

	r := New(routes)

	tests := []struct {
		path            string
		expectedService string
		expectedMatch   bool
	}{
		{"/api/users/123", "user-svc", true},
		{"/api/orders/456", "order-svc", true},
		{"/api/other", "api-svc", true},
		{"/health", "", false},
		{"/unknown/path", "", false},
	}

	for _, tc := range tests {
		req := &http.Request{URL: &url.URL{Path: tc.path}}
		route, ok := r.Match(req)
		if ok != tc.expectedMatch {
			t.Errorf("路径 %q: 期望 match=%v, 实际 match=%v", tc.path, tc.expectedMatch, ok)
			continue
		}
		if tc.expectedMatch && route.Service != tc.expectedService {
			t.Errorf("路径 %q: 期望服务 %q, 实际服务 %q", tc.path, tc.expectedService, route.Service)
		}
	}
}

// TestRouter_Match_LongestPrefix 测试最长前缀优先匹配，确保更具体的路由优先于更短的路由
func TestRouter_Match_LongestPrefix(t *testing.T) {
	routes := []config.RouteConfig{
		{Name: "通用API", Prefix: "/api", Service: "api-svc"},
		{Name: "用户V1", Prefix: "/api/v1/users", Service: "user-v1-svc"},
	}

	r := New(routes)

	req := &http.Request{URL: &url.URL{Path: "/api/v1/users/123"}}
	route, ok := r.Match(req)
	if !ok {
		t.Fatal("期望匹配成功，实际未匹配")
	}
	if route.Service != "user-v1-svc" {
		t.Errorf("期望匹配最长前缀路由 user-v1-svc，实际匹配到 %q", route.Service)
	}

	// 不匹配长前缀时，应回退到短前缀路由
	req2 := &http.Request{URL: &url.URL{Path: "/api/v2/products"}}
	route2, ok2 := r.Match(req2)
	if !ok2 {
		t.Fatal("期望匹配成功，实际未匹配")
	}
	if route2.Service != "api-svc" {
		t.Errorf("期望回退匹配 api-svc，实际匹配到 %q", route2.Service)
	}
}
