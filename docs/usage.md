# Gateway 使用说明

一个基于 Go 实现的高性能 API 网关，支持动态路由、多协议代理、服务发现、可观测性等能力。

## 快速开始

### 编译

```bash
go build -o gateway ./cmd/gateway/
```

### 运行

```bash
./gateway -config gateway.yaml
```

## 配置说明

网关通过 `gateway.yaml` 进行配置，支持两种加载方式：

1. 本地文件加载（默认）
2. etcd 配置中心加载（通过 `.env` 中的 `CONFIG_CENTER_*` 环境变量启用）

---

### server — 服务器

```yaml
server:
  listen: ":8080"                  # 监听地址
  max_request_body_size: 10485760  # 请求体最大字节数，默认 10MB
  shutdown_timeout: 30s            # 优雅关闭超时
```

### log — 日志

```yaml
log:
  debug: true       # 开启 debug 模式
  level: "info"     # 日志级别: debug / info / warn / error
```

### telemetry — 链路追踪（OpenTelemetry）

```yaml
telemetry:
  enabled: true
  service_name: "gateway"
  exporter:
    protocol: "grpc"             # grpc 或 http
    endpoint: "localhost:4317"   # OTel Collector 地址
  sampler:
    type: "ratio"                # always / never / ratio
    ratio: 0.1                   # ratio 模式下的采样比例
```

支持从上游请求头接续 trace context：
- 标准 W3C `traceparent` 头（优先）
- 自定义 `X-Trace-Id` 头（fallback）

### metrics — 指标采集（OpenTelemetry）

```yaml
metrics:
  enabled: true
  exporter:
    protocol: "grpc"
    endpoint: "localhost:4317"
```

通过 OTLP 协议推送到 OTel Collector，由 Collector 对接 Prometheus/Grafana 等后端。

### health — 健康检查

```yaml
health:
  path: "/health"
  check:
    - name: "discovery"    # 检查服务发现可用性
```

---

## 中间件

中间件分为全局中间件和路由级中间件。全局中间件（CORS、IP 过滤等）在顶层配置，路由级中间件在每条路由的 `middleware` 字段中配置。

### cors — 跨域

```yaml
cors:
  allowed_origins: ["*"]
  allowed_methods: ["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"]
  allowed_headers: ["Authorization", "Content-Type"]
  max_age: 3600
```

### ip_filter — IP 过滤

```yaml
ip_filter:
  mode: "blacklist"    # blacklist 或 whitelist
  list:
    - "192.168.1.100"
```

路由级也可单独配置 `ip_filter`，覆盖全局设置。

### rate_limit — 限流

```yaml
rate_limit:
  storage: "local"              # local（本地内存）或 redis
  algorithm: "sliding_window"   # 限流算法: sliding_window
  # redis:                      # storage 为 redis 时需要配置
  #   addr: "localhost:6379"
  #   password: ""
  #   db: 0
```

路由级启用：

```yaml
routes:
  - name: "api"
    middleware:
      rate_limit:
        enabled: true
        qps: 100
```

### request_sign — 请求签名验证

```yaml
request_sign:
  enabled: false
  algorithm: "hmac-sha256"
  sign_header: "X-Signature"
  timestamp_header: "X-Timestamp"
  expire: 300              # 签名有效期（秒）
  secret: "your-secret"
```

### auth_schemes — 认证方案

支持三种认证类型：JWT、API Key、OAuth2。

```yaml
auth_schemes:
  # JWT 认证
  app_user:
    type: "jwt"
    jwt:
      secret: "your-secret-key"
      algorithms: ["HS256"]
      header: "Authorization"
      claims_to_headers:         # 将 JWT claims 映射到请求头传给后端
        user_id: "X-User-Id"

  # API Key 认证
  internal:
    type: "api_key"
    api_key:
      header: "X-API-Key"       # 从 header 读取
      # query: "api_key"        # 或从 query 参数读取

  # OAuth2 认证
  third_party:
    type: "oauth2"
    oauth2:
      introspect_endpoint: "https://auth.example.com/introspect"
      client_id: "gateway"
      client_secret: "secret"
      claims_to_headers:
        sub: "X-User-Id"
```

---

## 服务发现

### static — 静态配置

```yaml
discovery:
  type: "static"
  static:
    services:
      user-service:
        - host: "127.0.0.1"
          port: 8081
          weight: 1
        - host: "127.0.0.1"
          port: 8082
          weight: 2
```

### etcd — etcd 注册中心

```yaml
discovery:
  type: "etcd"
  etcd:
    endpoints:
      - "localhost:2379"
    prefix: "/services"      # key 前缀，默认 /services
    timeout: 5s
    username: ""
    password: ""
```

etcd 中的服务实例以 JSON 格式注册在 `{prefix}/{serviceName}/{instanceId}` 路径下：

```json
{
  "host": "10.0.0.1",
  "port": 8081,
  "weight": 1,
  "metadata": { "version": "v2" }
}
```

网关启动时全量加载，之后通过 Watch 实时感知实例上下线。

---

## 路由配置

```yaml
routes:
  - name: "user-api"
    prefix: "/api/v1/users"        # URL 前缀匹配
    strip_prefix: false             # 是否去掉前缀再转发
    service: "user-service"         # 后端服务名（对应 discovery 中的服务）
    type: "http"                    # 协议类型: http（默认）/ websocket / sse
    timeout: 5s                     # 请求超时
    load_balancer: "round_robin"    # 负载均衡策略
    max_request_body_size: 5242880  # 路由级请求体大小限制（可选，覆盖全局）
    retry:                          # 重试配置
      count: 2
      conditions: ["5xx"]
    middleware:                      # 路由级中间件
      auth:
        scheme: "app_user"
      rate_limit:
        enabled: true
        qps: 100
      request_sign:
        enabled: true
      ip_filter:
        mode: "whitelist"
        list: ["10.0.0.0/8"]
      rewrite:
        add_headers:
          X-Gateway: "true"
        remove_headers:
          - "X-Internal"
```

### 负载均衡策略

| 策略 | 值 | 说明 |
|------|-----|------|
| 轮询 | `round_robin` | 依次分配请求 |
| 加权轮询 | `weighted` | 按 weight 比例分配 |
| 随机 | `random` | 随机选择实例 |
| IP 哈希 | `ip_hash` | 同一 IP 固定到同一实例 |
| 最少连接 | `least_conn` | 选择当前连接数最少的实例 |

### WebSocket 路由

```yaml
routes:
  - name: "ws"
    prefix: "/ws"
    service: "ws-service"
    type: "websocket"
    websocket:
      heartbeat: 30s
      max_connections: 1000
```

### SSE 路由

```yaml
routes:
  - name: "events"
    prefix: "/events"
    service: "event-service"
    type: "sse"
    sse:
      retry: 3000           # 客户端重连间隔（毫秒）
      keepalive: 30s         # 心跳间隔
```

### 灰度发布

```yaml
routes:
  - name: "user-api"
    prefix: "/api/v1/users"
    service: "user-service"
    canary:
      - weight: 90
        service: "user-service"
      - weight: 10
        service: "user-service-v2"
        match:
          headers:
            X-Canary: "true"   # 匹配特定请求头
```

---

## 配置中心（可选）

通过 `.env` 文件启用 etcd 配置中心，网关启动时先从 etcd 加载完整配置：

```env
CONFIG_CENTER_TYPE=etcd
CONFIG_CENTER_ETCD_ENDPOINTS=localhost:2379
CONFIG_CENTER_ETCD_KEY=/gateway/config
CONFIG_CENTER_ETCD_TIMEOUT=5s
```

`.env` 中的环境变量可覆盖配置中心的值，优先级：`.env` > 配置中心 > 本地文件。
