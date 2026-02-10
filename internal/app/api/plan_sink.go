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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"rag-platform/internal/agent/runtime/executor"
	"rag-platform/internal/runtime/jobstore"
)

// PlanGeneratedSinkImpl 将 Plan 结果写入事件流，供 Trace/Replay 使用
type PlanGeneratedSinkImpl struct {
	store jobstore.JobStore
}

// NewPlanGeneratedSink 创建 PlanGenerated 事件写入器；store 为 nil 时不写入
func NewPlanGeneratedSink(store jobstore.JobStore) executor.PlanGeneratedSink {
	return &PlanGeneratedSinkImpl{store: store}
}

// AppendPlanGenerated 实现 executor.PlanGeneratedSink
func (s *PlanGeneratedSinkImpl) AppendPlanGenerated(ctx context.Context, jobID string, taskGraphJSON []byte, goal string) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	stepIndex := ver + 1
	planHash := ""
	if len(taskGraphJSON) > 0 {
		h := sha256.Sum256(taskGraphJSON)
		planHash = hex.EncodeToString(h[:])
	}
	payload, err := json.Marshal(map[string]interface{}{
		"task_graph":     json.RawMessage(taskGraphJSON),
		"goal":           goal,
		"plan_hash":      planHash, // 决策记录完整性校验与调试（design/workflow-decision-record.md）
		"trace_span_id":  "plan",
		"parent_span_id": "root",
		"step_index":     stepIndex,
	})
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.PlanGenerated, Payload: payload,
	})
	return err
}
