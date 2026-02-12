# Trace 2.0 — Cognition Trace 数据模型与消费

本文档定义 Aetheris 2.0 的 **Cognition Trace**（认知追踪）：在 [trace-event-schema-v0.9.md](trace-event-schema-v0.9.md) 的 workflow/step 与语义事件基础上，增加 **memory read/write**、**plan evolution** 等事件类型，约定从事件流构建 **decision tree**、**tool dependency graph** 与 **reasoning step timeline** 的消费模型，以及 Debug UI 的推荐展示结构。所有新事件**不参与 Replay**，仅用于 Trace 叙事与可观测性。

---

## 1. 目标

- **Reasoning step timeline**：按 step 聚合 thought、decision、tool 的时序视图，便于理解「Agent 如何一步步思考」。
- **Decision tree**：由 decision_made、tool_selected、node_finished 推导的分支与选择结构。
- **Plan evolution**：PlanGenerated 与 DecisionSnapshot 序列，展示目标与计划的演变。
- **Tool dependency graph**：DAG 节点依赖 + 每步 tool 调用关系，便于理解工具链。
- **Memory read/write**：显式记录 Agent 对 Working/Long-Term/Episodic 记忆的读写，便于调试「为何做出某决策」。

---

## 2. 新增事件类型（设计约定）

以下事件类型可在实现时加入 [internal/runtime/jobstore/event.go](../internal/runtime/jobstore/event.go)；此处仅约定语义与 payload，不修改代码。

### 2.1 memory_read

**语义**：Agent 在某步读取了某类记忆（working / long_term / episodic）。

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| job_id | string | 否 | 所属 Job |
| node_id | string | 否 | 所属节点 |
| step_index | int | 否 | 1-based |
| memory_type | string | 是 | working \| long_term \| episodic |
| key_or_scope | string | 否 | 键或范围（如 namespace:key） |
| summary | string | 否 | 简要描述（如「读取最近 5 条 episodic」） |

**Written by**：Planner/Memory 适配器或 Runner 在 Recall/Load 时发出（可选实现）。

### 2.2 memory_write

**语义**：Agent 在某步写入了某类记忆。

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| job_id | string | 否 | 所属 Job |
| node_id | string | 否 | 所属节点 |
| step_index | int | 否 | 1-based |
| memory_type | string | 是 | working \| long_term \| episodic |
| key_or_scope | string | 否 | 键或范围 |
| summary | string | 否 | 简要描述 |

**Written by**：同上，在 Store/Append 时发出。

### 2.3 plan_evolution

**语义**：计划版本或目标的演变（可选；若已有 DecisionSnapshot 可复用其序列，不强制新事件）。

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| plan_version | int | 否 | 计划版本号 |
| diff_summary | string | 否 | 与上一版的差异摘要 |

若实现选择不新增此类型，Trace 2.0 消费端可直接用事件流中 **plan_generated** 与 **decision_snapshot** 的序列作为 plan evolution 数据源。

---

## 3. 与 trace-event-schema-v0.9 的兼容

- 现有事件类型与扩展 payload **不变**：node_started、node_finished、state_checkpointed、agent_thought_recorded、decision_made、tool_selected、tool_result_summarized、recovery_*、step_compensated、reasoning_snapshot、decision_snapshot 继续按 v0.9 定义。
- **新增**：memory_read、memory_write、（可选）plan_evolution；均为**加性**，旧 Job 无这些事件时 Trace 2.0 仅不展示 memory 时间线与 plan evolution 详情。
- **Replay**：仍仅依赖 plan_generated、node_finished、command_committed、tool_invocation_finished；所有 Cognition Trace 专用事件不参与 Replay。

---

## 4. Trace 2.0 消费模型

消费端（Trace 叙事 pipeline 或 Debug UI）从 `ListEvents(jobID)` 或等价接口读取事件流，按以下维度聚合与展示。

### 4.1 Reasoning Step Timeline

- **数据源**：node_started、node_finished、agent_thought_recorded、decision_made、tool_selected、tool_result_summarized，按 node_id / step_index 分组。
- **结构**：时间轴按 step 顺序；每个 step 下挂载：
  - 该 step 的 thought（agent_thought_recorded）
  - 该 step 的 decision（decision_made）
  - 该 step 选用的 tool（tool_selected）及结果摘要（tool_result_summarized）
- **展示**：水平时间条 + 每步展开的「思考 → 决策 → 工具 → 结果」卡片。

### 4.2 Decision Tree

- **数据源**：decision_made、tool_selected、node_finished（含 state：ok/failed/retryable）。
- **推导规则**：
  - 每个 node 对应树上一个节点；node 的 decision_made/tool_selected 为该节点的「选择」标签。
  - 子节点由 DAG 的边（plan_generated 的 TaskGraph）确定；若某节点有多个后继，则形成分支。
  - node_finished 的 state 可标注为叶节点（ok=成功，failed=失败，retryable=可重试）。
- **展示**：树状或层级图，节点显示 decision/tool 摘要，边显示执行顺序。

### 4.3 Plan Evolution

- **数据源**：plan_generated（首个计划）、decision_snapshot（若有多次规划或 replan）。
- **结构**：按事件顺序的「计划快照」序列；每个快照含 goal、TaskGraph 或摘要、reasoning 摘要（见 execution-forensics.md）。
- **展示**：时间线或版本列表，可对比两版 diff（若事件含 diff_summary 或由消费端计算）。

### 4.4 Tool Dependency Graph

- **数据源**：plan_generated 的 TaskGraph（节点与边）、node_finished 的 node_id 顺序、tool_selected / tool_result_summarized 的 node_id。
- **结构**：DAG 节点为 step；边为「某节点依赖前序节点输出」；仅标注「包含 tool 调用的节点」为 tool 节点，其余为 LLM/逻辑节点。
- **展示**：有向图，节点可点击查看该步的 tool 名称、输入输出摘要、external_id（见 [effect-log-and-provenance.md](effect-log-and-provenance.md)）。

### 4.5 Memory Read/Write Timeline

- **数据源**：memory_read、memory_write 事件（若实现发出）。
- **结构**：按时间顺序的读写记录；每条含 memory_type、key_or_scope、summary。
- **展示**：独立时间线或与 Reasoning Step Timeline 并排，标注「某 step 读了/写了哪类记忆」。

---

## 5. Cognition Trace 聚合定义

**Cognition Trace** = 对单次 Job 事件流的以下聚合视图的统称：

| 维度 | 数据源 | 用途 |
|------|--------|------|
| Reasoning step timeline | node_*、agent_thought_recorded、decision_made、tool_selected、tool_result_summarized | 逐步理解推理过程 |
| Decision tree | decision_made、tool_selected、node_finished、TaskGraph | 理解分支与选择 |
| Plan evolution | plan_generated、decision_snapshot | 理解目标与计划演变 |
| Tool dependency graph | TaskGraph、tool_selected、tool_result_summarized | 理解工具链与依赖 |
| Memory read/write | memory_read、memory_write | 理解记忆如何影响决策 |

存储不强制新表；可由事件流 + 可选物化视图或 API 层实时聚合。Trace UI 请求「某 job 的 cognition trace」时，后端 ListEvents(jobID) 后按上述规则构建并返回 JSON 或前端直接消费事件流自建。

---

## 6. Debug UI 推荐展示结构

- **顶部**：Job 元数据（job_id、agent_id、goal、status、cursor）。
- **Tab 1 — Timeline**：Reasoning step timeline（水平条 + 每步展开 thought/decision/tool/result）。
- **Tab 2 — Decision Tree**：树状或层级图，可折叠/展开节点。
- **Tab 3 — Plan**：Plan evolution 列表 + 当前 TaskGraph 可视化（DAG）。
- **Tab 4 — Tools**：Tool dependency graph + 每 tool 节点详情（输入/输出摘要、external_id、provenance）。
- **Tab 5 — Memory**（若实现 memory_read/write）：Memory 读写时间线，按 memory_type 过滤。
- **Tab 6 — Events**：原始事件流列表（与现有 Trace 一致），便于排查与导出。

---

## 7. 参考

- [trace-event-schema-v0.9.md](trace-event-schema-v0.9.md) — 语义事件类型与 payload、Replay 无关性
- [effect-log-and-provenance.md](effect-log-and-provenance.md) — Tool provenance、LLM decision log、Effect Log
- [execution-forensics.md](execution-forensics.md) — DecisionSnapshot、proof chain
- [durable-memory-layer.md](durable-memory-layer.md) — Working/Long-Term/Episodic 记忆类型
