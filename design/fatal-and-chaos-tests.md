# Fatal and Chaos Tests — 致命场景与混沌测试清单

本文档维护 Aetheris at-most-once 与 Confirmation Replay 的**致命场景测试**及**混沌/压力测试**清单，并标注每个用例对应的设计文档与代码位置。用于发布前回归与 CTO/审计核对。参见 [1.0-runtime-semantics.md](1.0-runtime-semantics.md)、[provable-semantics-table.md](provable-semantics-table.md)。

---

## 1. 致命四场景（1.0 证明）

Runtime 1.0 的证明目标：任意 agent step 在**崩溃、重启、双 worker 抢同一 step、replay 恢复**下均**不会**重复触发外部副作用。以下四类场景必须由自动化测试覆盖。

| 场景 | 设计语义 | 测试名称 | 代码位置 | 说明 |
|------|----------|----------|----------|------|
| **(1) Worker 在 tool 执行前崩溃** | 无已提交记录时，Replay 执行一次并 Commit，之后仅恢复 | `TestLedger_1_CrashBeforeCommit_ReplayReExecutesAndCommits` | [internal/agent/runtime/executor/ledger_1_0_test.go](internal/agent/runtime/executor/ledger_1_0_test.go) | Acquire 得 AllowExecute → Commit → 再次 Acquire 得 ReturnRecordedResult，不执行 tool |
| **(2) Tool 执行后、commit 前崩溃** | 若 Store 已有 committed 记录（或 Replay 从事件流恢复），Acquire 返回 ReturnRecordedResult；Effect Store catch-up 覆盖「Execute 成功、Append 前」窗口 | `TestLedger_2_CrashAfterCommit_ReplayRestoresNoSecondCall` | 同上 | 先 Commit，再 Acquire（无 replayResult）→ ReturnRecordedResult；catch-up 路径在 [node_adapter.go](internal/agent/runtime/executor/node_adapter.go) runNode 内 |
| **(3) 两 worker 同时抢同一 step** | 仅一次 Commit；先 Acquire 得 AllowExecute，后 Acquire 得 WaitOtherWorker | `TestLedger_3_DoubleWorker_OnlyOneCommit` | 同上 | 并发 Acquire，仅一个 AllowExecute，其余 WaitOtherWorker；单次 Commit |
| **(4) Replay 恢复输出不重复调用 tool** | replayResult（事件流恢复）注入时 Acquire 返回 ReturnRecordedResult，0 次 tool 调用 | `TestLedger_5_ReplayRecovery_FromEventsRestoresNoDuplicateSideEffect`、`TestAdapter_Replay_InjectsResult_NoToolCall` | 同上 | Ledger 层：Acquire(..., replayResult) → ReturnRecordedResult；Adapter 层：CompletedToolInvocations 注入时仅恢复、不调用 Tools.Execute |

运行方式：

```bash
go test ./internal/agent/runtime/executor -run 'TestLedger_1|TestLedger_2|TestLedger_3|TestLedger_5|TestAdapter_Replay_InjectsResult' -v
```

---

## 2. PendingToolInvocations（Activity Log Barrier）

| 场景 | 设计语义 | 测试名称 | 代码位置 | 说明 |
|------|----------|----------|----------|------|
| **事件流有 started、无 finished** | 禁止再次执行；仅可从 Ledger/Effect Store 恢复后 catch-up 写回 finished，或永久失败（invocation in flight or lost） | `TestAdapter_PendingToolInvocations_NoDoubleExecute` | [internal/agent/runtime/executor/ledger_1_0_test.go](internal/agent/runtime/executor/ledger_1_0_test.go) | 两子用例：pending 且无 Store 记录 → 永久失败、0 次 tool 调用；pending 且 Store 已 committed → catch-up 注入、0 次 tool 调用 |

Replay 构建：`BuildFromEvents` 在 [internal/agent/replay/replay.go](internal/agent/replay/replay.go) 中，`tool_invocation_started` 加入 `PendingToolInvocations`，`tool_invocation_finished` 删除。Runner 注入： [internal/agent/runtime/executor/runner.go](internal/agent/runtime/executor/runner.go) `WithPendingToolInvocations`。Adapter 判断： [internal/agent/runtime/executor/node_adapter.go](internal/agent/runtime/executor/node_adapter.go) 第 328 行附近，`PendingToolInvocationsFromContext` 命中则走 Ledger/Store 恢复或返回永久失败。

---

## 3. 混沌/压力测试

| 场景 | 测试名称 | 代码位置 | 说明 |
|------|----------|----------|------|
| 多 Worker 争抢同一步 | `TestStress_MultiWorkerRace` | [internal/agent/runtime/executor/stress_test.go](internal/agent/runtime/executor/stress_test.go) | 3 workers 并发 Acquire/Commit 同一 key；仅单次 Commit |
| 大量独立 Job/step | `TestStress_ManyJobs` | 同上 | 多 key 各 Acquire/Commit 一次 |
| Tool 执行后、Commit 前“崩溃” | `TestStress_CrashAfterToolBeforeCommit` | 同上 | 模拟 Commit 前失败；第二 worker 应得到 WaitOtherWorker 或（超时后）可重试语义 |
| 双 Worker 同 step 单 Commit | `TestLedger_3_DoubleWorker_OnlyOneCommit`（见上） | ledger_1_0_test.go | 严格单次 Commit |

运行方式（见 [docs/runtime-guarantees.md](docs/runtime-guarantees.md)）：

```bash
go test ./internal/agent/runtime/executor -run 'TestStress_MultiWorkerRace|TestStress_ManyJobs|TestStress_CrashAfterToolBeforeCommit|TestLedger_3' -v
```

---

## 4. 设计文档与引用

- **可证明语义**（crash 窗口、幂等键、顺序）：[provable-semantics-table.md](provable-semantics-table.md)
- **三机制与 Execution Proof Chain**：[1.0-runtime-semantics.md](1.0-runtime-semantics.md)
- **序列图**：[execution-proof-sequence.md](execution-proof-sequence.md)
- **两步提交与 catch-up**：[effect-system.md](effect-system.md)
- **用户可见保证**：[docs/runtime-guarantees.md](docs/runtime-guarantees.md)

---

## 5. 发布前检查

- [ ] 致命四场景测试全部通过（TestLedger_1/2/3/5、TestAdapter_Replay_InjectsResult_NoToolCall）
- [ ] PendingToolInvocations 行为有显式测试覆盖（`TestAdapter_PendingToolInvocations_NoDoubleExecute`）
- [ ] 混沌/压力测试在 CI 或发布前跑通（Stress_MultiWorkerRace、Stress_ManyJobs、Stress_CrashAfterToolBeforeCommit）
