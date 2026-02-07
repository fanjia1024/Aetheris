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
	if e.logger != nil {
		e.logger.Info("执行 ingest_pipeline")
	}

	file, _ := params["file"].(*multipart.FileHeader)
	if file == nil {
		return nil, fmt.Errorf("ingest_pipeline 需要 params[\"file\"] 为 *multipart.FileHeader")
	}

	pipeCtx := common.NewPipelineContext(ctx, fmt.Sprintf("ingest-%d", time.Now().UnixNano()))
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
	out, err := e.loader.Execute(pipeCtx, file)
	if err != nil {
		return nil, fmt.Errorf("ingest loader: %w", err)
	}
	doc, ok := out.(*common.Document)
	if !ok {
		return nil, fmt.Errorf("ingest loader 未返回 *common.Document")
	}

	// parser
	out, err = e.parser.Execute(pipeCtx, doc)
	if err != nil {
		return nil, fmt.Errorf("ingest parser: %w", err)
	}
	doc, ok = out.(*common.Document)
	if !ok {
		return nil, fmt.Errorf("ingest parser 未返回 *common.Document")
	}

	// splitter
	out, err = e.splitter.Execute(pipeCtx, doc)
	if err != nil {
		return nil, fmt.Errorf("ingest splitter: %w", err)
	}
	doc, ok = out.(*common.Document)
	if !ok {
		return nil, fmt.Errorf("ingest splitter 未返回 *common.Document")
	}

	// embedding（可选）
	if e.embedding != nil {
		out, err = e.embedding.Execute(pipeCtx, doc)
		if err != nil {
			return nil, fmt.Errorf("ingest embedding: %w", err)
		}
		doc, ok = out.(*common.Document)
		if !ok {
			return nil, fmt.Errorf("ingest embedding 未返回 *common.Document")
		}
	}

	// indexer（可选）
	if e.indexer != nil {
		out, err = e.indexer.Execute(pipeCtx, doc)
		if err != nil {
			return nil, fmt.Errorf("ingest indexer: %w", err)
		}
		doc, ok = out.(*common.Document)
		if !ok {
			return nil, fmt.Errorf("ingest indexer 未返回 *common.Document")
		}
	}

	return map[string]interface{}{
		"status":   "success",
		"doc_id":   doc.ID,
		"chunks":   len(doc.Chunks),
		"metadata": params["metadata"],
	}, nil
}

// queryWorkflowExecutor 执行 query 工作流（可注入 retriever + generator + queryEmbedder）
type queryWorkflowExecutor struct {
	retriever     *query.Retriever
	generator     *query.Generator
	queryEmbedder *embedding.Embedder
	logger        *log.Logger
}

// NewQueryWorkflowExecutor 创建可执行的 query 工作流（由 app 装配后注册到 Engine）
func NewQueryWorkflowExecutor(retriever *query.Retriever, generator *query.Generator, queryEmbedder *embedding.Embedder, logger *log.Logger) WorkflowExecutor {
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
		return nil, fmt.Errorf("query 无向量且未配置 queryEmbedder，无法检索")
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
		"query":           q,
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
		"status":   "success",
		"query_id": q.ID,
		"answer":   genResult.Answer,
		"references": genResult.References,
		"process_time_ms": genResult.ProcessTime.Milliseconds(),
	}, nil
}
