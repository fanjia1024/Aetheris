package query

import (
	"fmt"
	"time"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/storage/vector"
)

// Retriever 检索器
type Retriever struct {
	name           string
	vectorStore    *vector.Store
	topK           int
	scoreThreshold float64
}

// NewRetriever 创建新的检索器
func NewRetriever(vectorStore *vector.Store, topK int, scoreThreshold float64) *Retriever {
	if topK <= 0 {
		topK = 10
	}
	if scoreThreshold <= 0 {
		scoreThreshold = 0.3
	}

	return &Retriever{
		name:           "retriever",
		vectorStore:    vectorStore,
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
		return nil, common.NewPipelineError(r.name, "输入验证失败", err)
	}

	// 检索查询
	query, ok := input.(*common.Query)
	if !ok {
		return nil, common.NewPipelineError(r.name, "输入类型错误", fmt.Errorf("expected *common.Query, got %T", input))
	}

	// 处理查询
	result, err := r.ProcessQuery(query)
	if err != nil {
		return nil, common.NewPipelineError(r.name, "检索查询失败", err)
	}

	return result, nil
}

// Validate 验证输入
func (r *Retriever) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	if _, ok := input.(*common.Query); !ok {
		return fmt.Errorf("不支持的输入类型: %T", input)
	}

	if r.vectorStore == nil {
		return fmt.Errorf("未初始化向量存储")
	}

	return nil
}

// ProcessQuery 处理查询
func (r *Retriever) ProcessQuery(query *common.Query) (*common.RetrievalResult, error) {
	// 检查查询是否有嵌入
	if len(query.Embedding) == 0 {
		return nil, common.NewPipelineError(r.name, "查询未向量化", fmt.Errorf("query has no embedding"))
	}

	// 执行检索
	startTime := time.Now()
	results, err := r.retrieve(query)
	if err != nil {
		return nil, common.NewPipelineError(r.name, "执行检索失败", err)
	}

	// 计算处理时间
	processTime := time.Since(startTime)

	return results, nil
}

// retrieve 执行检索
func (r *Retriever) retrieve(query *common.Query) (*common.RetrievalResult, error) {
	// 构建检索请求
	retrievalReq := vector.RetrievalRequest{
		Query:          query.Text,
		Embedding:      query.Embedding,
		TopK:           r.topK,
		ScoreThreshold: r.scoreThreshold,
		Filters:        query.Metadata,
	}

	// 从向量存储检索
	vectorResults, err := r.vectorStore.Retrieve(retrievalReq)
	if err != nil {
		return nil, fmt.Errorf("从向量存储检索失败: %w", err)
	}

	// 转换为检索结果
	chunks := make([]common.Chunk, len(vectorResults.Matches))
	scores := make([]float64, len(vectorResults.Matches))

	for i, match := range vectorResults.Matches {
		chunks[i] = common.Chunk{
			ID:          match.ID,
			Content:     match.Content,
			Metadata:    match.Metadata,
			Embedding:   match.Embedding,
			DocumentID:  match.DocumentID,
			Index:       match.Index,
			TokenCount:  match.TokenCount,
		}
		scores[i] = match.Score
	}

	// 创建检索结果
	result := &common.RetrievalResult{
		Chunks:      chunks,
		Scores:      scores,
		TotalCount:  len(chunks),
		ProcessTime: vectorResults.ProcessTime,
	}

	return result, nil
}

// SetVectorStore 设置向量存储
func (r *Retriever) SetVectorStore(store *vector.Store) {
	r.vectorStore = store
}

// GetVectorStore 获取向量存储
func (r *Retriever) GetVectorStore() *vector.Store {
	return r.vectorStore
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
