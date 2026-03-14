package config

import "context"

// Source 配置源接口，支持从不同后端拉取完整配置
type Source interface {
	// Load 从配置源加载完整配置内容（YAML 格式字节流）
	Load() ([]byte, error)
	// Type 返回配置源类型标识
	Type() string
	// Watch 监听配置变更，变更时将新的 YAML 内容发送到 channel。
	// ctx 取消时停止监听并关闭 channel。
	Watch(ctx context.Context) (<-chan []byte, error)
}
