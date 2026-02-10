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

package api

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/runtime/session"
)

const (
	mainAgentName        = "main_agent"
	mainAgentDescription = "主 Agent：支持检索、生成、文档入库与工作流等工具。"
)

// MemoryCheckPointStore 内存实现的 CheckPointStore，供 ADK Runner 中断/恢复使用
type MemoryCheckPointStore struct {
	mu sync.RWMutex
	m  map[string][]byte
}

// NewMemoryCheckPointStore 创建内存 CheckPointStore
func NewMemoryCheckPointStore() *MemoryCheckPointStore {
	return &MemoryCheckPointStore{m: make(map[string][]byte)}
}

// Get 实现 compose.CheckPointStore
func (s *MemoryCheckPointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[checkPointID]
	return v, ok, nil
}

// Set 实现 compose.CheckPointStore
func (s *MemoryCheckPointStore) Set(_ context.Context, checkPointID string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string][]byte)
	}
	s.m[checkPointID] = data
	return nil
}

// Ensure *MemoryCheckPointStore 实现 adk.CheckPointStore（即 compose.CheckPointStore）
var _ adk.CheckPointStore = (*MemoryCheckPointStore)(nil)

// SessionToADKMessages 将 session 历史转为 adk.Message 列表（仅 user/assistant 文本，最近 maxRounds 轮；0 表示不限制）
func SessionToADKMessages(sess *session.Session, maxRounds int) []adk.Message {
	if sess == nil {
		return nil
	}
	msgs := sess.CopyMessages()
	if len(msgs) == 0 {
		return nil
	}
	if maxRounds > 0 {
		rounds := 0
		for i := len(msgs) - 1; i >= 0 && rounds < maxRounds; i-- {
			if msgs[i].Role == "user" || msgs[i].Role == "assistant" {
				rounds++
			}
		}
		start := 0
		for i, m := range msgs {
			if m.Role == "user" || m.Role == "assistant" {
				rounds--
				if rounds < 0 {
					start = i
					break
				}
			}
		}
		msgs = msgs[start:]
	}
	out := make([]adk.Message, 0, len(msgs))
	for _, m := range msgs {
		role := sessionRoleToSchema(m.Role)
		out = append(out, &schema.Message{Role: role, Content: m.Content})
	}
	return out
}

func sessionRoleToSchema(role string) schema.RoleType {
	switch role {
	case "user":
		return schema.User
	case "assistant":
		return schema.Assistant
	case "system":
		return schema.System
	default:
		return schema.RoleType(role)
	}
}

// ADKMessagesToSession 将 ADK 消息列表追加到 session（仅追加 user/assistant 的 Content）
func ADKMessagesToSession(sess *session.Session, messages []adk.Message) {
	if sess == nil || len(messages) == 0 {
		return
	}
	for _, m := range messages {
		if m == nil || m.Content == "" {
			continue
		}
		role := string(m.Role)
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		sess.AddMessage(role, m.Content)
	}
}

// NewMainADKRunner 创建主 ADK Runner（ChatModelAgent + 全部 Builtin 工具 + 可选 CheckPointStore）
func NewMainADKRunner(ctx context.Context, engine *eino.Engine, checkpointStore adk.CheckPointStore) (*adk.Runner, error) {
	if engine == nil {
		return nil, nil
	}
	tools := eino.GetDefaultTools(engine)
	chatModel, _ := engine.CreateChatModel(ctx)
	cfg := &adk.ChatModelAgentConfig{
		Name:        mainAgentName,
		Description: mainAgentDescription,
		Instruction: "你是一个有帮助的助手，可以使用检索、生成、文档加载与解析、切片、向量化、建索引等工具完成任务。",
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
		},
	}
	if chatModel != nil {
		cfg.Model = chatModel
	}
	agent, err := adk.NewChatModelAgent(ctx, cfg)
	if err != nil {
		return nil, err
	}
	runnerCfg := adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: checkpointStore,
	}
	return adk.NewRunner(ctx, runnerCfg), nil
}
