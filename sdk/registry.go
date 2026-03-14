package sdk

import "context"

// Registry 服务注册接口
type Registry interface {
	// Register 注册服务实例，注册后自动维持心跳，支持断线重注册。
	// 返回填充后的 ServiceInstance（含自动生成的 ID 等）。
	Register(ctx context.Context, instance ServiceInstance) (ServiceInstance, error)
	// Deregister 注销服务实例（主动下线）
	Deregister(ctx context.Context, instance ServiceInstance) error
	// Close 关闭注册器，注销所有实例并释放资源
	Close() error
}

// HealthChecker 健康检查函数，返回 nil 表示健康
type HealthChecker func(ctx context.Context) error

// ServiceInstance 服务实例
type ServiceInstance struct {
	Name     string            // 服务名称
	ID       string            // 实例唯一标识，为空时自动生成 UUID
	Host     string            // 主机地址
	Port     int               // 端口
	Weight   int               // 权重，默认 1
	Version  string            // 服务版本号
	Metadata map[string]string // 自定义元数据
}
