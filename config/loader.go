package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Load 从指定路径加载YAML配置文件并应用默认值
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	applyDefaults(cfg)
	return cfg, nil
}

// applyDefaults 为未设置的配置项填充默认值
func applyDefaults(cfg *Config) {
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":8080"
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = 30 * time.Second
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Output == "" {
		cfg.Log.Output = "stdout"
	}
	if cfg.Health.Path == "" {
		cfg.Health.Path = "/health"
	}
	if cfg.Metrics.Path == "" {
		cfg.Metrics.Path = "/metrics"
	}
	if cfg.RateLimit.Storage == "" {
		cfg.RateLimit.Storage = "local"
	}
	if cfg.RateLimit.Algorithm == "" {
		cfg.RateLimit.Algorithm = "sliding_window"
	}
	for i := range cfg.Routes {
		if cfg.Routes[i].Timeout == 0 {
			cfg.Routes[i].Timeout = 30 * time.Second
		}
		if cfg.Routes[i].LoadBalancer == "" {
			cfg.Routes[i].LoadBalancer = "round_robin"
		}
	}
}
