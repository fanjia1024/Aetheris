# M2 Retention Guide - 数据留存与合规删除

## 概述

Aetheris 2.0-M2 提供数据生命周期管理，支持自动归档、合规删除、tombstone 审计，确保数据留存符合法规要求。

---

## 核心概念

### Retention Policy（留存策略）

定义数据保留时长：
- **Retention Days**: 数据保留天数（之后可删除）
- **Archive Days**: 归档天数（移到冷存储）
- **Auto Delete**: 是否自动删除过期数据

### Tombstone（墓碑）

删除后的审计记录：
- 记录 job 曾经存在
- 记录删除原因和操作人
- 记录归档位置（如果归档）
- 永久保留（或更长 retention）

---

## 配置

### 基本配置

**configs/api.yaml**:
```yaml
retention:
  enable: true
  default_retention_days: 90      # 默认保留 90 天
  archive_after_days: 30          # 30 天后归档
  auto_delete: false              # 不自动删除（需要手动审批）
  scan_interval: "24h"            # 每 24 小时扫描一次
```

### 按 Job 类型配置

不同类型的 jobs 可配置不同留存策略：

```yaml
retention:
  enable: true
  default_retention_days: 90
  policies:
    - job_type: "critical"
      retention_days: 365        # 关键任务保留 1 年
    - job_type: "financial"
      retention_days: 2555       # 金融任务保留 7 年
    - job_type: "test"
      retention_days: 7          # 测试任务只保留 7 天
```

---

## 数据生命周期

### 阶段 1: 活跃期（0-30 天）

- 数据在主数据库
- 可快速访问
- 支持 replay 和 trace

### 阶段 2: 归档期（30-90 天）

- 导出证据包到冷存储（S3/GCS）
- 主数据库保留索引
- 可从归档恢复

### 阶段 3: 删除期（90 天后）

- 主数据库数据删除
- 创建 tombstone 记录
- 冷存储保留证据包（可选）

---

## 归档流程

### 自动归档

Worker 定期扫描并归档过期 jobs：

```go
// 在 worker 启动时
if cfg.Retention.Enable {
    go func() {
        ticker := time.NewTicker(cfg.Retention.ScanInterval)
        for range ticker.C {
            count, _ := retentionEngine.RunRetentionScan(ctx)
            log.Infof("Archived %d jobs", count)
        }
    }()
}
```

### 手动归档

```bash
# CLI 手动归档
aetheris archive job_123

# API 手动归档
curl -X POST http://api/api/jobs/job_123/archive
```

### 归档存储

证据包存储到冷存储：

```
s3://aetheris-archive/
  └── tenant_a/
      ├── 2026-01/
      │   ├── job_123.zip
      │   └── job_456.zip
      └── 2026-02/
          └── job_789.zip
```

---

## 删除流程

### 合规删除

删除包含 3 个步骤：

1. **导出证据包**（如果配置归档）
2. **写入 tombstone 事件**
3. **删除主数据库记录**

### Tombstone 记录

删除后创建的审计记录：

```json
{
  "job_id": "job_123",
  "tenant_id": "tenant_a",
  "agent_id": "agent_1",
  "deleted_at": "2026-05-12T10:00:00Z",
  "deleted_by": "system_retention_policy",
  "reason": "retention_policy_expired",
  "event_count": 482,
  "retention_days": 90,
  "archive_ref": "s3://archive/tenant_a/job_123.zip"
}
```

### 查询 Tombstone

```sql
-- 查询已删除的 jobs
SELECT job_id, deleted_at, reason, archive_ref
FROM job_tombstones
WHERE tenant_id = 'tenant_a'
ORDER BY deleted_at DESC
LIMIT 10;

-- 检查 job 是否被删除
SELECT * FROM job_tombstones WHERE job_id = 'job_123';
```

---

## 从归档恢复

### 恢复流程

1. 从冷存储下载证据包
2. 解压并验证完整性
3. 重新导入事件流（可选）

```bash
# 1. 下载归档
aws s3 cp s3://archive/tenant_a/job_123.zip ./

# 2. 验证完整性
aetheris verify job_123.zip

# 3. 查看内容
unzip -l job_123.zip
```

---

## 留存策略矩阵

### 推荐配置

| Job 类型 | 留存天数 | 归档天数 | 自动删除 | 说明 |
|----------|----------|----------|----------|------|
| Critical | 365 | 30 | No | 关键任务，保留 1 年 |
| Financial | 2555 | 30 | No | 金融任务，保留 7 年 |
| Healthcare | 1825 | 30 | No | 医疗任务，保留 5 年 |
| Default | 90 | 30 | Yes | 默认任务，保留 90 天 |
| Test | 7 | 0 | Yes | 测试任务，保留 7 天 |
| Ephemeral | 1 | 0 | Yes | 临时任务，保留 1 天 |

### 合规要求示例

- **GDPR**: 个人数据保留最多 90 天（除非业务需要）
- **SOX**: 金融记录保留 7 年
- **HIPAA**: 医疗记录保留 6 年

---

## Tombstone 机制

### 为什么需要 Tombstone？

1. **审计追溯**: 证明 job 存在过，何时被删除
2. **防止误删**: 有删除记录，可追责
3. **合规要求**: 监管机构可能要求保留删除记录
4. **归档引用**: 如需恢复，可找到归档位置

### Tombstone vs 完全删除

| 操作 | Job Events | Tool Ledger | Tombstone | Archive |
|------|------------|-------------|-----------|---------|
| 归档 | 保留 | 保留 | 创建 | 创建 |
| 合规删除 | 删除 | 删除 | 创建 | 创建（可选）|
| 完全删除 | 删除 | 删除 | 删除 | 删除 |

**推荐**: 始终使用合规删除（保留 tombstone）

---

## 性能影响

- **归档扫描**: 每 24 小时 1 次，耗时约 1-5 分钟（取决于 job 数量）
- **归档导出**: 每个 job 约 100-500ms
- **Tombstone 创建**: 每个 job 约 1ms
- **存储开销**: Tombstone 表约占原数据的 0.1%

---

## 最佳实践

1. **分层留存**: 不同重要性的 jobs 配置不同留存期
2. **先归档后删除**: 重要 jobs 先归档再删除
3. **定期审查**: 每月审查 tombstone 表，确认删除合理
4. **冷存储配置**: 使用 S3 Glacier / Azure Archive 降低成本
5. **监控告警**: 监控归档失败、删除异常

---

## CLI 命令

```bash
# 查看留存策略
aetheris retention policies

# 手动归档
aetheris archive job_123

# 手动删除（创建 tombstone）
aetheris delete job_123 --reason "manual_cleanup"

# 查看 tombstones
aetheris tombstones --tenant tenant_a --limit 10

# 恢复归档
aetheris restore job_123 --from s3://archive/job_123.zip
```

---

## 故障排查

### 问题: 归档失败

```
Error: failed to archive job_123: S3 upload failed
```

**解决**:
1. 检查 S3 凭证配置
2. 检查网络连接
3. 检查存储桶权限

### 问题: 自动删除未执行

**原因**: `auto_delete: false` 或 retention scan 未启动

**解决**:
```yaml
retention:
  enable: true
  auto_delete: true
  scan_interval: "24h"
```

重启 worker 生效。

---

## 合规检查清单

- [ ] 配置适当的留存策略
- [ ] 启用归档（重要 jobs）
- [ ] 启用 tombstone（所有删除）
- [ ] 定期审查 tombstone 表
- [ ] 监控归档失败告警
- [ ] 文档化删除审批流程
- [ ] 测试归档恢复流程

---

## 下一步

- **M3**: Evidence Graph + Forensics Query API
- 查看 `docs/m2-rbac-guide.md` 了解访问控制
- 查看 `docs/m2-redaction-guide.md` 了解脱敏策略
