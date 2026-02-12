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

package proof

import (
	"context"
	"time"
)

// EvidencePackage 证据包结构
type EvidencePackage struct {
	Manifest Manifest
	Events   []Event
	Ledger   []ToolInvocation
	Proof    ProofSummary
	Metadata JobMetadata
}

// Manifest 证据包清单
type Manifest struct {
	Version        string            `json:"version"` // "2.0"
	JobID          string            `json:"job_id"`
	ExportedAt     time.Time         `json:"exported_at"`
	EventCount     int               `json:"event_count"`
	LedgerCount    int               `json:"ledger_count"`
	FirstEventHash string            `json:"first_event_hash"`
	LastEventHash  string            `json:"last_event_hash"`
	FileHashes     map[string]string `json:"file_hashes"` // filename -> SHA256
	RuntimeVersion string            `json:"runtime_version"`
	SchemaVersion  string            `json:"schema_version"`
}

// ProofSummary 证明摘要
type ProofSummary struct {
	JobID           string `json:"job_id"`
	RootHash        string `json:"root_hash"` // == LastEventHash
	ChainValidated  bool   `json:"chain_validated"`
	LedgerValidated bool   `json:"ledger_validated"`
	GeneratedBy     string `json:"generated_by"`
	Signature       string `json:"signature,omitempty"` // Optional, 预留签名字段
}

// Event 事件（对应 jobstore.JobEvent）
type Event struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	Type      string    `json:"type"`
	Payload   string    `json:"payload"` // JSON string
	CreatedAt time.Time `json:"created_at"`
	PrevHash  string    `json:"prev_hash"`
	Hash      string    `json:"hash"`
}

// ToolInvocation 工具调用记录（对应 ledger）
type ToolInvocation struct {
	ID             string `json:"id"`
	JobID          string `json:"job_id"`
	IdempotencyKey string `json:"idempotency_key"`
	StepID         string `json:"step_id"`
	ToolName       string `json:"tool_name"`
	ArgsHash       string `json:"args_hash"`
	Status         string `json:"status"`
	Result         string `json:"result"` // JSON string
	Committed      bool   `json:"committed"`
	Timestamp      string `json:"timestamp"`
	ExternalID     string `json:"external_id,omitempty"`
}

// JobMetadata Job 元信息
type JobMetadata struct {
	JobID      string    `json:"job_id"`
	AgentID    string    `json:"agent_id"`
	Goal       string    `json:"goal"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	RetryCount int       `json:"retry_count"`
}

// ExportOptions 导出选项
type ExportOptions struct {
	RuntimeVersion   string
	SchemaVersion    string
	IncludeReasoning bool
	RedactionEnabled bool   // 2.0-M2: 是否启用脱敏
	RedactionSalt    string // 2.0-M2: Hash 模式的 salt
}

// VerifyResult 验证结果
type VerifyResult struct {
	OK             bool
	Errors         []string
	Events         []Event
	EventsValid    bool
	LedgerValid    bool
	HashChainValid bool
	ManifestValid  bool
}

// JobStore 接口（用于导出）
type JobStore interface {
	ListEvents(ctx context.Context, jobID string) ([]Event, error)
}

// Ledger 接口（用于导出）
type Ledger interface {
	ListToolInvocations(ctx context.Context, jobID string) ([]ToolInvocation, error)
}
