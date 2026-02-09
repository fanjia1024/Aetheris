package job

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// JobStore 任务存储：创建、查询、更新状态、拉取 Pending、更新恢复游标
type JobStore interface {
	Create(ctx context.Context, job *Job) (string, error)
	Get(ctx context.Context, jobID string) (*Job, error)
	// GetByAgentAndIdempotencyKey 按 Agent 与幂等键查已有 Job，用于 Idempotency-Key 去重；无则返回 nil, nil
	GetByAgentAndIdempotencyKey(ctx context.Context, agentID, idempotencyKey string) (*Job, error)
	ListByAgent(ctx context.Context, agentID string) ([]*Job, error)
	UpdateStatus(ctx context.Context, jobID string, status JobStatus) error
	// UpdateCursor 更新 Job 的恢复游标（Checkpoint ID），用于恢复时从 LastCheckpoint 继续
	UpdateCursor(ctx context.Context, jobID string, cursor string) error
	// ClaimNextPending 原子取出一条 Pending 并置为 Running，无则返回 nil, nil
	ClaimNextPending(ctx context.Context) (*Job, error)
	// Requeue 将 Job 重新入队为 Pending（用于重试；会递增 RetryCount）
	Requeue(ctx context.Context, job *Job) error
	// RequestCancel 请求取消执行中的 Job；Worker 轮询 Get 时发现 CancelRequestedAt 非零则取消 runCtx
	RequestCancel(ctx context.Context, jobID string) error
}

// JobStoreMem 内存实现：map + Pending 队列，Create 时入队，ClaimNextPending 取队首并置 Running
type JobStoreMem struct {
	mu     sync.Mutex
	byID   map[string]*Job
	pending []string
	cond   *sync.Cond
}

// NewJobStoreMem 创建内存 JobStore
func NewJobStoreMem() *JobStoreMem {
	j := &JobStoreMem{
		byID:   make(map[string]*Job),
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

func (s *JobStoreMem) ListByAgent(ctx context.Context, agentID string) ([]*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []*Job
	for _, j := range s.byID {
		if j.AgentID == agentID {
			cp := *j
			list = append(list, &cp)
		}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.pending) > 0 {
		id := s.pending[0]
		s.pending = s.pending[1:]
		j, ok := s.byID[id]
		if !ok {
			continue
		}
		if j.Status != StatusPending {
			continue
		}
		j.Status = StatusRunning
		j.UpdatedAt = time.Now()
		cp := *j
		return &cp, nil
	}
	return nil, nil
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
