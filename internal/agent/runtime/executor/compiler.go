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

package executor

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
)

// Compiler 将 TaskGraph 编译为 eino compose.Graph
type Compiler struct {
	adapters map[string]NodeAdapter
}

// NewCompiler 创建编译器，adapters 按 TaskNode.Type 索引（如 planner.NodeLLM, planner.NodeTool, planner.NodeWorkflow）
func NewCompiler(adapters map[string]NodeAdapter) *Compiler {
	if adapters == nil {
		adapters = make(map[string]NodeAdapter)
	}
	return &Compiler{adapters: adapters}
}

// Register 注册某类型的 NodeAdapter
func (c *Compiler) Register(nodeType string, adapter NodeAdapter) {
	if c.adapters == nil {
		c.adapters = make(map[string]NodeAdapter)
	}
	c.adapters[nodeType] = adapter
}

// Compile 将 TaskGraph 转为 compose.Graph，并连接 START/END
func (c *Compiler) Compile(ctx context.Context, g *planner.TaskGraph, agent *runtime.Agent) (*compose.Graph[*AgentDAGPayload, *AgentDAGPayload], error) {
	if g == nil || len(g.Nodes) == 0 {
		return nil, fmt.Errorf("executor: TaskGraph 为空")
	}
	graph := compose.NewGraph[*AgentDAGPayload, *AgentDAGPayload]()

	nodeIDs := make(map[string]struct{})
	for i := range g.Nodes {
		node := &g.Nodes[i]
		nodeIDs[node.ID] = struct{}{}
		adapter := c.adapters[node.Type]
		if adapter == nil {
			return nil, fmt.Errorf("executor: 未知节点类型 %q (节点 %s)", node.Type, node.ID)
		}
		lambda, err := adapter.ToDAGNode(node, agent)
		if err != nil {
			return nil, fmt.Errorf("executor: 节点 %s 适配失败: %w", node.ID, err)
		}
		if err := graph.AddLambdaNode(node.ID, lambda); err != nil {
			return nil, fmt.Errorf("executor: 添加节点 %s 失败: %w", node.ID, err)
		}
	}

	for _, edge := range g.Edges {
		if err := graph.AddEdge(edge.From, edge.To); err != nil {
			return nil, fmt.Errorf("executor: 添加边 %s->%s 失败: %w", edge.From, edge.To, err)
		}
	}

	hasIncoming := make(map[string]bool)
	hasOutgoing := make(map[string]bool)
	for _, e := range g.Edges {
		hasIncoming[e.To] = true
		hasOutgoing[e.From] = true
	}
	for id := range nodeIDs {
		if !hasIncoming[id] {
			if err := graph.AddEdge(compose.START, id); err != nil {
				return nil, fmt.Errorf("executor: 连接 START->%s 失败: %w", id, err)
			}
		}
		if !hasOutgoing[id] {
			if err := graph.AddEdge(id, compose.END); err != nil {
				return nil, fmt.Errorf("executor: 连接 %s->END 失败: %w", id, err)
			}
		}
	}

	return graph, nil
}
