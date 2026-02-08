package planner

import (
	"context"

	"rag-platform/internal/agent/memory"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/runtime/session"
)

// RulePlanner 规则规划器：不调用 LLM，返回固定或简单规则生成的 TaskGraph，用于稳定调试 Executor
type RulePlanner struct {
	// DefaultGraph 若不为 nil，PlanGoal 优先返回其副本（可带占位 goal）；否则使用内置单节点 llm 图
	DefaultGraph *TaskGraph
}

// NewRulePlanner 创建规则规划器
func NewRulePlanner() *RulePlanner {
	return &RulePlanner{}
}

// Plan 实现 Planner：返回简单 finish，无工具步骤
func (p *RulePlanner) Plan(ctx context.Context, query string, toolsSchemaJSON []byte, history []llm.Message) (*PlanResult, error) {
	return &PlanResult{
		Steps:       nil,
		Next:        "finish",
		FinalAnswer: "[RulePlanner] 请使用 PlanGoal 与 TaskGraph 执行。",
	}, nil
}

// Next 实现 Planner：直接返回最终回答占位
func (p *RulePlanner) Next(ctx context.Context, sess *session.Session, userQuery string, toolsSchemaJSON []byte) (*Step, error) {
	return &Step{Final: "[RulePlanner] 单步模式未使用，请通过 PlanGoal 执行。"}, nil
}

// PlanGoal 实现 Planner：返回固定或简单规则生成的 TaskGraph（单节点 llm，goal 写入 config）
func (p *RulePlanner) PlanGoal(ctx context.Context, goal string, mem memory.Memory) (*TaskGraph, error) {
	if p.DefaultGraph != nil && len(p.DefaultGraph.Nodes) > 0 {
		g := &TaskGraph{
			Nodes: make([]TaskNode, len(p.DefaultGraph.Nodes)),
			Edges: make([]TaskEdge, len(p.DefaultGraph.Edges)),
		}
		copy(g.Nodes, p.DefaultGraph.Nodes)
		copy(g.Edges, p.DefaultGraph.Edges)
		for i := range g.Nodes {
			if g.Nodes[i].Config == nil {
				g.Nodes[i].Config = map[string]any{"goal": goal}
			} else {
				g.Nodes[i].Config["goal"] = goal
			}
		}
		return g, nil
	}
	return &TaskGraph{
		Nodes: []TaskNode{
			{ID: "n1", Type: NodeLLM, Config: map[string]any{"goal": goal}},
		},
		Edges: nil,
	}, nil
}
