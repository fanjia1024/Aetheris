package ingest

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/splitter"
)

// DocumentSplitter 文档切片器；可委托 internal/splitter.Engine 统一切片（设计：所有切片逻辑收敛为 Splitter Engine）
type DocumentSplitter struct {
	name         string
	chunkSize    int
	chunkOverlap int
	maxChunks    int
	engine       *splitter.Engine
	splitterName string
}

// NewDocumentSplitter 创建新的文档切片器（默认自实现；可再 SetEngine 委托 Splitter Engine）
func NewDocumentSplitter(chunkSize, chunkOverlap, maxChunks int) *DocumentSplitter {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if chunkOverlap <= 0 {
		chunkOverlap = 100
	}
	if maxChunks <= 0 {
		maxChunks = 1000
	}
	return &DocumentSplitter{
		name:         "splitter",
		chunkSize:    chunkSize,
		chunkOverlap: chunkOverlap,
		maxChunks:    maxChunks,
		splitterName: "structural",
	}
}

// SetEngine 设置 Splitter Engine，此后切片委托给 Engine（统一抽象）
func (s *DocumentSplitter) SetEngine(engine *splitter.Engine, splitterName string) {
	s.engine = engine
	if splitterName != "" {
		s.splitterName = splitterName
	}
}

// Name 返回组件名称
func (s *DocumentSplitter) Name() string {
	return s.name
}

// Execute 执行切片操作
func (s *DocumentSplitter) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	// 验证输入
	if err := s.Validate(input); err != nil {
		return nil, common.NewPipelineError(s.name, "输入验证失败", err)
	}

	// 切片文档
	doc, ok := input.(*common.Document)
	if !ok {
		return nil, common.NewPipelineError(s.name, "输入类型错误", fmt.Errorf("expected *common.Document, got %T", input))
	}

	// 处理文档
	splitDoc, err := s.ProcessDocument(doc)
	if err != nil {
		return nil, common.NewPipelineError(s.name, "切片文档失败", err)
	}

	return splitDoc, nil
}

// Validate 验证输入
func (s *DocumentSplitter) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	if _, ok := input.(*common.Document); !ok {
		return fmt.Errorf("不支持的输入类型: %T", input)
	}

	return nil
}

// ProcessDocument 处理文档
func (s *DocumentSplitter) ProcessDocument(doc *common.Document) (*common.Document, error) {
	// 执行切片
	chunks, err := s.splitDocument(doc)
	if err != nil {
		return nil, common.NewPipelineError(s.name, "切片文档失败", err)
	}

	// 更新文档
	doc.Chunks = chunks
	doc.Metadata["chunked"] = true
	doc.Metadata["chunk_count"] = len(chunks)
	doc.Metadata["splitter"] = s.name
	doc.Metadata["chunk_size"] = s.chunkSize
	doc.Metadata["chunk_overlap"] = s.chunkOverlap

	return doc, nil
}

// splitDocument 切片文档（优先委托 Splitter Engine，否则使用自实现）
func (s *DocumentSplitter) splitDocument(doc *common.Document) ([]common.Chunk, error) {
	content := doc.Content
	if content == "" {
		return []common.Chunk{}, nil
	}

	if s.engine != nil {
		options := map[string]interface{}{
			"chunk_size":    s.chunkSize,
			"chunk_overlap": s.chunkOverlap,
			"max_chunks":    s.maxChunks,
		}
		chunks, err := s.engine.Split(content, s.splitterName, options)
		if err != nil {
			return nil, err
		}
		for i := range chunks {
			chunks[i].DocumentID = doc.ID
		}
		if len(chunks) > s.maxChunks {
			chunks = chunks[:s.maxChunks]
		}
		return chunks, nil
	}

	// 自实现：按段落分割并合并切片
	paragraphs := s.splitByParagraph(content)
	chunks := s.mergeAndSplit(paragraphs, doc.ID)
	if len(chunks) > s.maxChunks {
		chunks = chunks[:s.maxChunks]
	}
	return chunks, nil
}

// splitByParagraph 按段落分割
func (s *DocumentSplitter) splitByParagraph(content string) []string {
	// 按换行符分割
	lines := strings.Split(content, "\n")

	var paragraphs []string
	var currentParagraph strings.Builder

	for _, line := range lines {
		// 清理空白
		trimmedLine := strings.TrimSpace(line)

		if trimmedLine == "" {
			// 空行，结束当前段落
			if currentParagraph.Len() > 0 {
				paragraphs = append(paragraphs, currentParagraph.String())
				currentParagraph.Reset()
			}
		} else {
			// 添加到当前段落
			if currentParagraph.Len() > 0 {
				currentParagraph.WriteString(" ")
			}
			currentParagraph.WriteString(trimmedLine)
		}
	}

	// 添加最后一个段落
	if currentParagraph.Len() > 0 {
		paragraphs = append(paragraphs, currentParagraph.String())
	}

	return paragraphs
}

// mergeAndSplit 合并并切片
func (s *DocumentSplitter) mergeAndSplit(paragraphs []string, documentID string) []common.Chunk {
	var chunks []common.Chunk
	var currentChunk strings.Builder
	var chunkIndex int

	for _, paragraph := range paragraphs {
		// 检查当前段落是否超过 chunkSize
		if len(paragraph) > s.chunkSize {
			// 段落过长，单独切片
			if currentChunk.Len() > 0 {
				// 先保存当前 chunk
				chunks = append(chunks, s.createChunk(currentChunk.String(), documentID, chunkIndex))
				chunkIndex++
				currentChunk.Reset()
			}

			// 切片长段落
			longChunks := s.splitLongText(paragraph, documentID, chunkIndex)
			chunks = append(chunks, longChunks...)
			chunkIndex += len(longChunks)
		} else {
			// 检查添加当前段落后是否超过 chunkSize
			if currentChunk.Len()+len(paragraph)+1 > s.chunkSize {
				// 保存当前 chunk
				chunks = append(chunks, s.createChunk(currentChunk.String(), documentID, chunkIndex))
				chunkIndex++

				// 开始新 chunk，添加重叠部分
				if currentChunk.Len() > s.chunkOverlap {
					overlap := currentChunk.String()[currentChunk.Len()-s.chunkOverlap:]
					currentChunk.Reset()
					currentChunk.WriteString(overlap)
					currentChunk.WriteString(" ")
				} else {
					currentChunk.Reset()
				}
			}

			// 添加当前段落
			if currentChunk.Len() > 0 {
				currentChunk.WriteString(" ")
			}
			currentChunk.WriteString(paragraph)
		}
	}

	// 保存最后一个 chunk
	if currentChunk.Len() > 0 {
		chunks = append(chunks, s.createChunk(currentChunk.String(), documentID, chunkIndex))
	}

	return chunks
}

// splitLongText 切片长文本
func (s *DocumentSplitter) splitLongText(text string, documentID string, startIndex int) []common.Chunk {
	var chunks []common.Chunk
	var chunkIndex int = startIndex

	for i := 0; i < len(text); i += s.chunkSize - s.chunkOverlap {
		end := i + s.chunkSize
		if end > len(text) {
			end = len(text)
		}

		chunkText := text[i:end]
		chunks = append(chunks, s.createChunk(chunkText, documentID, chunkIndex))
		chunkIndex++
	}

	return chunks
}

// createChunk 创建切片
func (s *DocumentSplitter) createChunk(content string, documentID string, index int) common.Chunk {
	return common.Chunk{
		ID:         uuid.New().String(),
		Content:    content,
		Metadata:   make(map[string]interface{}),
		DocumentID: documentID,
		Index:      index,
		TokenCount: len([]rune(content)), // 简单的 token 计数
	}
}
