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
	"sync"

	"rag-platform/internal/model/embedding"
	"rag-platform/internal/pipeline/common"
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
		return nil, common.NewPipelineError(e.name, "输入验证failed", err)
	}

	// 向量化文档
	doc, ok := input.(*common.Document)
	if !ok {
		return nil, common.NewPipelineError(e.name, "输入类型error", fmt.Errorf("expected *common.Document, got %T", input))
	}

	// 处理文档
	embeddedDoc, err := e.ProcessDocument(doc)
	if err != nil {
		return nil, common.NewPipelineError(e.name, "vectorize document failed", err)
	}

	return embeddedDoc, nil
}

// Validate 验证输入
func (e *DocumentEmbedding) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	if _, ok := input.(*common.Document); !ok {
		return fmt.Errorf("unsupported input type输入类型: %T", input)
	}

	if e.embedder == nil {
		return fmt.Errorf("not initialized嵌入器")
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
		return nil, common.NewPipelineError(e.name, "向量化切片failed", err)
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
				// 向量化：Embed(ctx, []string) ([][]float64, error)
				vecs, err := e.embedder.Embed(context.Background(), []string{chunk.Content})
				if err != nil {
					errChan <- fmt.Errorf("vectorize chunk %d failed: %w", idx, err)
					return
				}
				if len(vecs) > 0 {
					chunk.Embedding = vecs[0]
				}
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

	// 检查error
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
	vecs, err := e.embedder.Embed(context.Background(), []string{doc.Content})
	if err != nil {
		return fmt.Errorf("vectorize document failed: %w", err)
	}
	if len(vecs) > 0 {
		doc.Embedding = vecs[0]
	}
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
