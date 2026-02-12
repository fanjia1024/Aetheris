# Effect System — 确定性边界与 Replay 协议

Agent execution = **Deterministic State Machine + Recorded Effects**。本文档规定哪些行为属于“效应”（必须记录、Replay 时禁止重执行），以及 Replay 的严格规则。参见 [execution-state-machine.md](execution-state-machine.md) 与 [event-replay-recovery.md](event-replay-recovery.md)。

## Effect Log（副作用日志）— 第一公民

**Effect Log** 是 Replay 与审计的**唯一数据源**。所有会产生副作用的操作必须先写入事件流中的效应事件，Replay 时**仅读取**这些事件注入结果，**禁止**再次执行 LLM、Tool 或外部 IO。

### 记录内容

| 效应类型 | 记录内容 | 事件类型 |
|----------|----------|----------|
| **Tool** | input + output | `tool_invocation_started`、`tool_invocation_finished`、`command_committed`（node 级 result） |
| **LLM** | prompt（input）+ response（result） | `command_emitted`（含 input）、`command_committed`（含 result） |
| **External** | 调用参数与结果 | `command_committed` 或 `state_changed` |

### 契约

- Replay 的**唯一**数据源是事件流中的效应事件（PlanGenerated、CommandCommitted、ToolInvocationFinished、NodeFinished 等）。
- 任何「已提交」效应（即事件流中已存在对应 command_committed / tool_invocation_finished）**不得**在 Replay 时重新执行；Runner 仅注入 CommandResults / CompletedToolInvocations 并推进游标。
- **100% 可重现**语义：已提交步骤仅通过事件注入推进；唯一可能真实执行的是「下一个未提交步骤」，且执行后立即写 command_committed / tool_invocation_finished，再推进。

### Effect Log 逻辑视图（可选）

由事件流推导的「副作用序列」可视为逻辑上的 **effect_log**：按事件顺序的 command_committed、tool_invocation_finished 等，供审计与测试断言使用。实现上不要求单独存储，由 `ListEvents(jobID)` 过滤即可。

## 目标

- **形式化定义**：执行由确定性状态机 + 已记录效应组成，而不是“一堆步骤 + 存点日志”。
- **Replay 协议**：Replay 时**禁止**真实调用 LLM、Tool、外部 IO；只读 Effect Log（事件流中的效应事件）注入结果。

## Effect 类型与 Replay 行为

| 类型 | 含义 | Replay 时允许 | 对应事件/机制 |
|------|------|----------------|----------------|
| **Pure** | 无副作用的计算（纯函数、无 IO） | 可重新执行 | 无独立事件；节点 result_type=pure 时 Replay 可重算（当前实现中 llm/tool/workflow 均按 SideEffect 处理，Pure 为约定） |
| **LLM** | 调用大模型得到输出 | 禁止调用；只读已记录结果 | `command_committed`（含 result）；Replay 从 CommandResults 注入 |
| **Tool** | 调用工具（HTTP/DB/外部系统） | 禁止调用；只读已记录结果 | `tool_invocation_finished`、`command_committed`；Replay 从 CompletedToolInvocations/CommandResults 注入 |
| **External IO** | 其他外部调用（HTTP/DB 等） | 禁止调用；只读已记录结果 | `command_committed` 或 `state_changed`；Replay 从 CommandResults/StateChangesByStep 注入 |
| **Time / Random** | 依赖当前时间或随机数 | 禁止重算；只读已记录值 | 未来：`TimerFired`、`RandomRecorded`；Replay 时从事件注入 context |

## Replay 规则（协议）

以下为**系统约束**，所有 Runner/Adapter 实现必须遵守。

### 禁止在 Replay 时执行

- **禁止**调用 LLM（任何 Generate/Complete 类 API）。
- **禁止**调用 Tool（Tools.Execute）。
- **禁止**发起外部 HTTP、DB、文件 IO 等会改变外部世界的操作。

### Replay 时只读

- **PlanGenerated**：TaskGraph 为权威，Replay 不重新 Plan。
- **CommandCommitted**：command_id → result，Replay 仅注入到 payload，不执行节点逻辑。
- **ToolInvocationFinished**（outcome=success）：idempotency_key → result，Replay 仅注入，不执行 tool。
- **NodeFinished**：CompletedNodeIDs、PayloadResults，用于恢复游标与累积状态。
- （若实现）**TimerFired**、**RandomRecorded**：Replay 时仅从事件注入时间/种子到 context。

### 副作用屏障（Activity Log Barrier）

对 **Tool 调用**：须先将本次调用的声明（`tool_invocation_started`）成功追加到事件流后，才允许执行 `Tools.Execute`。Replay 时若事件流中存在该调用的 started 且无对应 finished，**不得再次执行**，仅可：从 Ledger/tool_invocations 恢复已提交结果并写回 `tool_invocation_finished`（catch-up），或确定性地失败该 step（invocation in flight or lost）。这样可消除「Execute 成功、持久化 Finished 前崩溃」导致的二次执行风险。

### 写入顺序（与 execution-state-machine 一致）

对会产生副作用的节点，写入顺序必须为：

1. **先** 写 `tool_invocation_started`（声明），再执行 Tool；执行完成后 **立即** 写 `command_committed`（或 `tool_invocation_finished`）；
2. 再写 `node_finished`；
3. 再写 **`step_committed`**（2.0 显式 Step Commit Barrier；payload 含 node_id、step_id、command_id、可选 idempotency_key）；
4. 再 checkpoint / UpdateCursor。

这样 Replay 以事件流为权威时，已提交命令永不重放；且「已 started 无 finished」时禁止再执行，仅恢复或失败。

### Effect Store 与强 Replay（两步提交）

当配置 **Effect Store**（副作用存储）时，Replay 升级为 **Execution Replay**：同一 logical step 的副作用只执行一次，重放时只读已记录的效应；崩溃发生在「Execute 成功、尚未 Append command_committed」时，新 Worker 可从 Effect Store **catch-up**，写回事件流而不重执行 Tool/LLM。

- **两步提交**  
  - **Phase 1**：Execute 成功后**先**将 effect（input + output + 元数据）写入 Effect Store（`PutEffect`）。  
  - **Phase 2**：Effect Store 写入成功后，再 Append `command_committed` / `tool_invocation_finished` 与 NodeFinished、Checkpoint。  
  约定：仅当事件流中出现 `command_committed`（或等价「step 已提交」）后，该 step 才视为完成；Replay 以「事件流 + Effect Store」为事实来源。

- **恢复语义（catch-up）**  
  Replay 时若发现：事件流中该 step **无** `command_committed`，但 Effect Store 中**已有**该 step 的 effect（按 job_id + idempotency_key 或 job_id + command_id 查）→ 视为「上一 Worker 已执行并持久化效应、未完成 Phase 2」。执行 **catch-up**：向事件流追加 `command_committed` / `tool_invocation_finished`（payload 从 Effect Store 读取），并可选更新 Ledger/InvocationStore，**不**调用 Tool/LLM。

- **效应类型与必填**  
  Effect Store 记录类型至少包括：**Tool**（input + output + error）、**LLM**（prompt + response + model/temperature 等元数据）、**Time/Random**（占位）、**Human**（审批/人工输入）。接口见 `internal/agent/runtime/executor/effect_store.go`；内存实现见 `effect_store_mem.go`，多 Worker 时需 PG 等共享存储。

## 与现有事件类型映射

| EffectKind（逻辑） | 存储为 EventType | 说明 |
|-------------------|------------------|------|
| LLMResponseRecorded | command_committed | node_id, command_id, result |
| ToolResultRecorded | tool_invocation_finished + command_committed | idempotency_key, result；command_committed 存 node 级 result |
| ExternalCallRecorded | command_committed 或 state_changed | 视是否需资源审计 |
| TimerScheduled / TimerFired | （未来）timer_fired | 1.5 后期或 2.0 |
| RetryDecision | （可选）显式事件或由 Requeue 体现 | 当前由 Scheduler 逻辑体现 |

## 实现位置

- **Replay 构建**：[internal/agent/replay/replay.go](internal/agent/replay/replay.go) — `BuildFromEvents` 解析 PlanGenerated、NodeFinished、CommandCommitted、ToolInvocationFinished。
- **Replay 决策**：[internal/agent/replay/sandbox/policy.go](internal/agent/replay/sandbox/policy.go) — `ReplayPolicy.Decide` 对 llm/tool/workflow 返回 SideEffect，有记录则 Inject。
- **Runner**：[internal/agent/runtime/executor/runner.go](internal/agent/runtime/executor/runner.go) — 无 Cursor 时优先从事件流重建；runLoop 内 command_id ∈ CompletedCommandIDs 时只注入、不调用 step.Run。
- **Adapter**：[internal/agent/runtime/executor/node_adapter.go](internal/agent/runtime/executor/node_adapter.go) — Ledger 路径下仅 `AllowExecute` 才执行 tool；Replay 注入时不调用 tool。**Activity Log Barrier**：若 idempotency_key ∈ PendingToolInvocations（事件流已 started 无 finished），禁止执行，仅 Recover 后 catch-up 写 Finished 或返回永久失败。**Effect Store**：当 `EffectStore != nil` 时，Execute 成功后先 `PutEffect` 再 Append（两步提交）；runNode 内若事件流无 command_committed 但 EffectStore 有该 idempotency_key 的 effect，则 catch-up 写回事件并注入结果，不执行 Tool。

## Idempotency 契约（Idempotency Contract）

- **要求**：所有 Tool 调用在设计上必须**幂等**；同一逻辑操作多次执行（如重试、Replay 后误执行）应产生相同效果或可安全去重。
- **Runtime 保证**：同一 ExecutionKey 最多一次真实执行；Replay 时仅注入已记录结果，不再次调用 Tool。
- **对外 API**：Tool 执行时可通过 `executor.ExecutionKeyFromContext(ctx)` 取得稳定执行键（等价于 job_id + step_id + idempotency_key），供实现方做幂等或传给下游作 idempotency key。Runner 在调用 `Tools.Execute` 前将当前调用的 idempotency_key 注入 context。
- **ToolInvocationID / RetryPolicy / Compensation**：见 [tool-contract.md](tool-contract.md)。ToolInvocationID 与 invocation_id / idempotency_key 对应，在 Trace 与 API 中统一暴露；RetryPolicy 与 Compensation API 为可选扩展。

## Step Idempotency for External World（外部副作用唯一）

对会产生**外部副作用**的 Tool（发邮件、支付、webhook、调用第三方 API），必须将**步级幂等键**传给下游，否则崩溃重试可能导致「同一逻辑步执行两次」（如客户收到两封账单）。Runtime 提供规范格式的步级幂等键，Tool 实现方**必须**将其传给 email/payment/webhook/API。

- **键格式**：`aetheris:<job_id>:<step_id>:<attempt_id>`；`attempt_id` 由 Worker Claim 时注入，空时用 `"0"`。
- **API**：`executor.StepIdempotencyKeyForExternal(ctx, jobID, stepID string) string`；Tool 在执行发邮件/支付/webhook 前取得该键并作为下游的 idempotency key 或请求头传递。
- **保证**：在配置 Effect Store + Ledger + 两步提交的前提下，Runtime 保证同一逻辑步至多一次真实执行；Tool 将 StepIdempotencyKeyForExternal 传给下游后，下游可据此去重，实现「崩溃后重试不重复副作用」。

## LLM 非确定性边界

- **Plan**：来自事件流中的 `PlanGenerated`；Replay 不重新调用 Planner。
- **每节点最多执行一次**：首次执行后立即写入 command_committed（或 tool_invocation_finished），Replay 时仅注入该结果，**不会**再次调用 LLM 或 Tool。因此已 command_committed 的 LLM 步在 Replay 时不会产生非确定性。
- **LLM Effect Capture**：当配置 Effect Store 时，LLMNodeAdapter 在 Generate 成功后写入完整 effect（prompt、response、可选 model/temperature 至 Metadata）；Replay 时 Runner 已跳过已提交命令；Adapter 层若 EffectStore 已有该 command_id 记录则直接注入不调用 LLM（defence in depth）。**Replay 期间绝不调用 LLM**；完整 effect 存于 Effect Store 供审计与确定性重放。
- **强保证**：配置 Effect Store 时，Aetheris **保证** Replay **绝不**调用 LLM API。两层防护：(1) Runner 层检查 `CompletedCommandIDs`，已提交命令不调用 `step.Run`；(2) Adapter 层检查 EffectStore，已有 effect 直接注入不调用 `Generate`。**生产环境必须配置 Effect Store**，否则无法保证 LLM 不可重现性（开发模式可选）。
- **未来**：若支持单步内多次 LLM 调用，每次调用须有独立 command_id 并写入 Effect Log，Replay 时全部从事件注入。

## 扩展阅读

- [effect-log-and-provenance.md](effect-log-and-provenance.md) — Effect Log 显式视图、LLM Decision Log、Tool Provenance、Rollback-Safe 语义、Verifiable Agent 验收条件。

## 断言与测试

- **Replay 确定性**：同一 job 事件流 Replay 两次（BuildFromEvents 或 RunForJob 从事件恢复），得到的 CompletedNodeIDs、CommandResults 注入与最终状态一致。
- **Replay 不触发副作用**：Replay 路径下（事件流已含所有节点的 command_committed / NodeFinished）**不触发真实 LLM/Tool 调用**；单测用 mock 验证调用次数为 0。见 `internal/agent/runtime/executor` 下 Replay 相关测试。
