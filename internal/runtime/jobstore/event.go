package jobstore

import "time"

// EventType 任务事件类型（事件流语义，用于重放与审计）
type EventType string

const (
	JobCreated    EventType = "job_created"
	PlanGenerated EventType = "plan_generated"
	NodeStarted   EventType = "node_started"
	NodeFinished  EventType = "node_finished"
	ToolCalled    EventType = "tool_called"
	ToolReturned  EventType = "tool_returned"
	JobCompleted  EventType = "job_completed"
	JobFailed     EventType = "job_failed"
)

// JobEvent 单条不可变事件；Job 的真实形态是事件流
type JobEvent struct {
	ID        string    // 单条事件唯一 ID，用于排序/去重；Append 时为空可由实现生成
	JobID     string    // 所属任务流 ID
	Type      EventType // 事件类型
	Payload   []byte    // JSON，由各 EventType 语义定义
	CreatedAt time.Time
}
