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

// GetOrCreate 若 id 为空则 Create，否则 Get；若 Get 不存在则创建新 Session 并使用该 id
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
