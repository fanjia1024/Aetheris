package ingest

import (
	"fmt"
	"sync"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/model/embedding"
)

// DocumentEmbedding 文档向量化器
type DocumentEmbedding struct {
	name        string
	embedder    *embedding.Embedder
	concurrency int
}

// NewDocumentEmbedding 创建新的文档向量化器
func NewDocumentEmbedding(embedder *embedding.Embedder, concurrency int) *DocumentEmbedding {
	if concurrency <= 0 {
		concurrency = 4
	}

	return &DocumentEmbedding{
		name:        "embedding",
		embedder:    embedder,
		concurrency: concurrency,
	}
}

// Name 返回组件名称
func (e *DocumentEmbedding) Name() string {
	return e.name
}

// Execute 执行向量化操作
func (e *DocumentEmbedding) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	// 验证输入
	if err := e.Validate(input); err != nil {
		return nil, common.NewPipelineError(e.name, "输入验证失败", err)
	}

	// 向量化文档
	doc, ok := input.(*common.Document)
	if !ok {
		return nil, common.NewPipelineError(e.name, "输入类型错误", fmt.Errorf("expected *common.Document, got %T", input))
	}

	// 处理文档
	embeddedDoc, err := e.ProcessDocument(doc)
	if err != nil {
		return nil, common.NewPipelineError(e.name, "向量化文档失败", err)
	}

	return embeddedDoc, nil
}

// Validate 验证输入
func (e *DocumentEmbedding) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	if _, ok := input.(*common.Document); !ok {
		return fmt.Errorf("不支持的输入类型: %T", input)
	}

	if e.embedder == nil {
		return fmt.Errorf("未初始化嵌入器")
	}

	return nil
}

// ProcessDocument 处理文档
func (e *DocumentEmbedding) ProcessDocument(doc *common.Document) (*common.Document, error) {
	// 检查是否有切片
	if len(doc.Chunks) == 0 {
		return nil, common.NewPipelineError(e.name, "文档没有切片", fmt.Errorf("document has no chunks"))
	}

	// 批量向量化切片
	if err := e.embedChunks(doc); err != nil {
		return nil, common.NewPipelineError(e.name, "向量化切片失败", err)
	}

	// 更新文档元数据
	doc.Metadata["embedded"] = true
	doc.Metadata["embedding_model"] = e.embedder.Model()
	doc.Metadata["embedding_dimension"] = e.embedder.Dimension()

	return doc, nil
}

// embedChunks 向量化切片
func (e *DocumentEmbedding) embedChunks(doc *common.Document) error {
	chunks := doc.Chunks
	if len(chunks) == 0 {
		return nil
	}

	// 限制并发
	if len(chunks) < e.concurrency {
		e.concurrency = len(chunks)
	}

	// 创建工作池
	var wg sync.WaitGroup
	chunkChan := make(chan int, len(chunks))
	errChan := make(chan error, len(chunks))

	// 启动工作协程
	for i := 0; i < e.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range chunkChan {
				chunk := &chunks[idx]
				
				// 向量化
				embedding, err := e.embedder.Embed(chunk.Content)
				if err != nil {
					errChan <- fmt.Errorf("向量化切片 %d 失败: %w", idx, err)
					return
				}
				
				// 更新切片
				chunk.Embedding = embedding
				chunk.Metadata["embedded"] = true
			}
			}()
	}

	// 分发任务
	for i := range chunks {
		chunkChan <- i
	}
	close(chunkChan)

	// 等待完成
	wg.Wait()
	close(errChan)

	// 检查错误
	for err := range errChan {
		return err
	}

	return nil
}

// embedDocument 向量化整个文档（可选）
func (e *DocumentEmbedding) embedDocument(doc *common.Document) error {
	if doc.Content == "" {
		return nil
	}

	// 向量化文档内容
	embedding, err := e.embedder.Embed(doc.Content)
	if err != nil {
		return fmt.Errorf("向量化文档失败: %w", err)
	}

	// 更新文档
	doc.Embedding = embedding
	doc.Metadata["document_embedded"] = true

	return nil
}

// SetEmbedder 设置嵌入器
func (e *DocumentEmbedding) SetEmbedder(embedder *embedding.Embedder) {
	e.embedder = embedder
}

// GetEmbedder 获取嵌入器
func (e *DocumentEmbedding) GetEmbedder() *embedding.Embedder {
	return e.embedder
}
