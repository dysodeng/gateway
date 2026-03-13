package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dysodeng/gateway/config"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// etcdInstance etcd 中存储的服务实例 JSON 结构
type etcdInstance struct {
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	Weight   int               `json:"weight"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// EtcdDiscovery 基于 etcd 的服务发现实现
type EtcdDiscovery struct {
	client *clientv3.Client
	prefix string

	mu        sync.RWMutex
	instances map[string][]ServiceInstance // serviceName -> instances

	cancel context.CancelFunc // 用于停止所有 Watch goroutine
	wg     sync.WaitGroup     // 等待 Watch goroutine 退出
}

// NewEtcdDiscovery 创建 etcd 服务发现实例，连接 etcd 并加载全量服务实例
func NewEtcdDiscovery(cfg *config.EtcdConfig) (*EtcdDiscovery, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "/services"
	}
	// 确保 prefix 以 / 结尾，便于前缀查询
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: timeout,
		Username:    cfg.Username,
		Password:    cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 etcd 失败: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &EtcdDiscovery{
		client:    client,
		prefix:    prefix,
		instances: make(map[string][]ServiceInstance),
		cancel:    cancel,
	}

	// 加载全量实例
	if err = d.loadAll(ctx, timeout); err != nil {
		cancel()
		_ = client.Close()
		return nil, fmt.Errorf("加载 etcd 服务实例失败: %w", err)
	}

	// 启动全局 Watch
	d.wg.Add(1)
	go d.watchAll(ctx)

	return d, nil
}

// loadAll 通过前缀 Get 加载所有服务实例到本地缓存
func (d *EtcdDiscovery) loadAll(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := d.client.Get(ctx, d.prefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("前缀查询 etcd 失败: %w", err)
	}

	instances := make(map[string][]ServiceInstance)
	for _, kv := range resp.Kvs {
		serviceName, instanceID := d.parseKey(string(kv.Key))
		if serviceName == "" {
			continue
		}

		inst, err := d.parseValue(serviceName, instanceID, kv.Value)
		if err != nil {
			slog.Warn("解析 etcd 服务实例失败", "key", string(kv.Key), "error", err)
			continue
		}
		instances[serviceName] = append(instances[serviceName], inst)
	}

	d.mu.Lock()
	d.instances = instances
	d.mu.Unlock()

	return nil
}

// parseKey 从 etcd key 中解析出 serviceName 和 instanceID
// key 格式: {prefix}{serviceName}/{instanceID}
func (d *EtcdDiscovery) parseKey(key string) (serviceName, instanceID string) {
	// 去掉前缀
	rel := strings.TrimPrefix(key, d.prefix)
	parts := strings.SplitN(rel, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}

// parseValue 将 etcd value（JSON）解析为 ServiceInstance
func (d *EtcdDiscovery) parseValue(serviceName, instanceID string, value []byte) (ServiceInstance, error) {
	var inst etcdInstance
	if err := json.Unmarshal(value, &inst); err != nil {
		return ServiceInstance{}, fmt.Errorf("JSON 反序列化失败: %w", err)
	}
	return ServiceInstance{
		ID:       instanceID,
		Name:     serviceName,
		Host:     inst.Host,
		Port:     inst.Port,
		Weight:   inst.Weight,
		Metadata: inst.Metadata,
	}, nil
}

// watchAll 通过前缀 Watch 监听所有服务实例变更
func (d *EtcdDiscovery) watchAll(ctx context.Context) {
	defer d.wg.Done()

	watchCh := d.client.Watch(ctx, d.prefix, clientv3.WithPrefix())
	for {
		select {
		case <-ctx.Done():
			return
		case resp, ok := <-watchCh:
			if !ok {
				return
			}
			if resp.Err() != nil {
				slog.Error("etcd Watch 错误", "error", resp.Err())
				continue
			}
			for _, ev := range resp.Events {
				d.handleEvent(ev)
			}
		}
	}
}

// handleEvent 处理单个 etcd Watch 事件
func (d *EtcdDiscovery) handleEvent(ev *clientv3.Event) {
	serviceName, instanceID := d.parseKey(string(ev.Kv.Key))
	if serviceName == "" {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	switch ev.Type {
	case clientv3.EventTypePut:
		inst, err := d.parseValue(serviceName, instanceID, ev.Kv.Value)
		if err != nil {
			slog.Warn("解析 etcd Watch 事件失败", "key", string(ev.Kv.Key), "error", err)
			return
		}
		// 更新或新增实例
		instances := d.instances[serviceName]
		found := false
		for i, existing := range instances {
			if existing.ID == instanceID {
				instances[i] = inst
				found = true
				break
			}
		}
		if found {
			d.instances[serviceName] = instances
		} else {
			d.instances[serviceName] = append(instances, inst)
		}

	case clientv3.EventTypeDelete:
		// 删除实例
		instances := d.instances[serviceName]
		for i, existing := range instances {
			if existing.ID == instanceID {
				d.instances[serviceName] = append(instances[:i], instances[i+1:]...)
				break
			}
		}
		// 如果服务下没有实例了，删除整个 key
		if len(d.instances[serviceName]) == 0 {
			delete(d.instances, serviceName)
		}
	}
}

// GetInstances 获取指定服务名的所有实例（从本地缓存读取）
func (d *EtcdDiscovery) GetInstances(serviceName string) ([]ServiceInstance, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	instances, ok := d.instances[serviceName]
	if !ok || len(instances) == 0 {
		return nil, fmt.Errorf("服务 %q 未找到", serviceName)
	}

	// 返回深拷贝，避免调用方修改缓存（Metadata 是 map 引用类型）
	result := make([]ServiceInstance, len(instances))
	for i, inst := range instances {
		result[i] = inst
		if inst.Metadata != nil {
			m := make(map[string]string, len(inst.Metadata))
			for k, v := range inst.Metadata {
				m[k] = v
			}
			result[i].Metadata = m
		}
	}
	return result, nil
}

// Watch 监听指定服务的实例变更（当前通过全局 Watch 实现，此方法预留）
func (d *EtcdDiscovery) Watch(_ string, _ func([]ServiceInstance)) error {
	// 全局 watchAll 已覆盖所有服务的变更监听
	return nil
}

// Stop 停止 Watch 并关闭 etcd 客户端
func (d *EtcdDiscovery) Stop() error {
	d.cancel()
	d.wg.Wait()
	return d.client.Close()
}

// Ping 检测 etcd 连接是否可用（实现 health.DiscoveryPinger 接口）
func (d *EtcdDiscovery) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := d.client.Get(ctx, "ping")
	return err
}
