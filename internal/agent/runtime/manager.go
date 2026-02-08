package runtime

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Manager Agent 生命周期管理：Create / Get / List
type Manager struct {
	mu     sync.RWMutex
	agents map[string]*Agent
}

// NewManager 创建 Agent Manager（内存存储）
func NewManager() *Manager {
	return &Manager{
		agents: make(map[string]*Agent),
	}
}

// Create 创建新 Agent
func (m *Manager) Create(ctx context.Context, name string, session *Session, memory MemoryProvider, planner PlannerProvider, tools ToolsProvider) (*Agent, error) {
	id := "agent-" + uuid.New().String()
	if session == nil {
		session = NewSession("", id)
	}
	if session.AgentID == "" {
		session.AgentID = id
	}
	agent := NewAgent(id, name, session, memory, planner, tools)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[id] = agent
	return agent, nil
}

// Get 按 ID 获取 Agent
func (m *Manager) Get(ctx context.Context, id string) (*Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.agents[id]
	if !ok {
		return nil, nil
	}
	return a, nil
}

// List 返回所有 Agent
func (m *Manager) List(ctx context.Context) ([]*Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Agent, 0, len(m.agents))
	for _, a := range m.agents {
		list = append(list, a)
	}
	return list, nil
}

// Delete 移除 Agent（若存在）
func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.agents, id)
	return nil
}
