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

package jobstore

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultLeaseDuration = 30 * time.Second
const watchPollInterval = 500 * time.Millisecond

// pgStore PostgreSQL 实现：事件表 + 租约表，实现 JobStore 接口
type pgStore struct {
	pool     *pgxpool.Pool
	leaseDur time.Duration
}

// NewPostgresStore 创建基于 PostgreSQL 的 JobStore；dsn 为连接串，leaseDuration 为租约时长（≤0 则 30s）
func NewPostgresStore(ctx context.Context, dsn string, leaseDuration time.Duration) (JobStore, error) {
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
	if leaseDuration <= 0 {
		leaseDuration = defaultLeaseDuration
	}
	return &pgStore{pool: pool, leaseDur: leaseDuration}, nil
}

// Close 关闭连接池（可选，用于优雅退出）
func (s *pgStore) Close() {
	s.pool.Close()
}

func (s *pgStore) ListEvents(ctx context.Context, jobID string) ([]JobEvent, int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, job_id, version, type, payload, created_at FROM job_events WHERE job_id = $1 ORDER BY version`,
		jobID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var events []JobEvent
	for rows.Next() {
		var e JobEvent
		var id int64
		var version int
		var typeStr string
		var payload []byte
		if err := rows.Scan(&id, &e.JobID, &version, &typeStr, &payload, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		e.ID = strconv.FormatInt(id, 10)
		e.Type = EventType(typeStr)
		_ = version // 已按 version 排序，返回值用 len(events)
		if len(payload) > 0 {
			e.Payload = make([]byte, len(payload))
			copy(e.Payload, payload)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	version := len(events)
	return events, version, nil
}

func (s *pgStore) Append(ctx context.Context, jobID string, expectedVersion int, event JobEvent) (int, error) {
	if jobID == "" {
		return 0, ErrVersionMismatch
	}
	newVersion := expectedVersion + 1
	event.JobID = jobID
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	payload := event.Payload
	if payload == nil {
		payload = []byte("null")
	}

	// CAS：仅当当前 max(version) = expectedVersion 时插入
	var currentMax *int
	err := s.pool.QueryRow(ctx, `SELECT MAX(version) FROM job_events WHERE job_id = $1`, jobID).Scan(&currentMax)
	if err != nil {
		return 0, err
	}
	cur := 0
	if currentMax != nil {
		cur = *currentMax
	}
	if cur != expectedVersion {
		return 0, ErrVersionMismatch
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO job_events (job_id, version, type, payload, created_at) VALUES ($1, $2, $3, $4, $5)`,
		jobID, newVersion, string(event.Type), payload, event.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrVersionMismatch
		}
		return 0, err
	}
	return newVersion, nil
}

func (s *pgStore) Claim(ctx context.Context, workerID string) (string, int, error) {
	now := time.Now()
	expires := now.Add(s.leaseDur)
	terminal1, terminal2, terminal3 := string(JobCompleted), string(JobFailed), string(JobCancelled)

	// 在事务中：找一条「可执行」的 job（最后事件非 terminal，且无有效租约），锁定该行后插入/更新 claim
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback(ctx)

	// 选出一个 claimable 的 (job_id, version)：最后事件类型非 completed/failed/cancelled，且无未过期 claim
	var claimedID string
	var claimedVersion int
	err = tx.QueryRow(ctx, `
		SELECT e.job_id, e.version FROM job_events e
		INNER JOIN (
			SELECT job_id, MAX(version) AS v FROM job_events GROUP BY job_id
		) m ON e.job_id = m.job_id AND e.version = m.v
		WHERE e.type NOT IN ($1, $2, $3)
		AND NOT EXISTS (
			SELECT 1 FROM job_claims c WHERE c.job_id = e.job_id AND c.expires_at > $4
		)
		ORDER BY e.created_at
		LIMIT 1
		FOR UPDATE OF e SKIP LOCKED
	`, terminal1, terminal2, terminal3, now).Scan(&claimedID, &claimedVersion)
	if err != nil {
		if errNoRows(err) {
			return "", 0, ErrNoJob
		}
		return "", 0, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO job_claims (job_id, worker_id, expires_at) VALUES ($1, $2, $3)
		 ON CONFLICT (job_id) DO UPDATE SET worker_id = $2, expires_at = $3`,
		claimedID, workerID, expires)
	if err != nil {
		return "", 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", 0, err
	}
	return claimedID, claimedVersion, nil
}

func (s *pgStore) ClaimJob(ctx context.Context, workerID string, jobID string) (int, error) {
	now := time.Now()
	expires := now.Add(s.leaseDur)
	terminal1, terminal2, terminal3 := string(JobCompleted), string(JobFailed), string(JobCancelled)

	var version int
	err := s.pool.QueryRow(ctx, `
		SELECT e.version FROM job_events e
		INNER JOIN (SELECT job_id, MAX(version) AS v FROM job_events WHERE job_id = $1 GROUP BY job_id) m ON e.job_id = m.job_id AND e.version = m.v
		WHERE e.job_id = $1 AND e.type NOT IN ($2, $3, $4)
		AND NOT EXISTS (SELECT 1 FROM job_claims c WHERE c.job_id = $1 AND c.expires_at > $5)
	`, jobID, terminal1, terminal2, terminal3, now).Scan(&version)
	if err != nil {
		if errNoRows(err) {
			return 0, ErrNoJob
		}
		return 0, err
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO job_claims (job_id, worker_id, expires_at) VALUES ($1, $2, $3)
		 ON CONFLICT (job_id) DO UPDATE SET worker_id = $2, expires_at = $3`,
		jobID, workerID, expires)
	if err != nil {
		return 0, err
	}
	return version, nil
}

func (s *pgStore) Heartbeat(ctx context.Context, workerID string, jobID string) error {
	expires := time.Now().Add(s.leaseDur)
	cmd, err := s.pool.Exec(ctx,
		`UPDATE job_claims SET expires_at = $1 WHERE job_id = $2 AND worker_id = $3`,
		expires, jobID, workerID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrClaimNotFound
	}
	return nil
}

// ListActiveWorkerIDs 返回当前有未过期租约的 worker_id 列表（供运维 CLI / API 展示）
func (s *pgStore) ListActiveWorkerIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT worker_id FROM job_claims WHERE expires_at > now() ORDER BY worker_id`)
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

func (s *pgStore) Watch(ctx context.Context, jobID string) (<-chan JobEvent, error) {
	ch := make(chan JobEvent, 16)
	_, version, err := s.ListEvents(ctx, jobID)
	if err != nil {
		return nil, err
	}
	lastVersion := version
	go func() {
		defer close(ch)
		ticker := time.NewTicker(watchPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				events, curVer, err := s.ListEvents(ctx, jobID)
				if err != nil {
					return
				}
				for i := lastVersion; i < curVer && i < len(events); i++ {
					e := events[i]
					if len(e.Payload) > 0 {
						payload := make([]byte, len(e.Payload))
						copy(payload, e.Payload)
						e.Payload = payload
					}
					select {
					case ch <- e:
					case <-ctx.Done():
						return
					default:
						// channel full, drop or keep trying
					}
				}
				lastVersion = curVer
			}
		}
	}()
	return ch, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func errNoRows(err error) bool {
	return err != nil && errors.Is(err, pgx.ErrNoRows)
}
