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

package eino

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

// ChatModel 聊天模型接口
type ChatModel interface {
	// 这里定义必要的方法
}

// Config 配置接口
type Config interface {
	// 这里定义必要的方法
}

// AgentConfig Agent 配置
type AgentConfig struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tools       []string `json:"tools"`
	Model       string   `json:"model"`
	Memory      bool     `json:"memory"`
	Temperature float64  `json:"temperature"`
}

// NewChatModelAgent 创建新的 ChatModelAgent 实例
func NewChatModelAgent(ctx context.Context, config *adk.ChatModelAgentConfig) (adk.Agent, error) {
	agent, err := adk.NewChatModelAgent(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("创建 ChatModelAgent failed: %w", err)
	}

	return agent, nil
}

// CreateAgent 创建 Agent 实例
func CreateAgent(ctx context.Context, agentType string, config *AgentConfig) (adk.Agent, error) {
	switch agentType {
	case "chat_model":
		// 创建 ChatModelAgent 配置
		chatModelConfig := &adk.ChatModelAgentConfig{}

		return NewChatModelAgent(ctx, chatModelConfig)
	default:
		return nil, fmt.Errorf("unsupported input Agent 类型: %s", agentType)
	}
}
