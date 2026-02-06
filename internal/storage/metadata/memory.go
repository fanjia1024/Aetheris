package metadata

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStore 内存元数据存储实现
type MemoryStore struct {
	docs map[string]*Document
	mu   sync.RWMutex
}

// NewMemoryStore 创建新的内存元数据存储
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		docs: make(map[string]*Document),
	}
}

// Create 创建文档元数据
func (s *MemoryStore) Create(ctx context.Context, doc *Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.docs[doc.ID]; exists {
		return fmt.Errorf("document with ID %s already exists", doc.ID)
	}

	now := time.Now().Unix()
	doc.CreatedAt = now
	doc.UpdatedAt = now

	s.docs[doc.ID] = doc
	return nil
}

// Get 根据 ID 获取文档元数据
func (s *MemoryStore) Get(ctx context.Context, id string) (*Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, exists := s.docs[id]
	if !exists {
		return nil, fmt.Errorf("document with ID %s not found", id)
	}

	return doc, nil
}

// Update 更新文档元数据
func (s *MemoryStore) Update(ctx context.Context, doc *Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.docs[doc.ID]; !exists {
		return fmt.Errorf("document with ID %s not found", doc.ID)
	}

	doc.UpdatedAt = time.Now().Unix()
	s.docs[doc.ID] = doc
	return nil
}

// Delete 根据 ID 删除文档元数据
func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.docs[id]; !exists {
		return fmt.Errorf("document with ID %s not found", id)
	}

	delete(s.docs, id)
	return nil
}

// List 列出文档元数据
func (s *MemoryStore) List(ctx context.Context, filter *Filter, pagination *Pagination) ([]*Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Document

	// 应用过滤条件
	for _, doc := range s.docs {
		if filter != nil {
			// 过滤 ID
			if len(filter.IDs) > 0 {
				found := false
				for _, id := range filter.IDs {
					if doc.ID == id {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// 过滤类型
			if len(filter.Types) > 0 {
				found := false
				for _, typ := range filter.Types {
					if doc.Type == typ {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// 过滤状态
			if len(filter.Status) > 0 {
				found := false
				for _, status := range filter.Status {
					if doc.Status == status {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// 过滤元数据
			if len(filter.Metadata) > 0 {
				for key, value := range filter.Metadata {
					if doc.Metadata == nil || doc.Metadata[key] != value {
						continue
					}
				}
			}

			// 搜索关键词
			if filter.Search != "" {
				if doc.Name != filter.Search && doc.Path != filter.Search {
					continue
				}
			}
		}

		results = append(results, doc)
	}

	// 应用分页
	if pagination != nil {
		start := pagination.Offset
		end := start + pagination.Limit

		if start >= len(results) {
			return []*Document{}, nil
		}

		if end > len(results) {
			end = len(results)
		}

		results = results[start:end]
	}

	return results, nil
}

// Count 统计文档数量
func (s *MemoryStore) Count(ctx context.Context, filter *Filter) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int64

	// 应用过滤条件
	for _, doc := range s.docs {
		if filter != nil {
			// 过滤 ID
			if len(filter.IDs) > 0 {
				found := false
				for _, id := range filter.IDs {
					if doc.ID == id {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// 过滤类型
			if len(filter.Types) > 0 {
				found := false
				for _, typ := range filter.Types {
					if doc.Type == typ {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// 过滤状态
			if len(filter.Status) > 0 {
				found := false
				for _, status := range filter.Status {
					if doc.Status == status {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// 过滤元数据
			if len(filter.Metadata) > 0 {
				for key, value := range filter.Metadata {
					if doc.Metadata == nil || doc.Metadata[key] != value {
						continue
					}
				}
			}

			// 搜索关键词
			if filter.Search != "" {
				if doc.Name != filter.Search && doc.Path != filter.Search {
					continue
				}
			}
		}

		count++
	}

	return count, nil
}

// Close 关闭存储连接
func (s *MemoryStore) Close() error {
	return nil
}
