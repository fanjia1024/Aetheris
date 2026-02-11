# Agent 进程模型（Agent as Process）

将 Agent 执行视为**持久进程**而非一次性任务脚本：支持 Mailbox、Signal、Query、Interrupt、Resume 等进程语义，便于人类审批、外部触发、SLA 中断与优先级控制。

## 目标

- **Mailbox**：事件流即 Agent 的「收件箱」；除系统事件外，可写入外部消息（如 `agent_message`），Runner 在合适语义点（如 Wait 节点）消费。
- **Signal**：向挂起或运行中的 Job 发送信号；当前已实现 Wait 节点 + `job_waiting` / `wait_completed`，通过 POST `/api/jobs/:id/signal` 携带 correlation_key 解除阻塞。
- **Query**：只读当前执行状态（不推进执行）；GET `/api/jobs/:id` 与 GET `/api/jobs/:id/replay` 提供元数据与事件流；可扩展为返回「当前 state」（已完成节点、当前节点、payload 摘要），即 **Query** 语义。
- **Interrupt**：请求暂停执行；当前通过 POST `/api/jobs/:id/stop`（RequestCancel）由 Worker 轮询后取消 runCtx，Job 进入 CANCELLED；未来可细化为「挂起」与「取消」两种。
- **Resume**：对已暂停/等待的 Job 继续；对 Wait 节点通过 JobSignal 写入 `wait_completed` 后 Job 回到 Pending 被认领继续，即 **Resume**。

## 当前实现

| 能力 | 实现 | API / 事件 |
|------|------|------------|
| Signal（Wait） | 已实现 | POST `/api/jobs/:id/signal`，correlation_key 匹配 job_waiting |
| Query（只读状态） | 已实现 | GET `/api/jobs/:id`（元数据）、GET `/api/jobs/:id/replay`（事件流）；可扩展 replay 响应含 current_state |
| Interrupt | 已实现（取消） | POST `/api/jobs/:id/stop` → RequestCancel → Worker 取消 runCtx |
| Resume（Wait 后） | 已实现 | 同上 Signal；wait_completed 后 Job 重新入队 |
| Mailbox（通用消息） | 已实现 | POST `/api/jobs/:id/message` 写入 `agent_message`；Wait 节点 wait_kind=message、config.channel 为信道名，匹配即写 wait_completed 并重新入队 |

## 事件形态（契约）

- **job_waiting**：payload 含 correlation_key、wait_type、node_id；signal 需 correlation_key 一致解除；wait_type=message 时 correlation_key 可为 channel，由 POST message 的 channel 匹配解除。
- **agent_message**：payload 含 message_id、channel、correlation_key、payload；POST `/api/jobs/:id/message` 写入；若 Job 处于 Waiting 且当前 job_waiting 的 wait_type=message 且 channel/correlation_key 匹配，则追加 wait_completed 并将 Job 置为 Pending。
- **wait_completed**：payload 含 node_id、可选 payload；Replay 将对应节点视为完成并注入 payload。
- **job_interrupted**（可选）：未来若区分「暂停」与「取消」，可写入 job_interrupted，Runner 在步间检查后挂起。

## 实现位置

- Signal： [internal/api/http/handler.go](../internal/api/http/handler.go) `JobSignal`、[internal/agent/job/state.go](../internal/agent/job/state.go) `IsJobBlocked`。
- Query：GET job、GET replay、GET trace；Replay 响应可增加 `current_state`（由 BuildFromEvents 推导的 completed_node_ids、cursor_node、payload_results 摘要）。
- Interrupt/Stop：`JobStop`、`RequestCancel`、Worker 轮询 CancelRequestedAt。

## Wakeup Index（事件驱动唤醒）

当 Job 因 signal/message 变为 Pending 时，若仅靠 Scheduler 轮询，会有延迟与无效 polling。**WakeupQueue** 提供「mailbox → scheduler 的触发」：API 在写入 wait_completed 并 UpdateStatus(Pending) 后调用 `NotifyReady(ctx, jobID)`；Worker 在无 job 时调用 `Receive(ctx, pollInterval)` 替代固定 sleep，从而在收到 NotifyReady 后立即继续 Claim，实现事件驱动唤醒。

- **接口**：[internal/agent/job/wakeup.go](../internal/agent/job/wakeup.go) — `NotifyReady`、`Receive`；内存实现 `WakeupQueueMem`（带缓冲 channel）。
- **API**：Handler 可选 `SetWakeupQueue(q)`；JobSignal/JobMessage 在 UpdateStatus(Pending) 后若 `wakeupQueue != nil` 则 `NotifyReady(ctx, jobID)`。
- **Worker**：AgentJobRunner 可选 `SetWakeupQueue(q)`；无 job 时以 `Receive(ctx, pollInterval)` 替代 `time.After(pollInterval)`。
- **单进程部署**：创建同一 `WakeupQueueMem` 实例并注入 Handler 与 AgentJobRunner，则 signal/message 后 Worker 可立即唤醒；多进程时需 Redis/PG 等分布式队列实现。

## Continuation Semantics（等待后仍是同一思维）

Agent 在 Wait 节点挂起后由 signal 唤醒继续执行，**恢复时必须保证思维连续性**：不是"新执行"，而是"同一 continuation"。

### Resumption Context

`job_waiting` 事件 payload 包含 **resumption_context**：

```json
{
  "node_id": "wait1",
  "wait_type": "human",
  "correlation_key": "approval-123",
  "resumption_context": {
    "payload_results": {...},        // 等待前的 payload.Results snapshot
    "plan_decision_id": "sha256:...", // 绑定到具体 Plan（避免 replan）
    "cursor_node": "wait1"
  }
}
```

**写入时机**：Runner 在遇到 Wait 节点时，在写 `job_waiting` 前将当前 `payload.Results`、`plan_decision_id`、`cursor_node` 序列化为 resumption_context。

**恢复时**：
- Signal 写入 `wait_completed`，payload 包含 signal 传入的数据
- Runner 恢复时从 resumption_context 读取等待前的完整 state
- 下一步执行的 input = resumption_context.payload_results + signal.payload（合并）

**保证**：
- 等待 3 天后恢复，state 来自 wait 点 snapshot，不是"旧 checkpoint"或"空 state"
- Agent 继续的是"原先的执行路径"（same plan, same reasoning state），不是"重新规划"

### 实现位置

- **事件 payload**：[internal/runtime/jobstore/event.go](../internal/runtime/jobstore/event.go) `JobWaitingPayload.ResumptionContext`
- **写入**：[internal/agent/runtime/executor/runner.go](../internal/agent/runtime/executor/runner.go) Wait 节点路径，构造 resumption_context 并传给 `AppendJobWaiting`
- **恢复**：[internal/agent/replay/replay.go](../internal/agent/replay/replay.go) `BuildFromEvents` 在遇到 `wait_completed` 时，可从对应 `job_waiting` 的 resumption_context 恢复 payload_results（Phase 2 增强）

---

## Process State Semantics（WAITING vs PARKED）

Agent 等待外部事件时有两种状态语义，区分"短暂等待"与"长时间等待"：

### WAITING（短暂等待）

- **语义**：等待时间 <1 分钟（或配置的短超时）；Scheduler 仍扫描该 Job（防止 signal 丢失时通过 poll 兜底）。
- **适用场景**：等待短暂 API 响应、等待快速审批（预期几分钟内完成）。
- **Scheduler 行为**：ClaimNextPending 时会扫描到 StatusWaiting Job（但当前实现为不 Claim blocked job，依赖 signal 唤醒）。
- **实现**：Wait 节点 config 未设置 `park: true` 时，写 job_waiting 后 UpdateStatus(StatusWaiting)。

### PARKED（长时间等待）

- **语义**：等待时间 >1 分钟（如 3 天人工审批）；Scheduler **跳过**该 Job，不扫描、不占资源；**仅由 signal 通过 WakeupQueue 唤醒**。
- **适用场景**：人工审批（可能 3 天）、定时任务（等待特定时间）、异步回调（等待第三方系统）。
- **Scheduler 行为**：ClaimNextPending 和 Reclaim 都跳过 StatusParked Job；signal 唤醒后置为 StatusPending，才会被 Scheduler 认领。
- **实现**：Wait 节点 config 设置 `park: true` 时，写 job_waiting 后 UpdateStatus(StatusParked)；signal 后 UpdateStatus(StatusPending) + WakeupQueue.NotifyReady。

**关键差异**：

| 维度 | WAITING | PARKED |
|------|---------|--------|
| 预期等待时间 | <1 分钟 | >1 分钟（小时/天） |
| Scheduler 扫描 | 是（兜底） | 否（跳过） |
| 资源占用 | 低（但仍在扫描队列） | 极低（不在扫描队列） |
| 唤醒方式 | signal 或 poll（兜底） | 仅 signal（必须配置 WakeupQueue） |
| 典型场景 | 等待 API 响应 | 人工审批、定时任务 |

**状态迁移**：
```
Running → Wait(park=false) → StatusWaiting → Signal → StatusPending → Running
Running → Wait(park=true) → StatusParked → Signal → StatusPending → Running
```

**Blocking Semantics**：Scheduler 不得 reclaim StatusWaiting 或 StatusParked Job（见 [runtime-contract.md](runtime-contract.md) §2）；只有收到匹配 signal 并写入 `wait_completed` 后，Job 才重新变为可 Claim。

---

## 分阶段实现

1. **Phase 1（当前）**：Query 强化 — replay 响应增加 current_state 字段；文档化 Signal/Query/Interrupt/Resume 与现有 API 的对应关系。
2. **Phase 2（已实现）**：Mailbox — `agent_message` 事件与 POST `/api/jobs/:id/message`；Wait 节点 config 设 wait_kind=message、channel=信道名，Runner 写 job_waiting 时 correlation_key=channel，消息 API 匹配后写 wait_completed 并重新入队。见 [internal/runtime/jobstore/event.go](../internal/runtime/jobstore/event.go) AgentMessage、[internal/api/http/handler.go](../internal/api/http/handler.go) JobMessage。
3. **Phase 3（已实现）**：Process State — StatusParked 状态；Wait 节点 config.park 控制短暂 vs 长时间等待；Scheduler 跳过 StatusParked Job；signal 唤醒通过 WakeupQueue。
4. **Phase 4**：Interrupt 细化为「暂停」与「取消」；Resume 对已暂停 Job 的显式恢复 API。
