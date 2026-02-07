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
