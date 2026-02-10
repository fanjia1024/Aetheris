# Scheduler Correctness (P2)

本设计描述 **Execution Ownership** 的下一阶段目标：在 job 级租约与 attempt_id 基础上，进一步保证「执行权归属」明确，避免网络抖动下多 Worker 同时推进同一执行状态。当前实现见 [runtime-contract.md](runtime-contract.md) §3、§5。

---

## 当前已有

- **Job 级租约**：Claim / ClaimJob 获得租约，Heartbeat 续租；Reclaim 仅依据 event store 的 `ListJobIDsWithExpiredClaim`，不凭 metadata 时间。
- **Attempt_id**：Claim 时生成，Append 时校验；非当前 attempt 的写入被拒绝（ErrStaleAttempt），避免旧 Worker 写事件。
- **Blocked Job 不回收**：Reclaim 排除「最后事件为 job_waiting」的 Job。

因此 **Job 所有权** 已具备基本正确性；**Step 所有权** 尚未显式建模。

---

## P2 目标（后续实现）

1. **Lease fencing**  
   所有写操作（事件 Append、Ledger Commit）在 writer 的 attempt_id 非当前 job 持有者时均被拒绝。当前 Append 已按 attempt_id 校验；Ledger 若需跨 Worker 可见，可考虑在 Commit 时校验 job 的当前 attempt 或 worker。

2. **Step heartbeat / step-level lease（可选）**  
   若单步执行时间很长，可引入「步级」心跳或租约，便于观测「谁正在执行哪一步」；Reclaim 时若需更细粒度，可基于步级租约而非仅 job 级。当前 Reclaim 仅 job 级，新 Worker 认领后通过 Replay 继续，语义正确。

3. **Worker epoch / stale worker kill**  
   明确约定：Worker 在失去租约（如 Reclaim 后由其他 Worker 认领）后**必须停止执行**，不再发起 Append。可选：在长耗时步前再次校验 attempt_id，若已失效则主动退出。当前 attempt_id 校验已能拒绝过期 Worker 的写入；文档化「Worker 必须在发现租约丢失后停止」即可形成闭环。

---

## 实现状态

- **当前**：Job 租约 + attempt_id + Reclaim 以 event store 为准 + Append 校验。见 [internal/runtime/jobstore/store.go](../internal/runtime/jobstore/store.go)、[internal/agent/job/reclaim.go](../internal/agent/job/reclaim.go)。
- **P2**：Lease fencing 强化、Step heartbeat（可选）、Worker epoch 文档与必要时校验。具体实现待优先级确定后补全。
