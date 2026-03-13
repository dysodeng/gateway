package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dysodeng/gateway/pkg/health"
)

// alwaysHealthy 始终返回 nil 的健康检查器（模拟正常服务）
type alwaysHealthy struct {
	name string
}

func (c *alwaysHealthy) Name() string { return c.name }
func (c *alwaysHealthy) Check(_ context.Context) error {
	return nil
}

// alwaysUnhealthy 始终返回错误的健康检查器（模拟故障服务）
type alwaysUnhealthy struct {
	name string
	err  string
}

func (c *alwaysUnhealthy) Name() string { return c.name }
func (c *alwaysUnhealthy) Check(_ context.Context) error {
	return errors.New(c.err)
}

// TestAllHealthy 验证所有检查器均健康时返回 200 且 status 为 healthy
func TestAllHealthy(t *testing.T) {
	handler := health.Handler(
		&alwaysHealthy{name: "config_center"},
		&alwaysHealthy{name: "discovery"},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	// 验证 HTTP 状态码
	if rec.Code != http.StatusOK {
		t.Errorf("期望状态码 200，实际得到 %d", rec.Code)
	}

	// 解析 JSON 响应
	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("解析响应 JSON 失败: %v", err)
	}

	// 验证 status 字段
	if result["status"] != "healthy" {
		t.Errorf("期望 status=healthy，实际得到 %v", result["status"])
	}

	// 验证 checks 字段
	checks, ok := result["checks"].(map[string]interface{})
	if !ok {
		t.Fatalf("checks 字段类型错误，实际: %T", result["checks"])
	}
	if checks["config_center"] != "up" {
		t.Errorf("期望 config_center=up，实际得到 %v", checks["config_center"])
	}
	if checks["discovery"] != "up" {
		t.Errorf("期望 discovery=up，实际得到 %v", checks["discovery"])
	}
}

// TestOneUnhealthy 验证有检查器故障时返回 503 且 status 为 unhealthy
func TestOneUnhealthy(t *testing.T) {
	handler := health.Handler(
		&alwaysHealthy{name: "config_center"},
		&alwaysUnhealthy{name: "discovery", err: "connection refused"},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	// 验证 HTTP 状态码为 503
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("期望状态码 503，实际得到 %d", rec.Code)
	}

	// 解析 JSON 响应
	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("解析响应 JSON 失败: %v", err)
	}

	// 验证 status 字段
	if result["status"] != "unhealthy" {
		t.Errorf("期望 status=unhealthy，实际得到 %v", result["status"])
	}

	// 验证 checks 字段
	checks, ok := result["checks"].(map[string]interface{})
	if !ok {
		t.Fatalf("checks 字段类型错误，实际: %T", result["checks"])
	}
	if checks["config_center"] != "up" {
		t.Errorf("期望 config_center=up，实际得到 %v", checks["config_center"])
	}
	// 故障的检查器应以 "down: <错误信息>" 格式记录
	discoveryStatus, ok := checks["discovery"].(string)
	if !ok {
		t.Fatalf("discovery checks 值类型错误，实际: %T", checks["discovery"])
	}
	if discoveryStatus != "down: connection refused" {
		t.Errorf("期望 discovery=down: connection refused，实际得到 %v", discoveryStatus)
	}
}

// TestNoCheckers 验证无检查器时返回 200 且 checks 为空对象
func TestNoCheckers(t *testing.T) {
	handler := health.Handler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	// 验证 HTTP 状态码为 200
	if rec.Code != http.StatusOK {
		t.Errorf("期望状态码 200，实际得到 %d", rec.Code)
	}

	// 解析 JSON 响应
	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("解析响应 JSON 失败: %v", err)
	}

	// 验证 status 字段
	if result["status"] != "healthy" {
		t.Errorf("期望 status=healthy，实际得到 %v", result["status"])
	}

	// 验证 checks 为空 map
	checks, ok := result["checks"].(map[string]interface{})
	if !ok {
		t.Fatalf("checks 字段类型错误，实际: %T", result["checks"])
	}
	if len(checks) != 0 {
		t.Errorf("期望 checks 为空，实际得到 %v", checks)
	}
}
