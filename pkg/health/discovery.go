package health

import (
	"context"
	"fmt"
)

// DiscoveryPinger 服务发现健康检查接口
type DiscoveryPinger interface {
	// Ping 检测服务发现是否可用
	Ping(ctx context.Context) error
}

// discoveryChecker 将 DiscoveryPinger 适配为 Checker
type discoveryChecker struct {
	pinger DiscoveryPinger
}

// NewDiscoveryChecker 创建服务发现健康检查器
func NewDiscoveryChecker(pinger DiscoveryPinger) Checker {
	if pinger == nil {
		return &nopChecker{name: "discovery"}
	}
	return &discoveryChecker{pinger: pinger}
}

func (c *discoveryChecker) Name() string {
	return "discovery"
}

func (c *discoveryChecker) Check(ctx context.Context) error {
	if err := c.pinger.Ping(ctx); err != nil {
		return fmt.Errorf("服务发现不可用: %w", err)
	}
	return nil
}

// nopChecker 无操作检查器，用于不支持 Ping 的服务发现实现
type nopChecker struct {
	name string
}

func (c *nopChecker) Name() string            { return c.name }
func (c *nopChecker) Check(context.Context) error { return nil }
