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
	"sync"
	"time"

	"github.com/google/uuid"
)

// Session AI 任务进程：唯一状态载体
type Session struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time

	Messages     []*Message       // 对话历史（short-term）
	WorkingState map[string]any   // 当前推理中间态
	ToolCalls    []ToolCallRecord // 工具调用记录

	Metadata map[string]any

	mu sync.RWMutex
}

// New 创建新 Session（ID 由调用方或 Store 分配时可传空）
func New(id string) *Session {
	now := time.Now()
	if id == "" {
		id = "session-" + uuid.New().String()
	}
	return &Session{
		ID:           id,
		CreatedAt:    now,
		UpdatedAt:    now,
		Messages:     nil,
		WorkingState: make(map[string]any),
		ToolCalls:    nil,
		Metadata:     make(map[string]any),
	}
}

// AddMessage 追加一条对话消息
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
	s.Messages = append(s.Messages, &Message{Role: role, Content: content, Timestamp: s.UpdatedAt})
}

// AddObservation 追加一次工具调用观察（结果写回 Session）
func (s *Session) AddObservation(tool string, input map[string]any, output string, errStr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
	s.ToolCalls = append(s.ToolCalls, ToolCallRecord{
		Tool:   tool,
		Input:  input,
		Output: output,
		Err:    errStr,
		At:     s.UpdatedAt,
	})
}

// WorkingStateGet 读取 WorkingState 键
func (s *Session) WorkingStateGet(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.WorkingState[key]
	return v, ok
}

// WorkingStateSet 写入 WorkingState
func (s *Session) WorkingStateSet(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
	if s.WorkingState == nil {
		s.WorkingState = make(map[string]any)
	}
	s.WorkingState[key] = value
}

// CopyMessages 返回 Messages 的副本（供 Planner 等只读使用）
func (s *Session) CopyMessages() []*Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.Messages) == 0 {
		return nil
	}
	out := make([]*Message, len(s.Messages))
	for i, m := range s.Messages {
		out[i] = &Message{Role: m.Role, Content: m.Content, Timestamp: m.Timestamp}
	}
	return out
}

// CopyToolCalls 返回 ToolCalls 的副本
func (s *Session) CopyToolCalls() []ToolCallRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.ToolCalls) == 0 {
		return nil
	}
	out := make([]ToolCallRecord, len(s.ToolCalls))
	copy(out, s.ToolCalls)
	return out
}
