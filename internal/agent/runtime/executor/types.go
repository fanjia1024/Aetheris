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

	"rag-platform/internal/agent/runtime"
)

// agentContextKey 用于在 context 中传递 *runtime.Agent（ToolExec 等可从 ctx 取 agent）
type agentContextKey struct{}

// jobIDContextKey 用于在 context 中传递 jobID，供 Tool 节点写入 ToolCalled/ToolReturned
type jobIDContextKey struct{}

var theAgentContextKey = agentContextKey{}
var theJobIDContextKey = jobIDContextKey{}

// WithAgent 将 agent 放入 ctx，供 Runner.Invoke 时传入节点
func WithAgent(ctx context.Context, agent *runtime.Agent) context.Context {
	return context.WithValue(ctx, theAgentContextKey, agent)
}

// AgentFromContext 从 context 取出 *runtime.Agent
func AgentFromContext(ctx context.Context) *runtime.Agent {
	v := ctx.Value(theAgentContextKey)
	if v == nil {
		return nil
	}
	a, _ := v.(*runtime.Agent)
	return a
}

// WithJobID 将 jobID 放入 ctx，供 Tool 节点写入事件
func WithJobID(ctx context.Context, jobID string) context.Context {
	return context.WithValue(ctx, theJobIDContextKey, jobID)
}

// JobIDFromContext 从 context 取出 jobID
func JobIDFromContext(ctx context.Context) string {
	v := ctx.Value(theJobIDContextKey)
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// AgentDAGPayload DAG 统一载荷：整图节点入参/出参一致，便于多前驱时合并结果
type AgentDAGPayload struct {
	Goal      string
	AgentID   string
	SessionID string
	// Results 各节点输出，key 为 TaskNode.ID
	Results map[string]any
}

// NewAgentDAGPayload 构造初始 payload
func NewAgentDAGPayload(goal, agentID, sessionID string) *AgentDAGPayload {
	return &AgentDAGPayload{
		Goal:      goal,
		AgentID:   agentID,
		SessionID: sessionID,
		Results:   make(map[string]any),
	}
}

// Clone 返回副本并确保 Results 可写（供节点写入本节点结果）
func (p *AgentDAGPayload) Clone() *AgentDAGPayload {
	if p == nil {
		return NewAgentDAGPayload("", "", "")
	}
	results := make(map[string]any, len(p.Results))
	for k, v := range p.Results {
		results[k] = v
	}
	return &AgentDAGPayload{
		Goal:      p.Goal,
		AgentID:   p.AgentID,
		SessionID: p.SessionID,
		Results:   results,
	}
}
