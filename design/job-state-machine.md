# Job State Machine — 权威状态模型

Job 的当前状态由**事件流**推导或由「事件流 + metadata 投影」一致定义；状态迁移**仅由事件驱动**。参见 [effect-system.md](effect-system.md)、[event-replay-recovery.md](event-replay-recovery.md)。

## 目标

- **单一事实来源**：事件流为权威；metadata 表的 `status` 为查询用投影，应与事件流推导结果一致。
- **状态迁移仅由事件驱动**：任何状态变更对应「先 Append 一条事件，再更新 metadata status（或由 DeriveStatusFromEvents 推导）」。

## 状态集合

| 状态 | 含义 | 对应事件（最后一条） |
|------|------|----------------------|
| **Queued** | 可被调度（等待 Claim） | job_created 或 job_requeued |
| **Leased** | 已被某 Worker Claim，未进入 runLoop（可选，可与 Running 合并） | job_leased |
| **Running** | 正在执行 DAG | job_running |
| **Waiting** | 在 Wait 节点挂起，等待 signal/continue | job_waiting |
| **Retrying** | 失败后等待重试（可选显式状态；当前可与 Queued 合并） | job_requeued |
| **Completed** | 成功结束 | job_completed |
| **Failed** | 失败结束 | job_failed |
| **Cancelled** | 已取消 | job_cancelled |

与现有 [internal/agent/job/job.go](internal/agent/job/job.go) 的 `JobStatus` 映射：

- `StatusPending` / `StatusQueued`：Queued（可合并，Pending 即 Queued）
- `StatusRunning`：Running（或 Leased + Running 合并）
- `StatusWaiting`：Waiting
- `StatusRetrying`：Retrying（可选）
- `StatusCompleted`：Completed
- `StatusFailed`：Failed
- `StatusCancelled`：Cancelled

## 事件与迁移

| 事件类型 | 迁移目标状态 | 说明 |
|----------|--------------|------|
| job_created | Queued | Job 创建入队 |
| job_requeued | Queued / Retrying | 重试时重新入队 |
| job_leased | Leased | Worker Claim 成功（可选，可与 job_running 合并） |
| job_running | Running | Worker 开始执行 runLoop |
| job_waiting | Waiting | 进入 Wait 节点挂起 |
| wait_completed | Queued 或 Running | 收到 signal 后重新可 Claim 或继续 |
| job_completed | Completed | 成功结束 |
| job_failed | Failed | 失败结束 |
| job_cancelled | Cancelled | 取消 |

规定：**每个迁移由一条事件驱动**；写入事件后应更新 metadata 的 `status`（或由 `DeriveStatusFromEvents` 推导）。

## 谁写事件、谁更新状态

**方案 A（推荐）**：双存储，事件为真源。

- 任何终态或重要迁移（Running、Waiting、Completed、Failed、Cancelled、Requeued）先 **Append 到 jobstore 事件流**。
- 再由「状态投影」在写入事件后更新 metadata 表的 `status`（例如 Worker Claim 成功后写 `job_running` 并 `UpdateStatus(Running)`；Requeue 时写 `job_requeued` 并 `UpdateStatus(Pending)`）。
- API 与 Scheduler 读 metadata 做列表/筛选；语义上事件流为权威，metadata 与之一致。

**方案 B**：完全由事件流推导。每次 GET job 时 `ListEvents`，用 `DeriveStatusFromEvents(events)` 得到 status。适合小规模或强一致性场景。

## 实现

- **DeriveStatusFromEvents**：[internal/agent/job/state.go](internal/agent/job/state.go) — 顺序扫描事件，最后一条状态相关事件决定当前 status。
- **JobStatus 扩展**：`StatusQueued`（或沿用 Pending）、`StatusWaiting`、`StatusRetrying`；与现有 Pending/Running/Completed/Failed/Cancelled 共存。
- **事件类型**：在 [internal/runtime/jobstore/event.go](internal/runtime/jobstore/event.go) 中已有 `JobCompleted`、`JobFailed`、`JobCancelled`；新增 `JobQueued`、`JobLeased`、`JobRunning`、`JobWaiting`、`JobRequeued`、`WaitCompleted`（见 workflow wait 设计）。
- Scheduler/Worker：Claim 成功后写 `job_running`（或 `job_leased`）并 `UpdateStatus(Running)`；Requeue 时写 `job_requeued` 并 `UpdateStatus(Pending)`。
