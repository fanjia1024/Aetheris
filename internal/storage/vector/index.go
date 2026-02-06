package vector

import (
	"context"
	"fmt"
)

// IndexInfo 索引元信息（与 Index 配合，便于查询）
type IndexInfo struct {
	Name      string
	Dimension int
	Distance  string
}

// EnsureIndex 若索引不存在则创建，存在则跳过（与 Store 配合的辅助）
func EnsureIndex(ctx context.Context, s Store, name string, dimension int, distance string) error {
	if distance == "" {
		distance = "cosine"
	}
	list, err := s.ListIndexes(ctx)
	if err != nil {
		return fmt.Errorf("列出索引失败: %w", err)
	}
	for _, n := range list {
		if n == name {
			return nil
		}
	}
	return s.Create(ctx, &Index{
		Name:      name,
		Dimension: dimension,
		Distance:  distance,
	})
}
