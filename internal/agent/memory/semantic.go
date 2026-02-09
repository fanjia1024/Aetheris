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

package memory

import (
	"context"
	"time"
)

// SemanticRetriever 语义检索接口（可由 RAG/向量库实现）
type SemanticRetriever interface {
	Search(ctx context.Context, query string, topK int) ([]struct {
		Content  string
		Metadata map[string]any
	}, error)
}

// Semantic 知识记忆：包装 RAG/向量库，实现 Memory 接口
type Semantic struct {
	retriever SemanticRetriever
	topK      int
}

// NewSemantic 创建 Semantic Memory
func NewSemantic(retriever SemanticRetriever, topK int) *Semantic {
	if topK <= 0 {
		topK = 10
	}
	return &Semantic{retriever: retriever, topK: topK}
}

// Recall 从向量库检索
func (s *Semantic) Recall(ctx context.Context, query string) ([]MemoryItem, error) {
	if s.retriever == nil {
		return nil, nil
	}
	results, err := s.retriever.Search(ctx, query, s.topK)
	if err != nil || len(results) == 0 {
		return nil, err
	}
	items := make([]MemoryItem, 0, len(results))
	for _, r := range results {
		items = append(items, MemoryItem{
			Type:     "semantic",
			Content:  r.Content,
			Metadata: r.Metadata,
			At:       time.Now(),
		})
	}
	return items, nil
}

// Store 语义记忆通常由 RAG ingest 写入，此处可空实现或转发到 ingest
func (s *Semantic) Store(ctx context.Context, item MemoryItem) error {
	// 若需支持从 Agent 直接写入知识库，可在此调用 vector store 的 upsert
	return nil
}
