# Aetheris Runtime Contract

本契约规定运行时语义的固定规则，供实现与测试对齐。参见 [effect-system.md](effect-system.md)、[job-state-machine.md](job-state-machine.md)。

---

## 一、哪些行为可以重放？哪些只能记录？

- **可重放**：无副作用的计算（Pure）；Replay 时从事件流注入已记录结果，或按策略允许重算。
- **只记录、禁止重放**：LLM 调用、Tool 调用、外部 IO、时间/随机数依赖。Replay 时**禁止**真实调用，只读事件流中的 `command_committed`、`tool_invocation_finished`、`PlanGenerated`、`NodeFinished` 等注入结果。详见 [effect-system.md](effect-system.md)。
- **Tool 执行以事件流声明为屏障**：事件流中若存在某 idempotency_key 的 `tool_invocation_started` 且无对应 `tool_invocation_finished`（Pending），则**禁止再次执行**该 Tool；仅可从 Ledger/tool_invocations 恢复结果并写回 Finished（catch-up），或确定性地失败该 step。见 [effect-system.md](effect-system.md) 副作用屏障。

Reclaim 后再次 Claim 的 Job 会走 Replay；**安全前提**是 Effect 边界成立，否则会出现重复执行副作用（如发 2 封邮件、扣 2 次款）。

---

## 二、Job 在等待什么？（Blocking Semantics）

- 等待条件属于 **Job 的显式状态**，由事件流中的 `job_waiting` 表示。
- **Payload 契约**：`job_waiting` 必须包含 `correlation_key` 与 `wait_type`（webhook | human | timer | signal）。`correlation_key` 唯一标识「这次等待」。
- **恢复规则**：只有携带**相同 correlation_key**（及可选 `wait_type` 匹配）的 signal 才允许写入 `wait_completed` 并解除阻塞；否则 API 返回 400。
- Replay/恢复时，若事件流最后一条状态相关事件为 `job_waiting`，则该 Job 处于 **Blocked**；Scheduler **不得**将其当作可 reclaim 的 Running 或普通 Pending 调度。只有收到匹配 signal 并写入 `wait_completed` 后，Job 才重新变为可 Claim。

**接口**：见 [internal/runtime/jobstore/event.go](../internal/runtime/jobstore/event.go)（JobWaitingPayload）、[internal/api/http/handler.go](../internal/api/http/handler.go)（JobSignal 校验 correlation_key）、[internal/agent/job/state.go](../internal/agent/job/state.go)（IsJobBlocked）。

### 外部事件送达保证（External Event Guarantee）

- **送达语义**：Signal / Message 为 **at-least-once**。一旦 `wait_completed` 已写入且 Job 已置为 Pending，该 Job 将被 Scheduler 认领并继续执行；不会丢失「已送达」的 signal。
- **重复幂等**：同一 `correlation_key` 的 signal（或 message 解除同一等待）若被多次调用，仅第一次会追加 `wait_completed`；后续请求若发现事件流中最后一条已是 `wait_completed` 且 `correlation_key` 一致，则直接返回 200（已送达），不再追加事件，避免重复 unblock。

### Durable External Interaction Model（2.0 at-least-once）

- **Signal 先入持久化 inbox**：当配置 **SignalInbox** 时，JobSignal API 先调用 `SignalInbox.Append(jobID, correlationKey, payload)` 将 signal 持久化，再 Append `wait_completed` 并 UpdateStatus(Pending)。若 API 在「收到请求后、Append wait_completed 前」崩溃，signal 已落盘，可后续重试或由后台补写 wait_completed，保证「人类点击一次 → agent 一定收到」。
- **Ack 机制**：`wait_completed` 成功写入且 Job 已置为 Pending 后，对对应 inbox 记录调用 `MarkAcked`，避免重复消费。实现见 [internal/agent/signal](../internal/agent/signal)；PG 表 `signal_inbox` 见 [internal/runtime/jobstore/schema.sql](../internal/runtime/jobstore/schema.sql)。
- **At-least-once 已满足**：当前实现（先写 inbox、再 Append、再 MarkAcked）已满足 at-least-once delivery；Worker 或 runtime 重启后 signal 不丢。若需「重投递」语义（按 offset 重试消费、未 ack 的 signal 再次投递），可在 signal inbox 表/接口上扩展 offset 与重试策略，属可选增强。

**接口**：JobSignal / JobMessage 在 Append 前通过 `lastEventIsWaitCompletedWithCorrelationKey(events, correlationKey)` 判断并短路返回。

---

## 三、谁拥有执行权？（租约 + Heartbeat）

- **执行权 = 租约持有者**。Claim / ClaimJob 获得租约；Heartbeat 续租。
- **Reclaim 仅依据 event store 租约过期**：调用 `ListJobIDsWithExpiredClaim(ctx)`，对返回的 `job_id` 在 metadata 侧置回 Pending（且**不回收** Blocked Job，见 §2）。**不得**仅凭 metadata 的 `updated_at` 判断孤儿，否则活着的 Worker 可能被误回收。
- 单一事实源：event store 的租约与心跳为权威；metadata 的 status 仅用于查询与 Reclaim 时的「置回 Pending」操作。

**接口**：见 [internal/runtime/jobstore/store.go](../internal/runtime/jobstore/store.go)（ListJobIDsWithExpiredClaim）、[internal/agent/job/reclaim.go](../internal/agent/job/reclaim.go)（ReclaimOrphanedFromEventStore）、[internal/app/worker/agent_job.go](../internal/app/worker/agent_job.go)（Reclaim 调用）。

---

## 四、决策来源是否可追溯？（PlanGenerated 必须存在才执行）

- **契约**：任意执行路径（含 Replay、恢复、首次运行）在**未**从事件流中读到 `PlanGenerated`（或等价规划记录）时，**不得**调用 Planner.Plan；若没有则应**失败**（Job 置为 Failed），而不是重新 Plan。
- Replay 禁止调用 Planner；执行仅允许在「已有该 Job 的 PlanGenerated 记录」的前提下进行。运维如需「重新规划」应通过显式 API 写入新的 Plan 事件。
- **Workflow 确定性边界**：执行与 Replay 仅依赖**已记录的决策**（当前即 PlanGenerated）；不在 Replay 中调用任何 Planner/LLM 决定执行路径。即「LLM 提议，Runtime 决定」— 见 [workflow-decision-record.md](workflow-decision-record.md)。

**接口**：见 [internal/agent/runtime/executor/runner.go](../internal/agent/runtime/executor/runner.go)（无 PlanGenerated 则返回错误并置 Failed）、[effect-system.md](effect-system.md)。

---

## 五、执行 Epoch：谁可以写事件？

- **仅当前 attempt 可写事件**。Claim / ClaimJob 时生成 `attempt_id` 并写入 event store 的 claim 记录；Heartbeat 不改变 `attempt_id`。
- **Append 规则**：当 context 中携带 `attempt_id`（Worker 执行路径）时，store 校验「本请求的 attempt_id 与当前该 job 的 claim.attempt_id 一致」；不一致则返回 `ErrStaleAttempt`，拒绝写入。避免旧 Worker 恢复后写事件污染事件流（split-brain）。
- 非 Worker 路径（如 API 创建 Job、写入 PlanGenerated、JobSignal 写入 wait_completed）不传 `attempt_id`，Append 不校验 attempt，允许写入。

**接口**：见 [internal/runtime/jobstore/store.go](../internal/runtime/jobstore/store.go)（WithAttemptID、AttemptIDFromContext、ErrStaleAttempt）、Claim/ClaimJob 返回 attemptID；pg/memory Store 的 Append 校验 context 中的 attempt_id。

---

## 六、Scheduler 正确性（P2）

- **当前**：Job 级租约、attempt_id、Reclaim 以 event store 为准、Append 校验已实现；见 §3、§5。
- **P2 目标**：Execution ownership 强化 — lease fencing、step heartbeat（可选）、worker epoch / stale worker 检测与文档化。详见 [scheduler-correctness.md](scheduler-correctness.md)。

---

## 七、Cross-Version Replay（跨版本恢复）

### 契约

- **Execution Version Binding**：Job 创建时可记录 `execution_version`（代码版本，如 git tag）、`planner_version`（Planner 版本）；Replay 时检查版本是否匹配。
- **Version Mismatch 策略**：
  - **warning mode**（1.0 默认）：版本不匹配时记录 warning 日志，继续 Replay（假设向后兼容）
  - **strict mode**（可选）：版本不匹配时拒绝执行，返回 `ErrVersionMismatch`
  - **auto-migrate**（未来）：按 version 路由到旧代码或执行 schema migration
- **PlanGenerated Versioning**：`plan_generated` payload 可包含 `planner_version`、`task_graph_schema_version`；Replay 时可据此判断"旧 Plan schema"与"新 Plan schema"是否兼容。

**用途**：
- 系统演进（Planner 更新、Tool 更新）后，旧 Job 仍可恢复（若版本兼容）
- 审计可追溯"执行时用的哪个版本代码"
- 版本不兼容时显式失败，而非静默错误

**接口**：
- **Job**：[internal/agent/job/job.go](../internal/agent/job/job.go) 增加 `ExecutionVersion`、`PlannerVersion` 字段
- **Runner**：[internal/agent/runtime/executor/runner.go](../internal/agent/runtime/executor/runner.go) `RunForJob` 开始时检查 `j.ExecutionVersion`；不匹配时 warning 或 fail
- **PlanGenerated**：payload 可增加 `planner_version`、`schema_version`

详见 [versioning.md](versioning.md) § Cross-Version Replay。

---

## 实现顺序与测试

1. **Reclaim 以 event store 为准** + **不回收 Blocked**（§2、§3）。
2. **Blocking semantics**：`job_waiting` 含 `correlation_key`，JobSignal 校验；Reclaim 排除 blocked（IsJobBlocked）。
3. **Execution epoch**：Claim 生成 attempt_id，Append 校验；Runner/Sink 通过 context 携带 attempt_id。
4. **Planner 锁定**：无 PlanGenerated 不执行，移除「无 Plan 则重新 Plan」的兼容路径（§4）。
5. 单测应体现「违反契约则失败」：例如 Append 带错误 attempt_id 被拒绝、JobSignal 不带或错 correlation_key 被拒绝、无 PlanGenerated 时 RunForJob 返回错误并置 Job Failed。
