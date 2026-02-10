# Checkpoint Barrier：可迁移恢复点

## 定义

**Checkpoint Barrier** 表示一个**状态冻结点**：该点之前所有 `command_committed` / `NodeFinished` 已持久化到事件存储；Replay 从该点恢复时仅注入结果、从下一节点执行；任意 Worker 均可从该点接管（可迁移执行）。

与「仅停止执行」的区别：

| 当前语义 | Checkpoint Barrier 语义 |
|----------|-------------------------|
| 运行停了 | 可迁移执行 |
| 单机恢复 | 任意 Worker 恢复 |
| 依赖进程内状态 | 依赖事件流 + Cursor |

## 与事件持久化顺序的关系

1. **事件优先**：每步执行完成后，**先** Append 本步所有事件（含 `tool_invocation_finished`、`command_committed`、`node_finished`），**再** Save checkpoint 与 UpdateCursor。这样崩溃后 Replay 以事件流为权威来源时，能先看到本步完成，再根据 Cursor 从下一节点继续。
2. **NodeFinished 先于 UpdateCursor**：Runner 在 runLoop 内顺序为：`AppendNodeFinished` → `Save(checkpoint)` → `UpdateCursor`。保证「Replay 可见的完成状态」与「Job 的恢复游标」一致。
3. **无中间崩溃窗口**：不在「事件未写完就 UpdateCursor」或「只写 Store 不写事件」的情况下推进游标，避免 Replay 与 Cursor 不一致。见 [execution-state-machine.md](execution-state-machine.md) 的 Tool completion visibility 与事件顺序。

## 实现要点

- Runner：见 [runner.go](internal/agent/runtime/executor/runner.go) runLoop 内每步末尾的 AppendNodeFinished → Save checkpoint → UpdateCursor。
- Adapter：Tool/LLM 节点先写 completion 事件再写 Store，见 [node_adapter.go](internal/agent/runtime/executor/node_adapter.go)。
- JobStore.UpdateCursor 持久化 Cursor；多 Worker 通过 Claim 取得 Job 后，若无 Cursor 则从事件流 Replay 构建上下文，若有 Cursor 则从 Checkpoint 恢复。两种路径均从「barrier 后的下一节点」继续，实现可迁移恢复。

## 参考

- [event-replay-recovery.md](event-replay-recovery.md)
- [execution-state-machine.md](execution-state-machine.md)
