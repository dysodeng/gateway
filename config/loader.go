package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Load 从指定路径加载配置文件并应用默认值
// 两阶段加载：
//  1. 读本地 YAML 获取引导配置（含 config_center 连接信息）
//  2. 如果配置了 config_center，从远程配置源拉取完整配置覆盖
func Load(path string) (*Config, error) {
	// 加载 .env 文件中的环境变量（文件不存在时忽略）
	_ = godotenv.Load()

	// 阶段1：读取本地引导配置
	bootstrap := viper.New()
	bootstrap.SetConfigFile(path)
	configureEnvOverrides(bootstrap)
	setDefaults(bootstrap)

	if err := bootstrap.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析引导配置，检查是否配置了配置中心
	bootstrapCfg := &Config{}
	if err := bootstrap.Unmarshal(bootstrapCfg); err != nil {
		return nil, fmt.Errorf("解析引导配置失败: %w", err)
	}

	// 阶段2：如果配置了配置中心，尝试从远程拉取完整配置
	if source := createSource(bootstrapCfg.ConfigCenter); source != nil {
		cfg, err := loadFromSource(source)
		if err != nil {
			slog.Warn("从配置中心加载失败，回退使用本地配置",
				"source", source.Type(),
				"error", err,
			)
		} else {
			applyRouteDefaults(cfg)
			return cfg, nil
		}
	}

	// 回退：使用本地配置
	applyRouteDefaults(bootstrapCfg)
	return bootstrapCfg, nil
}

// configureEnvOverrides 配置环境变量覆盖
func configureEnvOverrides(v *viper.Viper) {
	v.SetEnvPrefix("GATEWAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
}

// setDefaults 设置配置默认值
func setDefaults(v *viper.Viper) {
	v.SetDefault("server.listen", ":8080")
	v.SetDefault("server.shutdown_timeout", 30*time.Second)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.output", "stdout")
	v.SetDefault("health.path", "/health")
	v.SetDefault("rate_limit.storage", "local")
	v.SetDefault("rate_limit.algorithm", "sliding_window")
}

// createSource 根据配置中心配置创建对应的 Source
func createSource(cc *ConfigCenterConfig) Source {
	if cc == nil || cc.Type == "" {
		return nil
	}
	switch cc.Type {
	case "etcd":
		if cc.Etcd == nil {
			return nil
		}
		return NewEtcdSource(cc.Etcd)
	default:
		slog.Warn("不支持的配置中心类型", "type", cc.Type)
		return nil
	}
}

// loadFromSource 从配置源加载完整配置
func loadFromSource(source Source) (*Config, error) {
	data, err := source.Load()
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigType("yaml")
	setDefaults(v)

	if err := v.ReadConfig(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("解析远程配置失败: %w", err)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("反序列化远程配置失败: %w", err)
	}
	return cfg, nil
}

// applyRouteDefaults 为路由设置默认值
func applyRouteDefaults(cfg *Config) {
	for i := range cfg.Routes {
		if cfg.Routes[i].Timeout == 0 {
			cfg.Routes[i].Timeout = 30 * time.Second
		}
		if cfg.Routes[i].LoadBalancer == "" {
			cfg.Routes[i].LoadBalancer = "round_robin"
		}
	}
}
