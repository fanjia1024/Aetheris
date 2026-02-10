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

package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/replay"
	"rag-platform/internal/agent/runtime"
)

// StepResultType classifies a step completion (Production Semantics Phase A). See design/step-result-failure-model.md.
type StepResultType string

const (
	StepResultSuccess              StepResultType = "success"
	StepResultRetryableFailure     StepResultType = "retryable_failure"
	StepResultPermanentFailure     StepResultType = "permanent_failure"
	StepResultCompensatableFailure StepResultType = "compensatable_failure"
)

// Sentinel errors for adapters to mark failure kind; Runner uses these to set result_type.
var (
	ErrRetryable     = errors.New("retryable")
	ErrPermanent     = errors.New("permanent")
	ErrCompensatable = errors.New("compensatable")
)

// StepFailure wraps an error with a StepResultType for classification.
type StepFailure struct {
	Type  StepResultType
	Inner error
}

func (e *StepFailure) Error() string {
	if e.Inner != nil {
		return e.Inner.Error()
	}
	return string(e.Type)
}

func (e *StepFailure) Unwrap() error { return e.Inner }

// ClassifyError maps runErr to (resultType, reason). Default is PermanentFailure.
func ClassifyError(runErr error) (StepResultType, string) {
	if runErr == nil {
		return StepResultSuccess, ""
	}
	reason := runErr.Error()
	var sf *StepFailure
	if errors.As(runErr, &sf) {
		return sf.Type, reason
	}
	if errors.Is(runErr, ErrRetryable) {
		return StepResultRetryableFailure, reason
	}
	if errors.Is(runErr, ErrCompensatable) {
		return StepResultCompensatableFailure, reason
	}
	if errors.Is(runErr, ErrPermanent) {
		return StepResultPermanentFailure, reason
	}
	return StepResultPermanentFailure, reason
}

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
// resultType/reason 为 Phase A 失败语义；仅 result_type==success 时 Replay 将节点视为完成
type NodeEventSink interface {
	AppendNodeStarted(ctx context.Context, jobID string, nodeID string, attempt int, workerID string) error
	AppendNodeFinished(ctx context.Context, jobID string, nodeID string, payloadResults []byte, durationMs int64, state string, attempt int, resultType StepResultType, reason string) error
	AppendStateCheckpointed(ctx context.Context, jobID string, nodeID string, stateBefore, stateAfter []byte) error
}

// ToolEventSink 工具调用事件写入：Tool 节点执行前后写入 ToolCalled/ToolReturned，供 Trace/审计与恢复短路
type ToolEventSink interface {
	AppendToolCalled(ctx context.Context, jobID string, nodeID string, toolName string, input []byte) error
	AppendToolReturned(ctx context.Context, jobID string, nodeID string, output []byte) error
	AppendToolResultSummarized(ctx context.Context, jobID string, nodeID string, toolName string, summary string, errMsg string, idempotent bool) error
}

// NodeAndToolEventSink 同时支持节点与工具事件（同一实现可传 Runner 与 Compiler）
type NodeAndToolEventSink interface {
	NodeEventSink
	ToolEventSink
}

// CommandEventSink 命令级事件写入：执行副作用前 command_emitted、成功后立即 command_committed，供 Replay 判定已提交命令永不重放
type CommandEventSink interface {
	AppendCommandEmitted(ctx context.Context, jobID string, nodeID string, commandID string, kind string, input []byte) error
	AppendCommandCommitted(ctx context.Context, jobID string, nodeID string, commandID string, result []byte) error
}

// NodeToolAndCommandEventSink 同时支持节点、工具与命令级事件（同一实现可传 Runner 与 Compiler/Adapter）
type NodeToolAndCommandEventSink interface {
	NodeAndToolEventSink
	CommandEventSink
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
	// completedSet：事件流/Checkpoint 推导的已完成节点集合，runLoop 中步前检查，避免重复执行
	var completedSet map[string]struct{}
	// replayCtx：仅从事件流 Replay 进入时非空，含 CompletedCommandIDs/CommandResults，用于命令级跳过与注入
	var replayCtx *replay.ReplayContext

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
		completedSet = make(map[string]struct{})
		for i, s := range steps {
			completedSet[s.NodeID] = struct{}{}
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
						completedSet = rctx.CompletedNodeIDs
						replayCtx = rctx
						startIndex = 0
						for i, s := range steps {
							if _, done := completedSet[s.NodeID]; !done {
								startIndex = i
								break
							}
						}
						goto runLoop
					}
				}
			}
		}
		// 1.0：API 在 Job 创建时已写 PlanGenerated，上段 Replay 应命中；此处仅兼容无 Plan 的旧 Job
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

	ctx = WithJobID(ctx, j.ID)
	const statusCompleted = 2 // 对应 job.StatusCompleted
	graphBytes, _ := taskGraph.Marshal()
	for i := startIndex; i < len(steps); i++ {
		step := steps[i]
		commandID := step.NodeID // 单命令每节点
		// 命令级跳过：事件流中已 command_committed 的永不重放，仅注入结果并推进游标
		if replayCtx != nil {
			if _, committed := replayCtx.CompletedCommandIDs[commandID]; committed {
				if resultBytes, ok := replayCtx.CommandResults[commandID]; ok && len(resultBytes) > 0 {
					var nodeResult interface{}
					if err := json.Unmarshal(resultBytes, &nodeResult); err == nil {
						if payload.Results == nil {
							payload.Results = make(map[string]any)
						}
						payload.Results[step.NodeID] = nodeResult
					}
				}
				payloadResults, _ := json.Marshal(payload.Results)
				if completedSet != nil {
					if _, done := completedSet[step.NodeID]; !done {
						if r.nodeEventSink != nil {
							_ = r.nodeEventSink.AppendNodeFinished(ctx, j.ID, step.NodeID, payloadResults, 0, "", 0, StepResultSuccess, "")
						}
						completedSet[step.NodeID] = struct{}{}
					}
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
				continue
			}
		}
		if completedSet != nil {
			if _, done := completedSet[step.NodeID]; done {
				continue
			}
		}
		stateBefore, _ := json.Marshal(payload.Results)
		if r.nodeEventSink != nil {
			_ = r.nodeEventSink.AppendNodeStarted(ctx, j.ID, step.NodeID, 1, "")
		}
		stepStart := time.Now()
		ctx = WithAgent(ctx, agent)
		var runErr error
		payload, runErr = step.Run(ctx, payload)
		durationMs := time.Since(stepStart).Milliseconds()
		resultType, reason := ClassifyError(runErr)
		payloadResults, _ := json.Marshal(payload.Results)
		if runErr != nil && len(payloadResults) == 0 {
			payloadResults = []byte("{}")
		}
		if r.nodeEventSink != nil {
			stateStr := "ok"
			if resultType != StepResultSuccess {
				stateStr = string(resultType)
			}
			_ = r.nodeEventSink.AppendNodeFinished(ctx, j.ID, step.NodeID, payloadResults, durationMs, stateStr, 1, resultType, reason)
		}
		if resultType != StepResultSuccess {
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			return fmt.Errorf("executor: 节点 %s 执行失败 (%s): %w", step.NodeID, resultType, runErr)
		}
		if r.nodeEventSink != nil {
			_ = r.nodeEventSink.AppendStateCheckpointed(ctx, j.ID, step.NodeID, stateBefore, payloadResults)
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
