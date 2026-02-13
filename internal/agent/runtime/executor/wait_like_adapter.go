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

// ApprovalNodeAdapter 审批节点适配器；运行时挂起逻辑由 Runner 统一处理（与 wait 一致）。
type ApprovalNodeAdapter struct{}

// ConditionNodeAdapter 条件等待节点适配器；运行时挂起逻辑由 Runner 统一处理（与 wait 一致）。
type ConditionNodeAdapter struct{}

func (ApprovalNodeAdapter) ToDAGNode(task *planner.TaskNode, _ *runtime.Agent) (*compose.Lambda, error) {
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return p, nil
	}), nil
}

func (ApprovalNodeAdapter) ToNodeRunner(task *planner.TaskNode, _ *runtime.Agent) (NodeRunner, error) {
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return p, nil
	}, nil
}

func (ConditionNodeAdapter) ToDAGNode(task *planner.TaskNode, _ *runtime.Agent) (*compose.Lambda, error) {
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return p, nil
	}), nil
}

func (ConditionNodeAdapter) ToNodeRunner(task *planner.TaskNode, _ *runtime.Agent) (NodeRunner, error) {
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return p, nil
	}, nil
}
