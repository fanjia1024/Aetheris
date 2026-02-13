// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package job

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// status 与 JobStatus 一致：0=Pending, 1=Running, 2=Completed, 3=Failed, 4=Cancelled, 5=Waiting, 6=Retrying
const (
	pgStatusPending   = 0
	pgStatusRunning   = 1
	pgStatusCompleted = 2
	pgStatusFailed    = 3
	pgStatusCancelled = 4
	pgStatusWaiting   = 5
	pgStatusParked    = 6 // 长时间等待，scheduler 跳过
	pgStatusRetrying  = 7
)

// JobStorePg Postgres 实现：jobs 表，供 API 与 Worker 共享
type JobStorePg struct {
	pool *pgxpool.Pool
}

// NewJobStorePg 创建基于 PostgreSQL 的 JobStore；dsn 为连接串（与 jobstore 事件表同库）
func NewJobStorePg(ctx context.Context, dsn string) (*JobStorePg, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &JobStorePg{pool: pool}, nil
}

// Close 关闭连接池
func (s *JobStorePg) Close() {
	s.pool.Close()
}

func statusToPg(s JobStatus) int {
	switch s {
	case StatusPending:
		return pgStatusPending
	case StatusRunning:
		return pgStatusRunning
	case StatusCompleted:
		return pgStatusCompleted
	case StatusFailed:
		return pgStatusFailed
	case StatusCancelled:
		return pgStatusCancelled
	case StatusWaiting:
		return pgStatusWaiting
	case StatusParked:
		return pgStatusParked
	case StatusRetrying:
		return pgStatusRetrying
	default:
		return pgStatusPending
	}
}

func pgToStatus(i int) JobStatus {
	switch i {
	case pgStatusPending:
		return StatusPending
	case pgStatusRunning:
		return StatusRunning
	case pgStatusCompleted:
		return StatusCompleted
	case pgStatusFailed:
		return StatusFailed
	case pgStatusCancelled:
		return StatusCancelled
	case pgStatusWaiting:
		return StatusWaiting
	case pgStatusParked:
		return StatusParked
	case pgStatusRetrying:
		return StatusRetrying
	default:
		return StatusPending
	}
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func capsToPg(caps []string) interface{} {
	if len(caps) == 0 {
		return nil
	}
	return strings.Join(caps, ",")
}

func pgToCaps(s *string) []string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	parts := strings.Split(*s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (s *JobStorePg) Create(ctx context.Context, j *Job) (string, error) {
	if j == nil {
		return "", errors.New("job is nil")
	}
	id := j.ID
	if id == "" {
		id = "job-" + uuid.New().String()
	}
	now := time.Now()
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	if j.UpdatedAt.IsZero() {
		j.UpdatedAt = now
	}
	tenantID := j.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO jobs (id, agent_id, tenant_id, goal, status, cursor, retry_count, session_id, cancel_requested_at, created_at, updated_at, idempotency_key, required_capabilities)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		id, j.AgentID, nullStr(tenantID), j.Goal, statusToPg(StatusPending), j.Cursor, j.RetryCount, nullStr(j.SessionID), nullTime(j.CancelRequestedAt), j.CreatedAt, j.UpdatedAt, nullStr(j.IdempotencyKey), capsToPg(j.RequiredCapabilities))
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *JobStorePg) Get(ctx context.Context, jobID string) (*Job, error) {
	var j Job
	var status int
	var cursor, sessionID, idempotencyKey, requiredCaps, tenantID *string
	var retryCount int
	var cancelRequestedAt *time.Time
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, agent_id, COALESCE(tenant_id, 'default'), goal, status, cursor, retry_count, session_id, cancel_requested_at, created_at, updated_at, idempotency_key, required_capabilities FROM jobs WHERE id = $1`,
		jobID).Scan(&j.ID, &j.AgentID, &tenantID, &j.Goal, &status, &cursor, &retryCount, &sessionID, &cancelRequestedAt, &createdAt, &updatedAt, &idempotencyKey, &requiredCaps)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if tenantID != nil {
		j.TenantID = *tenantID
	} else {
		j.TenantID = "default"
	}
	j.Status = pgToStatus(status)
	if cursor != nil {
		j.Cursor = *cursor
	}
	if sessionID != nil {
		j.SessionID = *sessionID
	}
	if cancelRequestedAt != nil {
		j.CancelRequestedAt = *cancelRequestedAt
	}
	j.RetryCount = retryCount
	j.CreatedAt = createdAt
	j.UpdatedAt = updatedAt
	if idempotencyKey != nil {
		j.IdempotencyKey = *idempotencyKey
	}
	j.RequiredCapabilities = pgToCaps(requiredCaps)
	return &j, nil
}

func (s *JobStorePg) GetByAgentAndIdempotencyKey(ctx context.Context, agentID, idempotencyKey string) (*Job, error) {
	if idempotencyKey == "" {
		return nil, nil
	}
	var j Job
	var status int
	var cursor, sessionID, key, requiredCaps, tenantID *string
	var retryCount int
	var cancelRequestedAt *time.Time
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, agent_id, COALESCE(tenant_id, 'default'), goal, status, cursor, retry_count, session_id, cancel_requested_at, created_at, updated_at, idempotency_key, required_capabilities FROM jobs WHERE agent_id = $1 AND idempotency_key = $2`,
		agentID, idempotencyKey).Scan(&j.ID, &j.AgentID, &tenantID, &j.Goal, &status, &cursor, &retryCount, &sessionID, &cancelRequestedAt, &createdAt, &updatedAt, &key, &requiredCaps)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if tenantID != nil {
		j.TenantID = *tenantID
	} else {
		j.TenantID = "default"
	}
	j.Status = pgToStatus(status)
	if cursor != nil {
		j.Cursor = *cursor
	}
	if sessionID != nil {
		j.SessionID = *sessionID
	}
	if cancelRequestedAt != nil {
		j.CancelRequestedAt = *cancelRequestedAt
	}
	if key != nil {
		j.IdempotencyKey = *key
	}
	j.RetryCount = retryCount
	j.CreatedAt = createdAt
	j.UpdatedAt = updatedAt
	j.RequiredCapabilities = pgToCaps(requiredCaps)
	return &j, nil
}

func (s *JobStorePg) ListByAgent(ctx context.Context, agentID string, tenantID string) ([]*Job, error) {
	query := `SELECT id, agent_id, COALESCE(tenant_id, 'default'), goal, status, cursor, retry_count, session_id, cancel_requested_at, created_at, updated_at, idempotency_key, required_capabilities FROM jobs WHERE agent_id = $1`
	args := []interface{}{agentID}
	if tenantID != "" {
		query += ` AND (tenant_id = $2 OR (tenant_id IS NULL AND $2 = 'default'))`
		args = append(args, tenantID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Job
	for rows.Next() {
		var j Job
		var status int
		var cursor, sessionID, idempotencyKey, requiredCaps, tid *string
		var retryCount int
		var cancelRequestedAt *time.Time
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&j.ID, &j.AgentID, &tid, &j.Goal, &status, &cursor, &retryCount, &sessionID, &cancelRequestedAt, &createdAt, &updatedAt, &idempotencyKey, &requiredCaps); err != nil {
			return nil, err
		}
		if tid != nil {
			j.TenantID = *tid
		} else {
			j.TenantID = "default"
		}
		j.Status = pgToStatus(status)
		if cursor != nil {
			j.Cursor = *cursor
		}
		if sessionID != nil {
			j.SessionID = *sessionID
		}
		if cancelRequestedAt != nil {
			j.CancelRequestedAt = *cancelRequestedAt
		}
		if idempotencyKey != nil {
			j.IdempotencyKey = *idempotencyKey
		}
		j.RetryCount = retryCount
		j.CreatedAt = createdAt
		j.UpdatedAt = updatedAt
		j.RequiredCapabilities = pgToCaps(requiredCaps)
		list = append(list, &j)
	}
	return list, rows.Err()
}

func (s *JobStorePg) UpdateStatus(ctx context.Context, jobID string, status JobStatus) error {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE jobs SET status = $1, updated_at = now() WHERE id = $2`,
		statusToPg(status), jobID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return nil // 不存在则静默
	}
	return nil
}

func (s *JobStorePg) UpdateCursor(ctx context.Context, jobID string, cursor string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE jobs SET cursor = $1, updated_at = now() WHERE id = $2`,
		cursor, jobID)
	return err
}

func (s *JobStorePg) ClaimNextPending(ctx context.Context) (*Job, error) {
	return s.ClaimNextPendingFromQueue(ctx, "")
}

func (s *JobStorePg) ClaimNextPendingFromQueue(ctx context.Context, queueClass string) (*Job, error) {
	return s.ClaimNextPendingForWorker(ctx, queueClass, nil, "")
}

func (s *JobStorePg) ClaimNextPendingForWorker(ctx context.Context, queueClass string, workerCapabilities []string, tenantID string) (*Job, error) {
	_ = queueClass // 当前 PG 未按队列过滤，与 ClaimNextPendingFromQueue 一致
	if len(workerCapabilities) == 0 {
		return s.claimNextPendingPg(ctx, tenantID)
	}
	var j Job
	var status int
	var cursor, sessionID, requiredCaps, tid *string
	var retryCount int
	var createdAt, updatedAt time.Time
	subWhere := `status = $2 AND (required_capabilities IS NULL OR trim(required_capabilities) = '' OR (SELECT bool_and(trim(c) = ANY($3)) FROM unnest(string_to_array(required_capabilities, ',')) AS c))`
	args := []interface{}{pgStatusRunning, pgStatusPending, workerCapabilities}
	if tenantID != "" {
		subWhere += ` AND (tenant_id = $4 OR (tenant_id IS NULL AND $4 = 'default'))`
		args = append(args, tenantID)
	}
	query := `UPDATE jobs SET status = $1, updated_at = now()
		 WHERE id = (SELECT id FROM jobs WHERE ` + subWhere + ` ORDER BY created_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED)
		 RETURNING id, agent_id, COALESCE(tenant_id, 'default'), goal, status, cursor, retry_count, session_id, created_at, updated_at, required_capabilities`
	err := s.pool.QueryRow(ctx, query, args...).Scan(&j.ID, &j.AgentID, &tid, &j.Goal, &status, &cursor, &retryCount, &sessionID, &createdAt, &updatedAt, &requiredCaps)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if tid != nil {
		j.TenantID = *tid
	} else {
		j.TenantID = "default"
	}
	j.Status = StatusRunning
	if cursor != nil {
		j.Cursor = *cursor
	}
	if sessionID != nil {
		j.SessionID = *sessionID
	}
	j.RetryCount = retryCount
	j.CreatedAt = createdAt
	j.UpdatedAt = updatedAt
	j.RequiredCapabilities = pgToCaps(requiredCaps)
	return &j, nil
}

func (s *JobStorePg) claimNextPendingPg(ctx context.Context, tenantID string) (*Job, error) {
	var j Job
	var status int
	var cursor, sessionID, requiredCaps, tid *string
	var retryCount int
	var createdAt, updatedAt time.Time
	query := `UPDATE jobs SET status = $1, updated_at = now()
		 WHERE id = (SELECT id FROM jobs WHERE status = $2`
	args := []interface{}{pgStatusRunning, pgStatusPending}
	if tenantID != "" {
		query += ` AND (tenant_id = $3 OR (tenant_id IS NULL AND $3 = 'default'))`
		args = append(args, tenantID)
	}
	query += ` ORDER BY created_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED)
		 RETURNING id, agent_id, COALESCE(tenant_id, 'default'), goal, status, cursor, retry_count, session_id, created_at, updated_at, required_capabilities`
	err := s.pool.QueryRow(ctx, query, args...).Scan(&j.ID, &j.AgentID, &tid, &j.Goal, &status, &cursor, &retryCount, &sessionID, &createdAt, &updatedAt, &requiredCaps)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if tid != nil {
		j.TenantID = *tid
	} else {
		j.TenantID = "default"
	}
	j.Status = StatusRunning
	if cursor != nil {
		j.Cursor = *cursor
	}
	if sessionID != nil {
		j.SessionID = *sessionID
	}
	j.RetryCount = retryCount
	j.CreatedAt = createdAt
	j.UpdatedAt = updatedAt
	j.RequiredCapabilities = pgToCaps(requiredCaps)
	return &j, nil
}

func (s *JobStorePg) Requeue(ctx context.Context, j *Job) error {
	if j == nil {
		return nil
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE jobs SET status = $1, retry_count = $2, updated_at = now() WHERE id = $3`,
		pgStatusPending, j.RetryCount+1, j.ID)
	return err
}

func (s *JobStorePg) RequestCancel(ctx context.Context, jobID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE jobs SET cancel_requested_at = now(), updated_at = now() WHERE id = $1`,
		jobID)
	return err
}

// ReclaimOrphanedJobs 将 status=Running 且 updated_at 早于 (now - olderThan) 的 Job 置回 Pending；olderThan 应 ≥ event store 的 lease_ttl
func (s *JobStorePg) ReclaimOrphanedJobs(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	cmd, err := s.pool.Exec(ctx,
		`UPDATE jobs SET status = $1, updated_at = now() WHERE status = $2 AND updated_at < $3`,
		pgStatusPending, pgStatusRunning, cutoff)
	if err != nil {
		return 0, err
	}
	return int(cmd.RowsAffected()), nil
}

// CountPending 实现 ObservabilityReader；queue 当前未按列过滤，返回全部 Pending 数
func (s *JobStorePg) CountPending(ctx context.Context, queue string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE status = $1`, pgStatusPending).Scan(&n)
	return n, err
}

// ListStuckRunningJobIDs 实现 ObservabilityReader；返回 Running 且 updated_at 早于 (now - olderThan) 的 job_id
func (s *JobStorePg) ListStuckRunningJobIDs(ctx context.Context, olderThan time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := s.pool.Query(ctx, `SELECT id FROM jobs WHERE status = $1 AND updated_at < $2`, pgStatusRunning, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CountByStatus 实现 ObservabilityReader；返回各状态 Job 数量，用于 job_state gauge（P0 SLO）
func (s *JobStorePg) CountByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT status, count(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var status int
		var n int64
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[pgToStatus(status).String()] = n
	}
	return out, rows.Err()
}
