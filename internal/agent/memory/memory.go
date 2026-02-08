package memory

import (
	"context"
	"time"
)

// MemoryItem 单条记忆：类型、内容、时间等
type MemoryItem struct {
	Type     string         `json:"type"`     // working / episodic / semantic
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
	At       time.Time      `json:"at"`
}

// Memory 统一记忆接口：Recall 与 Store
type Memory interface {
	Recall(ctx context.Context, query string) ([]MemoryItem, error)
	Store(ctx context.Context, item MemoryItem) error
}

// CompositeMemory 组合多类 Memory，Recall 时合并结果，Store 时写入所有
type CompositeMemory struct {
	backends []Memory
}

// NewCompositeMemory 创建组合记忆
func NewCompositeMemory(backends ...Memory) *CompositeMemory {
	return &CompositeMemory{backends: backends}
}

// Recall 依次从各 backend Recall 并合并（去重或按时间排序由实现决定）
func (c *CompositeMemory) Recall(ctx context.Context, query string) ([]MemoryItem, error) {
	var out []MemoryItem
	for _, b := range c.backends {
		items, err := b.Recall(ctx, query)
		if err != nil {
			continue
		}
		out = append(out, items...)
	}
	return out, nil
}

// Store 写入所有 backend
func (c *CompositeMemory) Store(ctx context.Context, item MemoryItem) error {
	for _, b := range c.backends {
		_ = b.Store(ctx, item)
	}
	return nil
}
