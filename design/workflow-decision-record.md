# Workflow Decision Record

本设计规定 **Workflow 确定性边界**：执行与 Replay 仅依赖已记录的决策，不依赖 LLM/Planner 的实时输出。参见 [runtime-contract.md](runtime-contract.md) §4、[effect-system.md](effect-system.md)。

---

## 规则

- **No execution path depends on LLM output that is not already in the event log.**
- 执行路径（含 Replay、恢复、首次运行）所依赖的「决策」（如：执行哪几步、顺序、分支）必须已写入事件流；Replay 时**只读**这些记录，**不得**再调用 Planner/LLM 生成结构。

即：**LLM 提议，Runtime 决定** — 提议一旦落盘即成为权威决策，后续执行与重放均以该记录为准。

---

## 当前实现（1.0）

- **唯一决策类型**：规划（Plan）。对应事件为 `plan_generated`。
- **写入时机**：Job 创建时（API 层）在调用 Planner 得到 TaskGraph 后，将 `task_graph`（及 `goal`）写入 `PlanGenerated`，再返回 202。见 [internal/api/http/handler.go](../internal/api/http/handler.go)（SetPlanAtJobCreation）、[internal/app/api/plan_sink.go](../internal/app/api/plan_sink.go)。
- **执行**：Runner 在 RunForJob 中**从不**调用 Planner；若事件流中无 `PlanGenerated` 则直接失败（Job 置为 Failed）。Replay 时从事件流重建 ReplayContext，TaskGraph 来自 `PlanGenerated`。见 [internal/agent/runtime/executor/runner.go](../internal/agent/runtime/executor/runner.go)、[internal/agent/replay/replay.go](../internal/agent/replay/replay.go)。

因此 **PlanGenerated 即当前唯一的 Workflow Decision Record**（决策记录）：携带 `task_graph`（及可选 `plan_hash`），执行与 Replay 仅依赖此记录。

---

## 未来决策类型（2.0 及以后）

以下决策若引入，必须采用同一模式：**先写入决策记录，再让执行依赖该记录**；Replay 只读记录，不调用 LLM。

| 决策种类 | 说明 | 预留事件名（可选） |
|---------|------|-------------------|
| plan | 当前已实现，即 PlanGenerated | `plan_generated` |
| branch | 条件分支选择（如 human_in_the_loop 选 A/B） | `decision_recorded`（kind=branch） |
| retry | 重试/跳过/补偿决策 | `decision_recorded`（kind=retry） |
| re_plan | 中途重新规划 | `decision_recorded`（kind=re_plan） |

约定：凡「执行路径依赖的、由 LLM 或外部输入的决策」，均须先写入事件流（如 `DecisionRecorded(decision_id, kind, payload)`），再执行；Replay 仅读取该记录。

---

## 可选：Plan Hash

为便于校验与调试，可在 `PlanGenerated` payload 中增加 `plan_hash`（如 SHA256(task_graph JSON)）。Replay 与审计时可验证事件流中的 plan 未被篡改。实现见 [internal/app/api/plan_sink.go](../internal/app/api/plan_sink.go)（可选字段）。
