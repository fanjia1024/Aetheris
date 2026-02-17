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

import (
	"context"
	"time"
)

// ToolInvocationStatus 持久化记录状态（与常见命名对应：pending→started, running→started, succeeded→success, failed→failure）
// Ledger 状态机语义：无记录=NEW，started=INFLIGHT，success/confirmed+Committed=COMMITTED；事件流恢复=RECOVERABLE。见 design/1.0-runtime-semantics.md
const (
	ToolInvocationStatusStarted   = "started"   // 执行前创建，等价 INFLIGHT
	ToolInvocationStatusSuccess   = "success"   // 执行成功并已持久化，等价 COMMITTED
	ToolInvocationStatusFailure   = "failure"   // 执行失败，等价 failed
	ToolInvocationStatusTimeout   = "timeout"   // 执行超时
	ToolInvocationStatusConfirmed = "confirmed" // 已通过 ResourceVerifier 校验，等价 COMMITTED
)

// ToolInvocationRecord 工具调用持久化记录；committed=true 表示外部世界已改变，replay 不得再执行
type ToolInvocationRecord struct {
	InvocationID   string // 唯一
	JobID          string
	StepID         string
	ToolName       string
	ArgsHash       string
	IdempotencyKey string
	Status         string     // started | success | failure | timeout | confirmed
	Result         []byte     // 成功时的 result JSON（result_snapshot）
	Committed      bool       // true 仅当已执行且结果已持久化，跨进程权威
	CreatedAt      time.Time  // 2.1: 创建时间（用于 Evidence Export）
	UpdatedAt      time.Time  // 2.1: 更新时间
	ConfirmedAt    *time.Time // 2.1: 确认时间（可选）
	ExternalID     string     // 2.1: 外部 ID（可选）
}

// ToolInvocationStore 工具调用持久化存储；Runner/Adapter 先查再执行，避免 double-commit
type ToolInvocationStore interface {
	// GetByJobAndIdempotencyKey 按 job_id + idempotency_key 查询；用于执行前判断是否已提交
	GetByJobAndIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*ToolInvocationRecord, error)
	// SetStarted 创建或更新为 started（执行前调用）
	SetStarted(ctx context.Context, r *ToolInvocationRecord) error
	// SetFinished 更新为完成态并设置 committed（执行成功后调用，再写事件）；externalID 可选，非空时写入 tool_invocations.external_id（provenance）
	SetFinished(ctx context.Context, idempotencyKey string, status string, result []byte, committed bool, externalID string) error
	// ListByJobID 列出指定 job 的所有工具调用（2.1 Evidence Export）
	ListByJobID(ctx context.Context, jobID string) ([]ToolInvocationRecord, error)
}
