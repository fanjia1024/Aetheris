# M2 RBAC Guide - 多租户与访问控制

## 概述

Aetheris 2.0-M2 引入多租户隔离和基于角色的访问控制（RBAC），确保不同租户之间的数据隔离，并提供细粒度的权限管理。

---

## 核心概念

### Tenant（租户）

租户是资源隔离的基本单位。每个 tenant 拥有：
- 独立的 jobs、agents、证据包
- 独立的配额（quota）
- 独立的审计日志

### Role（角色）

系统预定义 4 种角色，权限递减：

| 角色 | 权限 | 说明 |
|------|------|------|
| Admin | 全部权限 | 管理员，可以创建/查看/导出/停止/审计 |
| Operator | 查看 + 导出 + 停止 | 运维人员，可操作但不能管理 |
| Auditor | 只读 + 导出 + 审计 | 审计员，只读权限但可导出证据 |
| User | 基本操作 | 普通用户，可创建和查看自己的 jobs |

### Permission（权限）

细粒度权限：
- `job:view` - 查看 job
- `job:create` - 创建 job
- `job:stop` - 停止 job
- `job:export` - 导出证据包
- `trace:view` - 查看执行 trace
- `tool:execute` - 执行 tool
- `agent:manage` - 管理 agent
- `audit:view` - 查看审计日志

---

## 配置

### 启用 RBAC

**configs/api.yaml**:
```yaml
auth:
  enable: true
  mode: "jwt"
  multi_tenant: true
  
rbac:
  enable: true
  default_role: "user"
```

### 数据库初始化

```sql
-- 创建租户
INSERT INTO tenants (id, name, status, quota_json) VALUES
  ('tenant_a', 'Company A', 'active', '{"max_jobs":1000,"max_exports":100}'),
  ('tenant_b', 'Company B', 'active', '{"max_jobs":500,"max_exports":50}');

-- 分配角色
INSERT INTO user_roles (user_id, tenant_id, role) VALUES
  ('user_admin', 'tenant_a', 'admin'),
  ('user_auditor', 'tenant_a', 'auditor'),
  ('user_operator', 'tenant_b', 'operator');
```

---

## API 使用

### 认证

使用 JWT 进行认证，token 中包含 `tenant_id` 和 `user_id`：

```bash
# 登录获取 token
curl -X POST http://api/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}'

# 响应
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "tenant_id": "tenant_a",
  "role": "admin"
}
```

### 携带 Token 访问

```bash
# 查看 job（需要 job:view 权限）
curl -H "Authorization: Bearer <token>" \
     http://api/api/jobs/job_123

# 导出证据包（需要 job:export 权限）
curl -X POST -H "Authorization: Bearer <token>" \
     http://api/api/jobs/job_123/export \
     --output evidence.zip
```

---

## Tenant 隔离

### 自动过滤

所有 API 自动按 tenant_id 过滤：

```go
// 示例：ListAgentJobs 只返回当前 tenant 的 jobs
func (h *Handler) ListAgentJobs(ctx context.Context, c *app.RequestContext) {
    tenantID := auth.GetTenantID(ctx)
    agentID := c.Param("id")
    
    // 只查询当前 tenant 的 jobs
    jobs, err := h.jobStore.ListByAgentAndTenant(ctx, agentID, tenantID)
    // ...
}
```

### 跨租户访问被拒绝

```bash
# Tenant A 的 token 访问 Tenant B 的 job
curl -H "Authorization: Bearer <tenant_a_token>" \
     http://api/api/jobs/<tenant_b_job_id>

# 响应: 403 Forbidden
{
  "error": "permission denied"
}
```

---

## 权限检查

### 编程式检查

```go
// 在 handler 中检查权限
if !h.rbac.CheckPermission(ctx, tenantID, userID, auth.PermissionJobExport, jobID) {
    return errors.New("permission denied")
}
```

### 中间件检查

```go
// 在路由中使用权限中间件
jobs.POST("/:id/export", 
    authHandler, 
    authz.RequirePermission(auth.PermissionJobExport), 
    r.handler.ExportJobForensics)
```

---

## 审计日志

所有 API 访问自动记录到 `access_audit_log` 表：

```sql
SELECT 
    user_id,
    action,
    resource_type,
    resource_id,
    success,
    created_at
FROM access_audit_log
WHERE tenant_id = 'tenant_a'
  AND created_at > now() - interval '24 hours'
ORDER BY created_at DESC;
```

### 审计日志字段

- `tenant_id`: 租户 ID
- `user_id`: 用户 ID
- `action`: 操作类型（view_job, export_evidence, etc.）
- `resource_type`: 资源类型（job, agent, evidence_package）
- `resource_id`: 资源 ID
- `success`: 是否成功
- `duration_ms`: 耗时（毫秒）
- `created_at`: 时间戳

---

## 配额管理

### Tenant 配额

每个 tenant 可配置配额：

```go
type TenantQuota struct {
    MaxJobs      int   // 最大 job 数
    MaxStorage   int64 // 最大存储（bytes）
    MaxExports   int   // 每天最大导出次数
    MaxAgents    int   // 最大 agent 数
}
```

### 配额检查

导出证据包前检查配额：

```go
if exceeded, _ := h.quotaChecker.CheckExportQuota(ctx, tenantID); exceeded {
    return errors.New("export quota exceeded")
}
```

---

## 最佳实践

1. **最小权限原则**: 默认分配 User 角色，按需提升
2. **定期审计**: 定期检查 `access_audit_log` 发现异常访问
3. **配额设置**: 根据 tenant 规模合理设置配额
4. **角色分离**: 生产环境使用 Operator，审计使用 Auditor

---

## 故障排查

### 问题: 403 Forbidden

**原因**: 用户没有所需权限

**解决**: 
```sql
-- 检查用户角色
SELECT role FROM user_roles 
WHERE user_id = 'xxx' AND tenant_id = 'yyy';

-- 提升权限
UPDATE user_roles SET role = 'admin' 
WHERE user_id = 'xxx' AND tenant_id = 'yyy';
```

### 问题: Quota exceeded

**原因**: Tenant 达到配额上限

**解决**:
```sql
-- 检查配额
SELECT quota_json FROM tenants WHERE id = 'tenant_a';

-- 提升配额
UPDATE tenants SET quota_json = '{"max_exports":200}' 
WHERE id = 'tenant_a';
```

---

## 下一步

- **M3**: Evidence Graph + Forensics API
- 查看 `docs/m2-redaction-guide.md` 了解脱敏策略
- 查看 `docs/m2-retention-guide.md` 了解留存策略
