package config

import "time"

// Config 网关顶层配置结构
type Config struct {
	Server      ServerConfig                `yaml:"server"`
	Log         LogConfig                   `yaml:"log"`
	Telemetry   TelemetryConfig             `yaml:"telemetry"`
	Metrics     MetricsConfig               `yaml:"metrics"`
	Health      HealthConfig                `yaml:"health"`
	CORS        CORSConfig                  `yaml:"cors"`
	IPFilter    IPFilterConfig              `yaml:"ip_filter"`
	RateLimit   RateLimitConfig             `yaml:"rate_limit"`
	RequestSign RequestSignConfig           `yaml:"request_sign"`
	AuthSchemes map[string]AuthSchemeConfig `yaml:"auth_schemes"`
	Routes      []RouteConfig               `yaml:"routes"`
	Discovery   DiscoveryConfig             `yaml:"discovery"`
}

// ServerConfig HTTP服务器配置
type ServerConfig struct {
	Listen             string        `yaml:"listen"`
	MaxRequestBodySize int64         `yaml:"max_request_body_size"`
	ShutdownTimeout    time.Duration `yaml:"shutdown_timeout"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string        `yaml:"level"`
	Output string        `yaml:"output"`
	File   LogFileConfig `yaml:"file"`
}

// LogFileConfig 日志文件配置
type LogFileConfig struct {
	Path       string `yaml:"path"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

// TelemetryConfig 链路追踪配置
type TelemetryConfig struct {
	Enabled     bool           `yaml:"enabled"`
	ServiceName string         `yaml:"service_name"`
	Exporter    ExporterConfig `yaml:"exporter"`
	Sampler     SamplerConfig  `yaml:"sampler"`
}

// ExporterConfig 追踪导出器配置
type ExporterConfig struct {
	Type     string `yaml:"type"`
	Protocol string `yaml:"protocol"`
	Endpoint string `yaml:"endpoint"`
}

// SamplerConfig 采样器配置
type SamplerConfig struct {
	Type  string  `yaml:"type"`
	Ratio float64 `yaml:"ratio"`
}

// MetricsConfig Prometheus指标配置
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// HealthConfig 健康检查配置
type HealthConfig struct {
	Path   string              `yaml:"path"`
	Checks []HealthCheckConfig `yaml:"check"`
}

// HealthCheckConfig 单个健康检查项配置
type HealthCheckConfig struct {
	Name string `yaml:"name"`
}

// CORSConfig 跨域资源共享配置
type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
	MaxAge         int      `yaml:"max_age"`
}

// IPFilterConfig IP过滤配置
type IPFilterConfig struct {
	Mode string   `yaml:"mode"`
	List []string `yaml:"list"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Storage   string      `yaml:"storage"`
	Redis     RedisConfig `yaml:"redis"`
	Algorithm string      `yaml:"algorithm"`
}

// RedisConfig Redis连接配置
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// RequestSignConfig 请求签名配置
type RequestSignConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Algorithm       string `yaml:"algorithm"`
	SignHeader      string `yaml:"sign_header"`
	TimestampHeader string `yaml:"timestamp_header"`
	Expire          int    `yaml:"expire"`
}

// AuthSchemeConfig 认证方案配置
type AuthSchemeConfig struct {
	Type   string        `yaml:"type"`
	JWT    *JWTConfig    `yaml:"jwt,omitempty"`
	APIKey *APIKeyConfig `yaml:"api_key,omitempty"`
	OAuth2 *OAuth2Config `yaml:"oauth2,omitempty"`
}

// JWTConfig JWT认证配置
type JWTConfig struct {
	Secret          string            `yaml:"secret"`
	Algorithms      []string          `yaml:"algorithms"`
	Header          string            `yaml:"header"`
	ClaimsToHeaders map[string]string `yaml:"claims_to_headers"`
}

// APIKeyConfig API Key认证配置
type APIKeyConfig struct {
	Header string `yaml:"header"`
	Query  string `yaml:"query"`
}

// OAuth2Config OAuth2认证配置
type OAuth2Config struct {
	IntrospectEndpoint string            `yaml:"introspect_endpoint"`
	ClientID           string            `yaml:"client_id"`
	ClientSecret       string            `yaml:"client_secret"`
	ClaimsToHeaders    map[string]string `yaml:"claims_to_headers"`
}

// RouteConfig 路由配置
type RouteConfig struct {
	Name               string                `yaml:"name"`
	Prefix             string                `yaml:"prefix"`
	StripPrefix        bool                  `yaml:"strip_prefix"`
	Service            string                `yaml:"service"`
	Type               string                `yaml:"type"`
	Timeout            time.Duration         `yaml:"timeout"`
	Retry              RetryConfig           `yaml:"retry"`
	LoadBalancer       string                `yaml:"load_balancer"`
	Middleware         RouteMiddlewareConfig  `yaml:"middleware"`
	Canary             []CanaryRuleConfig    `yaml:"canary"`
	WebSocket          *WebSocketConfig      `yaml:"websocket,omitempty"`
	SSE                *SSEConfig            `yaml:"sse,omitempty"`
	MaxRequestBodySize *int64                `yaml:"max_request_body_size,omitempty"`
}

// RetryConfig 重试配置
type RetryConfig struct {
	Count      int      `yaml:"count"`
	Conditions []string `yaml:"conditions"`
}

// RouteMiddlewareConfig 路由中间件配置
type RouteMiddlewareConfig struct {
	Auth        *RouteAuthConfig        `yaml:"auth,omitempty"`
	RateLimit   *RouteRateLimitConfig   `yaml:"rate_limit,omitempty"`
	RequestSign *RouteRequestSignConfig `yaml:"request_sign,omitempty"`
	IPFilter    *IPFilterConfig         `yaml:"ip_filter,omitempty"`
}

// RouteAuthConfig 路由认证中间件配置
type RouteAuthConfig struct {
	Scheme string `yaml:"scheme"`
}

// RouteRateLimitConfig 路由限流中间件配置
type RouteRateLimitConfig struct {
	Enabled bool `yaml:"enabled"`
	QPS     int  `yaml:"qps"`
}

// RouteRequestSignConfig 路由请求签名中间件配置
type RouteRequestSignConfig struct {
	Enabled bool `yaml:"enabled"`
}

// CanaryRuleConfig 灰度发布规则配置
type CanaryRuleConfig struct {
	Weight  int          `yaml:"weight"`
	Service string       `yaml:"service"`
	Match   *CanaryMatch `yaml:"match,omitempty"`
}

// CanaryMatch 灰度匹配条件
type CanaryMatch struct {
	Headers map[string]string `yaml:"headers"`
}

// WebSocketConfig WebSocket协议配置
type WebSocketConfig struct {
	Heartbeat      time.Duration `yaml:"heartbeat"`
	MaxConnections int           `yaml:"max_connections"`
}

// SSEConfig Server-Sent Events配置
type SSEConfig struct {
	Retry     int           `yaml:"retry"`
	Keepalive time.Duration `yaml:"keepalive"`
}

// DiscoveryConfig 服务发现配置
type DiscoveryConfig struct {
	Type   string                 `yaml:"type"`
	Static *StaticDiscoveryConfig `yaml:"static,omitempty"`
	Etcd   *EtcdConfig            `yaml:"etcd,omitempty"`
}

// StaticDiscoveryConfig 静态服务发现配置
type StaticDiscoveryConfig struct {
	Services map[string][]StaticInstanceConfig `yaml:"services"`
}

// StaticInstanceConfig 静态服务实例配置
type StaticInstanceConfig struct {
	Host     string            `yaml:"host"`
	Port     int               `yaml:"port"`
	Weight   int               `yaml:"weight"`
	Metadata map[string]string `yaml:"metadata,omitempty"`
}

// EtcdConfig etcd服务发现配置
type EtcdConfig struct {
	Endpoints []string      `yaml:"endpoints"`
	Timeout   time.Duration `yaml:"timeout"`
}
