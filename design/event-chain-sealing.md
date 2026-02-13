# Event Chain Sealing — 事件链密封

事件链密封在 [Verification Mode](verification-mode.md) 的 **Event chain root hash** 基础上，增加**可选**的密码学签名与公钥校验，用于审计防篡改与「可证明未被修改」的合规需求。实现排期在 Verification Mode 之后。

---

## 目标

- **防篡改**：事件流一旦写入，任何对事件或顺序的修改会导致 root hash 变化；若 root 被私钥签名，则仅持有公钥的第三方可验证「该 root 由持有私钥方签发」，从而证明事件链未被篡改（或篡改可被检测）。
- **与 Verification 衔接**：`aetheris verify <job-id>` 已输出 `event_chain_root_hash`；密封协议在此 root 上增加「可选签名」与 `verify` 时的公钥校验步骤。
- **与 Evidence 结合**：与 [execution-forensics.md](execution-forensics.md) 的 Evidence、审计日志结合，满足金融/医疗等「可审计且不可抵赖」场景。

---

## 协议

### 1. 事件链根 hash（已有）

与 [verification-mode.md](verification-mode.md) 一致：按 ListEvents 顺序，H_i = SHA256(H_{i-1} || event_id || type || base64(payload))，root = H_n。由 `verify` 包计算，不改变 Append 语义。

### 2. 可选签名

- **输入**：job_id、event_chain_root_hash、可选 timestamp（防止重放）。
- **签名内容**：建议 `content = job_id + "\n" + event_chain_root_hash + "\n" + timestamp_rfc3339`，再对 content 做私钥签名（如 ECDSA P-256 或 Ed25519）。
- **输出**：signature（base64 或 hex）、algorithm、可选 public_key_id（标识用于校验的公钥）。

### 3. 校验

- **输入**：job_id、event_chain_root_hash、signature、public_key。
- **步骤**：使用同一 content 构造方式，用公钥验证 signature；再校验当前从事件流计算得到的 root 与签名时的 event_chain_root_hash 一致。
- **结果**：一致且签名有效则「密封通过」；否则「密封失败」（事件被篡改或签名无效）。

### 4. 存储与 API

- **存储**：签名可与 Job 元数据一起存储（如 `job_verify_seal` 表：job_id, root_hash, signature, algorithm, signed_at），或由外部系统在 verify 后自行签名存储。
- **API**：可在 `GET /api/jobs/:id/verify` 响应中增加可选字段 `event_chain_seal`: { "signature", "algorithm", "signed_at" }；若配置了签名私钥则返回，否则不返回。校验可由 CLI `aetheris verify --public-key=...` 或独立校验接口完成。

---

## 实现顺序

1. **Phase 1（当前）**：Verification Mode 已输出 event_chain_root_hash；无签名。
2. **Phase 2**：在配置私钥时，Verify 完成后对 root 签名并写入存储或返回；API/CLI 可选返回 seal。
3. **Phase 3**：提供公钥校验接口或 CLI 参数，供第三方验证「该 Job 的事件链 root 由某方签发且与当前计算一致」。

---

## 参考

- [verification-mode.md](verification-mode.md) — Event chain root、Verify API/CLI
- [execution-forensics.md](execution-forensics.md) — Evidence、审计
- [1.0-runtime-semantics.md](1.0-runtime-semantics.md) — Execution Proof Chain
