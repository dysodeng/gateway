package loadbalancer

import (
	"net/http"
	"testing"

	"github.com/dysodeng/gateway/discovery"
)

// TestIPHash_SameIPSameInstance 测试相同 IP 始终选中同一实例
func TestIPHash_SameIPSameInstance(t *testing.T) {
	h := NewIPHash()
	instances := []discovery.ServiceInstance{
		{ID: "a", Host: "10.0.0.1", Port: 8080},
		{ID: "b", Host: "10.0.0.2", Port: 8080},
		{ID: "c", Host: "10.0.0.3", Port: 8080},
	}

	// 使用同一个客户端 IP 发起多次请求，应始终选中相同实例
	clientIP := "192.168.1.100:54321"
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = clientIP

	first, err := h.Select(instances, req)
	if err != nil {
		t.Fatalf("Select 返回错误: %v", err)
	}

	for i := 1; i < 10; i++ {
		got, err := h.Select(instances, req)
		if err != nil {
			t.Fatalf("第 %d 次 Select 返回错误: %v", i+1, err)
		}
		if got.ID != first.ID {
			t.Errorf("相同 IP 第 %d 次选中 ID=%s，与首次 ID=%s 不一致", i+1, got.ID, first.ID)
		}
	}
	t.Logf("IP %s 始终路由到实例 %s", clientIP, first.ID)
}

// TestIPHash_DifferentIPsMayDiffer 测试不同 IP 可能选中不同实例
func TestIPHash_DifferentIPsMayDiffer(t *testing.T) {
	h := NewIPHash()
	instances := []discovery.ServiceInstance{
		{ID: "a", Host: "10.0.0.1", Port: 8080},
		{ID: "b", Host: "10.0.0.2", Port: 8080},
		{ID: "c", Host: "10.0.0.3", Port: 8080},
	}

	clientIPs := []string{
		"192.168.1.1:1001",
		"192.168.1.2:1002",
		"10.0.0.50:2000",
		"172.16.0.1:3000",
		"8.8.8.8:4000",
	}

	selected := make(map[string]bool)
	for _, ip := range clientIPs {
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ip
		got, err := h.Select(instances, req)
		if err != nil {
			t.Fatalf("Select 返回错误: %v", err)
		}
		selected[got.ID] = true
		t.Logf("IP %s 路由到实例 %s", ip, got.ID)
	}

	// 多个不同 IP 中应该至少有两个不同的实例被选中
	if len(selected) < 2 {
		t.Errorf("5 个不同 IP 只路由到了 %d 个不同实例，期望至少 2 个", len(selected))
	}
}

// TestIPHash_NoPort 测试无端口号的 IP 地址格式
func TestIPHash_NoPort(t *testing.T) {
	h := NewIPHash()
	instances := []discovery.ServiceInstance{
		{ID: "a", Host: "10.0.0.1", Port: 8080},
		{ID: "b", Host: "10.0.0.2", Port: 8080},
	}

	// RemoteAddr 不含端口号的情况
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.50"

	got, err := h.Select(instances, req)
	if err != nil {
		t.Fatalf("Select 返回错误: %v", err)
	}
	t.Logf("无端口 IP 路由到实例 %s", got.ID)
}

// TestIPHash_Empty 测试空实例列表返回错误
func TestIPHash_Empty(t *testing.T) {
	h := NewIPHash()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	_, err := h.Select([]discovery.ServiceInstance{}, req)
	if err == nil {
		t.Fatal("空实例列表应返回错误，但返回了 nil")
	}
	if err.Error() != "没有可用的服务实例" {
		t.Errorf("错误信息不匹配: %s", err.Error())
	}
}
