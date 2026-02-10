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

## 实现顺序与测试

1. **Reclaim 以 event store 为准** + **不回收 Blocked**（§2、§3）。
2. **Blocking semantics**：`job_waiting` 含 `correlation_key`，JobSignal 校验；Reclaim 排除 blocked（IsJobBlocked）。
3. **Execution epoch**：Claim 生成 attempt_id，Append 校验；Runner/Sink 通过 context 携带 attempt_id。
4. **Planner 锁定**：无 PlanGenerated 不执行，移除「无 Plan 则重新 Plan」的兼容路径（§4）。
5. 单测应体现「违反契约则失败」：例如 Append 带错误 attempt_id 被拒绝、JobSignal 不带或错 correlation_key 被拒绝、无 PlanGenerated 时 RunForJob 返回错误并置 Job Failed。
