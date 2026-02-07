package splitter

import (
	"context"
	"math"
	"strings"

	"github.com/google/uuid"
	"rag-platform/internal/pipeline/common"
)

// TextEmbedder 用于切片的文本向量化接口，与 internal/model/embedding.Embedder 同签名
type TextEmbedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

// SemanticSplitter 语义切片器
type SemanticSplitter struct {
	name     string
	embedder TextEmbedder
}

// NewSemanticSplitter 创建新的语义切片器；embedder 可为 nil，此时语义逻辑降级为按长度/overlap 合并
func NewSemanticSplitter(embedder TextEmbedder) *SemanticSplitter {
	return &SemanticSplitter{
		name:     "semantic_splitter",
		embedder: embedder,
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

// mergeBySemantics 基于语义合并句子：相似度低于 threshold 时断块，同时遵守 chunkSize/chunkOverlap
func (s *SemanticSplitter) mergeBySemantics(sentences []string, chunkSize, chunkOverlap int, semanticThreshold float64) []common.Chunk {
	var chunks []common.Chunk
	var currentChunk strings.Builder
	var lastSentenceInChunk string
	chunkIndex := 0
	ctx := context.Background()

	for i, sentence := range sentences {
		// 若有 embedder，检查当前 chunk 最后一句与下一句的语义相似度，低于阈值则先断块
		if s.embedder != nil && lastSentenceInChunk != "" {
			sim, err := s.calculateSemanticSimilarity(ctx, lastSentenceInChunk, sentence)
			if err == nil && sim < semanticThreshold {
				if currentChunk.Len() > 0 {
					chunks = append(chunks, s.createChunk(currentChunk.String(), chunkIndex))
					chunkIndex++
					if currentChunk.Len() > chunkOverlap {
						cur := currentChunk.String()
						overlap := cur[len(cur)-chunkOverlap:]
						currentChunk.Reset()
						currentChunk.WriteString(overlap)
						currentChunk.WriteString(" ")
					} else {
						currentChunk.Reset()
					}
					lastSentenceInChunk = ""
				}
			}
		}

		// 检查添加当前句子后是否超过 chunkSize
		if currentChunk.Len()+len(sentence)+1 > chunkSize {
			if currentChunk.Len() > 0 {
				chunks = append(chunks, s.createChunk(currentChunk.String(), chunkIndex))
				chunkIndex++
				if currentChunk.Len() > chunkOverlap {
					cur := currentChunk.String()
					overlap := cur[len(cur)-chunkOverlap:]
					currentChunk.Reset()
					currentChunk.WriteString(overlap)
					currentChunk.WriteString(" ")
				} else {
					currentChunk.Reset()
				}
				lastSentenceInChunk = ""
			}
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
		}
		currentChunk.WriteString(sentence)
		lastSentenceInChunk = sentence
		_ = i
	}

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

// calculateSemanticSimilarity 计算语义相似度；embedder 为 nil 时返回 0.5
func (s *SemanticSplitter) calculateSemanticSimilarity(ctx context.Context, text1, text2 string) (float64, error) {
	if s.embedder == nil || text1 == "" || text2 == "" {
		return 0.5, nil
	}
	vecs, err := s.embedder.Embed(ctx, []string{text1, text2})
	if err != nil || len(vecs) != 2 || len(vecs[0]) == 0 || len(vecs[1]) == 0 {
		return 0.5, nil
	}
	return cosineSimilarity(vecs[0], vecs[1]), nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	v := dot / (math.Sqrt(normA) * math.Sqrt(normB))
	if v < -1 {
		v = -1
	}
	if v > 1 {
		v = 1
	}
	return v
}
