package loadbalancer

import (
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/dysodeng/gateway/discovery"
)

// RoundRobin 轮询负载均衡策略，使用原子计数器保证线程安全
type RoundRobin struct {
	counter atomic.Uint64
}

// NewRoundRobin 创建一个新的轮询负载均衡器
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Select 按轮询顺序从实例列表中选择一个实例
func (r *RoundRobin) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("没有可用的服务实例")
	}
	// 原子递增计数器，对实例数量取模得到索引
	idx := r.counter.Add(1) - 1
	selected := instances[idx%uint64(len(instances))]
	return &selected, nil
}
