# Formal Job State Machine — 形式化状态机

本文档对 Job 生命周期做**形式化**定义：状态集合、输入事件集合、迁移表与不变式。与 [job-state-machine.md](job-state-machine.md) 一致，与实现 [internal/agent/job/state.go](../internal/agent/job/state.go) 的 `DeriveStatusFromEvents` 对齐。用于合规、对标与 Verification 的语义基准。

---

## 1. 形式化状态集合

| 状态 (State) | 含义 | JobStatus 常量 (job.go) |
|--------------|------|-------------------------|
| **Queued** | 可被调度（等待 Claim） | StatusPending |
| **Running** | 已被 Claim 且正在执行 DAG | StatusRunning |
| **Waiting** | 在 Wait 节点挂起，等待 signal | StatusWaiting |
| **Parked** | 长时间等待，scheduler 跳过；由 signal 唤醒 | StatusParked |
| **Retrying** | 失败后等待重试（可选显式） | StatusRetrying |
| **Completed** | 成功结束（终态） | StatusCompleted |
| **Failed** | 失败结束（终态） | StatusFailed |
| **Cancelled** | 已取消（终态） | StatusCancelled |

说明：**Leased** 在现有实现中与 Running 合并（Claim 成功后直接写 job_running），故形式化状态中不单独列出；若需区分「已 Claim 未进 runLoop」可扩展为 Leased → Running。

---

## 2. 输入事件集合（状态相关）

仅列出**驱动状态迁移**的事件类型；与 [internal/runtime/jobstore/event.go](../internal/runtime/jobstore/event.go) 一致。

| 事件 (Event) | 常量名 | 说明 |
|--------------|--------|------|
| job_created | JobCreated | Job 创建入队 |
| job_queued | JobQueued | 显式入队（若使用） |
| job_requeued | JobRequeued | 重试时重新入队 |
| job_leased | JobLeased | Worker Claim 成功（可选） |
| job_running | JobRunning | Worker 开始 runLoop |
| job_waiting | JobWaiting | 进入 Wait 节点挂起 |
| wait_completed | WaitCompleted | 收到 signal 后继续 |
| job_completed | JobCompleted | 成功结束 |
| job_failed | JobFailed | 失败结束 |
| job_cancelled | JobCancelled | 取消 |

其他事件（plan_generated、node_started、tool_invocation_* 等）**不改变** Job 的 Queued/Running/Completed 等状态，仅用于 Replay、Trace 与 Verification。

---

## 3. 状态迁移表 (state × event → next_state)

约定：`—` 表示该 (state, event) 组合**非法**或不应出现（写入方应保证只写入合法事件序列）。

| 当前状态 \ 事件 | job_created | job_queued | job_requeued | job_leased | job_running | job_waiting | wait_completed | job_completed | job_failed | job_cancelled |
|-----------------|--------------|------------|--------------|------------|-------------|-------------|----------------|---------------|------------|---------------|
| **(初始)** | Queued | Queued | Queued | — | — | — | — | — | — | — |
| **Queued** | — | — | Queued | Running | Running | — | Queued | — | — | — |
| **Running** | — | — | Queued | — | — | Waiting | Queued | Completed | Failed | Cancelled |
| **Waiting** | — | — | — | — | — | — | Queued | — | — | Cancelled |
| **Parked** | — | — | — | — | — | — | Queued | — | — | Cancelled |
| **Retrying** | — | — | Queued | Running | Running | — | Queued | — | — | — |
| **Completed** | — | — | — | — | — | — | — | — | — | — |
| **Failed** | — | — | Queued | — | — | — | — | — | — | — |
| **Cancelled** | — | — | — | — | — | — | — | — | — | — |

说明：

- **终态**（Completed、Failed、Cancelled）后不再接受任何状态迁移事件；若事件流出现终态后事件，推导时以**最后一条**状态相关事件为准（与 `DeriveStatusFromEvents` 一致）。
- **wait_completed**：在 Running 下通常不写（Wait 节点才写 job_waiting）；在 Waiting/Parked 下写后迁移到 Queued（重新可被 Claim）。
- **job_requeued**：失败重试时写入，目标 Queued；Running 下写 job_requeued 表示 Requeue，下一状态 Queued。

---

## 4. 与 DeriveStatusFromEvents 的对齐

实现 [internal/agent/job/state.go](../internal/agent/job/state.go) 中：

```go
func DeriveStatusFromEvents(events []jobstore.JobEvent) JobStatus
```

- **顺序扫描**：按事件顺序遍历，每遇到上表中的「状态相关事件」即更新内部 `status`。
- **映射关系**：JobCreated/JobQueued/JobRequeued → StatusPending（Queued）；JobLeased/JobRunning → StatusRunning；JobWaiting → StatusWaiting；WaitCompleted → StatusPending；JobCompleted → StatusCompleted；JobFailed → StatusFailed；JobCancelled → StatusCancelled。
- **一致性**：上表与 `DeriveStatusFromEvents` 的语义一致；任何合法事件流经该函数得到的 JobStatus 与按本表逐事件推导的结果相同。
- **Parked / Retrying**：当前实现未单独区分 Parked（可与 Waiting 合并）与 Retrying（可与 Pending 合并）；若需显式状态可在 state.go 与事件类型中扩展。

---

## 5. 不变式 (Invariants)

以下在**合法事件流**下应始终成立：

1. **终态唯一**：若存在 job_completed 或 job_failed 或 job_cancelled，则最后一条状态相关事件必为三者之一；之后不应再出现 job_running、job_leased、job_requeued 等。
2. **Waiting 必有 job_waiting**：状态为 Waiting（或 Parked）时，最后一条状态相关事件必为 job_waiting；解除等待必先有 wait_completed。
3. **Running 必有 job_running 或 job_leased**：状态为 Running 时，最后一条状态相关事件必为 job_running 或 job_leased，且其后无 job_completed/job_failed/job_cancelled。
4. **Blocked 不回收**：若最后一条状态相关事件为 job_waiting，则 Reclaim 不得回收该 Job（见 [runtime-contract.md](runtime-contract.md) §2）；只有 wait_completed 后 Job 才重新变为 Queued 并可被其他 Worker Claim。

---

## 6. 参考

- [job-state-machine.md](job-state-machine.md) — 权威状态模型与事件迁移
- [internal/agent/job/state.go](../internal/agent/job/state.go) — DeriveStatusFromEvents、IsJobBlocked
- [internal/runtime/jobstore/event.go](../internal/runtime/jobstore/event.go) — EventType 常量
- [runtime-contract.md](runtime-contract.md) — 租约、attempt_id、Blocked Job
