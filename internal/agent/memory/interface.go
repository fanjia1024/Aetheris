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
