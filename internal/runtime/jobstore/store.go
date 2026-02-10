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
)

// JobStore 任务事件存储：事件流 + 调度语义（版本化追加、Claim 租约、Heartbeat、Watch）
type JobStore interface {
	// ListEvents 返回该 job 的完整事件列表（按序）及当前 version（事件条数；0 表示尚无事件）
	ListEvents(ctx context.Context, jobID string) ([]JobEvent, int, error)
	// Append 仅当 expectedVersion 等于当前 version 时追加，返回 newVersion；否则返回 ErrVersionMismatch
	Append(ctx context.Context, jobID string, expectedVersion int, event JobEvent) (newVersion int, err error)
	// Claim 尝试占用一个可执行的 job，成功返回 jobID 与当前 version；无可用 job 返回 ErrNoJob
	Claim(ctx context.Context, workerID string) (jobID string, version int, err error)
	// ClaimJob 占用指定 jobID（用于能力调度：先由 metadata store 按能力选出 Job，再在此占租约）；若该 job 已终止或已被占用则返回错误
	ClaimJob(ctx context.Context, workerID string, jobID string) (version int, err error)
	// Heartbeat 续租；仅当该 job 被同一 workerID 占用时延长过期时间
	Heartbeat(ctx context.Context, workerID string, jobID string) error
	// Watch 订阅该 job 的新事件；实现层在每次对该 job 成功 Append 后向返回的 channel 发送新事件
	Watch(ctx context.Context, jobID string) (<-chan JobEvent, error)
	// ListJobIDsWithExpiredClaim 返回租约已过期的 job_id 列表，供 Scheduler 在 metadata 侧回收孤儿（design/job-state-machine.md）
	ListJobIDsWithExpiredClaim(ctx context.Context) ([]string, error)
}
