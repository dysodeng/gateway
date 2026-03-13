# API Gateway Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go microservice API gateway supporting RESTful, WebSocket, and SSE protocols with service discovery, traffic control, and full observability.

**Architecture:** Standard layered architecture with two-phase middleware (Pre-Route and Post-Route), prefix-based routing with canary support, and transparent proxy for HTTP/WS/SSE. Stateless deployment behind external LB.

**Tech Stack:**
- Go 1.25, net/http (stdlib)
- `log/slog` for structured logging
- `gopkg.in/yaml.v3` for config parsing
- `github.com/gorilla/websocket` for WebSocket proxy
- `github.com/golang-jwt/jwt/v5` for JWT auth
- `github.com/redis/go-redis/v9` for distributed rate limiting
- `go.opentelemetry.io/otel` + `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` + `otlptracehttp` for tracing
- `go.opentelemetry.io/otel/exporters/prometheus` for metrics
- `github.com/prometheus/client_golang` for metrics endpoint

**Spec:** `docs/superpowers/specs/2026-03-12-api-gateway-design.md`

---

## File Structure

```
gateway/
├── cmd/gateway/main.go                    # Entry point, wiring, graceful shutdown
├── config/
│   ├── config.go                          # Config struct definitions (all YAML sections)
│   ├── config_test.go                     # Config loading tests
│   ├── loader.go                          # YAML file loading + validation
│   └── watcher.go                         # Config Watcher interface
├── discovery/
│   ├── discovery.go                       # Discovery interface + ServiceInstance type
│   ├── static.go                          # Static file-based discovery
│   ├── static_test.go                     # Static discovery tests
│   ├── etcd.go                            # Etcd discovery (stub, interface only)
│   ├── consul.go                          # Consul discovery (stub, interface only)
│   └── nacos.go                           # Nacos discovery (stub, interface only)
├── router/
│   ├── router.go                          # Prefix trie routing + route context
│   ├── router_test.go                     # Router matching tests
│   ├── canary.go                          # Canary decision tree
│   ├── canary_test.go                     # Canary tests
│   └── loadbalancer/
│       ├── balancer.go                    # Balancer interface
│       ├── roundrobin.go                  # Round robin implementation
│       ├── roundrobin_test.go
│       ├── weighted.go                    # Weighted round robin
│       ├── weighted_test.go
│       ├── random.go                      # Random selection
│       ├── random_test.go
│       ├── iphash.go                      # IP hash
│       ├── iphash_test.go
│       ├── leastconn.go                   # Least connections
│       └── leastconn_test.go
├── proxy/
│   ├── proxy.go                           # Dispatcher: detect REST/WS/SSE, delegate
│   ├── proxy_test.go                      # Dispatcher tests
│   ├── http.go                            # HTTP reverse proxy
│   ├── http_test.go
│   ├── websocket.go                       # WebSocket transparent proxy
│   ├── websocket_test.go
│   ├── sse.go                             # SSE transparent proxy
│   ├── sse_test.go
│   ├── circuitbreaker.go                  # Per-service circuit breaker
│   ├── circuitbreaker_test.go
│   ├── retry.go                           # Retry with conditions
│   └── retry_test.go
├── middleware/
│   ├── chain.go                           # Two-phase middleware chain builder
│   ├── chain_test.go
│   ├── recovery.go                        # [Pre-Route] Panic recovery
│   ├── recovery_test.go
│   ├── accesslog.go                       # [Pre-Route] Access logging
│   ├── accesslog_test.go
│   ├── cors.go                            # [Pre-Route] CORS handling
│   ├── cors_test.go
│   ├── tracing.go                         # [Pre-Route] OpenTelemetry span
│   ├── tracing_test.go
│   ├── ipfilter.go                        # Global + Route-level IP filter
│   ├── ipfilter_test.go
│   ├── auth.go                            # [Post-Route] JWT/APIKey/OAuth2
│   ├── auth_test.go
│   ├── ratelimit.go                       # [Post-Route] Rate limiting
│   ├── ratelimit_test.go
│   ├── requestsign.go                     # [Post-Route] Request signature
│   ├── requestsign_test.go
│   ├── rewrite.go                         # [Post-Route] Path/header rewrite
│   └── rewrite_test.go
├── server/
│   ├── server.go                          # HTTP server, graceful shutdown, wiring
│   └── server_test.go
├── pkg/
│   ├── log/
│   │   ├── log.go                         # Structured logging init (slog JSON handler)
│   │   └── log_test.go
│   ├── trace/
│   │   ├── trace.go                       # OTel provider init (gRPC + HTTP exporter)
│   │   └── trace_test.go
│   └── health/
│       ├── health.go                      # Health check endpoint
│       └── health_test.go
└── gateway.yaml                           # Example config file
```

---

## Chunk 1: Foundation — Config, Discovery, Server Skeleton

### Task 1: Initialize Go module and dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add all required dependencies**

```bash
cd /Users/dysodeng/project/go/gateway
go get gopkg.in/yaml.v3
go get github.com/gorilla/websocket
go get github.com/golang-jwt/jwt/v5
go get github.com/redis/go-redis/v9
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go get go.opentelemetry.io/otel/exporters/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
go get go.opentelemetry.io/otel/sdk/metric
```

- [ ] **Step 2: Verify go.mod is correct**

Run: `go mod tidy && cat go.mod`
Expected: All dependencies listed, no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: add project dependencies"
```

---

### Task 2: Config struct definitions

**Files:**
- Create: `config/config.go`
- Create: `gateway.yaml`

- [ ] **Step 1: Write config struct definitions**

Create `config/config.go` containing the complete config model matching the spec YAML:

```go
package config

import "time"

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Log       LogConfig       `yaml:"log"`
	Telemetry TelemetryConfig `yaml:"telemetry"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Health    HealthConfig    `yaml:"health"`
	CORS      CORSConfig      `yaml:"cors"`
	IPFilter  IPFilterConfig  `yaml:"ip_filter"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	RequestSign RequestSignConfig `yaml:"request_sign"`
	AuthSchemes map[string]AuthSchemeConfig `yaml:"auth_schemes"`
	Routes    []RouteConfig   `yaml:"routes"`
	Discovery DiscoveryConfig `yaml:"discovery"`
}

type ServerConfig struct {
	Listen             string        `yaml:"listen"`
	MaxRequestBodySize int64         `yaml:"max_request_body_size"`
	ShutdownTimeout    time.Duration `yaml:"shutdown_timeout"`
}

type LogConfig struct {
	Level  string         `yaml:"level"`
	Output string         `yaml:"output"`
	File   LogFileConfig  `yaml:"file"`
}

type LogFileConfig struct {
	Path       string `yaml:"path"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

type TelemetryConfig struct {
	Enabled     bool            `yaml:"enabled"`
	ServiceName string          `yaml:"service_name"`
	Exporter    ExporterConfig  `yaml:"exporter"`
	Sampler     SamplerConfig   `yaml:"sampler"`
}

type ExporterConfig struct {
	Type     string `yaml:"type"`
	Protocol string `yaml:"protocol"`
	Endpoint string `yaml:"endpoint"`
}

type SamplerConfig struct {
	Type  string  `yaml:"type"`
	Ratio float64 `yaml:"ratio"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

type HealthConfig struct {
	Path   string              `yaml:"path"`
	Checks []HealthCheckConfig `yaml:"check"`
}

type HealthCheckConfig struct {
	Name string `yaml:"name"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
	MaxAge         int      `yaml:"max_age"`
}

type IPFilterConfig struct {
	Mode string   `yaml:"mode"`
	List []string `yaml:"list"`
}

type RateLimitConfig struct {
	Storage   string      `yaml:"storage"`
	Redis     RedisConfig `yaml:"redis"`
	Algorithm string      `yaml:"algorithm"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type RequestSignConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Algorithm       string `yaml:"algorithm"`
	SignHeader      string `yaml:"sign_header"`
	TimestampHeader string `yaml:"timestamp_header"`
	Expire          int    `yaml:"expire"`
}

type AuthSchemeConfig struct {
	Type   string       `yaml:"type"`
	JWT    *JWTConfig   `yaml:"jwt,omitempty"`
	APIKey *APIKeyConfig `yaml:"api_key,omitempty"`
	OAuth2 *OAuth2Config `yaml:"oauth2,omitempty"`
}

type JWTConfig struct {
	Secret          string            `yaml:"secret"`
	Algorithms      []string          `yaml:"algorithms"`
	Header          string            `yaml:"header"`
	ClaimsToHeaders map[string]string `yaml:"claims_to_headers"`
}

type APIKeyConfig struct {
	Header string `yaml:"header"`
	Query  string `yaml:"query"`
}

type OAuth2Config struct {
	IntrospectEndpoint string            `yaml:"introspect_endpoint"`
	ClientID           string            `yaml:"client_id"`
	ClientSecret       string            `yaml:"client_secret"`
	ClaimsToHeaders    map[string]string `yaml:"claims_to_headers"`
}

type RouteConfig struct {
	Name         string              `yaml:"name"`
	Prefix       string              `yaml:"prefix"`
	StripPrefix  bool                `yaml:"strip_prefix"`
	Service      string              `yaml:"service"`
	Type         string              `yaml:"type"`
	Timeout      time.Duration       `yaml:"timeout"`
	Retry        RetryConfig         `yaml:"retry"`
	LoadBalancer string              `yaml:"load_balancer"`
	Middleware   RouteMiddlewareConfig `yaml:"middleware"`
	Canary       []CanaryRuleConfig  `yaml:"canary"`
	WebSocket    *WebSocketConfig    `yaml:"websocket,omitempty"`
	SSE          *SSEConfig          `yaml:"sse,omitempty"`
	MaxRequestBodySize *int64        `yaml:"max_request_body_size,omitempty"`
}

type RetryConfig struct {
	Count      int      `yaml:"count"`
	Conditions []string `yaml:"conditions"`
}

type RouteMiddlewareConfig struct {
	Auth        *RouteAuthConfig        `yaml:"auth,omitempty"`
	RateLimit   *RouteRateLimitConfig   `yaml:"rate_limit,omitempty"`
	RequestSign *RouteRequestSignConfig `yaml:"request_sign,omitempty"`
	IPFilter    *IPFilterConfig         `yaml:"ip_filter,omitempty"`
}

type RouteAuthConfig struct {
	Scheme string `yaml:"scheme"`
}

type RouteRateLimitConfig struct {
	Enabled bool `yaml:"enabled"`
	QPS     int  `yaml:"qps"`
}

type RouteRequestSignConfig struct {
	Enabled bool `yaml:"enabled"`
}

type CanaryRuleConfig struct {
	Weight  int              `yaml:"weight"`
	Service string           `yaml:"service"`
	Match   *CanaryMatch     `yaml:"match,omitempty"`
}

type CanaryMatch struct {
	Headers map[string]string `yaml:"headers"`
}

type WebSocketConfig struct {
	Heartbeat      time.Duration `yaml:"heartbeat"`
	MaxConnections int           `yaml:"max_connections"`
}

type SSEConfig struct {
	Retry     int           `yaml:"retry"`
	Keepalive time.Duration `yaml:"keepalive"`
}

type DiscoveryConfig struct {
	Type   string             `yaml:"type"`
	Static *StaticDiscoveryConfig `yaml:"static,omitempty"`
	Etcd   *EtcdConfig        `yaml:"etcd,omitempty"`
}

type StaticDiscoveryConfig struct {
	Services map[string][]StaticInstanceConfig `yaml:"services"`
}

type StaticInstanceConfig struct {
	Host   string            `yaml:"host"`
	Port   int               `yaml:"port"`
	Weight int               `yaml:"weight"`
	Metadata map[string]string `yaml:"metadata,omitempty"`
}

type EtcdConfig struct {
	Endpoints []string      `yaml:"endpoints"`
	Timeout   time.Duration `yaml:"timeout"`
}
```

- [ ] **Step 2: Create example gateway.yaml config file**

Create `gateway.yaml` with a complete example matching the spec:

```yaml
server:
  listen: ":8080"
  max_request_body_size: 10485760
  shutdown_timeout: 30s

log:
  level: "info"
  output: "stdout"

telemetry:
  enabled: true
  service_name: "gateway"
  exporter:
    type: "otlp"
    protocol: "grpc"
    endpoint: "localhost:4317"
  sampler:
    type: "ratio"
    ratio: 0.1

metrics:
  enabled: true
  path: "/metrics"

health:
  path: "/health"
  check:
    - name: "discovery"

cors:
  allowed_origins: ["*"]
  allowed_methods: ["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"]
  allowed_headers: ["Authorization", "Content-Type"]
  max_age: 3600

ip_filter:
  mode: "blacklist"
  list: []

rate_limit:
  storage: "local"
  algorithm: "sliding_window"

request_sign:
  enabled: false
  algorithm: "hmac-sha256"
  sign_header: "X-Signature"
  timestamp_header: "X-Timestamp"
  expire: 300

auth_schemes:
  app_user:
    type: "jwt"
    jwt:
      secret: "your-secret-key"
      algorithms: ["HS256"]
      header: "Authorization"
      claims_to_headers:
        user_id: "X-User-Id"

discovery:
  type: "static"
  static:
    services:
      user-service:
        - host: "127.0.0.1"
          port: 8081
          weight: 1

routes:
  - name: "user-service"
    prefix: "/api/v1/users"
    service: "user-service"
    timeout: 5s
    load_balancer: "round_robin"
    middleware:
      auth:
        scheme: "app_user"
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/dysodeng/project/go/gateway && go build ./config/...`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add config/config.go gateway.yaml
git commit -m "feat: add config struct definitions and example config"
```

---

### Task 3: Config file loading

**Files:**
- Create: `config/loader.go`
- Create: `config/config_test.go`

- [ ] **Step 1: Write config loading test**

Create `config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yaml := `
server:
  listen: ":9090"
  max_request_body_size: 5242880
  shutdown_timeout: 10s
log:
  level: "debug"
  output: "stdout"
discovery:
  type: "static"
  static:
    services:
      test-svc:
        - host: "127.0.0.1"
          port: 8081
          weight: 1
routes:
  - name: "test"
    prefix: "/api/test"
    service: "test-svc"
    timeout: 3s
    load_balancer: "round_robin"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Listen != ":9090" {
		t.Errorf("Server.Listen = %q, want %q", cfg.Server.Listen, ":9090")
	}
	if cfg.Server.MaxRequestBodySize != 5242880 {
		t.Errorf("MaxRequestBodySize = %d, want %d", cfg.Server.MaxRequestBodySize, 5242880)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("len(Routes) = %d, want 1", len(cfg.Routes))
	}
	if cfg.Routes[0].Prefix != "/api/test" {
		t.Errorf("Routes[0].Prefix = %q, want %q", cfg.Routes[0].Prefix, "/api/test")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	yaml := `
server:
  listen: ":8080"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.ShutdownTimeout == 0 {
		t.Error("expected default ShutdownTimeout, got 0")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level 'info', got %q", cfg.Log.Level)
	}
	if cfg.Health.Path != "/health" {
		t.Errorf("expected default health path '/health', got %q", cfg.Health.Path)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/dysodeng/project/go/gateway && go test ./config/ -v -run TestLoad`
Expected: FAIL — `Load` not defined.

- [ ] **Step 3: Implement config loader**

Create `config/loader.go`:

```go
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(cfg)
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":8080"
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = 30 * time.Second
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Output == "" {
		cfg.Log.Output = "stdout"
	}
	if cfg.Health.Path == "" {
		cfg.Health.Path = "/health"
	}
	if cfg.Metrics.Path == "" {
		cfg.Metrics.Path = "/metrics"
	}
	if cfg.RateLimit.Storage == "" {
		cfg.RateLimit.Storage = "local"
	}
	if cfg.RateLimit.Algorithm == "" {
		cfg.RateLimit.Algorithm = "sliding_window"
	}
	for i := range cfg.Routes {
		if cfg.Routes[i].Timeout == 0 {
			cfg.Routes[i].Timeout = 30 * time.Second
		}
		if cfg.Routes[i].LoadBalancer == "" {
			cfg.Routes[i].LoadBalancer = "round_robin"
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/dysodeng/project/go/gateway && go test ./config/ -v -run TestLoad`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add config/loader.go config/config_test.go
git commit -m "feat: add config file loading with defaults"
```

---

### Task 4: Watcher interface

**Files:**
- Create: `config/watcher.go`

- [ ] **Step 1: Write Watcher interface**

Create `config/watcher.go`:

```go
package config

// Watcher defines the interface for dynamic configuration sources.
// Implementations include etcd, nacos, etc. The gateway uses this to
// hot-reload routes, rate limits, canary rules, and auth schemes.
type Watcher interface {
	Get(key string) ([]byte, error)
	Watch(key string, callback func(value []byte)) error
	Stop() error
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./config/...`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add config/watcher.go
git commit -m "feat: add Watcher interface"
```

---

### Task 5: Discovery interface and static implementation

**Files:**
- Create: `discovery/discovery.go`
- Create: `discovery/static.go`
- Create: `discovery/static_test.go`

- [ ] **Step 1: Write Discovery interface and ServiceInstance type**

Create `discovery/discovery.go`:

```go
package discovery

import "fmt"

type ServiceInstance struct {
	ID       string
	Name     string
	Host     string
	Port     int
	Weight   int
	Metadata map[string]string
}

func (si ServiceInstance) Addr() string {
	return fmt.Sprintf("%s:%d", si.Host, si.Port)
}

type Discovery interface {
	GetInstances(serviceName string) ([]ServiceInstance, error)
	Watch(serviceName string, callback func([]ServiceInstance)) error
	Stop() error
}
```

- [ ] **Step 2: Write static discovery test**

Create `discovery/static_test.go`:

```go
package discovery

import (
	"testing"

	"github.com/dysodeng/gateway/config"
)

func TestStaticDiscovery_GetInstances(t *testing.T) {
	cfg := &config.StaticDiscoveryConfig{
		Services: map[string][]config.StaticInstanceConfig{
			"user-svc": {
				{Host: "127.0.0.1", Port: 8081, Weight: 1},
				{Host: "127.0.0.1", Port: 8082, Weight: 2},
			},
		},
	}

	d := NewStaticDiscovery(cfg)

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
	if instances[1].Weight != 2 {
		t.Errorf("instances[1].Weight = %d, want 2", instances[1].Weight)
	}
}

func TestStaticDiscovery_GetInstances_NotFound(t *testing.T) {
	cfg := &config.StaticDiscoveryConfig{
		Services: map[string][]config.StaticInstanceConfig{},
	}

	d := NewStaticDiscovery(cfg)

	_, err := d.GetInstances("unknown-svc")
	if err == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./discovery/ -v -run TestStaticDiscovery`
Expected: FAIL — `NewStaticDiscovery` not defined.

- [ ] **Step 4: Implement static discovery**

Create `discovery/static.go`:

```go
package discovery

import (
	"fmt"

	"github.com/dysodeng/gateway/config"
)

type StaticDiscovery struct {
	instances map[string][]ServiceInstance
}

func NewStaticDiscovery(cfg *config.StaticDiscoveryConfig) *StaticDiscovery {
	instances := make(map[string][]ServiceInstance)
	for name, cfgInstances := range cfg.Services {
		for i, inst := range cfgInstances {
			instances[name] = append(instances[name], ServiceInstance{
				ID:       fmt.Sprintf("%s-%d", name, i),
				Name:     name,
				Host:     inst.Host,
				Port:     inst.Port,
				Weight:   inst.Weight,
				Metadata: inst.Metadata,
			})
		}
	}
	return &StaticDiscovery{instances: instances}
}

func (d *StaticDiscovery) GetInstances(serviceName string) ([]ServiceInstance, error) {
	instances, ok := d.instances[serviceName]
	if !ok || len(instances) == 0 {
		return nil, fmt.Errorf("service %q not found", serviceName)
	}
	return instances, nil
}

func (d *StaticDiscovery) Watch(serviceName string, callback func([]ServiceInstance)) error {
	return nil
}

func (d *StaticDiscovery) Stop() error {
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./discovery/ -v -run TestStaticDiscovery`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add discovery/
git commit -m "feat: add discovery interface and static implementation"
```

---

### Task 6: Discovery registry stubs

**Files:**
- Create: `discovery/etcd.go`
- Create: `discovery/consul.go`
- Create: `discovery/nacos.go`

- [ ] **Step 1: Create stub files for registry implementations**

Create `discovery/etcd.go`:

```go
package discovery

// TODO: Implement etcd-based service discovery.
// Will implement Discovery interface using etcd Watch API.
```

Create `discovery/consul.go`:

```go
package discovery

// TODO: Implement consul-based service discovery.
// Will implement Discovery interface using Consul Health API.
```

Create `discovery/nacos.go`:

```go
package discovery

// TODO: Implement nacos-based service discovery.
// Will implement Discovery interface using Nacos naming API.
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./discovery/...`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add discovery/etcd.go discovery/consul.go discovery/nacos.go
git commit -m "feat: add discovery registry stubs (etcd, consul, nacos)"
```

---

## Chunk 2: Router, Load Balancer, Canary

### Task 7: Load balancer interface and round robin

**Files:**
- Create: `router/loadbalancer/balancer.go`
- Create: `router/loadbalancer/roundrobin.go`
- Create: `router/loadbalancer/roundrobin_test.go`

- [ ] **Step 1: Write balancer interface**

Create `router/loadbalancer/balancer.go`:

```go
package loadbalancer

import (
	"net/http"

	"github.com/dysodeng/gateway/discovery"
)

type Balancer interface {
	Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error)
}
```

- [ ] **Step 2: Write round robin test**

Create `router/loadbalancer/roundrobin_test.go`:

```go
package loadbalancer

import (
	"net/http"
	"testing"

	"github.com/dysodeng/gateway/discovery"
)

func TestRoundRobin_Select(t *testing.T) {
	instances := []discovery.ServiceInstance{
		{ID: "a", Host: "127.0.0.1", Port: 8081},
		{ID: "b", Host: "127.0.0.1", Port: 8082},
		{ID: "c", Host: "127.0.0.1", Port: 8083},
	}

	rr := NewRoundRobin()
	req, _ := http.NewRequest("GET", "/", nil)

	selected := make([]string, 6)
	for i := 0; i < 6; i++ {
		inst, err := rr.Select(instances, req)
		if err != nil {
			t.Fatalf("Select() error: %v", err)
		}
		selected[i] = inst.ID
	}

	// Should cycle: a, b, c, a, b, c
	expected := []string{"a", "b", "c", "a", "b", "c"}
	for i, id := range selected {
		if id != expected[i] {
			t.Errorf("iteration %d: got %q, want %q", i, id, expected[i])
		}
	}
}

func TestRoundRobin_Select_Empty(t *testing.T) {
	rr := NewRoundRobin()
	req, _ := http.NewRequest("GET", "/", nil)

	_, err := rr.Select(nil, req)
	if err == nil {
		t.Fatal("expected error for empty instances")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./router/loadbalancer/ -v -run TestRoundRobin`
Expected: FAIL

- [ ] **Step 4: Implement round robin**

Create `router/loadbalancer/roundrobin.go`:

```go
package loadbalancer

import (
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/dysodeng/gateway/discovery"
)

type RoundRobin struct {
	counter atomic.Uint64
}

func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

func (rr *RoundRobin) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("no available instances")
	}
	idx := rr.counter.Add(1) - 1
	inst := instances[idx%uint64(len(instances))]
	return &inst, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./router/loadbalancer/ -v -run TestRoundRobin`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add router/loadbalancer/
git commit -m "feat: add load balancer interface and round robin implementation"
```

---

### Task 8: Remaining load balancers (weighted, random, iphash, leastconn)

**Files:**
- Create: `router/loadbalancer/weighted.go`, `router/loadbalancer/weighted_test.go`
- Create: `router/loadbalancer/random.go`, `router/loadbalancer/random_test.go`
- Create: `router/loadbalancer/iphash.go`, `router/loadbalancer/iphash_test.go`
- Create: `router/loadbalancer/leastconn.go`, `router/loadbalancer/leastconn_test.go`

- [ ] **Step 1: Write tests for all four balancers**

`weighted_test.go` — test that higher-weight instances are selected proportionally more over many iterations.

`random_test.go` — test that selection returns valid instances, all instances selected over many iterations.

`iphash_test.go` — test that same client IP always selects the same instance, different IPs may select different instances.

`leastconn_test.go` — test that the instance with fewest active connections is selected. Test `IncrConn`/`DecrConn` tracking.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./router/loadbalancer/ -v`
Expected: FAIL

- [ ] **Step 3: Implement weighted round robin**

```go
// router/loadbalancer/weighted.go
package loadbalancer

import (
	"errors"
	"net/http"
	"sync"

	"github.com/dysodeng/gateway/discovery"
)

type Weighted struct {
	mu      sync.Mutex
	current map[string]int // effective weight tracker per instance ID
}

func NewWeighted() *Weighted {
	return &Weighted{current: make(map[string]int)}
}

func (w *Weighted) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("no available instances")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	totalWeight := 0
	var best *discovery.ServiceInstance
	bestWeight := 0

	for i := range instances {
		inst := &instances[i]
		weight := inst.Weight
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight
		w.current[inst.ID] += weight
		if best == nil || w.current[inst.ID] > bestWeight {
			best = inst
			bestWeight = w.current[inst.ID]
		}
	}

	w.current[best.ID] -= totalWeight
	return best, nil
}
```

- [ ] **Step 4: Implement random**

```go
// router/loadbalancer/random.go
package loadbalancer

import (
	"errors"
	"math/rand/v2"
	"net/http"

	"github.com/dysodeng/gateway/discovery"
)

type Random struct{}

func NewRandom() *Random { return &Random{} }

func (r *Random) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("no available instances")
	}
	inst := instances[rand.IntN(len(instances))]
	return &inst, nil
}
```

- [ ] **Step 5: Implement IP hash**

```go
// router/loadbalancer/iphash.go
package loadbalancer

import (
	"errors"
	"hash/fnv"
	"net"
	"net/http"

	"github.com/dysodeng/gateway/discovery"
)

type IPHash struct{}

func NewIPHash() *IPHash { return &IPHash{} }

func (h *IPHash) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("no available instances")
	}
	ip, _, _ := net.SplitHostPort(req.RemoteAddr)
	if ip == "" {
		ip = req.RemoteAddr
	}
	hasher := fnv.New32a()
	hasher.Write([]byte(ip))
	idx := hasher.Sum32() % uint32(len(instances))
	inst := instances[idx]
	return &inst, nil
}
```

- [ ] **Step 6: Implement least connections**

```go
// router/loadbalancer/leastconn.go
package loadbalancer

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/dysodeng/gateway/discovery"
)

type LeastConn struct {
	mu    sync.RWMutex
	conns map[string]*atomic.Int64
}

func NewLeastConn() *LeastConn {
	return &LeastConn{conns: make(map[string]*atomic.Int64)}
}

func (lc *LeastConn) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("no available instances")
	}

	var best *discovery.ServiceInstance
	bestConns := int64(-1)

	for i := range instances {
		c := lc.getCounter(instances[i].ID)
		n := c.Load()
		if best == nil || n < bestConns {
			best = &instances[i]
			bestConns = n
		}
	}

	return best, nil
}

func (lc *LeastConn) IncrConn(id string) {
	lc.getCounter(id).Add(1)
}

func (lc *LeastConn) DecrConn(id string) {
	lc.getCounter(id).Add(-1)
}

func (lc *LeastConn) getCounter(id string) *atomic.Int64 {
	lc.mu.RLock()
	c, ok := lc.conns[id]
	lc.mu.RUnlock()
	if ok {
		return c
	}
	lc.mu.Lock()
	defer lc.mu.Unlock()
	c, ok = lc.conns[id]
	if !ok {
		c = &atomic.Int64{}
		lc.conns[id] = c
	}
	return c
}
```

- [ ] **Step 7: Run all load balancer tests**

Run: `go test ./router/loadbalancer/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add router/loadbalancer/
git commit -m "feat: add weighted, random, iphash, leastconn load balancers"
```

---

### Task 9: Router — prefix matching

**Files:**
- Create: `router/router.go`
- Create: `router/router_test.go`

- [ ] **Step 1: Write router test**

Create `router/router_test.go`:

```go
package router

import (
	"net/http"
	"testing"
	"time"

	"github.com/dysodeng/gateway/config"
)

func TestRouter_Match(t *testing.T) {
	routes := []config.RouteConfig{
		{Name: "users", Prefix: "/api/v1/users", Service: "user-svc", Timeout: 5 * time.Second},
		{Name: "orders", Prefix: "/api/v1/orders", Service: "order-svc", Timeout: 5 * time.Second},
		{Name: "api-catch-all", Prefix: "/api", Service: "api-svc", Timeout: 5 * time.Second},
	}

	r := New(routes)

	tests := []struct {
		path        string
		wantRoute   string
		wantMatched bool
	}{
		{"/api/v1/users/123", "users", true},
		{"/api/v1/orders", "orders", true},
		{"/api/v2/anything", "api-catch-all", true},
		{"/other/path", "", false},
	}

	for _, tt := range tests {
		req, _ := http.NewRequest("GET", tt.path, nil)
		route, matched := r.Match(req)
		if matched != tt.wantMatched {
			t.Errorf("Match(%q) matched=%v, want %v", tt.path, matched, tt.wantMatched)
			continue
		}
		if matched && route.Name != tt.wantRoute {
			t.Errorf("Match(%q) route=%q, want %q", tt.path, route.Name, tt.wantRoute)
		}
	}
}

func TestRouter_Match_LongestPrefix(t *testing.T) {
	routes := []config.RouteConfig{
		{Name: "short", Prefix: "/api", Service: "svc-a", Timeout: 5 * time.Second},
		{Name: "long", Prefix: "/api/v1/users", Service: "svc-b", Timeout: 5 * time.Second},
	}

	r := New(routes)
	req, _ := http.NewRequest("GET", "/api/v1/users/123", nil)

	route, matched := r.Match(req)
	if !matched {
		t.Fatal("expected match")
	}
	if route.Name != "long" {
		t.Errorf("got route %q, want 'long' (longest prefix)", route.Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./router/ -v -run TestRouter_Match`
Expected: FAIL

- [ ] **Step 3: Implement router**

Create `router/router.go`:

```go
package router

import (
	"net/http"
	"sort"
	"strings"

	"github.com/dysodeng/gateway/config"
)

type Router struct {
	routes []config.RouteConfig // sorted by prefix length descending
}

func New(routes []config.RouteConfig) *Router {
	sorted := make([]config.RouteConfig, len(routes))
	copy(sorted, routes)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Prefix) > len(sorted[j].Prefix)
	})
	return &Router{routes: sorted}
}

func (r *Router) Match(req *http.Request) (*config.RouteConfig, bool) {
	path := req.URL.Path
	for i := range r.routes {
		if strings.HasPrefix(path, r.routes[i].Prefix) {
			return &r.routes[i], true
		}
	}
	return nil, false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./router/ -v -run TestRouter_Match`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add router/router.go router/router_test.go
git commit -m "feat: add prefix-based router with longest-match"
```

---

### Task 10: Canary decision tree

**Files:**
- Create: `router/canary.go`
- Create: `router/canary_test.go`

- [ ] **Step 1: Write canary tests**

Create `router/canary_test.go`:

```go
package router

import (
	"net/http"
	"testing"

	"github.com/dysodeng/gateway/config"
)

func TestCanary_HeaderMatchWeightZero(t *testing.T) {
	rules := []config.CanaryRuleConfig{
		{
			Weight:  0,
			Service: "svc-v2",
			Match:   &config.CanaryMatch{Headers: map[string]string{"x-canary": "true"}},
		},
	}

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("x-canary", "true")

	service := ResolveCanary(rules, "svc-v1", req)
	if service != "svc-v2" {
		t.Errorf("expected svc-v2, got %q", service)
	}
}

func TestCanary_NoMatch_DefaultService(t *testing.T) {
	rules := []config.CanaryRuleConfig{
		{
			Weight:  0,
			Service: "svc-v2",
			Match:   &config.CanaryMatch{Headers: map[string]string{"x-canary": "true"}},
		},
	}

	req, _ := http.NewRequest("GET", "/", nil)
	// No canary header set

	service := ResolveCanary(rules, "svc-v1", req)
	if service != "svc-v1" {
		t.Errorf("expected svc-v1 (default), got %q", service)
	}
}

func TestCanary_WeightOnlyRule(t *testing.T) {
	// weight=100 with no header match → always route to canary
	rules := []config.CanaryRuleConfig{
		{Weight: 100, Service: "svc-v3"},
	}

	req, _ := http.NewRequest("GET", "/", nil)

	service := ResolveCanary(rules, "svc-v1", req)
	if service != "svc-v3" {
		t.Errorf("expected svc-v3 (100%% weight), got %q", service)
	}
}

func TestCanary_EmptyRules(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	service := ResolveCanary(nil, "svc-v1", req)
	if service != "svc-v1" {
		t.Errorf("expected svc-v1 (default), got %q", service)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./router/ -v -run TestCanary`
Expected: FAIL

- [ ] **Step 3: Implement canary**

Create `router/canary.go`:

```go
package router

import (
	"math/rand/v2"
	"net/http"

	"github.com/dysodeng/gateway/config"
)

// ResolveCanary applies canary rules and returns the target service name.
// Decision tree:
// 1. For each rule, check match.headers conditions
// 2. Header match + weight > 0: probability-based split
// 3. Header match + weight == 0: 100% canary (pure header match)
// 4. No header condition + weight > 0: weight-based random split on all traffic
// 5. No match: return defaultService
func ResolveCanary(rules []config.CanaryRuleConfig, defaultService string, req *http.Request) string {
	for _, rule := range rules {
		hasHeaderMatch := rule.Match != nil && len(rule.Match.Headers) > 0

		if hasHeaderMatch {
			if !matchHeaders(rule.Match.Headers, req) {
				continue
			}
			// Header matched
			if rule.Weight == 0 {
				return rule.Service
			}
			if rand.IntN(100) < rule.Weight {
				return rule.Service
			}
			continue
		}

		// No header condition — weight-only rule
		if rule.Weight > 0 {
			if rand.IntN(100) < rule.Weight {
				return rule.Service
			}
		}
	}

	return defaultService
}

func matchHeaders(expected map[string]string, req *http.Request) bool {
	for key, val := range expected {
		if req.Header.Get(key) != val {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./router/ -v -run TestCanary`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add router/canary.go router/canary_test.go
git commit -m "feat: add canary decision tree for traffic splitting"
```

---

## Chunk 3: Proxy Layer (HTTP, WebSocket, SSE, Circuit Breaker, Retry)

### Task 11: HTTP reverse proxy

**Files:**
- Create: `proxy/proxy.go`
- Create: `proxy/http.go`
- Create: `proxy/http_test.go`

- [ ] **Step 1: Write proxy dispatcher and HTTP proxy test**

Create `proxy/proxy_test.go` (for dispatcher) and `proxy/http_test.go`:

`proxy/http_test.go`:
```go
package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dysodeng/gateway/discovery"
)

func TestHTTPProxy_Forward(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "true")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	// Parse backend host/port
	host, port := parseHostPort(t, backend.URL)

	instance := &discovery.ServiceInstance{
		ID: "test", Host: host, Port: port,
	}

	p := NewHTTPProxy()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/test", nil)

	p.Forward(rec, req, instance, false, "")

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello from backend" {
		t.Errorf("body = %q, want %q", string(body), "hello from backend")
	}
}

func TestHTTPProxy_StripPrefix(t *testing.T) {
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	host, port := parseHostPort(t, backend.URL)
	instance := &discovery.ServiceInstance{ID: "test", Host: host, Port: port}

	p := NewHTTPProxy()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/users/123", nil)

	p.Forward(rec, req, instance, true, "/api/v1/users")

	if receivedPath != "/123" {
		t.Errorf("backend received path %q, want %q", receivedPath, "/123")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./proxy/ -v -run TestHTTPProxy`
Expected: FAIL

- [ ] **Step 3: Implement HTTP proxy**

Create `proxy/http.go`:

```go
package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/dysodeng/gateway/discovery"
)

type HTTPProxy struct{}

func NewHTTPProxy() *HTTPProxy {
	return &HTTPProxy{}
}

func (p *HTTPProxy) Forward(w http.ResponseWriter, r *http.Request, instance *discovery.ServiceInstance, stripPrefix bool, prefix string) {
	target := &url.URL{
		Scheme: "http",
		Host:   instance.Addr(),
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			if stripPrefix && prefix != "" {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
				if req.URL.Path == "" {
					req.URL.Path = "/"
				}
			}
		},
	}

	proxy.ServeHTTP(w, r)
}
```

Create `proxy/proxy.go` with helpers:

```go
package proxy

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"testing"
)

// parseHostPort is a test helper to extract host and port from a URL string.
func parseHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}
```

Wait — the test helper should be in a test file. Let me fix that.

Create `proxy/proxy.go`:

```go
package proxy
```

Create `proxy/testhelper_test.go`:

```go
package proxy

import (
	"net"
	"net/url"
	"strconv"
	"testing"
)

func parseHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./proxy/ -v -run TestHTTPProxy`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add proxy/
git commit -m "feat: add HTTP reverse proxy"
```

---

### Task 12: WebSocket transparent proxy

**Files:**
- Create: `proxy/websocket.go`
- Create: `proxy/websocket_test.go`

- [ ] **Step 1: Write WebSocket proxy test**

Create `proxy/websocket_test.go`:

```go
package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dysodeng/gateway/discovery"
	"github.com/gorilla/websocket"
)

func TestWebSocketProxy_BiDirectional(t *testing.T) {
	// Backend WS server that echoes messages
	upgrader := websocket.Upgrader{}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("backend upgrade error: %v", err)
			return
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			conn.WriteMessage(mt, msg)
		}
	}))
	defer backend.Close()

	host, port := parseHostPort(t, backend.URL)
	instance := &discovery.ServiceInstance{ID: "ws-test", Host: host, Port: port}

	// Gateway WS proxy handler
	wsp := NewWebSocketProxy()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsp.Forward(w, r, instance)
	}))
	defer gateway.Close()

	// Client connects to gateway
	wsURL := "ws" + strings.TrimPrefix(gateway.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send message and verify echo
	msg := "hello websocket"
	if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, received, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(received) != msg {
		t.Errorf("received %q, want %q", string(received), msg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./proxy/ -v -run TestWebSocketProxy`
Expected: FAIL

- [ ] **Step 3: Implement WebSocket proxy**

Create `proxy/websocket.go`:

```go
package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/dysodeng/gateway/discovery"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WebSocketProxy struct{}

func NewWebSocketProxy() *WebSocketProxy {
	return &WebSocketProxy{}
}

func (p *WebSocketProxy) Forward(w http.ResponseWriter, r *http.Request, instance *discovery.ServiceInstance) {
	// Upgrade client connection
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}
	defer clientConn.Close()

	// Connect to backend
	backendURL := url.URL{
		Scheme: "ws",
		Host:   instance.Addr(),
		Path:   r.URL.Path,
	}

	backendConn, _, err := websocket.DefaultDialer.Dial(backendURL.String(), nil)
	if err != nil {
		slog.Error("websocket backend dial failed", "error", err, "addr", backendURL.String())
		clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "backend unavailable"))
		return
	}
	defer backendConn.Close()

	// Bidirectional relay
	done := make(chan struct{})

	// Backend → Client
	go func() {
		defer close(done)
		relay(backendConn, clientConn)
	}()

	// Client → Backend
	relay(clientConn, backendConn)
	<-done
}

func relay(src, dst *websocket.Conn) {
	for {
		messageType, reader, err := src.NextReader()
		if err != nil {
			dst.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
		writer, err := dst.NextWriter(messageType)
		if err != nil {
			return
		}
		io.Copy(writer, reader)
		writer.Close()
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./proxy/ -v -run TestWebSocketProxy`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add proxy/websocket.go proxy/websocket_test.go
git commit -m "feat: add WebSocket transparent proxy"
```

---

### Task 13: SSE transparent proxy

**Files:**
- Create: `proxy/sse.go`
- Create: `proxy/sse_test.go`

- [ ] **Step 1: Write SSE proxy test**

Create `proxy/sse_test.go`:

```go
package proxy

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dysodeng/gateway/discovery"
)

func TestSSEProxy_Forward(t *testing.T) {
	// Backend SSE server that sends 3 events then closes
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}
		for i := 0; i < 3; i++ {
			w.Write([]byte("data: hello\n\n"))
			flusher.Flush()
		}
	}))
	defer backend.Close()

	host, port := parseHostPort(t, backend.URL)
	instance := &discovery.ServiceInstance{ID: "sse-test", Host: host, Port: port}

	sp := NewSSEProxy()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sp.Forward(w, r, instance, false, "")
	}))
	defer gateway.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(gateway.URL + "/events")
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			count++
		}
	}
	if count != 3 {
		t.Errorf("received %d events, want 3", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./proxy/ -v -run TestSSEProxy`
Expected: FAIL

- [ ] **Step 3: Implement SSE proxy**

Create `proxy/sse.go`:

```go
package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/dysodeng/gateway/discovery"
)

type SSEProxy struct{}

func NewSSEProxy() *SSEProxy {
	return &SSEProxy{}
}

func (p *SSEProxy) Forward(w http.ResponseWriter, r *http.Request, instance *discovery.ServiceInstance, stripPrefix bool, prefix string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	path := r.URL.Path
	if stripPrefix && prefix != "" {
		path = strings.TrimPrefix(path, prefix)
		if path == "" {
			path = "/"
		}
	}

	backendURL := "http://" + instance.Addr() + path
	if r.URL.RawQuery != "" {
		backendURL += "?" + r.URL.RawQuery
	}

	backendReq, err := http.NewRequestWithContext(r.Context(), r.Method, backendURL, nil)
	if err != nil {
		slog.Error("sse create request failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Copy headers from client
	for key, vals := range r.Header {
		for _, v := range vals {
			backendReq.Header.Add(key, v)
		}
	}

	resp, err := http.DefaultClient.Do(backendReq)
	if err != nil {
		slog.Error("sse backend request failed", "error", err)
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream body with flushing
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				slog.Error("sse read error", "error", err)
			}
			return
		}
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./proxy/ -v -run TestSSEProxy`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add proxy/sse.go proxy/sse_test.go
git commit -m "feat: add SSE transparent proxy"
```

---

### Task 14: Circuit breaker

**Files:**
- Create: `proxy/circuitbreaker.go`
- Create: `proxy/circuitbreaker_test.go`

- [ ] **Step 1: Write circuit breaker tests**

Create `proxy/circuitbreaker_test.go` with tests for:
- Closed state: requests pass through, failures recorded
- Open state: after threshold failures, requests are rejected immediately
- Half-open state: after timeout, one probe request is allowed; success → closed, failure → open

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./proxy/ -v -run TestCircuitBreaker`
Expected: FAIL

- [ ] **Step 3: Implement circuit breaker**

Create `proxy/circuitbreaker.go`:

```go
package proxy

import (
	"errors"
	"sync"
	"time"
)

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type CircuitBreaker struct {
	mu           sync.Mutex
	state        CircuitState
	failures     int
	threshold    int
	timeout      time.Duration
	lastFailure  time.Time
	probing      bool // true when a probe request is in-flight during half-open state
}

func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		timeout:   timeout,
	}
}

func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		if time.Since(cb.lastFailure) > cb.timeout {
			cb.state = StateHalfOpen
			cb.probing = true
			return nil
		}
		return ErrCircuitOpen
	case StateHalfOpen:
		if cb.probing {
			// Only one probe request allowed in half-open state
			return ErrCircuitOpen
		}
		cb.probing = true
		return nil
	}
	return nil
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = StateClosed
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cb.threshold {
		cb.state = StateOpen
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./proxy/ -v -run TestCircuitBreaker`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add proxy/circuitbreaker.go proxy/circuitbreaker_test.go
git commit -m "feat: add per-service circuit breaker"
```

---

### Task 15: Retry logic

**Files:**
- Create: `proxy/retry.go`
- Create: `proxy/retry_test.go`

- [ ] **Step 1: Write retry tests**

Tests should verify:
- Retries on 5xx when condition is `"5xx"`
- No retry on non-idempotent methods (POST/PATCH/DELETE) by default
- Retries on timeout when condition is `"timeout"`
- Stops after max count
- Returns last error when all retries exhausted

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./proxy/ -v -run TestRetry`
Expected: FAIL

- [ ] **Step 3: Implement retry**

Create `proxy/retry.go`:

```go
package proxy

import (
	"net/http"
)

type RetryConfig struct {
	Count      int
	Conditions []string // "5xx", "timeout"
}

// ShouldRetry determines if a request should be retried based on the response
// status code, error, and retry configuration.
// Non-idempotent methods (POST, PATCH, DELETE) are not retried by default.
func ShouldRetry(method string, statusCode int, err error, cfg RetryConfig) bool {
	if !isIdempotent(method) {
		return false
	}
	for _, cond := range cfg.Conditions {
		switch cond {
		case "5xx":
			if statusCode >= 500 && statusCode < 600 {
				return true
			}
		case "timeout":
			if err != nil {
				return true
			}
		}
	}
	return false
}

func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut:
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./proxy/ -v -run TestRetry`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add proxy/retry.go proxy/retry_test.go
git commit -m "feat: add retry logic with idempotency checks"
```

---

### Task 16: Proxy dispatcher

**Files:**
- Modify: `proxy/proxy.go`
- Create: `proxy/proxy_test.go`

- [ ] **Step 1: Write dispatcher test**

Create `proxy/proxy_test.go` testing that:
- Normal requests are dispatched to HTTP proxy
- Requests with `Connection: Upgrade` + `Upgrade: websocket` are dispatched to WebSocket proxy
- Routes with `type: "sse"` are dispatched to SSE proxy

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./proxy/ -v -run TestDispatcher`
Expected: FAIL

- [ ] **Step 3: Implement dispatcher**

Update `proxy/proxy.go`:

```go
package proxy

import (
	"net/http"
	"strings"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
)

type Dispatcher struct {
	httpProxy *HTTPProxy
	wsProxy   *WebSocketProxy
	sseProxy  *SSEProxy
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		httpProxy: NewHTTPProxy(),
		wsProxy:   NewWebSocketProxy(),
		sseProxy:  NewSSEProxy(),
	}
}

func (d *Dispatcher) Forward(w http.ResponseWriter, r *http.Request, route *config.RouteConfig, instance *discovery.ServiceInstance) {
	if isWebSocketRequest(r) {
		d.wsProxy.Forward(w, r, instance)
		return
	}

	if route.Type == "sse" {
		d.sseProxy.Forward(w, r, instance, route.StripPrefix, route.Prefix)
		return
	}

	d.httpProxy.Forward(w, r, instance, route.StripPrefix, route.Prefix)
}

func isWebSocketRequest(r *http.Request) bool {
	// Connection header can contain multiple comma-separated values
	// e.g. "keep-alive, Upgrade"
	for _, v := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(v), "upgrade") {
			return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./proxy/ -v -run TestDispatcher`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add proxy/proxy.go proxy/proxy_test.go
git commit -m "feat: add proxy dispatcher for REST/WS/SSE"
```

---

## Chunk 4: Pre-Route Middleware

### Task 17: Middleware chain

**Files:**
- Create: `middleware/chain.go`
- Create: `middleware/chain_test.go`

- [ ] **Step 1: Write chain test**

Test that Pre-Route middleware executes in order, then router is called, then Post-Route middleware executes in order.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./middleware/ -v -run TestChain`
Expected: FAIL

- [ ] **Step 3: Implement middleware chain**

Create `middleware/chain.go`:

```go
package middleware

import "net/http"

// Middleware is a function that wraps an http.Handler.
type Middleware func(next http.Handler) http.Handler

// Chain applies a sequence of middleware to a handler.
// Middleware are applied in the order provided:
// Chain(m1, m2, m3)(handler) => m1(m2(m3(handler)))
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./middleware/ -v -run TestChain`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add middleware/chain.go middleware/chain_test.go
git commit -m "feat: add middleware chain builder"
```

---

### Task 18: Recovery middleware

**Files:**
- Create: `middleware/recovery.go`
- Create: `middleware/recovery_test.go`

- [ ] **Step 1: Write test** — handler that panics should return 500, not crash the server.

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Implement** — catch panics with `recover()`, log with slog, return 500.

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Commit**

```bash
git add middleware/recovery.go middleware/recovery_test.go
git commit -m "feat: add recovery middleware"
```

---

### Task 19: Access log middleware

**Files:**
- Create: `middleware/accesslog.go`
- Create: `middleware/accesslog_test.go`

- [ ] **Step 1: Write test** — verify that a request/response pair produces a structured log entry with method, path, status, latency, trace_id.

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Implement** — use `slog` to log JSON with method, path, status code, latency_ms, trace_id from request header, upstream info.

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Commit**

```bash
git add middleware/accesslog.go middleware/accesslog_test.go
git commit -m "feat: add access log middleware"
```

---

### Task 20: CORS middleware

**Files:**
- Create: `middleware/cors.go`
- Create: `middleware/cors_test.go`

- [ ] **Step 1: Write tests** — preflight OPTIONS returns correct headers, normal requests get CORS headers, origins are checked against allowed list.

- [ ] **Step 2: Run tests, verify fail**

- [ ] **Step 3: Implement** — handle OPTIONS preflight, set `Access-Control-Allow-*` headers from `CORSConfig`.

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Commit**

```bash
git add middleware/cors.go middleware/cors_test.go
git commit -m "feat: add CORS middleware"
```

---

### Task 21: IP filter middleware

**Files:**
- Create: `middleware/ipfilter.go`
- Create: `middleware/ipfilter_test.go`

- [ ] **Step 1: Write tests** — whitelist mode: allowed IPs pass, others blocked. Blacklist mode: blocked IPs rejected, others pass. CIDR ranges work. Test both `NewGlobalIPFilter` and `NewRouteIPFilter`.

- [ ] **Step 2: Run tests, verify fail**

- [ ] **Step 3: Implement** — parse CIDR ranges, check `RemoteAddr` against list, support both modes. Export two constructors: `NewGlobalIPFilter` (Pre-Route, uses global config) and `NewRouteIPFilter` (Post-Route, uses route config).

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Commit**

```bash
git add middleware/ipfilter.go middleware/ipfilter_test.go
git commit -m "feat: add IP filter middleware (global + route-level)"
```

---

### Task 22: OpenTelemetry tracing setup and middleware

**Files:**
- Create: `pkg/trace/trace.go`
- Create: `pkg/trace/trace_test.go`
- Create: `middleware/tracing.go`
- Create: `middleware/tracing_test.go`

- [ ] **Step 1: Write OTel provider init**

Create `pkg/trace/trace.go` — initialize `TracerProvider` based on `TelemetryConfig`. Support both gRPC (`otlptracegrpc`) and HTTP (`otlptracehttp`) exporters based on `config.protocol`. Configure sampler (always/ratio/never). Return a shutdown function.

- [ ] **Step 2: Write tracing middleware test** — verify that a request passing through the middleware gets `X-Trace-Id` injected into the request headers (for downstream propagation).

- [ ] **Step 3: Implement tracing middleware** — start a span, inject `X-Trace-Id` header with the trace ID, defer span end.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/trace/ -v && go test ./middleware/ -v -run TestTracing`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/trace/ middleware/tracing.go middleware/tracing_test.go
git commit -m "feat: add OpenTelemetry tracing setup and middleware"
```

---

## Chunk 5: Post-Route Middleware

### Task 23: Auth middleware (JWT)

**Files:**
- Create: `middleware/auth.go`
- Create: `middleware/auth_test.go`

- [ ] **Step 1: Write JWT auth tests**

Tests:
- Valid JWT → passes, claims injected into headers per `claims_to_headers`
- Invalid JWT → 401 response
- Expired JWT → 401 response
- Missing Authorization header → 401 response
- Auth disabled (no scheme on route) → passes through

- [ ] **Step 2: Run tests, verify fail**

- [ ] **Step 3: Implement JWT auth**

Use `github.com/golang-jwt/jwt/v5`. Parse token from `Authorization: Bearer <token>`, validate with the scheme's secret and algorithm, extract claims, inject into request headers per `claims_to_headers` mapping.

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Commit**

```bash
git add middleware/auth.go middleware/auth_test.go
git commit -m "feat: add JWT auth middleware"
```

---

### Task 24: Auth middleware (API Key + OAuth2)

**Files:**
- Modify: `middleware/auth.go`
- Modify: `middleware/auth_test.go`

- [ ] **Step 1: Write API Key auth tests** — valid key in header/query passes, missing key → 401.

- [ ] **Step 2: Write OAuth2 introspection tests** — mock introspection endpoint, valid token → pass with claims injected, invalid token → 401.

- [ ] **Step 3: Implement** — API Key: check header then query param against config. OAuth2: POST to introspect_endpoint with token, parse response, inject claims.

- [ ] **Step 4: Run all auth tests**

Run: `go test ./middleware/ -v -run TestAuth`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add middleware/auth.go middleware/auth_test.go
git commit -m "feat: add API key and OAuth2 introspection auth"
```

---

### Task 25: Rate limit middleware

**Files:**
- Create: `middleware/ratelimit.go`
- Create: `middleware/ratelimit_test.go`

- [ ] **Step 1: Write rate limit tests**

Tests:
- Under limit: requests pass
- Over limit: requests get 429
- Disabled: requests pass regardless
- Test local (in-memory) storage mode using sliding window

- [ ] **Step 2: Run tests, verify fail**

- [ ] **Step 3: Implement** — sliding window counter in memory (local mode). Redis mode can be a stub for now since it depends on Redis. Key by route name + client IP. Return 429 with `Retry-After` header when exceeded.

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Commit**

```bash
git add middleware/ratelimit.go middleware/ratelimit_test.go
git commit -m "feat: add rate limiting middleware (local mode)"
```

---

### Task 26: Request signature middleware

**Files:**
- Create: `middleware/requestsign.go`
- Create: `middleware/requestsign_test.go`

- [ ] **Step 1: Write tests** — valid signature passes, invalid signature → 403, expired timestamp → 403, missing headers → 403.

- [ ] **Step 2: Run tests, verify fail**

- [ ] **Step 3: Implement** — extract `X-Signature` and `X-Timestamp` headers, verify timestamp is within `expire` seconds, compute HMAC-SHA256 over request body + timestamp, compare with provided signature.

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Commit**

```bash
git add middleware/requestsign.go middleware/requestsign_test.go
git commit -m "feat: add request signature verification middleware"
```

---

### Task 27: Rewrite middleware

**Files:**
- Create: `middleware/rewrite.go`
- Create: `middleware/rewrite_test.go`

- [ ] **Step 1: Write tests** — path rewrite changes `r.URL.Path`, header injection adds/removes headers.

- [ ] **Step 2: Run tests, verify fail**

- [ ] **Step 3: Implement** — modify request path and headers based on route rewrite config. `strip_prefix` is handled by proxy layer; this middleware handles custom header injection/removal.

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Commit**

```bash
git add middleware/rewrite.go middleware/rewrite_test.go
git commit -m "feat: add request rewrite middleware"
```

---

## Chunk 6: Observability, Health, Server, Main Entry

### Task 28: Prometheus metrics

**Files:**
- Modify: `pkg/trace/trace.go` (add metrics provider)

- [ ] **Step 1: Write metrics test** — verify that after handling a request, metrics counters are incremented.

- [ ] **Step 2: Implement** — set up OTel metrics provider with Prometheus exporter. Register `gateway_request_total` (counter), `gateway_request_duration` (histogram), `gateway_active_connections` (gauge), `gateway_circuit_breaker_state` (gauge).

- [ ] **Step 3: Run tests, verify pass**

- [ ] **Step 4: Commit**

```bash
git add pkg/trace/
git commit -m "feat: add Prometheus metrics via OpenTelemetry"
```

---

### Task 29: Health check endpoint

**Files:**
- Create: `pkg/health/health.go`
- Create: `pkg/health/health_test.go`

- [ ] **Step 1: Write test** — GET /health returns JSON with status and checks. When a checker reports unhealthy, status is "unhealthy".

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Implement** — `Health` handler that runs registered checkers (discovery, config center), returns JSON `{"status": "healthy", "checks": {...}}`.

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Commit**

```bash
git add pkg/health/
git commit -m "feat: add health check endpoint"
```

---

### Task 30: Server wiring

**Files:**
- Create: `server/server.go`
- Create: `server/server_test.go`

- [ ] **Step 1: Write server test** — verify the server starts, routes a request through the full pipeline (middleware → router → proxy), and handles graceful shutdown.

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Implement server**

Create `server/server.go` that wires everything together:

```go
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
	"github.com/dysodeng/gateway/middleware"
	"github.com/dysodeng/gateway/proxy"
	"github.com/dysodeng/gateway/router"
	"github.com/dysodeng/gateway/router/loadbalancer"
)

type Server struct {
	cfg        *config.Config
	httpServer *http.Server
	discovery  discovery.Discovery
}

func New(cfg *config.Config, disc discovery.Discovery) *Server {
	s := &Server{cfg: cfg, discovery: disc}

	r := router.New(cfg.Routes)
	dispatcher := proxy.NewDispatcher()

	// Build balancer registry
	balancers := buildBalancers(cfg.Routes)

	// Core handler: router → post-route middleware → proxy
	coreHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		route, matched := r.Match(req)
		if !matched {
			http.NotFound(w, req)
			return
		}

		// Resolve canary
		serviceName := router.ResolveCanary(route.Canary, route.Service, req)

		// Get instances
		instances, err := disc.GetInstances(serviceName)
		if err != nil {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		// Load balance
		balancer := balancers[route.Name]
		instance, err := balancer.Select(instances, req)
		if err != nil {
			http.Error(w, "no available instance", http.StatusServiceUnavailable)
			return
		}

		// Apply post-route middleware
		postRoute := buildPostRouteMiddleware(cfg, route)
		finalHandler := postRoute(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			dispatcher.Forward(w, req, route, instance)
		}))
		finalHandler.ServeHTTP(w, req)
	})

	// Build pre-route middleware chain
	preRoute := middleware.Chain(
		middleware.NewRecovery(),
		// middleware.NewAccessLog(),
		// middleware.NewCORS(cfg.CORS),
		// middleware.NewTracing(),
		// middleware.NewGlobalIPFilter(cfg.IPFilter),
	)

	// Health and metrics bypass middleware
	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Health.Path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"healthy"}`))
	})
	if cfg.Metrics.Enabled {
		mux.Handle(cfg.Metrics.Path, promhttp.Handler())
	}
	mux.Handle("/", preRoute(coreHandler))

	s.httpServer = &http.Server{
		Addr:    cfg.Server.Listen,
		Handler: mux,
	}

	return s
}

func (s *Server) Start() error {
	slog.Info("gateway starting", "addr", s.cfg.Server.Listen)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("gateway shutting down")
	return s.httpServer.Shutdown(ctx)
}

func buildBalancers(routes []config.RouteConfig) map[string]loadbalancer.Balancer {
	balancers := make(map[string]loadbalancer.Balancer)
	for _, route := range routes {
		switch route.LoadBalancer {
		case "weighted":
			balancers[route.Name] = loadbalancer.NewWeighted()
		case "random":
			balancers[route.Name] = loadbalancer.NewRandom()
		case "ip_hash":
			balancers[route.Name] = loadbalancer.NewIPHash()
		case "least_conn":
			balancers[route.Name] = loadbalancer.NewLeastConn()
		default:
			balancers[route.Name] = loadbalancer.NewRoundRobin()
		}
	}
	return balancers
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./server/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/
git commit -m "feat: add server wiring with full request pipeline"
```

---

### Task 31: Main entry point

**Files:**
- Create: `cmd/gateway/main.go`

- [ ] **Step 1: Implement main.go**

```go
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dysodeng/gateway/config"
	"github.com/dysodeng/gateway/discovery"
	"github.com/dysodeng/gateway/server"
)

func main() {
	configPath := flag.String("config", "gateway.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize discovery
	var disc discovery.Discovery
	switch cfg.Discovery.Type {
	case "static":
		if cfg.Discovery.Static == nil {
			slog.Error("static discovery config required")
			os.Exit(1)
		}
		disc = discovery.NewStaticDiscovery(cfg.Discovery.Static)
	default:
		slog.Error("unsupported discovery type", "type", cfg.Discovery.Type)
		os.Exit(1)
	}

	// Initialize OTel tracing if enabled
	var shutdownTracer func(context.Context) error
	if cfg.Telemetry.Enabled {
		shutdown, err := trace.InitProvider(cfg.Telemetry)
		if err != nil {
			slog.Error("failed to init OTel tracer", "error", err)
			os.Exit(1)
		}
		shutdownTracer = shutdown
	}

	srv := server.New(cfg, disc)

	// Graceful shutdown (spec: 5-step sequence)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()

		// 1+2: Stop accepting new connections, drain in-flight requests
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "error", err)
		}

		// 3: Stop config watcher (if any)
		// TODO: stop config watcher when implemented

		// 4: Stop discovery watcher
		if disc != nil {
			disc.Stop()
		}

		// 5: Flush OTel exporter
		if shutdownTracer != nil {
			if err := shutdownTracer(ctx); err != nil {
				slog.Error("OTel shutdown error", "error", err)
			}
		}
	}()

	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}

	slog.Info("gateway stopped")
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/gateway/`
Expected: No errors.

- [ ] **Step 3: Smoke test with example config**

Run: `./gateway -config gateway.yaml &` then `curl http://localhost:8080/health` then kill the process.
Expected: `{"status":"healthy"}`

- [ ] **Step 4: Commit**

```bash
git add cmd/gateway/main.go
git commit -m "feat: add main entry point with graceful shutdown"
```

---

### Task 32: Integration test — full pipeline

**Files:**
- Create: `server/server_test.go` (integration test)

- [ ] **Step 1: Write integration test**

Test the full pipeline: spin up a backend HTTP server, create a config pointing to it via static discovery, start the gateway server, send a request through, verify it reaches the backend and returns correctly. Test REST, WebSocket, and SSE paths.

- [ ] **Step 2: Run test, verify it passes**

Run: `go test ./server/ -v -run TestIntegration`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add server/server_test.go
git commit -m "test: add integration test for full gateway pipeline"
```

---

### Task 33: Structured logging initialization

**Files:**
- Create: `pkg/log/log.go`
- Create: `pkg/log/log_test.go`

- [ ] **Step 1: Write test** — verify that `InitLogger` with `level: "debug"` and `output: "stdout"` configures slog with JSON handler and correct level. Verify log output is valid JSON with expected fields.

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Implement**

Create `pkg/log/log.go` that:
- Creates `slog.JSONHandler` for stdout or file output
- Sets log level from config (`debug/info/warn/error`)
- For file output, uses `lumberjack` or similar for rotation (max_size, max_backups, max_age)
- Sets the configured handler as `slog.SetDefault`

- [ ] **Step 4: Run tests, verify pass**

- [ ] **Step 5: Wire into main.go** — call `log.InitLogger(cfg.Log)` before anything else in `main()`.

- [ ] **Step 6: Commit**

```bash
git add pkg/log/
git commit -m "feat: add structured logging initialization"
```

---

### Task 34: Wire post-route middleware into server

**Files:**
- Modify: `server/server.go`

- [ ] **Step 1: Implement `buildPostRouteMiddleware`**

Add to `server/server.go`:

```go
func buildPostRouteMiddleware(cfg *config.Config, route *config.RouteConfig) middleware.Middleware {
	var mws []middleware.Middleware

	// Route-level IP filter
	if route.Middleware.IPFilter != nil {
		mws = append(mws, middleware.NewRouteIPFilter(*route.Middleware.IPFilter))
	}

	// Auth
	if route.Middleware.Auth != nil {
		scheme, ok := cfg.AuthSchemes[route.Middleware.Auth.Scheme]
		if ok {
			mws = append(mws, middleware.NewAuth(scheme))
		}
	}

	// Rate limit
	if route.Middleware.RateLimit != nil && route.Middleware.RateLimit.Enabled {
		mws = append(mws, middleware.NewRateLimit(cfg.RateLimit, *route.Middleware.RateLimit))
	}

	// Request sign
	if route.Middleware.RequestSign != nil && route.Middleware.RequestSign.Enabled {
		mws = append(mws, middleware.NewRequestSign(cfg.RequestSign))
	}

	// Rewrite
	mws = append(mws, middleware.NewRewrite())

	return middleware.Chain(mws...)
}
```

- [ ] **Step 2: Write test** — verify that a request going through server with auth scheme configured gets 401 without token, 200 with valid token.

- [ ] **Step 3: Run tests, verify pass**

- [ ] **Step 4: Commit**

```bash
git add server/server.go server/server_test.go
git commit -m "feat: wire post-route middleware into request pipeline"
```

---

### Task 35: Enforce max_request_body_size

**Files:**
- Modify: `server/server.go`

- [ ] **Step 1: Write test** — send request with body exceeding `max_request_body_size`, verify 413 response. Verify WebSocket and SSE routes are not affected.

- [ ] **Step 2: Implement** — in the core handler, wrap `req.Body` with `http.MaxBytesReader` using route-level override or global default. Skip for WebSocket and SSE route types.

- [ ] **Step 3: Run tests, verify pass**

- [ ] **Step 4: Commit**

```bash
git add server/server.go server/server_test.go
git commit -m "feat: enforce max_request_body_size with route override"
```

---

### Task 36: WebSocket heartbeat and connection limits

**Files:**
- Modify: `proxy/websocket.go`
- Modify: `proxy/websocket_test.go`

- [ ] **Step 1: Write tests** — verify ping/pong heartbeat is sent at configured interval. Verify connections beyond `max_connections` are rejected with 503.

- [ ] **Step 2: Implement**

Update `WebSocketProxy` to:
- Accept `WebSocketConfig` parameter with heartbeat and max_connections
- Track active connection count with `atomic.Int64`, reject with 503 when full
- Start a goroutine per connection that sends ping frames at `heartbeat` interval
- Handle pong responses, close connection on pong timeout

- [ ] **Step 3: Run tests, verify pass**

- [ ] **Step 4: Commit**

```bash
git add proxy/websocket.go proxy/websocket_test.go
git commit -m "feat: add WebSocket heartbeat and connection limits"
```

---

### Task 37: SSE keepalive and retry

**Files:**
- Modify: `proxy/sse.go`
- Modify: `proxy/sse_test.go`

- [ ] **Step 1: Write tests** — verify keepalive comment lines (`: keepalive\n\n`) are sent at configured interval. Verify `retry:` field is sent to client.

- [ ] **Step 2: Implement**

Update `SSEProxy.Forward` to:
- Accept `SSEConfig` parameter
- Send `retry: <ms>\n\n` as the first event to configure client reconnection
- Start a goroutine that sends `: keepalive\n\n` comment lines at the configured interval

- [ ] **Step 3: Run tests, verify pass**

- [ ] **Step 4: Commit**

```bash
git add proxy/sse.go proxy/sse_test.go
git commit -m "feat: add SSE keepalive heartbeat and retry config"
```

---

### Task 38: Final verification and cleanup

- [ ] **Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 2: Run linter**

Run: `go vet ./...`
Expected: No errors

- [ ] **Step 3: Verify build**

Run: `go build -o gateway ./cmd/gateway/`
Expected: Binary produced successfully

- [ ] **Step 4: Commit any remaining changes**

```bash
git add -A
git commit -m "chore: final cleanup and verification"
```