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
	"errors"

	"rag-platform/internal/agent/runtime"
)

// ErrJobWaiting 表示 Job 在 Wait 节点挂起，等待 signal/continue 后由其他 Worker 认领继续（design/job-state-machine.md）
var ErrJobWaiting = errors.New("executor: job waiting for signal")

// agentContextKey 用于在 context 中传递 *runtime.Agent（ToolExec 等可从 ctx 取 agent）
type agentContextKey struct{}

// jobIDContextKey 用于在 context 中传递 jobID，供 Tool 节点写入 ToolCalled/ToolReturned
type jobIDContextKey struct{}

// replayContextKey 用于在 context 中传递 Replay 已完成的工具调用结果（idempotency_key -> result JSON），供 Tool 节点幂等跳过
type replayContextKey struct{}

// stateChangesByStepContextKey 用于在 context 中传递 Replay 的「按 step 的 state_changed」列表，供 Confirmation Replay 校验
type stateChangesByStepContextKey struct{}

// pendingToolInvocationsContextKey 用于在 context 中传递事件流「已 started 无 finished」的 idempotency_key 集合（Activity Log Barrier），禁止再次执行
type pendingToolInvocationsContextKey struct{}

// executionStepIDContextKey 用于在 context 中传递确定性步身份（design/step-identity.md），供 Ledger/事件写入与 Replay 一致
type executionStepIDContextKey struct{}

// toolExecutionKeyContextKey 用于在 context 中传递当前工具执行的稳定身份，供 Tool 实现方做幂等（传给下游 idempotency key）；见 design/effect-system.md Idempotency contract
type toolExecutionKeyContextKey struct{}

var theAgentContextKey = agentContextKey{}
var theJobIDContextKey = jobIDContextKey{}
var theReplayContextKey = replayContextKey{}
var theStateChangesByStepContextKey = stateChangesByStepContextKey{}
var thePendingToolInvocationsContextKey = pendingToolInvocationsContextKey{}
var theExecutionStepIDContextKey = executionStepIDContextKey{}
var theToolExecutionKeyContextKey = toolExecutionKeyContextKey{}

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

// WithCompletedToolInvocations 将 Replay 得到的已完成工具调用结果放入 ctx（idempotency_key -> result JSON），供 Tool 节点幂等跳过
func WithCompletedToolInvocations(ctx context.Context, m map[string][]byte) context.Context {
	return context.WithValue(ctx, theReplayContextKey, m)
}

// CompletedToolInvocationsFromContext 从 context 取出已完成工具调用结果
func CompletedToolInvocationsFromContext(ctx context.Context) map[string][]byte {
	v := ctx.Value(theReplayContextKey)
	if v == nil {
		return nil
	}
	m, _ := v.(map[string][]byte)
	return m
}

// WithPendingToolInvocations 将 Replay 得到的「已 started 无 finished」的 idempotency_key 集合放入 ctx（Activity Log Barrier），供 Tool 节点禁止再次执行
func WithPendingToolInvocations(ctx context.Context, pending map[string]struct{}) context.Context {
	return context.WithValue(ctx, thePendingToolInvocationsContextKey, pending)
}

// PendingToolInvocationsFromContext 从 context 取出 pending 集合
func PendingToolInvocationsFromContext(ctx context.Context) map[string]struct{} {
	v := ctx.Value(thePendingToolInvocationsContextKey)
	if v == nil {
		return nil
	}
	m, _ := v.(map[string]struct{})
	return m
}

// StateChangeForVerify Confirmation Replay 时单条待校验的外部资源变更（由 Runner 从 ReplayContext 转换注入）
type StateChangeForVerify struct {
	ResourceType string
	ResourceID   string
	Operation    string
	ExternalRef  string
}

// WithStateChangesByStep 将 Replay 的「按 node_id 的 state_changed」放入 ctx，供 Tool 节点 Confirmation 时校验
func WithStateChangesByStep(ctx context.Context, m map[string][]StateChangeForVerify) context.Context {
	return context.WithValue(ctx, theStateChangesByStepContextKey, m)
}

// StateChangesByStepFromContext 从 context 取出按 step 的 state_changes
func StateChangesByStepFromContext(ctx context.Context) map[string][]StateChangeForVerify {
	v := ctx.Value(theStateChangesByStepContextKey)
	if v == nil {
		return nil
	}
	m, _ := v.(map[string][]StateChangeForVerify)
	return m
}

// WithExecutionStepID 将确定性步身份放入 ctx（design/step-identity.md），供 Ledger 与事件写入使用
func WithExecutionStepID(ctx context.Context, stepID string) context.Context {
	return context.WithValue(ctx, theExecutionStepIDContextKey, stepID)
}

// ExecutionStepIDFromContext 从 context 取出步身份；空表示使用 planner node ID（向后兼容）
func ExecutionStepIDFromContext(ctx context.Context) string {
	v := ctx.Value(theExecutionStepIDContextKey)
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// WithToolExecutionKey 将当前工具调用的稳定执行键放入 ctx，供 Tool 实现方做幂等（Idempotency contract）；见 ExecutionKeyFromContext
func WithToolExecutionKey(ctx context.Context, executionKey string) context.Context {
	return context.WithValue(ctx, theToolExecutionKeyContextKey, executionKey)
}

// ExecutionKeyFromContext 从 context 取出当前工具调用的稳定执行键（job+step+idempotency_key 的等价标识）。Tool 实现方应据此做幂等或传给下游作 idempotency key；Runtime 保证同一 ExecutionKey 最多一次真实执行，Replay 仅注入已记录结果。
func ExecutionKeyFromContext(ctx context.Context) string {
	v := ctx.Value(theToolExecutionKeyContextKey)
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
