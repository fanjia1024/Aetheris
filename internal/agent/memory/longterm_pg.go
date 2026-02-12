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
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LongTermMemoryStorePg Postgres 实现
type LongTermMemoryStorePg struct {
	pool *pgxpool.Pool
}

// NewLongTermMemoryStorePg 创建基于 PostgreSQL 的 LongTermMemoryStore
func NewLongTermMemoryStorePg(ctx context.Context, dsn string) (*LongTermMemoryStorePg, error) {
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
	return &LongTermMemoryStorePg{pool: pool}, nil
}

// Close 关闭连接池
func (s *LongTermMemoryStorePg) Close() {
	s.pool.Close()
}

func (s *LongTermMemoryStorePg) Get(ctx context.Context, agentID, namespace, key string) ([]byte, error) {
	var value []byte
	err := s.pool.QueryRow(ctx,
		`SELECT value FROM agent_long_term_memory WHERE agent_id = $1 AND namespace = $2 AND key = $3`,
		agentID, namespace, key).Scan(&value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return value, nil
}

func (s *LongTermMemoryStorePg) Set(ctx context.Context, agentID, namespace, key string, value []byte) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_long_term_memory (agent_id, namespace, key, value, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (agent_id, namespace, key) DO UPDATE SET value = $4, updated_at = now()`,
		agentID, namespace, key, value)
	return err
}

func (s *LongTermMemoryStorePg) ListByAgent(ctx context.Context, agentID string, namespace string, limit int) ([]KeyValue, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows pgx.Rows
	var err error
	if namespace != "" {
		rows, err = s.pool.Query(ctx,
			`SELECT namespace, key, value FROM agent_long_term_memory WHERE agent_id = $1 AND namespace = $2 ORDER BY updated_at DESC LIMIT $3`,
			agentID, namespace, limit)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT namespace, key, value FROM agent_long_term_memory WHERE agent_id = $1 ORDER BY updated_at DESC LIMIT $2`,
			agentID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KeyValue
	for rows.Next() {
		var ns, k string
		var val []byte
		if err := rows.Scan(&ns, &k, &val); err != nil {
			return nil, err
		}
		out = append(out, KeyValue{Namespace: ns, Key: k, Value: val})
	}
	return out, rows.Err()
}
