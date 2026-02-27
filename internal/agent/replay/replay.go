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
	"fmt"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/runtime/jobstore"
)

// StateChangeRecord 单条外部资源变更（从 state_changed 事件解析），供 Confirmation Replay 校验
type StateChangeRecord struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Operation    string `json:"operation"`
	StepID       string `json:"step_id,omitempty"`
	ToolName     string `json:"tool_name,omitempty"`
	Version      string `json:"version,omitempty"`
	Etag         string `json:"etag,omitempty"`
	ExternalRef  string `json:"external_ref,omitempty"`
}

// ReplayContext 从事件流重建的执行上下文，供 Runner 恢复时使用（不重复执行已完成节点）
type ReplayContext struct {
	TaskGraphState           []byte                         // PlanGenerated 的 task_graph
	CursorNode               string                         // 最后一条 NodeFinished 的 node_id，兼容 Trace/旧逻辑
	PayloadResults           []byte                         // 最后一条 NodeFinished 的 payload_results（累积状态）
	CompletedNodeIDs         map[string]struct{}            // 所有已出现 NodeFinished 的 node_id 集合，供确定性重放
	PayloadResultsByNode     map[string][]byte              // 按 node_id 的 payload_results，供跳过时合并（可选）
	CompletedCommandIDs      map[string]struct{}            // 所有已出现 command_committed 的 command_id，已提交命令永不重放
	CommandResults           map[string][]byte              // command_id -> 该命令的 result JSON，Replay 时注入 payload
	CompletedToolInvocations map[string][]byte              // idempotency_key -> 成功完成的工具调用 result JSON，Replay 时跳过执行并注入
	PendingToolInvocations   map[string]struct{}            // 事件流中「有 tool_invocation_started 无对应 tool_invocation_finished」的 idempotency_key，禁止再次执行（Activity Log Barrier）
	StateChangesByStep       map[string][]StateChangeRecord // node_id -> 该步的 state_changed 列表，供 Confirmation Replay
	ApprovedCorrelationKeys  map[string]struct{}            // wait_completed 中的 correlation_key 集合，供 CapabilityPolicyChecker 审批后放行（design/capability-policy.md）
	// WorkingMemorySnapshot 最近一次 job_waiting 的 resumption_context.memory_snapshot.working_memory（AgentState JSON）；恢复时 Apply 到 Session（design/durable-memory-layer.md）
	WorkingMemorySnapshot []byte
	// Phase 由事件流推导的执行阶段（plan 3.4），用于观测与「Agent 即长期进程」表述
	Phase ExecutionPhase
	// RecordedTime effect_id -> 记录的时间（UnixNano）；来自 timer_fired 事件，Replay 时仅注入（2.0 确定性）
	RecordedTime map[string]int64
	// RecordedRandom effect_id -> 记录的随机值 JSON；来自 random_recorded 事件
	RecordedRandom map[string][]byte
	// RecordedUUID effect_id -> 记录的 UUID 字符串；来自 uuid_recorded 事件
	RecordedUUID map[string]string
	// RecordedHTTP effect_id -> 记录的 HTTP 响应 body（JSON）；来自 http_recorded 事件
	RecordedHTTP map[string][]byte
}

// ExecutionPhase 执行阶段（可选显式状态机，plan 3.4）：由事件流推导
type ExecutionPhase int

const (
	PhaseUnknown   ExecutionPhase = iota
	PhasePlanning                 // 无 PlanGenerated 或尚未完成规划
	PhaseExecuting                // 有 PlanGenerated，正在执行节点
	PhaseCompleted                // 已 job_completed
	PhaseFailed                   // 已 job_failed
	PhaseCancelled                // 已 job_cancelled
)

// ExecutionState 执行状态：由事件流（或 Checkpoint）推导，供 Advance 决定下一步；ReplayContext 为其一种实现（plan 3.1 A）
type ExecutionState struct {
	*ReplayContext
	// LastAppendedVersion 当前已持久化的事件版本（可选）；Advance 仅依赖已落盘事件
	LastAppendedVersion int
}

// NewExecutionState 从 ReplayContext 构造 ExecutionState
func NewExecutionState(rc *ReplayContext) *ExecutionState {
	if rc == nil {
		return nil
	}
	return &ExecutionState{ReplayContext: rc}
}

// ReplayContextBuilder 从 JobStore 事件流构建 ReplayContext
type ReplayContextBuilder interface {
	BuildFromEvents(ctx context.Context, jobID string) (*ReplayContext, error)
	// BuildFromSnapshot 从快照开始构建（2.0 performance optimization）
	BuildFromSnapshot(ctx context.Context, jobID string) (*ReplayContext, error)
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
		CompletedNodeIDs:         make(map[string]struct{}),
		PayloadResultsByNode:     make(map[string][]byte),
		CompletedCommandIDs:      make(map[string]struct{}),
		CommandResults:           make(map[string][]byte),
		CompletedToolInvocations: make(map[string][]byte),
		PendingToolInvocations:   make(map[string]struct{}),
		StateChangesByStep:       make(map[string][]StateChangeRecord),
		ApprovedCorrelationKeys:  make(map[string]struct{}),
		RecordedTime:             make(map[string]int64),
		RecordedRandom:           make(map[string][]byte),
		RecordedUUID:             make(map[string]string),
		RecordedHTTP:             make(map[string][]byte),
	}
	var lastType jobstore.EventType
	for _, e := range events {
		lastType = e.Type
		switch e.Type {
		case jobstore.ToolInvocationStarted:
			var pl struct {
				IdempotencyKey string `json:"idempotency_key"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.IdempotencyKey == "" {
				continue
			}
			out.PendingToolInvocations[pl.IdempotencyKey] = struct{}{}
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
				StepID         string          `json:"step_id"` // 确定性步身份（design/step-identity.md）；有则用其作为 CompletedNodeIDs 的 key，否则用 node_id 向后兼容
				PayloadResults json.RawMessage `json:"payload_results"`
				ResultType     string          `json:"result_type"` // Phase A: only success (or empty for old events) advances CompletedNodeIDs
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				continue
			}
			// pure / success / side_effect_committed / compensated 均视为节点完成；缺省为 success 以兼容旧事件
			switch payload.ResultType {
			case "", "success", "pure", "side_effect_committed", "compensated":
				// advance
			default:
				continue
			}
			completedKey := payload.NodeID
			if payload.StepID != "" {
				completedKey = payload.StepID
			}
			out.CompletedNodeIDs[completedKey] = struct{}{}
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
		case jobstore.ToolInvocationFinished:
			var pl struct {
				IdempotencyKey string          `json:"idempotency_key"`
				Outcome        string          `json:"outcome"`
				Result         json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil {
				continue
			}
			if pl.IdempotencyKey != "" {
				delete(out.PendingToolInvocations, pl.IdempotencyKey)
			}
			if pl.Outcome != "success" || pl.IdempotencyKey == "" {
				continue
			}
			if len(pl.Result) > 0 {
				out.CompletedToolInvocations[pl.IdempotencyKey] = []byte(pl.Result)
			} else {
				out.CompletedToolInvocations[pl.IdempotencyKey] = []byte("{}")
			}
		case jobstore.StateChanged:
			var pl struct {
				NodeID       string              `json:"node_id"`
				StateChanges []StateChangeRecord `json:"state_changes"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil {
				continue
			}
			if pl.NodeID == "" || len(pl.StateChanges) == 0 {
				continue
			}
			out.StateChangesByStep[pl.NodeID] = append(out.StateChangesByStep[pl.NodeID], pl.StateChanges...)
		case jobstore.JobWaiting:
			p, err := jobstore.ParseJobWaitingPayload(e.Payload)
			if err != nil || len(p.ResumptionContext) == 0 {
				continue
			}
			var resumption map[string]interface{}
			if json.Unmarshal(p.ResumptionContext, &resumption) != nil {
				continue
			}
			ms, _ := resumption["memory_snapshot"].(map[string]interface{})
			if ms != nil {
				if wm, ok := ms["working_memory"]; ok && wm != nil {
					switch v := wm.(type) {
					case string:
						out.WorkingMemorySnapshot = []byte(v)
					case []byte:
						out.WorkingMemorySnapshot = v
					case json.RawMessage:
						out.WorkingMemorySnapshot = v
					default:
						if b, err := json.Marshal(wm); err == nil {
							out.WorkingMemorySnapshot = b
						}
					}
				}
			}
		case jobstore.WaitCompleted:
			var pl struct {
				NodeID         string          `json:"node_id"`
				Payload        json.RawMessage `json:"payload"`
				CorrelationKey string          `json:"correlation_key"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.NodeID == "" {
				continue
			}
			out.CompletedNodeIDs[pl.NodeID] = struct{}{}
			out.CursorNode = pl.NodeID
			out.CompletedCommandIDs[pl.NodeID] = struct{}{}
			if pl.CorrelationKey != "" {
				out.ApprovedCorrelationKeys[pl.CorrelationKey] = struct{}{}
			}
			// Continuation: 恢复时优先从 resumption_context 读取 wait 点的 payload_results（design/agent-process-model.md § Continuation）
			// 若 signal payload 非空，合并到 command result；resumption_context 在对应 job_waiting 中，需二次查找（Phase 2）
			if len(pl.Payload) > 0 {
				out.CommandResults[pl.NodeID] = []byte(pl.Payload)
			} else {
				out.CommandResults[pl.NodeID] = []byte("{}")
			}
		case jobstore.TimerFired:
			var pl struct {
				EffectID string `json:"effect_id"`
				UnixNano int64  `json:"unix_nano"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.EffectID == "" {
				continue
			}
			out.RecordedTime[pl.EffectID] = pl.UnixNano
		case jobstore.RandomRecorded:
			var pl struct {
				EffectID string          `json:"effect_id"`
				Values   json.RawMessage `json:"values"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.EffectID == "" {
				continue
			}
			out.RecordedRandom[pl.EffectID] = []byte(pl.Values)
		case jobstore.UUIDRecorded:
			var pl struct {
				EffectID string `json:"effect_id"`
				UUID     string `json:"uuid"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.EffectID == "" {
				continue
			}
			out.RecordedUUID[pl.EffectID] = pl.UUID
		case jobstore.HTTPRecorded:
			var pl struct {
				EffectID string          `json:"effect_id"`
				Response json.RawMessage `json:"response"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.EffectID == "" {
				continue
			}
			out.RecordedHTTP[pl.EffectID] = []byte(pl.Response)
		}
	}
	// 推导 Phase（plan 3.4）
	switch lastType {
	case jobstore.JobCompleted:
		out.Phase = PhaseCompleted
	case jobstore.JobFailed:
		out.Phase = PhaseFailed
	case jobstore.JobCancelled:
		out.Phase = PhaseCancelled
	default:
		if len(out.TaskGraphState) > 0 {
			out.Phase = PhaseExecuting
		} else {
			out.Phase = PhasePlanning
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

// BuildFromSnapshot 从快照开始构建 ReplayContext，然后叠加快照之后的增量事件（2.0 性能优化）
func (b *replayBuilder) BuildFromSnapshot(ctx context.Context, jobID string) (*ReplayContext, error) {
	if b.store == nil {
		return nil, nil
	}

	// 尝试获取最新快照
	snapshot, err := b.store.GetLatestSnapshot(ctx, jobID)
	if err != nil {
		// 快照读取failed，降级到全事件重放
		return b.BuildFromEvents(ctx, jobID)
	}
	if snapshot == nil {
		// 无快照，使用全事件重放
		return b.BuildFromEvents(ctx, jobID)
	}

	// 反序列化快照
	rc, err := deserializeSnapshot(snapshot.Snapshot)
	if err != nil {
		// 快照损坏，降级到全事件重放
		return b.BuildFromEvents(ctx, jobID)
	}

	// 获取快照之后的增量事件
	events, _, err := b.store.ListEvents(ctx, jobID)
	if err != nil {
		return nil, err
	}

	// 只处理快照版本之后的事件
	incrementalEvents := []jobstore.JobEvent{}
	for _, e := range events {
		// 事件按 version 排序，version 从 1 开始
		// 快照覆盖到 version N，则处理 version > N 的事件
		// 这里简化处理：使用事件数量作为版本号判断
		if len(incrementalEvents) >= snapshot.Version {
			incrementalEvents = append(incrementalEvents, e)
		} else if len(events) > snapshot.Version {
			// 如果总事件数大于快照版本，说明有增量
			continue
		}
	}

	// 如果没有增量事件，直接返回快照
	if len(incrementalEvents) == 0 {
		return rc, nil
	}

	// 应用增量事件到快照状态
	return b.applyIncrementalEvents(rc, incrementalEvents), nil
}

// deserializeSnapshot 反序列化快照数据为 ReplayContext
func deserializeSnapshot(snapshotData []byte) (*ReplayContext, error) {
	if len(snapshotData) == 0 {
		return nil, fmt.Errorf("empty snapshot data")
	}

	var payload jobstore.SnapshotPayload
	if err := json.Unmarshal(snapshotData, &payload); err != nil {
		return nil, err
	}

	rc := &ReplayContext{
		TaskGraphState:           []byte(payload.TaskGraphState),
		CursorNode:               payload.CursorNode,
		PayloadResults:           []byte(payload.PayloadResults),
		CompletedNodeIDs:         make(map[string]struct{}),
		PayloadResultsByNode:     make(map[string][]byte),
		CompletedCommandIDs:      make(map[string]struct{}),
		CommandResults:           make(map[string][]byte),
		CompletedToolInvocations: make(map[string][]byte),
		PendingToolInvocations:   make(map[string]struct{}),
		StateChangesByStep:       make(map[string][]StateChangeRecord),
		ApprovedCorrelationKeys:  make(map[string]struct{}),
		WorkingMemorySnapshot:    []byte(payload.WorkingMemorySnapshot),
		Phase:                    ExecutionPhase(payload.Phase),
		RecordedTime:             payload.RecordedTime,
		RecordedRandom:           make(map[string][]byte),
		RecordedUUID:             payload.RecordedUUID,
		RecordedHTTP:             make(map[string][]byte),
	}

	// 转换 slice 为 map
	for _, nodeID := range payload.CompletedNodeIDs {
		rc.CompletedNodeIDs[nodeID] = struct{}{}
	}
	for _, cmdID := range payload.CompletedCommandIDs {
		rc.CompletedCommandIDs[cmdID] = struct{}{}
	}
	for _, key := range payload.PendingToolInvocations {
		rc.PendingToolInvocations[key] = struct{}{}
	}
	for _, corrKey := range payload.ApprovedCorrelationKeys {
		rc.ApprovedCorrelationKeys[corrKey] = struct{}{}
	}

	// 转换 map[string]json.RawMessage 为 map[string][]byte
	for k, v := range payload.PayloadResultsByNode {
		rc.PayloadResultsByNode[k] = []byte(v)
	}
	for k, v := range payload.CommandResults {
		rc.CommandResults[k] = []byte(v)
	}
	for k, v := range payload.CompletedToolInvocations {
		rc.CompletedToolInvocations[k] = []byte(v)
	}
	for k, v := range payload.RecordedRandom {
		rc.RecordedRandom[k] = []byte(v)
	}
	for k, v := range payload.RecordedHTTP {
		rc.RecordedHTTP[k] = []byte(v)
	}

	// 反序列化 StateChangesByStep
	for nodeID, changes := range payload.StateChangesByStep {
		var records []StateChangeRecord
		if err := json.Unmarshal(changes, &records); err == nil {
			rc.StateChangesByStep[nodeID] = records
		}
	}

	return rc, nil
}

// applyIncrementalEvents 将增量事件应用到已有的 ReplayContext
func (b *replayBuilder) applyIncrementalEvents(rc *ReplayContext, events []jobstore.JobEvent) *ReplayContext {
	if rc == nil {
		rc = &ReplayContext{
			CompletedNodeIDs:         make(map[string]struct{}),
			PayloadResultsByNode:     make(map[string][]byte),
			CompletedCommandIDs:      make(map[string]struct{}),
			CommandResults:           make(map[string][]byte),
			CompletedToolInvocations: make(map[string][]byte),
			PendingToolInvocations:   make(map[string]struct{}),
			StateChangesByStep:       make(map[string][]StateChangeRecord),
			ApprovedCorrelationKeys:  make(map[string]struct{}),
			RecordedTime:             make(map[string]int64),
			RecordedRandom:           make(map[string][]byte),
			RecordedUUID:             make(map[string]string),
			RecordedHTTP:             make(map[string][]byte),
		}
	}

	// 使用与 BuildFromEvents 相同的逻辑处理增量事件
	var lastType jobstore.EventType
	for _, e := range events {
		lastType = e.Type
		switch e.Type {
		case jobstore.ToolInvocationStarted:
			var pl struct {
				IdempotencyKey string `json:"idempotency_key"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.IdempotencyKey == "" {
				continue
			}
			rc.PendingToolInvocations[pl.IdempotencyKey] = struct{}{}
		case jobstore.PlanGenerated:
			var payload struct {
				TaskGraph json.RawMessage `json:"task_graph"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil || len(payload.TaskGraph) == 0 {
				continue
			}
			rc.TaskGraphState = []byte(payload.TaskGraph)
		case jobstore.NodeFinished:
			var payload struct {
				NodeID         string          `json:"node_id"`
				StepID         string          `json:"step_id"`
				PayloadResults json.RawMessage `json:"payload_results"`
				ResultType     string          `json:"result_type"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				continue
			}
			switch payload.ResultType {
			case "", "success", "pure", "side_effect_committed", "compensated":
				// advance
			default:
				continue
			}
			completedKey := payload.NodeID
			if payload.StepID != "" {
				completedKey = payload.StepID
			}
			rc.CompletedNodeIDs[completedKey] = struct{}{}
			rc.CursorNode = payload.NodeID
			if len(payload.PayloadResults) > 0 {
				rc.PayloadResults = []byte(payload.PayloadResults)
				rc.PayloadResultsByNode[payload.NodeID] = []byte(payload.PayloadResults)
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
			rc.CompletedCommandIDs[cmdID] = struct{}{}
			if len(payload.Result) > 0 {
				rc.CommandResults[cmdID] = []byte(payload.Result)
			}
		case jobstore.ToolInvocationFinished:
			var pl struct {
				IdempotencyKey string          `json:"idempotency_key"`
				Outcome        string          `json:"outcome"`
				Result         json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil {
				continue
			}
			if pl.IdempotencyKey != "" {
				delete(rc.PendingToolInvocations, pl.IdempotencyKey)
			}
			if pl.Outcome != "success" || pl.IdempotencyKey == "" {
				continue
			}
			if len(pl.Result) > 0 {
				rc.CompletedToolInvocations[pl.IdempotencyKey] = []byte(pl.Result)
			} else {
				rc.CompletedToolInvocations[pl.IdempotencyKey] = []byte("{}")
			}
		case jobstore.StateChanged:
			var pl struct {
				NodeID       string              `json:"node_id"`
				StateChanges []StateChangeRecord `json:"state_changes"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil {
				continue
			}
			if pl.NodeID == "" || len(pl.StateChanges) == 0 {
				continue
			}
			rc.StateChangesByStep[pl.NodeID] = append(rc.StateChangesByStep[pl.NodeID], pl.StateChanges...)
		case jobstore.JobWaiting:
			p, err := jobstore.ParseJobWaitingPayload(e.Payload)
			if err != nil || len(p.ResumptionContext) == 0 {
				continue
			}
			var resumption map[string]interface{}
			if json.Unmarshal(p.ResumptionContext, &resumption) != nil {
				continue
			}
			ms, _ := resumption["memory_snapshot"].(map[string]interface{})
			if ms != nil {
				if wm, ok := ms["working_memory"]; ok && wm != nil {
					switch v := wm.(type) {
					case string:
						rc.WorkingMemorySnapshot = []byte(v)
					case []byte:
						rc.WorkingMemorySnapshot = v
					case json.RawMessage:
						rc.WorkingMemorySnapshot = v
					default:
						if b, err := json.Marshal(wm); err == nil {
							rc.WorkingMemorySnapshot = b
						}
					}
				}
			}
		case jobstore.WaitCompleted:
			var pl struct {
				NodeID         string          `json:"node_id"`
				Payload        json.RawMessage `json:"payload"`
				CorrelationKey string          `json:"correlation_key"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.NodeID == "" {
				continue
			}
			rc.CompletedNodeIDs[pl.NodeID] = struct{}{}
			rc.CursorNode = pl.NodeID
			rc.CompletedCommandIDs[pl.NodeID] = struct{}{}
			if pl.CorrelationKey != "" {
				rc.ApprovedCorrelationKeys[pl.CorrelationKey] = struct{}{}
			}
			if len(pl.Payload) > 0 {
				rc.CommandResults[pl.NodeID] = []byte(pl.Payload)
			} else {
				rc.CommandResults[pl.NodeID] = []byte("{}")
			}
		case jobstore.TimerFired:
			var pl struct {
				EffectID string `json:"effect_id"`
				UnixNano int64  `json:"unix_nano"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.EffectID == "" {
				continue
			}
			rc.RecordedTime[pl.EffectID] = pl.UnixNano
		case jobstore.RandomRecorded:
			var pl struct {
				EffectID string          `json:"effect_id"`
				Values   json.RawMessage `json:"values"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.EffectID == "" {
				continue
			}
			rc.RecordedRandom[pl.EffectID] = []byte(pl.Values)
		case jobstore.UUIDRecorded:
			var pl struct {
				EffectID string `json:"effect_id"`
				UUID     string `json:"uuid"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.EffectID == "" {
				continue
			}
			rc.RecordedUUID[pl.EffectID] = pl.UUID
		case jobstore.HTTPRecorded:
			var pl struct {
				EffectID string          `json:"effect_id"`
				Response json.RawMessage `json:"response"`
			}
			if err := json.Unmarshal(e.Payload, &pl); err != nil || pl.EffectID == "" {
				continue
			}
			rc.RecordedHTTP[pl.EffectID] = []byte(pl.Response)
		}
	}

	// 更新 Phase
	switch lastType {
	case jobstore.JobCompleted:
		rc.Phase = PhaseCompleted
	case jobstore.JobFailed:
		rc.Phase = PhaseFailed
	case jobstore.JobCancelled:
		rc.Phase = PhaseCancelled
	default:
		if len(rc.TaskGraphState) > 0 {
			rc.Phase = PhaseExecuting
		} else {
			rc.Phase = PhasePlanning
		}
	}

	return rc
}

// SerializeReplayContext 将 ReplayContext 序列化为快照数据
func SerializeReplayContext(rc *ReplayContext) ([]byte, error) {
	if rc == nil {
		return nil, nil
	}

	payload := jobstore.SnapshotPayload{
		TaskGraphState:           json.RawMessage(rc.TaskGraphState),
		CursorNode:               rc.CursorNode,
		PayloadResults:           json.RawMessage(rc.PayloadResults),
		CompletedNodeIDs:         make([]string, 0, len(rc.CompletedNodeIDs)),
		PayloadResultsByNode:     make(map[string]json.RawMessage),
		CompletedCommandIDs:      make([]string, 0, len(rc.CompletedCommandIDs)),
		CommandResults:           make(map[string]json.RawMessage),
		CompletedToolInvocations: make(map[string]json.RawMessage),
		PendingToolInvocations:   make([]string, 0, len(rc.PendingToolInvocations)),
		StateChangesByStep:       make(map[string]json.RawMessage),
		ApprovedCorrelationKeys:  make([]string, 0, len(rc.ApprovedCorrelationKeys)),
		WorkingMemorySnapshot:    json.RawMessage(rc.WorkingMemorySnapshot),
		Phase:                    int(rc.Phase),
		RecordedTime:             rc.RecordedTime,
		RecordedRandom:           make(map[string]json.RawMessage),
		RecordedUUID:             rc.RecordedUUID,
		RecordedHTTP:             make(map[string]json.RawMessage),
	}

	// 转换 map 为 slice
	for nodeID := range rc.CompletedNodeIDs {
		payload.CompletedNodeIDs = append(payload.CompletedNodeIDs, nodeID)
	}
	for cmdID := range rc.CompletedCommandIDs {
		payload.CompletedCommandIDs = append(payload.CompletedCommandIDs, cmdID)
	}
	for key := range rc.PendingToolInvocations {
		payload.PendingToolInvocations = append(payload.PendingToolInvocations, key)
	}
	for corrKey := range rc.ApprovedCorrelationKeys {
		payload.ApprovedCorrelationKeys = append(payload.ApprovedCorrelationKeys, corrKey)
	}

	// 转换 map[string][]byte 为 map[string]json.RawMessage
	for k, v := range rc.PayloadResultsByNode {
		payload.PayloadResultsByNode[k] = json.RawMessage(v)
	}
	for k, v := range rc.CommandResults {
		payload.CommandResults[k] = json.RawMessage(v)
	}
	for k, v := range rc.CompletedToolInvocations {
		payload.CompletedToolInvocations[k] = json.RawMessage(v)
	}
	for k, v := range rc.RecordedRandom {
		payload.RecordedRandom[k] = json.RawMessage(v)
	}
	for k, v := range rc.RecordedHTTP {
		payload.RecordedHTTP[k] = json.RawMessage(v)
	}

	// 序列化 StateChangesByStep
	for nodeID, changes := range rc.StateChangesByStep {
		if changesJSON, err := json.Marshal(changes); err == nil {
			payload.StateChangesByStep[nodeID] = json.RawMessage(changesJSON)
		}
	}

	return json.Marshal(payload)
}
