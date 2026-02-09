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

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
)

// SteppableStep 单步：节点 ID + 执行函数（按拓扑序）
type SteppableStep struct {
	NodeID string
	Run    NodeRunner
}

// TopoOrder 从 TaskGraph 计算拓扑序（Kahn）；仅包含业务节点，不含 START/END
func TopoOrder(g *planner.TaskGraph) ([]string, error) {
	if g == nil || len(g.Nodes) == 0 {
		return nil, nil
	}
	nodeSet := make(map[string]struct{})
	for i := range g.Nodes {
		nodeSet[g.Nodes[i].ID] = struct{}{}
	}
	inDegree := make(map[string]int)
	for id := range nodeSet {
		inDegree[id] = 0
	}
	for _, e := range g.Edges {
		if _, ok := nodeSet[e.To]; ok {
			inDegree[e.To]++
		}
	}
	var queue []string
	for id, d := range inDegree {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	var order []string
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		order = append(order, u)
		for _, e := range g.Edges {
			if e.From != u {
				continue
			}
			v := e.To
			if _, ok := nodeSet[v]; !ok {
				continue
			}
			inDegree[v]--
			if inDegree[v] == 0 {
				queue = append(queue, v)
			}
		}
	}
	if len(order) != len(nodeSet) {
		return nil, fmt.Errorf("executor: TaskGraph 存在环，无法拓扑排序")
	}
	return order, nil
}

// CompileSteppable 将 TaskGraph 编译为按拓扑序的 SteppableStep 列表，供逐节点执行与 checkpoint
func (c *Compiler) CompileSteppable(ctx context.Context, g *planner.TaskGraph, agent *runtime.Agent) ([]SteppableStep, error) {
	if g == nil || len(g.Nodes) == 0 {
		return nil, fmt.Errorf("executor: TaskGraph 为空")
	}
	order, err := TopoOrder(g)
	if err != nil {
		return nil, err
	}
	nodeByID := make(map[string]*planner.TaskNode)
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	var steps []SteppableStep
	for _, id := range order {
		node := nodeByID[id]
		if node == nil {
			return nil, fmt.Errorf("executor: 节点 %s 不在图中", id)
		}
		adapter := c.adapters[node.Type]
		if adapter == nil {
			return nil, fmt.Errorf("executor: 未知节点类型 %q (节点 %s)", node.Type, id)
		}
		run, err := adapter.ToNodeRunner(node, agent)
		if err != nil {
			return nil, fmt.Errorf("executor: 节点 %s ToNodeRunner 失败: %w", id, err)
		}
		steps = append(steps, SteppableStep{NodeID: id, Run: run})
	}
	return steps, nil
}
