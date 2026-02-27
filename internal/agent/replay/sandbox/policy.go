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

// Package sandbox 定义 Replay 时的可执行边界：哪些操作在 replay 时允许重算（Deterministic）、
// 哪些forbidden执行仅从 event 注入（SideEffect/External），用于 execution reconstruction 而非 debug playback。
package sandbox

import (
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/replay"
)

// OperationKind 表示 Replay 时该操作的允许行为
type OperationKind string

const (
	// Deterministic replay 时允许重新执行（纯计算、无副作用）
	Deterministic OperationKind = "deterministic"
	// SideEffect 有副作用，replay 时forbidden执行，仅从 event 注入结果
	SideEffect OperationKind = "side_effect"
	// External 依赖外部世界，replay 时forbidden执行，仅从 event 恢复
	External OperationKind = "external"
)

// ReplayDecision 策略对单步的决策结果
type ReplayDecision struct {
	Kind   OperationKind // 操作类型
	Inject bool          // true 表示应从 ReplayContext 注入结果并跳过执行
	Result []byte        // 注入时使用的 result（来自 CommandResults 或 CompletedToolInvocations）
}

// ReplayPolicy 给定节点信息与 ReplayContext，返回 Replay 时应执行还是注入
type ReplayPolicy interface {
	// Decide 返回该步在 Replay 时的决策；replayCtx 为 nil 表示非 Replay 路径，应执行
	Decide(nodeID, commandID, nodeType string, replayCtx *replay.ReplayContext) ReplayDecision
}

// DefaultPolicy 默认策略：llm/tool/workflow 均为 SideEffect（replay 时仅注入，不重执行）
type DefaultPolicy struct{}

// Decide 实现 ReplayPolicy
func (DefaultPolicy) Decide(nodeID, commandID, nodeType string, replayCtx *replay.ReplayContext) ReplayDecision {
	if replayCtx == nil {
		return ReplayDecision{Kind: kindForNodeType(nodeType), Inject: false, Result: nil}
	}
	// 有 ReplayContext 时，若已提交则注入
	switch nodeType {
	case planner.NodeTool:
		// Tool 结果可能从 CommandResults（command_id）或 CompletedToolInvocations 取，Runner 侧已处理 CompletedToolInvocations
		if result, ok := replayCtx.CommandResults[commandID]; ok && len(result) > 0 {
			return ReplayDecision{Kind: SideEffect, Inject: true, Result: result}
		}
		return ReplayDecision{Kind: SideEffect, Inject: false, Result: nil}
	case planner.NodeLLM, planner.NodeWorkflow:
		if result, ok := replayCtx.CommandResults[commandID]; ok && len(result) > 0 {
			return ReplayDecision{Kind: SideEffect, Inject: true, Result: result}
		}
		return ReplayDecision{Kind: SideEffect, Inject: false, Result: nil}
	default:
		// 未知类型视为 SideEffect，有则注入
		if result, ok := replayCtx.CommandResults[commandID]; ok && len(result) > 0 {
			return ReplayDecision{Kind: SideEffect, Inject: true, Result: result}
		}
		return ReplayDecision{Kind: SideEffect, Inject: false, Result: nil}
	}
}

func kindForNodeType(nodeType string) OperationKind {
	switch nodeType {
	case planner.NodeTool, planner.NodeLLM, planner.NodeWorkflow:
		return SideEffect
	default:
		return SideEffect
	}
}
