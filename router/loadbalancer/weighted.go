package loadbalancer

import (
	"errors"
	"net/http"
	"sync"

	"github.com/dysodeng/gateway/discovery"
)

// weightedInstance 记录平滑加权轮询所需的每个实例状态
type weightedInstance struct {
	instance        discovery.ServiceInstance
	currentWeight   int // 当前有效权重
	effectiveWeight int // 有效权重（初始等于配置权重，可根据健康状态动态调整）
}

// WeightedRoundRobin 平滑加权轮询负载均衡策略（Nginx 风格）
// 算法: 每次选择当前权重最高的实例，选中后减去总权重；其余实例各加上自身有效权重
type WeightedRoundRobin struct {
	mu      sync.Mutex
	weights map[string]*weightedInstance // key 为实例 ID
}

// NewWeightedRoundRobin 创建一个新的平滑加权轮询负载均衡器
func NewWeightedRoundRobin() *WeightedRoundRobin {
	return &WeightedRoundRobin{
		weights: make(map[string]*weightedInstance),
	}
}

// Select 使用平滑加权轮询算法从实例列表中选择一个实例
func (w *WeightedRoundRobin) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("没有可用的服务实例")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// 初始化或同步权重状态
	for _, inst := range instances {
		if _, ok := w.weights[inst.ID]; !ok {
			weight := inst.Weight
			if weight <= 0 {
				weight = 1
			}
			w.weights[inst.ID] = &weightedInstance{
				instance:        inst,
				currentWeight:   0,
				effectiveWeight: weight,
			}
		}
	}

	// 计算总有效权重，并为每个实例累加当前权重
	totalWeight := 0
	var best *weightedInstance
	for _, inst := range instances {
		wi := w.weights[inst.ID]
		wi.instance = inst // 同步最新实例信息
		wi.currentWeight += wi.effectiveWeight
		totalWeight += wi.effectiveWeight

		// 选取当前权重最大的实例
		if best == nil || wi.currentWeight > best.currentWeight {
			best = wi
		}
	}

	// 被选中的实例减去总权重
	best.currentWeight -= totalWeight
	selected := best.instance
	return &selected, nil
}
