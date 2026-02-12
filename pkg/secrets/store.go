// Copyright 2026 fanjia1024
// Secret management abstraction

package secrets

import (
	"context"
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
	// TODO: Implement Vault and K8s Secrets
	default:
		return NewMemoryStore(), nil
	}
}
