package loadbalancer

import (
	"errors"
	"hash/fnv"
	"net"
	"net/http"

	"github.com/dysodeng/gateway/discovery"
)

// IPHash 基于客户端 IP 哈希的负载均衡策略
// 相同 IP 始终路由到同一实例，保证会话粘性
type IPHash struct{}

// NewIPHash 创建一个新的 IP 哈希负载均衡器
func NewIPHash() *IPHash {
	return &IPHash{}
}

// Select 根据客户端 IP 的 FNV-32a 哈希值选择实例
func (h *IPHash) Select(instances []discovery.ServiceInstance, req *http.Request) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("没有可用的服务实例")
	}

	// 从 RemoteAddr 提取客户端 IP
	ip := extractIP(req.RemoteAddr)

	// 使用 FNV-32a 计算哈希值
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(ip))
	hash := hasher.Sum32()

	// 对实例数量取模确定目标实例
	idx := int(hash) % len(instances)
	if idx < 0 {
		idx = -idx
	}
	selected := instances[idx]
	return &selected, nil
}

// extractIP 从 addr 字符串中提取 IP 部分
// 支持 "host:port" 和纯 "host" 两种格式
func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// 没有端口号，直接返回原始地址作为 IP
		return addr
	}
	return host
}
