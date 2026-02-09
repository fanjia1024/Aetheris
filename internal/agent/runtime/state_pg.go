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

package runtime

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentStateStorePg Postgres 实现，与 jobs/job_events 同库，供 API 与 Worker 共享状态
type AgentStateStorePg struct {
	pool *pgxpool.Pool
}

// NewAgentStateStorePg 创建基于 PostgreSQL 的 AgentStateStore
func NewAgentStateStorePg(ctx context.Context, dsn string) (*AgentStateStorePg, error) {
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
	return &AgentStateStorePg{pool: pool}, nil
}

// Close 关闭连接池
func (s *AgentStateStorePg) Close() {
	s.pool.Close()
}

func (s *AgentStateStorePg) SaveAgentState(ctx context.Context, agentID, sessionID string, state *AgentState) error {
	if state == nil {
		return nil
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO agent_states (agent_id, session_id, payload, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (agent_id, session_id) DO UPDATE SET payload = $3, updated_at = now()`,
		agentID, sessionID, payload)
	return err
}

func (s *AgentStateStorePg) LoadAgentState(ctx context.Context, agentID, sessionID string) (*AgentState, error) {
	var payload []byte
	err := s.pool.QueryRow(ctx,
		`SELECT payload FROM agent_states WHERE agent_id = $1 AND session_id = $2`,
		agentID, sessionID).Scan(&payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var state AgentState
	if err := json.Unmarshal(payload, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
