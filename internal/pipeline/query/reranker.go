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
	"fmt"
	"sort"
	"time"

	"rag-platform/internal/pipeline/common"
)

// Reranker 重排器
type Reranker struct {
	name           string
	topK           int
	scoreThreshold float64
}

// NewReranker 创建新的重排器
func NewReranker(topK int, scoreThreshold float64) *Reranker {
	if topK <= 0 {
		topK = 5
	}
	if scoreThreshold <= 0 {
		scoreThreshold = 0.5
	}

	return &Reranker{
		name:           "reranker",
		topK:           topK,
		scoreThreshold: scoreThreshold,
	}
}

// Name 返回组件名称
func (r *Reranker) Name() string {
	return r.name
}

// Execute 执行重排操作
func (r *Reranker) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	// 验证输入
	if err := r.Validate(input); err != nil {
		return nil, common.NewPipelineError(r.name, "输入验证failed", err)
	}

	// 重排检索结果
	result, ok := input.(*common.RetrievalResult)
	if !ok {
		return nil, common.NewPipelineError(r.name, "输入类型error", fmt.Errorf("expected *common.RetrievalResult, got %T", input))
	}

	// 处理重排
	rerankedResult, err := r.rerank(result)
	if err != nil {
		return nil, common.NewPipelineError(r.name, "重排结果failed", err)
	}

	return rerankedResult, nil
}

// Validate 验证输入
func (r *Reranker) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	if _, ok := input.(*common.RetrievalResult); !ok {
		return fmt.Errorf("unsupported input输入类型: %T", input)
	}

	return nil
}

// ProcessQuery 处理查询
func (r *Reranker) ProcessQuery(query *common.Query) (*common.Query, error) {
	// 这里可以添加查询预处理逻辑
	return query, nil
}

// rerank 重排检索结果
func (r *Reranker) rerank(result *common.RetrievalResult) (*common.RetrievalResult, error) {
	if result == nil || len(result.Chunks) == 0 {
		return result, nil
	}

	startTime := time.Now()

	// 过滤低分数结果
	filteredChunks := make([]common.Chunk, 0, len(result.Chunks))
	filteredScores := make([]float64, 0, len(result.Scores))

	for i, score := range result.Scores {
		if score >= r.scoreThreshold {
			filteredChunks = append(filteredChunks, result.Chunks[i])
			filteredScores = append(filteredScores, score)
		}
	}

	// 按分数排序
	sort.Sort(&chunkScorePair{
		chunks: filteredChunks,
		scores: filteredScores,
	})

	// 限制返回数量
	if len(filteredChunks) > r.topK {
		filteredChunks = filteredChunks[:r.topK]
		filteredScores = filteredScores[:r.topK]
	}

	// 创建重排结果
	rerankedResult := &common.RetrievalResult{
		Chunks:      filteredChunks,
		Scores:      filteredScores,
		TotalCount:  len(filteredChunks),
		ProcessTime: time.Since(startTime),
	}

	return rerankedResult, nil
}

// chunkScorePair 用于排序的切片分数对
type chunkScorePair struct {
	chunks []common.Chunk
	scores []float64
}

// Len 返回长度
func (c *chunkScorePair) Len() int {
	return len(c.scores)
}

// Less 比较两个元素
func (c *chunkScorePair) Less(i, j int) bool {
	return c.scores[i] > c.scores[j] // 降序排序
}

// Swap 交换两个元素
func (c *chunkScorePair) Swap(i, j int) {
	c.chunks[i], c.chunks[j] = c.chunks[j], c.chunks[i]
	c.scores[i], c.scores[j] = c.scores[j], c.scores[i]
}

// SetTopK 设置返回结果数量
func (r *Reranker) SetTopK(topK int) {
	if topK > 0 {
		r.topK = topK
	}
}

// SetScoreThreshold 设置分数阈值
func (r *Reranker) SetScoreThreshold(threshold float64) {
	if threshold > 0 {
		r.scoreThreshold = threshold
	}
}
