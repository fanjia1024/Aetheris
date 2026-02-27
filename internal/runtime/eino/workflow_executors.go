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

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"

	"rag-platform/internal/model/embedding"
	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/pipeline/ingest"
	"rag-platform/internal/pipeline/query"
	"rag-platform/pkg/log"
)

// ingestWorkflowExecutor 执行 ingest 工作流：loader → parser → splitter → [embedding] → [indexer]
type ingestWorkflowExecutor struct {
	loader    *ingest.DocumentLoader
	parser    *ingest.DocumentParser
	splitter  *ingest.DocumentSplitter
	embedding *ingest.DocumentEmbedding
	indexer   *ingest.DocumentIndexer
	logger    *log.Logger
}

// NewIngestWorkflowExecutor 创建可执行的 ingest 工作流（由 app 装配后注册到 Engine）
func NewIngestWorkflowExecutor(loader *ingest.DocumentLoader, parser *ingest.DocumentParser, splitter *ingest.DocumentSplitter, embedding *ingest.DocumentEmbedding, indexer *ingest.DocumentIndexer, logger *log.Logger) WorkflowExecutor {
	return &ingestWorkflowExecutor{
		loader:    loader,
		parser:    parser,
		splitter:  splitter,
		embedding: embedding,
		indexer:   indexer,
		logger:    logger,
	}
}

// Execute 实现 WorkflowExecutor
// 请求 context 已带 HTTP 层 span，可在此处用 otel trace.SpanFromContext(ctx) 为 loader/parser/splitter/embedding/indexer 创建子 span 以细化链路。
func (e *ingestWorkflowExecutor) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	ingestID := fmt.Sprintf("ingest-%d", time.Now().UnixNano())
	if e.logger != nil {
		e.logger.Info("ingest_pipeline 开始", "ingest_id", ingestID)
	}

	var loaderInput interface{}
	if content, ok := params["content"].([]byte); ok {
		loaderInput = content
	} else if file, ok := params["file"].(*multipart.FileHeader); ok {
		loaderInput = file
	} else {
		return nil, fmt.Errorf("ingest_pipeline 需要 params[\"file\"] (*multipart.FileHeader) 或 params[\"content\"] ([]byte)")
	}

	pipeCtx := common.NewPipelineContext(ctx, ingestID)
	if e.loader == nil {
		e.loader = ingest.NewDocumentLoader()
	}
	if e.parser == nil {
		e.parser = ingest.NewDocumentParser()
	}
	if e.splitter == nil {
		e.splitter = ingest.NewDocumentSplitter(1000, 100, 1000)
	}

	// loader
	if e.logger != nil {
		e.logger.Info("ingest 阶段开始", "ingest_id", ingestID, "ingest_step", "loader")
	}
	loaderStart := time.Now()
	out, err := e.loader.Execute(pipeCtx, loaderInput)
	if err != nil {
		if e.logger != nil {
			e.logger.Error("ingest 阶段failed", "ingest_id", ingestID, "ingest_step", "loader", "error", err)
		}
		return nil, fmt.Errorf("ingest loader: %w", err)
	}
	doc, ok := out.(*common.Document)
	if !ok {
		return nil, fmt.Errorf("ingest loader 未返回 *common.Document")
	}
	if e.logger != nil {
		e.logger.Info("ingest 阶段完成", "ingest_id", ingestID, "ingest_step", "loader", "doc_id", doc.ID, "chunks", len(doc.Chunks), "duration_ms", time.Since(loaderStart).Milliseconds())
	}

	// parser
	if e.logger != nil {
		e.logger.Info("ingest 阶段开始", "ingest_id", ingestID, "ingest_step", "parser")
	}
	parserStart := time.Now()
	out, err = e.parser.Execute(pipeCtx, doc)
	if err != nil {
		if e.logger != nil {
			e.logger.Error("ingest 阶段failed", "ingest_id", ingestID, "ingest_step", "parser", "doc_id", doc.ID, "error", err)
		}
		return nil, fmt.Errorf("ingest parser: %w", err)
	}
	doc, ok = out.(*common.Document)
	if !ok {
		return nil, fmt.Errorf("ingest parser 未返回 *common.Document")
	}
	if e.logger != nil {
		e.logger.Info("ingest 阶段完成", "ingest_id", ingestID, "ingest_step", "parser", "doc_id", doc.ID, "chunks", len(doc.Chunks), "duration_ms", time.Since(parserStart).Milliseconds())
	}

	// splitter
	if e.logger != nil {
		e.logger.Info("ingest 阶段开始", "ingest_id", ingestID, "ingest_step", "splitter")
	}
	splitterStart := time.Now()
	out, err = e.splitter.Execute(pipeCtx, doc)
	if err != nil {
		if e.logger != nil {
			e.logger.Error("ingest 阶段failed", "ingest_id", ingestID, "ingest_step", "splitter", "doc_id", doc.ID, "error", err)
		}
		return nil, fmt.Errorf("ingest splitter: %w", err)
	}
	doc, ok = out.(*common.Document)
	if !ok {
		return nil, fmt.Errorf("ingest splitter 未返回 *common.Document")
	}
	if e.logger != nil {
		e.logger.Info("ingest 阶段完成", "ingest_id", ingestID, "ingest_step", "splitter", "doc_id", doc.ID, "chunks", len(doc.Chunks), "duration_ms", time.Since(splitterStart).Milliseconds())
	}

	// embedding（可选）
	if e.embedding != nil {
		if e.logger != nil {
			e.logger.Info("ingest 阶段开始", "ingest_id", ingestID, "ingest_step", "embedding")
		}
		embedStart := time.Now()
		out, err = e.embedding.Execute(pipeCtx, doc)
		if err != nil {
			if e.logger != nil {
				e.logger.Error("ingest 阶段failed", "ingest_id", ingestID, "ingest_step", "embedding", "doc_id", doc.ID, "error", err)
			}
			return nil, fmt.Errorf("ingest embedding: %w", err)
		}
		doc, ok = out.(*common.Document)
		if !ok {
			return nil, fmt.Errorf("ingest embedding 未返回 *common.Document")
		}
		if e.logger != nil {
			e.logger.Info("ingest 阶段完成", "ingest_id", ingestID, "ingest_step", "embedding", "doc_id", doc.ID, "chunks", len(doc.Chunks), "duration_ms", time.Since(embedStart).Milliseconds())
		}
	}

	// indexer（可选）
	if e.indexer != nil {
		if e.logger != nil {
			e.logger.Info("ingest 阶段开始", "ingest_id", ingestID, "ingest_step", "indexer")
		}
		indexerStart := time.Now()
		out, err = e.indexer.Execute(pipeCtx, doc)
		if err != nil {
			if e.logger != nil {
				e.logger.Error("ingest 阶段failed", "ingest_id", ingestID, "ingest_step", "indexer", "doc_id", doc.ID, "error", err)
			}
			return nil, fmt.Errorf("ingest indexer: %w", err)
		}
		doc, ok = out.(*common.Document)
		if !ok {
			return nil, fmt.Errorf("ingest indexer 未返回 *common.Document")
		}
		if e.logger != nil {
			e.logger.Info("ingest 阶段完成", "ingest_id", ingestID, "ingest_step", "indexer", "doc_id", doc.ID, "chunks", len(doc.Chunks), "duration_ms", time.Since(indexerStart).Milliseconds())
		}
	}

	if e.logger != nil {
		e.logger.Info("ingest_pipeline 完成", "ingest_id", ingestID, "doc_id", doc.ID, "chunks", len(doc.Chunks))
	}
	return map[string]interface{}{
		"status":   "success",
		"doc_id":   doc.ID,
		"chunks":   len(doc.Chunks),
		"metadata": params["metadata"],
	}, nil
}

// QueryRetrieverForWorkflow 供 query 工作流使用的检索器（*query.Retriever 或 Eino Retriever 适配器均实现此接口）
type QueryRetrieverForWorkflow interface {
	SetTopK(topK int)
	Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error)
}

// queryWorkflowExecutor 执行 query 工作流（可注入 retriever + generator + queryEmbedder）
type queryWorkflowExecutor struct {
	retriever     QueryRetrieverForWorkflow
	generator     *query.Generator
	queryEmbedder *embedding.Embedder
	logger        *log.Logger
}

// NewQueryWorkflowExecutor 创建可执行的 query 工作流（由 app 装配后注册到 Engine）
func NewQueryWorkflowExecutor(retriever QueryRetrieverForWorkflow, generator *query.Generator, queryEmbedder *embedding.Embedder, logger *log.Logger) WorkflowExecutor {
	return &queryWorkflowExecutor{
		retriever:     retriever,
		generator:     generator,
		queryEmbedder: queryEmbedder,
		logger:        logger,
	}
}

// Execute 实现 WorkflowExecutor：embed query（若需）→ retriever → generator
// 请求 context 已带 HTTP 层 span，可在此处为 retrieve/generate 等步骤创建子 span 以细化链路。
func (e *queryWorkflowExecutor) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if e.logger != nil {
		e.logger.Info("执行 query_pipeline")
	}

	q, _ := params["query"].(*common.Query)
	topK := 10
	if k, ok := params["top_k"].(int); ok && k > 0 {
		topK = k
	}

	// 未注入 retriever/generator 或无 query 时返回占位
	if e.retriever == nil || e.generator == nil {
		if q != nil {
			return map[string]interface{}{
				"status":   "success",
				"query_id": q.ID,
				"answer":   "Query pipeline placeholder (wire retriever+generator in app).",
			}, nil
		}
		return map[string]interface{}{
			"status": "success",
			"answer": "Query pipeline placeholder.",
		}, nil
	}
	if q == nil {
		return nil, fmt.Errorf("query_pipeline 需要 params[\"query\"] 为 *common.Query")
	}

	pipeCtx := common.NewPipelineContext(ctx, fmt.Sprintf("query-%d", time.Now().UnixNano()))

	// 若查询无向量则先做 query embedding
	if len(q.Embedding) == 0 && e.queryEmbedder != nil {
		vecs, err := e.queryEmbedder.Embed(ctx, []string{q.Text})
		if err != nil {
			return nil, fmt.Errorf("query embedding: %w", err)
		}
		if len(vecs) > 0 && len(vecs[0]) > 0 {
			q.Embedding = vecs[0]
		}
	}

	if len(q.Embedding) == 0 {
		return nil, fmt.Errorf("query has no embedding and queryEmbedder not configured, cannot retrieve")
	}

	// 可选：设置 retriever topK
	e.retriever.SetTopK(topK)

	// Retriever
	out, err := e.retriever.Execute(pipeCtx, q)
	if err != nil {
		return nil, fmt.Errorf("query retriever: %w", err)
	}
	retrievalResult, ok := out.(*common.RetrievalResult)
	if !ok {
		return nil, fmt.Errorf("retriever 未返回 *common.RetrievalResult")
	}

	// Generator
	genInput := map[string]interface{}{
		"query":            q,
		"retrieval_result": retrievalResult,
	}
	out, err = e.generator.Execute(pipeCtx, genInput)
	if err != nil {
		return nil, fmt.Errorf("query generator: %w", err)
	}
	genResult, ok := out.(*common.GenerationResult)
	if !ok {
		return nil, fmt.Errorf("generator 未返回 *common.GenerationResult")
	}

	return map[string]interface{}{
		"status":          "success",
		"query_id":        q.ID,
		"answer":          genResult.Answer,
		"references":      genResult.References,
		"process_time_ms": genResult.ProcessTime.Milliseconds(),
	}, nil
}
