package metadata

import (
	"context"
)

// Store 元数据存储接口
type Store interface {
	// Create 创建文档元数据
	Create(ctx context.Context, doc *Document) error
	// Get 根据 ID 获取文档元数据
	Get(ctx context.Context, id string) (*Document, error)
	// Update 更新文档元数据
	Update(ctx context.Context, doc *Document) error
	// Delete 根据 ID 删除文档元数据
	Delete(ctx context.Context, id string) error
	// List 列出文档元数据
	List(ctx context.Context, filter *Filter, pagination *Pagination) ([]*Document, error)
	// Count 统计文档数量
	Count(ctx context.Context, filter *Filter) (int64, error)
	// Close 关闭存储连接
	Close() error
}

// Document 文档元数据
type Document struct {
	ID          string            `json:"id"`          // 文档唯一标识
	Name        string            `json:"name"`        // 文档名称
	Type        string            `json:"type"`        // 文档类型
	Size        int64             `json:"size"`        // 文档大小
	Path        string            `json:"path"`        // 文档路径
	Status      string            `json:"status"`      // 文档状态
	Chunks      int               `json:"chunks"`      // 文档切片数量
	VectorCount int               `json:"vector_count"` // 向量数量
	Metadata    map[string]string `json:"metadata"`    // 额外元数据
	CreatedAt   int64             `json:"created_at"`  // 创建时间
	UpdatedAt   int64             `json:"updated_at"`  // 更新时间
}

// Filter 过滤条件
type Filter struct {
	IDs     []string            `json:"ids"`     // 文档 ID 列表
	Types   []string            `json:"types"`   // 文档类型列表
	Status  []string            `json:"status"`  // 文档状态列表
	Metadata map[string]string  `json:"metadata"` // 元数据过滤
	Search  string              `json:"search"`  // 搜索关键词
}

// Pagination 分页参数
type Pagination struct {
	Offset int `json:"offset"` // 偏移量
	Limit  int `json:"limit"`  // 限制数量
}
