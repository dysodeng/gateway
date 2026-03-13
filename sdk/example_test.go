package sdk_test

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/dysodeng/gateway/sdk"
	"github.com/dysodeng/gateway/sdk/etcd"
)

// 基本用法：注册 + 优雅关闭
//
// 实际使用时在 main 中配合 os.Signal 等待退出信号，
// 退出前调用 registry.Close() 即可自动注销所有实例。
//
//	quit := make(chan os.Signal, 1)
//	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
//	<-quit
//	registry.Close()
func Example_basic() {
	registry, err := etcd.NewRegistry(
		[]string{"127.0.0.1:2379"},
		etcd.WithPrefix("/services/"),
		etcd.WithAuth("root", "123456"),
	)
	if err != nil {
		fmt.Println("连接失败:", err)
		return
	}
	defer registry.Close()

	instance := sdk.ServiceInstance{
		Name:    "user-service",
		Host:    "10.0.0.1",
		Port:    8080,
		Weight:  1,
		Version: "v1.0.0",
		Metadata: map[string]string{
			"env": "production",
		},
	}

	if err = registry.Register(context.Background(), instance); err != nil {
		fmt.Println("注册失败:", err)
		return
	}

	fmt.Println("服务已注册")
	// Output: 服务已注册
}

// 带健康检查的用法
//
// 健康检查失败时自动从 etcd 摘除实例，恢复后自动重新注册。
func Example_withHealthCheck() {
	healthCheck := func(ctx context.Context) error {
		conn, err := net.DialTimeout("tcp", "10.0.0.1:8080", time.Second)
		if err != nil {
			return err
		}
		return conn.Close()
	}

	registry, err := etcd.NewRegistry(
		[]string{"127.0.0.1:2379"},
		etcd.WithHealthChecker(healthCheck, 5*time.Second),
	)
	if err != nil {
		fmt.Println("连接失败:", err)
		return
	}
	defer registry.Close()

	instance := sdk.ServiceInstance{
		Name: "order-service",
		Host: "10.0.0.1",
		Port: 8080,
	}

	if err = registry.Register(context.Background(), instance); err != nil {
		fmt.Println("注册失败:", err)
		return
	}

	fmt.Println("服务已注册（带健康检查）")
	// Output: 服务已注册（带健康检查）
}
