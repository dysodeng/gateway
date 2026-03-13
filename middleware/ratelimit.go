package middleware

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dysodeng/gateway/config"
)

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

// rateLimiter 限流器，维护所有路由+IP 组合的滑动窗口计数器
type rateLimiter struct {
	mu        sync.Mutex
	counters  map[string]*slidingWindowCounter
	qps       int
	routeName string
	storage   string // "local" | "redis"
}

// newRateLimiter 创建新的限流器实例
func newRateLimiter(globalCfg config.RateLimitConfig, routeCfg config.RouteRateLimitConfig, routeName string) *rateLimiter {
	return &rateLimiter{
		counters:  make(map[string]*slidingWindowCounter),
		qps:       routeCfg.QPS,
		routeName: routeName,
		storage:   globalCfg.Storage,
	}
}

// getCounter 获取或创建指定键对应的滑动窗口计数器
func (rl *rateLimiter) getCounter(key string) *slidingWindowCounter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if c, ok := rl.counters[key]; ok {
		return c
	}
	c := &slidingWindowCounter{
		requests: make([]time.Time, 0, rl.qps),
		qps:      rl.qps,
	}
	rl.counters[key] = c
	return c
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

		var allowed bool
		switch rl.storage {
		case "redis":
			// TODO: 实现基于 Redis 的分布式滑动窗口限流
			allowed = true
		default:
			// 本地内存滑动窗口限流
			counter := rl.getCounter(key)
			allowed = counter.allow()
		}

		if !allowed {
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
	rl := newRateLimiter(globalCfg, routeCfg, routeName)
	return rl.middleware
}
