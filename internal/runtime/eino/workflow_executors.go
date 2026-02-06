package eino

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/pipeline/ingest"
	"rag-platform/pkg/log"
)

// ingestWorkflowExecutor 执行 ingest 工作流：loader → parser → splitter（最小实现）
type ingestWorkflowExecutor struct {
	loader   *ingest.DocumentLoader
	parser   *ingest.DocumentParser
	splitter *ingest.DocumentSplitter
	logger   *log.Logger
}

// Execute 实现 WorkflowExecutor
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

	return map[string]interface{}{
		"status":   "success",
		"doc_id":   doc.ID,
		"chunks":   len(doc.Chunks),
		"metadata": params["metadata"],
	}, nil
}

// queryWorkflowExecutor 执行 query 工作流（占位：可后续由 app 注入带 retriever+generator 的实现）
type queryWorkflowExecutor struct {
	logger *log.Logger
}

// Execute 实现 WorkflowExecutor
func (e *queryWorkflowExecutor) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if e.logger != nil {
		e.logger.Info("执行 query_pipeline")
	}

	query, _ := params["query"].(*common.Query)
	topK := 10
	if k, ok := params["top_k"].(int); ok && k > 0 {
		topK = k
	}

	if query != nil {
		_ = topK
		return map[string]interface{}{
			"status":  "success",
			"query_id": query.ID,
			"answer":   "Query pipeline placeholder (wire retriever+generator in app).",
		}, nil
	}
	return map[string]interface{}{
		"status":  "success",
		"answer":  "Query pipeline placeholder.",
	}, nil
}
