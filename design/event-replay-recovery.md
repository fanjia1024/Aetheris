# Runner 从 Event Replay 重建执行状态

无 Checkpoint 时（例如 Worker 刚 Claim 到某 Job、或重启后再次 Claim），Runner 不重新 Plan，而是从**事件流**恢复执行上下文并从上一完成节点继续。这是「事件驱动 + 事件不丢」后的恢复路径，与 Durable JobStore 一起构成 1.0 的「操作系统保证」，也是 workflow engine 与普通 agent 框架的分水岭。

## 目的

- 避免 Worker 接管或重启后**重复执行**已完成的 Plan 与节点。
- 以**事件流为权威来源**：TaskGraph、已完成节点游标、中间结果均从 `ListEvents(jobID)` 推导，不依赖进程内状态。

## 数据流

```
ListEvents(jobID)
  → ReplayContextBuilder.BuildFromEvents(ctx, jobID)
  → 解析 PlanGenerated（TaskGraph）、每条 NodeFinished 加入 CompletedNodeIDs、最后一条得到 CursorNode + PayloadResults
  → ReplayContext{ TaskGraphState, CursorNode, PayloadResults, CompletedNodeIDs, PayloadResultsByNode }
```

- **TaskGraphState**：来自事件流中 `plan_generated` 的 payload.`task_graph`。
- **CompletedNodeIDs**：所有出现过 `node_finished` 的 `node_id` 集合，供 Runner 确定性重放（步前检查、已完成的 step 不重跑）。见 [execution-state-machine.md](execution-state-machine.md)。
- **CursorNode**：来自事件流中**最后一次** `node_finished` 的 payload.`node_id`（兼容 Trace/旧逻辑）。
- **PayloadResults**：来自同一条 `node_finished` 的 payload.`payload_results`（累积状态），供 DAG 下一节点使用。

实现见 [internal/agent/replay/replay.go](internal/agent/replay/replay.go)：`BuildFromEvents` 顺序扫描事件，遇到 `PlanGenerated` 更新 TaskGraphState，遇到 `NodeFinished` 将 node_id 加入 CompletedNodeIDs 并更新 CursorNode、PayloadResults、PayloadResultsByNode。

## Runner 行为

Runner 在「无 Cursor（无 Checkpoint）」时**优先**尝试从事件流重建上下文，再决定是否调用 Planner：

1. 调用 `replayBuilder.BuildFromEvents(ctx, j.ID)`。
2. 若返回有效 `ReplayContext` 且能反序列化出 `TaskGraph`：
   - 使用该 TaskGraph 做 `CompileSteppable`，得到 steps。
   - 用 `ReplayContext.CompletedNodeIDs` 计算 **startIndex** = 第一个不在 CompletedNodeIDs 中的 step 索引。
   - 将 `PayloadResults` 反序列化进当前 DAG payload.Results。
   - **直接进入 runLoop**，传入 completedSet = CompletedNodeIDs；runLoop 内每步若 node_id 已在 completedSet 则跳过，否则执行并先 Append NodeFinished 再 checkpoint（见 [execution-state-machine.md](execution-state-machine.md)）。**不调用 Planner**。
3. 若 Replay 失败或无 PlanGenerated（例如旧 Job）：走原有路径，调用 Planner 生成 Plan 并写入 `PlanGenerated` 事件。

代码位置：[internal/agent/runtime/executor/runner.go](internal/agent/runtime/executor/runner.go)，「无 Cursor 时优先尝试从事件流重建」分支（约 191–215 行），成功后 `goto runLoop`。

## 与 1.0 的关系

- **Durable JobStore**（Postgres 事件流 + 租约）保证事件不丢、多 Worker Claim。
- **Event Replay 恢复**保证：重启或新 Worker 接管后，Runner 能从事件流恢复「已 Plan、已执行到哪一步、中间结果是什么」，并从中断处继续，而不是重新规划或重复执行。
- 二者 together 构成「事件驱动 + 操作系统保证」，支撑 1.0 的崩溃恢复、长任务与可审计回放。
