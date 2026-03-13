package loadbalancer

import (
	"net/http"
	"testing"

	"github.com/dysodeng/gateway/discovery"
)

// TestRandom_AllInstancesSelected 测试随机策略在足够多次迭代后能选中所有实例
func TestRandom_AllInstancesSelected(t *testing.T) {
	r := NewRandom()
	instances := []discovery.ServiceInstance{
		{ID: "a", Host: "10.0.0.1", Port: 8080},
		{ID: "b", Host: "10.0.0.2", Port: 8080},
		{ID: "c", Host: "10.0.0.3", Port: 8080},
	}
	req, _ := http.NewRequest(http.MethodGet, "/", nil)

	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		got, err := r.Select(instances, req)
		if err != nil {
			t.Fatalf("Select 返回错误: %v", err)
		}
		counts[got.ID]++
	}

	// 每个实例都应至少被选中一次
	for _, inst := range instances {
		if counts[inst.ID] == 0 {
			t.Errorf("实例 %s 在 1000 次迭代中从未被选中", inst.ID)
		}
	}
	t.Logf("各实例选中次数: a=%d, b=%d, c=%d", counts["a"], counts["b"], counts["c"])
}

// TestRandom_Empty 测试空实例列表返回错误
func TestRandom_Empty(t *testing.T) {
	r := NewRandom()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	_, err := r.Select([]discovery.ServiceInstance{}, req)
	if err == nil {
		t.Fatal("空实例列表应返回错误，但返回了 nil")
	}
	if err.Error() != "没有可用的服务实例" {
		t.Errorf("错误信息不匹配: %s", err.Error())
	}
}
