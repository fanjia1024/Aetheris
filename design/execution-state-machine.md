# 执行状态机与确定性 Replay

Runner 由**事件流驱动状态机推进**，保证重放时**不重跑已提交步骤**（deterministic reconstruction），与 Durable JobStore、Event Replay 一起构成 1.0 的「可托管生产」可靠性闭环。

## 目标

- **重放日志 = 重建执行位置**，而不是「重放日志 ≈ 重新执行代码」。
- 每步执行前检查「该 step 是否已有 NodeFinished」；有则跳过，无才执行并**先**写入 NodeFinished（step-level commit），再 checkpoint。

## 状态

由事件流推导：

- **CompletedNodeIDs**：所有在事件流中出现过 `NodeFinished` 的 `node_id` 集合。Replay 时由 [internal/agent/replay/replay.go](internal/agent/replay/replay.go) 的 `BuildFromEvents` 顺序扫描，对每条 `NodeFinished` 将 `node_id` 加入 `ReplayContext.CompletedNodeIDs`。
- **PayloadResults**：最后一条 `NodeFinished` 的 `payload_results`（累积的 payload.Results），供恢复时反序列化进 DAG payload。
- 从 Checkpoint 恢复时，等价地构造 **completedSet**：按拓扑序 steps，从 `steps[0]` 到 `cp.CursorNode`（含）的 node_id 集合。见 [internal/agent/runtime/executor/runner.go](internal/agent/runtime/executor/runner.go) 中 Checkpoint 分支。

## 推进规则

1. **startIndex**：第一个不在 CompletedNodeIDs 中的 step 索引（Replay 路径）；或 Checkpoint 路径下「CursorNode 的下一索引」。
2. **runLoop**：对 `i := startIndex; i < len(steps); i++`：
   - 若 `steps[i].NodeID` 已在 completedSet 中：**不调用** `step.Run`，**不写入** `NodeStarted`/`NodeFinished`，`continue`。
   - 否则：`AppendNodeStarted` → `step.Run` → 成功后**立即** `AppendNodeFinished`（step-level commit）→ Save checkpoint、UpdateCursor。
3. NodeFinished 的写入**先于** checkpoint/UpdateCursor，确保重放时「已完成集合」包含本步，避免重复执行。

代码位置：Runner [internal/agent/runtime/executor/runner.go](internal/agent/runtime/executor/runner.go) 中 `completedSet` 的构建（Checkpoint 与 Replay 分支）、runLoop 内对 `completedSet` 的步前检查与跳过逻辑。

## 与 1.0 的关系

- **Durable JobStore**：事件不丢、多 Worker Claim。
- **Event Replay**：无 Checkpoint 时从事件流恢复 TaskGraph、CompletedNodeIDs、PayloadResults。见 [event-replay-recovery.md](event-replay-recovery.md)。
- **执行状态机**：Runner 按「已完成集合」推进，步前检查、步后 commit，保证确定性重建，不重复副作用。

三者 together 构成「事件驱动 + 操作系统保证」，支撑 1.0 的崩溃恢复、长任务与可审计回放。
