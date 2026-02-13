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

package forensics

import (
	"time"
)

// QueryRequest 取证查询请求（2.0-M3）
type QueryRequest struct {
	TenantID     string    `json:"tenant_id"`
	TimeRange    TimeRange `json:"time_range"`
	ToolFilter   []string  `json:"tool_filter"`  // ["stripe*", "github*"]
	EventFilter  []string  `json:"event_filter"` // ["approve", "payment"]
	AgentFilter  []string  `json:"agent_filter"`
	StatusFilter []string  `json:"status_filter"`
	Limit        int       `json:"limit"`
	Offset       int       `json:"offset"`
}

// TimeRange 时间范围
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// QueryResponse 查询响应
type QueryResponse struct {
	Jobs       []JobSummary `json:"jobs"`
	TotalCount int          `json:"total_count"`
	Page       int          `json:"page"`
}

// JobSummary Job 摘要（用于列表）
type JobSummary struct {
	JobID      string    `json:"job_id"`
	AgentID    string    `json:"agent_id"`
	TenantID   string    `json:"tenant_id"`
	CreatedAt  time.Time `json:"created_at"`
	Status     string    `json:"status"`
	EventCount int       `json:"event_count"`
	ToolCalls  []string  `json:"tool_calls"` // 调用过的 tools
	KeyEvents  []string  `json:"key_events"` // 关键事件类型
}

// ConsistencyReport 一致性检查报告
type ConsistencyReport struct {
	JobID            string   `json:"job_id"`
	HashChainValid   bool     `json:"hash_chain_valid"`
	LedgerConsistent bool     `json:"ledger_consistent"`
	EvidenceComplete bool     `json:"evidence_complete"`
	Issues           []string `json:"issues,omitempty"`
}

// BatchExportTask 批量导出任务
type BatchExportTask struct {
	TaskID    string    `json:"task_id"`
	JobIDs    []string  `json:"job_ids"`
	Status    string    `json:"status"`   // pending | processing | completed | failed
	Progress  int       `json:"progress"` // 0-100
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ResultURL string    `json:"result_url,omitempty"`
	Error     string    `json:"error,omitempty"`
}
