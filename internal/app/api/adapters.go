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
	"fmt"
	"strconv"
	"time"

	einoretriever "github.com/cloudwego/eino/components/retriever"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/pipeline/ingest"
	"rag-platform/internal/pipeline/query"
	"rag-platform/internal/runtime/eino"
)

// Embedder 用于查询向量化的接口（与 model/embedding.Embedder 一致）
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

// retrieverAdapter 将 Eino retriever.Retriever 适配为 eino.Retriever（供 agent 使用）
type retrieverAdapter struct {
	einoRetriever einoretriever.Retriever
	einoEmbedder  *EinoEmbedderAdapter
	scoreThresh   float64
}

// NewRetrieverAdapter 创建检索适配器（基于 Eino retriever.Retriever，需传入 Embedder 用于 WithEmbedding）
func NewRetrieverAdapter(embedder Embedder, einoRetriever einoretriever.Retriever, scoreThreshold float64) eino.Retriever {
	if scoreThreshold <= 0 {
		scoreThreshold = 0.3
	}
	return &retrieverAdapter{
		einoRetriever: einoRetriever,
		einoEmbedder:  NewEinoEmbedderAdapter(embedder),
		scoreThresh:   scoreThreshold,
	}
}

// Retrieve 实现 eino.Retriever
func (a *retrieverAdapter) Retrieve(ctx context.Context, queryText, collection string, topK int) ([]eino.Chunk, error) {
	if a.einoRetriever == nil || a.einoEmbedder == nil {
		return nil, nil
	}
	opts := []einoretriever.Option{
		einoretriever.WithIndex(collection),
		einoretriever.WithTopK(topK),
		einoretriever.WithScoreThreshold(a.scoreThresh),
		einoretriever.WithEmbedding(a.einoEmbedder),
	}
	docs, err := a.einoRetriever.Retrieve(ctx, queryText, opts...)
	if err != nil {
		return nil, err
	}
	chunks := make([]eino.Chunk, len(docs))
	for i, d := range docs {
		docID := ""
		if d.MetaData != nil {
			if id, ok := d.MetaData["document_id"].(string); ok {
				docID = id
			}
		}
		chunks[i] = eino.Chunk{
			ID:         d.ID,
			Content:    d.Content,
			DocumentID: docID,
			Metadata:   d.MetaData,
		}
	}
	return chunks, nil
}

// EinoRetrieverQueryAdapter 将 Eino retriever.Retriever 适配为 query 工作流使用的检索器（实现 eino.QueryRetrieverForWorkflow）
type EinoRetrieverQueryAdapter struct {
	EinoRetriever einoretriever.Retriever
	Embedder      Embedder
	TopK          int
}

// SetTopK 设置返回结果数量
func (a *EinoRetrieverQueryAdapter) SetTopK(topK int) {
	if topK > 0 {
		a.TopK = topK
	}
}

// Execute 执行检索，返回 *common.RetrievalResult
func (a *EinoRetrieverQueryAdapter) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	q, ok := input.(*common.Query)
	if !ok {
		return nil, common.NewPipelineError("eino_retriever", "输入类型error", fmt.Errorf("expected *common.Query, got %T", input))
	}
	if q == nil {
		return nil, common.ErrInvalidInput
	}
	opts := []einoretriever.Option{
		einoretriever.WithTopK(a.TopK),
		einoretriever.WithEmbedding(NewEinoEmbedderAdapter(a.Embedder)),
	}
	var reqCtx context.Context
	if ctx != nil {
		reqCtx = ctx.Context
	} else {
		reqCtx = context.Background()
	}
	docs, err := a.EinoRetriever.Retrieve(reqCtx, q.Text, opts...)
	if err != nil {
		return nil, common.NewPipelineError("eino_retriever", "检索failed", err)
	}
	chunks := make([]common.Chunk, len(docs))
	scores := make([]float64, len(docs))
	for i, d := range docs {
		meta := make(map[string]interface{})
		if d.MetaData != nil {
			for k, v := range d.MetaData {
				meta[k] = v
			}
		}
		docID := ""
		if d.MetaData != nil {
			if id, ok := d.MetaData["document_id"].(string); ok {
				docID = id
			}
		}
		idx := 0
		if d.MetaData != nil && d.MetaData["index"] != nil {
			switch v := d.MetaData["index"].(type) {
			case int:
				idx = v
			case string:
				idx, _ = strconv.Atoi(v)
			}
		}
		tokenCount := 0
		if d.MetaData != nil && d.MetaData["token_count"] != nil {
			switch v := d.MetaData["token_count"].(type) {
			case int:
				tokenCount = v
			case string:
				tokenCount, _ = strconv.Atoi(v)
			}
		}
		chunks[i] = common.Chunk{
			ID:         d.ID,
			Content:    d.Content,
			Metadata:   meta,
			DocumentID: docID,
			Index:      idx,
			TokenCount: tokenCount,
		}
		scores[i] = 1.0
	}
	return &common.RetrievalResult{
		Chunks:      chunks,
		Scores:      scores,
		TotalCount:  len(chunks),
		ProcessTime: time.Duration(0),
	}, nil
}

// Ensure *EinoRetrieverQueryAdapter 实现 eino.QueryRetrieverForWorkflow
var _ eino.QueryRetrieverForWorkflow = (*EinoRetrieverQueryAdapter)(nil)

// ragGeneratorAdapter 将 query.Generator + eino.Retriever 适配为 eino.Generator（RAG）
type ragGeneratorAdapter struct {
	retriever         eino.Retriever
	generator         *query.Generator
	embedder          Embedder
	defaultCollection string // 默认检索集合名，空则 "default"
}

// NewRAGGeneratorAdapter 创建 RAG 生成适配器。defaultCollection 为空时使用 "default"。
func NewRAGGeneratorAdapter(retriever eino.Retriever, generator *query.Generator, embedder Embedder, defaultCollection string) eino.Generator {
	if defaultCollection == "" {
		defaultCollection = "default"
	}
	return &ragGeneratorAdapter{retriever: retriever, generator: generator, embedder: embedder, defaultCollection: defaultCollection}
}

// Generate 实现 eino.Generator：先检索再生成
func (a *ragGeneratorAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	if a.retriever == nil || a.generator == nil {
		return "", nil
	}
	collection := a.defaultCollection
	if collection == "" {
		collection = "default"
	}
	chunks, err := a.retriever.Retrieve(ctx, prompt, collection, 10)
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
