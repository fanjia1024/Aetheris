package runtime

import (
	"context"
	"sync"
	"time"
)

// ToolCallRecord 单次工具调用记录（供持久化与恢复、Trace 展示）
type ToolCallRecord struct {
	ToolName string    `json:"tool_name"`
	Input    string    `json:"input,omitempty"`
	Output   string    `json:"output,omitempty"`
	At       time.Time `json:"at"`
}

// AgentState 可序列化的 Agent/会话状态（供持久化与恢复）
type AgentState struct {
	AgentID        string           `json:"agent_id"`
	SessionID      string           `json:"session_id"`
	Messages       []Message        `json:"messages"`
	Variables      map[string]any   `json:"variables,omitempty"`
	ToolCalls      []ToolCallRecord `json:"tool_calls,omitempty"`
	Scratchpad     string           `json:"scratchpad,omitempty"`
	LastCheckpoint string           `json:"last_checkpoint,omitempty"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// AgentStateStore 持久化 Agent 状态，供 Worker 恢复会话与多实例共享
type AgentStateStore interface {
	SaveAgentState(ctx context.Context, agentID, sessionID string, state *AgentState) error
	LoadAgentState(ctx context.Context, agentID, sessionID string) (*AgentState, error)
}

// SessionToAgentState 从 Session 转为 AgentState
func SessionToAgentState(s *Session) *AgentState {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := &AgentState{
		AgentID:        s.AgentID,
		SessionID:      s.ID,
		LastCheckpoint: s.LastCheckpoint,
		UpdatedAt:      s.UpdatedAt,
	}
	if len(s.Messages) > 0 {
		state.Messages = make([]Message, len(s.Messages))
		copy(state.Messages, s.Messages)
	}
	if len(s.Variables) > 0 {
		state.Variables = make(map[string]any, len(s.Variables))
		for k, v := range s.Variables {
			state.Variables[k] = v
		}
	}
	if len(s.ToolCalls) > 0 {
		state.ToolCalls = make([]ToolCallRecord, len(s.ToolCalls))
		copy(state.ToolCalls, s.ToolCalls)
	}
	if s.Scratchpad != "" {
		state.Scratchpad = s.Scratchpad
	}
	return state
}

// ApplyAgentState 将 AgentState 应用到 Session（不替换 Session ID/AgentID，只填充 Messages/Variables/LastCheckpoint）
func ApplyAgentState(s *Session, state *AgentState) {
	if s == nil || state == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(state.Messages) > 0 {
		s.Messages = make([]Message, len(state.Messages))
		copy(s.Messages, state.Messages)
	}
	if len(state.Variables) > 0 {
		s.Variables = make(map[string]any, len(state.Variables))
		for k, v := range state.Variables {
			s.Variables[k] = v
		}
	}
	if len(state.ToolCalls) > 0 {
		s.ToolCalls = make([]ToolCallRecord, len(state.ToolCalls))
		copy(s.ToolCalls, state.ToolCalls)
	}
	if state.Scratchpad != "" {
		s.Scratchpad = state.Scratchpad
	}
	s.LastCheckpoint = state.LastCheckpoint
	s.UpdatedAt = state.UpdatedAt
}

// agentStateStoreMem 内存实现
type agentStateStoreMem struct {
	mu   sync.RWMutex
	byKey map[string]*AgentState // key = agentID + "\x00" + sessionID
}

// NewAgentStateStoreMem 创建内存版 AgentStateStore
func NewAgentStateStoreMem() AgentStateStore {
	return &agentStateStoreMem{byKey: make(map[string]*AgentState)}
}

func stateKey(agentID, sessionID string) string {
	return agentID + "\x00" + sessionID
}

func (s *agentStateStoreMem) SaveAgentState(ctx context.Context, agentID, sessionID string, state *AgentState) error {
	if state == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *state
	cp.AgentID = agentID
	cp.SessionID = sessionID
	s.byKey[stateKey(agentID, sessionID)] = &cp
	return nil
}

func (s *agentStateStoreMem) LoadAgentState(ctx context.Context, agentID, sessionID string) (*AgentState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.byKey[stateKey(agentID, sessionID)]
	if !ok || st == nil {
		return nil, nil
	}
	cp := *st
	return &cp, nil
}
