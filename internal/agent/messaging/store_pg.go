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

package messaging

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StorePg Postgres 实现
type StorePg struct {
	pool *pgxpool.Pool
}

// NewStorePg 创建基于 PostgreSQL 的消息存储
func NewStorePg(ctx context.Context, dsn string) (*StorePg, error) {
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
	return &StorePg{pool: pool}, nil
}

// Close 关闭连接池
func (s *StorePg) Close() {
	s.pool.Close()
}

func (s *StorePg) Send(ctx context.Context, fromAgentID, toAgentID string, payload map[string]any, opts *SendOptions) (string, error) {
	id := "msg-" + uuid.New().String()
	kind := KindUser
	channel := ""
	causationID := ""
	var scheduledAt, expiresAt, deliveredAt *time.Time
	now := time.Now()
	deliveredAt = &now
	if opts != nil {
		if opts.Kind != "" {
			kind = opts.Kind
		}
		channel = opts.Channel
		causationID = opts.CausationID
		scheduledAt = opts.ScheduledAt
		expiresAt = opts.ExpiresAt
	}
	pl, _ := json.Marshal(payload)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_messages (id, from_agent_id, to_agent_id, channel, kind, payload, causation_id, scheduled_at, expires_at, created_at, delivered_at)
		 VALUES ($1, NULLIF($2,''), $3, NULLIF($4,''), $5, $6, NULLIF($7,''), $8, $9, $10, $11)`,
		id, fromAgentID, toAgentID, channel, kind, pl, causationID, scheduledAt, expiresAt, now, deliveredAt)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *StorePg) SendDelayed(ctx context.Context, toAgentID string, payload map[string]any, at time.Time, opts *SendOptions) (string, error) {
	id := "msg-" + uuid.New().String()
	kind := KindTimer
	channel := ""
	causationID := ""
	var expiresAt *time.Time
	if opts != nil {
		if opts.Kind != "" {
			kind = opts.Kind
		}
		channel = opts.Channel
		causationID = opts.CausationID
		expiresAt = opts.ExpiresAt
	}
	pl, _ := json.Marshal(payload)
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_messages (id, from_agent_id, to_agent_id, channel, kind, payload, causation_id, scheduled_at, expires_at, created_at)
		 VALUES ($1, '', $2, NULLIF($3,''), $4, $5, NULLIF($6,''), $7, $8, $9)`,
		id, toAgentID, channel, kind, pl, causationID, at, expiresAt, now)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *StorePg) PeekInbox(ctx context.Context, agentID string, limit int) ([]*Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, COALESCE(from_agent_id,''), to_agent_id, COALESCE(channel,''), kind, COALESCE(payload,'{}'::jsonb),
		 COALESCE(causation_id,''), scheduled_at, expires_at, created_at, delivered_at, COALESCE(consumed_by_job_id,''), consumed_at
		 FROM agent_messages WHERE to_agent_id = $1 AND consumed_by_job_id IS NULL
		 AND (delivered_at IS NOT NULL OR (scheduled_at IS NOT NULL AND scheduled_at <= now()))
		 ORDER BY created_at ASC LIMIT $2`,
		agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (s *StorePg) ConsumeInbox(ctx context.Context, agentID string, limit int) ([]*Message, error) {
	return s.PeekInbox(ctx, agentID, limit)
}

func (s *StorePg) MarkConsumed(ctx context.Context, messageID, jobID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agent_messages SET consumed_by_job_id = $1, consumed_at = now() WHERE id = $2`,
		jobID, messageID)
	return err
}

// ListAgentIDsWithUnconsumedMessages 返回有未消费消息的 to_agent_id 列表（design/plan.md Phase A）
func (s *StorePg) ListAgentIDsWithUnconsumedMessages(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT to_agent_id FROM agent_messages WHERE consumed_by_job_id IS NULL AND (delivered_at IS NOT NULL OR (scheduled_at IS NOT NULL AND scheduled_at <= now())) LIMIT $1`,
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func scanMessages(rows pgx.Rows) ([]*Message, error) {
	var out []*Message
	for rows.Next() {
		var id, fromID, toID, channel, kind, causationID string
		var payload []byte
		var scheduledAt, expiresAt, deliveredAt *time.Time
		var createdAt time.Time
		var consumedByJobID string
		var consumedAt *time.Time
		err := rows.Scan(&id, &fromID, &toID, &channel, &kind, &payload, &causationID, &scheduledAt, &expiresAt, &createdAt, &deliveredAt, &consumedByJobID, &consumedAt)
		if err != nil {
			return nil, err
		}
		m := &Message{
			ID:              id,
			FromAgentID:     fromID,
			ToAgentID:       toID,
			Channel:         channel,
			Kind:            kind,
			CausationID:     causationID,
			CreatedAt:       createdAt,
			ScheduledAt:     scheduledAt,
			ExpiresAt:       expiresAt,
			DeliveredAt:     deliveredAt,
			ConsumedByJobID: consumedByJobID,
			ConsumedAt:      consumedAt,
		}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &m.Payload)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
