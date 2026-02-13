# Evidence Package - 可验证证据包系统

> 当前状态（main）：`aetheris export <job_id>` 与 `aetheris verify <evidence.zip>` 均可用。

## 概述

Evidence Package 是 Aetheris 2.0-M1 的核心特性，提供**可离线验证**的证据包系统。任何人拿到一个 job 的证据包（ZIP 文件），在没有数据库、没有运行时环境的情况下，也能验证：

- 事件流未被篡改（通过哈希链）
- Tool 没有被重复执行（通过 ledger 一致性）
- Replay 符合 1.0 语义
- 所有证据彼此一致

---

## 证据包结构

证据包是一个 ZIP 文件，包含以下内容：

```
evidence.zip
├── manifest.json     # 证据总清单（文件哈希、元信息）
├── events.ndjson     # 完整事件流（NDJSON 格式）
├── ledger.ndjson     # Tool 调用账本（NDJSON 格式）
├── proof.json        # 证明摘要（root hash、验证状态）
└── metadata.json     # Job 元信息
```

### manifest.json

证据包的"目录"和"校验和"：

```json
{
  "version": "2.0",
  "job_id": "job_abc123",
  "exported_at": "2026-02-12T21:30:00Z",
  "event_count": 482,
  "ledger_count": 17,
  "first_event_hash": "a1b2c3...",
  "last_event_hash": "d4e5f6...",
  "file_hashes": {
    "events.ndjson": "sha256_hash_of_events",
    "ledger.ndjson": "sha256_hash_of_ledger",
    "proof.json": "sha256_hash_of_proof"
  },
  "runtime_version": "2.0.0",
  "schema_version": "2.0"
}
```

### events.ndjson

完整事件流，每行一个 JSON 对象（NDJSON 格式）：

```
{"id":"1","job_id":"job_123","type":"job_created","payload":"...","created_at":"...","prev_hash":"","hash":"a1b2..."}
{"id":"2","job_id":"job_123","type":"plan_generated","payload":"...","created_at":"...","prev_hash":"a1b2...","hash":"c3d4..."}
...
```

每个事件包含：
- `prev_hash`: 上一个事件的 hash
- `hash`: 当前事件的 hash（SHA256）

### ledger.ndjson

Tool 调用账本，记录所有工具执行：

```
{"id":"inv_1","job_id":"job_123","idempotency_key":"key_abc","tool_name":"github_create_issue","status":"success","committed":true,...}
...
```

### proof.json

证明摘要，包含验证结果：

```json
{
  "job_id": "job_123",
  "root_hash": "d4e5f6...",
  "chain_validated": true,
  "ledger_validated": true,
  "generated_by": "aetheris 2.0.0"
}
```

---

## CLI 使用

### 导出证据包

```bash
# 基本用法
aetheris export <job_id>

# 指定输出路径
aetheris export job_abc123 --output /path/to/evidence.zip

# 示例
$ aetheris export job_abc123
Exporting evidence package for job job_abc123...
✓ Evidence package exported to: evidence-job_abc123.zip
  To verify: aetheris verify evidence-job_abc123.zip
```

### 验证证据包

```bash
# 基本用法
aetheris verify <evidence.zip>

# 示例：验证通过
$ aetheris verify evidence.zip
Verifying evidence package: evidence.zip

=== Verification Results ===
✓ Verification PASSED
  - Events: 482 valid
  - Hash chain: OK
  - Ledger consistency: OK
  - Manifest: OK
```

---

## 哈希链原理

### 计算规则

每个事件的 hash 由以下内容计算（SHA256）：

```
Hash = SHA256(JobID + "|" + Type + "|" + Payload + "|" + Timestamp + "|" + PrevHash)
```

### 链式结构

```
Event 0: PrevHash=""          Hash="a1b2..."
         ↓
Event 1: PrevHash="a1b2..."   Hash="c3d4..."
         ↓
Event 2: PrevHash="c3d4..."   Hash="e5f6..."
         ↓
         ...
```

任何事件被篡改、插入、删除，都会导致后续所有事件的 hash 不匹配，从而被检测。

---

## 验证逻辑

验证分为 5 个步骤：

1. **文件完整性**：验证 manifest 中声明的文件 SHA256 哈希
2. **哈希链完整性**：验证每个事件的 `prev_hash == 前一个事件的 hash`，并重新计算 hash 验证
3. **Ledger 一致性**：验证 events 中的 `tool_invocation_finished` 与 ledger 中的记录对齐
4. **Proof 一致性**：验证 `proof.root_hash == 最后一个事件的 hash`
5. **Manifest 一致性**：验证 manifest 中的 event_count、first_event_hash、last_event_hash

---

## 安全保证

### 防篡改

- **事件内容篡改**：Hash 不匹配，验证失败
- **事件插入/删除**：哈希链断裂，验证失败
- **事件重排序**：Timestamp + PrevHash 保证顺序

### 防重放

- Ledger 记录所有 tool invocations（去重）
- 每个 `tool_invocation_finished` 必须对应 ledger 中的记录
- Committed=true 表示外部世界已改变，replay 不可重复执行

### 可审计

- 完整事件流（从 job_created 到 job_completed/failed）
- Tool 调用记录（包含 external_id 溯源）
- Reasoning snapshots（可选，记录决策依据）

---

## 适用场景

1. **合规审计**：监管机构要求提供 agent 决策的完整证据链
2. **安全取证**：安全事件后需要验证 agent 行为未被篡改
3. **离线分析**：在隔离环境中分析 agent 执行历史
4. **多方验证**：多个组织独立验证 agent 执行结果
5. **法律证据**：提供可在法庭上使用的技术证据

---

## API 集成

除了 CLI，也可以通过 API 导出证据包：

```bash
# 导出证据包
POST /api/jobs/:id/export

# 返回 ZIP 文件（Content-Type: application/zip）
```

---

## 限制与注意事项

1. **性能开销**：哈希计算会增加约 5-10% 的事件写入延迟
2. **存储开销**：每个事件增加 2 个 hash 字段（约 128 bytes）
3. **历史数据**：M1 之前的事件没有 hash 字段，验证会跳过
4. **大 Job**：事件数超过 10 万的 job，导出/验证可能需要数分钟

---

## 常见问题

**Q: 证据包可以被压缩吗？**  
A: ZIP 本身已经是压缩格式。如需进一步压缩，可以使用 `gzip evidence.zip`。

**Q: 证据包可以离线验证多少次？**  
A: 无限次。验证是纯计算过程，不依赖任何外部系统。

**Q: 如果 hash 字段为空怎么办？**  
A: M1 之前的事件 hash 为空，验证会警告但不会失败。建议使用 migration 工具回填。

**Q: 证据包可以合并吗？**  
A: 不建议。每个 job 应该是独立的证据包。如需批量分析，可以使用 Forensics API。

**Q: 签名（Signature）什么时候实现？**  
A: M1 预留了签名字段，M2 会实现基于 Ed25519 的签名机制。

---

## 下一步

- **M2**: RBAC + 脱敏 + 留存策略
- **M3**: Evidence Graph + Forensics API + UI 取证视图

更多信息见 `docs/migration-to-m1.md`。
