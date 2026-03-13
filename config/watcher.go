package config

// Watcher 定义动态配置源的接口。
// 具体实现包括 etcd、nacos 等。网关通过此接口
// 热更新路由规则、限流配置、灰度策略和认证方案。
type Watcher interface {
	// Get 获取指定 key 的配置值
	Get(key string) ([]byte, error)
	// Watch 监听指定 key 的配置变更
	Watch(key string, callback func(value []byte)) error
	// Stop 停止监听
	Stop() error
}
