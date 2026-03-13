# Etcd 服务发现实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现基于 etcd 的服务发现，支持通过 etcd 前缀 Watch 实时感知后端服务实例的上下线。

**Architecture:** 服务实例以 JSON 格式注册在 etcd 的 `/services/{serviceName}/{instanceId}` 路径下。`EtcdDiscovery` 启动时通过前缀 Get 加载全量实例到本地缓存，随后通过 etcd Watch API 持续监听变更（PUT/DELETE 事件），实时更新本地缓存。`GetInstances` 直接读取本地缓存，零网络开销。

**Tech Stack:** `go.etcd.io/etcd/client/v3`（已在 go.mod 中）、`encoding/json`、`sync.RWMutex`

---

## 设计决策

### etcd Key 约定

```
{prefix}/{serviceName}/{instanceId}
```

- `prefix` 默认 `/services`，可通过配置覆盖
- `instanceId` 由注册方自行生成（如 UUID 或 `host:port`）
- Value 为 JSON：`{"host":"10.0.0.1","port":8081,"weight":1,"metadata":{"version":"v2"}}`

### EtcdConfig 扩展

当前 `EtcdConfig` 只有 `Endpoints` 和 `Timeout`，需要新增：
- `Prefix string` — key 前缀，默认 `/services`
- `Username string` — 认证用户名（可选）
- `Password string` — 认证密码（可选）

### 本地缓存

```
map[serviceName][]ServiceInstance  // 受 sync.RWMutex 保护
```

- `GetInstances` 使用 RLock 读取，返回深拷贝（含 Metadata map）
- Watch 回调使用 Lock 写入
- 全量实例通过前缀 Get 初始化

### Watch 断线说明

当前 `watchAll` 在 Watch channel 关闭时直接退出，不做自动重连。这是 MVP 实现，后续可按需添加带退避的重连逻辑。

### gateway.yaml 示例（etcd 服务发现）

```yaml
discovery:
  type: "etcd"
  etcd:
    endpoints:
      - "localhost:2379"
    prefix: "/services"
    timeout: 5s
    # username: ""
    # password: ""
```

---

## 文件结构

| 文件 | 操作 | 职责 |
|------|------|------|
| `config/config.go` | 修改 | `EtcdConfig` 新增 `Prefix`、`Username`、`Password` 字段 |
| `discovery/etcd.go` | 重写 | `EtcdDiscovery` 完整实现 |
| `discovery/etcd_test.go` | 新建 | 单元测试（mock etcd 或使用 embed） |
| `cmd/gateway/main.go` | 修改 | 添加 `"etcd"` case 到 discovery switch |

---

## Chunk 1: 核心实现

### Task 1: 扩展 EtcdConfig

**Files:**
- Modify: `config/config.go:269-273`

- [ ] **Step 1: 修改 EtcdConfig 结构体**

```go
// EtcdConfig etcd服务发现配置
type EtcdConfig struct {
	Endpoints []string      `mapstructure:"endpoints"`
	Prefix    string        `mapstructure:"prefix"`
	Timeout   time.Duration `mapstructure:"timeout"`
	Username  string        `mapstructure:"username"`
	Password  string        `mapstructure:"password"`
}
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./config/...`
Expected: 编译成功，无错误

- [ ] **Step 3: 提交**

```bash
git add config/config.go
git commit -m "feat(config): EtcdConfig 新增 Prefix/Username/Password 字段"
```

---

### Task 2: 实现 EtcdDiscovery

**Files:**
- Rewrite: `discovery/etcd.go`

- [ ] **Step 1: 编写 EtcdDiscovery 结构体和构造函数**

```go
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
	if err := d.loadAll(ctx, timeout); err != nil {
		cancel()
		client.Close()
		return nil, fmt.Errorf("加载 etcd 服务实例失败: %w", err)
	}

	// 启动全局 Watch
	d.wg.Add(1)
	go d.watchAll(ctx)

	return d, nil
}
```

- [ ] **Step 2: 实现 loadAll 方法（前缀 Get 加载全量实例）**

```go
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
```

- [ ] **Step 3: 实现 parseKey 和 parseValue 辅助方法**

```go
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
```

- [ ] **Step 4: 实现 watchAll 方法（前缀 Watch 监听变更）**

```go
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
```

- [ ] **Step 5: 实现 Discovery 接口方法**

```go
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
```

- [ ] **Step 6: 验证编译通过**

Run: `go build ./discovery/...`
Expected: 编译成功

- [ ] **Step 7: 提交**

```bash
git add discovery/etcd.go
git commit -m "feat(discovery): 实现基于 etcd 的服务发现"
```

---

### Task 3: 编写 EtcdDiscovery 单元测试

**Files:**
- Create: `discovery/etcd_test.go`

> 注意：测试使用 `parseKey` 和 `parseValue` 的间接验证，以及对 `GetInstances` 的缓存行为测试。
> 由于 etcd 集成测试需要真实 etcd 实例，这里只测试不依赖网络的逻辑。

- [ ] **Step 1: 编写 parseKey 测试**

```go
package discovery

import (
	"testing"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/api/v3/mvccpb"
)

func TestEtcdDiscovery_parseKey(t *testing.T) {
	d := &EtcdDiscovery{prefix: "/services/"}

	tests := []struct {
		key         string
		wantService string
		wantID      string
	}{
		{"/services/user-svc/inst-1", "user-svc", "inst-1"},
		{"/services/order-svc/10.0.0.1:8080", "order-svc", "10.0.0.1:8080"},
		{"/services/", "", ""},           // 缺少 serviceName 和 instanceID
		{"/services/user-svc", "", ""},   // 缺少 instanceID
		{"/services/user-svc/", "", ""},  // instanceID 为空
		{"/other/user-svc/inst-1", "", ""}, // 前缀不匹配（TrimPrefix 后格式不对）
	}

	for _, tt := range tests {
		svc, id := d.parseKey(tt.key)
		if svc != tt.wantService || id != tt.wantID {
			t.Errorf("parseKey(%q) = (%q, %q), want (%q, %q)",
				tt.key, svc, id, tt.wantService, tt.wantID)
		}
	}
}
```

- [ ] **Step 2: 编写 parseValue 测试**

```go
func TestEtcdDiscovery_parseValue(t *testing.T) {
	d := &EtcdDiscovery{}

	value := []byte(`{"host":"10.0.0.1","port":8081,"weight":2,"metadata":{"version":"v2"}}`)
	inst, err := d.parseValue("user-svc", "inst-1", value)
	if err != nil {
		t.Fatalf("parseValue() error: %v", err)
	}
	if inst.ID != "inst-1" {
		t.Errorf("ID = %q, want %q", inst.ID, "inst-1")
	}
	if inst.Host != "10.0.0.1" {
		t.Errorf("Host = %q, want %q", inst.Host, "10.0.0.1")
	}
	if inst.Port != 8081 {
		t.Errorf("Port = %d, want 8081", inst.Port)
	}
	if inst.Weight != 2 {
		t.Errorf("Weight = %d, want 2", inst.Weight)
	}
	if inst.Metadata["version"] != "v2" {
		t.Errorf("Metadata[version] = %q, want %q", inst.Metadata["version"], "v2")
	}
}

func TestEtcdDiscovery_parseValue_InvalidJSON(t *testing.T) {
	d := &EtcdDiscovery{}

	_, err := d.parseValue("svc", "id", []byte("not-json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
```

- [ ] **Step 3: 编写 GetInstances 缓存读取测试**

```go
func TestEtcdDiscovery_GetInstances_FromCache(t *testing.T) {
	d := &EtcdDiscovery{
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081, Weight: 1},
				{ID: "inst-2", Name: "user-svc", Host: "10.0.0.2", Port: 8082, Weight: 2},
			},
		},
	}

	instances, err := d.GetInstances("user-svc")
	if err != nil {
		t.Fatalf("GetInstances() error: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("got %d instances, want 2", len(instances))
	}
	if instances[0].Port != 8081 {
		t.Errorf("instances[0].Port = %d, want 8081", instances[0].Port)
	}
}

func TestEtcdDiscovery_GetInstances_NotFound(t *testing.T) {
	d := &EtcdDiscovery{
		instances: make(map[string][]ServiceInstance),
	}

	_, err := d.GetInstances("unknown-svc")
	if err == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
}

func TestEtcdDiscovery_GetInstances_ReturnsCopy(t *testing.T) {
	d := &EtcdDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {
				{ID: "1", Name: "svc", Host: "10.0.0.1", Port: 8080, Metadata: map[string]string{"k": "v"}},
			},
		},
	}

	result, _ := d.GetInstances("svc")
	result[0].Host = "mutated"
	result[0].Metadata["k"] = "mutated"

	original, _ := d.GetInstances("svc")
	if original[0].Host == "mutated" {
		t.Fatal("GetInstances 未返回 slice 副本")
	}
	if original[0].Metadata["k"] == "mutated" {
		t.Fatal("GetInstances 未深拷贝 Metadata map")
	}
}
```

- [ ] **Step 4: 编写 handleEvent 测试**

```go
func TestEtcdDiscovery_handleEvent_Put(t *testing.T) {
	d := &EtcdDiscovery{
		prefix:    "/services/",
		instances: make(map[string][]ServiceInstance),
	}

	// 模拟 PUT 事件
	d.handleEvent(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("/services/user-svc/inst-1"),
			Value: []byte(`{"host":"10.0.0.1","port":8081,"weight":1}`),
		},
	})

	instances, err := d.GetInstances("user-svc")
	if err != nil {
		t.Fatalf("GetInstances() error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	if instances[0].Host != "10.0.0.1" {
		t.Errorf("Host = %q, want %q", instances[0].Host, "10.0.0.1")
	}
}

func TestEtcdDiscovery_handleEvent_PutUpdate(t *testing.T) {
	d := &EtcdDiscovery{
		prefix: "/services/",
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081, Weight: 1},
			},
		},
	}

	// 模拟更新已有实例
	d.handleEvent(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("/services/user-svc/inst-1"),
			Value: []byte(`{"host":"10.0.0.2","port":9090,"weight":3}`),
		},
	})

	instances, _ := d.GetInstances("user-svc")
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	if instances[0].Host != "10.0.0.2" || instances[0].Port != 9090 {
		t.Errorf("instance not updated: %+v", instances[0])
	}
}

func TestEtcdDiscovery_handleEvent_Delete(t *testing.T) {
	d := &EtcdDiscovery{
		prefix: "/services/",
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081},
				{ID: "inst-2", Name: "user-svc", Host: "10.0.0.2", Port: 8082},
			},
		},
	}

	// 删除 inst-1
	d.handleEvent(&clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv: &mvccpb.KeyValue{
			Key: []byte("/services/user-svc/inst-1"),
		},
	})

	instances, _ := d.GetInstances("user-svc")
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	if instances[0].ID != "inst-2" {
		t.Errorf("wrong instance remaining: %+v", instances[0])
	}
}

func TestEtcdDiscovery_handleEvent_DeleteLast(t *testing.T) {
	d := &EtcdDiscovery{
		prefix: "/services/",
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081},
			},
		},
	}

	// 删除最后一个实例
	d.handleEvent(&clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv: &mvccpb.KeyValue{
			Key: []byte("/services/user-svc/inst-1"),
		},
	})

	_, err := d.GetInstances("user-svc")
	if err == nil {
		t.Fatal("expected error after deleting last instance, got nil")
	}
}
```

- [ ] **Step 5: 运行测试**

Run: `go test ./discovery/ -v -run TestEtcd`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add discovery/etcd_test.go
git commit -m "test(discovery): 添加 EtcdDiscovery 单元测试"
```

---

### Task 4: 接入 main.go

**Files:**
- Modify: `cmd/gateway/main.go:35-47`

- [ ] **Step 1: 在 discovery switch 中添加 etcd case**

在 `cmd/gateway/main.go` 的 `switch cfg.Discovery.Type` 中，在 `case "static":` 之后添加：

```go
	case "etcd":
		if cfg.Discovery.Etcd == nil {
			slog.Error("etcd 服务发现配置缺失")
			os.Exit(1)
		}
		var etcdErr error
		disc, etcdErr = discovery.NewEtcdDiscovery(cfg.Discovery.Etcd)
		if etcdErr != nil {
			slog.Error("初始化 etcd 服务发现失败", "error", etcdErr)
			os.Exit(1)
		}
```

> `NewEtcdDiscovery` 返回 `(*EtcdDiscovery, error)`，`*EtcdDiscovery` 实现了 `Discovery` 接口，
> 因此可以直接赋值给 `disc`。使用 `etcdErr` 避免与外层 `err` 变量冲突。

- [ ] **Step 2: 验证编译通过**

Run: `go build ./cmd/gateway/`
Expected: 编译成功

- [ ] **Step 3: 提交**

```bash
git add cmd/gateway/main.go
git commit -m "feat(gateway): main.go 接入 etcd 服务发现"
```

---

### Task 5: 全量验证

- [ ] **Step 1: 运行全部测试**

Run: `go test ./... -count=1`
Expected: 全部 PASS

- [ ] **Step 2: 编译网关**

Run: `go build ./cmd/gateway/`
Expected: 编译成功

- [ ] **Step 3: 最终提交（如有遗漏）**

确认所有变更已提交。
