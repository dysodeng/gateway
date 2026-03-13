package loadbalancer

import (
	"net/http"
	"testing"

	"github.com/dysodeng/gateway/discovery"
)

// TestWeightedRoundRobin_Proportional 测试加权轮询按权重比例分配流量
func TestWeightedRoundRobin_Proportional(t *testing.T) {
	wrr := NewWeightedRoundRobin()
	instances := []discovery.ServiceInstance{
		{ID: "a", Host: "10.0.0.1", Port: 8080, Weight: 5},
		{ID: "b", Host: "10.0.0.2", Port: 8080, Weight: 1},
	}
	req, _ := http.NewRequest(http.MethodGet, "/", nil)

	counts := make(map[string]int)
	total := 600
	for i := 0; i < total; i++ {
		got, err := wrr.Select(instances, req)
		if err != nil {
			t.Fatalf("Select 返回错误: %v", err)
		}
		counts[got.ID]++
	}

	// 权重 5:1，实例 a 应约占 83%，实例 b 约占 17%
	ratioA := float64(counts["a"]) / float64(total)
	ratioB := float64(counts["b"]) / float64(total)

	t.Logf("实例 a 选中比例: %.2f%%, 实例 b 选中比例: %.2f%%", ratioA*100, ratioB*100)

	// 允许 5% 的误差范围
	if ratioA < 0.78 || ratioA > 0.88 {
		t.Errorf("实例 a 比例期望约 83%%，实际为 %.2f%%", ratioA*100)
	}
	if ratioB < 0.12 || ratioB > 0.22 {
		t.Errorf("实例 b 比例期望约 17%%，实际为 %.2f%%", ratioB*100)
	}
}

// TestWeightedRoundRobin_Empty 测试空实例列表返回错误
func TestWeightedRoundRobin_Empty(t *testing.T) {
	wrr := NewWeightedRoundRobin()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	_, err := wrr.Select([]discovery.ServiceInstance{}, req)
	if err == nil {
		t.Fatal("空实例列表应返回错误，但返回了 nil")
	}
	if err.Error() != "没有可用的服务实例" {
		t.Errorf("错误信息不匹配: %s", err.Error())
	}
}
