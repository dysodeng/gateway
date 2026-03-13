// Package health 提供网关健康检查功能。
// 支持注册多个 Checker，HTTP GET 请求时逐个执行检查并以 JSON 格式返回结果。
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Checker 健康检查器接口。
// 实现该接口的组件可注册到健康检查 Handler，
// 在每次健康检查请求时被调用。
type Checker interface {
	// Name 返回检查器的唯一名称，用于在响应 JSON 的 checks 字段中标识该检查项
	Name() string
	// Check 执行实际的健康检查逻辑，返回 nil 表示健康，返回 error 表示故障
	Check(ctx context.Context) error
}

// response 健康检查响应结构
type response struct {
	// Status 整体状态：healthy 或 unhealthy
	Status string `json:"status"`
	// Checks 各检查项的状态映射：key 为检查器名称，value 为 "up" 或 "down: <错误信息>"
	Checks map[string]string `json:"checks"`
}

// Handler 返回健康检查 HTTP Handler。
// 注册多个 Checker，GET 请求时逐个执行检查，将结果序列化为 JSON 返回。
// 若所有检查器均通过，返回 HTTP 200 和 {"status":"healthy",...}；
// 若有任意检查器失败，返回 HTTP 503 和 {"status":"unhealthy",...}。
func Handler(checkers ...Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		checks := make(map[string]string, len(checkers))
		allHealthy := true

		// 逐个执行健康检查
		for _, checker := range checkers {
			if err := checker.Check(ctx); err != nil {
				// 检查失败，记录错误信息
				checks[checker.Name()] = fmt.Sprintf("down: %s", err.Error())
				allHealthy = false
			} else {
				// 检查通过
				checks[checker.Name()] = "up"
			}
		}

		// 构造响应
		resp := response{
			Checks: checks,
		}
		if allHealthy {
			resp.Status = "healthy"
		} else {
			resp.Status = "unhealthy"
		}

		// 设置响应头并写入状态码
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if !allHealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		// 序列化并写入响应体
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			// 此处无法再修改状态码，仅记录错误（实际项目中可接入日志系统）
			_ = err
		}
	}
}
