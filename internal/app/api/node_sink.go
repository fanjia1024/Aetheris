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

package api

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"rag-platform/internal/agent/replay"
	agentexec "rag-platform/internal/agent/runtime/executor"
	"rag-platform/internal/runtime/jobstore"
)

// Ensure node_sink implements the extended NodeEventSink with resultType/reason.
var _ agentexec.NodeEventSink = (*nodeEventSinkImpl)(nil)

// nodeEventSinkImpl 将节点级事件写入 JobStore，供 Replay 重建执行上下文
type nodeEventSinkImpl struct {
	store jobstore.JobStore
}

// NewNodeEventSink 创建节点/工具/命令事件 Sink；store 为 nil 时不写入。返回值可同时用于 SetNodeEventSink 与 NewDAGCompiler 的 toolEventSink/commandEventSink 参数。
func NewNodeEventSink(store jobstore.JobStore) agentexec.NodeToolAndCommandEventSink {
	return &nodeEventSinkImpl{store: store}
}

// AppendNodeStarted 实现 NodeEventSink（v0.9 含 attempt, worker_id）
func (s *nodeEventSinkImpl) AppendNodeStarted(ctx context.Context, jobID string, nodeID string, attempt int, workerID string) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	pl := map[string]interface{}{
		"node_id":        nodeID,
		"trace_span_id":  nodeID,
		"parent_span_id": "plan",
		"step_index":     stepIndex,
	}
	if attempt > 0 {
		pl["attempt"] = attempt
	}
	if workerID != "" {
		pl["worker_id"] = workerID
	}
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.NodeStarted, Payload: payload,
	})
	return err
}

// AppendNodeFinished 实现 NodeEventSink；resultType/reason 为 Phase A 失败语义，Replay 仅当 result_type==success 时视节点完成；stepID 为空时用 nodeID，inputHash 供确定性 Replay（plan 3.3）
func (s *nodeEventSinkImpl) AppendNodeFinished(ctx context.Context, jobID string, nodeID string, payloadResults []byte, durationMs int64, state string, attempt int, resultType agentexec.StepResultType, reason string, stepID string, inputHash string) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	stepIDVal := stepID
	if stepIDVal == "" {
		stepIDVal = nodeID
	}
	pl := map[string]interface{}{
		"node_id":        nodeID,
		"step_id":        stepIDVal,
		"trace_span_id":  nodeID,
		"parent_span_id": "plan",
		"step_index":     stepIndex,
		"result_type":    string(resultType), // required for Replay; default "" treated as success for old events
	}
	if len(payloadResults) > 0 {
		pl["payload_results"] = json.RawMessage(payloadResults)
	}
	if durationMs > 0 {
		pl["duration_ms"] = durationMs
	}
	if state != "" {
		pl["state"] = state
	}
	if attempt > 0 {
		pl["attempt"] = attempt
	}
	if reason != "" {
		pl["reason"] = reason
	}
	if inputHash != "" {
		pl["input_hash"] = inputHash
	}
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.NodeFinished, Payload: payload,
	})
	return err
}

// AppendStateCheckpointed 实现 NodeEventSink；v0.9 语义事件，供 Trace 展示 state diff；opts 可选携带 changed_keys、tool_side_effects、resource_refs
func (s *nodeEventSinkImpl) AppendStateCheckpointed(ctx context.Context, jobID string, nodeID string, stateBefore, stateAfter []byte, opts *agentexec.StateCheckpointOpts) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	pl := map[string]interface{}{
		"node_id":     nodeID,
		"step_index":  stepIndex,
		"state_after": json.RawMessage(stateAfter),
	}
	if len(stateBefore) > 0 {
		pl["state_before"] = json.RawMessage(stateBefore)
	}
	if opts != nil {
		if len(opts.ChangedKeys) > 0 {
			pl["changed_keys"] = opts.ChangedKeys
		}
		if len(opts.ToolSideEffects) > 0 {
			pl["tool_side_effects"] = opts.ToolSideEffects
		}
		if len(opts.ResourceRefs) > 0 {
			pl["resource_refs"] = opts.ResourceRefs
		}
	}
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.StateCheckpointed, Payload: payload,
	})
	return err
}

// AppendToolCalled 实现 ToolEventSink
func (s *nodeEventSinkImpl) AppendToolCalled(ctx context.Context, jobID string, nodeID string, toolName string, input []byte) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	traceSpanID := nodeID + ":tool:" + toolName + ":" + strconv.Itoa(stepIndex)
	payload, err := json.Marshal(map[string]interface{}{
		"node_id":        nodeID,
		"tool_name":      toolName,
		"input":          json.RawMessage(input),
		"trace_span_id":  traceSpanID,
		"parent_span_id": nodeID,
		"step_index":     stepIndex,
	})
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.ToolCalled, Payload: payload,
	})
	return err
}

// AppendToolReturned 实现 ToolEventSink
func (s *nodeEventSinkImpl) AppendToolReturned(ctx context.Context, jobID string, nodeID string, output []byte) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	payload, err := json.Marshal(map[string]interface{}{
		"node_id":        nodeID,
		"output":         json.RawMessage(output),
		"parent_span_id": nodeID,
		"step_index":     stepIndex,
	})
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.ToolReturned, Payload: payload,
	})
	return err
}

// AppendToolResultSummarized 实现 ToolEventSink；v0.9 语义事件，供 Trace 展示工具结果摘要
func (s *nodeEventSinkImpl) AppendToolResultSummarized(ctx context.Context, jobID string, nodeID string, toolName string, summary string, errMsg string, idempotent bool) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	pl := map[string]interface{}{
		"node_id":    nodeID,
		"tool_name":  toolName,
		"step_index": stepIndex,
	}
	if summary != "" {
		pl["summary"] = summary
	}
	if errMsg != "" {
		pl["error"] = errMsg
	}
	pl["idempotent"] = idempotent
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.ToolResultSummarized, Payload: payload,
	})
	return err
}

// AppendToolInvocationStarted 实现 ToolEventSink；写入 tool_invocation_started，含 idempotency_key 供 Replay 查找
func (s *nodeEventSinkImpl) AppendToolInvocationStarted(ctx context.Context, jobID string, nodeID string, payload *agentexec.ToolInvocationStartedPayload) error {
	if s.store == nil || payload == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	pl := map[string]interface{}{
		"node_id":         nodeID,
		"invocation_id":   payload.InvocationID,
		"tool_name":       payload.ToolName,
		"idempotency_key": payload.IdempotencyKey,
		"started_at":      payload.StartedAt,
	}
	if payload.ArgumentsHash != "" {
		pl["arguments_hash"] = payload.ArgumentsHash
	}
	payloadBytes, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.ToolInvocationStarted, Payload: payloadBytes,
	})
	return err
}

// AppendToolInvocationFinished 实现 ToolEventSink；outcome 为 success 时 Replay 会加入 CompletedToolInvocations
func (s *nodeEventSinkImpl) AppendToolInvocationFinished(ctx context.Context, jobID string, nodeID string, payload *agentexec.ToolInvocationFinishedPayload) error {
	if s.store == nil || payload == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	pl := map[string]interface{}{
		"node_id":         nodeID,
		"invocation_id":   payload.InvocationID,
		"idempotency_key": payload.IdempotencyKey,
		"outcome":         payload.Outcome,
		"finished_at":     payload.FinishedAt,
	}
	if len(payload.Result) > 0 {
		pl["result"] = json.RawMessage(payload.Result)
	}
	if payload.Error != "" {
		pl["error"] = payload.Error
	}
	payloadBytes, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.ToolInvocationFinished, Payload: payloadBytes,
	})
	return err
}

// AppendCommandEmitted 实现 CommandEventSink；执行副作用前写入，供审计
func (s *nodeEventSinkImpl) AppendCommandEmitted(ctx context.Context, jobID string, nodeID string, commandID string, kind string, input []byte) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	payload, err := json.Marshal(map[string]interface{}{
		"node_id":    nodeID,
		"command_id": commandID,
		"kind":       kind,
		"input":      json.RawMessage(input),
		"step_index": stepIndex,
	})
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.CommandEmitted, Payload: payload,
	})
	return err
}

// AppendCommandCommitted 实现 CommandEventSink；命令执行成功后立即写入，Replay 时已提交命令永不重放；inputHash 供确定性 Replay（plan 3.3）
func (s *nodeEventSinkImpl) AppendCommandCommitted(ctx context.Context, jobID string, nodeID string, commandID string, result []byte, inputHash string) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	pl := map[string]interface{}{
		"node_id":    nodeID,
		"command_id": commandID,
		"step_id":    commandID, // 单命令节点下 command_id = step_id
		"result":     json.RawMessage(result),
		"step_index": stepIndex,
	}
	if inputHash != "" {
		pl["input_hash"] = inputHash
	}
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.CommandCommitted, Payload: payload,
	})
	return err
}

// AppendStateChanged 实现 StateChangeSink；写入 state_changed 事件，供 Trace 审计「本步改变了什么」
func (s *nodeEventSinkImpl) AppendStateChanged(ctx context.Context, jobID string, nodeID string, changes []agentexec.StateChanged) error {
	if s.store == nil || len(changes) == 0 {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	pl := map[string]interface{}{
		"node_id":       nodeID,
		"step_index":    stepIndex,
		"state_changes": changes,
	}
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.StateChanged, Payload: payload,
	})
	return err
}

// AppendJobWaiting 实现 NodeEventSink；写入 job_waiting 事件，payload 含 correlation_key、wait_type、resumption_context（design/runtime-contract.md, design/agent-process-model.md § Continuation）
func (s *nodeEventSinkImpl) AppendJobWaiting(ctx context.Context, jobID string, nodeID string, waitKind, reason string, expiresAt time.Time, correlationKey string, resumptionContext []byte) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	waitType := waitKind
	if waitType != "webhook" && waitType != "human" && waitType != "timer" && waitType != "signal" && waitType != "message" {
		waitType = "signal"
	}
	pl := jobstore.JobWaitingPayload{
		NodeID:            nodeID,
		WaitType:          waitType,
		CorrelationKey:    correlationKey,
		WaitKind:          waitKind,
		Reason:            reason,
		ExpiresAtRFC3339:  expiresAt.Format(time.RFC3339),
		ResumptionContext: resumptionContext,
	}
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.JobWaiting, Payload: payload,
	})
	return err
}

// AppendReasoningSnapshot 实现 NodeEventSink；写入 reasoning_snapshot 事件，供因果调试（Causal Debugging）
func (s *nodeEventSinkImpl) AppendReasoningSnapshot(ctx context.Context, jobID string, payload []byte) error {
	if s.store == nil || len(payload) == 0 {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.ReasoningSnapshot, Payload: payload,
	})
	return err
}

// AppendMemoryRead 实现 NodeEventSink；Trace 2.0 memory_read（design/trace-2.0-cognition.md）
func (s *nodeEventSinkImpl) AppendMemoryRead(ctx context.Context, jobID string, nodeID string, stepIndex int, memoryType, keyOrScope, summary string) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	pl := jobstore.MemoryReadPayload{JobID: jobID, NodeID: nodeID, StepIndex: stepIndex, MemoryType: memoryType, KeyOrScope: keyOrScope, Summary: summary}
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{JobID: jobID, Type: jobstore.MemoryRead, Payload: payload})
	return err
}

// AppendMemoryWrite 实现 NodeEventSink；Trace 2.0 memory_write（design/trace-2.0-cognition.md）
func (s *nodeEventSinkImpl) AppendMemoryWrite(ctx context.Context, jobID string, nodeID string, stepIndex int, memoryType, keyOrScope, summary string) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	pl := jobstore.MemoryWritePayload{JobID: jobID, NodeID: nodeID, StepIndex: stepIndex, MemoryType: memoryType, KeyOrScope: keyOrScope, Summary: summary}
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{JobID: jobID, Type: jobstore.MemoryWrite, Payload: payload})
	return err
}

// AppendPlanEvolution 实现 NodeEventSink；Trace 2.0 plan_evolution（design/trace-2.0-cognition.md），可选
func (s *nodeEventSinkImpl) AppendPlanEvolution(ctx context.Context, jobID string, planVersion int, diffSummary string) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	pl := jobstore.PlanEvolutionPayload{PlanVersion: planVersion, DiffSummary: diffSummary}
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{JobID: jobID, Type: jobstore.PlanEvolution, Payload: payload})
	return err
}

// NewReplayContextBuilder 创建从事件流重建 ReplayContext 的 Builder（供 Runner 无 Checkpoint 时恢复）
func NewReplayContextBuilder(store jobstore.JobStore) replay.ReplayContextBuilder {
	return replay.NewReplayContextBuilder(store)
}
