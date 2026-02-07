package memory

import (
	"rag-platform/internal/model/llm"
)

// ShortTermMemory 短期记忆：当前对话上下文（按 session 存储最近 N 条消息）
type ShortTermMemory interface {
	GetMessages(sessionID string) []llm.Message
	Append(sessionID string, role, content string)
	Clear(sessionID string)
}

// WorkingMemory 工作记忆：当前任务中间结果（步骤的 input/output）
type WorkingMemory interface {
	GetStepResults(sessionID string) []StepResult
	SetStepResults(sessionID string, results []StepResult)
	Clear(sessionID string)
}

// StepResult 单步执行结果（供 Working 存储）
type StepResult struct {
	Tool   string `json:"tool"`
	Input  string `json:"input,omitempty"`
	Output string `json:"output"`
	Err    string `json:"error,omitempty"`
}
