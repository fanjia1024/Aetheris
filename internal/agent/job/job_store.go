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

package job

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// JobStore 任务存储：创建、查询、更新状态、拉取 Pending、更新恢复游标；多租户时 tenantID 过滤
type JobStore interface {
	Create(ctx context.Context, job *Job) (string, error)
	Get(ctx context.Context, jobID string) (*Job, error)
	// GetByAgentAndIdempotencyKey 按 Agent 与幂等键查已有 Job，用于 Idempotency-Key 去重；无则返回 nil, nil；tenantID 为空时不过滤
	GetByAgentAndIdempotencyKey(ctx context.Context, agentID, idempotencyKey string) (*Job, error)
	// ListByAgent 按 Agent 列出 Job；tenantID 非空时仅返回该租户下的 Job
	ListByAgent(ctx context.Context, agentID string, tenantID string) ([]*Job, error)
	UpdateStatus(ctx context.Context, jobID string, status JobStatus) error
	// UpdateCursor 更新 Job 的恢复游标（Checkpoint ID），用于恢复时从 LastCheckpoint 继续
	UpdateCursor(ctx context.Context, jobID string, cursor string) error
	// ClaimNextPending 原子取出一条 Pending 并置为 Running，无则返回 nil, nil；tenantID 非空时仅认领该租户的 Job
	ClaimNextPending(ctx context.Context) (*Job, error)
	// ClaimNextPendingFromQueue 从指定队列取出一条 Pending（同队列内按 Priority 降序）；queueClass 为空时等价 ClaimNextPending
	ClaimNextPendingFromQueue(ctx context.Context, queueClass string) (*Job, error)
	// ClaimNextPendingForWorker 从指定队列取出一条 Pending 且该 Job 的 RequiredCapabilities 被 workerCapabilities 覆盖；tenantID 非空时仅认领该租户
	ClaimNextPendingForWorker(ctx context.Context, queueClass string, workerCapabilities []string, tenantID string) (*Job, error)
	// Requeue 将 Job 重新入队为 Pending（用于重试；会递增 RetryCount）
	Requeue(ctx context.Context, job *Job) error
	// RequestCancel 请求取消执行中的 Job；Worker 轮询 Get 时发现 CancelRequestedAt 非零则取消 runCtx
	RequestCancel(ctx context.Context, jobID string) error
	// ReclaimOrphanedJobs 将 status=Running 且 updated_at 早于 (now - olderThan) 的 Job 置回 Pending，供其他 Worker 认领；返回回收数量（design/job-state-machine.md）
	ReclaimOrphanedJobs(ctx context.Context, olderThan time.Duration) (int, error)
}

// jobMatchesCapabilities 判断 Job 的 RequiredCapabilities 是否被 workerCapabilities 覆盖；jobRequired 为空表示任意 Worker 可执行；workerCapabilities 为空表示不按能力过滤
func jobMatchesCapabilities(jobRequired, workerCapabilities []string) bool {
	if len(jobRequired) == 0 {
		return true
	}
	if len(workerCapabilities) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(workerCapabilities))
	for _, c := range workerCapabilities {
		set[c] = struct{}{}
	}
	for _, r := range jobRequired {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}

// JobStoreMem 内存实现：map + Pending 队列，Create 时入队，ClaimNextPending 取队首并置 Running
type JobStoreMem struct {
	mu      sync.Mutex
	byID    map[string]*Job
	pending []string
	cond    *sync.Cond
}

// NewJobStoreMem 创建内存 JobStore
func NewJobStoreMem() *JobStoreMem {
	j := &JobStoreMem{
		byID:    make(map[string]*Job),
		pending: nil,
	}
	j.cond = sync.NewCond(&j.mu)
	return j
}

func (s *JobStoreMem) Create(ctx context.Context, job *Job) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.ID == "" {
		job.ID = "job-" + uuid.New().String()
	}
	if job.TenantID == "" {
		job.TenantID = "default"
	}
	job.Status = StatusPending
	job.CreatedAt = time.Now()
	job.UpdatedAt = job.CreatedAt
	cp := *job
	s.byID[job.ID] = &cp
	s.pending = append(s.pending, job.ID)
	s.cond.Signal()
	return job.ID, nil
}

func (s *JobStoreMem) Get(ctx context.Context, jobID string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.byID[jobID]
	if !ok {
		return nil, nil
	}
	cp := *j
	return &cp, nil
}

func (s *JobStoreMem) GetByAgentAndIdempotencyKey(ctx context.Context, agentID, idempotencyKey string) (*Job, error) {
	if idempotencyKey == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.byID {
		if j.AgentID == agentID && j.IdempotencyKey == idempotencyKey {
			cp := *j
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *JobStoreMem) ListByAgent(ctx context.Context, agentID string, tenantID string) ([]*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []*Job
	for _, j := range s.byID {
		if j.AgentID != agentID {
			continue
		}
		if tenantID != "" && j.TenantID != tenantID {
			continue
		}
		cp := *j
		list = append(list, &cp)
	}
	return list, nil
}

func (s *JobStoreMem) UpdateStatus(ctx context.Context, jobID string, status JobStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.byID[jobID]
	if !ok {
		return nil
	}
	j.Status = status
	j.UpdatedAt = time.Now()
	return nil
}

func (s *JobStoreMem) UpdateCursor(ctx context.Context, jobID string, cursor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.byID[jobID]
	if !ok {
		return nil
	}
	j.Cursor = cursor
	j.UpdatedAt = time.Now()
	return nil
}

func (s *JobStoreMem) ClaimNextPending(ctx context.Context) (*Job, error) {
	return s.ClaimNextPendingFromQueue(ctx, "")
}

func (s *JobStoreMem) ClaimNextPendingFromQueue(ctx context.Context, queueClass string) (*Job, error) {
	return s.ClaimNextPendingForWorker(ctx, queueClass, nil, "")
}

func (s *JobStoreMem) ClaimNextPendingForWorker(ctx context.Context, queueClass string, workerCapabilities []string, tenantID string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bestID string
	var bestPriority int
	var bestIdx int = -1
	for idx, id := range s.pending {
		j, ok := s.byID[id]
		if !ok || j.Status != StatusPending {
			continue
		}
		if tenantID != "" && j.TenantID != tenantID {
			continue
		}
		if queueClass != "" && j.QueueClass != "" && j.QueueClass != queueClass {
			continue
		}
		if !jobMatchesCapabilities(j.RequiredCapabilities, workerCapabilities) {
			continue
		}
		if bestIdx < 0 || j.Priority > bestPriority {
			bestID = id
			bestPriority = j.Priority
			bestIdx = idx
		}
	}
	if bestIdx < 0 {
		return nil, nil
	}
	// 从 pending 中移除 bestID（保持顺序）
	s.pending = append(s.pending[:bestIdx], s.pending[bestIdx+1:]...)
	j := s.byID[bestID]
	j.Status = StatusRunning
	j.UpdatedAt = time.Now()
	cp := *j
	return &cp, nil
}

func (s *JobStoreMem) Requeue(ctx context.Context, job *Job) error {
	if job == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.byID[job.ID]
	if !ok {
		return nil
	}
	j.RetryCount = job.RetryCount + 1
	j.Status = StatusPending
	j.UpdatedAt = time.Now()
	s.pending = append(s.pending, job.ID)
	s.cond.Signal()
	return nil
}

func (s *JobStoreMem) RequestCancel(ctx context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.byID[jobID]
	if !ok {
		return nil
	}
	j.CancelRequestedAt = time.Now()
	j.UpdatedAt = j.CancelRequestedAt
	return nil
}

// ReclaimOrphanedJobs 内存实现：单进程无租约过期语义，返回 0
func (s *JobStoreMem) ReclaimOrphanedJobs(ctx context.Context, olderThan time.Duration) (int, error) {
	_ = olderThan
	return 0, nil
}

// WaitNextPending 阻塞直到有 Pending 或 ctx 取消，然后尝试 Claim；无则返回 nil, nil
func (s *JobStoreMem) WaitNextPending(ctx context.Context) (*Job, error) {
	done := ctx.Done()
	for {
		s.mu.Lock()
		for len(s.pending) == 0 {
			s.cond.Wait()
			select {
			case <-done:
				s.mu.Unlock()
				return nil, ctx.Err()
			default:
			}
		}
		id := s.pending[0]
		s.pending = s.pending[1:]
		j, ok := s.byID[id]
		if !ok {
			s.mu.Unlock()
			continue
		}
		if j.Status != StatusPending {
			s.mu.Unlock()
			continue
		}
		j.Status = StatusRunning
		j.UpdatedAt = time.Now()
		cp := *j
		s.mu.Unlock()
		return &cp, nil
	}
}
