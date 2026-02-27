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

	"github.com/google/uuid"
)

// SessionManager 管理 Session 生命周期
type SessionManager interface {
	Create(ctx context.Context) (*Session, error)
	Get(ctx context.Context, id string) (*Session, error)
	GetOrCreate(ctx context.Context, id string) (*Session, error)
	Save(ctx context.Context, s *Session) error
}

// Manager 基于 SessionStore 的实现
type Manager struct {
	store SessionStore
	mu    sync.Mutex
}

// NewManager 创建 SessionManager
func NewManager(store SessionStore) *Manager {
	return &Manager{store: store}
}

// Create 创建新 Session
func (m *Manager) Create(ctx context.Context) (*Session, error) {
	m.mu.Lock()
	id := "session-" + uuid.New().String()
	m.mu.Unlock()
	s := New(id)
	if err := m.store.Put(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Get 按 ID 获取 Session
func (m *Manager) Get(ctx context.Context, id string) (*Session, error) {
	return m.store.Get(ctx, id)
}

// GetOrCreate 若 id 为空则 Create，否则 Get；若 Get not found则创建新 Session 并使用该 id
func (m *Manager) GetOrCreate(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return m.Create(ctx)
	}
	s, err := m.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if s != nil {
		return s, nil
	}
	s = New(id)
	if err := m.store.Put(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Save 持久化 Session
func (m *Manager) Save(ctx context.Context, s *Session) error {
	if s == nil {
		return nil
	}
	return m.store.Put(ctx, s)
}
