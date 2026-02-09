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
