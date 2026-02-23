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
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckpointStorePg PostgreSQL 实现，多进程共享；需先执行 schema 中 checkpoints 表。
type CheckpointStorePg struct {
	pool *pgxpool.Pool
}

// NewCheckpointStorePg 创建基于 PostgreSQL 的 CheckpointStore。
func NewCheckpointStorePg(pool *pgxpool.Pool) CheckpointStore {
	return &CheckpointStorePg{pool: pool}
}

func cloneCheckpoint(cp *Checkpoint) *Checkpoint {
	if cp == nil {
		return nil
	}
	out := *cp
	if len(cp.TaskGraphState) > 0 {
		out.TaskGraphState = make([]byte, len(cp.TaskGraphState))
		copy(out.TaskGraphState, cp.TaskGraphState)
	}
	if len(cp.MemoryState) > 0 {
		out.MemoryState = make([]byte, len(cp.MemoryState))
		copy(out.MemoryState, cp.MemoryState)
	}
	if len(cp.PayloadResults) > 0 {
		out.PayloadResults = make([]byte, len(cp.PayloadResults))
		copy(out.PayloadResults, cp.PayloadResults)
	}
	return &out
}

// Save 实现 CheckpointStore。
func (s *CheckpointStorePg) Save(ctx context.Context, cp *Checkpoint) (string, error) {
	if cp == nil {
		return "", nil
	}
	id := cp.ID
	if id == "" {
		id = "cp-" + uuid.New().String()
		cp.ID = id
	}
	createdAt := cp.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
		cp.CreatedAt = createdAt
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO checkpoints
		 (id, agent_id, session_id, job_id, task_graph_state, memory_state, cursor_node, payload_results, created_at, updated_at)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, NULLIF($7, ''), $8, $9, now())
		 ON CONFLICT (id) DO UPDATE SET
		   agent_id = EXCLUDED.agent_id,
		   session_id = EXCLUDED.session_id,
		   job_id = EXCLUDED.job_id,
		   task_graph_state = EXCLUDED.task_graph_state,
		   memory_state = EXCLUDED.memory_state,
		   cursor_node = EXCLUDED.cursor_node,
		   payload_results = EXCLUDED.payload_results,
		   updated_at = now()`,
		id, cp.AgentID, cp.SessionID, cp.JobID, cp.TaskGraphState, cp.MemoryState, cp.CursorNode, cp.PayloadResults, createdAt,
	)
	return id, err
}

// Load 实现 CheckpointStore。
func (s *CheckpointStorePg) Load(ctx context.Context, id string) (*Checkpoint, error) {
	if id == "" {
		return nil, nil
	}
	var cp Checkpoint
	var jobID *string
	var cursorNode *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, agent_id, session_id, job_id, task_graph_state, memory_state, cursor_node, payload_results, created_at
		 FROM checkpoints
		 WHERE id = $1`,
		id,
	).Scan(&cp.ID, &cp.AgentID, &cp.SessionID, &jobID, &cp.TaskGraphState, &cp.MemoryState, &cursorNode, &cp.PayloadResults, &cp.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if jobID != nil {
		cp.JobID = *jobID
	}
	if cursorNode != nil {
		cp.CursorNode = *cursorNode
	}
	return cloneCheckpoint(&cp), nil
}

// ListByAgent 实现 CheckpointStore。
func (s *CheckpointStorePg) ListByAgent(ctx context.Context, agentID string) ([]*Checkpoint, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, session_id, job_id, task_graph_state, memory_state, cursor_node, payload_results, created_at
		 FROM checkpoints
		 WHERE agent_id = $1
		 ORDER BY created_at DESC`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Checkpoint, 0)
	for rows.Next() {
		var cp Checkpoint
		var jobID *string
		var cursorNode *string
		if err := rows.Scan(&cp.ID, &cp.AgentID, &cp.SessionID, &jobID, &cp.TaskGraphState, &cp.MemoryState, &cursorNode, &cp.PayloadResults, &cp.CreatedAt); err != nil {
			return nil, err
		}
		if jobID != nil {
			cp.JobID = *jobID
		}
		if cursorNode != nil {
			cp.CursorNode = *cursorNode
		}
		out = append(out, cloneCheckpoint(&cp))
	}
	return out, rows.Err()
}
