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
| Mailbox（通用消息） | 设计 | 未来：POST `/api/jobs/:id/message` 写入 `agent_message`，Runner 在收件节点消费 |

## 事件形态（契约）

- **job_waiting**：payload 含 correlation_key、wait_type、node_id；只有相同 correlation_key 的 signal 可解除。
- **wait_completed**：payload 含 node_id、可选 payload；Replay 将对应节点视为完成并注入 payload。
- **job_interrupted**（可选）：未来若区分「暂停」与「取消」，可写入 job_interrupted，Runner 在步间检查后挂起。

## 实现位置

- Signal： [internal/api/http/handler.go](../internal/api/http/handler.go) `JobSignal`、[internal/agent/job/state.go](../internal/agent/job/state.go) `IsJobBlocked`。
- Query：GET job、GET replay、GET trace；Replay 响应可增加 `current_state`（由 BuildFromEvents 推导的 completed_node_ids、cursor_node、payload_results 摘要）。
- Interrupt/Stop：`JobStop`、`RequestCancel`、Worker 轮询 CancelRequestedAt。

## 分阶段实现

1. **Phase 1（当前）**：Query 强化 — replay 响应增加 current_state 字段；文档化 Signal/Query/Interrupt/Resume 与现有 API 的对应关系。
2. **Phase 2**：Mailbox — 定义 `agent_message` 事件与 POST `/api/jobs/:id/message`；Runner 在 Wait 或专用「收件」节点消费。
3. **Phase 3**：Interrupt 细化为「暂停」与「取消」；Resume 对已暂停 Job 的显式恢复 API。
