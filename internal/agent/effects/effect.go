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

// Package effects 定义 Effect 类型与 EffectLog 接口，将「会对外部世界产生影响的」操作统一记录到事件流。
// 参见 design/effect-system.md：Replay 时禁止真实调用 LLM/Tool/IO，只读已记录效应注入结果。
package effects

import (
	"context"
	"encoding/json"
	"time"

	"rag-platform/internal/runtime/jobstore"
)

// EffectKind 效应类型（逻辑分类）；存储时映射为 jobstore.EventType，见 design/effect-system.md
type EffectKind string

const (
	// EffectKindLLMResponseRecorded LLM 输出已记录；对应 command_committed
	EffectKindLLMResponseRecorded EffectKind = "llm_response_recorded"
	// EffectKindToolResultRecorded 工具结果已记录；对应 tool_invocation_finished + command_committed
	EffectKindToolResultRecorded EffectKind = "tool_result_recorded"
	// EffectKindExternalCallRecorded 外部调用结果已记录；对应 command_committed 或 state_changed
	EffectKindExternalCallRecorded EffectKind = "external_call_recorded"
	// EffectKindTimerScheduled 定时触发（未来）；对应 timer_fired
	EffectKindTimerScheduled EffectKind = "timer_scheduled"
	// EffectKindRetryDecision 重试决策（可选显式事件）
	EffectKindRetryDecision EffectKind = "retry_decision"
)

// EffectLog 效应日志：所有会对外部世界产生影响的操作经此写入事件流，保证 Replay 时只读不执行。
// 实现可委托 jobstore.JobStore.Append，写入顺序见 design/execution-state-machine.md（command_committed 先于 node_finished）。
type EffectLog interface {
	// AppendEffect 追加一条效应记录；payload 为 JSON，由调用方按 EffectKind 约定构造。
	// 实现内部通过 ListEvents 取当前 version 再 Append，若 version 冲突返回 ErrVersionMismatch。
	AppendEffect(ctx context.Context, jobID string, kind EffectKind, payload []byte) error
}

// JobStoreEffectLog 基于 jobstore.JobStore 的 EffectLog 实现；将 EffectKind 映射为现有事件类型。
type JobStoreEffectLog struct {
	store jobstore.JobStore
}

// NewJobStoreEffectLog 创建委托 JobStore 的 EffectLog；store 为 nil 时 AppendEffect 为 no-op
func NewJobStoreEffectLog(store jobstore.JobStore) *JobStoreEffectLog {
	return &JobStoreEffectLog{store: store}
}

// AppendEffect 实现 EffectLog：根据 kind 映射为 jobstore 事件类型并 Append。
// 当前仅做映射占位；实际写入 command_committed / tool_invocation_finished 仍由 NodeEventSink/Adapter 按现有路径执行，此处供扩展（如 TimerFired、RetryDecision）。
func (e *JobStoreEffectLog) AppendEffect(ctx context.Context, jobID string, kind EffectKind, payload []byte) error {
	if e.store == nil || jobID == "" {
		return nil
	}
	_, ver, err := e.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	eventType := effectKindToEventType(kind)
	if eventType == "" {
		// 未映射的 kind 可写入通用 effect_recorded 或忽略；此处忽略以保持与现有写入路径一致
		return nil
	}
	ev := jobstore.JobEvent{
		JobID:     jobID,
		Type:      eventType,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	_, err = e.store.Append(ctx, jobID, ver, ev)
	return err
}

// effectKindToEventType 将 EffectKind 映射为 jobstore.EventType；仅扩展用，现有 LLM/Tool 仍由 Adapter 写 command_committed
func effectKindToEventType(kind EffectKind) jobstore.EventType {
	switch kind {
	case EffectKindLLMResponseRecorded, EffectKindToolResultRecorded, EffectKindExternalCallRecorded:
		return jobstore.CommandCommitted
	default:
		return ""
	}
}

// PayloadForCommandCommitted 构造 command_committed 的 payload JSON（node_id, command_id, result）
func PayloadForCommandCommitted(nodeID, commandID string, result []byte) ([]byte, error) {
	if commandID == "" {
		commandID = nodeID
	}
	return json.Marshal(map[string]interface{}{
		"node_id":    nodeID,
		"command_id": commandID,
		"result":     json.RawMessage(result),
	})
}
