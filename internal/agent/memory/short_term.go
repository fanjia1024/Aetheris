package memory

import (
	"sync"

	"rag-platform/internal/model/llm"
)

const defaultMaxMessages = 50

// ShortTerm 短期记忆的 in-memory 实现
type ShortTerm struct {
	mu       sync.RWMutex
	sessions map[string][]llm.Message
	maxPer   int
}

// NewShortTerm 创建短期记忆，maxMessagesPerSession 为每 session 最多保留消息数，0 表示默认 50
func NewShortTerm(maxMessagesPerSession int) *ShortTerm {
	if maxMessagesPerSession <= 0 {
		maxMessagesPerSession = defaultMaxMessages
	}
	return &ShortTerm{
		sessions: make(map[string][]llm.Message),
		maxPer:   maxMessagesPerSession,
	}
}

// GetMessages 返回该 session 的对话历史（最近 maxPer 条）
func (s *ShortTerm) GetMessages(sessionID string) []llm.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := s.sessions[sessionID]
	if len(list) == 0 {
		return nil
	}
	out := make([]llm.Message, len(list))
	copy(out, list)
	return out
}

// Append 追加一条消息，超过 maxPer 时丢弃最旧的
func (s *ShortTerm) Append(sessionID string, role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.sessions[sessionID]
	list = append(list, llm.Message{Role: role, Content: content})
	if len(list) > s.maxPer {
		list = list[len(list)-s.maxPer:]
	}
	s.sessions[sessionID] = list
}

// Clear 清空该 session 的对话
func (s *ShortTerm) Clear(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}
