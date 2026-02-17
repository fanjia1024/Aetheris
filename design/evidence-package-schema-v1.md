# Evidence Package Schema v1.0

## Overview

Evidence Package 是 Aetheris 证据包的标准格式，用于导出、验证、归档 agent 执行的完整证据链。

---

## Package Structure

```
evidence.zip
├── manifest.json      # 证据清单（文件哈希、元信息）
├── events.ndjson      # 事件流（NDJSON 格式）
├── ledger.ndjson      # Tool 调用账本（NDJSON 格式）
├── proof.json         # 证明摘要（root hash）
└── metadata.json      # Job 元信息
```

---

## File Formats

### manifest.json

```json
{
  "version": "1.0",
  "job_id": "job_abc123",
  "exported_at": "2026-02-12T10:30:00Z",
  "event_count": 482,
  "ledger_count": 17,
  "first_event_hash": "a1b2c3d4...",
  "last_event_hash": "e5f6g7h8...",
  "file_hashes": {
    "events.ndjson": "sha256_of_events",
    "ledger.ndjson": "sha256_of_ledger",
    "proof.json": "sha256_of_proof",
    "metadata.json": "sha256_of_metadata"
  },
  "runtime_version": "2.1.0",
  "schema_version": "1.0"
}
```

### events.ndjson

Newline Delimited JSON，每行一个事件：

```
{"id":"1","job_id":"job_123","type":"job_created","payload":"{...}","created_at":"...","prev_hash":"","hash":"a1b2..."}
{"id":"2","job_id":"job_123","type":"plan_generated","payload":"{...}","created_at":"...","prev_hash":"a1b2...","hash":"c3d4..."}
```

字段规范：
- `id`: 事件唯一 ID
- `job_id`: Job ID
- `type`: 事件类型（见事件类型清单）
- `payload`: JSON string（事件数据）
- `created_at`: ISO 8601 timestamp
- `prev_hash`: 前一个事件的 SHA256
- `hash`: 当前事件的 SHA256

### ledger.ndjson

Tool 调用记录：

```
{"id":"inv_1","job_id":"job_123","idempotency_key":"key_abc","tool_name":"stripe.charge","status":"success","committed":true,"result":"{...}","timestamp":"..."}
```

字段规范：
- `id`: Invocation ID
- `job_id`: Job ID
- `idempotency_key`: 幂等键
- `tool_name`: 工具名称
- `status`: 状态（success/failure/timeout）
- `committed`: 是否已提交（true=外部世界已改变）
- `result`: JSON string（工具输出）
- `timestamp`: ISO 8601 timestamp
- `external_id`: 可选，外部系统 ID

### proof.json

证明摘要：

```json
{
  "job_id": "job_123",
  "root_hash": "e5f6g7h8...",
  "chain_validated": true,
  "ledger_validated": true,
  "generated_by": "aetheris 2.1.0",
  "signature": ""
}
```

### metadata.json

Job 元信息：

```json
{
  "job_id": "job_123",
  "agent_id": "agent_1",
  "goal": "Process payment",
  "status": "completed",
  "created_at": "2026-02-12T10:00:00Z",
  "updated_at": "2026-02-12T10:05:00Z",
  "retry_count": 0
}
```

---

## Verification Rules

### 1. File Integrity

验证 manifest.file_hashes 中的每个文件：
```
SHA256(file_content) == manifest.file_hashes[filename]
```

### 2. Hash Chain

验证事件链：
```
For each event[i] (i > 0):
  event[i].prev_hash == event[i-1].hash
  SHA256(event[i].data) == event[i].hash
```

### 3. Ledger Consistency

验证 ledger 与 events 对齐：
```
For each tool_invocation_finished event:
  - Must exist in ledger
  - idempotency_key matches
  - tool_name matches
  - If committed=true in ledger, must have finished event
```

### 4. Proof Summary

验证：
```
proof.root_hash == events[last].hash
```

---

## Version Evolution

### v1.0 (Current)

- Basic structure
- Hash chain
- Ledger consistency

### v1.1 (Planned)

- Add `reasoning.ndjson` (optional)
- Add `attachments/` directory (optional)

### v2.0 (Future)

- Digital signatures
- Multi-org sync metadata

---

## Compatibility

- Backward compatible: Old packages can be verified with new verifier
- Forward compatible: New fields can be ignored by old verifier
- Schema version in manifest enables routing

---

## Implementation Reference

- Export: `pkg/proof/export.go`
- Verify: `pkg/proof/verify.go`
- Hash: `pkg/proof/hash.go`
- Tests: `pkg/proof/verify_test.go`

---

**Schema Version**: 1.0  
**Status**: Stable  
**Last Updated**: 2026-02-12
