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

package query

import (
	"context"
	"fmt"

	einoretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"rag-platform/internal/storage/vector"
)

// MemoryRetriever 基于 vector.Store 实现的 Eino retriever.Retriever（memory 后端）
type MemoryRetriever struct {
	vectorStore      vector.Store
	defaultIndex     string
	defaultTopK      int
	defaultThreshold float64
}

// MemoryRetrieverConfig MemoryRetriever 构造参数
type MemoryRetrieverConfig struct {
	VectorStore      vector.Store
	DefaultIndex     string
	DefaultTopK      int
	DefaultThreshold float64
}

// NewMemoryRetriever 创建基于 vector.Store 的 Eino Retriever
func NewMemoryRetriever(cfg *MemoryRetrieverConfig) (*MemoryRetriever, error) {
	if cfg == nil || cfg.VectorStore == nil {
		return nil, fmt.Errorf("MemoryRetriever requires VectorStore")
	}
	idx := cfg.DefaultIndex
	if idx == "" {
		idx = "default"
	}
	topK := cfg.DefaultTopK
	if topK <= 0 {
		topK = 10
	}
	thresh := cfg.DefaultThreshold
	if thresh <= 0 {
		thresh = 0.3
	}
	return &MemoryRetriever{
		vectorStore:      cfg.VectorStore,
		defaultIndex:     idx,
		defaultTopK:      topK,
		defaultThreshold: thresh,
	}, nil
}

// Retrieve 实现 github.com/cloudwego/eino/components/retriever.Retriever
func (m *MemoryRetriever) Retrieve(ctx context.Context, query string, opts ...einoretriever.Option) ([]*schema.Document, error) {
	options := einoretriever.GetCommonOptions(nil, opts...)
	if options == nil {
		options = &einoretriever.Options{}
	}
	indexName := m.defaultIndex
	if options.Index != nil && *options.Index != "" {
		indexName = *options.Index
	}
	topK := m.defaultTopK
	if options.TopK != nil && *options.TopK > 0 {
		topK = *options.TopK
	}
	threshold := m.defaultThreshold
	if options.ScoreThreshold != nil {
		threshold = *options.ScoreThreshold
	}

	if options.Embedding == nil {
		return nil, fmt.Errorf("Retriever requires WithEmbedding 选项以对 query 做向量化")
	}
	vecs, err := options.Embedding.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("retriever embedding: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedding returned empty")
	}
	queryVector := vecs[0]

	searchResults, err := m.vectorStore.Search(ctx, indexName, queryVector, &vector.SearchOptions{
		TopK:      topK,
		Threshold: threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("vector store search: %w", err)
	}

	docs := make([]*schema.Document, 0, len(searchResults))
	for _, sr := range searchResults {
		meta := make(map[string]any)
		if sr.Metadata != nil {
			for k, v := range sr.Metadata {
				meta[k] = v
			}
		}
		content := ""
		if sr.Metadata != nil {
			content = sr.Metadata["content"]
		}
		d := &schema.Document{
			ID:       sr.ID,
			Content:  content,
			MetaData: meta,
		}
		d.WithScore(sr.Score)
		docs = append(docs, d)
	}
	return docs, nil
}
