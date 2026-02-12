# Replay Sandbox：确定性重放边界

## 目的

Replay 的目标是 **execution reconstruction**（从事件流重建执行状态并从中断处继续），而不是 **debug playback**（仅回放日志、结果可能因 LLM/外部 API/时间不同而不同）。为此需要显式定义「可重放边界」：哪些操作在 Replay 时允许重新执行，哪些禁止执行、仅从 event 注入结果。

## 三类操作

| 类型 | 行为 | 典型节点 |
|------|------|----------|
| **Deterministic** | Replay 时允许重新执行（纯计算、无副作用） | 纯函数、本地计算 |
| **SideEffect** | Replay 时禁止执行，仅从 event 注入结果 | tool、llm、workflow |
| **External** | Replay 时禁止执行，仅从 event 恢复 | 同上，强调依赖外部世界 |

当前实现中，Planner 产出的节点类型为 `llm` / `tool` / `workflow`，均视为 **SideEffect**：Replay 时若事件流中已有 `command_committed` 或 `tool_invocation_finished`，则注入结果并跳过执行；若无记录则**禁止执行并失败**（避免二次副作用）。

## 与现有事件的关系

- **CompletedCommandIDs / CommandResults**：来自 `command_committed` 事件，Replay 时用于注入 LLM/tool/workflow 节点结果。
- **CompletedToolInvocations**：来自 `tool_invocation_finished`（outcome=success），Replay 时 Tool 节点在 Adapter 内通过 `CompletedToolInvocationsFromContext` 跳过真实调用并注入结果。
- **ReplayPolicy**：在 [internal/agent/replay/sandbox](internal/agent/replay/sandbox) 中定义，Runner 的 runLoop **先**查策略再决定执行 / 跳过并注入 / 禁止执行并失败。为 nil 时保留原有「已 command_committed 则注入」逻辑。

## Execution reconstruction vs debug playback

- **Reconstruction**：事件流为权威来源；Replay 后从某一步继续执行时，该步之前的所有副作用结果均来自事件，不重新调用 LLM/外部 API，保证「重放结果 = 原执行结果」。
- **Debug playback**：仅按时间线展示事件、便于排查问题，不保证重放时再执行会得到相同结果。Trace UI 等属于此类。

Replay Sandbox 通过 ReplayPolicy 与 Runner 的「禁止无记录时执行 SideEffect」保证 reconstruction 语义。

## Replay Safety（2.0）

- **Replay 模式下禁止未记录的非确定性操作**：Step 内不得直接使用 `time.Now()`、`rand`、`uuid.New()`、`http.Get` 等；须通过 **Recorded Effects API**（`runtime.Now(ctx)`、`runtime.UUID(ctx)`、`runtime.HTTP(ctx)`）由 Runtime 记录，Replay 时仅从事件注入。参见 [internal/agent/runtime/effects](internal/agent/runtime/effects)。
- **可选严格模式**：当启用 determinism.ReplayGuard 且 StrictReplay 为 true 时，Replay 路径下若检测到禁止操作可 **panic**（job_id/step_id 便于排查）。实现见 [internal/agent/determinism](internal/agent/determinism)。

## Recorded Effects 契约

- **Clock**：`effects.Now(ctx)` → 仅从 EventRecorder/Runtime 取时间；Replay 时从 `timer_fired` 事件注入。
- **UUID**：`effects.UUID(ctx)` → 由 Runtime 生成并记录；Replay 时从 `uuid_recorded` 事件注入。
- **HTTP**：`effects.HTTP(ctx, effectID, doRequest)` → 经 Runtime 记录请求/响应；Replay 时从 `http_recorded` 事件注入。

## 参考

- [event-replay-recovery.md](event-replay-recovery.md) — 事件流恢复与 command_committed
- [execution-state-machine.md](execution-state-machine.md) — 命令级 commit 与 NodeFinished 顺序
- [internal/agent/replay/sandbox/policy.go](internal/agent/replay/sandbox/policy.go) — OperationKind、ReplayPolicy、DefaultPolicy
