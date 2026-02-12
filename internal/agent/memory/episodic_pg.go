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

package memory

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EpisodicMemoryStorePg Postgres 实现
type EpisodicMemoryStorePg struct {
	pool *pgxpool.Pool
}

// NewEpisodicMemoryStorePg 创建基于 PostgreSQL 的 EpisodicMemoryStore
func NewEpisodicMemoryStorePg(ctx context.Context, dsn string) (*EpisodicMemoryStorePg, error) {
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
	return &EpisodicMemoryStorePg{pool: pool}, nil
}

// Close 关闭连接池
func (s *EpisodicMemoryStorePg) Close() {
	s.pool.Close()
}

func (s *EpisodicMemoryStorePg) Append(ctx context.Context, entry *EpisodicEntry) error {
	if entry == nil {
		return nil
	}
	if entry.ID == "" {
		entry.ID = "ep-" + uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	pl, _ := json.Marshal(entry.Payload)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_episodic_chunks (id, agent_id, session_id, job_id, summary, payload, created_at)
		 VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, $7)`,
		entry.ID, entry.AgentID, entry.SessionID, entry.JobID, entry.Summary, pl, entry.CreatedAt)
	return err
}

func (s *EpisodicMemoryStorePg) ListByAgent(ctx context.Context, agentID string, limit int) ([]*EpisodicEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.listEpisodic(ctx, `WHERE agent_id = $1 ORDER BY created_at DESC LIMIT $2`, agentID, limit)
}

func (s *EpisodicMemoryStorePg) ListBySession(ctx context.Context, agentID, sessionID string, limit int) ([]*EpisodicEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.listEpisodic(ctx, `WHERE agent_id = $1 AND session_id = $2 ORDER BY created_at DESC LIMIT $3`, agentID, sessionID, limit)
}

func (s *EpisodicMemoryStorePg) listEpisodic(ctx context.Context, where string, args ...any) ([]*EpisodicEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, COALESCE(session_id,''), COALESCE(job_id,''), COALESCE(summary,''), COALESCE(payload,'{}'::jsonb), created_at FROM agent_episodic_chunks `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*EpisodicEntry
	for rows.Next() {
		var id, aID, sessID, jobID, summary string
		var payload []byte
		var createdAt time.Time
		if err := rows.Scan(&id, &aID, &sessID, &jobID, &summary, &payload, &createdAt); err != nil {
			return nil, err
		}
		e := &EpisodicEntry{ID: id, AgentID: aID, SessionID: sessID, JobID: jobID, Summary: summary, CreatedAt: createdAt}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &e.Payload)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
