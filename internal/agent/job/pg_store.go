package job

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// status 与 JobStatus 一致：0=Pending, 1=Running, 2=Completed, 3=Failed, 4=Cancelled
const (
	pgStatusPending   = 0
	pgStatusRunning   = 1
	pgStatusCompleted = 2
	pgStatusFailed    = 3
	pgStatusCancelled = 4
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
	_, err := s.pool.Exec(ctx,
		`INSERT INTO jobs (id, agent_id, goal, status, cursor, retry_count, session_id, cancel_requested_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, j.AgentID, j.Goal, statusToPg(StatusPending), j.Cursor, j.RetryCount, nullStr(j.SessionID), nullTime(j.CancelRequestedAt), j.CreatedAt, j.UpdatedAt)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *JobStorePg) Get(ctx context.Context, jobID string) (*Job, error) {
	var j Job
	var status int
	var cursor, sessionID *string
	var retryCount int
	var cancelRequestedAt *time.Time
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, agent_id, goal, status, cursor, retry_count, session_id, cancel_requested_at, created_at, updated_at FROM jobs WHERE id = $1`,
		jobID).Scan(&j.ID, &j.AgentID, &j.Goal, &status, &cursor, &retryCount, &sessionID, &cancelRequestedAt, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
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
	return &j, nil
}

func (s *JobStorePg) ListByAgent(ctx context.Context, agentID string) ([]*Job, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, goal, status, cursor, retry_count, session_id, cancel_requested_at, created_at, updated_at FROM jobs WHERE agent_id = $1 ORDER BY created_at DESC`,
		agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Job
	for rows.Next() {
		var j Job
		var status int
		var cursor, sessionID *string
		var retryCount int
		var cancelRequestedAt *time.Time
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&j.ID, &j.AgentID, &j.Goal, &status, &cursor, &retryCount, &sessionID, &cancelRequestedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
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
	// 原子取一条 status=0 并置为 1
	var j Job
	var status int
	var cursor, sessionID *string
	var retryCount int
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx,
		`UPDATE jobs SET status = $1, updated_at = now()
		 WHERE id = (SELECT id FROM jobs WHERE status = $2 ORDER BY created_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED)
		 RETURNING id, agent_id, goal, status, cursor, retry_count, session_id, created_at, updated_at`,
		pgStatusRunning, pgStatusPending).Scan(&j.ID, &j.AgentID, &j.Goal, &status, &cursor, &retryCount, &sessionID, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
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
