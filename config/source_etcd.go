package config

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdSource 基于 etcd 的配置源实现
type EtcdSource struct {
	cfg *EtcdSourceConfig
}

// NewEtcdSource 创建 etcd 配置源
func NewEtcdSource(cfg *EtcdSourceConfig) *EtcdSource {
	return &EtcdSource{cfg: cfg}
}

func (s *EtcdSource) Type() string {
	return "etcd"
}

// Load 从 etcd 拉取完整配置（YAML 格式）
func (s *EtcdSource) Load() ([]byte, error) {
	timeout := s.cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   s.cfg.Endpoints,
		DialTimeout: timeout,
		Username:    s.cfg.Username,
		Password:    s.cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 etcd 失败: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.Get(ctx, s.cfg.Key)
	if err != nil {
		return nil, fmt.Errorf("从 etcd 读取配置失败: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("etcd 中未找到配置 key: %s", s.cfg.Key)
	}

	return resp.Kvs[0].Value, nil
}
