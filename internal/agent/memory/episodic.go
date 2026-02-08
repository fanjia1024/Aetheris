package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Episodic 经历记忆：调用过什么工具、失败记录等
type Episodic struct {
	mu    sync.RWMutex
	items []MemoryItem
	max   int
}

// NewEpisodic 创建经历记忆，maxItems 为最多保留条数（0 表示默认 1000）
func NewEpisodic(maxItems int) *Episodic {
	if maxItems <= 0 {
		maxItems = 1000
	}
	return &Episodic{items: nil, max: maxItems}
}

// Recall 按时间倒序返回与 query 相关的经历（简单实现：返回最近 N 条；可扩展为按类型/关键词过滤）
func (e *Episodic) Recall(ctx context.Context, query string) ([]MemoryItem, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.items)
	if n == 0 {
		return nil, nil
	}
	limit := 50
	if limit > n {
		limit = n
	}
	out := make([]MemoryItem, limit)
	for i := 0; i < limit; i++ {
		out[i] = e.items[n-1-i]
	}
	return out, nil
}

// Store 追加一条经历
func (e *Episodic) Store(ctx context.Context, item MemoryItem) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if item.Type == "" {
		item.Type = "episodic"
	}
	if item.At.IsZero() {
		item.At = time.Now()
	}
	if item.Metadata == nil {
		item.Metadata = make(map[string]any)
	}
	if _, ok := item.Metadata["id"]; !ok {
		item.Metadata["id"] = uuid.New().String()
	}
	e.items = append(e.items, item)
	if len(e.items) > e.max {
		e.items = e.items[len(e.items)-e.max:]
	}
	return nil
}
