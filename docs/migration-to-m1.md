# Migration Guide: 升级到 2.0-M1

## 概述

本文档说明如何从 Aetheris 1.0 升级到 2.0-M1（Evidence Package + Offline Verify）。

---

## 主要变更

### 1. 数据库 Schema 变更

`job_events` 表新增 2 个字段：

```sql
ALTER TABLE job_events ADD COLUMN IF NOT EXISTS prev_hash TEXT DEFAULT '';
ALTER TABLE job_events ADD COLUMN IF NOT EXISTS hash TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_job_events_hash ON job_events (hash);
```

### 2. 代码变更

- `JobEvent` 结构新增 `PrevHash` 和 `Hash` 字段
- `JobStore.Append` 自动计算并存储哈希
- 新增 `pkg/proof` 包（导出/验证逻辑）
- CLI 新增 `export` 和 `verify` 命令

### 3. 向后兼容

- 旧版本事件（hash 为空）仍可正常读取
- 新版本会自动为新事件计算 hash
- 证据包验证会跳过 hash 为空的事件（但会警告）

---

## 升级步骤

### Step 1: 停止服务（可选，零停机升级可跳过）

```bash
# 停止 API 和 Worker
pkill -f "go run ./cmd/api"
pkill -f "go run ./cmd/worker"
```

### Step 2: 备份数据库

```bash
# PostgreSQL 备份
pg_dump -h localhost -U aetheris -d aetheris > backup-before-m1.sql
```

### Step 3: 运行 Database Migration

```bash
# 连接到数据库并执行 migration
psql -h localhost -U aetheris -d aetheris -f internal/runtime/jobstore/schema.sql
```

或者使用已有的表时（增量更新）：

```bash
psql -h localhost -U aetheris -d aetheris << 'EOF'
ALTER TABLE job_events ADD COLUMN IF NOT EXISTS prev_hash TEXT DEFAULT '';
ALTER TABLE job_events ADD COLUMN IF NOT EXISTS hash TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_job_events_hash ON job_events (hash);
EOF
```

### Step 4: 更新代码

```bash
# 拉取最新代码
git pull origin main

# 重新编译
go build ./...

# 运行测试
go test ./pkg/proof/...
```

### Step 5: 启动服务

```bash
# 启动 API
go run ./cmd/api &

# 启动 Worker
go run ./cmd/worker &
```

### Step 6: 验证升级

```bash
# 测试 CLI 命令
aetheris version

# 创建测试 job
aetheris agent create test-agent

# 导出证据包（如果有 job）
aetheris export <job_id> --output test-evidence.zip

# 验证证据包
aetheris verify test-evidence.zip
```

---

## 历史数据处理

### 选项 1: 不处理（推荐）

- 旧事件 hash 为空，新事件有 hash
- 证据包验证会警告但不会失败
- 适合大多数场景

### 选项 2: 回填 hash（可选）

如果需要对历史数据进行审计，可以使用 CLI 回填导出事件文件中的哈希链：

```bash
aetheris migrate backfill-hashes \
  --input events.ndjson \
  --output events.backfilled.ndjson
```

**警告**：回填 hash 可能需要数小时（取决于事件数量）。

---

## 零停机升级

M1 支持零停机升级：

1. **滚动部署 API**：
   - 启动新版本 API 实例
   - 等待健康检查通过
   - 停止旧版本实例

2. **滚动部署 Worker**：
   - 启动新版本 Worker 实例
   - 旧版本 Worker 自然完成当前 job 后退出
   - 新 job 由新版本 Worker 处理

3. **数据库 Migration**：
   - `ADD COLUMN IF NOT EXISTS` 不会锁表
   - 创建索引可能需要数秒（在线 DDL）

---

## 回滚步骤

如果升级后发现问题，可以回滚：

### Step 1: 回滚代码

```bash
git checkout <previous-version-tag>
go build ./...
```

### Step 2: 重启服务

```bash
# 重启 API 和 Worker
pkill -f "go run ./cmd/api"
pkill -f "go run ./cmd/worker"
go run ./cmd/api &
go run ./cmd/worker &
```

### Step 3: 数据库回滚（可选）

M1 的 schema 变更是向后兼容的，旧代码会忽略 `prev_hash` 和 `hash` 字段。如果需要完全回滚：

```bash
psql -h localhost -U aetheris -d aetheris << 'EOF'
ALTER TABLE job_events DROP COLUMN IF EXISTS prev_hash;
ALTER TABLE job_events DROP COLUMN IF EXISTS hash;
EOF
```

**警告**：删除列会丢失已生成的 hash，且可能锁表。

---

## 性能影响

### 写入延迟

- **哈希计算**：每个事件增加约 0.1-0.5ms
- **额外查询**：每次 Append 需要查询前一个事件的 hash（+1 查询）
- **总体影响**：写入延迟增加约 5-10%

### 存储开销

- **Hash 字段**：每个事件约 128 bytes（2 个 SHA256 哈希）
- **索引开销**：hash 索引约占事件表大小的 5-10%

### 优化建议

1. 使用 PostgreSQL 连接池
2. 启用 prepared statements
3. 对高频 job，考虑使用 snapshot 减少回放成本

---

## 兼容性矩阵

| 功能 | 1.0 | M1 | M2 | M3 |
|------|-----|----|----|-----|
| Event 存储 | ✓ | ✓ | ✓ | ✓ |
| Proof chain | - | ✓ | ✓ | ✓ |
| Evidence export | - | ✓ | ✓ | ✓ |
| Offline verify | - | ✓ | ✓ | ✓ |
| RBAC | - | - | ✓ | ✓ |
| 脱敏 | - | - | ✓ | ✓ |
| Forensics API | - | - | - | ✓ |

---

## 故障排查

### 问题: Migration 失败

```
ERROR: column "prev_hash" of relation "job_events" already exists
```

**解决**：使用 `ADD COLUMN IF NOT EXISTS`，或手动检查列是否存在。

### 问题: 验证失败（hash 为空）

```
✗ Verification FAILED
  - event 0 hash is empty
```

**原因**：证据包包含 M1 之前的事件。

**解决**：运行 `aetheris migrate backfill-hashes --input events.ndjson --output events.backfilled.ndjson` 或忽略警告（不影响功能）。

### 问题: 导出超时

```
Export failed: context deadline exceeded
```

**原因**：Job 事件数量过多（> 10 万）。

**解决**：
1. 增加 API timeout 配置
2. 使用异步导出（M2 特性）

---

## 技术支持

如有问题，请：
1. 查看 `docs/evidence-package.md`
2. 运行 `aetheris debug <job_id>` 检查 job 状态
3. 提交 GitHub Issue（附上 `aetheris version` 输出）

---

## 下一步

升级完成后，建议：
1. 运行测试导出/验证
2. 更新监控告警（关注 hash 计算延迟）
3. 阅读 M2 规划（RBAC + 脱敏）
