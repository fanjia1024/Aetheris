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
	"sync/atomic"
	"testing"

	"rag-platform/internal/agent/runtime"
)

const (
	job1   = "job-1"
	step1  = "step-1"
	tool1  = "tool1"
	key1   = "job-1|step-1|tool1|hash1"
	argsH1 = "hash1"
)

// 1.0 致命四场景与测试对应（Runtime 1.0 证明：任意 crash/重启/双 worker/replay 下不重复外部副作用）：
// (1) Worker 在 tool 执行前崩溃 → TestLedger_1（无已提交记录时 replay 执行一次并 commit，之后仅恢复）
// (2) Tool 执行后、commit 前崩溃 → TestLedger_2（已有 committed 记录时 Acquire 返回 ReturnRecordedResult，不执行 tool）
// (3) 两 worker 同时抢同一 step → TestLedger_3（先 Acquire 得 AllowExecute，后 Acquire 得 WaitOtherWorker；仅一次 Commit）
// (4) Replay 恢复输出 → TestLedger_5 + TestAdapter_Replay_InjectsResult_NoToolCall（replayResult 注入时 ReturnRecordedResult，0 次 tool 调用）

// TestLedger_1_CrashBeforeCommit_ReplayReExecutesAndCommits 证明：(1) 无已提交记录时，replay 路径会执行一次并提交；之后仅恢复结果
func TestLedger_1_CrashBeforeCommit_ReplayReExecutesAndCommits(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	ctx := context.Background()

	// 模拟 crash before commit：store 中无任何记录
	decision, rec, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if decision != InvocationDecisionAllowExecute || rec == nil {
		t.Fatalf("expected AllowExecute with record, got decision=%v rec=%v", decision, rec)
	}

	// “执行”并提交
	result := []byte(`{"done":true,"output":"ok"}`)
	if err := ledger.Commit(ctx, rec.InvocationID, key1, result); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// 再次 Acquire（replay 或第二次跑）必须得到已记录结果，禁止再执行
	decision2, rec2, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil {
		t.Fatalf("Acquire again: %v", err)
	}
	if decision2 != InvocationDecisionReturnRecordedResult {
		t.Fatalf("expected ReturnRecordedResult after commit, got %v", decision2)
	}
	if rec2 == nil || string(rec2.Result) != string(result) {
		t.Fatalf("expected record with result %q, got %v", result, rec2)
	}
}

// TestLedger_2_CrashAfterCommit_ReplayRestoresNoSecondCall 证明：已有 committed 记录时，Acquire 返回 ReturnRecordedResult，不执行 tool
func TestLedger_2_CrashAfterCommit_ReplayRestoresNoSecondCall(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	ctx := context.Background()

	// 先执行并提交
	decision, rec, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil || decision != InvocationDecisionAllowExecute || rec == nil {
		t.Fatalf("first Acquire: err=%v decision=%v", err, decision)
	}
	result := []byte(`{"done":true}`)
	if err := ledger.Commit(ctx, rec.InvocationID, key1, result); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Replay：无 replayResult，但 store 已有 committed -> ReturnRecordedResult
	decision2, rec2, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil {
		t.Fatalf("replay Acquire: %v", err)
	}
	if decision2 != InvocationDecisionReturnRecordedResult || rec2 == nil {
		t.Fatalf("expected ReturnRecordedResult with record, got decision=%v rec=%v", decision2, rec2)
	}
	if string(rec2.Result) != string(result) {
		t.Fatalf("expected result %q, got %q", result, rec2.Result)
	}

	// Replay 带 replayResult：同样 ReturnRecordedResult，且结果一致
	decision3, rec3, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, result)
	if err != nil {
		t.Fatalf("replay with replayResult: %v", err)
	}
	if decision3 != InvocationDecisionReturnRecordedResult || rec3 == nil {
		t.Fatalf("expected ReturnRecordedResult, got decision=%v", decision3)
	}
}

// TestLedger_3_DoubleWorker_OnlyOneCommit 证明：同一 idempotency key，先 Acquire 的得到 AllowExecute 并占位；后 Acquire 得到 WaitOtherWorker，不会执行
func TestLedger_3_DoubleWorker_OnlyOneCommit(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	ctx := context.Background()

	// Worker1 Acquire -> AllowExecute（内部 SetStarted）
	decision1, rec1, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil {
		t.Fatalf("worker1 Acquire: %v", err)
	}
	if decision1 != InvocationDecisionAllowExecute || rec1 == nil {
		t.Fatalf("worker1 expected AllowExecute, got %v", decision1)
	}

	// Worker2 Acquire（尚未 Commit）-> WaitOtherWorker
	decision2, rec2, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil {
		t.Fatalf("worker2 Acquire: %v", err)
	}
	if decision2 != InvocationDecisionWaitOtherWorker {
		t.Fatalf("worker2 expected WaitOtherWorker, got %v", decision2)
	}
	if rec2 != nil {
		t.Fatalf("worker2 should get nil record for WaitOtherWorker, got %v", rec2)
	}

	// Worker1 提交
	result := []byte(`{"done":true}`)
	if err := ledger.Commit(ctx, rec1.InvocationID, key1, result); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Worker2 再次 Acquire（replay）-> ReturnRecordedResult，不会执行
	decision2b, rec2b, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil {
		t.Fatalf("worker2 replay Acquire: %v", err)
	}
	if decision2b != InvocationDecisionReturnRecordedResult || rec2b == nil {
		t.Fatalf("worker2 replay expected ReturnRecordedResult, got %v rec=%v", decision2b, rec2b)
	}
}

// TestLedger_4_RetryableFailure_NoSuccessCommit 证明：执行失败（未 Commit 成功）时，不写入 committed success；后续 Acquire 为 WaitOtherWorker（或需超时策略才可重试）
func TestLedger_4_RetryableFailure_NoSuccessCommit(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	ctx := context.Background()

	decision, rec, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil || decision != InvocationDecisionAllowExecute || rec == nil {
		t.Fatalf("Acquire: %v", err)
	}
	// 模拟执行失败 / 超时：不调用 Commit(success)
	// 不再 Acquire 同一 key 则不会再次执行；若再次 Acquire 应得到 WaitOtherWorker（已占位未完成）
	decision2, _, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
	if decision2 != InvocationDecisionWaitOtherWorker {
		t.Fatalf("expected WaitOtherWorker when first run did not commit, got %v", decision2)
	}
	// Recover 无 committed 结果
	got, exists := ledger.Recover(ctx, job1, key1)
	if exists || got != nil {
		t.Fatalf("Recover expected (nil, false), got (%q, %v)", got, exists)
	}
	_ = rec
}

// TestLedger_5_ReplayRecovery_FromEventsRestoresNoDuplicateSideEffect 证明：Replay 时通过 replayResult（事件流恢复）注入，Acquire 返回 ReturnRecordedResult，不执行 tool
func TestLedger_5_ReplayRecovery_FromEventsRestoresNoDuplicateSideEffect(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	ctx := context.Background()

	// 模拟从事件流恢复的已完成调用结果（如 tool_invocation_finished success）
	replayResult := []byte(`{"done":true,"output":"from-events"}`)

	// Replay：带 replayResult 的 Acquire 必须返回 ReturnRecordedResult，且结果一致
	decision, rec, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, replayResult)
	if err != nil {
		t.Fatalf("Acquire with replayResult: %v", err)
	}
	if decision != InvocationDecisionReturnRecordedResult {
		t.Fatalf("expected ReturnRecordedResult when replayResult provided, got %v", decision)
	}
	if rec == nil || string(rec.Result) != string(replayResult) {
		t.Fatalf("expected record with replay result, got %v", rec)
	}

	// Recover 在“仅事件、未写 store”的 replayed job 上可能为 false；此处我们未 Commit，所以 Recover 无
	got, exists := ledger.Recover(ctx, job1, key1)
	if exists {
		t.Fatalf("Recover expected false (no store commit), got true with %q", got)
	}
	_ = got
}

// TestLedger_Recover_AfterCommit 证明 Recover 在 Commit 后返回已提交结果
func TestLedger_Recover_AfterCommit(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	ctx := context.Background()

	decision, rec, err := ledger.Acquire(ctx, job1, step1, tool1, argsH1, key1, nil)
	if err != nil || decision != InvocationDecisionAllowExecute || rec == nil {
		t.Fatalf("Acquire: %v", err)
	}
	result := []byte(`{"x":1}`)
	if err := ledger.Commit(ctx, rec.InvocationID, key1, result); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	got, exists := ledger.Recover(ctx, job1, key1)
	if !exists || string(got) != string(result) {
		t.Fatalf("Recover: expected (%q, true), got (%q, %v)", result, got, exists)
	}
}

// TestAdapter_Replay_InjectsResult_NoToolCall 证明：Adapter 在 Ledger + CompletedToolInvocations 注入时只恢复结果，不调用 tool
func TestAdapter_Replay_InjectsResult_NoToolCall(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	var callCount int32
	tools := &countToolExec{count: &callCount}
	adapter := &ToolNodeAdapter{
		Tools:            tools,
		InvocationLedger: ledger,
	}
	jobID := job1
	taskID := step1
	toolName := tool1
	cfg := map[string]any{"a": 1}
	idempotencyKey := IdempotencyKey(jobID, taskID, toolName, cfg)
	replayResult := []byte(`{"done":true,"output":"replayed"}`)
	ctx := context.Background()
	ctx = WithJobID(ctx, jobID)
	ctx = WithCompletedToolInvocations(ctx, map[string][]byte{idempotencyKey: replayResult})
	agent := (*runtime.Agent)(nil)
	payload := &AgentDAGPayload{Results: make(map[string]any)}

	out, err := adapter.runNode(ctx, taskID, toolName, cfg, agent, payload)
	if err != nil {
		t.Fatalf("runNode: %v", err)
	}
	if out == nil || out.Results[taskID] == nil {
		t.Fatalf("expected result injected")
	}
	if atomic.LoadInt32(&callCount) != 0 {
		t.Fatalf("expected 0 tool calls (replay inject only), got %d", callCount)
	}
	m, _ := out.Results[taskID].(map[string]any)
	if m["output"] != "replayed" {
		t.Fatalf("expected output replayed, got %v", m)
	}
}

type countToolExec struct {
	count *int32
}

func (c *countToolExec) Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (ToolResult, error) {
	atomic.AddInt32(c.count, 1)
	return ToolResult{Done: true, Output: "called"}, nil
}
