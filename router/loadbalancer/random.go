package loadbalancer

import (
	"errors"
	"math/rand/v2"
	"net/http"

	"github.com/dysodeng/gateway/discovery"
)

// Random 随机负载均衡策略，使用 math/rand/v2 生成随机索引
type Random struct{}

// NewRandom 创建一个新的随机负载均衡器
func NewRandom() *Random {
	return &Random{}
}

// Select 从实例列表中随机选择一个实例
func (r *Random) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("没有可用的服务实例")
	}
	// 随机生成 [0, len(instances)) 范围内的索引
	idx := rand.IntN(len(instances))
	selected := instances[idx]
	return &selected, nil
}
