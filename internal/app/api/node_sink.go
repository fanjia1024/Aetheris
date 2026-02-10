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

// nodeEventSinkImpl 将节点级事件写入 JobStore，供 Replay 重建执行上下文
type nodeEventSinkImpl struct {
	store jobstore.JobStore
}

// NewNodeEventSink 创建节点/工具/命令事件 Sink；store 为 nil 时不写入。返回值可同时用于 SetNodeEventSink 与 NewDAGCompiler 的 toolEventSink/commandEventSink 参数。
func NewNodeEventSink(store jobstore.JobStore) agentexec.NodeToolAndCommandEventSink {
	return &nodeEventSinkImpl{store: store}
}

// AppendNodeStarted 实现 NodeEventSink
func (s *nodeEventSinkImpl) AppendNodeStarted(ctx context.Context, jobID string, nodeID string) error {
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
		"trace_span_id":  nodeID,
		"parent_span_id": "plan",
		"step_index":     stepIndex,
	})
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.NodeStarted, Payload: payload,
	})
	return err
}

// AppendNodeFinished 实现 NodeEventSink；payloadResults 为当前 payload.Results 的 JSON
func (s *nodeEventSinkImpl) AppendNodeFinished(ctx context.Context, jobID string, nodeID string, payloadResults []byte) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	payload, err := json.Marshal(map[string]interface{}{
		"node_id":         nodeID,
		"payload_results": json.RawMessage(payloadResults),
		"trace_span_id":   nodeID,
		"parent_span_id":  "plan",
		"step_index":      stepIndex,
	})
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.NodeFinished, Payload: payload,
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
