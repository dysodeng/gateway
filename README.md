# Gateway

一个基于 Go 实现的轻量级高性能 API 网关，支持动态路由、多协议代理、服务发现、认证鉴权、可观测性等企业级能力。

## 特性

- **多协议代理** — HTTP、WebSocket、SSE 透明转发
- **服务发现** — 静态配置 / etcd 注册中心，Watch 实时感知实例上下线
- **负载均衡** — Round Robin、加权轮询、随机、IP Hash、最少连接
- **认证鉴权** — JWT、API Key、OAuth2，按路由独立配置
- **流量治理** — 滑动窗口限流、IP 黑白名单、请求签名验证
- **灰度发布** — 基于权重和请求头匹配的 Canary 路由
- **可观测性** — OpenTelemetry 链路追踪 + 指标采集（OTLP 协议），结构化访问日志
- **熔断重试** — 内置熔断器与可配置重试策略
- **配置中心** — 支持 etcd 远程配置，`.env` 环境变量覆盖

## 架构

```
Client → Gateway → [中间件链] → 负载均衡 → 后端服务
                     ├── Recovery
                     ├── AccessLog
                     ├── Tracing
                     ├── CORS
                     ├── IP Filter
                     ├── Rate Limit
                     ├── Auth
                     ├── Request Sign
                     └── Rewrite
```

## 快速开始

### 环境要求

- Go 1.24+

### 编译运行

```bash
# 编译
go build -o gateway ./cmd/gateway/

# 运行（默认读取 gateway.yaml）
./gateway

# 指定配置文件
./gateway -config /path/to/gateway.yaml
```

### 最小配置

```yaml
server:
  listen: ":8080"

discovery:
  type: "static"
  static:
    services:
      my-api:
        - host: "127.0.0.1"
          port: 8081

routes:
  - name: "my-api"
    prefix: "/api"
    service: "my-api"
```

## 项目结构

```
├── cmd/gateway/       # 入口
├── config/            # 配置加载与结构定义
├── server/            # HTTP 服务器
├── router/            # 路由匹配
│   └── loadbalancer/  # 负载均衡策略
├── proxy/             # 代理实现（HTTP/WebSocket/SSE/熔断/重试）
├── middleware/         # 中间件
├── discovery/         # 服务发现（static/etcd）
└── pkg/               # 公共包
    ├── logger/        # 结构化日志（zap）
    ├── trace/         # OpenTelemetry 链路追踪
    ├── metrics/       # OpenTelemetry 指标采集
    └── health/        # 健康检查
```

## 文档

详细配置说明和使用手册请参阅 [docs/usage.md](docs/usage.md)。

## License

MIT
