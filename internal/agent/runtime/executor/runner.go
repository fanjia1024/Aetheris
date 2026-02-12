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
	"sort"
	"time"

	"github.com/google/uuid"
	"rag-platform/internal/agent/determinism"
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/replay"
	replaysandbox "rag-platform/internal/agent/replay/sandbox"
	"rag-platform/internal/agent/runtime"
	agenteffects "rag-platform/internal/agent/runtime/effects"
	"rag-platform/pkg/agent/sdk"
)

// StepResultType classifies a step completion (Production Semantics Phase A). See design/step-result-failure-model.md.
type StepResultType string

const (
	StepResultSuccess              StepResultType = "success"               // 通用成功；非 tool 步会记为 StepResultPure
	StepResultPure                 StepResultType = "pure"                  // 无副作用完成（纯计算，如 LLM 步）；replay 可安全重放
	StepResultSideEffectCommitted  StepResultType = "side_effect_committed" // 外部世界已改变，replay 不得重放
	StepResultRetryableFailure     StepResultType = "retryable_failure"
	StepResultPermanentFailure     StepResultType = "permanent_failure"
	StepResultCompensatableFailure StepResultType = "compensatable_failure"
	StepResultCompensated          StepResultType = "compensated" // 已回滚，视为终态
)

// Sentinel errors for adapters to mark failure kind; Runner uses these to set result_type.
var (
	ErrRetryable     = errors.New("retryable")
	ErrPermanent     = errors.New("permanent")
	ErrCompensatable = errors.New("compensatable")
)

// StepFailure wraps an error with a StepResultType for classification; NodeID is the step that failed (for compensation).
type StepFailure struct {
	Type   StepResultType
	Inner  error
	NodeID string
}

func (e *StepFailure) Error() string {
	if e.Inner != nil {
		return e.Inner.Error()
	}
	return string(e.Type)
}

func (e *StepFailure) Unwrap() error { return e.Inner }

// FailedNodeID returns the node_id of the step that failed (for compensation hook).
func (e *StepFailure) FailedNodeID() string { return e.NodeID }

// isStepFailure 表示该 result_type 为失败，应终止 job 并可能触发重试/补偿
func isStepFailure(t StepResultType) bool {
	switch t {
	case StepResultRetryableFailure, StepResultPermanentFailure, StepResultCompensatableFailure:
		return true
	default:
		return false
	}
}

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
// AppendStateCheckpointed 的 opts 可选携带 changed_keys、tool_side_effects、resource_refs 供 Trace UI「本步变更」展示
// AppendJobWaiting 写入 job_waiting，表示 Job 在 Wait 节点挂起（design/job-state-machine.md）
type NodeEventSink interface {
	AppendNodeStarted(ctx context.Context, jobID string, nodeID string, attempt int, workerID string) error
	// AppendNodeFinished 写入 node_finished；stepID 为空时用 nodeID；inputHash 非空时写入 payload 供 Replay 确定性判定（plan 3.3）
	AppendNodeFinished(ctx context.Context, jobID string, nodeID string, payloadResults []byte, durationMs int64, state string, attempt int, resultType StepResultType, reason string, stepID string, inputHash string) error
	// AppendStepCommitted 写入 step_committed，显式 step 提交屏障（2.0 Exactly-Once）；顺序为 command_committed → node_finished → step_committed
	AppendStepCommitted(ctx context.Context, jobID string, nodeID string, stepID string, commandID string, idempotencyKey string) error
	AppendStateCheckpointed(ctx context.Context, jobID string, nodeID string, stateBefore, stateAfter []byte, opts *StateCheckpointOpts) error
	AppendJobWaiting(ctx context.Context, jobID string, nodeID string, waitKind, reason string, expiresAt time.Time, correlationKey string, resumptionContext []byte) error
	// AppendReasoningSnapshot 写入 reasoning_snapshot 事件，供因果调试（design：Causal Debugging）
	AppendReasoningSnapshot(ctx context.Context, jobID string, payload []byte) error
	// AppendStepCompensated 写入 step_compensated 事件（2.0 Tool Contract）；补偿回调执行后调用
	AppendStepCompensated(ctx context.Context, jobID string, nodeID string, stepID string, commandID string, reason string) error
	// Trace 2.0 Cognition（design/trace-2.0-cognition.md）：memory_read / memory_write / plan_evolution，不参与 Replay
	AppendMemoryRead(ctx context.Context, jobID string, nodeID string, stepIndex int, memoryType, keyOrScope, summary string) error
	AppendMemoryWrite(ctx context.Context, jobID string, nodeID string, stepIndex int, memoryType, keyOrScope, summary string) error
	AppendPlanEvolution(ctx context.Context, jobID string, planVersion int, diffSummary string) error
}

// ToolEventSink 工具调用事件写入：Tool 节点执行前后写入 ToolCalled/ToolReturned 与 tool_invocation_started/finished，供 Trace/审计与幂等重放
type ToolEventSink interface {
	AppendToolCalled(ctx context.Context, jobID string, nodeID string, toolName string, input []byte) error
	AppendToolReturned(ctx context.Context, jobID string, nodeID string, output []byte) error
	AppendToolResultSummarized(ctx context.Context, jobID string, nodeID string, toolName string, summary string, errMsg string, idempotent bool) error
	// AppendToolInvocationStarted 写入 tool_invocation_started，含 idempotency_key 供 Replay 查找
	AppendToolInvocationStarted(ctx context.Context, jobID string, nodeID string, payload *ToolInvocationStartedPayload) error
	// AppendToolInvocationFinished 写入 tool_invocation_finished，outcome 为 success 时 Replay 会加入 CompletedToolInvocations
	AppendToolInvocationFinished(ctx context.Context, jobID string, nodeID string, payload *ToolInvocationFinishedPayload) error
}

// NodeAndToolEventSink 同时支持节点与工具事件（同一实现可传 Runner 与 Compiler）
type NodeAndToolEventSink interface {
	NodeEventSink
	ToolEventSink
}

// CommandEventSink 命令级事件写入：执行副作用前 command_emitted、成功后立即 command_committed，供 Replay 判定已提交命令永不重放
type CommandEventSink interface {
	AppendCommandEmitted(ctx context.Context, jobID string, nodeID string, commandID string, kind string, input []byte) error
	// AppendCommandCommitted 写入 command_committed；inputHash 非空时写入 payload 供 Replay 确定性判定（plan 3.3）
	AppendCommandCommitted(ctx context.Context, jobID string, nodeID string, commandID string, result []byte, inputHash string) error
}

// NodeToolAndCommandEventSink 同时支持节点、工具与命令级事件（同一实现可传 Runner 与 Compiler/Adapter）
type NodeToolAndCommandEventSink interface {
	NodeAndToolEventSink
	CommandEventSink
}

// Runner 单轮执行：PlanGoal → Compile → Invoke；可选 Checkpoint/JobStore 时支持 RunForJob 逐节点 checkpoint 与恢复
type Runner struct {
	compiler                *Compiler
	checkpointStore         runtime.CheckpointStore
	jobStore                JobStoreForRunner
	planGeneratedSink       PlanGeneratedSink
	nodeEventSink           NodeEventSink
	recordedEffectsRecorder agenteffects.RecordedEffectsRecorder // 可选；2.0 Step Contract，step 内 Now/UUID/HTTP 经此记录
	compensationRegistry    CompensationRegistry                 // 可选；compensatable_failure 时调用补偿并写 step_compensated
	replayBuilder           replay.ReplayContextBuilder
	replayPolicy            replaysandbox.ReplayPolicy // 可选；Replay 时按策略决定执行或注入
	stepTimeout             time.Duration              // 可选；单步最大执行时间，超时按 retryable_failure（design/scheduler-correctness.md Step timeout）
	stepValidators          []StepValidator            // 可选；Step Contract 2.0 校验（design/step-contract.md）
	maxParallelSteps        int                        // 可选；>0 时同层节点可并行执行（design/dag-parallel-execution.md），0=仅顺序
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

// SetRecordedEffectsRecorder 设置 Recorded Effects 记录器（可选）；2.0 Step Contract，step 内 Now/UUID/HTTP 经此记录，Replay 时从事件注入
func (r *Runner) SetRecordedEffectsRecorder(rec agenteffects.RecordedEffectsRecorder) {
	r.recordedEffectsRecorder = rec
}

// SetCompensationRegistry 设置补偿注册表（可选）；某步返回 compensatable_failure 时调用对应补偿并写 step_compensated
func (r *Runner) SetCompensationRegistry(registry CompensationRegistry) {
	r.compensationRegistry = registry
}

// SetReplayContextBuilder 设置从事件流重建执行上下文的 Builder（可选）；无 Checkpoint 时尝试从事件恢复
func (r *Runner) SetReplayContextBuilder(b replay.ReplayContextBuilder) {
	r.replayBuilder = b
}

// SetReplayPolicy 设置 Replay 策略（可选）；为 nil 时使用默认“已 command_committed 则注入”逻辑
func (r *Runner) SetReplayPolicy(p replaysandbox.ReplayPolicy) {
	r.replayPolicy = p
}

// SetStepTimeout 设置单步最大执行时间；超时后该步按 retryable_failure 处理（design/scheduler-correctness.md）
func (r *Runner) SetStepTimeout(d time.Duration) {
	r.stepTimeout = d
}

// SetStepValidators 设置 Step Contract 校验器（可选）；任一返回错误则视为契约违反，步标记为 permanent_failure（design/step-contract.md § StepValidator 2.0）
func (r *Runner) SetStepValidators(vs ...StepValidator) {
	r.stepValidators = nil
	if len(vs) > 0 {
		r.stepValidators = make([]StepValidator, len(vs))
		copy(r.stepValidators, vs)
	}
}

// runStepValidators 调用所有已注册的 StepValidator，返回第一个错误
func (r *Runner) runStepValidators(ctx context.Context, jobID, stepID, nodeID, nodeType string, runInSandbox func(context.Context) error) error {
	for _, v := range r.stepValidators {
		if v == nil {
			continue
		}
		req := StepValidationRequest{JobID: jobID, StepID: stepID, NodeID: nodeID, NodeType: nodeType, RunInSandbox: runInSandbox}
		if err := v.ValidateStep(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// SetMaxParallelSteps 设置同层最大并行步数；0=仅顺序执行，>0 时同层节点可并行（design/dag-parallel-execution.md）
func (r *Runner) SetMaxParallelSteps(n int) {
	if n < 0 {
		n = 0
	}
	r.maxParallelSteps = n
}

// nextRunnableBatch 返回下一可执行步的索引列表（同层或单步）。completedSet 的 key 为 effectiveStepID。
// 若 levelGroups 为 nil 则按顺序返回第一个未完成的步。
func (r *Runner) nextRunnableBatch(steps []SteppableStep, levelGroups [][]string, completedSet map[string]struct{}, jobID, decisionID string) []int {
	nodeToIndex := make(map[string]int)
	for i, s := range steps {
		nodeToIndex[s.NodeID] = i
	}
	if levelGroups != nil && r.maxParallelSteps > 0 {
		for _, level := range levelGroups {
			var indices []int
			for _, nodeID := range level {
				idx, ok := nodeToIndex[nodeID]
				if !ok {
					continue
				}
				effectiveStepID := DeterministicStepID(jobID, decisionID, idx, steps[idx].NodeType)
				if _, done := completedSet[effectiveStepID]; !done {
					indices = append(indices, idx)
				}
			}
			if len(indices) > 0 {
				return indices
			}
		}
		return nil
	}
	// Sequential: first incomplete step
	for i := range steps {
		effectiveStepID := DeterministicStepID(jobID, decisionID, i, steps[i].NodeType)
		if _, done := completedSet[effectiveStepID]; !done {
			return []int{i}
		}
	}
	return nil
}

// runParallelLevel runs all steps in batch in parallel, merges payload.Results, writes events, updates completedSet and cursor. See design/dag-parallel-execution.md.
func (r *Runner) runParallelLevel(
	ctx context.Context, j *JobForRunner, steps []SteppableStep, batch []int, taskGraph *planner.TaskGraph,
	payload *AgentDAGPayload, agent *runtime.Agent, replayCtx *replay.ReplayContext, completedSet map[string]struct{},
	graphBytes []byte, runLoopDecisionID, sessionID string,
) error {
	type result struct {
		idx     int
		payload *AgentDAGPayload
		err     error
	}
	runCtx := WithJobID(ctx, j.ID)
	runCtx = WithAgent(runCtx, agent)
	if replayCtx != nil && len(replayCtx.CompletedToolInvocations) > 0 {
		runCtx = WithCompletedToolInvocations(runCtx, replayCtx.CompletedToolInvocations)
	}
	if replayCtx != nil && len(replayCtx.PendingToolInvocations) > 0 {
		runCtx = WithPendingToolInvocations(runCtx, replayCtx.PendingToolInvocations)
	}
	if replayCtx != nil && len(replayCtx.StateChangesByStep) > 0 {
		m := make(map[string][]StateChangeForVerify)
		for nodeID, recs := range replayCtx.StateChangesByStep {
			for _, rec := range recs {
				m[nodeID] = append(m[nodeID], StateChangeForVerify{ResourceType: rec.ResourceType, ResourceID: rec.ResourceID, Operation: rec.Operation, ExternalRef: rec.ExternalRef})
			}
		}
		runCtx = WithStateChangesByStep(runCtx, m)
	}
	if replayCtx != nil && len(replayCtx.ApprovedCorrelationKeys) > 0 {
		runCtx = WithApprovedCorrelationKeys(runCtx, replayCtx.ApprovedCorrelationKeys)
	}

	// NodeStarted for each step (deterministic order by node ID)
	nodeIDsForBatch := make([]string, 0, len(batch))
	for _, idx := range batch {
		nodeIDsForBatch = append(nodeIDsForBatch, steps[idx].NodeID)
	}
	sort.Strings(nodeIDsForBatch)
	for _, nodeID := range nodeIDsForBatch {
		if r.nodeEventSink != nil {
			_ = r.nodeEventSink.AppendNodeStarted(ctx, j.ID, nodeID, 1, "")
		}
	}
	ch := make(chan result, len(batch))
	for _, idx := range batch {
		idx := idx
		step := steps[idx]
		effectiveStepID := DeterministicStepID(j.ID, runLoopDecisionID, idx, step.NodeType)
		stepCtx := WithExecutionStepID(runCtx, effectiveStepID)
		if replayCtx != nil {
			stepCtx = runtime.WithClock(stepCtx, runtime.ReplayClock(j.ID, effectiveStepID))
			stepCtx = runtime.WithRNG(stepCtx, runtime.ReplayRNG(j.ID, effectiveStepID))
		} else {
			stepCtx = runtime.WithClock(stepCtx, func() time.Time { return time.Now() })
		}
		if r.stepTimeout > 0 {
			var cancel context.CancelFunc
			stepCtx, cancel = context.WithTimeout(stepCtx, r.stepTimeout)
			defer cancel()
		}
		payloadCopy := &AgentDAGPayload{Goal: payload.Goal, AgentID: payload.AgentID, SessionID: payload.SessionID, Results: make(map[string]any)}
		for k, v := range payload.Results {
			payloadCopy.Results[k] = v
		}
		s, sCtx, eid := step, stepCtx, effectiveStepID
		go func() {
			var runErr error
			if len(r.stepValidators) > 0 {
				runErr = r.runStepValidators(sCtx, j.ID, eid, s.NodeID, s.NodeType, nil)
			}
			if runErr == nil {
				_, runErr = s.Run(sCtx, payloadCopy)
			}
			ch <- result{idx: idx, payload: payloadCopy, err: runErr}
		}()
	}
	var firstErr error
	var failedIdx int
	results := make([]result, 0, len(batch))
	for range batch {
		res := <-ch
		results = append(results, res)
		if res.err != nil && firstErr == nil {
			firstErr = res.err
			failedIdx = res.idx
		}
	}
	if firstErr != nil {
		step := steps[failedIdx]
		effectiveStepID := DeterministicStepID(j.ID, runLoopDecisionID, failedIdx, step.NodeType)
		resultType, reason := ClassifyError(firstErr)
		if errors.Is(firstErr, context.DeadlineExceeded) {
			resultType = StepResultRetryableFailure
			reason = "step timeout"
		}
		if r.nodeEventSink != nil {
			_ = r.nodeEventSink.AppendNodeFinished(ctx, j.ID, step.NodeID, []byte("{}"), 0, string(resultType), 1, resultType, reason, effectiveStepID, "")
		}
		_ = r.jobStore.UpdateStatus(ctx, j.ID, 3)
		return fmt.Errorf("executor: 节点 %s 并行执行失败: %w", step.NodeID, firstErr)
	}
	// Merge results (deterministic order by node ID)
	nodeIDs := make([]string, 0, len(batch))
	for _, res := range results {
		nodeIDs = append(nodeIDs, steps[res.idx].NodeID)
	}
	sort.Strings(nodeIDs)
	for _, nodeID := range nodeIDs {
		for _, res := range results {
			if steps[res.idx].NodeID != nodeID {
				continue
			}
			if v, ok := res.payload.Results[nodeID]; ok {
				if payload.Results == nil {
					payload.Results = make(map[string]any)
				}
				payload.Results[nodeID] = v
			}
			break
		}
	}
	// NodeFinished for each (sorted by node ID)
	payloadResultsMerged, _ := json.Marshal(payload.Results)
	for _, nodeID := range nodeIDs {
		for _, res := range results {
			if steps[res.idx].NodeID != nodeID {
				continue
			}
			step := steps[res.idx]
			effectiveStepID := DeterministicStepID(j.ID, runLoopDecisionID, res.idx, step.NodeType)
			rt := StepResultPure
			if step.NodeType == "tool" {
				rt = StepResultSideEffectCommitted
			}
			if r.nodeEventSink != nil {
				_ = r.nodeEventSink.AppendNodeFinished(ctx, j.ID, step.NodeID, payloadResultsMerged, 0, "ok", 1, rt, "", effectiveStepID, "")
				_ = r.nodeEventSink.AppendStepCommitted(ctx, j.ID, step.NodeID, effectiveStepID, effectiveStepID, "")
			}
			if completedSet != nil {
				completedSet[effectiveStepID] = struct{}{}
			}
			break
		}
	}
	payloadResults, _ := json.Marshal(payload.Results)
	lastIdx := results[len(results)-1].idx
	lastNodeID := steps[lastIdx].NodeID
	cp := runtime.NewNodeCheckpoint(agent.ID, sessionID, j.ID, lastNodeID, graphBytes, payloadResults, nil)
	cpID, saveErr := r.checkpointStore.Save(ctx, cp)
	if saveErr != nil {
		_ = r.jobStore.UpdateStatus(ctx, j.ID, 3)
		return fmt.Errorf("executor: 保存 checkpoint 失败: %w", saveErr)
	}
	if agent.Session != nil {
		agent.Session.SetLastCheckpoint(cpID)
	}
	_ = r.jobStore.UpdateCursor(ctx, j.ID, cpID)
	return nil
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

// Advance 根据当前 state（仅由事件流或 Checkpoint 推导）执行下一原子步并写事件；若无下一步则标记完成并返回 done=true（plan 3.2 事件驱动循环）
func (r *Runner) Advance(ctx context.Context, jobID string, state *replay.ExecutionState, agent *runtime.Agent, j *JobForRunner) (done bool, err error) {
	if state == nil || state.ReplayContext == nil {
		return true, nil
	}
	taskGraph, gerr := state.TaskGraph()
	if gerr != nil || taskGraph == nil {
		return true, nil
	}
	steps, compErr := r.compiler.CompileSteppable(ctx, taskGraph, agent)
	if compErr != nil {
		return false, compErr
	}
	sessionID := ""
	if agent.Session != nil {
		sessionID = agent.Session.ID
	}
	payload := NewAgentDAGPayload(j.Goal, agent.ID, sessionID)
	if len(state.PayloadResults) > 0 {
		_ = json.Unmarshal(state.PayloadResults, &payload.Results)
	}
	completedSet := state.CompletedNodeIDs
	replayCtx := state.ReplayContext
	startIndex := -1
	for i, s := range steps {
		if _, ok := completedSet[s.NodeID]; !ok {
			startIndex = i
			break
		}
	}
	const statusCompleted = 2
	const statusFailed = 3
	if startIndex < 0 || startIndex >= len(steps) {
		_ = r.jobStore.UpdateStatus(ctx, jobID, statusCompleted)
		return true, nil
	}
	step := steps[startIndex]
	commandID := step.NodeID
	graphBytes, _ := taskGraph.Marshal()
	decisionID := PlanDecisionID(graphBytes)
	effectiveStepID := DeterministicStepID(jobID, decisionID, startIndex, step.NodeType)
	ctx = WithJobID(ctx, jobID)
	ctx = WithExecutionStepID(ctx, effectiveStepID)

	// 命令级跳过与注入（同 runLoop）
	if replayCtx != nil {
		if r.replayPolicy != nil {
			decision := r.replayPolicy.Decide(step.NodeID, commandID, step.NodeType, replayCtx)
			if decision.Inject && len(decision.Result) > 0 {
				var nodeResult interface{}
				if err := json.Unmarshal(decision.Result, &nodeResult); err == nil {
					if payload.Results == nil {
						payload.Results = make(map[string]any)
					}
					payload.Results[step.NodeID] = nodeResult
				}
				payloadResults, _ := json.Marshal(payload.Results)
				if _, done := completedSet[effectiveStepID]; !done && r.nodeEventSink != nil {
					rt := StepResultPure
					if step.NodeType == "tool" {
						rt = StepResultSideEffectCommitted
					}
					_ = r.nodeEventSink.AppendNodeFinished(ctx, jobID, step.NodeID, payloadResults, 0, "", 0, rt, "", effectiveStepID, "")
					_ = r.nodeEventSink.AppendStepCommitted(ctx, jobID, step.NodeID, effectiveStepID, commandID, "")
				}
				cp := runtime.NewNodeCheckpoint(agent.ID, sessionID, jobID, step.NodeID, graphBytes, payloadResults, nil)
				cpID, saveErr := r.checkpointStore.Save(ctx, cp)
				if saveErr != nil {
					_ = r.jobStore.UpdateStatus(ctx, jobID, statusFailed)
					return false, fmt.Errorf("executor: 保存 checkpoint 失败: %w", saveErr)
				}
				if agent.Session != nil {
					agent.Session.SetLastCheckpoint(cpID)
				}
				_ = r.jobStore.UpdateCursor(ctx, jobID, cpID)
				return false, nil
			}
			if !decision.Inject && (decision.Kind == replaysandbox.SideEffect || decision.Kind == replaysandbox.External) {
				_ = r.jobStore.UpdateStatus(ctx, jobID, statusFailed)
				return false, fmt.Errorf("executor: replay 时副作用节点 %s 无已记录结果，禁止执行", step.NodeID)
			}
		} else {
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
				if _, done := completedSet[effectiveStepID]; !done && r.nodeEventSink != nil {
					rt := StepResultPure
					if step.NodeType == "tool" {
						rt = StepResultSideEffectCommitted
					}
					_ = r.nodeEventSink.AppendNodeFinished(ctx, jobID, step.NodeID, payloadResults, 0, "", 0, rt, "", effectiveStepID, "")
					_ = r.nodeEventSink.AppendStepCommitted(ctx, jobID, step.NodeID, effectiveStepID, commandID, "")
				}
				cp := runtime.NewNodeCheckpoint(agent.ID, sessionID, jobID, step.NodeID, graphBytes, payloadResults, nil)
				cpID, saveErr := r.checkpointStore.Save(ctx, cp)
				if saveErr != nil {
					_ = r.jobStore.UpdateStatus(ctx, jobID, statusFailed)
					return false, fmt.Errorf("executor: 保存 checkpoint 失败: %w", saveErr)
				}
				if agent.Session != nil {
					agent.Session.SetLastCheckpoint(cpID)
				}
				_ = r.jobStore.UpdateCursor(ctx, jobID, cpID)
				return false, nil
			}
		}
	}
	if _, done := completedSet[effectiveStepID]; done {
		return false, nil
	}
	// 执行一步
	stateBefore, _ := json.Marshal(payload.Results)
	if r.nodeEventSink != nil {
		_ = r.nodeEventSink.AppendNodeStarted(ctx, jobID, step.NodeID, 1, "")
	}
	stepStart := time.Now()
	ctx = WithAgent(ctx, agent)
	if replayCtx != nil && len(replayCtx.CompletedToolInvocations) > 0 {
		ctx = WithCompletedToolInvocations(ctx, replayCtx.CompletedToolInvocations)
	}
	if replayCtx != nil && len(replayCtx.PendingToolInvocations) > 0 {
		ctx = WithPendingToolInvocations(ctx, replayCtx.PendingToolInvocations)
	}
	if replayCtx != nil && len(replayCtx.StateChangesByStep) > 0 {
		m := make(map[string][]StateChangeForVerify)
		for nodeID, recs := range replayCtx.StateChangesByStep {
			for _, rec := range recs {
				m[nodeID] = append(m[nodeID], StateChangeForVerify{ResourceType: rec.ResourceType, ResourceID: rec.ResourceID, Operation: rec.Operation, ExternalRef: rec.ExternalRef})
			}
		}
		ctx = WithStateChangesByStep(ctx, m)
	}
	runCtx := ctx
	// Inject Step Contract helpers so steps stay deterministic on replay (design/step-contract.md).
	if replayCtx != nil {
		runCtx = runtime.WithClock(runCtx, runtime.ReplayClock(jobID, effectiveStepID))
		runCtx = runtime.WithRNG(runCtx, runtime.ReplayRNG(jobID, effectiveStepID))
	} else {
		runCtx = runtime.WithClock(runCtx, func() time.Time { return time.Now() })
	}
	if r.stepTimeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(runCtx, r.stepTimeout)
		defer cancel()
	}
	// 2.0 Deterministic Replay：标记 Replay 模式，step/effects 内可通过 determinism.IsReplay(ctx) 判断；ReplayGuard 可据此 panic
	runCtx = determinism.WithReplay(runCtx, replayCtx != nil)
	// 2.0 Step Contract：注入 RecordedEffects 与 sdk.RuntimeContext，step 内仅能通过 Runtime Now/UUID/HTTP
	if r.recordedEffectsRecorder != nil || replayCtx != nil {
		runCtx = agenteffects.WithRecordedEffects(runCtx, jobID, effectiveStepID, replayCtx, r.recordedEffectsRecorder)
	}
	runCtx = sdk.WithRuntimeContext(runCtx, newRuntimeContextAdapter(jobID, effectiveStepID))
	var runErr error
	if len(r.stepValidators) > 0 {
		if err := r.runStepValidators(runCtx, jobID, effectiveStepID, step.NodeID, step.NodeType, nil); err != nil {
			runErr = err
		}
	}
	if runErr == nil {
		payload, runErr = step.Run(runCtx, payload)
	}
	durationMs := time.Since(stepStart).Milliseconds()
	resultType, reason := ClassifyError(runErr)
	if runErr != nil && errors.Is(runErr, context.DeadlineExceeded) {
		resultType = StepResultRetryableFailure
		reason = "step timeout"
	}
	if resultType == StepResultSuccess {
		if step.NodeType == "tool" {
			resultType = StepResultSideEffectCommitted
		} else {
			resultType = StepResultPure
		}
	}
	payloadResults, _ := json.Marshal(payload.Results)
	if runErr != nil && len(payloadResults) == 0 {
		payloadResults = []byte("{}")
	}
	if r.nodeEventSink != nil {
		stateStr := "ok"
		if resultType != StepResultSuccess && resultType != StepResultPure && resultType != StepResultSideEffectCommitted && resultType != StepResultCompensated {
			stateStr = string(resultType)
		}
		_ = r.nodeEventSink.AppendNodeFinished(ctx, jobID, step.NodeID, payloadResults, durationMs, stateStr, 1, resultType, reason, effectiveStepID, "")
		_ = r.nodeEventSink.AppendStepCommitted(ctx, jobID, step.NodeID, effectiveStepID, effectiveStepID, "")
	}
	if isStepFailure(resultType) {
		if resultType == StepResultCompensatableFailure && r.compensationRegistry != nil {
			if fn := r.compensationRegistry.GetCompensation(step.NodeID); fn != nil {
				_ = fn(ctx, jobID, step.NodeID, effectiveStepID, effectiveStepID)
				if r.nodeEventSink != nil {
					_ = r.nodeEventSink.AppendStepCompensated(ctx, jobID, step.NodeID, effectiveStepID, effectiveStepID, reason)
				}
			}
		}
		_ = r.jobStore.UpdateStatus(ctx, jobID, statusFailed)
		sf := &StepFailure{Type: resultType, Inner: runErr, NodeID: step.NodeID}
		return false, fmt.Errorf("executor: 节点 %s 执行失败 (%s): %w", step.NodeID, resultType, sf)
	}
	if r.nodeEventSink != nil {
		opts := &StateCheckpointOpts{ChangedKeys: ChangedKeysFromState(stateBefore, payloadResults)}
		_ = r.nodeEventSink.AppendStateCheckpointed(ctx, jobID, step.NodeID, stateBefore, payloadResults, opts)
	}
	cp := runtime.NewNodeCheckpoint(agent.ID, sessionID, jobID, step.NodeID, graphBytes, payloadResults, nil)
	cpID, saveErr := r.checkpointStore.Save(ctx, cp)
	if saveErr != nil {
		_ = r.jobStore.UpdateStatus(ctx, jobID, statusFailed)
		return false, fmt.Errorf("executor: 保存 checkpoint 失败: %w", saveErr)
	}
	if agent.Session != nil {
		agent.Session.SetLastCheckpoint(cpID)
	}
	_ = r.jobStore.UpdateCursor(ctx, jobID, cpID)
	return false, nil
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
		// 确定性步身份：completedSet 同时按 execution_step_id 登记，供 runLoop 用 effectiveStepID 检查（design/step-identity.md）
		decisionID := PlanDecisionID(cp.TaskGraphState)
		for idx := 0; idx < startIndex && idx < len(steps); idx++ {
			s := steps[idx]
			effectiveStepID := DeterministicStepID(j.ID, decisionID, idx, s.NodeType)
			completedSet[effectiveStepID] = struct{}{}
		}
		payload = NewAgentDAGPayload(j.Goal, agent.ID, sessionID)
		if len(cp.PayloadResults) > 0 {
			if err := json.Unmarshal(cp.PayloadResults, &payload.Results); err != nil {
				_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
				return fmt.Errorf("executor: 反序列化 PayloadResults 失败: %w", err)
			}
		}
		// 有 replayBuilder 时从 checkpoint 构建 state，走事件驱动循环
		if r.replayBuilder != nil {
			rc := &replay.ReplayContext{
				TaskGraphState:           cp.TaskGraphState,
				CursorNode:               cp.CursorNode,
				PayloadResults:           cp.PayloadResults,
				CompletedNodeIDs:         completedSet,
				PayloadResultsByNode:     make(map[string][]byte),
				CompletedCommandIDs:      make(map[string]struct{}),
				CommandResults:           make(map[string][]byte),
				CompletedToolInvocations: make(map[string][]byte),
				StateChangesByStep:       make(map[string][]replay.StateChangeRecord),
				Phase:                    replay.PhaseExecuting,
			}
			state := replay.NewExecutionState(rc)
			for {
				done, advErr := r.Advance(ctx, j.ID, state, agent, j)
				if advErr != nil {
					return advErr
				}
				if done {
					return nil
				}
				rctx, _ := r.replayBuilder.BuildFromEvents(ctx, j.ID)
				if rctx == nil {
					break // 回退到 runLoop
				}
				state = replay.NewExecutionState(rctx)
			}
			goto runLoop
		}
	} else {
		// 无 Cursor 时优先从事件流重建状态，走事件驱动循环：state → Advance → 再 state（plan 3.2）。
		// Replay 协议（design/effect-system.md）：禁止真实调用 LLM/Tool/IO，只读 PlanGenerated、CommandCommitted、ToolInvocationFinished 注入结果。
		if r.replayBuilder != nil {
			rctx, rerr := r.replayBuilder.BuildFromEvents(ctx, j.ID)
			if rerr == nil && rctx != nil {
				if recoveredGraph, rerr := rctx.TaskGraph(); rerr == nil && recoveredGraph != nil {
					if len(rctx.WorkingMemorySnapshot) > 0 && agent != nil && agent.Session != nil {
						var as runtime.AgentState
						if json.Unmarshal(rctx.WorkingMemorySnapshot, &as) == nil {
							runtime.ApplyAgentState(agent.Session, &as)
						}
					}
					state := replay.NewExecutionState(rctx)
					for {
						done, advErr := r.Advance(ctx, j.ID, state, agent, j)
						if advErr != nil {
							return advErr
						}
						if done {
							return nil
						}
						// 刷新 state 与持久化事件一致
						rctx, _ = r.replayBuilder.BuildFromEvents(ctx, j.ID)
						if rctx == nil {
							break
						}
						if len(rctx.WorkingMemorySnapshot) > 0 && agent != nil && agent.Session != nil {
							var as runtime.AgentState
							if json.Unmarshal(rctx.WorkingMemorySnapshot, &as) == nil {
								runtime.ApplyAgentState(agent.Session, &as)
							}
						}
						state = replay.NewExecutionState(rctx)
					}
				}
			}
		}
		// design/runtime-contract.md §4：决策来源可追溯 — 事件流中必须存在 PlanGenerated 才执行；禁止无记录时重新规划
		_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
		return fmt.Errorf("executor: 事件流中无 PlanGenerated，禁止执行（design/runtime-contract.md）")
	}

runLoop:

	ctx = WithJobID(ctx, j.ID)
	const statusCompleted = 2 // 对应 job.StatusCompleted
	const statusWaiting = 5   // 对应 job.StatusWaiting（design/job-state-machine.md）
	graphBytes, _ := taskGraph.Marshal()
	runLoopDecisionID := PlanDecisionID(graphBytes)
	levelGroups, _ := LevelGroups(taskGraph)
	for {
		batch := r.nextRunnableBatch(steps, levelGroups, completedSet, j.ID, runLoopDecisionID)
		if len(batch) == 0 {
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusCompleted)
			return nil
		}
		hasWait := false
		for _, idx := range batch {
			if steps[idx].NodeType == planner.NodeWait {
				hasWait = true
				break
			}
		}
		if len(batch) > 1 && r.maxParallelSteps > 0 && !hasWait {
			if err := r.runParallelLevel(ctx, j, steps, batch, taskGraph, payload, agent, replayCtx, completedSet, graphBytes, runLoopDecisionID, sessionID); err != nil {
				return err
			}
			continue
		}
		i := batch[0]
		step := steps[i]
		effectiveStepID := DeterministicStepID(j.ID, runLoopDecisionID, i, step.NodeType)
		ctx = WithExecutionStepID(ctx, effectiveStepID)
		commandID := step.NodeID // 单命令每节点
		// 命令级跳过（design/effect-system.md）：事件流中已 command_committed 的永不重放，仅注入结果并推进游标（或按 ReplayPolicy 决策）
		if replayCtx != nil {
			if r.replayPolicy != nil {
				decision := r.replayPolicy.Decide(step.NodeID, commandID, step.NodeType, replayCtx)
				if decision.Inject && len(decision.Result) > 0 {
					var nodeResult interface{}
					if err := json.Unmarshal(decision.Result, &nodeResult); err == nil {
						if payload.Results == nil {
							payload.Results = make(map[string]any)
						}
						payload.Results[step.NodeID] = nodeResult
					}
					payloadResults, _ := json.Marshal(payload.Results)
					if completedSet != nil {
						if _, done := completedSet[effectiveStepID]; !done {
							rt := StepResultPure
							if step.NodeType == "tool" {
								rt = StepResultSideEffectCommitted
							}
							if r.nodeEventSink != nil {
								_ = r.nodeEventSink.AppendNodeFinished(ctx, j.ID, step.NodeID, payloadResults, 0, "", 0, rt, "", effectiveStepID, "")
								_ = r.nodeEventSink.AppendStepCommitted(ctx, j.ID, step.NodeID, effectiveStepID, commandID, "")
							}
							completedSet[effectiveStepID] = struct{}{}
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
				if !decision.Inject && (decision.Kind == replaysandbox.SideEffect || decision.Kind == replaysandbox.External) {
					_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
					return fmt.Errorf("executor: replay 时副作用节点 %s 无已记录结果，禁止执行", step.NodeID)
				}
			} else {
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
						if _, done := completedSet[effectiveStepID]; !done {
							rt := StepResultPure
							if step.NodeType == "tool" {
								rt = StepResultSideEffectCommitted
							}
							if r.nodeEventSink != nil {
								_ = r.nodeEventSink.AppendNodeFinished(ctx, j.ID, step.NodeID, payloadResults, 0, "", 0, rt, "", effectiveStepID, "")
								_ = r.nodeEventSink.AppendStepCommitted(ctx, j.ID, step.NodeID, effectiveStepID, commandID, "")
							}
							completedSet[effectiveStepID] = struct{}{}
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
		}
		if completedSet != nil {
			if _, done := completedSet[effectiveStepID]; done {
				continue
			}
		}
		stateBefore, _ := json.Marshal(payload.Results)
		if r.nodeEventSink != nil {
			_ = r.nodeEventSink.AppendNodeStarted(ctx, j.ID, step.NodeID, 1, "")
		}
		// Wait 节点：不执行，写 job_waiting 并置为 Waiting，由 API signal 后重新入队继续（design/job-state-machine.md）
		if step.NodeType == planner.NodeWait {
			waitKind, reason, waitChannel := "", "", ""
			var expiresAt time.Time
			for _, n := range taskGraph.Nodes {
				if n.ID == step.NodeID && n.Config != nil {
					if k, ok := n.Config["wait_kind"].(string); ok {
						waitKind = k
					}
					if r, ok := n.Config["reason"].(string); ok {
						reason = r
					}
					if ch, ok := n.Config["channel"].(string); ok {
						waitChannel = ch
					}
					if e, ok := n.Config["expires_at"].(string); ok {
						if t, err := time.Parse(time.RFC3339, e); err == nil {
							expiresAt = t
						}
					}
					break
				}
			}
			if expiresAt.IsZero() {
				expiresAt = time.Now().Add(24 * time.Hour)
			}
			if r.nodeEventSink != nil {
				correlationKey := "wait-" + uuid.New().String()
				if waitKind == planner.WaitKindMessage && waitChannel != "" {
					correlationKey = waitChannel
				}
				// Continuation: 保存等待时的完整上下文（payload.Results snapshot + plan_decision_id），恢复时绑定 state（design/agent-process-model.md § Continuation Semantics）
				resumptionCtx := map[string]interface{}{
					"payload_results":  payload.Results,
					"plan_decision_id": PlanDecisionID(graphBytes),
					"cursor_node":      step.NodeID,
				}
				if agent != nil && agent.Session != nil {
					state := runtime.SessionToAgentState(agent.Session)
					if state != nil {
						wm, _ := json.Marshal(state)
						resumptionCtx["memory_snapshot"] = map[string]interface{}{
							"working_memory": json.RawMessage(wm),
							"snapshot_at":    time.Now().Format(time.RFC3339),
						}
					}
				}
				resumptionBytes, _ := json.Marshal(resumptionCtx)
				_ = r.nodeEventSink.AppendJobWaiting(ctx, j.ID, step.NodeID, waitKind, reason, expiresAt, correlationKey, resumptionBytes)
			}
			// StatusWaiting vs StatusParked：通过 config.park 控制（design/agent-process-model.md § Process State）
			targetStatus := statusWaiting
			for _, n := range taskGraph.Nodes {
				if n.ID == step.NodeID && n.Config != nil {
					if park, ok := n.Config["park"].(bool); ok && park {
						targetStatus = 6 // StatusParked
						break
					}
				}
			}
			_ = r.jobStore.UpdateStatus(ctx, j.ID, targetStatus)
			return ErrJobWaiting
		}
		stepStart := time.Now()
		ctx = WithAgent(ctx, agent)
		if replayCtx != nil && len(replayCtx.CompletedToolInvocations) > 0 {
			ctx = WithCompletedToolInvocations(ctx, replayCtx.CompletedToolInvocations)
		}
		if replayCtx != nil && len(replayCtx.PendingToolInvocations) > 0 {
			ctx = WithPendingToolInvocations(ctx, replayCtx.PendingToolInvocations)
		}
		if replayCtx != nil && len(replayCtx.StateChangesByStep) > 0 {
			m := make(map[string][]StateChangeForVerify)
			for nodeID, recs := range replayCtx.StateChangesByStep {
				for _, r := range recs {
					m[nodeID] = append(m[nodeID], StateChangeForVerify{ResourceType: r.ResourceType, ResourceID: r.ResourceID, Operation: r.Operation, ExternalRef: r.ExternalRef})
				}
			}
			ctx = WithStateChangesByStep(ctx, m)
		}
		if replayCtx != nil && len(replayCtx.ApprovedCorrelationKeys) > 0 {
			ctx = WithApprovedCorrelationKeys(ctx, replayCtx.ApprovedCorrelationKeys)
		}
		runCtx := ctx
		// Inject Step Contract helpers so steps stay deterministic on replay (design/step-contract.md).
		if replayCtx != nil {
			runCtx = runtime.WithClock(runCtx, runtime.ReplayClock(j.ID, effectiveStepID))
			runCtx = runtime.WithRNG(runCtx, runtime.ReplayRNG(j.ID, effectiveStepID))
		} else {
			runCtx = runtime.WithClock(runCtx, func() time.Time { return time.Now() })
		}
		if r.stepTimeout > 0 {
			var cancel context.CancelFunc
			runCtx, cancel = context.WithTimeout(runCtx, r.stepTimeout)
			defer cancel()
		}
		// 2.0 Deterministic Replay：标记 Replay 模式
		runCtx = determinism.WithReplay(runCtx, replayCtx != nil)
		// 2.0 Step Contract：注入 RecordedEffects 与 sdk.RuntimeContext
		if r.recordedEffectsRecorder != nil || replayCtx != nil {
			runCtx = agenteffects.WithRecordedEffects(runCtx, j.ID, effectiveStepID, replayCtx, r.recordedEffectsRecorder)
		}
		runCtx = sdk.WithRuntimeContext(runCtx, newRuntimeContextAdapter(j.ID, effectiveStepID))
		var runErr error
		if len(r.stepValidators) > 0 {
			if err := r.runStepValidators(runCtx, j.ID, effectiveStepID, step.NodeID, step.NodeType, nil); err != nil {
				runErr = err
			}
		}
		if runErr == nil {
			payload, runErr = step.Run(runCtx, payload)
		}
		durationMs := time.Since(stepStart).Milliseconds()
		var capReq *CapabilityRequiresApproval
		if errors.As(runErr, &capReq) && capReq != nil {
			if r.nodeEventSink != nil {
				// Capability approval wait: also save resumption context
				resumptionCtx := map[string]interface{}{
					"payload_results":  payload.Results,
					"plan_decision_id": PlanDecisionID(graphBytes),
					"cursor_node":      step.NodeID,
				}
				resumptionBytes, _ := json.Marshal(resumptionCtx)
				_ = r.nodeEventSink.AppendJobWaiting(ctx, j.ID, step.NodeID, "signal", "capability_approval", time.Now().Add(24*time.Hour), capReq.CorrelationKey, resumptionBytes)
			}
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusWaiting)
			return ErrJobWaiting
		}
		resultType, reason := ClassifyError(runErr)
		if runErr != nil && errors.Is(runErr, context.DeadlineExceeded) {
			resultType = StepResultRetryableFailure
			reason = "step timeout"
		}
		// 世界语义：tool 成功 = 已提交副作用；非 tool 成功 = 纯计算（Pure），replay 可重放
		if resultType == StepResultSuccess {
			if step.NodeType == "tool" {
				resultType = StepResultSideEffectCommitted
			} else {
				resultType = StepResultPure
			}
		}
		payloadResults, _ := json.Marshal(payload.Results)
		if runErr != nil && len(payloadResults) == 0 {
			payloadResults = []byte("{}")
		}
		if r.nodeEventSink != nil {
			stateStr := "ok"
			if resultType != StepResultSuccess && resultType != StepResultPure && resultType != StepResultSideEffectCommitted && resultType != StepResultCompensated {
				stateStr = string(resultType)
			}
			_ = r.nodeEventSink.AppendNodeFinished(ctx, j.ID, step.NodeID, payloadResults, durationMs, stateStr, 1, resultType, reason, effectiveStepID, "")
			_ = r.nodeEventSink.AppendStepCommitted(ctx, j.ID, step.NodeID, effectiveStepID, effectiveStepID, "")
		}
		if isStepFailure(resultType) {
			if resultType == StepResultCompensatableFailure && r.compensationRegistry != nil {
				if fn := r.compensationRegistry.GetCompensation(step.NodeID); fn != nil {
					_ = fn(ctx, j.ID, step.NodeID, effectiveStepID, effectiveStepID)
					if r.nodeEventSink != nil {
						_ = r.nodeEventSink.AppendStepCompensated(ctx, j.ID, step.NodeID, effectiveStepID, effectiveStepID, reason)
					}
				}
			}
			_ = r.jobStore.UpdateStatus(ctx, j.ID, statusFailed)
			sf := &StepFailure{Type: resultType, Inner: runErr, NodeID: step.NodeID}
			return fmt.Errorf("executor: 节点 %s 执行失败 (%s): %w", step.NodeID, resultType, sf)
		}
		if r.nodeEventSink != nil {
			opts := &StateCheckpointOpts{ChangedKeys: ChangedKeysFromState(stateBefore, payloadResults)}
			_ = r.nodeEventSink.AppendStateCheckpointed(ctx, j.ID, step.NodeID, stateBefore, payloadResults, opts)
			// 推理快照：供因果调试（该步的决策上下文）
			snapshot := map[string]interface{}{
				"node_id":     step.NodeID,
				"step_id":     effectiveStepID,
				"node_type":   step.NodeType,
				"goal":        j.Goal,
				"duration_ms": durationMs,
				"timestamp":   time.Now().Format(time.RFC3339),
			}
			if len(stateBefore) > 0 {
				snapshot["state_before"] = json.RawMessage(stateBefore)
			}
			if len(payloadResults) > 0 {
				snapshot["state_after"] = json.RawMessage(payloadResults)
			}
			if step.NodeType == "tool" {
				snapshot["tool_name"] = step.NodeID // 或从 step 取 tool name 若有
			}
			if step.NodeType == planner.NodeLLM {
				snapshot["llm_request"] = j.Goal
				if payload.Results != nil {
					if resp, ok := payload.Results[step.NodeID]; ok && resp != nil {
						snapshot["llm_response"] = resp
					}
				}
			}
			// Evidence Graph：从工具步结果中取出 _evidence 写入 reasoning_snapshot（design/execution-forensics.md）
			if payload.Results != nil {
				if m, ok := payload.Results[step.NodeID].(map[string]interface{}); ok && m["_evidence"] != nil {
					snapshot["evidence"] = m["_evidence"]
				}
			}
			// Causal Chain Phase 1：基于 state keys 推导因果依赖（design/execution-forensics.md § Causal Dependency）
			inputKeys, outputKeys := extractStateKeys(stateBefore, payloadResults)
			if len(inputKeys) > 0 {
				snapshot["input_keys"] = inputKeys
			}
			if len(outputKeys) > 0 {
				snapshot["output_keys"] = outputKeys
			}
			if snapshotBytes, err := json.Marshal(snapshot); err == nil {
				_ = r.nodeEventSink.AppendReasoningSnapshot(ctx, j.ID, snapshotBytes)
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
