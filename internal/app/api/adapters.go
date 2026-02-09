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

package api

import (
	"context"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/pipeline/ingest"
	"rag-platform/internal/pipeline/query"
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/storage/vector"
)

// Embedder 用于查询向量化的接口（与 model/embedding.Embedder 一致）
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

// retrieverAdapter 将 query.Retriever + Embedder 适配为 eino.Retriever
type retrieverAdapter struct {
	embedder     Embedder
	vectorStore  vector.Store
	scoreThresh  float64
}

// NewRetrieverAdapter 创建检索适配器（每次 Retrieve 使用传入的 collection/topK 创建临时 Retriever）
func NewRetrieverAdapter(embedder Embedder, vectorStore vector.Store, scoreThreshold float64) eino.Retriever {
	if scoreThreshold <= 0 {
		scoreThreshold = 0.3
	}
	return &retrieverAdapter{embedder: embedder, vectorStore: vectorStore, scoreThresh: scoreThreshold}
}

// Retrieve 实现 eino.Retriever
func (a *retrieverAdapter) Retrieve(ctx context.Context, queryText, collection string, topK int) ([]eino.Chunk, error) {
	if a.embedder == nil || a.vectorStore == nil {
		return nil, nil
	}
	vecs, err := a.embedder.Embed(ctx, []string{queryText})
	if err != nil || len(vecs) == 0 {
		return nil, err
	}
	r := query.NewRetriever(a.vectorStore, collection, topK, a.scoreThresh)
	q := &common.Query{Text: queryText, Embedding: vecs[0]}
	result, err := r.ProcessQuery(q)
	if err != nil {
		return nil, err
	}
	chunks := make([]eino.Chunk, len(result.Chunks))
	for i, c := range result.Chunks {
		chunks[i] = eino.Chunk{
			ID:         c.ID,
			Content:    c.Content,
			DocumentID: c.DocumentID,
			Metadata:   c.Metadata,
		}
	}
	return chunks, nil
}

// ragGeneratorAdapter 将 query.Generator + eino.Retriever 适配为 eino.Generator（RAG）
type ragGeneratorAdapter struct {
	retriever eino.Retriever
	generator *query.Generator
	embedder  Embedder
}

// NewRAGGeneratorAdapter 创建 RAG 生成适配器
func NewRAGGeneratorAdapter(retriever eino.Retriever, generator *query.Generator, embedder Embedder) eino.Generator {
	return &ragGeneratorAdapter{retriever: retriever, generator: generator, embedder: embedder}
}

// Generate 实现 eino.Generator：先检索再生成
func (a *ragGeneratorAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	if a.retriever == nil || a.generator == nil {
		return "", nil
	}
	chunks, err := a.retriever.Retrieve(ctx, prompt, "default", 10)
	if err != nil {
		return "", err
	}
	// 转为 common.RetrievalResult
	commonChunks := make([]common.Chunk, len(chunks))
	scores := make([]float64, len(chunks))
	for i, c := range chunks {
		commonChunks[i] = common.Chunk{ID: c.ID, Content: c.Content, DocumentID: c.DocumentID, Metadata: c.Metadata}
		scores[i] = 1.0
	}
	result := &common.RetrievalResult{Chunks: commonChunks, Scores: scores, TotalCount: len(commonChunks)}
	// 构建 Query（需 embedding 供 generator 内部使用）
	var emb []float64
	if a.embedder != nil {
		vecs, _ := a.embedder.Embed(ctx, []string{prompt})
		if len(vecs) > 0 {
			emb = vecs[0]
		}
	}
	q := &common.Query{Text: prompt, Embedding: emb}
	genResult, err := a.generator.GenerateWithRetrieval(q, result)
	if err != nil {
		return "", err
	}
	return genResult.Answer, nil
}

// loaderAdapter 将 ingest.DocumentLoader 适配为 eino.DocumentLoader
type loaderAdapter struct {
	loader *ingest.DocumentLoader
}

func NewLoaderAdapter(loader *ingest.DocumentLoader) eino.DocumentLoader {
	return &loaderAdapter{loader: loader}
}

func (a *loaderAdapter) Load(ctx context.Context, input interface{}) (interface{}, error) {
	pc := common.NewPipelineContext(ctx, "eino")
	return a.loader.Execute(pc, input)
}

// parserAdapter 将 ingest.DocumentParser 适配为 eino.DocumentParser
type parserAdapter struct {
	parser *ingest.DocumentParser
}

func NewParserAdapter(parser *ingest.DocumentParser) eino.DocumentParser {
	return &parserAdapter{parser: parser}
}

func (a *parserAdapter) Parse(ctx context.Context, doc interface{}) (interface{}, error) {
	pc := common.NewPipelineContext(ctx, "eino")
	return a.parser.Execute(pc, doc)
}

// splitterAdapter 将 ingest.DocumentSplitter 适配为 eino.DocumentSplitter
type splitterAdapter struct {
	splitter *ingest.DocumentSplitter
}

func NewSplitterAdapter(splitter *ingest.DocumentSplitter) eino.DocumentSplitter {
	return &splitterAdapter{splitter: splitter}
}

func (a *splitterAdapter) Split(ctx context.Context, doc interface{}) (interface{}, error) {
	pc := common.NewPipelineContext(ctx, "eino")
	return a.splitter.Execute(pc, doc)
}

// embeddingAdapter 将 ingest.DocumentEmbedding 适配为 eino.DocumentEmbedding
type embeddingAdapter struct {
	embedding *ingest.DocumentEmbedding
}

func NewEmbeddingAdapter(embedding *ingest.DocumentEmbedding) eino.DocumentEmbedding {
	return &embeddingAdapter{embedding: embedding}
}

func (a *embeddingAdapter) Embed(ctx context.Context, doc interface{}) (interface{}, error) {
	pc := common.NewPipelineContext(ctx, "eino")
	return a.embedding.Execute(pc, doc)
}

// indexerAdapter 将 ingest.DocumentIndexer 适配为 eino.DocumentIndexer
type indexerAdapter struct {
	indexer *ingest.DocumentIndexer
}

func NewIndexerAdapter(indexer *ingest.DocumentIndexer) eino.DocumentIndexer {
	return &indexerAdapter{indexer: indexer}
}

func (a *indexerAdapter) Index(ctx context.Context, doc interface{}) (interface{}, error) {
	pc := common.NewPipelineContext(ctx, "eino")
	return a.indexer.Execute(pc, doc)
}
