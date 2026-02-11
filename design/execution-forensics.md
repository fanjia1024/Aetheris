# Execution Forensics — 执行取证系统

目标：能回答**「这封错误邮件是谁让 AI 发出去的？在哪一步、哪次 LLM 输出/哪次 Tool 结果决定的？」**。即可审计、可归因、可解释每次决策与副作用。

## 目标

- **决策时间线（Decision timeline）**：事件流即时间线；每步对应 node_started / node_finished、command_emitted / command_committed、tool_invocation_*。
- **推理快照（Reasoning snapshot）**：每步完成后的决策上下文（goal、state_before、state_after、node_type）；可选 llm_request / llm_response 摘要（LLM 节点时）。
- **步因果（Step causality）**：plan → node → tool 的父子关系（执行树）；可反推「哪一步的输入/输出导致下一步」。
- **工具溯源（Tool provenance）**：tool_called / tool_returned、tool_invocation_started/finished、command_committed 记录每次调用的 input/output 与结果。

每个决策点可关联：goal、state_before、state_after、node_id、command_id、以及可选的 llm_request/llm_response（见 [causal-debugging.md](causal-debugging.md) 扩展）。

## 数据基础

| 能力 | 数据来源 |
|------|----------|
| Decision timeline | 事件流（ListEvents） |
| **Decision Snapshot**（Planner 级） | decision_snapshot 事件（PlanGoal 返回后写入）；含 goal、task_graph_summary、plan_hash；回答「为什么选这个 TaskGraph」；GET trace 在 plan 节点与顶层返回 |
| Reasoning snapshot | reasoning_snapshot 事件，按 node_id 挂到 StepNarrative |
| Step causality | 执行树（plan_generated → node_* → tool_*） |
| Tool provenance | tool_invocation_*、command_committed |

## API

- **GET /api/jobs/:id/trace**：返回 timeline、execution_tree、per-step 的 reasoning_snapshot、tool input/output；见 [execution-trace.md](execution-trace.md)、[internal/api/http/trace_narrative.go](../internal/api/http/trace_narrative.go)。
- **GET /api/jobs/:id/replay**：只读 Replay 视图，可扩展 forensics 视图（当前 state、已完成节点、因果链）。

## Decision Snapshot（Planner 可追责）

每个「决策点」除节点级 reasoning_snapshot 外，**Planner 级**决策在 PlanGoal 返回后写入 **decision_snapshot** 事件：goal、task_graph_summary（或完整 TaskGraph）、可选 memory_keys、reasoning 摘要。GET /api/jobs/:id/trace 在 execution_tree 的 plan 节点挂载 decision_snapshot，并在顶层返回 decision_snapshot 字段，供 UI 展示「为什么生成这个计划」。实现： [internal/app/api/plan_sink.go](../internal/app/api/plan_sink.go) 在 AppendPlanGenerated 后追加 DecisionSnapshot； [internal/api/http/trace_tree.go](../internal/api/http/trace_tree.go) BuildExecutionTree 将事件挂到 plan 节点。

## Evidence Graph（可证明的决策依据）

为满足合规与审计（「该决策依据了哪些数据？」），每个决策点可引用**证据**：RAG 文档 ID、工具调用 ID、记忆条目 ID、策略规则 ID。Trace/API 可据此展示「Evidence graph」（决策 → 证据引用）。

### 证据负载结构（Evidence payload schema）

在 **reasoning_snapshot**（以及可选的 decision_snapshot）中增加可选字段 **evidence**：

```json
{
  "evidence": {
    "rag_doc_ids": ["doc-id-1", "chunk-id-2"],
    "tool_invocation_ids": ["idempotency-key-or-invocation-id"],
    "memory_entry_ids": ["mem-id-1"],
    "policy_rule_ids": ["policy-rule-id"]
  }
}
```

或统一使用 **evidence_refs**（二选一或并存均可）：

```json
{
  "evidence_refs": [
    { "type": "tool_invocation", "id": "job:step:tool:hash", "summary": "optional" },
    { "type": "rag_doc", "id": "chunk-id" },
    { "type": "memory", "id": "mem-id" },
    { "type": "policy", "id": "rule-id" }
  ]
}
```

- **谁写入**：Runner 或 Node Sink 在步完成时（reasoning_snapshot）附加；Planner 层若有 RAG/记忆输入可写入 decision_snapshot。Tool 步由 Adapter 将 idempotency_key / invocation_id 通过 payload 传回 Runner，Runner 写入 reasoning_snapshot.evidence。
- **Phase 1**：reasoning_snapshot 中增加可选 `evidence`；Tool 步填充 `tool_invocation_ids`（idempotency_key）。Phase 2：RAG/Memory/Policy 在子系统暴露 ID 后填充对应字段。
- **Trace**：GET /api/jobs/:id/trace 与 GET node 的 step 或 node 负载中返回 reasoning_snapshot 原始 JSON，其中已含 `evidence`，供 UI 展示 Evidence graph。

## 与 Causal Debugging 的关系

[causal-debugging.md](causal-debugging.md) 定义 ReasoningSnapshot 事件与因果链；本页强调**审计/取证**能力与产品表述：不仅「可追踪」，而且「可审计、可归因、可回答为什么」。Decision Snapshot 为 Accountability 的数据基础，回答「为什么 AI 做出了这个决定」。
