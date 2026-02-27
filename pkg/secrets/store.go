// Copyright 2026 fanjia1024
// Secret management abstraction

package secrets

import (
	"context"
	"fmt"
)

// Store Secret 存储接口
type Store interface {
	// Get 获取 secret 值
	Get(ctx context.Context, key string) (string, error)

	// Set 设置 secret 值
	Set(ctx context.Context, key string, value string) error

	// Delete 删除 secret
	Delete(ctx context.Context, key string) error

	// List 列出所有 secret keys
	List(ctx context.Context, prefix string) ([]string, error)
}

// Config Secret Store 配置
type Config struct {
	Provider string            `yaml:"provider"` // vault | k8s | env | memory
	Config   map[string]string `yaml:"config"`   // Provider-specific config
}

// NewStore 创建 Secret Store
func NewStore(config Config) (Store, error) {
	switch config.Provider {
	case "memory":
		return NewMemoryStore(), nil
	case "env":
		return NewEnvStore(), nil
	case "vault":
		return NewVaultStore(VaultConfig{
			Address:    getConfigString(config.Config, "address", "http://localhost:8200"),
			Token:      getConfigString(config.Config, "token", ""),
			PathPrefix: getConfigString(config.Config, "path_prefix", "secret"),
		})
	case "k8s":
		return NewK8sStore(K8sConfig{
			ServiceAccountPath: getConfigString(config.Config, "service_account_path", ""),
			Namespace:          getConfigString(config.Config, "namespace", "default"),
			SecretsPath:        getConfigString(config.Config, "secrets_path", "/etc/secrets"),
		})
	default:
		return nil, fmt.Errorf("unsupported secret provider: %q", config.Provider)
	}
}

// getConfigString 从 config map 中获取字符串值
func getConfigString(config map[string]string, key, defaultValue string) string {
	if val, ok := config[key]; ok && val != "" {
		return val
	}
	return defaultValue
}
