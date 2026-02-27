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

package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/tool"
)

// RAGSearchTool 实现 knowledge.search：调用 query_pipeline
type RAGSearchTool struct {
	engine *eino.Engine
}

// NewRAGSearchTool 创建 knowledge.search 工具
func NewRAGSearchTool(engine *eino.Engine) *RAGSearchTool {
	return &RAGSearchTool{engine: engine}
}

// Name 实现 tool.Tool
func (t *RAGSearchTool) Name() string { return "knowledge.search" }

// Description 实现 tool.Tool
func (t *RAGSearchTool) Description() string {
	return "在知识库中检索与问题相关的文档片段并返回检索结果，可用于 RAG 问答。"
}

// Schema 实现 tool.Tool
func (t *RAGSearchTool) Schema() tool.Schema {
	return tool.Schema{
		Type:        "object",
		Description: "检索参数",
		Properties: map[string]tool.SchemaProperty{
			"query":      {Type: "string", Description: "检索问题或关键词"},
			"collection": {Type: "string", Description: "集合名，默认 default"},
			"top_k":      {Type: "integer", Description: "返回条数，默认 10"},
		},
		Required: []string{"query"},
	}
}

// Execute 实现 tool.Tool
func (t *RAGSearchTool) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	if t.engine == nil {
		return tool.ToolResult{Err: "engine not configured"}, nil
	}
	queryText, _ := input["query"].(string)
	if queryText == "" {
		return tool.ToolResult{Err: "query is required"}, nil
	}
	topK := 10
	if k, ok := input["top_k"]; ok {
		switch v := k.(type) {
		case int:
			topK = v
		case float64:
			topK = int(v)
		}
	}
	if topK <= 0 {
		topK = 10
	}
	q := &common.Query{
		ID:        fmt.Sprintf("query-%d", time.Now().UnixNano()),
		Text:      queryText,
		CreatedAt: time.Now(),
	}
	result, err := t.engine.ExecuteWorkflow(ctx, "query_pipeline", map[string]interface{}{
		"query": q,
		"top_k": topK,
	})
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}
	out, err := json.Marshal(result)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}
	return tool.ToolResult{Content: string(out)}, nil
}
