package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dysodeng/gateway/sdk"
	"github.com/google/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// 服务实例状态常量，与网关 discovery 侧保持一致
const (
	statusUp   = "up"   // 正常服务
	statusDown = "down" // 已下线
)

// instanceValue etcd 中存储的服务实例 JSON 结构，与网关 discovery 侧 etcdInstance 保持一致
type instanceValue struct {
	ID           string            `json:"id"`
	ServiceName  string            `json:"service_name"`
	Host         string            `json:"host"`
	Port         int               `json:"port"`
	Weight       int               `json:"weight"`
	Version      string            `json:"version,omitempty"`
	Status       string            `json:"status"`
	RegisteredAt string            `json:"registered_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Option 配置选项
type Option func(*options)

type options struct {
	prefix      string
	ttl         time.Duration
	dialTimeout time.Duration
	username    string
	password    string

	// 健康检查配置
	healthChecker  sdk.HealthChecker
	healthInterval time.Duration
}

func defaultOptions() *options {
	return &options{
		prefix:         "/services/",
		ttl:            10 * time.Second,
		dialTimeout:    5 * time.Second,
		healthInterval: 5 * time.Second,
	}
}

// WithPrefix 设置 key 前缀，默认 "/services/"
func WithPrefix(prefix string) Option {
	return func(o *options) { o.prefix = prefix }
}

// WithTTL 设置租约 TTL，默认 10s
func WithTTL(ttl time.Duration) Option {
	return func(o *options) { o.ttl = ttl }
}

// WithDialTimeout 设置连接超时，默认 5s
func WithDialTimeout(timeout time.Duration) Option {
	return func(o *options) { o.dialTimeout = timeout }
}

// WithAuth 设置 etcd 认证信息
func WithAuth(username, password string) Option {
	return func(o *options) {
		o.username = username
		o.password = password
	}
}

// WithHealthChecker 设置健康检查函数和检查间隔
// 当健康检查失败时自动注销实例，恢复后自动重新注册
func WithHealthChecker(checker sdk.HealthChecker, interval time.Duration) Option {
	return func(o *options) {
		o.healthChecker = checker
		if interval > 0 {
			o.healthInterval = interval
		}
	}
}

// registeredInstance 已注册实例的运行时状态
type registeredInstance struct {
	instance     sdk.ServiceInstance
	registeredAt string // 注册时间戳
	leaseID      clientv3.LeaseID
	cancel       context.CancelFunc // 取消该实例的后台 goroutine
	healthy      bool               // 当前健康状态
}

// Registry 基于 etcd 的服务注册实现
type Registry struct {
	client *clientv3.Client
	opts   *options

	mu        sync.Mutex
	instances map[string]*registeredInstance // instanceKey -> state
	closed    bool
}

// NewRegistry 创建 etcd 服务注册器
func NewRegistry(endpoints []string, opts ...Option) (*Registry, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	// 确保 prefix 以 / 结尾
	if !strings.HasSuffix(o.prefix, "/") {
		o.prefix += "/"
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: o.dialTimeout,
		Username:    o.username,
		Password:    o.password,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 etcd 失败: %w", err)
	}

	return &Registry{
		client:    client,
		opts:      o,
		instances: make(map[string]*registeredInstance),
	}, nil
}

// Register 注册服务实例到 etcd
// 内部自动维持心跳，KeepAlive 断开后自动重注册，配置了健康检查时不健康会自动摘除
func (r *Registry) Register(ctx context.Context, instance sdk.ServiceInstance) error {
	if instance.Weight <= 0 {
		instance.Weight = 1
	}
	if instance.ID == "" {
		instance.ID = uuid.New().String()
	}

	val, err := json.Marshal(instanceValue{
		ID:           instance.ID,
		ServiceName:  instance.Name,
		Host:         instance.Host,
		Port:         instance.Port,
		Weight:       instance.Weight,
		Version:      instance.Version,
		Status:       statusUp,
		RegisteredAt: time.Now().Format(time.RFC3339),
		Metadata:     instance.Metadata,
	})
	if err != nil {
		return fmt.Errorf("序列化实例数据失败: %w", err)
	}

	key := r.instanceKey(instance)

	// 执行首次注册
	leaseID, err := r.grantAndPut(ctx, key, val)
	if err != nil {
		return err
	}

	instCtx, instCancel := context.WithCancel(context.Background())

	ri := &registeredInstance{
		instance:     instance,
		registeredAt: time.Now().Format(time.RFC3339),
		leaseID:      leaseID,
		cancel:       instCancel,
		healthy:      true,
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		instCancel()
		return fmt.Errorf("注册器已关闭")
	}
	r.instances[key] = ri
	r.mu.Unlock()

	// 启动 KeepAlive 维持 + 断线重注册
	go r.keepAliveLoop(instCtx, key, ri)

	// 启动 key 监听，被外部删除时自动重新写入
	go r.watchKeyLoop(instCtx, key, ri)

	// 启动健康检查（如果配置了）
	if r.opts.healthChecker != nil {
		go r.healthCheckLoop(instCtx, key, ri)
	}

	return nil
}

// Deregister 主动注销服务实例
func (r *Registry) Deregister(ctx context.Context, instance sdk.ServiceInstance) error {
	if instance.ID == "" {
		return fmt.Errorf("实例 ID 不能为空")
	}

	key := r.instanceKey(instance)

	r.mu.Lock()
	ri, ok := r.instances[key]
	if ok {
		delete(r.instances, key)
	}
	r.mu.Unlock()

	if !ok {
		return nil
	}

	// 停止后台 goroutine
	ri.cancel()

	// 撤销租约，自动删除关联 key
	if _, err := r.client.Revoke(ctx, ri.leaseID); err != nil {
		return fmt.Errorf("撤销 etcd 租约失败: %w", err)
	}

	return nil
}

// Close 关闭注册器，注销所有实例并关闭 etcd 客户端
func (r *Registry) Close() error {
	r.mu.Lock()
	r.closed = true
	instances := make(map[string]*registeredInstance, len(r.instances))
	for k, v := range r.instances {
		instances[k] = v
	}
	r.instances = make(map[string]*registeredInstance)
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), r.opts.dialTimeout)
	defer cancel()

	// 停止所有后台 goroutine 并撤销租约
	for _, ri := range instances {
		ri.cancel()
		if _, err := r.client.Revoke(ctx, ri.leaseID); err != nil {
			slog.Warn("关闭时撤销租约失败", "error", err)
		}
	}

	return r.client.Close()
}

// grantAndPut 创建租约并写入 key
func (r *Registry) grantAndPut(ctx context.Context, key string, val []byte) (clientv3.LeaseID, error) {
	ttlSeconds := int64(r.opts.ttl.Seconds())
	if ttlSeconds < 1 {
		ttlSeconds = 1
	}

	lease, err := r.client.Grant(ctx, ttlSeconds)
	if err != nil {
		return 0, fmt.Errorf("创建 etcd 租约失败: %w", err)
	}

	_, err = r.client.Put(ctx, key, string(val), clientv3.WithLease(lease.ID))
	if err != nil {
		return 0, fmt.Errorf("写入 etcd 失败: %w", err)
	}

	return lease.ID, nil
}

// keepAliveLoop 维持心跳，KeepAlive 通道关闭后自动重注册
func (r *Registry) keepAliveLoop(ctx context.Context, key string, ri *registeredInstance) {
	for {
		ch, err := r.client.KeepAlive(ctx, ri.leaseID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("启动 KeepAlive 失败，稍后重试", "key", key, "error", err)
			if !r.sleepOrDone(ctx, r.opts.ttl/3) {
				return
			}
			continue
		}

		// 消费 KeepAlive 响应
		alive := true
		for alive {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-ch:
				if !ok {
					alive = false
				}
			}
		}

		// KeepAlive 通道关闭，说明租约已失效（网络断开、etcd 重启等）
		slog.Warn("KeepAlive 通道关闭，尝试重新注册", "key", key)

		if err := r.reRegister(ctx, key, ri); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("重新注册失败，稍后重试", "key", key, "error", err)
		}
	}
}

// watchKeyLoop 监听自身 key，被外部删除时立即重新写入
// 解决的问题：key 被手动删除但租约仍存活时，KeepAlive 不会感知到 key 丢失
func (r *Registry) watchKeyLoop(ctx context.Context, key string, ri *registeredInstance) {
	for {
		watchCh := r.client.Watch(ctx, key)
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-watchCh:
				if !ok {
					// Watch 通道关闭，重建
					goto RESTART
				}
				if resp.Err() != nil {
					slog.Warn("Watch key 错误，重建监听", "key", key, "error", resp.Err())
					goto RESTART
				}
				for _, ev := range resp.Events {
					if ev.Type == clientv3.EventTypeDelete {
						r.handleKeyDeleted(ctx, key, ri)
					}
				}
			}
		}
	RESTART:
		if !r.sleepOrDone(ctx, time.Second) {
			return
		}
	}
}

// handleKeyDeleted key 被外部删除时，重新用当前租约写入
func (r *Registry) handleKeyDeleted(ctx context.Context, key string, ri *registeredInstance) {
	r.mu.Lock()
	healthy := ri.healthy
	leaseID := ri.leaseID
	r.mu.Unlock()

	if !healthy {
		slog.Info("key 被删除但实例不健康，跳过重写", "key", key)
		return
	}

	slog.Warn("key 被外部删除，重新写入", "key", key)

	// 用当前租约重新 Put，无需新建租约
	val, err := r.buildValue(ri)
	if err != nil {
		slog.Error("构建实例数据失败", "key", key, "error", err)
		return
	}
	_, err = r.client.Put(ctx, key, string(val), clientv3.WithLease(leaseID))
	if err != nil {
		slog.Error("重新写入 key 失败", "key", key, "error", err)
	}
}

// reRegister 重新创建租约并写入 key，带退避重试
func (r *Registry) reRegister(ctx context.Context, key string, ri *registeredInstance) error {
	backoff := time.Second
	maxBackoff := r.opts.ttl

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 仅在健康状态下重注册
		r.mu.Lock()
		healthy := ri.healthy
		r.mu.Unlock()

		if !healthy {
			if !r.sleepOrDone(ctx, backoff) {
				return ctx.Err()
			}
			continue
		}

		val, err := r.buildValue(ri)
		if err != nil {
			slog.Warn("构建实例数据失败", "key", key, "error", err)
			if !r.sleepOrDone(ctx, backoff) {
				return ctx.Err()
			}
			continue
		}

		leaseID, err := r.grantAndPut(ctx, key, val)
		if err == nil {
			r.mu.Lock()
			ri.leaseID = leaseID
			r.mu.Unlock()
			slog.Info("重新注册成功", "key", key)
			return nil
		}

		slog.Warn("重新注册失败", "key", key, "error", err, "retry_after", backoff)
		if !r.sleepOrDone(ctx, backoff) {
			return ctx.Err()
		}

		// 指数退避
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// healthCheckLoop 定期执行健康检查，不健康时注销，恢复后重新注册
func (r *Registry) healthCheckLoop(ctx context.Context, key string, ri *registeredInstance) {
	ticker := time.NewTicker(r.opts.healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkCtx, cancel := context.WithTimeout(ctx, r.opts.healthInterval)
			err := r.opts.healthChecker(checkCtx)
			cancel()

			r.mu.Lock()
			wasHealthy := ri.healthy
			r.mu.Unlock()

			if err != nil && wasHealthy {
				// 健康 -> 不健康：主动摘除
				slog.Warn("健康检查失败，摘除实例", "key", key, "error", err)
				r.mu.Lock()
				ri.healthy = false
				r.mu.Unlock()
				r.revokeQuietly(ri.leaseID)

			} else if err == nil && !wasHealthy {
				// 不健康 -> 健康：重新注册
				slog.Info("健康检查恢复，重新注册实例", "key", key)
				r.mu.Lock()
				ri.healthy = true
				r.mu.Unlock()

				val, buildErr := r.buildValue(ri)
				if buildErr != nil {
					slog.Error("构建实例数据失败", "key", key, "error", buildErr)
				} else {
					leaseID, regErr := r.grantAndPut(ctx, key, val)
					if regErr != nil {
						slog.Error("健康恢复后重注册失败", "key", key, "error", regErr)
					} else {
						r.mu.Lock()
						ri.leaseID = leaseID
						r.mu.Unlock()
					}
				}
			}
		}
	}
}

// revokeQuietly 静默撤销租约
func (r *Registry) revokeQuietly(leaseID clientv3.LeaseID) {
	ctx, cancel := context.WithTimeout(context.Background(), r.opts.dialTimeout)
	defer cancel()
	if _, err := r.client.Revoke(ctx, leaseID); err != nil {
		slog.Warn("撤销租约失败", "error", err)
	}
}

// sleepOrDone 等待指定时间或 context 取消，返回 false 表示 context 已取消
func (r *Registry) sleepOrDone(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// instanceKey 生成 etcd key: {prefix}{serviceName}/{instanceID}
func (r *Registry) instanceKey(inst sdk.ServiceInstance) string {
	return r.opts.prefix + inst.Name + "/" + inst.ID
}

// buildValue 根据当前实例状态构建 JSON value
func (r *Registry) buildValue(ri *registeredInstance) ([]byte, error) {
	status := statusUp
	if !ri.healthy {
		status = statusDown
	}
	return json.Marshal(instanceValue{
		ID:           ri.instance.ID,
		ServiceName:  ri.instance.Name,
		Host:         ri.instance.Host,
		Port:         ri.instance.Port,
		Weight:       ri.instance.Weight,
		Version:      ri.instance.Version,
		Status:       status,
		RegisteredAt: ri.registeredAt,
		Metadata:     ri.instance.Metadata,
	})
}
