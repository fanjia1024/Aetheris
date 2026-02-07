package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rag-platform/internal/agent/executor"
	"rag-platform/internal/agent/memory"
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/tool"
	"rag-platform/internal/tool/registry"
)

// RunResult Agent 单次 Run 的结果
type RunResult struct {
	Answer   string        `json:"answer"`
	Steps    int           `json:"steps"`
	Duration time.Duration `json:"duration"`
}

// Agent 入口：持有 Planner、Executor、ToolRegistry，可选 Memory，执行 Plan -> Execute -> 循环
type Agent struct {
	planner   planner.Planner
	executor  executor.Executor
	registry  *registry.Registry
	shortTerm memory.ShortTermMemory
	working   memory.WorkingMemory
	maxSteps  int
}

// AgentOption 可选配置
type AgentOption func(*Agent)

// WithMaxSteps 设置单次 Run 最大步数
func WithMaxSteps(n int) AgentOption {
	return func(a *Agent) {
		a.maxSteps = n
	}
}

// WithShortTermMemory 设置短期记忆（对话上下文）
func WithShortTermMemory(m memory.ShortTermMemory) AgentOption {
	return func(a *Agent) {
		a.shortTerm = m
	}
}

// WithWorkingMemory 设置工作记忆（任务步骤结果）
func WithWorkingMemory(m memory.WorkingMemory) AgentOption {
	return func(a *Agent) {
		a.working = m
	}
}

// New 创建 Agent
func New(planner planner.Planner, exec executor.Executor, reg *registry.Registry, opts ...AgentOption) *Agent {
	a := &Agent{
		planner:  planner,
		executor: exec,
		registry: reg,
		maxSteps: 20,
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

// Run 执行一次任务：Plan -> ExecuteStep(s) -> 若 next==continue 则带结果再 Plan，直到 finish 或超步数
func (a *Agent) Run(ctx context.Context, sessionID string, userQuery string, history []llm.Message) (*RunResult, error) {
	start := time.Now()
	if a.planner == nil || a.executor == nil || a.registry == nil {
		return &RunResult{
			Answer:   "Agent 未正确配置（缺少 Planner/Executor/Registry）。",
			Duration: time.Since(start),
		}, nil
	}
	schemas, err := a.registry.SchemasForLLM()
	if err != nil {
		return nil, fmt.Errorf("获取工具 Schema 失败: %w", err)
	}
	// 若有短期记忆，从其中加载对话历史并追加本轮用户输入
	if a.shortTerm != nil {
		history = a.shortTerm.GetMessages(sessionID)
		a.shortTerm.Append(sessionID, "user", userQuery)
	}
	totalSteps := 0
	var allStepResults []memory.StepResult
	conversationHistory := make([]llm.Message, 0, len(history)+4)
	conversationHistory = append(conversationHistory, history...)

	for totalSteps < a.maxSteps {
		planResult, err := a.planner.Plan(ctx, userQuery, schemas, conversationHistory)
		if err != nil {
			return nil, fmt.Errorf("Planner 失败: %w", err)
		}
		if len(planResult.Steps) == 0 && planResult.Next == "finish" {
			ans := planResult.FinalAnswer
			if ans == "" {
				ans = "无进一步步骤。"
			}
			a.finishRun(sessionID, ans, allStepResults)
			return &RunResult{Answer: ans, Steps: totalSteps, Duration: time.Since(start)}, nil
		}
		// 执行本轮的 steps
		stepResults, err := executor.ExecuteSteps(ctx, a.executor, planResult.Steps)
		if err != nil {
			return nil, fmt.Errorf("执行步骤失败: %w", err)
		}
		for i, step := range planResult.Steps {
			inputStr := ""
			if len(step.Input) > 0 {
				b, _ := json.Marshal(step.Input)
				inputStr = string(b)
			}
			out := tool.ToolResult{}
			if i < len(stepResults) {
				out = stepResults[i]
			}
			allStepResults = append(allStepResults, memory.StepResult{
				Tool:   step.Tool,
				Input:  inputStr,
				Output: out.Content,
				Err:    out.Err,
			})
		}
		totalSteps += len(planResult.Steps)
		resultsText := executor.FormatStepResultsForLLM(stepResults)
		// 将本轮步骤执行结果加入对话，供下一轮 Plan 使用
		conversationHistory = append(conversationHistory,
			llm.Message{Role: "assistant", Content: "计划步骤执行结果：" + resultsText},
		)
		if planResult.Next == "finish" {
			finalAnswer := planResult.FinalAnswer
			if finalAnswer == "" {
				finalAnswer = "步骤已执行完成。最后结果：" + resultsText
			}
			a.finishRun(sessionID, finalAnswer, allStepResults)
			return &RunResult{Answer: finalAnswer, Steps: totalSteps, Duration: time.Since(start)}, nil
		}
		// next == "continue"：继续下一轮 Plan（conversationHistory 已包含本轮结果）
	}
	ans := "已达到最大步数限制，任务未完成。"
	a.finishRun(sessionID, ans, allStepResults)
	return &RunResult{
		Answer:   ans,
		Steps:    totalSteps,
		Duration: time.Since(start),
	}, nil
}

func (a *Agent) finishRun(sessionID, answer string, stepResults []memory.StepResult) {
	if a.shortTerm != nil {
		a.shortTerm.Append(sessionID, "assistant", answer)
	}
	if a.working != nil {
		a.working.SetStepResults(sessionID, stepResults)
	}
}
