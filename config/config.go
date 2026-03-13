package config

import "time"

const (
	VarPath  string = "var"
	LogPath         = VarPath + "/logs"
	TempPath        = VarPath + "/tmp"
)

// Config 网关顶层配置结构
type Config struct {
	ConfigCenter *ConfigCenterConfig         `mapstructure:"config_center,omitempty"`
	Server       ServerConfig                `mapstructure:"server"`
	Log          LogConfig                   `mapstructure:"log"`
	Telemetry    TelemetryConfig             `mapstructure:"telemetry"`
	Metrics      MetricsConfig               `mapstructure:"metrics"`
	Health       HealthConfig                `mapstructure:"health"`
	CORS         CORSConfig                  `mapstructure:"cors"`
	IPFilter     IPFilterConfig              `mapstructure:"ip_filter"`
	RateLimit    RateLimitConfig             `mapstructure:"rate_limit"`
	RequestSign  RequestSignConfig           `mapstructure:"request_sign"`
	AuthSchemes  map[string]AuthSchemeConfig `mapstructure:"auth_schemes"`
	Routes       []RouteConfig               `mapstructure:"routes"`
	Discovery    DiscoveryConfig             `mapstructure:"discovery"`
}

// ConfigCenterConfig 配置中心配置
type ConfigCenterConfig struct {
	Type string            `mapstructure:"type"` // "etcd" / "nacos" / "consul"
	Etcd *EtcdSourceConfig `mapstructure:"etcd,omitempty"`
}

// EtcdSourceConfig etcd配置中心连接配置
type EtcdSourceConfig struct {
	Endpoints []string      `mapstructure:"endpoints"`
	Key       string        `mapstructure:"key"` // 存储完整配置的 key，如 "/gateway/config"
	Timeout   time.Duration `mapstructure:"timeout"`
	Username  string        `mapstructure:"username"`
	Password  string        `mapstructure:"password"`
}

// ServerConfig HTTP服务器配置
type ServerConfig struct {
	Listen             string        `mapstructure:"listen"`
	MaxRequestBodySize int64         `mapstructure:"max_request_body_size"`
	ShutdownTimeout    time.Duration `mapstructure:"shutdown_timeout"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string        `mapstructure:"level"`
	Output string        `mapstructure:"output"`
	File   LogFileConfig `mapstructure:"file"`
}

// LogFileConfig 日志文件配置
type LogFileConfig struct {
	Path       string `mapstructure:"path"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

// TelemetryConfig 链路追踪配置
type TelemetryConfig struct {
	Enabled     bool           `mapstructure:"enabled"`
	ServiceName string         `mapstructure:"service_name"`
	Exporter    ExporterConfig `mapstructure:"exporter"`
	Sampler     SamplerConfig  `mapstructure:"sampler"`
}

// ExporterConfig 追踪导出器配置
type ExporterConfig struct {
	Type     string `mapstructure:"type"`
	Protocol string `mapstructure:"protocol"`
	Endpoint string `mapstructure:"endpoint"`
}

// SamplerConfig 采样器配置
type SamplerConfig struct {
	Type  string  `mapstructure:"type"`
	Ratio float64 `mapstructure:"ratio"`
}

// MetricsConfig 指标采集配置（OTLP 协议导出）
type MetricsConfig struct {
	Enabled  bool           `mapstructure:"enabled"`
	Exporter ExporterConfig `mapstructure:"exporter"`
}

// HealthConfig 健康检查配置
type HealthConfig struct {
	Path   string              `mapstructure:"path"`
	Checks []HealthCheckConfig `mapstructure:"check"`
}

// HealthCheckConfig 单个健康检查项配置
type HealthCheckConfig struct {
	Name string `mapstructure:"name"`
}

// CORSConfig 跨域资源共享配置
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	AllowedMethods []string `mapstructure:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
	MaxAge         int      `mapstructure:"max_age"`
}

// IPFilterConfig IP过滤配置
type IPFilterConfig struct {
	Mode string   `mapstructure:"mode"`
	List []string `mapstructure:"list"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Storage   string      `mapstructure:"storage"`
	Redis     RedisConfig `mapstructure:"redis"`
	Algorithm string      `mapstructure:"algorithm"`
}

// RedisConfig Redis连接配置
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// RequestSignConfig 请求签名配置
type RequestSignConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Algorithm       string `mapstructure:"algorithm"`
	SignHeader      string `mapstructure:"sign_header"`      // 签名请求头名称，默认 "X-Signature"
	TimestampHeader string `mapstructure:"timestamp_header"` // 时间戳请求头名称，默认 "X-Timestamp"
	Expire          int    `mapstructure:"expire"`           // 签名有效期（秒）
	Secret          string `mapstructure:"secret"`           // HMAC 签名密钥
}

// AuthSchemeConfig 认证方案配置
type AuthSchemeConfig struct {
	Type   string        `mapstructure:"type"`
	JWT    *JWTConfig    `mapstructure:"jwt,omitempty"`
	APIKey *APIKeyConfig `mapstructure:"api_key,omitempty"`
	OAuth2 *OAuth2Config `mapstructure:"oauth2,omitempty"`
}

// JWTConfig JWT认证配置
type JWTConfig struct {
	Secret          string            `mapstructure:"secret"`
	Algorithms      []string          `mapstructure:"algorithms"`
	Header          string            `mapstructure:"header"`
	ClaimsToHeaders map[string]string `mapstructure:"claims_to_headers"`
}

// APIKeyConfig API Key认证配置
type APIKeyConfig struct {
	Header string `mapstructure:"header"`
	Query  string `mapstructure:"query"`
}

// OAuth2Config OAuth2认证配置
type OAuth2Config struct {
	IntrospectEndpoint string            `mapstructure:"introspect_endpoint"`
	ClientID           string            `mapstructure:"client_id"`
	ClientSecret       string            `mapstructure:"client_secret"`
	ClaimsToHeaders    map[string]string `mapstructure:"claims_to_headers"`
}

// RouteConfig 路由配置
type RouteConfig struct {
	Name               string                `mapstructure:"name"`
	Prefix             string                `mapstructure:"prefix"`
	StripPrefix        bool                  `mapstructure:"strip_prefix"`
	Service            string                `mapstructure:"service"`
	Type               string                `mapstructure:"type"`
	Timeout            time.Duration         `mapstructure:"timeout"`
	Retry              RetryConfig           `mapstructure:"retry"`
	LoadBalancer       string                `mapstructure:"load_balancer"`
	Middleware         RouteMiddlewareConfig `mapstructure:"middleware"`
	Canary             []CanaryRuleConfig    `mapstructure:"canary"`
	WebSocket          *WebSocketConfig      `mapstructure:"websocket,omitempty"`
	SSE                *SSEConfig            `mapstructure:"sse,omitempty"`
	MaxRequestBodySize *int64                `mapstructure:"max_request_body_size,omitempty"`
}

// RetryConfig 重试配置
type RetryConfig struct {
	Count      int      `mapstructure:"count"`
	Conditions []string `mapstructure:"conditions"`
}

// RouteMiddlewareConfig 路由中间件配置
type RouteMiddlewareConfig struct {
	Auth        *RouteAuthConfig        `mapstructure:"auth,omitempty"`
	RateLimit   *RouteRateLimitConfig   `mapstructure:"rate_limit,omitempty"`
	RequestSign *RouteRequestSignConfig `mapstructure:"request_sign,omitempty"`
	IPFilter    *IPFilterConfig         `mapstructure:"ip_filter,omitempty"`
	Rewrite     *RouteRewriteConfig     `mapstructure:"rewrite,omitempty"`
}

// RouteAuthConfig 路由认证中间件配置
type RouteAuthConfig struct {
	Scheme string `mapstructure:"scheme"`
}

// RouteRateLimitConfig 路由限流中间件配置
type RouteRateLimitConfig struct {
	Enabled bool `mapstructure:"enabled"`
	QPS     int  `mapstructure:"qps"`
}

// RouteRequestSignConfig 路由请求签名中间件配置
type RouteRequestSignConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// RouteRewriteConfig 路由重写中间件配置
type RouteRewriteConfig struct {
	AddHeaders    map[string]string `mapstructure:"add_headers"`    // 需要注入到请求的请求头
	RemoveHeaders []string          `mapstructure:"remove_headers"` // 需要从请求中移除的请求头
}

// CanaryRuleConfig 灰度发布规则配置
type CanaryRuleConfig struct {
	Weight  int          `mapstructure:"weight"`
	Service string       `mapstructure:"service"`
	Match   *CanaryMatch `mapstructure:"match,omitempty"`
}

// CanaryMatch 灰度匹配条件
type CanaryMatch struct {
	Headers map[string]string `mapstructure:"headers"`
}

// WebSocketConfig WebSocket协议配置
type WebSocketConfig struct {
	Heartbeat      time.Duration `mapstructure:"heartbeat"`
	MaxConnections int           `mapstructure:"max_connections"`
}

// SSEConfig Server-Sent Events配置
type SSEConfig struct {
	Retry     int           `mapstructure:"retry"`
	Keepalive time.Duration `mapstructure:"keepalive"`
}

// DiscoveryConfig 服务发现配置
type DiscoveryConfig struct {
	Type   string                 `mapstructure:"type"`
	Static *StaticDiscoveryConfig `mapstructure:"static,omitempty"`
	Etcd   *EtcdConfig            `mapstructure:"etcd,omitempty"`
}

// StaticDiscoveryConfig 静态服务发现配置
type StaticDiscoveryConfig struct {
	Services map[string][]StaticInstanceConfig `mapstructure:"services"`
}

// StaticInstanceConfig 静态服务实例配置
type StaticInstanceConfig struct {
	Host     string            `mapstructure:"host"`
	Port     int               `mapstructure:"port"`
	Weight   int               `mapstructure:"weight"`
	Metadata map[string]string `mapstructure:"metadata,omitempty"`
}

// EtcdConfig etcd服务发现配置
type EtcdConfig struct {
	Endpoints []string      `mapstructure:"endpoints"`
	Timeout   time.Duration `mapstructure:"timeout"`
}
