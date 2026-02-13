# Aetheris 1.x -> 2.0 Upgrade and Rollback Guide

## 1. 适用范围

- 来源版本: Aetheris `1.0.x` 到 `2.0.x`
- 目标: 在不丢失 Job 与审计数据的前提下完成升级，并提供可执行回滚路径
- 依赖: PostgreSQL（推荐）、`aetheris` CLI、`scripts/local-2.0-stack.sh`

## 2. 变更摘要

2.0 相比 1.x 的关键变化:
- Job 运行时增强（DAG/Step/Signal/Replay）
- 取证能力增强（证据导出、离线校验、一致性接口）
- 可观测能力增强（Trace API 与 UI 页面）

数据库侧是向后兼容扩展为主，重点关注 `job_events` 与 `jobs` 表的新增字段。

## 3. 升级前检查（Gate）

执行前必须满足:
- 代码基线可构建: `go build ./...`
- 全量测试通过: `go test ./...`
- 数据库可备份并可恢复
- 已记录当前版本 tag / commit
- 已准备上一版本镜像或二进制用于回滚

推荐执行:

```bash
./scripts/release-2.0.sh
```

## 4. 标准升级步骤

### 4.1 备份

```bash
pg_dump -h <host> -U <user> -d <db> > backup-before-2.0.sql
```

### 4.2 应用 Schema

使用最新 schema:

```bash
psql -h <host> -U <user> -d <db> -f internal/runtime/jobstore/schema.sql
```

如是增量升级，至少确认以下字段存在:

```sql
ALTER TABLE job_events ADD COLUMN IF NOT EXISTS prev_hash TEXT DEFAULT '';
ALTER TABLE job_events ADD COLUMN IF NOT EXISTS hash TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_job_events_hash ON job_events (hash);
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ;
```

### 4.3 部署顺序

1. 先部署 API（新版本）
2. 再滚动部署 Worker（新版本）
3. 等待旧 Worker 清空在途任务后下线

## 5. 升级后验证（必须）

1. 健康检查:

```bash
curl http://localhost:8080/api/health
```

2. 核心链路:
- 创建 Agent
- 发送消息触发 Job
- 观察 Job 终态（completed/failed）

3. 取证链路:
- 导出证据包
- 离线校验证据包
- 调用一致性接口

4. Trace/UI:
- 打开 `GET /api/jobs/:id/trace/page`
- 打开 `GET /api/trace/overview/page`

## 6. 回滚策略

### 6.1 触发条件

满足任一条件建议回滚:
- 核心 API 错误率持续升高
- Job 大量卡住或 signal/replay 不可用
- 证据导出或 verify 大面积失败

### 6.2 回滚步骤

1. 回滚应用版本到上一 tag/镜像
2. 重启 API 与 Worker
3. 观察 10-30 分钟核心指标恢复

```bash
git checkout <previous-tag>
go build ./...
```

### 6.3 数据库回滚原则

- 默认不做 schema 回退（兼容字段可被旧版本忽略）
- 仅在必须时回退，并通过维护窗口执行
- 回退前再次备份

如需强制删除新增字段（高风险，不推荐）:

```sql
ALTER TABLE job_events DROP COLUMN IF EXISTS prev_hash;
ALTER TABLE job_events DROP COLUMN IF EXISTS hash;
ALTER TABLE jobs DROP COLUMN IF EXISTS cancel_requested_at;
```

## 7. 升级窗口建议

- 建议在业务低峰执行
- 执行人: 1 名发布负责人 + 1 名观测负责人
- 升级后至少观察 30 分钟再宣布完成

## 8. 关联文档

- `docs/release-checklist-2.0.md`
- `docs/migration-to-m1.md`
- `docs/api-contract.md`
- `docs/observability.md`
