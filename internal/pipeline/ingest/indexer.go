package ingest

import (
	"fmt"
	"sync"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/storage/vector"
	"rag-platform/internal/storage/metadata"
)

// DocumentIndexer 文档索引器
type DocumentIndexer struct {
	name           string
	vectorStore    *vector.Store
	metadataStore  *metadata.Store
	concurrency    int
	batchSize      int
}

// NewDocumentIndexer 创建新的文档索引器
func NewDocumentIndexer(vectorStore *vector.Store, metadataStore *metadata.Store, concurrency, batchSize int) *DocumentIndexer {
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
	indexedDoc, err := i.ProcessDocument(doc)
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
func (i *DocumentIndexer) ProcessDocument(doc *common.Document) (*common.Document, error) {
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
	if err := i.storeDocumentMetadata(doc); err != nil {
		return nil, common.NewPipelineError(i.name, "存储文档元数据失败", err)
	}

	// 批量索引切片
	if err := i.indexChunks(doc); err != nil {
		return nil, common.NewPipelineError(i.name, "索引切片失败", err)
	}

	// 更新文档元数据
	doc.Metadata["indexed"] = true
	doc.Metadata["indexer"] = i.name
	doc.Metadata["vector_store"] = i.vectorStore.Type()

	return doc, nil
}

// storeDocumentMetadata 存储文档元数据
func (i *DocumentIndexer) storeDocumentMetadata(doc *common.Document) error {
	// 创建文档记录
	documentRecord := &metadata.Document{
		ID:        doc.ID,
		Content:   doc.Content,
		Metadata:  doc.Metadata,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}

	// 存储到元数据存储
	if err := i.metadataStore.CreateDocument(documentRecord); err != nil {
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

// indexBatch 索引批次
func (i *DocumentIndexer) indexBatch(chunks []common.Chunk, documentID string) error {
	// 准备向量数据
	vectorData := make([]vector.VectorData, len(chunks))
	metadataRecords := make([]*metadata.Chunk, len(chunks))

	for idx, chunk := range chunks {
		// 准备向量数据
		vectorData[idx] = vector.VectorData{
			ID:        chunk.ID,
			Vector:    chunk.Embedding,
			Metadata:  chunk.Metadata,
			DocumentID: documentID,
		}

		// 准备元数据记录
		metadataRecords[idx] = &metadata.Chunk{
			ID:        chunk.ID,
			Content:   chunk.Content,
			Metadata:  chunk.Metadata,
			DocumentID: documentID,
			Index:     chunk.Index,
			TokenCount: chunk.TokenCount,
			CreatedAt: chunk.CreatedAt,
		}
	}

	// 并行处理
	var wg sync.WaitGroup
	var vectorErr, metadataErr error

	// 索引向量
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := i.vectorStore.IndexVectors(vectorData); err != nil {
			vectorErr = err
		}
	}()

	// 存储元数据
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := i.metadataStore.CreateChunks(metadataRecords); err != nil {
			metadataErr = err
		}
	}()

	// 等待完成
	wg.Wait()

	// 检查错误
	if vectorErr != nil {
		return fmt.Errorf("索引向量失败: %w", vectorErr)
	}
	if metadataErr != nil {
		return fmt.Errorf("存储切片元数据失败: %w", metadataErr)
	}

	return nil
}

// SetVectorStore 设置向量存储
func (i *DocumentIndexer) SetVectorStore(store *vector.Store) {
	i.vectorStore = store
}

// GetVectorStore 获取向量存储
func (i *DocumentIndexer) GetVectorStore() *vector.Store {
	return i.vectorStore
}

// SetMetadataStore 设置元数据存储
func (i *DocumentIndexer) SetMetadataStore(store *metadata.Store) {
	i.metadataStore = store
}

// GetMetadataStore 获取元数据存储
func (i *DocumentIndexer) GetMetadataStore() *metadata.Store {
	return i.metadataStore
}
