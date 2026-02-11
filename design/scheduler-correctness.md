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
   所有写操作（事件 Append、Ledger Commit）在 writer 的 attempt_id 非当前 job 持有者时均被拒绝。
   - **事件 Append**：已按 attempt_id 校验（store 层）；非当前 attempt 返回 `ErrStaleAttempt`。
   - **Ledger Commit**：InvocationLedger 可选配置 `AttemptValidator`；Commit 时从 context 取 job_id 与 attempt_id，校验与 event store 当前 claim 的 attempt_id 一致，否则返回 `ErrStaleAttempt`，避免失去租约的 Worker 写入 Ledger 导致双执行或状态错乱。
   - **约定**：所有「会改变该 job 执行状态」的写操作（Append、Ledger Commit、Cursor 更新等）必须在同一 attempt 下；失去租约的 Worker 必须停止执行并不再发起写入。

2. **Step heartbeat / step-level lease（可选）**  
   若单步执行时间很长，可引入「步级」心跳或租约，便于观测「谁正在执行哪一步」；Reclaim 时若需更细粒度，可基于步级租约而非仅 job 级。当前 Reclaim 仅 job 级，新 Worker 认领后通过 Replay 继续，语义正确。

3. **Step timeout**  
   - 为单步执行配置最大执行时间（如 Runner 的 StepTimeout）；超时后将该步标记为失败（可配置为 retryable 或 permanent），并写入 node_finished（result_type=retryable_failure 或 permanent_failure），便于 Reclaim 后由新 Worker 重试或终止 Job。
   - 实现方式：Runner 在调用 step.Run 时使用 `context.WithTimeout(ctx, StepTimeout)`；若返回 `context.DeadlineExceeded`，按可重试失败处理（或按配置决定）。
   - 与 Lease fencing 一致：超时后当前 Worker 不再写入该步的 command_committed（因未成功完成）；Job 级租约仍由 Heartbeat 维持，或由 Reclaim 回收后由新 Worker 从事件流 Replay 继续。

4. **Worker epoch / stale worker kill**  
   明确约定：Worker 在失去租约（如 Reclaim 后由其他 Worker 认领）后**必须停止执行**，不再发起 Append。可选：在长耗时步前再次校验 attempt_id，若已失效则主动退出。当前 attempt_id 校验已能拒绝过期 Worker 的写入；文档化「Worker 必须在发现租约丢失后停止」即可形成闭环。

---

## Step 两阶段提交与 Effect Store

为保证同一 step 的副作用**最多执行一次**，当配置 **Effect Store** 时采用两步提交：

- **Phase 1**：Execute 成功后先将 effect 写入 Effect Store。
- **Phase 2**：Effect Store 写入成功后再 Append `command_committed` / `tool_invocation_finished` 与 NodeFinished。

Replay/恢复时：若事件流无 command_committed 但 Effect Store 有该 step 的 effect，则 **catch-up**（写回事件、不重执行）；若 Effect Store 也无，才执行 Tool/LLM 并先写 Effect Store 再 Append。详见 [effect-system.md](effect-system.md)、[execution-state-machine.md](execution-state-machine.md)。

## 实现状态

- **当前**：Job 租约 + attempt_id + Reclaim 以 event store 为准 + Append 校验。见 [internal/runtime/jobstore/store.go](../internal/runtime/jobstore/store.go)、[internal/agent/job/reclaim.go](../internal/agent/job/reclaim.go)。
- **P2**：Lease fencing 已实现（Ledger Commit + AttemptValidator）；Step timeout 已实现最小可用（Runner.StepTimeout，超时按 retryable_failure）；Step heartbeat（可选）、Worker epoch 文档与必要时校验。见 [internal/agent/runtime/executor/runner.go](../internal/agent/runtime/executor/runner.go) StepTimeout 与 runLoop 内 WithTimeout。
- **两步提交**：Effect Store 接口与内存实现见 [internal/agent/runtime/executor/effect_store.go](../internal/agent/runtime/executor/effect_store.go)；Adapter 先 PutEffect 再 Append，runNode 内 Effect Store catch-up 见 [node_adapter.go](../internal/agent/runtime/executor/node_adapter.go)。
