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

package auth

import (
	"context"
	"sync"
)

// MemoryRoleStore 内存角色存储，用于单机或测试；生产可替换为 Postgres 实现
type MemoryRoleStore struct {
	mu    sync.RWMutex
	roles map[string]Role // key: tenantID + "\x00" + userID
}

// NewMemoryRoleStore 创建内存 RoleStore
func NewMemoryRoleStore() *MemoryRoleStore {
	return &MemoryRoleStore{roles: make(map[string]Role)}
}

func (s *MemoryRoleStore) key(tenantID, userID string) string {
	return tenantID + "\x00" + userID
}

// GetUserRole 获取用户在租户中的角色；未设置时返回 RoleUser
func (s *MemoryRoleStore) GetUserRole(ctx context.Context, tenantID string, userID string) (Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.roles[s.key(tenantID, userID)]; ok {
		return r, nil
	}
	return RoleUser, nil
}

// SetUserRole 设置用户在租户中的角色
func (s *MemoryRoleStore) SetUserRole(ctx context.Context, tenantID string, userID string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roles[s.key(tenantID, userID)] = role
	return nil
}
