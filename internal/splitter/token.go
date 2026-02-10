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

package splitter

import (
	"strings"

	"github.com/google/uuid"
	"rag-platform/internal/pipeline/common"
)

// TokenSplitter Token 切片器
type TokenSplitter struct {
	name string
}

// NewTokenSplitter 创建新的 Token 切片器
func NewTokenSplitter() *TokenSplitter {
	return &TokenSplitter{
		name: "token_splitter",
	}
}

// Name 返回切片器名称
func (s *TokenSplitter) Name() string {
	return s.name
}

// Split 执行 Token 切片
func (s *TokenSplitter) Split(content string, options map[string]interface{}) ([]common.Chunk, error) {
	// 获取配置选项
	maxTokens := 512
	chunkOverlap := 100

	if tokens, ok := options["max_tokens"].(int); ok && tokens > 0 {
		maxTokens = tokens
	}
	if overlap, ok := options["chunk_overlap"].(int); ok && overlap > 0 {
		chunkOverlap = overlap
	}

	// 按 Token 分割
	chunks := s.splitByTokens(content, maxTokens, chunkOverlap)

	return chunks, nil
}

// splitByTokens 按 Token 分割
func (s *TokenSplitter) splitByTokens(content string, maxTokens, chunkOverlap int) []common.Chunk {
	// 简单的 Token 计数（实际应用中应使用真实的 Tokenizer）
	tokens := s.tokenize(content)
	var chunks []common.Chunk
	var currentTokens []string
	var chunkIndex int

	for _, token := range tokens {
		// 检查添加当前 Token 后是否超过最大 Token 数
		if len(currentTokens)+1 > maxTokens {
			// 保存当前 chunk
			chunkText := s.detokenize(currentTokens)
			chunks = append(chunks, s.createChunk(chunkText, chunkIndex))
			chunkIndex++

			// 开始新 chunk，添加重叠部分
			if len(currentTokens) > chunkOverlap {
				overlapTokens := currentTokens[len(currentTokens)-chunkOverlap:]
				currentTokens = overlapTokens
			} else {
				currentTokens = []string{}
			}
		}

		// 添加当前 Token
		currentTokens = append(currentTokens, token)
	}

	// 保存最后一个 chunk
	if len(currentTokens) > 0 {
		chunkText := s.detokenize(currentTokens)
		chunks = append(chunks, s.createChunk(chunkText, chunkIndex))
	}

	return chunks
}

// tokenize 简单的 Token 分词
func (s *TokenSplitter) tokenize(content string) []string {
	// 简单的空格分词（实际应用中应使用真实的 Tokenizer）
	return strings.Fields(content)
}

// detokenize 将 Token 转换回文本
func (s *TokenSplitter) detokenize(tokens []string) string {
	return strings.Join(tokens, " ")
}

// createChunk 创建切片
func (s *TokenSplitter) createChunk(content string, index int) common.Chunk {
	return common.Chunk{
		ID:      uuid.New().String(),
		Content: content,
		Metadata: map[string]interface{}{
			"splitter": "token",
			"type":     "token",
		},
		Index:      index,
		TokenCount: len(s.tokenize(content)),
	}
}
