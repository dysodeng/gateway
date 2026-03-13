package proxy

import (
	"errors"
	"sync"
	"time"
)

// CircuitState 熔断器状态
type CircuitState int

const (
	StateClosed   CircuitState = iota // 关闭（正常通过）
	StateOpen                         // 打开（拒绝请求）
	StateHalfOpen                     // 半开（允许探测请求）
)

// ErrCircuitOpen 熔断器处于打开状态时返回的错误
var ErrCircuitOpen = errors.New("熔断器已打开")

// CircuitBreaker 按服务实例维度的熔断器
type CircuitBreaker struct {
	mu          sync.Mutex
	state       CircuitState
	failures    int
	threshold   int           // 触发熔断的失败次数阈值
	timeout     time.Duration // 从打开到半开的等待时间
	lastFailure time.Time
	probing     bool // 半开状态下是否已有探测请求在进行
}

// NewCircuitBreaker 创建熔断器实例
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		timeout:   timeout,
	}
}

// Allow 检查是否允许请求通过
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
			// 半开状态下只允许一个探测请求
			return ErrCircuitOpen
		}
		cb.probing = true
		return nil
	}
	return nil
}

// RecordSuccess 记录请求成功，重置熔断器
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.probing = false
	cb.state = StateClosed
}

// RecordFailure 记录请求失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	cb.probing = false
	if cb.failures >= cb.threshold {
		cb.state = StateOpen
	}
}

// State 获取当前熔断器状态
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
