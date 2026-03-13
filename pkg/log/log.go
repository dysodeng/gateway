// Package log 提供结构化日志初始化功能，基于标准库 log/slog 实现。
package log

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/dysodeng/gateway/config"
)

// InitLogger 根据配置初始化全局结构化日志记录器。
// 支持标准输出和文件两种输出目标，日志格式固定为 JSON。
func InitLogger(cfg config.LogConfig) error {
	level := parseLevel(cfg.Level)

	// 根据 output 配置选择输出目标
	var handler slog.Handler
	switch strings.ToLower(cfg.Output) {
	case "file":
		// 文件输出：以追加模式打开日志文件
		// TODO: 支持按 max_size/max_backups/max_age 进行日志轮转
		f, err := openLogFile(cfg.File.Path)
		if err != nil {
			return fmt.Errorf("打开日志文件失败: %w", err)
		}
		handler = slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	default:
		// 默认输出到标准输出（stdout）
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	// 设置全局默认日志记录器
	slog.SetDefault(slog.New(handler))
	return nil
}

// parseLevel 将日志级别字符串转换为 slog.Level。
// 支持大小写不敏感的 "debug"、"info"、"warn"、"error"，
// 无法识别的字符串统一降级为 slog.LevelInfo。
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		// 未知级别降级为 Info，避免丢失重要日志
		return slog.LevelInfo
	}
}

// openLogFile 以追加模式打开（或创建）指定路径的日志文件。
// 若文件所在目录不存在则自动创建。
func openLogFile(path string) (*os.File, error) {
	if path == "" {
		return nil, fmt.Errorf("日志文件路径不能为空")
	}

	// 自动创建所需目录
	dir := dirOf(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建日志目录失败: %w", err)
		}
	}

	// 以追加模式打开文件，文件不存在时自动创建
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// dirOf 返回文件路径的目录部分。
// 避免引入 path/filepath 以外的依赖，仅做简单字符串截取。
func dirOf(path string) string {
	// 从右向左查找最后一个路径分隔符
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	// 没有目录分隔符，说明是纯文件名，目录为当前目录
	return ""
}
