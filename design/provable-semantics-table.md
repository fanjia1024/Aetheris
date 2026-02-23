# Provable Semantics Table — 可证明语义表

本文档将 at-most-once 与 Confirmation Replay 的**可证明语义**固化为表格与约定，供 CTO/审计追问「Ledger 与事件是否同一事务」「各 crash 窗口行为」时直接引用。参见 [1.0-runtime-semantics.md](1.0-runtime-semantics.md)、[execution-proof-sequence.md](execution-proof-sequence.md)、[effect-system.md](effect-system.md)。

---

## 1. 事务边界与一致性模型

**当前实现不使用 Ledger 与 JobStore 的同一 DB 事务。** 保证依赖以下两点：

- **写入顺序**：Tool 成功路径上，先写 Effect Store（若配置），再 Append 事件，再 Ledger.Commit（即 Store.SetFinished）。顺序见 §3。
- **补偿约定**：Replay 时以**事件流为权威**；若事件流无 `tool_invocation_finished`/`command_committed` 但 Effect Store 或 Ledger/Store 已有该步的已提交结果，则执行 **catch-up**（写回事件、注入结果、不再次执行 Tool）。因此「Execute 成功、Append 前崩溃」的窗口由 Effect Store catch-up 覆盖；「Append 成功、Commit 前崩溃」的窗口由 Replay 从事件流恢复结果、Ledger Acquire 时 replayResult 命中覆盖。

若未来需要「可证明」更强的原子性（例如 JobStore 与 ToolInvocationStore 同库时的单事务写），需重新评估：Replay 以事件流为权威，若 Commit 先于 Append 落盘，Replay 构建 ReplayContext 时尚无事件，可能误判为未执行；因此当前顺序（先 Append 再 Commit）与设计一致。

---

## 2. Crash 窗口表

Tool 步成功路径上的各阶段及 Worker 在该阶段崩溃后的 Replay 行为、是否可能重复副作用如下。实现位置：`internal/agent/runtime/executor/node_adapter.go`（`runNodeExecute` 及 Acquire/catch-up 路径）。

| Crash 窗口 | 事件流状态 | Ledger/Store 状态 | Effect Store | Replay 行为 | 是否可能重复副作用 |
|------------|------------|-------------------|--------------|-------------|--------------------|
| **1. Tool 执行前** | 无 started | 无记录或 NEW | 无 | 新 Worker Replay 后 Acquire 得 AllowExecute，执行一次 Tool，再 Append + Commit | 否（仅执行一次） |
| **2. Execute 后、PutEffect 前** | 有 started，无 finished | 有 SetStarted（INFLIGHT）或无 | 无 | Acquire 得 WaitOtherWorker 或（无 Ledger 时）无 committed 记录；若 Reclaim 后另一 Worker 从事件流 BuildFromEvents 得 PendingToolInvocations，禁止再执行、仅 catch-up 或永久失败；若 Effect Store 无记录则无 catch-up，按「invocation in flight or lost」永久失败或重试（取决于配置） | 否（不执行 Tool；可能永久失败） |
| **3. PutEffect 后、Append 前** | 有 started，无 finished | INFLIGHT 或无 | 有 | Replay 时事件流无 command_committed，但 EffectStore.GetEffectByJobAndIdempotencyKey 有记录 → **catch-up**：写回 tool_invocation_finished/command_committed，注入结果，不调用 Tool | 否 |
| **4. Append 后、Commit 前** | 有 started + finished + command_committed | INFLIGHT 或未 SetFinished | 有 | Replay 从事件流 BuildFromEvents 得到 CompletedToolInvocations[idempotencyKey]；Acquire(..., replayResult) 命中 → ReturnRecordedResult，注入结果，不调用 Tool | 否 |
| **5. Commit 后** | 有 started + finished + command_committed | COMMITTED | 有 | 同上；Replay 注入，或 Ledger.GetByJobAndIdempotencyKey 得 committed 记录 → ReturnRecordedResult | 否 |

结论：在配置 InvocationLedger（及共享 ToolInvocationStore）与可选 Effect Store 的前提下，**任意 crash 窗口下均不会重复执行同一 Tool 步的副作用**；窗口 2 可能产生「invocation in flight or lost」的永久失败，需 Reclaim/超时策略或人工介入。

---

## 3. Ledger 与 Append 顺序（当前实现）

成功路径（`runNodeExecute`）的**实际顺序**如下；Replay 以事件流为权威，因此事件必须先于 Ledger Commit 可见（或通过 catch-up 补全事件）。

1. **Append** `tool_invocation_started`（Activity Log Barrier；先声明再执行）
2. **Execute** `Tools.Execute(toolName, cfg, state)`
3. **PutEffect**（若配置 Effect Store）— Phase 1
4. **Append** `tool_invocation_finished`（outcome=success）、**Append** `command_committed` — Phase 2
5. **Ledger.Commit** → **Store.SetFinished**（idempotency_key, success, result, committed=true）
6. 后续由 Runner Append NodeFinished、StateCheckpointed 等

原因：ReplayContext 由 `ListEvents(jobID)` 构建；若先 Commit 再 Append，极端情况下事件尚未可见时 Replay 会认为该步未完成，可能误判；当前顺序保证「事件流中已有 finished/committed」时，Ledger/Store 上必有或可由 catch-up 补全的已提交记录。

---

## 4. 幂等键规则

### 4.1 内部 idempotency_key（Ledger / 事件流）

- **公式**：`idempotency_key = SHA256(jobID + "\x00" + nodeID + "\x00" + toolName + "\x00" + canonicalize(args))`
- **实现**：`internal/agent/runtime/executor/invocation.go` 的 `IdempotencyKey(jobID, nodeID, toolName, args)`
- **用途**：ToolInvocationStore / Ledger 按 `(job_id, idempotency_key)` 查重；事件流中 `tool_invocation_started`/`tool_invocation_finished` 的 payload 含 `idempotency_key`；Replay 时 `CompletedToolInvocations[idempotency_key]` 注入结果。
- **约定**：同一逻辑步（同一 job、同一 node/step、同一 tool、同一 args）在任何 Replay 或 retry 下得到同一 idempotency_key；nodeID 应为确定性 StepID（见 [step-identity.md](step-identity.md)）。

### 4.2 对外幂等键（下游 API 去重）

- **公式**：`aetheris:job_id:step_id:attempt_id`（attempt_id 来自 Worker Claim 的 context，空时用 `"0"`）
- **实现**：`internal/agent/runtime/executor/types.go` 的 `StepIdempotencyKeyForExternal(ctx, jobID, stepID)`
- **用途**：Tool 实现应将此键传给下游（email、payment、webhook、API），下游据此做 at-most-once 去重。
- **约定**：同一 Job、同一 Step、同一 attempt 下唯一；Reclaim 后 attempt 变化，新 attempt 的步会得到新键，避免与旧 attempt 的已提交步混淆。

---

## 5. 回放策略分级（Replay Verification Mode）

Confirmation Replay 时，若 **ResourceVerifier** 校验「外部世界与事件流一致」失败，按 **ReplayVerificationMode** 处理。类型与常量见 `internal/agent/runtime/executor/state_diff.go`；`ToolNodeAdapter.ReplayVerificationMode` 可配置，默认 `ReplayVerificationStrict`。

| 策略 | 含义 | 当前实现 |
|------|------|----------|
| **strict** | 验证失败 → job 永久失败 | 是（默认）；`ReplayVerificationStrict` |
| **warn** | 验证失败 → 记录 `ConfirmationReplayWarnTotal` 指标并继续注入结果，不失败 job | 是；`ReplayVerificationWarn` |
| **human-in-loop** | 验证失败 → 返回 `ErrReplayVerificationHumanRequired`，调用方（Runner/Worker）可据此 park job 并等待人工确认后恢复 | 是；`ReplayVerificationHumanInLoop`；Worker 层 park 集成为可选扩展 |

调用方判断 human-in-loop：`errors.Is(err, executor.ErrReplayVerificationHumanRequired)` 为 true 时可将 Job 置为 Parked 并发送通知，待人工确认后再 signal 恢复。

ResourceVerifier 生产默认 nil（不执行具体校验）；需至少一个具体实现并在 bootstrap 中挂载才能使「校验外部世界」非 no-op。可选占位：`executor.NoOpResourceVerifier` 始终通过，仅用于占位；**生产环境应替换为具体 Verifier**（如 [internal/agent/runtime/executor/verifier/github.go](internal/agent/runtime/executor/verifier/github.go)）。见 [1.0-runtime-semantics.md](1.0-runtime-semantics.md) 末段。

---

## 6. 参考

- [1.0-runtime-semantics.md](1.0-runtime-semantics.md) — 三机制与 Execution Proof Chain
- [execution-proof-sequence.md](execution-proof-sequence.md) — Runner–Ledger–JobStore 序列图
- [execution-guarantees.md](execution-guarantees.md) — 保证一览与条件
- [effect-system.md](effect-system.md) — 两步提交与 catch-up
- [step-identity.md](step-identity.md) — Step 身份与确定性 StepID
- [verification-mode.md](verification-mode.md) — 离线验证与证明输出
