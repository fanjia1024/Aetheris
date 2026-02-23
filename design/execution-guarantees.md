# Aetheris Execution Guarantees — 可信执行运行时契约

本文档是 Aetheris 作为 **Trusted Execution Runtime** 的正式语义契约：在给定条件下，Runtime 对以下保证的成立性做出明确承诺。企业接入与合规审计可据此判断「是否成立」及「成立条件」。参见 [effect-system.md](effect-system.md)、[scheduler-correctness.md](scheduler-correctness.md)、[runtime-contract.md](runtime-contract.md)。

---

## 保证一览

| 保证 | 是否成立 | 条件 |
|------|----------|------|
| **Step 至少执行一次**（在重试下） | 是 | Scheduler 对可重试失败进行 Requeue；Job 级租约与 Reclaim 保证最终由某 Worker 推进。需配置 RetryMax/Backoff；Reclaim 以 event store 租约为准，不回收 Blocked Job。 |
| **Step 至多执行一次**（同一逻辑步不重复副作用） | 是 | 配置 **InvocationLedger**（及共享 ToolInvocationStore）时，同一 idempotency_key 仅允许一次 Commit；Replay 从事件流/Ledger 注入结果，不再次执行。配置 **Effect Store** 时采用两步提交（先 PutEffect 再 Append），崩溃恢复从 Effect Store catch-up，不重执行。见 [effect-system.md](effect-system.md)、[scheduler-correctness.md](scheduler-correctness.md)。 |
| **Signal/Message 不丢失**（至少一次投递） | 是 | 写入 `wait_completed` 并 UpdateStatus(Pending) 后，Job 进入可 Claim 队列；重复 signal 同 correlation_key 做幂等处理。交付语义：**at-least-once**；重复发送相同 correlation_key 为 no-op 或安全。见 [agent-process-model.md](agent-process-model.md)、[runtime-contract.md](runtime-contract.md) § 二。 |
| **Replay 不改变行为**（不重新调用 LLM/Tool） | 是 | Runner 在 Replay 路径下仅依据事件流中的 `CompletedCommandIDs`/`CommandResults`、`CompletedToolInvocations` 注入结果，不调用 `step.Run` 对已提交命令的节点；LLM/Tool 节点 Replay 时**禁止**真实调用。见 [effect-system.md](effect-system.md)、[1.0-runtime-semantics.md](1.0-runtime-semantics.md)。 |
| **Replay 绝不调用 LLM**（不可重现性完全避免） | 是 | 配置 **Effect Store** 时，LLM 调用先 PutEffect 再 Append `command_committed`；Replay 时 Runner 从 `CompletedCommandIDs` 注入不调用 `step.Run`，Adapter 从 EffectStore 注入不调用 `Generate`。**未配置 Effect Store 时不保证**（开发模式可接受；生产环境必须配置）。见 [effect-system.md](effect-system.md) § LLM Effect Capture。 |
| **崩溃后不重复副作用** | 是 | 配置 **Effect Store** 时：Execute 成功后先 PutEffect，再 Append `command_committed`；Replay 时若事件流无 command_committed 但 Effect Store 有该步 effect，则 catch-up 写回事件并注入结果，不执行 Tool/LLM。Tool 路径另有 Activity Log Barrier（已 started 无 finished 禁止再执行，仅恢复或失败）。见 [effect-system.md](effect-system.md) § Effect Store 与强 Replay、[execution-state-machine.md](execution-state-machine.md)。 |

---

## 条件摘要

- **持久化 Job（跨进程/多 Worker）**：必须配置 **JobStore**（如 PostgreSQL）、**Event Store**（含租约/attempt_id 校验）、**InvocationLedger**（及共享 ToolInvocationStore）。可选 **Effect Store** 以达成「崩溃后不重复副作用」与两步提交 catch-up。
- **单进程/兼容路径**：无 Ledger 时 Tool 步不提供跨 Worker 的 at-most-once 保证；仅适合开发或单 Worker。
- **Blocked Job**：Reclaim 不得回收「最后事件为 job_waiting」的 Job；只有 wait_completed 后 Job 才重新变为 Pending。见 [runtime-contract.md](runtime-contract.md) § 二、§ 三。
- **无全局事务**：Ledger 写入与事件提交**不在同一 DB 事务**中；保证依赖**写入顺序**（先 Append 再 Ledger.Commit）与 **Effect Store catch-up**。可证明语义（crash 窗口、幂等键、顺序与补偿）见 [provable-semantics-table.md](provable-semantics-table.md)。

---

## 参考

- [effect-system.md](effect-system.md) — 效应类型、Replay 协议、Effect Store 与两步提交
- [scheduler-correctness.md](scheduler-correctness.md) — 租约、两步提交、Step 状态
- [runtime-contract.md](runtime-contract.md) — 重放边界、Blocking、租约、PlanGenerated、attempt_id
- [1.0-runtime-semantics.md](1.0-runtime-semantics.md) — Execution Proof Chain、Ledger 决策、World Safety
- [provable-semantics-table.md](provable-semantics-table.md) — 可证明语义表（crash 窗口、幂等键、Ledger/Append 顺序）
- [agent-process-model.md](agent-process-model.md) — Signal、Mailbox、External Event 交付
