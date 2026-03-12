# API Gateway 设计文档

## 概述

基于 Go 的微服务 API 网关，支持 RESTful、WebSocket、SSE 三种协议，后端服务语言无关，统一走 HTTP 协议通信。采用标准分层架构，无状态部署，支持水平扩展。

## 架构

```
Client → HTTP Server → Pre-Route Middleware → Router → Post-Route Middleware → Proxy → Backend (HTTP)
                                                                                 ├─ HTTP 反向代理 (REST)
                                                                                 ├─ WebSocket 透传
                                                                                 └─ SSE 透传
```

### 核心分层

| 层 | 职责 |
|---|------|
| Server | HTTP Server 启动、优雅关闭 |
| Pre-Route Middleware | 全局中间件，不依赖路由信息：Recovery、AccessLog、CORS、Tracing、IPFilter |
| Router | 路由匹配、灰度分流、负载均衡选实例 |
| Post-Route Middleware | 路由级中间件，依赖路由配置：Auth、RateLimit、RequestSign、Rewrite |
| Proxy | 请求转发（含熔断、重试），按协议类型（REST/WS/SSE）选择对应代理 |
| Discovery | 接口化服务发现，注册中心不可用时回退静态路由 |
| Config | 文件加载基础配置 + 配置中心 Watch 动态配置 |

### 中间件两阶段执行

中间件分为 Pre-Route（路由前）和 Post-Route（路由后）两个阶段：

- **Pre-Route**（全局，与路由无关）：Recovery → AccessLog → CORS → Tracing → IPFilter（全局规则）
- **Post-Route**（路由级，依赖路由匹配结果）：IPFilter（路由级规则） → Auth → RateLimit → RequestSign → Rewrite

每个中间件通过配置控制启用/禁用，未启用直接跳过。

### 熔断与重试

熔断器和重试逻辑位于 Proxy 层而非中间件层，因为它们按后端服务维度工作：

- **熔断器**：按目标服务实例跟踪失败率，达到阈值后短路请求，进入半开状态探测恢复
- **重试**：仅在 5xx/超时时触发，非幂等请求（POST/PATCH/DELETE）默认不重试

## 项目结构

```
gateway/
├── cmd/
│   └── gateway/
│       └── main.go              # 入口
├── config/
│   ├── config.go                # 配置结构定义
│   ├── loader.go                # 文件配置加载
│   └── watcher.go               # 配置中心 Watch 接口
├── server/
│   └── server.go                # HTTP Server 启动、优雅关闭
├── middleware/
│   ├── chain.go                 # 中间件链编排（Pre-Route & Post-Route）
│   ├── accesslog.go             # [Pre-Route]
│   ├── recovery.go              # [Pre-Route]
│   ├── cors.go                  # [Pre-Route]
│   ├── tracing.go               # [Pre-Route]
│   ├── ipfilter.go              # 导出 NewGlobalIPFilter (Pre-Route) 和 NewRouteIPFilter (Post-Route)
│   ├── auth.go                  # [Post-Route] JWT/APIKey/OAuth2
│   ├── ratelimit.go             # [Post-Route]
│   ├── requestsign.go           # [Post-Route]
│   └── rewrite.go               # [Post-Route]
├── router/
│   ├── router.go                # 路由匹配
│   ├── canary.go                # 灰度分流
│   └── loadbalancer/
│       ├── balancer.go          # 接口定义
│       ├── roundrobin.go
│       ├── weighted.go
│       ├── random.go
│       ├── iphash.go
│       └── leastconn.go
├── proxy/
│   ├── proxy.go                 # 代理调度（识别 REST/WS/SSE）
│   ├── http.go                  # HTTP 反向代理
│   ├── websocket.go             # WebSocket 透传
│   ├── sse.go                   # SSE 透传
│   ├── circuitbreaker.go        # 熔断器（按服务实例维度）
│   └── retry.go                 # 重试逻辑
├── discovery/
│   ├── discovery.go             # 服务发现接口
│   ├── static.go                # 静态路由实现
│   ├── etcd.go
│   ├── consul.go
│   └── nacos.go
└── pkg/
    ├── trace/
    │   └── trace.go             # OpenTelemetry 初始化 & Trace 传播
    └── health/
        └── health.go            # 健康检查
```

## 核心接口

### 服务发现

```go
// discovery/discovery.go
type ServiceInstance struct {
    ID       string
    Name     string
    Host     string
    Port     int
    Weight   int
    Metadata map[string]string
}

type Discovery interface {
    // 获取服务实例列表
    GetInstances(serviceName string) ([]ServiceInstance, error)
    // 监听服务变更
    Watch(serviceName string, callback func([]ServiceInstance)) error
    // 停止监听
    Stop() error
}
```

启动时优先使用注册中心实现，不可用时自动降级到静态路由实现。

### 负载均衡

```go
// router/loadbalancer/balancer.go
// Balancer 依赖 discovery.ServiceInstance 类型
type Balancer interface {
    Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error)
}
```

支持策略：轮询、加权轮询、随机、IP Hash、最少连接数，路由级别可配置。

### 配置中心 Watch

```go
// config/watcher.go
type ConfigWatcher interface {
    Get(key string) ([]byte, error)
    Watch(key string, callback func(value []byte)) error
    Stop() error
}
```

## 路由匹配与流量控制

### 路由配置

```yaml
routes:
  - name: "user-service"
    prefix: "/api/v1/users"
    strip_prefix: false
    service: "user-service"
    timeout: 5s
    retry:
      count: 2
      conditions: ["5xx", "timeout"]
    load_balancer: "round_robin"
    middleware:
      auth:
        scheme: "app_user"
      rate_limit:
        enabled: true
        qps: 100
    canary:
      - weight: 10
        service: "user-service-v2"
        match:
          headers:
            x-canary: "true"
      - weight: 0
        service: "user-service-v2"
        match:
          headers:
            x-user-group: "beta"
      - weight: 5                    # 无 Header 条件，5% 全量流量随机分流
        service: "user-service-v3"
```

### 匹配流程

```
请求进入 → 最长前缀匹配路由 → 检查灰度规则 → 选定目标服务 → 负载均衡选实例 → 转发
```

- 灰度分流决策树：
  1. 遍历 canary 规则，检查 `match.headers` 条件
  2. 若 Header 匹配且 `weight > 0`：按权重概率决定是否走灰度服务
  3. 若 Header 匹配且 `weight == 0`：100% 走灰度服务（纯 Header 匹配模式）
  4. 若无 Header 条件但 `weight > 0`：按权重概率对所有流量随机分流
  5. 所有规则都不命中：走默认服务
- 重试与熔断：位于 Proxy 层，详见"熔断与重试"章节
- 路径改写：支持 `strip_prefix` 和自定义 rewrite 规则

## WebSocket 与 SSE

### 协议识别

- **WebSocket**：`Connection: Upgrade` + `Upgrade: websocket`
- **SSE**：路由配置 `type: "sse"`（仅通过路由配置识别，不依赖 Accept Header 以避免误判）
- **REST**：其余请求

### WebSocket 透明代理

```
Client ←──WS──→ Gateway ←──WS──→ Backend
```

- 网关 hijack 连接，与后端建立 WebSocket 连接，双向透传帧数据
- 中间件在握手阶段执行（认证、限流等），建连后不再介入
- 连接生命周期管理：心跳、超时断开
- 后端断开时通知客户端，支持自动重连后端

```yaml
routes:
  - name: "realtime"
    prefix: "/ws/chat"
    type: "websocket"
    service: "chat-service"
    websocket:
      heartbeat: 30s
      max_connections: 10000
```

### SSE 透明代理

```
Client ←──SSE──→ Gateway ←──HTTP(chunked)──→ Backend
```

- 网关向后端发起请求，后端以 chunked/streaming 方式响应，网关实时 flush 给客户端

```yaml
routes:
  - name: "notifications"
    prefix: "/events/notify"
    type: "sse"
    service: "notify-service"
    sse:
      retry: 3000                 # 客户端重连间隔(ms)
      keepalive: 15s              # 心跳注释行间隔
```

### 共同机制

- 长连接不计入限流 QPS（握手/建连时计一次）
- 连接数上限可配置
- 后端实例下线时，网关优雅断开并引导客户端重连到新实例

## 认证与安全

### 多用户类型认证

全局定义认证方案，路由通过名称引用：

```yaml
auth_schemes:
  admin:
    type: "jwt"
    jwt:
      secret: "${from_config_center}"
      algorithms: ["RS256"]
      header: "Authorization"
      claims_to_headers:
        admin_id: "X-Admin-Id"
        role: "X-Admin-Role"
  app_user:
    type: "jwt"
    jwt:
      secret: "${from_config_center}"
      algorithms: ["HS256"]
      header: "Authorization"
      claims_to_headers:
        user_id: "X-User-Id"
        vip_level: "X-Vip-Level"
  open_api:
    type: "api_key"
    api_key:
      header: "X-API-Key"
      query: "api_key"
  third_party:
    type: "oauth2"
    oauth2:
      introspect_endpoint: "${from_config_center}"  # Token 内省端点 (RFC 7662)
      client_id: "${from_config_center}"
      client_secret: "${from_config_center}"
      claims_to_headers:          # 内省结果注入 Header
        sub: "X-OAuth-Subject"
        scope: "X-OAuth-Scope"
```

路由引用：

```yaml
routes:
  - name: "admin-api"
    prefix: "/api/admin"
    middleware:
      auth:
        scheme: "admin"
```

认证通过后，网关将用户信息注入 Header 传给后端。

### CORS

```yaml
cors:
  allowed_origins: ["*"]
  allowed_methods: ["GET", "POST", "PUT", "DELETE"]
  allowed_headers: ["Authorization", "Content-Type"]
  max_age: 3600
```

全局配置，路由可覆盖。

### 限流

```yaml
rate_limit:
  storage: "redis"                # local | redis
  redis:
    addr: "localhost:6379"
    password: ""
    db: 0
  algorithm: "sliding_window"     # token_bucket | sliding_window
```

路由级 QPS 配置见路由配置中的 `middleware.rate_limit.qps`。多实例部署时使用 Redis 保证限流一致性，单实例可用 local（内存）。

### IP 黑白名单

```yaml
ip_filter:
  mode: "whitelist"               # whitelist | blacklist
  list:
    - "10.0.0.0/8"
    - "192.168.1.100"
```

全局 + 路由级配置。

### 请求签名验证

```yaml
request_sign:
  enabled: true
  algorithm: "hmac-sha256"
  sign_header: "X-Signature"
  timestamp_header: "X-Timestamp"
  expire: 300                     # 签名有效期(秒)
```

全局配置定义签名参数，路由级通过 `middleware.request_sign.enabled` 控制是否启用：

```yaml
routes:
  - name: "payment-api"
    prefix: "/api/v1/payment"
    middleware:
      request_sign:
        enabled: true
```

## 可观测性

### OpenTelemetry

```yaml
telemetry:
  enabled: true
  service_name: "gateway"
  exporter:
    type: "otlp"
    protocol: "grpc"              # grpc | http
    endpoint: "localhost:4317"    # grpc 默认 4317，http 默认 4318
  sampler:
    type: "ratio"                 # always | ratio | never
    ratio: 0.1
```

### Trace 传播

- **REST**：注入 `X-Trace-Id` 到 Header
- **WebSocket**：握手阶段注入 `X-Trace-Id`，建连后不追踪单条消息
- **SSE**：初始请求注入 `X-Trace-Id`

### Metrics

通过 OpenTelemetry Metrics 暴露 Prometheus 格式指标：

```yaml
metrics:
  enabled: true
  path: "/metrics"
```

核心指标：
- `gateway_request_total` — 请求总数（按路由、状态码、方法）
- `gateway_request_duration` — 请求延迟分布
- `gateway_active_connections` — 当前活跃连接数（含 WS/SSE）
- `gateway_circuit_breaker_state` — 熔断器状态

### 日志

结构化 JSON 日志，包含 trace_id：

```yaml
log:
  level: "info"                   # debug | info | warn | error
  output: "stdout"                # stdout | file
  file:
    path: "/var/log/gateway/gateway.log"
    max_size: 100                 # MB
    max_backups: 7
    max_age: 30                   # 天
```

日志示例：

```json
{"level":"info","trace_id":"abc123","method":"GET","path":"/api/v1/users","status":200,"latency_ms":12,"upstream":"user-service:8080"}
```

## 配置管理

### 配置分层

| 配置类型 | 来源 | 示例 |
|---------|------|------|
| 基础配置 | YAML 文件 | 监听端口、日志级别、OTel exporter 地址 |
| 动态配置 | 配置中心 | 路由规则、限流阈值、灰度策略、认证密钥 |

启动流程：加载文件配置 → 连接配置中心获取动态配置 → Watch 变更热更新

配置中心不可用时，动态配置回退到文件中的默认值。

### 热更新

- **支持热更新**：路由规则、限流配置、灰度策略、IP 黑白名单、认证方案
- **需要重启**：监听端口、OTel exporter 地址、日志输出方式

## 健康检查

网关暴露健康端点：

```yaml
health:
  path: "/health"
  check:
    - name: "config_center"
    - name: "discovery"
```

返回：

```json
{"status": "healthy", "checks": {"config_center": "up", "discovery": "up"}}
```

后端服务健康检查依赖服务发现机制（注册中心自带健康检查，静态路由可配置主动探活）。

`/health` 和 `/metrics` 端点绕过中间件链直接处理，不受认证、限流等影响。

## Server 配置

```yaml
server:
  listen: ":8080"                   # 监听地址
  max_request_body_size: 10485760   # 10MB，0 表示不限制
  shutdown_timeout: 30s             # 优雅关闭超时时间
```

`max_request_body_size` 路由级别可覆盖全局设置，WebSocket 和 SSE 不受此限制。

## 优雅关闭

关闭顺序：
1. 停止接受新连接
2. 等待处理中的请求完成（含 WS/SSE 长连接优雅断开）
3. 停止配置中心 Watcher
4. 停止服务发现 Watcher
5. 关闭 OpenTelemetry exporter，flush 剩余 span

## 部署

- 网关本身无状态，通过外部负载均衡（Nginx/云 LB）部署多实例水平扩展
- 限流等状态依赖外部存储（如 Redis），保证多实例一致性
