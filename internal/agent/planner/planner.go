package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"rag-platform/internal/model/llm"
)

// PlanStep 计划中的单步：调用的工具及入参
type PlanStep struct {
	Tool  string         `json:"tool"`
	Input map[string]any `json:"input"`
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
