// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"

	"rag-platform/internal/storage/metadata"
)

// DocumentInfo 文档信息 DTO，供 API 层使用，不依赖 storage 具体类型
type DocumentInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Size        int64             `json:"size"`
	Path        string            `json:"path"`
	Status      string            `json:"status"`
	Chunks      int               `json:"chunks"`
	VectorCount int               `json:"vector_count"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
}

// DocumentService 文档门面：API 层仅依赖此接口，不直接调用 storage
type DocumentService interface {
	ListDocuments(ctx context.Context) ([]*DocumentInfo, error)
	GetDocument(ctx context.Context, id string) (*DocumentInfo, error)
	DeleteDocument(ctx context.Context, id string) error
}

// documentService 使用 metadata.Store 实现 DocumentService
type documentService struct {
	store metadata.Store
}

// NewDocumentService 创建文档门面（由 bootstrap 或 app 装配时调用）
func NewDocumentService(store metadata.Store) DocumentService {
	return &documentService{store: store}
}

func (s *documentService) ListDocuments(ctx context.Context) ([]*DocumentInfo, error) {
	docs, err := s.store.List(ctx, nil, &metadata.Pagination{Offset: 0, Limit: 1000})
	if err != nil {
		return nil, err
	}
	out := make([]*DocumentInfo, len(docs))
	for i, d := range docs {
		out[i] = docToInfo(d)
	}
	return out, nil
}

func (s *documentService) GetDocument(ctx context.Context, id string) (*DocumentInfo, error) {
	d, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return docToInfo(d), nil
}

func (s *documentService) DeleteDocument(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

func docToInfo(d *metadata.Document) *DocumentInfo {
	if d == nil {
		return nil
	}
	return &DocumentInfo{
		ID:          d.ID,
		Name:        d.Name,
		Type:        d.Type,
		Size:        d.Size,
		Path:        d.Path,
		Status:      d.Status,
		Chunks:      d.Chunks,
		VectorCount: d.VectorCount,
		Metadata:    d.Metadata,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}
