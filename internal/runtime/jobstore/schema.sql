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
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_job_claims_expires_at ON job_claims (expires_at);

-- Job 元数据表（API 与 Worker 共享；与 internal/agent/job 语义一致）
CREATE TABLE IF NOT EXISTS jobs (
    id                   TEXT PRIMARY KEY,
    agent_id             TEXT NOT NULL,
    goal                 TEXT NOT NULL,
    status               INT  NOT NULL,
    cursor               TEXT,
    retry_count          INT  NOT NULL DEFAULT 0,
    session_id           TEXT,
    cancel_requested_at  TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 升级已有库时如缺少 cancel_requested_at 可执行：
-- ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_jobs_agent_id ON jobs (agent_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs (status);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs (created_at);

-- Agent 状态表（会话/记忆快照），供 Worker 恢复与多实例共享
CREATE TABLE IF NOT EXISTS agent_states (
    agent_id   TEXT NOT NULL,
    session_id TEXT NOT NULL,
    payload    JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, session_id)
);
