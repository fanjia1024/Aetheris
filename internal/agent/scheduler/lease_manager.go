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

// Package scheduler 提供调度层抽象：租约管理、心跳、过期回收，使「worker 死亡 → job 自动迁移」为一等能力。参见 design/scheduler-correctness.md。
package scheduler

import (
	"context"
	"time"

	"rag-platform/internal/runtime/jobstore"
)

// LeaseManager 对「租约持有、续租、过期」的接口抽象；可包装 jobstore 的 Claim/Heartbeat/ListJobIDsWithExpiredClaim。
type LeaseManager interface {
	// Claim 尝试占用一个可执行 job，成功返回 jobID、version、attemptID；无可用返回 jobstore.ErrNoJob
	Claim(ctx context.Context, workerID string) (jobID string, version int, attemptID string, err error)
	// ClaimJob 占用指定 jobID（能力调度）；成功返回 version 与 attemptID
	ClaimJob(ctx context.Context, workerID string, jobID string) (version int, attemptID string, err error)
	// Heartbeat 续租；仅当该 job 被同一 workerID 占用时延长过期时间
	Heartbeat(ctx context.Context, workerID string, jobID string) error
	// ListJobIDsWithExpiredClaim 返回租约已过期的 job_id 列表，供 Reclaim 回收
	ListJobIDsWithExpiredClaim(ctx context.Context) ([]string, error)
	// GetCurrentAttemptID 返回该 job 当前持有租约的 attempt_id；无租约或已过期returned empty
	GetCurrentAttemptID(ctx context.Context, jobID string) (string, error)
}

// LeaseConfig 租约与心跳配置
type LeaseConfig struct {
	// LeaseDuration 租约 TTL；超过未 Heartbeat 则视为过期，可被 Reclaim
	LeaseDuration time.Duration
	// HeartbeatInterval 建议的心跳间隔；应小于 LeaseDuration，通常为 LeaseDuration/2 或更短
	HeartbeatInterval time.Duration
}

// leaseManagerImpl 包装 jobstore.JobStore 实现 LeaseManager
type leaseManagerImpl struct {
	store jobstore.JobStore
	cfg   LeaseConfig
}

// NewLeaseManager 从 JobStore 创建 LeaseManager；cfg 可选，用于文档化 TTL/心跳间隔
func NewLeaseManager(store jobstore.JobStore, cfg LeaseConfig) LeaseManager {
	return &leaseManagerImpl{store: store, cfg: cfg}
}

func (m *leaseManagerImpl) Claim(ctx context.Context, workerID string) (string, int, string, error) {
	return m.store.Claim(ctx, workerID)
}

func (m *leaseManagerImpl) ClaimJob(ctx context.Context, workerID string, jobID string) (int, string, error) {
	return m.store.ClaimJob(ctx, workerID, jobID)
}

func (m *leaseManagerImpl) Heartbeat(ctx context.Context, workerID string, jobID string) error {
	return m.store.Heartbeat(ctx, workerID, jobID)
}

func (m *leaseManagerImpl) ListJobIDsWithExpiredClaim(ctx context.Context) ([]string, error) {
	return m.store.ListJobIDsWithExpiredClaim(ctx)
}

func (m *leaseManagerImpl) GetCurrentAttemptID(ctx context.Context, jobID string) (string, error) {
	return m.store.GetCurrentAttemptID(ctx, jobID)
}
