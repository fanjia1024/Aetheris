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

package einoext

import (
	"context"
	"fmt"

	redisindexer "github.com/cloudwego/eino-ext/components/indexer/redis"
	redisretriever "github.com/cloudwego/eino-ext/components/retriever/redis"
	einoembed "github.com/cloudwego/eino/components/embedding"
	einoindexer "github.com/cloudwego/eino/components/indexer"
	einoretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/redis/go-redis/v9"

	"rag-platform/internal/pipeline/ingest"
	"rag-platform/internal/pipeline/query"
	"rag-platform/internal/storage/vector"
	"rag-platform/pkg/config"
)

const (
	defaultBatchSize  = 100
	defaultTopK       = 10
	defaultThreshold  = 0.3
	defaultCollection = "default"
)

// NewIndexer 根据 VectorConfig 创建 Eino Indexer（memory 用现有 vector.Store；redis 用 eino-ext）
func NewIndexer(ctx context.Context, cfg config.VectorConfig, vectorStore vector.Store, embedder einoembed.Embedder) (einoindexer.Indexer, error) {
	t := cfg.Type
	if t == "" {
		t = "memory"
	}
	switch t {
	case "memory":
		if vectorStore == nil {
			return nil, fmt.Errorf("vector type is memory but VectorStore is nil")
		}
		coll := cfg.Collection
		if coll == "" {
			coll = defaultCollection
		}
		return ingest.NewMemoryIndexer(&ingest.MemoryIndexerConfig{
			VectorStore:       vectorStore,
			DefaultCollection: coll,
			BatchSize:         defaultBatchSize,
		})
	case "redis":
		opts, err := RedisOptionsFromVectorConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("redis options: %w", err)
		}
		client := redis.NewClient(opts)
		if err := client.Ping(ctx).Err(); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("redis ping: %w", err)
		}
		coll := cfg.Collection
		if coll == "" {
			coll = defaultCollection
		}
		idx, err := redisindexer.NewIndexer(ctx, &redisindexer.IndexerConfig{
			Client:    client,
			KeyPrefix: coll,
			BatchSize: defaultBatchSize,
			Embedding: embedder,
		})
		if err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("redis indexer: %w", err)
		}
		return idx, nil
	default:
		return nil, fmt.Errorf("unsupported vector type: %s", t)
	}
}

// NewRetriever 根据 VectorConfig 创建 Eino Retriever（memory 用现有 vector.Store；redis 用 eino-ext）
func NewRetriever(ctx context.Context, cfg config.VectorConfig, vectorStore vector.Store, embedder einoembed.Embedder) (einoretriever.Retriever, error) {
	t := cfg.Type
	if t == "" {
		t = "memory"
	}
	switch t {
	case "memory":
		if vectorStore == nil {
			return nil, fmt.Errorf("vector type is memory but VectorStore is nil")
		}
		idx := cfg.Collection
		if idx == "" {
			idx = defaultCollection
		}
		return query.NewMemoryRetriever(&query.MemoryRetrieverConfig{
			VectorStore:      vectorStore,
			DefaultIndex:     idx,
			DefaultTopK:      defaultTopK,
			DefaultThreshold: defaultThreshold,
		})
	case "redis":
		opts, err := RedisOptionsFromVectorConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("redis options: %w", err)
		}
		client := redis.NewClient(opts)
		if err := client.Ping(ctx).Err(); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("redis ping: %w", err)
		}
		indexName := cfg.Collection
		if indexName == "" {
			indexName = defaultCollection
		}
		ret, err := redisretriever.NewRetriever(ctx, &redisretriever.RetrieverConfig{
			Client:    client,
			Index:     indexName,
			TopK:      defaultTopK,
			Embedding: embedder,
		})
		if err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("redis retriever: %w", err)
		}
		return ret, nil
	default:
		return nil, fmt.Errorf("unsupported vector type: %s", t)
	}
}
