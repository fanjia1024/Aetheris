package splitter

import (
	"strings"

	"github.com/google/uuid"
	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/model/embedding"
)

// SemanticSplitter 语义切片器
type SemanticSplitter struct {
	name      string
	embedder  *embedding.Embedder
}

// NewSemanticSplitter 创建新的语义切片器
func NewSemanticSplitter() *SemanticSplitter {
	return &SemanticSplitter{
		name: "semantic_splitter",
		// TODO: 初始化 embedder
	}
}

// Name 返回切片器名称
func (s *SemanticSplitter) Name() string {
	return s.name
}

// Split 执行语义切片
func (s *SemanticSplitter) Split(content string, options map[string]interface{}) ([]common.Chunk, error) {
	// 获取配置选项
	chunkSize := 1000
	chunkOverlap := 100
	semanticThreshold := 0.3

	if size, ok := options["chunk_size"].(int); ok && size > 0 {
		chunkSize = size
	}
	if overlap, ok := options["chunk_overlap"].(int); ok && overlap > 0 {
		chunkOverlap = overlap
	}
	if threshold, ok := options["semantic_threshold"].(float64); ok && threshold > 0 {
		semanticThreshold = threshold
	}

	// 按句子分割
	sentences := s.splitBySentence(content)

	// 基于语义合并句子
	chunks := s.mergeBySemantics(sentences, chunkSize, chunkOverlap, semanticThreshold)

	return chunks, nil
}

// splitBySentence 按句子分割
func (s *SemanticSplitter) splitBySentence(content string) []string {
	// 按中文和英文句子结束符分割
	sentenceEnders := []string{". ", "。", "! ", "！", "? ", "？", "\n"}
	content = content

	for _, ender := range sentenceEnders {
		content = strings.ReplaceAll(content, ender, "\n")
	}

	// 分割并清理
	lines := strings.Split(content, "\n")
	var sentences []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			sentences = append(sentences, trimmed)
		}
	}

	return sentences
}

// mergeBySemantics 基于语义合并句子
func (s *SemanticSplitter) mergeBySemantics(sentences []string, chunkSize, chunkOverlap int, semanticThreshold float64) []common.Chunk {
	var chunks []common.Chunk
	var currentChunk strings.Builder
	var chunkIndex int

	for i, sentence := range sentences {
		// 检查添加当前句子后是否超过 chunkSize
		if currentChunk.Len()+len(sentence)+1 > chunkSize {
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

		// 添加当前句子
		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
		}
		currentChunk.WriteString(sentence)
	}

	// 保存最后一个 chunk
	if currentChunk.Len() > 0 {
		chunks = append(chunks, s.createChunk(currentChunk.String(), chunkIndex))
	}

	return chunks
}

// createChunk 创建切片
func (s *SemanticSplitter) createChunk(content string, index int) common.Chunk {
	return common.Chunk{
		ID:         uuid.New().String(),
		Content:    content,
		Metadata:   map[string]interface{}{
			"splitter": "semantic",
			"type":     "semantic",
		},
		Index:      index,
		TokenCount: len([]rune(content)),
	}
}

// calculateSemanticSimilarity 计算语义相似度
// TODO: 实现语义相似度计算
func (s *SemanticSplitter) calculateSemanticSimilarity(text1, text2 string) (float64, error) {
	// 暂时返回模拟值
	return 0.5, nil
}
