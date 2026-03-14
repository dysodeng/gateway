# 配置中心热更新 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 当 etcd 配置中心的网关配置发生变更时，网关自动感知并热更新运行时行为，无需重启。

**Architecture:** 在 `Source` 接口上扩展 `Watch` 能力，`EtcdSource` 通过 etcd Watch 监听 key 变更。引入 `config.Watcher` 组件解析变更后的 YAML 并通过回调通知上层。`server.Server` 持有可原子替换的 handler 管线——配置变更时重建路由、中间件链和负载均衡器，通过 `atomic.Value` 原子切换，正在处理的请求不受影响。不可热更新的字段（`server.listen`、`discovery`）变更时仅打印警告日志。

**Tech Stack:** Go stdlib (`sync/atomic`), etcd Watch API (`clientv3.Watch`)

---

## 热更新范围界定

| 分类 | 配置项 | 热更新 | 说明 |
|------|--------|--------|------|
| 路由 | routes (prefix, service, timeout, lb, middleware) | ✅ | 重建路由表+中间件链 |
| 认证 | auth_schemes | ✅ | 重建认证中间件 |
| CORS | cors | ✅ | 重建 CORS 中间件 |
| IP 过滤 | ip_filter (全局+路由级) | ✅ | 重建 IP 过滤中间件 |
| 限流 | rate_limit (全局+路由级) | ✅ | 重建限流中间件 |
| 请求签名 | request_sign | ✅ | 重建签名中间件 |
| 服务器 | server.listen | ❌ | 变更时打印警告 |
| 服务发现 | discovery | ❌ | 变更时打印警告 |
| 日志 | log | ❌ | 变更时打印警告 |
| 链路追踪 | telemetry | ❌ | 变更时打印警告 |
| 指标 | metrics | ❌ | 变更时打印警告 |

## 文件结构

| 操作 | 文件 | 职责 |
|------|------|------|
| 修改 | `config/source.go` | Source 接口增加 Watch 方法 |
| 修改 | `config/source_etcd.go` | EtcdSource 实现 Watch（etcd Watch API） |
| 修改 | `config/watcher.go` | 删除旧 Watcher 接口，新增 Watcher 结构体：监听配置源变更 → 解析 YAML → 回调通知 |
| 新建 | `config/watcher_test.go` | Watcher 单元测试 |
| 修改 | `config/loader.go` | LoadResult 增加 Source 引用，供 Watcher 使用 |
| 修改 | `server/server.go` | Server 支持原子替换 handler 管线，接收配置变更回调 |
| 新建 | `server/server_reload_test.go` | 热更新集成测试 |
| 修改 | `cmd/gateway/main.go` | 启动 Watcher，注册 Server.Reload 回调 |

---

## Chunk 1: 配置源 Watch 能力

### Task 1: Source 接口扩展 Watch 方法

**Files:**
- Modify: `config/source.go`

- [ ] **Step 1: 扩展 Source 接口**

```go
// Source 配置源接口
type Source interface {
	Load() ([]byte, error)
	Type() string
	// Watch 监听配置变更，变更时将新的 YAML 内容发送到 channel。
	// ctx 取消时停止监听并关闭 channel。
	Watch(ctx context.Context) (<-chan []byte, error)
}
```

需要在文件顶部增加 `"context"` import。

- [ ] **Step 2: 验证编译通过**

Run: `go build ./config/...`
Expected: 编译失败，因为 EtcdSource 未实现 Watch 方法（这是预期的，Task 2 会修复）

- [ ] **Step 3: Commit**

```bash
git add config/source.go
git commit -m "refactor(config): Source 接口增加 Watch 方法"
```

---

### Task 2: EtcdSource 实现 Watch

**Files:**
- Modify: `config/source_etcd.go`

- [ ] **Step 1: 重构 EtcdSource，持有长连接 client**

当前 `Load()` 每次调用都创建新连接再关闭，Watch 需要长连接。重构为在首次调用时创建 client 并复用：

```go
type EtcdSource struct {
	cfg    *EtcdSourceConfig
	mu     sync.Mutex
	client *clientv3.Client
}

// getClient 获取或创建 etcd 客户端（懒初始化）
func (s *EtcdSource) getClient() (*clientv3.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client, nil
	}
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
	s.client = client
	return s.client, nil
}
```

同时重写 `Load()` 使用 `getClient()`：

```go
func (s *EtcdSource) Load() ([]byte, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	timeout := s.cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
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
```

需要在 import 中增加 `"sync"`。

- [ ] **Step 2: 实现 Watch 方法**

```go
// Watch 监听 etcd key 变更，变更时将新的 YAML 内容发送到 channel
func (s *EtcdSource) Watch(ctx context.Context) (<-chan []byte, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	ch := make(chan []byte, 1)
	go func() {
		defer close(ch)
		watchCh := client.Watch(ctx, s.cfg.Key)
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-watchCh:
				if !ok {
					return
				}
				for _, event := range resp.Events {
					if event.Type == clientv3.EventTypePut && event.Kv != nil {
						select {
						case ch <- event.Kv.Value:
						default:
							// 丢弃旧的未消费变更，保留最新
							<-ch
							ch <- event.Kv.Value
						}
					}
				}
			}
		}
	}()
	return ch, nil
}
```

- [ ] **Step 3: 增加 Close 方法**

```go
// Close 关闭 etcd 客户端连接
func (s *EtcdSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		err := s.client.Close()
		s.client = nil
		return err
	}
	return nil
}
```

- [ ] **Step 4: 验证编译通过**

Run: `go build ./config/...`
Expected: PASS

- [ ] **Step 5: 运行现有测试确保无回归**

Run: `go test ./config/... -v`
Expected: 全部 PASS

- [ ] **Step 6: Commit**

```bash
git add config/source_etcd.go
git commit -m "feat(config): EtcdSource 实现 Watch 和长连接复用"
```

---

### Task 3: Watcher 组件

**Files:**
- Modify: `config/watcher.go`（删除旧的未使用 `Watcher` 接口，替换为 `Watcher` 结构体实现）
- Create: `config/watcher_test.go`

- [ ] **Step 1: 编写 Watcher 测试**

`config/watcher_test.go`:

```go
package config

import (
	"context"
	"testing"
	"time"
)

// mockWatchSource 支持 Watch 的 mock 配置源
type mockWatchSource struct {
	data    []byte
	err     error
	watchCh chan []byte
}

func (m *mockWatchSource) Load() ([]byte, error)                            { return m.data, m.err }
func (m *mockWatchSource) Type() string                                     { return "mock" }
func (m *mockWatchSource) Watch(ctx context.Context) (<-chan []byte, error)  { return m.watchCh, nil }

func TestWatcher_ReceivesUpdate(t *testing.T) {
	watchCh := make(chan []byte, 1)
	src := &mockWatchSource{watchCh: watchCh}

	var received *Config
	callback := func(cfg *Config) {
		received = cfg
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWatcher(src, callback)
	w.Start(ctx)

	// 模拟配置变更
	newYAML := []byte("server:\n  listen: \":9999\"\nroutes: []\n")
	watchCh <- newYAML

	// 等待回调触发
	time.Sleep(100 * time.Millisecond)

	if received == nil {
		t.Fatal("回调未触发")
	}
	if received.Server.Listen != ":9999" {
		t.Errorf("Server.Listen = %q, want %q", received.Server.Listen, ":9999")
	}
}

func TestWatcher_InvalidYAML(t *testing.T) {
	watchCh := make(chan []byte, 1)
	src := &mockWatchSource{watchCh: watchCh}

	callCount := 0
	callback := func(cfg *Config) {
		callCount++
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWatcher(src, callback)
	w.Start(ctx)

	// 发送无效 YAML
	watchCh <- []byte("{{invalid yaml")
	time.Sleep(100 * time.Millisecond)

	if callCount != 0 {
		t.Errorf("无效 YAML 不应触发回调，实际触发了 %d 次", callCount)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./config/... -run TestWatcher -v`
Expected: FAIL（Watcher 未定义）

- [ ] **Step 3: 实现 Watcher**

替换 `config/watcher.go` 的全部内容（删除旧的未使用 `Watcher` 接口）：

```go
package config

import (
	"bytes"
	"context"
	"log/slog"

	"github.com/spf13/viper"
)

// WatchCallback 配置变更回调函数
type WatchCallback func(cfg *Config)

// Watcher 监听配置源变更并触发回调
type Watcher struct {
	source   Source
	callback WatchCallback
}

// NewWatcher 创建配置监听器
func NewWatcher(source Source, callback WatchCallback) *Watcher {
	return &Watcher{
		source:   source,
		callback: callback,
	}
}

// Start 开始监听配置变更（非阻塞，内部启动 goroutine）
func (w *Watcher) Start(ctx context.Context) error {
	ch, err := w.source.Watch(ctx)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-ch:
				if !ok {
					return
				}
				cfg, err := parseConfig(data)
				if err != nil {
					slog.Error("解析配置变更失败，忽略本次更新", "error", err)
					continue
				}
				applyRouteDefaults(cfg)
				slog.Info("检测到配置变更，正在应用")
				w.callback(cfg)
			}
		}
	}()
	return nil
}

// parseConfig 将 YAML 字节流解析为 Config
func parseConfig(data []byte) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	setDefaults(v)
	if err := v.ReadConfig(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./config/... -run TestWatcher -v`
Expected: PASS

- [ ] **Step 5: 运行全部 config 测试确保无回归**

Run: `go test ./config/... -v`
Expected: 全部 PASS

- [ ] **Step 6: Commit**

```bash
git add config/watcher.go config/watcher_test.go
git commit -m "feat(config): 新增 Watcher 监听配置源变更"
```

---

## Chunk 2: Server 热更新与集成

### Task 4: LoadResult 暴露 Source 引用

**Files:**
- Modify: `config/loader.go`

- [ ] **Step 1: LoadResult 增加 WatchSource 字段**

仅当配置来源为配置中心时才设置此字段，本地配置不需要 Watch：

```go
type LoadResult struct {
	Config      *Config
	Source      string // 配置来源: "local" 或配置中心类型如 "etcd"
	SourcePath  string // 配置路径: 本地文件路径或远程 key
	WatchSource Source // 可 Watch 的配置源（仅配置中心模式下非 nil）
}
```

在 `Load()` 中，配置中心加载成功时设置 `WatchSource`：

```go
// 阶段2 成功分支
return &LoadResult{
	Config:      cfg,
	Source:      source.Type(),
	SourcePath:  sourceKey(bootstrapCfg.ConfigCenter),
	WatchSource: source,
}, nil
```

- [ ] **Step 2: 验证编译和测试通过**

Run: `go build ./... && go test ./config/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add config/loader.go
git commit -m "refactor(config): LoadResult 暴露 WatchSource 供热更新使用"
```

---

### Task 5: Server 支持原子替换 handler

**Files:**
- Modify: `server/server.go`
- Create: `server/server_reload_test.go`

- [ ] **Step 1: 编写热更新测试**

`server/server_reload_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
)

// stubDiscovery 用于测试的 stub 服务发现
type stubDiscovery struct{}

func (s *stubDiscovery) GetInstances(service string) ([]*discovery.ServiceInstance, error) {
	return []*discovery.ServiceInstance{
		{Host: "127.0.0.1", Port: 8081, Weight: 1},
	}, nil
}
func (s *stubDiscovery) Stop() error { return nil }

func TestServer_Reload(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Listen: ":0"},
		CORS:   config.CORSConfig{AllowedOrigins: []string{"*"}},
		Health: config.HealthConfig{Path: "/health"},
		Routes: []config.RouteConfig{
			{
				Name:         "test",
				Prefix:       "/api/test",
				Service:      "test-svc",
				Timeout:      5 * time.Second,
				LoadBalancer: "round_robin",
			},
		},
	}

	srv := New(cfg, &stubDiscovery{})

	// 初始请求 — /api/test 应匹配
	req := httptest.NewRequest(http.MethodGet, "/api/test/hello", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Error("初始配置下 /api/test 不应返回 404")
	}

	// Reload：改变路由前缀
	newCfg := *cfg
	newCfg.Routes = []config.RouteConfig{
		{
			Name:         "test-v2",
			Prefix:       "/api/v2/test",
			Service:      "test-svc",
			Timeout:      5 * time.Second,
			LoadBalancer: "round_robin",
		},
	}
	srv.Reload(&newCfg)

	// 旧路由应返回 404
	req = httptest.NewRequest(http.MethodGet, "/api/test/hello", nil)
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Reload 后旧路由应返回 404，实际: %d", w.Code)
	}

	// 新路由应匹配
	req = httptest.NewRequest(http.MethodGet, "/api/v2/test/hello", nil)
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Error("Reload 后新路由不应返回 404")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./server/... -run TestServer_Reload -v`
Expected: FAIL（Reload 方法未定义）

- [ ] **Step 3: 重构 Server，handler 管线可替换**

核心思路：将 `http.Server.Handler` 指向一个委托 handler，它内部通过 `atomic.Value` 持有实际的 handler。Reload 时重建管线并原子替换。

修改 `server/server.go`：

```go
type Server struct {
	cfg        *config.Config
	httpServer *http.Server
	discovery  discovery.Discovery
	handler    atomic.Value // 存储 http.Handler
	healthPath string
}
```

抽取 `buildHandler` 方法，将当前 `New()` 中构建 mux 的逻辑提取出来：

```go
// buildHandler 根据配置构建完整的请求处理管线
func (s *Server) buildHandler(cfg *config.Config) http.Handler {
	r := router.New(cfg.Routes)
	dispatcher := proxy.NewDispatcher()
	balancers := buildBalancers(cfg.Routes)

	coreHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// ... 现有的路由匹配 → 代理转发逻辑（使用 cfg 参数而非 s.cfg）
	})

	preRoute := middleware.Chain(
		middleware.NewRecovery(),
		middleware.NewAccessLog(),
		middleware.NewCORS(cfg.CORS),
		middleware.NewTracing(),
		middleware.NewGlobalIPFilter(cfg.IPFilter),
	)

	checkers := buildHealthCheckers(cfg, s.discovery)
	mux := http.NewServeMux()
	mux.HandleFunc(s.healthPath, health.Handler(checkers...))
	mux.Handle("/", preRoute(coreHandler))
	return mux
}
```

修改 `New()`：

```go
func New(cfg *config.Config, disc discovery.Discovery) *Server {
	s := &Server{
		cfg:        cfg,
		discovery:  disc,
		healthPath: cfg.Health.Path,
	}

	h := s.buildHandler(cfg)
	s.handler.Store(h)

	s.httpServer = &http.Server{
		Addr: cfg.Server.Listen,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.handler.Load().(http.Handler).ServeHTTP(w, r)
		}),
	}
	return s
}
```

增加 `Reload` 方法（注意：`buildHandler` 仅使用参数 `cfg`，不引用 `s.cfg`，因此 `s.cfg = newCfg` 的赋值顺序是安全的——新 handler 已经原子存储后才更新 `s.cfg`，`s.cfg` 仅用于下次 Reload 时的变更检测）：

```go
// Reload 热更新配置，重建请求处理管线并原子替换
func (s *Server) Reload(newCfg *config.Config) {
	// 不可热更新字段变更检测
	if newCfg.Server.Listen != s.cfg.Server.Listen {
		slog.Warn("server.listen 变更需要重启才能生效",
			"current", s.cfg.Server.Listen,
			"new", newCfg.Server.Listen,
		)
	}
	if newCfg.Discovery.Type != s.cfg.Discovery.Type {
		slog.Warn("discovery.type 变更需要重启才能生效",
			"current", s.cfg.Discovery.Type,
			"new", newCfg.Discovery.Type,
		)
	}

	h := s.buildHandler(newCfg)
	s.handler.Store(h)
	s.cfg = newCfg
	slog.Info("配置热更新完成", "routes", len(newCfg.Routes))
}
```

需要在 import 中增加 `"sync/atomic"`。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./server/... -run TestServer_Reload -v`
Expected: PASS

- [ ] **Step 5: 运行全部测试确保无回归**

Run: `go test ./... -v`
Expected: 全部 PASS

- [ ] **Step 6: Commit**

```bash
git add server/server.go server/server_reload_test.go
git commit -m "feat(server): 支持配置热更新，原子替换 handler 管线"
```

---

### Task 6: main.go 集成 Watcher

**Files:**
- Modify: `cmd/gateway/main.go`

- [ ] **Step 1: 启动 Watcher 并注册 Reload 回调**

在 `main()` 中，`server.New()` 之后、`srv.Start()` 之前，增加 Watcher 启动逻辑：

```go
// 如果配置来自配置中心，启动热更新监听
if result.WatchSource != nil {
	watcher := config.NewWatcher(result.WatchSource, func(newCfg *config.Config) {
		srv.Reload(newCfg)
	})
	watchCtx, watchCancel := context.WithCancel(context.Background())
	if err := watcher.Start(watchCtx); err != nil {
		slog.Error("启动配置监听失败", "error", err)
		watchCancel()
	} else {
		slog.Info("配置热更新已启用", "source", result.Source, "path", result.SourcePath)
		shutdowns = append(shutdowns, func(ctx context.Context) error {
			watchCancel()
			return nil
		})
	}
}
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./cmd/gateway/...`
Expected: PASS

- [ ] **Step 3: 运行全部测试**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: 全部 PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/gateway/main.go
git commit -m "feat: 集成配置热更新，配置中心变更自动 Reload"
```

---

### Task 7: 端到端验证

- [ ] **Step 1: 启动网关连接 etcd 配置中心**

确认启动日志包含：
```
INFO 配置加载完成 source=etcd path=/ai-adp/dev/gateway.yaml
INFO 配置热更新已启用 source=etcd path=/ai-adp/dev/gateway.yaml
```

- [ ] **Step 2: 在 etcd 中修改配置**

通过 etcdctl 修改路由配置，确认网关日志输出：
```
INFO 检测到配置变更，正在应用
INFO 配置热更新完成 routes=N
```

- [ ] **Step 3: 验证新配置生效**

发送请求验证新路由/中间件配置已生效。

- [ ] **Step 4: 验证不可热更新字段的警告**

修改 etcd 中的 `server.listen`，确认日志输出警告而非崩溃。
