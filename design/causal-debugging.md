# Causal Debugging — 推理快照与因果链

支持回答：「这个 agent 在 2 小时前为什么做了这个决定？」即从某个 Tool 执行反推到：哪个计划步骤、哪次 LLM 输出、哪段上下文导致。参见 [trace-event-schema-v0.9.md](trace-event-schema-v0.9.md)。

## 目标

- 从任意 `node_finished` 或 `tool_invocation_finished` 可沿事件流找到对应 **ReasoningSnapshot**，得到「当时看到的上下文与计划」。
- Trace API 返回 per-step 的 `reasoning_snapshot`，供 UI 展示该步的完整决策上下文。

## ReasoningSnapshot 事件

**事件类型**：`reasoning_snapshot`（[internal/runtime/jobstore/event.go](internal/runtime/jobstore/event.go)）

**写入时机**：Runner 在**节点完成**后（AppendNodeFinished、AppendStateCheckpointed 之后）写一条 ReasoningSnapshot。

**Payload 结构**：

| 字段 | 类型 | 说明 |
|------|------|------|
| node_id | string | 节点 ID（step 标识） |
| step_id | string | 同 node_id |
| node_type | string | llm / tool / workflow / wait |
| goal | string | Job 目标 |
| duration_ms | int64 | 本步耗时 |
| timestamp | string | RFC3339 |
| state_before | object | 本步前 payload.Results（可选） |
| state_after | object | 本步后 payload.Results |
| tool_name | string | 若为 tool 步（可选） |

因果链：Step → ReasoningSnapshot → goal / state_before / state_after / node_type → 可反推上游计划与输入。

## 实现位置

- **写入**：[internal/agent/runtime/executor/runner.go](internal/agent/runtime/executor/runner.go) — runLoop 内每步完成后调用 `NodeEventSink.AppendReasoningSnapshot`。
- **Sink**：[internal/app/api/node_sink.go](internal/app/api/node_sink.go) — `AppendReasoningSnapshot` 写入 `reasoning_snapshot` 事件。
- **Trace**：[internal/api/http/trace_narrative.go](internal/api/http/trace_narrative.go) — `BuildNarrative` 解析 `reasoning_snapshot`，按 `node_id` 挂到对应 `StepNarrative.ReasoningSnapshot`。

## 扩展（可选）

- **LLM 节点**：Runner 在 node_type=llm 时已写入 snapshot 的 `llm_request`（goal）、`llm_response`（该步结果，即 payload.Results[node_id]）；供执行取证与因果链反推。
- 若需按 step 存 plan_slice，可在 payload 中增加 `plan_slice`（当前计划片段 JSON）。
