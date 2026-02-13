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

package evidence

import (
	"time"
)

// EvidenceNode 证据节点（统一 schema，2.0-M3）
type EvidenceNode struct {
	Type     EvidenceType   `json:"type"`
	ID       string         `json:"id"`
	Summary  string         `json:"summary,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// EvidenceType 证据类型
type EvidenceType string

const (
	EvidenceTypeRAGDoc         EvidenceType = "rag_doc"
	EvidenceTypeToolInvocation EvidenceType = "tool_invocation"
	EvidenceTypeMemoryEntry    EvidenceType = "memory_entry"
	EvidenceTypeLLMDecision    EvidenceType = "llm_decision"
	EvidenceTypeHumanApproval  EvidenceType = "human_approval"
	EvidenceTypePolicyRule     EvidenceType = "policy_rule"
	EvidenceTypeSignal         EvidenceType = "signal"
)

// Evidence 证据集合（在 reasoning_snapshot 中）
type Evidence struct {
	Nodes       []EvidenceNode       `json:"nodes,omitempty"`
	InputKeys   []string             `json:"input_keys,omitempty"`   // 读取的 state keys（因果依赖）
	OutputKeys  []string             `json:"output_keys,omitempty"`  // 写入的 state keys（因果依赖）
	LLMDecision *LLMDecisionEvidence `json:"llm_decision,omitempty"` // LLM 决策详情
}

// LLMDecisionEvidence LLM 决策证据
type LLMDecisionEvidence struct {
	Model       string  `json:"model"`
	Provider    string  `json:"provider"`
	Temperature float64 `json:"temperature"`
	PromptHash  string  `json:"prompt_hash"`
	TokenCount  int     `json:"token_count"`
}

// DependencyGraph 因果依赖图
type DependencyGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode 图节点（对应一个 step）
type GraphNode struct {
	StepID     string    `json:"step_id"`
	NodeID     string    `json:"node_id"`
	Type       string    `json:"type"` // plan | node | tool
	Label      string    `json:"label"`
	Evidence   Evidence  `json:"evidence"`
	OutputKeys []string  `json:"output_keys"`
	Timestamp  time.Time `json:"timestamp"`
}

// GraphEdge 图边（因果关系）
type GraphEdge struct {
	From     string `json:"from"`     // source step_id
	To       string `json:"to"`       // target step_id
	Relation string `json:"relation"` // "uses_output" | "invokes_tool" | "reads_memory"
	DataKey  string `json:"data_key"` // 传递的 state key
}

// Event 事件（用于构建图）
type Event struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	Type      string    `json:"type"`
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}
