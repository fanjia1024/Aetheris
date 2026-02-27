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
	"strconv"
	"time"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/storage/vector"
)

// Retriever 检索器（使用 vector.Store.Search 适配，不依赖 Retrieve 接口）
type Retriever struct {
	name           string
	vectorStore    vector.Store
	indexName      string
	topK           int
	scoreThreshold float64
}

// NewRetriever 创建新的检索器
func NewRetriever(vectorStore vector.Store, indexName string, topK int, scoreThreshold float64) *Retriever {
	if topK <= 0 {
		topK = 10
	}
	if scoreThreshold <= 0 {
		scoreThreshold = 0.3
	}
	if indexName == "" {
		indexName = "default"
	}

	return &Retriever{
		name:           "retriever",
		vectorStore:    vectorStore,
		indexName:      indexName,
		topK:           topK,
		scoreThreshold: scoreThreshold,
	}
}

// Name 返回组件名称
func (r *Retriever) Name() string {
	return r.name
}

// Execute 执行检索操作
func (r *Retriever) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	// 验证输入
	if err := r.Validate(input); err != nil {
		return nil, common.NewPipelineError(r.name, "输入验证failed", err)
	}

	// 检索查询
	query, ok := input.(*common.Query)
	if !ok {
		return nil, common.NewPipelineError(r.name, "输入类型error", fmt.Errorf("expected *common.Query, got %T", input))
	}

	// 处理查询
	result, err := r.ProcessQuery(query)
	if err != nil {
		return nil, common.NewPipelineError(r.name, "检索查询failed", err)
	}

	return result, nil
}

// Validate 验证输入
func (r *Retriever) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	if _, ok := input.(*common.Query); !ok {
		return fmt.Errorf("unsupported input type输入类型: %T", input)
	}

	if r.vectorStore == nil {
		return fmt.Errorf("not initialized向量存储")
	}

	return nil
}

// ProcessQuery 处理查询
func (r *Retriever) ProcessQuery(query *common.Query) (*common.RetrievalResult, error) {
	// 检查查询是否有嵌入
	if len(query.Embedding) == 0 {
		return nil, common.NewPipelineError(r.name, "查询未向量化", fmt.Errorf("query has no embedding"))
	}

	results, err := r.retrieve(query)
	if err != nil {
		return nil, common.NewPipelineError(r.name, "执行检索failed", err)
	}
	return results, nil
}

// retrieve 执行检索（使用 Store.Search 适配）
func (r *Retriever) retrieve(query *common.Query) (*common.RetrievalResult, error) {
	ctx := context.Background()
	opts := &vector.SearchOptions{
		TopK:      r.topK,
		Threshold: r.scoreThreshold,
	}
	if query.Metadata != nil {
		opts.Filter = make(map[string]string)
		for k, v := range query.Metadata {
			if s, ok := v.(string); ok {
				opts.Filter[k] = s
			}
		}
	}

	searchResults, err := r.vectorStore.Search(ctx, r.indexName, query.Embedding, opts)
	if err != nil {
		return nil, fmt.Errorf("retrieve from vector store failed: %w", err)
	}

	startTime := time.Now()
	chunks := make([]common.Chunk, len(searchResults))
	scores := make([]float64, len(searchResults))

	for i, sr := range searchResults {
		meta := make(map[string]interface{})
		if sr.Metadata != nil {
			for k, v := range sr.Metadata {
				meta[k] = v
			}
		}
		content := ""
		if sr.Metadata != nil {
			content = sr.Metadata["content"]
		}
		docID := ""
		if sr.Metadata != nil {
			docID = sr.Metadata["document_id"]
		}
		idx := 0
		if sr.Metadata != nil && sr.Metadata["index"] != "" {
			idx, _ = strconv.Atoi(sr.Metadata["index"])
		}
		tokenCount := 0
		if sr.Metadata != nil && sr.Metadata["token_count"] != "" {
			tokenCount, _ = strconv.Atoi(sr.Metadata["token_count"])
		}

		chunks[i] = common.Chunk{
			ID:         sr.ID,
			Content:    content,
			Metadata:   meta,
			Embedding:  sr.Values,
			DocumentID: docID,
			Index:      idx,
			TokenCount: tokenCount,
		}
		scores[i] = sr.Score
	}

	return &common.RetrievalResult{
		Chunks:      chunks,
		Scores:      scores,
		TotalCount:  len(chunks),
		ProcessTime: time.Since(startTime),
	}, nil
}

// SetVectorStore 设置向量存储
func (r *Retriever) SetVectorStore(store vector.Store) {
	r.vectorStore = store
}

// GetVectorStore 获取向量存储
func (r *Retriever) GetVectorStore() vector.Store {
	return r.vectorStore
}

// SetIndexName 设置检索使用的索引名
func (r *Retriever) SetIndexName(name string) {
	if name != "" {
		r.indexName = name
	}
}

// SetTopK 设置返回结果数量
func (r *Retriever) SetTopK(topK int) {
	if topK > 0 {
		r.topK = topK
	}
}

// SetScoreThreshold 设置分数阈值
func (r *Retriever) SetScoreThreshold(threshold float64) {
	if threshold > 0 {
		r.scoreThreshold = threshold
	}
}
