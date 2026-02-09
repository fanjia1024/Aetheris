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

package vector

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
)

// MemoryStore 内存向量存储实现
type MemoryStore struct {
	indexes map[string]*index
	mu      sync.RWMutex
}

// index 内存索引实现
type index struct {
	index      *Index
	vectors    map[string]*Vector
	dimension  int
}

// NewMemoryStore 创建新的内存向量存储
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		indexes: make(map[string]*index),
	}
}

// Create 创建向量索引
func (s *MemoryStore) Create(ctx context.Context, idx *Index) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.indexes[idx.Name]; exists {
		return fmt.Errorf("index with name %s already exists", idx.Name)
	}

	s.indexes[idx.Name] = &index{
		index:      idx,
		vectors:    make(map[string]*Vector),
		dimension:  idx.Dimension,
	}

	return nil
}

// Add 添加向量
func (s *MemoryStore) Add(ctx context.Context, indexName string, vectors []*Vector) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, exists := s.indexes[indexName]
	if !exists {
		return fmt.Errorf("index with name %s not found", indexName)
	}

	for _, vector := range vectors {
		if len(vector.Values) != idx.dimension {
			return fmt.Errorf("vector dimension %d does not match index dimension %d", len(vector.Values), idx.dimension)
		}

		idx.vectors[vector.ID] = vector
	}

	return nil
}

// Search 搜索向量
func (s *MemoryStore) Search(ctx context.Context, indexName string, query []float64, options *SearchOptions) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idx, exists := s.indexes[indexName]
	if !exists {
		return nil, fmt.Errorf("index with name %s not found", indexName)
	}

	if len(query) != idx.dimension {
		return nil, fmt.Errorf("query dimension %d does not match index dimension %d", len(query), idx.dimension)
	}

	if options == nil {
		options = &SearchOptions{
			TopK:      10,
			Threshold: 0.0,
		}
	}

	var results []*SearchResult

	// 计算相似度
	for id, vector := range idx.vectors {
		// 应用过滤条件
		if len(options.Filter) > 0 {
			match := true
			for key, value := range options.Filter {
				if vector.Metadata == nil || vector.Metadata[key] != value {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		// 计算相似度
		score := s.calculateSimilarity(query, vector.Values, idx.index.Distance)

		// 应用阈值过滤
		if score < options.Threshold {
			continue
		}

		// 构建搜索结果
		result := &SearchResult{
			ID:       id,
			Score:    score,
			Metadata: vector.Metadata,
		}

		// 是否包含向量值
		if options.IncludeVectors {
			result.Values = vector.Values
		}

		results = append(results, result)
	}

	// 按相似度排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 限制返回数量
	if len(results) > options.TopK {
		results = results[:options.TopK]
	}

	return results, nil
}

// Get 根据 ID 获取向量
func (s *MemoryStore) Get(ctx context.Context, indexName string, id string) (*Vector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idx, exists := s.indexes[indexName]
	if !exists {
		return nil, fmt.Errorf("index with name %s not found", indexName)
	}

	vector, exists := idx.vectors[id]
	if !exists {
		return nil, fmt.Errorf("vector with ID %s not found", id)
	}

	return vector, nil
}

// Delete 删除向量
func (s *MemoryStore) Delete(ctx context.Context, indexName string, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, exists := s.indexes[indexName]
	if !exists {
		return fmt.Errorf("index with name %s not found", indexName)
	}

	if _, exists := idx.vectors[id]; !exists {
		return fmt.Errorf("vector with ID %s not found", id)
	}

	delete(idx.vectors, id)
	return nil
}

// DeleteIndex 删除索引
func (s *MemoryStore) DeleteIndex(ctx context.Context, indexName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.indexes[indexName]; !exists {
		return fmt.Errorf("index with name %s not found", indexName)
	}

	delete(s.indexes, indexName)
	return nil
}

// ListIndexes 列出所有索引
func (s *MemoryStore) ListIndexes(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var indexes []string
	for name := range s.indexes {
		indexes = append(indexes, name)
	}

	return indexes, nil
}

// Close 关闭存储连接
func (s *MemoryStore) Close() error {
	return nil
}

// calculateSimilarity 计算向量相似度
func (s *MemoryStore) calculateSimilarity(query, vector []float64, distance string) float64 {
	switch distance {
	case "cosine":
		return s.cosineSimilarity(query, vector)
	case "euclidean":
		return 1.0 / (1.0 + s.euclideanDistance(query, vector))
	case "manhattan":
		return 1.0 / (1.0 + s.manhattanDistance(query, vector))
	default:
		return s.cosineSimilarity(query, vector)
	}
}

// cosineSimilarity 计算余弦相似度
func (s *MemoryStore) cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	dotProduct := 0.0
	normA := 0.0
	normB := 0.0

	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// euclideanDistance 计算欧几里得距离
func (s *MemoryStore) euclideanDistance(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	sum := 0.0
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return math.Sqrt(sum)
}

// manhattanDistance 计算曼哈顿距离
func (s *MemoryStore) manhattanDistance(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	sum := 0.0
	for i := range a {
		sum += math.Abs(a[i] - b[i])
	}

	return sum
}
