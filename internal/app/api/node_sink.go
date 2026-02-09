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

// NewNodeEventSink 创建 NodeEventSink + ToolEventSink；store 为 nil 时不写入。返回值可同时用于 SetNodeEventSink 与 NewDAGCompiler 的 toolEventSink 参数。
func NewNodeEventSink(store jobstore.JobStore) agentexec.NodeAndToolEventSink {
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
		"node_id":         nodeID,
		"trace_span_id":   nodeID,
		"parent_span_id":  "plan",
		"step_index":      stepIndex,
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
		"node_id":          nodeID,
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
		"node_id":         nodeID,
		"tool_name":       toolName,
		"input":           json.RawMessage(input),
		"trace_span_id":   traceSpanID,
		"parent_span_id":  nodeID,
		"step_index":      stepIndex,
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

// NewReplayContextBuilder 创建从事件流重建 ReplayContext 的 Builder（供 Runner 无 Checkpoint 时恢复）
func NewReplayContextBuilder(store jobstore.JobStore) replay.ReplayContextBuilder {
	return replay.NewReplayContextBuilder(store)
}
