package loadbalancer

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/dysodeng/gateway/discovery"
)

// connCounter 记录单个实例的活跃连接数
type connCounter struct {
	count atomic.Int64
}

// LeastConn 最少连接数负载均衡策略
// 将请求路由到当前活跃连接数最少的实例
type LeastConn struct {
	mu       sync.Mutex
	counters map[string]*connCounter // key 为实例 ID
}

// NewLeastConn 创建一个新的最少连接数负载均衡器
func NewLeastConn() *LeastConn {
	return &LeastConn{
		counters: make(map[string]*connCounter),
	}
}

// getCounter 获取指定实例 ID 的连接计数器，不存在时自动创建
func (lc *LeastConn) getCounter(id string) *connCounter {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if _, ok := lc.counters[id]; !ok {
		lc.counters[id] = &connCounter{}
	}
	return lc.counters[id]
}

// IncrConn 增加指定实例的活跃连接数
func (lc *LeastConn) IncrConn(id string) {
	lc.getCounter(id).count.Add(1)
}

// DecrConn 减少指定实例的活跃连接数
func (lc *LeastConn) DecrConn(id string) {
	lc.getCounter(id).count.Add(-1)
}

// Select 选择当前活跃连接数最少的实例
func (lc *LeastConn) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("没有可用的服务实例")
	}

	var best *discovery.ServiceInstance
	var bestCount int64

	for i := range instances {
		inst := &instances[i]
		counter := lc.getCounter(inst.ID)
		count := counter.count.Load()

		if best == nil || count < bestCount {
			best = inst
			bestCount = count
		}
	}

	selected := *best
	return &selected, nil
}
