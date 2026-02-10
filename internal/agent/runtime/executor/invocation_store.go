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

import "context"

// ToolInvocationStatus 持久化记录状态
const (
	ToolInvocationStatusStarted = "started"
	ToolInvocationStatusSuccess = "success"
	ToolInvocationStatusFailure = "failure"
	ToolInvocationStatusTimeout = "timeout"
)

// ToolInvocationRecord 工具调用持久化记录；committed=true 表示外部世界已改变，replay 不得再执行
type ToolInvocationRecord struct {
	InvocationID   string // 唯一
	JobID          string
	StepID         string
	ToolName       string
	ArgsHash       string
	IdempotencyKey string
	Status         string // started | success | failure | timeout
	Result         []byte // 成功时的 result JSON
	Committed      bool   // true 仅当已执行且结果已持久化，跨进程权威
}

// ToolInvocationStore 工具调用持久化存储；Runner/Adapter 先查再执行，避免 double-commit
type ToolInvocationStore interface {
	// GetByJobAndIdempotencyKey 按 job_id + idempotency_key 查询；用于执行前判断是否已提交
	GetByJobAndIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*ToolInvocationRecord, error)
	// SetStarted 创建或更新为 started（执行前调用）
	SetStarted(ctx context.Context, r *ToolInvocationRecord) error
	// SetFinished 更新为完成态并设置 committed（执行成功后调用，再写事件）
	SetFinished(ctx context.Context, idempotencyKey string, status string, result []byte, committed bool) error
}
