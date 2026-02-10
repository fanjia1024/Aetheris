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

import "context"

// InvocationDecision 执行许可决策：Ledger 裁决是否允许调用 tool
type InvocationDecision int

const (
	// InvocationDecisionAllowExecute 允许执行；无已提交记录，可调用 tool 后 Commit
	InvocationDecisionAllowExecute InvocationDecision = iota
	// InvocationDecisionReturnRecordedResult 恢复已记录结果；已有 committed 成功记录，禁止再执行，直接注入结果
	InvocationDecisionReturnRecordedResult
	// InvocationDecisionWaitOtherWorker 记录存在但未提交，可能其他 worker 正在执行；可等待或重试
	InvocationDecisionWaitOtherWorker
	// InvocationDecisionRejected 拒绝（如参数无效等）
	InvocationDecisionRejected
)

// InvocationLedger 执行许可账本：Runner 只申请许可，Ledger 裁决；Replay 时查询历史而非执行代码
type InvocationLedger interface {
	// Acquire 请求执行许可；replayResult 为事件流重放得到的已完成结果（可选）。返回决策与可选记录
	Acquire(ctx context.Context, jobID, stepID, toolName, argsHash, idempotencyKey string, replayResult []byte) (InvocationDecision, *ToolInvocationRecord, error)
	// Commit 执行成功后提交结果，标记 committed
	Commit(ctx context.Context, invocationID, idempotencyKey string, result []byte) error
	// Recover 按 job+idempotencyKey 恢复已提交结果；用于 replay 路径统一入口
	Recover(ctx context.Context, jobID, idempotencyKey string) (result []byte, exists bool)
}
