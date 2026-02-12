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

package instance

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StorePg Postgres 实现的 AgentInstanceStore；与 jobs/job_events 同库
type StorePg struct {
	pool *pgxpool.Pool
}

// NewStorePg 创建基于 PostgreSQL 的 AgentInstanceStore
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

func (s *StorePg) Get(ctx context.Context, agentID string) (*AgentInstance, error) {
	var id, tenantID, name, status, defaultSessionID string
	var createdAt, updatedAt time.Time
	var meta []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, COALESCE(tenant_id,''), COALESCE(name,''), status, COALESCE(default_session_id,''),
		 created_at, updated_at, COALESCE(meta, '{}'::jsonb)
		 FROM agent_instances WHERE id = $1`,
		agentID).Scan(&id, &tenantID, &name, &status, &defaultSessionID, &createdAt, &updatedAt, &meta)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	out := &AgentInstance{
		ID:               id,
		TenantID:         tenantID,
		Name:             name,
		Status:           status,
		DefaultSessionID: defaultSessionID,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &out.Meta)
	}
	return out, nil
}

func (s *StorePg) Create(ctx context.Context, instance *AgentInstance) error {
	if instance == nil || instance.ID == "" {
		return nil
	}
	meta, _ := json.Marshal(instance.Meta)
	now := time.Now()
	if instance.CreatedAt.IsZero() {
		instance.CreatedAt = now
	}
	if instance.UpdatedAt.IsZero() {
		instance.UpdatedAt = now
	}
	if instance.Status == "" {
		instance.Status = StatusIdle
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_instances (id, tenant_id, name, status, default_session_id, created_at, updated_at, meta)
		 VALUES ($1, NULLIF($2,''), NULLIF($3,''), $4, NULLIF($5,''), $6, $7, $8)
		 ON CONFLICT (id) DO NOTHING`,
		instance.ID, instance.TenantID, instance.Name, instance.Status, instance.DefaultSessionID,
		instance.CreatedAt, instance.UpdatedAt, meta)
	return err
}

func (s *StorePg) UpdateStatus(ctx context.Context, agentID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agent_instances SET status = $1, updated_at = now() WHERE id = $2`,
		status, agentID)
	return err
}

func (s *StorePg) Update(ctx context.Context, instance *AgentInstance) error {
	if instance == nil || instance.ID == "" {
		return nil
	}
	meta, _ := json.Marshal(instance.Meta)
	_, err := s.pool.Exec(ctx,
		`UPDATE agent_instances SET tenant_id = NULLIF($1,''), name = NULLIF($2,''), status = $3,
		 default_session_id = NULLIF($4,''), updated_at = now(), meta = $5 WHERE id = $6`,
		instance.TenantID, instance.Name, instance.Status, instance.DefaultSessionID, meta, instance.ID)
	return err
}

func (s *StorePg) ListByTenant(ctx context.Context, tenantID string, limit int) ([]*AgentInstance, error) {
	q := `SELECT id, COALESCE(tenant_id,''), COALESCE(name,''), status, COALESCE(default_session_id,''),
	      created_at, updated_at, COALESCE(meta, '{}'::jsonb) FROM agent_instances`
	args := []any{}
	if tenantID != "" {
		q += ` WHERE tenant_id = $1`
		args = append(args, tenantID)
	}
	q += ` ORDER BY updated_at DESC`
	if limit > 0 {
		if len(args) > 0 {
			q += ` LIMIT $2`
			args = append(args, limit)
		} else {
			q += ` LIMIT $1`
			args = append(args, limit)
		}
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AgentInstance
	for rows.Next() {
		var id, tID, name, status, defaultSessionID string
		var createdAt, updatedAt time.Time
		var meta []byte
		if err := rows.Scan(&id, &tID, &name, &status, &defaultSessionID, &createdAt, &updatedAt, &meta); err != nil {
			return nil, err
		}
		inst := &AgentInstance{
			ID:               id,
			TenantID:         tID,
			Name:             name,
			Status:           status,
			DefaultSessionID: defaultSessionID,
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &inst.Meta)
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}
