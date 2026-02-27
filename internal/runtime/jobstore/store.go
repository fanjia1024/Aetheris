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

package jobstore

import (
	"context"
	"errors"
)

var (
	// ErrNoJob 无可用 job 可被 Claim
	ErrNoJob = errors.New("jobstore: no job available to claim")
	// ErrVersionMismatch Append 时当前 version 与 expectedVersion 不一致
	ErrVersionMismatch = errors.New("jobstore: version mismatch on append")
	// ErrClaimNotFound 或租约已过期，Heartbeat 无法续租
	ErrClaimNotFound = errors.New("jobstore: claim not found or expired")
	// ErrStaleAttempt Append 时 context 中的 attempt_id 与当前租约的 attempt_id 不一致（design/runtime-contract.md §3.2）
	ErrStaleAttempt = errors.New("jobstore: stale attempt, cannot append")
)

type contextKey string

const attemptIDContextKey contextKey = "jobstore.attempt_id"

// WithAttemptID 将当前执行 attempt 的 ID 放入 context；Worker Claim 后注入，Append 时校验（design/runtime-contract.md）
func WithAttemptID(ctx context.Context, attemptID string) context.Context {
	return context.WithValue(ctx, attemptIDContextKey, attemptID)
}

// AttemptIDFromContext 从 context 读取 attempt_id；无则returned empty字符串
func AttemptIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v := ctx.Value(attemptIDContextKey)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// JobStore 任务事件存储：事件流 + 调度语义（版本化追加、Claim 租约、Heartbeat、Watch）。
// 语义见 design/runtime-contract.md（租约 §3、attempt_id §5、Append 校验）。
type JobStore interface {
	// ListEvents 返回该 job 的完整事件列表（按序）及当前 version（事件条数；0 表示尚无事件）
	ListEvents(ctx context.Context, jobID string) ([]JobEvent, int, error)
	// Append 仅当 expectedVersion 等于当前 version 时追加，返回 newVersion；否则返回 ErrVersionMismatch。
	// 若 ctx 带 attempt_id（WithAttemptID），则校验与当前 claim 的 attempt_id 一致，否则返回 ErrStaleAttempt（design/runtime-contract.md §5）。
	Append(ctx context.Context, jobID string, expectedVersion int, event JobEvent) (newVersion int, err error)
	// Claim 尝试占用一个可执行的 job，成功返回 jobID、当前 version 与 attemptID；无可用 job 返回 ErrNoJob（design/runtime-contract.md §3.2）
	Claim(ctx context.Context, workerID string) (jobID string, version int, attemptID string, err error)
	// ClaimJob 占用指定 jobID（用于能力调度）；成功返回 version 与 attemptID；若该 job 已终止或已被占用则返回error
	ClaimJob(ctx context.Context, workerID string, jobID string) (version int, attemptID string, err error)
	// Heartbeat 续租；仅当该 job 被同一 workerID 占用时延长过期时间
	Heartbeat(ctx context.Context, workerID string, jobID string) error
	// Watch 订阅该 job 的新事件；实现层在每次对该 job 成功 Append 后向返回的 channel 发送新事件
	Watch(ctx context.Context, jobID string) (<-chan JobEvent, error)
	// ListJobIDsWithExpiredClaim 返回租约已过期的 job_id 列表，供 Scheduler 在 metadata 侧回收孤儿（design/job-state-machine.md）
	ListJobIDsWithExpiredClaim(ctx context.Context) ([]string, error)
	// GetCurrentAttemptID 返回该 job 当前持有租约的 attempt_id；无租约或已过期returned empty字符串（供 Lease fencing：Ledger Commit 等写操作校验）
	GetCurrentAttemptID(ctx context.Context, jobID string) (string, error)

	// Snapshot methods (2.0 performance optimization for long-running jobs)
	// CreateSnapshot 创建事件流快照，覆盖 job 从版本 0 到 upToVersion 的所有状态
	CreateSnapshot(ctx context.Context, jobID string, upToVersion int, snapshot []byte) error
	// GetLatestSnapshot 获取最新的快照；若无快照返回 nil, nil
	GetLatestSnapshot(ctx context.Context, jobID string) (*JobSnapshot, error)
	// DeleteSnapshotsBefore 删除指定版本之前的所有快照（用于 compaction）
	DeleteSnapshotsBefore(ctx context.Context, jobID string, beforeVersion int) error
}
