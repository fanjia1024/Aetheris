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
	"time"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/storage/metadata"
	"rag-platform/internal/storage/vector"
)

// DocumentIndexer 文档索引器
type DocumentIndexer struct {
	name           string
	vectorStore    vector.Store
	metadataStore  metadata.Store
	concurrency    int
	batchSize      int
}

// NewDocumentIndexer 创建新的文档索引器
func NewDocumentIndexer(vectorStore vector.Store, metadataStore metadata.Store, concurrency, batchSize int) *DocumentIndexer {
	if concurrency <= 0 {
		concurrency = 4
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	return &DocumentIndexer{
		name:          "index_builder",
		vectorStore:   vectorStore,
		metadataStore: metadataStore,
		concurrency:   concurrency,
		batchSize:     batchSize,
	}
}

// Name 返回组件名称
func (i *DocumentIndexer) Name() string {
	return i.name
}

// Execute 执行索引操作
func (i *DocumentIndexer) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	// 验证输入
	if err := i.Validate(input); err != nil {
		return nil, common.NewPipelineError(i.name, "输入验证失败", err)
	}

	// 索引文档
	doc, ok := input.(*common.Document)
	if !ok {
		return nil, common.NewPipelineError(i.name, "输入类型错误", fmt.Errorf("expected *common.Document, got %T", input))
	}

	// 处理文档
	indexedDoc, err := i.ProcessDocument(ctx, doc)
	if err != nil {
		return nil, common.NewPipelineError(i.name, "索引文档失败", err)
	}

	return indexedDoc, nil
}

// Validate 验证输入
func (i *DocumentIndexer) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	if _, ok := input.(*common.Document); !ok {
		return fmt.Errorf("不支持的输入类型: %T", input)
	}

	if i.vectorStore == nil {
		return fmt.Errorf("未初始化向量存储")
	}

	if i.metadataStore == nil {
		return fmt.Errorf("未初始化元数据存储")
	}

	return nil
}

// ProcessDocument 处理文档
func (i *DocumentIndexer) ProcessDocument(ctx *common.PipelineContext, doc *common.Document) (*common.Document, error) {
	// 检查是否有向量化的切片
	if len(doc.Chunks) == 0 {
		return nil, common.NewPipelineError(i.name, "文档没有切片", fmt.Errorf("document has no chunks"))
	}

	// 检查切片是否已向量化
	for _, chunk := range doc.Chunks {
		if len(chunk.Embedding) == 0 {
			return nil, common.NewPipelineError(i.name, "切片未向量化", fmt.Errorf("chunk %s has no embedding", chunk.ID))
		}
	}

	// 存储文档元数据
	if err := i.storeDocumentMetadata(ctx.Context, doc); err != nil {
		return nil, common.NewPipelineError(i.name, "存储文档元数据失败", err)
	}

	// 批量索引切片
	if err := i.indexChunks(doc); err != nil {
		return nil, common.NewPipelineError(i.name, "索引切片失败", err)
	}

	// 更新文档元数据
	doc.Metadata["indexed"] = true
	doc.Metadata["indexer"] = i.name
	doc.Metadata["vector_store"] = "memory"

	return doc, nil
}

// storeDocumentMetadata 存储文档元数据
func (i *DocumentIndexer) storeDocumentMetadata(ctx context.Context, doc *common.Document) error {
	meta := make(map[string]string)
	for k, v := range doc.Metadata {
		if s, ok := v.(string); ok {
			meta[k] = s
		}
	}
	var createdAt, updatedAt int64
	if !doc.CreatedAt.IsZero() {
		createdAt = doc.CreatedAt.Unix()
	}
	if !doc.UpdatedAt.IsZero() {
		updatedAt = doc.UpdatedAt.Unix()
	} else {
		updatedAt = time.Now().Unix()
	}
	documentRecord := &metadata.Document{
		ID:        doc.ID,
		Name:      doc.ID,
		Type:      "document",
		Size:      0,
		Path:      "",
		Status:    "indexed",
		Chunks:    len(doc.Chunks),
		Metadata:  meta,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	if err := i.metadataStore.Create(ctx, documentRecord); err != nil {
		return fmt.Errorf("创建文档记录失败: %w", err)
	}
	return nil
}

// indexChunks 索引切片
func (i *DocumentIndexer) indexChunks(doc *common.Document) error {
	chunks := doc.Chunks
	if len(chunks) == 0 {
		return nil
	}

	// 分批处理
	for start := 0; start < len(chunks); start += i.batchSize {
		end := start + i.batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		batch := chunks[start:end]
		if err := i.indexBatch(batch, doc.ID); err != nil {
			return fmt.Errorf("索引批次失败: %w", err)
		}
	}

	return nil
}

// indexBatch 索引批次（使用 vector.Store.Add）
func (i *DocumentIndexer) indexBatch(chunks []common.Chunk, documentID string) error {
	if len(chunks) == 0 {
		return nil
	}
	indexName := "default"
	vecs := make([]*vector.Vector, 0, len(chunks))
	for idx, chunk := range chunks {
		meta := make(map[string]string)
		meta["document_id"] = documentID
		meta["content"] = chunk.Content
		meta["index"] = strconv.Itoa(idx)
		meta["token_count"] = strconv.Itoa(chunk.TokenCount)
		vecs = append(vecs, &vector.Vector{
			ID:       chunk.ID,
			Values:   chunk.Embedding,
			Metadata: meta,
		})
	}
	ctx := context.Background()
	if err := i.vectorStore.Add(ctx, indexName, vecs); err != nil {
		return fmt.Errorf("索引向量失败: %w", err)
	}
	return nil
}

// SetVectorStore 设置向量存储
func (i *DocumentIndexer) SetVectorStore(store vector.Store) {
	i.vectorStore = store
}

// GetVectorStore 获取向量存储
func (i *DocumentIndexer) GetVectorStore() vector.Store {
	return i.vectorStore
}

// SetMetadataStore 设置元数据存储
func (i *DocumentIndexer) SetMetadataStore(store metadata.Store) {
	i.metadataStore = store
}

// GetMetadataStore 获取元数据存储
func (i *DocumentIndexer) GetMetadataStore() metadata.Store {
	return i.metadataStore
}
