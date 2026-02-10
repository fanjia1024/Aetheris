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

package ingestqueue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ingestQueuePg PostgreSQL 实现 IngestQueue，使用 ingest_tasks 表
type ingestQueuePg struct {
	pool *pgxpool.Pool
}

// NewIngestQueuePg 创建基于 PostgreSQL 的入库队列；pool 与 JobStore 共用 DSN 即可
func NewIngestQueuePg(pool *pgxpool.Pool) IngestQueue {
	return &ingestQueuePg{pool: pool}
}

// Enqueue 实现 IngestQueue
func (q *ingestQueuePg) Enqueue(ctx context.Context, payload map[string]interface{}) (taskID string, err error) {
	if payload == nil {
		return "", errors.New("payload 不能为空")
	}
	taskID = uuid.New().String()
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	_, err = q.pool.Exec(ctx,
		`INSERT INTO ingest_tasks (id, payload, status) VALUES ($1, $2, 'pending')`,
		taskID, payloadJSON,
	)
	return taskID, err
}

// ClaimOne 实现 IngestQueue；原子认领一条 pending
func (q *ingestQueuePg) ClaimOne(ctx context.Context, workerID string) (taskID string, payload map[string]interface{}, err error) {
	var id string
	var payloadBytes []byte
	err = q.pool.QueryRow(ctx,
		`WITH sel AS (
  SELECT id, payload FROM ingest_tasks WHERE status = 'pending' ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED
)
UPDATE ingest_tasks SET status = 'claimed', worker_id = $1, claimed_at = now()
FROM sel WHERE ingest_tasks.id = sel.id
RETURNING ingest_tasks.id, ingest_tasks.payload`,
		workerID,
	).Scan(&id, &payloadBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, nil
		}
		return "", nil, err
	}
	if len(payloadBytes) > 0 {
		_ = json.Unmarshal(payloadBytes, &payload)
	}
	if payload == nil {
		payload = make(map[string]interface{})
	}
	return id, payload, nil
}

// MarkCompleted 实现 IngestQueue
func (q *ingestQueuePg) MarkCompleted(ctx context.Context, taskID string, result interface{}) error {
	resultJSON, _ := json.Marshal(result)
	_, err := q.pool.Exec(ctx,
		`UPDATE ingest_tasks SET status = 'completed', result = $1, error = NULL, completed_at = now() WHERE id = $2`,
		resultJSON, taskID,
	)
	return err
}

// MarkFailed 实现 IngestQueue
func (q *ingestQueuePg) MarkFailed(ctx context.Context, taskID string, errMsg string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE ingest_tasks SET status = 'failed', error = $1, completed_at = now() WHERE id = $2`,
		errMsg, taskID,
	)
	return err
}

// GetStatus 实现 IngestQueue
func (q *ingestQueuePg) GetStatus(ctx context.Context, taskID string) (status string, result interface{}, errMsg string, completedAt interface{}, err error) {
	var st string
	var resultBytes []byte
	var errText *string
	var completed *time.Time
	err = q.pool.QueryRow(ctx,
		`SELECT status, result, error, completed_at FROM ingest_tasks WHERE id = $1`,
		taskID,
	).Scan(&st, &resultBytes, &errText, &completed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, "", nil, nil
		}
		return "", nil, "", nil, err
	}
	if len(resultBytes) > 0 {
		var r interface{}
		_ = json.Unmarshal(resultBytes, &r)
		result = r
	}
	if errText != nil {
		errMsg = *errText
	}
	if completed != nil {
		completedAt = completed
	}
	return st, result, errMsg, completedAt, nil
}
