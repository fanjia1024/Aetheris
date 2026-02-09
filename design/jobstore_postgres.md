# JobStore PostgreSQL 设计

当前内存实现见 [internal/runtime/jobstore/memory_store.go](internal/runtime/jobstore/memory_store.go)，接口定义见 [internal/runtime/jobstore/store.go](internal/runtime/jobstore/store.go)。本设计为 Postgres 实现与配置切换方案。

## 目标

将 `internal/runtime/jobstore` 的事件流与租约语义持久化到 PostgreSQL，实现崩溃恢复、多 Worker、任务接管与审计回放。Postgres 实现与现有内存实现实现同一 `JobStore` 接口，通过配置切换。

## 事件模型（与现有一致）

- **EventType**：`job_created` | `plan_generated` | `node_started` | `node_finished` | `tool_called` | `tool_returned` | `job_completed` | `job_failed`
- **JobEvent**：ID、JobID、Type、Payload（JSON）、CreatedAt
- **Version**：某 job 的 version = 该 job 已有事件条数；Append 时要求 expectedVersion == 当前 version，成功后返回 newVersion = current + 1（乐观并发）

## 表结构

### job_events（事件流）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID | 主键，默认 gen_random_uuid() |
| job_id | TEXT | 任务流 ID |
| version | INT | 按 job 单调递增，与「事件条数」一致 |
| type | TEXT | 事件类型 |
| payload | JSONB | 可选 |
| created_at | TIMESTAMPTZ | 默认 now() |

- 唯一约束：`(job_id, version)`，保证 Append 的 CAS 语义
- 索引：job_id、created_at

### job_claims（租约）

| 列 | 类型 | 说明 |
|----|------|------|
| job_id | TEXT | 主键 |
| worker_id | TEXT | 占用该 job 的 Worker |
| expires_at | TIMESTAMPTZ | 租约过期时间 |

- 索引：expires_at（便于清理过期租约、Claim 时查找可抢占 job）

## 操作语义

- **ListEvents**：`SELECT * FROM job_events WHERE job_id = $1 ORDER BY version`
- **Append**：在事务中 SELECT max(version) WHERE job_id = $1；若 max != expectedVersion 则返回 ErrVersionMismatch；否则 INSERT (job_id, version+1, type, payload)，返回 newVersion
- **Claim**：在事务中找「存在事件且最后事件 type 不在 (job_completed, job_failed) 且 (无 claim 或 expires_at < now())」的 job，INSERT/UPDATE job_claims，返回 job_id 与 version
- **Heartbeat**：UPDATE job_claims SET expires_at = now() + lease WHERE job_id = $1 AND worker_id = $2；影响行数 0 则 ErrClaimNotFound
- **Watch**：轮询或 LISTEN/NOTIFY；本实现采用轮询（简单、无额外连接），goroutine 定期拉取 version > lastVersion 的新事件写入 channel

## 配置

- jobstore.type：`memory` | `postgres`
- jobstore.dsn：Postgres 连接串（type=postgres 时必填）
- jobstore.lease_duration：租约时长，如 "30s"，默认 30s

## jobs 表（Job 元数据，API 与 Worker 共享）

当使用 Postgres 时，除事件流与租约外，需共享 Job 元数据以便 Worker 进程拉取执行。

| 列 | 类型 | 说明 |
|----|------|------|
| id | TEXT | 主键，job ID |
| agent_id | TEXT | 所属 Agent |
| goal | TEXT | 目标/消息 |
| status | INT | 0=Pending, 1=Running, 2=Completed, 3=Failed |
| cursor | TEXT | 恢复游标（Checkpoint ID） |
| retry_count | INT | 已重试次数 |
| created_at | TIMESTAMPTZ | 创建时间 |
| updated_at | TIMESTAMPTZ | 更新时间 |

- 实现：`internal/agent/job/pg_store.go`，实现 `job.JobStore` 接口（Create, Get, ListByAgent, UpdateStatus, UpdateCursor, ClaimNextPending, Requeue）。
- API 与 Worker 共用同一 DSN 时，均使用此表读写 Job。

## 迁移

- SQL 脚本：`internal/runtime/jobstore/schema.sql`（含 job_events、job_claims、jobs）
- 部署时执行即可创建表与索引；可选后续引入 migrate 库做版本化迁移
