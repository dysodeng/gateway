package router

import (
	"math/rand/v2"
	"net/http"

	"github.com/dysodeng/gateway/config"
)

// ResolveCanary 根据灰度规则决定目标服务名。
// 决策树：
// 1. 遍历 canary 规则，检查 match.headers 条件
// 2. Header 匹配且 weight > 0：按权重概率决定是否走灰度服务
// 3. Header 匹配且 weight == 0：100% 走灰度服务（纯 Header 匹配模式）
// 4. 无 Header 条件但 weight > 0：按权重概率对所有流量随机分流
// 5. 所有规则都不命中：返回默认服务
func ResolveCanary(rules []config.CanaryRuleConfig, defaultService string, req *http.Request) string {
	for _, rule := range rules {
		hasHeaderMatch := rule.Match != nil && len(rule.Match.Headers) > 0

		if hasHeaderMatch {
			if !matchHeaders(rule.Match.Headers, req) {
				continue
			}
			// Header 已匹配
			if rule.Weight == 0 {
				return rule.Service
			}
			if rand.IntN(100) < rule.Weight {
				return rule.Service
			}
			continue
		}

		// 无 Header 条件 - 仅按权重分流
		if rule.Weight > 0 {
			if rand.IntN(100) < rule.Weight {
				return rule.Service
			}
		}
	}

	return defaultService
}

// matchHeaders 检查请求头是否匹配所有期望的键值对
func matchHeaders(expected map[string]string, req *http.Request) bool {
	for key, val := range expected {
		if req.Header.Get(key) != val {
			return false
		}
	}
	return true
}
