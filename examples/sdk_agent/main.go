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

// sdk_agent 演示 pkg/agent/sdk 的高层用法：NewAgent(runtime, agentID)、RegisterTool、Run。
// 本示例使用内存 MockRuntime，直接返回固定回答；对接真实 API 时注入实现 AgentRuntime 的客户端即可。
package main

import (
	"context"
	"fmt"
	"log"

	"rag-platform/pkg/agent/sdk"
)

// MockRuntime 内存实现的 AgentRuntime，仅用于示例：Submit 立即返回 jobID，WaitCompleted 立即返回 completed + 固定回答
type MockRuntime struct {
	answer string
}

func (m *MockRuntime) Submit(ctx context.Context, agentID, goal, sessionID string) (string, error) {
	return "job-mock-1", nil
}

func (m *MockRuntime) WaitCompleted(ctx context.Context, jobID string) (status string, answer string, err error) {
	return "completed", m.answer, nil
}

func main() {
	ctx := context.Background()
	runtime := &MockRuntime{answer: "Hello from SDK Agent (mock)."}
	agent := sdk.NewAgent(runtime, "agent-1")
	agent.RegisterTool("search", func(ctx context.Context, input map[string]any) (string, error) {
		return "mock search result", nil
	})
	answer, err := agent.Run(ctx, "Say hello.")
	if err != nil {
		log.Fatalf("Run: %v", err)
	}
	fmt.Println("Answer:", answer)
}
