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

package job

import "time"

// JobStatus 任务状态
type JobStatus int

const (
	StatusPending JobStatus = iota
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusCancelled
)

func (s JobStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// Job Agent 任务实体：message 创建 Job，由 JobRunner 拉取并执行
type Job struct {
	ID        string
	AgentID   string
	Goal      string
	Status    JobStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	// Cursor 恢复游标（Checkpoint ID），恢复时从下一节点继续
	Cursor string
	// RetryCount 已重试次数，供 Scheduler 重试与 backoff
	RetryCount int
	// SessionID 关联会话，Worker 恢复时 LoadAgentState(AgentID, SessionID)；空时用 AgentID 作为 sessionID
	SessionID string
	// CancelRequestedAt 非零表示已请求取消，Worker 应取消 runCtx 并将状态置为 Cancelled
	CancelRequestedAt time.Time
	// IdempotencyKey 幂等键：POST message 时可选 Idempotency-Key header，同 Agent 下相同 key 在有效窗口内只创建一次 Job
	IdempotencyKey string
	// Priority 优先级，数值越大越先被调度；空/0 为默认
	Priority int
	// QueueClass 队列类型（realtime / default / background / heavy），Scheduler 可按队列拉取
	QueueClass string
	// RequiredCapabilities 执行该 Job 所需能力（如 llm, tool, rag）；空表示任意 Worker 可执行；Scheduler 按能力派发
	RequiredCapabilities []string
}
