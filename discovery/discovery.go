package discovery

import "fmt"

// ServiceInstance 表示一个后端服务实例
type ServiceInstance struct {
	ID       string
	Name     string
	Host     string
	Port     int
	Weight   int
	Metadata map[string]string
}

// Addr 返回 host:port 格式的地址
func (si ServiceInstance) Addr() string {
	return fmt.Sprintf("%s:%d", si.Host, si.Port)
}

// Discovery 定义服务发现接口。
// 启动时优先使用注册中心实现，不可用时自动降级到静态路由实现。
type Discovery interface {
	// GetInstances 获取指定服务名的所有实例
	GetInstances(serviceName string) ([]ServiceInstance, error)
	// Watch 监听指定服务的实例变更
	Watch(serviceName string, callback func([]ServiceInstance)) error
	// Stop 停止服务发现
	Stop() error
}
