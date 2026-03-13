package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dysodeng/gateway/config"
)

// limiter 限流器接口，不同算法实现此接口
type limiter interface {
	allow() bool
}

// slidingWindowCounter 滑动窗口计数器，用于实现本地内存限流
type slidingWindowCounter struct {
	mu       sync.Mutex
	requests []time.Time // 记录每次请求的时间戳
	qps      int         // 每秒最大请求数
}

// allow 判断当前请求是否允许通过，返回 true 表示允许，false 表示超限
func (c *slidingWindowCounter) allow() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-time.Second)

	// 移除窗口外的旧记录
	valid := c.requests[:0]
	for _, t := range c.requests {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}
	c.requests = valid

	// 判断窗口内请求数是否已达上限
	if len(c.requests) >= c.qps {
		return false
	}

	// 记录本次请求时间戳
	c.requests = append(c.requests, now)
	return true
}

// tokenBucket 令牌桶限流器
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64   // 当前可用令牌数
	maxTokens  float64   // 桶容量（等于 QPS）
	refillRate float64   // 每秒补充的令牌数（等于 QPS）
	lastRefill time.Time // 上次补充令牌的时间
}

// allow 判断是否允许通过，消耗一个令牌
func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	// 根据经过时间补充令牌
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	// 尝试消耗一个令牌
	if tb.tokens < 1 {
		return false
	}
	tb.tokens--
	return true
}

// rateLimiter 限流器，维护所有路由+IP 组合的限流实例
type rateLimiter struct {
	mu        sync.Mutex
	limiters  map[string]limiter
	qps       int
	routeName string
	algorithm string // "sliding_window" | "token_bucket"
}

// newRateLimiter 创建新的限流器实例
func newRateLimiter(globalCfg config.RateLimitConfig, routeCfg config.RouteRateLimitConfig, routeName string) *rateLimiter {
	algorithm := globalCfg.Algorithm
	if algorithm == "" {
		algorithm = "sliding_window"
	}
	return &rateLimiter{
		limiters:  make(map[string]limiter),
		qps:       routeCfg.QPS,
		routeName: routeName,
		algorithm: algorithm,
	}
}

// getLimiter 获取或创建指定键对应的限流器
func (rl *rateLimiter) getLimiter(key string) limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if l, ok := rl.limiters[key]; ok {
		return l
	}

	var l limiter
	switch rl.algorithm {
	case "token_bucket":
		l = &tokenBucket{
			tokens:     float64(rl.qps),
			maxTokens:  float64(rl.qps),
			refillRate: float64(rl.qps),
			lastRefill: time.Now(),
		}
	default:
		// 默认使用滑动窗口
		l = &slidingWindowCounter{
			requests: make([]time.Time, 0, rl.qps),
			qps:      rl.qps,
		}
	}
	rl.limiters[key] = l
	return l
}

// buildKey 根据路由名称和客户端 IP 构造限流键
func (rl *rateLimiter) buildKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return fmt.Sprintf("%s:%s", rl.routeName, host)
}

// middleware 返回限流中间件处理函数
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := rl.buildKey(r)
		l := rl.getLimiter(key)

		if !l.allow() {
			// 超限时返回 429，并设置 Retry-After 头（1 秒后重试）
			w.Header().Set("Retry-After", "1")
			http.Error(w, "请求过于频繁，请稍后重试", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// NewRateLimit 创建路由级限流中间件
// globalCfg 为全局限流配置，routeCfg 为路由级限流配置，routeName 为路由名称（用于限流键隔离）
func NewRateLimit(globalCfg config.RateLimitConfig, routeCfg config.RouteRateLimitConfig, routeName string) Middleware {
	// 限流未启用时直接透传
	if !routeCfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	// Redis 存储模式尚未实现，启动时报错避免静默放行所有请求
	if globalCfg.Storage == "redis" {
		slog.Error("Redis 限流存储尚未实现，回退到本地内存模式", "route", routeName)
		globalCfg.Storage = "local"
	}

	rl := newRateLimiter(globalCfg, routeCfg, routeName)
	return rl.middleware
}
