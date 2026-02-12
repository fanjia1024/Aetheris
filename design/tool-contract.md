# Tool Contract — 幂等、重试与补偿（2.0）

本文档规定 Tool 调用的对外契约：ToolInvocationID、RetryPolicy、Compensation API，与 [effect-system.md](effect-system.md) Idempotency 契约一致。

## ToolInvocationID

每次 Tool 调用的唯一标识，在 API 与 Trace 中统一暴露，便于审计与下游去重。

- **语义**：与 `idempotency_key` 或事件中的 `invocation_id` 对应；单步单 Tool 场景下二者可一致。
- **暴露位置**：`tool_invocation_started` / `tool_invocation_finished` 事件的 `invocation_id`、`idempotency_key`；Trace 的 `reasoning_snapshot.evidence.tool_invocation_ids`。
- **下游使用**：Tool 实现方可将 `executor.ExecutionKeyFromContext(ctx)` 或 `StepIdempotencyKeyForExternal(ctx, jobID, stepID)` 作为对外 idempotency key 传给第三方 API。

## RetryPolicy

Tool 或节点级重试策略，失败时由 Runner/Adapter 按策略重试并写入事件（如重试计数），供可观测性使用。

- **配置**：`RetryPolicy` 含 `MaxRetries`（最大重试次数）、`Backoff`（退避时间或策略）、可选 `RetryableErrors`（可重试错误类型/消息匹配）。
- **位置**：Tool 注册时可附带 RetryPolicy；或节点配置（task_graph 节点 config）中指定。
- **行为**：执行失败且错误属于 RetryableErrors（或默认视为可重试）时，在 MaxRetries 内按 Backoff 重试；超过后写入 node_finished(result_type=retryable_failure 或 permanent_failure)。重试次数可写入事件或 trace 供 Retry Visualization。

类型定义见 `internal/agent/runtime/executor/retry.go`（可选实现）；当前 Runner 已有 StepTimeout 与失败分类，RetryPolicy 可在此基础上扩展为「同步内重试」或由 Scheduler Requeue 体现。

## Compensation API

补偿语义：在何种条件下触发、如何与 `StepCompensated` 事件关联。

- **触发条件**：节点标记为 `compensatable_failure` 或显式请求补偿时，Runtime 可调用已注册的 Compensation 回调，执行「回滚」或补偿操作（如取消预订、退款）。
- **事件**：补偿执行后写入 `step_compensated` 事件；Replay 时已补偿步骤视为终态，不重执行原逻辑。
- **契约**：补偿仅针对**已提交效应**（即已写 command_committed / tool_invocation_finished 的步骤）；补偿本身应幂等。Compensation 不破坏 exactly-once 进度：原 step 已提交则只补偿一次，由事件流与补偿记录去重。
- **API**：Tool 注册时可选 `RegisterCompensation(nodeID string, fn CompensationFunc)`；Runner 在检测到 compensatable_failure 或补偿请求时调用对应 fn，再写入 step_compensated。

实现上可为 Phase 2：先文档化契约，后续在 Runner/Adapter 中接入 RetryPolicy 与 Compensation 回调。

## 不可幂等工具与 Compensation 契约

凡会产生**不可逆外部副作用**的 Tool（如支付、发邮件、预订），**必须**在应用层注册 Compensation（通过 `Runner.SetCompensationRegistry` 或等价方式）；在 step 返回 `compensatable_failure` 时由 Runtime 调用对应补偿回调并写入 `step_compensated`。未注册补偿的不可幂等 Tool 在失败时仅能标记为永久失败，无法由 Runtime 自动回滚。

## Retry 与 InvocationID 契约

所有 Tool 重试与执行均绑定**同一** InvocationID / idempotency_key；Trace 与事件中统一暴露 ToolInvocationID（见 `executor.ToolInvocationID(invocationID, idempotencyKey)`），便于审计与下游去重。当前实现已满足：同一 step 内 RetryPolicy 重试使用同一 idempotency_key，仅第一次真实执行，后续重试由 Ledger/Replay 约束。

## 与 at-most-once 的关系

- **Tool 执行**：同一 idempotency_key 至多一次真实执行；Replay 与 Ledger/Effect Store 保证不重复。
- **RetryPolicy**：重试仍受 Ledger 约束；同一 step 重试时使用同一 idempotency_key，仅第一次会真实执行，后续重试由 Replay 注入或失败终止。
- **Compensation**：仅对已提交的 step 执行补偿；补偿调用次数由 Runtime 保证一次（或幂等）。

## 参考

- [effect-system.md](effect-system.md) — Idempotency 契约、StepIdempotencyKeyForExternal
- [execution-state-machine.md](execution-state-machine.md) — command_committed、NodeFinished、StepCommitted
- [internal/agent/runtime/executor/types.go](internal/agent/runtime/executor/types.go) — ExecutionKeyFromContext、StepIdempotencyKeyForExternal
