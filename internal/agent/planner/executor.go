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

package planner

import (
	"context"
	"fmt"
	"time"
)

// ToolRunner 执行单工具调用（由 agent/tools.Registry 等适配）
type ToolRunner interface {
	Execute(ctx context.Context, toolName string, input map[string]any) (output string, err error)
}

// WorkflowRunner 执行工作流（如 eino Engine）
type WorkflowRunner interface {
	ExecuteWorkflow(ctx context.Context, name string, params map[string]any) (interface{}, error)
}

// LLMRunner 执行 LLM 生成
type LLMRunner interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// TaskGraphExecutor 将 TaskGraph 按节点顺序执行，映射到 tools / workflow / llm
type TaskGraphExecutor struct {
	Tools    ToolRunner
	Workflow WorkflowRunner
	LLM      LLMRunner
}

// NewTaskGraphExecutor 创建 TaskGraph 执行器
func NewTaskGraphExecutor(tools ToolRunner, workflow WorkflowRunner, llm LLMRunner) *TaskGraphExecutor {
	return &TaskGraphExecutor{Tools: tools, Workflow: workflow, LLM: llm}
}

// Execute 按 Nodes 顺序执行图（后续可依 Edges 做拓扑排序）
func (e *TaskGraphExecutor) Execute(ctx context.Context, graph *TaskGraph) ([]TaskResult, error) {
	if graph == nil || len(graph.Nodes) == 0 {
		return nil, nil
	}
	results := make([]TaskResult, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		res := e.executeNode(ctx, node)
		results = append(results, res)
		if res.Err != "" {
			break
		}
	}
	return results, nil
}

func (e *TaskGraphExecutor) executeNode(ctx context.Context, node TaskNode) TaskResult {
	out := TaskResult{NodeID: node.ID, At: time.Now()}
	switch node.Type {
	case NodeTool:
		if e.Tools == nil {
			out.Err = "ToolRunner 未配置"
			return out
		}
		input := node.Config
		if input == nil {
			input = make(map[string]any)
		}
		if node.ToolName != "" {
			output, err := e.Tools.Execute(ctx, node.ToolName, input)
			if err != nil {
				out.Err = err.Error()
				return out
			}
			out.Output = output
		} else {
			out.Err = "节点缺少 tool_name"
		}
	case NodeWorkflow:
		if e.Workflow == nil {
			out.Err = "WorkflowRunner 未配置"
			return out
		}
		params := node.Config
		if params == nil {
			params = make(map[string]any)
		}
		result, err := e.Workflow.ExecuteWorkflow(ctx, node.Workflow, params)
		if err != nil {
			out.Err = err.Error()
			return out
		}
		out.Output = fmt.Sprint(result)
	case NodeLLM:
		if e.LLM == nil {
			out.Err = "LLMRunner 未配置"
			return out
		}
		prompt := ""
		if node.Config != nil {
			if p, ok := node.Config["goal"].(string); ok {
				prompt = p
			}
		}
		text, err := e.LLM.Generate(ctx, prompt)
		if err != nil {
			out.Err = err.Error()
			return out
		}
		out.Output = text
	default:
		out.Err = "未知节点类型: " + node.Type
	}
	return out
}
