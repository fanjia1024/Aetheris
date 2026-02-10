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

package replay

import (
	"context"
	"encoding/json"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/runtime/jobstore"
)

// ReplayContext 从事件流重建的执行上下文，供 Runner 恢复时使用（不重复执行已完成节点）
type ReplayContext struct {
	TaskGraphState       []byte              // PlanGenerated 的 task_graph
	CursorNode           string              // 最后一条 NodeFinished 的 node_id，兼容 Trace/旧逻辑
	PayloadResults       []byte              // 最后一条 NodeFinished 的 payload_results（累积状态）
	CompletedNodeIDs     map[string]struct{} // 所有已出现 NodeFinished 的 node_id 集合，供确定性重放
	PayloadResultsByNode map[string][]byte   // 按 node_id 的 payload_results，供跳过时合并（可选）
	CompletedCommandIDs  map[string]struct{} // 所有已出现 command_committed 的 command_id，已提交命令永不重放
	CommandResults       map[string][]byte   // command_id -> 该命令的 result JSON，Replay 时注入 payload
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
// PlanGenerated 得到 TaskGraph；每条 NodeFinished 加入 CompletedNodeIDs 并更新最后 CursorNode/PayloadResults；按 node 存 PayloadResultsByNode
func (b *replayBuilder) BuildFromEvents(ctx context.Context, jobID string) (*ReplayContext, error) {
	if b.store == nil {
		return nil, nil
	}
	events, _, err := b.store.ListEvents(ctx, jobID)
	if err != nil || len(events) == 0 {
		return nil, err
	}
	out := ReplayContext{
		CompletedNodeIDs:     make(map[string]struct{}),
		PayloadResultsByNode: make(map[string][]byte),
		CompletedCommandIDs:  make(map[string]struct{}),
		CommandResults:       make(map[string][]byte),
	}
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
			out.CompletedNodeIDs[payload.NodeID] = struct{}{}
			out.CursorNode = payload.NodeID
			if len(payload.PayloadResults) > 0 {
				out.PayloadResults = []byte(payload.PayloadResults)
				out.PayloadResultsByNode[payload.NodeID] = []byte(payload.PayloadResults)
			}
		case jobstore.CommandCommitted:
			var payload struct {
				NodeID    string          `json:"node_id"`
				CommandID string          `json:"command_id"`
				Result    json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				continue
			}
			cmdID := payload.CommandID
			if cmdID == "" {
				cmdID = payload.NodeID
			}
			out.CompletedCommandIDs[cmdID] = struct{}{}
			if len(payload.Result) > 0 {
				out.CommandResults[cmdID] = []byte(payload.Result)
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
