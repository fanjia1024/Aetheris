# Deterministic Step Identity

本设计规定 **Step 身份** 的确定性公式，使 Ledger、Replay、retry 始终关联到同一逻辑步，不因 Planner 输出或重规划而错位。参见 [workflow-decision-record.md](workflow-decision-record.md)、[effect-system.md](effect-system.md)。

---

## 目标

- Step 身份 = **可由 (job, decision, position, type) 推导**，而非由 Planner 自由命名。
- 同一 Job、同一决策记录、同一图中位置的节点，在任何 Replay 或 retry 下得到**同一 StepID**，从而：
  - Ledger / idempotency_key 稳定；
  - command_committed、tool_invocation_*、CompletedNodeIDs 等事件与「步」的对应关系唯一。

---

## 公式（1.0）

执行步身份由以下四元组确定性生成：

- `jobID`：Job 标识
- `decisionRecordId`：本步所属决策记录的唯一标识（如 PlanGenerated 事件的 version 或 payload 的 hash）
- `indexInGraph`：该节点在「决策输出图」中的稳定序号（如拓扑序或数组下标）
- `nodeType`：节点类型（tool / llm / workflow / wait）

示例（与实现一致）：

```
StepID = hash(jobID + "\x00" + decisionRecordId + "\x00" + fmt.Sprint(indexInGraph) + "\x00" + nodeType)
```

输出可为 hex 或 base64 截断（如前 16 字符），保证可读且唯一性足够。

---

## 使用位置

- **Ledger / IdempotencyKey**：继续包含 jobID、step 身份、toolName、args；step 身份使用 Deterministic StepID，保证同一逻辑步对应同一 key。
- **事件 payload**：command_committed、tool_invocation_started/finished、NodeFinished 等可写入 `execution_step_id`（或统一用 StepID 作为 node_id 的权威），便于 Replay 按步注入。
- **ReplayContext**：CompletedNodeIDs、CommandResults、CompletedToolInvocations 等按 StepID 索引；BuildFromEvents 解析事件时，若有 `execution_step_id` 则用之，否则回退为 `node_id`（兼容旧事件）。

---

## 与 Planner Node ID 的关系

- **Planner 的 TaskNode.ID**：可保留为展示/ Trace 用（display label）。
- **执行与 Ledger**：以 Deterministic StepID 为准；Runner 在从 PlanGenerated 构建 steps 时为每个节点计算 StepID，并用于后续写入与 Replay。详见 [internal/agent/runtime/executor](internal/agent/runtime/executor) 中 DeterministicStepID 与 SteppableStep 的配合。

---

## 兼容旧事件

- 旧 Job 的 PlanGenerated 与事件流中多为「planner 的 node_id」。Replay 时若事件中**无** `execution_step_id`，则使用 `node_id` 作为 step 身份，与现有行为一致。
- 新写入（1.0 引入 StepID 之后）统一使用 Deterministic StepID，保证新 Job 的 step 身份稳定、可推导。
