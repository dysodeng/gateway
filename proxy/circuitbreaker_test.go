package proxy

import (
	"testing"
	"time"
)

// TestCircuitBreaker_ClosedState 测试熔断器初始关闭状态下请求正常通过
func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	// 初始状态应为关闭
	if cb.State() != StateClosed {
		t.Fatalf("期望初始状态为 StateClosed，实际为 %v", cb.State())
	}

	// 关闭状态下请求应允许通过
	if err := cb.Allow(); err != nil {
		t.Fatalf("期望 Allow() 返回 nil，实际得到 %v", err)
	}

	// 记录几次失败但未达到阈值
	cb.RecordFailure()
	cb.RecordFailure()

	// 状态仍应为关闭
	if cb.State() != StateClosed {
		t.Errorf("期望状态为 StateClosed，实际为 %v", cb.State())
	}
}

// TestCircuitBreaker_OpensAfterThreshold 测试达到失败阈值后熔断器进入打开状态
func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	// 记录失败达到阈值
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// 状态应变为打开
	if cb.State() != StateOpen {
		t.Fatalf("期望状态为 StateOpen，实际为 %v", cb.State())
	}

	// 打开状态下 Allow() 应返回 ErrCircuitOpen
	if err := cb.Allow(); err != ErrCircuitOpen {
		t.Errorf("期望 Allow() 返回 ErrCircuitOpen，实际得到 %v", err)
	}
}

// TestCircuitBreaker_HalfOpenAfterTimeout 测试超时后熔断器从打开转为半开状态
func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	// 使用极短超时（1ms）以便测试快速完成
	cb := NewCircuitBreaker(1, 1*time.Millisecond)

	// 触发熔断
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("期望状态为 StateOpen，实际为 %v", cb.State())
	}

	// 等待超时后状态应转为半开
	time.Sleep(5 * time.Millisecond)

	if err := cb.Allow(); err != nil {
		t.Fatalf("超时后期望 Allow() 返回 nil，实际得到 %v", err)
	}

	if cb.State() != StateHalfOpen {
		t.Errorf("期望状态为 StateHalfOpen，实际为 %v", cb.State())
	}
}

// TestCircuitBreaker_HalfOpenOnlyOneProbe 测试半开状态下只允许一个探测请求
func TestCircuitBreaker_HalfOpenOnlyOneProbe(t *testing.T) {
	cb := NewCircuitBreaker(1, 1*time.Millisecond)

	// 触发熔断并等待超时
	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond)

	// 第一个 Allow() 应成功（探测请求）
	if err := cb.Allow(); err != nil {
		t.Fatalf("第一个 Allow() 期望 nil，实际得到 %v", err)
	}

	// 第二个 Allow() 在探测进行中时应返回 ErrCircuitOpen
	if err := cb.Allow(); err != ErrCircuitOpen {
		t.Errorf("第二个 Allow() 期望 ErrCircuitOpen，实际得到 %v", err)
	}
}

// TestCircuitBreaker_HalfOpenSuccess 测试半开状态探测成功后熔断器恢复关闭
func TestCircuitBreaker_HalfOpenSuccess(t *testing.T) {
	cb := NewCircuitBreaker(1, 1*time.Millisecond)

	// 触发熔断并等待超时
	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond)

	// 允许探测请求通过
	if err := cb.Allow(); err != nil {
		t.Fatalf("Allow() 期望 nil，实际得到 %v", err)
	}

	// 探测成功，熔断器应恢复关闭状态
	cb.RecordSuccess()

	if cb.State() != StateClosed {
		t.Errorf("探测成功后期望状态为 StateClosed，实际为 %v", cb.State())
	}

	// 关闭状态下请求应正常通过
	if err := cb.Allow(); err != nil {
		t.Errorf("恢复后 Allow() 期望 nil，实际得到 %v", err)
	}
}

// TestCircuitBreaker_HalfOpenFailure 测试半开状态探测失败后熔断器重新打开
func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(1, 1*time.Millisecond)

	// 触发熔断并等待超时
	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond)

	// 允许探测请求通过
	if err := cb.Allow(); err != nil {
		t.Fatalf("Allow() 期望 nil，实际得到 %v", err)
	}

	// 探测失败，熔断器应重新打开
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("探测失败后期望状态为 StateOpen，实际为 %v", cb.State())
	}

	// 打开状态下请求应被拒绝
	if err := cb.Allow(); err != ErrCircuitOpen {
		t.Errorf("重新打开后 Allow() 期望 ErrCircuitOpen，实际得到 %v", err)
	}
}
