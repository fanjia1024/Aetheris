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

package instance

import (
	"context"
	"sync"
	"time"
)

// StoreMem 内存实现的 AgentInstanceStore
type StoreMem struct {
	mu   sync.RWMutex
	byID map[string]*AgentInstance
}

// NewStoreMem 创建内存版 AgentInstanceStore
func NewStoreMem() *StoreMem {
	return &StoreMem{byID: make(map[string]*AgentInstance)}
}

// Get 按 agentID 查询；不存在返回 nil, nil
func (s *StoreMem) Get(ctx context.Context, agentID string) (*AgentInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := s.byID[agentID]
	if cp == nil {
		return nil, nil
	}
	out := *cp
	if cp.Meta != nil {
		out.Meta = make(map[string]any, len(cp.Meta))
		for k, v := range cp.Meta {
			out.Meta[k] = v
		}
	}
	return &out, nil
}

// Create 创建 Instance；ID 必填
func (s *StoreMem) Create(ctx context.Context, instance *AgentInstance) error {
	if instance == nil || instance.ID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if instance.CreatedAt.IsZero() {
		instance.CreatedAt = now
	}
	if instance.UpdatedAt.IsZero() {
		instance.UpdatedAt = now
	}
	if instance.Status == "" {
		instance.Status = StatusIdle
	}
	cp := *instance
	if instance.Meta != nil {
		cp.Meta = make(map[string]any, len(instance.Meta))
		for k, v := range instance.Meta {
			cp.Meta[k] = v
		}
	}
	s.byID[instance.ID] = &cp
	return nil
}

// UpdateStatus 更新状态
func (s *StoreMem) UpdateStatus(ctx context.Context, agentID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.byID[agentID]; p != nil {
		p.Status = status
		p.UpdatedAt = time.Now()
	}
	return nil
}

// Update 全量更新
func (s *StoreMem) Update(ctx context.Context, instance *AgentInstance) error {
	if instance == nil || instance.ID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.byID[instance.ID]
	if p == nil {
		return nil
	}
	p.TenantID = instance.TenantID
	p.Name = instance.Name
	p.Status = instance.Status
	p.DefaultSessionID = instance.DefaultSessionID
	p.UpdatedAt = time.Now()
	if instance.Meta != nil {
		p.Meta = make(map[string]any, len(instance.Meta))
		for k, v := range instance.Meta {
			p.Meta[k] = v
		}
	}
	return nil
}

// ListByTenant 按租户列出，最多 limit 条
func (s *StoreMem) ListByTenant(ctx context.Context, tenantID string, limit int) ([]*AgentInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*AgentInstance
	for _, p := range s.byID {
		if tenantID != "" && p.TenantID != tenantID {
			continue
		}
		cp := *p
		if p.Meta != nil {
			cp.Meta = make(map[string]any, len(p.Meta))
			for k, v := range p.Meta {
				cp.Meta[k] = v
			}
		}
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
