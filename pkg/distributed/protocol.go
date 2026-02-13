// Copyright 2026 fanjia1024
// Distributed Ledger sync protocol (3.0-M4)

package distributed

import (
	"context"
	"time"
)

// LedgerSyncRequest 账本同步请求
type LedgerSyncRequest struct {
	OrgID     string  `json:"org_id"`
	JobID     string  `json:"job_id"`
	Events    []Event `json:"events"`
	Signature string  `json:"signature"`
}

// Event 事件（简化）
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Payload   []byte    `json:"payload"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
}

// LedgerSyncResponse 同步响应
type LedgerSyncResponse struct {
	Accepted    bool     `json:"accepted"`
	ConflictIDs []string `json:"conflict_ids"`
	LocalHash   string   `json:"local_hash"`
}

// SyncProtocol 同步协议接口
type SyncProtocol interface {
	Push(ctx context.Context, targetOrg string, req LedgerSyncRequest) (*LedgerSyncResponse, error)
	Pull(ctx context.Context, sourceOrg string, jobID string) ([]Event, error)
	Resolve(ctx context.Context, conflicts []string) error
}
