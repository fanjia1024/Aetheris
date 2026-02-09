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

package eino

import "context"

// Chunk 检索结果切片（供工具序列化，与 pipeline/common.Chunk 语义一致）
type Chunk struct {
	ID         string                 `json:"id"`
	Content    string                 `json:"content"`
	DocumentID string                `json:"document_id"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Retriever 检索器（供 qa_agent 工具调用）
type Retriever interface {
	Retrieve(ctx context.Context, query, collection string, topK int) ([]Chunk, error)
}

// Generator 生成器（供 qa_agent 工具调用，如 RAG 生成）
type Generator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// DocumentLoader 文档加载（供 ingest_agent 工具调用）
type DocumentLoader interface {
	Load(ctx context.Context, input interface{}) (interface{}, error)
}

// DocumentParser 文档解析
type DocumentParser interface {
	Parse(ctx context.Context, doc interface{}) (interface{}, error)
}

// DocumentSplitter 文档切片
type DocumentSplitter interface {
	Split(ctx context.Context, doc interface{}) (interface{}, error)
}

// DocumentEmbedding 文档向量化
type DocumentEmbedding interface {
	Embed(ctx context.Context, doc interface{}) (interface{}, error)
}

// DocumentIndexer 文档索引写入
type DocumentIndexer interface {
	Index(ctx context.Context, doc interface{}) (interface{}, error)
}
