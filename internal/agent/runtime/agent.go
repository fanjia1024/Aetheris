package runtime

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// AgentStatus Agent 运行状态
type AgentStatus int

const (
	StatusIdle AgentStatus = iota
	StatusRunning
	StatusWaitingTool
	StatusSuspended
	StatusFailed
)

func (s AgentStatus) String() string {
	switch s {
	case StatusIdle:
		return "idle"
	case StatusRunning:
		return "running"
	case StatusWaitingTool:
		return "waiting_tool"
	case StatusSuspended:
		return "suspended"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Agent 第一公民：具有状态、记忆、目标，可被调度执行的长期实体
type Agent struct {
	ID        string
	Name      string
	CreatedAt time.Time

	Session *Session
	Memory  MemoryProvider
	Planner PlannerProvider
	Tools   ToolsProvider

	Status AgentStatus
	mu     sync.RWMutex
}

// MemoryProvider 提供 Memory 能力（如 agent/memory.CompositeMemory）
type MemoryProvider interface {
	Recall(ctx interface{}, query string) (interface{}, error)
	Store(ctx interface{}, item interface{}) error
}

// PlannerProvider 提供规划能力（如 agent/planner.Planner）
type PlannerProvider interface {
	Plan(ctx interface{}, goal string, mem interface{}) (interface{}, error)
}

// ToolsProvider 提供工具注册表（如 agent/tools.Registry）
type ToolsProvider interface {
	Get(name string) (interface{}, bool)
	List() []interface{}
}

// NewAgent 创建新 Agent
func NewAgent(id, name string, session *Session, memory MemoryProvider, planner PlannerProvider, tools ToolsProvider) *Agent {
	now := time.Now()
	if id == "" {
		id = "agent-" + uuid.New().String()
	}
	if name == "" {
		name = id
	}
	return &Agent{
		ID:        id,
		Name:      name,
		CreatedAt: now,
		Session:   session,
		Memory:   memory,
		Planner:  planner,
		Tools:    tools,
		Status:   StatusIdle,
	}
}

// SetStatus 设置状态
func (a *Agent) SetStatus(s AgentStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Status = s
}

// GetStatus 读取状态
func (a *Agent) GetStatus() AgentStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Status
}
