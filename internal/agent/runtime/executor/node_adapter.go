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
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/google/uuid"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
)

// NodeRunner 单节点执行函数（用于 Steppable 执行与 node-level checkpoint）
type NodeRunner func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error)

// NodeAdapter 将 TaskNode 转为 eino DAG 节点（闭包持有 task、agent 及依赖）
type NodeAdapter interface {
	ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error)
	// ToNodeRunner 返回同签名的可执行函数，供 Steppable 逐节点执行与 checkpoint
	ToNodeRunner(task *planner.TaskNode, agent *runtime.Agent) (NodeRunner, error)
}

// LLMGen 执行 LLM 生成（由应用层注入）
type LLMGen interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// ToolExec 执行单工具调用（由应用层注入）；state 为再入时传入的上次状态，返回 ToolResult 支持 Done/State/Output
type ToolExec interface {
	Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (ToolResult, error)
}

// ToolResult 工具结果：未完成可挂起并携带 State 供再入
type ToolResult struct {
	Done   bool
	State  interface{}
	Output string
	Err    string
}

// WorkflowExec 执行工作流（由应用层注入，如 Engine.ExecuteWorkflow）
type WorkflowExec interface {
	ExecuteWorkflow(ctx context.Context, name string, params map[string]any) (interface{}, error)
}

// LLMNodeAdapter 将 llm 型 TaskNode 转为 DAG 节点
type LLMNodeAdapter struct {
	LLM              LLMGen
	CommandEventSink CommandEventSink // 可选；执行成功后立即写 command_committed，保证副作用安全
}

func (a *LLMNodeAdapter) runNode(ctx context.Context, taskID string, cfg map[string]any, agent *runtime.Agent, p *AgentDAGPayload) (*AgentDAGPayload, error) {
	prompt := p.Goal
	if cfg != nil {
		if g, ok := cfg["goal"].(string); ok && g != "" {
			prompt = g
		}
	}
	if a.CommandEventSink != nil {
		if jobID := JobIDFromContext(ctx); jobID != "" {
			inputBytes, _ := json.Marshal(map[string]any{"prompt": prompt})
			_ = a.CommandEventSink.AppendCommandEmitted(ctx, jobID, taskID, taskID, "llm", inputBytes)
		}
	}
	resp, err := a.LLM.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}
	if a.CommandEventSink != nil {
		if jobID := JobIDFromContext(ctx); jobID != "" {
			resultBytes, _ := json.Marshal(resp)
			_ = a.CommandEventSink.AppendCommandCommitted(ctx, jobID, taskID, taskID, resultBytes, "")
		}
	}
	if p.Results == nil {
		p.Results = make(map[string]any)
	}
	p.Results[taskID] = resp
	if agent != nil && agent.Session != nil {
		agent.Session.AddMessage("assistant", resp)
	}
	return p, nil
}

// ToDAGNode 实现 NodeAdapter
func (a *LLMNodeAdapter) ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error) {
	if a.LLM == nil {
		return nil, fmt.Errorf("LLMNodeAdapter: LLM 未配置")
	}
	taskID, cfg := task.ID, task.Config
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, cfg, agent, p)
	}), nil
}

// ToNodeRunner 实现 NodeAdapter
func (a *LLMNodeAdapter) ToNodeRunner(task *planner.TaskNode, agent *runtime.Agent) (NodeRunner, error) {
	if a.LLM == nil {
		return nil, fmt.Errorf("LLMNodeAdapter: LLM 未配置")
	}
	taskID, cfg := task.ID, task.Config
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, cfg, agent, p)
	}, nil
}

// ToolNodeAdapter 将 tool 型 TaskNode 转为 DAG 节点
type ToolNodeAdapter struct {
	Tools            ToolExec
	ToolEventSink    ToolEventSink    // 可选；Tool 执行前后写 ToolCalled/ToolReturned
	CommandEventSink CommandEventSink // 可选；执行成功后立即写 command_committed，保证副作用安全
	// InvocationLedger 执行许可账本；非 nil 时所有 tool 调用经 Acquire/Commit，禁止直接拥有执行权
	InvocationLedger InvocationLedger
	// InvocationStore 可选；当 InvocationLedger 为 nil 时用于兼容旧逻辑（先查再执行）
	InvocationStore ToolInvocationStore
	// EffectStore 副作用存储；非 nil 时两步提交：Execute 后先 PutEffect，再 Append；Replay/catch-up 时先查此处（强 Replay）
	EffectStore EffectStore
	// CapabilityPolicyChecker 执行前校验；非 nil 时在 Tools.Execute 前按能力/策略 Check，deny 则失败该步，require_approval 则返回 CapabilityRequiresApproval（design/capability-policy.md）
	CapabilityPolicyChecker CapabilityPolicyChecker
	ResourceVerifier        ResourceVerifier // 可选；Confirmation Replay 时校验外部资源仍存在，不通过则失败 job
}

// runConfirmation 在注入前校验本步的 StateChanged；若 verifier 存在且有待校验项且任一项失败则返回永久失败错误
func (a *ToolNodeAdapter) runConfirmation(ctx context.Context, jobID, taskID string, stepChanges []StateChangeForVerify) error {
	if a.ResourceVerifier == nil || len(stepChanges) == 0 {
		return nil
	}
	for _, c := range stepChanges {
		ok, err := a.ResourceVerifier.Verify(ctx, jobID, taskID, c.ResourceType, c.ResourceID, c.Operation, c.ExternalRef)
		if err != nil {
			return &StepFailure{Type: StepResultPermanentFailure, Inner: err, NodeID: taskID}
		}
		if !ok {
			return &StepFailure{Type: StepResultPermanentFailure, Inner: fmt.Errorf("confirmation failed: resource %s %s %s", c.ResourceType, c.ResourceID, c.Operation), NodeID: taskID}
		}
	}
	return nil
}

// writeCatchUpFinished 恢复路径：向事件流追加 tool_invocation_finished 与 command_committed（catch-up），使事件流与已提交结果一致（Activity Log Barrier）
func (a *ToolNodeAdapter) writeCatchUpFinished(ctx context.Context, jobID, taskID, nodeIDForEvent, idempotencyKey, argsHash string, resultBytes []byte) error {
	if nodeIDForEvent == "" {
		nodeIDForEvent = taskID
	}
	invocationID := "catchup-" + idempotencyKey
	finishedAt := time.Now().UTC()
	if a.ToolEventSink != nil {
		if err := a.ToolEventSink.AppendToolInvocationFinished(ctx, jobID, nodeIDForEvent, &ToolInvocationFinishedPayload{
			InvocationID:   invocationID,
			IdempotencyKey: idempotencyKey,
			Outcome:        ToolInvocationOutcomeSuccess,
			Result:         resultBytes,
			FinishedAt:     FormatStartedAt(finishedAt),
		}); err != nil {
			return err
		}
	}
	if a.CommandEventSink != nil {
		if err := a.CommandEventSink.AppendCommandCommitted(ctx, jobID, nodeIDForEvent, taskID, resultBytes, argsHash); err != nil {
			return err
		}
	}
	return nil
}

func (a *ToolNodeAdapter) runNode(ctx context.Context, taskID, toolName string, cfg map[string]any, agent *runtime.Agent, p *AgentDAGPayload) (*AgentDAGPayload, error) {
	jobID := JobIDFromContext(ctx)
	stepIDForLedger := ExecutionStepIDFromContext(ctx)
	if stepIDForLedger == "" {
		stepIDForLedger = taskID
	}
	idempotencyKey := IdempotencyKey(jobID, stepIDForLedger, toolName, cfg)
	argsHash := ArgumentsHash(cfg)
	stepChanges := StateChangesByStepFromContext(ctx)[stepIDForLedger]
	if stepChanges == nil {
		stepChanges = StateChangesByStepFromContext(ctx)[taskID]
	}

	// Activity Log Barrier：事件流中已 started 无 finished 时禁止再次执行，仅恢复或失败（design/effect-system.md）
	if pending := PendingToolInvocationsFromContext(ctx); pending != nil {
		if _, isPending := pending[idempotencyKey]; isPending {
			var resultBytes []byte
			if a.InvocationLedger != nil && jobID != "" {
				var exists bool
				resultBytes, exists = a.InvocationLedger.Recover(ctx, jobID, idempotencyKey)
				if !exists {
					resultBytes = nil
				}
			}
			if len(resultBytes) == 0 && a.InvocationStore != nil && jobID != "" {
				rec, _ := a.InvocationStore.GetByJobAndIdempotencyKey(ctx, jobID, idempotencyKey)
				if rec != nil && rec.Committed && len(rec.Result) > 0 {
					resultBytes = rec.Result
				}
			}
			if len(resultBytes) > 0 {
				if err := a.runConfirmation(ctx, jobID, taskID, stepChanges); err != nil {
					return nil, err
				}
				if err := a.writeCatchUpFinished(ctx, jobID, taskID, stepIDForLedger, idempotencyKey, argsHash, resultBytes); err != nil {
					return nil, err
				}
				var nodeResult map[string]any
				_ = json.Unmarshal(resultBytes, &nodeResult)
				if nodeResult == nil {
					nodeResult = make(map[string]any)
				}
				if p.Results == nil {
					p.Results = make(map[string]any)
				}
				p.Results[taskID] = nodeResult
				return p, nil
			}
			return nil, &StepFailure{
				Type:   StepResultPermanentFailure,
				Inner:  fmt.Errorf("invocation in flight or lost, idempotency_key=%s", idempotencyKey),
				NodeID: taskID,
			}
		}
	}

	// Ledger 为仲裁：先申请执行许可，禁止在 ReturnRecordedResult 时调用 tool
	if a.InvocationLedger != nil && jobID != "" {
		var replayResult []byte
		if completed := CompletedToolInvocationsFromContext(ctx); completed != nil {
			replayResult = completed[idempotencyKey]
		}
		decision, rec, err := a.InvocationLedger.Acquire(ctx, jobID, taskID, toolName, argsHash, idempotencyKey, replayResult)
		if err != nil {
			return nil, err
		}
		switch decision {
		case InvocationDecisionReturnRecordedResult:
			if err := a.runConfirmation(ctx, jobID, taskID, stepChanges); err != nil {
				return nil, err
			}
			var nodeResult map[string]any
			if rec != nil && len(rec.Result) > 0 {
				_ = json.Unmarshal(rec.Result, &nodeResult)
			}
			if nodeResult == nil {
				nodeResult = make(map[string]any)
			}
			if p.Results == nil {
				p.Results = make(map[string]any)
			}
			p.Results[taskID] = nodeResult
			return p, nil
		case InvocationDecisionWaitOtherWorker:
			return nil, &StepFailure{Type: StepResultRetryableFailure, Inner: fmt.Errorf("invocation in progress for %s", idempotencyKey), NodeID: taskID}
		case InvocationDecisionRejected:
			return nil, &StepFailure{Type: StepResultPermanentFailure, Inner: fmt.Errorf("ledger rejected invocation %s", idempotencyKey), NodeID: taskID}
		case InvocationDecisionAllowExecute:
			// 仅在此分支执行 tool，然后 Commit
			return a.runNodeExecute(ctx, jobID, taskID, toolName, cfg, idempotencyKey, argsHash, rec, stepChanges, agent, p)
		default:
			return nil, &StepFailure{Type: StepResultPermanentFailure, Inner: fmt.Errorf("unknown ledger decision"), NodeID: taskID}
		}
	}

	// 无 Ledger：仅兼容单进程/旧逻辑；1.0 持久化 job 必须配置 InvocationLedger，否则不保证 at-most-once
	if a.InvocationStore != nil && jobID != "" {
		rec, _ := a.InvocationStore.GetByJobAndIdempotencyKey(ctx, jobID, idempotencyKey)
		if rec != nil && rec.Committed && (rec.Status == ToolInvocationStatusSuccess || rec.Status == ToolInvocationStatusConfirmed) && len(rec.Result) > 0 {
			if err := a.runConfirmation(ctx, jobID, taskID, stepChanges); err != nil {
				return nil, err
			}
			_ = a.InvocationStore.SetFinished(ctx, idempotencyKey, ToolInvocationStatusConfirmed, rec.Result, true)
			var nodeResult map[string]any
			_ = json.Unmarshal(rec.Result, &nodeResult)
			if nodeResult == nil {
				nodeResult = make(map[string]any)
			}
			if p.Results == nil {
				p.Results = make(map[string]any)
			}
			p.Results[taskID] = nodeResult
			return p, nil
		}
	}
	if completed := CompletedToolInvocationsFromContext(ctx); completed != nil {
		if resultJSON, ok := completed[idempotencyKey]; ok {
			if err := a.runConfirmation(ctx, jobID, taskID, stepChanges); err != nil {
				return nil, err
			}
			var nodeResult map[string]any
			if len(resultJSON) > 0 {
				_ = json.Unmarshal(resultJSON, &nodeResult)
			}
			if nodeResult == nil {
				nodeResult = make(map[string]any)
			}
			if p.Results == nil {
				p.Results = make(map[string]any)
			}
			p.Results[taskID] = nodeResult
			return p, nil
		}
	}
	// Effect Store catch-up：事件流无 command_committed 但 EffectStore 已有 effect（上一 Worker 执行后崩溃），则只写回事件不重执行（强 Replay）
	if a.EffectStore != nil && jobID != "" {
		eff, err := a.EffectStore.GetEffectByJobAndIdempotencyKey(ctx, jobID, idempotencyKey)
		if err == nil && eff != nil && len(eff.Output) > 0 {
			if err := a.runConfirmation(ctx, jobID, taskID, stepChanges); err != nil {
				return nil, err
			}
			if err := a.writeCatchUpFinished(ctx, jobID, taskID, stepIDForLedger, idempotencyKey, argsHash, eff.Output); err != nil {
				return nil, err
			}
			if a.InvocationLedger != nil {
				_ = a.InvocationLedger.Commit(ctx, "catchup-"+idempotencyKey, idempotencyKey, eff.Output)
			}
			if a.InvocationStore != nil {
				_ = a.InvocationStore.SetFinished(ctx, idempotencyKey, ToolInvocationStatusSuccess, eff.Output, true)
			}
			var nodeResult map[string]any
			_ = json.Unmarshal(eff.Output, &nodeResult)
			if nodeResult == nil {
				nodeResult = make(map[string]any)
			}
			if p.Results == nil {
				p.Results = make(map[string]any)
			}
			p.Results[taskID] = nodeResult
			return p, nil
		}
	}
	// 1.0 短路：Replay 已注入的节点结果不再执行
	if prev, ok := p.Results[taskID]; ok {
		if m, ok := prev.(map[string]any); ok {
			if _, hasDone := m["done"]; hasDone {
				return p, nil
			}
			if _, hasOut := m["output"]; hasOut {
				return p, nil
			}
		}
	}
	return a.runNodeExecute(ctx, jobID, taskID, toolName, cfg, idempotencyKey, argsHash, nil, stepChanges, agent, p)
}

// runNodeExecute 执行 tool 并写事件；当 Ledger 存在时 rec 非 nil 且已 SetStarted，成功后需 Commit
func (a *ToolNodeAdapter) runNodeExecute(ctx context.Context, jobID, taskID, toolName string, cfg map[string]any, idempotencyKey, argsHash string, ledgerRec *ToolInvocationRecord, stepChanges []StateChangeForVerify, agent *runtime.Agent, p *AgentDAGPayload) (*AgentDAGPayload, error) {
	nodeIDForEvent := ExecutionStepIDFromContext(ctx)
	if nodeIDForEvent == "" {
		nodeIDForEvent = taskID
	}
	var state interface{}
	if prev, ok := p.Results[taskID]; ok {
		if m, ok := prev.(map[string]any); ok {
			state = m["state"]
		}
	}
	invocationID := uuid.New().String()
	if ledgerRec != nil {
		invocationID = ledgerRec.InvocationID
	}
	startedAt := time.Now().UTC()
	if a.InvocationLedger == nil && a.InvocationStore != nil && jobID != "" {
		_ = a.InvocationStore.SetStarted(ctx, &ToolInvocationRecord{
			InvocationID:   invocationID,
			JobID:          jobID,
			StepID:         taskID,
			ToolName:       toolName,
			ArgsHash:       argsHash,
			IdempotencyKey: idempotencyKey,
			Status:         ToolInvocationStatusStarted,
		})
	}
	if a.ToolEventSink != nil && jobID != "" {
		_ = a.ToolEventSink.AppendToolInvocationStarted(ctx, jobID, nodeIDForEvent, &ToolInvocationStartedPayload{
			InvocationID:   invocationID,
			ToolName:       toolName,
			ArgumentsHash:  argsHash,
			IdempotencyKey: idempotencyKey,
			StartedAt:      FormatStartedAt(startedAt),
		})
	}
	if a.CommandEventSink != nil && jobID != "" {
		inputBytes, _ := json.Marshal(cfg)
		_ = a.CommandEventSink.AppendCommandEmitted(ctx, jobID, nodeIDForEvent, taskID, "tool", inputBytes)
	}
	if a.ToolEventSink != nil && jobID != "" {
		inputBytes, _ := json.Marshal(cfg)
		_ = a.ToolEventSink.AppendToolCalled(ctx, jobID, nodeIDForEvent, toolName, inputBytes)
	}
	ctx = WithToolExecutionKey(ctx, idempotencyKey)
	// Capability 执行前校验（design/capability-policy.md）
	if a.CapabilityPolicyChecker != nil && jobID != "" {
		approvedKeys := ApprovedCorrelationKeysFromContext(ctx)
		capability := toolName
		allowed, requiredApproval, checkErr := a.CapabilityPolicyChecker.Check(ctx, jobID, toolName, capability, idempotencyKey, approvedKeys)
		if checkErr != nil {
			return nil, &StepFailure{Type: StepResultPermanentFailure, Inner: checkErr, NodeID: taskID}
		}
		if !allowed && requiredApproval {
			return nil, &CapabilityRequiresApproval{CorrelationKey: "cap-approval-" + idempotencyKey}
		}
		if !allowed {
			return nil, &StepFailure{Type: StepResultPermanentFailure, Inner: fmt.Errorf("capability denied: %s", capability), NodeID: taskID}
		}
	}
	result, err := a.Tools.Execute(ctx, toolName, cfg, state)
	finishedAt := time.Now().UTC()
	if err != nil {
		if a.InvocationLedger == nil && a.InvocationStore != nil && jobID != "" {
			errResult, _ := json.Marshal(map[string]any{"error": err.Error()})
			_ = a.InvocationStore.SetFinished(ctx, idempotencyKey, ToolInvocationStatusFailure, errResult, false)
		}
		if p.Results == nil {
			p.Results = make(map[string]any)
		}
		p.Results[taskID] = map[string]any{"error": err.Error(), "at": time.Now()}
		if a.ToolEventSink != nil && jobID != "" {
			_ = a.ToolEventSink.AppendToolInvocationFinished(ctx, jobID, nodeIDForEvent, &ToolInvocationFinishedPayload{
				InvocationID:   invocationID,
				IdempotencyKey: idempotencyKey,
				Outcome:        ToolInvocationOutcomeFailure,
				Error:          err.Error(),
				FinishedAt:     FormatStartedAt(finishedAt),
			})
			outBytes, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
			_ = a.ToolEventSink.AppendToolReturned(ctx, jobID, nodeIDForEvent, outBytes)
			_ = a.ToolEventSink.AppendToolResultSummarized(ctx, jobID, nodeIDForEvent, toolName, truncateStr(err.Error(), 200), err.Error(), false)
		}
		if agent != nil && agent.Session != nil {
			inputStr := ""
			if len(cfg) > 0 {
				b, _ := json.Marshal(cfg)
				inputStr = string(b)
			}
			agent.Session.AddToolCall(toolName, inputStr, "error: "+err.Error())
		}
		return nil, err
	}
	nodeResult := map[string]any{
		"done": result.Done, "state": result.State, "output": result.Output, "error": result.Err,
	}
	resultBytes, _ := json.Marshal(nodeResult)
	// 两步提交 Phase 1：先写 Effect Store，崩溃恢复时 Replay 可从 Effect Store catch-up，不重执行（强 Replay）
	if a.EffectStore != nil && jobID != "" {
		inputBytes, _ := json.Marshal(cfg)
		_ = a.EffectStore.PutEffect(ctx, &EffectRecord{
			JobID:          jobID,
			CommandID:      nodeIDForEvent,
			IdempotencyKey: idempotencyKey,
			Kind:           EffectKindTool,
			Input:          inputBytes,
			Output:         resultBytes,
			Error:          result.Err,
		})
	}
	// Phase 2：写事件流与 Ledger/Store
	if a.ToolEventSink != nil && jobID != "" {
		_ = a.ToolEventSink.AppendToolInvocationFinished(ctx, jobID, nodeIDForEvent, &ToolInvocationFinishedPayload{
			InvocationID:   invocationID,
			IdempotencyKey: idempotencyKey,
			Outcome:        ToolInvocationOutcomeSuccess,
			Result:         resultBytes,
			FinishedAt:     FormatStartedAt(finishedAt),
		})
	}
	if a.CommandEventSink != nil && jobID != "" {
		_ = a.CommandEventSink.AppendCommandCommitted(ctx, jobID, nodeIDForEvent, taskID, resultBytes, argsHash)
	}
	if a.InvocationLedger != nil && jobID != "" {
		_ = a.InvocationLedger.Commit(ctx, invocationID, idempotencyKey, resultBytes)
	} else if a.InvocationStore != nil && jobID != "" {
		_ = a.InvocationStore.SetFinished(ctx, idempotencyKey, ToolInvocationStatusSuccess, resultBytes, true)
	}
	if p.Results == nil {
		p.Results = make(map[string]any)
	}
	p.Results[taskID] = nodeResult
	if a.ToolEventSink != nil && jobID != "" {
		outBytes, _ := json.Marshal(map[string]interface{}{"output": result.Output, "error": result.Err, "done": result.Done})
		_ = a.ToolEventSink.AppendToolReturned(ctx, jobID, nodeIDForEvent, outBytes)
		summary := truncateStr(result.Output, 200)
		if result.Err != "" {
			summary = truncateStr("error: "+result.Err, 200)
		}
		_ = a.ToolEventSink.AppendToolResultSummarized(ctx, jobID, nodeIDForEvent, toolName, summary, result.Err, false)
	}
	msg := result.Output
	if result.Err != "" {
		msg = "error: " + result.Err
	}
	if agent != nil && agent.Session != nil {
		if msg != "" {
			agent.Session.AddMessage("tool", msg)
		}
		inputStr := ""
		if len(cfg) > 0 {
			b, _ := json.Marshal(cfg)
			inputStr = string(b)
		}
		outStr := result.Output
		if result.Err != "" {
			outStr = "error: " + result.Err
		}
		agent.Session.AddToolCall(toolName, inputStr, outStr)
	}
	return p, nil
}

// ToDAGNode 实现 NodeAdapter
func (a *ToolNodeAdapter) ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error) {
	if a.Tools == nil {
		return nil, fmt.Errorf("ToolNodeAdapter: Tools 未配置")
	}
	if task.ToolName == "" {
		return nil, fmt.Errorf("ToolNodeAdapter: 节点 %s 缺少 tool_name", task.ID)
	}
	taskID, toolName, cfg := task.ID, task.ToolName, task.Config
	if cfg == nil {
		cfg = make(map[string]any)
	}
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, toolName, cfg, agent, p)
	}), nil
}

// ToNodeRunner 实现 NodeAdapter
func (a *ToolNodeAdapter) ToNodeRunner(task *planner.TaskNode, agent *runtime.Agent) (NodeRunner, error) {
	if a.Tools == nil {
		return nil, fmt.Errorf("ToolNodeAdapter: Tools 未配置")
	}
	if task.ToolName == "" {
		return nil, fmt.Errorf("ToolNodeAdapter: 节点 %s 缺少 tool_name", task.ID)
	}
	taskID, toolName, cfg := task.ID, task.ToolName, task.Config
	if cfg == nil {
		cfg = make(map[string]any)
	}
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, toolName, cfg, agent, p)
	}, nil
}

// WorkflowNodeAdapter 将 workflow 型 TaskNode 转为 DAG 节点
type WorkflowNodeAdapter struct {
	Workflow         WorkflowExec
	CommandEventSink CommandEventSink // 可选；执行成功后立即写 command_committed，保证副作用安全
}

func (a *WorkflowNodeAdapter) runNode(ctx context.Context, taskID, name string, params map[string]any, p *AgentDAGPayload) (*AgentDAGPayload, error) {
	if a.CommandEventSink != nil {
		if jobID := JobIDFromContext(ctx); jobID != "" {
			inputBytes, _ := json.Marshal(map[string]any{"workflow": name, "params": params})
			_ = a.CommandEventSink.AppendCommandEmitted(ctx, jobID, taskID, taskID, "workflow", inputBytes)
		}
	}
	result, err := a.Workflow.ExecuteWorkflow(ctx, name, params)
	if err != nil {
		if p.Results == nil {
			p.Results = make(map[string]any)
		}
		p.Results[taskID] = map[string]any{"error": err.Error(), "at": time.Now()}
		return nil, err
	}
	if a.CommandEventSink != nil {
		if jobID := JobIDFromContext(ctx); jobID != "" {
			resultBytes, _ := json.Marshal(result)
			_ = a.CommandEventSink.AppendCommandCommitted(ctx, jobID, taskID, taskID, resultBytes, "")
		}
	}
	if p.Results == nil {
		p.Results = make(map[string]any)
	}
	p.Results[taskID] = result
	return p, nil
}

// ToDAGNode 实现 NodeAdapter
func (a *WorkflowNodeAdapter) ToDAGNode(task *planner.TaskNode, agent *runtime.Agent) (*compose.Lambda, error) {
	if a.Workflow == nil {
		return nil, fmt.Errorf("WorkflowNodeAdapter: Workflow 未配置")
	}
	if task.Workflow == "" {
		return nil, fmt.Errorf("WorkflowNodeAdapter: 节点 %s 缺少 workflow", task.ID)
	}
	taskID, name, params := task.ID, task.Workflow, task.Config
	if params == nil {
		params = make(map[string]any)
	}
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, name, params, p)
	}), nil
}

// ToNodeRunner 实现 NodeAdapter
func (a *WorkflowNodeAdapter) ToNodeRunner(task *planner.TaskNode, agent *runtime.Agent) (NodeRunner, error) {
	if a.Workflow == nil {
		return nil, fmt.Errorf("WorkflowNodeAdapter: Workflow 未配置")
	}
	if task.Workflow == "" {
		return nil, fmt.Errorf("WorkflowNodeAdapter: 节点 %s 缺少 workflow", task.ID)
	}
	taskID, name, params := task.ID, task.Workflow, task.Config
	if params == nil {
		params = make(map[string]any)
	}
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, name, params, p)
	}, nil
}

func truncateStr(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}
