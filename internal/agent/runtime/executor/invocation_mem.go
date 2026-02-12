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
	"sync"
)

// ToolInvocationStoreMem 内存实现，单进程有效；多 worker 时需用 PG 等持久化实现
type ToolInvocationStoreMem struct {
	mu    sync.RWMutex
	byKey map[string]*ToolInvocationRecord // idempotency_key -> record
}

// NewToolInvocationStoreMem 创建内存版 ToolInvocation 存储
func NewToolInvocationStoreMem() *ToolInvocationStoreMem {
	return &ToolInvocationStoreMem{byKey: make(map[string]*ToolInvocationRecord)}
}

// GetByJobAndIdempotencyKey 实现 ToolInvocationStore
func (s *ToolInvocationStoreMem) GetByJobAndIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*ToolInvocationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.byKey[idempotencyKey]
	if !ok || r == nil {
		return nil, nil
	}
	if r.JobID != jobID {
		return nil, nil
	}
	// 返回副本，避免调用方修改
	cp := *r
	if len(r.Result) > 0 {
		cp.Result = make([]byte, len(r.Result))
		copy(cp.Result, r.Result)
	}
	return &cp, nil
}

// SetStarted 实现 ToolInvocationStore
func (s *ToolInvocationStoreMem) SetStarted(ctx context.Context, r *ToolInvocationRecord) error {
	if r == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := r.IdempotencyKey
	if key == "" {
		return nil
	}
	existing := s.byKey[key]
	if existing != nil && existing.Committed {
		return nil
	}
	cp := *r
	if len(r.Result) > 0 {
		cp.Result = make([]byte, len(r.Result))
		copy(cp.Result, r.Result)
	}
	s.byKey[key] = &cp
	return nil
}

// SetFinished 实现 ToolInvocationStore；externalID 在内存实现中不持久化
func (s *ToolInvocationStoreMem) SetFinished(ctx context.Context, idempotencyKey string, status string, result []byte, committed bool, externalID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.byKey[idempotencyKey]
	if r == nil {
		r = &ToolInvocationRecord{IdempotencyKey: idempotencyKey}
		s.byKey[idempotencyKey] = r
	}
	r.Status = status
	r.Result = make([]byte, len(result))
	copy(r.Result, result)
	r.Committed = committed
	return nil
}
