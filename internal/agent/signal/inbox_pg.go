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

package signal

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type inboxPg struct {
	pool *pgxpool.Pool
}

// NewInboxPg 创建基于 PostgreSQL 的 SignalInbox；需先执行 schema 中的 signal_inbox 表
func NewInboxPg(pool *pgxpool.Pool) SignalInbox {
	return &inboxPg{pool: pool}
}

func (p *inboxPg) Append(ctx context.Context, jobID, correlationKey string, payload []byte) (string, error) {
	id := "sig-" + uuid.New().String()
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO signal_inbox (id, job_id, correlation_key, payload) VALUES ($1, $2, $3, $4)`,
		id, jobID, correlationKey, payload)
	return id, err
}

func (p *inboxPg) MarkAcked(ctx context.Context, jobID, id string) error {
	now := time.Now()
	_, err := p.pool.Exec(ctx,
		`UPDATE signal_inbox SET acked_at = $1 WHERE id = $2 AND job_id = $3`,
		now, id, jobID)
	return err
}

var _ SignalInbox = (*inboxPg)(nil)
