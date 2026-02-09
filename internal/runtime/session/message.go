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
	"time"

	"rag-platform/internal/model/llm"
)

// Message 对话消息（与 llm.Message 语义对齐，带时间戳）
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ToLLM 转为 llm.Message（供 Planner 等使用）
func (m *Message) ToLLM() llm.Message {
	return llm.Message{Role: m.Role, Content: m.Content}
}

// FromLLM 从 llm.Message 创建
func FromLLM(l llm.Message) *Message {
	return &Message{Role: l.Role, Content: l.Content, Timestamp: time.Now()}
}

// MessagesToLLM 将 []*Message 转为 []llm.Message
func MessagesToLLM(list []*Message) []llm.Message {
	if len(list) == 0 {
		return nil
	}
	out := make([]llm.Message, len(list))
	for i, m := range list {
		out[i] = m.ToLLM()
	}
	return out
}
