package loadbalancer

import (
	"net/http"

	"github.com/dysodeng/gateway/discovery"
)

// Balancer 定义负载均衡策略接口
type Balancer interface {
	// Select 从实例列表中选择一个实例
	Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error)
}
