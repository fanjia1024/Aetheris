package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/replay"
	"rag-platform/internal/agent/runtime"
)

// JobStoreForRunner 供 Runner 更新 Job 游标与状态的最小接口，避免 executor 依赖 job 包
type JobStoreForRunner interface {
	UpdateCursor(ctx context.Context, jobID string, cursor string) error
	UpdateStatus(ctx context.Context, jobID string, status int) error
}

// PlanGeneratedSink 规划结果事件化：Plan 成功后由 Runner 调用，便于 Trace/Replay 确定复现
type PlanGeneratedSink interface {
	AppendPlanGenerated(ctx context.Context, jobID string, taskGraphJSON []byte, goal string) error
}

// NodeEventSink 节点级事件写入：RunForJob 每步前后写入 NodeStarted/NodeFinished，供 Replay 重建上下文
type NodeEventSink interface {
	AppendNodeStarted(ctx context.Context, jobID string, nodeID string) error
	AppendNodeFinished(ctx context.Context, jobID string, nodeID string, payloadResults []byte) error
}

// Runner 单轮执行：PlanGoal → Compile → Invoke；可选 Checkpoint/JobStore 时支持 RunForJob 逐节点 checkpoint 与恢复
type Runner struct {
	compiler          *Compiler
	checkpointStore   runtime.CheckpointStore
	jobStore          JobStoreForRunner
	planGeneratedSink PlanGeneratedSink
	nodeEventSink     NodeEventSink
	replayBuilder     replay.ReplayContextBuilder
}

// NewRunner 创建 Runner（仅编译与单次 Invoke）
func NewRunner(compiler *Compiler) *Runner {
	return &Runner{compiler: compiler}
}

// SetCheckpointStores 设置 Checkpoint 与 Job 存储，启用 RunForJob 的 node-level checkpoint 与恢复
func (r *Runner) SetCheckpointStores(cp runtime.CheckpointStore, js JobStoreForRunner) {
	r.checkpointStore = cp
	r.jobStore = js
}

// SetPlanGeneratedSink 设置规划事件写入（可选）；Plan 成功后 Append PlanGenerated，供 Trace/Replay 使用
func (r *Runner) SetPlanGeneratedSink(sink PlanGeneratedSink) {
	r.planGeneratedSink = sink
}

// SetNodeEventSink 设置节点事件写入（可选）；RunForJob 每步前后写入 NodeStarted/NodeFinished，供 Replay 使用
func (r *Runner) SetNodeEventSink(sink NodeEventSink) {
	r.nodeEventSink = sink
}

// SetReplayContextBuilder 设置从事件流重建执行上下文的 Builder（可选）；无 Checkpoint 时尝试从事件恢复
func (r *Runner) SetReplayContextBuilder(b replay.ReplayContextBuilder) {
	r.replayBuilder = b
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

// Job 供 RunForJob 使用的最小 Job 信息（避免 executor 依赖 job 包）
type JobForRunner struct {
	ID      string
	AgentID string
	Goal    string
	Cursor  string
}

// RunForJob 按 Job 执行：有 CheckpointStore/JobStore 时走 Steppable 逐节点执行并落盘 checkpoint、恢复时从 Job.Cursor 继续；否则退化为 Run(ctx, agent, goal)
func (r *Runner) RunForJob(ctx context.Context, agent *runtime.Agent, j *JobForRunner) error {
	if agent == nil || j == nil {
		return fmt.Errorf("executor: agent 或 job 为空")
	}
	if r.checkpointStore == nil || r.jobStore == nil {
		return r.Run(ctx, agent, j.Goal)
	}

	agent.SetStatus(runtime.StatusRunning)
	defer func() { agent.SetStatus(runtime.StatusIdle) }()

	sessionID := ""
	if agent.Session != nil {
		sessionID = agent.Session.ID
	}

	var taskGraph *planner.TaskGraph
	var steps []SteppableStep
	var payload *AgentDAGPayload
	startIndex := 0

	// 与 job.JobStatus 对应，避免 executor 依赖 job 包：2=Completed, 3=Failed
	const statusFailed = 3
	if j.Cursor != "" {
		cp, loadErr := r.checkpointStore.Load(ctx, j.Cursor)
		if loadErr != nil || cp == nil {
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: 恢复 checkpoint %s 失败: %w", j.Cursor, loadErr)
		}
		taskGraph = &planner.TaskGraph{}
		if err := taskGraph.Unmarshal(cp.TaskGraphState); err != nil {
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: 反序列化 TaskGraph 失败: %w", err)
		}
		var compErr error
		steps, compErr = r.compiler.CompileSteppable(ctx, taskGraph, agent)
		if compErr != nil {
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: CompileSteppable 失败: %w", compErr)
		}
		for i, s := range steps {
			if s.NodeID == cp.CursorNode {
				startIndex = i + 1
				break
			}
		}
		payload = NewAgentDAGPayload(j.Goal, agent.ID, sessionID)
		if len(cp.PayloadResults) > 0 {
			if err := json.Unmarshal(cp.PayloadResults, &payload.Results); err != nil {
				_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
				return fmt.Errorf("executor: 反序列化 PayloadResults 失败: %w", err)
			}
		}
	} else {
		// 无 Cursor 时优先尝试从事件流重建上下文（Replay），避免重复 Plan/执行
		if r.replayBuilder != nil {
			rctx, rerr := r.replayBuilder.BuildFromEvents(ctx, j.ID)
			if rerr == nil && rctx != nil {
				recoveredGraph, rerr := rctx.TaskGraph()
				if rerr == nil && recoveredGraph != nil {
					var compErr error
					steps, compErr = r.compiler.CompileSteppable(ctx, recoveredGraph, agent)
					if compErr == nil {
						taskGraph = recoveredGraph
						payload = NewAgentDAGPayload(j.Goal, agent.ID, sessionID)
						if len(rctx.PayloadResults) > 0 {
							_ = json.Unmarshal(rctx.PayloadResults, &payload.Results)
						}
						for i, s := range steps {
							if s.NodeID == rctx.CursorNode {
								startIndex = i + 1
								break
							}
						}
						goto runLoop
					}
				}
			}
		}
		if agent.Planner == nil {
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: agent.Planner 未配置")
		}
		planOut, planErr := agent.Planner.Plan(ctx, j.Goal, agent.Memory)
		if planErr != nil {
			agent.SetStatus(runtime.StatusFailed)
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: Plan 失败: %w", planErr)
		}
		var ok bool
		taskGraph, ok = planOut.(*planner.TaskGraph)
		if !ok || taskGraph == nil {
			agent.SetStatus(runtime.StatusFailed)
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: Planner 未返回 *TaskGraph")
		}
		// 规划事件化：写入 PlanGenerated 便于 Trace/Replay 确定复现
		if r.planGeneratedSink != nil {
			graphBytes, _ := taskGraph.Marshal()
			_ = r.planGeneratedSink.AppendPlanGenerated(ctx, j.ID, graphBytes, j.Goal)
		}
		var compErr error
		steps, compErr = r.compiler.CompileSteppable(ctx, taskGraph, agent)
		if compErr != nil {
			agent.SetStatus(runtime.StatusFailed)
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: CompileSteppable 失败: %w", compErr)
		}
		payload = NewAgentDAGPayload(j.Goal, agent.ID, sessionID)
	}

runLoop:

	const statusCompleted = 2 // 对应 job.StatusCompleted
	graphBytes, _ := taskGraph.Marshal()
	for i := startIndex; i < len(steps); i++ {
		step := steps[i]
		if r.nodeEventSink != nil {
			_ = r.nodeEventSink.AppendNodeStarted(ctx, j.ID, step.NodeID)
		}
		ctx = WithAgent(ctx, agent)
		var runErr error
		payload, runErr = step.Run(ctx, payload)
		if runErr != nil {
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: 节点 %s 执行失败: %w", step.NodeID, runErr)
		}
		payloadResults, _ := json.Marshal(payload.Results)
		if r.nodeEventSink != nil {
			_ = r.nodeEventSink.AppendNodeFinished(ctx, j.ID, step.NodeID, payloadResults)
		}
		cp := runtime.NewNodeCheckpoint(agent.ID, sessionID, j.ID, step.NodeID, graphBytes, payloadResults, nil)
		cpID, saveErr := r.checkpointStore.Save(ctx, cp)
		if saveErr != nil {
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: 保存 checkpoint 失败: %w", saveErr)
		}
		if agent.Session != nil {
			agent.Session.SetLastCheckpoint(cpID)
		}
		_ = r.jobStore.UpdateCursor(ctx, j.ID, cpID)
	}

	_ = r.jobStore.UpdateStatus(ctx, j.ID, statusCompleted)
	return nil
}
