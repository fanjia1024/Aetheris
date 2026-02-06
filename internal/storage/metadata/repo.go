package metadata

import (
	"context"
)

// Repository 封装 Store，提供业务方法，供 app 层或 DocumentService 内部复用（设计 struct.md 3.6）
type Repository struct {
	store Store
}

// NewRepository 从 Store 创建 Repository
func NewRepository(store Store) *Repository {
	return &Repository{store: store}
}

// ListDocuments 列出文档（默认分页）
func (r *Repository) ListDocuments(ctx context.Context, filter *Filter, pagination *Pagination) ([]*Document, error) {
	if pagination == nil {
		pagination = &Pagination{Offset: 0, Limit: 1000}
	}
	return r.store.List(ctx, filter, pagination)
}

// GetDocument 按 ID 获取文档
func (r *Repository) GetDocument(ctx context.Context, id string) (*Document, error) {
	return r.store.Get(ctx, id)
}

// CreateDocument 创建文档
func (r *Repository) CreateDocument(ctx context.Context, doc *Document) error {
	return r.store.Create(ctx, doc)
}

// UpdateDocument 更新文档
func (r *Repository) UpdateDocument(ctx context.Context, doc *Document) error {
	return r.store.Update(ctx, doc)
}

// DeleteDocument 按 ID 删除文档
func (r *Repository) DeleteDocument(ctx context.Context, id string) error {
	return r.store.Delete(ctx, id)
}

// CountDocuments 统计文档数
func (r *Repository) CountDocuments(ctx context.Context, filter *Filter) (int64, error) {
	return r.store.Count(ctx, filter)
}
