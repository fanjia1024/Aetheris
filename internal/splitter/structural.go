package splitter

import (
	"strings"

	"github.com/google/uuid"
	"rag-platform/internal/pipeline/common"
)

// StructuralSplitter 结构切片器
type StructuralSplitter struct {
	name string
}

// NewStructuralSplitter 创建新的结构切片器
func NewStructuralSplitter() *StructuralSplitter {
	return &StructuralSplitter{
		name: "structural_splitter",
	}
}

// Name 返回切片器名称
func (s *StructuralSplitter) Name() string {
	return s.name
}

// Split 执行结构切片
func (s *StructuralSplitter) Split(content string, options map[string]interface{}) ([]common.Chunk, error) {
	// 获取配置选项
	chunkSize := 1000
	chunkOverlap := 100

	if size, ok := options["chunk_size"].(int); ok && size > 0 {
		chunkSize = size
	}
	if overlap, ok := options["chunk_overlap"].(int); ok && overlap > 0 {
		chunkOverlap = overlap
	}

	// 按段落分割
	paragraphs := s.splitByParagraph(content)

	// 合并并切片
	chunks := s.mergeAndSplit(paragraphs, chunkSize, chunkOverlap)

	return chunks, nil
}

// splitByParagraph 按段落分割
func (s *StructuralSplitter) splitByParagraph(content string) []string {
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
func (s *StructuralSplitter) mergeAndSplit(paragraphs []string, chunkSize, chunkOverlap int) []common.Chunk {
	var chunks []common.Chunk
	var currentChunk strings.Builder
	var chunkIndex int

	for _, paragraph := range paragraphs {
		// 检查当前段落是否超过 chunkSize
		if len(paragraph) > chunkSize {
			// 段落过长，单独切片
			if currentChunk.Len() > 0 {
				// 先保存当前 chunk
				chunks = append(chunks, s.createChunk(currentChunk.String(), chunkIndex))
				chunkIndex++
				currentChunk.Reset()
			}

			// 切片长段落
			longChunks := s.splitLongParagraph(paragraph, chunkIndex, chunkSize, chunkOverlap)
			chunks = append(chunks, longChunks...)
			chunkIndex += len(longChunks)
		} else {
			// 检查添加当前段落后是否超过 chunkSize
			if currentChunk.Len()+len(paragraph)+1 > chunkSize {
				// 保存当前 chunk
				chunks = append(chunks, s.createChunk(currentChunk.String(), chunkIndex))
				chunkIndex++

				// 开始新 chunk，添加重叠部分
				if currentChunk.Len() > chunkOverlap {
					overlap := currentChunk.String()[currentChunk.Len()-chunkOverlap:]
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
		chunks = append(chunks, s.createChunk(currentChunk.String(), chunkIndex))
	}

	return chunks
}

// splitLongParagraph 切片长段落
func (s *StructuralSplitter) splitLongParagraph(paragraph string, startIndex, chunkSize, chunkOverlap int) []common.Chunk {
	var chunks []common.Chunk
	var chunkIndex int = startIndex

	for i := 0; i < len(paragraph); i += chunkSize - chunkOverlap {
		end := i + chunkSize
		if end > len(paragraph) {
			end = len(paragraph)
		}

		chunkText := paragraph[i:end]
		chunks = append(chunks, s.createChunk(chunkText, chunkIndex))
		chunkIndex++
	}

	return chunks
}

// createChunk 创建切片
func (s *StructuralSplitter) createChunk(content string, index int) common.Chunk {
	return common.Chunk{
		ID:         uuid.New().String(),
		Content:    content,
		Metadata:   map[string]interface{}{
			"splitter": "structural",
			"type":     "paragraph",
		},
		Index:      index,
		TokenCount: len([]rune(content)),
	}
}
