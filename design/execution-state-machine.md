# 执行状态机与确定性 Replay

Runner 由**事件流驱动状态机推进**，保证重放时**不重跑已提交步骤**（deterministic reconstruction），与 Durable JobStore、Event Replay 一起构成 1.0 的「可托管生产」可靠性闭环。1.0 起引入**命令级（command-level）**事件，实现**副作用安全**：已 `command_committed` 的命令永不重放。

## 目标

- **重放日志 = 重建执行位置**，而不是「重放日志 ≈ 重新执行代码」。
- 每步执行前检查「该 step 是否已有 NodeFinished」或「该 command 是否已 command_committed」；有则跳过（或仅注入结果并推进游标），无才执行并**先**写入 command_committed（Adapter）/ NodeFinished（Runner），再 checkpoint。
- **副作用安全**：节点内副作用（Tool/LLM/Workflow）执行成功后**立即**写 `command_committed`，再写 `node_finished`；crash 后 Replay 仅依据事件流推进游标，已提交命令不再执行。

## 状态

由事件流推导：

- **CompletedNodeIDs**：所有在事件流中出现过 `NodeFinished` 的 `node_id` 集合。Replay 时由 [internal/agent/replay/replay.go](internal/agent/replay/replay.go) 的 `BuildFromEvents` 顺序扫描，对每条 `NodeFinished` 将 `node_id` 加入 `ReplayContext.CompletedNodeIDs`。
- **CompletedCommandIDs**：所有在事件流中出现过 `command_committed` 的 `command_id` 集合（单命令节点下 command_id = node_id）。已提交命令永不重放。
- **CommandResults**：`command_id` → 该命令的 result JSON，Replay 时用于注入 payload，不重新执行节点。
- **PayloadResults**：最后一条 `NodeFinished` 的 `payload_results`（累积的 payload.Results），供恢复时反序列化进 DAG payload。
- 从 Checkpoint 恢复时，等价地构造 **completedSet**：按拓扑序 steps，从 `steps[0]` 到 `cp.CursorNode`（含）的 node_id 集合。见 [internal/agent/runtime/executor/runner.go](internal/agent/runtime/executor/runner.go) 中 Checkpoint 分支。

## 事件顺序（副作用安全）

对会产生副作用的节点（Tool/LLM/Workflow），必须满足：

`command_emitted`（可选）→ 执行 → **`command_committed`**（持久化）→ `node_finished` → checkpoint。

Adapter 在 Execute 成功后**立即**写 `command_committed`，再由 Runner 写 `node_finished`。

## 推进规则

1. **startIndex**：第一个不在 CompletedNodeIDs 中的 step 索引（Replay 路径）；或 Checkpoint 路径下「CursorNode 的下一索引」。
2. **runLoop**：对 `i := startIndex; i < len(steps); i++`：
   - 若该 step 的 `command_id` 已在 **CompletedCommandIDs** 中：不调用 `step.Run`，从 **CommandResults** 注入结果到 payload，必要时写 `NodeFinished`，Save checkpoint、UpdateCursor，`continue`。
   - 若 `steps[i].NodeID` 已在 completedSet 中：**不调用** `step.Run`，**不写入** `NodeStarted`/`NodeFinished`，`continue`。
   - 否则：`AppendNodeStarted` → `step.Run`（内部：command_emitted → Execute → command_committed）→ `AppendNodeFinished` → Save checkpoint、UpdateCursor。
3. NodeFinished 的写入**先于** checkpoint/UpdateCursor；command_committed 的写入**先于** NodeFinished，确保重放时「已提交命令」与「已完成节点」语义一致，不重复副作用。

代码位置：Runner [internal/agent/runtime/executor/runner.go](internal/agent/runtime/executor/runner.go) 中 replayCtx、completedSet 的构建与 runLoop 内命令级/节点级跳过逻辑；Adapter [internal/agent/runtime/executor/node_adapter.go](internal/agent/runtime/executor/node_adapter.go) 中 command_emitted/command_committed 的写入。

## 与 1.0 的关系

- **Durable JobStore**：事件不丢、多 Worker Claim。
- **Event Replay**：无 Checkpoint 时从事件流恢复 TaskGraph、CompletedNodeIDs、CompletedCommandIDs、CommandResults、PayloadResults。见 [event-replay-recovery.md](event-replay-recovery.md)。
- **执行状态机**：Runner 按「已提交命令」与「已完成节点」推进，命令级 commit 保证副作用安全，支撑生产级 1.0 Stable。

三者 together 构成「事件驱动 + 操作系统保证 + 副作用安全」，支撑 1.0 的崩溃恢复、长任务与可审计回放。
