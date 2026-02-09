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
}
