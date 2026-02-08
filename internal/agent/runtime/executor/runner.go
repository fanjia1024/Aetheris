package executor

import (
	"context"
	"fmt"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
)

// Runner 单轮执行：PlanGoal → Compile → Invoke
type Runner struct {
	compiler *Compiler
}

// NewRunner 创建 Runner
func NewRunner(compiler *Compiler) *Runner {
	return &Runner{compiler: compiler}
}

// Run 执行单轮：通过 Agent 的 Planner 得到 TaskGraph，编译为 DAG 并执行
func (r *Runner) Run(ctx context.Context, agent *runtime.Agent, goal string) error {
	if agent == nil {
		return fmt.Errorf("executor: agent 为空")
	}
	if agent.Planner == nil {
		return fmt.Errorf("executor: agent.Planner 未配置")
	}
	agent.SetStatus(runtime.StatusRunning)
	defer func() {
		agent.SetStatus(runtime.StatusIdle)
	}()

	planOut, err := agent.Planner.Plan(ctx, goal, agent.Memory)
	if err != nil {
		agent.SetStatus(runtime.StatusFailed)
		return fmt.Errorf("executor: Plan 失败: %w", err)
	}
	taskGraph, ok := planOut.(*planner.TaskGraph)
	if !ok || taskGraph == nil {
		agent.SetStatus(runtime.StatusFailed)
		return fmt.Errorf("executor: Planner 未返回 *TaskGraph")
	}

	graph, err := r.compiler.Compile(ctx, taskGraph, agent)
	if err != nil {
		agent.SetStatus(runtime.StatusFailed)
		return fmt.Errorf("executor: Compile 失败: %w", err)
	}

	ctx = WithAgent(ctx, agent)
	runnable, err := graph.Compile(ctx)
	if err != nil {
		agent.SetStatus(runtime.StatusFailed)
		return fmt.Errorf("executor: 图编译失败: %w", err)
	}

	sessionID := ""
	if agent.Session != nil {
		sessionID = agent.Session.ID
	}
	payload := NewAgentDAGPayload(goal, agent.ID, sessionID)
	out, err := runnable.Invoke(ctx, payload)
	if err != nil {
		agent.SetStatus(runtime.StatusFailed)
		return fmt.Errorf("executor: Invoke 失败: %w", err)
	}
	_ = out // 结果已在 payload.Results 与 Session 中写回
	return nil
}
