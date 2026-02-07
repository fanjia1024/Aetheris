package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/tool"
)

// IngestTool 实现 knowledge.add_document：调用 ingest_pipeline
type IngestTool struct {
	engine *eino.Engine
}

// NewIngestTool 创建 knowledge.add_document 工具
func NewIngestTool(engine *eino.Engine) *IngestTool {
	return &IngestTool{engine: engine}
}

// Name 实现 tool.Tool
func (t *IngestTool) Name() string { return "knowledge.add_document" }

// Description 实现 tool.Tool
func (t *IngestTool) Description() string {
	return "将文档内容加入知识库：支持传入文本 content（或 path 文件路径），会进行解析、切片、向量化并写入索引。"
}

// Schema 实现 tool.Tool
func (t *IngestTool) Schema() tool.Schema {
	return tool.Schema{
		Type:        "object",
		Description: "入库参数",
		Properties: map[string]tool.SchemaProperty{
			"content": {Type: "string", Description: "文档文本内容"},
			"path":    {Type: "string", Description: "本地文件路径（与 content 二选一）"},
		},
		Required: []string{},
	}
}

// Execute 实现 tool.Tool
func (t *IngestTool) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	if t.engine == nil {
		return tool.ToolResult{Err: "engine 未配置"}, nil
	}
	var content []byte
	if s, ok := input["content"].(string); ok && s != "" {
		content = []byte(s)
	}
	if path, ok := input["path"].(string); ok && path != "" && len(content) == 0 {
		data, err := readFile(path)
		if err != nil {
			return tool.ToolResult{Err: fmt.Sprintf("读取文件失败: %v", err)}, nil
		}
		content = data
	}
	if len(content) == 0 {
		return tool.ToolResult{Err: "需要提供 content 或 path"}, nil
	}
	result, err := t.engine.ExecuteWorkflow(ctx, "ingest_pipeline", map[string]interface{}{
		"content": content,
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

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
