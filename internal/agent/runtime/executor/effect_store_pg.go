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
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EffectStorePg PostgreSQL 实现，多 Worker 共享；需先执行 schema 中 effects 表。
type EffectStorePg struct {
	pool *pgxpool.Pool
}

// NewEffectStorePg 创建基于 PostgreSQL 的 EffectStore；pool 需由调用方创建并传入。
func NewEffectStorePg(pool *pgxpool.Pool) *EffectStorePg {
	return &EffectStorePg{pool: pool}
}

// PutEffect 实现 EffectStore。
func (s *EffectStorePg) PutEffect(ctx context.Context, r *EffectRecord) error {
	if r == nil || r.JobID == "" {
		return nil
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	meta, err := json.Marshal(r.Metadata)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO effects (job_id, command_id, idempotency_key, kind, input, output, error, metadata, created_at)
		 VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6, $7, $8::jsonb, $9)
		 ON CONFLICT (job_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET
		   command_id = COALESCE(NULLIF(EXCLUDED.command_id, ''), effects.command_id),
		   kind = EXCLUDED.kind,
		   input = EXCLUDED.input,
		   output = EXCLUDED.output,
		   error = EXCLUDED.error,
		   metadata = EXCLUDED.metadata,
		   created_at = EXCLUDED.created_at`,
		r.JobID, r.CommandID, r.IdempotencyKey, r.Kind, r.Input, r.Output, r.Error, string(meta), r.CreatedAt,
	)
	if err == nil {
		return nil
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO effects (job_id, command_id, idempotency_key, kind, input, output, error, metadata, created_at)
		 VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6, $7, $8::jsonb, $9)
		 ON CONFLICT (job_id, command_id) WHERE command_id IS NOT NULL DO UPDATE SET
		   idempotency_key = COALESCE(NULLIF(EXCLUDED.idempotency_key, ''), effects.idempotency_key),
		   kind = EXCLUDED.kind,
		   input = EXCLUDED.input,
		   output = EXCLUDED.output,
		   error = EXCLUDED.error,
		   metadata = EXCLUDED.metadata,
		   created_at = EXCLUDED.created_at`,
		r.JobID, r.CommandID, r.IdempotencyKey, r.Kind, r.Input, r.Output, r.Error, string(meta), r.CreatedAt,
	)
	return err
}

func scanEffectRecord(scanFn func(dest ...any) error) (*EffectRecord, error) {
	var rec EffectRecord
	var commandID *string
	var idempotencyKey *string
	var input []byte
	var output []byte
	var metadataBytes []byte
	if err := scanFn(&rec.JobID, &commandID, &idempotencyKey, &rec.Kind, &input, &output, &rec.Error, &metadataBytes, &rec.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if commandID != nil {
		rec.CommandID = *commandID
	}
	if idempotencyKey != nil {
		rec.IdempotencyKey = *idempotencyKey
	}
	if len(input) > 0 {
		rec.Input = make([]byte, len(input))
		copy(rec.Input, input)
	}
	if len(output) > 0 {
		rec.Output = make([]byte, len(output))
		copy(rec.Output, output)
	}
	if len(metadataBytes) > 0 && string(metadataBytes) != "null" {
		_ = json.Unmarshal(metadataBytes, &rec.Metadata)
	}
	return &rec, nil
}

// GetEffectByJobAndIdempotencyKey 实现 EffectStore。
func (s *EffectStorePg) GetEffectByJobAndIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*EffectRecord, error) {
	if jobID == "" || idempotencyKey == "" {
		return nil, nil
	}
	return scanEffectRecord(func(dest ...any) error {
		return s.pool.QueryRow(ctx,
			`SELECT job_id, command_id, idempotency_key, kind, input, output, error, metadata, created_at
			 FROM effects
			 WHERE job_id = $1 AND idempotency_key = $2`,
			jobID, idempotencyKey,
		).Scan(dest...)
	})
}

// GetEffectByJobAndCommandID 实现 EffectStore。
func (s *EffectStorePg) GetEffectByJobAndCommandID(ctx context.Context, jobID, commandID string) (*EffectRecord, error) {
	if jobID == "" || commandID == "" {
		return nil, nil
	}
	return scanEffectRecord(func(dest ...any) error {
		return s.pool.QueryRow(ctx,
			`SELECT job_id, command_id, idempotency_key, kind, input, output, error, metadata, created_at
			 FROM effects
			 WHERE job_id = $1 AND command_id = $2`,
			jobID, commandID,
		).Scan(dest...)
	})
}
