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

	"github.com/cloudwego/eino/compose"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
)

// WaitNodeAdapter 将 wait 型 TaskNode 转为 DAG 节点；实际挂起与恢复由 Runner 在 runLoop 中处理（写 JobWaiting、返回 ErrJobWaiting）
type WaitNodeAdapter struct{}

// ToDAGNode 返回 no-op lambda；Runner 在遇到 wait 节点时不调用 Run，直接写 JobWaiting 并返回 ErrJobWaiting
func (WaitNodeAdapter) ToDAGNode(task *planner.TaskNode, _ *runtime.Agent) (*compose.Lambda, error) {
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return p, nil
	}), nil
}

// ToNodeRunner 返回 no-op runner；Runner 在 runLoop 中先判断 NodeType == planner.NodeWait 并处理，不会执行到此处
func (WaitNodeAdapter) ToNodeRunner(task *planner.TaskNode, _ *runtime.Agent) (NodeRunner, error) {
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return p, nil
	}, nil
}
