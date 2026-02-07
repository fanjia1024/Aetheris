package agent

import (
	"context"
	"fmt"
	"time"

	"rag-platform/internal/agent/executor"
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/runtime/session"
	"rag-platform/internal/tool"
)

// RunResult Agent 单次 Run 的结果
type RunResult struct {
	Answer   string        `json:"answer"`
	Steps    int           `json:"steps"`
	Duration time.Duration `json:"duration"`
}

// SchemaProvider 提供供 LLM 使用的工具 Schema（如 tool/registry.Registry 或 agent/tools.Registry）
type SchemaProvider interface {
	SchemasForLLM() ([]byte, error)
}

// Agent 入口：持有 Planner、Executor、SchemaProvider，执行 Plan -> Execute -> 循环；记忆由 Session 承载
type Agent struct {
	planner        planner.Planner
	executor       executor.Executor
	schemaProvider SchemaProvider
	maxSteps       int
}

// AgentOption 可选配置
type AgentOption func(*Agent)

// WithMaxSteps 设置单次 Run 最大步数
func WithMaxSteps(n int) AgentOption {
	return func(a *Agent) {
		a.maxSteps = n
	}
}

// New 创建 Agent（schemaProvider 可为 tool/registry.Registry 或 agent/tools.Registry）
func New(planner planner.Planner, exec executor.Executor, schemaProvider SchemaProvider, opts ...AgentOption) *Agent {
	a := &Agent{
		planner:        planner,
		executor:       exec,
		schemaProvider: schemaProvider,
		maxSteps:       20,
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

// RunWithSession 基于 Session 执行一次任务（Runtime 入口）：先取 session 历史，执行后写回 session
func (a *Agent) RunWithSession(ctx context.Context, sess *session.Session, userQuery string) (*RunResult, error) {
	if sess == nil {
		return a.Run(ctx, "", userQuery, nil)
	}
	history := session.MessagesToLLM(sess.CopyMessages())
	result, err := a.runInternal(ctx, sess.ID, userQuery, history, sess)
	if err != nil {
		return nil, err
	}
	sess.AddMessage("user", userQuery)
	sess.AddMessage("assistant", result.Answer)
	return result, nil
}

// Run 执行一次任务：Plan -> ExecuteStep(s) -> 若 next==continue 则带结果再 Plan，直到 finish 或超步数
func (a *Agent) Run(ctx context.Context, sessionID string, userQuery string, history []llm.Message) (*RunResult, error) {
	return a.runInternal(ctx, sessionID, userQuery, history, nil)
}

func (a *Agent) runInternal(ctx context.Context, sessionID string, userQuery string, history []llm.Message, sess *session.Session) (*RunResult, error) {
	start := time.Now()
	if a.planner == nil || a.executor == nil || a.schemaProvider == nil {
		return &RunResult{
			Answer:   "Agent 未正确配置（缺少 Planner/Executor/Registry）。",
			Duration: time.Since(start),
		}, nil
	}
	schemas, err := a.schemaProvider.SchemasForLLM()
	if err != nil {
		return nil, fmt.Errorf("获取工具 Schema 失败: %w", err)
	}
	// 若无 Session 则创建临时 Session 并灌入 history
	if sess == nil {
		sess = session.New(sessionID)
		for _, m := range history {
			sess.AddMessage(m.Role, m.Content)
		}
	}
	totalSteps := 0

	// 基于 Session 的单步循环：Next -> Execute -> AddObservation
	for totalSteps < a.maxSteps {
		step, err := a.planner.Next(ctx, sess, userQuery, schemas)
		if err != nil {
			return nil, fmt.Errorf("Planner Next 失败: %w", err)
		}
		if step.Final != "" {
			return &RunResult{Answer: step.Final, Steps: totalSteps, Duration: time.Since(start)}, nil
		}
		if step.Tool == "" {
			return &RunResult{Answer: "无进一步步骤。", Steps: totalSteps, Duration: time.Since(start)}, nil
		}
		// 执行单步并写回 Session
		planStep := planner.PlanStep{Tool: step.Tool, Input: step.Input}
		if planStep.Input == nil {
			planStep.Input = make(map[string]any)
		}
		res, err := a.executor.ExecuteStep(ctx, sess, planStep)
		if err != nil {
			res = tool.ToolResult{Err: err.Error()}
		}
		sess.AddObservation(step.Tool, step.Input, res.Content, res.Err)
		totalSteps++
	}
	return &RunResult{
		Answer:   "已达到最大步数限制，任务未完成。",
		Steps:    totalSteps,
		Duration: time.Since(start),
	}, nil
}
