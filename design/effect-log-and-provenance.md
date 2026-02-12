# Effect Log 与 Provenance — 显式结构与可验证执行

本文档在 [effect-system.md](effect-system.md) 与 [internal/agent/runtime/executor/effect_store.go](../internal/agent/runtime/executor/effect_store.go) 基础上，约定 **Effect Log 显式视图**、**LLM Decision Log**、**Tool Effect Provenance**、**Rollback-Safe Retry** 语义，以及 **Verifiable Agent Execution** 的验收条件。不改变现有 Replay/Catch-up 协议，仅扩展数据模型与审计契约。

---

## 1. 与现有 Effect System 的关系

- **权威来源不变**：Replay 的权威数据源仍为**事件流**（PlanGenerated、CommandCommitted、ToolInvocationFinished、NodeFinished）；Effect Store 用于两步提交与 catch-up，见 [effect-system.md](effect-system.md)。
- **本文档扩展**：为审计、溯源与 Trace 2.0 提供显式 Effect Log 视图、LLM/Tool 的决策与溯源字段、以及 rollback 语义的明确约定。

---

## 2. Effect Log 显式视图

### 2.1 逻辑视图

**Effect Log** = 事件流中 `command_committed` 与 `tool_invocation_finished` 的有序序列（按 job_id + 事件顺序）。逻辑上可视为「该 Job 的副作用序列」，供审计与测试断言。实现上可由 `ListEvents(jobID)` 过滤得到，无需单独存储。

### 2.2 物理视图（可选）

为便于审计与 provenance 查询，可物化一张表或由事件流异步推导：

| 字段 | 类型 | 说明 |
|------|------|------|
| job_id | string | 所属 Job |
| step_index | int | 步序号（拓扑序） |
| command_id | string | 命令/节点 ID |
| kind | string | tool \| llm \| http \| human \| time \| random |
| input_hash | string | 可选，input 的哈希，用于变更检测 |
| output_hash | string | 可选，output 的哈希 |
| created_at | timestamp | 对应事件时间 |

- **物化方式**：在 Append `command_committed` / `tool_invocation_finished` 时同步双写此表；或后台任务按事件流异步物化。
- **Replay**：不依赖此表；Replay 仍以事件流 + Effect Store 为准。此表仅用于查询与 Trace/Provenance UI。

---

## 3. LLM Decision Log

### 3.1 目标

每条 LLM 调用除记录 prompt/response 外，可关联「决策快照」（为何做此决策），便于因果调试与可追责。

### 3.2 扩展约定

- **EffectRecord（现有）**：Kind=llm 时已含 Input（prompt）、Output（response）、Metadata（model、temperature 等）。扩展 Metadata 或新字段：
  - **prompt_hash**（可选）：prompt 的哈希，用于去重与变更检测。
  - **response_hash**（可选）：response 的哈希。
  - **decision_snapshot_id**（可选）：关联事件流中 `decision_snapshot` 事件的 ID 或 payload 引用；表示「该 LLM 输出对应的 Planner 决策上下文」。
- **事件流**：已有 [DecisionSnapshot](internal/runtime/jobstore/event.go)（design/execution-forensics.md）；PlanGoal 后写入，含 goal、memory 摘要、reasoning 摘要、TaskGraph。LLM 节点执行时若有对应「本步所属的决策」，可将 decision_snapshot 的 ID 或 job_id+version 写入 EffectRecord.Metadata["decision_snapshot_id"]。

### 3.3 Replay 行为

- 与 [effect-system.md](effect-system.md) 一致：Replay 时**禁止**再次调用 LLM；仅从 CommandCommitted / EffectStore 注入结果。LLM Decision Log 仅用于审计与 Trace，不参与执行路径。

---

## 4. Tool Effect Provenance

### 4.1 目标

每条 Tool 调用记录「对外部世界的可追溯标识」（如外部系统返回的 ticket_id、payment_id），便于审计与问题排查。

### 4.2 扩展约定

- **EffectRecord（现有）**：Kind=tool 时已含 JobID、IdempotencyKey、CommandID、Input、Output、Error。扩展 Metadata 或新字段：
  - **external_id**（可选）：外部系统返回的 ID（如工单 ID、支付流水号），由 Tool 实现返回并写入 EffectRecord.Metadata["external_id"]。
  - **tool_name**：已有；可同时在 Metadata 中冗余便于查询。
- **tool_invocations 表**（现有）：若存在 [schema.sql](../internal/runtime/jobstore/schema.sql) 中的 tool_invocations，可增加列 `external_id TEXT`；Tool Adapter 在 Execute 成功后从 Tool 结果中提取并写入。

### 4.3 接口约定

- Tool 实现可返回结构化结果，其中包含可选字段 `Provenance.ExternalID`；Adapter 在 PutEffect 时写入 EffectRecord.Metadata["external_id"]，并可选写入 tool_invocations.external_id。
- Replay/Catch-up 不依赖 provenance；仅用于 Trace 与审计展示。

---

## 5. Rollback-Safe Retry 语义

### 5.1 约定

- **仅当 Effect Store 中无该 step 的 effect 时才执行**：与现有协议一致；Runner 在 Replay 时若事件流无 command_committed，则检查 EffectStore；若已有 effect 则 catch-up 写回事件流并注入结果，**不**再次执行 Tool/LLM。
- **Rollback 语义**：本系统不定义「回滚已提交 effect」的操作。已提交的 command_committed / tool_invocation_finished **不可撤销**。所谓 rollback-safe 指：
  - **重试**：对未提交步骤可重试；对已提交步骤不重试（仅注入）。
  - **补偿**：若业务需要「撤销」某步效果，由应用层实现补偿逻辑（如调用补偿 API），并可选写入 `step_compensated` 等事件；Runtime 不自动回滚 Effect Store 或事件流。

### 5.2 验收

- 崩溃发生在「Execute 成功、PutEffect 成功、Append 前」：新 Worker Replay 时从 EffectStore catch-up，写回 command_committed，**不**再次执行 Tool/LLM。
- 崩溃发生在「Execute 成功、PutEffect 前」：无 effect 记录，Replay 会再次执行该步；此时依赖 Tool 幂等与 StepIdempotencyKeyForExternal 传递，保证外部至多一次。

---

## 6. Verifiable Agent Execution 验收条件

满足以下条件时，可称一次 Agent 执行**可验证**：

1. **可审计**：事件流完整记录 PlanGenerated、NodeFinished、CommandCommitted、ToolInvocationFinished；可选物化 Effect Log 表可查询每步的 kind、input_hash、output_hash。
2. **可回放**：同一事件流 Replay 得到相同 CompletedNodeIDs、CommandResults 与最终状态；且 Replay 过程中**不**触发真实 LLM/Tool 调用。
3. **无重复副作用**：每个逻辑步（command_id / idempotency_key）至多一次真实执行；已提交 effect 仅通过注入推进。
4. **Provenance 可追溯**（可选）：LLM 步可关联 decision_snapshot；Tool 步可记录 external_id；Trace 2.0 可展示 decision tree 与 tool dependency graph，见 [trace-2.0-cognition.md](trace-2.0-cognition.md)。

---

## 7. 参考

- [effect-system.md](effect-system.md) — Effect Log 逻辑、Replay 协议、Effect Store 两步提交、Catch-up
- [execution-state-machine.md](execution-state-machine.md) — 状态机与写入顺序
- [internal/agent/runtime/executor/effect_store.go](../internal/agent/runtime/executor/effect_store.go) — EffectRecord、EffectStore 接口
- [trace-2.0-cognition.md](trace-2.0-cognition.md) — Cognition Trace 与 decision tree
