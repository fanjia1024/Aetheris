# Verification Mode — 执行验证模式

Verification Mode 允许对任意已完成或已失败的 Job 做**离线验证**，输出可审计的证明摘要，用于合规与「可证明执行正确性」的演示。与 [1.0-runtime-semantics.md](1.0-runtime-semantics.md) 的 Execution Proof Chain 衔接。

---

## 目标

- **何时用**：Job 执行结束后（completed / failed / cancelled），调用 `GET /api/jobs/:id/verify` 或 CLI `aetheris verify <job-id>`，获取四类证明。
- **输出含义**：Execution hash（执行路径摘要）、Event chain root hash（事件流完整性）、Tool invocation ledger proof（at-most-once 证明）、Replay proof result（只读 Replay 一致性）。
- **与 Execution Proof Chain 的关系**：Proof Chain 是运行时机制（Ledger 裁决、Confirmation Replay）；Verification Mode 是**事后**对同一事件流与 Ledger 状态的只读校验与摘要输出，不改变任何状态。

---

## 输出项定义

| 输出项 | 含义 | 数据来源与计算方式 |
|--------|------|---------------------|
| **Execution hash** | 本次执行路径的确定性摘要 | 从事件流提取：PlanGenerated 的 plan_hash（若有）或 task_graph 的 SHA256；按顺序拼接各 NodeFinished 的 (node_id, result_type)，再 SHA256。无 PlanGenerated 时用事件流中首条 plan 相关信息的 hash。 |
| **Event chain root hash** | 事件流顺序的根 hash，证明事件未被篡改、顺序未变 | 顺序链：H0=empty, H_i = SHA256(H_{i-1} \|\| event_id \|\| type \|\| payload)。最终 H_n 为 root。或每事件参与一次 hash 链（见 [event-chain-sealing.md](event-chain-sealing.md)）。 |
| **Tool invocation ledger proof** | 证明所有 tool 调用均为 at-most-once | 扫描事件流：每条 tool_invocation_started 必须存在唯一匹配的 tool_invocation_finished（outcome success 或 确定性 failure）；无未闭合的 started。结果：ok / 列出未闭合的 idempotency_key。 |
| **Replay proof result** | 只读 Replay 与事件流一致 | 调用 ReplayContextBuilder.BuildFromEvents；校验：CompletedNodeIDs、CompletedCommandIDs、CompletedToolInvocations、PendingToolInvocations 与事件流推导一致；无异常则 ok。 |

---

## 事件链摘要协议（Event chain root）

- **参与字段**：对每条事件，将 `event.ID`、`event.Type`、`event.Payload`（原始 JSON 字节）按固定顺序拼接后参与 hash。
- **顺序**：与 ListEvents 返回顺序一致（按 version/created_at）。
- **链式**：root_0 = ""；root_i = SHA256(root_{i-1} + "\n" + event_id + " " + event_type + " " + base64(payload))。最终 root = root_n。
- **实现**：只读计算，不改变 Append 语义；见 [internal/agent/verify/chainhash.go](../internal/agent/verify/chainhash.go)（或等价位置）。

---

## API

- **GET /api/jobs/:id/verify**：返回 JSON：
  - `execution_hash`: string
  - `event_chain_root_hash`: string
  - `tool_invocation_ledger_proof`: { "ok": bool, "pending_idempotency_keys": []string }
  - `replay_proof_result`: { "ok": bool, "error": string }

若 Job 不存在或事件存储未启用，返回 404 / 503。

---

## CLI

- **aetheris verify \<job_id\>**：调用 GET /api/jobs/:id/verify，打印上述四项（表格或 JSON，与 trace 风格一致）。

---

## 参考

- [1.0-runtime-semantics.md](1.0-runtime-semantics.md) — Execution Proof Chain、Ledger、Confirmation Replay
- [execution-proof-sequence.md](execution-proof-sequence.md) — Runner–Ledger–JobStore 序列
- [docs/runtime-guarantees.md](../docs/runtime-guarantees.md) — Verification Mode 用户说明
- [event-chain-sealing.md](event-chain-sealing.md) — 事件链密封（2.0 延伸，可选签名）
