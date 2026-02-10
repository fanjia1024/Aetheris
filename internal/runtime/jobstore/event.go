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

package jobstore

import "time"

// EventType 任务事件类型（事件流语义，用于重放与审计）
type EventType string

const (
	JobCreated       EventType = "job_created"
	PlanGenerated    EventType = "plan_generated"
	NodeStarted      EventType = "node_started"
	NodeFinished     EventType = "node_finished"
	CommandEmitted   EventType = "command_emitted"
	CommandCommitted EventType = "command_committed"
	ToolCalled               EventType = "tool_called"
	ToolReturned             EventType = "tool_returned"
	ToolInvocationStarted   EventType = "tool_invocation_started"
	ToolInvocationFinished  EventType = "tool_invocation_finished"
	JobCompleted            EventType = "job_completed"
	JobFailed        EventType = "job_failed"
	JobCancelled     EventType = "job_cancelled"

	// Semantic events for Trace narrative (v0.9); see design/trace-event-schema-v0.9.md
	StateCheckpointed      EventType = "state_checkpointed"
	AgentThoughtRecorded   EventType = "agent_thought_recorded"
	DecisionMade           EventType = "decision_made"
	ToolSelected           EventType = "tool_selected"
	ToolResultSummarized   EventType = "tool_result_summarized"
	RecoveryStarted        EventType = "recovery_started"
	RecoveryCompleted      EventType = "recovery_completed"
	StepCompensated        EventType = "step_compensated"
)

// JobEvent 单条不可变事件；Job 的真实形态是事件流
type JobEvent struct {
	ID        string    // 单条事件唯一 ID，用于排序/去重；Append 时为空可由实现生成
	JobID     string    // 所属任务流 ID
	Type      EventType // 事件类型
	Payload   []byte    // JSON，由各 EventType 语义定义
	CreatedAt time.Time
}
