package cache

import (
	"fmt"

	"rag-platform/pkg/config"
)

// NewCache 根据配置创建缓存（设计 struct.md 3.6 cache.go 统一入口）
func NewCache(cfg config.CacheConfig) (Store, error) {
	switch cfg.Type {
	case "", "memory":
		return NewMemoryStore(), nil
	default:
		return nil, fmt.Errorf("不支持的缓存类型: %s", cfg.Type)
	}
}
