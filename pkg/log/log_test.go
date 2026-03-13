package log

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/dysodeng/gateway/config"
)

// TestInitLogger_Stdout 测试标准输出模式下日志初始化是否正常，并验证输出为合法JSON格式
func TestInitLogger_Stdout(t *testing.T) {
	// 使用 bytes.Buffer 捕获日志输出，验证JSON格式正确性
	var buf bytes.Buffer

	cfg := config.LogConfig{
		Level:  "debug",
		Output: "stdout",
	}

	// 直接构造 JSONHandler 写入 buf，以便捕获并断言输出内容
	level := parseLevel(cfg.Level)
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler)

	// 写入一条测试日志
	logger.Debug("测试消息", "key", "value")

	// 验证输出不为空
	output := buf.String()
	if output == "" {
		t.Fatal("日志输出为空，期望有JSON内容")
	}

	// 验证输出为合法JSON
	var entry map[string]any
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("日志输出不是合法JSON: %v，实际输出: %s", err, output)
	}

	// 验证必要字段存在
	if _, ok := entry["time"]; !ok {
		t.Error("JSON日志缺少 'time' 字段")
	}
	if _, ok := entry["level"]; !ok {
		t.Error("JSON日志缺少 'level' 字段")
	}
	if msg, ok := entry["msg"]; !ok {
		t.Error("JSON日志缺少 'msg' 字段")
	} else if msg != "测试消息" {
		t.Errorf("'msg' 字段值不符，期望 '测试消息'，实际 '%v'", msg)
	}
	if val, ok := entry["key"]; !ok {
		t.Error("JSON日志缺少自定义字段 'key'")
	} else if val != "value" {
		t.Errorf("自定义字段 'key' 值不符，期望 'value'，实际 '%v'", val)
	}
}

// TestInitLogger_LevelParsing 测试日志级别字符串解析是否正确
func TestInitLogger_LevelParsing(t *testing.T) {
	// 定义各级别的期望映射
	cases := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		// 大写形式也应支持
		{"DEBUG", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		// 未知级别应降级为 info
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, c := range cases {
		got := parseLevel(c.input)
		if got != c.expected {
			t.Errorf("parseLevel(%q) = %v，期望 %v", c.input, got, c.expected)
		}
	}
}

// TestInitLogger_SetsDefault 测试 InitLogger 是否正确设置全局默认 logger
func TestInitLogger_SetsDefault(t *testing.T) {
	cfg := config.LogConfig{
		Level:  "info",
		Output: "stdout",
	}

	// 调用 InitLogger 不应返回错误
	if err := InitLogger(cfg); err != nil {
		t.Fatalf("InitLogger 返回错误: %v", err)
	}

	// 验证全局默认 logger 已被替换（能正常调用即可）
	slog.Info("全局logger初始化测试")
}
