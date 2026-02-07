package builtin

import (
	"context"
	"encoding/json"

	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/tool"
)

// WorkflowTool 实现 workflow.run：按名称执行已注册工作流
type WorkflowTool struct {
	engine *eino.Engine
}

// NewWorkflowTool 创建 workflow.run 工具
func NewWorkflowTool(engine *eino.Engine) *WorkflowTool {
	return &WorkflowTool{engine: engine}
}

// Name 实现 tool.Tool
func (t *WorkflowTool) Name() string { return "workflow.run" }

// Description 实现 tool.Tool
func (t *WorkflowTool) Description() string {
	return "按名称执行已注册的工作流，如 ingest_pipeline、query_pipeline 等。传入 name 和 params。"
}

// Schema 实现 tool.Tool
func (t *WorkflowTool) Schema() tool.Schema {
	return tool.Schema{
		Type:        "object",
		Description: "工作流执行参数",
		Properties: map[string]tool.SchemaProperty{
			"name":   {Type: "string", Description: "工作流名称，如 query_pipeline、ingest_pipeline"},
			"params": {Type: "object", Description: "工作流参数字典"},
		},
		Required: []string{"name"},
	}
}

// Execute 实现 tool.Tool
func (t *WorkflowTool) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	if t.engine == nil {
		return tool.ToolResult{Err: "engine 未配置"}, nil
	}
	name, _ := input["name"].(string)
	if name == "" {
		return tool.ToolResult{Err: "name 不能为空"}, nil
	}
	var params map[string]interface{}
	if p, ok := input["params"]; ok && p != nil {
		if m, ok := p.(map[string]interface{}); ok {
			params = m
		}
		if m, ok := p.(map[string]any); ok {
			params = make(map[string]interface{})
			for k, v := range m {
				params[k] = v
			}
		}
	}
	if params == nil {
		params = make(map[string]interface{})
	}
	result, err := t.engine.ExecuteWorkflow(ctx, name, params)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}
	out, err := json.Marshal(result)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}
	return tool.ToolResult{Content: string(out)}, nil
}
