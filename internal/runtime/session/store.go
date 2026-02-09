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

package session

import (
	"context"
	"sync"
)

// SessionStore 存储抽象
type SessionStore interface {
	Get(ctx context.Context, id string) (*Session, error)
	Put(ctx context.Context, s *Session) error
}

// MemoryStore 内存实现（map + mutex）
type MemoryStore struct {
	mu   sync.RWMutex
	sess map[string]*Session
}

// NewMemoryStore 创建内存 Session 存储
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sess: make(map[string]*Session)}
}

// Get 实现 SessionStore
func (m *MemoryStore) Get(ctx context.Context, id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sess[id]
	if !ok {
		return nil, nil
	}
	return s, nil
}

// Put 实现 SessionStore
func (m *MemoryStore) Put(ctx context.Context, s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s == nil {
		return nil
	}
	m.sess[s.ID] = s
	return nil
}
