package discovery

import (
	"fmt"

	"github.com/dysodeng/gateway/config"
)

// StaticDiscovery 基于静态配置文件的服务发现实现
type StaticDiscovery struct {
	instances map[string][]ServiceInstance
}

// NewStaticDiscovery 从静态配置创建服务发现实例
func NewStaticDiscovery(cfg *config.StaticDiscoveryConfig) *StaticDiscovery {
	instances := make(map[string][]ServiceInstance)
	for name, cfgInstances := range cfg.Services {
		for i, inst := range cfgInstances {
			instances[name] = append(instances[name], ServiceInstance{
				ID:       fmt.Sprintf("%s-%d", name, i),
				Name:     name,
				Host:     inst.Host,
				Port:     inst.Port,
				Weight:   inst.Weight,
				Metadata: inst.Metadata,
			})
		}
	}
	return &StaticDiscovery{instances: instances}
}

// GetInstances 获取指定服务名的所有实例
func (d *StaticDiscovery) GetInstances(serviceName string) ([]ServiceInstance, error) {
	instances, ok := d.instances[serviceName]
	if !ok || len(instances) == 0 {
		return nil, fmt.Errorf("服务 %q 未找到", serviceName)
	}
	return instances, nil
}

// Watch 静态配置不支持动态监听，直接返回 nil
func (d *StaticDiscovery) Watch(serviceName string, callback func([]ServiceInstance)) error {
	return nil
}

// Stop 静态配置无需清理资源
func (d *StaticDiscovery) Stop() error {
	return nil
}
