package loadbalancer

import (
	"net/http"
	"testing"

	"github.com/dysodeng/gateway/discovery"
)

// TestRoundRobin_Select 测试轮询策略按顺序循环选择实例
func TestRoundRobin_Select(t *testing.T) {
	rr := NewRoundRobin()
	instances := []discovery.ServiceInstance{
		{ID: "a", Host: "10.0.0.1", Port: 8080},
		{ID: "b", Host: "10.0.0.2", Port: 8080},
		{ID: "c", Host: "10.0.0.3", Port: 8080},
	}
	req, _ := http.NewRequest(http.MethodGet, "/", nil)

	// 期望 6 次选择依次为 a, b, c, a, b, c
	expected := []string{"a", "b", "c", "a", "b", "c"}
	for i, wantID := range expected {
		got, err := rr.Select(instances, req)
		if err != nil {
			t.Fatalf("第 %d 次 Select 返回错误: %v", i+1, err)
		}
		if got.ID != wantID {
			t.Errorf("第 %d 次 Select: 期望 ID=%s, 实际 ID=%s", i+1, wantID, got.ID)
		}
	}
}

// TestRoundRobin_Empty 测试空实例列表返回错误
func TestRoundRobin_Empty(t *testing.T) {
	rr := NewRoundRobin()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	_, err := rr.Select([]discovery.ServiceInstance{}, req)
	if err == nil {
		t.Fatal("空实例列表应返回错误，但返回了 nil")
	}
	if err.Error() != "没有可用的服务实例" {
		t.Errorf("错误信息不匹配: %s", err.Error())
	}
}
