package object

import (
	"fmt"

	"rag-platform/pkg/config"
)

// NewStore 根据配置创建对象存储（设计 struct.md 3.6；当前仅支持 memory）
func NewStore(cfg config.ObjectConfig) (Store, error) {
	switch cfg.Type {
	case "", "memory":
		return NewMemoryStore(), nil
	default:
		return nil, fmt.Errorf("不支持的对象存储类型: %s", cfg.Type)
	}
}
