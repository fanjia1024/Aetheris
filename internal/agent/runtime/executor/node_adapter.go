package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/compose"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
)

// NodeRunner 单节点执行函数（用于 Steppable 执行与 node-level checkpoint）
type NodeRunner func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error)

// NodeAdapter 将 TaskNode 转为 eino DAG 节点（闭包持有 task、agent 及依赖）
type NodeAdapter interface {
	ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error)
	// ToNodeRunner 返回同签名的可执行函数，供 Steppable 逐节点执行与 checkpoint
	ToNodeRunner(task *planner.TaskNode, agent *runtime.Agent) (NodeRunner, error)
}

// LLMGen 执行 LLM 生成（由应用层注入）
type LLMGen interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// ToolExec 执行单工具调用（由应用层注入）；state 为再入时传入的上次状态，返回 ToolResult 支持 Done/State/Output
type ToolExec interface {
	Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (ToolResult, error)
}

// ToolResult 工具结果：未完成可挂起并携带 State 供再入
type ToolResult struct {
	Done   bool
	State  interface{}
	Output string
	Err    string
}

// WorkflowExec 执行工作流（由应用层注入，如 Engine.ExecuteWorkflow）
type WorkflowExec interface {
	ExecuteWorkflow(ctx context.Context, name string, params map[string]any) (interface{}, error)
}

// LLMNodeAdapter 将 llm 型 TaskNode 转为 DAG 节点
type LLMNodeAdapter struct {
	LLM LLMGen
}

func (a *LLMNodeAdapter) runNode(ctx context.Context, taskID string, cfg map[string]any, agent *runtime.Agent, p *AgentDAGPayload) (*AgentDAGPayload, error) {
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
}

// ToDAGNode 实现 NodeAdapter
func (a *LLMNodeAdapter) ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error) {
	if a.LLM == nil {
		return nil, fmt.Errorf("LLMNodeAdapter: LLM 未配置")
	}
	taskID, cfg := task.ID, task.Config
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, cfg, agent, p)
	}), nil
}

// ToNodeRunner 实现 NodeAdapter
func (a *LLMNodeAdapter) ToNodeRunner(task *planner.TaskNode, agent *runtime.Agent) (NodeRunner, error) {
	if a.LLM == nil {
		return nil, fmt.Errorf("LLMNodeAdapter: LLM 未配置")
	}
	taskID, cfg := task.ID, task.Config
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, cfg, agent, p)
	}, nil
}

// ToolNodeAdapter 将 tool 型 TaskNode 转为 DAG 节点
type ToolNodeAdapter struct {
	Tools          ToolExec
	ToolEventSink  ToolEventSink // 可选；Tool 执行前后写 ToolCalled/ToolReturned
}

func (a *ToolNodeAdapter) runNode(ctx context.Context, taskID, toolName string, cfg map[string]any, agent *runtime.Agent, p *AgentDAGPayload) (*AgentDAGPayload, error) {
	// 1.0 短路：Replay 已注入的节点结果不再执行真实 Tool，避免二次副作用
	if prev, ok := p.Results[taskID]; ok {
		if m, ok := prev.(map[string]any); ok {
			if _, hasDone := m["done"]; hasDone {
				return p, nil
			}
			if _, hasOut := m["output"]; hasOut {
				return p, nil
			}
		}
	}
	var state interface{}
	if prev, ok := p.Results[taskID]; ok {
		if m, ok := prev.(map[string]any); ok {
			state = m["state"]
		}
	}
	if a.ToolEventSink != nil {
		if jobID := JobIDFromContext(ctx); jobID != "" {
			inputBytes, _ := json.Marshal(cfg)
			_ = a.ToolEventSink.AppendToolCalled(ctx, jobID, taskID, toolName, inputBytes)
		}
	}
	result, err := a.Tools.Execute(ctx, toolName, cfg, state)
	if err != nil {
		if p.Results == nil {
			p.Results = make(map[string]any)
		}
		p.Results[taskID] = map[string]any{"error": err.Error(), "at": time.Now()}
		if a.ToolEventSink != nil {
			if jobID := JobIDFromContext(ctx); jobID != "" {
				outBytes, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
				_ = a.ToolEventSink.AppendToolReturned(ctx, jobID, taskID, outBytes)
			}
		}
		if agent != nil && agent.Session != nil {
			inputStr := ""
			if len(cfg) > 0 {
				b, _ := json.Marshal(cfg)
				inputStr = string(b)
			}
			agent.Session.AddToolCall(toolName, inputStr, "error: "+err.Error())
		}
		return nil, err
	}
	if p.Results == nil {
		p.Results = make(map[string]any)
	}
	p.Results[taskID] = map[string]any{
		"done": result.Done, "state": result.State, "output": result.Output, "error": result.Err,
	}
	if a.ToolEventSink != nil {
		if jobID := JobIDFromContext(ctx); jobID != "" {
			outBytes, _ := json.Marshal(map[string]interface{}{"output": result.Output, "error": result.Err, "done": result.Done})
			_ = a.ToolEventSink.AppendToolReturned(ctx, jobID, taskID, outBytes)
		}
	}
	msg := result.Output
	if result.Err != "" {
		msg = "error: " + result.Err
	}
	if agent != nil && agent.Session != nil {
		if msg != "" {
			agent.Session.AddMessage("tool", msg)
		}
		inputStr := ""
		if len(cfg) > 0 {
			b, _ := json.Marshal(cfg)
			inputStr = string(b)
		}
		outStr := result.Output
		if result.Err != "" {
			outStr = "error: " + result.Err
		}
		agent.Session.AddToolCall(toolName, inputStr, outStr)
	}
	return p, nil
}

// ToDAGNode 实现 NodeAdapter
func (a *ToolNodeAdapter) ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error) {
	if a.Tools == nil {
		return nil, fmt.Errorf("ToolNodeAdapter: Tools 未配置")
	}
	if task.ToolName == "" {
		return nil, fmt.Errorf("ToolNodeAdapter: 节点 %s 缺少 tool_name", task.ID)
	}
	taskID, toolName, cfg := task.ID, task.ToolName, task.Config
	if cfg == nil {
		cfg = make(map[string]any)
	}
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, toolName, cfg, agent, p)
	}), nil
}

// ToNodeRunner 实现 NodeAdapter
func (a *ToolNodeAdapter) ToNodeRunner(task *planner.TaskNode, agent *runtime.Agent) (NodeRunner, error) {
	if a.Tools == nil {
		return nil, fmt.Errorf("ToolNodeAdapter: Tools 未配置")
	}
	if task.ToolName == "" {
		return nil, fmt.Errorf("ToolNodeAdapter: 节点 %s 缺少 tool_name", task.ID)
	}
	taskID, toolName, cfg := task.ID, task.ToolName, task.Config
	if cfg == nil {
		cfg = make(map[string]any)
	}
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, toolName, cfg, agent, p)
	}, nil
}

// WorkflowNodeAdapter 将 workflow 型 TaskNode 转为 DAG 节点
type WorkflowNodeAdapter struct {
	Workflow WorkflowExec
}

func (a *WorkflowNodeAdapter) runNode(ctx context.Context, taskID, name string, params map[string]any, p *AgentDAGPayload) (*AgentDAGPayload, error) {
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
}

// ToDAGNode 实现 NodeAdapter
func (a *WorkflowNodeAdapter) ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error) {
	if a.Workflow == nil {
		return nil, fmt.Errorf("WorkflowNodeAdapter: Workflow 未配置")
	}
	if task.Workflow == "" {
		return nil, fmt.Errorf("WorkflowNodeAdapter: 节点 %s 缺少 workflow", task.ID)
	}
	taskID, name, params := task.ID, task.Workflow, task.Config
	if params == nil {
		params = make(map[string]any)
	}
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, name, params, p)
	}), nil
}

// ToNodeRunner 实现 NodeAdapter
func (a *WorkflowNodeAdapter) ToNodeRunner(task *planner.TaskNode, agent *runtime.Agent) (NodeRunner, error) {
	if a.Workflow == nil {
		return nil, fmt.Errorf("WorkflowNodeAdapter: Workflow 未配置")
	}
	if task.Workflow == "" {
		return nil, fmt.Errorf("WorkflowNodeAdapter: 节点 %s 缺少 workflow", task.ID)
	}
	taskID, name, params := task.ID, task.Workflow, task.Config
	if params == nil {
		params = make(map[string]any)
	}
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, name, params, p)
	}, nil
}
