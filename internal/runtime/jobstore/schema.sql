-- JobStore PostgreSQL schema for event stream and claims.
-- Run against your Postgres DB before using jobstore.type=postgres.
-- job_events.id 使用 BIGSERIAL，无需任何扩展，兼容 Alpine 等精简镜像。

CREATE TABLE IF NOT EXISTS job_events (
    id          BIGSERIAL PRIMARY KEY,
    job_id      TEXT NOT NULL,
    version     INT  NOT NULL,
    type        TEXT NOT NULL,
    payload     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (job_id, version)
);

CREATE INDEX IF NOT EXISTS idx_job_events_job_id ON job_events (job_id);
CREATE INDEX IF NOT EXISTS idx_job_events_created_at ON job_events (created_at);

CREATE TABLE IF NOT EXISTS job_claims (
    job_id      TEXT PRIMARY KEY,
    worker_id   TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    attempt_id  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_job_claims_expires_at ON job_claims (expires_at);

-- 升级已有库：为 job_claims 添加 attempt_id（design/runtime-contract.md §3.2）
ALTER TABLE job_claims ADD COLUMN IF NOT EXISTS attempt_id TEXT NOT NULL DEFAULT '';

-- Job 元数据表（API 与 Worker 共享；与 internal/agent/job 语义一致）
CREATE TABLE IF NOT EXISTS jobs (
    id                     TEXT PRIMARY KEY,
    agent_id               TEXT NOT NULL,
    goal                   TEXT NOT NULL,
    status                 INT  NOT NULL,
    cursor                 TEXT,
    retry_count            INT  NOT NULL DEFAULT 0,
    session_id             TEXT,
    cancel_requested_at    TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    idempotency_key        TEXT,
    required_capabilities  TEXT
);

-- 升级已有库时如缺少 cancel_requested_at 可执行：
-- ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ;
-- 升级已有库时如缺少 idempotency_key 可执行：
-- ALTER TABLE jobs ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
-- CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_agent_idempotency ON jobs (agent_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
-- Worker 能力调度：Job 所需能力，逗号分隔（如 'llm,tool'）；空或 NULL 表示任意 Worker 可执行（升级已有库时执行下一行）
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS required_capabilities TEXT;

CREATE INDEX IF NOT EXISTS idx_jobs_agent_id ON jobs (agent_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs (status);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs (created_at);
-- 同一 Agent 下幂等键唯一，用于 Idempotency-Key header 去重
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_agent_idempotency ON jobs (agent_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

-- Agent 状态表（会话/记忆快照），供 Worker 恢复与多实例共享
CREATE TABLE IF NOT EXISTS agent_states (
    agent_id   TEXT NOT NULL,
    session_id TEXT NOT NULL,
    payload    JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, session_id)
);

-- 工具调用账本（多 Worker 共享）：job_id + idempotency_key 唯一，Confirmation Replay 与防重放
CREATE TABLE IF NOT EXISTS tool_invocations (
    job_id          TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    invocation_id   TEXT NOT NULL,
    step_id         TEXT NOT NULL,
    tool_name       TEXT NOT NULL,
    args_hash       TEXT NOT NULL,
    status          TEXT NOT NULL,
    result          BYTEA,
    committed       BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at    TIMESTAMPTZ,
    PRIMARY KEY (job_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_tool_invocations_job_id ON tool_invocations (job_id);

-- 升级已有库时如缺少 confirmed_at 可执行：
-- ALTER TABLE tool_invocations ADD COLUMN IF NOT EXISTS confirmed_at TIMESTAMPTZ;
-- 工具调用溯源：外部系统返回的 ID（design/effect-log-and-provenance.md）
ALTER TABLE tool_invocations ADD COLUMN IF NOT EXISTS external_id TEXT;

-- 入库任务队列（API 入队、Worker 认领执行 ingest_pipeline）
CREATE TABLE IF NOT EXISTS ingest_tasks (
    id          TEXT PRIMARY KEY,
    payload     JSONB NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    worker_id   TEXT,
    result      JSONB,
    error       TEXT
);

CREATE INDEX IF NOT EXISTS idx_ingest_tasks_status ON ingest_tasks (status);
CREATE INDEX IF NOT EXISTS idx_ingest_tasks_created_at ON ingest_tasks (created_at);

-- Agent Instance 表（design/agent-instance-model.md）；2.0 第一公民身份
CREATE TABLE IF NOT EXISTS agent_instances (
    id                     TEXT PRIMARY KEY,
    tenant_id              TEXT,
    name                   TEXT,
    status                 TEXT NOT NULL DEFAULT 'idle',
    default_session_id     TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    meta                   JSONB
);
CREATE INDEX IF NOT EXISTS idx_agent_instances_tenant_id ON agent_instances (tenant_id);
CREATE INDEX IF NOT EXISTS idx_agent_instances_status ON agent_instances (status);
-- design/plan.md Phase B：Instance 当前 Job 与行为引用
ALTER TABLE agent_instances ADD COLUMN IF NOT EXISTS current_job_id TEXT;
ALTER TABLE agent_instances ADD COLUMN IF NOT EXISTS behavior_id TEXT;
CREATE INDEX IF NOT EXISTS idx_agent_instances_current_job_id ON agent_instances (current_job_id) WHERE current_job_id IS NOT NULL;

-- Agent 级消息表（design/agent-messaging-bus.md）
CREATE TABLE IF NOT EXISTS agent_messages (
    id                     TEXT PRIMARY KEY,
    from_agent_id          TEXT,
    to_agent_id            TEXT NOT NULL,
    channel                TEXT,
    kind                   TEXT NOT NULL,
    payload                JSONB,
    scheduled_at           TIMESTAMPTZ,
    expires_at             TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at           TIMESTAMPTZ,
    consumed_by_job_id     TEXT,
    consumed_at            TIMESTAMPTZ
);
-- design/plan.md Phase C：消息因果链（上游 message_id 或 job_id）
ALTER TABLE agent_messages ADD COLUMN IF NOT EXISTS causation_id TEXT;
CREATE INDEX IF NOT EXISTS idx_agent_messages_to_agent ON agent_messages (to_agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_messages_to_agent_consumed ON agent_messages (to_agent_id) WHERE consumed_by_job_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_agent_messages_scheduled ON agent_messages (scheduled_at) WHERE scheduled_at IS NOT NULL;

-- Long-Term Memory（design/durable-memory-layer.md）
CREATE TABLE IF NOT EXISTS agent_long_term_memory (
    agent_id   TEXT NOT NULL,
    namespace  TEXT NOT NULL DEFAULT '',
    key        TEXT NOT NULL,
    value      BYTEA NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, namespace, key)
);
CREATE INDEX IF NOT EXISTS idx_agent_long_term_memory_agent ON agent_long_term_memory (agent_id);

-- Episodic Memory（design/durable-memory-layer.md）
CREATE TABLE IF NOT EXISTS agent_episodic_chunks (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL,
    session_id TEXT,
    job_id     TEXT,
    summary    TEXT,
    payload    JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_agent_episodic_chunks_agent ON agent_episodic_chunks (agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_episodic_chunks_session ON agent_episodic_chunks (agent_id, session_id);

-- Signal 收件箱（2.0 at-least-once）：JobSignal 先写此处再 Append wait_completed，API 崩溃不丢 signal
CREATE TABLE IF NOT EXISTS signal_inbox (
    id               TEXT PRIMARY KEY,
    job_id           TEXT NOT NULL,
    correlation_key  TEXT NOT NULL,
    payload          JSONB NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    acked_at         TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_signal_inbox_job_id ON signal_inbox (job_id);
CREATE INDEX IF NOT EXISTS idx_signal_inbox_acked ON signal_inbox (job_id) WHERE acked_at IS NULL;

-- Job Snapshots（2.0 event stream compaction）：优化长跑 job 的 replay 性能
CREATE TABLE IF NOT EXISTS job_snapshots (
    job_id      TEXT NOT NULL,
    version     INT  NOT NULL,
    snapshot    BYTEA NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, version)
);
CREATE INDEX IF NOT EXISTS idx_job_snapshots_job_id ON job_snapshots (job_id);
CREATE INDEX IF NOT EXISTS idx_job_snapshots_created_at ON job_snapshots (created_at);

-- 2.0-M1: Proof chain fields for event tamper detection
ALTER TABLE job_events ADD COLUMN IF NOT EXISTS prev_hash TEXT DEFAULT '';
ALTER TABLE job_events ADD COLUMN IF NOT EXISTS hash TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_job_events_hash ON job_events (hash);

-- 2.0-M2: Multi-tenant and RBAC
CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    quota_json  JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Add tenant_id to jobs table
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS tenant_id TEXT;
CREATE INDEX IF NOT EXISTS idx_jobs_tenant_id ON jobs (tenant_id);

-- User-Tenant-Role mapping (2.0-M2)
CREATE TABLE IF NOT EXISTS user_roles (
    user_id    TEXT NOT NULL,
    tenant_id  TEXT NOT NULL,
    role       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, tenant_id)
);

-- Audit log for access (2.0-M2)
CREATE TABLE IF NOT EXISTS access_audit_log (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    user_id       TEXT NOT NULL,
    action        TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    success       BOOLEAN NOT NULL,
    duration_ms   INT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_access_audit_tenant ON access_audit_log (tenant_id);
CREATE INDEX IF NOT EXISTS idx_access_audit_user ON access_audit_log (user_id);
CREATE INDEX IF NOT EXISTS idx_access_audit_created ON access_audit_log (created_at);

-- Job tombstones (2.0-M2): 删除后的审计记录
CREATE TABLE IF NOT EXISTS job_tombstones (
    job_id         TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    agent_id       TEXT NOT NULL,
    deleted_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_by     TEXT NOT NULL,
    reason         TEXT NOT NULL,
    event_count    INT,
    retention_days INT,
    archive_ref    TEXT,
    metadata_json  JSONB
);
CREATE INDEX IF NOT EXISTS idx_job_tombstones_tenant ON job_tombstones (tenant_id);
CREATE INDEX IF NOT EXISTS idx_job_tombstones_deleted ON job_tombstones (deleted_at);

-- 3.0-M4: Signing keys
CREATE TABLE IF NOT EXISTS signing_keys (
    key_id       TEXT PRIMARY KEY,
    public_key   TEXT NOT NULL,
    private_key  TEXT,
    key_type     TEXT NOT NULL DEFAULT 'ed25519',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    status       TEXT NOT NULL DEFAULT 'active'
);

-- 3.0-M4: Organizations (for distributed ledger)
CREATE TABLE IF NOT EXISTS organizations (
    org_id        TEXT PRIMARY KEY,
    org_name      TEXT NOT NULL,
    public_key    TEXT NOT NULL,
    api_endpoint  TEXT NOT NULL,
    trust_level   INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3.0-M4: Cross-org sync log
CREATE TABLE IF NOT EXISTS ledger_sync_log (
    id            BIGSERIAL PRIMARY KEY,
    job_id        TEXT NOT NULL,
    source_org    TEXT NOT NULL,
    target_org    TEXT NOT NULL,
    event_count   INT NOT NULL,
    sync_status   TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ledger_sync_job ON ledger_sync_log (job_id);
CREATE INDEX IF NOT EXISTS idx_ledger_sync_created ON ledger_sync_log (created_at);
