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

// AppendNodeFinished 实现 NodeEventSink；resultType/reason 为 Phase A 失败语义，Replay 仅当 result_type==success 时视节点完成
func (s *nodeEventSinkImpl) AppendNodeFinished(ctx context.Context, jobID string, nodeID string, payloadResults []byte, durationMs int64, state string, attempt int, resultType agentexec.StepResultType, reason string) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	pl := map[string]interface{}{
		"node_id":      nodeID,
		"trace_span_id": nodeID,
		"parent_span_id": "plan",
		"step_index":   stepIndex,
		"result_type":  string(resultType), // required for Replay; default "" treated as success for old events
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
	payload, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.NodeFinished, Payload: payload,
	})
	return err
}

// AppendStateCheckpointed 实现 NodeEventSink；v0.9 语义事件，供 Trace 展示 state diff
func (s *nodeEventSinkImpl) AppendStateCheckpointed(ctx context.Context, jobID string, nodeID string, stateBefore, stateAfter []byte) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	pl := map[string]interface{}{
		"node_id":      nodeID,
		"step_index":   stepIndex,
		"state_after":  json.RawMessage(stateAfter),
	}
	if len(stateBefore) > 0 {
		pl["state_before"] = json.RawMessage(stateBefore)
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
		"node_id":   nodeID,
		"tool_name": toolName,
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

// AppendCommandCommitted 实现 CommandEventSink；命令执行成功后立即写入，Replay 时已提交命令永不重放
func (s *nodeEventSinkImpl) AppendCommandCommitted(ctx context.Context, jobID string, nodeID string, commandID string, result []byte) error {
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
		"result":     json.RawMessage(result),
		"step_index": stepIndex,
	})
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.CommandCommitted, Payload: payload,
	})
	return err
}

// NewReplayContextBuilder 创建从事件流重建 ReplayContext 的 Builder（供 Runner 无 Checkpoint 时恢复）
func NewReplayContextBuilder(store jobstore.JobStore) replay.ReplayContextBuilder {
	return replay.NewReplayContextBuilder(store)
}
