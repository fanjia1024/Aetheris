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
    "policy_rule_ids": ["policy-rule-id"],
    "signal_payload_id": "signal-456",
    "llm_decision": {
      "model": "gpt-4o-2024-11-20",
      "provider": "openai",
      "temperature": 0.7,
      "prompt_hash": "sha256:abc123...",
      "token_count": 1234
    }
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
    { "type": "policy", "id": "rule-id" },
    { "type": "llm_decision", "id": "model:gpt-4o-2024-11-20", "summary": "temp=0.7" }
  ]
}
```

- **谁写入**：Runner 或 Node Sink 在步完成时（reasoning_snapshot）附加；Planner 层若有 RAG/记忆输入可写入 decision_snapshot。Tool 步由 Adapter 将 idempotency_key / invocation_id 通过 payload 传回 Runner；LLM 步由 LLMNodeAdapter 在 result 中附加 `_evidence.llm_decision`（model, temperature, prompt_hash），Runner 写入 reasoning_snapshot.evidence。
- **Phase 1**：reasoning_snapshot 中增加可选 `evidence`；Tool 步填充 `tool_invocation_ids`（idempotency_key）；LLM 步填充 `llm_decision`（model, provider, temperature, prompt_hash, token_count）。**Causal Chain Phase 1**：增加 `input_keys`（本步读取的 state keys）和 `output_keys`（本步写入的 keys），供 Trace 构建 dependency graph。Phase 2：RAG/Memory/Policy 在子系统暴露 ID 后填充对应字段（rag_doc_ids, memory_entry_ids, policy_rule_ids）。
- **Trace**：GET /api/jobs/:id/trace 与 GET node 的 step 或 node 负载中返回 reasoning_snapshot 原始 JSON，其中已含 `evidence`，供 UI 展示 Evidence graph。
- **审计级证据**：与 Causal Debugging 区分：Causal 是工程师调试（reasoning 文本、state diff），Evidence 是法务/审计（可回答"为什么做这个决策？依据哪些输入？使用哪个模型？"）。Evidence Graph 必须记录所有决策输入（RAG 文档、工具调用、LLM 模型版本）以满足合规需求。

### Causal Dependency（因果依赖链）

**Phase 1** — 基于 State Keys 推导因果关系：

reasoning_snapshot 包含 `input_keys`（本步读取的 state keys）和 `output_keys`（本步写入的 keys）。Trace 可据此构建 **dependency graph**：

```
Step A: output_keys=["order_status"]
Step B: input_keys=["order_status"], output_keys=["refund_amount"]
Step C: input_keys=["refund_amount"]
→ Dependency: A → B → C (order_status 传递)
```

**用途**：
- 审计可沿链追溯："Step C 发送退款邮件，因为 Step B 计算退款金额，因为 Step A 返回订单异常"
- Trace UI 可展示 dependency DAG，不只是 timeline
- 合规可证明"决策依据链完整"

**实现**：
- Runner 在构建 reasoning_snapshot 时，调用 `extractStateKeys(stateBefore, stateAfter)` 提取 input/output keys
- Trace 可遍历所有 reasoning_snapshot，按 output_keys → input_keys 构建边（Step A.output_key X → Step B.input_key X）

**Phase 2（未来）** — 显式因果关系：

增加 `caused_by` 字段（参见 plan § 7.1）：

```json
{
  "caused_by": [
    {"type": "tool_result", "id": "inv-1", "key": "order_status", "value_summary": "异常"}
  ]
}
```

由 Planner 或 Runner 显式声明"本步因为 X 而执行"。

## 与 Causal Debugging 的关系

[causal-debugging.md](causal-debugging.md) 定义 ReasoningSnapshot 事件与因果链；本页强调**审计/取证**能力与产品表述：不仅「可追踪」，而且「可审计、可归因、可回答为什么」。Decision Snapshot 为 Accountability 的数据基础，回答「为什么 AI 做出了这个决定」。
