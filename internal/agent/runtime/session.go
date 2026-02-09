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

package runtime

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Message Agent 思考轨迹（working memory 的一条）
type Message struct {
	Role    string    `json:"role"`    // user / assistant / tool / system
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

// Session v1：归属某 Agent，承载当前对话与任务状态
type Session struct {
	ID        string
	AgentID   string

	Messages   []Message
	Variables  map[string]any
	ToolCalls  []ToolCallRecord
	Scratchpad string

	CurrentTask    string
	LastCheckpoint string

	UpdatedAt time.Time

	mu sync.RWMutex
}

// NewSession 创建新 Session
func NewSession(id, agentID string) *Session {
	now := time.Now()
	if id == "" {
		id = "session-" + uuid.New().String()
	}
	return &Session{
		ID:        id,
		AgentID:   agentID,
		Messages:  nil,
		Variables: make(map[string]any),
		UpdatedAt: now,
	}
}

// AddMessage 追加一条消息
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
	s.Messages = append(s.Messages, Message{Role: role, Content: content, Time: s.UpdatedAt})
}

// SetVariable 设置变量
func (s *Session) SetVariable(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
	if s.Variables == nil {
		s.Variables = make(map[string]any)
	}
	s.Variables[key] = value
}

// GetVariable 读取变量
func (s *Session) GetVariable(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.Variables[key]
	return v, ok
}

// AddToolCall 追加一条工具调用记录（供持久化与恢复、Trace）
func (s *Session) AddToolCall(toolName, input, output string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
	s.ToolCalls = append(s.ToolCalls, ToolCallRecord{ToolName: toolName, Input: input, Output: output, At: s.UpdatedAt})
}

// SetScratchpad 设置推理草稿（可选，供恢复上下文）
func (s *Session) SetScratchpad(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
	s.Scratchpad = text
}

// SetCurrentTask 设置当前任务
func (s *Session) SetCurrentTask(task string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
	s.CurrentTask = task
}

// SetLastCheckpoint 设置最近一次 checkpoint ID
func (s *Session) SetLastCheckpoint(cp string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
	s.LastCheckpoint = cp
}

// GetCurrentTask 返回当前任务（并发安全）
func (s *Session) GetCurrentTask() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentTask
}

// GetLastCheckpoint 返回最近 checkpoint ID（并发安全）
func (s *Session) GetLastCheckpoint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastCheckpoint
}

// GetUpdatedAt 返回最后更新时间（并发安全）
func (s *Session) GetUpdatedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.UpdatedAt
}

// CopyMessages 返回 Messages 副本
func (s *Session) CopyMessages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.Messages) == 0 {
		return nil
	}
	out := make([]Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}
