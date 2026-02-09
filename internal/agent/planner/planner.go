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

package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"rag-platform/internal/agent/memory"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/runtime/session"
)

// PlanStep 计划中的单步：调用的工具及入参
type PlanStep struct {
	Tool  string         `json:"tool"`
	Input map[string]any `json:"input"`
}

// Step 单步决策：要么调用工具，要么返回最终回答
type Step struct {
	Tool  string         `json:"tool"`
	Input map[string]any `json:"input"`
	Final string         `json:"final_answer"`
}

// PlanResult Planner 输出：步骤列表、是否继续、最终回答（若 finish）
type PlanResult struct {
	Steps       []PlanStep `json:"steps"`
	Next        string     `json:"next"`         // "continue" | "finish"
	FinalAnswer string     `json:"final_answer"` // 当 next=="finish" 时可为非空
}

// Planner Agent 大脑：将用户问题转为可执行计划
type Planner interface {
	Plan(ctx context.Context, query string, toolsSchemaJSON []byte, history []llm.Message) (*PlanResult, error)
	// Next 基于 Session 做单步决策：返回要执行的工具步骤或最终回答
	Next(ctx context.Context, sess *session.Session, userQuery string, toolsSchemaJSON []byte) (*Step, error)
	// PlanGoal 根据目标与记忆产出 TaskGraph（供 eino 执行内核驱动）
	PlanGoal(ctx context.Context, goal string, mem memory.Memory) (*TaskGraph, error)
}

// LLMPlanner 基于 LLM 的最小实现：生成 JSON Plan
type LLMPlanner struct {
	client llm.Client
}

// NewLLMPlanner 创建基于 LLM 的 Planner
func NewLLMPlanner(client llm.Client) *LLMPlanner {
	return &LLMPlanner{client: client}
}

// Plan 实现 Planner
func (p *LLMPlanner) Plan(ctx context.Context, query string, toolsSchemaJSON []byte, history []llm.Message) (*PlanResult, error) {
	if p.client == nil {
		return &PlanResult{
			Steps:       nil,
			Next:        "finish",
			FinalAnswer: "Planner 未配置 LLM，无法生成计划。",
		}, nil
	}
	toolsDesc := string(toolsSchemaJSON)
	if toolsDesc == "" {
		toolsDesc = "[]"
	}
	systemPrompt := `你是一个任务规划器。根据用户问题和可用工具，输出一个 JSON 计划。
可用工具列表（JSON）：
` + toolsDesc + `

输出格式（仅输出合法 JSON，不要其他文字）：
{"steps":[{"tool":"工具名","input":{...}}, ...], "next":"continue" 或 "finish", "final_answer":"当 next 为 finish 时的最终回答（可选）"}
- 若一步即可完成（如直接回答或只调一个工具即可），next 设为 "finish" 并可在 final_answer 中写回答。
- 若需多步，先输出要执行的 steps，next 设为 "continue"；后续轮次再根据上一步结果决定下一步或 finish。`

	messages := make([]llm.Message, 0, len(history)+2)
	messages = append(messages, llm.Message{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: "user", Content: "用户问题：" + query})

	opts := llm.GenerateOptions{MaxTokens: 2048, Temperature: 0.2}
	reply, err := p.client.ChatWithContext(ctx, messages, opts)
	if err != nil {
		return nil, fmt.Errorf("Planner LLM 调用失败: %w", err)
	}
	reply = strings.TrimSpace(reply)
	// 尝试从回复中提取 JSON（可能被 markdown 包裹）
	if idx := strings.Index(reply, "{"); idx >= 0 {
		if end := strings.LastIndex(reply, "}"); end > idx {
			reply = reply[idx : end+1]
		}
	}
	var result PlanResult
	if err := json.Unmarshal([]byte(reply), &result); err != nil {
		return nil, fmt.Errorf("解析 Planner 输出 JSON 失败: %w", err)
	}
	if result.Next == "" {
		result.Next = "finish"
	}
	return &result, nil
}

// Next 实现 Planner：基于 session 的对话与工具结果做单步决策
func (p *LLMPlanner) Next(ctx context.Context, sess *session.Session, userQuery string, toolsSchemaJSON []byte) (*Step, error) {
	if p.client == nil {
		return &Step{Final: "Planner 未配置 LLM。"}, nil
	}
	toolsDesc := string(toolsSchemaJSON)
	if toolsDesc == "" {
		toolsDesc = "[]"
	}
	// 从 session 构建对话历史（含工具调用结果）
	messages := session.MessagesToLLM(sess.CopyMessages())
	messages = append(messages, llm.Message{Role: "user", Content: "用户问题：" + userQuery})
	for _, tc := range sess.CopyToolCalls() {
		obs := tc.Output
		if tc.Err != "" {
			obs = "error: " + tc.Err
		}
		messages = append(messages, llm.Message{Role: "assistant", Content: "工具 " + tc.Tool + " 结果: " + obs})
	}
	systemPrompt := `你是一个任务规划器。根据用户问题和已有工具调用结果，输出下一步（仅一步）。
可用工具（JSON）：` + toolsDesc + `

输出格式（仅输出合法 JSON）：
- 若需调用工具：{"tool":"工具名","input":{...}}
- 若可给出最终回答：{"final_answer":"回答内容"}
只输出一种，不要同时写 tool 和 final_answer。`

	messages = append([]llm.Message{{Role: "system", Content: systemPrompt}}, messages...)
	messages = append(messages, llm.Message{Role: "user", Content: "请输出下一步（单个 JSON 对象）："})

	opts := llm.GenerateOptions{MaxTokens: 1024, Temperature: 0.2}
	reply, err := p.client.ChatWithContext(ctx, messages, opts)
	if err != nil {
		return nil, fmt.Errorf("Planner Next LLM 调用失败: %w", err)
	}
	reply = strings.TrimSpace(reply)
	if idx := strings.Index(reply, "{"); idx >= 0 {
		if end := strings.LastIndex(reply, "}"); end > idx {
			reply = reply[idx : end+1]
		}
	}
	var step Step
	if err := json.Unmarshal([]byte(reply), &step); err != nil {
		return nil, fmt.Errorf("解析 Planner Next JSON 失败: %w", err)
	}
	return &step, nil
}

// PlanGoal 实现 Planner：根据 goal 与 mem 产出 TaskGraph（最小实现：单节点 llm）
func (p *LLMPlanner) PlanGoal(ctx context.Context, goal string, mem memory.Memory) (*TaskGraph, error) {
	if p.client == nil {
		return &TaskGraph{
			Nodes: []TaskNode{{ID: "n1", Type: NodeLLM, Config: map[string]any{"goal": goal}}},
			Edges: nil,
		}, nil
	}
	items, _ := mem.Recall(ctx, goal)
	contextStr := ""
	for _, it := range items {
		contextStr += it.Content + "\n"
	}
	systemPrompt := `根据用户目标生成任务图（JSON）。格式：{"nodes":[{"id":"n1","type":"tool|workflow|llm","tool_name":"xxx 或 workflow 名"}],"edges":[{"from":"n1","to":"n2"}]}。若单步可完成，一个节点即可。`
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "目标：" + goal + "\n上下文：\n" + contextStr},
	}
	opts := llm.GenerateOptions{MaxTokens: 1024, Temperature: 0.2}
	reply, err := p.client.ChatWithContext(ctx, messages, opts)
	if err != nil {
		return nil, fmt.Errorf("PlanGoal LLM 调用失败: %w", err)
	}
	reply = strings.TrimSpace(reply)
	if idx := strings.Index(reply, "{"); idx >= 0 {
		if end := strings.LastIndex(reply, "}"); end > idx {
			reply = reply[idx : end+1]
		}
	}
	var g TaskGraph
	if err := json.Unmarshal([]byte(reply), &g); err != nil {
		return &TaskGraph{
			Nodes: []TaskNode{{ID: "n1", Type: NodeLLM, Config: map[string]any{"goal": goal}}},
			Edges: nil,
		}, nil
	}
	return &g, nil
}
