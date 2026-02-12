# M3 Forensics API Guide - 取证查询接口

## 概述

Aetheris 2.0-M3 Forensics API 提供"案件式"查询能力，支持按时间范围、tool 类型、关键事件等维度检索 jobs，并支持批量导出和一致性检查。

---

## API Endpoints

### 1. 复杂查询

**POST /api/forensics/query**

按多个维度查询 jobs。

**请求**:
```json
{
  "tenant_id": "tenant_a",
  "time_range": {
    "start": "2026-02-01T00:00:00Z",
    "end": "2026-02-12T23:59:59Z"
  },
  "tool_filter": ["stripe*", "sendgrid*"],
  "event_filter": ["payment_executed", "email_sent"],
  "agent_filter": ["agent_123"],
  "status_filter": ["completed"],
  "limit": 20,
  "offset": 0
}
```

**响应**:
```json
{
  "jobs": [
    {
      "job_id": "job_abc",
      "agent_id": "agent_123",
      "tenant_id": "tenant_a",
      "created_at": "2026-02-10T14:30:00Z",
      "status": "completed",
      "event_count": 482,
      "tool_calls": ["stripe.charge", "sendgrid.send"],
      "key_events": ["payment_executed", "email_sent"]
    }
  ],
  "total_count": 15,
  "page": 0
}
```

### 2. 批量导出

**POST /api/forensics/batch-export**

批量导出多个 jobs 的证据包（异步）。

**请求**:
```json
{
  "job_ids": ["job_1", "job_2", "job_3"],
  "redaction": true
}
```

**响应**:
```json
{
  "task_id": "task_xyz",
  "status": "processing",
  "poll_url": "/api/forensics/export-status/task_xyz"
}
```

### 3. 查询导出状态

**GET /api/forensics/export-status/:task_id**

**响应**:
```json
{
  "task_id": "task_xyz",
  "status": "completed",
  "progress": 100,
  "download_url": "/api/forensics/download/task_xyz"
}
```

Status 可能值:
- `pending`: 等待处理
- `processing`: 处理中
- `completed`: 完成
- `failed`: 失败

### 4. 一致性检查

**GET /api/forensics/consistency/:job_id**

检查证据链完整性。

**响应**:
```json
{
  "job_id": "job_123",
  "hash_chain_valid": true,
  "ledger_consistent": true,
  "evidence_complete": true,
  "issues": []
}
```

如果有问题：
```json
{
  "job_id": "job_456",
  "hash_chain_valid": false,
  "ledger_consistent": true,
  "evidence_complete": false,
  "issues": [
    "hash chain broken at event 42",
    "evidence node tool:inv_123 not found in ledger"
  ]
}
```

---

## 查询语法

### 时间范围

```json
{
  "time_range": {
    "start": "2026-02-01T00:00:00Z",
    "end": "2026-02-12T23:59:59Z"
  }
}
```

### Tool 过滤

支持通配符：

```json
{
  "tool_filter": [
    "stripe*",      // 所有 stripe 工具
    "github.create*", // github.create_issue, github.create_pr
    "sendgrid.send"   // 精确匹配
  ]
}
```

### 事件过滤

按关键事件类型：

```json
{
  "event_filter": [
    "payment_executed",
    "email_sent",
    "human_approval_given",
    "critical_decision_made"
  ]
}
```

### 分页

```json
{
  "limit": 20,   // 每页 20 条
  "offset": 0    // 从第 0 条开始
}
```

---

## 使用示例

### 示例 1: 查找所有支付相关的 jobs

```bash
curl -X POST http://api/forensics/query \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "tenant_a",
    "time_range": {
      "start": "2026-02-01T00:00:00Z",
      "end": "2026-02-12T23:59:59Z"
    },
    "tool_filter": ["stripe*"],
    "event_filter": ["payment_executed"]
  }'
```

### 示例 2: 查找包含人类审批的 jobs

```bash
curl -X POST http://api/forensics/query \
  -d '{
    "tenant_id": "tenant_a",
    "event_filter": ["human_approval_given"]
  }'
```

### 示例 3: 批量导出证据包

```bash
# 1. 提交批量导出任务
curl -X POST http://api/forensics/batch-export \
  -d '{
    "job_ids": ["job_1", "job_2", "job_3"],
    "redaction": true
  }'

# 响应: {"task_id": "task_xyz", "status": "processing"}

# 2. 轮询状态
curl http://api/forensics/export-status/task_xyz

# 响应: {"status": "completed", "download_url": "/api/forensics/download/task_xyz"}

# 3. 下载结果
curl http://api/forensics/download/task_xyz --output batch-export.zip
```

### 示例 4: 检查一致性

```bash
curl http://api/forensics/consistency/job_123

# 响应
{
  "job_id": "job_123",
  "hash_chain_valid": true,
  "ledger_consistent": true,
  "evidence_complete": true
}
```

---

## 查询优化

### 索引建议

为提高查询性能，建议添加索引：

```sql
-- 时间范围查询
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs (created_at);

-- Tenant 过滤
CREATE INDEX IF NOT EXISTS idx_jobs_tenant_created ON jobs (tenant_id, created_at);

-- 审计日志查询
CREATE INDEX IF NOT EXISTS idx_access_audit_action ON access_audit_log (action, created_at);
```

### 缓存策略

- 查询结果缓存 5 分钟
- Evidence Graph 缓存 10 分钟
- 批量导出结果保留 24 小时

---

## 权限要求

| API | 需要权限 |
|-----|----------|
| POST /api/forensics/query | `job:view` |
| POST /api/forensics/batch-export | `job:export` |
| GET /api/forensics/consistency/:id | `job:view` |
| GET /api/jobs/:id/evidence-graph | `trace:view` |

---

## 限流与配额

- **查询频率**: 每分钟 60 次
- **批量导出**: 每次最多 100 个 jobs
- **并发任务**: 每个 tenant 最多 3 个并发导出任务

---

## 故障排查

### 问题: Query 返回空结果

**原因**: 过滤条件太严格或时间范围不对

**解决**: 放宽条件，先只用 time_range 查询

### 问题: Batch export 超时

**原因**: Jobs 太多或单个 job 事件数过大

**解决**: 分批导出（每批 10-20 个 jobs）

### 问题: Consistency check 失败

**原因**: 数据损坏或 M1 之前的事件缺少 hash

**解决**: 运行 migration 回填 hash

---

## CLI 支持

```bash
# 查询
aetheris forensics query --tenant tenant_a --tool stripe* --time-range 7d

# 批量导出
aetheris forensics batch-export job_1 job_2 job_3 --output batch.zip

# 一致性检查
aetheris forensics check job_123
```

---

## 下一步

- 查看 `docs/m3-evidence-graph-guide.md` 了解 Evidence Graph
- 查看 `docs/m3-ui-guide.md` 了解 UI 操作
