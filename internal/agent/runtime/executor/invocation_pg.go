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

package executor

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ToolInvocationStorePg PostgreSQL 实现，多 Worker 共享；需先执行 schema 中 tool_invocations 表
type ToolInvocationStorePg struct {
	pool *pgxpool.Pool
}

// NewToolInvocationStorePg 创建基于 PostgreSQL 的 ToolInvocationStore；pool 需由调用方创建并传入
func NewToolInvocationStorePg(pool *pgxpool.Pool) *ToolInvocationStorePg {
	return &ToolInvocationStorePg{pool: pool}
}

// GetByJobAndIdempotencyKey 实现 ToolInvocationStore
func (s *ToolInvocationStorePg) GetByJobAndIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*ToolInvocationRecord, error) {
	var r ToolInvocationRecord
	var result []byte
	err := s.pool.QueryRow(ctx,
		`SELECT invocation_id, job_id, step_id, tool_name, args_hash, idempotency_key, status, result, committed
		 FROM tool_invocations WHERE job_id = $1 AND idempotency_key = $2`,
		jobID, idempotencyKey,
	).Scan(&r.InvocationID, &r.JobID, &r.StepID, &r.ToolName, &r.ArgsHash, &r.IdempotencyKey, &r.Status, &result, &r.Committed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if len(result) > 0 {
		r.Result = make([]byte, len(result))
		copy(r.Result, result)
	}
	return &r, nil
}

// SetStarted 实现 ToolInvocationStore；若已存在且 committed 则不再覆盖
func (s *ToolInvocationStorePg) SetStarted(ctx context.Context, r *ToolInvocationRecord) error {
	if r == nil || r.IdempotencyKey == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO tool_invocations (job_id, idempotency_key, invocation_id, step_id, tool_name, args_hash, status, committed, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, false, now())
		 ON CONFLICT (job_id, idempotency_key) DO UPDATE SET
		   invocation_id = EXCLUDED.invocation_id, step_id = EXCLUDED.step_id, tool_name = EXCLUDED.tool_name,
		   args_hash = EXCLUDED.args_hash, status = EXCLUDED.status, updated_at = now()
		 WHERE NOT tool_invocations.committed`,
		r.JobID, r.IdempotencyKey, r.InvocationID, r.StepID, r.ToolName, r.ArgsHash, r.Status,
	)
	return err
}

// SetFinished 实现 ToolInvocationStore；按 idempotency_key 更新；externalID 非空时写入 external_id 列（provenance）
func (s *ToolInvocationStorePg) SetFinished(ctx context.Context, idempotencyKey string, status string, result []byte, committed bool, externalID string) error {
	if status == ToolInvocationStatusConfirmed {
		_, err := s.pool.Exec(ctx,
			`UPDATE tool_invocations SET status = $1, result = $2, committed = $3, updated_at = now(), confirmed_at = now(), external_id = NULLIF($5, '')
			 WHERE idempotency_key = $4`,
			status, result, committed, idempotencyKey, externalID,
		)
		return err
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE tool_invocations SET status = $1, result = $2, committed = $3, updated_at = now(), external_id = NULLIF($5, '')
		 WHERE idempotency_key = $4`,
		status, result, committed, idempotencyKey, externalID,
	)
	return err
}
