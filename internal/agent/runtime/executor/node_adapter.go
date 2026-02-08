package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/compose"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
)

// NodeAdapter 将 TaskNode 转为 eino DAG 节点（闭包持有 task、agent 及依赖）
type NodeAdapter interface {
	ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error)
}

// LLMGen 执行 LLM 生成（由应用层注入）
type LLMGen interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// ToolExec 执行单工具调用（由应用层注入）
type ToolExec interface {
	Execute(ctx context.Context, toolName string, input map[string]any) (string, error)
}

// WorkflowExec 执行工作流（由应用层注入，如 Engine.ExecuteWorkflow）
type WorkflowExec interface {
	ExecuteWorkflow(ctx context.Context, name string, params map[string]any) (interface{}, error)
}

// LLMNodeAdapter 将 llm 型 TaskNode 转为 DAG 节点
type LLMNodeAdapter struct {
	LLM LLMGen
}

// ToDAGNode 实现 NodeAdapter
func (a *LLMNodeAdapter) ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error) {
	if a.LLM == nil {
		return nil, fmt.Errorf("LLMNodeAdapter: LLM 未配置")
	}
	taskID := task.ID
	cfg := task.Config
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		prompt := p.Goal
		if cfg != nil {
			if g, ok := cfg["goal"].(string); ok && g != "" {
				prompt = g
			}
		}
		resp, err := a.LLM.Generate(ctx, prompt)
		if err != nil {
			return nil, err
		}
		if p.Results == nil {
			p.Results = make(map[string]any)
		}
		p.Results[taskID] = resp
		if agent != nil && agent.Session != nil {
			agent.Session.AddMessage("assistant", resp)
		}
		return p, nil
	}), nil
}

// ToolNodeAdapter 将 tool 型 TaskNode 转为 DAG 节点
type ToolNodeAdapter struct {
	Tools ToolExec
}

// ToDAGNode 实现 NodeAdapter
func (a *ToolNodeAdapter) ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error) {
	if a.Tools == nil {
		return nil, fmt.Errorf("ToolNodeAdapter: Tools 未配置")
	}
	toolName := task.ToolName
	if toolName == "" {
		return nil, fmt.Errorf("ToolNodeAdapter: 节点 %s 缺少 tool_name", task.ID)
	}
	taskID := task.ID
	cfg := task.Config
	if cfg == nil {
		cfg = make(map[string]any)
	}
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		output, err := a.Tools.Execute(ctx, toolName, cfg)
		if err != nil {
			if p.Results == nil {
				p.Results = make(map[string]any)
			}
			p.Results[taskID] = map[string]any{"error": err.Error(), "at": time.Now()}
			return nil, err
		}
		if p.Results == nil {
			p.Results = make(map[string]any)
		}
		p.Results[taskID] = output
		if agent != nil && agent.Session != nil {
			agent.Session.AddMessage("tool", output)
		}
		return p, nil
	}), nil
}

// WorkflowNodeAdapter 将 workflow 型 TaskNode 转为 DAG 节点
type WorkflowNodeAdapter struct {
	Workflow WorkflowExec
}

// ToDAGNode 实现 NodeAdapter
func (a *WorkflowNodeAdapter) ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error) {
	if a.Workflow == nil {
		return nil, fmt.Errorf("WorkflowNodeAdapter: Workflow 未配置")
	}
	name := task.Workflow
	if name == "" {
		return nil, fmt.Errorf("WorkflowNodeAdapter: 节点 %s 缺少 workflow", task.ID)
	}
	taskID := task.ID
	params := task.Config
	if params == nil {
		params = make(map[string]any)
	}
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		result, err := a.Workflow.ExecuteWorkflow(ctx, name, params)
		if err != nil {
			if p.Results == nil {
				p.Results = make(map[string]any)
			}
			p.Results[taskID] = map[string]any{"error": err.Error(), "at": time.Now()}
			return nil, err
		}
		if p.Results == nil {
			p.Results = make(map[string]any)
		}
		p.Results[taskID] = result
		return p, nil
	}), nil
}
