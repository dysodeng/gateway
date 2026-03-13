package proxy

import "net/http"

// ShouldRetry 判断是否应该重试请求。
// 非幂等方法（POST/PATCH/DELETE）默认不重试。
func ShouldRetry(method string, statusCode int, err error, conditions []string) bool {
	if !isIdempotent(method) {
		return false
	}
	for _, cond := range conditions {
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

// isIdempotent 判断 HTTP 方法是否为幂等方法
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut:
		return true
	}
	return false
}
