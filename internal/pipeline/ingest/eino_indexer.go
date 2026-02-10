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

package ingest

import (
	"context"
	"fmt"
	"strconv"

	einoindexer "github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"

	"rag-platform/internal/storage/vector"
)

// MemoryIndexer 基于 vector.Store 实现的 Eino indexer.Indexer（memory 后端）
type MemoryIndexer struct {
	vectorStore      vector.Store
	defaultCollection string
	batchSize        int
}

// MemoryIndexerConfig MemoryIndexer 构造参数
type MemoryIndexerConfig struct {
	VectorStore      vector.Store
	DefaultCollection string
	BatchSize        int
}

// NewMemoryIndexer 创建基于 vector.Store 的 Eino Indexer
func NewMemoryIndexer(cfg *MemoryIndexerConfig) (*MemoryIndexer, error) {
	if cfg == nil || cfg.VectorStore == nil {
		return nil, fmt.Errorf("MemoryIndexer 需要 VectorStore")
	}
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	collection := cfg.DefaultCollection
	if collection == "" {
		collection = "default"
	}
	return &MemoryIndexer{
		vectorStore:      cfg.VectorStore,
		defaultCollection: collection,
		batchSize:        batchSize,
	}, nil
}

// Store 实现 github.com/cloudwego/eino/components/indexer.Indexer
func (m *MemoryIndexer) Store(ctx context.Context, docs []*schema.Document, opts ...einoindexer.Option) (ids []string, err error) {
	if len(docs) == 0 {
		return nil, nil
	}
	options := einoindexer.GetCommonOptions(nil, opts...)
	indexName := m.defaultCollection
	if options != nil && len(options.SubIndexes) > 0 {
		indexName = options.SubIndexes[0]
	}
	if indexName == "" {
		indexName = "default"
	}

	// 若传入 Embedding，对无向量的 doc 做向量化
	if options != nil && options.Embedding != nil {
		for _, doc := range docs {
			if doc == nil {
				continue
			}
			if len(doc.DenseVector()) == 0 && doc.Content != "" {
				vecs, e := options.Embedding.EmbedStrings(ctx, []string{doc.Content})
				if e != nil {
					return nil, fmt.Errorf("indexer embedding: %w", e)
				}
				if len(vecs) > 0 {
					doc.WithDenseVector(vecs[0])
				}
			}
		}
	}

	// 转为 []*vector.Vector 并分批写入
	allIDs := make([]string, 0, len(docs))
	for i := 0; i < len(docs); i += m.batchSize {
		end := i + m.batchSize
		if end > len(docs) {
			end = len(docs)
		}
		batch := docs[i:end]
		vecs := make([]*vector.Vector, 0, len(batch))
		for idx, doc := range batch {
			if doc == nil {
				continue
			}
			vec := doc.DenseVector()
			if len(vec) == 0 {
				return nil, fmt.Errorf("doc %s has no vector and no Embedding option", doc.ID)
			}
			meta := metaToMapStringString(doc.MetaData)
			if meta == nil {
				meta = make(map[string]string)
			}
			if doc.Content != "" {
				meta["content"] = doc.Content
			}
			meta["index"] = strconv.Itoa(idx)
			vecs = append(vecs, &vector.Vector{
				ID:       doc.ID,
				Values:   vec,
				Metadata: meta,
			})
			allIDs = append(allIDs, doc.ID)
		}
		if len(vecs) == 0 {
			continue
		}
		if err := m.vectorStore.Add(ctx, indexName, vecs); err != nil {
			return nil, fmt.Errorf("vector store add: %w", err)
		}
	}
	return allIDs, nil
}

// metaToMapStringString 将 map[string]any 转为 map[string]string（仅 string 值）
func metaToMapStringString(meta map[string]any) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
