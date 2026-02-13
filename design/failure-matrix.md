# Failure Matrix — 故障 × 保证 × 行为 × 配置

本文档将 Aetheris 在各类故障下的**保证是否成立**、**系统行为**与**推荐配置**整理为单一矩阵表。内容抽取自 [docs/runtime-guarantees.md](../docs/runtime-guarantees.md) 与 [design/scheduler-correctness.md](scheduler-correctness.md)，用于合规说明与运维决策。

---

## 1. 故障类型（行）

| 故障 | 描述 |
|------|------|
| **Worker crash (before tool)** | Worker 在调用 Tool 前崩溃（如 Plan 后、某步 LLM 后） |
| **Worker crash (after tool, before commit)** | Tool 已执行，在写入 command_committed / tool_invocation_finished 前崩溃 |
| **Worker crash (after commit)** | command_committed 已写入，在下一步或 checkpoint 前崩溃 |
| **Step timeout** | 单步执行超过配置的 step_timeout |
| **Signal lost** | POST /api/jobs/:id/signal 请求失败或未到达 |
| **Two workers same step** | 租约刚过期，两 Worker 同时 Claim 同一 Job |
| **Network partition** | Worker 与 JobStore 网络断开，另一 Worker Claim 同一 Job |
| **Ledger conflict** | 同一 idempotency_key 下多 Worker 竞争 Commit |
| **JobStore / Event Store 不可用** | Postgres 或共享存储不可用 |
| **Reclaim** | 租约过期，Reclaim 回收 Job 由其他 Worker 认领 |
| **Blocked Job (job_waiting)** | Job 在 Wait 节点挂起，最后事件为 job_waiting |

---

## 2. 保证类型（列）

| 保证 | 含义 |
|------|------|
| **Step 至少执行一次** | 在重试/Reclaim 下，可重试的步最终由某 Worker 推进 |
| **Step 至多执行一次（副作用）** | 同一逻辑步的 Tool/LLM 不重复执行；Replay 注入结果 |
| **Replay 不重执行** | Replay 路径不调用 LLM/Tool，仅从事件流/Ledger/Effect Store 注入 |
| **Signal 不丢（至少一次）** | wait_completed 写入后 Job 一定会被重新调度 |
| **Job 不丢** | 非终态 Job 不会因 Worker 崩溃而永久丢失 |
| **事件流不被污染** | 非当前 attempt_id 的写入被拒绝（ErrStaleAttempt） |
| **Tool 仅执行一次** | Ledger 仲裁下同一 idempotency_key 仅一次 Commit |

---

## 3. 故障 × 保证 矩阵

| 故障 | Step ≥1 | Step ≤1 | Replay 不重执行 | Signal 不丢 | Job 不丢 | 事件流不污染 | Tool 一次 |
|------|---------|---------|-----------------|-------------|----------|--------------|----------|
| Worker crash (before tool) | ✅ Reclaim 后继续 | ✅ 未执行 | ✅ Replay 注入 | ✅ | ✅ | ✅ | ✅ |
| Worker crash (after tool, before commit) | ✅ Reclaim 后继续 | ✅ Effect Store catch-up 或 Barrier 阻重执行 | ✅ Catch-up / 注入 | ✅ | ✅ | ✅ | ✅ Ledger/Barrier |
| Worker crash (after commit) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Step timeout | ✅ 可 Requeue | ✅ 未 commit 不重执行 | ✅ | ✅ | ✅ | ✅ | ✅ |
| Signal lost | — | — | — | ⚠️ 需重试 signal | ✅ | ✅ | — |
| Two workers same step | ✅ 仅一人推进 | ✅ attempt_id + Ledger | ✅ | ✅ | ✅ | ✅ | ✅ Ledger |
| Network partition | ✅ 仅持有者推进 | ✅ attempt_id + Ledger | ✅ | ✅ | ✅ | ✅ ErrStaleAttempt | ✅ |
| Ledger conflict | ✅ | ✅ 仅一人 Commit | ✅ | ✅ | ✅ | ✅ | ✅ 仲裁 |
| JobStore 不可用 | ❌ 无法推进 | — | — | — | ❌ 可能丢 | — | — |
| Reclaim | ✅ 新 Worker 继续 | ✅ Replay 注入 | ✅ | ✅ | ✅ | ✅ | ✅ |
| Blocked Job | — | — | — | ✅ 写 wait_completed 后必调度 | ✅ 不回收 Blocked | ✅ | — |

说明：✅ = 在推荐配置下成立；⚠️ = 需操作（如重试）；❌ = 不成立；— = 不适用。

---

## 4. 故障 × 系统行为

| 故障 | 系统行为 | 备注 |
|------|----------|------|
| Worker crash (before tool) | 租约过期 → Reclaim → Job 回 Pending → 新 Worker Claim → Replay 从 Checkpoint 继续 | 已提交步注入，未提交步重跑 |
| Worker crash (after tool, before commit) | Replay 时：Effect Store 有则 catch-up 写事件并注入；无则 Activity Log Barrier 禁止再执行，从 Ledger 恢复或永久失败 | 两步提交保证 |
| Worker crash (after commit) | Reclaim 后 Replay 到最新，继续下一节点 | 无副作用重放 |
| Step timeout | context 取消 → retryable_failure → Job Failed 或 Requeue | 配置 retry_max、step_timeout |
| Signal lost | Job 保持 Waiting/Parked；重试 POST signal 幂等 | 同 correlation_key 仅一次 wait_completed |
| Two workers same step | 一人 Append 成功，另一人 ErrStaleAttempt；Ledger Acquire 仅一人 AllowExecute | 事件 + Ledger 双重保证 |
| Network partition | 失联 Worker 租约过期；新 Worker Claim；旧 Worker 恢复后写入被拒 | 共享 JobStore 必须 |
| Ledger conflict | 同一 idempotency_key 仅一人 Commit，他人 ReturnRecordedResult 或 WaitOtherWorker | 共享 ToolInvocationStore |
| JobStore 不可用 | 无法 Append、无法 Claim；Job 可能丢（若内存仅存） | 生产必须持久化 JobStore |
| Reclaim | ListJobIDsWithExpiredClaim → ReclaimOrphanedFromEventStore；排除 Blocked Job | 见 runtime-contract §2 |
| Blocked Job | Reclaim 不回收；仅 wait_completed 后变为 Queued | IsJobBlocked(events)==true 则跳过 |

---

## 5. 故障 × 推荐配置

| 故障 | 推荐配置 |
|------|----------|
| Worker crash (any) | JobStore: Postgres（或共享存储）；Event Store: 同 JobStore；InvocationLedger: 启用；Effect Store: 启用（after tool before commit 场景）；CheckpointStore: 配置 |
| Step timeout | executor.step_timeout 配置；retry_max / backoff 按需 |
| Signal lost | 无特殊配置；调用方重试 signal；可选 WakeupQueue（Redis）减少轮询延迟 |
| Two workers / Network partition | 共享 JobStore + 共享 ToolInvocationStore；attempt_id 校验（已实现）；Ledger 启用 |
| Ledger conflict | InvocationLedger + 共享 ToolInvocationStore（如 Postgres）；可选 AttemptValidator |
| JobStore 不可用 | 高可用 Postgres；监控与告警 |
| Reclaim / Blocked | Lease TTL、Heartbeat 配置；Reclaim 排除最后事件为 job_waiting 的 Job（已实现） |

---

## 6. 参考

- [docs/runtime-guarantees.md](../docs/runtime-guarantees.md) — 各场景详细说明与示例
- [design/scheduler-correctness.md](scheduler-correctness.md) — 租约、attempt_id、两步提交
- [design/execution-guarantees.md](execution-guarantees.md) — 正式保证与成立条件
- [design/formal-state-machine.md](formal-state-machine.md) — Job 状态机形式化
