package metadata

import (
	"fmt"

	"rag-platform/pkg/config"
)

// NewStore 根据配置创建元数据存储（当前仅支持 memory）
func NewStore(cfg config.MetadataConfig) (Store, error) {
	switch cfg.Type {
	case "", "memory":
		return NewMemoryStore(), nil
	default:
		return nil, fmt.Errorf("不支持的元数据存储类型: %s", cfg.Type)
	}
}
