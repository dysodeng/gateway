package proxy

import (
	"net"
	"net/url"
	"strconv"
	"testing"
)

// parseHostPort 从 URL 字符串中解析出 host 和 port（测试辅助函数）
func parseHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("解析 URL 失败: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("分离 host:port 失败: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("解析端口号失败: %v", err)
	}
	return host, port
}
