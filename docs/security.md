# Aetheris 2.0 Security Baseline

## 1. 目标

定义 2.0 发布前必须满足的最小安全基线，覆盖认证授权、数据保护、审计追溯与运维安全。

## 2. P0 安全控制

### 2.1 认证与授权

- 生产环境必须启用认证（禁止匿名写接口）
- 多租户场景必须启用 tenant 隔离与 RBAC
- 高风险接口（export/stop/signal）需要显式权限

参考: `docs/m2-rbac-guide.md`

### 2.2 敏感数据保护

- 导出证据包前必须可配置脱敏策略
- 至少支持 `redact` 与 `hash` 模式
- 脱敏后证据包仍可 verify

参考: `docs/m2-redaction-guide.md`

### 2.3 数据留存与删除

- 必须定义 retention policy
- 删除必须保留 tombstone 审计信息
- 重要业务建议“先归档再删除”

参考: `docs/m2-retention-guide.md`

### 2.4 审计与追踪

- 访问审计日志需覆盖关键操作:
  - 创建/停止 job
  - 导出证据包
  - 权限拒绝事件
- 审计日志应具备 tenant/user/action/result/timestamp

### 2.5 传输与密钥

- 生产环境建议启用 TLS
- 禁止将密钥硬编码在代码与配置仓库
- 使用环境变量或密钥管理系统注入

## 3. 发布前安全检查（P0 Gate）

发布前至少确认:
- [ ] 认证默认开启（生产配置）
- [ ] RBAC 权限矩阵通过抽样验证
- [ ] 脱敏策略在导出路径生效
- [ ] 一次 retention + tombstone 流程演练通过
- [ ] 审计日志可查询并包含关键字段

## 4. 运维建议

- 最小权限原则（默认 user，按需提权）
- 定期轮换凭据（数据库、API key、JWT secret）
- 每月审计一次高风险操作
- 对导出与校验失败设置告警

## 5. 非目标（2.0）

以下不作为 2.0 发布阻断项，但建议后续迭代:
- 自动化渗透测试流水线
- 完整 SBOM 与供应链签名
- 细粒度 KMS 集成模板

## 6. 关联文档

- `docs/m2-rbac-guide.md`
- `docs/m2-redaction-guide.md`
- `docs/m2-retention-guide.md`
- `docs/release-checklist-2.0.md`
