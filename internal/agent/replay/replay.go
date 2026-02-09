package replay

import (
	"context"
	"encoding/json"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/runtime/jobstore"
)

// ReplayContext 从事件流重建的执行上下文，供 Runner 恢复时使用（不重复执行已完成节点）
type ReplayContext struct {
	TaskGraphState []byte
	CursorNode     string
	PayloadResults []byte
}

// ReplayContextBuilder 从 JobStore 事件流构建 ReplayContext
type ReplayContextBuilder interface {
	BuildFromEvents(ctx context.Context, jobID string) (*ReplayContext, error)
}

// replayBuilder 基于 jobstore.JobStore 的 ReplayContext 构建器
type replayBuilder struct {
	store jobstore.JobStore
}

// NewReplayContextBuilder 创建从事件流构建 ReplayContext 的 Builder
func NewReplayContextBuilder(store jobstore.JobStore) ReplayContextBuilder {
	return &replayBuilder{store: store}
}

// BuildFromEvents 从 job 的事件列表重建执行上下文；事件流为权威来源
// 若有 PlanGenerated 则得到 TaskGraph；若有 NodeFinished 则取最后一条得到 CursorNode 与 PayloadResults
func (b *replayBuilder) BuildFromEvents(ctx context.Context, jobID string) (*ReplayContext, error) {
	if b.store == nil {
		return nil, nil
	}
	events, _, err := b.store.ListEvents(ctx, jobID)
	if err != nil || len(events) == 0 {
		return nil, err
	}
	var out ReplayContext
	for _, e := range events {
		switch e.Type {
		case jobstore.PlanGenerated:
			var payload struct {
				TaskGraph json.RawMessage `json:"task_graph"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil || len(payload.TaskGraph) == 0 {
				continue
			}
			out.TaskGraphState = []byte(payload.TaskGraph)
		case jobstore.NodeFinished:
			var payload struct {
				NodeID         string          `json:"node_id"`
				PayloadResults json.RawMessage `json:"payload_results"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				continue
			}
			out.CursorNode = payload.NodeID
			if len(payload.PayloadResults) > 0 {
				out.PayloadResults = []byte(payload.PayloadResults)
			}
		}
	}
	if len(out.TaskGraphState) == 0 {
		return nil, nil
	}
	return &out, nil
}

// TaskGraph 反序列化 ReplayContext 中的 TaskGraph
func (r *ReplayContext) TaskGraph() (*planner.TaskGraph, error) {
	if r == nil || len(r.TaskGraphState) == 0 {
		return nil, nil
	}
	var g planner.TaskGraph
	if err := g.Unmarshal(r.TaskGraphState); err != nil {
		return nil, err
	}
	return &g, nil
}
