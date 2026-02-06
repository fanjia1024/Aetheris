package vector

import (
	"context"
)

// Store 向量存储接口
type Store interface {
	// Create 创建向量索引
	Create(ctx context.Context, index *Index) error
	// Add 添加向量
	Add(ctx context.Context, indexName string, vectors []*Vector) error
	// Search 搜索向量
	Search(ctx context.Context, indexName string, query []float64, options *SearchOptions) ([]*SearchResult, error)
	// Get 根据 ID 获取向量
	Get(ctx context.Context, indexName string, id string) (*Vector, error)
	// Delete 删除向量
	Delete(ctx context.Context, indexName string, id string) error
	// DeleteIndex 删除索引
	DeleteIndex(ctx context.Context, indexName string) error
	// ListIndexes 列出所有索引
	ListIndexes(ctx context.Context) ([]string, error)
	// Close 关闭存储连接
	Close() error
}

// Index 向量索引
type Index struct {
	Name      string `json:"name"`      // 索引名称
	Dimension int    `json:"dimension"` // 向量维度
	Distance  string `json:"distance"`  // 距离度量方式
	Metadata  map[string]string `json:"metadata"` // 索引元数据
}

// Vector 向量数据
type Vector struct {
	ID       string            `json:"id"`       // 向量唯一标识
	Values   []float64         `json:"values"`   // 向量值
	Metadata map[string]string `json:"metadata"` // 向量元数据
}

// SearchOptions 搜索选项
type SearchOptions struct {
	TopK         int               `json:"top_k"`         // 返回前 K 个结果
	Filter       map[string]string `json:"filter"`        // 元数据过滤
	Threshold    float64           `json:"threshold"`     // 相似度阈值
	IncludeVectors bool             `json:"include_vectors"` // 是否包含向量值
}

// SearchResult 搜索结果
type SearchResult struct {
	ID       string            `json:"id"`       // 向量唯一标识
	Score    float64           `json:"score"`     // 相似度得分
	Metadata map[string]string `json:"metadata"` // 向量元数据
	Values   []float64         `json:"values,omitempty"` // 向量值（可选）
}
